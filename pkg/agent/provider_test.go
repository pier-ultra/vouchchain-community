package agent

import (
	"context"
	"strings"
	"testing"
)

// 验证外部工具 provider 拼接机制（模拟 mcpclient 注册）
func TestExternalToolProviderConcat(t *testing.T) {
	builtin := ChatTools()
	if len(builtin) != 9 {
		t.Fatalf("内置工具应为 9，实际 %d", len(builtin))
	}
	SetExternalToolProvider(func() []ChatTool {
		return []ChatTool{{
			Name: "ext_1_query_contracts",
			Desc: "外部工具（自连测试）",
			Fn: func(ctx context.Context, args map[string]any, actor ChatActor) (string, error) {
				return "ext-ok", nil
			},
		}}
	})
	defer SetExternalToolProvider(nil)
	tools := ChatTools()
	found := false
	for _, x := range tools {
		if strings.HasPrefix(x.Name, "ext_") {
			found = true
		}
	}
	if !found {
		t.Fatalf("外部工具未拼接进 ChatTools")
	}
	if len(tools) != 10 {
		t.Fatalf("拼接后应为 10，实际 %d", len(tools))
	}
	t.Logf("拼接验证通过：内置 %d + 外部 %d = %d", 9, 1, len(tools))
}
