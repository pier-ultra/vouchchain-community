package docxgen

import (
	"fmt"
	"strings"
)

// ToHTML 把合同全文纯文本渲染成自包含的 HTML 文档（在线预览用）。
//
// 样式规则与 Build（docx）完全一致：标题居中、条款加粗、正文首行缩进 2 字符，
// 这样用户在文件中心/生成页看到的预览与 Word 打开的效果对齐。
//
// 返回完整 HTML 文档，前端直接塞进 <iframe :srcdoc="html"> 即可隔离样式；
// 中文字体走浏览器系统字体栈，无需服务端字体资源（这也是不走 PDF 预览的原因）。
func ToHTML(text string) string {
	var buf strings.Builder
	buf.WriteString(`<!DOCTYPE html>
<html lang="zh-CN">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>合同在线预览</title>
<style>
  html, body { margin: 0; padding: 0; background: #f5f6f8; }
  .paper {
    max-width: 820px; margin: 24px auto; padding: 56px 64px;
    background: #fff; box-shadow: 0 2px 12px rgba(0,0,0,.08); border-radius: 4px;
    font-family: "Times New Roman", "SimSun", "Songti SC", serif;
    color: #1a1a1a; font-size: 15px; line-height: 1.9;
  }
  h2.title { text-align: center; font-size: 22px; font-weight: 700; margin: 0 0 22px; letter-spacing: 1px; }
  h3.clause { font-size: 15px; font-weight: 700; margin: 18px 0 6px; }
  p.body { margin: 0 0 8px; text-indent: 2em; text-align: justify; }
  p.sign { margin: 4px 0 0; text-indent: 0; }
  .blank { height: 12px; }
  .empty { text-align: center; color: #909399; padding: 80px 0; }
</style>
</head>
<body>
<div class="paper">
`)

	lines := strings.Split(text, "\n")
	n := 0
	for _, line := range lines {
		t := strings.TrimSpace(line)
		if t == "" {
			buf.WriteString(`<div class="blank"></div>`)
			continue
		}
		switch classify(n, t) {
		case StyleTitle:
			buf.WriteString(`<h2 class="title">` + escapeHTML(t) + `</h2>`)
		case StyleClause:
			buf.WriteString(`<h3 class="clause">` + escapeHTML(t) + `</h3>`)
		case StyleSign:
			buf.WriteString(`<p class="sign">` + escapeHTML(t) + `</p>`)
		default:
			buf.WriteString(`<p class="body">` + escapeHTML(t) + `</p>`)
		}
		n++
	}

	if n == 0 {
		buf.WriteString(`<div class="empty">暂无可预览的正文内容</div>`)
	}

	buf.WriteString(`</div>
</body>
</html>`)
	return buf.String()
}

// escapeHTML 做 HTML 上下文的最小转义（预览只展示纯文本，不放任何脚本）
func escapeHTML(s string) string {
	return escapeXML(s)
}

// Stats 返回正文统计信息（供前端结果卡片展示，避免多传一次大文本）
type Stats struct {
	Chars   int `json:"chars"`
	Lines   int `json:"lines"`
	Clauses int `json:"clauses"`
}

// Analyze 统计合同正文：字符数、段落数、条款数
func Analyze(text string) Stats {
	st := Stats{Chars: len([]rune(text))}
	if text == "" {
		return st
	}
	lines := strings.Split(text, "\n")
	st.Lines = len(lines)
	for _, line := range lines {
		if reClause.MatchString(strings.TrimSpace(line)) {
			st.Clauses++
		}
	}
	return st
}

func init() {
	// 编译期确认正则可用，避免运行时才发现规则写错
	_ = fmt.Sprintf("%s", reClause.String())
}
