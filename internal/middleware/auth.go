package middleware

import (
	"fmt"
	"strings"
	"time"

	"github.com/gogf/gf/v2/net/ghttp"
	"github.com/golang-jwt/jwt/v4"
)

// 统一鉴权中间件（SSO 认证 → RBAC 授权的前置）。
//
// 关键事实：Casdoor 应用签发的是标准 JWT，校验只依赖应用证书里的公钥（RS256/ES256），
// **纯本地验签、不联网**。所以「接 Casdoor」= 配好应用证书即可，本机不需要再跑一个
// Casdoor 服务；只有「用户在 Casdoor 页面登录、用 code 换 token」才需要可访问的实例
// （那一半由 controller.AuthCallback 走 casdoor-go-sdk 的 GetOAuthToken 完成）。
//
// 这里没有直接用 SDK 的 ParseJwtToken：它的 Claims 把 roles 定成 []*Role 强类型，
// 遇到 JWT-Standard / 自定义 claims 里 ["legal"] 这种字符串数组会整体解析失败。
// 改成 MapClaims 验签后，两种格式都能归一化成角色名，且与 Casdoor 版本解耦。
//
//   - app.devMode=true（本地演示默认）：旁路放行，注入 dev 用户，X-User-Role 模拟角色
//   - app.devMode=false（生产/自托管）：校验 Authorization: Bearer <casdoor-jwt>，
//     用户名、角色完全取自验签后的 claims，伪造 X-User-Name / X-User-Role 一律不认
//
// 生产切换只需环境变量：APP_DEV_MODE=false + CASDOOR_CERTIFICATE（或 CASDOOR_CERT_FILE），
// 无需修改源码或数据库。
func Auth() ghttp.HandlerFunc {
	return func(r *ghttp.Request) {
		cfg := LoadSsoConfig(r.Context())
		if IsPublicPath(r.URL.Path) {
			r.SetCtxVar("authMode", modeOf(cfg))
			r.Middleware.Next()
			return
		}
		if cfg.DevMode {
			user := r.Header.Get("X-User-Name")
			if user == "" {
				user = "dev"
			}
			if IsScimUserDisabled(user) {
				r.Response.WriteStatusExit(401, "user disabled by SCIM")
				return
			}
			r.SetCtxVar("user", user)
			r.SetCtxVar("authMode", "dev")
			r.Middleware.Next()
			return
		}

		token := strings.TrimSpace(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))
		if token == "" && strings.HasPrefix(r.URL.Path, "/api/ws/") {
			// 浏览器 WebSocket 不能自定义请求头，允许经 query 传 token
			token = r.URL.Query().Get("token")
		}
		if token == "" || token == "dev-token" {
			r.Response.WriteStatusExit(401, "missing bearer token")
			return
		}
		if cfg.Certificate == "" {
			r.Response.WriteStatusExit(500, "casdoor certificate not configured (CASDOOR_CERTIFICATE / CASDOOR_CERT_FILE / app.casdoor.certificate)")
			return
		}
		id, err := VerifyToken(cfg, token)
		if err != nil {
			r.Response.WriteStatusExit(401, "invalid or expired token: "+err.Error())
			return
		}
		if id.TokenType == "refresh-token" {
			r.Response.WriteStatusExit(401, "refresh token is not an access token")
			return
		}
		if id.Name == "" || id.Forbidden || id.Deleted {
			r.Response.WriteStatusExit(401, "user disabled or invalid")
			return
		}
		// SCIM 推送的离职/禁用优先于 JWT：即使 token 还没过期也立刻失效
		if IsScimUserDisabled(id.Name) {
			r.Response.WriteStatusExit(401, "user disabled by SCIM")
			return
		}
		if cfg.Audience != "" && !containsString(id.Audience, cfg.Audience) {
			r.Response.WriteStatusExit(401, "token audience mismatch")
			return
		}
		if cfg.Issuer != "" && id.Issuer != cfg.Issuer {
			r.Response.WriteStatusExit(401, "token issuer mismatch")
			return
		}

		r.SetCtxVar("user", id.Name)
		r.SetCtxVar("roles", id.Roles)
		r.SetCtxVar("authMode", "sso")
		r.SetCtxVar("displayName", firstNonEmpty(id.DisplayName, id.Name))
		r.SetCtxVar("email", id.Email)
		r.SetCtxVar("avatar", id.Avatar)
		r.SetCtxVar("owner", id.Owner)
		r.SetCtxVar("scopes", id.Scope)
		r.Middleware.Next()
	}
}

// SsoIdentity 从验签后的 claims 归一化出来的身份
type SsoIdentity struct {
	Name, DisplayName, Email, Avatar, Owner string
	Roles                                   []string
	TokenType, Scope, Issuer                string
	Audience                                []string
	Forbidden, Deleted                      bool
	ExpiresAt                               time.Time
}

func publicKeyForToken(token *jwt.Token, cert string) (any, error) {
	switch token.Method.Alg() {
	case jwt.SigningMethodES256.Alg(), jwt.SigningMethodES512.Alg():
		return jwt.ParseECPublicKeyFromPEM([]byte(cert))
	case jwt.SigningMethodRS256.Alg(), jwt.SigningMethodRS512.Alg():
		return jwt.ParseRSAPublicKeyFromPEM([]byte(cert))
	default:
		return nil, fmt.Errorf("unsupported signing method: %v", token.Header["alg"])
	}
}

// verifyToken 用 Casdoor 应用证书本地验签（exp/nbf 由 jwt 库校验），不发起网络请求
func VerifyToken(cfg SsoConfig, token string) (*SsoIdentity, error) {
	claims := jwt.MapClaims{}
	parsed, err := jwt.ParseWithClaims(token, claims, func(t *jwt.Token) (any, error) {
		return publicKeyForToken(t, cfg.Certificate)
	})
	if err != nil || parsed == nil || !parsed.Valid {
		if err == nil {
			err = fmt.Errorf("token not valid")
		}
		return nil, err
	}
	id := &SsoIdentity{
		Name:        claimString(claims, "name"),
		DisplayName: claimString(claims, "displayName"),
		Email:       claimString(claims, "email"),
		Avatar:      claimString(claims, "avatar"),
		Owner:       claimString(claims, "owner"),
		TokenType:   firstNonEmpty(claimString(claims, "tokenType"), claimString(claims, "TokenType")),
		Scope:       claimString(claims, "scope"),
		Issuer:      claimString(claims, "iss"),
		Roles:       normalizeRoleClaim(claims["roles"]),
		Audience:    normalizeStringClaim(claims["aud"]),
		Forbidden:   claimBool(claims, "isForbidden"),
		Deleted:     claimBool(claims, "isDeleted"),
	}
	if exp, ok := claims["exp"].(float64); ok {
		id.ExpiresAt = time.Unix(int64(exp), 0)
	}
	if id.Name == "" {
		id.Name = claimString(claims, "sub") // JWT-Standard 兜底
	}
	return id, nil
}

func claimString(c jwt.MapClaims, key string) string {
	if v, ok := c[key].(string); ok {
		return v
	}
	return ""
}

func claimBool(c jwt.MapClaims, key string) bool {
	if v, ok := c[key].(bool); ok {
		return v
	}
	return false
}

func normalizeStringClaim(v any) []string {
	out := []string{}
	switch x := v.(type) {
	case string:
		out = append(out, x)
	case []any:
		for _, item := range x {
			if s, ok := item.(string); ok {
				out = append(out, s)
			}
		}
	case []string:
		out = append(out, x...)
	}
	return out
}

// normalizeRoleClaim 兼容 ["legal"] / [{"name":"legal"}] / "legal" 三种 claims 形态
func normalizeRoleClaim(v any) []string {
	out := []string{}
	appendName := func(s string) {
		s = strings.TrimSpace(s)
		if s != "" && !containsString(out, s) {
			out = append(out, s)
		}
	}
	switch list := v.(type) {
	case string:
		appendName(list)
	case []string:
		for _, s := range list {
			appendName(s)
		}
	case []any:
		for _, item := range list {
			switch x := item.(type) {
			case string:
				appendName(x)
			case map[string]any:
				if n, ok := x["name"].(string); ok {
					appendName(n)
				} else if n, ok := x["displayName"].(string); ok {
					appendName(n)
				}
			}
		}
	}
	return out
}

func modeOf(cfg SsoConfig) string {
	if cfg.DevMode {
		return "dev"
	}
	return "sso"
}

func containsString(list []string, want string) bool {
	for _, v := range list {
		if v == want {
			return true
		}
	}
	return false
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

// ClaimRoles 生产态从验签后的 JWT claims 取角色；dev 态为空（由请求头模拟）。
func ClaimRoles(r *ghttp.Request) []string {
	v := r.GetCtxVar("roles")
	if v.IsEmpty() {
		return nil
	}
	switch roles := v.Val().(type) {
	case []string:
		return roles
	case []interface{}:
		out := make([]string, 0, len(roles))
		for _, x := range roles {
			if s, ok := x.(string); ok {
				out = append(out, s)
			}
		}
		return out
	}
	return nil
}

// IsDevMode 当前请求是否处于开发旁路态（供控制器展示/兼容判断）
func IsDevMode(r *ghttp.Request) bool {
	return r.GetCtxVar("authMode").String() != "sso"
}
