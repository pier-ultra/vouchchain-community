// Package isolate：AG5 多维度数据隔离的纯逻辑层（零外部依赖，可单独抽成开源库）。
//
// 口径（重要，配置时按此理解）：
//   - 三条维度：entity（法人主体）/ dept（组织节点）/ project（项目编码），可叠加，同时满足才放行。
//   - 主体优先级：user 级 > role 级 > all 级。某维度一旦存在更具体级别，就完全忽略更宽泛级别
//     （"给某人单独收紧"才不会被角色的宽授权冲掉）。
//   - 同级别多条策略取并集（管理员常按主体分段配置，交集会把多条策略互相抵消成空集）。
//   - 未配置任何启用策略 = Enforced=false，不改变系统原有行为；配置后即为强制 AND 条件，
//     压过可伪造的 X-Legal-Entity-Id 请求头（这是「强」隔离与 K3 基础归属过滤的区别）。
//   - 历史未归属数据在 entity/dept 维度记为 "0"、project 维度记为 ""，只有显式授权才可见。
package isolate

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

const (
	DimEntity  = "entity"
	DimDept    = "dept"
	DimProject = "project"
)

type Policy struct {
	ID              int64
	Name            string
	Dimension       string
	SubjectType     string // user / role / all
	Subject         string
	Values          []string
	IncludeChildren bool
	Enabled         bool
	Priority        int
}

type Subject struct {
	User string
	Role string
}

type Object struct {
	EntityID int64
	DeptID   int64
	Project  string
}

// Expander 由调用方提供：按父子树把 allowed_values 展开成最终允许集。
// 纯逻辑包不依赖数据库/组织树，便于单测与复用。
type Expander func(dimension string, values []string, includeChildren bool) []string

type Decision struct {
	Enforced     bool
	Allowed      map[string][]string // 维度 → 展开后的允许值（字符串形式）
	EntityIDs    []int64             // 供业务列表 WhereIn 注入
	DeptIDs      []int64
	Projects     []string
	Matched      []int64
	Overrides    []string // 哪些维度走了 user / role 级覆盖
	SourceLabels map[string]string
}

func validDim(d string) bool {
	return d == DimEntity || d == DimDept || d == DimProject
}

func subjectRank(p Policy, s Subject) int {
	switch p.SubjectType {
	case "user":
		if p.Subject != "" && p.Subject == s.User {
			return 3
		}
	case "role":
		if p.Subject != "" && p.Subject == s.Role {
			return 2
		}
	case "all", "*", "":
		return 1
	}
	return 0
}

func dedup(in []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(in))
	for _, v := range in {
		v = strings.TrimSpace(v)
		if v == "" || seen[v] {
			continue
		}
		seen[v] = true
		out = append(out, v)
	}
	sort.Strings(out)
	return out
}

// Resolve 计算当前用户生效的隔离范围。
func Resolve(policies []Policy, s Subject, expand Expander) Decision {
	d := Decision{Allowed: map[string][]string{}, SourceLabels: map[string]string{}}
	for _, dim := range []string{DimEntity, DimDept, DimProject} {
		bestRank, chosen := 0, []Policy{}
		for _, p := range policies {
			if !p.Enabled || !validDim(p.Dimension) || p.Dimension != dim {
				continue
			}
			rank := subjectRank(p, s)
			if rank == 0 {
				continue
			}
			if rank > bestRank {
				bestRank, chosen = rank, []Policy{p}
			} else if rank == bestRank {
				chosen = append(chosen, p)
			}
		}
		if bestRank == 0 || len(chosen) == 0 {
			continue
		}
		values := []string{}
		for _, p := range chosen {
			d.Matched = append(d.Matched, p.ID)
			if expand != nil {
				values = append(values, expand(dim, dedup(p.Values), p.IncludeChildren)...)
			} else {
				values = append(values, dedup(p.Values)...)
			}
			if p.SubjectType == "user" || p.SubjectType == "role" {
				d.SourceLabels[dim] = p.SubjectType + ":" + p.Subject + " / " + p.Name
			} else {
				d.SourceLabels[dim] = "all / " + p.Name
			}
		}
		values = dedup(values)
		if len(values) == 0 {
			// 配了策略但一个允许值都没有 → 显式「什么都不给看」，仍然算强制
			d.Enforced = true
			d.Allowed[dim] = []string{}
			continue
		}
		d.Enforced = true
		d.Allowed[dim] = values
		if bestRank > 1 {
			d.Overrides = append(d.Overrides, dim)
		}
		for _, v := range values {
			switch dim {
			case DimEntity:
				if id, err := strconv.ParseInt(v, 10, 64); err == nil {
					d.EntityIDs = append(d.EntityIDs, id)
				}
			case DimDept:
				if id, err := strconv.ParseInt(v, 10, 64); err == nil {
					d.DeptIDs = append(d.DeptIDs, id)
				}
			case DimProject:
				d.Projects = append(d.Projects, v)
			}
		}
	}
	sort.Slice(d.Matched, func(i, j int) bool { return d.Matched[i] < d.Matched[j] })
	return d
}

// Allow 判定单个业务对象是否在当前生效范围内（对象级越权拦截用）。
func (d Decision) Allow(o Object) (bool, string) {
	if !d.Enforced {
		return true, ""
	}
	checks := []struct {
		dim   string
		value string
	}{
		{DimEntity, strconv.FormatInt(o.EntityID, 10)},
		{DimDept, strconv.FormatInt(o.DeptID, 10)},
		{DimProject, o.Project},
	}
	for _, c := range checks {
		allowed, ok := d.Allowed[c.dim]
		if !ok {
			continue
		}
		hit := false
		for _, v := range allowed {
			if v == "*" || v == c.value {
				hit = true
				break
			}
		}
		if !hit {
			label := map[string]string{DimEntity: "法人主体", DimDept: "组织部门", DimProject: "项目"}[c.dim]
			return false, fmt.Sprintf("%s=%s 不在授权范围 %v", label, c.value, allowed)
		}
	}
	return true, ""
}
