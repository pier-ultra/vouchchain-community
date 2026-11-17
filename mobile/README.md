# contract-hub 移动端（uni-app）

与 `web/`（桌面管理台）**并列的第二套前端**，共用同一个后端 `/api`。
一套代码编译到 **H5 / 微信小程序 / 安卓 / iOS**（可扩展鸿蒙）。

## 定位

- 覆盖「**执行 / 审批 / 查看**」高频场景（合同、审批待办、发票、用印、等保证据、收入确认）。
- 治理配置类（RBAC、隔离、组织树、主数据、MDM、模型配置）**保留在 Web 管理台**，不上移动端。
- 逻辑可复用：`core/`（业务逻辑）、`services/`（请求/存储）、`stores/`（登录态）从 Web 端程序搬。
- UI 必须重写：Element Plus / `div` / `table` 在小程序与 App **不可用**，故页面用 uni-app 组件（`view`/`text`/`input`）重写。

## 目录结构

```
mobile/
├─ src/
│  ├─ core/        # 平台无关：roles(角色/权限判断)、format(金额/日期/状态)
│  ├─ services/    # http(uni.request 封装)、storage(uni storage)、api(接口清单)
│  ├─ stores/      # auth(Pinia 登录态)
│  ├─ pages/       # login / home / approvals / contracts / invoices / seals / dengbao / revrec / mine
│  │              # + contract-detail / approval-detail / invoice-detail（二级详情/操作页）
│  ├─ pages.json   # 路由 + tabBar（首页/审批/合同/我的）
│  ├─ manifest.json
│  ├─ App.vue  main.ts  uni.scss  env.d.ts
├─ index.html  package.json  tsconfig.json  vite.config.ts
```

## 命令

```bash
npm install                 # 安装依赖（见下方 C 盘注意事项）
npm run dev:h5              # H5 开发（默认 http://localhost:5273，/api 代理到 127.0.0.1:8000）
npm run build:h5            # 产出 dist/build/h5
npm run dev:mp-weixin       # 微信小程序开发
npm run build:mp-weixin     # 产出 dist/build/mp-weixin → 微信开发者工具导入
npm run build:app           # 产出 dist/build/app → HBuilderX 导入运行/打包
```

## 关键约定（与 Web 端一致）

- **请求**：`services/http.ts` 统一走同源 `/api`；请求头带 `Authorization: Bearer`、
  `X-User-Role`、`X-User-Name`、`X-Legal-Entity-Id`；**响应体即业务体**（`code/data/message`），
  是否成功由调用方判 `code`；401 自动清会话并跳登录。
- **登录态**：`stores/auth.ts` + `services/storage.ts`，storage key 为 `ch_token / ch_user / ch_role`；
  dev 态任意用户名可进，SSO 态为 Casdoor 签发的 JWT。
- **审批动作**：`approvalAct(id, actor, action, comment)`，`action` 取 `approve` / `reject`（驳回必须填意见）。

## 注意事项

1. **uni-app 全部子包版本必须一致**。当前锁定 `3.0.0-alpha-5030620261106001`
   （npm 镜像上并非每个 build 都同步齐全，`...1112001` 的 `uni-cli-shared` 缺失，故整体退到 `1106001`）。
2. **本机 C 盘紧张**。npm 缓存/临时目录请指到 E 盘（本环境无 G 盘）：
   ```bash
   TMPDIR=/e/wbbuild/tmp TEMP=/e/wbbuild/tmp npm install --cache /e/npmcache
   ```
3. **App 打包受 uni-app 许可约束**：只能用 DCloud 官方云打包或离线打包，不可用第三方打包平台。
4. **发票导入已接后端**：`/api/invoices/import`，multipart 字段 `file`，仅收 **OFD/XML 数电票**（不收照片）；
   重复（发票号）可弹窗确认后带 `force=1` 强制放行，字节幂等重复直接跳过。
   拍照→OCR→识别是**另一条链**（P1，PaddleOCR 验真），未接入 import 接口。

## 功能清单（docs §6 P1：6 大高频模块闭环）

| 模块 | 页面 | 已覆盖能力 |
|---|---|---|
| 合同 | contracts + contract-detail | 列表/搜索、详情、原文预览、版本历史（状态流转展示）|
| 审批 | approvals + approval-detail | 待办/全部、同意/驳回、节点时间线、流转记录、转交、加签 |
| 发票 | invoices + invoice-detail | 列表、OFD/XML 导入（重复可 force）、八字段明细、查验状态、四性检测、单套制归档、凭证链路 |
| 用印 | seals | 印章台账、用印申请提交、审批同意/驳回、盖章、风险提示、台账导出 |
| 等保 | dengbao | 留存状态、权限变更证据链、证据包(zip)/单块(xlsx)导出 |
| 收入确认 | revrec | 确认单列表、draft→confirmed 确认落账 |
| AI 助手 | chat（**底部 tab**）| 会话列表/切换/新建/重命名/删除、**SSE 流式回复**（逐字+工具步骤实时展示+可中断）、引导问题；非流式 `/chat` 作降级 |

> 治理配置（RBAC/组织/主数据/督办/费控/三重一大）按 docs §6.3 明确**不上移动端**，保留 Web 管理台。
> AI 助手按 docs「纯文本轻量可用」口径接入；语音/OCR/多模态直传属 P2-b 重投入，暂不做（见多模态调研 §5/§6）。

### AI 对话 SSE 实现要点

- `api.ts` 的 `chatStream()`：H5 走 `fetch`+ReadableStream，小程序/App 走 `uni.request` 的 `enableChunked`+`onChunkReceived`；
  共用 `makeSSEParser`（跨 chunk 缓冲、按空行切帧），解析 `start/tool/token/done/error`。返回取消函数支持"停止"。
- 后端 `/chat/stream` 曾有两个字段 bug 已修：`ChatStream` 新建会话写 `user`→`username`（否则会话查不到）、
  assistant 落库 `steps`→`tool_trace`（否则工具步骤丢失）；`pkg/agent.ChatTurnStream` 增加非流式兜底——
  国产兼容端点多不支持 SSE（`base.Stream` 建立失败），此时把已生成的非流式回复整段作为 token 推送，避免空回复。

## 本地联调（后端跑本地 + SSH 隧道连远程 PG）

本环境 instabox 端口不对外，采用「本地起后端 + SSH 隧道连远程 PG」：

```bash
# 1) 隧道：远程 pgvector:5432 → 本地 15432（保持常驻）
ssh -N -L 15432:127.0.0.1:5432 -p 2222 <user>@ssh.instabox.sh

# 2) 本地 config/config.yaml 的 database.link 指向 127.0.0.1:15432（已配好，devMode=true）

# 3) 本地起后端（Go 在 E:\Go\bin，需在 contract-hub 目录启动以找到 web/dist 与 migrations）
cd contract-hub && E:/Go/bin/go.exe build -o ch-bin.exe . && ./ch-bin.exe   # :8000

# 4) dev:h5：vite proxy /api → 127.0.0.1:8000，浏览器开 http://localhost:5273
```

- 后端启动期会跑 `RunMigrations`（经隧道较慢，约 10-20s），看到 `:8000` 监听后即就绪。
- 需 `web/dist/index.html` 存在（否则 `AddSearchPath` FATA 退出）；联调只用 /api，占位即可。
- 等保 evidence/blocks 首查走隧道约 10s，属隧道延迟非代码问题。

## 验证状态（2026-11-16 功能迭代后实测）

- ✅ 依赖安装成功；首轮因反复中断导致 `@babel/runtime-corejs3` 半成品（缺 `core-js-pure` 引用），
  用 `npm install --force --legacy-peer-deps` 补齐后恢复正常。**勿在中途 kill npm install**。
- ✅ 三端构建全部通过（Compiler 5.36 vue3）：
  - H5 → `dist/build/h5/`（EXIT 0）
  - 微信小程序 → `dist/build/mp-weixin/`（EXIT 0，含 `#ifdef MP-WEIXIN` 下载分支）
  - App → `dist/build/app/`（EXIT 0；打 apk/ipa 需 HBuilderX/云打包）
- ✅ 13 个 `.vue` 页面经 `@vue/compiler-sfc` 独立 parse + compileScript + compileTemplate 全部通过（13 OK / 0 FAIL）
- ✅ AI 对话（底部 tab，第 4 个）三端构建通过；会话列表实测返回真实会话
- ✅ **SSE 流式实测通过**：`POST /chat/stream` 返回 `start → token → done` 帧序列，token 含真实回复、done 的 reply 与 token 一致；流式新建会话（id=15）在 `/chat/sessions` 可查到（验证 username 字段修复）
- ✅ 后端 `pkg/agent.ChatTurnStream` 加非流式兜底：国产兼容端点不支持 SSE 时整段推送回复，避免空回复（实测生效）
- ✅ tabBar 现为 5 项（首页/审批/合同/AI助手/我的）；tab 页跳转统一用 `uni.switchTab`
- ✅ 后端本地启动 + SSH 隧道连远程 PG：`/api/health` = `{"status":"ok"}`，
  6 大模块接口（contracts/approvals/invoices/seals/seal-applies/revrec/dengbao）全部 HTTP 200
- ✅ 写接口链路实测：用印申请提交成功（id=3，status=pending，列表可见）
- ✅ 导出接口返回二进制：`/seal-applies/export`（xlsx 7KB）、`/dengbao/evidence/pack`（zip 76KB）
- ✅ 修复既有 bug：审批实例状态是 `running` 而非 `pending`，首页待办数/审批列表过滤已改 `status=running`（实测 4 条待办）
- ✅ 发票导入已接 `/api/invoices/import`（选 OFD/XML → multipart 上传 → 重复可 force）
- 说明：`uni-app` 未配 Appid 的 statistics 提示为正常告警，不影响构建。
