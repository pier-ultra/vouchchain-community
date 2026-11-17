package controller

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"vouchchain/pkg/mdmatch"
	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/net/ghttp"
)

// ---------- 产品② US-G6 MDM 实体解析与去重（AG6.1–AG6.5） ----------
// 深化 Core K5：可配置匹配字段/阈值 → 扫描疑似重复 → 合并建议 → 确认合并保留权威版本 +
// 稳定 ID 映射 + 历史关联关系重定向。匹配内核在 pkg/mdmatch，零 DB 依赖。

func mdCandidate(row gdb.Record) mdmatch.Candidate {
	attrs := map[string]interface{}{}
	_ = json.Unmarshal([]byte(row["attributes"].String()), &attrs)
	return mdmatch.Candidate{
		ID: row["id"].Int64(), UID: row["md_uid"].String(), Code: row["code"].String(),
		Name: row["name"].String(), Status: row["status"].String(), Attributes: attrs,
	}
}

func loadMatchRule(id int64) (int64, string, mdmatch.Rule, error) {
	m := g.DB().Model("md_match_rule").Where("enabled", true)
	if id > 0 {
		m = m.Where("id", id)
	}
	row, err := m.Order("id asc").One()
	if err != nil || row.IsEmpty() {
		return 0, "", mdmatch.Rule{}, fmt.Errorf("未找到启用的匹配规则")
	}
	fields := []string{}
	_ = json.Unmarshal([]byte(row["fields"].String()), &fields)
	return row["id"].Int64(), row["md_type"].String(), mdmatch.Rule{
		Fields: fields, Threshold: row["threshold"].Float64(),
	}, nil
}

func MdmRuleList(r *ghttp.Request) {
	rows, err := g.DB().Model("md_match_rule").Order("id asc").All()
	if err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	r.Response.WriteJsonExit(g.Map{"code": 0, "data": rows})
}

// MdmRuleSave AG6.5：匹配字段与相似度阈值可配置（id=0 新建，否则更新）
func MdmRuleSave(r *ghttp.Request) {
	var req struct {
		Id        int64    `json:"id"`
		Name      string   `json:"name"`
		MdType    string   `json:"mdType"`
		Fields    []string `json:"fields"`
		Threshold float64  `json:"threshold"`
		Enabled   *bool    `json:"enabled"`
	}
	if err := r.Parse(&req); err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	if strings.TrimSpace(req.Name) == "" || len(req.Fields) == 0 {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": "规则名称与匹配字段必填"})
		return
	}
	if req.Threshold < 1 || req.Threshold > 100 {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": "相似度阈值须在 1–100 之间"})
		return
	}
	if req.MdType == "" {
		req.MdType = "supplier"
	}
	fj, _ := json.Marshal(req.Fields)
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	data := g.Map{
		"name": req.Name, "md_type": req.MdType, "fields": string(fj),
		"threshold": req.Threshold, "enabled": enabled, "updated_at": time.Now(),
	}
	if req.Id > 0 {
		if _, err := g.DB().Model("md_match_rule").Data(data).Where("id", req.Id).Update(); err != nil {
			r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
			return
		}
		audit(r, "mdm.rule.update", "md_match_rule", req.Id, g.Map{"threshold": req.Threshold, "fields": req.Fields})
		r.Response.WriteJsonExit(g.Map{"code": 0, "data": g.Map{"id": req.Id}})
		return
	}
	res, err := g.DB().Model("md_match_rule").Data(data).Insert()
	if err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	id, _ := res.LastInsertId()
	audit(r, "mdm.rule.create", "md_match_rule", id, g.Map{"threshold": req.Threshold, "fields": req.Fields})
	r.Response.WriteJsonExit(g.Map{"code": 0, "data": g.Map{"id": id}})
}

// MdmScan AG6.1/AG6.2：扫描疑似重复并生成合并建议（同类型两两比对，已存在 pair 刷新分数）
func MdmScan(r *ghttp.Request) {
	var req struct {
		RuleId int64 `json:"ruleId"`
	}
	_ = r.Parse(&req)
	ruleId, mdType, matchRule, err := loadMatchRule(req.RuleId)
	if err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	rows, err := g.DB().Model("master_data").Where("md_type", mdType).Where("status", "active").Order("id asc").Limit(500).All()
	if err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	pairs, created, compared := 0, 0, 0
	for i := 0; i < len(rows); i++ {
		for j := i + 1; j < len(rows); j++ {
			compared++
			a, b := mdCandidate(rows[i]), mdCandidate(rows[j])
			m := mdmatch.Compare(a, b, matchRule)
			if !m.Duplicate {
				continue
			}
			pairs++
			survivor, _, suggestion := mdmatch.SuggestSurvivor(a, b)
			_ = survivor
			reasons, _ := json.Marshal(m.Reasons)
			leftId, rightId := a.ID, b.ID
			exist, _ := g.DB().Model("md_duplicate_pair").Where("left_id", leftId).Where("right_id", rightId).One()
			if !exist.IsEmpty() {
				// 已合并/已忽略的不回写状态，只刷新最近一次相似度明细
				_, _ = g.DB().Model("md_duplicate_pair").Data(g.Map{
					"score": m.Score, "reasons": string(reasons), "suggestion": suggestion,
					"match_rule_id": ruleId,
				}).Where("id", exist["id"].Int64()).Update()
				continue
			}
			_, err := g.DB().Model("md_duplicate_pair").Data(g.Map{
				"match_rule_id": ruleId, "left_id": leftId, "right_id": rightId,
				"left_uid": a.UID, "right_uid": b.UID, "left_name": a.Name, "right_name": b.Name,
				"score": m.Score, "reasons": string(reasons), "suggestion": suggestion,
				"status": "pending", "created_at": time.Now(),
			}).Insert()
			if err == nil {
				created++
			}
		}
	}
	audit(r, "mdm.scan", "md_match_rule", ruleId, g.Map{"compared": compared, "pairs": pairs, "created": created})
	r.Response.WriteJsonExit(g.Map{"code": 0, "data": g.Map{
		"mdType": mdType, "records": len(rows), "compared": compared,
		"pairs": pairs, "created": created, "threshold": matchRule.Threshold,
	}})
}

func MdmDuplicateList(r *ghttp.Request) {
	m := g.DB().Model("md_duplicate_pair")
	if v := r.Get("status").String(); v != "" {
		m = m.Where("status", v)
	}
	rows, err := m.Order("score desc, id desc").Limit(300).All()
	if err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	out := make([]g.Map, 0, len(rows))
	for _, row := range rows {
		m := row.Map()
		rs := []string{}
		_ = json.Unmarshal([]byte(row["reasons"].String()), &rs)
		m["reasons"] = rs
		out = append(out, m)
	}
	r.Response.WriteJsonExit(g.Map{"code": 0, "data": out})
}

// MdmMerge AG6.3/AG6.4：确认合并 → 权威版本 + 稳定 ID 映射 + 历史关联关系重定向（不丢关系）
func MdmMerge(r *ghttp.Request) {
	pairId := r.Get("id").Int64()
	var req struct {
		SurvivorId int64  `json:"survivorId"`
		Note       string `json:"note"`
	}
	if err := r.Parse(&req); err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	pair, err := g.DB().Model("md_duplicate_pair").Where("id", pairId).One()
	if err != nil || pair.IsEmpty() {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": "重复对不存在"})
		return
	}
	if pair["status"].String() != "pending" {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": "该重复对已处理，不能重复合并"})
		return
	}
	leftId, rightId := pair["left_id"].Int64(), pair["right_id"].Int64()
	la, err := g.DB().Model("master_data").Where("id", leftId).One()
	lb, err2 := g.DB().Model("master_data").Where("id", rightId).One()
	if err != nil || err2 != nil || la.IsEmpty() || lb.IsEmpty() {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": "主数据记录不存在（可能已删除）"})
		return
	}
	a, b := mdCandidate(la), mdCandidate(lb)
	survivor, loser, _ := mdmatch.SuggestSurvivor(a, b)
	// 人工可覆盖合并建议：显式指定哪一条作为权威记录（0 = 采用建议）
	if req.SurvivorId > 0 {
		switch req.SurvivorId {
		case leftId:
			survivor, loser = a, b
		case rightId:
			survivor, loser = b, a
		default:
			r.Response.WriteJsonExit(g.Map{"code": 1, "message": "survivorId 必须是该重复对中的一条记录"})
			return
		}
	}

	// 属性合并：权威记录缺失字段从被合并方补齐（不覆盖已有值）
	survivorRow := la
	if survivor.ID == rightId {
		survivorRow = lb
	}
	survivorAttrs := map[string]interface{}{}
	_ = json.Unmarshal([]byte(survivorRow["attributes"].String()), &survivorAttrs)
	for k, v := range loser.Attributes {
		if _, ok := survivorAttrs[k]; !ok || strings.TrimSpace(fmt.Sprint(survivorAttrs[k])) == "" {
			survivorAttrs[k] = v
		}
	}
	aj, _ := json.Marshal(survivorAttrs)

	// AG6.4 关系保持：把指向被合并方的关联边重定向到权威记录，并清理重定向后产生的重复边
	links, _ := g.DB().Model("master_data_link").Where("from_id", loser.ID).WhereOr("to_id", loser.ID).All()
	redirected, dropped := 0, 0
	for _, lk := range links {
		fromId, toId := lk["from_id"].Int64(), lk["to_id"].Int64()
		if fromId == loser.ID {
			fromId = survivor.ID
		}
		if toId == loser.ID {
			toId = survivor.ID
		}
		if fromId == toId { // 自环（两条重复实体互相关联）合并后无意义
			_, _ = g.DB().Model("master_data_link").Where("id", lk["id"].Int64()).Delete()
			dropped++
			continue
		}
		dup, _ := g.DB().Model("master_data_link").Where("from_id", fromId).Where("to_id", toId).Where("relation", lk["relation"].String()).One()
		if !dup.IsEmpty() {
			_, _ = g.DB().Model("master_data_link").Where("id", lk["id"].Int64()).Delete()
			dropped++
			continue
		}
		_, _ = g.DB().Model("master_data_link").Data(g.Map{"from_id": fromId, "to_id": toId}).Where("id", lk["id"].Int64()).Update()
		redirected++
	}

	// AG6.3 稳定 ID 映射：被合并方原 ID 永久可反查
	_, _ = g.DB().Model("md_id_mapping").Data(g.Map{
		"old_id": loser.ID, "old_uid": loser.UID, "old_name": loser.Name,
		"survivor_id": survivor.ID, "survivor_uid": survivor.UID,
		"pair_id": pairId, "merged_at": time.Now(),
	}).Insert()
	_, _ = g.DB().Model("master_data").Data(g.Map{
		"attributes": string(aj), "updated_at": time.Now(),
	}).Where("id", survivor.ID).Update()
	_, _ = g.DB().Model("master_data").Data(g.Map{
		"status": "merged", "updated_at": time.Now(),
	}).Where("id", loser.ID).Update()
	_, _ = g.DB().Model("md_duplicate_pair").Data(g.Map{
		"status": "merged", "handled_by": r.GetCtxVar("user").String(),
		"handled_note": req.Note, "handled_at": time.Now(),
	}).Where("id", pairId).Update()
	audit(r, "mdm.merge", "master_data", survivor.ID, g.Map{
		"loserId": loser.ID, "loserUid": loser.UID, "redirected": redirected, "dropped": dropped,
	})
	r.Response.WriteJsonExit(g.Map{"code": 0, "data": g.Map{
		"survivorId": survivor.ID, "survivorUid": survivor.UID,
		"mergedId": loser.ID, "mergedUid": loser.UID,
		"linksRedirected": redirected, "duplicateLinksDropped": dropped,
	}})
}

func MdmDuplicateIgnore(r *ghttp.Request) {
	id := r.Get("id").Int64()
	var req struct {
		Note string `json:"note"`
	}
	if err := r.Parse(&req); err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	if strings.TrimSpace(req.Note) == "" {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": "判为不同实体必须填写说明（合规留痕）"})
		return
	}
	if _, err := g.DB().Model("md_duplicate_pair").Data(g.Map{
		"status": "ignored", "handled_by": r.GetCtxVar("user").String(),
		"handled_note": req.Note, "handled_at": time.Now(),
	}).Where("id", id).Where("status", "pending").Update(); err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	audit(r, "mdm.ignore", "md_duplicate_pair", id, g.Map{"note": req.Note})
	r.Response.WriteJsonExit(g.Map{"code": 0, "message": "ok"})
}

func MdmMappingList(r *ghttp.Request) {
	rows, err := g.DB().Model("md_id_mapping").Order("id desc").Limit(300).All()
	if err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	r.Response.WriteJsonExit(g.Map{"code": 0, "data": rows})
}

func MdmSummary(r *ghttp.Request) {
	pending, _ := g.DB().Model("md_duplicate_pair").Where("status", "pending").Count()
	merged, _ := g.DB().Model("md_duplicate_pair").Where("status", "merged").Count()
	mappings, _ := g.DB().Model("md_id_mapping").Count()
	records, _ := g.DB().Model("master_data").Where("status", "active").Count()
	r.Response.WriteJsonExit(g.Map{"code": 0, "data": g.Map{
		"records": records, "pending": pending, "merged": merged, "mappings": mappings,
	}})
}
