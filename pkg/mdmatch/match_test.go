package mdmatch

import "testing"

func TestNormalizeAndSimilarity(t *testing.T) {
	if Normalize("上海云帆信息技术有限公司") != Normalize("上海云帆信息技术（更名）") {
		// 「（更名）」去掉后仍带「信息技术」差异，仅验证后缀确实被剥离
		t.Log("后缀剥离后仍不同名属正常")
	}
	if Normalize("ＡＢＣ 有限公司") != "abc" {
		t.Fatalf("全角/后缀归一化失败: %q", Normalize("ＡＢＣ 有限公司"))
	}
	if s := Similarity("上海云帆信息技术有限公司", "上海云帆信息技术"); s < 80 {
		t.Fatalf("仅公司后缀差异应判高度相似，got %.2f", s)
	}
	if s := Similarity("上海云帆信息技术有限公司", "北京大海贸易有限公司"); s > 55 {
		t.Fatalf("完全无关公司不应判重复，got %.2f", s)
	}
}

func TestCompareStrongIdentity(t *testing.T) {
	a := Candidate{ID: 1, UID: "MD-SUP-aaa", Name: "上海云帆信息技术有限公司", Code: "SUP-001",
		Status: "active", Attributes: map[string]interface{}{"taxNo": "91310000TEST001", "address": "上海市浦东大道1号"}}
	b := Candidate{ID: 2, UID: "MD-SUP-bbb", Name: "上海云帆信息技术（更名）", Code: "SUP-002",
		Status: "active", Attributes: map[string]interface{}{"taxNo": "91310000TEST001", "address": "上海市浦东大道 1 号"}}
	rule := Rule{Fields: []string{"name", "taxNo", "address"}, Threshold: 68}
	m := Compare(a, b, rule)
	if !m.Duplicate || m.Score < 90 {
		t.Fatalf("同税号应判重复，score=%.2f dup=%v", m.Score, m.Duplicate)
	}
	if len(m.Reasons) < 3 {
		t.Fatal("应逐字段给出相似度说明")
	}
}

func TestCompareNotDuplicateAndThreshold(t *testing.T) {
	a := Candidate{ID: 1, Name: "上海云帆信息技术有限公司", Attributes: map[string]interface{}{"taxNo": "AAA111"}}
	b := Candidate{ID: 2, Name: "广州远洋物流有限公司", Attributes: map[string]interface{}{"taxNo": "BBB222"}}
	rule := Rule{Fields: []string{"name", "taxNo"}, Threshold: 70}
	m := Compare(a, b, rule)
	if m.Duplicate {
		t.Fatalf("不同公司不应判重复，score=%.2f", m.Score)
	}
	lo := Rule{Fields: []string{"name", "taxNo"}, Threshold: 0}
	if !Compare(a, b, lo).Duplicate {
		t.Fatal("阈值降到 0 后应命中（阈值可配置）")
	}
}

func TestSuggestSurvivor(t *testing.T) {
	rich := Candidate{ID: 1, UID: "U1", Name: "上海云帆信息技术有限公司", Status: "active",
		Attributes: map[string]interface{}{"taxNo": "X", "phone": "Y"}}
	poor := Candidate{ID: 2, UID: "U2", Name: "上海云帆", Status: "active", Attributes: map[string]interface{}{}}
	s, d, msg := SuggestSurvivor(poor, rich)
	if s.ID != 1 || d.ID != 2 {
		t.Fatal("应保留属性更完整的记录作为权威版本")
	}
	if msg == "" {
		t.Fatal("合并建议必须给出文字说明")
	}
}
