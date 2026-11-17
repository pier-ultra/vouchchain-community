package agent

import (
	"context"
	"strings"
	"testing"

	"github.com/cloudwego/eino/schema"
)

// 工具清单：名字唯一、参数描述可构造（WithTools 序列化不炸）
func TestChatToolsRegistry(t *testing.T) {
	tools := ChatTools()
	if len(tools) < 6 {
		t.Fatalf("工具数量异常: %d", len(tools))
	}
	seen := map[string]bool{}
	for _, tl := range toolInfos() {
		if tl.Name == "" || tl.Desc == "" {
			t.Fatalf("工具 %s 名字或描述为空", tl.Name)
		}
		if seen[tl.Name] {
			t.Fatalf("工具重名: %s", tl.Name)
		}
		seen[tl.Name] = true
	}
	for _, want := range []string{"query_contracts", "get_party_profile", "start_contract_approval", "act_approval"} {
		if !seen[want] {
			t.Fatalf("缺少必备工具: %s", want)
		}
	}
}

// 未知工具与坏 JSON 参数必须拦下（不 panic），错误回喂模型
func TestExecToolGuard(t *testing.T) {
	byName := map[string]ChatTool{}
	for _, tl := range ChatTools() {
		byName[tl.Name] = tl
	}
	trace, result := execTool(context.Background(), byName, schema.ToolCall{
		ID: "t1", Function: schema.FunctionCall{Name: "no_such_tool", Arguments: "{}"},
	}, ChatActor{Username: "u"})
	if trace.OK || !strings.Contains(result, "未知工具") {
		t.Fatalf("未知工具未拦截: %+v %s", trace, result)
	}
	trace, result = execTool(context.Background(), byName, schema.ToolCall{
		ID: "t2", Function: schema.FunctionCall{Name: "get_contract", Arguments: "not-json"},
	}, ChatActor{Username: "u"})
	if trace.OK || !strings.Contains(result, "JSON") {
		t.Fatalf("坏 JSON 未拦截: %+v %s", trace, result)
	}
}

// 必填参数缺失走工具自身校验（返回错误文本而非 panic）
func TestToolArgValidation(t *testing.T) {
	byName := map[string]ChatTool{}
	for _, tl := range ChatTools() {
		byName[tl.Name] = tl
	}
	_, result := execTool(context.Background(), byName, schema.ToolCall{
		ID: "t3", Function: schema.FunctionCall{Name: "act_approval", Arguments: `{"instanceId":1,"action":"删除"}`},
	}, ChatActor{Username: "u"})
	if strings.Contains(result, "异常") {
		t.Fatalf("非法 action 不应 panic: %s", result)
	}
}
