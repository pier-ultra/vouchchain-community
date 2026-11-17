package partyprofile

import (
	"strings"
	"testing"
	"time"
)

var fixedNow = time.Date(2026, 11, 15, 0, 0, 0, 0, time.UTC)

func TestEmptyPartyWarnsSample(t *testing.T) {
	p := Build("新供应商", nil, nil, nil, fixedNow)
	if p.RiskScore != 100 || p.RiskLevel != "low" || p.SampleWarning == "" {
		t.Fatalf("空样本应满分但明确警告：%+v", p)
	}
	if !strings.Contains(strings.Join(p.Tags, ","), "新相对方") {
		t.Fatal("应标注新相对方")
	}
}

func TestAggregateAndOnTimeRate(t *testing.T) {
	contracts := []ContractRow{
		{ID: 1, CounterParty: "甲公司", Amount: 100, Status: "active", SignDate: "2025-01-02"},
		{ID: 2, CounterParty: "甲公司", Amount: 250, Status: "rejected", SignDate: "2026-03-04"},
		{ID: 3, CounterParty: "甲公司", Amount: 50, Status: "draft", SignDate: "2024-06-01"},
	}
	obligations := []ObligationRow{
		{ID: 1, Status: "done", DueDate: "2026-01-10", CompletedAt: "2026-01-09"},
		{ID: 2, Status: "done", DueDate: "2026-02-10", CompletedAt: "2026-02-20"},
		{ID: 3, Status: "pending", DueDate: "2026-10-01"},
	}
	clauses := []ClauseRow{
		{ID: 1, Type: "违约", Risk: "high", Summary: "逾期违约金"},
		{ID: 2, Type: "争议解决", Risk: "medium"},
		{ID: 3, Type: "保密", Risk: "low"},
	}
	p := Build("甲公司", contracts, obligations, clauses, fixedNow)
	if p.ContractCount != 3 || p.TotalAmount != 400 || p.ActiveCount != 1 || p.RejectedCount != 1 {
		t.Fatalf("合同聚合错误：%+v", p)
	}
	if p.FirstSignDate != "2024-06-01" || p.LastSignDate != "2026-03-04" {
		t.Fatalf("签约日期区间错误：%s ~ %s", p.FirstSignDate, p.LastSignDate)
	}
	if p.ObligationDone != 2 || p.OnTimeCount != 1 || p.LateDoneCount != 1 || p.OverduePendingCount != 1 {
		t.Fatalf("义务统计错误：%+v", p)
	}
	if p.FulfillRate != 50 {
		t.Fatalf("按时履约比例应为 50，得到 %v", p.FulfillRate)
	}
	if p.DisputeClauseCount != 2 || p.HighRiskClauseCount != 1 || len(p.ClauseCategories) != 2 {
		t.Fatalf("争议条款统计错误：%+v", p)
	}
	if p.RiskScore >= 60 {
		t.Fatalf("多项扣分后应为重点关注，得到 %d", p.RiskScore)
	}
	if p.RiskLevel != "high" {
		t.Fatalf("风险等级错误：%s", p.RiskLevel)
	}
	tags := strings.Join(p.Tags, ",")
	for _, want := range []string{"履约偏低", "存在逾期", "高风险条款", "历史驳回"} {
		if !strings.Contains(tags, want) {
			t.Fatalf("缺少标签 %s：%s", want, tags)
		}
	}
}

func TestCleanPartyKeepsHighScore(t *testing.T) {
	p := Build("稳健供应商",
		[]ContractRow{{ID: 1, Amount: 10, Status: "active"}, {ID: 2, Amount: 20, Status: "active"}},
		[]ObligationRow{{ID: 1, Status: "done", DueDate: "2026-01-01", CompletedAt: "2026-01-01"}},
		[]ClauseRow{{ID: 1, Type: "保密", Risk: "low"}}, fixedNow)
	if p.RiskScore != 100 || p.RiskLevel != "low" || p.SampleWarning != "" {
		t.Fatalf("全按时且无争议应保持满分：%+v", p)
	}
}

func TestDisputeKeywordDetection(t *testing.T) {
	p := Build("乙公司", []ContractRow{{ID: 1}, {ID: 2}}, nil,
		[]ClauseRow{{Type: "违约责任", Risk: "none"}, {Type: "争议解决", Risk: "none"}, {Type: "金额", Risk: "high"}}, fixedNow)
	if p.DisputeClauseCount != 3 || p.HighRiskClauseCount != 1 {
		t.Fatalf("争议关键词与高风险识别错误：%+v", p)
	}
}
