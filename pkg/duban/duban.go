package duban

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/gogf/gf/v2/frame/g"
)

// 公文督办小件（v23 线D R-D1/R-D2）：会议决议/领导批示/其他 → 督办任务 → 期限 → 催办 → 完成留痕。
// 通用公文流转不做（O2OA 已覆盖 GB/T 9704 收发文/套红/归档），只做与合同/义务咬合的督办。

const (
	SourceMeeting   = "meeting"   // 会议决议
	SourceDirective = "directive" // 领导批示
	SourceOther     = "other"     // 其他
)

const (
	StatusTodo    = "todo"    // 进行中
	StatusDone    = "done"    // 已完成
	StatusOverdue = "overdue" // 逾期（派生态：未完成且过期限，不落库）
)

const (
	PriorityHigh = "high"
	PriorityMid  = "mid"
	PriorityLow  = "low"
)

// validSourceType 来源类型校验（纯函数）
func validSourceType(s string) bool {
	return s == SourceMeeting || s == SourceDirective || s == SourceOther
}

// validPriority 优先级校验（纯函数）
func validPriority(s string) bool {
	return s == PriorityHigh || s == PriorityMid || s == PriorityLow
}

// IsOverdue 逾期判定（纯函数）：未完成且已过期限当天末尾。期限当天全天有效，次日 0 点整起算逾期。
func IsOverdue(deadline, now time.Time) bool {
	if deadline.IsZero() || now.IsZero() {
		return false
	}
	return !now.Before(deadline.Add(24 * time.Hour))
}

// StatusOf 派生态（纯函数）：已完成保持 done；未完成且逾期 → overdue；其余 todo。
func StatusOf(deadline time.Time, status string, now time.Time) string {
	if status == StatusDone {
		return StatusDone
	}
	if IsOverdue(deadline, now) {
		return StatusOverdue
	}
	return StatusTodo
}

func logAction(ctx context.Context, taskID int64, action, actor, note string) {
	g.DB().Model("duban_log").Data(g.Map{
		"task_id": taskID, "action": action, "actor": actor, "note": note, "created_at": time.Now(),
	}).Insert()
}

// CreateTask R-D1：建督办任务（来源类型 + 责任人 + 期限 + 优先级）
func CreateTask(ctx context.Context, title, content, owner, sourceType, priority, deadlineStr string, cfID int64, remark, creator string) (int64, error) {
	if title == "" {
		return 0, fmt.Errorf("督办标题不能为空")
	}
	if !validSourceType(sourceType) {
		return 0, fmt.Errorf("来源类型只能是 meeting/directive/other")
	}
	if !validPriority(priority) {
		return 0, fmt.Errorf("优先级只能是 high/mid/low")
	}
	data := g.Map{
		"source_type": sourceType, "title": title, "content": content, "owner": owner,
		"creator": creator, "priority": priority, "status": StatusTodo,
		"contract_file_id": cfID, "remark": remark, "created_at": time.Now(), "updated_at": time.Now(),
	}
	if deadlineStr != "" {
		if _, err := time.Parse("2006-01-02", deadlineStr); err != nil {
			return 0, fmt.Errorf("期限格式需 YYYY-MM-DD")
		}
		data["deadline"] = deadlineStr
	}
	res, err := g.DB().Model("duban_task").Data(data).Insert()
	if err != nil {
		return 0, err
	}
	id, _ := res.LastInsertId()
	logAction(ctx, id, "create", creator, "建督办任务")
	return id, nil
}

// BoardFilter 看板/列表过滤条件
type BoardFilter struct {
	Status     string // todo / overdue / done（按派生态过滤）
	Priority   string // high / mid / low
	SourceType string // meeting / directive / other
	OnlyOwner  string // 只看我负责的
}

// ListTasks R-D2：督办任务列表（按条件过滤 + 派生态计算）
func ListTasks(ctx context.Context, f BoardFilter, limit int) ([]g.Map, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	m := g.DB().Model("duban_task")
	if f.SourceType != "" {
		m = m.Where("source_type", f.SourceType)
	}
	if f.Priority != "" {
		m = m.Where("priority", f.Priority)
	}
	if f.OnlyOwner != "" {
		m = m.Where("owner", f.OnlyOwner)
	}
	if f.Status == StatusDone {
		m = m.Where("status", StatusDone)
	}
	if f.Status == StatusTodo || f.Status == StatusOverdue {
		m = m.Where("status", StatusTodo)
	}
	rows, err := m.Order("priority asc, deadline asc nulls last, id desc").Limit(limit).All()
	if err != nil {
		return nil, err
	}
	now := time.Now()
	out := make([]g.Map, 0, len(rows))
	for _, r := range rows {
		var dl time.Time
		if v := r["deadline"]; !v.IsNil() {
			dl = v.Time()
		}
		st := StatusOf(dl, r["status"].String(), now)
		if f.Status != "" && st != f.Status {
			continue
		}
		row := r.Map()
		row["derived_status"] = st
		out = append(out, row)
	}
	return out, nil
}

// Board R-D2：督办看板（待办/逾期/已完成三列）
func Board(ctx context.Context, f BoardFilter) (map[string][]g.Map, error) {
	rows, err := ListTasks(ctx, f, 500)
	if err != nil {
		return nil, err
	}
	board := map[string][]g.Map{StatusTodo: {}, StatusOverdue: {}, StatusDone: {}}
	for _, r := range rows {
		k := r["derived_status"].(string)
		board[k] = append(board[k], r)
	}
	return board, nil
}

// Remind 催办：计数 + 留痕（不改状态）
func Remind(ctx context.Context, taskID int64, actor, note string) error {
	row, err := g.DB().Model("duban_task").Where("id", taskID).One()
	if err != nil || row.IsEmpty() {
		return fmt.Errorf("督办任务不存在")
	}
	if row["status"].String() == StatusDone {
		return fmt.Errorf("任务已完成，无需催办")
	}
	n := row["remind_cnt"].Int() + 1
	g.DB().Model("duban_task").Data(g.Map{
		"remind_cnt": n, "last_remind_at": time.Now(), "updated_at": time.Now(),
	}).Where("id", taskID).Update()
	logAction(ctx, taskID, "remind", actor, note)
	return nil
}

// Done 完成督办（留痕，不可再催办）
func Done(ctx context.Context, taskID int64, actor, note string) error {
	row, err := g.DB().Model("duban_task").Where("id", taskID).One()
	if err != nil || row.IsEmpty() {
		return fmt.Errorf("督办任务不存在")
	}
	g.DB().Model("duban_task").Data(g.Map{
		"status": StatusDone, "updated_at": time.Now(),
	}).Where("id", taskID).Update()
	logAction(ctx, taskID, "done", actor, note)
	return nil
}

// LogList 任务留痕（时间正序 = 督办链顺序）
func LogList(ctx context.Context, taskID int64, limit int) ([]g.Map, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	rows, err := g.DB().Model("duban_log").Where("task_id", taskID).Order("id asc").Limit(limit).All()
	if err != nil {
		return nil, err
	}
	out := make([]g.Map, 0, len(rows))
	for _, r := range rows {
		out = append(out, r.Map())
	}
	return out, nil
}

// Stats 督办统计：总数/待办/逾期/已完成 + 各来源类型分布
func Stats(ctx context.Context) (map[string]any, error) {
	rows, err := g.DB().GetAll(ctx, `SELECT source_type, status, deadline, COUNT(*) AS cnt FROM duban_task GROUP BY 1,2,3`)
	if err != nil {
		return nil, err
	}
	now := time.Now()
	bySource := map[string]int{}
	var total, todo, overdue, done int
	for _, r := range rows {
		cnt := int(r["cnt"].Int())
		src := r["source_type"].String()
		bySource[src] += cnt
		total += cnt
		var dl time.Time
		if v := r["deadline"]; !v.IsNil() {
			dl = v.Time()
		}
		switch StatusOf(dl, r["status"].String(), now) {
		case StatusDone:
			done += cnt
		case StatusOverdue:
			overdue += cnt
		default:
			todo += cnt
		}
	}
	return map[string]any{
		"total": total, "todo": todo, "overdue": overdue, "done": done,
		"bySource": bySource, "sourceOrder": sortSources(bySource),
	}, nil
}

// sortSources 来源类型稳定排序（看板展示用）
func sortSources(s map[string]int) []string {
	keys := make([]string, 0, len(s))
	for k := range s {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
