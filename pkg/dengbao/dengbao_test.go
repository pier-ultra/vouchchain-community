package dengbao

import (
	"strings"
	"testing"
	"time"

	"github.com/gogf/gf/v2/container/gvar"
	"github.com/gogf/gf/v2/database/gdb"
)

// R-C2：留存期限口径（等保 2.0 安全审计不少于 6 个月）
func TestRetentionCompliant(t *testing.T) {
	cases := []struct {
		days int
		want bool
	}{
		{179, false},
		{180, true},
		{365, true},
		{0, false},
		{-1, false},
	}
	for _, c := range cases {
		if got := RetentionCompliant(c.days); got != c.want {
			t.Fatalf("RetentionCompliant(%d) = %v，期望 %v", c.days, got, c.want)
		}
	}
}

func TestValidateRetention(t *testing.T) {
	if err := ValidateRetention(365); err != nil {
		t.Fatalf("365 天应通过，得到：%v", err)
	}
	if err := ValidateRetention(90); err == nil {
		t.Fatal("90 天低于等保下限，必须被拒")
	} else if !strings.Contains(err.Error(), "180") {
		t.Fatalf("错误信息应带下限天数：%v", err)
	}
}

func TestCutoff(t *testing.T) {
	now := time.Date(2026, 11, 16, 0, 0, 0, 0, time.UTC)
	want := time.Date(2026, 5, 20, 0, 0, 0, 0, time.UTC)
	if got := Cutoff(now, RetentionFloorDays); !got.Equal(want) {
		t.Fatalf("Cutoff = %v，期望 %v", got, want)
	}
}

func TestCoverageOK(t *testing.T) {
	now := time.Date(2026, 11, 16, 0, 0, 0, 0, time.UTC)
	enough := now.AddDate(0, 0, -200)
	short := now.AddDate(0, 0, -30)
	if !CoverageOK(enough, now, RetentionFloorDays) {
		t.Fatal("覆盖 200 天应判连续")
	}
	if CoverageOK(short, now, RetentionFloorDays) {
		t.Fatal("只覆盖 30 天应判断链")
	}
	if CoverageOK(time.Time{}, now, RetentionFloorDays) {
		t.Fatal("无日志时应判不连续")
	}
}

// R-C3/R-C4：证据块分类口径（单一事实源，Go 判定与 SQL 过滤共用）
func TestIsPermissionChange(t *testing.T) {
	cases := map[string]bool{
		"rbac.role.update":   true,
		"rbac.binding.bind":  true,
		"scim.user.purge":    true,
		"scim.role_map.save": true,
		"seal.create":        false,
		"audit_log.export":   false,
		"xrbac.role.update":  false,
	}
	for action, want := range cases {
		if got := IsPermissionChange(action); got != want {
			t.Fatalf("IsPermissionChange(%q) = %v，期望 %v", action, got, want)
		}
	}
}

func TestClassifyBlock(t *testing.T) {
	cases := map[string]Block{
		"auth.login":              BlockAuth,
		"context.switch":          BlockAuth,
		"scim.user.purge":         BlockAuth,
		"rbac.role.update":        BlockAccess,
		"isolation.policy.toggle": BlockAccess,
		"compliance.rule.create":  BlockAccess,
		"seal.approval_rule":      BlockAccess,
		"seal.create":             BlockAudit,
		"contract.create":         BlockAudit,
	}
	for action, want := range cases {
		if got := ClassifyBlock(action); got != want {
			t.Fatalf("ClassifyBlock(%q) = %v，期望 %v", action, got, want)
		}
	}
}

func TestBlockWhere(t *testing.T) {
	if BlockWhere(BlockAudit) != "" {
		t.Fatal("安全审计块=全量，过滤条件应为空")
	}
	auth := BlockWhere(BlockAuth)
	if !strings.HasPrefix(auth, "action ~ '^(") || !strings.HasSuffix(auth, ")'") {
		t.Fatalf("身份鉴别过滤条件异常：%s", auth)
	}
	if !strings.Contains(auth, `auth\.`) {
		t.Fatalf("点号必须转义，避免误匹配：%s", auth)
	}
	perm := PermissionWhere()
	if !strings.Contains(perm, `scim\.user\.`) || !strings.Contains(perm, `rbac\.`) {
		t.Fatalf("权限变更过滤条件异常：%s", perm)
	}
	if PermissionWhere() == BlockWhere(BlockAccess) {
		t.Fatal("权限变更口径与访问控制口径不同，不应复用同一串")
	}
}

// R-C9：证据包命名与打包产物
func TestEvidencePackName(t *testing.T) {
	name := EvidencePackName(time.Date(2026, 11, 16, 4, 5, 6, 0, time.UTC))
	if name != "dengbao-evidence-20261116-040506.zip" {
		t.Fatalf("证据包文件名异常：%s", name)
	}
}

func TestEvidenceTableHeaders(t *testing.T) {
	// 导出已改 Excel：证据列口径走表头+数据行双通道（不再产 CSV）
	rec := []gdb.Record{{
		"scope": gvar.New("live"), "id": gvar.New(int64(1)), "username": gvar.New("admin"),
		"action": gvar.New("rbac.binding.bind"), "target_type": gvar.New("role"),
		"target_id": gvar.New(int64(0)), "detail": gvar.New(`{"roleCode":"legal"}`),
		"ip": gvar.New("127.0.0.1"), "created_at": gvar.New(time.Now()),
	}}
	headers, rows := EvidenceTable(rec)
	for _, label := range []string{"证据来源", "操作人", "动作", "来源IP", "时间"} {
		if !contains(headers, label) {
			t.Fatalf("表头缺少「%s」：%v", label, headers)
		}
	}
	if len(rows) != 1 || !contains(rows[0], "rbac.binding.bind") {
		t.Fatalf("明细内容丢失：%v", rows)
	}
	// 证据包 zip 内的明细必须是 xlsx（PK 头），不能退回 CSV 文本
	b := packXLSX("证据", headers, rows)
	if len(b) < 4 || b[0] != 'P' || b[1] != 'K' {
		t.Fatalf("证据明细应输出 xlsx，实际 %d 字节", len(b))
	}
}

func contains(list []string, v string) bool {
	for _, s := range list {
		if s == v {
			return true
		}
	}
	return false
}

func TestRecordRowsMissingColumn(t *testing.T) {
	// 归档表缺列 / NULL 一律输出空串，不能报错
	rows := recordRows([]gdb.Record{{"scope": gvar.New("archive")}}, evidenceColumns)
	if len(rows) != 1 || len(rows[0]) != len(evidenceColumns) {
		t.Fatalf("列数不匹配：%v", rows)
	}
	if rows[0][1] != "" || rows[0][2] != "" {
		t.Fatalf("缺列应输出空串：%v", rows[0])
	}
}

func TestPackTotalRows(t *testing.T) {
	p := Pack{Manifest: []ManifestEntry{
		{Name: "安全审计证据.xlsx", Rows: 10},
		{Name: "清单.xlsx", Rows: 5},
	}}
	if got := p.TotalRows(); got != 10 {
		t.Fatalf("TotalRows = %d，清单自身不应计入", got)
	}
}

// R-C7 自查结果取值 + 自查模板完整性
func TestSelfcheckResultAndTemplate(t *testing.T) {
	for _, ok := range []string{"", SelfcheckTodo, SelfcheckYes, SelfcheckNo, SelfcheckPartial, SelfcheckNA} {
		if !ValidSelfcheckResult(ok) {
			t.Fatalf("合法取值被拒：%q", ok)
		}
	}
	for _, bad := range []string{"YES", "yes", "unknown", "Y", "部分"} {
		if ValidSelfcheckResult(bad) {
			t.Fatalf("非法取值被放行：%q", bad)
		}
	}
	if len(selfcheckTemplate) < 20 {
		t.Fatalf("自查模板过少：%d 条", len(selfcheckTemplate))
	}
	seen := map[string]bool{}
	for _, item := range selfcheckTemplate {
		if item.Code == "" || item.Item == "" || item.Domain == "" {
			t.Fatalf("模板条目字段缺失：%v", item)
		}
		if seen[item.Code] {
			t.Fatalf("模板编号重复：%s", item.Code)
		}
		seen[item.Code] = true
	}
}
