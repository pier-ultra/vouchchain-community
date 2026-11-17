// Package partyprofile：US-C8 相对方风险画像的纯逻辑层。
//
// 诚实边界：只基于本系统内已登记的合同台账、履约义务、条款复核结果做「内部行为画像」，
// 不接外部工商/司法征信，也不宣称是法律意义上的信用评级。样本少时明确标注「样本不足」。
package partyprofile

import (
	"sort"
	"strings"
	"time"
)

type ContractRow struct {
	ID           int64   `json:"id"`
	CounterParty string  `json:"counterParty"`
	Title        string  `json:"title"`
	Amount       float64 `json:"amount"`
	Status       string  `json:"status"`
	SignDate     string  `json:"signDate"`
}

type ObligationRow struct {
	ID           int64  `json:"id"`
	CounterParty string `json:"counterParty"`
	Status       string `json:"status"`
	DueDate      string `json:"dueDate"`
	CompletedAt  string `json:"completedAt"`
}

type ClauseRow struct {
	ID           int64  `json:"id"`
	CounterParty string `json:"counterParty"`
	Type         string `json:"type"`
	Risk         string `json:"risk"`
	Summary      string `json:"summary"`
}

type CategoryCount struct {
	Type  string `json:"type"`
	Count int    `json:"count"`
}

type Profile struct {
	CounterParty        string          `json:"counterParty"`
	ContractCount       int             `json:"contractCount"`
	ActiveCount         int             `json:"activeCount"`
	RejectedCount       int             `json:"rejectedCount"`
	TotalAmount         float64         `json:"totalAmount"`
	FirstSignDate       string          `json:"firstSignDate"`
	LastSignDate        string          `json:"lastSignDate"`
	ObligationTotal     int             `json:"obligationTotal"`
	ObligationDone      int             `json:"obligationDone"`
	OnTimeCount         int             `json:"onTimeCount"`
	LateDoneCount       int             `json:"lateDoneCount"`
	OverduePendingCount int             `json:"overduePendingCount"`
	FulfillRate         float64         `json:"fulfillRate"` // 已完结义务中按时完成占比
	DisputeClauseCount  int             `json:"disputeClauseCount"`
	HighRiskClauseCount int             `json:"highRiskClauseCount"`
	ClauseCategories    []CategoryCount `json:"clauseCategories"`
	RiskScore           int             `json:"riskScore"` // 100=最稳，越低越需关注
	RiskLevel           string          `json:"riskLevel"` // low / medium / high
	Tags                []string        `json:"tags"`
	Reasons             []string        `json:"reasons"`
	SampleWarning       string          `json:"sampleWarning"`
	Contracts           []ContractRow   `json:"contracts"`
}

func day(s string) string {
	if len(s) >= 10 {
		return s[:10]
	}
	return s
}

func disputeClause(c ClauseRow) bool {
	t := c.Type
	risk := strings.ToLower(c.Risk)
	for _, kw := range []string{"违约", "争议", "赔偿", "责任", "解除", "罚"} {
		if strings.Contains(t, kw) {
			return true
		}
	}
	return risk == "high"
}

func clamp(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// Build 聚合一个相对方的全部内部数据并给出画像。now 由调用方注入，便于单测固定时间。
func Build(name string, contracts []ContractRow, obligations []ObligationRow, clauses []ClauseRow, now time.Time) Profile {
	today := now.Format("2006-01-02")
	p := Profile{
		CounterParty: name, RiskScore: 100, RiskLevel: "low",
		Tags: []string{}, Reasons: []string{},
		ClauseCategories: []CategoryCount{}, Contracts: []ContractRow{},
	}
	for _, c := range contracts {
		p.ContractCount++
		p.TotalAmount += c.Amount
		if strings.EqualFold(c.Status, "active") || strings.EqualFold(c.Status, "signed") || strings.EqualFold(c.Status, "approved") {
			p.ActiveCount++
		}
		if strings.EqualFold(c.Status, "rejected") || strings.EqualFold(c.Status, "terminated") || strings.EqualFold(c.Status, "废止") {
			p.RejectedCount++
		}
		d := day(c.SignDate)
		if d != "" {
			if p.FirstSignDate == "" || d < p.FirstSignDate {
				p.FirstSignDate = d
			}
			if p.LastSignDate == "" || d > p.LastSignDate {
				p.LastSignDate = d
			}
		}
		p.Contracts = append(p.Contracts, c)
	}
	sort.Slice(p.Contracts, func(i, j int) bool { return p.Contracts[i].ID > p.Contracts[j].ID })

	for _, o := range obligations {
		p.ObligationTotal++
		if strings.EqualFold(o.Status, "done") {
			p.ObligationDone++
			if day(o.CompletedAt) != "" && day(o.DueDate) != "" && day(o.CompletedAt) <= day(o.DueDate) {
				p.OnTimeCount++
			} else {
				p.LateDoneCount++
			}
		} else if day(o.DueDate) != "" && day(o.DueDate) < today {
			p.OverduePendingCount++
		}
	}
	if p.ObligationDone > 0 {
		p.FulfillRate = float64(p.OnTimeCount) / float64(p.ObligationDone) * 100
	}

	categoryMap := map[string]int{}
	for _, c := range clauses {
		if disputeClause(c) {
			p.DisputeClauseCount++
			categoryMap[c.Type]++
		}
		if strings.EqualFold(c.Risk, "high") {
			p.HighRiskClauseCount++
		}
	}
	keys := make([]string, 0, len(categoryMap))
	for k := range categoryMap {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		p.ClauseCategories = append(p.ClauseCategories, CategoryCount{Type: k, Count: categoryMap[k]})
	}

	// 评分：只扣有证据的行为项；标签与扣分理由一一对应，避免「黑箱打分」。
	if p.ObligationDone > 0 {
		penalty := (100 - p.FulfillRate) * 0.35
		p.RiskScore -= int(clamp(penalty, 0, 35) + 0.5)
		if p.FulfillRate < 100 {
			p.Reasons = append(p.Reasons, "已完结义务按时完成比例偏低，扣分")
			p.Tags = append(p.Tags, "履约偏低")
		} else {
			p.Tags = append(p.Tags, "履约稳定")
		}
	}
	if p.OverduePendingCount > 0 {
		p.RiskScore -= minInt(p.OverduePendingCount*4, 20)
		p.Reasons = append(p.Reasons, "存在逾期未完成义务，扣分")
		p.Tags = append(p.Tags, "存在逾期")
	}
	if p.DisputeClauseCount > 0 {
		p.RiskScore -= minInt(p.DisputeClauseCount*3, 15)
		p.Reasons = append(p.Reasons, "历史条款中出现争议/违约类条款，扣分")
		if p.DisputeClauseCount >= 3 {
			p.Tags = append(p.Tags, "争议条款密集")
		}
	}
	if p.HighRiskClauseCount > 0 {
		p.RiskScore -= minInt(p.HighRiskClauseCount*5, 15)
		p.Tags = append(p.Tags, "高风险条款")
	}
	if p.ContractCount > 0 && p.RejectedCount > 0 {
		p.RiskScore -= minInt(p.RejectedCount*10, 20)
		p.Reasons = append(p.Reasons, "历史合同存在驳回/终止记录，扣分")
		p.Tags = append(p.Tags, "历史驳回")
	}
	if p.ContractCount <= 1 {
		p.Tags = append(p.Tags, "新相对方")
		p.SampleWarning = "内部样本不足（合同数 ≤ 1），风险分仅供参考"
	}
	if p.ObligationTotal == 0 {
		p.SampleWarning = "尚无履约义务记录，履约维度未参与评分"
	}
	if p.RiskScore < 0 {
		p.RiskScore = 0
	}
	switch {
	case p.RiskScore >= 80:
		p.RiskLevel = "low"
	case p.RiskScore >= 60:
		p.RiskLevel = "medium"
	default:
		p.RiskLevel = "high"
	}
	if len(p.Reasons) == 0 {
		p.Reasons = append(p.Reasons, "现有内部记录未发现扣分项")
	}
	return p
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// LevelLabel 前端展示用
func LevelLabel(level string) string {
	switch level {
	case "high":
		return "需重点关注"
	case "medium":
		return "一般关注"
	default:
		return "相对平稳"
	}
}
