package middleware

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"os"
	"strconv"
	"strings"

	"vouchchain/pkg/scimcore"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/net/ghttp"
)

// ---------- SCIM 2.0 服务端独立鉴权 ----------
// /scim/v2/* 是给身份提供方（IdP）推送用的机器到机器接口，与用户登录用的
// Casdoor JWT 是两套凭据：这里只认部署时设置的 SCIM_TOKEN（环境变量优先）。
// 不落库、不写文件；未配置时端点一律 503，绝不无鉴权开放开户接口。

// ScimToken 返回当前进程配置的 SCIM Bearer token（空串=未配置）
func ScimToken() string {
	if v := strings.TrimSpace(os.Getenv("SCIM_TOKEN")); v != "" {
		return v
	}
	return strings.TrimSpace(g.Cfg().MustGet(context.Background(), "app.scim.token", "").String())
}

// ScimConfigured 供管理页显示接入就绪状态（不返回 token 明文）
func ScimConfigured() bool { return ScimToken() != "" }

func ScimAuth() ghttp.HandlerFunc {
	return func(r *ghttp.Request) {
		want := ScimToken()
		if want == "" {
			writeScimError(r, 503, "SCIM 未启用：请设置环境变量 SCIM_TOKEN（或 config app.scim.token）后重启服务", "serviceUnavailable")
			return
		}
		auth := r.Header.Get("Authorization")
		got := ""
		if len(auth) > 7 && strings.EqualFold(auth[:7], "Bearer ") {
			got = strings.TrimSpace(auth[7:])
		}
		if got == "" || subtle.ConstantTimeCompare([]byte(got), []byte(want)) != 1 {
			writeScimError(r, 401, "无效的 SCIM Bearer token", "invalidToken")
			return
		}
		r.SetCtxVar("scimActor", "scim-client")
		r.Middleware.Next()
	}
}

func writeScimError(r *ghttp.Request, status int, detail, scimType string) {
	r.Response.Header().Set("Content-Type", "application/scim+json")
	body, _ := json.Marshal(map[string]any{
		"schemas":  []string{scimcore.SchemaError},
		"status":   strconv.Itoa(status),
		"scimType": scimType,
		"detail":   detail,
	})
	r.Response.WriteStatusExit(status, string(body))
}

// IsScimUserDisabled 离职/禁用即时生效：只要 SCIM 推过 active=false（或 DELETE 软删），
// 无论 dev 演示态还是 SSO 生产态，该用户名一律拒绝访问（已签发的 JWT 也进不来）。
// 查不到记录 / 表不存在都视为未禁用，避免数据库异常把整站锁死。
func IsScimUserDisabled(username string) bool {
	if strings.TrimSpace(username) == "" {
		return false
	}
	row, err := g.DB().Model("scim_user").Fields("active").Where("username", username).One()
	if err != nil || row.IsEmpty() {
		return false
	}
	return !row["active"].Bool()
}
