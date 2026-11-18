package controller

import (
	"strings"
	"time"

	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/net/ghttp"
	"vouchchain/internal/middleware"
	"vouchchain/pkg/isolate"
	"vouchchain/pkg/partyprofile"
)

// ---------- 产品① US-C8（P1）相对方风险画像（AC8.1–AC8.5） ----------
// 数据只来自本系统：合同台账、已绑定的合同文件下的义务与条款。
// 聚合同样受 AG5 强隔离约束，不能借画像接口绕过数据权限看到别的主体/项目的数据。

type partyBucket struct {
	contracts   []partyprofile.ContractRow
	obligations []partyprofile.ObligationRow
	clauses     []partyprofile.ClauseRow
}

func partyVisible(r *ghttp.Request) func(int64, int64, string) bool {
	d := middleware.IsolationFor(r)
	return func(entity, dept int64, project string) bool {
		ok, _ := d.Allow(isolate.Object{EntityID: entity, DeptID: dept, Project: project})
		return ok
	}
}

func loadPartyBuckets(r *ghttp.Request) (map[string]*partyBucket, map[int64]string) {
	visible := partyVisible(r)
	buckets := map[string]*partyBucket{}
	ensure := func(name string) *partyBucket {
		if buckets[name] == nil {
			buckets[name] = &partyBucket{}
		}
		return buckets[name]
	}
	// 合同文件 → 相对方映射（用于把义务/条款归集到相对方）
	fileParty := map[int64]string{}
	fileRows, err := g.DB().Model("contract_file").Fields("id, counter_party, legal_entity_id, dept_id, project_code").All()
	if err == nil {
		for _, row := range fileRows {
			name := strings.TrimSpace(row["counter_party"].String())
			if name == "" || !visible(row["legal_entity_id"].Int64(), row["dept_id"].Int64(), row["project_code"].String()) {
				continue
			}
			fileParty[row["id"].Int64()] = name
		}
	}
	if rows, err := g.DB().Model("contract").Fields(
		"id, title, counter_party, amount, status, sign_date, legal_entity_id, dept_id, project_code").All(); err == nil {
		for _, row := range rows {
			name := strings.TrimSpace(row["counter_party"].String())
			if name == "" || !visible(row["legal_entity_id"].Int64(), row["dept_id"].Int64(), row["project_code"].String()) {
				continue
			}
			b := ensure(name)
			b.contracts = append(b.contracts, partyprofile.ContractRow{
				ID: row["id"].Int64(), CounterParty: name, Title: row["title"].String(),
				Amount: row["amount"].Float64(), Status: row["status"].String(), SignDate: row["sign_date"].String(),
			})
		}
	}
	if rows, err := g.DB().Model("obligation").Fields("id, file_id, status, due_date, completed_at").All(); err == nil {
		for _, row := range rows {
			name := fileParty[row["file_id"].Int64()]
			if name == "" {
				continue
			}
			b := ensure(name)
			b.obligations = append(b.obligations, partyprofile.ObligationRow{
				ID: row["id"].Int64(), CounterParty: name, Status: row["status"].String(),
				DueDate: row["due_date"].String(), CompletedAt: row["completed_at"].String(),
			})
		}
	}
	if rows, err := g.DB().Model("contract_clause").Fields("id, file_id, type, risk, summary").All(); err == nil {
		for _, row := range rows {
			name := fileParty[row["file_id"].Int64()]
			if name == "" {
				continue
			}
			b := ensure(name)
			b.clauses = append(b.clauses, partyprofile.ClauseRow{
				ID: row["id"].Int64(), CounterParty: name, Type: row["type"].String(),
				Risk: row["risk"].String(), Summary: row["summary"].String(),
			})
		}
	}
	return buckets, fileParty
}

func buildPartyProfile(name string, b *partyBucket) partyprofile.Profile {
	if b == nil {
		b = &partyBucket{}
	}
	return partyprofile.Build(name, b.contracts, b.obligations, b.clauses, time.Now())
}

// PartyProfileList AC8.1/8.4：全库相对方聚合清单（按关注度排序，便于法务先看高风险的）
func PartyProfileList(r *ghttp.Request) {
	keyword := strings.TrimSpace(r.Get("keyword").String())
	buckets, _ := loadPartyBuckets(r)
	list := make([]g.Map, 0, len(buckets))
	for name, b := range buckets {
		if keyword != "" && !strings.Contains(name, keyword) {
			continue
		}
		p := buildPartyProfile(name, b)
		list = append(list, g.Map{
			"name": name, "contractCount": p.ContractCount, "totalAmount": p.TotalAmount,
			"lastSignDate": p.LastSignDate, "riskScore": p.RiskScore, "riskLevel": p.RiskLevel,
			"riskLabel": partyprofile.LevelLabel(p.RiskLevel), "tags": p.Tags,
			"fulfillRate": p.FulfillRate, "disputeClauseCount": p.DisputeClauseCount,
			"sampleWarning": p.SampleWarning,
		})
	}
	// 关注度高的排前面：风险分升序（越低越需关注），同分按合同数降序
	for i := 0; i < len(list); i++ {
		for j := i + 1; j < len(list); j++ {
			a, b := list[i]["riskScore"].(int), list[j]["riskScore"].(int)
			if b < a || (a == b && list[j]["contractCount"].(int) > list[i]["contractCount"].(int)) {
				list[i], list[j] = list[j], list[i]
			}
		}
	}
	r.Response.WriteJsonExit(g.Map{"code": 0, "data": list})
}

// PartyProfileDetail AC8.2/8.3/8.4：单个相对方完整画像
func PartyProfileDetail(r *ghttp.Request) {
	name := strings.TrimSpace(r.Get("name").String())
	if name == "" {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": "name required"})
		return
	}
	buckets, _ := loadPartyBuckets(r)
	p := buildPartyProfile(name, buckets[name])
	r.Response.WriteJsonExit(g.Map{"code": 0, "data": g.Map{
		"profile": p, "riskLabel": partyprofile.LevelLabel(p.RiskLevel),
		"scope": "内部合同库行为画像（不含外部工商/司法数据）",
	}})
}

// PartyProfileSummary 统计卡：相对方数 / 需关注数 / 覆盖合同数 / 有义务记录数
func PartyProfileSummary(r *ghttp.Request) {
	buckets, _ := loadPartyBuckets(r)
	contracts, attention, withObligation := 0, 0, 0
	for name, b := range buckets {
		p := buildPartyProfile(name, b)
		contracts += p.ContractCount
		if p.RiskLevel == "high" {
			attention++
		}
		if p.ObligationTotal > 0 {
			withObligation++
		}
	}
	r.Response.WriteJsonExit(g.Map{"code": 0, "data": g.Map{
		"parties": len(buckets), "attention": attention,
		"contracts": contracts, "withObligation": withObligation,
	}})
}

// PartyProfileFiles 该相对方已关联的合同文件（AC8.1 归集明细）
func PartyProfileFiles(r *ghttp.Request) {
	name := strings.TrimSpace(r.Get("name").String())
	rows, err := g.DB().Model("contract_file").Fields("id, filename, status, counter_party, created_at").
		Where("counter_party", name).Order("id desc").Limit(200).All()
	if err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	r.Response.WriteJsonExit(g.Map{"code": 0, "data": rows})
}

// PartyProfileLink 存量合同文件补绑相对方：上传时未登记相对方的文件在这里归集（AC8.1）
func PartyProfileLink(r *ghttp.Request) {
	var req struct {
		FileId       int64  `json:"fileId"`
		CounterParty string `json:"counterParty"`
	}
	if err := r.Parse(&req); err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	req.CounterParty = strings.TrimSpace(req.CounterParty)
	if req.FileId <= 0 {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": "fileId required"})
		return
	}
	row, err := g.DB().Model("contract_file").Where("id", req.FileId).One()
	if err != nil || row.IsEmpty() {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": "合同文件不存在"})
		return
	}
	if _, err := g.DB().Model("contract_file").Data(g.Map{
		"counter_party": req.CounterParty, "updated_at": time.Now(),
	}).Where("id", req.FileId).Update(); err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	audit(r, "party.link", "contract_file", req.FileId, g.Map{
		"counterParty": req.CounterParty, "previous": row["counter_party"].String(),
	})
	r.Response.WriteJsonExit(g.Map{"code": 0, "data": g.Map{"fileId": req.FileId, "counterParty": req.CounterParty}})
}
