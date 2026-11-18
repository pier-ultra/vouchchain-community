package controller

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/net/ghttp"
	"vouchchain/internal/middleware"
)

// ---------- 产品② US-G1 RBAC 角色与权限配置（AG1.1–AG1.6） ----------
// 自定义角色 / 权限分配（功能+数据范围+字段级）/ 用户授权 / 实时校验 / 预置角色 / 变更审计。
// 实时校验落在 middleware（每次请求读缓存），列表可见范围落在 scopeFilter，字段掩码落在 applyFieldMask。

func RoleList(r *ghttp.Request) {
	rows, err := g.DB().Model("rbac_role").Order("id asc").All()
	if err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	list := make([]g.Map, 0, len(rows))
	for _, row := range rows {
		perms, masks := []string{}, []string{}
		_ = json.Unmarshal([]byte(row["permissions"].String()), &perms)
		_ = json.Unmarshal([]byte(row["field_mask"].String()), &masks)
		cnt, _ := g.DB().Model("rbac_user_role").Where("role_code", row["code"].String()).Count()
		list = append(list, g.Map{
			"id": row["id"].Int64(), "code": row["code"].String(), "name": row["name"].String(),
			"description": row["description"].String(), "isBuiltin": row["is_builtin"].Bool(),
			"permissions": perms, "dataScope": row["data_scope"].String(), "fieldMask": masks,
			"enabled": row["enabled"].Bool(), "userCount": cnt, "createdAt": row["created_at"].String(),
		})
	}
	r.Response.WriteJsonExit(g.Map{"code": 0, "data": list})
}

type roleReq struct {
	Id          int64    `json:"id"`
	Code        string   `json:"code"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Permissions []string `json:"permissions"`
	DataScope   string   `json:"dataScope"`
	FieldMask   []string `json:"fieldMask"`
	Enabled     *bool    `json:"enabled"`
}

func (q roleReq) validate() error {
	if strings.TrimSpace(q.Code) == "" || strings.TrimSpace(q.Name) == "" {
		return fmt.Errorf("角色编码与名称必填")
	}
	switch q.DataScope {
	case "", "all", "entity", "dept", "self":
	default:
		return fmt.Errorf("数据范围必须是 all / entity / dept / self")
	}
	for _, p := range q.Permissions {
		pp := strings.TrimPrefix(p, "-")
		parts := strings.SplitN(pp, " ", 2)
		if len(parts) != 2 {
			return fmt.Errorf("权限项格式应为 POST /api/xxx、MENU /xxx 或 ACTION xxx：%s", p)
		}
		// 菜单、按钮类权限必须是白名单里的合法项，避免拼错后静默失效
		switch parts[0] {
		case "MENU":
			if !middleware.ValidMenuPath(parts[1]) {
				return fmt.Errorf("未知菜单路径：%s", parts[1])
			}
		case "ACTION":
			if !middleware.ValidAction(parts[1]) {
				return fmt.Errorf("未知按钮权限：%s", parts[1])
			}
		}
	}
	return nil
}

// RoleSave AG1.1/AG1.2/AG1.6：新建或修改自定义角色（内置角色不可改编码与内置标记）
func RoleSave(r *ghttp.Request) {
	var req roleReq
	if err := r.Parse(&req); err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	if err := req.validate(); err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	scope := req.DataScope
	if scope == "" {
		scope = "all"
	}
	perms, _ := json.Marshal(req.Permissions)
	masks, _ := json.Marshal(req.FieldMask)
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	if req.Id > 0 {
		old, err := g.DB().Model("rbac_role").Where("id", req.Id).One()
		if err != nil || old.IsEmpty() {
			r.Response.WriteJsonExit(g.Map{"code": 1, "message": "角色不存在"})
			return
		}
		if old["is_builtin"].Bool() && old["code"].String() != req.Code {
			r.Response.WriteJsonExit(g.Map{"code": 1, "message": "预置角色编码不可修改"})
			return
		}
		_, err = g.DB().Model("rbac_role").Data(g.Map{
			"name": req.Name, "description": req.Description, "permissions": string(perms),
			"data_scope": scope, "field_mask": string(masks), "enabled": enabled, "updated_at": time.Now(),
		}).Where("id", req.Id).Update()
		if err != nil {
			r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
			return
		}
		middleware.InvalidateRoleCache()
		audit(r, "rbac.role.update", "rbac_role", req.Id, g.Map{
			"code": old["code"].String(), "permissions": req.Permissions, "dataScope": scope, "fieldMask": req.FieldMask,
		})
		r.Response.WriteJsonExit(g.Map{"code": 0, "data": g.Map{"id": req.Id}})
		return
	}
	cnt, _ := g.DB().Model("rbac_role").Where("code", req.Code).Count()
	if cnt > 0 {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": "角色编码已存在"})
		return
	}
	res, err := g.DB().Model("rbac_role").Data(g.Map{
		"code": req.Code, "name": req.Name, "description": req.Description, "is_builtin": false,
		"permissions": string(perms), "data_scope": scope, "field_mask": string(masks),
		"enabled": enabled, "created_at": time.Now(), "updated_at": time.Now(),
	}).Insert()
	if err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	id, _ := res.LastInsertId()
	middleware.InvalidateRoleCache()
	audit(r, "rbac.role.create", "rbac_role", id, g.Map{"code": req.Code, "permissions": req.Permissions})
	r.Response.WriteJsonExit(g.Map{"code": 0, "data": g.Map{"id": id}})
}

func RoleDelete(r *ghttp.Request) {
	id := r.Get("id").Int64()
	row, err := g.DB().Model("rbac_role").Where("id", id).One()
	if err != nil || row.IsEmpty() {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": "角色不存在"})
		return
	}
	if row["is_builtin"].Bool() {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": "预置角色不可删除（AG1.5）"})
		return
	}
	cnt, _ := g.DB().Model("rbac_user_role").Where("role_code", row["code"].String()).Count()
	if cnt > 0 {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": "仍有用户绑定该角色，请先解绑"})
		return
	}
	if _, err := g.DB().Model("rbac_role").Where("id", id).Delete(); err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	middleware.InvalidateRoleCache()
	audit(r, "rbac.role.delete", "rbac_role", id, g.Map{"code": row["code"].String()})
	r.Response.WriteJsonExit(g.Map{"code": 0, "message": "ok"})
}

func RoleToggle(r *ghttp.Request) {
	id := r.Get("id").Int64()
	row, err := g.DB().Model("rbac_role").Where("id", id).One()
	if err != nil || row.IsEmpty() {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": "角色不存在"})
		return
	}
	next := !row["enabled"].Bool()
	if _, err := g.DB().Model("rbac_role").Data(g.Map{"enabled": next, "updated_at": time.Now()}).Where("id", id).Update(); err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	middleware.InvalidateRoleCache()
	audit(r, "rbac.role.toggle", "rbac_role", id, g.Map{"enabled": next})
	r.Response.WriteJsonExit(g.Map{"code": 0, "data": g.Map{"enabled": next}})
}

// RoleBindingList AG1.3
func RoleBindingList(r *ghttp.Request) {
	rows, err := g.DB().Model("rbac_user_role").Order("id asc").All()
	if err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	r.Response.WriteJsonExit(g.Map{"code": 0, "data": rows})
}

func RoleBindingSave(r *ghttp.Request) {
	var req struct {
		Username string `json:"username"`
		RoleCode string `json:"roleCode"`
		Action   string `json:"action"`
	}
	if err := r.Parse(&req); err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	if strings.TrimSpace(req.Username) == "" || strings.TrimSpace(req.RoleCode) == "" {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": "用户名与角色编码必填"})
		return
	}
	if req.Action == "unbind" {
		if _, err := g.DB().Model("rbac_user_role").Where("username", req.Username).Where("role_code", req.RoleCode).Delete(); err != nil {
			r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
			return
		}
		middleware.InvalidateRoleCache()
		audit(r, "rbac.binding.unbind", "rbac_user_role", 0, g.Map{"username": req.Username, "roleCode": req.RoleCode})
		r.Response.WriteJsonExit(g.Map{"code": 0, "message": "ok"})
		return
	}
	row, err := g.DB().Model("rbac_role").Where("code", req.RoleCode).One()
	if err != nil || row.IsEmpty() || !row["enabled"].Bool() {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": "角色不存在或已停用"})
		return
	}
	if _, err := g.DB().Model("rbac_user_role").Data(g.Map{
		"username": req.Username, "role_code": req.RoleCode, "created_at": time.Now(),
	}).Insert(); err != nil && !strings.Contains(err.Error(), "duplicate") && !strings.Contains(err.Error(), "唯一") {
		// 唯一索引冲突视为已绑定，保持幂等
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	middleware.InvalidateRoleCache()
	audit(r, "rbac.binding.bind", "rbac_user_role", 0, g.Map{"username": req.Username, "roleCode": req.RoleCode})
	r.Response.WriteJsonExit(g.Map{"code": 0, "message": "ok"})
}

// RoleResources 可分配权限项（API 规则 + 菜单 + 按钮），供角色配置页勾选。
// 菜单项一经保存即通过 /api/me 影响前端侧栏，实现「后端配置 → 菜单实时变化」。
func RoleResources(r *ghttp.Request) {
	items := make([]g.Map, 0)
	for _, p := range middleware.StaticRules() {
		res := p[1]
		if strings.HasSuffix(res, "/") {
			res += "*" // 以 / 结尾的静态规则本就是前缀规则，转成显式通配
		}
		items = append(items, g.Map{"value": p[0] + " " + res, "method": p[0], "path": p[1], "label": "接口 · " + p[2], "kind": "api"})
	}
	for _, m := range middleware.MenuCatalog() {
		items = append(items, g.Map{
			"value": "MENU " + m.Path, "method": "MENU", "path": m.Path,
			"label": "菜单 · " + m.Title + "（" + m.Group + "）", "kind": "menu",
		})
	}
	for _, a := range middleware.ActionCatalog() {
		items = append(items, g.Map{
			"value": "ACTION " + a, "method": "ACTION", "path": a,
			"label": "按钮 · " + a, "kind": "action",
		})
	}
	r.Response.WriteJsonExit(g.Map{"code": 0, "data": items})
}

// RoleEffective 当前请求的有效角色与权限（AG1.4 实时校验的可视化）
func RoleEffective(r *ghttp.Request) {
	role := middleware.CurrentRole(r)
	def, ok := middleware.RoleDefFor(role)
	r.Response.WriteJsonExit(g.Map{"code": 0, "data": g.Map{
		"username": r.GetCtxVar("user").String(), "headerRole": r.Header.Get("X-User-Role"),
		"boundRole":     middleware.BoundRole(r.GetCtxVar("user").String()),
		"effectiveRole": role, "found": ok,
		"definition": def,
	}})
}

// applyFieldMask AG1.2 字段级权限：列表返回前把不可见字段替换为 ***
func applyFieldMask(r *ghttp.Request, rows []g.Map) {
	def, ok := middleware.RoleDefFor(middleware.CurrentRole(r))
	if !ok || len(def.FieldMask) == 0 {
		return
	}
	for _, row := range rows {
		for _, f := range def.FieldMask {
			if _, exists := row[f]; exists {
				row[f] = "***"
			}
		}
	}
}
