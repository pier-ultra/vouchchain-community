package middleware

import (
	"context"
	"os"
	"strings"

	"github.com/gogf/gf/v2/frame/g"
)

// SsoConfig 认证配置。环境变量优先于 config.yaml，便于同一份构建产物
// 在自托管部署时只换环境变量、不改文件（证书公钥不是机密，但仍不写死在代码里）。
type SsoConfig struct {
	DevMode      bool
	Endpoint     string
	ClientId     string
	ClientSecret string
	Certificate  string // PEM 公钥 / x509 证书内容，Casdoor 应用 Cert 页复制
	OrgName      string
	AppName      string
	Audience     string // 可选：期望的 aud（一般填 clientId），防止别的应用签发的 token 混用
	Issuer       string // 可选：期望的 iss（一般填 Casdoor endpoint）
}

func envOrConfig(ctx context.Context, envKey, cfgKey, def string) string {
	if v := strings.TrimSpace(os.Getenv(envKey)); v != "" {
		return v
	}
	return g.Cfg().MustGet(ctx, cfgKey, def).String()
}

func LoadSsoConfig(ctx context.Context) SsoConfig {
	dev := g.Cfg().MustGet(ctx, "app.devMode", true).Bool()
	if v := strings.TrimSpace(os.Getenv("APP_DEV_MODE")); v != "" {
		dev = v == "1" || strings.EqualFold(v, "true") || strings.EqualFold(v, "yes")
	}
	cert := os.Getenv("CASDOOR_CERTIFICATE")
	if cert == "" {
		if f := os.Getenv("CASDOOR_CERT_FILE"); f != "" {
			if b, err := os.ReadFile(f); err == nil {
				cert = string(b)
			}
		}
	}
	if cert == "" {
		cert = g.Cfg().MustGet(ctx, "app.casdoor.certificate", "").String()
	}
	return SsoConfig{
		DevMode:      dev,
		Endpoint:     strings.TrimRight(envOrConfig(ctx, "CASDOOR_ENDPOINT", "app.casdoor.endpoint", ""), "/"),
		ClientId:     envOrConfig(ctx, "CASDOOR_CLIENT_ID", "app.casdoor.clientId", ""),
		ClientSecret: envOrConfig(ctx, "CASDOOR_CLIENT_SECRET", "app.casdoor.clientSecret", ""),
		Certificate:  cert,
		OrgName:      envOrConfig(ctx, "CASDOOR_ORG_NAME", "app.casdoor.orgName", ""),
		AppName:      envOrConfig(ctx, "CASDOOR_APP_NAME", "app.casdoor.appName", ""),
		Audience:     envOrConfig(ctx, "CASDOOR_AUDIENCE", "app.casdoor.audience", ""),
		Issuer:       envOrConfig(ctx, "CASDOOR_ISSUER", "app.casdoor.issuer", ""),
	}
}

// IsPublicPath 无需 Bearer token 的端点：登录页配置、OAuth 回调、健康探针。
// 静态资源（非 /api 前缀）也必须放行，否则生产态连登录页都打不开。
func IsPublicPath(path string) bool {
	if !strings.HasPrefix(path, "/api/") {
		return true
	}
	switch path {
	case "/api/auth/config", "/api/auth/callback", "/api/health":
		return true
	}
	return false
}
