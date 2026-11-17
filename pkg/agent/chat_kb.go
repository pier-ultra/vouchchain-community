package agent

import (
	"context"
	"fmt"
	"strings"

	"vouchchain/pkg/kb"
)

// toolSearchLegalKB 法律条款知识库检索（AI 对话第 9 个工具）
// 检索模式如实标注（vector+rerank / vector / keyword 降级），账号欠费时走关键词兜底。
func toolSearchLegalKB(ctx context.Context, args map[string]any, actor ChatActor) (string, error) {
	query := argStr(args, "query")
	if query == "" {
		return "", fmt.Errorf("query 必填")
	}
	topK := int(argInt(args, "topK"))
	res, err := kb.SearchClauses(ctx, query, topK)
	if err != nil {
		return "", err
	}
	if len(res.Clauses) == 0 {
		return "知识库中没有找到相关法条。", nil
	}
	var b strings.Builder
	mode := map[string]string{
		"vector+rerank": "向量检索+重排序",
		"vector":        "向量检索",
		"keyword":       "关键词检索（向量服务不可用已降级）",
	}[res.Mode]
	fmt.Fprintf(&b, "检索模式：%s，命中 %d 条：\n", mode, len(res.Clauses))
	for _, c := range res.Clauses {
		fmt.Fprintf(&b, "· %s《%s》[%s]：%s\n", c.Source, c.Title, c.Category, kb.Truncate(c.Content, 100))
	}
	return b.String(), nil
}
