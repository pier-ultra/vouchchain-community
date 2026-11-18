package controller

import (
	"encoding/json"
	"strconv"
	"strings"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/net/ghttp"
	"vouchchain/internal/middleware"
	"vouchchain/pkg/scimcore"
)

// ---------- SCIM → 本系统 RBAC 的联动 ----------
// 关键约定：SCIM 只覆盖 source='scim' 的绑定，管理员在「角色权限」页手工绑的角色不会被冲掉。
// 组名先查 scim_role_map 映射表；没配映射时，组名本身若正好是 rbac_role.code 也生效。

func jsonStrings(raw string) []string {
	out := []string{}
	if raw != "" {
		_ = json.Unmarshal([]byte(raw), &out)
	}
	if out == nil {
		out = []string{}
	}
	return out
}

func scimUserAttrs(row gdb.Record) scimcore.UserAttrs {
	active := row["active"].Bool()
	return scimcore.UserAttrs{
		ExternalId:  row["external_id"].String(),
		UserName:    row["username"].String(),
		DisplayName: row["display_name"].String(),
		Email:       row["email"].String(),
		Title:       row["title"].String(),
		Department:  row["department"].String(),
		Active:      &active,
		Groups:      jsonStrings(row["groups"].String()),
	}
}

func scimUserResource(row gdb.Record, r *ghttp.Request) map[string]any {
	id := row["id"].Int64()
	groups := jsonStrings(row["groups"].String())
	groupRefs := make([]map[string]any, 0, len(groups))
	for _, name := range groups {
		groupRefs = append(groupRefs, map[string]any{"value": name, "display": name})
	}
	emails := []map[string]any{}
	if e := row["email"].String(); e != "" {
		emails = append(emails, map[string]any{"value": e, "type": "work", "primary": true})
	}
	return map[string]any{
		"schemas":                     []string{scimcore.SchemaUser},
		"id":                          strconv.FormatInt(id, 10),
		"externalId":                  row["external_id"].String(),
		"userName":                    row["username"].String(),
		"name":                        map[string]any{"formatted": row["display_name"].String()},
		"displayName":                 row["display_name"].String(),
		"emails":                      emails,
		"active":                      row["active"].Bool(),
		"groups":                      groupRefs,
		"title":                       row["title"].String(),
		scimcore.SchemaEnterpriseUser: map[string]any{"department": row["department"].String()},
		"urn:vouchchain:scim:2.0:User": map[string]any{
			"roleCodes": jsonStrings(row["roles"].String()),
		},
		"meta": map[string]any{
			"resourceType": "User", "created": row["created_at"].String(),
			"lastModified": row["updated_at"].String(), "location": scimLocation(r, "Users", id),
		},
	}
}

func scimGroupResource(row gdb.Record, r *ghttp.Request) map[string]any {
	id := row["id"].Int64()
	members := jsonStrings(row["members"].String())
	refs := make([]map[string]any, 0, len(members))
	for _, m := range members {
		refs = append(refs, map[string]any{"value": m, "display": m})
	}
	return map[string]any{
		"schemas":     []string{scimcore.SchemaGroup},
		"id":          strconv.FormatInt(id, 10),
		"externalId":  row["external_id"].String(),
		"displayName": row["display_name"].String(),
		"members":     refs,
		"meta": map[string]any{
			"resourceType": "Group", "created": row["created_at"].String(),
			"lastModified": row["updated_at"].String(), "location": scimLocation(r, "Groups", id),
		},
	}
}

// expandGroupNames User.groups 里可能放组 id / externalId，统一换成组名再做角色映射。
func expandGroupNames(names []string) []string {
	out := []string{}
	add := func(s string) {
		s = strings.TrimSpace(s)
		if s == "" {
			return
		}
		for _, old := range out {
			if old == s {
				return
			}
		}
		out = append(out, s)
	}
	for _, name := range names {
		q := g.DB().Model("scim_group")
		if id, err := strconv.ParseInt(name, 10, 64); err == nil {
			q = q.Where("id", id)
		} else {
			q = q.Where("external_id = ? OR display_name = ?", name, name)
		}
		row, err := q.One()
		if err == nil && !row.IsEmpty() {
			add(row["display_name"].String())
		} else {
			add(name)
		}
	}
	return out
}

// resolveRoleCodes 组名 → 角色编码（映射表优先，其次是同名角色），只保留真实存在的角色。
func resolveRoleCodes(names []string) []string {
	maps := map[string]string{}
	if rows, err := g.DB().Model("scim_role_map").All(); err == nil {
		for _, row := range rows {
			maps[strings.ToLower(row["group_name"].String())] = row["role_code"].String()
		}
	}
	out := []string{}
	for _, name := range names {
		code := maps[strings.ToLower(name)]
		if code == "" {
			code = name
		}
		if !middleware.IsKnownRole(code) {
			continue
		}
		found := false
		for _, old := range out {
			if old == code {
				found = true
			}
		}
		if !found {
			out = append(out, code)
		}
	}
	return out
}

// syncUserRoles 用 SCIM 推送的组重写该用户的 scim 来源绑定。
// active=false（离职/禁用）不写任何绑定，并清掉此前 SCIM 写的绑定。
func syncUserRoles(username string, groupNames []string, active bool) ([]string, error) {
	if _, err := g.DB().Model("rbac_user_role").Where("username", username).Where("source", "scim").Delete(); err != nil {
		return nil, err
	}
	roles := []string{}
	if active {
		roles = resolveRoleCodes(expandGroupNames(groupNames))
		for _, code := range roles {
			_, err := g.DB().Model("rbac_user_role").Data(g.Map{
				"username": username, "role_code": code, "source": "scim",
			}).Insert()
			if err != nil && !strings.Contains(err.Error(), "duplicate") &&
				!strings.Contains(err.Error(), "唯一") && !strings.Contains(err.Error(), "23505") {
				return roles, err
			}
		}
	}
	middleware.InvalidateRoleCache()
	return roles, nil
}

// effectiveGroupNames 合并两处来源：User.groups 直接带的组 + 所有 Group.members 里包含该用户的组。
// IdP 有的只推 User.groups，有的只推 Group.members，两种都要能驱动角色。
func effectiveGroupNames(row gdb.Record) []string {
	names := jsonStrings(row["groups"].String())
	id := strconv.FormatInt(row["id"].Int64(), 10)
	username := row["username"].String()
	externalId := row["external_id"].String()
	rows, err := g.DB().Model("scim_group").All()
	if err != nil {
		return names
	}
	for _, gr := range rows {
		for _, m := range jsonStrings(gr["members"].String()) {
			if m == id || m == username || (externalId != "" && m == externalId) {
				name := gr["display_name"].String()
				found := false
				for _, old := range names {
					if old == name {
						found = true
					}
				}
				if !found {
					names = append(names, name)
				}
				break
			}
		}
	}
	return names
}

// refreshGroupMemberRoles 组成员变更 / 组改名 / 组删除后，重算所有相关用户的角色。
func refreshGroupMemberRoles(keys ...string) {
	seen := map[string]bool{}
	all, err := g.DB().Model("scim_user").All()
	if err != nil {
		return
	}
	for _, row := range all {
		id := strconv.FormatInt(row["id"].Int64(), 10)
		username := row["username"].String()
		hit := len(keys) == 0
		for _, k := range keys {
			if k == id || k == username || (row["external_id"].String() != "" && k == row["external_id"].String()) {
				hit = true
				break
			}
		}
		if !hit || seen[username] {
			continue
		}
		seen[username] = true
		roles, err := syncUserRoles(username, effectiveGroupNames(row), row["active"].Bool())
		if err != nil {
			continue
		}
		b, _ := json.Marshal(roles)
		_, _ = g.DB().Model("scim_user").Data(g.Map{"roles": string(b)}).Where("id", row["id"].Int64()).Update()
	}
}
