package controller

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"vouchchain/pkg/scimcore"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/net/ghttp"
)

// ---------- SCIM 2.0 协议层（RFC 7644） ----------
// 响应统一 application/scim+json；列表是 ListResponse，错误是 scim:Error。
// 资源持久化在 scim_user / scim_group，与业务表解耦；角色联动见 scim_sync.go。

func scimContentType(r *ghttp.Request) {
	r.Response.Header().Set("Content-Type", "application/scim+json")
}

func scimBody(r *ghttp.Request) (map[string]any, error) {
	raw := r.GetBody()
	if len(strings.TrimSpace(string(raw))) == 0 {
		return nil, fmt.Errorf("请求体为空")
	}
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("JSON 解析失败：%w", err)
	}
	return doc, nil
}

func scimPatchOps(r *ghttp.Request) ([]scimcore.PatchOp, error) {
	doc, err := scimBody(r)
	if err != nil {
		return nil, err
	}
	raw, ok := doc["Operations"]
	if !ok {
		return nil, fmt.Errorf("缺少 Operations（SCIM PatchOp）")
	}
	b, _ := json.Marshal(raw)
	var ops []scimcore.PatchOp
	if err := json.Unmarshal(b, &ops); err != nil || len(ops) == 0 {
		return nil, fmt.Errorf("Operations 格式非法或为空")
	}
	return ops, nil
}

func scimErr(r *ghttp.Request, status int, detail, scimType string) {
	scimContentType(r)
	body, _ := json.Marshal(map[string]any{
		"schemas":  []string{scimcore.SchemaError},
		"status":   strconv.Itoa(status),
		"scimType": scimType,
		"detail":   detail,
	})
	r.Response.WriteStatusExit(status, string(body))
}

func scimOK(r *ghttp.Request, status int, resource map[string]any) {
	scimContentType(r)
	body, err := json.Marshal(resource)
	if err != nil {
		scimErr(r, 500, err.Error(), "serverError")
		return
	}
	// 必须一次性写完整 JSON：WriteStatus(status) 会先落一段 http.StatusText，
	// 再 WriteJsonExit 追加对象会得到 "OK{...}" 这种非法响应体，IdP 解析不了。
	r.Response.WriteStatusExit(status, string(body))
}

func scimNoContent(r *ghttp.Request) {
	r.Response.WriteStatusExit(204, "")
}

func scimList(r *ghttp.Request, resources []map[string]any, total, start, count int) {
	scimOK(r, 200, map[string]any{
		"schemas":      []string{scimcore.SchemaListResponse},
		"totalResults": total,
		"startIndex":   start,
		"itemsPerPage": len(resources),
		"Resources":    resources,
	})
	_ = count
}

func scimPagination(r *ghttp.Request) (int, int) {
	start := r.Get("startIndex").Int()
	if start < 1 {
		start = 1
	}
	count := r.Get("count").Int()
	if count < 1 {
		count = 100
	}
	if count > 200 {
		count = 200
	}
	return start, count
}

func scimLocation(r *ghttp.Request, resource string, id int64) string {
	scheme := "http"
	if r.Header.Get("X-Forwarded-Proto") != "" {
		scheme = r.Header.Get("X-Forwarded-Proto")
	} else if r.TLS != nil {
		scheme = "https"
	}
	return scheme + "://" + r.Host + "/scim/v2/" + resource + "/" + strconv.FormatInt(id, 10)
}

// scimLog 每次推送留痕（不记录 Authorization 头 / token）
func scimLog(r *ghttp.Request, op, resourceType, resourceId, username, externalId string, status int, message string, payload any) {
	body := ""
	if payload != nil {
		b, err := json.Marshal(payload)
		if err == nil {
			body = string(b)
			if len(body) > 4000 {
				body = body[:4000] + "…"
			}
		}
	}
	_, err := g.DB().Model("scim_sync_log").Data(g.Map{
		"operation":     op,
		"resource_type": resourceType,
		"resource_id":   resourceId,
		"username":      username,
		"external_id":   externalId,
		"http_status":   status,
		"message":       message,
		"actor":         r.GetCtxVar("scimActor").String(),
		"payload":       body,
	}).Insert()
	if err != nil {
		g.Log().Warningf(r.Context(), "[scim] 写同步日志失败：%v", err)
	}
}

// ---------- 发现端点：IdP 接入时先探测能力 ----------

func ScimServiceProviderConfig(r *ghttp.Request) {
	scimOK(r, 200, map[string]any{
		"schemas":          []string{scimcore.SchemaServiceProvider},
		"documentationUri": "https://www.rfc-editor.org/rfc/rfc7644",
		"patch":            map[string]any{"supported": true},
		"bulk":             map[string]any{"supported": false, "maxOperations": 0, "maxPayloadSize": 0},
		"filter":           map[string]any{"supported": true, "maxResults": 200},
		"changePassword":   map[string]any{"supported": false},
		"sort":             map[string]any{"supported": false},
		"etag":             map[string]any{"supported": false},
		"authenticationSchemes": []map[string]any{{
			"type": "oauthbearertoken", "name": "SCIM Bearer Token", "primary": true,
			"description": "部署时通过环境变量 SCIM_TOKEN 配置，IdP 以 Authorization: Bearer <token> 推送",
			"specUri":     "https://www.rfc-editor.org/rfc/rfc6750",
		}},
	})
}

func ScimResourceTypes(r *ghttp.Request) {
	scimOK(r, 200, map[string]any{
		"schemas":      []string{scimcore.SchemaListResponse},
		"totalResults": 2,
		"startIndex":   1,
		"itemsPerPage": 2,
		"Resources": []map[string]any{
			{
				"schemas": []string{scimcore.SchemaResourceType}, "id": "User", "name": "User",
				"endpoint": "/Users", "description": "用户账号（自动开户 / 改资料 / 离职禁用）",
				"schema": scimcore.SchemaUser,
				"meta":   map[string]any{"resourceType": "ResourceType", "location": "/scim/v2/ResourceTypes/User"},
			},
			{
				"schemas": []string{scimcore.SchemaResourceType}, "id": "Group", "name": "Group",
				"endpoint": "/Groups", "description": "组（成员关系驱动本系统角色绑定）",
				"schema": scimcore.SchemaGroup,
				"meta":   map[string]any{"resourceType": "ResourceType", "location": "/scim/v2/ResourceTypes/Group"},
			},
		},
	})
}

func ScimSchemas(r *ghttp.Request) {
	attr := func(name, typ string, required bool) map[string]any {
		return map[string]any{"name": name, "type": typ, "multiValued": false, "required": required, "caseExact": false, "returned": "default", "uniqueness": "none"}
	}
	scimList(r, []map[string]any{
		{
			"schemas": []string{scimcore.SchemaSchema}, "id": scimcore.SchemaUser, "name": "User",
			"description": "SCIM 核心用户", "attributes": []map[string]any{
				attr("userName", "string", true), attr("displayName", "string", false),
				attr("externalId", "string", false), attr("active", "boolean", false),
				attr("emails", "complex", false), attr("groups", "complex", false),
			},
			"meta": map[string]any{"resourceType": "Schema", "location": "/scim/v2/Schemas/" + scimcore.SchemaUser},
		},
		{
			"schemas": []string{scimcore.SchemaSchema}, "id": scimcore.SchemaGroup, "name": "Group",
			"description": "SCIM 核心组", "attributes": []map[string]any{
				attr("displayName", "string", true), attr("externalId", "string", false),
				attr("members", "complex", false),
			},
			"meta": map[string]any{"resourceType": "Schema", "location": "/scim/v2/Schemas/" + scimcore.SchemaGroup},
		},
	}, 2, 1, 2)
}
