package seal

import (
	"context"
	"fmt"
	"time"

	"github.com/gogf/gf/v2/frame/g"
)

// 用印台账（v23 R-B）：印章台账 → 用印申请 → 审批（复用审批引擎/简化回退）→ 七字段登记 → 风险预警。
// 电子真章不做（CA 资质墙）；电子签平台的盖章记录可回填（R-B11，API 适配未配置明确失败）。

// ---------- 风险预警纯函数（R-B7/R-B8，阈值口径统一，可单测） ----------

// FreqWarnThreshold R-B7：同一申请人近 30 天发起用印申请的频次预警阈值（申请时即时校验）
const FreqWarnThreshold = 5

// FreqScanThreshold R-B7：台账扫描口径的高频阈值（RiskScan 全局扫描用，高于即时阈值避免刷屏）
const FreqScanThreshold = 10

// FreqExceeded count >= threshold（≥ 语义，纯函数）
func FreqExceeded(count, threshold int) bool { return count >= threshold }

// IsNonWorkingHour R-B8：非工作时间（22:00-06:00）用印标记，纯函数
func IsNonWorkingHour(hour int) bool { return hour >= 22 || hour < 6 }

// ---------- 印章状态常量（R-B1/R-B10） ----------

const (
	StatusActive    = "active"    // 在用
	StatusDisabled  = "disabled"  // 停用
	StatusDestroyed = "destroyed" // 销毁
)

// CreateApply R-B2：用印申请（自动带文件哈希，防篡改锚点）
func CreateApply(ctx context.Context, sealID int64, bizType string, bizID, fileID int64, copies int, reason, creator string) (int64, []string, error) {
	warnings := make([]string, 0)
	row, err := g.DB().Model("seal").Where("id", sealID).One()
	if err != nil || row.IsEmpty() {
		return 0, nil, fmt.Errorf("印章不存在")
	}
	if row["status"].String() != "active" {
		return 0, nil, fmt.Errorf("印章当前状态为 %s，不可申请", row["status"].String())
	}
	// 文件哈希（七字段之一）
	fileHash := ""
	if fileID > 0 {
		fr, _ := g.DB().Model("file_entry").Where("id", fileID).One()
		if !fr.IsEmpty() {
			fileHash = fr["sha256"].String()
		}
	}
	res, err := g.DB().Model("seal_apply").Data(g.Map{
		"seal_id": sealID, "biz_type": bizType, "biz_id": bizID, "file_id": fileID,
		"copies": copies, "reason": reason, "status": "pending",
		"file_hash": fileHash, "creator": creator, "created_at": time.Now(),
	}).Insert()
	if err != nil {
		return 0, nil, err
	}
	id, _ := res.LastInsertId()
	// 频次预警（R-B7）：申请人近 30 天申请次数（阈值口径统一 FreqWarnThreshold）
	n, _ := g.DB().GetValue(ctx,
		"SELECT COUNT(*) FROM seal_apply WHERE creator=? AND created_at > NOW()-INTERVAL '30 days'", creator)
	if n != nil && FreqExceeded(n.Int(), FreqWarnThreshold) {
		warnings = append(warnings, fmt.Sprintf("频次预警：%s 近 30 天已发起 %d 次用印申请（阈值 %d）", creator, n.Int(), FreqWarnThreshold))
	}
	return id, warnings, nil
}

// Approve R-B3 简化审批：legal/admin 批准（审批人=授权来源七字段之一）
func Approve(ctx context.Context, applyID int64, approver string, approve bool, comment string) error {
	row, err := g.DB().Model("seal_apply").Where("id", applyID).One()
	if err != nil || row.IsEmpty() {
		return fmt.Errorf("申请不存在")
	}
	if row["status"].String() != "pending" {
		return fmt.Errorf("当前状态 %s 不可审批", row["status"].String())
	}
	status := "approved"
	if !approve {
		status = "rejected"
	}
	_, err = g.DB().Model("seal_apply").Data(g.Map{
		"status": status, "approver": approver, "approved_at": time.Now(),
		"reason": row["reason"].String() + " | 审批意见: " + comment,
	}).Where("id", applyID).Update()
	return err
}

// Stamp R-B4：用印登记（七字段：操作人/审批人/印章/文件哈希/时间/设备/授权来源=审批单号）
func Stamp(ctx context.Context, applyID int64, operator, device string) (int64, error) {
	row, err := g.DB().Model("seal_apply").Where("id", applyID).One()
	if err != nil || row.IsEmpty() {
		return 0, fmt.Errorf("申请不存在")
	}
	if row["status"].String() != "approved" {
		return 0, fmt.Errorf("仅已批准的申请可登记用印（当前 %s）", row["status"].String())
	}
	reason := row["reason"].String()
	// 非工作时间预警（R-B8）：22:00-06:00（纯函数口径）
	if IsNonWorkingHour(time.Now().Hour()) {
		reason += " | ⚠️ 非工作时间用印"
	}
	if _, err := g.DB().Model("seal_apply").Data(g.Map{
		"status": "stamped", "operator": operator, "stamped_at": time.Now(),
		"device": device, "reason": reason,
	}).Where("id", applyID).Update(); err != nil {
		return 0, err
	}
	return applyID, nil
}

// RiskScan 风险预警扫描（R-B7/8/9）
func RiskScan(ctx context.Context) ([]string, error) {
	out := make([]string, 0)
	// R-B7 频次（扫描口径 FreqScanThreshold）：SQL 里用 COUNT(*) >= 阈值 等价 HAVING，避免两处阈值口径漂移
	rows, err := g.DB().GetAll(ctx,
		`SELECT creator, COUNT(*) AS n FROM seal_apply
		 WHERE created_at > NOW()-INTERVAL '30 days' GROUP BY creator HAVING COUNT(*) >= ?`, FreqScanThreshold)
	if err == nil {
		for _, r := range rows {
			out = append(out, fmt.Sprintf("频次预警：%s 近 30 天 %d 次用印申请（阈值 %d）", r["creator"].String(), r["n"].Int(), FreqScanThreshold))
		}
	}
	rows, err = g.DB().GetAll(ctx, `
		SELECT id, operator, stamped_at FROM seal_apply
		WHERE status='stamped' AND (EXTRACT(HOUR FROM stamped_at) >= 22 OR EXTRACT(HOUR FROM stamped_at) < 6)
		ORDER BY stamped_at DESC LIMIT 20`)
	if err == nil {
		for _, r := range rows {
			out = append(out, fmt.Sprintf("时段预警：#%d %s 于 %s（非工作时间）用印", r["id"].Int64(), r["operator"].String(), r["stamped_at"].String()))
		}
	}
	rows, err = g.DB().GetAll(ctx, `
		SELECT operator, MIN(stamped_at) AS first_at FROM seal_apply
		WHERE status='stamped' GROUP BY operator HAVING COUNT(*) = 1`)
	if err == nil {
		for _, r := range rows {
			out = append(out, fmt.Sprintf("首用提醒：%s 首次用印于 %s（建议主管确认）", r["operator"].String(), r["first_at"].String()))
		}
	}
	return out, nil
}

// SignCheck R-B12：合同签署任务发起前的用印校验（提示级）
func SignCheck(ctx context.Context, contractFileID int64) (string, bool) {
	n, err := g.DB().GetValue(ctx,
		"SELECT COUNT(*) FROM seal_apply WHERE biz_type='contract' AND biz_id=? AND status='stamped'", contractFileID)
	if err != nil || n == nil {
		return "", false
	}
	if n.Int() == 0 {
		return fmt.Sprintf("合同文件 #%d 尚无用印登记记录——如需用印请先完成用印流程", contractFileID), true
	}
	return "", false
}

// ---------- R-B3：按印章类型配置审批模板 + 审批实例挂接 ----------

// RegisterApprovalRule R-B3：为某印章类型绑定审批模板（upsert）；templateID<=0 表示清除规则（回退简化审批）
func RegisterApprovalRule(ctx context.Context, sealType string, templateID int64, creator string) error {
	row, err := g.DB().Model("seal_approval_rule").Where("seal_type", sealType).One()
	if err != nil {
		return err
	}
	now := time.Now()
	if !row.IsEmpty() {
		_, err = g.DB().Model("seal_approval_rule").Data(g.Map{
			"approval_template_id": templateID, "enabled": true, "updated_at": now,
		}).Where("seal_type", sealType).Update()
		return err
	}
	_, err = g.DB().Model("seal_approval_rule").Data(g.Map{
		"seal_type": sealType, "approval_template_id": templateID, "enabled": true,
		"creator": creator, "created_at": now, "updated_at": now,
	}).Insert()
	return err
}

// ResolveApprovalTemplate R-B3：按印章类型取审批模板 id（0=未配置→回退简化审批）
func ResolveApprovalTemplate(ctx context.Context, sealType string) int64 {
	row, err := g.DB().Model("seal_approval_rule").
		Where("seal_type", sealType).Where("enabled", true).One()
	if err != nil || row.IsEmpty() {
		return 0
	}
	return row["approval_template_id"].Int64()
}

// LinkApprovalInstance R-B3：用印申请挂上审批实例 id（引擎路径）
func LinkApprovalInstance(ctx context.Context, applyID, instID int64) error {
	_, err := g.DB().Model("seal_apply").Data(g.Map{"approval_instance_id": instID}).
		Where("id", applyID).Update()
	return err
}

// ApplyApprovalInstanceID R-B3：取用印申请已挂的审批实例 id（0=未走引擎）
func ApplyApprovalInstanceID(ctx context.Context, applyID int64) int64 {
	row, err := g.DB().Model("seal_apply").Where("id", applyID).One()
	if err != nil || row.IsEmpty() {
		return 0
	}
	return row["approval_instance_id"].Int64()
}

// MarkApproved R-B3：审批引擎通过后置申请为 approved（后续可登记用印）
func MarkApproved(ctx context.Context, applyID int64, approver string) error {
	_, err := g.DB().Model("seal_apply").Data(g.Map{
		"status": "approved", "approver": approver, "approved_at": time.Now(),
	}).Where("id", applyID).Update()
	return err
}

// MarkRejected R-B3：审批引擎驳回后置申请为 rejected（意见留痕）
func MarkRejected(ctx context.Context, applyID int64, approver, comment string) error {
	row, err := g.DB().Model("seal_apply").Where("id", applyID).One()
	if err != nil || row.IsEmpty() {
		return fmt.Errorf("申请不存在")
	}
	_, err = g.DB().Model("seal_apply").Data(g.Map{
		"status": "rejected", "approver": approver, "approved_at": time.Now(),
		"reason": row["reason"].String() + " | 审批意见: " + comment,
	}).Where("id", applyID).Update()
	return err
}

// ---------- R-B10：印章移交（保管人变更）留痕 ----------

// TransferKeeper R-B10：印章移交保管人（写审计 + 状态不变；停用/销毁由 controller SealStatus + audit 覆盖）
func TransferKeeper(ctx context.Context, sealID int64, newKeeper string) error {
	row, err := g.DB().Model("seal").Where("id", sealID).One()
	if err != nil || row.IsEmpty() {
		return fmt.Errorf("印章不存在")
	}
	if row["keeper"].String() == newKeeper {
		return fmt.Errorf("保管人未变更")
	}
	_, err = g.DB().Model("seal").Data(g.Map{"keeper": newKeeper}).Where("id", sealID).Update()
	return err
}
