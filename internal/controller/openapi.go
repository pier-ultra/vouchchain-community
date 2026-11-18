package controller

import (
	"regexp"
	"sort"
	"strings"

	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/net/ghttp"
)

// OpenAPI 自生成 + Scalar 接口文档。
// 背景：GoFrame 内置 OpenAPI（SetOpenApiPath）只认 strict route（g.Meta），
// 而本项目路由是普通 func(r *ghttp.Request) handler（IsStrictRoute=false），
// 因此内置 OpenAPI 生成的 spec 为空。这里改为遍历 g.Server().GetRoutes()
// 自建 OpenAPI 3.0 spec（path/method/tag 级别），供 Scalar 展示。
// 参数 schema 受限于 legacy handler 无法静态推断，留待按需补充。

// 常见 HTTP 方法（OpenAPI 支持的 operation 键）
var openapiMethods = []string{"get", "post", "put", "delete", "patch", "head", "options"}

// camelRe 驼峰拆分：ApprovalInstance → Approval-Instance
var camelRe = regexp.MustCompile(`([a-z0-9])([A-Z])`)

// OpenApiSpec 返回自生成的 OpenAPI 3.0 文档（GET /openapi.json）
func OpenApiSpec(r *ghttp.Request) {
	paths := map[string]map[string]any{}
	for _, item := range g.Server().GetRoutes() {
		// 只保留真实 handler / object，跳过 middleware / hook
		if item.Type != ghttp.HandlerTypeHandler && item.Type != ghttp.HandlerTypeObject {
			continue
		}
		route := item.Route
		if route == "" || route == "/" {
			continue
		}
		method := strings.ToUpper(strings.TrimSpace(item.Method))
		methods := []string{}
		if method == "" || method == "ALL" {
			// ALL：展开为常见方法（如 /mcp）
			for _, m := range openapiMethods {
				methods = append(methods, strings.ToUpper(m))
			}
		} else {
			methods = append(methods, method)
		}
		// GoFrame 路由里的 :param 与 {param} 都转 OpenAPI 路径参数
		oapiPath := route
		if strings.Contains(oapiPath, ":") {
			oapiPath = regexp.MustCompile(`:([^/]+)`).ReplaceAllString(oapiPath, `{$1}`)
		}
		if _, ok := paths[oapiPath]; !ok {
			paths[oapiPath] = map[string]any{}
		}
		for _, m := range methods {
			// 每个方法单独构建：requestBody 取决于方法（POST/PUT/PATCH 才有）
			paths[oapiPath][strings.ToLower(m)] = buildOperation(item, oapiPath, m)
		}
	}

	spec := map[string]any{
		"openapi": "3.0.0",
		"info": map[string]any{
			"title":       "VouchChain API",
			"version":     "v0.2.0 社区版",
			"description": "合同与履约中枢 · 社区版接口文档（自生成，path/method 级别）",
		},
		"servers": []map[string]any{{"url": "/"}},
		"paths":   paths,
	}
	r.Response.WriteJson(spec)
}

// ScalarPage 返回 Scalar 交互式文档页面（GET /scalar）
func ScalarPage(r *ghttp.Request) {
	r.Response.Write(ScalarHTML)
}

// buildOperation 依据路由 item、OpenAPI 路径与方法构建单个 operation
func buildOperation(item ghttp.RouterItem, oapiPath, method string) map[string]any {
	handlerName := ""
	if item.Handler != nil {
		handlerName = item.Handler.Name
	}
	summary := summarize(handlerName)
	m := strings.ToLower(method)
	jsonObj := map[string]any{"type": "object"}
	op := map[string]any{
		"summary": summary,
		// operationId 加方法后缀：同一 handler 可能被多个方法引用，避免重复
		"operationId": operationId(handlerName) + "_" + m,
		"tags":        []string{tagOf(handlerName, oapiPath)},
		"responses": map[string]any{
			// 响应声明 JSON content：Scalar 据此渲染响应面板
			"200": map[string]any{
				"description": "OK",
				"content": map[string]any{
					"application/json": map[string]any{"schema": jsonObj},
				},
			},
		},
	}
	// 路径参数（{xxx}）→ in:path required
	if params := pathParams(oapiPath); len(params) > 0 {
		op["parameters"] = params
	}
	// 有请求体的方法：声明 JSON body。
	// 这是 Scalar 生成「Try it」请求体编辑器与发送按钮的前提——
	// 没有 requestBody 时页面上不会出现任何发送入口。
	if hasRequestBody(method) {
		op["requestBody"] = map[string]any{
			"required": true,
			"content": map[string]any{
				"application/json": map[string]any{"schema": jsonObj},
			},
		}
	}
	return op
}

// hasRequestBody 判断方法是否携带请求体
func hasRequestBody(method string) bool {
	switch strings.ToUpper(method) {
	case "POST", "PUT", "PATCH":
		return true
	}
	return false
}

// summarize 从 handler 全名提取可读摘要（最后一段函数名）
func summarize(handlerName string) string {
	idx := strings.LastIndex(handlerName, ".")
	if idx >= 0 && idx < len(handlerName)-1 {
		return handlerName[idx+1:]
	}
	return handlerName
}

// operationId 生成唯一 operationId
func operationId(handlerName string) string {
	if handlerName == "" {
		return ""
	}
	parts := strings.Split(handlerName, ".")
	return parts[len(parts)-1]
}

// tagOf 推断模块 tag：优先 handler 名去动词后缀转驼峰连字符，其次路径第一段
func tagOf(handlerName, oapiPath string) string {
	base := summarize(handlerName)
	// 去常见动词后缀（List/Create/Get/Update/Delete/Save/Detail/Act/...）
	for _, suffix := range []string{"List", "Create", "Get", "Update", "Delete", "Save", "Detail", "Act", "Add", "Cancel", "Approve", "Reject", "Export", "Import", "Poll", "Transfer", "Stamp", "Toggle", "Purge"} {
		if strings.HasSuffix(base, suffix) {
			base = strings.TrimSuffix(base, suffix)
			break
		}
	}
	// 匿名函数（如 main.main.func3.1）或纯数字兜底用路径
	if base == "" || isNumeric(base) {
		return pathTag(oapiPath)
	}
	return camelToKebab(base)
}

// camelToKebab 驼峰转连字符：ApprovalInstance → approval-instance
func camelToKebab(s string) string {
	return strings.ToLower(camelRe.ReplaceAllString(s, "$1-$2"))
}

// isNumeric 判断是否为纯数字（匿名函数名尾段）
func isNumeric(s string) bool {
	if s == "" {
		return false
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

// pathTag 从路径第一段推断 tag（如 /api/contracts → contracts）
func pathTag(oapiPath string) string {
	trimmed := strings.Trim(oapiPath, "/")
	if strings.Contains(trimmed, "/") {
		trimmed = strings.Split(trimmed, "/")[0]
	}
	if trimmed != "" {
		return trimmed
	}
	return "api"
}

// pathParams 从 OpenAPI 路径提取 {param} 列表
func pathParams(oapiPath string) []map[string]any {
	re := regexp.MustCompile(`\{([^}]+)\}`)
	matches := re.FindAllStringSubmatch(oapiPath, -1)
	if len(matches) == 0 {
		return nil
	}
	seen := map[string]bool{}
	out := []map[string]any{}
	for _, m := range matches {
		name := m[1]
		if seen[name] {
			continue
		}
		seen[name] = true
		out = append(out, map[string]any{
			"name":     name,
			"in":       "path",
			"required": true,
			"schema":   map[string]any{"type": "string"},
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i]["name"].(string) < out[j]["name"].(string) })
	return out
}

// ScalarHTML 内嵌 Scalar 页面（本地 bundle /vendor/scalar-standalone.js，不依赖 CDN），指向 /openapi.json
const ScalarHTML = `<!DOCTYPE html>
<html lang="zh-CN">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>VouchChain API 文档</title>
  <link rel="icon" href="data:,">
  <style>
    body { margin: 0; font-family: var(--default-font-family, ui-sans-serif, system-ui, sans-serif); }
    #api-reference { width: 100%; height: 100vh; }
  </style>
  <!-- Scalar API Reference（本地 bundle，不依赖 CDN；npm run build 后由 scripts/copy-scalar.mjs 复制到 web/dist/vendor/） -->
  <script src="/vendor/scalar-standalone.js"></script>
</head>
<body>
  <div id="api-reference"></div>
  <script>
    if (window.Scalar && window.Scalar.createApiReference) {
      // 1.78.x standalone 签名为双参数：createApiReference(el, configuration)
      window.Scalar.createApiReference('#api-reference', {
        url: '/openapi.json',
        theme: 'default',
        darkMode: false,
        metaData: {
          title: 'VouchChain API 文档',
          description: '合同与履约中枢 · 社区版'
        }
      });
    } else {
      document.getElementById('api-reference').innerHTML =
        '<div style="padding:24px;font-family:system-ui"><h3>Scalar 未加载</h3>' +
        '<p>本地 bundle 缺失：请在 web/ 目录执行 <code>npm install &amp;&amp; npm run build</code>，' +
        '使 <code>dist/vendor/scalar-standalone.js</code> 生成。</p></div>';
    }
  </script>
</body>
</html>`
