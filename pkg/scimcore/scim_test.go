package scimcore

import (
	"encoding/json"
	"testing"
)

func parseDoc(t *testing.T, s string) map[string]any {
	t.Helper()
	var doc map[string]any
	if err := json.Unmarshal([]byte(s), &doc); err != nil {
		t.Fatal(err)
	}
	return doc
}

func TestParseUser(t *testing.T) {
	u, err := ParseUser(parseDoc(t, `{
		"schemas":["urn:ietf:params:scim:schemas:core:2.0:User"],
		"externalId":"casdoor-1001",
		"userName":"zhangsan",
		"name":{"formatted":"张三","familyName":"张"},
		"emails":[{"value":"z@x.com","type":"work","primary":true},{"value":"backup@x.com"}],
		"active":true,
		"groups":[{"value":"g-1","display":"法务组"},{"display":"business"}],
		"urn:ietf:params:scim:schemas:extension:enterprise:2.0:User":{"department":"法务部","title":"法务经理"}
	}`))
	if err != nil {
		t.Fatal(err)
	}
	if u.UserName != "zhangsan" || u.DisplayName != "张三" || u.Email != "z@x.com" {
		t.Fatalf("基础字段解析错误：%+v", u)
	}
	if u.ExternalId != "casdoor-1001" || u.Department != "法务部" || u.Title != "法务经理" {
		t.Fatalf("扩展字段解析错误：%+v", u)
	}
	if u.Active == nil || !*u.Active {
		t.Fatal("active=true 未解析")
	}
	if len(u.Groups) != 2 || u.Groups[0] != "法务组" || u.Groups[1] != "business" {
		t.Fatalf("groups 解析错误：%v", u.Groups)
	}

	if _, err := ParseUser(parseDoc(t, `{"displayName":"没有用户名"}`)); err == nil {
		t.Fatal("缺 userName 应报错")
	}
	partial, err := ParseUser(parseDoc(t, `{"userName":"lisi"}`))
	if err != nil || partial.Active != nil {
		t.Fatal("文档未带 active 时应保持 nil（PATCH/PUT 场景不误禁用）")
	}
}

func TestApplyUserPatch(t *testing.T) {
	base, _ := ParseUser(parseDoc(t, `{"userName":"wangwu","active":true,"emails":[{"value":"old@x.com"}],"groups":[{"display":"业务组"}]}`))
	base.Email = "old@x.com"
	base.Groups = []string{"业务组"}

	// 常见离职推送：replace active=false + 无 path 的 value 对象 + 换组
	out, err := ApplyUserPatch(base, []PatchOp{
		{Op: "replace", Path: "active", Value: false},
		{Op: "replace", Value: map[string]any{"displayName": "王五", "emails": []any{map[string]any{"value": "new@x.com", "primary": true}}}},
		{Op: "replace", Path: "groups", Value: []any{map[string]any{"display": "法务组"}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.Active == nil || *out.Active {
		t.Fatal("replace active=false 未生效")
	}
	if out.DisplayName != "王五" || out.Email != "new@x.com" {
		t.Fatalf("无 path 对象补丁未生效：%+v", out)
	}
	if len(out.Groups) != 1 || out.Groups[0] != "法务组" {
		t.Fatalf("replace groups 未生效：%v", out.Groups)
	}

	// add groups 追加、remove groups 清空、remove 带筛选路径
	out2, err := ApplyUserPatch(out, []PatchOp{{Op: "add", Path: "groups", Value: []any{"business"}}})
	if err != nil || len(out2.Groups) != 2 {
		t.Fatalf("add groups 应追加：%v %v", out2.Groups, err)
	}
	out3, err := ApplyUserPatch(out2, []PatchOp{{Op: "remove", Path: "groups"}})
	if err != nil || len(out3.Groups) != 0 {
		t.Fatalf("remove groups 应清空：%v %v", out3.Groups, err)
	}

	// 带筛选路径的 email
	out4, err := ApplyUserPatch(base, []PatchOp{{Op: "replace", Path: `emails[type eq "work"].value`, Value: "work@x.com"}})
	if err != nil || out4.Email != "work@x.com" {
		t.Fatalf("带筛选的 email path 未生效：%q %v", out4.Email, err)
	}
	if _, err := ApplyUserPatch(base, []PatchOp{{Op: "replace", Path: "unknownField", Value: "x"}}); err == nil {
		t.Fatal("未知 path 应报错")
	}
	if _, err := ApplyUserPatch(base, []PatchOp{{Op: "remove", Path: "userName"}}); err == nil {
		t.Fatal("userName 不允许删除")
	}
}

func TestMatchUserFilter(t *testing.T) {
	u, _ := ParseUser(parseDoc(t, `{"userName":"zhangsan","displayName":"张三","active":true,"externalId":"ext-1"}`))
	cases := []struct {
		filter string
		want   bool
		err    bool
	}{
		{`userName eq "zhangsan"`, true, false},
		{`userName eq "lisi"`, false, false},
		{`userName eq "zhangsan" and active eq true`, true, false},
		{`userName eq "zhangsan" and active eq false`, false, false},
		{`displayName co "张"`, true, false},
		{`userName sw "zhang"`, true, false},
		{`externalId ne "other"`, true, false},
		{`emails[type eq "work"].value eq "z@x.com"`, false, false},
		{`unknownAttr eq "x"`, false, true},
		{`userName gt "a"`, false, true},
		{`userName eq "a" or active eq true`, false, true},
	}
	for _, c := range cases {
		got, err := MatchUserFilter(c.filter, u, 7)
		if (err != nil) != c.err || got != c.want {
			t.Errorf("filter %q => %v, %v（期望 %v, err=%v）", c.filter, got, err, c.want, c.err)
		}
	}
	if disabled, _ := MatchUserFilter(`active eq false`, UserAttrs{UserName: "x"}, 1); disabled {
		t.Error("未带 active 的用户不应匹配 active eq false")
	}
}

func TestGroupPatchAndFilter(t *testing.T) {
	g, err := ParseGroup(parseDoc(t, `{"displayName":"法务组","externalId":"g-1","members":[{"value":"1"},{"value":"zhangsan"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(g.Members) != 2 || g.Members[1] != "zhangsan" {
		t.Fatalf("成员解析错误：%v", g.Members)
	}
	g2, err := ApplyGroupPatch(g, []PatchOp{{Op: "remove", Path: `members[value eq "1"]`}})
	if err != nil || len(g2.Members) != 1 || g2.Members[0] != "zhangsan" {
		t.Fatalf("带筛选路径移除成员失败：%v %v", g2.Members, err)
	}
	g3, err := ApplyGroupPatch(g2, []PatchOp{{Op: "add", Path: "members", Value: []any{map[string]any{"value": "lisi"}}}})
	if err != nil || len(g3.Members) != 2 {
		t.Fatalf("追加成员失败：%v %v", g3.Members, err)
	}
	if ok, err := MatchGroupFilter(`displayName eq "法务组"`, g, 3); err != nil || !ok {
		t.Fatalf("组 filter 失败：%v %v", ok, err)
	}
}
