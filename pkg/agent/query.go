package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/cloudwego/eino/schema"
)

// AC5 自然语言查组合风险：MVP 用"全量条款+义务作上下文"的直接问答，
// 每条回答强制标注引用（合同文件名 + 原文片段），检索无结果时明确说不知道，不编造。
// 知识库阶段（pgvector 向量化）落地后，这里升级为向量检索召回 + 同一作答链路。

type QueryRef struct {
	FileId   int64  `json:"fileId"`
	Filename string `json:"filename"`
	Quote    string `json:"quote"`
}

type QueryAnswer struct {
	Answer string     `json:"answer"`
	Refs   []QueryRef `json:"refs"`
}

const querySystemPrompt = `你是合同组合风险问答引擎。用户的合同库条款与义务清单以 JSON 提供给您。
只输出 JSON，不要输出任何其他文字：
{"answer":"<中文回答，分条说明，明确指出结论与依据>","refs":[{"fileId":<合同文件编号>,"filename":"<文件名>","quote":"<支撑该结论的原文片段，逐字引用≤80字>"}]}
规则：
1. 回答必须基于给出的条款/义务数据，引用的 quote 必须逐字来自对应条款原文。
2. 数据不足以回答时，answer 明确写"合同库中未找到足够依据"，refs 给空数组；不得编造。
3. 涉及对比/汇总类问题（如历年报价对比），在 answer 中给出结构化对比表述。`

// AnswerQuery 自然语言查询（MVP：全量上下文直答 + 强制引用）
func AnswerQuery(ctx context.Context, question string, corpusJSON string) (*QueryAnswer, error) {
	apiKey, baseUrl, model := resolveLLMConfig()
	if apiKey == "" {
		return nil, fmt.Errorf("未配置大模型密钥：请在「模型配置」页为 llm 类别配置 API Key")
	}
	chatModel, err := openaiNewChatModel(ctx, baseUrl, model, apiKey)
	if err != nil {
		return nil, fmt.Errorf("init chat model: %w", err)
	}
	content, err := generateWithRetry(ctx, chatModel, []*schema.Message{
		{Role: schema.System, Content: querySystemPrompt},
		{Role: schema.User, Content: "【合同库数据】\n" + corpusJSON + "\n\n【问题】\n" + question},
	})
	if err != nil {
		return nil, fmt.Errorf("generate: %w", err)
	}
	return parseQueryJSON(content)
}

func parseQueryJSON(s string) (*QueryAnswer, error) {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "```json")
	s = strings.TrimPrefix(s, "```")
	s = strings.TrimSuffix(s, "```")
	start := strings.Index(s, "{")
	end := strings.LastIndex(s, "}")
	if start < 0 || end <= start {
		return nil, fmt.Errorf("模型输出中未找到 JSON")
	}
	var out QueryAnswer
	if err := json.Unmarshal([]byte(s[start:end+1]), &out); err != nil {
		return nil, fmt.Errorf("解析问答结果失败: %w", err)
	}
	return &out, nil
}
