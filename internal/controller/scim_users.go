package controller

import (
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/net/ghttp"
	"vouchchain/pkg/scimcore"
)

// ---------- /scim/v2/Users：IdP 推送开户、改资料、改组、离职禁用 ----------

func ScimUserList(r *ghttp.Request) {
	rows, err := g.DB().Model("scim_user").Order("id asc").All()
	if err != nil {
		scimErr(r, 500, err.Error(), "serverError")
		return
	}
	start, count := scimPagination(r)
	filter := r.Get("filter").String()
	resources := []map[string]any{}
	total := 0
	for _, row := range rows {
		ok, ferr := scimcore.MatchUserFilter(filter, scimUserAttrs(row), row["id"].Int64())
		if ferr != nil {
			scimErr(r, 400, ferr.Error(), "invalidFilter")
			return
		}
		if !ok {
			continue
		}
		total++
		if total >= start && len(resources) < count {
			resources = append(resources, scimUserResource(row, r))
		}
	}
	scimList(r, resources, total, start, count)
}

func ScimUserGet(r *ghttp.Request) {
	id := r.Get("id").Int64()
	row, err := g.DB().Model("scim_user").Where("id", id).One()
	if err != nil || row.IsEmpty() {
		scimErr(r, 404, "User 不存在："+strconv.FormatInt(id, 10), "notFound")
		return
	}
	scimOK(r, 200, scimUserResource(row, r))
}

func ScimUserCreate(r *ghttp.Request) {
	doc, err := scimBody(r)
	if err != nil {
		scimErr(r, 400, err.Error(), "invalidSyntax")
		return
	}
	u, err := scimcore.ParseUser(doc)
	if err != nil {
		scimErr(r, 400, err.Error(), "invalidValue")
		return
	}
	if scimUserConflict(r, u, 0) {
		return
	}
	resource, err := scimWriteUser(r, 0, u, doc, "create")
	if err != nil {
		scimErr(r, 500, err.Error(), "serverError")
		return
	}
	newId, _ := strconv.ParseInt(fmt.Sprint(resource["id"]), 10, 64)
	r.Response.Header().Set("Location", scimLocation(r, "Users", newId))
	scimOK(r, 201, resource)
}

// ScimUserPut 全量替换：未带的字段按空处理；未带 active 视为启用（避免 IdP 漏字段把人误禁用）。
func ScimUserPut(r *ghttp.Request) {
	id := r.Get("id").Int64()
	old, err := g.DB().Model("scim_user").Where("id", id).One()
	if err != nil || old.IsEmpty() {
		scimErr(r, 404, "User 不存在", "notFound")
		return
	}
	doc, err := scimBody(r)
	if err != nil {
		scimErr(r, 400, err.Error(), "invalidSyntax")
		return
	}
	u, err := scimcore.ParseUser(doc)
	if err != nil {
		scimErr(r, 400, err.Error(), "invalidValue")
		return
	}
	if u.Active == nil {
		b := true
		u.Active = &b
	}
	if scimUserConflict(r, u, id) {
		return
	}
	resource, err := scimWriteUser(r, id, u, doc, "replace")
	if err != nil {
		scimErr(r, 500, err.Error(), "serverError")
		return
	}
	scimOK(r, 200, resource)
}

func ScimUserPatch(r *ghttp.Request) {
	id := r.Get("id").Int64()
	old, err := g.DB().Model("scim_user").Where("id", id).One()
	if err != nil || old.IsEmpty() {
		scimErr(r, 404, "User 不存在", "notFound")
		return
	}
	ops, err := scimPatchOps(r)
	if err != nil {
		scimErr(r, 400, err.Error(), "invalidSyntax")
		return
	}
	u, err := scimcore.ApplyUserPatch(scimUserAttrs(old), ops)
	if err != nil {
		scimErr(r, 400, err.Error(), "invalidPath")
		return
	}
	if scimUserConflict(r, u, id) {
		return
	}
	resource, err := scimWriteUser(r, id, u, map[string]any{"Operations": ops}, "patch")
	if err != nil {
		scimErr(r, 500, err.Error(), "serverError")
		return
	}
	scimOK(r, 200, resource)
}

// ScimUserDelete 按 SCIM 惯例做软删（置 active=false），保留同步轨迹；
// 离职后已签发的 JWT 也会被 auth 中间件挡掉。
func ScimUserDelete(r *ghttp.Request) {
	id := r.Get("id").Int64()
	row, err := g.DB().Model("scim_user").Where("id", id).One()
	if err != nil || row.IsEmpty() {
		scimErr(r, 404, "User 不存在", "notFound")
		return
	}
	username := row["username"].String()
	if _, err := g.DB().Model("scim_user").Data(g.Map{
		"active": false, "roles": "[]", "updated_at": time.Now(), "last_synced_at": time.Now(),
	}).Where("id", id).Update(); err != nil {
		scimErr(r, 500, err.Error(), "serverError")
		return
	}
	_, _ = syncUserRoles(username, nil, false)
	scimLog(r, "delete", "User", strconv.FormatInt(id, 10), username, row["external_id"].String(), 204,
		"按 SCIM 惯例软删为禁用（保留账号轨迹）", nil)
	scimNoContent(r)
}

// scimUserConflict 用户名 / externalId 唯一性检查，命中则写 409
func scimUserConflict(r *ghttp.Request, u scimcore.UserAttrs, excludeId int64) bool {
	q := g.DB().Model("scim_user").Where("username", u.UserName)
	if excludeId > 0 {
		q = q.Where("id <> ?", excludeId)
	}
	dup, err := q.One()
	if err == nil && !dup.IsEmpty() {
		scimErr(r, 409, "userName 已存在："+u.UserName, "uniqueness")
		return true
	}
	if u.ExternalId != "" {
		q = g.DB().Model("scim_user").Where("external_id", u.ExternalId)
		if excludeId > 0 {
			q = q.Where("id <> ?", excludeId)
		}
		dup, err = q.One()
		if err == nil && !dup.IsEmpty() {
			scimErr(r, 409, "externalId 已存在："+u.ExternalId, "uniqueness")
			return true
		}
	}
	return false
}

// scimWriteUser 落库 + 重写 scim 来源角色绑定 + 留痕，Create/PUT/PATCH 共用。
func scimWriteUser(r *ghttp.Request, id int64, u scimcore.UserAttrs, raw map[string]any, op string) (map[string]any, error) {
	active := true
	if u.Active != nil {
		active = *u.Active
	}
	groupsJSON, _ := json.Marshal(u.Groups)
	rawJSON, _ := json.Marshal(raw)
	data := g.Map{
		"external_id": u.ExternalId, "username": u.UserName, "display_name": u.DisplayName,
		"email": u.Email, "title": u.Title, "department": u.Department,
		"active": active, "groups": string(groupsJSON), "raw": string(rawJSON),
		"updated_at": time.Now(), "last_synced_at": time.Now(),
	}
	if id > 0 {
		old, _ := g.DB().Model("scim_user").Where("id", id).One()
		if !old.IsEmpty() && old["username"].String() != u.UserName {
			// 改名：先清掉旧用户名下 SCIM 写的绑定，避免幽灵授权
			_, _ = g.DB().Model("rbac_user_role").Where("username", old["username"].String()).Where("source", "scim").Delete()
		}
		if _, err := g.DB().Model("scim_user").Data(data).Where("id", id).Update(); err != nil {
			return nil, err
		}
	} else {
		res, err := g.DB().Model("scim_user").Data(data).Insert()
		if err != nil {
			return nil, err
		}
		id, _ = res.LastInsertId()
	}
	roles, err := syncUserRoles(u.UserName, u.Groups, active)
	if err != nil {
		return nil, fmt.Errorf("同步角色失败：%w", err)
	}
	rolesJSON, _ := json.Marshal(roles)
	if _, err := g.DB().Model("scim_user").Data(g.Map{"roles": string(rolesJSON)}).Where("id", id).Update(); err != nil {
		return nil, err
	}
	row, err := g.DB().Model("scim_user").Where("id", id).One()
	if err != nil || row.IsEmpty() {
		return nil, fmt.Errorf("写回后读取用户失败")
	}
	status := 200
	if op == "create" {
		status = 201
	}
	scimLog(r, op, "User", strconv.FormatInt(id, 10), u.UserName, u.ExternalId, status,
		fmt.Sprintf("active=%v groups=%v roles=%v", active, u.Groups, roles), raw)
	return scimUserResource(row, r), nil
}

// scimUserFind 组变更时按 id / username / externalId 反查用户
func scimUserFind(key string) (gdb.Record, error) {
	q := g.DB().Model("scim_user")
	if id, err := strconv.ParseInt(key, 10, 64); err == nil {
		return q.Where("id", id).One()
	}
	return q.Where("username = ? OR external_id = ?", key, key).One()
}
