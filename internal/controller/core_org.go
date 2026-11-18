package controller

import (
	"encoding/csv"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/net/ghttp"
	"vouchchain/internal/middleware"
	"vouchchain/pkg/isolate"
	"vouchchain/pkg/orgcore"
	"vouchchain/pkg/schedcore"
)

// ---------- Core 共用内核：K3 多组织法人主体（AK3.1–AK3.4） ----------
// AK3.4 口径：给业务表打底层归属标记；默认上下文 0 = 全部主体（历史数据可见），
// 切换到具体主体后按该主体严格过滤。这里是「归属标记 + 基础过滤」，
// 完整的多租户强隔离属于产品② AG5，不在本轮范围。

// scopeEntity 取当前主体上下文：优先无状态请求头，其次用户已保存的切换结果
func scopeEntity(r *ghttp.Request) int64 {
	if v := r.Header.Get("X-Legal-Entity-Id"); v != "" {
		var id int64
		if _, err := fmt.Sscan(v, &id); err == nil && id >= 0 {
			return id
		}
	}
	username := r.GetCtxVar("user").String()
	if username == "" {
		return 0
	}
	row, err := g.DB().Model("user_context").Where("username", username).One()
	if err != nil || row.IsEmpty() {
		return 0
	}
	return row["legal_entity_id"].Int64()
}

// nodeIndex 读取任意父子表的 id/parent_id/name，构造递归索引（法人主体表与组织节点表共用）。
func nodeIndex(table string) (*orgcore.Tree, map[int64]string) {
	rows, err := g.DB().Model(table).Fields("id, parent_id, name").All()
	parents := map[int64]int64{}
	names := map[int64]string{}
	if err == nil {
		for _, row := range rows {
			id := row["id"].Int64()
			parents[id] = row["parent_id"].Int64()
			names[id] = row["name"].String()
		}
	}
	return orgcore.New(parents), names
}

func entityTree() (*orgcore.Tree, map[int64]string)   { return nodeIndex("legal_entity") }
func orgTreeIndex() (*orgcore.Tree, map[int64]string) { return nodeIndex("org_node") }

// includeSubtree 顶栏「含下级」开关：优先无状态请求头，其次用户已保存的上下文。
func includeSubtree(r *ghttp.Request) bool {
	if v := r.Header.Get("X-Include-Sub-Entities"); v != "" {
		return v == "1" || strings.EqualFold(v, "true")
	}
	username := r.GetCtxVar("user").String()
	if username == "" {
		return false
	}
	row, err := g.DB().Model("user_context").Where("username", username).One()
	if err != nil || row.IsEmpty() {
		return false
	}
	return row["include_subtree"].Bool()
}

// entityScopeIds 当前主体可见范围：默认只有自己；开关打开则按父子树递归展开所有下级主体。
func entityScopeIds(r *ghttp.Request, entityId int64) []int64 {
	if entityId <= 0 {
		return nil
	}
	if !includeSubtree(r) {
		return []int64{entityId}
	}
	tr, _ := entityTree()
	return tr.Descendants(entityId)
}

// scopeFilter 业务列表统一过滤：AG5 强制隔离策略（压过请求头）+ K3 主体归属 + G1 数据权限范围。
// AG5 策略一旦启用即为强制 AND 条件：即便调用方伪造 X-Legal-Entity-Id 或选「全部主体」，
// 也只能看到策略允许范围与所选范围的交集，不会因切换上下文放大权限。
func scopeFilter(m *gdb.Model, r *ghttp.Request) *gdb.Model {
	entityId := scopeEntity(r)
	iso := middleware.IsolationFor(r)
	if iso.Enforced {
		if allowed, configured := iso.Allowed[isolate.DimEntity]; configured {
			ids := append([]int64{}, iso.EntityIDs...)
			if entityId > 0 {
				ids = intersectIds(ids, entityScopeIds(r, entityId))
			}
			if len(ids) == 0 {
				m = m.Where("1 = 0")
			} else {
				m = m.WhereIn("legal_entity_id", ids)
			}
			_ = allowed
		} else if entityId > 0 {
			m = m.WhereIn("legal_entity_id", entityScopeIds(r, entityId))
		}
		if _, configured := iso.Allowed[isolate.DimDept]; configured {
			if len(iso.DeptIDs) == 0 {
				m = m.Where("1 = 0")
			} else {
				m = m.WhereIn("dept_id", iso.DeptIDs)
			}
		}
		if values, configured := iso.Allowed[isolate.DimProject]; configured {
			if len(values) == 0 {
				m = m.Where("1 = 0")
			} else if !containsWildcard(values) {
				m = m.WhereIn("project_code", values)
			}
		}
	} else if entityId > 0 {
		m = m.WhereIn("legal_entity_id", entityScopeIds(r, entityId))
	}
	username := r.GetCtxVar("user").String()
	def, ok := middleware.RoleDefFor(middleware.CurrentRole(r))
	if !ok {
		return m
	}
	switch def.DataScope {
	case "entity":
		// 不允许「全部主体」越权：退回用户已保存的主体上下文
		if entityId <= 0 {
			row, err := g.DB().Model("user_context").Where("username", username).One()
			eid := int64(0)
			if err == nil && !row.IsEmpty() {
				eid = row["legal_entity_id"].Int64()
			}
			if eid > 0 {
				m = m.WhereIn("legal_entity_id", entityScopeIds(r, eid))
			} else {
				m = m.Where("owner", username)
			}
		}
	case "self":
		m = m.Where("owner", username)
	case "dept":
		members := deptMembers(username)
		if len(members) == 0 {
			m = m.Where("owner", username)
		} else {
			m = m.WhereIn("owner", members)
		}
	}
	return m
}

// intersectIds 取两个主体集合的交集（AG5 策略 ∩ 当前所选主体范围）
func intersectIds(a, b []int64) []int64 {
	set := map[int64]bool{}
	for _, id := range b {
		set[id] = true
	}
	out := []int64{}
	for _, id := range a {
		if set[id] {
			out = append(out, id)
		}
	}
	return out
}

func containsWildcard(values []string) bool {
	for _, v := range values {
		if v == "*" {
			return true
		}
	}
	return false
}

// deptMembers G1 dept 数据范围：用户挂载节点 + 这些节点的整棵子树里的所有成员。
// 旧实现只取「同一节点」的人，父组织看不到子组织的数据；这里按父子树递归补齐。
func deptMembers(username string) []string {
	if username == "" {
		return nil
	}
	nodes, err := g.DB().Model("org_member").Where("username", username).All()
	if err != nil || nodes.IsEmpty() {
		return nil
	}
	seeds := make([]int64, 0, len(nodes))
	for _, n := range nodes {
		seeds = append(seeds, n["node_id"].Int64())
	}
	tr, _ := orgTreeIndex()
	subtree := tr.Descendants(seeds...)
	if len(subtree) == 0 {
		return []string{username}
	}
	rows, err := g.DB().Model("org_member").WhereIn("node_id", subtree).Order("id asc").All()
	if err != nil {
		return nil
	}
	set := map[string]bool{username: true}
	out := []string{username}
	for _, row := range rows {
		u := row["username"].String()
		if u != "" && !set[u] {
			set[u] = true
			out = append(out, u)
		}
	}
	return out
}

// LegalEntityList AK3.1：既返回扁平清单（下拉框 / 编辑表单用），也返回按 parent_id 递归拼出的树（表格展开用）。
// 每个节点带 depth / path / 直接发票数 / 含下级主体累计数，父子层级在界面上可见。
func LegalEntityList(r *ghttp.Request) {
	rows, err := g.DB().Model("legal_entity").Order("id asc").All()
	if err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	tr, names := entityTree()
	// 一次分组统计代替逐行 Count（N+1）
	direct := map[int64]int{}
	if cntRows, cerr := g.DB().Model("invoice").
		Fields("legal_entity_id, COUNT(*) AS cnt").Group("legal_entity_id").All(); cerr == nil {
		for _, cr := range cntRows {
			direct[cr["legal_entity_id"].Int64()] = cr["cnt"].Int()
		}
	}
	list := make([]g.Map, 0, len(rows))
	byId := map[int64]g.Map{}
	for _, row := range rows {
		id := row["id"].Int64()
		desc := tr.Descendants(id)
		total := 0
		for _, d := range desc {
			total += direct[d]
		}
		item := g.Map{
			"id": id, "code": row["code"].String(), "name": row["name"].String(),
			"entityType": row["entity_type"].String(), "parentId": row["parent_id"].Int64(),
			"status": row["status"].String(), "invoiceCount": direct[id],
			"totalInvoiceCount": total, "depth": tr.Depth(id),
			"path":       orgcore.PathNames(tr.Ancestors(id), names, " / "),
			"childCount": len(tr.Children(id)), "descendantCount": len(desc) - 1,
			"createdAt": row["created_at"].String(),
		}
		byId[id] = item
		list = append(list, item)
	}
	// 拼树：只挂直接子节点；脏数据成环时不挂（作为根平铺），避免 JSON 序列化无限递归
	tree := make([]g.Map, 0)
	for _, item := range list {
		id, _ := item["id"].(int64)
		parentId, _ := item["parentId"].(int64)
		parent, exists := byId[parentId]
		cyclic := false
		for _, a := range tr.Ancestors(parentId) {
			if a == id {
				cyclic = true
				break
			}
		}
		if parentId > 0 && exists && !cyclic {
			children, _ := parent["children"].([]g.Map)
			parent["children"] = append(children, item)
			continue
		}
		tree = append(tree, item)
	}
	r.Response.WriteJsonExit(g.Map{"code": 0, "data": g.Map{"list": list, "tree": tree}})
}

func LegalEntityCreate(r *ghttp.Request) {
	var req struct {
		Code       string `json:"code"`
		Name       string `json:"name"`
		EntityType string `json:"entityType"`
		ParentId   int64  `json:"parentId"`
	}
	if err := r.Parse(&req); err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	req.Code = strings.TrimSpace(req.Code)
	if req.Code == "" || strings.TrimSpace(req.Name) == "" {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": "主体编码与名称必填"})
		return
	}
	switch req.EntityType {
	case "headquarters", "subsidiary", "branch":
	case "":
		req.EntityType = "subsidiary"
	default:
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": "entityType 只能是 headquarters / subsidiary / branch"})
		return
	}
	if exist, _ := g.DB().Model("legal_entity").Where("code", req.Code).One(); !exist.IsEmpty() {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": "主体编码已存在：" + req.Code})
		return
	}
	if req.ParentId > 0 {
		if p, perr := g.DB().Model("legal_entity").Where("id", req.ParentId).One(); perr != nil || p.IsEmpty() {
			r.Response.WriteJsonExit(g.Map{"code": 1, "message": "上级主体不存在"})
			return
		}
	}
	res, err := g.DB().Model("legal_entity").Data(g.Map{
		"code": req.Code, "name": strings.TrimSpace(req.Name), "entity_type": req.EntityType,
		"parent_id": req.ParentId, "status": "active", "created_at": time.Now(), "updated_at": time.Now(),
	}).Insert()
	if err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	id, _ := res.LastInsertId()
	audit(r, "legal_entity.create", "legal_entity", id, g.Map{"code": req.Code, "name": req.Name})
	r.Response.WriteJsonExit(g.Map{"code": 0, "data": g.Map{"id": id}})
}

func LegalEntityUpdate(r *ghttp.Request) {
	id := r.Get("id").Int64()
	var req struct {
		Name       string `json:"name"`
		EntityType string `json:"entityType"`
		ParentId   *int64 `json:"parentId"` // 指针：只改状态时不能误把上级主体清成 0
		Status     string `json:"status"`
	}
	if err := r.Parse(&req); err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	if req.ParentId != nil {
		pid := *req.ParentId
		if pid == id {
			r.Response.WriteJsonExit(g.Map{"code": 1, "message": "上级主体不能是自己"})
			return
		}
		if pid > 0 {
			if p, perr := g.DB().Model("legal_entity").Where("id", pid).One(); perr != nil || p.IsEmpty() {
				r.Response.WriteJsonExit(g.Map{"code": 1, "message": "上级主体不存在"})
				return
			}
			// 递归防环：新上级不能是当前主体的子/孙主体，否则 parent_id 会闭合成环
			tr, _ := entityTree()
			if tr.IsDescendant(id, pid) {
				r.Response.WriteJsonExit(g.Map{"code": 1, "message": "不能把主体挂到自己的下级主体下（会形成环）"})
				return
			}
		}
	}
	data := g.Map{"updated_at": time.Now()}
	if strings.TrimSpace(req.Name) != "" {
		data["name"] = strings.TrimSpace(req.Name)
	}
	if req.EntityType != "" {
		data["entity_type"] = req.EntityType
	}
	if req.Status == "active" || req.Status == "disabled" {
		data["status"] = req.Status
	}
	if req.ParentId != nil {
		data["parent_id"] = *req.ParentId
	}
	if _, err := g.DB().Model("legal_entity").Data(data).Where("id", id).Update(); err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	audit(r, "legal_entity.update", "legal_entity", id, data)
	r.Response.WriteJsonExit(g.Map{"code": 0, "message": "ok"})
}

func LegalEntityDelete(r *ghttp.Request) {
	id := r.Get("id").Int64()
	row, err := g.DB().Model("legal_entity").Where("id", id).One()
	if err != nil || row.IsEmpty() {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": "主体不存在"})
		return
	}
	if row["code"].String() == "DEFAULT" {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": "默认主体不可删除"})
		return
	}
	for _, table := range []string{"invoice", "contract", "org_node", "master_data"} {
		cnt, _ := g.DB().Model(table).Where("legal_entity_id", id).Count()
		if cnt > 0 {
			r.Response.WriteJsonExit(g.Map{"code": 1, "message": fmt.Sprintf("该主体在 %s 下已有 %d 条数据，不可删除（可改为停用）", table, cnt)})
			return
		}
	}
	if _, err := g.DB().Model("legal_entity").Where("id", id).Delete(); err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	audit(r, "legal_entity.delete", "legal_entity", id, nil)
	r.Response.WriteJsonExit(g.Map{"code": 0, "message": "ok"})
}

// ContextGet AK3.2：返回当前上下文与可切换主体
func ContextGet(r *ghttp.Request) {
	id := scopeEntity(r)
	rows, _ := g.DB().Model("legal_entity").Where("status", "active").Order("id asc").All()
	tr, names := entityTree()
	name := "全部主体"
	list := make([]g.Map, 0, len(rows))
	for _, row := range rows {
		eid := row["id"].Int64()
		if eid == id {
			name = row["name"].String()
		}
		list = append(list, g.Map{
			"id": eid, "code": row["code"].String(), "name": row["name"].String(),
			"entityType": row["entity_type"].String(), "parentId": row["parent_id"].Int64(),
			"status": row["status"].String(), "depth": tr.Depth(eid),
			"path": orgcore.PathNames(tr.Ancestors(eid), names, " / "),
		})
	}
	r.Response.WriteJsonExit(g.Map{"code": 0, "data": g.Map{
		"legalEntityId": id, "legalEntityName": name, "entities": list,
		"includeSubtree": includeSubtree(r), "username": r.GetCtxVar("user").String(),
	}})
}

// ContextSwitch AK3.2：切换当前主体；0 = 回到全部主体
func ContextSwitch(r *ghttp.Request) {
	var req struct {
		LegalEntityId  int64 `json:"legalEntityId"`
		IncludeSubtree *bool `json:"includeSubtree"` // 可选：顶栏「含下级」开关，默认沿用旧值
	}
	if err := r.Parse(&req); err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	if req.LegalEntityId < 0 {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": "legalEntityId 非法"})
		return
	}
	if req.LegalEntityId > 0 {
		row, err := g.DB().Model("legal_entity").Where("id", req.LegalEntityId).One()
		if err != nil || row.IsEmpty() {
			r.Response.WriteJsonExit(g.Map{"code": 1, "message": "主体不存在"})
			return
		}
		if row["status"].String() != "active" {
			r.Response.WriteJsonExit(g.Map{"code": 1, "message": "该主体已停用"})
			return
		}
	}
	username := r.GetCtxVar("user").String()
	exist, _ := g.DB().Model("user_context").Where("username", username).One()
	data := g.Map{"legal_entity_id": req.LegalEntityId, "updated_at": time.Now()}
	if req.IncludeSubtree != nil {
		data["include_subtree"] = *req.IncludeSubtree
	}
	var err error
	if exist.IsEmpty() {
		data["username"] = username
		_, err = g.DB().Model("user_context").Data(data).Insert()
	} else {
		_, err = g.DB().Model("user_context").Data(data).Where("username", username).Update()
	}
	if err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	audit(r, "context.switch", "user_context", req.LegalEntityId, g.Map{
		"username": username, "legalEntityId": req.LegalEntityId, "includeSubtree": req.IncludeSubtree,
	})
	r.Response.WriteJsonExit(g.Map{"code": 0, "data": g.Map{
		"legalEntityId": req.LegalEntityId, "includeSubtree": includeSubtree(r),
	}})
}

// ---------- Core K2 组织树（AK2.1–AK2.5） ----------

func orgScope(r *ghttp.Request) int64 {
	if v := r.Get("legalEntityId").Int64(); v > 0 {
		return v
	}
	return scopeEntity(r)
}

// OrgNodeTree AK2.4：树视图 + 节点人数（直接挂载人数 / 含子树合计）
func OrgNodeTree(r *ghttp.Request) {
	entityId := orgScope(r)
	m := g.DB().Model("org_node")
	if entityId > 0 {
		m = m.Where("legal_entity_id", entityId)
	}
	rows, err := m.Order("sort_order asc, id asc").All()
	if err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	mm := g.DB().Model("org_member")
	if entityId > 0 {
		mm = mm.Where("legal_entity_id", entityId)
	}
	memberRows, _ := mm.All()
	direct := map[int64]int{}
	for _, row := range memberRows {
		direct[row["node_id"].Int64()]++
	}
	type node struct {
		Id               int64   `json:"id"`
		LegalEntityId    int64   `json:"legalEntityId"`
		ParentId         int64   `json:"parentId"`
		Code             string  `json:"code"`
		Name             string  `json:"name"`
		NodeType         string  `json:"nodeType"`
		SortOrder        int     `json:"sortOrder"`
		Depth            int     `json:"depth"`
		Path             string  `json:"path"`
		SubtreeNodes     int     `json:"subtreeNodes"`
		MemberCount      int     `json:"memberCount"`
		TotalMemberCount int     `json:"totalMemberCount"`
		Children         []*node `json:"children"`
	}
	// 递归口径统一走 orgcore：层级 / 路径 / 子树节点数 / 含子树人数用同一棵父子索引
	parents, nodeNames := map[int64]int64{}, map[int64]string{}
	for _, row := range rows {
		id := row["id"].Int64()
		parents[id] = row["parent_id"].Int64()
		nodeNames[id] = row["name"].String()
	}
	tr := orgcore.New(parents)
	byId := map[int64]*node{}
	list := make([]*node, 0, len(rows))
	for _, row := range rows {
		id := row["id"].Int64()
		n := &node{
			Id: id, LegalEntityId: row["legal_entity_id"].Int64(),
			ParentId: row["parent_id"].Int64(), Code: row["code"].String(),
			Name: row["name"].String(), NodeType: row["node_type"].String(),
			SortOrder: row["sort_order"].Int(), MemberCount: direct[id],
			Depth: tr.Depth(id), Path: orgcore.PathNames(tr.Ancestors(id), nodeNames, " / "),
			SubtreeNodes: len(tr.Descendants(id)) - 1, Children: []*node{},
		}
		byId[n.Id] = n
		list = append(list, n)
	}
	for _, n := range list {
		sum := 0
		for _, id := range tr.Descendants(n.Id) {
			sum += direct[id]
		}
		n.TotalMemberCount = sum
	}
	roots := make([]*node, 0)
	for _, n := range list {
		p, ok := byId[n.ParentId]
		// 脏数据父子互挂时不挂 children，否则 JSON 序列化会顺着环无限展开
		cyclic := false
		for _, a := range tr.Ancestors(n.ParentId) {
			if a == n.Id {
				cyclic = true
				break
			}
		}
		if ok && p != n && !cyclic {
			p.Children = append(p.Children, n)
		} else {
			roots = append(roots, n)
		}
	}
	r.Response.WriteJsonExit(g.Map{"code": 0, "data": g.Map{"legalEntityId": entityId, "tree": roots, "flat": list}})
}

// OrgNodeSubtree 递归展开某节点的整棵子树：节点清单（层级 / 路径）+ 去重成员清单。
// 前端「本节点 / 含子树」切换用它，也可直接核对 G1 dept 数据范围到底覆盖到哪些人。
func OrgNodeSubtree(r *ghttp.Request) {
	id := r.Get("id").Int64()
	if id <= 0 {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": "id required"})
		return
	}
	root, err := g.DB().Model("org_node").Where("id", id).One()
	if err != nil || root.IsEmpty() {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": "组织节点不存在"})
		return
	}
	rows, _ := g.DB().Model("org_node").Fields("id, parent_id, name, node_type").Order("id asc").All()
	parents, names, types := map[int64]int64{}, map[int64]string{}, map[int64]string{}
	for _, row := range rows {
		nid := row["id"].Int64()
		parents[nid] = row["parent_id"].Int64()
		names[nid] = row["name"].String()
		types[nid] = row["node_type"].String()
	}
	tr := orgcore.New(parents)
	ids := tr.Descendants(id)
	nodes := make([]g.Map, 0, len(ids))
	for _, nid := range ids {
		nodes = append(nodes, g.Map{
			"id": nid, "parentId": parents[nid], "name": names[nid],
			"nodeType": types[nid], "depth": tr.Depth(nid),
			"path": orgcore.PathNames(tr.Ancestors(nid), names, " / "),
		})
	}
	memberRows, _ := g.DB().Model("org_member").WhereIn("node_id", ids).Order("id asc").All()
	members := make([]g.Map, 0, len(memberRows))
	seen := map[string]bool{}
	for _, row := range memberRows {
		u := row["username"].String()
		if u == "" || seen[u] {
			continue
		}
		seen[u] = true
		nid := row["node_id"].Int64()
		members = append(members, g.Map{
			"id": row["id"].Int64(), "username": u, "nodeId": nid,
			"nodeName": names[nid], "isDirect": nid == id,
		})
	}
	r.Response.WriteJsonExit(g.Map{"code": 0, "data": g.Map{
		"nodeId": id, "nodeName": root["name"].String(),
		"path":      orgcore.PathNames(tr.Ancestors(id), names, " / "),
		"nodeCount": len(ids), "nodes": nodes, "members": members,
	}})
}

func orgData(r *ghttp.Request) (g.Map, error) {
	var req struct {
		LegalEntityId int64  `json:"legalEntityId"`
		ParentId      int64  `json:"parentId"`
		Code          string `json:"code"`
		Name          string `json:"name"`
		NodeType      string `json:"nodeType"`
		SortOrder     int    `json:"sortOrder"`
	}
	if err := r.Parse(&req); err != nil {
		return nil, err
	}
	if strings.TrimSpace(req.Name) == "" {
		return nil, fmt.Errorf("节点名称必填")
	}
	switch req.NodeType {
	case "dept", "position":
	case "":
		req.NodeType = "dept"
	default:
		return nil, fmt.Errorf("nodeType 只能是 dept / position")
	}
	if req.LegalEntityId <= 0 {
		req.LegalEntityId = scopeEntity(r)
	}
	if req.LegalEntityId > 0 {
		row, err := g.DB().Model("legal_entity").Where("id", req.LegalEntityId).One()
		if err != nil || row.IsEmpty() {
			return nil, fmt.Errorf("法人主体 #%d 不存在", req.LegalEntityId)
		}
	}
	return g.Map{
		"legal_entity_id": req.LegalEntityId, "parent_id": req.ParentId,
		"code": strings.TrimSpace(req.Code), "name": strings.TrimSpace(req.Name),
		"node_type": req.NodeType, "sort_order": req.SortOrder, "updated_at": time.Now(),
	}, nil
}

// OrgNodeCreate AK2.1 层级部门 / 岗位节点
func OrgNodeCreate(r *ghttp.Request) {
	data, err := orgData(r)
	if err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	if data["parent_id"].(int64) > 0 {
		p, err := g.DB().Model("org_node").Where("id", data["parent_id"]).One()
		if err != nil || p.IsEmpty() {
			r.Response.WriteJsonExit(g.Map{"code": 1, "message": "上级节点不存在"})
			return
		}
	}
	data["created_at"] = time.Now()
	res, err := g.DB().Model("org_node").Data(data).Insert()
	if err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	id, _ := res.LastInsertId()
	audit(r, "org_node.create", "org_node", id, data)
	r.Response.WriteJsonExit(g.Map{"code": 0, "data": g.Map{"id": id}})
}

// OrgNodeUpdate AK2.2 名称 / 上级 / 排序可编辑，并防止把节点挂到自己的子树下
func OrgNodeUpdate(r *ghttp.Request) {
	id := r.Get("id").Int64()
	old, err := g.DB().Model("org_node").Where("id", id).One()
	if err != nil || old.IsEmpty() {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": "节点不存在"})
		return
	}
	data, err := orgData(r)
	if err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	parentId := data["parent_id"].(int64)
	if parentId > 0 {
		if p, perr := g.DB().Model("org_node").Where("id", parentId).One(); perr != nil || p.IsEmpty() {
			r.Response.WriteJsonExit(g.Map{"code": 1, "message": "上级节点不存在"})
			return
		}
		tr, _ := orgTreeIndex()
		if tr.IsDescendant(id, parentId) {
			r.Response.WriteJsonExit(g.Map{"code": 1, "message": "不能把节点挂到自己或其子节点下（会形成环）"})
			return
		}
	}
	if _, err := g.DB().Model("org_node").Data(data).Where("id", id).Update(); err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	audit(r, "org_node.update", "org_node", id, g.Map{
		"before": g.Map{"name": old["name"].String(), "parentId": old["parent_id"].Int64(), "sortOrder": old["sort_order"].Int()},
		"after":  data,
	})
	r.Response.WriteJsonExit(g.Map{"code": 0, "message": "ok"})
}

func OrgNodeDelete(r *ghttp.Request) {
	id := r.Get("id").Int64()
	cnt, _ := g.DB().Model("org_node").Where("parent_id", id).Count()
	if cnt > 0 {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": fmt.Sprintf("该节点有 %d 个子节点，请先删除子节点", cnt)})
		return
	}
	mcnt, _ := g.DB().Model("org_member").Where("node_id", id).Count()
	if mcnt > 0 {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": fmt.Sprintf("该节点挂载了 %d 名人员，请先卸载", mcnt)})
		return
	}
	if _, err := g.DB().Model("org_node").Where("id", id).Delete(); err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	audit(r, "org_node.delete", "org_node", id, nil)
	r.Response.WriteJsonExit(g.Map{"code": 0, "message": "ok"})
}

func OrgMemberList(r *ghttp.Request) {
	m := g.DB().Model("org_member")
	if v := r.Get("nodeId").Int64(); v > 0 {
		m = m.Where("node_id", v)
	}
	rows, err := m.Order("id asc").Limit(500).All()
	if err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	r.Response.WriteJsonExit(g.Map{"code": 0, "data": rows})
}

// OrgMemberAttach AK2.3 人员挂载到部门 / 岗位节点
func OrgMemberAttach(r *ghttp.Request) {
	var req struct {
		NodeId   int64  `json:"nodeId"`
		Username string `json:"username"`
	}
	if err := r.Parse(&req); err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	req.Username = strings.TrimSpace(req.Username)
	if req.NodeId <= 0 || req.Username == "" {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": "nodeId 与 username 必填"})
		return
	}
	node, err := g.DB().Model("org_node").Where("id", req.NodeId).One()
	if err != nil || node.IsEmpty() {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": "组织节点不存在"})
		return
	}
	exist, _ := g.DB().Model("org_member").Where("node_id", req.NodeId).Where("username", req.Username).One()
	if !exist.IsEmpty() {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": "该人员已挂载在此节点"})
		return
	}
	res, err := g.DB().Model("org_member").Data(g.Map{
		"node_id": req.NodeId, "legal_entity_id": node["legal_entity_id"].Int64(),
		"username": req.Username, "created_at": time.Now(),
	}).Insert()
	if err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	id, _ := res.LastInsertId()
	audit(r, "org_member.attach", "org_member", id, g.Map{"nodeId": req.NodeId, "username": req.Username})
	r.Response.WriteJsonExit(g.Map{"code": 0, "data": g.Map{"id": id}})
}

func OrgMemberRemove(r *ghttp.Request) {
	id := r.Get("id").Int64()
	if _, err := g.DB().Model("org_member").Where("id", id).Delete(); err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	audit(r, "org_member.remove", "org_member", id, nil)
	r.Response.WriteJsonExit(g.Map{"code": 0, "message": "ok"})
}

// OrgNodeImport AK2.5 CSV / HR 批量导入（占位接口，首版可手工建；这里做真实 CSV 解析兜底）
// 列：code,name,node_type,parent_code,sort_order；上级按 code 在本文件内或库中已存在的节点解析。
func OrgNodeImport(r *ghttp.Request) {
	file := r.GetUploadFile("file")
	if file == nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": "缺少 CSV 文件（字段名 file）"})
		return
	}
	fh, err := file.Open()
	if err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	defer fh.Close()
	raw, err := io.ReadAll(fh)
	if err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	records, err := csv.NewReader(strings.NewReader(strings.TrimPrefix(string(raw), "\ufeff"))).ReadAll()
	if err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": "CSV 解析失败: " + err.Error()})
		return
	}
	nodes, parseErrs := schedcore.ParseOrgCsv(records)
	entityId := orgScope(r)
	// 两遍：先按文件中出现顺序建节点（允许父节点在同文件先出现），code→id 映射供上级解析
	sort.SliceStable(nodes, func(i, j int) bool { return nodes[i].ParentCode == "" && nodes[j].ParentCode != "" })
	byCode := map[string]int64{}
	existing, _ := g.DB().Model("org_node").All()
	for _, row := range existing {
		if c := row["code"].String(); c != "" {
			byCode[c] = row["id"].Int64()
		}
	}
	created := 0
	errors := append([]string{}, parseErrs...)
	for _, n := range nodes {
		parentId := int64(0)
		if n.ParentCode != "" {
			parentId = byCode[n.ParentCode]
			if parentId == 0 {
				errors = append(errors, fmt.Sprintf("节点 %s：上级编码 %s 不存在，已挂到根", n.Name, n.ParentCode))
			}
		}
		res, err := g.DB().Model("org_node").Data(g.Map{
			"legal_entity_id": entityId, "parent_id": parentId, "code": n.Code,
			"name": n.Name, "node_type": n.NodeType, "sort_order": n.SortOrder,
			"created_at": time.Now(), "updated_at": time.Now(),
		}).Insert()
		if err != nil {
			errors = append(errors, fmt.Sprintf("节点 %s：%v", n.Name, err))
			continue
		}
		id, _ := res.LastInsertId()
		if n.Code != "" {
			byCode[n.Code] = id
		}
		created++
	}
	audit(r, "org_node.import", "org_node", 0, g.Map{"created": created, "errors": len(errors), "file": file.Filename})
	r.Response.WriteJsonExit(g.Map{"code": 0, "data": g.Map{"created": created, "errors": errors}})
}
