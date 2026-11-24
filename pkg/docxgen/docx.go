package docxgen

import (
	"archive/zip"
	"bytes"
	"fmt"
	"regexp"
	"strings"
	"time"
)

// docxgen：合同全文纯文本 → .docx 字节流（零第三方依赖：archive/zip + WordprocessingML）。
//
// 为什么自己做而不引第三方库：
//   - 项目无中文字体资源，PDF 生成会出方块，先不做；
//   - .docx 本质是 ZIP 包（[Content_Types].xml + _rels/.rels + word/document.xml），
//     用标准库手写即可产出 Word/WPS 可直接打开的文件，避免引入带商业许可的 docx 库。
//
// 段落样式按内容启发式分派（合同全文的行结构相对固定）：
//   - 首行且较短            → 居中标题（加粗 16pt）
//   - "第X条 / 一、 / 1. / （一）" → 加粗条款标题（左对齐）
//   - "甲方 / 乙方 / 签署 / 日期"   → 落款（左对齐，不缩进）
//   - 其余                    → 正文（首行缩进 2 字符）

// 段落样式常量
const (
	StyleTitle  = iota // 合同标题
	StyleClause        // 条款标题
	StyleSign          // 落款签署
	StyleBody          // 正文段落
)

const fontTag = `<w:rFonts w:ascii="Times New Roman" w:hAnsi="Times New Roman" w:eastAsia="宋体"/>`

// 条款编号：第X条 / 第X章 / 一、 / 1. / 1) / （一）
var reClause = regexp.MustCompile(
	`^(第[一二三四五六七八九十百零]+[条章节]|[一二三四五六七八九十]+[、.．]|\d+[.、．)）]|（[一二三四五六七八九十]+）|\([一二三四五六七八九十]+\))`)

// 落款行：甲方（…）/ 乙方：… / 签订日期：…。
// 关键收紧：甲方|乙方 后必须紧跟「（」或「：」，否则视为正文，
// 避免「乙方向甲方供应…」「乙方逾期交货的…」这类正文被误判成落款。
var reSign = regexp.MustCompile(
	`^(（甲方）|（乙方）|（丙方）|\(甲方\)|\(乙方\)|甲方（|乙方（|丙方（|甲方：|乙方：|丙方：|签署日期|签订日期|生效日期|日期：)`)

// escapeXML 做 WordprocessingML 必需的五字符转义
func escapeXML(s string) string {
	r := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;", "'", "&apos;")
	return r.Replace(s)
}

// classify 判定一段文本应使用的段落样式（idx 为段落序号，0 为首段）
func classify(idx int, line string) int {
	t := strings.TrimSpace(line)
	if t == "" {
		return StyleBody
	}
	if idx == 0 && len([]rune(t)) <= 40 && !strings.Contains(t, "。") {
		return StyleTitle
	}
	if reClause.MatchString(t) {
		return StyleClause
	}
	if reSign.MatchString(t) {
		return StyleSign
	}
	return StyleBody
}

// paraXML 生成一个 <w:p> 段落
func paraXML(style int, line string) string {
	t := strings.TrimSpace(line)
	if t == "" {
		return `<w:p/>`
	}
	safe := escapeXML(t)
	var buf strings.Builder

	switch style {
	case StyleTitle:
		buf.WriteString(`<w:p><w:pPr><w:jc w:val="center"/><w:spacing w:after="240"/>`)
		buf.WriteString(`<w:rPr>` + fontTag + `<w:b/><w:sz w:val="32"/></w:rPr></w:pPr>`)
		buf.WriteString(`<w:r><w:rPr>` + fontTag + `<w:b/><w:sz w:val="32"/></w:rPr>`)
		buf.WriteString(`<w:t xml:space="preserve">` + safe + `</w:t></w:r></w:p>`)
	case StyleClause:
		buf.WriteString(`<w:p><w:pPr><w:spacing w:before="120" w:after="60"/>`)
		buf.WriteString(`<w:rPr>` + fontTag + `<w:b/><w:sz w:val="24"/></w:rPr></w:pPr>`)
		buf.WriteString(`<w:r><w:rPr>` + fontTag + `<w:b/><w:sz w:val="24"/></w:rPr>`)
		buf.WriteString(`<w:t xml:space="preserve">` + safe + `</w:t></w:r></w:p>`)
	case StyleSign:
		buf.WriteString(`<w:p><w:pPr>` + fontTag + `</w:pPr>`)
		buf.WriteString(`<w:r><w:rPr>` + fontTag + `<w:sz w:val="24"/></w:rPr>`)
		buf.WriteString(`<w:t xml:space="preserve">` + safe + `</w:t></w:r></w:p>`)
	default: // StyleBody 正文：首行缩进 2 字符（480 twips）
		buf.WriteString(`<w:p><w:pPr><w:ind w:firstLine="480" w:firstLineChars="200"/>`)
		buf.WriteString(`<w:spacing w:line="360" w:lineRule="auto"/>`)
		buf.WriteString(`<w:rPr>` + fontTag + `<w:sz w:val="24"/></w:rPr></w:pPr>`)
		buf.WriteString(`<w:r><w:rPr>` + fontTag + `<w:sz w:val="24"/></w:rPr>`)
		buf.WriteString(`<w:t xml:space="preserve">` + safe + `</w:t></w:r></w:p>`)
	}
	return buf.String()
}

const contentTypesXML = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">
<Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>
<Default Extension="xml" ContentType="application/xml"/>
<Override PartName="/word/document.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"/>
<Override PartName="/docProps/core.xml" ContentType="application/vnd.openxmlformats-package.core-properties+xml"/>
</Types>`

const rootRelsXML = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="word/document.xml"/>
<Relationship Id="rId2" Type="http://schemas.openxmlformats.org/package/2006/relationships/metadata/core-properties" Target="docProps/core.xml"/>
</Relationships>`

const docRelsXML = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"/>`

const docXMLHead = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body>`

const docXMLTail = `<w:sectPr><w:pgSz w:w="11906" w:h="16838"/><w:pgMar w:top="1440" w:right="1800" w:bottom="1440" w:left="1800"/></w:sectPr></w:body></w:document>`

// Build 把合同全文纯文本转成 .docx 文件字节
func Build(text string) ([]byte, error) {
	if strings.TrimSpace(text) == "" {
		return nil, fmt.Errorf("合同正文为空，无法生成 docx")
	}

	var body strings.Builder
	idx := 0
	for _, line := range strings.Split(text, "\n") {
		body.WriteString(paraXML(classify(idx, line), line))
		idx++
	}

	title := strings.TrimSpace(strings.SplitN(text, "\n", 2)[0])
	if len([]rune(title)) > 60 {
		title = title[:12] + "…"
	}
	core := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<cp:coreProperties xmlns:cp="http://schemas.openxmlformats.org/package/2006/metadata/core-properties" xmlns:dc="http://purl.org/dc/elements/1.1/" xmlns:dcterms="http://purl.org/dc/terms/" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance">
<dc:title>%s</dc:title>
<dc:creator>VouchChain AI</dc:creator>
<cp:lastModifiedBy>VouchChain AI</cp:lastModifiedBy>
<dcterms:created xsi:type="dcterms:W3CDTF">%s</dcterms:created>
<dcterms:modified xsi:type="dcterms:W3CDTF">%s</dcterms:modified>
</cp:coreProperties>`,
		escapeXML(title), time.Now().UTC().Format("2006-01-02T15:04:05Z"), time.Now().UTC().Format("2006-01-02T15:04:05Z"))

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	writestr := func(name, content string) error {
		w, err := zw.Create(name)
		if err != nil {
			return err
		}
		_, err = w.Write([]byte(content))
		return err
	}
	if err := writestr("[Content_Types].xml", contentTypesXML); err != nil {
		return nil, fmt.Errorf("写入 [Content_Types].xml 失败: %w", err)
	}
	if err := writestr("_rels/.rels", rootRelsXML); err != nil {
		return nil, fmt.Errorf("写入 _rels/.rels 失败: %w", err)
	}
	if err := writestr("docProps/core.xml", core); err != nil {
		return nil, fmt.Errorf("写入 docProps/core.xml 失败: %w", err)
	}
	if err := writestr("word/_rels/document.xml.rels", docRelsXML); err != nil {
		return nil, fmt.Errorf("写入 word/_rels/document.xml.rels 失败: %w", err)
	}
	if err := writestr("word/document.xml", docXMLHead+body.String()+docXMLTail); err != nil {
		return nil, fmt.Errorf("写入 word/document.xml 失败: %w", err)
	}
	if err := zw.Close(); err != nil {
		return nil, fmt.Errorf("关闭 docx 包失败: %w", err)
	}
	return buf.Bytes(), nil
}
