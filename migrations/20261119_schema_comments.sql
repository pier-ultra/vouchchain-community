-- schema 注释元数据：表注释 + 字段注释（2026-11-19）
-- 由 internal/bootstrap/comments.go 在启动迁移后幂等同步到各数据库元数据层：
--   pgsql / opengauss / dm / kingbase：原生执行 COMMENT ON（dm/kingbase 保留字列名自动加引号）；
--   mysql（MySQL/TiDB/OceanBase）：表注释转 ALTER ... COMMENT，字段注释按 information_schema 比对后 MODIFY；
--   sqlite：无注释机制，跳过。
-- 本文件不进 schema_migrations 标准循环（RunMigrations 内已跳过），每次启动重放对齐。
-- 注：注释文本不得包含半角单引号与分号。

-- ============ 合同台账（0001_core_tables） ============
COMMENT ON TABLE contract IS '合同台账';
COMMENT ON COLUMN contract.id IS '主键';
COMMENT ON COLUMN contract.title IS '合同名称';
COMMENT ON COLUMN contract.counter_party IS '合同相对方';
COMMENT ON COLUMN contract.amount IS '合同金额';
COMMENT ON COLUMN contract.currency IS '币种，默认 CNY';
COMMENT ON COLUMN contract.sign_date IS '签订日期 YYYY-MM-DD';
COMMENT ON COLUMN contract.status IS '状态，draft 草稿 / active 履行中 / done 已完结';
COMMENT ON COLUMN contract.legal_entity_id IS '归属法人主体 id，0 未归属';
COMMENT ON COLUMN contract.dept_id IS '归属部门 id，0 未归属';
COMMENT ON COLUMN contract.project_code IS '项目编码，隔离维度之一';
COMMENT ON COLUMN contract.owner IS '创建人，self/dept 数据可见范围依据';
COMMENT ON COLUMN contract.created_at IS '创建时间';
COMMENT ON COLUMN contract.updated_at IS '更新时间';

COMMENT ON TABLE contract_file IS '合同文件与 AI 抽取流水线';
COMMENT ON COLUMN contract_file.id IS '主键';
COMMENT ON COLUMN contract_file.filename IS '上传文件名';
COMMENT ON COLUMN contract_file.name IS '展示名';
COMMENT ON COLUMN contract_file.path IS '存储路径';
COMMENT ON COLUMN contract_file.file_hash IS '文件哈希，防重复';
COMMENT ON COLUMN contract_file.file_id IS '文件中心 file_entry id';
COMMENT ON COLUMN contract_file.status IS '处理状态，pending / extracting / done / failed';
COMMENT ON COLUMN contract_file.error_msg IS '失败原因';
COMMENT ON COLUMN contract_file.stage IS '当前抽取阶段';
COMMENT ON COLUMN contract_file.progress IS '总进度 0-100';
COMMENT ON COLUMN contract_file.pipeline IS '各阶段明细 JSON';
COMMENT ON COLUMN contract_file.review_state IS '复核状态，in_progress / complete';
COMMENT ON COLUMN contract_file.template IS '模板类型，general / purchase / sales / lease';
COMMENT ON COLUMN contract_file.group_id IS '版本组 id，组内首个文件 id，0 即自身';
COMMENT ON COLUMN contract_file.version_no IS '组内版本号';
COMMENT ON COLUMN contract_file.approval_state IS '审批状态，none / running / approved / rejected';
COMMENT ON COLUMN contract_file.provider IS '抽取模型提供方';
COMMENT ON COLUMN contract_file.text_content IS '全文文本内容';
COMMENT ON COLUMN contract_file.counter_party IS '合同相对方（AI 抽取回填）';
COMMENT ON COLUMN contract_file.legal_entity_id IS '归属法人主体 id';
COMMENT ON COLUMN contract_file.dept_id IS '归属部门 id';
COMMENT ON COLUMN contract_file.project_code IS '项目编码';
COMMENT ON COLUMN contract_file.created_at IS '创建时间';
COMMENT ON COLUMN contract_file.updated_at IS '更新时间';

COMMENT ON TABLE contract_clause IS '合同 AI 抽取出的条款条目';
COMMENT ON COLUMN contract_clause.id IS '主键';
COMMENT ON COLUMN contract_clause.file_id IS '所属合同文件 id';
COMMENT ON COLUMN contract_clause.type IS '条款类型，主体 / 权利义务 / 金额 / 日期 / 违约 / 保密 / 争议解决 / 其他';
COMMENT ON COLUMN contract_clause.summary IS '条款摘要';
COMMENT ON COLUMN contract_clause.quote IS '原文引用';
COMMENT ON COLUMN contract_clause.risk IS '风险等级，high / medium / low / none';
COMMENT ON COLUMN contract_clause.review_status IS '人审状态，unconfirmed / confirmed / doubt';
COMMENT ON COLUMN contract_clause.review_remark IS '人审备注';
COMMENT ON COLUMN contract_clause.created_at IS '创建时间';
COMMENT ON COLUMN contract_clause.updated_at IS '更新时间';

COMMENT ON TABLE obligation IS '义务清单：合同义务到期的履约跟踪';
COMMENT ON COLUMN obligation.id IS '主键';
COMMENT ON COLUMN obligation.file_id IS '所属合同文件 id';
COMMENT ON COLUMN obligation.type IS '义务类型，付款 / 交付 / 报告 / 续约 / 其他';
COMMENT ON COLUMN obligation.description IS '义务描述';
COMMENT ON COLUMN obligation.owner IS '责任人';
COMMENT ON COLUMN obligation.due_date IS '到期日 YYYY-MM-DD';
COMMENT ON COLUMN obligation.source_quote IS '合同原文引用';
COMMENT ON COLUMN obligation.status IS '状态，pending / done';
COMMENT ON COLUMN obligation.remark IS '备注';
COMMENT ON COLUMN obligation.evidence_name IS '完成证据文件名';
COMMENT ON COLUMN obligation.evidence_path IS '完成证据路径';
COMMENT ON COLUMN obligation.completed_at IS '完成时间';
COMMENT ON COLUMN obligation.confirm_mode IS '收入确认方式，point 时点法 / time 时段法投入法';
COMMENT ON COLUMN obligation.share_ratio IS '分摊比例，多义务按单独售价比例';
COMMENT ON COLUMN obligation.tax_rate IS '销项税率，货物 13 / 服务 6';
COMMENT ON COLUMN obligation.planned_cost IS '时段法预计总成本';
COMMENT ON COLUMN obligation.accum_cost IS '时段法累计已发生成本';
COMMENT ON COLUMN obligation.created_at IS '创建时间';
COMMENT ON COLUMN obligation.updated_at IS '更新时间';

COMMENT ON TABLE sign_task IS '电子签任务';
COMMENT ON COLUMN sign_task.id IS '主键';
COMMENT ON COLUMN sign_task.file_id IS '所属合同文件 id';
COMMENT ON COLUMN sign_task.provider IS '签署平台，mock / docuseal / 商业 CA';
COMMENT ON COLUMN sign_task.status IS '状态，created / sent / signed / failed';
COMMENT ON COLUMN sign_task.external_id IS '外部平台任务 id';
COMMENT ON COLUMN sign_task.fail_msg IS '失败信息';
COMMENT ON COLUMN sign_task.created_at IS '创建时间';
COMMENT ON COLUMN sign_task.updated_at IS '更新时间';

COMMENT ON TABLE model_config IS 'AI 与 ML 接入配置：llm / ocr / rerank / embedding / vector / esign';
COMMENT ON COLUMN model_config.id IS '主键';
COMMENT ON COLUMN model_config.name IS '配置名称';
COMMENT ON COLUMN model_config.category IS '类别，llm / ocr / rerank / embedding / vector / esign';
COMMENT ON COLUMN model_config.provider IS '提供方';
COMMENT ON COLUMN model_config.base_url IS '接口地址';
COMMENT ON COLUMN model_config.model_name IS '模型名';
COMMENT ON COLUMN model_config.api_key IS '密钥密文 enc:v1，AES-256-GCM，需 APP_AES_KEY';
COMMENT ON COLUMN model_config.extra_config IS '扩展配置 JSON';
COMMENT ON COLUMN model_config.enabled IS '是否启用';
COMMENT ON COLUMN model_config.created_at IS '创建时间';
COMMENT ON COLUMN model_config.updated_at IS '更新时间';

COMMENT ON TABLE audit_log IS '审计日志：五要素加来源 IP';
COMMENT ON COLUMN audit_log.id IS '主键';
COMMENT ON COLUMN audit_log.username IS '操作人';
COMMENT ON COLUMN audit_log.action IS '动作';
COMMENT ON COLUMN audit_log.target_type IS '目标类型';
COMMENT ON COLUMN audit_log.target_id IS '目标 id';
COMMENT ON COLUMN audit_log.detail IS '详情 JSON，含前后值';
COMMENT ON COLUMN audit_log.ip IS '来源 IP';
COMMENT ON COLUMN audit_log.created_at IS '操作时间';

COMMENT ON TABLE payment_plan IS '应付付款计划：义务到发票到打款钉死';
COMMENT ON COLUMN payment_plan.id IS '主键';
COMMENT ON COLUMN payment_plan.invoice_id IS '关联发票 id';
COMMENT ON COLUMN payment_plan.obligation_id IS '关联义务 id';
COMMENT ON COLUMN payment_plan.payee IS '收款方';
COMMENT ON COLUMN payment_plan.amount IS '计划付款金额';
COMMENT ON COLUMN payment_plan.due_date IS '计划付款日 YYYY-MM-DD';
COMMENT ON COLUMN payment_plan.status IS '状态，planned / paid / cancelled';
COMMENT ON COLUMN payment_plan.paid_date IS '实际付款日';
COMMENT ON COLUMN payment_plan.receipt_id IS '关联银行回单 id';
COMMENT ON COLUMN payment_plan.remark IS '备注';
COMMENT ON COLUMN payment_plan.dept_id IS '归属部门 id';
COMMENT ON COLUMN payment_plan.project_code IS '项目编码';
COMMENT ON COLUMN payment_plan.legal_entity_id IS '归属法人主体 id';
COMMENT ON COLUMN payment_plan.created_at IS '创建时间';
COMMENT ON COLUMN payment_plan.updated_at IS '更新时间';

COMMENT ON TABLE journal_draft IS 'AI 分录草稿：AI 只出建议，会计确认才落账';
COMMENT ON COLUMN journal_draft.id IS '主键';
COMMENT ON COLUMN journal_draft.invoice_id IS '关联发票 id';
COMMENT ON COLUMN journal_draft.category IS '分类';
COMMENT ON COLUMN journal_draft.entries IS '分录明细 JSON';
COMMENT ON COLUMN journal_draft.source IS '来源，llm / rule / rule_fallback';
COMMENT ON COLUMN journal_draft.reason IS '生成依据说明';
COMMENT ON COLUMN journal_draft.status IS '状态，draft / confirmed / rejected';
COMMENT ON COLUMN journal_draft.voucher_id IS '确认后生成的凭证 id';
COMMENT ON COLUMN journal_draft.confirmed_by IS '确认人';
COMMENT ON COLUMN journal_draft.dept_id IS '归属部门 id';
COMMENT ON COLUMN journal_draft.legal_entity_id IS '归属法人主体 id';
COMMENT ON COLUMN journal_draft.split_id IS '关联报销切片 expense_split id';
COMMENT ON COLUMN journal_draft.created_at IS '创建时间';
COMMENT ON COLUMN journal_draft.updated_at IS '更新时间';

COMMENT ON TABLE master_data IS '主数据：客户 / 供应商 / 物料 / 组织 / 人员统一登记';
COMMENT ON COLUMN master_data.id IS '主键';
COMMENT ON COLUMN master_data.md_uid IS '稳定唯一 ID，供各产品引用';
COMMENT ON COLUMN master_data.md_type IS '主数据类型，customer / supplier / material / organization / person';
COMMENT ON COLUMN master_data.code IS '编码';
COMMENT ON COLUMN master_data.name IS '名称';
COMMENT ON COLUMN master_data.legal_entity_id IS '归属法人主体 id';
COMMENT ON COLUMN master_data.attributes IS '扩展属性 JSON';
COMMENT ON COLUMN master_data.status IS '状态，active / disabled';
COMMENT ON COLUMN master_data.dept_id IS '归属部门 id';
COMMENT ON COLUMN master_data.project_code IS '项目编码';
COMMENT ON COLUMN master_data.owner IS '创建人';
COMMENT ON COLUMN master_data.created_at IS '创建时间';
COMMENT ON COLUMN master_data.updated_at IS '更新时间';

COMMENT ON TABLE bridge_log IS '总账桥接日志：导出与推送留痕，绝不谎报成功';
COMMENT ON COLUMN bridge_log.id IS '主键';
COMMENT ON COLUMN bridge_log.mode IS '方式，export 导出 / push 推送';
COMMENT ON COLUMN bridge_log.target IS '目标系统，kingdee_k3 / yonyou_u8 / kingdee_cloud';
COMMENT ON COLUMN bridge_log.voucher_id IS '凭证 id';
COMMENT ON COLUMN bridge_log.status IS '结果，ok / failed';
COMMENT ON COLUMN bridge_log.detail IS '详情';
COMMENT ON COLUMN bridge_log.file_path IS '导出文件路径';
COMMENT ON COLUMN bridge_log.legal_entity_id IS '归属法人主体 id';
COMMENT ON COLUMN bridge_log.created_at IS '创建时间';

-- ============ 审批与消息（20261114_approval_notification + 20261115_ag5） ============
COMMENT ON TABLE approval_template IS '审批流模板：节点定义 JSON';
COMMENT ON COLUMN approval_template.id IS '主键';
COMMENT ON COLUMN approval_template.name IS '模板名称';
COMMENT ON COLUMN approval_template.nodes IS '节点定义 JSON 数组';
COMMENT ON COLUMN approval_template.created_at IS '创建时间';
COMMENT ON COLUMN approval_template.updated_at IS '更新时间';

COMMENT ON TABLE approval_instance IS '审批实例';
COMMENT ON COLUMN approval_instance.id IS '主键';
COMMENT ON COLUMN approval_instance.target_type IS '目标业务类型，默认 contract_file';
COMMENT ON COLUMN approval_instance.target_id IS '目标业务 id';
COMMENT ON COLUMN approval_instance.template_id IS '模板 id';
COMMENT ON COLUMN approval_instance.status IS '状态，running / approved / rejected / revoked';
COMMENT ON COLUMN approval_instance.current_seq IS '当前节点序号';
COMMENT ON COLUMN approval_instance.created_by IS '发起人';
COMMENT ON COLUMN approval_instance.context IS '条件分支上下文 JSON';
COMMENT ON COLUMN approval_instance.revoked_by IS '撤回人';
COMMENT ON COLUMN approval_instance.revoked_at IS '撤回时间';
COMMENT ON COLUMN approval_instance.created_at IS '创建时间';
COMMENT ON COLUMN approval_instance.updated_at IS '更新时间';

COMMENT ON TABLE approval_node IS '审批节点：实例内的逐节点状态';
COMMENT ON COLUMN approval_node.id IS '主键';
COMMENT ON COLUMN approval_node.instance_id IS '所属审批实例 id';
COMMENT ON COLUMN approval_node.seq IS '节点序号';
COMMENT ON COLUMN approval_node.name IS '节点名称';
COMMENT ON COLUMN approval_node.approver IS '审批人';
COMMENT ON COLUMN approval_node.mode IS '审批方式，or 任一通过 / and 全部通过';
COMMENT ON COLUMN approval_node.status IS '状态，pending / approved / rejected / skipped';
COMMENT ON COLUMN approval_node.comment IS '审批意见';
COMMENT ON COLUMN approval_node.acted_at IS '审批时间';
COMMENT ON COLUMN approval_node.origin IS '节点来源，template / addsign 加签 / transfer 转办';
COMMENT ON COLUMN approval_node.when_expr IS '条件分支表达式，key=value，空即无条件';
COMMENT ON COLUMN approval_node.created_at IS '创建时间';

COMMENT ON TABLE approval_event IS '审批全程留痕：动作时间线';
COMMENT ON COLUMN approval_event.id IS '主键';
COMMENT ON COLUMN approval_event.instance_id IS '所属审批实例 id';
COMMENT ON COLUMN approval_event.seq IS '节点序号';
COMMENT ON COLUMN approval_event.actor IS '操作人';
COMMENT ON COLUMN approval_event.action IS '动作，create / approve / reject / skip / transfer / addsign / revoke';
COMMENT ON COLUMN approval_event.from_user IS '转出人';
COMMENT ON COLUMN approval_event.to_user IS '转入人';
COMMENT ON COLUMN approval_event.comment IS '意见';
COMMENT ON COLUMN approval_event.detail IS '明细 JSON';
COMMENT ON COLUMN approval_event.created_at IS '创建时间';

COMMENT ON TABLE notification IS '站内消息中心';
COMMENT ON COLUMN notification.id IS '主键';
COMMENT ON COLUMN notification.username IS '接收人';
COMMENT ON COLUMN notification.type IS '消息类型';
COMMENT ON COLUMN notification.title IS '标题';
COMMENT ON COLUMN notification.content IS '内容';
COMMENT ON COLUMN notification.is_read IS '是否已读';
COMMENT ON COLUMN notification.created_at IS '创建时间';

-- ============ 发票与凭证（20261114_invoice_line） ============
COMMENT ON TABLE invoice IS '发票台账：验真 / 归档 / 红冲 / 分类全链路';
COMMENT ON COLUMN invoice.id IS '主键';
COMMENT ON COLUMN invoice.invoice_no IS '发票号码，数电票 8-20 位';
COMMENT ON COLUMN invoice.issue_date IS '开票日期';
COMMENT ON COLUMN invoice.buyer_name IS '购买方名称';
COMMENT ON COLUMN invoice.seller_name IS '销售方名称';
COMMENT ON COLUMN invoice.amount IS '不含税金额';
COMMENT ON COLUMN invoice.tax IS '税额';
COMMENT ON COLUMN invoice.total IS '价税合计';
COMMENT ON COLUMN invoice.invoice_type IS '发票类型';
COMMENT ON COLUMN invoice.items IS '商品明细行 JSON';
COMMENT ON COLUMN invoice.file_path IS '发票影像路径';
COMMENT ON COLUMN invoice.file_name IS '发票影像文件名';
COMMENT ON COLUMN invoice.file_hash IS '影像哈希，防重复入账';
COMMENT ON COLUMN invoice.verify_status IS '验真状态，unverified / verified / failed';
COMMENT ON COLUMN invoice.verify_remark IS '验真备注';
COMMENT ON COLUMN invoice.integrity_report IS '完整性检查报告 JSON';
COMMENT ON COLUMN invoice.archive_status IS '归档状态，none / archived';
COMMENT ON COLUMN invoice.archive_path IS '归档路径';
COMMENT ON COLUMN invoice.voucher_id IS '入账凭证 id';
COMMENT ON COLUMN invoice.remark IS '备注';
COMMENT ON COLUMN invoice.category IS '自动分类结果';
COMMENT ON COLUMN invoice.category_source IS '分类来源，rule / ai / manual，空未分类';
COMMENT ON COLUMN invoice.original_invoice_no IS '红字发票指向的原蓝字发票号';
COMMENT ON COLUMN invoice.red_flush_status IS '红冲标记，空正常 / red 红字 / reversed 被红冲';
COMMENT ON COLUMN invoice.dept_id IS '归属部门 id';
COMMENT ON COLUMN invoice.project_code IS '项目编码';
COMMENT ON COLUMN invoice.legal_entity_id IS '归属法人主体 id';
COMMENT ON COLUMN invoice.owner IS '创建人';
COMMENT ON COLUMN invoice.created_at IS '创建时间';
COMMENT ON COLUMN invoice.updated_at IS '更新时间';

COMMENT ON TABLE voucher IS '记账凭证';
COMMENT ON COLUMN voucher.id IS '主键';
COMMENT ON COLUMN voucher.voucher_no IS '凭证号';
COMMENT ON COLUMN voucher.summary IS '摘要';
COMMENT ON COLUMN voucher.amount IS '金额';
COMMENT ON COLUMN voucher.voucher_date IS '凭证日期';
COMMENT ON COLUMN voucher.dept_id IS '归属部门 id';
COMMENT ON COLUMN voucher.project_code IS '项目编码';
COMMENT ON COLUMN voucher.legal_entity_id IS '归属法人主体 id';
COMMENT ON COLUMN voucher.owner IS '创建人';
COMMENT ON COLUMN voucher.created_at IS '创建时间';

COMMENT ON TABLE bank_receipt IS '银行回单：收款与发票匹配';
COMMENT ON COLUMN bank_receipt.id IS '主键';
COMMENT ON COLUMN bank_receipt.payer IS '付款方';
COMMENT ON COLUMN bank_receipt.payee IS '收款方';
COMMENT ON COLUMN bank_receipt.amount IS '金额';
COMMENT ON COLUMN bank_receipt.receipt_date IS '回单日期';
COMMENT ON COLUMN bank_receipt.matched_invoice_id IS '匹配到的发票 id';
COMMENT ON COLUMN bank_receipt.match_rule IS '匹配规则说明';
COMMENT ON COLUMN bank_receipt.remark IS '备注';
COMMENT ON COLUMN bank_receipt.dept_id IS '归属部门 id';
COMMENT ON COLUMN bank_receipt.project_code IS '项目编码';
COMMENT ON COLUMN bank_receipt.legal_entity_id IS '归属法人主体 id';
COMMENT ON COLUMN bank_receipt.created_at IS '创建时间';

COMMENT ON TABLE account_mapping IS '科目映射：费用类型与收入确认事项对应会计科目，AI 只能从这里选科目';
COMMENT ON COLUMN account_mapping.id IS '主键';
COMMENT ON COLUMN account_mapping.category IS '业务类别';
COMMENT ON COLUMN account_mapping.subject_code IS '会计科目编码';
COMMENT ON COLUMN account_mapping.subject_name IS '会计科目名称';
COMMENT ON COLUMN account_mapping.direction IS '借贷方向，debit / credit';
COMMENT ON COLUMN account_mapping.created_at IS '创建时间';

-- ============ AG5 数据隔离（20261115_ag5_flow_party） ============
COMMENT ON TABLE isolation_policy IS 'AG5 多维度数据隔离策略：法人 / 部门 / 项目三维度';
COMMENT ON COLUMN isolation_policy.id IS '主键';
COMMENT ON COLUMN isolation_policy.name IS '策略名称';
COMMENT ON COLUMN isolation_policy.dimension IS '隔离维度，entity / dept / project';
COMMENT ON COLUMN isolation_policy.subject_type IS '主体类型，user / role / all';
COMMENT ON COLUMN isolation_policy.subject IS '主体，用户名 / 角色码，all 留空';
COMMENT ON COLUMN isolation_policy.allowed_values IS '允许值 JSON 数组，0 代表未归属历史数据';
COMMENT ON COLUMN isolation_policy.include_children IS '允许值沿父子树递归展开';
COMMENT ON COLUMN isolation_policy.enabled IS '是否启用';
COMMENT ON COLUMN isolation_policy.priority IS '优先级，大者先匹配';
COMMENT ON COLUMN isolation_policy.created_by IS '创建人';
COMMENT ON COLUMN isolation_policy.created_at IS '创建时间';
COMMENT ON COLUMN isolation_policy.updated_at IS '更新时间';

COMMENT ON TABLE isolation_violation IS 'AG5.4 越权访问尝试留痕：拦截加告警';
COMMENT ON COLUMN isolation_violation.id IS '主键';
COMMENT ON COLUMN isolation_violation.username IS '尝试访问的用户';
COMMENT ON COLUMN isolation_violation.role IS '用户角色';
COMMENT ON COLUMN isolation_violation.method IS 'HTTP 方法';
COMMENT ON COLUMN isolation_violation.path IS '请求路径';
COMMENT ON COLUMN isolation_violation.resource IS '资源类型';
COMMENT ON COLUMN isolation_violation.object_id IS '资源对象 id';
COMMENT ON COLUMN isolation_violation.dimension IS '命中的隔离维度';
COMMENT ON COLUMN isolation_violation.attempted_value IS '尝试访问的越权值';
COMMENT ON COLUMN isolation_violation.policy_ids IS '命中的策略 id JSON 数组';
COMMENT ON COLUMN isolation_violation.status IS '处理状态，open / resolved / waived';
COMMENT ON COLUMN isolation_violation.handled_by IS '处理人';
COMMENT ON COLUMN isolation_violation.handled_note IS '处理说明';
COMMENT ON COLUMN isolation_violation.created_at IS '发生时间';
COMMENT ON COLUMN isolation_violation.handled_at IS '处理时间';

-- ============ 组织与法人（20261115_core_org_schedule） ============
COMMENT ON TABLE legal_entity IS 'K3 法人主体：多组织隔离的顶层维度';
COMMENT ON COLUMN legal_entity.id IS '主键';
COMMENT ON COLUMN legal_entity.code IS '主体编码';
COMMENT ON COLUMN legal_entity.name IS '主体名称';
COMMENT ON COLUMN legal_entity.entity_type IS '类型，headquarters 总部 / subsidiary 子公司 / branch 分支';
COMMENT ON COLUMN legal_entity.parent_id IS '上级主体 id，0 为顶层';
COMMENT ON COLUMN legal_entity.status IS '状态，active / disabled';
COMMENT ON COLUMN legal_entity.created_at IS '创建时间';
COMMENT ON COLUMN legal_entity.updated_at IS '更新时间';

COMMENT ON TABLE user_context IS '用户当前主体上下文：按用户名记录主体切换';
COMMENT ON COLUMN user_context.username IS '用户名，主键';
COMMENT ON COLUMN user_context.legal_entity_id IS '当前主体 id，0 为全部主体';
COMMENT ON COLUMN user_context.include_subtree IS '是否含下级主体（父子树递归展开）';
COMMENT ON COLUMN user_context.updated_at IS '更新时间';

COMMENT ON TABLE org_node IS 'K2 组织树节点：部门 / 岗位';
COMMENT ON COLUMN org_node.id IS '主键';
COMMENT ON COLUMN org_node.legal_entity_id IS '归属法人主体 id';
COMMENT ON COLUMN org_node.parent_id IS '上级节点 id，0 为顶层';
COMMENT ON COLUMN org_node.code IS '节点编码';
COMMENT ON COLUMN org_node.name IS '节点名称';
COMMENT ON COLUMN org_node.node_type IS '类型，dept 部门 / position 岗位';
COMMENT ON COLUMN org_node.sort_order IS '排序号';
COMMENT ON COLUMN org_node.created_at IS '创建时间';
COMMENT ON COLUMN org_node.updated_at IS '更新时间';

COMMENT ON TABLE org_member IS '组织成员：用户挂到组织节点';
COMMENT ON COLUMN org_member.id IS '主键';
COMMENT ON COLUMN org_member.node_id IS '组织节点 id';
COMMENT ON COLUMN org_member.legal_entity_id IS '归属法人主体 id';
COMMENT ON COLUMN org_member.username IS '用户名';
COMMENT ON COLUMN org_member.created_at IS '创建时间';

COMMENT ON TABLE master_data_type IS '主数据类型字典';
COMMENT ON COLUMN master_data_type.code IS '类型编码，主键';
COMMENT ON COLUMN master_data_type.name IS '类型名称';
COMMENT ON COLUMN master_data_type.description IS '用途说明';
COMMENT ON COLUMN master_data_type.sort_order IS '排序号';

COMMENT ON TABLE master_data_link IS '主数据关系：主数据之间的引用关系';
COMMENT ON COLUMN master_data_link.id IS '主键';
COMMENT ON COLUMN master_data_link.from_id IS '源主数据 id';
COMMENT ON COLUMN master_data_link.to_id IS '目标主数据 id';
COMMENT ON COLUMN master_data_link.relation IS '关系类型，belongs_to / contact_of / supplies';
COMMENT ON COLUMN master_data_link.created_at IS '创建时间';

-- ============ 归档调度（US-I9，AI9.1-AI9.4） ============
COMMENT ON TABLE archive_schedule IS '定期自动归档调度配置';
COMMENT ON COLUMN archive_schedule.id IS '主键';
COMMENT ON COLUMN archive_schedule.name IS '调度名称';
COMMENT ON COLUMN archive_schedule.period_type IS '账期类型，weekly / monthly / quarterly';
COMMENT ON COLUMN archive_schedule.due_days IS '账期结束后第 N 天执行';
COMMENT ON COLUMN archive_schedule.scope_type IS '归档范围，all / legal_entity';
COMMENT ON COLUMN archive_schedule.scope_id IS '范围对象 id';
COMMENT ON COLUMN archive_schedule.require_verified IS '仅归档已验真发票';
COMMENT ON COLUMN archive_schedule.require_voucher IS '仅归档已入账发票';
COMMENT ON COLUMN archive_schedule.require_paid IS '仅归档已收款发票';
COMMENT ON COLUMN archive_schedule.enabled IS '是否启用';
COMMENT ON COLUMN archive_schedule.next_run_at IS '下次执行时间';
COMMENT ON COLUMN archive_schedule.last_run_at IS '上次执行时间';
COMMENT ON COLUMN archive_schedule.created_at IS '创建时间';
COMMENT ON COLUMN archive_schedule.updated_at IS '更新时间';

COMMENT ON TABLE archive_schedule_run IS '归档调度执行批次';
COMMENT ON COLUMN archive_schedule_run.id IS '主键';
COMMENT ON COLUMN archive_schedule_run.schedule_id IS '所属调度 id';
COMMENT ON COLUMN archive_schedule_run.trigger IS '触发方式，auto 自动 / manual 手动';
COMMENT ON COLUMN archive_schedule_run.status IS '状态，running / done / failed';
COMMENT ON COLUMN archive_schedule_run.total IS '处理总数';
COMMENT ON COLUMN archive_schedule_run.success IS '成功数';
COMMENT ON COLUMN archive_schedule_run.failed IS '失败数';
COMMENT ON COLUMN archive_schedule_run.skipped IS '跳过数';
COMMENT ON COLUMN archive_schedule_run.cutoff_date IS '归档截止日期';
COMMENT ON COLUMN archive_schedule_run.message IS '执行摘要';
COMMENT ON COLUMN archive_schedule_run.started_at IS '开始时间';
COMMENT ON COLUMN archive_schedule_run.finished_at IS '结束时间';

COMMENT ON TABLE archive_schedule_item IS '归档批次明细：每张发票的处理结果';
COMMENT ON COLUMN archive_schedule_item.id IS '主键';
COMMENT ON COLUMN archive_schedule_item.run_id IS '所属批次 id';
COMMENT ON COLUMN archive_schedule_item.invoice_id IS '发票 id';
COMMENT ON COLUMN archive_schedule_item.invoice_no IS '发票号码';
COMMENT ON COLUMN archive_schedule_item.result IS '结果，ok / failed / skipped';
COMMENT ON COLUMN archive_schedule_item.message IS '结果说明';
COMMENT ON COLUMN archive_schedule_item.archive_path IS '归档路径';
COMMENT ON COLUMN archive_schedule_item.created_at IS '创建时间';

COMMENT ON TABLE archive_exception IS '归档失败人工处理队列（AI9.4）';
COMMENT ON COLUMN archive_exception.id IS '主键';
COMMENT ON COLUMN archive_exception.run_id IS '来源批次 id';
COMMENT ON COLUMN archive_exception.invoice_id IS '发票 id';
COMMENT ON COLUMN archive_exception.invoice_no IS '发票号码';
COMMENT ON COLUMN archive_exception.reason IS '失败原因';
COMMENT ON COLUMN archive_exception.status IS '状态，pending / resolved / ignored';
COMMENT ON COLUMN archive_exception.handled_by IS '处理人';
COMMENT ON COLUMN archive_exception.handled_note IS '处理说明';
COMMENT ON COLUMN archive_exception.created_at IS '创建时间';
COMMENT ON COLUMN archive_exception.handled_at IS '处理时间';

-- ============ 费控三件套（20261115_expense_control） ============
COMMENT ON TABLE expense_split IS '报销切片：一张发票拆多条报销记录，合计不超价税合计';
COMMENT ON COLUMN expense_split.id IS '主键';
COMMENT ON COLUMN expense_split.invoice_id IS '来源发票 id';
COMMENT ON COLUMN expense_split.slice_no IS '切片序号';
COMMENT ON COLUMN expense_split.amount IS '切片金额';
COMMENT ON COLUMN expense_split.expense_type IS '用途，对齐发票分类';
COMMENT ON COLUMN expense_split.dept_id IS '归属部门 id';
COMMENT ON COLUMN expense_split.bearer IS '承担方，company 公司 / personal 个人垫付';
COMMENT ON COLUMN expense_split.remark IS '备注';
COMMENT ON COLUMN expense_split.creator IS '创建人';
COMMENT ON COLUMN expense_split.created_at IS '创建时间';

COMMENT ON TABLE budget IS '费控预算：部门加月份加分类的上限';
COMMENT ON COLUMN budget.id IS '主键';
COMMENT ON COLUMN budget.dept_id IS '部门 id';
COMMENT ON COLUMN budget.period IS '预算期间 YYYY-MM';
COMMENT ON COLUMN budget.category IS '预算分类，对齐发票分类';
COMMENT ON COLUMN budget.amount_limit IS '预算上限';
COMMENT ON COLUMN budget.remark IS '备注';
COMMENT ON COLUMN budget.creator IS '创建人';
COMMENT ON COLUMN budget.created_at IS '创建时间';

COMMENT ON TABLE travel_standard IS '差旅标准：分类内单笔上限，超限即预警';
COMMENT ON COLUMN travel_standard.id IS '主键';
COMMENT ON COLUMN travel_standard.category IS '分类，差旅交通 / 餐饮招待 / 住宿等';
COMMENT ON COLUMN travel_standard.limit_amount IS '单笔上限';
COMMENT ON COLUMN travel_standard.note IS '说明';
COMMENT ON COLUMN travel_standard.enabled IS '是否启用';
COMMENT ON COLUMN travel_standard.creator IS '创建人';
COMMENT ON COLUMN travel_standard.created_at IS '创建时间';

-- ============ 文件中心（20261115_file_entry） ============
COMMENT ON TABLE file_entry IS '统一文件中心：双后端 local / s3';
COMMENT ON COLUMN file_entry.id IS '主键';
COMMENT ON COLUMN file_entry.biz_type IS '业务类型，contract / invoice / receipt / kb / other';
COMMENT ON COLUMN file_entry.biz_id IS '关联业务 id，0 未挂接';
COMMENT ON COLUMN file_entry.filename IS '文件名';
COMMENT ON COLUMN file_entry.content_type IS 'MIME 类型';
COMMENT ON COLUMN file_entry.size_bytes IS '文件大小字节';
COMMENT ON COLUMN file_entry.sha256 IS 'SHA256 哈希，去重';
COMMENT ON COLUMN file_entry.storage IS '存储后端，local / s3';
COMMENT ON COLUMN file_entry.storage_key IS '存储键';
COMMENT ON COLUMN file_entry.uploader IS '上传人';
COMMENT ON COLUMN file_entry.created_at IS '创建时间';

-- ============ G1 RBAC（20261115_governance_g1g4g6） ============
COMMENT ON TABLE rbac_role IS 'RBAC 角色：权限配置化';
COMMENT ON COLUMN rbac_role.id IS '主键';
COMMENT ON COLUMN rbac_role.code IS '角色编码，内置 admin / legal / business / viewer';
COMMENT ON COLUMN rbac_role.name IS '角色名称';
COMMENT ON COLUMN rbac_role.description IS '角色说明';
COMMENT ON COLUMN rbac_role.is_builtin IS '内置角色不可删';
COMMENT ON COLUMN rbac_role.permissions IS '权限 JSON 数组，资源模式如 POST /api/xxx，空走静态规则';
COMMENT ON COLUMN rbac_role.data_scope IS '数据范围，all / entity / dept / self';
COMMENT ON COLUMN rbac_role.field_mask IS '不可见字段 JSON 数组，字段级权限';
COMMENT ON COLUMN rbac_role.enabled IS '是否启用';
COMMENT ON COLUMN rbac_role.created_at IS '创建时间';
COMMENT ON COLUMN rbac_role.updated_at IS '更新时间';

COMMENT ON TABLE rbac_user_role IS '用户角色绑定';
COMMENT ON COLUMN rbac_user_role.id IS '主键';
COMMENT ON COLUMN rbac_user_role.username IS '用户名';
COMMENT ON COLUMN rbac_user_role.role_code IS '角色编码';
COMMENT ON COLUMN rbac_user_role.source IS '来源，manual 手工 / scim 自动同步';
COMMENT ON COLUMN rbac_user_role.created_at IS '创建时间';

-- ============ G4 合规策略引擎 ============
COMMENT ON TABLE compliance_rule IS 'G4 合规策略：制度条款落到可执行规则';
COMMENT ON COLUMN compliance_rule.id IS '主键';
COMMENT ON COLUMN compliance_rule.code IS '规则编码，如 CR-CONTRACT-PARTY';
COMMENT ON COLUMN compliance_rule.name IS '规则名称';
COMMENT ON COLUMN compliance_rule.description IS '规则说明';
COMMENT ON COLUMN compliance_rule.rule_type IS '类型，required / format / relation / threshold';
COMMENT ON COLUMN compliance_rule.object_type IS '目标对象，contract / invoice / voucher / master_data';
COMMENT ON COLUMN compliance_rule.field IS '校验字段';
COMMENT ON COLUMN compliance_rule.op IS '操作符，gt / gte / lt / lte / eq / regex / exists';
COMMENT ON COLUMN compliance_rule.rule_value IS '阈值 / 正则 / 被关联字段';
COMMENT ON COLUMN compliance_rule.condition_expr IS '绑定条件，key=value，空不限';
COMMENT ON COLUMN compliance_rule.severity IS '级别，block 拦截 / warn 告警';
COMMENT ON COLUMN compliance_rule.enabled IS '是否启用';
COMMENT ON COLUMN compliance_rule.version IS '版本号';
COMMENT ON COLUMN compliance_rule.doc IS '制度条款出处';
COMMENT ON COLUMN compliance_rule.created_at IS '创建时间';
COMMENT ON COLUMN compliance_rule.updated_at IS '更新时间';

COMMENT ON TABLE compliance_rule_history IS '合规规则版本历史：每次编辑留快照';
COMMENT ON COLUMN compliance_rule_history.id IS '主键';
COMMENT ON COLUMN compliance_rule_history.rule_id IS '规则 id';
COMMENT ON COLUMN compliance_rule_history.version IS '版本号';
COMMENT ON COLUMN compliance_rule_history.snapshot IS '规则快照 JSON';
COMMENT ON COLUMN compliance_rule_history.changed_by IS '修改人';
COMMENT ON COLUMN compliance_rule_history.changed_at IS '修改时间';

COMMENT ON TABLE compliance_violation IS '合规违规记录';
COMMENT ON COLUMN compliance_violation.id IS '主键';
COMMENT ON COLUMN compliance_violation.rule_id IS '命中规则 id';
COMMENT ON COLUMN compliance_violation.rule_code IS '规则编码';
COMMENT ON COLUMN compliance_violation.rule_name IS '规则名称';
COMMENT ON COLUMN compliance_violation.object_type IS '对象类型';
COMMENT ON COLUMN compliance_violation.object_id IS '对象 id';
COMMENT ON COLUMN compliance_violation.object_label IS '对象展示名';
COMMENT ON COLUMN compliance_violation.field IS '违规字段';
COMMENT ON COLUMN compliance_violation.message IS '违规说明';
COMMENT ON COLUMN compliance_violation.severity IS '级别，block / warn';
COMMENT ON COLUMN compliance_violation.status IS '状态，open / resolved / waived';
COMMENT ON COLUMN compliance_violation.owner IS '责任人';
COMMENT ON COLUMN compliance_violation.handled_by IS '处理人';
COMMENT ON COLUMN compliance_violation.handled_note IS '处理说明';
COMMENT ON COLUMN compliance_violation.created_at IS '发生时间';
COMMENT ON COLUMN compliance_violation.handled_at IS '处理时间';

-- ============ G6 MDM 实体解析去重 ============
COMMENT ON TABLE md_match_rule IS 'G6 主数据匹配规则';
COMMENT ON COLUMN md_match_rule.id IS '主键';
COMMENT ON COLUMN md_match_rule.name IS '规则名称';
COMMENT ON COLUMN md_match_rule.md_type IS '主数据类型，默认 supplier';
COMMENT ON COLUMN md_match_rule.fields IS '参与匹配字段 JSON 数组';
COMMENT ON COLUMN md_match_rule.threshold IS '相似度阈值';
COMMENT ON COLUMN md_match_rule.enabled IS '是否启用';
COMMENT ON COLUMN md_match_rule.created_at IS '创建时间';
COMMENT ON COLUMN md_match_rule.updated_at IS '更新时间';

COMMENT ON TABLE md_duplicate_pair IS '疑似重复主数据对：合并 / 忽略处置';
COMMENT ON COLUMN md_duplicate_pair.id IS '主键';
COMMENT ON COLUMN md_duplicate_pair.match_rule_id IS '命中规则 id';
COMMENT ON COLUMN md_duplicate_pair.left_id IS '左侧主数据 id';
COMMENT ON COLUMN md_duplicate_pair.right_id IS '右侧主数据 id';
COMMENT ON COLUMN md_duplicate_pair.left_uid IS '左侧稳定 ID';
COMMENT ON COLUMN md_duplicate_pair.right_uid IS '右侧稳定 ID';
COMMENT ON COLUMN md_duplicate_pair.left_name IS '左侧名称';
COMMENT ON COLUMN md_duplicate_pair.right_name IS '右侧名称';
COMMENT ON COLUMN md_duplicate_pair.score IS '相似度得分';
COMMENT ON COLUMN md_duplicate_pair.reasons IS '命中原因 JSON 数组';
COMMENT ON COLUMN md_duplicate_pair.suggestion IS '合并建议';
COMMENT ON COLUMN md_duplicate_pair.status IS '状态，pending / merged / ignored';
COMMENT ON COLUMN md_duplicate_pair.handled_by IS '处理人';
COMMENT ON COLUMN md_duplicate_pair.handled_note IS '处理说明';
COMMENT ON COLUMN md_duplicate_pair.created_at IS '创建时间';
COMMENT ON COLUMN md_duplicate_pair.handled_at IS '处理时间';

COMMENT ON TABLE md_id_mapping IS '合并后历史 ID 映射：被合并方稳定 ID 仍可反查';
COMMENT ON COLUMN md_id_mapping.id IS '主键';
COMMENT ON COLUMN md_id_mapping.old_id IS '被合并方主数据 id';
COMMENT ON COLUMN md_id_mapping.old_uid IS '被合并方稳定 ID';
COMMENT ON COLUMN md_id_mapping.old_name IS '被合并方名称';
COMMENT ON COLUMN md_id_mapping.survivor_id IS '保留方主数据 id';
COMMENT ON COLUMN md_id_mapping.survivor_uid IS '保留方稳定 ID';
COMMENT ON COLUMN md_id_mapping.pair_id IS '来源重复对 id';
COMMENT ON COLUMN md_id_mapping.merged_at IS '合并时间';

-- ============ 发票分类与知识库（20261115_invoice_class / kb_legal） ============
COMMENT ON TABLE invoice_class_rule IS '发票自动分类规则：关键词规则配置，AI 兜底';
COMMENT ON COLUMN invoice_class_rule.id IS '主键';
COMMENT ON COLUMN invoice_class_rule.name IS '规则名';
COMMENT ON COLUMN invoice_class_rule.category IS '命中后的分类';
COMMENT ON COLUMN invoice_class_rule.field IS '匹配字段，any 全部 / seller 开票方 / items 商品名';
COMMENT ON COLUMN invoice_class_rule.keywords IS '逗号分隔关键词';
COMMENT ON COLUMN invoice_class_rule.priority IS '优先级，小者先匹配';
COMMENT ON COLUMN invoice_class_rule.enabled IS '是否启用';
COMMENT ON COLUMN invoice_class_rule.created_at IS '创建时间';

COMMENT ON TABLE kb_clause IS '法律条款知识库：RAG 语义检索最小闭环';
COMMENT ON COLUMN kb_clause.id IS '主键';
COMMENT ON COLUMN kb_clause.title IS '条款标题';
COMMENT ON COLUMN kb_clause.content IS '条款正文';
COMMENT ON COLUMN kb_clause.source IS '来源，法规名与条文号';
COMMENT ON COLUMN kb_clause.category IS '条款分类';
COMMENT ON COLUMN kb_clause.embedding IS '语义向量，PG 系 pgvector，其余库降级关键词检索';
COMMENT ON COLUMN kb_clause.created_at IS '创建时间';

-- ============ AI 对话（20261115_ai_chat） ============
COMMENT ON TABLE chat_session IS 'AI 对话会话：对话即操作';
COMMENT ON COLUMN chat_session.id IS '主键';
COMMENT ON COLUMN chat_session.username IS '所属用户';
COMMENT ON COLUMN chat_session.title IS '会话标题，默认新对话';
COMMENT ON COLUMN chat_session.created_at IS '创建时间';
COMMENT ON COLUMN chat_session.updated_at IS '更新时间';

COMMENT ON TABLE chat_message IS 'AI 对话消息留痕';
COMMENT ON COLUMN chat_message.id IS '主键';
COMMENT ON COLUMN chat_message.session_id IS '所属会话 id';
COMMENT ON COLUMN chat_message.role IS '角色，user / assistant';
COMMENT ON COLUMN chat_message.content IS '消息内容';
COMMENT ON COLUMN chat_message.tool_trace IS '本轮工具执行步骤 JSON';
COMMENT ON COLUMN chat_message.created_at IS '创建时间';

-- ============ 收入确认（20261115_revrec_seal + 20261116_revrec_p0/p1） ============
COMMENT ON TABLE revrec_confirm IS '收入确认记录：AI 建议，人审确认铁律';
COMMENT ON COLUMN revrec_confirm.id IS '主键';
COMMENT ON COLUMN revrec_confirm.obligation_id IS '关联义务 id';
COMMENT ON COLUMN revrec_confirm.period IS '确认期间 YYYY-MM';
COMMENT ON COLUMN revrec_confirm.amount IS '本期确认收入，不含税';
COMMENT ON COLUMN revrec_confirm.tax IS '销项税额';
COMMENT ON COLUMN revrec_confirm.total IS '价税合计';
COMMENT ON COLUMN revrec_confirm.progress IS '时段法履约进度';
COMMENT ON COLUMN revrec_confirm.entries IS '分录 JSON，借应收或合同结算贷收入与销项';
COMMENT ON COLUMN revrec_confirm.source IS '来源，rule / ai';
COMMENT ON COLUMN revrec_confirm.status IS '状态，draft 到 confirmed 人审确认';
COMMENT ON COLUMN revrec_confirm.draft_id IS '关联分录草稿 id';
COMMENT ON COLUMN revrec_confirm.cost IS '本期确认成本，成本结转金额来源';
COMMENT ON COLUMN revrec_confirm.creator IS '创建人';
COMMENT ON COLUMN revrec_confirm.created_at IS '创建时间';
COMMENT ON COLUMN revrec_confirm.confirmed_at IS '确认时间';

COMMENT ON TABLE revrec_receipt IS '回款登记：乙方收款，现金流日历数据源';
COMMENT ON COLUMN revrec_receipt.id IS '主键';
COMMENT ON COLUMN revrec_receipt.contract_file_id IS '关联合同文件 id';
COMMENT ON COLUMN revrec_receipt.obligation_id IS '关联义务 id';
COMMENT ON COLUMN revrec_receipt.amount IS '回款金额';
COMMENT ON COLUMN revrec_receipt.received_at IS '回款日期';
COMMENT ON COLUMN revrec_receipt.remark IS '备注';
COMMENT ON COLUMN revrec_receipt.creator IS '创建人';
COMMENT ON COLUMN revrec_receipt.created_at IS '创建时间';

COMMENT ON TABLE revrec_settlement IS '结算登记：确认与结算与收款三条线口径';
COMMENT ON COLUMN revrec_settlement.id IS '主键';
COMMENT ON COLUMN revrec_settlement.contract_file_id IS '关联合同文件 id';
COMMENT ON COLUMN revrec_settlement.obligation_id IS '关联义务 id';
COMMENT ON COLUMN revrec_settlement.amount IS '结算金额';
COMMENT ON COLUMN revrec_settlement.settled_at IS '结算日期';
COMMENT ON COLUMN revrec_settlement.remark IS '备注';
COMMENT ON COLUMN revrec_settlement.creator IS '创建人';
COMMENT ON COLUMN revrec_settlement.created_at IS '创建时间';

-- ============ 用印管理（20261115_revrec_seal + 20261116_seal_p0/p2） ============
COMMENT ON TABLE seal IS '印章台账';
COMMENT ON COLUMN seal.id IS '主键';
COMMENT ON COLUMN seal.name IS '印章名称';
COMMENT ON COLUMN seal.type IS '类型，contract 合同章 / official 公章 / finance 财务章 / legal 法人章 / other';
COMMENT ON COLUMN seal.keeper IS '保管人';
COMMENT ON COLUMN seal.status IS '状态，active 在用 / disabled 停用 / destroyed 销毁';
COMMENT ON COLUMN seal.remark IS '备注';
COMMENT ON COLUMN seal.creator IS '创建人';
COMMENT ON COLUMN seal.created_at IS '创建时间';

COMMENT ON TABLE seal_apply IS '用印申请与登记：七字段对齐商业口径';
COMMENT ON COLUMN seal_apply.id IS '主键';
COMMENT ON COLUMN seal_apply.seal_id IS '印章 id';
COMMENT ON COLUMN seal_apply.biz_type IS '业务类型，contract / other';
COMMENT ON COLUMN seal_apply.biz_id IS '关联业务 id';
COMMENT ON COLUMN seal_apply.file_id IS '被盖章文件 file_entry id，哈希防篡改';
COMMENT ON COLUMN seal_apply.copies IS '份数';
COMMENT ON COLUMN seal_apply.reason IS '用印事由';
COMMENT ON COLUMN seal_apply.status IS '状态，pending 待批 / approved 已批 / stamped 已用印 / rejected 驳回';
COMMENT ON COLUMN seal_apply.approver IS '审批人';
COMMENT ON COLUMN seal_apply.approved_at IS '审批时间';
COMMENT ON COLUMN seal_apply.operator IS '用印操作人';
COMMENT ON COLUMN seal_apply.stamped_at IS '用印时间';
COMMENT ON COLUMN seal_apply.device IS '来源设备或 IP';
COMMENT ON COLUMN seal_apply.file_hash IS '盖章文件哈希';
COMMENT ON COLUMN seal_apply.approval_instance_id IS '审批实例 id，0 走简化审批';
COMMENT ON COLUMN seal_apply.es_platform IS '电子签平台标识';
COMMENT ON COLUMN seal_apply.es_file_no IS '电子签文件编号';
COMMENT ON COLUMN seal_apply.es_signer IS '电子签签署人';
COMMENT ON COLUMN seal_apply.es_signed_at IS '电子签签署时间';
COMMENT ON COLUMN seal_apply.creator IS '创建人';
COMMENT ON COLUMN seal_apply.created_at IS '创建时间';

COMMENT ON TABLE seal_approval_rule IS '按印章类型配置审批模板，未配置走简化审批';
COMMENT ON COLUMN seal_approval_rule.id IS '主键';
COMMENT ON COLUMN seal_approval_rule.seal_type IS '印章类型，唯一';
COMMENT ON COLUMN seal_approval_rule.approval_template_id IS '审批模板 id，0 回退简化审批';
COMMENT ON COLUMN seal_approval_rule.enabled IS '是否启用';
COMMENT ON COLUMN seal_approval_rule.creator IS '创建人';
COMMENT ON COLUMN seal_approval_rule.created_at IS '创建时间';
COMMENT ON COLUMN seal_approval_rule.updated_at IS '更新时间';

COMMENT ON TABLE seal_es_config IS '外部电子签平台配置：单行 app 级';
COMMENT ON COLUMN seal_es_config.id IS '主键';
COMMENT ON COLUMN seal_es_config.platform IS '平台标识，如 esign-ali';
COMMENT ON COLUMN seal_es_config.endpoint IS '盖章记录查询地址，空回填明确失败';
COMMENT ON COLUMN seal_es_config.token IS '凭据，不出接口不进导出';
COMMENT ON COLUMN seal_es_config.enabled IS '开关，on / off';
COMMENT ON COLUMN seal_es_config.remark IS '备注';
COMMENT ON COLUMN seal_es_config.updated_by IS '更新人';
COMMENT ON COLUMN seal_es_config.updated_at IS '更新时间';

COMMENT ON TABLE seal_es_record IS '电子签盖章记录回填留痕';
COMMENT ON COLUMN seal_es_record.id IS '主键';
COMMENT ON COLUMN seal_es_record.apply_id IS '关联用印申请 id';
COMMENT ON COLUMN seal_es_record.platform IS '来源平台';
COMMENT ON COLUMN seal_es_record.ext_ref IS '外部流水号';
COMMENT ON COLUMN seal_es_record.es_file_no IS '电子签文件编号';
COMMENT ON COLUMN seal_es_record.signer IS '签署人';
COMMENT ON COLUMN seal_es_record.signed_at IS '签署时间';
COMMENT ON COLUMN seal_es_record.evidence_url IS '证据链接';
COMMENT ON COLUMN seal_es_record.raw IS '平台原始返回，不含凭据';
COMMENT ON COLUMN seal_es_record.creator IS '创建人';
COMMENT ON COLUMN seal_es_record.created_at IS '创建时间';

-- ============ SCIM 身份同步（20261115_scim_identity） ============
COMMENT ON TABLE scim_user IS 'SCIM 同步用户：IdP 推送，active 为假即离职禁用';
COMMENT ON COLUMN scim_user.id IS '主键';
COMMENT ON COLUMN scim_user.external_id IS 'IdP 侧用户 id';
COMMENT ON COLUMN scim_user.username IS '用户名';
COMMENT ON COLUMN scim_user.display_name IS '显示名';
COMMENT ON COLUMN scim_user.email IS '邮箱';
COMMENT ON COLUMN scim_user.title IS '职位';
COMMENT ON COLUMN scim_user.department IS '部门';
COMMENT ON COLUMN scim_user.active IS '是否在职有效';
COMMENT ON COLUMN scim_user.groups IS '所属组 JSON 数组';
COMMENT ON COLUMN scim_user.roles IS '同步后生效角色 JSON 数组';
COMMENT ON COLUMN scim_user.raw IS '最近一次 SCIM User 文档';
COMMENT ON COLUMN scim_user.created_at IS '创建时间';
COMMENT ON COLUMN scim_user.updated_at IS '更新时间';
COMMENT ON COLUMN scim_user.last_synced_at IS '最近同步时间';

COMMENT ON TABLE scim_group IS 'SCIM 组：成员变化重算角色绑定';
COMMENT ON COLUMN scim_group.id IS '主键';
COMMENT ON COLUMN scim_group.external_id IS 'IdP 侧组 id';
COMMENT ON COLUMN scim_group.display_name IS '组显示名';
COMMENT ON COLUMN scim_group.members IS '成员 JSON 数组';
COMMENT ON COLUMN scim_group.raw IS '最近一次 SCIM Group 文档';
COMMENT ON COLUMN scim_group.created_at IS '创建时间';
COMMENT ON COLUMN scim_group.updated_at IS '更新时间';

COMMENT ON TABLE scim_role_map IS '组名到本系统角色编码映射';
COMMENT ON COLUMN scim_role_map.id IS '主键';
COMMENT ON COLUMN scim_role_map.group_name IS 'IdP 组名';
COMMENT ON COLUMN scim_role_map.role_code IS '本系统角色编码';
COMMENT ON COLUMN scim_role_map.note IS '映射说明';
COMMENT ON COLUMN scim_role_map.created_at IS '创建时间';

COMMENT ON TABLE scim_sync_log IS 'SCIM 推送审计轨迹，不含凭据';
COMMENT ON COLUMN scim_sync_log.id IS '主键';
COMMENT ON COLUMN scim_sync_log.operation IS '操作类型';
COMMENT ON COLUMN scim_sync_log.resource_type IS '资源类型，User / Group';
COMMENT ON COLUMN scim_sync_log.resource_id IS '资源 id';
COMMENT ON COLUMN scim_sync_log.username IS '涉及用户名';
COMMENT ON COLUMN scim_sync_log.external_id IS 'IdP 侧 id';
COMMENT ON COLUMN scim_sync_log.http_status IS '响应 HTTP 状态码';
COMMENT ON COLUMN scim_sync_log.message IS '结果说明';
COMMENT ON COLUMN scim_sync_log.actor IS '推送方标识';
COMMENT ON COLUMN scim_sync_log.payload IS '请求摘要';
COMMENT ON COLUMN scim_sync_log.created_at IS '创建时间';

-- ============ Agent 技能包（20261115_skill_pack） ============
COMMENT ON TABLE skill_pack IS 'Agent Skill 包：命中触发词即注入对话';
COMMENT ON COLUMN skill_pack.id IS '主键';
COMMENT ON COLUMN skill_pack.name IS '技能唯一名，目录名';
COMMENT ON COLUMN skill_pack.display_name IS '展示名';
COMMENT ON COLUMN skill_pack.description IS '技能说明';
COMMENT ON COLUMN skill_pack.triggers IS '逗号分隔触发词';
COMMENT ON COLUMN skill_pack.instructions IS '注入对话的技能指令，SKILL.md 正文';
COMMENT ON COLUMN skill_pack.version IS '版本';
COMMENT ON COLUMN skill_pack.path IS '磁盘权威源 skills 目录';
COMMENT ON COLUMN skill_pack.file_id IS '文件中心备份登记 id';
COMMENT ON COLUMN skill_pack.enabled IS '是否启用';
COMMENT ON COLUMN skill_pack.creator IS '创建人';
COMMENT ON COLUMN skill_pack.created_at IS '创建时间';

-- ============ 合同模板库（20261116_contract_template） ============
COMMENT ON TABLE contract_template IS '合同模板库加变量套打，对标法大大核心收费点';
COMMENT ON COLUMN contract_template.id IS '主键';
COMMENT ON COLUMN contract_template.name IS '模板名称';
COMMENT ON COLUMN contract_template.category IS '适用合同类型，采购 / 销售 / 服务 / 租赁 / 保密等';
COMMENT ON COLUMN contract_template.content IS '模板文本，含双花括号变量占位符';
COMMENT ON COLUMN contract_template.variables IS '变量清单 JSON，key / label / required / default';
COMMENT ON COLUMN contract_template.remark IS '备注';
COMMENT ON COLUMN contract_template.created_at IS '创建时间';
COMMENT ON COLUMN contract_template.updated_at IS '更新时间';

-- ============ 等保证据链（20261116_dengbao + dengbao_p2） ============
COMMENT ON TABLE dengbao_policy IS '等保留存策略：单行配置';
COMMENT ON COLUMN dengbao_policy.id IS '主键';
COMMENT ON COLUMN dengbao_policy.retention_days IS '审计日志留存天数，低于 180 代码层拒绝';
COMMENT ON COLUMN dengbao_policy.level IS '拟保测评级 1 / 2 / 3，自查口径';
COMMENT ON COLUMN dengbao_policy.updated_by IS '更新人';
COMMENT ON COLUMN dengbao_policy.updated_at IS '更新时间';

COMMENT ON TABLE audit_log_archive IS '到期审计日志归档表：归档是搬走不是删除';
COMMENT ON COLUMN audit_log_archive.id IS '原 audit_log id，查重用';
COMMENT ON COLUMN audit_log_archive.username IS '操作人';
COMMENT ON COLUMN audit_log_archive.action IS '动作';
COMMENT ON COLUMN audit_log_archive.target_type IS '目标类型';
COMMENT ON COLUMN audit_log_archive.target_id IS '目标 id';
COMMENT ON COLUMN audit_log_archive.detail IS '详情 JSON';
COMMENT ON COLUMN audit_log_archive.ip IS '来源 IP';
COMMENT ON COLUMN audit_log_archive.created_at IS '原日志时间，证据链连续性靠它';
COMMENT ON COLUMN audit_log_archive.archived_at IS '归档动作时间，本身也是留痕';

COMMENT ON TABLE dengbao_asset IS '等保资产台账：只登记不纳管';
COMMENT ON COLUMN dengbao_asset.id IS '主键';
COMMENT ON COLUMN dengbao_asset.name IS '系统 / 设备 / 应用名称';
COMMENT ON COLUMN dengbao_asset.category IS '类型，application / system / device';
COMMENT ON COLUMN dengbao_asset.level IS '拟保级别 1 / 2 / 3';
COMMENT ON COLUMN dengbao_asset.critical IS '重要性，critical 重要 / normal 一般';
COMMENT ON COLUMN dengbao_asset.owner IS '责任人';
COMMENT ON COLUMN dengbao_asset.department IS '所属部门';
COMMENT ON COLUMN dengbao_asset.addr IS '部署地址或网段';
COMMENT ON COLUMN dengbao_asset.remark IS '备注';
COMMENT ON COLUMN dengbao_asset.status IS '状态，active 在用 / retired 下线';
COMMENT ON COLUMN dengbao_asset.creator IS '创建人';
COMMENT ON COLUMN dengbao_asset.created_at IS '创建时间';
COMMENT ON COLUMN dengbao_asset.updated_at IS '更新时间';

COMMENT ON TABLE dengbao_finding IS '等保整改跟踪：测评问题到整改到复测';
COMMENT ON COLUMN dengbao_finding.id IS '主键';
COMMENT ON COLUMN dengbao_finding.item IS '控制点编号与名称';
COMMENT ON COLUMN dengbao_finding.description IS '问题描述';
COMMENT ON COLUMN dengbao_finding.level IS '等级，high / medium / low';
COMMENT ON COLUMN dengbao_finding.status IS '状态，open 待整改 / fixing 整改中 / verified 复测通过 / closed 关闭';
COMMENT ON COLUMN dengbao_finding.owner IS '整改责任人';
COMMENT ON COLUMN dengbao_finding.due_date IS '整改期限';
COMMENT ON COLUMN dengbao_finding.asset_id IS '关联资产 id，0 未关联';
COMMENT ON COLUMN dengbao_finding.verifier IS '复测人';
COMMENT ON COLUMN dengbao_finding.verify_note IS '复测结论';
COMMENT ON COLUMN dengbao_finding.verified_at IS '复测时间';
COMMENT ON COLUMN dengbao_finding.creator IS '创建人';
COMMENT ON COLUMN dengbao_finding.created_at IS '创建时间';
COMMENT ON COLUMN dengbao_finding.updated_at IS '更新时间';

COMMENT ON TABLE dengbao_finding_log IS '整改与复测动作时间线：全部留痕';
COMMENT ON COLUMN dengbao_finding_log.id IS '主键';
COMMENT ON COLUMN dengbao_finding_log.finding_id IS '所属整改项 id';
COMMENT ON COLUMN dengbao_finding_log.action IS '动作，create / update / verify / reopen / close';
COMMENT ON COLUMN dengbao_finding_log.actor IS '操作人';
COMMENT ON COLUMN dengbao_finding_log.note IS '说明';
COMMENT ON COLUMN dengbao_finding_log.created_at IS '创建时间';

COMMENT ON TABLE dengbao_backup_record IS '备份记录证据：DB 备份脚本输出登记';
COMMENT ON COLUMN dengbao_backup_record.id IS '主键';
COMMENT ON COLUMN dengbao_backup_record.source IS '备份对象，database / app / file';
COMMENT ON COLUMN dengbao_backup_record.tool IS '备份工具';
COMMENT ON COLUMN dengbao_backup_record.file_name IS '备份文件名';
COMMENT ON COLUMN dengbao_backup_record.size_bytes IS '文件大小字节';
COMMENT ON COLUMN dengbao_backup_record.checksum IS 'SHA256 校验和';
COMMENT ON COLUMN dengbao_backup_record.started_at IS '开始时间';
COMMENT ON COLUMN dengbao_backup_record.finished_at IS '结束时间';
COMMENT ON COLUMN dengbao_backup_record.status IS '结果，success / failed / partial';
COMMENT ON COLUMN dengbao_backup_record.note IS '备注';
COMMENT ON COLUMN dengbao_backup_record.creator IS '创建人';
COMMENT ON COLUMN dengbao_backup_record.created_at IS '创建时间';

COMMENT ON TABLE dengbao_selfcheck IS '等保合规自查清单：GB/T 22239-2019 控制点自评';
COMMENT ON COLUMN dengbao_selfcheck.id IS '主键';
COMMENT ON COLUMN dengbao_selfcheck.code IS '控制点编号，如 8.1.4';
COMMENT ON COLUMN dengbao_selfcheck.item IS '控制点名称';
COMMENT ON COLUMN dengbao_selfcheck.domain IS '所属安全控制族';
COMMENT ON COLUMN dengbao_selfcheck.level IS '适用级别 1 / 2 / 3';
COMMENT ON COLUMN dengbao_selfcheck.result IS '自查结果，todo / y / no / partial / na';
COMMENT ON COLUMN dengbao_selfcheck.evidence IS '证据链接或说明';
COMMENT ON COLUMN dengbao_selfcheck.note IS '备注';
COMMENT ON COLUMN dengbao_selfcheck.checked_by IS '自查人';
COMMENT ON COLUMN dengbao_selfcheck.checked_at IS '自查时间';
COMMENT ON COLUMN dengbao_selfcheck.created_at IS '创建时间';
COMMENT ON COLUMN dengbao_selfcheck.updated_at IS '更新时间';

-- ============ 督办（20261116_duban） ============
COMMENT ON TABLE duban_task IS '督办任务：会议决议与领导批示到期限到催办';
COMMENT ON COLUMN duban_task.id IS '主键';
COMMENT ON COLUMN duban_task.source_type IS '来源，meeting 会议决议 / directive 领导批示 / other';
COMMENT ON COLUMN duban_task.title IS '任务标题';
COMMENT ON COLUMN duban_task.content IS '任务内容';
COMMENT ON COLUMN duban_task.owner IS '责任人';
COMMENT ON COLUMN duban_task.creator IS '创建人';
COMMENT ON COLUMN duban_task.deadline IS '督办期限';
COMMENT ON COLUMN duban_task.status IS '状态，todo 进行中 / done 已完成';
COMMENT ON COLUMN duban_task.priority IS '优先级，high / mid / low';
COMMENT ON COLUMN duban_task.remind_cnt IS '催办次数';
COMMENT ON COLUMN duban_task.last_remind_at IS '最近催办时间';
COMMENT ON COLUMN duban_task.contract_file_id IS '关联合同文件 id，与合同线咬合';
COMMENT ON COLUMN duban_task.remark IS '备注';
COMMENT ON COLUMN duban_task.created_at IS '创建时间';
COMMENT ON COLUMN duban_task.updated_at IS '更新时间';

COMMENT ON TABLE duban_log IS '督办动作日志';
COMMENT ON COLUMN duban_log.id IS '主键';
COMMENT ON COLUMN duban_log.task_id IS '所属任务 id';
COMMENT ON COLUMN duban_log.action IS '动作，create / remind / done / update';
COMMENT ON COLUMN duban_log.actor IS '操作人';
COMMENT ON COLUMN duban_log.note IS '说明';
COMMENT ON COLUMN duban_log.created_at IS '创建时间';

-- ============ 长期保存国密合规（20261116_longterm） ============
COMMENT ON TABLE longterm_archive IS '长期保存存证：SM3 摘要加 SM4 加密件加时间戳';
COMMENT ON COLUMN longterm_archive.id IS '主键';
COMMENT ON COLUMN longterm_archive.source_type IS '源对象类型，invoice / contract / voucher';
COMMENT ON COLUMN longterm_archive.source_id IS '源对象 id';
COMMENT ON COLUMN longterm_archive.version IS '版本号，同源件重复归档自动加一';
COMMENT ON COLUMN longterm_archive.digest_alg IS '摘要算法，默认 sm3';
COMMENT ON COLUMN longterm_archive.digest IS 'SM3 摘要';
COMMENT ON COLUMN longterm_archive.encrypt_alg IS '加密算法，默认 sm4';
COMMENT ON COLUMN longterm_archive.enc_path IS '加密件路径';
COMMENT ON COLUMN longterm_archive.ts_token IS '时间戳存证 token';
COMMENT ON COLUMN longterm_archive.ts_time IS '登记可信时间';
COMMENT ON COLUMN longterm_archive.remark IS '备注';
COMMENT ON COLUMN longterm_archive.created_at IS '创建时间';
