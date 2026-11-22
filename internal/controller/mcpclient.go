package controller

import (
	"context"
	"encoding/json"
	"time"

	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/net/ghttp"

	"vouchchain/internal/entity"
	"vouchchain/pkg/cryptoutil"
	"vouchchain/pkg/mcpclient"
)

// ---------- MCP Client：外部 MCP server 连接管理 ----------
// CRUD + 连通性测试 + 工具列表 + 启停切换。
// 变更（增/改/删/启停）后触发 mcpclient.RefreshOne/Remove 动态刷新注册工具。

// McpServerList 全部外部 MCP 连接（token 脱敏，带当前连接状态）
func McpServerList(r *ghttp.Request) {
	list := make([]entity.McpServer, 0)
	if err := g.DB().Model("mcp_server").Order("id asc").Scan(&list); err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	for i := range list {
		list[i].Token = cryptoutil.Mask(list[i].Token)
		// 实时连接状态（懒加载后由 Manager 缓存）
		if conn := mcpclient.Get(list[i].Id); conn != nil {
			ok, n, em := conn.Status()
			if ok {
				list[i].ToolCount = n
				list[i].LastStatus = "ok"
				list[i].LastError = ""
			} else if em != "" {
				list[i].LastStatus = "error"
				list[i].LastError = em
			}
		} else if !list[i].Enabled {
			list[i].LastStatus = "idle"
		} else {
			list[i].LastStatus = "pending"
		}
	}
	r.Response.WriteJsonExit(g.Map{"code": 0, "data": list})
}

// McpServerCreate 新增外部 MCP 连接（token 加密入库）
func McpServerCreate(r *ghttp.Request) {
	var req entity.McpServer
	if err := r.Parse(&req); err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	if req.Endpoint == "" {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": "endpoint 必填"})
		return
	}
	token := req.Token
	if token != "" {
		enc, err := cryptoutil.Encrypt(token)
		if err != nil {
			r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
			return
		}
		token = enc
	}
	extra := req.ExtraHeaders
	if extra == "" {
		extra = "{}"
	}
	now := time.Now()
	res, err := g.DB().Model("mcp_server").Data(g.Map{
		"name": req.Name, "endpoint": req.Endpoint, "token": token,
		"extra_headers": extra, "description": req.Description,
		"enabled": req.Enabled, "created_at": now, "updated_at": now,
	}).Insert()
	if err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	id, _ := res.LastInsertId()
	// 触发连接刷新（异步，不阻塞返回）
	if req.Enabled {
		go mcpclient.RefreshOne(context.Background(), id)
	}
	r.Response.WriteJsonExit(g.Map{"code": 0, "message": "created", "data": g.Map{"id": id}})
}

// McpServerUpdate 更新外部 MCP 连接（token 空/脱敏值则不改）
func McpServerUpdate(r *ghttp.Request) {
	id := r.Get("id").Int64()
	if id <= 0 {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": "id required"})
		return
	}
	var req entity.McpServer
	if err := r.Parse(&req); err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	token := req.Token
	if token != "" && !hasTokenMask(token) {
		enc, err := cryptoutil.Encrypt(token)
		if err != nil {
			r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
			return
		}
		token = enc
	} else {
		token = "" // 不改 token
	}
	extra := req.ExtraHeaders
	if extra == "" {
		extra = "{}"
	}
	data := g.Map{
		"name": req.Name, "endpoint": req.Endpoint, "extra_headers": extra,
		"description": req.Description, "enabled": req.Enabled, "updated_at": time.Now(),
	}
	if token != "" {
		data["token"] = token
	}
	if _, err := g.DB().Model("mcp_server").Data(data).Where("id", id).Update(); err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	// 刷新连接
	go func() {
		if req.Enabled {
			mcpclient.RefreshOne(context.Background(), id)
		} else {
			mcpclient.Remove(id)
		}
	}()
	r.Response.WriteJsonExit(g.Map{"code": 0, "message": "updated"})
}

// McpServerDelete 删除连接
func McpServerDelete(r *ghttp.Request) {
	id := r.Get("id").Int64()
	if id <= 0 {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": "id required"})
		return
	}
	if _, err := g.DB().Model("mcp_server").Where("id", id).Delete(); err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	mcpclient.Remove(id)
	r.Response.WriteJsonExit(g.Map{"code": 0, "message": "deleted"})
}

// McpServerToggle 启停切换
func McpServerToggle(r *ghttp.Request) {
	id := r.Get("id").Int64()
	if id <= 0 {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": "id required"})
		return
	}
	enabled := r.Get("enabled").Bool()
	if _, err := g.DB().Model("mcp_server").Data(g.Map{
		"enabled": enabled, "updated_at": time.Now(),
	}).Where("id", id).Update(); err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	go func() {
		if enabled {
			mcpclient.RefreshOne(context.Background(), id)
		} else {
			mcpclient.Remove(id)
		}
	}()
	r.Response.WriteJsonExit(g.Map{"code": 0, "message": "ok"})
}

// McpServerTest 连通性测试（不落库）：连接 + 握手 + 拉取工具数
// 传 id 用库里配置（token 解密）；否则用表单草稿（token 明文）
func McpServerTest(r *ghttp.Request) {
	var req entity.McpServer
	if err := r.Parse(&req); err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	endpoint := req.Endpoint
	token := req.Token
	if req.Id > 0 && endpoint == "" {
		row, err := g.DB().Model("mcp_server").Where("id", req.Id).One()
		if err != nil || row.IsEmpty() {
			r.Response.WriteJsonExit(g.Map{"code": 1, "message": "配置不存在"})
			return
		}
		endpoint = row["endpoint"].String()
		plain, derr := cryptoutil.Decrypt(row["token"].String())
		if derr != nil {
			r.Response.WriteJsonExit(g.Map{"code": 1, "message": derr.Error()})
			return
		}
		token = plain
	}
	if endpoint == "" {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": "endpoint 必填"})
		return
	}
	extra := map[string]string{}
	_ = json.Unmarshal([]byte(req.ExtraHeaders), &extra)
	n, err := mcpclient.Test(r.Context(), endpoint, token, extra)
	if err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	r.Response.WriteJsonExit(g.Map{"code": 0, "data": g.Map{"tools": n}, "message": "连接成功"})
}

// McpServerTools 列出某连接拉取到的工具（供页面预览）
func McpServerTools(r *ghttp.Request) {
	id := r.Get("id").Int64()
	if id <= 0 {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": "id required"})
		return
	}
	conn := mcpclient.Get(id)
	if conn == nil {
		r.Response.WriteJsonExit(g.Map{"code": 0, "data": []g.Map{}, "message": "连接未就绪（可能已停用或尚未加载）"})
		return
	}
	raw := conn.RawTools()
	out := make([]g.Map, 0, len(raw))
	for _, t := range raw {
		out = append(out, g.Map{
			"name":        t.Name,
			"description": t.Description,
			"required":    t.InputSchema.Required,
		})
	}
	r.Response.WriteJsonExit(g.Map{"code": 0, "data": out})
}

// hasTokenMask 判断 token 是否为脱敏占位（前端不回传明文时）
func hasTokenMask(s string) bool {
	return s == "已加密存储" || len(s) <= 8 && (s == "••••••••" || s == "未配置")
}
