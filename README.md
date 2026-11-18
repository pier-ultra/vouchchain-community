# VouchChain · 合同与履约中枢（社区版）

**VouchChain** 是一套面向中小企业财务/合规团队的**合同与履约中枢**：把合同、发票、用印、等保 2.0、收入确认、公文督办等业务侧流程，收敛为「业务侧 → 会计已确认凭证 → 客户既有总账」的可审计桥接。

> 版本：**v0.3.1 社区版**（Apache 2.0，与最新 tag 一致；完整变更见 [CHANGELOG.md](CHANGELOG.md)）。另有**专业版（企业许可）**，含 3 个高级功能，见下方对比表。
>
> 定位：**纯开源社区版**，不做 open core 分层、不做 license key、不做功能开关收费。AI 深度条款审查 / 多主体合并报表桥接 / 等保自动测评报告 为专业版；其余核心业务与治理底座全部社区版免费。
>
> 红线：不做总账本体 / 报表 / 报税，只做业务侧到会计凭证的桥接；AI 只出建议、须人审入账；未配外部接口时明确失败不谎报。

> ⚠️ **安全警告：不要让默认配置上公网**
>
> 示例配置与 `docker compose` 默认 `devMode=true`，这是**本地演示态**：任意账号可登录、可在登录页自选角色、可用请求头 `X-User-Role` 模拟管理员，`/scalar` 接口文档免登录。这是为「一键看效果」设计的，**不是安全配置**。
>
> 任何非本机可信环境（公网服务器、云主机、共享内网）部署前，必须先完成下方 [生产部署必做清单](#生产部署必做清单)：关闭 devMode、配置 Casdoor 验签证书、替换演示口令与密钥、限制端口来源。**照抄演示命令裸奔公网 = 业务数据全公开。**

## 仓库地址

开源免费 · **Apache 2.0**，欢迎 Star / Fork / 提 Issue：

- **GitHub**：[github.com/pier-ultra/vouchchain-community](https://github.com/pier-ultra/vouchchain-community)
- **Gitee**：[gitee.com/pier-ultra/vouchchain-community](https://gitee.com/pier-ultra/vouchchain-community)

> 国内访问 GitHub 不稳时，可走 **Gitee** 或 `ghproxy` 镜像。

## 社区版 vs 专业版

| 功能 | 社区版（免费 · Apache 2.0） | 专业版（企业许可） |
|---|---|---|
| 合同台账 / 生命周期（起草·审批·签署·履约·归档） | ✅ | ✅ |
| 发票中心（数电票 / OFD / XML 导入、税务验真、红冲预警） | ✅ | ✅ |
| 用印管理（申请·审批·盖章记录·电子章回填·印章移交） | ✅ | ✅ |
| 等保 2.0 留存与证据链（GB/T 22239-2019，留存 ≥180 天） | ✅ | ✅ |
| 收入确认（履约义务·确认模式·结算·AR-CR 差异） | ✅ | ✅ |
| 公文督办（登记·催办·办结·逾期自动识别） | ✅ | ✅ |
| 每日工作台（Agent 聚合·晨/晚总结定时推送） | ✅ | ✅ |
| 治理底座（RBAC / SCIM v2 / 数据隔离 / 审计日志 / MCP / Casdoor SSO） | ✅ | ✅ |
| 移动端 App（uni-app 源码 + H5 / 微信小程序构建） | ✅ | ✅ |
| **AI 深度条款审查（含版本对照）** | — | ✅ |
| **多主体合并报表桥接（总账桥接）** | — | ✅ |
| **等保自动测评报告（合规自查清单）** | — | ✅ |
| Android / iOS 原生打包（证书·签名·上架） | 增值服务 | 增值服务 |

## 核心能力（社区版）

| 模块 | 说明 |
|---|---|
| **合同台账** | 全生命周期：起草 / 审批 / 签署 / 履约 / 变更 / 归档 |
| **发票中心** | 数电票 / OFD / XML 导入、税务验真、红冲预警 |
| **用印管理** | 用印申请、审批、盖章记录、电子章回填、印章移交 |
| **等保 2.0 审计** | GB/T 22239-2019，审计留存 ≥180 天（低于下限代码层拒绝）、归档非删除 |
| **收入确认** | 履约义务 / 确认模式（时点 / 时段）/ 结算 / AR-CR 差异 |
| **公文督办** | 任务登记、催办、办结、留痕、逾期自动识别 |
| **每日工作台** | 主动式 Agent：聚合本周关注 / 进行中追踪 / 下一步建议；晨 / 晚总结定时推送（WS 广播 + AI 润色，未配模型自动降级纯规则） |
| **治理底座** | RBAC / SCIM v2 全量 CRUD / 数据隔离 / 审计日志 / MCP 服务 / Casdoor SSO |

## 界面预览（社区版）

Web 管理台（1440×900）：

<p align="center">
  <img src="screenshots/web/01_login.png" width="45%" alt="登录">
  <img src="screenshots/web/02_dashboard.png" width="45%" alt="工作概览">
  <img src="screenshots/web/03_contracts.png" width="45%" alt="合同台账">
  <img src="screenshots/web/04_invoices.png" width="45%" alt="发票中心">
  <img src="screenshots/web/05_seals.png" width="45%" alt="用印管理">
  <img src="screenshots/web/06_revrec.png" width="45%" alt="收入确认">
  <img src="screenshots/web/07_dengbao.png" width="45%" alt="等保证据">
  <img src="screenshots/web/08_duban.png" width="45%" alt="公文督办">
  <img src="screenshots/web/09_chat.png" width="45%" alt="AI 助手">
  <img src="screenshots/web/10_skills.png" width="45%" alt="AI 技能">
</p>

移动端 App（uni-app H5 预览，390×844）：

<p align="center">
  <img src="screenshots/app/01_login.png" width="22%" alt="登录">
  <img src="screenshots/app/02_home.png" width="22%" alt="首页">
  <img src="screenshots/app/03_contracts.png" width="22%" alt="合同">
  <img src="screenshots/app/04_approvals.png" width="22%" alt="审批待办">
  <img src="screenshots/app/05_invoices.png" width="22%" alt="发票">
  <img src="screenshots/app/06_seals.png" width="22%" alt="用印">
  <img src="screenshots/app/07_dengbao.png" width="22%" alt="等保证据">
  <img src="screenshots/app/08_revrec.png" width="22%" alt="收入确认">
  <img src="screenshots/app/09_duban.png" width="22%" alt="公文督办">
  <img src="screenshots/app/10_mine.png" width="22%" alt="我的">
</p>

> 截图取自社区版构建（演示数据入库）；专业版专属的「条款审查 / 桥接 / 自查」入口不出现在社区版菜单。

## 技术栈

- **后端**：GoFrame v2.10.4（单二进制，跨平台）
- **Web**：Vue 3 + Element Plus + ECharts（亮 / 暗双主题）
- **移动端**：uni-app（Vue 3），H5 / 微信小程序 / App 三端
- **AI**：Eino LLM Agent（知识库问答 / 简报润色）
- **实时**：WebSocket 广播（`daily_brief` 等事件）
- **数据库**：SQLite（默认，零配置单文件，纯 Go 驱动无需 gcc）/ PostgreSQL（多用户/生产，可选 pgvector）/ MySQL 8 及其兼容库（TiDB、OceanBase 等）/ openGauss 5（华为，PG 内核，走 pgsql 驱动自动识别方言；无 pgvector，知识库降级关键词检索）。后两条国产/兼容线已在 main，尚未打 tag。

## 快速开始

```bash
# 1. 后端
go build -o vouchchain ./
cp config/config.yaml.example config/config.yaml   # 按环境填写（DB、密钥）
./vouchchain                                       # 默认 :8000

# 2. Web（开发）
cd web && npm install && npm run dev

# 3. 移动端（H5 预览）
cd mobile && npm install && npm run dev:h5
```

运行期需 `web/dist`（或 `mobile/dist`）旁置；生产构建见 `.goreleaser.yaml`。

## 接口文档

后端启动后访问 **http://localhost:8000/scalar**（免登录），页面由 [Scalar](https://github.com/scalar/scalar) 渲染，数据来源是自生成的 `GET /openapi.json`（覆盖全部路由，224 个路径）。

Scalar 为**本地静态 bundle**，不依赖任何 CDN（离线 / 内网可用）。`cd web && npm install && npm run build` 构建时，`scripts/copy-scalar.mjs` 会自动把 `node_modules/@scalar/api-reference/dist/browser/standalone.js` 复制到 `web/dist/vendor/scalar-standalone.js`，后端按 `web/dist` 提供静态资源。

## Docker Compose 一键启动

仓库内置 `Dockerfile` + `docker-compose.yml`，**免装 Go/Node/PostgreSQL**，一条命令起全套（Web 管理台 + 后端 + 内置 PG）：

```bash
docker compose up -d --build      # 首次构建需几分钟
# 浏览器打开 http://localhost:8000 （dev 演示态免登录，角色在登录页选择）
```

说明：

- **多阶段镜像**：镜像内自行构建前端 `web/dist`（Node 22）与 Go 后端（CGO_ENABLED=0 静态单二进制），宿主机只需 Docker。
- **数据库**：内置 `postgres:16-alpine`，数据存 `pgdata` 卷（`docker compose down` 不清数据；`down -v` 才清）。
- **连接串注入**：后端通过环境变量 `VOUCHCHAIN_DB_LINK` 覆盖 config 的 DB 连接串（config 文件零改动，DB 密码不入库、不入镜像）。
- **默认 demo 值**：`contracthub/contracthub123`（仅演示）。**生产必须覆盖**：

```bash
VOUCHCHAIN_DB_PASSWORD=强密码 APP_AES_KEY=$(openssl rand -hex 16) \
docker compose up -d --build
```

### 生产部署必做清单

默认配置只适合**本机演示**。对外提供服务前，以下 4 项**全部**要做完：

1. **关闭演示鉴权**：设环境变量 `APP_DEV_MODE=false`（或 `config/config.yaml` 的 `app.devMode: false`）。否则任意账号可登录、`X-User-Role` 可自选管理员角色。
2. **配置 SSO 验签**：在 `config/config.yaml` 填 `CASDOOR_CERTIFICATE`（或 `CASDOOR_CERT_FILE`）。SSO 签名纯本地校验、不联网。
3. **替换所有演示凭据**：`VOUCHCHAIN_DB_PASSWORD`（数据库）、`APP_AES_KEY`（用 `openssl rand -hex 16` 生成）、Casdoor `clientSecret`。示例值 `contracthub/contracthub123` 仅用于演示。
4. **限制访问面**：8000 端口只绑定 `127.0.0.1` 或内网；公网必须经反向代理 + HTTPS，且 `/scalar`、`/openapi.json` 一并做访问控制（这两条路由免登录）。

改完执行 `docker compose up -d --build` 重启生效。

- **升级**：`git pull && docker compose up -d --build`（启动期自动跑 `migrations/*.sql` 幂等迁移，无需手工 SQL）。

## 目录结构

```
├── main.go                 # 入口：~300 API 路由 + 调度器 + MCP + WS
├── internal/
│   ├── controller/         # HTTP 控制器（每业务域一个文件）
│   ├── middleware/         # RBAC / 数据隔离 / SCIM 认证
│   └── mcpserver/          # MCP 服务（外部 AI 客户端接入）
├── pkg/
│   ├── agent/              # Eino LLM Agent（LLM 配置解析 / 单轮生成）
│   ├── daily/              # 每日工作台纯规则聚合层（零 AI 依赖）
│   ├── duban/              # 公文督办状态机
│   ├── revrec/             # 收入确认规则
│   ├── ws/                 # WebSocket hub
│   └── kb/                 # 法律条款知识库
├── web/                    # Vue3 + Element Plus
├── mobile/                 # uni-app（H5 / 小程序 / App）
└── config/config.yaml.example
```

## 加入社区交流群

扫描下方二维码加入 QQ 群，或添加作者微信拉你进微信群：

<p align="center">
  <img src="assets/community/qrcode-qqgroup.jpg" width="190" alt="QQ 群二维码" title="QQ 群：VouchChain 合同与履约中枢 · 开源社区">
  <img src="assets/community/qrcode-wechat-personal.jpg" width="190" alt="个人微信二维码" title="微信：添加作者，拉你进 VouchChain 社区版交流群">
</p>

- **QQ 群**：扫码加入「VouchChain 合同与履约中枢 · 开源社区」——使用交流、部署答疑、Bug 反馈、需求建议
- **微信**：扫码添加作者微信 → 拉你进「VouchChain 社区版交流群」（请备注 VouchChain）

## 获取专业版

专业版为**企业许可**（非 Apache 2.0），含 AI 深度条款审查 / 多主体合并报表桥接 / 等保自动测评报告。早期支持者可申请 **90 天评估授权**（≤5 用户、内部自用）；生产商用 / 转售须签订商业协议。

## 许可

社区版 [Apache License 2.0](LICENSE) — © 2026 VouchChain Authors。
依赖遵循各自许可（GoFrame MIT、Eino Apache-2.0 等）；不内嵌 / 链接 AGPL / GPL 组件（仅可经干净 API 以外部服务调用）。
