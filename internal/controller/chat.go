package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/cloudwego/eino/schema"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/net/ghttp"

	"vouchchain/pkg/agent"
	"vouchchain/pkg/cache"
	"vouchchain/pkg/kb"
	"vouchchain/pkg/skills"
)

// AI 对话助手 HTTP 层：会话与消息落库，编排走 pkg/agent.ChatTurn。
// 会话按用户隔离（只能看自己的）；每轮工具步骤作为 tool_trace 存档可回放。
//
// 上下文注入（最近 N 条历史）经 pkg/cache 可插拔层做读缓存：DB 是权威主存储，
// KV 只缓存"最近 chatHistoryLimit 条"用于模型上下文。未配置外部 KV（cache.driver=memory）
// 时走进程内缓存，miss 仍查库回填，行为与早期直接查库一致；配置 kvrocks 后命中 KV 免查库。

const chatHistoryLimit = 20 // 送入模型的历史条数上限（token 成本与上下文平衡）

const (
	chatCacheTTL  = 2 * time.Hour // KV 上下文缓存存活（活跃会话足够；过期后重建成本一次查库）
	chatCacheName = "chat:history:"
)

var chatStore = cache.New(context.Background())

// chatHistMsg 上下文缓存条目（仅 role/content，tool_trace 不进模型上下文）
type chatHistMsg struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

func chatHistoryKey(sessionId int64) string { return chatCacheName + strconv.FormatInt(sessionId, 10) }

// loadHistory 读该会话最近 N 条历史供模型上下文。KV hit 直接用；miss 查库回填。
func loadHistory(ctx context.Context, sessionId int64) ([]*schema.Message, error) {
	if v, ok, err := chatStore.Get(ctx, chatHistoryKey(sessionId)); err == nil && ok {
		var msgs []chatHistMsg
		if json.Unmarshal([]byte(v), &msgs) == nil && len(msgs) > 0 {
			return toSchemaMessages(msgs), nil
		}
	}
	rows, err := g.DB().Model("chat_message").
		Where("session_id", sessionId).
		Order("id desc").Limit(chatHistoryLimit).All()
	if err != nil {
		return nil, err
	}
	msgs := []chatHistMsg{}
	for i := len(rows) - 1; i >= 0; i-- {
		if rows[i]["content"].String() == "" {
			continue
		}
		msgs = append(msgs, chatHistMsg{Role: rows[i]["role"].String(), Content: rows[i]["content"].String()})
	}
	if b, err := json.Marshal(msgs); err == nil {
		_ = chatStore.Set(ctx, chatHistoryKey(sessionId), string(b), chatCacheTTL)
	}
	return toSchemaMessages(msgs), nil
}

// chatHistoryTouch 写消息成功后同步更新 KV 上下文缓存（cache-aside 写；DB 已成功，KV 尽力同步）。
func chatHistoryTouch(ctx context.Context, sessionId int64, role, content string) {
	if content == "" {
		return
	}
	msgs := []chatHistMsg{}
	if v, ok, err := chatStore.Get(ctx, chatHistoryKey(sessionId)); err == nil && ok {
		_ = json.Unmarshal([]byte(v), &msgs)
	}
	msgs = append(msgs, chatHistMsg{Role: role, Content: content})
	if len(msgs) > chatHistoryLimit {
		msgs = msgs[len(msgs)-chatHistoryLimit:]
	}
	if b, err := json.Marshal(msgs); err == nil {
		_ = chatStore.Set(ctx, chatHistoryKey(sessionId), string(b), chatCacheTTL)
	}
}

func toSchemaMessages(msgs []chatHistMsg) []*schema.Message {
	out := make([]*schema.Message, 0, len(msgs))
	for _, m := range msgs {
		r := schema.User
		if m.Role == "assistant" {
			r = schema.Assistant
		}
		out = append(out, &schema.Message{Role: r, Content: m.Content})
	}
	return out
}

// ChatSend POST /api/chat {sessionId?, message}
func ChatSend(r *ghttp.Request) {
	var req struct {
		SessionId int64  `json:"sessionId"`
		Message   string `json:"message"`
	}
	if err := r.Parse(&req); err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	if req.Message == "" {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": "消息不能为空"})
		return
	}
	username := r.GetCtxVar("user").String()
	role := r.GetCtxVar("role").String()
	if role == "" {
		role = firstStr(r.GetCtxVar("roles").Strings())
	}

	// 会话：无 id 则新建（标题取消息前 20 字）；有 id 校验归属
	if req.SessionId > 0 {
		s, err := g.DB().Model("chat_session").Where("id", req.SessionId).One()
		if err != nil || s.IsEmpty() || s["username"].String() != username {
			r.Response.WriteJsonExit(g.Map{"code": 1, "message": "会话不存在或无权访问"})
			return
		}
	} else {
		res, err := g.DB().Model("chat_session").Data(g.Map{
			"username": username, "title": truncStr(req.Message, 20),
			"created_at": time.Now(), "updated_at": time.Now(),
		}).Insert()
		if err != nil {
			r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
			return
		}
		req.SessionId, _ = res.LastInsertId()
	}

	// 历史消息 → 模型上下文（最近 N 条，KV 命中免查库，miss 查库回填）
	history, err := loadHistory(r.Context(), req.SessionId)
	if err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}

	// 本轮用户消息先落库（DB 主存储）→ 同步上下文缓存
	if _, err := g.DB().Model("chat_message").Data(g.Map{
		"session_id": req.SessionId, "role": "user", "content": req.Message,
		"created_at": time.Now(),
	}).Insert(); err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	chatHistoryTouch(r.Context(), req.SessionId, "user", req.Message)

	reply, steps, err := agent.ChatTurn(r.Context(), history, req.Message, agent.ChatActor{Username: username, Role: role, SkillsPrompt: skills.MatchPrompt(context.Background(), req.Message)})
	if err != nil {
		msg := err.Error()
		code := 1
		if err == agent.ErrNoLLM {
			code = 2 // 前端识别后引导去模型配置页
			msg = "未配置 AI 模型：请到「模型配置」添加启用的 llm 配置（密钥加密存库），或设置环境变量密钥。"
		}
		r.Response.WriteJsonExit(g.Map{"code": code, "message": msg, "data": g.Map{"sessionId": req.SessionId}})
		return
	}

	// 助手回复 + 工具步骤落库（DB 主存储）→ 同步上下文缓存
	stepsJson, _ := json.Marshal(steps)
	if _, err := g.DB().Model("chat_message").Data(g.Map{
		"session_id": req.SessionId, "role": "assistant", "content": reply,
		"tool_trace": string(stepsJson), "created_at": time.Now(),
	}).Insert(); err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	chatHistoryTouch(r.Context(), req.SessionId, "assistant", reply)
	g.DB().Model("chat_session").Data(g.Map{"updated_at": time.Now()}).Where("id", req.SessionId).Update()

	r.Response.WriteJsonExit(g.Map{"code": 0, "data": g.Map{
		"sessionId": req.SessionId, "reply": reply, "steps": steps,
	}})
}

// ChatSessions GET /api/chat/sessions（当前用户的会话列表）
func ChatSessions(r *ghttp.Request) {
	username := r.GetCtxVar("user").String()
	rows, err := g.DB().Model("chat_session").
		Where("username", username).
		Fields("id,title,created_at,updated_at").
		Order("updated_at desc").Limit(50).All()
	if err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	r.Response.WriteJsonExit(g.Map{"code": 0, "data": rows})
}

// ChatMessages GET /api/chat/sessions/{id}/messages
func ChatMessages(r *ghttp.Request) {
	id := r.Get("id").Int64()
	username := r.GetCtxVar("user").String()
	s, err := g.DB().Model("chat_session").Where("id", id).One()
	if err != nil || s.IsEmpty() || s["username"].String() != username {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": "会话不存在或无权访问"})
		return
	}
	rows, err := g.DB().Model("chat_message").
		Where("session_id", id).
		Fields("id,role,content,tool_trace,created_at").
		Order("id asc").Limit(500).All()
	if err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	r.Response.WriteJsonExit(g.Map{"code": 0, "data": rows})
}

// ChatSessionRename PUT /api/chat/sessions/{id} {title}
func ChatSessionRename(r *ghttp.Request) {
	id := r.Get("id").Int64()
	var req struct {
		Title string `json:"title"`
	}
	if err := r.Parse(&req); err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	title := truncStr(req.Title, 60)
	if title == "" {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": "标题不能为空"})
		return
	}
	username := r.GetCtxVar("user").String()
	s, err := g.DB().Model("chat_session").Where("id", id).One()
	if err != nil || s.IsEmpty() || s["username"].String() != username {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": "会话不存在或无权访问"})
		return
	}
	g.DB().Model("chat_session").Data(g.Map{
		"title": title, "updated_at": time.Now(),
	}).Where("id", id).Update()
	r.Response.WriteJsonExit(g.Map{"code": 0})
}

// KBSearch GET /api/kb/search?query=xxx&topK=4 法律条款知识库检索（页面/调试用；AI 对话走工具）
func KBSearch(r *ghttp.Request) {
	query := r.Get("query").String()
	if query == "" {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": "query 必填"})
		return
	}
	res, err := kb.SearchClauses(r.Context(), query, r.Get("topK").Int())
	if err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	r.Response.WriteJsonExit(g.Map{"code": 0, "data": res})
}

// ChatSessionDelete DELETE /api/chat/sessions/{id}
func ChatSessionDelete(r *ghttp.Request) {
	id := r.Get("id").Int64()
	username := r.GetCtxVar("user").String()
	s, err := g.DB().Model("chat_session").Where("id", id).One()
	if err != nil || s.IsEmpty() || s["username"].String() != username {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": "会话不存在或无权访问"})
		return
	}
	g.DB().Model("chat_message").Where("session_id", id).Delete()
	g.DB().Model("chat_session").Where("id", id).Delete()
	_ = chatStore.Del(r.Context(), chatHistoryKey(id)) // 清上下文缓存，避免残留旧会话
	r.Response.WriteJsonExit(g.Map{"code": 0})
}

func truncStr(s string, n int) string {
	rs := []rune(s)
	if len(rs) <= n {
		return s
	}
	return string(rs[:n])
}

func firstStr(arr []string) string {
	if len(arr) > 0 {
		return arr[0]
	}
	return ""
}

// ---------- 流式对话（SSE）----------

// chatSSEEvent SSE 事件帧
func chatSSEWrite(r *ghttp.Request, event string, payload any) {
	data, _ := json.Marshal(payload)
	fmt.Fprintf(r.Response.Writer, "event: %s\ndata: %s\n\n", event, string(data))
	r.Response.Writer.Flush()
}

// ChatStream POST /api/chat/stream（SSE：event=tool 工具状态 / token 文本增量 / done 完整落库结果）
func ChatStream(r *ghttp.Request) {
	var req struct {
		SessionId int64  `json:"sessionId"`
		Message   string `json:"message" v:"required"`
	}
	if err := r.Parse(&req); err != nil || strings.TrimSpace(req.Message) == "" {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": "message 必填"})
		return
	}
	role := r.GetCtxVar("role").String()
	username := r.GetCtxVar("user").String()

	// 会话确保存在（流式端点也支持新会话）
	if req.SessionId <= 0 {
		res, err := g.DB().Model("chat_session").Data(g.Map{
			"title": truncRunes(req.Message, 24), "username": username,
			"created_at": time.Now(), "updated_at": time.Now(),
		}).Insert()
		if err != nil {
			r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
			return
		}
		req.SessionId, _ = res.LastInsertId()
	}

	// 历史（KV 命中免查库，miss 查库回填）
	history, err := loadHistory(r.Context(), req.SessionId)
	if err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	g.DB().Model("chat_message").Data(g.Map{
		"session_id": req.SessionId, "role": "user", "content": req.Message, "created_at": time.Now(),
	}).Insert()
	chatHistoryTouch(r.Context(), req.SessionId, "user", req.Message)

	// SSE 头
	r.Response.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
	r.Response.Header().Set("Cache-Control", "no-cache")
	r.Response.Header().Set("Connection", "keep-alive")
	r.Response.Header().Set("X-Accel-Buffering", "no")
	r.Response.WriteHeader(200)
	chatSSEWrite(r, "start", g.Map{"sessionId": req.SessionId})

	var finalReply string
	var lastErr error
	reply, steps, err := agent.ChatTurnStream(r.Context(), history, req.Message, agent.ChatActor{Username: username, Role: role, SkillsPrompt: skills.MatchPrompt(context.Background(), req.Message)}, func(kind agent.ChatEventKind, payload any) {
		switch kind {
		case agent.ChatEventTool:
			chatSSEWrite(r, "tool", payload)
		case agent.ChatEventToken:
			if s, ok := payload.(string); ok {
				finalReply += s
				chatSSEWrite(r, "token", g.Map{"t": s})
			}
		}
	})
	lastErr = err
	_ = lastErr

	// 落库 assistant 消息（DB 主存储）→ 同步上下文缓存
	if err == nil && reply != "" {
		g.DB().Model("chat_message").Data(g.Map{
			"session_id": req.SessionId, "role": "assistant", "content": reply,
			"tool_trace": mustJSON(steps), "created_at": time.Now(),
		}).Insert()
		chatHistoryTouch(r.Context(), req.SessionId, "assistant", reply)
	}
	// 更新会话活跃时间，保持会话列表排序与新会话一致
	g.DB().Model("chat_session").Data(g.Map{"updated_at": time.Now()}).Where("id", req.SessionId).Update()
	if err != nil {
		chatSSEWrite(r, "error", g.Map{"message": err.Error()})
		return
	}
	chatSSEWrite(r, "done", g.Map{"sessionId": req.SessionId, "reply": reply, "steps": steps})
}

func mustJSON(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

func truncRunes(s string, n int) string {
	r := []rune(strings.TrimSpace(s))
	if len(r) <= n {
		return string(r)
	}
	return string(r[:n])
}
