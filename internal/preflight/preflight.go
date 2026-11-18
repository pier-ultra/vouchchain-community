package preflight

import (
	"fmt"
	"os"
)

// defaultConfigYAML 是首次运行自动生成的默认配置。
// 零配置双击即跑：SQLite 单文件库（自动建库迁移）、devMode 免鉴权体验、监听 :8000。
// 与 config/config.yaml.example 保持一致的最小可用子集；需要自定义时直接改生成的 config.yml。
const defaultConfigYAML = `# VouchChain 社区版默认配置 —— 由程序首次运行自动生成，可自行修改后重启生效。
# 删除本文件后重启可恢复默认（会重新生成）。
server:
  address: ":8000"
  indexFiles: ["index.html"]
  openapi:
    path: "/swagger"

database:
  default:
    # 默认 SQLite（零依赖零配置，适合单机自托管 / 快速体验）
    type: "sqlite"
    link: "sqlite::@file(./data/vouchchain.db)"

app:
  # dev 态：跳过验签、注入 dev 用户，双击即可体验。
  # 生产/交付：设置环境变量 APP_DEV_MODE=false 切 SSO 态（需 CASDOOR_CERTIFICATE 或 CASDOOR_CERT_FILE）。
  devMode: true
  casdoor:
    endpoint: ""
    clientId: ""
    clientSecret: ""
    certificate: ""
    orgName: ""
    appName: ""
    audience: ""
    issuer: ""
  scim:
    token: ""
  eino:
    baseUrl: "https://api.agnes-ai.cn/v1"
    model: "agnes-3.0-flash"

cache:
  driver: "memory"

redis:
  default:
    address: "127.0.0.1:6666"
    db: 0
`

// EnsureDefaultConfig 保证服务可用所需的配置文件与数据目录存在（Windows / Linux 行为一致）。
//
// 规则（与 GoFrame 默认搜索顺序兼容）：
//   - 当前目录已有 config.yml 或 config.yaml → 尊重用户配置，不覆盖；
//   - 都没有 → 生成一份默认 config.yml（SQLite 零配置，devMode 可跑）；
//   - 顺带确保 data/ 目录存在（SQLite 默认库路径 ./data/vouchchain.db，缺目录会导致打不开库）。
//
// 必须在任何 g.Cfg()/g.DB() 调用之前执行，否则 GoFrame 在无配置文件时会 panic。
// 注意：该函数在 pkg/cache 的包级 init() 里被调用（cache.New → g.Cfg().MustGet 是最早的配置读取点），
// 而 Go 的包级变量初始化先于 main()，所以不能只依赖 main() 里的调用。
func EnsureDefaultConfig() error {
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("get working dir: %w", err)
	}
	// 已存在任一常用配置文件（GoFrame 按 config.toml>yaml>yml>json>ini>xml>properties 顺序识别）：
	// 尊重现有配置，绝不覆盖。
	for _, name := range []string{"config.yml", "config.yaml"} {
		if _, err := os.Stat(name); err == nil {
			return nil
		}
	}
	// 都没有：生成默认 config.yml
	cfgPath := cwd + string(os.PathSeparator) + "config.yml"
	if err := os.WriteFile(cfgPath, []byte(defaultConfigYAML), 0o644); err != nil {
		return fmt.Errorf("write default config.yml: %w", err)
	}
	// SQLite 默认库路径 ./data/vouchchain.db：缺 data/ 目录时建库失败，预创建。
	dataDir := cwd + string(os.PathSeparator) + "data"
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return fmt.Errorf("create data dir: %w", err)
	}
	return nil
}
