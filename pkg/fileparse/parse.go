package fileparse

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	pdflib "github.com/ledongthuc/pdf"

	"vouchchain/pkg/kb"
)

// 支持的格式与解析策略（用户 11-14 拍板）：
// - .docx：zip 内 word/document.xml 提取文本（零第三方依赖）
// - .pdf ：仅文字版（有文本层）；扫描件无文本层 → 明确报错提示 OCR 后续接入，不静默出错
// - 其它扩展名：拒绝（AC1.2 格式校验）

var SupportedExts = []string{".pdf", ".docx", ".txt"}

func IsSupported(filename string) bool {
	ext := strings.ToLower(filepath.Ext(filename))
	for _, e := range SupportedExts {
		if ext == e {
			return true
		}
	}
	return false
}

// ExtractText 按扩展名分发解析，返回全文文本
func ExtractText(path string) (string, error) {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".docx":
		return extractDocx(path)
	case ".txt":
		// AI 生成合同等纯文本场景（生成 → 存 .txt → 进同一流水线）
		data, err := os.ReadFile(path)
		if err != nil {
			return "", err
		}
		out := strings.TrimSpace(string(data))
		if out == "" {
			return "", errors.New("文本内容为空")
		}
		return out, nil
	case ".pdf":
		return extractPdf(path)
	default:
		return "", fmt.Errorf("不支持的格式 %s（仅 PDF/DOCX）", ext)
	}
}

// extractDocx 解析 docx：遍历 word/document.xml 的 <w:t> 文本节点，<w:p> 作为段落分隔
func extractDocx(path string) (string, error) {
	r, err := zip.OpenReader(path)
	if err != nil {
		return "", fmt.Errorf("打开 docx 失败: %w", err)
	}
	defer r.Close()

	var docFile *zip.File
	for _, f := range r.File {
		if f.Name == "word/document.xml" {
			docFile = f
			break
		}
	}
	if docFile == nil {
		return "", errors.New("docx 缺少 word/document.xml（文件可能损坏）")
	}

	rc, err := docFile.Open()
	if err != nil {
		return "", err
	}
	defer rc.Close()

	var sb strings.Builder
	dec := xml.NewDecoder(rc)
	inText := false
	depthInP := 0
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", fmt.Errorf("解析 document.xml 失败: %w", err)
		}
		switch t := tok.(type) {
		case xml.StartElement:
			switch t.Name.Local {
			case "p":
				depthInP++
			case "t":
				inText = true
			case "br":
				sb.WriteString("\n")
			}
		case xml.CharData:
			if inText {
				sb.Write(t)
			}
		case xml.EndElement:
			switch t.Name.Local {
			case "p":
				if depthInP > 0 {
					depthInP--
				}
				sb.WriteString("\n")
			case "t":
				inText = false
			}
		}
	}
	out := strings.TrimSpace(sb.String())
	if out == "" {
		return "", errors.New("docx 未解析到文本内容")
	}
	return out, nil
}

// extractPdf 解析文字版 PDF（逐页 GetPlainText）；无文本层（扫描件）报明确错误
func extractPdf(path string) (string, error) {
	f, r, err := pdflib.Open(path)
	if err != nil {
		return "", fmt.Errorf("打开 pdf 失败: %w", err)
	}
	defer f.Close()

	var buf bytes.Buffer
	pages := r.NumPage()
	for i := 1; i <= pages; i++ {
		p := r.Page(i)
		if p.V.IsNull() {
			continue
		}
		txt, err := p.GetPlainText(nil)
		if err != nil {
			continue // 单页解析失败跳过，不影响整体
		}
		buf.WriteString(txt)
		buf.WriteString("\n\n")
	}
	out := strings.TrimSpace(buf.String())
	if out == "" {
		// 扫描件/图片版 PDF：无文本层 → 走 OCR（未配置 OCR 模型时明确报错提示，不静默失败）
		data, rerr := os.ReadFile(path)
		if rerr != nil {
			return "", fmt.Errorf("读取上传文件失败: %w", rerr)
		}
		ocrText, oerr := kb.OCRExtract(context.Background(), filepath.Base(path), data)
		if oerr != nil {
			return "", fmt.Errorf("PDF 无文本层且 OCR 不可用：%w", oerr)
		}
		if strings.TrimSpace(ocrText) == "" {
			return "", errors.New("OCR 已运行但未识别到文字；请检查扫描件质量")
		}
		return ocrText, nil
	}
	return out, nil
}

// SaveUpload 保存上传文件到目录，返回保存路径与原始文件名
func SaveUpload(dir string, filename string, data []byte) (string, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	base := filepath.Base(filename)
	stamp := time.Now().Format("20060102150405")
	safe := stamp + "_" + base
	full := filepath.Join(dir, safe)
	if err := os.WriteFile(full, data, 0o644); err != nil {
		return "", err
	}
	return full, nil
}
