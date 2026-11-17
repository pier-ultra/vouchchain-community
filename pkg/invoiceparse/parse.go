package invoiceparse

import (
	"archive/zip"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// 数电票解析（US-I1）：OFD/XML 电子发票 → 八字段 + 明细行。
// OFD 本质是 zip 包 XML——用 Go 标准库解析（零第三方依赖，不内嵌 ofdrw 等 AGPL 组件，许可证红线无碍）。
// 解析策略：宽松别名匹配（中英文标签多套别名），字段缺失明确报错不静默（AI1.4）。
// 置信度说明：测试样例为合成数电票；真实票样（数电票 XML/OFD 实际导出）解析命中率需真实票据验证。

type Parsed struct {
	InvoiceNo   string                   `json:"invoiceNo"`
	IssueDate   string                   `json:"issueDate"`
	BuyerName   string                   `json:"buyerName"`
	SellerName  string                   `json:"sellerName"`
	Amount      string                   `json:"amount"` // 不含税金额（字符串入库前转数值）
	Tax         string                   `json:"tax"`
	Total       string                   `json:"total"` // 价税合计
	InvoiceType string                   `json:"invoiceType"`
	Items       []map[string]interface{} `json:"items"` // 明细行
}

// 字段别名表（覆盖常见数电票导出标签：中文标签与英文驼峰）
var fieldAliases = map[string][]string{
	"invoiceNo": {"发票号码", "InvoiceNo", "Fphm", "invoiceNumber"},
	"issueDate": {"开票日期", "IssueDate", "IssueTime", "Kprq", "Date"},
	"buyerName": {"购买方名称", "BuyerName", "GmfMc", "buyerName", "购买方"},
	"sellerName": {"销售方名称", "SellerName", "XsfMc", "sellerName", "销售方"},
	"amount":    {"合计金额", "不含税金额", "TotalAmount", "TotalTaxInclusiveAmountExcludeTax", "Hjje", "amount"},
	"tax":       {"合计税额", "税额", "TotalTaxAmount", "TaxAmount", "Hjse", "tax"},
	"total":     {"价税合计", "价税合计（大写）", "TotalTaxInclusiveAmount", "Jshj", "total"},
	"invoiceType": {"发票类型", "InvoiceType", "FplxDm", "invoiceType"},
}

func IsSupported(filename string) bool {
	ext := strings.ToLower(filepath.Ext(filename))
	return ext == ".ofd" || ext == ".xml"
}

// Parse 按扩展名分发
func Parse(path string) (*Parsed, error) {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".ofd":
		return parseOfd(path)
	case ".xml":
		return parseXmlFile(path)
	default:
		return nil, fmt.Errorf("不支持的格式 %s（发票导入仅支持 OFD / XML）", ext)
	}
}

// parseOfd 解包 OFD（zip），汇总其中 XML 文本后做字段提取
func parseOfd(path string) (*Parsed, error) {
	r, err := zip.OpenReader(path)
	if err != nil {
		return nil, fmt.Errorf("打开 OFD 失败（非有效 zip 包）: %w", err)
	}
	defer r.Close()

	var buf strings.Builder
	xmlCount := 0
	for _, f := range r.File {
		if !strings.HasSuffix(strings.ToLower(f.Name), ".xml") {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			continue
		}
		data, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			continue
		}
		buf.Write(data)
		buf.WriteString("\n")
		xmlCount++
	}
	if xmlCount == 0 {
		return nil, errors.New("OFD 内未找到任何 XML（可能不是电子发票版式文件）")
	}
	return extractFields(buf.String())
}

func parseXmlFile(path string) (*Parsed, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return extractFields(string(data))
}

// extractFields 宽松别名匹配：两遍扫描——先精确标签值，再任意文本前缀兜底
func extractFields(text string) (*Parsed, error) {
	elements := collectElements(text)
	p := &Parsed{}
	missing := []string{}

	for field, aliases := range fieldAliases {
		val := ""
		for _, a := range aliases {
			if v, ok := elements[a]; ok && strings.TrimSpace(v) != "" {
				val = strings.TrimSpace(v)
				break
			}
			// 前缀匹配（标签可能带命名空间，如 <ns:发票号码>）
			for k, v := range elements {
				ln := localName(k)
				if (strings.Contains(ln, a) || strings.Contains(a, ln)) && len(ln) > 1 && strings.TrimSpace(v) != "" {
					val = strings.TrimSpace(v)
					break
				}
			}
			if val != "" {
				break
			}
		}
		switch field {
		case "invoiceNo":
			p.InvoiceNo = cleanNo(val)
		case "issueDate":
			p.IssueDate = cleanDate(val)
		case "buyerName":
			p.BuyerName = val
		case "sellerName":
			p.SellerName = val
		case "amount":
			p.Amount = val
		case "tax":
			p.Tax = val
		case "total":
			p.Total = val
		case "invoiceType":
			p.InvoiceType = val
		}
	}
	if p.InvoiceNo == "" {
		missing = append(missing, "发票号码")
	}
	if p.Total == "" && p.Amount == "" {
		missing = append(missing, "金额/价税合计")
	}
	if p.BuyerName == "" && p.SellerName == "" {
		missing = append(missing, "购买方/销售方")
	}
	if len(missing) > 0 {
		return nil, fmt.Errorf("未解析到关键字段（%s）：文件可能不是数电票 XML/OFD，或字段命名超出当前别名表", strings.Join(missing, "、"))
	}
	p.Items = extractItems(text)
	if p.InvoiceType == "" {
		p.InvoiceType = guessType(text)
	}
	return p, nil
}

// extractItems 明细行：匹配重复的 <Item>/<明细>/... 块（宽松：任意含"项目名称/ItemName"的重复元素组）
func extractItems(text string) []map[string]interface{} {
	// 数电票明细常见：<Item><ItemName>...</ItemName><Amount>..</Amount></Item> 重复出现
	var items []map[string]interface{}
	re := regexp.MustCompile(`(?is)<(?:Item|InvoiceItem|明细行|Detail)\b[^>]*>(.*?)</(?:Item|InvoiceItem|明细行|Detail)>`)
	for _, block := range re.FindAllStringSubmatch(text, 50) {
		els := collectElements(block[1])
		if len(els) == 0 {
			continue
		}
		m := map[string]interface{}{}
		for k, v := range els {
			m[localName(k)] = v
		}
		items = append(items, m)
	}
	return items
}

func guessType(text string) string {
	switch {
	case strings.Contains(text, "专用发票"):
		return "电子发票（专用发票）"
	case strings.Contains(text, "普通发票"):
		return "电子发票（普通发票）"
	default:
		return "电子发票"
	}
}

// collectElements 把 XML（可能混杂多文档/无根）所有元素收进 map[标签名]首个文本值
func collectElements(text string) map[string]string {
	out := map[string]string{}
	dec := xml.NewDecoder(strings.NewReader(text))
	dec.Strict = false
	var stack []string
	for {
		tok, err := dec.Token()
		if err != nil {
			break
		}
		switch t := tok.(type) {
		case xml.StartElement:
			stack = append(stack, t.Name.Local)
		case xml.EndElement:
			if len(stack) > 0 {
				stack = stack[:len(stack)-1]
			}
		case xml.CharData:
			if len(stack) == 0 {
				continue
			}
			name := stack[len(stack)-1]
			v := strings.TrimSpace(string(t))
			if v == "" {
				continue
			}
			if _, exists := out[name]; !exists {
				out[name] = v
			}
		}
	}
	return out
}

func localName(tag string) string {
	if i := strings.LastIndex(tag, ":"); i >= 0 {
		return tag[i+1:]
	}
	return tag
}

var dateRe = regexp.MustCompile(`[0-9]{4}[-/年][0-9]{1,2}[-/月][0-9]{1,2}`)

func cleanNo(s string) string {
	return strings.TrimSpace(s)
}

func cleanDate(s string) string {
	if dateRe.MatchString(s) {
		return s
	}
	return strings.TrimSpace(s)
}
