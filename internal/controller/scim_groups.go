package controller

import (
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"vouchchain/pkg/scimcore"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/net/ghttp"
)

// ---------- /scim/v2/Groups：组成员变化驱动本系统角色绑定 ----------

func ScimGroupList(r *ghttp.Request) {
	rows, err := g.DB().Model("scim_group").Order("id asc").All()
	if err != nil {
		scimErr(r, 500, err.Error(), "serverError")
		return
	}
	start, count := scimPagination(r)
	filter := r.Get("filter").String()
	resources := []map[string]any{}
	total := 0
	for _, row := range rows {
		attrs := scimcore.GroupAttrs{
			ExternalId: row["external_id"].String(), DisplayName: row["display_name"].String(),
			Members: jsonStrings(row["members"].String()),
		}
		ok, ferr := scimcore.MatchGroupFilter(filter, attrs, row["id"].Int64())
		if ferr != nil {
			scimErr(r, 400, ferr.Error(), "invalidFilter")
			return
		}
		if !ok {
			continue
		}
		total++
		if total >= start && len(resources) < count {
			resources = append(resources, scimGroupResource(row, r))
		}
	}
	scimList(r, resources, total, start, count)
}

func ScimGroupGet(r *ghttp.Request) {
	id := r.Get("id").Int64()
	row, err := g.DB().Model("scim_group").Where("id", id).One()
	if err != nil || row.IsEmpty() {
		scimErr(r, 404, "Group 不存在："+strconv.FormatInt(id, 10), "notFound")
		return
	}
	scimOK(r, 200, scimGroupResource(row, r))
}

func ScimGroupCreate(r *ghttp.Request) {
	doc, err := scimBody(r)
	if err != nil {
		scimErr(r, 400, err.Error(), "invalidSyntax")
		return
	}
	g0, err := scimcore.ParseGroup(doc)
	if err != nil {
		scimErr(r, 400, err.Error(), "invalidValue")
		return
	}
	if scimGroupConflict(r, g0, 0) {
		return
	}
	resource, err := scimWriteGroup(r, 0, g0, doc, "create", nil)
	if err != nil {
		scimErr(r, 500, err.Error(), "serverError")
		return
	}
	newId, _ := strconv.ParseInt(fmt.Sprint(resource["id"]), 10, 64)
	r.Response.Header().Set("Location", scimLocation(r, "Groups", newId))
	scimOK(r, 201, resource)
}

func ScimGroupPut(r *ghttp.Request) {
	id := r.Get("id").Int64()
	old, err := g.DB().Model("scim_group").Where("id", id).One()
	if err != nil || old.IsEmpty() {
		scimErr(r, 404, "Group 不存在", "notFound")
		return
	}
	doc, err := scimBody(r)
	if err != nil {
		scimErr(r, 400, err.Error(), "invalidSyntax")
		return
	}
	g0, err := scimcore.ParseGroup(doc)
	if err != nil {
		scimErr(r, 400, err.Error(), "invalidValue")
		return
	}
	if scimGroupConflict(r, g0, id) {
		return
	}
	resource, err := scimWriteGroup(r, id, g0, doc, "replace", jsonStrings(old["members"].String()))
	if err != nil {
		scimErr(r, 500, err.Error(), "serverError")
		return
	}
	scimOK(r, 200, resource)
}

func ScimGroupPatch(r *ghttp.Request) {
	id := r.Get("id").Int64()
	old, err := g.DB().Model("scim_group").Where("id", id).One()
	if err != nil || old.IsEmpty() {
		scimErr(r, 404, "Group 不存在", "notFound")
		return
	}
	ops, err := scimPatchOps(r)
	if err != nil {
		scimErr(r, 400, err.Error(), "invalidSyntax")
		return
	}
	base := scimcore.GroupAttrs{
		ExternalId: old["external_id"].String(), DisplayName: old["display_name"].String(),
		Members: jsonStrings(old["members"].String()),
	}
	g0, err := scimcore.ApplyGroupPatch(base, ops)
	if err != nil {
		scimErr(r, 400, err.Error(), "invalidPath")
		return
	}
	if scimGroupConflict(r, g0, id) {
		return
	}
	resource, err := scimWriteGroup(r, id, g0, map[string]any{"Operations": ops}, "patch", base.Members)
	if err != nil {
		scimErr(r, 500, err.Error(), "serverError")
		return
	}
	scimOK(r, 200, resource)
}

func ScimGroupDelete(r *ghttp.Request) {
	id := r.Get("id").Int64()
	row, err := g.DB().Model("scim_group").Where("id", id).One()
	if err != nil || row.IsEmpty() {
		scimErr(r, 404, "Group 不存在", "notFound")
		return
	}
	if _, err := g.DB().Model("scim_group").Where("id", id).Delete(); err != nil {
		scimErr(r, 500, err.Error(), "serverError")
		return
	}
	// 组删掉后全量重算，避免残留由该组赋予的角色
	refreshGroupMemberRoles()
	scimLog(r, "delete", "Group", strconv.FormatInt(id, 10), "", row["external_id"].String(), 204,
		"删除组并重算全部用户角色", nil)
	scimNoContent(r)
}

func scimGroupConflict(r *ghttp.Request, gr scimcore.GroupAttrs, excludeId int64) bool {
	q := g.DB().Model("scim_group").Where("display_name", gr.DisplayName)
	if excludeId > 0 {
		q = q.Where("id <> ?", excludeId)
	}
	dup, err := q.One()
	if err == nil && !dup.IsEmpty() {
		scimErr(r, 409, "displayName 已存在："+gr.DisplayName, "uniqueness")
		return true
	}
	if gr.ExternalId != "" {
		q = g.DB().Model("scim_group").Where("external_id", gr.ExternalId)
		if excludeId > 0 {
			q = q.Where("id <> ?", excludeId)
		}
		dup, err = q.One()
		if err == nil && !dup.IsEmpty() {
			scimErr(r, 409, "externalId 已存在："+gr.ExternalId, "uniqueness")
			return true
		}
	}
	return false
}

func scimWriteGroup(r *ghttp.Request, id int64, gr scimcore.GroupAttrs, raw map[string]any, op string, oldMembers []string) (map[string]any, error) {
	membersJSON, _ := json.Marshal(gr.Members)
	rawJSON, _ := json.Marshal(raw)
	data := g.Map{
		"external_id": gr.ExternalId, "display_name": gr.DisplayName,
		"members": string(membersJSON), "raw": string(rawJSON), "updated_at": time.Now(),
	}
	if id > 0 {
		if _, err := g.DB().Model("scim_group").Data(data).Where("id", id).Update(); err != nil {
			return nil, err
		}
	} else {
		res, err := g.DB().Model("scim_group").Data(data).Insert()
		if err != nil {
			return nil, err
		}
		id, _ = res.LastInsertId()
	}
	// 旧成员 ∪ 新成员都要重算；组改名/删除走全量重算
	keys := append(append([]string{}, oldMembers...), gr.Members...)
	if len(keys) == 0 {
		refreshGroupMemberRoles()
	} else {
		refreshGroupMemberRoles(keys...)
	}
	row, err := g.DB().Model("scim_group").Where("id", id).One()
	if err != nil || row.IsEmpty() {
		return nil, fmt.Errorf("写回后读取组失败")
	}
	status := 200
	if op == "create" {
		status = 201
	}
	scimLog(r, op, "Group", strconv.FormatInt(id, 10), "", gr.ExternalId, status,
		fmt.Sprintf("displayName=%s members=%v → 已重算成员角色", gr.DisplayName, gr.Members), raw)
	return scimGroupResource(row, r), nil
}
