package invoice_redflush

import "testing"

func TestDetect(t *testing.T) {
	cases := []struct {
		name, invoiceType string
		total             float64
		want              string
	}{
		{"红字_类型含红", "增值税电子普通发票（红字）", 100, StatusRed},
		{"红字_类型含红字", "红字发票", 50, StatusRed},
		{"红字_金额为负", "增值税专用发票", -100.5, StatusRed},
		{"红字_类型含红且金额负", "数电票（红字）", -88, StatusRed},
		{"正常_蓝字正数", "增值税专用发票", 100, ""},
		{"正常_零税额", "增值税普通发票", 0, ""},
		{"正常_类型不含红但金额正", "电子发票（普通）", 13.6, ""},
	}
	for _, c := range cases {
		if got := Detect(c.invoiceType, c.total); got != c.want {
			t.Errorf("%s: Detect(%q, %v) = %q, want %q", c.name, c.invoiceType, c.total, got, c.want)
		}
	}
}
