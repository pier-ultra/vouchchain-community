package schedcore

import (
	"testing"
	"time"
)

func TestNextRunMonthly(t *testing.T) {
	now := time.Date(2026, 11, 15, 10, 30, 0, 0, time.Local)
	got := NextRun("monthly", 5, now)
	want := time.Date(2026, 12, 5, 2, 0, 0, 0, time.Local)
	if !got.Equal(want) {
		t.Fatalf("monthly next run: got %s want %s", got, want)
	}
	if c := Cutoff("monthly", now); c != "2026-10-31" {
		t.Fatalf("monthly cutoff: got %s", c)
	}
}

func TestNextRunQuarterlyAndWeekly(t *testing.T) {
	now := time.Date(2026, 11, 15, 10, 0, 0, 0, time.Local)
	if got, want := NextRun("quarterly", 3, now), time.Date(2027, 1, 3, 2, 0, 0, 0, time.Local); !got.Equal(want) {
		t.Fatalf("quarterly next run: got %s want %s", got, want)
	}
	// 2026-11-15 是周日，本周起点 11-09（周一），下期起点 11-16
	if got, want := NextRun("weekly", 2, now), time.Date(2026, 11, 17, 2, 0, 0, 0, time.Local); !got.Equal(want) {
		t.Fatalf("weekly next run: got %s want %s", got, want)
	}
}

func TestNormalizePeriod(t *testing.T) {
	if p, err := NormalizePeriod("季"); err != nil || p != "quarterly" {
		t.Fatalf("normalize 季: %s %v", p, err)
	}
	if _, err := NormalizePeriod("yearly"); err == nil {
		t.Fatal("非法账期应报错")
	}
	if got := NextRun("monthly", 0, time.Date(2026, 11, 15, 0, 0, 0, 0, time.Local)); got.Day() != 1 || got.Month() != 12 {
		t.Fatalf("dueDays 应兜底为 1，got %s", got)
	}
}

func TestParseOrgCsv(t *testing.T) {
	rows := [][]string{
		{"code", "name", "node_type", "parent_code", "sort_order"},
		{"D001", "总部", "dept", "", "1"},
		{"D002", "财务部", "部门", "D001", "abc"},
		{"P001", "会计", "position", "D002", "3"},
		{"D003", "", "dept", "", "1"},
		{"X1", "怪类型", "unknown", "", "1"},
	}
	nodes, errs := ParseOrgCsv(rows)
	if len(nodes) != 3 || len(errs) != 3 {
		t.Fatalf("rows=%d errs=%d: %+v %+v", len(nodes), len(errs), nodes, errs)
	}
	if nodes[1].ParentCode != "D001" || nodes[1].SortOrder != 0 || nodes[1].NodeType != "dept" {
		t.Fatalf("中文表头/排序兜底失败: %+v", nodes[1])
	}
	if nodes[2].NodeType != "position" {
		t.Fatalf("岗位解析失败: %+v", nodes[2])
	}
}

func TestIsDescendant(t *testing.T) {
	parents := map[int64]int64{1: 0, 2: 1, 3: 2, 4: 0}
	if !IsDescendant(1, 1, parents) || !IsDescendant(1, 3, parents) {
		t.Fatal("自身与后代都应判为不可作为上级")
	}
	if IsDescendant(1, 4, parents) {
		t.Fatal("无关节点应可作为上级")
	}
	if !IsDescendant(2, 3, parents) {
		t.Fatal("孙节点应被拦截")
	}
	// 已有环的脏数据：链条走不完也要保守拒绝，不能让组织树成环
	cyclic := map[int64]int64{5: 6, 6: 5}
	if !IsDescendant(5, 6, cyclic) {
		t.Fatal("环形链条应保守判为不可作为上级")
	}
}
