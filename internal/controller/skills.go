package controller

import (
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/net/ghttp"

	"vouchchain/pkg/filestore"
	"vouchchain/pkg/skills"
)

// Agent Skill 包接口：
//   POST   /api/skills           上传 zip（磁盘权威源 + 文件中心备份）
//   GET    /api/skills           列表
//   PUT    /api/skills/{id}/toggle 启停
//   DELETE /api/skills/{id}      删除（目录与库记录）
//   GET    /api/skills/prompt?message= 预览触发匹配（调试用）

func SkillUpload(r *ghttp.Request) {
	file := r.GetUploadFile("file")
	if file == nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": "缺少上传文件（zip）"})
		return
	}
	fh, err := file.Open()
	if err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	defer fh.Close()
	data, err := io.ReadAll(fh)
	if err != nil || len(data) == 0 || len(data) > 20<<20 {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": "文件为空或超过 20MB"})
		return
	}
	creator := r.GetCtxVar("user").String()
	id, err := skills.Install(r.Context(), creator, data)
	if err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	// zip 原件登记文件中心（biz_type=skill）：备份 + 双后端（磁盘/SeaweedFS）
	store := filestore.Pick(r.Context())
	row, _ := g.DB().Model("skill_pack").Where("id", id).One()
	name := row["name"].String()
	key := "skill/" + name + "/" + file.Filename
	_ = store.Put(r.Context(), key, data, "application/zip")
	fRes, _ := g.DB().Model("file_entry").Data(g.Map{
		"biz_type": "skill", "biz_id": id,
		"filename": file.Filename, "content_type": "application/zip",
		"size_bytes": len(data), "sha256": fmt.Sprintf("%x", sha256Bytes(data)),
		"storage": store.Kind(), "storage_key": key,
		"uploader": creator, "created_at": time.Now(),
	}).Insert()
	var fileId int64
	if fRes != nil {
		fileId, _ = fRes.LastInsertId()
		g.DB().Model("skill_pack").Data(g.Map{"file_id": fileId}).Where("id", id).Update()
	}
	audit(r, "skill.upload", "skill_pack", id, g.Map{"name": name, "storage": store.Kind()})
	r.Response.WriteJsonExit(g.Map{"code": 0, "data": g.Map{"id": id, "name": name, "fileId": fileId}})
}

func sha256Bytes(b []byte) []byte {
	h := sha256.Sum256(b)
	return h[:]
}

func SkillList(r *ghttp.Request) {
	list, err := skills.List(r.Context())
	if err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	r.Response.WriteJsonExit(g.Map{"code": 0, "data": list})
}

func SkillToggle(r *ghttp.Request) {
	id := r.Get("id").Int64()
	var req struct {
		Enabled bool `json:"enabled"`
	}
	if err := r.Parse(&req); err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	g.DB().Model("skill_pack").Data(g.Map{"enabled": req.Enabled}).Where("id", id).Update()
	audit(r, "skill.toggle", "skill_pack", id, g.Map{"enabled": req.Enabled})
	r.Response.WriteJsonExit(g.Map{"code": 0})
}

func SkillDelete(r *ghttp.Request) {
	id := r.Get("id").Int64()
	row, err := g.DB().Model("skill_pack").Where("id", id).One()
	if err == nil && !row.IsEmpty() {
		_ = os.RemoveAll(row["path"].String()) // 磁盘权威源一并删除
		// 文件中心备份保留（可恢复）；按需清理存储对象：
		if fr, ferr := g.DB().Model("file_entry").Where("id", row["file_id"].Int64()).One(); ferr == nil && !fr.IsEmpty() {
			_ = filestore.Pick(r.Context()).Delete(r.Context(), fr["storage_key"].String())
			g.DB().Model("file_entry").Where("id", fr["id"].Int64()).Delete()
		}
	}
	g.DB().Model("skill_pack").Where("id", id).Delete()
	audit(r, "skill.delete", "skill_pack", id, nil)
	r.Response.WriteJsonExit(g.Map{"code": 0})
}
