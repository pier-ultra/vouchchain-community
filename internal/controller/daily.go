package controller

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"vouchchain/pkg/agent"
	"vouchchain/pkg/cache"
	"vouchchain/pkg/daily"
	"vouchchain/pkg/ws"

	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/net/ghttp"
	"github.com/gogf/gf/v2/os/glog"
)

// ---------- 每日工作台：概览 + 晨/晚总结定时推送 ----------
// 纯规则聚合（pkg/daily），不依赖 AI 模型；AI 增强是后续可选增强。
// 主动推送走现有 ws hub（在线页面实时可见）；离线场景后续接 uni-push。
//
// 单飞去重与"最近一次简报"经 pkg/cache 可插拔层：默认 memory（进程内，行为与早期内联等价）；
// 配置 cache.driver=kvrocks 时切到外部 Kvrocks，多实例下保证同日只发一次、最近简报跨实例共享。

var (
	briefStore   = cache.New(context.Background())
	briefDoneKey = "daily:briefdone:"  // + kind + ":" + date
	lastBriefKey = "daily:lastbrief:latest"
)

// DailyOverview 聚合概览（本周关注 + 进行中追踪 + 下一步建议 + 最近总结）
func DailyOverview(r *ghttp.Request) {
	o, err := daily.Build(r.Context())
	if err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	r.Response.WriteJsonExit(g.Map{"code": 0, "data": g.Map{"overview": o, "lastBrief": getLastBrief()}})
}

// DailyBrief 手动/主动生成一次晨报或晚间总结，并 WS 广播给在线页面
func DailyBrief(r *ghttp.Request) {
	kind := r.Get("kind").String()
	if kind == "" {
		kind = daily.KindMorning
	}
	if kind != daily.KindMorning && kind != daily.KindEvening {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": "kind 只能是 morning / evening"})
		return
	}
	text, err := generateBrief(r.Context(), kind)
	if err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	setLastBrief(kind, text)
	broadcastBrief(kind, text)
	r.Response.WriteJsonExit(g.Map{"code": 0, "data": g.Map{"kind": kind, "text": text}})
}

// generateBrief 生成晨/晚总结：纯规则文本（pkg/daily）→ AI 增强（未配模型/调用失败自动降级纯规则）。
func generateBrief(ctx context.Context, kind string) (string, error) {
	ruleText, err := daily.Brief(ctx, kind)
	if err != nil {
		return "", err
	}
	return enhanceBrief(ctx, kind, ruleText), nil
}

// enhanceBrief AI 增强：用 pkg/agent 单轮生成把纯规则简报润色成自然总结。
// 未配置 LLM（model_config 无启用 llm 且无 AGNES_API_KEY）→ 原样返回纯规则文本；
// 模型调用失败/返回空 → 记录日志并降级返回纯规则文本，不阻断定时推送。
// 红线：只基于给定内容润色，严禁 AI 编造任何数据/事实/结论（保留全部关键数字）。
func enhanceBrief(ctx context.Context, kind string, ruleText string) string {
	apiKey, baseUrl, model := agent.ResolveLLMConfig()
	if apiKey == "" {
		return ruleText
	}
	chatModel, err := agent.NewOpenAIChatModel(ctx, baseUrl, model, apiKey)
	if err != nil {
		glog.Warningf(ctx, "[daily] 初始化 AI 增强模型失败，降级纯规则：%v", err)
		return ruleText
	}
	label := "晚间总结"
	if kind == daily.KindMorning {
		label = "晨间简报"
	}
	content, err := agent.GenerateOnce(ctx, chatModel,
		"你是一位严谨的运营助理。请把下面的结构化工作简报润色成一段自然、简洁、重点突出的中文"+label+"。\n"+
			"要求：只基于给定内容重组与润色，严禁编造或补充任何数据、数量、事实、结论；保留全部关键数字；语气务实不夸张；不要使用 Markdown 代码块标记。\n\n【原始简报】\n"+ruleText)
	if err != nil {
		glog.Warningf(ctx, "[daily] AI 增强失败，降级纯规则：%v", err)
		return ruleText
	}
	if out := strings.TrimSpace(content); out != "" {
		return out
	}
	return ruleText
}

// StartDailyScheduler 定时任务：每日 08:00 晨间简报 / 21:00 晚间总结。
// 启动 10s 后先扫一次；每 60s 检查；用 SetNX 防同日重复（memory 进程内 / kvrocks 跨实例）。
func StartDailyScheduler() {
	go func() {
		time.Sleep(10 * time.Second)
		scanDaily()
		ticker := time.NewTicker(60 * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			scanDaily()
		}
	}()
}

func scanDaily() {
	now := time.Now()
	if now.Hour() == 8 && now.Minute() < 2 {
		runBriefOnce(daily.KindMorning, now)
	}
	if now.Hour() == 21 && now.Minute() < 2 {
		runBriefOnce(daily.KindEvening, now)
	}
}

func runBriefOnce(kind string, now time.Time) {
	date := now.Format("2006-01-02")
	// SetNX：仅当该 kind+date 的 key 不存在才写入并返回 true（要生成）；已存在跳过（防同日重复）。
	// key 带日期，天然按天隔离；TTL 24h 兜底清理。
	ok, err := briefStore.SetNX(context.Background(), briefDoneKey+kind+":"+date, date, 24*time.Hour)
	if err != nil || !ok {
		return
	}
	text, err := generateBrief(context.Background(), kind)
	if err != nil {
		return
	}
	setLastBrief(kind, text)
	broadcastBrief(kind, text)
}

func broadcastBrief(kind, text string) {
	ws.Broadcast(ws.Event{
		Type: "daily_brief",
		Data: g.Map{"kind": kind, "text": text, "at": time.Now().Format("2006-01-02 15:04")},
	})
}

func setLastBrief(kind, text string) {
	b, _ := json.Marshal(g.Map{"kind": kind, "text": text, "at": time.Now().Format("2006-01-02 15:04")})
	_ = briefStore.Set(context.Background(), lastBriefKey, string(b), 7*24*time.Hour)
}

func getLastBrief() g.Map {
	v, ok, err := briefStore.Get(context.Background(), lastBriefKey)
	if err != nil || !ok || v == "" {
		return nil
	}
	var m g.Map
	if json.Unmarshal([]byte(v), &m) != nil {
		return nil
	}
	return m
}
