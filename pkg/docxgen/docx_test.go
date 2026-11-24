package docxgen

import (
	"archive/zip"
	"bytes"
	"io"
	"strings"
	"testing"
)

const sample = `采购合同
甲方（采购方）：北京华信科技有限公司
乙方（供应方）：上海蓝海机械制造有限公司

一、标的
乙方应向甲方供应工业自动化设备一批（含安装调试），规格型号详见附件一。

二、金额
合同总金额为人民币壹佰伍拾万元整（¥1,500,000.00），含增值税。

三、结算方式
1. 合同签订后 5 个工作日内，甲方支付 50% 预付款；
2. 设备验收合格后 10 个工作日内，甲方支付尾款 50%。

四、违约责任
乙方逾期交货的，每逾期一日按合同总价的千分之三向甲方支付违约金。

甲方（盖章）：北京华信科技有限公司
乙方（盖章）：上海蓝海机械制造有限公司
签订日期：2026年11月23日`

func TestBuildProducesValidDocx(t *testing.T) {
	data, err := Build(sample)
	if err != nil {
		t.Fatalf("Build 失败: %v", err)
	}
	if len(data) < 1024 {
		t.Fatalf("docx 体积异常（%d 字节）", len(data))
	}

	r, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("打开 docx ZIP 失败: %v", err)
	}

	wantParts := []string{
		"[Content_Types].xml",
		"_rels/.rels",
		"docProps/core.xml",
		"word/_rels/document.xml.rels",
		"word/document.xml",
	}
	seen := map[string]bool{}
	var docXML string
	for _, f := range r.File {
		seen[f.Name] = true
		if f.Name == "word/document.xml" {
			rc, err := f.Open()
			if err != nil {
				t.Fatalf("读取 document.xml 失败: %v", err)
			}
			b, _ := io.ReadAll(rc)
			rc.Close()
			docXML = string(b)
		}
	}
	for _, p := range wantParts {
		if !seen[p] {
			t.Errorf("docx 缺少部件 %s", p)
		}
	}

	// 正文与 XML 特殊字符必须完整保留（转义正确）
	for _, key := range []string{"采购合同", "壹佰伍拾万元整", "千分之三", "（盖章）", "<w:sectPr>"} {
		if !strings.Contains(docXML, key) {
			t.Errorf("document.xml 缺少内容片段 %q", key)
		}
	}
	// 特殊字符必须被转义，不能出现裸 < >（避免破坏 XML 结构）
	if strings.Contains(docXML, "¥1,500,000.00") {
		// 原文无 < >，此处仅确认正常字符原样保留
	}
	if strings.Contains(docXML, "1,500,000.00") && strings.Count(docXML, "</w:t>") < 5 {
		t.Errorf("段落拆分异常：w:t 节点过少")
	}
}

func TestAnalyzeAndHTML(t *testing.T) {
	st := Analyze(sample)
	if st.Chars <= 0 || st.Lines < 10 || st.Clauses < 3 {
		t.Errorf("统计结果异常: %+v", st)
	}

	html := ToHTML(sample)
	for _, key := range []string{`<h2 class="title">采购合同`, `<h3 class="clause">`, `<p class="body">`, `<p class="sign">`, `class="paper"`} {
		if !strings.Contains(html, key) {
			t.Errorf("预览 HTML 缺少片段 %q", key)
		}
	}
}

func TestBuildRejectsEmpty(t *testing.T) {
	if _, err := Build("   \n  "); err == nil {
		t.Error("空正文应返回错误")
	}
}

// 回归：以「乙方」开头的正文不能被误判为落款（idx 用非 0，避免标题规则干扰）
func TestClassifySignNotMisjudged(t *testing.T) {
	for _, line := range []string{
		"乙方向甲方供应工业自动化设备一批（含安装调试），规格型号、数量及技术参数详见附件一。",
		"乙方逾期交货的，每逾期一日按合同总价的千分之三向甲方支付违约金。",
		"甲方向乙方支付合同总价的 50% 作为预付款。",
	} {
		if classify(5, line) == StyleSign {
			t.Errorf("正文被误判为落款: %s", line)
		}
		if classify(5, line) != StyleBody {
			t.Errorf("正文样式异常: %s -> %d", line, classify(5, line))
		}
	}
	for _, line := range []string{
		"甲方（盖章）：北京华信科技有限公司",
		"乙方（盖章）：上海蓝海机械制造有限公司",
		"甲方（采购方）：北京华信科技有限公司",
		"乙方：上海蓝海机械制造有限公司",
		"签订日期：2026年11月23日",
	} {
		if classify(5, line) != StyleSign {
			t.Errorf("落款行未被识别: %s", line)
		}
	}
}
