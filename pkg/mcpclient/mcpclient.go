package mcpclient

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/cloudwego/eino/schema"
	"github.com/gogf/gf/v2/frame/g"
	mcpclient "github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/client/transport"
	mcp "github.com/mark3labs/mcp-go/mcp"

	"vouchchain/pkg/agent"
	"vouchchain/pkg/cryptoutil"
)

// MCP Client：VouchChain 作为客户端去连接外部 MCP server，
// 把外部 server 暴露的工具动态注册进 AI 对话的统一工具表（agent.ChatTools）。
//
// 设计：
//   - 数据源：mcp_server 表（endpoint/token/extra_headers/enabled），token 存 enc:v1: 密文
//   - Manager 懒加载：启动时与配置变更时按 enabled=TRUE 行连接并拉取 tools/list
//   - 每个外部工具包装成 agent.ChatTool，名字加前缀 ext_<serverId>_<toolName>（避免与内置工具冲突）
//   - 工具执行透传到外部 tools/call，结果转文本（TextContent 拼接）
//   - 连接缓存：同一 server 只连一次，失败记 last_status/last_error 供页面展示
//
// 鉴权：Bearer token 注入 Authorization 头；extra_headers 原样注入（如 X-Org）。

const toolPrefix = "ext_" // 外部工具名前缀：ext_<serverId>_<toolName>

// Conn 一个外部 MCP server 的客户端封装（懒连接 + 工具缓存）
type Conn struct {
	ID        int64
	Name      string
	Endpoint  string
	Token     string // 已解密明文
	ExtraHdrs map[string]string

	mu     sync.Mutex
	client *mcpclient.Client
	tools  []agent.ChatTool // 转好的 ChatTool（含前缀名），nil=未成功加载
	raw    []mcp.Tool       // 原始工具（供页面展示）
	err    error
}

// Manager 全局管理器：并发安全，持有所有已启用外部连接的缓存
type Manager struct {
	mu    sync.RWMutex
	conns map[int64]*Conn
}

var mgr = &Manager{conns: map[int64]*Conn{}}

// ---------- 连接与拉取 ----------

// connectRaw 建立连接并握手（不缓存，供 Test 用）
func connectRaw(ctx context.Context, endpoint, token string, extraHdrs map[string]string) (*mcpclient.Client, error) {
	headers := map[string]string{}
	if token != "" {
		headers["Authorization"] = "Bearer " + token
	}
	for k, v := range extraHdrs {
		headers[k] = v
	}
	opts := []transport.StreamableHTTPCOption{
		transport.WithHTTPHeaders(headers),
		transport.WithHTTPTimeout(15 * time.Second),
	}
	c, err := mcpclient.NewStreamableHttpClient(strings.TrimRight(endpoint, "/"), opts...)
	if err != nil {
		return nil, fmt.Errorf("创建客户端失败: %w", err)
	}
	if err := c.Start(ctx); err != nil {
		return nil, fmt.Errorf("启动连接失败: %w", err)
	}
	if _, err := c.Initialize(ctx, mcp.InitializeRequest{
		Params: mcp.InitializeParams{
			ProtocolVersion: mcp.LATEST_PROTOCOL_VERSION,
			Capabilities:    mcp.ClientCapabilities{},
			ClientInfo: mcp.Implementation{
				Name:    "vouchchain-mcp-client",
				Version: "1.0.0",
			},
		},
	}); err != nil {
		return nil, fmt.Errorf("MCP 握手失败: %w", err)
	}
	return c, nil
}

// listRawTools 连接后拉取工具列表（含分页聚合）
func listRawTools(ctx context.Context, c *mcpclient.Client) ([]mcp.Tool, error) {
	var all []mcp.Tool
	var cursor mcp.Cursor
	for {
		req := mcp.ListToolsRequest{}
		if cursor != "" {
			req.Params.Cursor = cursor
		}
		res, err := c.ListTools(ctx, req)
		if err != nil {
			return nil, fmt.Errorf("tools/list 失败: %w", err)
		}
		all = append(all, res.Tools...)
		if res.NextCursor == "" {
			break
		}
		cursor = res.NextCursor
	}
	return all, nil
}

// callTool 调用外部工具（透传），返回文本结果
func callTool(ctx context.Context, c *mcpclient.Client, name string, args map[string]any) (string, error) {
	res, err := c.CallTool(ctx, mcp.CallToolRequest{
		Params: mcp.CallToolParams{Name: name, Arguments: args},
	})
	if err != nil {
		return "", fmt.Errorf("tools/call 失败: %w", err)
	}
	if res.IsError {
		return "", fmt.Errorf("工具返回错误: %s", contentText(res))
	}
	return contentText(res), nil
}

func contentText(res *mcp.CallToolResult) string {
	if res == nil {
		return ""
	}
	parts := make([]string, 0, len(res.Content))
	for _, c := range res.Content {
		if tc, ok := c.(*mcp.TextContent); ok {
			parts = append(parts, tc.Text)
		}
	}
	return strings.TrimSpace(strings.Join(parts, "\n"))
}

// ---------- 工具转换 ----------

// toChatTool 把外部 mcp.Tool 转成 agent.ChatTool（名字加前缀，Fn 透传 call）
func toChatTool(serverID int64, serverName string, t mcp.Tool) agent.ChatTool {
	params := map[string]*schema.ParameterInfo{}
	// ToolInputSchema.Properties 是 map[string]any（JSON Schema 属性对象），
	// 每项含 type（string|[]string）与 description（string）
	for k, raw := range t.InputSchema.Properties {
		p := map[string]any{}
		if m, ok := raw.(map[string]any); ok {
			p = m
		}
		params[k] = &schema.ParameterInfo{
			Type:     mapType(schemaTypeStr(p["type"])),
			Desc:     fmt.Sprintf("%v", p["description"]),
			Required: false,
		}
	}
	for _, r := range t.InputSchema.Required {
		if p, ok := params[r]; ok {
			p.Required = true
		}
	}
	fullName := fmt.Sprintf("%s%d_%s", toolPrefix, serverID, t.Name)
	return agent.ChatTool{
		Name:   fullName,
		Desc:   t.Description,
		Params: params,
		Fn: func(ctx context.Context, args map[string]any, actor agent.ChatActor) (string, error) {
			conn := mgr.Get(serverID)
			if conn == nil || conn.Client() == nil {
				return "", fmt.Errorf("外部 MCP 连接 %s(%d) 未就绪", serverName, serverID)
			}
			return callTool(ctx, conn.Client(), t.Name, args)
		},
	}
}

// schemaTypeStr 解析 JSON Schema type（string 或 []any），取首个
func schemaTypeStr(v any) string {
	switch tv := v.(type) {
	case string:
		return tv
	case []any:
		if len(tv) > 0 {
			if s, ok := tv[0].(string); ok {
				return s
			}
		}
	}
	return "string"
}

func mapType(t string) schema.DataType {
	switch strings.ToLower(t) {
	case "number":
		return schema.Number
	case "integer":
		return schema.Integer
	case "boolean":
		return schema.Boolean
	case "object":
		return schema.Object
	case "array":
		return schema.Array
	default:
		return schema.String
	}
}

// ---------- Manager 方法 ----------

// Get 取指定 server 的连接（nil=不存在）
func (m *Manager) Get(id int64) *Conn {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.conns[id]
}

// Client 返回底层客户端（未连接/失败返回 nil）
func (c *Conn) Client() *mcpclient.Client {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.client
}

// RawTools 返回原始工具列表（页面展示用）
func (c *Conn) RawTools() []mcp.Tool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.raw
}

// ChatTools 返回转好的 ChatTool 列表
func (c *Conn) ChatTools() []agent.ChatTool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.tools
}

// Status 返回连接状态摘要
func (c *Conn) Status() (ok bool, toolCount int, errMsg string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.err != nil {
		return false, 0, c.err.Error()
	}
	return c.client != nil, len(c.tools), ""
}

// load 连接并拉取工具（供 Manager.Refresh 与单条刷新调用）
func (m *Manager) load(ctx context.Context, c *Conn) {
	c.mu.Lock()
	defer c.mu.Unlock()
	// 先释放旧连接
	if c.client != nil {
		_ = c.client.Close()
		c.client = nil
	}
	c.tools = nil
	c.raw = nil
	c.err = nil

	client, err := connectRaw(ctx, c.Endpoint, c.Token, c.ExtraHdrs)
	if err != nil {
		c.err = err
		return
	}
	raw, err := listRawTools(ctx, client)
	if err != nil {
		c.err = err
		_ = client.Close()
		return
	}
	c.client = client
	c.raw = raw
	c.tools = make([]agent.ChatTool, 0, len(raw))
	for _, t := range raw {
		c.tools = append(c.tools, toChatTool(c.ID, c.Name, t))
	}
}

// Refresh 重载全部 enabled 连接（启动时 / 配置变更后调用；不阻塞失败）
func (m *Manager) Refresh(ctx context.Context) {
	rows, err := g.DB().Model("mcp_server").Where("enabled", true).All()
	if err != nil {
		g.Log().Warningf(ctx, "[mcpclient] 查询 mcp_server 失败: %v", err)
		return
	}
	newConns := map[int64]*Conn{}
	for _, r := range rows {
		id := r["id"].Int64()
		tokenPlain, _ := decryptToken(r["token"].String())
		extra := map[string]string{}
		_ = json.Unmarshal([]byte(r["extra_headers"].String()), &extra)
		c := &Conn{
			ID:        id,
			Name:      r["name"].String(),
			Endpoint:  r["endpoint"].String(),
			Token:     tokenPlain,
			ExtraHdrs: extra,
		}
		// 异步加载，避免阻塞启动
		go m.load(ctx, c)
		newConns[id] = c
	}
	m.mu.Lock()
	m.conns = newConns
	m.mu.Unlock()
}

// RefreshOne 单条刷新（增/改/启停后调用）
func (m *Manager) RefreshOne(ctx context.Context, id int64) {
	row, err := g.DB().Model("mcp_server").Where("id", id).One()
	if err != nil || row.IsEmpty() {
		return
	}
	enabled := row["enabled"].Bool()
	m.mu.Lock()
	old, existed := m.conns[id]
	if !enabled {
		delete(m.conns, id)
		m.mu.Unlock()
		if existed && old != nil {
			if c := old.Client(); c != nil {
				_ = c.Close()
			}
		}
		return
	}
	m.mu.Unlock()
	if existed && old != nil {
		m.load(ctx, old)
		return
	}
	tokenPlain, _ := decryptToken(row["token"].String())
	extra := map[string]string{}
	_ = json.Unmarshal([]byte(row["extra_headers"].String()), &extra)
	c := &Conn{
		ID:        id,
		Name:      row["name"].String(),
		Endpoint:  row["endpoint"].String(),
		Token:     tokenPlain,
		ExtraHdrs: extra,
	}
	go m.load(ctx, c)
	m.mu.Lock()
	m.conns[id] = c
	m.mu.Unlock()
}

// Remove 移除一条连接（删除配置后调用）
func (m *Manager) Remove(id int64) {
	m.mu.Lock()
	old, existed := m.conns[id]
	delete(m.conns, id)
	m.mu.Unlock()
	if existed && old != nil {
		if c := old.Client(); c != nil {
			_ = c.Close()
		}
	}
}

// AllChatTools 汇总所有已连接外部工具（供 agent.ChatTools 追加）
func (m *Manager) AllChatTools() []agent.ChatTool {
	m.mu.RLock()
	conns := make([]*Conn, 0, len(m.conns))
	for _, c := range m.conns {
		conns = append(conns, c)
	}
	m.mu.RUnlock()
	out := []agent.ChatTool{}
	for _, c := range conns {
		out = append(out, c.ChatTools()...)
	}
	return out
}

// ToolCount 返回当前已连接外部工具总数
func (m *Manager) ToolCount() int {
	return len(m.AllChatTools())
}

// ---------- 包级入口 ----------

// Refresh 包级入口：重载全部（启动时调用）
func Refresh(ctx context.Context) { mgr.Refresh(ctx) }

// RefreshOne 包级入口：单条刷新
func RefreshOne(ctx context.Context, id int64) { mgr.RefreshOne(ctx, id) }

// Remove 包级入口：移除
func Remove(id int64) { mgr.Remove(id) }

// AllChatTools 包级入口：汇总全部外部工具
func AllChatTools() []agent.ChatTool { return mgr.AllChatTools() }

// ToolCount 包级入口
func ToolCount() int { return mgr.ToolCount() }

// Get 包级入口
func Get(id int64) *Conn { return mgr.Get(id) }

// Test 连通性测试（不落库）：连接 + 握手 + 拉取工具，返回工具数
func Test(ctx context.Context, endpoint, token string, extraHdrs map[string]string) (int, error) {
	c, err := connectRaw(ctx, endpoint, token, extraHdrs)
	if err != nil {
		return 0, err
	}
	defer c.Close()
	raw, err := listRawTools(ctx, c)
	if err != nil {
		return 0, err
	}
	return len(raw), nil
}

// ---------- token 解密 ----------

// decryptToken 解密 mcp_server.token（enc:v1: 密文）；明文透传
func decryptToken(stored string) (string, error) {
	if stored == "" {
		return "", nil
	}
	if strings.HasPrefix(stored, "enc:v1:") {
		return cryptoutil.Decrypt(stored)
	}
	return stored, nil
}
