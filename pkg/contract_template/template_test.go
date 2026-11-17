package contract_template

import (
	"reflect"
	"testing"
)

func TestExtractVars(t *testing.T) {
	content := "甲方{{partyA}}与乙方{{ partyB }}就{{subject}}签约，金额{{ amount }}元，{{partyA}}承担{{amount}}费用"
	got := ExtractVars(content)
	want := []string{"partyA", "partyB", "subject", "amount"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ExtractVars = %v, want %v", got, want)
	}
	// 中文变量名
	if got := ExtractVars("甲方{{甲方}}签署{{合同名称}}"); !reflect.DeepEqual(got, []string{"甲方", "合同名称"}) {
		t.Errorf("中文变量 = %v", got)
	}
	// 空/无变量
	if got := ExtractVars("无占位符"); len(got) != 0 {
		t.Errorf("无变量应返回空, got %v", got)
	}
}

func TestRender(t *testing.T) {
	content := "甲方{{partyA}}与乙方{{partyB}}，金额{{amount}}元，期限{{term}}"
	vars := []VarDecl{
		{Key: "partyA", Label: "甲方", Required: true},
		{Key: "partyB", Label: "乙方", Required: true},
		{Key: "amount", Label: "金额", Required: true, Default: "0"},
		{Key: "term", Label: "期限", Required: true},
	}
	out, missing := Render(content, map[string]string{"partyA": "甲公司", "amount": "5000"}, vars)
	// partyB 缺值但非 default → missing；term 缺值 → missing；amount 用 default 0 但传入 5000
	if out != "甲方甲公司与乙方，金额5000元，期限" {
		t.Errorf("Render = %q", out)
	}
	if len(missing) != 2 || !contains(missing, "partyB") || !contains(missing, "term") {
		t.Errorf("missing = %v, want [partyB term]", missing)
	}
}

func contains(ss []string, s string) bool {
	for _, x := range ss {
		if x == s {
			return true
		}
	}
	return false
}
