# Changelog

本项目遵循 [Keep a Changelog](https://keepachangelog.com/zh-CN/1.1.0/) 约定，版本号遵循 [SemVer](https://semver.org/lang/zh-CN/)。

## [Unreleased]

### 规划中
- 移动端离线推送（uni-push 2.0，三端：App 厂商聚合 / 小程序订阅 / H5 WebPush）
- 每日工作台主动式 Agent 的联网信息收集（需外部搜索 API Key，默认关闭）
- 收入确认 / 合同线 P1/P2 补全

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
