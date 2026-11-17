package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/cloudwego/eino-ext/components/model/openai"
	"github.com/cloudwego/eino/schema"
)

// AC1.4 结构化抽取：大模型按固定条款类型体系输出 JSON。
// 「合同专业知识」当前内嵌在提示词里（类型体系 + 要素清单）——
// 这是知识库的最小载体；后续量级上来再用 pgvector 建标准条款/审查要点向量库。
//
// 长合同策略（用户 11-14 拍板：正式合同十几~二十几页+附加条款，不能整篇塞给模型）：
// 按段落切块（约 3000 字/块）→ 逐块抽取 → 聚合去重。
// 进度 = (已完成块数 + 块内流式字数占比) / 总块数 —— 大合同按块走，小合同也能平滑走百分比。
// 后续向量化（知识库 ETL 的下一阶段）在此流水线上加 stage 即可。

type ExtractedClause struct {
	Type    string `json:"type"`    // 主体 / 权利义务 / 金额 / 日期 / 违约 / 保密 / 争议解决 / 其他
	Summary string `json:"summary"` // 条款要点
	Quote   string `json:"quote"`   // 原文片段（逐字引用，AC1.5 引用绑定的 MVP 形态）
	Risk    string `json:"risk"`    // high / medium / low / none
}

// ChunkProgress 分块进度回调：doneChunks 已完成块数，totalChunks 总块数，
// chunkFraction 当前块内流式进度 0~1，msg 可读说明
type ChunkProgress func(doneChunks, totalChunks int, chunkFraction float64, msg string)

const chunkRunes = 3000 // 每块约 3000 字

// SplitChunks 按段落切块：单段超长时整段成块，不截断句子
func SplitChunks(text string) []string {
	paras := strings.Split(text, "\n")
	var chunks []string
	var cur strings.Builder
	for _, p := range paras {
		if cur.Len()+len(p)+1 > chunkRunes && cur.Len() > 0 {
			chunks = append(chunks, strings.TrimSpace(cur.String()))
			cur.Reset()
		}
		cur.WriteString(p)
		cur.WriteString("\n")
	}
	if strings.TrimSpace(cur.String()) != "" {
		chunks = append(chunks, strings.TrimSpace(cur.String()))
	}
	if len(chunks) == 0 {
		chunks = []string{text}
	}
	return chunks
}

const extractSystemPrompt = `你是合同条款抽取引擎。从用户提供的合同文本片段中抽取关键条款，只输出 JSON，不要输出任何其他文字。
输出格式：
{"clauses":[{"type":"<类型>","summary":"<条款要点，中文一句话>","quote":"<原文片段，逐字引用不超过100字>","risk":"<high|medium|low|none>"}]}
条款 type 只能取：主体、权利义务、金额、日期、违约、保密、争议解决、其他。
必须覆盖的要素（文本中存在才输出）：合同主体（甲乙方）、合同价款/金额、履行期限/日期、违约责任、保密条款、争议解决方式。
quote 必须逐字来自原文，不得改写。文本片段中没有的类别不要编造。注意：这只是合同的片段之一，只抽取本片段中实际出现的内容。`

// AC1.8 抽取模板：按合同类型给抽取重点（影响抽取字段侧重，非必选）
var templateFocus = map[string]string{
	"purchase": "补充重点：本合同为采购类合同，额外关注——标的规格与验收标准、付款节点与方式、交付与安装义务、质保与售后条款。",
	"sales":    "补充重点：本合同为销售类合同，额外关注——交付与回款节点、质保责任、违约与赔偿上限、所有权转移。",
	"lease":    "补充重点：本合同为租赁类合同，额外关注——租期与续约、租金与递增机制、押金与退还条件、维修责任划分。",
}

func templatePrompt(template string) string {
	if focus, ok := templateFocus[template]; ok {
		return "\n" + focus
	}
	return ""
}

// ExtractClauses 分块流式抽取，聚合全部条款；template 为 AC1.8 抽取模板（general/purchase/sales/lease）
func ExtractClauses(ctx context.Context, contractText string, template string, onProgress ChunkProgress) ([]ExtractedClause, error) {
	if onProgress == nil {
		onProgress = func(int, int, float64, string) {}
	}
	apiKey, baseUrl, model := resolveLLMConfig()
	if apiKey == "" {
		return nil, fmt.Errorf("未配置大模型密钥：请在「模型配置」页为 llm 类别配置 API Key")
	}
	chatModel, err := openaiNewChatModel(ctx, baseUrl, model, apiKey)
	if err != nil {
		return nil, fmt.Errorf("init chat model: %w", err)
	}

	chunks := SplitChunks(contractText)
	total := len(chunks)
	var all []ExtractedClause
	seen := map[string]bool{} // 按 (type|summary) 简单去重：相邻块可能重复抽取边界条款

	for i, chunk := range chunks {
		sysPrompt := extractSystemPrompt + templatePrompt(template)
		clauses, err := extractOneChunk(ctx, chatModel, sysPrompt, chunk, func(frac float64) {
			onProgress(i, total, frac, fmt.Sprintf("AI 抽取中：第 %d/%d 块", i+1, total))
		})
		if err != nil {
			return nil, fmt.Errorf("第 %d/%d 块抽取失败: %w", i+1, total, err)
		}
		for _, c := range clauses {
			key := c.Type + "|" + c.Summary
			if seen[key] {
				continue
			}
			seen[key] = true
			all = append(all, c)
		}
		onProgress(i+1, total, 1.0, fmt.Sprintf("AI 抽取中：已完成 %d/%d 块", i+1, total))
	}
	if len(all) == 0 {
		return nil, fmt.Errorf("模型未抽取到任何条款")
	}
	return all, nil
}

// extractOneChunk 单块抽取：先流式（进度平滑），streamDeadline 内未完成或出错则回退非流式。
// 背景：Agnes 流式响应实测偏慢（单块 ~90s），非流式 ~25s；回退保证任务一定完成。
func extractOneChunk(ctx context.Context, chatModel *openai.ChatModel, systemPrompt string, chunk string, onFrac func(float64)) ([]ExtractedClause, error) {
	messages := []*schema.Message{
		{Role: schema.System, Content: systemPrompt},
		{Role: schema.User, Content: chunk},
	}

	const streamDeadline = 45 * time.Second
	sctx, cancel := context.WithTimeout(ctx, streamDeadline)
	stream, err := chatModel.Stream(sctx, messages)
	if err == nil {
		estOut := len(chunk) * 12 / 10 // 输出长度预估（含 JSON 结构开销）
		if estOut < 800 {
			estOut = 800
		}
		var sb strings.Builder
		received := 0
		for {
			msg, rerr := stream.Recv()
			if rerr == io.EOF {
				if out := strings.TrimSpace(sb.String()); out != "" {
					cancel()
					return parseClausesJSON(out)
				}
				break // 流式空输出 → 走回退
			}
			if rerr != nil {
				break // 超时/网络错误 → 走回退
			}
			sb.WriteString(msg.Content)
			received += len(msg.Content)
			if estOut > 0 && onFrac != nil {
				frac := float64(received) / float64(estOut)
				if frac > 0.98 {
					frac = 0.98 // 流式未完不报满
				}
				onFrac(frac)
			}
		}
	}
	cancel() // 释放流式定时器；回退用原 ctx

	// ---- 回退：非流式一次性生成（含 429 退避重试） ----
	content, err := generateWithRetry(ctx, chatModel, messages)
	if err != nil {
		return nil, fmt.Errorf("generate: %w", err)
	}
	if onFrac != nil {
		onFrac(0.99)
	}
	return parseClausesJSON(content)
}

// parseClausesJSON 容错解析模型输出（容忍 ```json 围栏与前后杂文字）
func parseClausesJSON(s string) ([]ExtractedClause, error) {
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
		Clauses []ExtractedClause `json:"clauses"`
	}
	if err := json.Unmarshal([]byte(s[start:end+1]), &out); err != nil {
		return nil, fmt.Errorf("解析抽取结果 JSON 失败: %w", err)
	}
	return out.Clauses, nil
}
