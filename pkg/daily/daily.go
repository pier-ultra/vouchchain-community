// Package daily 是「每日工作台」的纯逻辑聚合层：
// 把本周关注 / 进行中追踪 / 下一步建议，从各业务表聚合为一份概览。
// 纯规则生成，零 AI 依赖（未配模型也能跑）；AI 增强是可选的后续增强。
package daily

import (
	"context"
	"strconv"
	"time"

	"vouchchain/pkg/duban"

	"github.com/gogf/gf/v2/frame/g"
)

// Card 本周关注卡（角标数字 + 点击直达）
type Card struct {
	Key   string `json:"key"`
	Label string `json:"label"`
	Count int    `json:"count"`
	Tone  string `json:"tone"`
	URL   string `json:"url"`
}

// TrackItem 进行中追踪条目（进度：到哪一步）
type TrackItem struct {
	Kind     string `json:"kind"` // approval / invoice / contract
	Title    string `json:"title"`
	Status   string `json:"status"`
	Progress string `json:"progress"` // 如「第2/3环节」「待验真」
	URL      string `json:"url"`
}

// Insight 下一步建议（Agent 主动提示）
type Insight struct {
	Level string `json:"level"` // info / warn / danger
	Text  string `json:"text"`
}

// Overview 聚合概览
type Overview struct {
	Cards       []Card      `json:"cards"`
	Tracks      []TrackItem `json:"tracks"`
	Insights    []Insight   `json:"insights"`
	GeneratedAt string      `json:"generatedAt"`
}

// count 通用计数：len(where)==1 视为裸条件（如 "status <> 'done'"），
// len(where)>=2 视为 (字段, 值) 键值对。
func count(ctx context.Context, model string, where ...string) int {
	m := g.DB().Model(model)
	if len(where) == 1 {
		m = m.Where(where[0])
	} else if len(where) >= 2 {
		m = m.Where(where[0], where[1])
	}
	n, err := m.Count()
	if err != nil {
		return 0
	}
	return n
}

// Build 聚合当前概览
func Build(ctx context.Context) (*Overview, error) {
	o := &Overview{
		Cards:       []Card{},
		Tracks:      []TrackItem{},
		Insights:    []Insight{},
		GeneratedAt: time.Now().Format("2006-01-02 15:04"),
	}

	// ---- 本周关注卡（纯计数） ----
	contracts := count(ctx, "contract")
	approvals := count(ctx, "approval_instance", "status", "running")
	invUnverified := count(ctx, "invoice", "verify_status", "unverified")
	seals := count(ctx, "seal_apply", "status <> 'done'")
	overdue := dubanOverdue(ctx)

	o.Cards = append(o.Cards,
		Card{Key: "contracts", Label: "合同总数", Count: contracts, Tone: "#0f9d6e", URL: "/contracts"},
		Card{Key: "approvals", Label: "待审批", Count: approvals, Tone: "#4f46e5", URL: "/approvals"},
		Card{Key: "invoices", Label: "发票待验真", Count: invUnverified, Tone: "#c77c0a", URL: "/invoices"},
		Card{Key: "seals", Label: "用印待办", Count: seals, Tone: "#d64545", URL: "/seals"},
		Card{Key: "duban", Label: "督办逾期", Count: overdue, Tone: "#b91c1c", URL: "/duban"},
	)

	// ---- 进行中追踪 ----
	o.Tracks = append(o.Tracks, runningApprovals(ctx)...)
	o.Tracks = append(o.Tracks, unverifiedInvoices(ctx)...)
	o.Tracks = append(o.Tracks, activeContracts(ctx)...)

	// ---- 下一步建议（规则生成） ----
	if overdue > 0 {
		o.Insights = append(o.Insights, Insight{Level: "danger", Text: "有 " + itoa(overdue) + " 项督办逾期，建议尽快催办或办结。"})
	}
	if approvals > 0 {
		o.Insights = append(o.Insights, Insight{Level: "warn", Text: "有 " + itoa(approvals) + " 项审批正在流转，其中可能待你处理，建议去审批中心查看。"})
	}
	if invUnverified > 0 {
		o.Insights = append(o.Insights, Insight{Level: "warn", Text: "有 " + itoa(invUnverified) + " 张发票未验真，建议完成税务验真后再归档。"})
	}
	if len(o.Insights) == 0 {
		o.Insights = append(o.Insights, Insight{Level: "info", Text: "今日无待办。建议专注合同归档调度与收入确认进度，保持流程闭环。"})
	}
	return o, nil
}

func dubanOverdue(ctx context.Context) int {
	rows, err := g.DB().Model("duban_task").All()
	if err != nil {
		return 0
	}
	now := time.Now()
	n := 0
	for _, r := range rows {
		var dl time.Time
		if v := r["deadline"]; !v.IsNil() {
			dl = v.Time()
		}
		if duban.StatusOf(dl, r["status"].String(), now) == duban.StatusOverdue {
			n++
		}
	}
	return n
}

func runningApprovals(ctx context.Context) []TrackItem {
	rows, err := g.DB().Model("approval_instance").Where("status", "running").Order("id desc").Limit(5).All()
	if err != nil {
		return nil
	}
	out := make([]TrackItem, 0, len(rows))
	for _, r := range rows {
		out = append(out, TrackItem{
			Kind:     "approval",
			Title:    targetTitle(ctx, r["target_type"].String(), r["target_id"].Int64()),
			Status:   "running",
			Progress: "审批中·第" + itoa(r["current_seq"].Int()) + "环节",
			URL:      "/approvals",
		})
	}
	return out
}

func unverifiedInvoices(ctx context.Context) []TrackItem {
	rows, err := g.DB().Model("invoice").Where("verify_status", "unverified").Order("id desc").Limit(5).All()
	if err != nil {
		return nil
	}
	out := make([]TrackItem, 0, len(rows))
	for _, r := range rows {
		out = append(out, TrackItem{
			Kind:     "invoice",
			Title:    r["invoice_no"].String(),
			Status:   "unverified",
			Progress: "待验真",
			URL:      "/invoices",
		})
	}
	return out
}

func activeContracts(ctx context.Context) []TrackItem {
	rows, err := g.DB().Model("contract").Where("status", "active").Order("id desc").Limit(5).All()
	if err != nil {
		return nil
	}
	out := make([]TrackItem, 0, len(rows))
	for _, r := range rows {
		out = append(out, TrackItem{
			Kind:     "contract",
			Title:    r["title"].String(),
			Status:   "active",
			Progress: "生效中",
			URL:      "/contracts",
		})
	}
	return out
}

// targetTitle 从 target 关联表取标题（审批 target 通常是 contract_file）
func targetTitle(ctx context.Context, targetType string, targetId int64) string {
	if targetId <= 0 {
		return targetType
	}
	switch targetType {
	case "contract_file", "contract":
		if row, err := g.DB().Model(targetType).Where("id", targetId).Fields("title").One(); err == nil && !row.IsEmpty() {
			if t := row["title"].String(); t != "" {
				return t
			}
		}
	}
	return targetType + "#" + itoa64(targetId)
}

// BriefKind 晨/晚总结
const (
	KindMorning = "morning"
	KindEvening = "evening"
)

// Brief 生成晨报/晚间总结文本（纯规则；AI 增强留待后续接 pkg/agent）
func Brief(ctx context.Context, kind string) (string, error) {
	o, err := Build(ctx)
	if err != nil {
		return "", err
	}
	title := "晚间总结"
	greet := "今天的工作已告一段落。"
	if kind == KindMorning {
		title = "晨间简报"
		greet = "早上好，今天需要关注的事项如下。"
	}
	var b string
	b += "【" + title + "】\n" + greet + "\n\n"
	b += "本周关注：\n"
	for _, c := range o.Cards {
		b += "  · " + c.Label + "：" + itoa(c.Count) + "\n"
	}
	b += "\n进行中：\n"
	if len(o.Tracks) == 0 {
		b += "  · 暂无进行中事项\n"
	}
	for _, t := range o.Tracks {
		b += "  · " + t.Title + "（" + t.Progress + "）\n"
	}
	b += "\n下一步建议：\n"
	for _, ins := range o.Insights {
		b += "  · " + ins.Text + "\n"
	}
	return b, nil
}

func itoa(n int) string     { return strconv.Itoa(n) }
func itoa64(n int64) string { return strconv.FormatInt(n, 10) }
