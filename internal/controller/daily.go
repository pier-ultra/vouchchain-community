package controller

import (
	"context"
	"strings"
	"sync"
	"time"

	"vouchchain/pkg/agent"
	"vouchchain/pkg/daily"
	"vouchchain/pkg/ws"

	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/net/ghttp"
	"github.com/gogf/gf/v2/os/glog"
)

// ---------- 每日工作台：概览 + 晨/晚总结定时推送 ----------
// 纯规则聚合（pkg/daily），不依赖 AI 模型；AI 增强是后续可选增强。
// 主动推送走现有 ws hub（在线页面实时可见）；离线场景后续接 uni-push。

var (
	briefMu   sync.Mutex
	briefDone = map[string]string{} // kind -> date，防同日重复生成

	lastBriefMu sync.Mutex
	lastBrief   struct{ Kind, Text, At string }
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
// 启动 10s 后先扫一次；每 60s 检查；用 briefDone 防同日重复（进程内，单实例自托管）。
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
	briefMu.Lock()
	if briefDone[kind] == date {
		briefMu.Unlock()
		return
	}
	briefDone[kind] = date
	briefMu.Unlock()

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
	lastBriefMu.Lock()
	defer lastBriefMu.Unlock()
	lastBrief.Kind, lastBrief.Text, lastBrief.At = kind, text, time.Now().Format("2006-01-02 15:04")
}

func getLastBrief() g.Map {
	lastBriefMu.Lock()
	defer lastBriefMu.Unlock()
	if lastBrief.Text == "" {
		return nil
	}
	return g.Map{"kind": lastBrief.Kind, "text": lastBrief.Text, "at": lastBrief.At}
}
