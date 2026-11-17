package mcpserver

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/cloudwego/eino/schema"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/net/ghttp"
	"github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"

	"vouchchain/pkg/agent"
)

// MCP 服务（对外暴露 AI 工具，让 Claude Desktop / Cursor 等外部 AI 客户端操作系统）：
//  - 传输：Streamable HTTP，挂在同进程 /mcp 端点（自托管分发不需要额外端口）
//  - 鉴权：MCP_TOKEN 环境变量（未配置一律 503，同 SCIM_TOKEN 套路；token 不落库不落文件）
//  - 身份：默认 mcp_agent，可由请求头 X-MCP-User 指定操作身份（审计 username 按此留痕）
//  - 工具：直接复用 pkg/agent.ChatTools()（网页对话与 MCP 调的是同一份实现）
//  - 依赖：mark3labs/mcp-go v1.5.0（国人开源，Apache-2.0，许可红线满足）

type ctxKey struct{}

// Setup 构建 MCP streamable HTTP handler；MCP_TOKEN 未配置返回 nil（调用方降级为 503）
func Setup() http.Handler {
	s := mcpserver.NewMCPServer("contract-hub", "1.0.0",
		mcpserver.WithToolCapabilities(false),
	)
	for _, t := range agent.ChatTools() {
		s.AddTool(toMCPTool(t), handlerFor(t))
	}
	return mcpserver.NewStreamableHTTPServer(s)
}

// toMCPTool 把 agent 工具的参数描述转成 mcp-go Tool（类型映射：string/integer+number/boolean）
func toMCPTool(t agent.ChatTool) mcp.Tool {
	opts := []mcp.ToolOption{mcp.WithDescription(t.Desc)}
	for name, p := range t.Params {
		switch p.Type {
		case schema.Integer, schema.Number:
			o := []mcp.PropertyOption{mcp.Description(p.Desc)}
			if p.Required {
				o = append(o, mcp.Required())
			}
			opts = append(opts, mcp.WithNumber(name, o...))
		case schema.Boolean:
			o := []mcp.PropertyOption{mcp.Description(p.Desc)}
			if p.Required {
				o = append(o, mcp.Required())
			}
			opts = append(opts, mcp.WithBoolean(name, o...))
		default:
			o := []mcp.PropertyOption{mcp.Description(p.Desc)}
			if p.Required {
				o = append(o, mcp.Required())
			}
			opts = append(opts, mcp.WithString(name, o...))
		}
	}
	return mcp.NewTool(t.Name, opts...)
}

// handlerFor 工具执行：参数直接透传（GetArguments 返回 map[string]any，与 agent 工具签名一致），
// 身份从请求 context 取（McpAuth 中间件注入），执行结果转 CallToolResult。
func handlerFor(t agent.ChatTool) mcpserver.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		actor := ActorFromCtx(ctx)
		result, err := t.Fn(ctx, request.GetArguments(), actor)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("工具执行失败: %v", err)), nil
		}
		return mcp.NewToolResultText(result), nil
	}
}

// McpAuth Bearer token 校验 + 操作身份注入（挂 /mcp 独立路由组，不走 RBAC）
func McpAuth(r *ghttp.Request) {
	token := os.Getenv("MCP_TOKEN")
	if token == "" {
		r.Response.WriteStatusExit(503, `{"error":"MCP 服务未启用：请设置 MCP_TOKEN 环境变量后重启"}`)
		return
	}
	auth := r.Header.Get("Authorization")
	if auth != "Bearer "+token {
		r.Response.WriteStatusExit(401, `{"error":"MCP token 无效"}`)
		return
	}
	user := r.Header.Get("X-MCP-User")
	if user == "" {
		user = "mcp_agent"
	}
	ctx := context.WithValue(r.Context(), ctxKey{}, agent.ChatActor{Username: user, Role: "mcp", Source: "mcp"})
	r.SetCtx(ctx)
	r.Middleware.Next()
}

// ActorFromCtx 从请求 context 取身份（未注入时兜底 mcp_agent）
func ActorFromCtx(ctx context.Context) agent.ChatActor {
	if a, ok := ctx.Value(ctxKey{}).(agent.ChatActor); ok && a.Username != "" {
		return a
	}
	return agent.ChatActor{Username: "mcp_agent", Role: "mcp", Source: "mcp"}
}

// Status MCP 服务启用状态（前端/运维自检用，不回显 token）
func Status() g.Map {
	token := os.Getenv("MCP_TOKEN")
	endpoint := ""
	if token != "" {
		base := strings.TrimRight(g.Cfg().MustGet(context.Background(), "server.address").String(), "/")
		if base == "" {
			base = ":8000"
		}
		endpoint = "http://<host>" + base + "/mcp"
	}
	return g.Map{
		"enabled":  token != "",
		"endpoint": endpoint,
		"tools":    len(agent.ChatTools()),
		"headers":  g.Map{"Authorization": "Bearer <MCP_TOKEN>", "X-MCP-User": "可选，操作身份（默认 mcp_agent）"},
	}
}
