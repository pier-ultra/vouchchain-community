# Changelog

本项目遵循 [Keep a Changelog](https://keepachangelog.com/zh-CN/1.1.0/) 约定，版本号遵循 [SemVer](https://semver.org/lang/zh-CN/)。

## [Unreleased]

### 新增（已合并 main，尚未打 tag）
- MySQL 兼容（MySQL 8 / TiDB / OceanBase 等 MySQL 兼容库）：`pkg/dbutil` 增加 `IsMySQL`/`IsPG`，迁移层按方言改写 DDL、`ON CONFLICT`→`ON DUPLICATE KEY`、部分索引降级、保留字转义；`kb/search` 在非 PG 库自动降级为关键词模式
- 跨平台发布包构建脚本 `scripts/build-release.py`（linux/windows/darwin × amd64/arm64，纯 Go `CGO_ENABLED=0`）
- openGauss 5 兼容（华为，PG 内核）：复用 pgsql 驱动，`SELECT version()` 自动识别方言；迁移层裁剪 pgvector（DO 块/扩展/vector 列/向量索引）、`ADD COLUMN IF NOT EXISTS` 改列存在性检查、`ON CONFLICT` 改 `ON DUPLICATE KEY UPDATE`（非键列 no-op）；知识库无 pgvector 自动降级关键词。openGauss 5.0.0 空库实测 29/29 迁移 + 73 表 + seed 全量 + 11 端点 200 + 写路径落库，SQLite/MySQL 回归无退化

### 安全
- README / README.en 统一到最新 tag v0.3.1；新增醒目的 devMode 公网风险警告与「生产部署必做清单」（关闭 devMode、SSO 验签、替换演示凭据、限制端口暴露）

### 规划中
- 移动端离线推送（uni-push 2.0，三端：App 厂商聚合 / 小程序订阅 / H5 WebPush）
- 每日工作台主动式 Agent 的联网信息收集（需外部搜索 API Key，默认关闭）
- 收入确认 / 合同线 P1/P2 补全

## [0.3.1] - 2026-11-18

### 新增
- 单文件自包含分发：`go:embed web/dist`，二进制不再旁置前端目录
- 可插拔缓存层 `pkg/cache`（memory / kvrocks），AI 对话上下文注入兼容 DB / KV
- 移动端 UI 加深：登录页新增服务器地址输入框，appid 定稿

## [0.3.0] - 2026-11-17

### 新增
- 双数据库兼容：默认 SQLite（零配置单文件、无需装任何数据库），也支持 PostgreSQL（多用户/生产）
- 冷启动补齐核心表（`migrations/0001_core_tables.sql`）：修复全新数据库启动即中断的问题
- 一键构建脚本 `scripts/build.sh`（纯 Go sqlite 驱动 + `-trimpath -ldflags "-s -w"`，单二进制约 36MB，无需 gcc）

### 修复
- 全新数据库（含 PostgreSQL）冷启动会因核心表缺失/建表顺序依赖而中断迁移，导致服务起不来
- SQLite 方言：`BIGSERIAL`→`INTEGER`、`JSONB`→`TEXT`、pg 正则 `~`→`LIKE` 前缀匹配、pgvector 列与索引降级、`ALTER ADD COLUMN IF NOT EXISTS` 改为列存在性检查

### 发布
- 跨平台发布包 6 个（v0.3.0）：linux / windows / darwin × amd64 / arm64，均为纯 Go 单二进制（`CGO_ENABLED=0`，无需 gcc、无需装数据库），zip 内含 `config.yaml.example` 与 `web/dist`

## [0.2.0] - 2026-11-16

### 变更
- 双版本拆分：社区版（Apache 2.0）+ 专业版（企业许可）
- AI 深度条款审查 / 多主体合并报表桥接 / 等保自动测评报告 移入专业版（社区版移除）
- 移动端 App 纳入社区版（uni-app 三端，H5 / 小程序可自建）

## [0.1.0] - 2026-11-16

### Added
- 合同台账：起草 / 审批 / 签署 / 履约 / 变更 / 归档 全生命周期
- 发票中心：数电票 / OFD / XML 导入、税务验真、红冲预警、长期保存（国密）
- 用印管理：申请 / 审批 / 盖章 / 电子章回填 / 印章移交 / 风险看板
- 等保 2.0 审计：GB/T 22239-2019，留存下限 180 天（代码层拒绝）、归档非删除
- 收入确认：履约义务 / 时点-时段模式 / 结算 / AR-CR 差异 / 月度审计口径
- 公文督办：登记 / 催办 / 办结 / 留痕 / 逾期识别
- **每日工作台**（主动式 Agent）：概览聚合 + 晨 / 晚总结定时推送（WS 广播 + AI 润色，未配模型降级纯规则）
- 治理底座：RBAC / SCIM v2 / 数据隔离 / 审计日志 / MCP 服务 / Casdoor SSO / 知识库
- 前端：Web（Vue3 + Element Plus + ECharts，亮 / 暗双主题）+ 移动端（uni-app，H5 / 小程序 / App）
- 产品名定名 **VouchChain**（模块 `vouchchain`，二进制 `vouchchain`）

### Changed
- 前端 vendor 分包优化（Element Plus 独立 chunk）
- 移动端发票导入接后端（OFD / XML，重复可 force 放行）
- AI 对话接入讯飞四能力（语音 / 图片 / 文件朗读，活参数）

### Fixed
- Dengbao.vue 整页数据回填为空
- 请求失败兜底（响应拦截器业务码非 0 提示）
- 每日工作台聚合层裸条件计数（`seal_apply` 待办）
