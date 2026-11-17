package policycore

import "testing"

func TestRequired(t *testing.T) {
	r := Rule{Code: "R1", Name: "相对方必填", RuleType: "required", Field: "counterParty", Severity: "block", Enabled: true}
	if got := Evaluate(r, map[string]interface{}{"counterParty": "甲公司"}); !got.Passed {
		t.Fatal("有值应通过:", got.Message)
	}
	for _, v := range []interface{}{nil, "", "   ", 0} {
		if got := Evaluate(r, map[string]interface{}{"counterParty": v}); got.Passed {
			t.Fatalf("空值 %v 应拦截", v)
		}
	}
}

func TestFormat(t *testing.T) {
	r := Rule{Code: "R2", RuleType: "format", Field: "invoiceNo", Op: "regex", Value: `^[0-9]{8,20}$`, Severity: "warn", Enabled: true}
	if got := Evaluate(r, map[string]interface{}{"invoiceNo": "253170000010150001"}); !got.Passed {
		t.Fatal("18 位数电票号应通过:", got.Message)
	}
	if got := Evaluate(r, map[string]interface{}{"invoiceNo": "ABC-1"}); got.Passed {
		t.Fatal("非法票号应判失败")
	}
	bad := r
	bad.Value = "["
	if got := Evaluate(bad, map[string]interface{}{"invoiceNo": "1"}); got.Passed {
		t.Fatal("坏正则应判失败而不是 panic")
	}
}

func TestThreshold(t *testing.T) {
	r := Rule{Code: "R3", RuleType: "threshold", Field: "amount", Op: "lte", Value: "1000000", Severity: "block", Enabled: true}
	for _, c := range []struct {
		v   interface{}
		ok  bool
		msg string
	}{
		{1000.0, true, "小额应通过"},
		{1000000.0, true, "等于上限应通过"},
		{"2,000,000", false, "超上限应拦截"},
		{"", false, "空值应拦截"},
	} {
		if got := Evaluate(r, map[string]interface{}{"amount": c.v}); got.Passed != c.ok {
			t.Fatalf("%s: got passed=%v msg=%s", c.msg, got.Passed, got.Message)
		}
	}
	gt := Rule{RuleType: "threshold", Field: "amount", Op: "gt", Value: "0", Enabled: true}
	if got := Evaluate(gt, map[string]interface{}{"amount": 0}); got.Passed {
		t.Fatal("零金额凭证应拦截")
	}
}

func TestRelation(t *testing.T) {
	r := Rule{Code: "R4", RuleType: "relation", Field: "voucherId", Op: "exists", Value: "voucherId", Severity: "warn", Enabled: true}
	if got := Evaluate(r, map[string]interface{}{"voucherId": 0}); got.Passed {
		t.Fatal("未关联凭证应判失败")
	}
	if got := Evaluate(r, map[string]interface{}{"voucherId": 3}); !got.Passed {
		t.Fatal("已关联应通过")
	}
}

func TestDisabledAndSeverity(t *testing.T) {
	r := Rule{Code: "R5", RuleType: "required", Field: "x", Severity: "block"}
	if got := Evaluate(r, map[string]interface{}{}); !got.Passed {
		t.Fatal("停用规则不参与判定")
	}
	all := []Rule{
		{Code: "A", RuleType: "required", Field: "a", Severity: "block", Enabled: true},
		{Code: "B", RuleType: "required", Field: "b", Severity: "warn", Enabled: true},
	}
	bad := EvaluateAll(all, map[string]interface{}{})
	if len(bad) != 2 || !HasBlock(bad) {
		t.Fatal("应收集全部违规并识别出 block")
	}
	warnOnly := EvaluateAll(all[1:], map[string]interface{}{})
	if HasBlock(warnOnly) {
		t.Fatal("仅告警不应触发拦截")
	}
}
