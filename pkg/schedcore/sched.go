// Package schedcore 归档调度的纯逻辑层（零 DB / 零框架依赖，便于单测与未来抽包）。
// 口径：按「账期」调度——每个账期（周/月/季）结束后第 dueDays 天凌晨执行，
// 只归档上一账期及更早、且满足门槛的发票，避免把当期未走完流程的票提前归档。
package schedcore

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

const runHour = 2 // 凌晨 02:00 执行，避开白天业务高峰

// NormalizePeriod 校验并规范化账期类型
func NormalizePeriod(p string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(p)) {
	case "weekly", "周":
		return "weekly", nil
	case "monthly", "月":
		return "monthly", nil
	case "quarterly", "季":
		return "quarterly", nil
	default:
		return "", fmt.Errorf("periodType 只能是 weekly / monthly / quarterly")
	}
}

// PeriodStart 返回 t 所在账期的起始日 00:00（本地时区）
func PeriodStart(period string, t time.Time) time.Time {
	y, m, d := t.Date()
	start := time.Date(y, m, d, 0, 0, 0, 0, t.Location())
	switch period {
	case "weekly":
		// 周一为一周起点（time.Weekday: Sunday=0）
		offset := (int(start.Weekday()) + 6) % 7
		return start.AddDate(0, 0, -offset)
	case "monthly":
		return time.Date(y, m, 1, 0, 0, 0, 0, t.Location())
	case "quarterly":
		qm := time.Month(((int(m)-1)/3)*3 + 1)
		return time.Date(y, qm, 1, 0, 0, 0, 0, t.Location())
	default:
		return start
	}
}

// NextPeriodStart 返回 t 所在账期的下一个账期起点
func NextPeriodStart(period string, t time.Time) time.Time {
	start := PeriodStart(period, t)
	switch period {
	case "weekly":
		return start.AddDate(0, 0, 7)
	case "monthly":
		return start.AddDate(0, 1, 0)
	case "quarterly":
		return start.AddDate(0, 3, 0)
	default:
		return start.AddDate(0, 0, 1)
	}
}

// NextRun 计算下一次执行时刻：下一账期起点 + (dueDays-1) 天，凌晨 02:00。
// dueDays=1 表示账期结束次日 02:00 执行。
func NextRun(period string, dueDays int, now time.Time) time.Time {
	if dueDays < 1 {
		dueDays = 1
	}
	base := NextPeriodStart(period, now)
	return time.Date(base.Year(), base.Month(), base.Day()+dueDays-1, runHour, 0, 0, 0, now.Location())
}

// Cutoff 本次执行允许归档的最晚开票日：当前账期起点前一天。
// 只归档"上一账期及更早"，当期票留到下个周期。
func Cutoff(period string, now time.Time) string {
	return PeriodStart(period, now).AddDate(0, 0, -1).Format("2006-01-02")
}

// CsvNode CSV 导入的一行组织节点（AK2.5）
type CsvNode struct {
	Code       string
	Name       string
	NodeType   string
	ParentCode string
	SortOrder  int
}

// ParseOrgCsv 解析组织树 CSV：首行表头（中英文均可），列为
// code,name,node_type,parent_code,sort_order（或 编码,名称,类型,上级编码,排序）。
// 返回可导入行与逐行错误；单行出错不影响其它行。
func ParseOrgCsv(records [][]string) ([]CsvNode, []string) {
	if len(records) == 0 {
		return nil, []string{"CSV 为空"}
	}
	rows := records
	if isHeader(records[0]) {
		rows = records[1:]
	}
	out := make([]CsvNode, 0, len(rows))
	errs := []string{}
	for i, row := range rows {
		line := i + 1
		if isHeader(row) {
			continue
		}
		fields := make([]string, 5)
		copy(fields, row)
		code := strings.TrimSpace(fields[0])
		name := strings.TrimSpace(fields[1])
		if name == "" {
			errs = append(errs, fmt.Sprintf("第 %d 行：名称为空，跳过", line))
			continue
		}
		nt := strings.ToLower(strings.TrimSpace(fields[2]))
		switch nt {
		case "dept", "部门", "":
			nt = "dept"
		case "position", "岗位":
			nt = "position"
		default:
			errs = append(errs, fmt.Sprintf("第 %d 行：类型 %q 非法（dept/position）", line, nt))
			continue
		}
		sort := 0
		if s := strings.TrimSpace(fields[4]); s != "" {
			v, err := strconv.Atoi(s)
			if err != nil {
				errs = append(errs, fmt.Sprintf("第 %d 行：排序 %q 不是整数，按 0 处理", line, s))
			} else {
				sort = v
			}
		}
		out = append(out, CsvNode{
			Code: code, Name: name, NodeType: nt,
			ParentCode: strings.TrimSpace(fields[3]), SortOrder: sort,
		})
	}
	return out, errs
}

func isHeader(row []string) bool {
	if len(row) == 0 {
		return false
	}
	f := strings.ToLower(strings.TrimSpace(row[0]))
	return f == "code" || f == "编码" || strings.Contains(f, "编码")
}

// IsDescendant 判断 candidate 是否为 nodeId 自身或其后代；用于组织树改上级时防环。
// parents: nodeId → parentId 映射；纯内存判断，最多向上追 maxDepth 层。
func IsDescendant(nodeId, candidate int64, parents map[int64]int64) bool {
	cur, depth := candidate, 0
	for depth < 100 {
		if cur == nodeId {
			return true
		}
		p, ok := parents[cur]
		if !ok || p <= 0 {
			return false
		}
		cur = p
		depth++
	}
	return true // 链条异常长（疑似已有环）→ 保守拒绝
}
