package controller

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/net/ghttp"

	"vouchchain/pkg/docxgen"
	"vouchchain/pkg/fileparse"
	"vouchchain/pkg/filestore"
)

// 文件中心（P0 内嵌 + SeaweedFS 可插拔，兼容模式）：
//   POST   /api/files                 上传（multipart：file + bizType + bizId；同 sha256 同业务自动去重复用）
//   GET    /api/files                 列表（bizType/bizId/keyword 过滤）
//   GET    /api/files/{id}/download   下载（attachment）
//   GET    /api/files/{id}/preview    预览（inline，PDF/图片浏览器直出）
//   DELETE /api/files/{id}            删除（连存储对象一起删）

func storeKey(bizType, filename, sha string) string {
	return fmt.Sprintf("%s/%s/%s_%s", bizType, time.Now().Format("200601"), sha[:16], filename)
}

func sha256Hex(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}

// FileUpload POST /api/files
func FileUpload(r *ghttp.Request) {
	file := r.GetUploadFile("file")
	if file == nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": "缺少 file 字段"})
		return
	}
	bizType := r.Get("bizType").String()
	if bizType == "" {
		bizType = "other"
	}
	bizId := r.Get("bizId").Int64()
	data, err := file.Open()
	if err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	defer data.Close()
	content := make([]byte, 0)
	tmp := make([]byte, 1024*256)
	for {
		n, rerr := data.Read(tmp)
		content = append(content, tmp[:n]...)
		if rerr != nil {
			break
		}
	}
	filename := file.Filename
	sha := sha256Hex(content)
	username := r.GetCtxVar("user").String()

	// 同 sha256 + 同业务 + 同文件名 → 复用已有记录（去重）
	exist, _ := g.DB().Model("file_entry").Where(g.Map{
		"sha256": sha, "biz_type": bizType, "biz_id": bizId, "filename": filename,
	}).One()
	if !exist.IsEmpty() {
		r.Response.WriteJsonExit(g.Map{"code": 0, "data": g.Map{"id": exist["id"].Int64(), "dedup": true}})
		return
	}

	store := filestore.Pick(r.Context())
	key := storeKey(bizType, sanitizeName(filename), sha)
	if err := store.Put(r.Context(), key, content, file.Header.Get("Content-Type")); err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": "存储失败: " + err.Error()})
		return
	}
	res, err := g.DB().Model("file_entry").Data(g.Map{
		"biz_type": bizType, "biz_id": bizId,
		"filename": filename, "content_type": file.Header.Get("Content-Type"),
		"size_bytes": len(content), "sha256": sha,
		"storage": store.Kind(), "storage_key": key,
		"uploader": username, "created_at": time.Now(),
	}).Insert()
	if err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	id, _ := res.LastInsertId()
	audit(r, "file.upload", "file", id, g.Map{"filename": filename, "size": len(content), "storage": store.Kind(), "biz": bizType})
	r.Response.WriteJsonExit(g.Map{"code": 0, "data": g.Map{"id": id, "dedup": false, "storage": store.Kind()}})
}

// FileList GET /api/files
func FileList(r *ghttp.Request) {
	m := g.DB().Model("file_entry")
	if v := r.Get("bizType").String(); v != "" {
		m = m.Where("biz_type", v)
	}
	if v := r.Get("bizId").Int64(); v > 0 {
		m = m.Where("biz_id", v)
	}
	if v := r.Get("keyword").String(); v != "" {
		m = m.WhereLike("filename", "%"+v+"%")
	}
	list, err := m.Order("id desc").Limit(200).All()
	if err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	out := make([]g.Map, 0, len(list))
	for _, row := range list {
		out = append(out, g.Map{
			"id": row["id"].Int64(), "bizType": row["biz_type"].String(), "bizId": row["biz_id"].Int64(),
			"filename": row["filename"].String(), "contentType": row["content_type"].String(),
			"sizeBytes": row["size_bytes"].Int64(), "storage": row["storage"].String(),
			"uploader": row["uploader"].String(), "createdAt": row["created_at"].String(),
		})
	}
	r.Response.WriteJsonExit(g.Map{"code": 0, "data": out})
}

func loadFileEntry(r *ghttp.Request) (int64, string, string, string, bool) {
	id := r.Get("id").Int64()
	row, err := g.DB().Model("file_entry").Where("id", id).One()
	if err != nil || row.IsEmpty() {
		r.Response.WriteStatusExit(404, "文件不存在")
		return 0, "", "", "", false
	}
	return id, row["filename"].String(), row["content_type"].String(), row["storage_key"].String(), true
}

// FileDownload GET /api/files/{id}/download
func FileDownload(r *ghttp.Request) {
	id, filename, ct, key, ok := loadFileEntry(r)
	if !ok {
		return
	}
	data, err := filestore.Pick(r.Context()).Get(r.Context(), key)
	if err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": "读取失败: " + err.Error()})
		return
	}
	r.Response.Header().Set("Content-Type", ct)
	r.Response.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename*=UTF-8''%s", urlEscape(filename)))
	r.Response.Header().Set("X-File-Id", fmt.Sprintf("%d", id))
	r.Response.Write(data)
}

// FilePreview GET /api/files/{id}/preview（inline）
func FilePreview(r *ghttp.Request) {
	_, _, ct, key, ok := loadFileEntry(r)
	if !ok {
		return
	}
	data, err := filestore.Pick(r.Context()).Get(r.Context(), key)
	if err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": "读取失败: " + err.Error()})
		return
	}
	r.Response.Header().Set("Content-Type", ct)
	r.Response.Header().Set("Content-Disposition", "inline")
	r.Response.Write(data)
}

// FilePreviewHTML GET /api/files/{id}/preview-html：在线预览（docx/txt/pdf → 富文本 HTML）
//
// 浏览器原生只认 PDF/图片，docx 会直接下载或报打不开。这里统一把正文提出来
// 渲染成自包含 HTML（标题居中 / 条款加粗 / 正文首行缩进，与 Word 打开效果对齐），
// 前端用 <iframe :srcdoc> 呈现，这样 docx、txt、pdf 都能在页内直接预览。
func FilePreviewHTML(r *ghttp.Request) {
	id, filename, _, key, ok := loadFileEntry(r)
	if !ok {
		return
	}
	data, err := filestore.Pick(r.Context()).Get(r.Context(), key)
	if err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": "读取失败: " + err.Error()})
		return
	}
	ext := strings.ToLower(filepath.Ext(filename))

	var text string
	switch ext {
	case ".docx", ".pdf":
		// fileparse.ExtractText 按路径解析，先落临时文件再 defer 清理
		tmp := filepath.Join(os.TempDir(), fmt.Sprintf("vcprev_%d_%d%s", id, time.Now().UnixNano(), ext))
		if werr := os.WriteFile(tmp, data, 0o644); werr != nil {
			r.Response.WriteJsonExit(g.Map{"code": 1, "message": "写入临时文件失败: " + werr.Error()})
			return
		}
		text, err = fileparse.ExtractText(tmp)
		os.Remove(tmp)
		if err != nil {
			r.Response.WriteJsonExit(g.Map{"code": 1, "message": "解析文件内容失败: " + err.Error()})
			return
		}
	default:
		// .txt 及其他纯文本：直接渲染
		text = strings.TrimSpace(string(data))
	}
	if text == "" {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": "该文件没有可预览的正文内容"})
		return
	}
	r.Response.WriteJsonExit(g.Map{"code": 0, "data": g.Map{
		"fileId":   id,
		"filename": filename,
		"chars":    len([]rune(text)),
		"html":     docxgen.ToHTML(text),
	}})
}

// FileDelete DELETE /api/files/{id}
func FileDelete(r *ghttp.Request) {
	id, _, _, key, ok := loadFileEntry(r)
	if !ok {
		return
	}
	if err := filestore.Pick(r.Context()).Delete(r.Context(), key); err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": "删除存储对象失败: " + err.Error()})
		return
	}
	g.DB().Model("file_entry").Where("id", id).Delete()
	audit(r, "file.delete", "file", id, g.Map{"key": key})
	r.Response.WriteJsonExit(g.Map{"code": 0})
}

func sanitizeName(s string) string {
	out := make([]rune, 0, len(s))
	for _, c := range s {
		if c == '/' || c == '\\' || c == '\r' || c == '\n' {
			out = append(out, '_')
		} else {
			out = append(out, c)
		}
	}
	if len(out) == 0 {
		return "file"
	}
	return string(out)
}

func urlEscape(s string) string {
	hexChars := "0123456789ABCDEF"
	b := []byte(s)
	out := make([]byte, 0, len(b)*3)
	for _, c := range b {
		if (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') ||
			c == '-' || c == '_' || c == '.' || c == '~' {
			out = append(out, c)
		} else {
			out = append(out, '%', hexChars[c>>4], hexChars[c&0xF])
		}
	}
	return string(out)
}
