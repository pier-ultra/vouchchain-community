package kb

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/glog"

	"vouchchain/pkg/cryptoutil"
	"vouchchain/pkg/dbutil"
)

// 法律条款知识库（RAG 最小闭环）：
//  - 向量化/重排：走 model_config 表 category=embedding / rerank（密文同 llm，AES-256-GCM）
//  - 所有接入参数都是「活的」：base_url / model / api_key / 维度 / 端点路径 均可配置，
//    优先级：extra_config(JSONB) > 环境变量 > 内置默认值（旧硅基流动默认值保留，向后兼容）
//  - 维度可配：pgvector 列 vector 无维度约束时支持任意维度；如需固定维度可在 extra_config.dim 指定。
//  - 诚实边界：服务欠费/未开通时（402/403），检索自动降级为关键词 LIKE，功能不断但相关性弱，
//    回复里如实标注「关键词降级」来源。

const (
	defaultEmbedDim = 1024 // 仅作兜底默认；实际维度见 effectiveEmbedDim()
	httpTO          = 30 * time.Second
)

// envOr 读环境变量，空则取默认值（与 agent.getEnv 同语义，避免跨包耦合）
func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// intFrom 从 any（float64/string/int）解析正整数，失败返回 def
func intFrom(v any, def int) int {
	switch n := v.(type) {
	case float64:
		if n > 0 {
			return int(n)
		}
	case int:
		if n > 0 {
			return n
		}
	case string:
		if i, err := strconv.Atoi(strings.TrimSpace(n)); err == nil && i > 0 {
			return i
		}
	}
	return def
}

// intFromStr 解析字符串为正整数，失败返回 def
func intFromStr(s string, def int) int {
	if i, err := strconv.Atoi(strings.TrimSpace(s)); err == nil && i > 0 {
		return i
	}
	return def
}

// configuredEmbedDim 解析「显式配置」的 embedding 维度：
//  1. 环境变量 EMBED_DIM（最高优先级，运维可一行 env 切换）
//  2. 配置行 extra_config.dim
//  3. 都没配 → 返回 0，表示「动态接受任意维度」（以服务端实际返回为准）
//
// 这样既不破坏旧行为（硅基流动 1024 维照常写入），也让讯飞 xop3qwen8bembedding(768)
// 开箱即用；只有当你想强制校验维度时才需要配置 dim。
func configuredEmbedDim(extra map[string]any) int {
	if v := os.Getenv("EMBED_DIM"); v != "" {
		if i, err := strconv.Atoi(strings.TrimSpace(v)); err == nil && i > 0 {
			return i
		}
	}
	if extra != nil {
		if d, ok := extra["dim"]; ok {
			return intFrom(d, 0)
		}
	}
	return 0
}

// defaultBaseURL 按类别返回内置默认 base_url（保留旧硅基流动默认，向后兼容；
// 可用环境变量 EMBED_BASE_URL / RERANK_BASE_URL / OCR_BASE_URL 覆盖）。
func defaultBaseURL(category string) string {
	switch category {
	case "embedding":
		return envOr("EMBED_BASE_URL", "https://api.siliconflow.cn/v1")
	case "rerank":
		return envOr("RERANK_BASE_URL", "https://api.siliconflow.cn/v1")
	case "ocr":
		return envOr("OCR_BASE_URL", "")
	default:
		return ""
	}
}

// resolveCategoryConfigFull 按 category 取启用配置 + extra_config。
// 返回 (apiKey, baseUrl, model, extra)。extra 为空 map 而非 nil，便于直接读字段。
func resolveCategoryConfigFull(category string) (apiKey, baseUrl, model string, extra map[string]any) {
	baseUrl = defaultBaseURL(category)
	extra = map[string]any{}
	rows, err := g.DB().Model("model_config").
		Where("category", category).Where("enabled", true).
		Order("id asc").All()
	if err != nil {
		return "", baseUrl, "", extra
	}
	// 先取有可用密钥的；都没有则取任意一条 enabled（本地服务场景/无需鉴权的 OCR）
	var fallbackKey, fallbackUrl, fallbackModel string
	var fallbackExtra map[string]any
	for _, row := range rows {
		stored := row["api_key"].String()
		plain := ""
		if stored != "" {
			p, derr := cryptoutil.Decrypt(stored)
			if derr != nil || p == "" {
				continue
			}
			plain = p
		}
		url := row["base_url"].String()
		if url == "" {
			url = baseUrl
		}
		ex := map[string]any{}
		if raw := row["extra_config"].String(); raw != "" {
			_ = json.Unmarshal([]byte(raw), &ex)
		}
		if plain != "" {
			return plain, url, row["model_name"].String(), ex
		}
		if fallbackKey == "" {
			fallbackKey, fallbackUrl, fallbackModel, fallbackExtra = "", url, row["model_name"].String(), ex
		}
	}
	return fallbackKey, fallbackUrl, fallbackModel, fallbackExtra
}

// resolveCategoryConfig 兼容旧签名（只取前三值）。
func resolveCategoryConfig(category string) (apiKey, baseUrl, model string) {
	apiKey, baseUrl, model, _ = resolveCategoryConfigFull(category)
	return
}

// extraString 从 extra_config 取字符串：显式配置优先，其次环境变量 key。
func extraString(extra map[string]any, name, envKey, def string) string {
	if extra != nil {
		if v, ok := extra[name].(string); ok && v != "" {
			return v
		}
	}
	if envKey != "" {
		if v := os.Getenv(envKey); v != "" {
			return v
		}
	}
	return def
}

// Embed 文本批量向量化（向量模型/维度/端点全配置驱动；本地自托管服务允许空密钥）
func Embed(ctx context.Context, texts []string) ([][]float64, error) {
	apiKey, baseUrl, model, extra := resolveCategoryConfigFull("embedding")
	if baseUrl == "" || (baseUrl == "https://api.siliconflow.cn/v1" && apiKey == "") {
		return nil, errors.New("未配置向量模型（模型配置页 category=embedding）")
	}
	if model == "" {
		model = envOr("EMBED_MODEL", "BAAI/bge-m3")
	}
	path := extraString(extra, "path", "EMBED_PATH", "/embeddings")
	return callEmbeddings(ctx, baseUrl, apiKey, model, path, texts)
}

func callEmbeddings(ctx context.Context, baseUrl, apiKey, model, path string, texts []string) ([][]float64, error) {
	body, _ := json.Marshal(map[string]any{"model": model, "input": texts})
	req, err := http.NewRequestWithContext(ctx, "POST", strings.TrimRight(baseUrl, "/")+"/"+strings.TrimLeft(path, "/"), bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}
	cli := &http.Client{Timeout: httpTO}
	resp, err := cli.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("向量化失败 HTTP %d: %s", resp.StatusCode, trunc(string(raw), 160))
	}
	var out struct {
		Data []struct {
			Index     int       `json:"index"`
			Embedding []float64 `json:"embedding"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	vs := make([][]float64, len(texts))
	for _, d := range out.Data {
		if d.Index < len(vs) {
			vs[d.Index] = d.Embedding
		}
	}
	for _, v := range vs {
		if len(v) == 0 {
			return nil, errors.New("向量化响应缺行")
		}
	}
	return vs, nil
}

// Rerank 重排序：返回按相关性从高到低的文档下标
func Rerank(ctx context.Context, query string, docs []string) ([]int, error) {
	apiKey, baseUrl, model, extra := resolveCategoryConfigFull("rerank")
	if baseUrl == "" || baseUrl == "https://api.siliconflow.cn/v1" && apiKey == "" {
		return nil, errors.New("未配置重排模型（模型配置页 category=rerank）")
	}
	if model == "" {
		model = envOr("RERANK_MODEL", "BAAI/bge-reranker-v2-m3")
	}
	path := extraString(extra, "path", "RERANK_PATH", "/rerank")
	// TEI 用 texts，硅基流动/OpenAI 风格用 documents：两字段都带，服务端各取所需
	body, _ := json.Marshal(map[string]any{"model": model, "query": query, "texts": docs, "documents": docs})
	req, err := http.NewRequestWithContext(ctx, "POST", strings.TrimRight(baseUrl, "/")+"/"+strings.TrimLeft(path, "/"), bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}
	cli := &http.Client{Timeout: httpTO}
	resp, err := cli.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("重排失败 HTTP %d: %s", resp.StatusCode, trunc(string(raw), 160))
	}
	// 兼容两种响应格式：硅基流动/OpenAI 风格 {"results":[{index}]} 与 TEI 顶层数组 [{index,score}]
	var openaiStyle struct {
		Results []struct {
			Index int `json:"index"`
		} `json:"results"`
	}
	if err := json.Unmarshal(raw, &openaiStyle); err == nil && len(openaiStyle.Results) > 0 {
		idx := make([]int, 0, len(openaiStyle.Results))
		for _, r := range openaiStyle.Results {
			idx = append(idx, r.Index)
		}
		return idx, nil
	}
	var teiStyle []struct {
		Index int `json:"index"`
	}
	if err := json.Unmarshal(raw, &teiStyle); err != nil {
		return nil, fmt.Errorf("重排响应格式不识别: %s", trunc(string(raw), 120))
	}
	idx := make([]int, 0, len(teiStyle))
	for _, r := range teiStyle {
		idx = append(idx, r.Index)
	}
	return idx, nil
}

// Clause 知识库条款
type Clause struct {
	ID       int64  `json:"id"`
	Title    string `json:"title"`
	Content  string `json:"content"`
	Source   string `json:"source"`
	Category string `json:"category"`
}

// SearchResult 检索结果（mode 如实标注向量/重排/降级）
type SearchResult struct {
	Mode    string   `json:"mode"` // vector+rerank / vector / keyword
	Clauses []Clause `json:"clauses"`
}

// SearchClauses 知识库检索：向量优先（pgvector 余弦距离），重排精排；失败降级关键词
func SearchClauses(ctx context.Context, query string, topK int) (*SearchResult, error) {
	if topK <= 0 || topK > 10 {
		topK = 4
	}
	// 1) 向量检索（仅 pg：pgvector 余弦距离；sqlite/mysql/openGauss 无 pgvector，如实降级为关键词检索）
	if !dbutil.IsSQLite() && !dbutil.IsMySQL() && !dbutil.IsOpenGauss() {
		vs, err := Embed(ctx, []string{query})
		if err == nil && len(vs) == 1 {
			vec, _ := json.Marshal(vs[0])
			rows, qerr := g.DB().GetAll(ctx,
				"SELECT id, title, content, source, category FROM kb_clause WHERE embedding IS NOT NULL ORDER BY embedding <=> ?::vector LIMIT ?", string(vec), topK)
			if qerr == nil && len(rows) > 0 {
				clauses := toClauses(rows)
				mode := "vector"
				// 2) 重排精排（候选多于 1 条才有意义）
				if len(clauses) > 1 {
					docs := make([]string, len(clauses))
					for i, c := range clauses {
						docs[i] = c.Title + " " + c.Content
					}
					if order, rerr := Rerank(ctx, query, docs); rerr == nil && len(order) > 0 {
						sorted := make([]Clause, 0, len(clauses))
						for _, i := range order {
							if i < len(clauses) {
								sorted = append(sorted, clauses[i])
							}
						}
						clauses = sorted
						mode = "vector+rerank"
					}
				}
				return &SearchResult{Mode: mode, Clauses: clauses}, nil
			}
		}
	}
	// 3) 降级：关键词 LIKE（向量/重排不可用时不让检索断）
	rows, kerr := g.DB().GetAll(ctx,
		"SELECT id, title, content, source, category FROM kb_clause WHERE title LIKE ? OR content LIKE ? ORDER BY id LIMIT ?",
		"%"+query+"%", "%"+query+"%", topK)
	if kerr != nil {
		return nil, kerr
	}
	if len(rows) == 0 {
		// 关键词太长没命中的兜底：取前 topK 条展示
		rows, _ = g.DB().GetAll(ctx, "SELECT id, title, content, source, category FROM kb_clause ORDER BY id LIMIT ?", topK)
	}
	return &SearchResult{Mode: "keyword", Clauses: toClauses(rows)}, nil
}

// SeedIfEmpty 空库时种子民法典合同编相关条款（真实法条；文本以官方公布为准，建议核对）
func SeedIfEmpty(ctx context.Context) (int, error) {
	n, err := g.DB().Model("kb_clause").Count()
	if err != nil {
		return 0, err
	}
	if n > 0 {
		return 0, nil
	}
	seeds := []Clause{
		{Title: "违约金过高可请求减少", Source: "《民法典》第五百八十五条", Category: "违约",
			Content: "当事人可以约定一方违约时应当根据违约情况向对方支付一定数额的违约金，也可以约定因违约产生的损失赔偿额的计算方法。约定的违约金低于造成的损失的，人民法院或者仲裁机构可以根据当事人的请求予以增加；约定的违约金过分高于造成的损失的，人民法院或者仲裁机构可以根据当事人的请求予以适当减少。"},
		{Title: "损失赔偿以可预见为限", Source: "《民法典》第五百八十四条", Category: "违约",
			Content: "当事人一方不履行合同义务或者履行合同义务不符合约定，造成对方损失的，损失赔偿额应当相当于因违约所造成的损失，包括合同履行后可以获得的利益；但是，不得超过违约一方订立合同时预见到或者应当预见到的因违约可能造成的损失。"},
		{Title: "定金数额不得超过主合同标的额百分之二十", Source: "《民法典》第五百八十六条", Category: "定金",
			Content: "当事人可以约定一方向对方给付定金作为债权的担保。定金合同自实际交付定金时成立。定金的数额由当事人约定；但是，不得超过主合同标的额的百分之二十，超过部分不产生定金的效力。"},
		{Title: "定金罚则", Source: "《民法典》第五百八十七条", Category: "定金",
			Content: "债务人履行债务的，定金应当抵作价款或者收回。给付定金的一方不履行债务或者履行债务不符合约定，致使不能实现合同目的的，无权请求返还定金；收受定金的一方不履行债务或者履行债务不符合约定，致使不能实现合同目的的，应当双倍返还定金。"},
		{Title: "违约责任的一般规定", Source: "《民法典》第五百七十七条", Category: "违约",
			Content: "当事人一方不履行合同义务或者履行合同义务不符合约定的，应当承担继续履行、采取补救措施或者赔偿损失等违约责任。"},
		{Title: "合同解除的情形", Source: "《民法典》第五百六十三条", Category: "解除",
			Content: "有下列情形之一的，当事人可以解除合同：（一）因不可抗力致使不能实现合同目的；（二）在履行期限届满前，当事人一方明确表示或者以自己的行为表明不履行主要债务；（三）当事人一方迟延履行主要债务，经催告后在合理期限内仍未履行；（四）当事人一方迟延履行债务或者有其他违约行为致使不能实现合同目的；（五）法律规定的其他情形。"},
		{Title: "协商解除与约定解除", Source: "《民法典》第五百六十二条", Category: "解除",
			Content: "当事人协商一致，可以解除合同。当事人可以约定一方解除合同的事由。解除合同的事由发生时，解除权人可以解除合同。"},
		{Title: "格式条款的提示说明义务", Source: "《民法典》第四百九十六条", Category: "格式条款",
			Content: "格式条款是当事人为了重复使用而预先拟定，并在订立合同时未与对方协商的条款。采用格式条款订立合同的，提供格式条款的一方应当遵循公平原则确定当事人之间的权利和义务，并采取合理的方式提示对方注意免除或者减轻其责任等与对方有重大利害关系的条款，按照对方的要求，对该条款予以说明。提供格式条款的一方未履行提示或者说明义务，致使对方没有注意或者理解与其有重大利害关系的条款的，对方可以主张该条款不成为合同的内容。"},
		{Title: "格式条款无效情形", Source: "《民法典》第四百九十七条", Category: "格式条款",
			Content: "有下列情形之一的，该格式条款无效：（一）具有本法第一编第六章第三节和本法第五百零六条规定的无效情形；（二）提供格式条款一方不合理地免除或者减轻其责任、加重对方责任、限制对方主要权利；（三）提供格式条款一方排除对方主要权利。"},
		{Title: "诉讼时效三年", Source: "《民法典》第一百八十八条", Category: "时效",
			Content: "向人民法院请求保护民事权利的诉讼时效期间为三年。法律另有规定的，依照其规定。诉讼时效期间自权利人知道或者应当知道权利受到损害以及义务人之日起计算。法律另有规定的，依照其规定。但是，自权利受到损害之日起超过二十年的，人民法院不予保护。"},
		{Title: "合同成立：签名盖章按指印", Source: "《民法典》第四百九十条", Category: "成立生效",
			Content: "当事人采用合同书形式订立合同的，自当事人均签名、盖章或者按指印时合同成立。在签名、盖章或者按指印之前，当事人一方已经履行主要义务，对方接受时，该合同成立。法律、行政法规规定或者当事人约定合同应当采用书面形式订立，当事人未采用书面形式但是一方已经履行主要义务，对方接受时，该合同成立。"},
		{Title: "质量、价款等约定不明的补充规则", Source: "《民法典》第五百一十条、五百一十一条", Category: "约定不明",
			Content: "合同生效后，当事人就质量、价款或者报酬、履行地点等内容没有约定或者约定不明确的，可以协议补充；不能达成补充协议的，按照合同相关条款或者交易习惯确定。质量要求不明确的，按照强制性国家标准履行；价款或者报酬不明确的，按照订立合同时履行地的市场价格履行。"},
	}
	// 种子直接入库，embedding 留空（向量化服务可用时由 Enrich 增量补）
	for _, s := range seeds {
		if _, err := g.DB().Model("kb_clause").Data(g.Map{
			"title": s.Title, "content": s.Content, "source": s.Source, "category": s.Category,
			"created_at": time.Now(),
		}).Insert(); err != nil {
			return 0, err
		}
	}
	return len(seeds), nil
}

// Enrich 给未向量化的条款补向量（embedding 服务可用时调用；失败返回 0 不报错——不阻断）
func Enrich(ctx context.Context) int {
	rows, err := g.DB().GetAll(ctx, "SELECT id, title, content FROM kb_clause WHERE embedding IS NULL ORDER BY id LIMIT 50")
	if err != nil || len(rows) == 0 {
		return 0
	}
	// 分批：llama.cpp 单请求受上下文窗口限制（n_ctx 8192），每批 4 条稳妥；
	// 可用 EMBED_BATCH 环境变量调整
	batchSize := intFromStr(envOr("EMBED_BATCH", "4"), 4)
	_, _, _, embedExtra := resolveCategoryConfigFull("embedding")
	wantDim := configuredEmbedDim(embedExtra) // 0=动态，接受服务端返回的任意维度
	done := 0
	ids := make([]int64, 0, len(rows))
	texts := make([]string, 0, len(rows))
	flush := func(ids []int64, texts []string) {
		if len(ids) == 0 {
			return
		}
		vs, err := Embed(ctx, texts)
		if err != nil {
			glog.Warningf(ctx, "[kb] 向量化失败（%d 条）：%v", len(ids), err)
			return
		}
		for i, v := range vs {
			if i >= len(ids) || len(v) == 0 || (wantDim > 0 && len(v) != wantDim) {
				if len(v) > 0 && wantDim > 0 && len(v) != wantDim {
					glog.Warningf(ctx, "[kb] 向量维度 %d 与配置 dim=%d 不符，跳过", len(v), wantDim)
				}
				continue
			}
			vec, _ := json.Marshal(v)
			// pg：向量落 pgvector 列；sqlite/mysql/openGauss：embedding 为 TEXT 列，直接存 JSON 文本（无 ::vector cast）
			var uerr error
			if dbutil.IsSQLite() || dbutil.IsMySQL() || dbutil.IsOpenGauss() {
				_, uerr = g.DB().Exec(ctx, "UPDATE kb_clause SET embedding = ? WHERE id = ?", string(vec), ids[i])
			} else {
				_, uerr = g.DB().Exec(ctx, "UPDATE kb_clause SET embedding = ?::vector WHERE id = ?", string(vec), ids[i])
			}
			if uerr != nil {
				glog.Warningf(ctx, "[kb] 向量落库失败 #%d：%v", ids[i], uerr)
				continue
			}
			done++
		}
	}
	for _, r := range rows {
		ids = append(ids, r["id"].Int64())
		texts = append(texts, r["title"].String()+"\n"+r["content"].String())
		if len(ids) >= batchSize {
			flush(ids, texts)
			ids, texts = ids[:0], texts[:0]
		}
	}
	flush(ids, texts)
	return done
}

func toClauses(rows gdb.Result) []Clause {
	out := []Clause{}
	for _, r := range rows {
		out = append(out, Clause{
			ID: r["id"].Int64(), Title: r["title"].String(), Content: r["content"].String(),
			Source: r["source"].String(), Category: r["category"].String(),
		})
	}
	return out
}

func trunc(s string, n int) string {
	r := []rune(strings.TrimSpace(s))
	if len(r) <= n {
		return string(r)
	}
	return string(r[:n]) + "…"
}

// Truncate 导出的文本截断（AI 工具回显用）
func Truncate(s string, n int) string {
	return trunc(s, n)
}

// ResolveConfig 导出按类别取配置（fileparse 接 OCR 等复用；本地服务允许空密钥）
func ResolveConfig(category string) (apiKey, baseUrl, model string) {
	return resolveCategoryConfig(category)
}

// OCRExtract 调 OCR 服务识别文件，双协议兼容：
//   - protocol=multipart（默认，旧行为）：自建 RapidOCR，multipart file → {text, pages}
//   - protocol=openai-chat：OpenAI 兼容多模态（讯飞 xopdeepseekocr / xoppaddleocrv16 / xophunyuanocr），
//     POST {baseUrl}/chat/completions，content=[{type:text},{type:image_url,image_url:{url:data URI}}]
//     读 choices[0].message.content。base_url 填 https://maas-api.cn-huabei-1.xf-yun.com/v2
//
// 可用 extra_config.protocol / 环境变量 OCR_PROTOCOL 切换；其余（超时/prompt/路径）同样可配。
func OCRExtract(ctx context.Context, filename string, data []byte) (string, error) {
	apiKey, baseUrl, model, extra := resolveCategoryConfigFull("ocr")
	if baseUrl == "" {
		return "", errors.New("未配置 OCR 模型（模型配置页 category=ocr）；扫描件/图片版 PDF 无法识别，请改用文字版 PDF 或 DOCX")
	}
	protocol := extraString(extra, "protocol", "OCR_PROTOCOL", "multipart")
	timeout := time.Duration(intFromStr(extraString(extra, "timeoutSec", "OCR_TIMEOUT", "120"), 120)) * time.Second
	if strings.EqualFold(protocol, "openai-chat") {
		return ocrOpenAIChat(ctx, apiKey, baseUrl, model, extra, filename, data, timeout)
	}
	return ocrMultipart(ctx, baseUrl, filename, data, timeout)
}

// ocrMultipart 旧自建 OCR 协议（保持向后兼容）
func ocrMultipart(ctx context.Context, baseUrl, filename string, data []byte, timeout time.Duration) (string, error) {
	var buf bytes.Buffer
	bw := multipart.NewWriter(&buf)
	fw, _ := bw.CreateFormFile("file", filename)
	fw.Write(data)
	bw.Close()
	path := envOr("OCR_PATH", "/ocr")
	req, err := http.NewRequestWithContext(ctx, "POST", strings.TrimRight(baseUrl, "/")+"/"+strings.TrimLeft(path, "/"), &buf)
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", bw.FormDataContentType())
	cli := &http.Client{Timeout: timeout}
	resp, err := cli.Do(req)
	if err != nil {
		return "", fmt.Errorf("OCR 服务不可达: %w", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("OCR 失败 HTTP %d: %s", resp.StatusCode, trunc(string(raw), 160))
	}
	var out struct {
		Text  string `json:"text"`
		Pages int    `json:"pages"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return "", err
	}
	return out.Text, nil
}

// ocrOpenAIChat OpenAI 兼容多模态 OCR（讯飞 MaaS 等）
func ocrOpenAIChat(ctx context.Context, apiKey, baseUrl, model string, extra map[string]any, filename string, data []byte, timeout time.Duration) (string, error) {
	if model == "" {
		model = envOr("OCR_MODEL", "xopdeepseekocr")
	}
	prompt := extraString(extra, "prompt", "OCR_PROMPT", "请识别图片中的所有文字，按原文顺序输出，只输出识别结果，不要解释。")
	path := extraString(extra, "path", "OCR_PATH", "/chat/completions")
	endpoint := strings.TrimRight(baseUrl, "/") + "/" + strings.TrimLeft(path, "/")
	cli := &http.Client{Timeout: timeout}

	// 多模态 OCR 只接受图片。扫描件 PDF 内嵌的是 JPEG(DCTDecode) 帧，
	// 直接把整个 PDF 当 application/pdf data URI 会被讯飞等拒绝（unsupported image format）。
	// 这里先把 PDF 拆成一帧帧 JPEG，逐帧识别后拼接；图片文件保持原样直传。
	frames := [][]byte{data}
	mime := mimeByExt(filename)
	if strings.EqualFold(filepath.Ext(filename), ".pdf") {
		if jpegs := jpegsInPDF(data); len(jpegs) > 0 {
			frames, mime = jpegs, "image/jpeg"
		}
	}

	var merged strings.Builder
	for _, frame := range frames {
		dataURI := "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(frame)
		body, _ := json.Marshal(map[string]any{
			"model": model,
			"messages": []any{map[string]any{"role": "user", "content": []any{
				map[string]any{"type": "text", "text": prompt},
				map[string]any{"type": "image_url", "image_url": map[string]any{"url": dataURI}},
			}}},
			"stream": false,
		})
		req, err := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewReader(body))
		if err != nil {
			return "", err
		}
		req.Header.Set("Content-Type", "application/json")
		if apiKey != "" {
			req.Header.Set("Authorization", "Bearer "+apiKey)
		}
		resp, err := cli.Do(req)
		if err != nil {
			return "", fmt.Errorf("OCR 服务不可达: %w", err)
		}
		raw, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != 200 {
			return "", fmt.Errorf("OCR 失败 HTTP %d: %s", resp.StatusCode, trunc(string(raw), 200))
		}
		var out struct {
			Choices []struct {
				Message struct {
					Content string `json:"content"`
				} `json:"message"`
			} `json:"choices"`
		}
		if err := json.Unmarshal(raw, &out); err != nil {
			return "", err
		}
		if len(out.Choices) == 0 || strings.TrimSpace(out.Choices[0].Message.Content) == "" {
			return "", fmt.Errorf("OCR 返回空内容: %s", trunc(string(raw), 160))
		}
		merged.WriteString(strings.TrimSpace(out.Choices[0].Message.Content))
		merged.WriteString("\n")
	}
	return strings.TrimSpace(merged.String()), nil
}

// jpegsInPDF 从 PDF 字节流中提取内嵌 JPEG(DCTDecode) 帧。
// 扫描件/图片版 PDF 的每一页就是一个完整 JPEG（FFD8…FFD9），逐帧抠出即可喂给多模态 OCR，
// 无需引入重量级 PDF 渲染库。找不到（如纯文字版 PDF）时返回 nil，调用方按原样处理。
func jpegsInPDF(data []byte) [][]byte {
	var frames [][]byte
	for i := 0; i+4 < len(data); i++ {
		if data[i] == 0xFF && data[i+1] == 0xD8 { // JPEG SOI
			for j := i + 2; j+1 < len(data); j++ {
				if data[j] == 0xFF && data[j+1] == 0xD9 { // JPEG EOI
					frames = append(frames, data[i:j+2])
					i = j + 1
					break
				}
			}
		}
	}
	return frames
}

// mimeByExt 按扩展名推断 image media type（data URI 需要）
func mimeByExt(filename string) string {
	switch strings.ToLower(filepath.Ext(filename)) {
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".webp":
		return "image/webp"
	case ".gif":
		return "image/gif"
	case ".bmp":
		return "image/bmp"
	case ".pdf":
		return "application/pdf"
	default:
		return "image/png"
	}
}
