package agent

import (
	"context"
	"fmt"
	"strings"

	"github.com/cloudwego/eino/schema"
)

// AI 生成合同（用户 11-14 拍板的第一能力）：
// 简略信息/一段话/参考文件 → LLM 起草合同全文 → 存为 .txt 进同一流水线
// （抽取→复核→审批→签署）。参考文件为空时仅依据要素与要点生成。
// 商业对标：Ironclad Jurist / Contract AI 起草能力（收费版 AI 层加价 15-40%）。

type GenerateParams struct {
	ContractType  string `json:"contractType"`  // 采购 / 销售 / 租赁 / 服务 / 其他
	PartyA        string `json:"partyA"`        // 我方（甲方）
	PartyB        string `json:"partyB"`        // 对方（乙方）
	Subject       string `json:"subject"`       // 标的简述
	Amount        string `json:"amount"`        // 金额
	Term          string `json:"term"`          // 履行期限
	ExtraNotes    string `json:"extraNotes"`    // 其他要点（一段话/草稿）
	ReferenceText string `json:"referenceText"` // 可选参考文件文本
}

const generateSystemPrompt = `你是资深合同起草律师。根据用户提供的合同要素起草一份结构完整、条款严谨、可直接进入评审的中文合同。
要求：
1. 必备条款齐全：合同主体信息、标的条款、价款与支付方式、履行期限与方式、验收标准、违约责任、保密条款、争议解决、合同生效与其他约定。
2. 结构规范：标题居中，条款用中文序号（第一条、第二条…），条款内必要时用（一）（二）分层。
3. 语义严谨：权利义务明确，不留歧义表述；金额同时写大写与小写；日期具体。
4. 只输出合同全文纯文本，不要输出任何解释、前言或 Markdown 代码块标记。`

// GenerateContract 起草合同全文（单次 LLM 调用；参考文本仅作风格/条款参考）
func GenerateContract(ctx context.Context, p GenerateParams) (string, error) {
	apiKey, baseUrl, model := resolveLLMConfig()
	if apiKey == "" {
		return "", fmt.Errorf("未配置大模型密钥：请在「模型配置」页为 llm 类别配置 API Key")
	}
	chatModel, err := openaiNewChatModel(ctx, baseUrl, model, apiKey)
	if err != nil {
		return "", fmt.Errorf("init chat model: %w", err)
	}
	var user strings.Builder
	user.WriteString("【合同要素】\n")
	user.WriteString(fmt.Sprintf("- 合同类型：%s\n", orDefault(p.ContractType, "服务")))
	user.WriteString(fmt.Sprintf("- 甲方（我方）：%s\n", orDefault(p.PartyA, "（待补充）")))
	user.WriteString(fmt.Sprintf("- 乙方（对方）：%s\n", orDefault(p.PartyB, "（待补充）")))
	if p.Subject != "" {
		user.WriteString(fmt.Sprintf("- 标的：%s\n", p.Subject))
	}
	if p.Amount != "" {
		user.WriteString(fmt.Sprintf("- 金额：%s\n", p.Amount))
	}
	if p.Term != "" {
		user.WriteString(fmt.Sprintf("- 履行期限：%s\n", p.Term))
	}
	if p.ExtraNotes != "" {
		user.WriteString(fmt.Sprintf("- 其他要点/草稿：%s\n", p.ExtraNotes))
	}
	if p.ReferenceText != "" {
		ref := p.ReferenceText
		if len([]rune(ref)) > 6000 {
			ref = string([]rune(ref)[:6000]) + "…（已截断）"
		}
		user.WriteString("\n【参考文本（仅参考结构与措辞风格，事实以要素为准）】\n" + ref + "\n")
	}
	content, err := generateWithRetry(ctx, chatModel, []*schema.Message{
		{Role: schema.System, Content: generateSystemPrompt},
		{Role: schema.User, Content: user.String()},
	})
	if err != nil {
		return "", fmt.Errorf("generate: %w", err)
	}
	out := strings.TrimSpace(content)
	out = strings.TrimPrefix(out, "```text")
	out = strings.TrimPrefix(out, "```")
	out = strings.TrimSuffix(out, "```")
	if len([]rune(out)) < 200 {
		return "", fmt.Errorf("生成的合同内容过短，请补充要素后重试")
	}
	return out, nil
}

func orDefault(s, def string) string {
	if strings.TrimSpace(s) == "" {
		return def
	}
	return strings.TrimSpace(s)
}
