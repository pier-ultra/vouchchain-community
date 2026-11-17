package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/cloudwego/eino-ext/components/model/openai"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

// AI 对话编排器（对话即操作）：带工具循环的 agent loop。
// 模型每次可发起多个 tool calls，执行结果以 tool 消息回喂，直到产出纯文本回复
// 或达到轮次上限。写操作在工具内部以对话用户身份落库+审计（见 chat_tools.go）。

var ErrNoLLM = errors.New("未配置 AI 模型（模型配置页无启用的 llm 配置，且未设置环境变量密钥）")

const chatMaxRounds = 8

const chatSystemPromptTpl = `你是 VouchChain（合同与财务协同系统）的 AI 助手，嵌入在网页右侧对话栏里，通过工具帮用户查询和操作系统。

可用能力分两类：
- 查询：合同列表/详情、相对方风险画像、发票、义务、审批实例
- 操作：对合同文件发起审批（start_contract_approval）、审批同意/驳回（act_approval）

行为规则：
1. 用户提到合同/发票/相对方但没给 id 时，先用查询工具按名称找到具体 id，再执行操作；查不到就如实说明，不要编造 id。
2. 操作完成后用工具返回的结果如实回复（实例号、状态、跳过节点数），不要夸大。
3. 工具返回错误时把原因讲清楚，并给出下一步建议（例如没有模板时提示先去审批中心建模板）。
4. 相对方画像只基于内部合同库，主动向用户说明该边界，不臆测工商/司法数据。
5. 回复一律用简体中文，简洁分条，关键数字保留。

当前用户：%s（角色 %s）。所有以该用户身份执行的操作都会审计留痕。`

// ChatTools 全部注册工具（供 WithTools 与执行两处使用同一份清单）
func ChatTools() []ChatTool {
	return []ChatTool{
		{
			Name: "query_contracts",
			Desc: "按关键词/相对方/状态查询合同列表（返回 id、标题、金额、状态）",
			Params: map[string]*schema.ParameterInfo{
				"keyword":      {Type: schema.String, Desc: "标题或相对方关键词，可空"},
				"counterParty": {Type: schema.String, Desc: "相对方名称（模糊），可空"},
				"status":       {Type: schema.String, Desc: "合同状态如 active/done，可空"},
			},
			Fn: toolQueryContracts,
		},
		{
			Name: "get_contract",
			Desc: "查合同详情：金额/状态/项目，关联合同文件与审批状态、义务概览",
			Params: map[string]*schema.ParameterInfo{
				"contractId": {Type: schema.Integer, Desc: "合同 id（query_contracts 返回的 #id）", Required: true},
			},
			Fn: toolGetContract,
		},
		{
			Name: "get_party_profile",
			Desc: "相对方风险画像：合同份数、累计金额、义务完成率、逾期数、最近签署（仅内部合同库）",
			Params: map[string]*schema.ParameterInfo{
				"name": {Type: schema.String, Desc: "相对方名称（模糊匹配）", Required: true},
			},
			Fn: toolGetPartyProfile,
		},
		{
			Name: "query_invoices",
			Desc: "查发票列表：发票号、销方、价税合计、验真状态",
			Params: map[string]*schema.ParameterInfo{
				"keyword":      {Type: schema.String, Desc: "发票号或销/购方关键词，可空"},
				"verifyStatus": {Type: schema.String, Desc: "验真状态如 passed/failed/pending，可空"},
			},
			Fn: toolQueryInvoices,
		},
		{
			Name: "query_obligations",
			Desc: "查合同义务列表：类型、到期日、状态",
			Params: map[string]*schema.ParameterInfo{
				"keyword": {Type: schema.String, Desc: "义务描述关键词，可空"},
				"status":  {Type: schema.String, Desc: "状态如 pending/done/overdue，可空"},
			},
			Fn: toolQueryObligations,
		},
		{
			Name: "query_approvals",
			Desc: "查审批实例列表：对象、状态、当前节点、发起人",
			Params: map[string]*schema.ParameterInfo{
				"status": {Type: schema.String, Desc: "running/approved/rejected/revoked，可空"},
				"mine":   {Type: schema.Boolean, Desc: "true=只看我发起的", Required: false},
			},
			Fn: toolQueryApprovals,
		},
		{
			Name: "start_contract_approval",
			Desc: "对合同文件发起审批流（写操作）。需要 contractFileId（合同文件 id，get_contract 返回 文件#N）；模板名可空默认第一个模板",
			Params: map[string]*schema.ParameterInfo{
				"contractFileId": {Type: schema.Integer, Desc: "合同文件 id", Required: true},
				"templateName":   {Type: schema.String, Desc: "审批模板名（模糊），可空=第一个模板"},
			},
			Fn: toolStartContractApproval,
		},
		{
			Name: "act_approval",
			Desc: "对审批实例执行同意/驳回（写操作）。执行者必须是当前节点审批人，否则失败",
			Params: map[string]*schema.ParameterInfo{
				"instanceId": {Type: schema.Integer, Desc: "审批实例 id", Required: true},
				"action":     {Type: schema.String, Desc: "approve=同意 / reject=驳回", Required: true},
				"comment":    {Type: schema.String, Desc: "审批意见（驳回建议必填）"},
			},
			Fn: toolActApproval,
		},
		{
			Name: "search_legal_kb",
			Desc: "法律条款知识库检索（RAG）：按问题/条款内容查相关法条（民法典合同编为主），返回条文原文与出处；用于回答法律问题或给审查意见找依据",
			Params: map[string]*schema.ParameterInfo{
				"query": {Type: schema.String, Desc: "法律问题或条款内容关键词", Required: true},
				"topK":  {Type: schema.Integer, Desc: "返回条数（默认 4，最多 10）"},
			},
			Fn: toolSearchLegalKB,
		},
	}
}

func toolInfos() []*schema.ToolInfo {
	tools := ChatTools()
	infos := make([]*schema.ToolInfo, 0, len(tools))
	for _, t := range tools {
		infos = append(infos, &schema.ToolInfo{
			Name:        t.Name,
			Desc:        t.Desc,
			ParamsOneOf: schema.NewParamsOneOfByParams(t.Params),
		})
	}
	return infos
}

// retryGen 429 限流退避重试（与 generateWithRetry 同策略：5/15/30s，共 4 次）
func retryGen(ctx context.Context, fn func() (*schema.Message, error)) (*schema.Message, error) {
	delays := []time.Duration{5, 15, 30}
	for attempt := 0; ; attempt++ {
		resp, err := fn()
		if err == nil {
			return resp, nil
		}
		if !strings.Contains(err.Error(), "429") || attempt >= len(delays) {
			return nil, err
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(delays[attempt] * time.Second):
		}
	}
}

// ChatTurn 执行一轮对话：返回最终回复与工具执行步骤（前端展示「正在做什么」）。
func ChatTurn(ctx context.Context, history []*schema.Message, userMsg string, actor ChatActor) (string, []StepTrace, error) {
	apiKey, baseUrl, modelName := resolveLLMConfig()
	if apiKey == "" {
		return "", nil, ErrNoLLM
	}
	base, err := openaiNewChatModel(ctx, baseUrl, modelName, apiKey)
	if err != nil {
		return "", nil, fmt.Errorf("初始化模型失败: %w", err)
	}
	var cm model.ToolCallingChatModel
	cm, err = base.WithTools(toolInfos())
	if err != nil {
		return "", nil, fmt.Errorf("注册工具失败: %w", err)
	}

	tools := ChatTools()
	byName := map[string]ChatTool{}
	for _, t := range tools {
		byName[t.Name] = t
	}

	msgs := []*schema.Message{
		{Role: schema.System, Content: fmt.Sprintf(chatSystemPromptTpl, actor.Username, actor.Role) + actor.SkillsPrompt},
	}
	msgs = append(msgs, history...)
	msgs = append(msgs, &schema.Message{Role: schema.User, Content: userMsg})

	steps := []StepTrace{}
	for round := 0; round < chatMaxRounds; round++ {
		resp, err := retryGen(ctx, func() (*schema.Message, error) { return cm.Generate(ctx, msgs) })
		if err != nil {
			return "", steps, fmt.Errorf("模型调用失败: %w", err)
		}
		if len(resp.ToolCalls) == 0 {
			return strings.TrimSpace(resp.Content), steps, nil
		}
		msgs = append(msgs, resp) // 助手消息（含 tool calls）入历史
		for _, call := range resp.ToolCalls {
			trace, result := execTool(ctx, byName, call, actor)
			steps = append(steps, trace)
			msgs = append(msgs, &schema.Message{Role: schema.Tool, Content: result, ToolCallID: call.ID})
		}
	}
	return "", steps, errors.New("工具调用轮次超限，请把问题拆小一点")
}

func execTool(ctx context.Context, byName map[string]ChatTool, call schema.ToolCall, actor ChatActor) (StepTrace, string) {
	trace := StepTrace{Tool: call.Function.Name, Args: trunc(call.Function.Arguments, 200)}
	tool, ok := byName[call.Function.Name]
	if !ok {
		trace.Summary = "未知工具"
		return trace, "错误：未知工具 " + call.Function.Name
	}
	var args map[string]any
	if s := strings.TrimSpace(call.Function.Arguments); s != "" {
		if err := json.Unmarshal([]byte(s), &args); err != nil {
			trace.Summary = "参数解析失败"
			return trace, "错误：工具参数不是合法 JSON: " + err.Error()
		}
	}
	var (
		result string
		err    error
	)
	func() {
		defer func() {
			if r := recover(); r != nil {
				err = fmt.Errorf("工具执行异常: %v", r)
			}
		}()
		result, err = tool.Fn(ctx, args, actor)
	}()
	if err != nil {
		trace.Summary = "失败：" + trunc(err.Error(), 80)
		return trace, "工具执行失败: " + err.Error()
	}
	trace.OK = true
	trace.Summary = trunc(firstLine(result), 90)
	return trace, result
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i > 0 {
		return s[:i]
	}
	return s
}

// ---------- 流式对话（SSE）----------
// 设计：工具循环保持同步（每步通过 onEvent 推工具状态），最终回复用无工具模型 Stream 逐 token 推送。
// 代价：最终回复多一次模型调用；收益：真正的逐 token 输出 + 工具 call 聚合零风险。

// ChatEventKind 流式事件类型
//   tool  - payload=StepTrace（工具执行状态）
//   token - payload=string（回复文本增量）
type ChatEventKind string

const (
	ChatEventTool  ChatEventKind = "tool"
	ChatEventToken ChatEventKind = "token"
)

// ChatTurnStream 流式版对话：onEvent 回调推送工具步骤与文本增量，返回最终完整回复。
func ChatTurnStream(ctx context.Context, history []*schema.Message, userMsg string, actor ChatActor, onEvent func(kind ChatEventKind, payload any)) (string, []StepTrace, error) {
	apiKey, baseUrl, modelName := resolveLLMConfig()
	if apiKey == "" {
		return "", nil, ErrNoLLM
	}
	base, err := openaiNewChatModel(ctx, baseUrl, modelName, apiKey)
	if err != nil {
		return "", nil, fmt.Errorf("初始化模型失败: %w", err)
	}
	var cm model.ToolCallingChatModel
	cm, err = base.WithTools(toolInfos())
	if err != nil {
		return "", nil, fmt.Errorf("注册工具失败: %w", err)
	}

	tools := ChatTools()
	byName := map[string]ChatTool{}
	for _, t := range tools {
		byName[t.Name] = t
	}

	msgs := []*schema.Message{
		{Role: schema.System, Content: fmt.Sprintf(chatSystemPromptTpl, actor.Username, actor.Role) + actor.SkillsPrompt},
	}
	msgs = append(msgs, history...)
	msgs = append(msgs, &schema.Message{Role: schema.User, Content: userMsg})

	steps := []StepTrace{}
	for round := 0; round < chatMaxRounds; round++ {
		resp, err := retryGen(ctx, func() (*schema.Message, error) { return cm.Generate(ctx, msgs) })
		if err != nil {
			return "", steps, fmt.Errorf("模型调用失败: %w", err)
		}
		if len(resp.ToolCalls) == 0 {
			// 最终回复：用无工具模型重新流式生成（逐 token 推送）。
			// 部分国产兼容端点不支持 SSE（base.Stream 建立失败），此时把已拿到的
			// 非流式结果 resp.Content 作为兜底整段推送，避免前端收到空回复。
			final := streamFinalAnswer(ctx, base, msgs, onEvent, strings.TrimSpace(resp.Content))
			return final, steps, nil
		}
		msgs = append(msgs, resp)
		for _, call := range resp.ToolCalls {
			trace, result := execTool(ctx, byName, call, actor)
			steps = append(steps, trace)
			if onEvent != nil {
				onEvent(ChatEventTool, trace)
			}
			msgs = append(msgs, &schema.Message{Role: schema.Tool, Content: result, ToolCallID: call.ID})
		}
	}
	return "", steps, errors.New("工具调用轮次超限，请把问题拆小一点")
}

// streamFinalAnswer 最终回复流式生成（无工具，429 退避仅在建立流时生效）
// fallback 是非流式已生成的完整回复，当端点不支持流式/流中断时整段推送兜底
func streamFinalAnswer(ctx context.Context, base *openai.ChatModel, msgs []*schema.Message, onEvent func(kind ChatEventKind, payload any), fallback string) string {
	var sb strings.Builder
	// 结束时：优先返回已收到的流式内容；若为空则用非流式兜底，并补发一个 token 事件
	finish := func() string {
		out := sb.String()
		if out == "" && fallback != "" {
			out = fallback
			if onEvent != nil {
				onEvent(ChatEventToken, fallback)
			}
		}
		return out
	}
	for attempt := 0; ; attempt++ {
		stream, err := base.Stream(ctx, msgs)
		if err != nil {
			if strings.Contains(err.Error(), "429") && attempt < 3 {
				select {
				case <-ctx.Done():
					return finish()
				case <-time.After([]time.Duration{5, 15, 30}[attempt] * time.Second):
					continue
				}
			}
			return finish() // 端点不支持流式：整段推送非流式兜底
		}
		for {
			msg, rerr := stream.Recv()
			if rerr == io.EOF {
				return finish()
			}
			if rerr != nil {
				return finish() // 中途断流：已收到的部分 + 必要时兜底
			}
			if msg.Content != "" {
				sb.WriteString(msg.Content)
				if onEvent != nil {
					onEvent(ChatEventToken, msg.Content)
				}
			}
		}
	}
}
