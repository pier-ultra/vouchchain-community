package duban

import (
	"testing"
	"time"
)

// R-D1 逾期判定：期限当天全天有效，次日起算逾期
func TestIsOverdue(t *testing.T) {
	dl := time.Date(2026, 11, 15, 0, 0, 0, 0, time.Local)
	// 期限当天 23:59 未逾期
	if IsOverdue(dl, time.Date(2026, 11, 15, 23, 59, 0, 0, time.Local)) {
		t.Error("deadline day 23:59 should NOT be overdue")
	}
	// 次日 00:00 起逾期
	if !IsOverdue(dl, time.Date(2026, 11, 16, 0, 0, 1, 0, time.Local)) {
		t.Error("next day 00:00 should be overdue")
	}
	// 空期限不逾期
	if IsOverdue(time.Time{}, time.Now()) {
		t.Error("zero deadline should never be overdue")
	}
}

// R-D1 派生态：done 保持 done；未完成逾期→overdue；否则 todo
func TestStatusOf(t *testing.T) {
	dl := time.Date(2026, 11, 15, 0, 0, 0, 0, time.Local)
	now := time.Date(2026, 11, 16, 0, 0, 0, 0, time.Local)
	if StatusOf(dl, StatusDone, now) != StatusDone {
		t.Error("done stays done")
	}
	if StatusOf(dl, StatusTodo, now) != StatusOverdue {
		t.Error("todo+expired should be overdue")
	}
	if StatusOf(dl, StatusTodo, time.Date(2026, 11, 14, 0, 0, 0, 0, time.Local)) != StatusTodo {
		t.Error("todo+not expired should be todo")
	}
}

// R-D1 来源类型/优先级校验
func TestValidators(t *testing.T) {
	if !validSourceType(SourceMeeting) || !validSourceType(SourceDirective) || !validSourceType(SourceOther) {
		t.Error("valid source types rejected")
	}
	if validSourceType("xxx") {
		t.Error("invalid source type accepted")
	}
	if !validPriority(PriorityHigh) || validPriority("x") {
		t.Error("priority validator wrong")
	}
}
