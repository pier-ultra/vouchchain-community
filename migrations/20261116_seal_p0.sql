-- v23 线B 用印管理（轻量台账版）P0 补格（2026-11-16）
-- 本轮：R-B3 用印审批流——复用审批引擎 + 按印章类型配置审批模板；R-B6 台账导出由 controller 提供。
-- 铁律不变：电子真章不做（CA 资质墙）、物理印章 IoT 印控设备不做、静默批量盖章不做。
-- 向后兼容：未配置 seal_approval_rule 时走原有简化审批（legal/admin 单步批），配置后走审批引擎。
-- 幂等，可重复执行；服务启动迁移器按文件名顺序自动应用（20261116_revrec_p0.sql 之后）。

-- R-B3：按印章类型配置审批模板（seal_type → approval_template_id）
CREATE TABLE IF NOT EXISTS seal_approval_rule (
  id BIGSERIAL PRIMARY KEY,
  seal_type VARCHAR(20) NOT NULL UNIQUE,          -- contract合同章/official公章/finance财务章/legal法人章/other
  approval_template_id BIGINT NOT NULL DEFAULT 0, -- approval_template.id（0=未配置→回退简化审批）
  enabled BOOLEAN NOT NULL DEFAULT TRUE,
  creator VARCHAR(100) NOT NULL DEFAULT '',
  created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- R-B3：用印申请挂审批实例（审批引擎产物），0=未走引擎（简化路径）
ALTER TABLE seal_apply ADD COLUMN IF NOT EXISTS approval_instance_id BIGINT NOT NULL DEFAULT 0;
