package revrec

import (
	"testing"

	"vouchchain/pkg/agent"
)

// R-A5 分录生成（纯函数）：收入+成本双重确认，借贷平衡，科目来自 account_mapping（可配置）。

func TestBuildRecognitionEntries_NoCost(t *testing.T) {
	subj := defaultSubjectCodes()
	entries := BuildRecognitionEntries(1000, 0, 130, subj, "确认收入-履约义务#1")
	if len(entries) != 3 {
		t.Fatalf("无成本应 3 行分录，实际 %d", len(entries))
	}
	if !agent.Balanced(entries) {
		t.Fatalf("借贷不平衡：%+v", entries)
	}
	if entries[0].SubjectCode != "1122" || entries[0].Debit != 1130 {
		t.Fatalf("应收借方向错误：%+v", entries[0])
	}
	if entries[1].SubjectCode != "6001" || entries[1].Credit != 1000 {
		t.Fatalf("收入贷方向错误：%+v", entries[1])
	}
	if entries[2].SubjectCode != "22210105" || entries[2].Credit != 130 {
		t.Fatalf("销项税方向错误：%+v", entries[2])
	}
}

func TestBuildRecognitionEntries_WithCost(t *testing.T) {
	subj := defaultSubjectCodes()
	entries := BuildRecognitionEntries(1000, 600, 130, subj, "确认收入-履约义务#1")
	if len(entries) != 5 {
		t.Fatalf("含成本应 5 行分录，实际 %d", len(entries))
	}
	if !agent.Balanced(entries) {
		t.Fatalf("借贷不平衡：%+v", entries)
	}
	// 第 4/5 行：借 主营业务成本 / 贷 合同履约成本
	if entries[3].SubjectCode != "6401" || entries[3].Debit != 600 {
		t.Fatalf("成本借方向错误：%+v", entries[3])
	}
	if entries[4].SubjectCode != "1471" || entries[4].Credit != 600 {
		t.Fatalf("履约成本贷方向错误：%+v", entries[4])
	}
	// 借方合计 = 1130(应收) + 600(成本)；贷方合计 = 1000(收入) + 130(税) + 600(履约成本)
	var d, c float64
	for _, e := range entries {
		d += e.Debit
		c += e.Credit
	}
	if d != 1730 || c != 1730 {
		t.Fatalf("借贷金额错误：借 %.2f 贷 %.2f", d, c)
	}
}

func TestBuildRecognitionEntries_CustomSubjects(t *testing.T) {
	// 模拟 account_mapping 改过科目（复用映射，不硬编码）
	subj := defaultSubjectCodes()
	subj.Revenue = Subject{"600101", "主营业务收入—服务收入"}
	subj.Receivable = Subject{"112201", "应收账款—内部"}
	entries := BuildRecognitionEntries(5000, 0, 300, subj, "确认收入-履约义务#2")
	if entries[0].SubjectCode != "112201" || entries[0].SubjectName != "应收账款—内部" {
		t.Fatalf("应收科目未按映射：%+v", entries[0])
	}
	if entries[1].SubjectCode != "600101" || entries[1].SubjectName != "主营业务收入—服务收入" {
		t.Fatalf("收入科目未按映射：%+v", entries[1])
	}
	if !agent.Balanced(entries) {
		t.Fatalf("借贷不平衡：%+v", entries)
	}
}

func TestBuildRecognitionEntries_ZeroAmount(t *testing.T) {
	// 金额与税均为 0 时不应产出合法分录（Balanced 要求借方 > 0）
	subj := defaultSubjectCodes()
	entries := BuildRecognitionEntries(0, 0, 0, subj, "确认收入-履约义务#3")
	if agent.Balanced(entries) {
		t.Fatalf("零金额不应判定平衡：%+v", entries)
	}
}

// R-A5 成本结转：本期确认成本 = 累计已发生成本 − 已确认成本（时段法投入法）

func TestRecognitionCost_Basic(t *testing.T) {
	cases := []struct {
		accum, confirmed, want float64
	}{
		{0, 0, 0},            // 无成本数据 → 0
		{500, 300, 200},      // 常规
		{500, 700, 0},        // 已确认超累计 → 0（clamp）
		{500, 500, 0},        // 刚好
		{100.456, 0, 100.46}, // 四舍五入
	}
	for _, c := range cases {
		if got := recognitionCost(c.accum, c.confirmed); got != c.want {
			t.Fatalf("recognitionCost(%.3f, %.3f) = %.2f，期望 %.2f", c.accum, c.confirmed, got, c.want)
		}
	}
}

func TestRound2(t *testing.T) {
	cases := []struct{ in, want float64 }{
		{1.006, 1.01},
		{2.344, 2.34},
		{2.345, 2.35},
		{0.5, 0.5},
		{100.456, 100.46},
	}
	for _, c := range cases {
		if got := round2(c.in); got != c.want {
			t.Fatalf("round2(%.3f) = %.2f，期望 %.2f", c.in, got, c.want)
		}
	}
}
