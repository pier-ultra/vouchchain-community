package orgcore

import (
	"reflect"
	"testing"
)

// 组织结构：
//
//	1 总公司
//	├─ 2 华东
//	│  └─ 4 研发部
//	│     └─ 5 后端组
//	└─ 3 华南
//	6 独立主体
//	└─ 7 财务部
var sample = map[int64]int64{1: 0, 2: 1, 3: 1, 4: 2, 5: 4, 6: 0, 7: 6}

func TestDescendantsMultiLevel(t *testing.T) {
	tr := New(sample)
	got := tr.Descendants(1)
	want := []int64{1, 2, 3, 4, 5} // 广度优先，含自身，不含别人的子树
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Descendants(1) = %v, want %v", got, want)
	}
	if got := tr.Descendants(4); !reflect.DeepEqual(got, []int64{4, 5}) {
		t.Fatalf("Descendants(4) = %v", got)
	}
	if got := tr.Descendants(5); !reflect.DeepEqual(got, []int64{5}) {
		t.Fatalf("叶子节点 = %v", got)
	}
}

func TestDescendantsMultipleRootsAndDedup(t *testing.T) {
	tr := New(sample)
	got := tr.Descendants(2, 4, 6) // 4 是 2 的后代，重复给不应重复返回
	want := []int64{2, 4, 6, 5, 7}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("多根合并 = %v, want %v", got, want)
	}
	if got := tr.Descendants(0); len(got) != 0 {
		t.Fatalf("0 表示无节点，应返回空，得到 %v", got)
	}
	// 父节点被删、挂载关系残留时，根节点自身仍在表里查不到但必须保留在范围内
	if got := tr.Descendants(999); !reflect.DeepEqual(got, []int64{999}) {
		t.Fatalf("未知根节点应保留自身，得到 %v", got)
	}
}

func TestDescendantsToleratesCycle(t *testing.T) {
	// 历史脏数据：1 与 2 互挂，3 挂在 1 下
	tr := New(map[int64]int64{1: 2, 2: 1, 3: 1})
	got := tr.Descendants(1)
	if len(got) > 3 || got[0] != 1 {
		t.Fatalf("环下递归应终止且只包含可达节点，得到 %v", got)
	}
	self := New(map[int64]int64{1: 1})
	if got := self.Descendants(1); !reflect.DeepEqual(got, []int64{1}) {
		t.Fatalf("自环应只返回自身，得到 %v", got)
	}
}

func TestAncestorsDepthAndPath(t *testing.T) {
	tr := New(sample)
	if got := tr.Ancestors(5); !reflect.DeepEqual(got, []int64{5, 4, 2, 1}) {
		t.Fatalf("Ancestors(5) = %v", got)
	}
	if got := tr.Depth(5); got != 3 {
		t.Fatalf("Depth(5) = %d, want 3", got)
	}
	if got := tr.Depth(1); got != 0 {
		t.Fatalf("根节点 Depth = %d, want 0", got)
	}
	names := map[int64]string{1: "总公司", 2: "华东", 4: "研发部", 5: "后端组"}
	if got := PathNames(tr.Ancestors(5), names, " / "); got != "总公司 / 华东 / 研发部 / 后端组" {
		t.Fatalf("PathNames = %q", got)
	}
}

func TestIsDescendantPreventsCycle(t *testing.T) {
	tr := New(sample)
	if !tr.IsDescendant(2, 4) || !tr.IsDescendant(2, 5) {
		t.Fatal("后代应被识别")
	}
	if !tr.IsDescendant(2, 2) {
		t.Fatal("挂到自己下面也必须拦截")
	}
	if tr.IsDescendant(4, 2) {
		t.Fatal("祖先不是后代，不能误判")
	}
	cyclic := New(map[int64]int64{1: 2, 2: 1})
	if !cyclic.Cyclic(1) || !cyclic.Cyclic(2) {
		t.Fatal("父子互挂应被识别为异常链条")
	}
	if !cyclic.IsDescendant(1, 2) {
		t.Fatal("环里往上追到自己也要拦截")
	}
}
