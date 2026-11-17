package isolate

import (
	"reflect"
	"testing"
)

func TestNoPolicyKeepsOpen(t *testing.T) {
	d := Resolve(nil, Subject{User: "u", Role: "viewer"}, nil)
	if d.Enforced {
		t.Fatal("无启用策略时不应收紧")
	}
	if ok, reason := d.Allow(Object{EntityID: 9}); !ok || reason != "" {
		t.Fatalf("无策略应放行，得到 %v %s", ok, reason)
	}
}

func TestEntityScopeAndStrictUnassigned(t *testing.T) {
	ps := []Policy{
		{ID: 1, Dimension: DimEntity, SubjectType: "role", Subject: "business", Values: []string{"2", "3"}, Enabled: true},
	}
	d := Resolve(ps, Subject{User: "u", Role: "business"}, nil)
	if !d.Enforced || !reflect.DeepEqual(d.EntityIDs, []int64{2, 3}) {
		t.Fatalf("主体范围错误：%+v", d)
	}
	if ok, _ := d.Allow(Object{EntityID: 3}); !ok {
		t.Fatal("允许主体应放行")
	}
	if ok, reason := d.Allow(Object{EntityID: 4}); ok || reason == "" {
		t.Fatal("非授权主体应拒绝并给出原因")
	}
	if ok, _ := d.Allow(Object{EntityID: 0}); ok {
		t.Fatal("未归属历史数据默认严格拒绝")
	}
	d0 := Resolve([]Policy{{ID: 2, Dimension: DimEntity, SubjectType: "all", Values: []string{"0"}, Enabled: true}}, Subject{Role: "viewer"}, nil)
	if ok, _ := d0.Allow(Object{EntityID: 0}); !ok {
		t.Fatal("显式授权 0 后未归属数据应可见")
	}
}

func TestUserOverridesRole(t *testing.T) {
	ps := []Policy{
		{ID: 1, Dimension: DimEntity, SubjectType: "role", Subject: "legal", Values: []string{"1", "2", "3"}, Enabled: true, Priority: 99},
		{ID: 2, Dimension: DimEntity, SubjectType: "user", Subject: "wang", Values: []string{"2"}, Enabled: true, Priority: 1},
		{ID: 3, Dimension: DimEntity, SubjectType: "all", Values: []string{"9"}, Enabled: true},
	}
	d := Resolve(ps, Subject{User: "wang", Role: "legal"}, nil)
	if !reflect.DeepEqual(d.EntityIDs, []int64{2}) {
		t.Fatalf("用户级应完全覆盖角色/all 级，得到 %v", d.EntityIDs)
	}
	d2 := Resolve(ps, Subject{User: "li", Role: "legal"}, nil)
	if !reflect.DeepEqual(d2.EntityIDs, []int64{1, 2, 3}) {
		t.Fatalf("角色级应覆盖 all 级，得到 %v", d2.EntityIDs)
	}
}

func TestSameLevelUnionAndDisabledIgnored(t *testing.T) {
	ps := []Policy{
		{ID: 1, Dimension: DimProject, SubjectType: "role", Subject: "legal", Values: []string{"PRJ-A"}, Enabled: true},
		{ID: 2, Dimension: DimProject, SubjectType: "role", Subject: "legal", Values: []string{"PRJ-B", "PRJ-A"}, Enabled: true},
		{ID: 3, Dimension: DimProject, SubjectType: "role", Subject: "legal", Values: []string{"PRJ-C"}, Enabled: false},
		{ID: 4, Dimension: "unknown", SubjectType: "all", Values: []string{"x"}, Enabled: true},
	}
	d := Resolve(ps, Subject{User: "u", Role: "legal"}, nil)
	if !reflect.DeepEqual(d.Projects, []string{"PRJ-A", "PRJ-B"}) {
		t.Fatalf("同级并集/禁用过滤错误：%v", d.Projects)
	}
	if _, ok := d.Allowed["unknown"]; ok {
		t.Fatal("非法维度不应生效")
	}
}

func TestExpandChildren(t *testing.T) {
	expand := func(dim string, values []string, include bool) []string {
		if !include {
			return values
		}
		out := append([]string{}, values...)
		for _, v := range values {
			if v == "1" {
				out = append(out, "11", "12")
			}
		}
		return out
	}
	d := Resolve([]Policy{{ID: 1, Dimension: DimEntity, SubjectType: "all", Values: []string{"1"}, IncludeChildren: true, Enabled: true}},
		Subject{Role: "viewer"}, expand)
	if !reflect.DeepEqual(d.EntityIDs, []int64{1, 11, 12}) {
		t.Fatalf("含下级展开错误：%v", d.EntityIDs)
	}
	if ok, _ := d.Allow(Object{EntityID: 12}); !ok {
		t.Fatal("下级主体应放行")
	}
}

func TestMultiDimensionAllMustPass(t *testing.T) {
	ps := []Policy{
		{ID: 1, Dimension: DimEntity, SubjectType: "all", Values: []string{"1"}, Enabled: true},
		{ID: 2, Dimension: DimDept, SubjectType: "all", Values: []string{"7"}, Enabled: true},
		{ID: 3, Dimension: DimProject, SubjectType: "all", Values: []string{"*"}, Enabled: true},
	}
	d := Resolve(ps, Subject{User: "u", Role: "viewer"}, nil)
	if ok, _ := d.Allow(Object{EntityID: 1, DeptID: 7, Project: "随便"}); !ok {
		t.Fatal("三维都满足应放行")
	}
	if ok, reason := d.Allow(Object{EntityID: 1, DeptID: 8, Project: "随便"}); ok || reason == "" {
		t.Fatal("任一维度不满足应拒绝")
	}
}

func TestEmptyValuesMeansDenyAll(t *testing.T) {
	d := Resolve([]Policy{{ID: 1, Dimension: DimEntity, SubjectType: "all", Enabled: true}}, Subject{Role: "viewer"}, nil)
	if !d.Enforced {
		t.Fatal("配了策略即视为强制")
	}
	if ok, _ := d.Allow(Object{EntityID: 1}); ok {
		t.Fatal("空允许值应拒绝任何数据")
	}
}
