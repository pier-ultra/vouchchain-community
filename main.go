package main

import (
	"context"
	"embed"
	"io/fs"
	"net/http"
	"os"
	"strings"

	_ "github.com/gogf/gf/contrib/drivers/mysql/v2"  // 注册 MySQL 驱动（纯 Go，兼容 TiDB/OceanBase/GreatSQL）
	_ "github.com/gogf/gf/contrib/drivers/pgsql/v2"  // 注册 PostgreSQL 驱动（GoFrame 核心不含，需单独引入）
	_ "github.com/gogf/gf/contrib/drivers/sqlite/v2" // 注册 SQLite 驱动（纯 Go，无 CGO；默认数据库）
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/net/ghttp"
	"github.com/gogf/gf/v2/os/gcfg"
	"vouchchain/internal/bootstrap"
	"vouchchain/internal/controller"
	"vouchchain/internal/mcpserver"
	"vouchchain/internal/middleware"
	"vouchchain/pkg/invoiceclass"
	"vouchchain/pkg/kb"
)

// 前端静态资源（Vue3 + Element Plus 构建产物 web/dist）内嵌进二进制，实现单文件自包含分发。
// 运行目录若存在 web/dist（开发/热更）则优先用文件系统，否则回退到内嵌资源。
//
//go:embed all:web/dist
var webDistFS embed.FS

// serveFrontend 注册前端静态资源服务：优先文件系统 web/dist，缺失时回退内嵌资源（单文件分发）。
func serveFrontend(s *ghttp.Server) {
	// 开发/热更：运行目录存在 web/dist 时用文件系统（无需重新编译 Go）
	if _, err := os.Stat("web/dist"); err == nil {
		s.AddSearchPath("web/dist")
		return
	}
	// 单文件分发：从内嵌 FS 提供，SPA 路由回退 index.html
	sub, err := fs.Sub(webDistFS, "web/dist")
	if err != nil {
		g.Log().Fatalf(context.Background(), "[web] 内嵌前端加载失败：%v", err)
	}
	fileServer := http.FileServer(http.FS(sub))
	s.BindHandler("/*", func(r *ghttp.Request) {
		p := strings.TrimPrefix(r.URL.Path, "/")
		// API / SCIM / 文档等内部路径不参与 SPA 回退，保持 404
		if p == "api" || strings.HasPrefix(p, "api/") ||
			p == "scim" || strings.HasPrefix(p, "scim/") ||
			p == "openapi.json" || p == "scalar" || p == "swagger" {
			r.Response.WriteStatus(http.StatusNotFound)
			return
		}
		if p != "" {
			if _, err := fs.Stat(sub, p); err != nil {
				r.Request.URL.Path = "/" // SPA 回退 index.html
			}
		}
		fileServer.ServeHTTP(r.Response.BufferWriter, r.Request)
	})
}

func main() {
	// Docker Compose 一键启动：允许用环境变量 VOUCHCHAIN_DB_LINK 覆盖 DB 连接串（config 文件零改动）
	// 例：pgsql:contracthub:contracthub123@tcp(db:5432)/contracthub
	if link := os.Getenv("VOUCHCHAIN_DB_LINK"); link != "" {
		if adapter, ok := g.Cfg().GetAdapter().(*gcfg.AdapterFile); ok {
			_ = adapter.Set("database.default.link", link)
		}
	}
	// 启动期自动迁移：migrations/*.sql 按序幂等执行 + schema_migrations 记录（自托管分发不再手工跑 SQL）
	if err := bootstrap.RunMigrations(); err != nil {
		g.Log().Fatalf(context.Background(), "[migrate] 启动迁移失败：%v", err)
	}
	// 启动自检：库里有加密密钥但当前进程没有主密钥时，明确告警（不静默降级为"AI 不可用"）
	if os.Getenv("APP_AES_KEY") == "" {
		rows, err := g.DB().Model("model_config").Where("api_key LIKE ?", "enc:v1:%").All()
		if err == nil && len(rows) > 0 {
			g.Log().Warningf(context.Background(),
				"[crypto] 库内有 %d 条加密密钥，但本次启动未设置 APP_AES_KEY：AI/OCR 能力不可用。请带该环境变量重启，或在「模型配置」页重新录入密钥。", len(rows))
		}
	}
	s := g.Server()
	s.Use(middleware.Auth()) // 开关式鉴权：devMode 旁路 / Casdoor JWT 校验（SSO→RBAC 前置）

	// 前端静态资源（Vue3 + Element Plus 构建产物 web/dist），与 /api 同源提供。
	// 单文件分发（内嵌 web/dist）或运行目录存在 web/dist 时均可；二进制不再强制依赖外部目录。
	serveFrontend(s)

	// SCIM 2.0：身份提供方推送用户/组。独立 Bearer token，与用户登录 JWT 是两套凭据；
	// 走 /scim/v2 而非 /api，避免被应用级 RBAC 规则误判（机器到机器接口只认 SCIM_TOKEN）。
	s.Group("/scim/v2", func(group *ghttp.RouterGroup) {
		group.Middleware(middleware.ScimAuth())
		group.GET("/ServiceProviderConfig", controller.ScimServiceProviderConfig)
		group.GET("/ResourceTypes", controller.ScimResourceTypes)
		group.GET("/Schemas", controller.ScimSchemas)
		group.GET("/Users", controller.ScimUserList)
		group.POST("/Users", controller.ScimUserCreate)
		group.GET("/Users/{id}", controller.ScimUserGet)
		group.PUT("/Users/{id}", controller.ScimUserPut)
		group.PATCH("/Users/{id}", controller.ScimUserPatch)
		group.DELETE("/Users/{id}", controller.ScimUserDelete)
		group.GET("/Groups", controller.ScimGroupList)
		group.POST("/Groups", controller.ScimGroupCreate)
		group.GET("/Groups/{id}", controller.ScimGroupGet)
		group.PUT("/Groups/{id}", controller.ScimGroupPut)
		group.PATCH("/Groups/{id}", controller.ScimGroupPatch)
		group.DELETE("/Groups/{id}", controller.ScimGroupDelete)
	})

	// 自生成 OpenAPI + Scalar 接口文档（公开可访问；devMode 旁路鉴权，生产可另行收紧）
	s.BindHandler("GET:/openapi.json", controller.OpenApiSpec)
	s.BindHandler("GET:/scalar", controller.ScalarPage)

	s.Group("/api", func(group *ghttp.RouterGroup) {
		group.Middleware(middleware.Rbac())           // RBAC：角色→操作权限（AC6.7 授权半边）
		group.Middleware(middleware.IsolationGuard()) // AG5：按对象归属做强制隔离与越权告警
		group.GET("/health", controller.Health)
		// SSO 会话：/me 是前端菜单/角色/按钮权限的唯一来源；auth/config 供未登录的登录页读取
		group.GET("/me", controller.Me)
		group.GET("/auth/config", controller.AuthConfig)
		group.POST("/auth/callback", controller.AuthCallback)
		group.GET("/model-config", controller.ModelConfigList)
		group.POST("/model-config", controller.ModelConfigCreate)
		group.GET("/contracts", controller.ContractList)
		group.GET("/contracts/{id}", controller.ContractDetail)
		group.POST("/contracts", controller.ContractCreate)
		group.PUT("/contracts/{id}", controller.ContractUpdate)
		group.DELETE("/contracts/{id}", controller.ContractDelete)
		// AI 生成合同（产出自动进流水线）
		group.POST("/contracts/generate", controller.ContractGenerate)
		// 合同文件上传 + AI 抽取流水线（AC1.1-1.6）
		group.POST("/contracts/upload", controller.ContractFileUpload)
		group.GET("/contracts/files", controller.ContractFileList)
		group.GET("/contracts/files/{id}/clauses", controller.ContractFileClauses)
		group.POST("/contracts/files/{id}/retry", controller.ContractFileRetry)
		// 条款复核修正（AC2.2/2.3）+ 批量复核（AC2.4）+ 复核完成门槛（AC2.6）+ 审计轨迹（AC6.5）
		group.PUT("/contracts/clauses/{id}", controller.ClauseUpdate)
		group.POST("/contracts/clauses/batch-review", controller.ClauseBatchReview)
		group.POST("/contracts/files/{id}/complete-review", controller.ContractFileCompleteReview)
		group.GET("/audit-log", controller.AuditLogList)
		group.GET("/audit-log/export", controller.AuditLogExport) // AK4.5 审计导出
		// 抽取结果导出（AC1.7）
		group.GET("/contracts/files/{id}/export", controller.ContractFileExport)
		// A6 合同模板库 + 变量套打
		group.GET("/contract-templates", controller.ContractTemplateList)
		group.POST("/contract-templates", controller.ContractTemplateCreate)
		group.PUT("/contract-templates/{id}", controller.ContractTemplateUpdate)
		group.DELETE("/contract-templates/{id}", controller.ContractTemplateDelete)
		group.POST("/contract-templates/render", controller.ContractTemplateRender)
		// 义务清单（AC3.1-3.7）+ 证据附件（AC3.6）
		group.POST("/contracts/files/{id}/obligations/generate", controller.ObligationGenerate)
		group.GET("/obligations", controller.ObligationList)
		group.PUT("/obligations/{id}", controller.ObligationUpdate)
		group.POST("/obligations/{id}/evidence", controller.ObligationEvidenceUpload)
		group.GET("/obligations/{id}/evidence", controller.ObligationEvidenceDownload)
		// 原文定位（AC1.5）/ 版本历史（AC6.2）/ 双版对照（AC4）/ 自然语言查询（AC5）/ 电子签适配（AC7）
		group.GET("/contracts/files/{id}/text", controller.ContractFileText)
		group.GET("/contracts/files/{id}/versions", controller.ContractFileVersions)
		group.POST("/query", controller.NLQuery)
		group.POST("/contracts/files/{id}/sign", controller.SignCreate)
		group.GET("/sign-tasks", controller.SignList)
		group.POST("/sign-tasks/{id}/poll", controller.SignPoll)
		// 审批流（AC6.3/6.4）+ 消息中心
		group.POST("/approval-templates", controller.ApprovalTemplateCreate)
		group.GET("/approval-templates", controller.ApprovalTemplateList)
		group.POST("/approvals", controller.ApprovalInstanceCreate)
		group.GET("/approvals", controller.ApprovalInstanceList)
		group.GET("/approvals/{id}", controller.ApprovalInstanceDetail)
		group.POST("/approvals/{id}/act", controller.ApprovalAct)
		group.POST("/approvals/{id}/addsign", controller.ApprovalAddSign)
		group.POST("/approvals/{id}/transfer", controller.ApprovalTransfer)
		group.POST("/approvals/{id}/cancel", controller.ApprovalCancel)
		group.GET("/approvals/{id}/events", controller.ApprovalEvents)
		group.GET("/notifications", controller.NotificationList)
		group.POST("/notifications/{id}/read", controller.NotificationRead)
		group.POST("/notifications/read-all", controller.NotificationReadAll)
		// 发票线（US-I1~I8）
		group.POST("/invoices/import", controller.InvoiceImport)
		group.GET("/invoices", controller.InvoiceList)
		group.POST("/invoices/{id}/integrity", controller.InvoiceIntegrity)
		group.POST("/invoices/{id}/verify", controller.InvoiceVerify)
		group.POST("/invoices/{id}/archive", controller.InvoiceArchive)
		group.POST("/invoices/{id}/link-voucher", controller.InvoiceLinkVoucher)
		group.GET("/invoices/{id}/chain", controller.InvoiceChain)
		group.GET("/invoices/{id}/chain/export", controller.InvoiceChainExport)   // AI8.4 联查导出
		group.POST("/invoices/red-flush-scan", controller.InvoiceRedFlushScan)    // A2 红冲预警：全量扫描落库
		group.GET("/invoices/red-flush-alerts", controller.InvoiceRedFlushAlerts) // A2 红冲预警列表
		group.GET("/archives", controller.ArchiveList)                            // AI5.5 归档检索
		group.GET("/archives/{id}/download", controller.ArchiveDownload)          // AI5.5 归档调阅
		group.POST("/vouchers", controller.VoucherCreate)
		group.GET("/vouchers", controller.VoucherList)
		group.POST("/receipts", controller.ReceiptCreate)
		group.GET("/receipts", controller.ReceiptList)
		group.POST("/receipts/auto-match", controller.ReceiptAutoMatch)
		// A5 长期保存/国密合规封装层（2028 财会 9 号窗口）
		group.POST("/longterm/archive", controller.LongTermArchive)
		group.GET("/longterm", controller.LongTermList)
		group.POST("/longterm/{id}/verify", controller.LongTermVerify)
		// P2P 闭环补格：应付台账 + AI 分录草稿（社区版免费）
		group.POST("/payables/generate", controller.PayableGenerate)
		group.GET("/payables", controller.PayableList)
		group.PUT("/payables/{id}", controller.PayableUpdate)
		group.POST("/payables/{id}/pay", controller.PayablePay)
		group.GET("/account-mappings", controller.AccountMappingList)
		group.POST("/account-mappings", controller.AccountMappingSave)
		group.POST("/journal/drafts", controller.JournalDraftCreate)
		group.GET("/journal/drafts", controller.JournalDraftList)
		group.PUT("/journal/drafts/{id}", controller.JournalDraftUpdate)
		group.POST("/journal/drafts/{id}/confirm", controller.JournalDraftConfirm)
		// 总账桥接（社区版免费：金蝶 K3 引入文件 / 用友 U8 API / 金蝶云星空 WebAPI）
		// 任务中心 WebSocket 实时进度
		group.GET("/ws/tasks", controller.WsTasks)
		// Core K3 多组织法人主体（AK3.1–AK3.4）
		group.GET("/legal-entities", controller.LegalEntityList)
		group.POST("/legal-entities", controller.LegalEntityCreate)
		group.PUT("/legal-entities/{id}", controller.LegalEntityUpdate)
		group.DELETE("/legal-entities/{id}", controller.LegalEntityDelete)
		group.GET("/context", controller.ContextGet)
		group.POST("/context/switch", controller.ContextSwitch)
		// Core K2 组织树（AK2.1–AK2.5）
		group.GET("/org/tree", controller.OrgNodeTree)
		group.GET("/org/nodes/{id}/subtree", controller.OrgNodeSubtree) // 父子树递归展开（含子树成员）
		group.POST("/org/nodes", controller.OrgNodeCreate)
		group.PUT("/org/nodes/{id}", controller.OrgNodeUpdate)
		group.DELETE("/org/nodes/{id}", controller.OrgNodeDelete)
		group.POST("/org/nodes/import", controller.OrgNodeImport)
		group.GET("/org/members", controller.OrgMemberList)
		group.POST("/org/members", controller.OrgMemberAttach)
		group.DELETE("/org/members/{id}", controller.OrgMemberRemove)
		// Core K5 主数据雏形（AK5.1–AK5.5）
		group.GET("/master-data/types", controller.MasterDataTypeList)
		group.GET("/master-data", controller.MasterDataList)
		group.POST("/master-data", controller.MasterDataCreate)
		group.PUT("/master-data/{id}", controller.MasterDataUpdate)
		group.DELETE("/master-data/{id}", controller.MasterDataDelete)
		group.GET("/master-data/export", controller.MasterDataExport)
		group.GET("/master-data/links", controller.MasterDataLinkList)
		group.POST("/master-data/links", controller.MasterDataLinkCreate)
		group.DELETE("/master-data/links/{id}", controller.MasterDataLinkDelete)
		// ③ 发票线 P1：US-I9 归档调度（AI9.1–AI9.4）
		group.GET("/archive-schedules", controller.ArchiveScheduleList)
		group.POST("/archive-schedules", controller.ArchiveScheduleCreate)
		group.PUT("/archive-schedules/{id}", controller.ArchiveScheduleUpdate)
		group.DELETE("/archive-schedules/{id}", controller.ArchiveScheduleDelete)
		group.POST("/archive-schedules/{id}/toggle", controller.ArchiveScheduleToggle)
		group.POST("/archive-schedules/{id}/run", controller.ArchiveScheduleRunNow)
		group.GET("/archive-schedule-runs", controller.ArchiveScheduleRunList)
		group.GET("/archive-schedule-runs/{id}/items", controller.ArchiveScheduleRunItems)
		group.GET("/archive-exceptions", controller.ArchiveExceptionList)
		group.POST("/archive-exceptions/{id}/handle", controller.ArchiveExceptionHandle)
		// 产品② US-G4 合规策略引擎（AG4.1–AG4.6）
		group.GET("/compliance/rules", controller.ComplianceRuleList)
		group.POST("/compliance/rules", controller.ComplianceRuleCreate)
		group.PUT("/compliance/rules/{id}", controller.ComplianceRuleUpdate)
		group.POST("/compliance/rules/{id}/toggle", controller.ComplianceRuleToggle)
		group.GET("/compliance/rules/{id}/history", controller.ComplianceRuleHistory)
		group.GET("/compliance/violations", controller.ComplianceViolationList)
		group.POST("/compliance/violations/{id}/handle", controller.ComplianceViolationHandle)
		group.POST("/compliance/check", controller.ComplianceCheck)
		group.GET("/compliance/summary", controller.ComplianceSummary)
		// 产品② US-G6 MDM 实体解析与去重（AG6.1–AG6.5）
		group.GET("/mdm/rules", controller.MdmRuleList)
		group.POST("/mdm/rules", controller.MdmRuleSave)
		group.PUT("/mdm/rules/{id}", controller.MdmRuleSave)
		group.POST("/mdm/scan", controller.MdmScan)
		group.GET("/mdm/duplicates", controller.MdmDuplicateList)
		group.POST("/mdm/duplicates/{id}/merge", controller.MdmMerge)
		group.POST("/mdm/duplicates/{id}/ignore", controller.MdmDuplicateIgnore)
		group.GET("/mdm/mappings", controller.MdmMappingList)
		group.GET("/mdm/summary", controller.MdmSummary)
		// 产品② US-G1 RBAC 配置化（AG1.1–AG1.6）
		group.GET("/rbac/roles", controller.RoleList)
		group.POST("/rbac/roles", controller.RoleSave)
		group.PUT("/rbac/roles/{id}", controller.RoleSave)
		group.DELETE("/rbac/roles/{id}", controller.RoleDelete)
		group.POST("/rbac/roles/{id}/toggle", controller.RoleToggle)
		group.GET("/rbac/bindings", controller.RoleBindingList)
		group.POST("/rbac/bindings", controller.RoleBindingSave)
		group.GET("/rbac/resources", controller.RoleResources)
		group.GET("/rbac/effective", controller.RoleEffective)
		// 产品② SCIM 身份同步：状态与日志只读，组→角色映射仅管理员可改
		group.GET("/scim/status", controller.ScimStatus)
		group.GET("/scim/users", controller.ScimAdminUsers)
		group.DELETE("/scim/users/{id}", controller.ScimUserPurge)
		group.GET("/scim/groups", controller.ScimAdminGroups)
		group.GET("/scim/logs", controller.ScimAdminLogs)
		group.GET("/scim/role-mappings", controller.ScimRoleMapList)
		group.POST("/scim/role-mappings", controller.ScimRoleMapSave)
		group.PUT("/scim/role-mappings", controller.ScimRoleMapSave)
		group.DELETE("/scim/role-mappings/{id}", controller.ScimRoleMapDelete)
		// 产品② US-G5 AG5.1–AG5.5 强隔离：策略配置 / 生效范围 / 越权记录
		group.GET("/isolation/dimensions", controller.IsoDimensions)
		group.GET("/isolation/policies", controller.IsoPolicyList)
		group.POST("/isolation/policies", controller.IsoPolicySave)
		group.PUT("/isolation/policies/{id}", controller.IsoPolicySave)
		group.DELETE("/isolation/policies/{id}", controller.IsoPolicyDelete)
		group.POST("/isolation/policies/{id}/toggle", controller.IsoPolicyToggle)
		group.GET("/isolation/effective", controller.IsoEffective)
		group.GET("/isolation/violations", controller.IsoViolationList)
		group.POST("/isolation/violations/{id}/handle", controller.IsoViolationHandle)
		group.GET("/isolation/summary", controller.IsoSummary)
		// 产品① US-C8 相对方风险画像（AC8.1–AC8.5）
		group.GET("/party-profiles", controller.PartyProfileList)
		group.GET("/party-profile", controller.PartyProfileDetail)
		group.GET("/party-profile/summary", controller.PartyProfileSummary)
		group.GET("/party-profile/files", controller.PartyProfileFiles)
		group.POST("/party-profile/link", controller.PartyProfileLink)
		// AI 对话助手（对话即操作）：会话/消息，编排走 pkg/agent 工具循环
		group.POST("/chat", controller.ChatSend)
		group.POST("/chat/stream", controller.ChatStream)
		group.GET("/chat/sessions", controller.ChatSessions)
		group.GET("/chat/sessions/{id}/messages", controller.ChatMessages)
		group.PUT("/chat/sessions/{id}", controller.ChatSessionRename)
		group.DELETE("/chat/sessions/{id}", controller.ChatSessionDelete)
		// MCP 接入状态（admin 自检：端点/工具数/请求头说明；不回显 token）
		group.GET("/mcp/status", func(r *ghttp.Request) {
			r.Response.WriteJsonExit(g.Map{"code": 0, "data": mcpserver.Status()})
		})
		// 法律条款知识库检索（向量+重排，欠费时关键词降级）
		group.GET("/kb/search", controller.KBSearch)
		// 发票自动分类（规则配置 + AI 兜底 + 统计）
		group.GET("/invoice-class/rules", controller.InvoiceClassRuleList)
		group.POST("/invoice-class/rules", controller.InvoiceClassRuleSave)
		group.DELETE("/invoice-class/rules/{id}", controller.InvoiceClassRuleDelete)
		group.POST("/invoice-class/run", controller.InvoiceClassRun)
		group.GET("/invoice-class/stats", controller.InvoiceClassStats)
		// 费控三件套（报销切片/预算/差标）
		group.POST("/expense-splits", controller.ExpenseSplitCreate)
		group.GET("/expense-splits", controller.ExpenseSplitList)
		group.DELETE("/expense-splits/{id}", controller.ExpenseSplitDelete)
		group.GET("/budgets/usage", controller.BudgetUsageList)
		group.POST("/budgets", controller.BudgetSave)
		group.DELETE("/budgets/{id}", controller.BudgetDelete)
		group.GET("/travel-standards", controller.TravelStandardList)
		group.POST("/travel-standards", controller.TravelStandardSave)
		group.DELETE("/travel-standards/{id}", controller.TravelStandardDelete)
		group.POST("/travel-check", controller.TravelCheck)
		// 切片凭证联动 + Agent Skill 包
		group.POST("/expense-splits/{id}/voucher", controller.SplitVoucher)
		// 收入确认（乙方轻量版）+ 用印台账
		group.PUT("/revrec/mode/{obligationId}", controller.RevRecSetMode)
		group.POST("/revrec/suggest", controller.RevRecSuggest)
		group.POST("/revrec/confirm/{id}", controller.RevRecConfirm)
		group.GET("/revrec/list", controller.RevRecList)
		group.GET("/revrec/ledger", controller.RevRecLedger)
		group.POST("/revrec/receipt", controller.RevRecReceiptAdd)
		group.GET("/revrec/receipts", controller.RevRecReceiptList)
		group.POST("/revrec/settlement", controller.RevRecSettlementAdd)
		group.GET("/revrec/settlements", controller.RevRecSettlementList)
		group.GET("/revrec/ledger/v2", controller.RevRecLedgerV2)
		group.GET("/revrec/ar-cr-diff", controller.RevRecArCrDiff)
		group.GET("/revrec/monthly", controller.RevRecMonthly)
		group.GET("/revrec/export", controller.RevRecExport) // R-A14 审计口径 Excel 导出
		group.POST("/seals", controller.SealCreate)
		group.GET("/seals", controller.SealList)
		group.PUT("/seals/{id}/status", controller.SealStatus)
		group.POST("/seals/{id}/transfer", controller.SealTransfer) // R-B10 印章移交
		group.POST("/seal-applies", controller.SealApplyCreate)
		group.GET("/seal-applies", controller.SealApplyList)
		group.GET("/seal-applies/export", controller.SealApplyExport) // R-B6 台账导出
		group.POST("/seal-applies/{id}/approve", controller.SealApplyApprove)
		group.POST("/seal-applies/{id}/stamp", controller.SealApplyStamp)
		group.POST("/seals/approval-rules", controller.SealApprovalRuleSave) // R-B3 按类型配置审批模板
		group.GET("/seals/approval-rules", controller.SealApprovalRuleList)
		group.GET("/seal-risks", controller.SealRisks)
		group.GET("/es/config", controller.ESConfigGet) // R-B11 外部电子签平台配置
		group.POST("/es/config", controller.ESConfigSave)
		group.POST("/seal-applies/{id}/es-stamp", controller.SealESStamp) // R-B11 电子章记录回填
		group.GET("/es/records", controller.ESRecordList)
		// 等保证据链（企业侧，v23 线 C）
		group.GET("/dengbao/policy", controller.DengbaoPolicyGet)
		group.PUT("/dengbao/policy", controller.DengbaoPolicySave) // R-C2 留存策略
		group.GET("/dengbao/retention/status", controller.DengbaoRetentionStatus)
		group.POST("/dengbao/retention/archive", controller.DengbaoArchiveExpired) // R-C2 到期归档
		group.GET("/dengbao/perm-chain", controller.DengbaoPermChain)              // R-C3 权限变更证据链
		group.GET("/dengbao/perm-chain/export", controller.DengbaoPermChainExport)
		group.GET("/dengbao/evidence/blocks", controller.DengbaoEvidenceBlocks) // R-C4 三块证据统计
		group.GET("/dengbao/evidence/export", controller.DengbaoEvidenceExport)
		group.GET("/dengbao/evidence/pack", controller.DengbaoEvidencePack) // R-C9 zip+清单
		group.POST("/dengbao/assets", controller.DengbaoAssetSave)          // R-C1 资产台账
		group.GET("/dengbao/assets", controller.DengbaoAssetList)
		group.GET("/dengbao/assets/stats", controller.DengbaoAssetStats)
		group.POST("/dengbao/findings", controller.DengbaoFindingCreate) // R-C5 整改跟踪
		group.GET("/dengbao/findings", controller.DengbaoFindingList)
		group.GET("/dengbao/findings/stats", controller.DengbaoFindingStats)
		group.POST("/dengbao/findings/{id}/verify", controller.DengbaoFindingVerify)
		group.POST("/dengbao/findings/{id}/reopen", controller.DengbaoFindingReopen)
		group.POST("/dengbao/findings/{id}/close", controller.DengbaoFindingClose)
		group.GET("/dengbao/findings/{id}/log", controller.DengbaoFindingLog)
		group.POST("/dengbao/backups", controller.DengbaoBackupAdd) // R-C8 备份记录
		group.GET("/dengbao/backups", controller.DengbaoBackupList)
		group.GET("/dengbao/backups/stats", controller.DengbaoBackupStats)
		// R-C7 合规自查清单（GB/T 22239-2019 主要控制点自评）
		group.POST("/duban/tasks", controller.DubanCreate)
		group.GET("/duban/tasks", controller.DubanList)
		group.GET("/duban/board", controller.DubanBoard)
		group.POST("/duban/tasks/{id}/remind", controller.DubanRemind)
		group.POST("/duban/tasks/{id}/done", controller.DubanDone)
		group.GET("/duban/tasks/{id}/log", controller.DubanLog)
		group.GET("/duban/stats", controller.DubanStats)
		group.POST("/skills", controller.SkillUpload)
		group.GET("/skills", controller.SkillList)
		group.PUT("/skills/{id}/toggle", controller.SkillToggle)
		group.DELETE("/skills/{id}", controller.SkillDelete)
		// 文件中心（local 默认 / SeaweedFS 可插拔，兼容模式）
		group.POST("/files", controller.FileUpload)
		group.GET("/files", controller.FileList)
		group.GET("/files/{id}/download", controller.FileDownload)
		group.GET("/files/{id}/preview", controller.FilePreview)
		group.DELETE("/files/{id}", controller.FileDelete)
		// 每日工作台：概览聚合 + 晨/晚总结（主动式 Agent 推送，WS 广播）
		group.GET("/daily", controller.DailyOverview)
		group.POST("/daily/brief", controller.DailyBrief)
	})
	if mcpHandler := mcpserver.Setup(); mcpHandler != nil {
		s.Group("/mcp", func(group *ghttp.RouterGroup) {
			group.Middleware(mcpserver.McpAuth)
			group.ALL("/", func(r *ghttp.Request) {
				mcpHandler.ServeHTTP(r.Response.Writer, r.Request)
			})
		})
	} else {
		g.Log().Warning(context.Background(), "[mcp] MCP_TOKEN 未配置，MCP 服务未启用（外部 AI 客户端接入需配置后重启）")
	}
	// AI9.2 后台调度器：60s 扫描到期配置，原子抢锁，多实例不重复归档
	controller.StartArchiveScheduler()
	// 每日工作台：08:00 晨间简报 / 21:00 晚间总结（进程内单实例自托管，防同日重复）
	controller.StartDailyScheduler()
	// 法律条款知识库：空库时种子民法典条款；后台补向量（embedding 服务可用时）
	go func() {
		ctx := context.Background()
		if n, err := kb.SeedIfEmpty(ctx); err != nil {
			g.Log().Warningf(ctx, "[kb] 知识库种子失败：%v", err)
		} else if n > 0 {
			g.Log().Infof(ctx, "[kb] 已种子 %d 条法律条款", n)
		}
		if k := kb.Enrich(ctx); k > 0 {
			g.Log().Infof(ctx, "[kb] 已向量化 %d 条条款", k)
		}
		// 发票分类规则种子（用户 11-15：发票自动分类）
		if n, err := invoiceclass.SeedRulesIfEmpty(ctx); err != nil {
			g.Log().Warningf(ctx, "[invoice-class] 规则种子失败：%v", err)
		} else if n > 0 {
			g.Log().Infof(ctx, "[invoice-class] 已种子 %d 条分类规则", n)
		}
	}()
	s.Run()
}
