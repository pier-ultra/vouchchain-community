package seal

import (
	"strings"
	"testing"
)

// R-B7 频次预警阈值（口径统一，纯函数可测）
func TestFreqExceeded(t *testing.T) {
	cases := []struct {
		count, threshold int
		want             bool
	}{
		{4, 5, false}, // 未达阈值
		{5, 5, true},  // 等于阈值（≥ 语义）
		{6, 5, true},  // 超过阈值
		{0, 5, false},
		{11, 10, true},
		{10, 10, true},
		{9, 10, false},
	}
	for _, c := range cases {
		if got := FreqExceeded(c.count, c.threshold); got != c.want {
			t.Fatalf("FreqExceeded(%d, %d) = %v，期望 %v", c.count, c.threshold, got, c.want)
		}
	}
}

// R-B7 两级阈值：即时预警 < 全局扫描阈值（避免扫描刷屏）
func TestThresholdOrdering(t *testing.T) {
	if FreqWarnThreshold >= FreqScanThreshold {
		t.Fatalf("即时预警阈值(%d) 应小于扫描阈值(%d)", FreqWarnThreshold, FreqScanThreshold)
	}
}

// R-B8 非工作时间（22:00-06:00），纯函数可测
func TestIsNonWorkingHour(t *testing.T) {
	for h := 0; h < 24; h++ {
		want := h >= 22 || h < 6
		if got := IsNonWorkingHour(h); got != want {
			t.Fatalf("IsNonWorkingHour(%d) = %v，期望 %v", h, got, want)
		}
	}
}

// R-B10 印章状态常量（供状态机/留痕使用）
func TestSealStatusConstants(t *testing.T) {
	if StatusActive != "active" || StatusDisabled != "disabled" || StatusDestroyed != "destroyed" {
		t.Fatalf("状态常量异常：%s/%s/%s", StatusActive, StatusDisabled, StatusDestroyed)
	}
}

// R-B11 电子签回填：原始返回截断（不占满库、不丢结尾标记）
func TestTruncate(t *testing.T) {
	if got := truncate("abc", 10); got != "abc" {
		t.Fatalf("短文本应原样返回：%s", got)
	}
	long := strings.Repeat("测", 100)
	got := truncate(long, 10)
	if !strings.HasSuffix(got, "…") || len([]rune(got)) != 11 {
		t.Fatalf("超长文本应截断并带省略号：%s", got)
	}
}
