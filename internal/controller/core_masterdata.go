package controller

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/net/ghttp"
)

// ---------- Core K5 主数据基础实体（AK5.1–AK5.5） ----------
// 口径：主数据是「雏形」——只做类型、记录、稳定 ID、关联、导出，供 ①②③ 引用；
// 实体解析与去重（G6）属于产品② 深化范围，本轮不做，避免越界做重。

func MasterDataTypeList(r *ghttp.Request) {
	rows, err := g.DB().Model("master_data_type").Order("sort_order asc").All()
	if err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	r.Response.WriteJsonExit(g.Map{"code": 0, "data": rows})
}

// newMdUid AK5.3：随机 + 类型前缀的稳定唯一 ID，一次生成终身不变，不随名称/主体变动
func newMdUid(mdType string) (string, error) {
	buf := make([]byte, 8)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	prefix := strings.ToUpper(mdType)
	if len(prefix) > 6 {
		prefix = prefix[:6]
	}
	return fmt.Sprintf("MD-%s-%s", prefix, hex.EncodeToString(buf)), nil
}

func MasterDataList(r *ghttp.Request) {
	m := g.DB().Model("master_data")
	if v := r.Get("mdType").String(); v != "" {
		m = m.Where("md_type", v)
	}
	if v := r.Get("keyword").String(); v != "" {
		m = m.Where("name LIKE ? OR code LIKE ? OR md_uid LIKE ?", "%"+v+"%", "%"+v+"%", "%"+v+"%")
	}
	if v := r.Get("status").String(); v != "" {
		m = m.Where("status", v)
	}
	m = scopeFilter(m, r)
	rows, err := m.Order("id desc").Limit(500).All()
	if err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	r.Response.WriteJsonExit(g.Map{"code": 0, "data": rows})
}

type mdReq struct {
	MdType        string                 `json:"mdType"`
	Code          string                 `json:"code"`
	Name          string                 `json:"name"`
	LegalEntityId int64                  `json:"legalEntityId"`
	Attributes    map[string]interface{} `json:"attributes"`
	Status        string                 `json:"status"`
}

func mdValidate(req mdReq) error {
	if req.MdType == "" {
		return fmt.Errorf("mdType 必填")
	}
	row, err := g.DB().Model("master_data_type").Where("code", req.MdType).One()
	if err != nil || row.IsEmpty() {
		return fmt.Errorf("主数据类型 %s 未预置", req.MdType)
	}
	if strings.TrimSpace(req.Name) == "" {
		return fmt.Errorf("名称必填")
	}
	return nil
}

// MasterDataCreate AK5.2 / AK5.3
func MasterDataCreate(r *ghttp.Request) {
	var req mdReq
	if err := r.Parse(&req); err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	if err := mdValidate(req); err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	uid, err := newMdUid(req.MdType)
	if err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	if req.LegalEntityId <= 0 {
		req.LegalEntityId = scopeEntity(r)
	}
	if req.Status == "" {
		req.Status = "active"
	}
	policyRecord := map[string]interface{}{
		"mdType": req.MdType, "name": req.Name, "code": req.Code, "status": req.Status,
	}
	for k, v := range req.Attributes {
		policyRecord[k] = v
	}
	if policyGuard(r, "master_data", 0, req.Name, policyRecord) {
		return
	}
	attrs, _ := json.Marshal(req.Attributes)
	res, err := g.DB().Model("master_data").Data(g.Map{
		"md_uid": uid, "md_type": req.MdType, "code": strings.TrimSpace(req.Code),
		"name": strings.TrimSpace(req.Name), "legal_entity_id": req.LegalEntityId,
		"attributes": string(attrs), "status": req.Status, "owner": r.GetCtxVar("user").String(),
		"created_at": time.Now(), "updated_at": time.Now(),
	}).Insert()
	if err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	id, _ := res.LastInsertId()
	audit(r, "master_data.create", "master_data", id, g.Map{"mdUid": uid, "mdType": req.MdType, "name": req.Name})
	r.Response.WriteJsonExit(g.Map{"code": 0, "data": g.Map{"id": id, "mdUid": uid}})
}

// MasterDataUpdate 编辑（md_uid 与类型不可改，保证下游引用稳定）
func MasterDataUpdate(r *ghttp.Request) {
	id := r.Get("id").Int64()
	old, err := g.DB().Model("master_data").Where("id", id).One()
	if err != nil || old.IsEmpty() {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": "主数据记录不存在"})
		return
	}
	var req mdReq
	if err := r.Parse(&req); err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	req.MdType = old["md_type"].String()
	if err := mdValidate(req); err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	attrs, _ := json.Marshal(req.Attributes)
	data := g.Map{
		"code": strings.TrimSpace(req.Code), "name": strings.TrimSpace(req.Name),
		"attributes": string(attrs), "updated_at": time.Now(),
	}
	if req.Status == "active" || req.Status == "disabled" {
		data["status"] = req.Status
	}
	if req.LegalEntityId > 0 {
		data["legal_entity_id"] = req.LegalEntityId
	}
	if _, err := g.DB().Model("master_data").Data(data).Where("id", id).Update(); err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	audit(r, "master_data.update", "master_data", id, g.Map{"mdUid": old["md_uid"].String(), "after": data})
	r.Response.WriteJsonExit(g.Map{"code": 0, "message": "ok"})
}

func MasterDataDelete(r *ghttp.Request) {
	id := r.Get("id").Int64()
	row, err := g.DB().Model("master_data").Where("id", id).One()
	if err != nil || row.IsEmpty() {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": "主数据记录不存在"})
		return
	}
	if _, err := g.DB().Model("master_data_link").Where("from_id", id).Delete(); err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	if _, err := g.DB().Model("master_data_link").Where("to_id", id).Delete(); err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	if _, err := g.DB().Model("master_data").Where("id", id).Delete(); err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	audit(r, "master_data.delete", "master_data", id, g.Map{"mdUid": row["md_uid"].String()})
	r.Response.WriteJsonExit(g.Map{"code": 0, "message": "ok"})
}

// MasterDataLinkCreate AK5.4：主数据间建立关系（如供应商 → 所属组织）
func MasterDataLinkCreate(r *ghttp.Request) {
	var req struct {
		FromId   int64  `json:"fromId"`
		ToId     int64  `json:"toId"`
		Relation string `json:"relation"`
	}
	if err := r.Parse(&req); err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	if req.FromId <= 0 || req.ToId <= 0 || req.FromId == req.ToId {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": "fromId / toId 必填且不能相同"})
		return
	}
	for _, id := range []int64{req.FromId, req.ToId} {
		row, _ := g.DB().Model("master_data").Where("id", id).One()
		if row.IsEmpty() {
			r.Response.WriteJsonExit(g.Map{"code": 1, "message": fmt.Sprintf("主数据 #%d 不存在", id)})
			return
		}
	}
	if req.Relation == "" {
		req.Relation = "belongs_to"
	}
	res, err := g.DB().Model("master_data_link").Data(g.Map{
		"from_id": req.FromId, "to_id": req.ToId, "relation": req.Relation, "created_at": time.Now(),
	}).Insert()
	if err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	id, _ := res.LastInsertId()
	audit(r, "master_data.link", "master_data_link", id, g.Map{"fromId": req.FromId, "toId": req.ToId, "relation": req.Relation})
	r.Response.WriteJsonExit(g.Map{"code": 0, "data": g.Map{"id": id}})
}

func MasterDataLinkList(r *ghttp.Request) {
	m := g.DB().Model("master_data_link l").
		LeftJoin("master_data f", "f.id = l.from_id").
		LeftJoin("master_data t", "t.id = l.to_id").
		Fields("l.id", "l.from_id", "l.to_id", "l.relation", "l.created_at",
			"f.name as from_name", "f.md_uid as from_uid", "t.name as to_name", "t.md_uid as to_uid")
	if v := r.Get("fromId").Int64(); v > 0 {
		m = m.Where("l.from_id", v)
	}
	rows, err := m.Order("l.id desc").Limit(500).All()
	if err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	r.Response.WriteJsonExit(g.Map{"code": 0, "data": rows})
}

func MasterDataLinkDelete(r *ghttp.Request) {
	id := r.Get("id").Int64()
	if _, err := g.DB().Model("master_data_link").Where("id", id).Delete(); err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	audit(r, "master_data.unlink", "master_data_link", id, nil)
	r.Response.WriteJsonExit(g.Map{"code": 0, "message": "ok"})
}

// MasterDataExport AK5.5：导出供下游产品（合同相对方 / 发票购销方）引用
func MasterDataExport(r *ghttp.Request) {
	m := scopeFilter(g.DB().Model("master_data"), r)
	if v := r.Get("mdType").String(); v != "" {
		m = m.Where("md_type", v)
	}
	rows, err := m.Order("id asc").Limit(5000).All()
	if err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	headers := []string{"稳定ID", "类型", "编码", "名称", "归属主体ID", "状态", "扩展属性", "创建时间"}
	data := make([][]string, 0, len(rows))
	for _, row := range rows {
		data = append(data, []string{
			row["md_uid"].String(), row["md_type"].String(), row["code"].String(), row["name"].String(),
			row["legal_entity_id"].String(), row["status"].String(), row["attributes"].String(), row["created_at"].String(),
		})
	}
	audit(r, "master_data.export", "master_data", 0, g.Map{"count": len(rows), "mdType": r.Get("mdType").String()})
	if err := sendXLSX(r, "master-data.xlsx", "主数据", headers, data); err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
}
