// Package orgcore 组织树（org_node）与法人主体树（legal_entity）共用的父子递归逻辑。
// 纯内存、零 DB / 零框架依赖：父子树查询、层级、路径、防环都在这里收口，便于单测。
// 历史脏数据可能父子互挂，所有递归都带 visited 与深度上限，绝不因环把请求打死。
package orgcore

import (
	"sort"
	"strings"
)

const maxDepth = 200

// Tree 由 id → parentId 构造的父子索引；parentId <= 0 视为根节点。
type Tree struct {
	parentOf   map[int64]int64
	childrenOf map[int64][]int64
}

// New 建索引；自挂（id 的父是自己）在建 childrenOf 时忽略，避免自环无限递归。
func New(parentOf map[int64]int64) *Tree {
	t := &Tree{parentOf: map[int64]int64{}, childrenOf: map[int64][]int64{}}
	for id, p := range parentOf {
		t.parentOf[id] = p
	}
	for id, p := range parentOf {
		if p > 0 && p != id {
			t.childrenOf[p] = append(t.childrenOf[p], id)
		}
	}
	// 排序保证同样的父子数据每次返回同样的顺序（列表/树展示不做随机漂移）
	for p := range t.childrenOf {
		sort.Slice(t.childrenOf[p], func(i, j int) bool {
			return t.childrenOf[p][i] < t.childrenOf[p][j]
		})
	}
	return t
}

// Children 返回直接子节点（副本，调用方改动不影响索引）
func (t *Tree) Children(id int64) []int64 {
	out := make([]int64, len(t.childrenOf[id]))
	copy(out, t.childrenOf[id])
	return out
}

// Descendants 递归收集 roots 及其全部后代（含 roots 自身）：
// 广度优先 + visited 去重，既能处理「一个用户挂多个节点」，也能容忍数据里已有的环。
func (t *Tree) Descendants(roots ...int64) []int64 {
	visited := map[int64]bool{}
	out := make([]int64, 0, len(roots)*2)
	queue := make([]int64, 0, len(roots)*2)
	for _, r := range roots {
		if r > 0 && !visited[r] {
			visited[r] = true
			out = append(out, r)
			queue = append(queue, r)
		}
	}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		for _, c := range t.childrenOf[cur] {
			if !visited[c] {
				visited[c] = true
				out = append(out, c)
				queue = append(queue, c)
			}
		}
	}
	return out
}

// Ancestors 从 id 一路向上到根（含 id 自身），用于面包屑路径与防环。
func (t *Tree) Ancestors(id int64) []int64 {
	out := make([]int64, 0, 4)
	visited := map[int64]bool{}
	cur := id
	for cur > 0 && !visited[cur] && len(out) < maxDepth {
		visited[cur] = true
		out = append(out, cur)
		cur = t.parentOf[cur]
	}
	return out
}

// Cyclic 向上追链是否撞到深度上限或环：用于「改上级」时保守拒绝。
func (t *Tree) Cyclic(id int64) bool {
	chain := t.Ancestors(id)
	if len(chain) >= maxDepth {
		return true
	}
	p := t.parentOf[chain[len(chain)-1]]
	return p > 0 // 追到上限前没到根，说明链条异常（多半是环）
}

// Depth 层级：根为 0，子节点 1，以此类推
func (t *Tree) Depth(id int64) int {
	n := len(t.Ancestors(id))
	if n <= 1 {
		return 0
	}
	return n - 1
}

// IsDescendant 判断 candidate 是否为 nodeId 自身或其后代。
// 改上级时：若新上级是当前节点的后代，就会形成环，必须拒绝。
func (t *Tree) IsDescendant(nodeId, candidate int64) bool {
	for _, a := range t.Ancestors(candidate) {
		if a == nodeId {
			return true
		}
	}
	return t.Cyclic(candidate)
}

// PathNames 把 Ancestors 的结果转成「根 / … / 当前」可读路径。
func PathNames(chain []int64, names map[int64]string, sep string) string {
	if sep == "" {
		sep = " / "
	}
	parts := make([]string, 0, len(chain))
	for i := len(chain) - 1; i >= 0; i-- {
		name := names[chain[i]]
		if name == "" {
			continue
		}
		parts = append(parts, name)
	}
	return strings.Join(parts, sep)
}
