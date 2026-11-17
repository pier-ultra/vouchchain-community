package agent

import (
	"context"
	"os"
	"strings"
	"time"

	"github.com/cloudwego/eino-ext/components/model/openai"
	"github.com/cloudwego/eino/schema"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/glog"

	"vouchchain/pkg/cryptoutil"
)

func getEnv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// openaiNewChatModel 统一的 OpenAI 兼容模型构造（条款审查与结构化抽取共用）
func openaiNewChatModel(ctx context.Context, baseUrl, model, apiKey string) (*openai.ChatModel, error) {
	return openai.NewChatModel(ctx, &openai.ChatModelConfig{
		BaseURL: baseUrl,
		Model:   model,
		APIKey:  apiKey,
	})
}

// generateWithRetry 非流式生成 + 429 限流自动退避（Agnes 免费档 RPM 受限；退避 5/15/30s，共 4 次尝试）
func generateWithRetry(ctx context.Context, chatModel *openai.ChatModel, msgs []*schema.Message) (string, error) {
	delays := []time.Duration{5, 15, 30}
	for attempt := 0; ; attempt++ {
		resp, err := chatModel.Generate(ctx, msgs)
		if err == nil {
			return resp.Content, nil
		}
		if !strings.Contains(err.Error(), "429") || attempt >= len(delays) {
			return "", err
		}
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(delays[attempt] * time.Second):
		}
	}
}

// resolveLLMConfig 大模型接入配置的解析顺序（用户 11-14 拍板：密钥可加密存库，读时解密）：
//  1. model_config 表（category=llm、enabled、有密钥的第一条）：密文 AES-256-GCM 解密 + baseUrl/modelName
//  2. 环境变量兜底：AGNES_API_KEY / AGNES_BASE_URL / AGNES_MODEL
//  3. 都没有 → 空（调用方返回 dev 占位）
func resolveLLMConfig() (apiKey, baseUrl, model string) {
	baseUrl, model = getEnv("AGNES_BASE_URL", "https://api.agnes-ai.cn/v1"), getEnv("AGNES_MODEL", "agnes-3.0-flash")
	rows, err := g.DB().Model("model_config").
		Where("category", "llm").
		Where("enabled", true).
		Where("api_key <> ''").
		Order("id asc").All()
	if err != nil {
		glog.Warningf(context.Background(), "[llm] 读取模型配置失败（将回退环境变量）：%v", err)
	}
	// 逐条尝试：第一条密文解不开（如重启未带 APP_AES_KEY）不能让整站 AI 静默失效。
	// 这里明确打日志，配合 main.go 的启动告警与模型配置页的"密钥不可用"红标。
	for _, row := range rows {
		stored := row["api_key"].String()
		plain, derr := cryptoutil.Decrypt(stored)
		if derr != nil || plain == "" {
			glog.Warningf(context.Background(), "[llm] 配置 #%d 的密钥不可用（%v），跳过该条", row["id"].Int64(), derr)
			continue
		}
		if v := row["base_url"].String(); v != "" {
			baseUrl = v
		}
		if v := row["model_name"].String(); v != "" {
			model = v
		}
		return plain, baseUrl, model
	}
	if v := os.Getenv("AGNES_API_KEY"); v != "" {
		return v, baseUrl, model
	}
	return "", baseUrl, model
}

func trunc2(s string, n int) string {
	r := []rune(strings.TrimSpace(s))
	if len(r) <= n {
		return string(r)
	}
	return string(r[:n]) + "…"
}

// DecryptKey 导出给其他包（kb/ocr 等）复用的密文解密：model_config.api_key 存的 enc:v1: 密文
func DecryptKey(cipher string) (string, error) {
	return cryptoutil.Decrypt(cipher)
}

// ResolveLLMConfig 导出：其他包（发票分类等）复用 LLM 配置解析（密文解密→环境变量兜底）
func ResolveLLMConfig() (apiKey, baseUrl, model string) {
	return resolveLLMConfig()
}

// NewOpenAIChatModel 导出：按 OpenAI 兼容端点构建 chat model
func NewOpenAIChatModel(ctx context.Context, baseUrl, model, apiKey string) (*openai.ChatModel, error) {
	return openaiNewChatModel(ctx, baseUrl, model, apiKey)
}

// GenerateOnce 导出：单轮生成（无工具），429 退避复用
func GenerateOnce(ctx context.Context, m *openai.ChatModel, prompt string) (string, error) {
	return generateWithRetry(ctx, m, []*schema.Message{
		{Role: schema.User, Content: prompt},
	})
}
