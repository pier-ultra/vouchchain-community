# VouchChain · Contract & Performance Hub (Community Edition)

**VouchChain** is a **Contract & Performance Hub** for SMB finance/compliance teams: it converges business-side flows — contracts, invoices, seals, MLPS 2.0 (等保), revenue recognition, official-task follow-ups — into an auditable bridge of "business side → accounting-confirmed vouchers → existing customer ledger".

> Version: **v0.3.1 Community Edition** (Apache 2.0, matches the latest tag; see [CHANGELOG.md](CHANGELOG.md) for the full history). A **Professional Edition** (commercial license) exists with 3 advanced features; see the comparison below.
>
> Positioning: **pure open-source Community Edition** — no open-core layering, no license key, no feature-flag paywall. AI deep clause review / multi-entity consolidation bridge / automated MLPS report are Professional; everything else, including the governance base, is free in Community.
>
> Red lines: no GL core / financial statements / tax filing — only the bridge from business side to accounting vouchers; AI only suggests, humans must review before posting; fail loudly (not silently) when external integrations are unconfigured.

> ⚠️ **Security warning: do not expose the default configuration to the public internet**
>
> The sample config and `docker compose` default to `devMode=true`, which is a **local demo mode**: any credentials log in, any role can be selected on the login page, the `X-User-Role` header can impersonate an administrator, and `/scalar` API docs are publicly accessible. This is designed for "one-command preview" — **it is not a security configuration**.
>
> Before deploying anywhere beyond your own machine (public server, cloud VM, shared intranet), complete the [Production deployment checklist](#production-deployment-checklist) below: disable devMode, configure the Casdoor verification certificate, replace demo credentials and keys, and restrict port exposure. **Copy-pasting the demo commands onto a public host exposes all business data.**

## Repositories

Open source & free · **Apache 2.0**. Stars / forks / issues are welcome:

- **GitHub**: [github.com/pier-ultra/vouchchain-community](https://github.com/pier-ultra/vouchchain-community)
- **Gitee**: [gitee.com/pier-ultra/vouchchain-community](https://gitee.com/pier-ultra/vouchchain-community)

> If GitHub is slow/unreachable from your region, use **Gitee** or a `ghproxy` mirror.

## Community vs Professional

| Feature | Community (free · Apache 2.0) | Professional (commercial) |
|---|---|---|
| Contract ledger / lifecycle (draft·approve·sign·perform·archive) | ✅ | ✅ |
| Invoice center (e-invoice / OFD / XML import, tax verification, red-flush alerts) | ✅ | ✅ |
| Seal management (apply·approve·stamp log·e-seal backfill·seal handover) | ✅ | ✅ |
| MLPS 2.0 retention & evidence chain (GB/T 22239-2019, retain ≥180 days) | ✅ | ✅ |
| Revenue recognition (performance obligations·recognition mode·settlement·AR-CR diff) | ✅ | ✅ |
| Official-task follow-up (register·remind·complete·auto overdue detection) | ✅ | ✅ |
| Daily workbench (Agent aggregation·morning/evening scheduled summaries) | ✅ | ✅ |
| Governance base (RBAC / SCIM v2 / data isolation / audit log / MCP / Casdoor SSO) | ✅ | ✅ |
| Mobile app (uni-app source + H5 / WeChat mini-program builds) | ✅ | ✅ |
| **AI deep clause review (incl. version diff)** | — | ✅ |
| **Multi-entity consolidation bridge (ledger bridge)** | — | ✅ |
| **Automated MLPS self-assessment report** | — | ✅ |
| Android / iOS native packaging (cert·sign·store) | Value-add service | Value-add service |

## Core Capabilities (Community)

| Module | Description |
|---|---|
| **Contract ledger** | Full lifecycle: draft / approve / sign / perform / amend / archive |
| **Invoice center** | E-invoice / OFD / XML import, tax verification, red-flush alerts |
| **Seal management** | Apply, approve, stamp log, e-seal backfill, seal handover |
| **MLPS 2.0 audit** | GB/T 22239-2019; retention ≥180 days (rejected at code level below the floor), archive not delete |
| **Revenue recognition** | Performance obligations / recognition mode (point / over time) / settlement / AR-CR diff |
| **Official-task follow-up** | Task register, remind, complete, audit trail, auto overdue detection |
| **Daily workbench** | Proactive Agent: aggregate this week's focus / in-progress tracking / next-step suggestion; morning & evening scheduled push (WS broadcast + AI polish, auto-degrades to pure rules without a model) |
| **Governance base** | RBAC / SCIM v2 full CRUD / data isolation / audit log / MCP service / Casdoor SSO |

## UI Preview (Community)

Web console (1440×900):

<p align="center">
  <img src="screenshots/web/01_login.png" width="45%" alt="Login">
  <img src="screenshots/web/02_dashboard.png" width="45%" alt="Overview">
  <img src="screenshots/web/03_contracts.png" width="45%" alt="Contract ledger">
  <img src="screenshots/web/04_invoices.png" width="45%" alt="Invoice center">
  <img src="screenshots/web/05_seals.png" width="45%" alt="Seal management">
  <img src="screenshots/web/06_revrec.png" width="45%" alt="Revenue recognition">
  <img src="screenshots/web/07_dengbao.png" width="45%" alt="MLPS evidence">
  <img src="screenshots/web/08_duban.png" width="45%" alt="Official-task follow-up">
  <img src="screenshots/web/09_chat.png" width="45%" alt="AI assistant">
  <img src="screenshots/web/10_skills.png" width="45%" alt="AI skills">
</p>

Mobile app (uni-app H5 preview, 390×844):

<p align="center">
  <img src="screenshots/app/01_login.png" width="22%" alt="Login">
  <img src="screenshots/app/02_home.png" width="22%" alt="Home">
  <img src="screenshots/app/03_contracts.png" width="22%" alt="Contracts">
  <img src="screenshots/app/04_approvals.png" width="22%" alt="Approvals">
  <img src="screenshots/app/05_invoices.png" width="22%" alt="Invoices">
  <img src="screenshots/app/06_seals.png" width="22%" alt="Seals">
  <img src="screenshots/app/07_dengbao.png" width="22%" alt="MLPS evidence">
  <img src="screenshots/app/08_revrec.png" width="22%" alt="Revenue recognition">
  <img src="screenshots/app/09_duban.png" width="22%" alt="Task follow-up">
  <img src="screenshots/app/10_mine.png" width="22%" alt="Mine">
</p>

> Screenshots are from the Community build (demo data seeded); the Professional-only entries (Clause Review / Bridge / Self-check) are not in the Community menu.

## Tech Stack

- **Backend**: GoFrame v2.10.4 (single binary, cross-platform)
- **Web**: Vue 3 + Element Plus + ECharts (light / dark themes)
- **Mobile**: uni-app (Vue 3), H5 / WeChat mini-program / App
- **AI**: Eino LLM Agent (knowledge-base Q&A / brief polish)
- **Realtime**: WebSocket broadcast (`daily_brief` etc.)
- **Database**: SQLite (default, zero-config single file, pure-Go driver, no gcc) / PostgreSQL (multi-user / production, optional pgvector) / MySQL 8 and compatible engines (TiDB, OceanBase) / openGauss 5 (Huawei, PG kernel, auto-detected via the pgsql driver) / DM8 (Oracle-family, GoFrame dm driver) / KingBase (Oracle-compatible mode, thin pgsql driver wrapper). The three domestic engines have no pgvector, so KB falls back to keyword search; all are on `main`, not yet tagged.

## Quick Start

```bash
# 1. Backend
go build -o vouchchain ./
cp config/config.yaml.example config/config.yaml   # fill in DB / keys for your env
./vouchchain                                       # default :8000

# 2. Web (dev)
cd web && npm install && npm run dev

# 3. Mobile (H5 preview)
cd mobile && npm install && npm run dev:h5
```

A `web/dist` (or `mobile/dist`) must be present at runtime; see `.goreleaser.yaml` for production builds.

## API Documentation

After the backend starts, open **http://localhost:8000/scalar** (no login required). The page is rendered by [Scalar](https://github.com/scalar/scalar), fed by the self-generated `GET /openapi.json` (all routes, 224 paths).

Scalar ships as a **local static bundle** with no CDN dependency (works offline / on intranets). When you run `cd web && npm install && npm run build`, `scripts/copy-scalar.mjs` copies `node_modules/@scalar/api-reference/dist/browser/standalone.js` into `web/dist/vendor/scalar-standalone.js`, which the backend serves from `web/dist`.

## Docker Compose One-Click Start

The repo ships a `Dockerfile` + `docker-compose.yml` — **no Go/Node/PostgreSQL needed**, one command brings up the full stack (Web console + backend + built-in PG):

```bash
docker compose up -d --build      # first build takes a few minutes
# open http://localhost:8000 (dev demo mode: no login, pick a role on the login page)
```

Notes:

- **Multi-stage image**: the image builds `web/dist` (Node 22) and the Go backend (CGO_ENABLED=0 static binary) itself — the host only needs Docker.
- **Database**: built-in `postgres:16-alpine`, data in `pgdata` volume (`docker compose down` keeps data; `down -v` wipes it).
- **DSN injection**: the backend overrides the DB link via `VOUCHCHAIN_DB_LINK` (no config file change; DB password never enters the repo or image).
- **Default demo values**: `contracthub/contracthub123` (demo only). **Override in production**:

```bash
VOUCHCHAIN_DB_PASSWORD=strong-password APP_AES_KEY=$(openssl rand -hex 16) \
docker compose up -d --build
```

### Production deployment checklist

The default configuration is for **local demo only**. Complete **all four** items before exposing the service:

1. **Disable demo auth**: set `APP_DEV_MODE=false` (or `app.devMode: false` in `config/config.yaml`). Otherwise any credentials log in and `X-User-Role` can impersonate an administrator.
2. **Configure SSO verification**: fill `CASDOOR_CERTIFICATE` (or `CASDOOR_CERT_FILE`) in `config/config.yaml`. Signatures are verified locally, with no network call.
3. **Replace every demo credential**: `VOUCHCHAIN_DB_PASSWORD` (database), `APP_AES_KEY` (generate with `openssl rand -hex 16`), and the Casdoor `clientSecret`. The sample `contracthub/contracthub123` is demo-only.
4. **Restrict exposure**: bind port 8000 to `127.0.0.1` or an intranet only; on the public internet use a reverse proxy + HTTPS, and apply access control to `/scalar` and `/openapi.json` too (both routes are unauthenticated).

Then run `docker compose up -d --build` to apply the changes.

- **Upgrade**: `git pull && docker compose up -d --build` (idempotent `migrations/*.sql` run automatically on startup; no manual SQL).

## Directory Structure

```
├── main.go                 # entry: ~300 API routes + scheduler + MCP + WS
├── internal/
│   ├── controller/         # HTTP controllers (one file per business domain)
│   ├── middleware/         # RBAC / data isolation / SCIM auth
│   └── mcpserver/          # MCP server (external AI clients)
├── pkg/
│   ├── agent/              # Eino LLM Agent (config parse / single-turn generation)
│   ├── daily/              # daily workbench pure-rule aggregation (zero AI dep)
│   ├── duban/              # official-task follow-up state machine
│   ├── revrec/             # revenue recognition rules
│   ├── ws/                 # WebSocket hub
│   └── kb/                 # legal clause knowledge base
├── web/                    # Vue3 + Element Plus
├── mobile/                 # uni-app (H5 / mini-program / App)
└── config/config.yaml.example
```

## Community

Scan the QR codes to join the QQ group, or add the author on WeChat to be invited to the WeChat group:

<p align="center">
  <img src="assets/community/qrcode-qqgroup.jpg" width="190" alt="QQ group QR" title="QQ Group: VouchChain Contract & Performance Hub · Open Source Community">
  <img src="assets/community/qrcode-wechat-personal.jpg" width="190" alt="WeChat QR" title="WeChat: add the author to join the VouchChain Community group">
</p>

- **QQ group**: scan to join "VouchChain Contract & Performance Hub · Open Source Community" — usage, deployment Q&A, bug reports, feature requests
- **WeChat**: scan to add the author → join the "VouchChain Community Edition group" (note "VouchChain")

## Professional Edition

Professional is a **commercial license** (not Apache 2.0) and includes AI deep clause review / multi-entity consolidation bridge / automated MLPS report. Early supporters may apply for a **90-day evaluation license** (≤5 users, internal use); production/commercial use or resale requires a commercial agreement.

## License

Community Edition is [Apache License 2.0](LICENSE) — © 2026 VouchChain Authors.
Dependencies follow their own licenses (GoFrame MIT, Eino Apache-2.0, etc.); no AGPL/GPL components are embedded or linked (external service calls over clean APIs only).
