package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/cloudwego/eino/schema"
)

// AC3.1 义务清单生成：从「复核完成」合同的已抽取条款中识别可执行义务。
// 只有明确时限或明确可执行动作的才输出；没有可靠日期 dueDate 留空，不编造（与抽取同一诚实口径）。
// AC3.3 可引用合同原文日期：dueDate 由模型从条款原文解析，用户可在义务看板修正。

type ExtractedObligation struct {
	Type        string `json:"type"`        // 付款 / 交付 / 报告 / 续约 / 其他
	Description string `json:"description"` // 谁+做什么+何时
	DueDate     string `json:"dueDate"`     // YYYY-MM-DD 或空
	SourceQuote string `json:"sourceQuote"` // 依据条款原文（≤80字）
}

const obligationSystemPrompt = `你是合同义务提取引擎。从用户提供的已抽取合同条款列表中，识别"需要某方在某个时限内完成的义务"（付款、交付、报告、续约、保密期管理等）。
只输出 JSON，不要输出任何其他文字：
{"obligations":[{"type":"<付款|交付|报告|续约|其他>","description":"<义务描述，格式：谁+做什么+何时>","dueDate":"<YYYY-MM-DD 或空字符串>","sourceQuote":"<依据条款原文，逐字引用不超过80字>"}]}
规则：
1. 只有条款中明确要求某方执行、且有可辨认动作的才输出；一般性声明（如保密义务存续）酌情输出并归入"其他"。
2. dueDate 只在条款原文中出现明确日期或相对期限（如"签订后30日内"，结合签订日期推算）时填写；推算不出就留空字符串，不得编造。
3. sourceQuote 逐字来自条款原文。`

// ExtractObligations 从条款聚合生成义务清单（单次 LLM 调用；条款量大时依赖已分块抽取的聚合结果）
func ExtractObligations(ctx context.Context, clauses []ExtractedClause) ([]ExtractedObligation, error) {
	apiKey, baseUrl, model := resolveLLMConfig()
	if apiKey == "" {
		return nil, fmt.Errorf("未配置大模型密钥：请在「模型配置」页为 llm 类别配置 API Key")
	}
	chatModel, err := openaiNewChatModel(ctx, baseUrl, model, apiKey)
	if err != nil {
		return nil, fmt.Errorf("init chat model: %w", err)
	}
	input, err := json.Marshal(map[string]interface{}{"clauses": clauses})
	if err != nil {
		return nil, err
	}
	content, err := generateWithRetry(ctx, chatModel, []*schema.Message{
		{Role: schema.System, Content: obligationSystemPrompt},
		{Role: schema.User, Content: string(input)},
	})
	if err != nil {
		return nil, fmt.Errorf("generate: %w", err)
	}
	return parseObligationsJSON(content)
}

func parseObligationsJSON(s string) ([]ExtractedObligation, error) {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "```json")
	s = strings.TrimPrefix(s, "```")
	s = strings.TrimSuffix(s, "```")
	start := strings.Index(s, "{")
	end := strings.LastIndex(s, "}")
	if start < 0 || end <= start {
		return nil, fmt.Errorf("模型输出中未找到 JSON")
	}
	var out struct {
		Obligations []ExtractedObligation `json:"obligations"`
	}
	if err := json.Unmarshal([]byte(s[start:end+1]), &out); err != nil {
		return nil, fmt.Errorf("解析义务清单 JSON 失败: %w", err)
	}
	if len(out.Obligations) == 0 {
		return nil, fmt.Errorf("模型未识别到可执行义务")
	}
	return out.Obligations, nil
}
