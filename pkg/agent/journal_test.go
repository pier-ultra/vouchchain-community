package agent

import "testing"

func testSubjects() []AccountSubject {
	return []AccountSubject{
		{Category: "货物采购", SubjectCode: "1401", SubjectName: "原材料/库存商品", Direction: "debit"},
		{Category: "服务采购", SubjectCode: "6602", SubjectName: "管理费用", Direction: "debit"},
		{Category: "进项税额", SubjectCode: "22210101", SubjectName: "进项税额", Direction: "debit"},
		{Category: "应付账款", SubjectCode: "2202", SubjectName: "应付账款", Direction: "credit"},
	}
}

func TestRuleJournalBalanced(t *testing.T) {
	s := RuleJournal(JournalFacts{
		InvoiceNo: "FP-001", SellerName: "某供应商", BuyerName: "本公司",
		Category: "货物采购", Amount: 1000, Tax: 130, Total: 1130,
	}, testSubjects())
	if len(s.Entries) != 3 {
		t.Fatalf("预期 3 行分录，实际 %d", len(s.Entries))
	}
	if !Balanced(s.Entries) {
		t.Fatalf("借贷不平衡：%+v", s.Entries)
	}
	if s.Entries[0].SubjectCode != "1401" || s.Entries[1].SubjectCode != "22210101" || s.Entries[2].SubjectCode != "2202" {
		t.Fatalf("科目映射错误：%+v", s.Entries)
	}
	if s.Source != "rule" {
		t.Fatalf("来源应为 rule，实际 %s", s.Source)
	}
}

func TestRuleJournalZeroTaxAndAmountMismatch(t *testing.T) {
	s := RuleJournal(JournalFacts{Category: "服务采购", Amount: 0, Tax: 0, Total: 800}, testSubjects())
	if len(s.Entries) != 2 || !Balanced(s.Entries) {
		t.Fatalf("零税额应为两行且平衡：%+v", s.Entries)
	}
	if s.Entries[0].Debit != 800 || s.Entries[1].Credit != 800 {
		t.Fatalf("金额与价税合计不符时应按合计倒挤：%+v", s.Entries)
	}
}

func TestBalancedRejectsBadEntries(t *testing.T) {
	good := []JournalEntry{{Debit: 100}, {Credit: 100}}
	if !Balanced(good) {
		t.Fatal("平衡分录被误判")
	}
	bad := [][]JournalEntry{
		{{Debit: 100}},                              // 单行
		{{Debit: 100}, {Credit: 90}},                // 不平
		{{Debit: 100, Credit: 100}, {Credit: -10}},  // 同行双正 + 负数
	}
	for i, b := range bad {
		if Balanced(b) {
			t.Fatalf("第 %d 组非法分录被放行", i+1)
		}
	}
}

func TestParseJournalJSON(t *testing.T) {
	s := "```json\n{\"entries\":[{\"summary\":\"采购\",\"subjectCode\":\"1401\",\"subjectName\":\"原材料\",\"debit\":100,\"credit\":0}],\"reason\":\"x\"}\n```"
	out, err := parseJournalJSON(s)
	if err != nil || len(out.Entries) != 1 || out.Entries[0].Debit != 100 {
		t.Fatalf("解析失败：%v %+v", err, out)
	}
	if _, err := parseJournalJSON("模型没有返回 JSON"); err == nil {
		t.Fatal("无 JSON 时应报错")
	}
}
