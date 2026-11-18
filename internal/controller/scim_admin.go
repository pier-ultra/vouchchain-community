package controller

import (
	"strings"

	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/net/ghttp"
	"vouchchain/internal/middleware"
)

// ---------- /api/scim/*：管理面（走现有 SSO + RBAC，不下发 token 明文） ----------

func ScimStatus(r *ghttp.Request) {
	count := func(table string, where ...any) int {
		q := g.DB().Model(table)
		if len(where) == 2 {
			q = q.Where(where[0], where[1])
		}
		n, _ := q.Count()
		return n
	}
	last := g.Map{}
	if row, err := g.DB().Model("scim_sync_log").Order("id desc").One(); err == nil && !row.IsEmpty() {
		last = g.Map{
			"operation": row["operation"].String(), "resourceType": row["resource_type"].String(),
			"username": row["username"].String(), "message": row["message"].String(),
			"httpStatus": row["http_status"].Int(), "createdAt": row["created_at"].String(),
		}
	}
	token := middleware.ScimToken()
	r.Response.WriteJsonExit(g.Map{"code": 0, "data": g.Map{
		"configured":   token != "",
		"tokenLength":  len(token),
		"baseUrl":      "/scim/v2",
		"authMode":     map[bool]string{true: "dev", false: "sso"}[middleware.IsDevMode(r)],
		"users":        count("scim_user"),
		"activeUsers":  count("scim_user", "active", true),
		"groups":       count("scim_group"),
		"logs":         count("scim_sync_log"),
		"roleMappings": count("scim_role_map"),
		"lastSync":     last,
		"endpoints": []string{
			"GET  /scim/v2/ServiceProviderConfig", "GET  /scim/v2/ResourceTypes", "GET  /scim/v2/Schemas",
			"GET  /scim/v2/Users", "POST /scim/v2/Users", "GET  /scim/v2/Users/{id}",
			"PUT  /scim/v2/Users/{id}", "PATCH /scim/v2/Users/{id}", "DELETE /scim/v2/Users/{id}",
			"GET  /scim/v2/Groups", "POST /scim/v2/Groups", "PATCH /scim/v2/Groups/{id}",
			"PUT  /scim/v2/Groups/{id}", "DELETE /scim/v2/Groups/{id}",
		},
	}})
}

func ScimAdminUsers(r *ghttp.Request) {
	rows, err := g.DB().Model("scim_user").Order("id desc").All()
	if err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	list := make([]g.Map, 0, len(rows))
	for _, row := range rows {
		list = append(list, g.Map{
			"id": row["id"].Int64(), "externalId": row["external_id"].String(),
			"username": row["username"].String(), "displayName": row["display_name"].String(),
			"email": row["email"].String(), "title": row["title"].String(),
			"department": row["department"].String(), "active": row["active"].Bool(),
			"groups": jsonStrings(row["groups"].String()), "roles": jsonStrings(row["roles"].String()),
			"createdAt": row["created_at"].String(), "lastSyncedAt": row["last_synced_at"].String(),
		})
	}
	r.Response.WriteJsonExit(g.Map{"code": 0, "data": list})
}

// ScimUserPurge 管理员物理删除残留的同步账号（SCIM DELETE 是软删，清库/隐私清理走这里）。
func ScimUserPurge(r *ghttp.Request) {
	id := r.Get("id").Int64()
	row, err := g.DB().Model("scim_user").Where("id", id).One()
	if err != nil || row.IsEmpty() {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": "同步账号不存在"})
		return
	}
	username := row["username"].String()
	if _, err := g.DB().Model("scim_user").Where("id", id).Delete(); err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	_, _ = syncUserRoles(username, nil, false)
	audit(r, "scim.user.purge", "scim_user", id, g.Map{"username": username})
	r.Response.WriteJsonExit(g.Map{"code": 0, "message": "ok"})
}

func ScimAdminGroups(r *ghttp.Request) {
	rows, err := g.DB().Model("scim_group").Order("id desc").All()
	if err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	list := make([]g.Map, 0, len(rows))
	for _, row := range rows {
		list = append(list, g.Map{
			"id": row["id"].Int64(), "externalId": row["external_id"].String(),
			"displayName": row["display_name"].String(), "members": jsonStrings(row["members"].String()),
			"createdAt": row["created_at"].String(),
		})
	}
	r.Response.WriteJsonExit(g.Map{"code": 0, "data": list})
}

func ScimAdminLogs(r *ghttp.Request) {
	limit := r.Get("limit").Int()
	if limit < 1 || limit > 200 {
		limit = 50
	}
	rows, err := g.DB().Model("scim_sync_log").Order("id desc").Limit(limit).All()
	if err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	list := make([]g.Map, 0, len(rows))
	for _, row := range rows {
		list = append(list, g.Map{
			"id": row["id"].Int64(), "operation": row["operation"].String(),
			"resourceType": row["resource_type"].String(), "resourceId": row["resource_id"].String(),
			"username": row["username"].String(), "externalId": row["external_id"].String(),
			"httpStatus": row["http_status"].Int(), "message": row["message"].String(),
			"actor": row["actor"].String(), "payload": row["payload"].String(),
			"createdAt": row["created_at"].String(),
		})
	}
	r.Response.WriteJsonExit(g.Map{"code": 0, "data": list})
}

func ScimRoleMapList(r *ghttp.Request) {
	rows, err := g.DB().Model("scim_role_map").Order("id asc").All()
	if err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	list := make([]g.Map, 0, len(rows))
	for _, row := range rows {
		list = append(list, g.Map{
			"id": row["id"].Int64(), "groupName": row["group_name"].String(),
			"roleCode": row["role_code"].String(), "note": row["note"].String(),
		})
	}
	r.Response.WriteJsonExit(g.Map{"code": 0, "data": list})
}

func ScimRoleMapSave(r *ghttp.Request) {
	var req struct {
		Id        int64  `json:"id"`
		GroupName string `json:"groupName"`
		RoleCode  string `json:"roleCode"`
		Note      string `json:"note"`
	}
	if err := r.Parse(&req); err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	req.GroupName = strings.TrimSpace(req.GroupName)
	req.RoleCode = strings.TrimSpace(req.RoleCode)
	if req.GroupName == "" || !middleware.IsKnownRole(req.RoleCode) {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": "组名必填，角色编码必须是已存在的角色"})
		return
	}
	if req.Id > 0 {
		if _, err := g.DB().Model("scim_role_map").Data(g.Map{
			"group_name": req.GroupName, "role_code": req.RoleCode, "note": req.Note,
		}).Where("id", req.Id).Update(); err != nil {
			r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
			return
		}
	} else {
		if _, err := g.DB().Model("scim_role_map").Data(g.Map{
			"group_name": req.GroupName, "role_code": req.RoleCode, "note": req.Note,
		}).Insert(); err != nil {
			r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
			return
		}
	}
	// 映射改了，存量用户的角色要跟着变
	refreshGroupMemberRoles()
	audit(r, "scim.role_map.save", "scim_role_map", req.Id, g.Map{
		"groupName": req.GroupName, "roleCode": req.RoleCode,
	})
	r.Response.WriteJsonExit(g.Map{"code": 0, "message": "ok"})
}

func ScimRoleMapDelete(r *ghttp.Request) {
	id := r.Get("id").Int64()
	if _, err := g.DB().Model("scim_role_map").Where("id", id).Delete(); err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	refreshGroupMemberRoles()
	audit(r, "scim.role_map.delete", "scim_role_map", id, nil)
	r.Response.WriteJsonExit(g.Map{"code": 0, "message": "ok"})
}
