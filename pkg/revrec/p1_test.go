package revrec

import (
	"testing"
	"time"
)

// R-A6 差额分类：正=合同资产，负=合同负债，零=平衡
func TestClassifyBalance(t *testing.T) {
	cases := []struct {
		conf, set float64
		wantType  string
		wantBal   float64
	}{
		{100, 60, "asset", 40},     // 干得多结算得少 → 合同资产
		{60, 100, "liability", 40}, // 先结算/干得少 → 合同负债
		{100, 100, "even", 0},      // 平衡
		{0, 0, "even", 0},
		{0, 0.001, "even", 0}, // 容差内归平衡
	}
	for _, c := range cases {
		ty, bal := ClassifyBalance(c.conf, c.set)
		if ty != c.wantType || bal != c.wantBal {
			t.Errorf("ClassifyBalance(%v,%v)=(%s,%v) want (%s,%v)", c.conf, c.set, ty, bal, c.wantType, c.wantBal)
		}
	}
}

// R-A7 账龄：从最早确认日到今天的天数
func TestAgingDays(t *testing.T) {
	earliest := time.Date(2026, 5, 20, 0, 0, 0, 0, time.Local)
	now := time.Date(2026, 11, 16, 0, 0, 0, 0, time.Local)
	if got := AgingDays(earliest, now); got != 180 {
		t.Errorf("AgingDays=180 got %d", got)
	}
	if got := AgingDays(time.Time{}, now); got != 0 {
		t.Errorf("zero earliest should be 0, got %d", got)
	}
	if got := AgingDays(now.Add(24*time.Hour), now); got != 0 { // 未来确认日
		t.Errorf("future earliest should clamp 0, got %d", got)
	}
}

// R-A11 期间取月
func TestMonthOf(t *testing.T) {
	if MonthOf(time.Date(2026, 11, 5, 0, 0, 0, 0, time.Local)) != "2026-11" {
		t.Error("MonthOf(2026-11-05) != 2026-11")
	}
	if MonthOf(time.Time{}) != "" {
		t.Error("MonthOf(zero) should be empty")
	}
}
