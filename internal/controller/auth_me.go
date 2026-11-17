package controller

import (
	"encoding/json"
	"fmt"
	"time"

	"vouchchain/internal/middleware"
	"net/url"
	"strings"

	"github.com/casdoor/casdoor-go-sdk/casdoorsdk"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/net/ghttp"
)

// ---------- SSO 会话信息：前端菜单、角色、按钮权限的唯一来源 ----------

var builtinRoleNames = map[string]string{
	"admin": "管理员", "legal": "法务", "business": "业务", "viewer": "只读",
}

// Me 当前登录者信息 + 有效角色 + 可见菜单/动作。
// 生产态这些信息全部来自验签后的 Casdoor JWT claims 与服务端 RBAC 配置，前端不参与裁决。
func Me(r *ghttp.Request) {
	role := middleware.CurrentRole(r)
	// Casdoor 真实用户 JIT 身份同步：生产态（SSO）每次 /api/me 刷新 scim_user，
	// 身份总表统一 SSO 与 SCIM 两侧用户视图（组织同步的落点）
	if !middleware.IsDevMode(r) {
		jitUpsertSsoIdentity(r, role)
	}
	def, _ := middleware.RoleDefFor(role)
	name := def.Name
	if name == "" {
		name = builtinRoleNames[role]
	}
	if name == "" {
		name = role
	}
	username := r.GetCtxVar("user").String()
	display := r.GetCtxVar("displayName").String()
	if display == "" {
		display = username
	}
	if display == "" {
		display = "dev"
	}
	r.Response.WriteJsonExit(g.Map{"code": 0, "data": g.Map{
		"username":    username,
		"displayName": display,
		"email":       r.GetCtxVar("email").String(),
		"avatar":      r.GetCtxVar("avatar").String(),
		"owner":       r.GetCtxVar("owner").String(),
		"authMode":    r.GetCtxVar("authMode").String(),
		"devMode":     middleware.IsDevMode(r),
		"claimRoles":  middleware.ClaimRoles(r),
		"role":        role,
		"roleName":    name,
		"dataScope":   def.DataScope,
		"fieldMask":   def.FieldMask,
		"menus":       middleware.VisibleMenus(role),
		"actions":     middleware.VisibleActions(role),
	}})
}

// AuthConfig 登录页用：dev 演示态还是 SSO 态、OAuth 跳转地址、证书是否就绪。
// 该端点公开（未登录也要能取到），但不返回任何密钥或证书内容。
func AuthConfig(r *ghttp.Request) {
	cfg := middleware.LoadSsoConfig(r.Context())
	origin := r.Header.Get("Origin")
	if origin == "" {
		scheme := "http"
		if r.Header.Get("X-Forwarded-Proto") != "" {
			scheme = r.Header.Get("X-Forwarded-Proto")
		}
		origin = scheme + "://" + r.Host
	}
	redirectUri := strings.TrimRight(origin, "/") + "/#/login"
	authorizeBase := ""
	if cfg.Endpoint != "" && cfg.ClientId != "" {
		q := url.Values{}
		q.Set("client_id", cfg.ClientId)
		q.Set("response_type", "code")
		q.Set("redirect_uri", redirectUri)
		q.Set("scope", "read")
		authorizeBase = cfg.Endpoint + "/login/oauth/authorize?" + q.Encode() + "&state="
	}
	r.Response.WriteJsonExit(g.Map{"code": 0, "data": g.Map{
		"devMode":        cfg.DevMode,
		"ssoEnabled":     !cfg.DevMode,
		"endpoint":       cfg.Endpoint,
		"clientId":       cfg.ClientId,
		"orgName":        cfg.OrgName,
		"appName":        cfg.AppName,
		"certConfigured": strings.TrimSpace(cfg.Certificate) != "",
		"redirectUri":    redirectUri,
		"authorizeBase":  authorizeBase,
	}})
}

// AuthCallback OAuth 授权码换 token。只有部署了可访问的 Casdoor 实例时才会走这里；
// 本地/离线自托管可直接把 Casdoor 应用签发的 access token 贴进登录页（走 /api/me 验签）。
func AuthCallback(r *ghttp.Request) {
	var req struct {
		Code  string `json:"code"`
		State string `json:"state"`
	}
	if err := r.Parse(&req); err != nil || strings.TrimSpace(req.Code) == "" {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": "缺少授权码 code"})
		return
	}
	cfg := middleware.LoadSsoConfig(r.Context())
	if cfg.Endpoint == "" || cfg.ClientId == "" || cfg.ClientSecret == "" || cfg.Certificate == "" {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": "未配置 Casdoor 实例（endpoint/clientId/clientSecret/certificate），请改用粘贴 Access Token 登录"})
		return
	}
	client := casdoorsdk.NewClient(cfg.Endpoint, cfg.ClientId, cfg.ClientSecret, cfg.Certificate, cfg.OrgName, cfg.AppName)
	tok, err := client.GetOAuthToken(req.Code, req.State)
	if err != nil || tok == nil || tok.AccessToken == "" {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": "换取 token 失败：" + errString(err)})
		return
	}
	id, err := middleware.VerifyToken(cfg, tok.AccessToken)
	if err != nil || id == nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": "Casdoor 返回的 token 验签失败"})
		return
	}
	r.Response.WriteJsonExit(g.Map{"code": 0, "data": g.Map{
		"token": tok.AccessToken, "username": id.Name,
		"displayName": firstNonEmptyStr(id.DisplayName, id.Name),
		"roles":       id.Roles,
	}})
}

func errString(err error) string {
	if err == nil {
		return "未知错误"
	}
	return err.Error()
}

func firstNonEmptyStr(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

// jitUpsertSsoIdentity SSO 登录身份 JIT 同步进 scim_user（external_id=casdoor/{owner}/{name}）。
// 同一用户 SCIM 推送与 SSO 登录共用一条身份记录（SCIM 侧 externalId 不同则以用户名 + 来源归并）。
func jitUpsertSsoIdentity(r *ghttp.Request, role string) {
	username := r.GetCtxVar("user").String()
	if username == "" {
		return
	}
	rolesJson, _ := json.Marshal(middleware.ClaimRoles(r))
	owner := r.GetCtxVar("owner").String()
	externalId := "casdoor/" + owner + "/" + username
	exist, err := g.DB().Model("scim_user").Where("external_id", externalId).One()
	if err != nil {
		return
	}
	displayName := r.GetCtxVar("displayName").String()
	email := r.GetCtxVar("email").String()
	if exist.IsEmpty() {
		g.DB().Model("scim_user").Data(g.Map{
			"external_id": externalId, "username": username,
			"display_name": displayName, "email": email,
			"roles": string(rolesJson), "active": true,
			"raw":        fmt.Sprintf(`{"source":"sso-jit","owner":%q,"role":%q}`, owner, role),
			"created_at": time.Now(),
		}).Insert()
		return
	}
	// 已有 SCIM 推送的记录：保留其部门等字段，仅刷新 SSO 侧动态信息
	g.DB().Model("scim_user").Data(g.Map{
		"display_name": displayName, "email": email, "roles": string(rolesJson), "active": true,
	}).Where("id", exist["id"].Int64()).Update()
}
