package middleware

import (
	"encoding/json"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"vouchchain/pkg/isolate"
	"vouchchain/pkg/orgcore"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/net/ghttp"
)

// ---------- AG5 多维度数据隔离（策略执行 / 对象级越权拦截） ----------
// 与 G1 dataScope 的关系：G1 是角色自带的粗粒度范围（all/entity/dept/self），
// AG5 是可配置的强制 AND 条件，且优先于可伪造的 X-Legal-Entity-Id 请求头。
// 两者叠加取交集，不是互相替代。

var isoCache = struct {
	sync.RWMutex
	at       time.Time
	policies []isolate.Policy
	prepared bool
}{}

// InvalidateIsolationCache 策略增删改后由控制器调用，保证即时生效（AG5.5）
func InvalidateIsolationCache() {
	isoCache.Lock()
	isoCache.at = time.Time{}
	isoCache.Unlock()
}

func isolationPolicies() []isolate.Policy {
	isoCache.RLock()
	fresh := time.Since(isoCache.at) < 20*time.Second && isoCache.prepared
	isoCache.RUnlock()
	if fresh {
		isoCache.RLock()
		out := append([]isolate.Policy{}, isoCache.policies...)
		isoCache.RUnlock()
		return out
	}
	isoCache.Lock()
	defer isoCache.Unlock()
	if time.Since(isoCache.at) < 20*time.Second && isoCache.prepared {
		return append([]isolate.Policy{}, isoCache.policies...)
	}
	list := []isolate.Policy{}
	rows, err := g.DB().Model("isolation_policy").Order("id asc").All()
	if err == nil {
		for _, row := range rows {
			vals := []string{}
			_ = json.Unmarshal([]byte(row["allowed_values"].String()), &vals)
			list = append(list, isolate.Policy{
				ID: row["id"].Int64(), Name: row["name"].String(),
				Dimension: row["dimension"].String(), SubjectType: row["subject_type"].String(),
				Subject: row["subject"].String(), Values: vals,
				IncludeChildren: row["include_children"].Bool(), Enabled: row["enabled"].Bool(),
				Priority: row["priority"].Int(),
			})
		}
	}
	isoCache.policies, isoCache.at, isoCache.prepared = list, time.Now(), true
	return append([]isolate.Policy{}, list...)
}

// isolationExpand 沿父子树展开允许值；DB 异常时保留原值（不因树读失败而放大权限）。
func isolationExpand(dim string, values []string, include bool) []string {
	if !include || (dim != isolate.DimEntity && dim != isolate.DimDept) {
		return values
	}
	table := "legal_entity"
	if dim == isolate.DimDept {
		table = "org_node"
	}
	rows, err := g.DB().Model(table).Fields("id, parent_id").All()
	if err != nil {
		return values
	}
	parents := map[int64]int64{}
	for _, row := range rows {
		parents[row["id"].Int64()] = row["parent_id"].Int64()
	}
	tr := orgcore.New(parents)
	out := append([]string{}, values...)
	for _, v := range values {
		id, e := strconv.ParseInt(strings.TrimSpace(v), 10, 64)
		if e != nil || id <= 0 {
			continue
		}
		for _, child := range tr.Descendants(id) {
			out = append(out, strconv.FormatInt(child, 10))
		}
	}
	return out
}

// IsolationFor 当前请求生效的隔离范围（scopeFilter 与对象守卫共用同一份判定，避免两处口径漂移）。
func IsolationFor(r *ghttp.Request) isolate.Decision {
	return isolate.Resolve(isolationPolicies(), isolate.Subject{
		User: r.GetCtxVar("user").String(), Role: CurrentRole(r),
	}, isolationExpand)
}

// AllIsolationPolicies 供管理页展示原始策略
func AllIsolationPolicies() []isolate.Policy { return isolationPolicies() }

// 对象级守卫的资源白名单：列表接口不带 id 不受影响，按 id 的读写逐一校验归属。
type guardedResource struct {
	re    *regexp.Regexp
	table string
	label string
}

var guardedResources = []guardedResource{
	{regexp.MustCompile(`^/api/contracts/files/(\d+)(?:/.*)?$`), "contract_file", "合同文件"},
	{regexp.MustCompile(`^/api/contracts/(\d+)(?:/.*)?$`), "contract", "合同"},
	{regexp.MustCompile(`^/api/invoices/(\d+)(?:/.*)?$`), "invoice", "发票"},
	{regexp.MustCompile(`^/api/vouchers/(\d+)(?:/.*)?$`), "voucher", "凭证"},
	{regexp.MustCompile(`^/api/payables/(\d+)(?:/.*)?$`), "payment_plan", "应付计划"},
	{regexp.MustCompile(`^/api/journal/drafts/(\d+)(?:/.*)?$`), "journal_draft", "分录草稿"},
	{regexp.MustCompile(`^/api/receipts/(\d+)(?:/.*)?$`), "bank_receipt", "银行回单"},
	{regexp.MustCompile(`^/api/master-data/(\d+)$`), "master_data", "主数据"},
}

// IsolationGuard 必须挂在 Rbac 之后：已确定角色，再按对象归属做强制校验。
// 越权时写越权记录 + 审计轨迹 + 站内告警（AG5.4/AG5.5），返回 403。
func IsolationGuard() ghttp.HandlerFunc {
	return func(r *ghttp.Request) {
		decision := IsolationFor(r)
		if !decision.Enforced {
			r.Middleware.Next()
			return
		}
		var table, label string
		var objectId int64
		for _, res := range guardedResources {
			m := res.re.FindStringSubmatch(r.URL.Path)
			if m == nil {
				continue
			}
			table, label = res.table, res.label
			objectId, _ = strconv.ParseInt(m[1], 10, 64)
			break
		}
		if table == "" || objectId <= 0 {
			r.Middleware.Next()
			return
		}
		row, err := g.DB().Model(table).Fields("legal_entity_id, dept_id, project_code").Where("id", objectId).One()
		if err != nil || row.IsEmpty() {
			// 查不到对象（或迁移未就绪）交给业务层返回 404，不在这里把系统锁死
			r.Middleware.Next()
			return
		}
		obj := isolate.Object{
			EntityID: row["legal_entity_id"].Int64(), DeptID: row["dept_id"].Int64(),
			Project: row["project_code"].String(),
		}
		if ok, reason := decision.Allow(obj); ok {
			r.Middleware.Next()
			return
		} else {
			recordIsolationDeny(r, decision, table, label, objectId, obj, reason)
		}
	}
}

func recordIsolationDeny(r *ghttp.Request, decision isolate.Decision, table, label string, objectId int64, obj isolate.Object, reason string) {
	username := r.GetCtxVar("user").String()
	role := CurrentRole(r)
	ids, _ := json.Marshal(decision.Matched)
	now := time.Now()
	g.DB().Model("isolation_violation").Data(g.Map{
		"username": username, "role": role, "method": r.Method, "path": r.URL.Path,
		"resource": table, "object_id": objectId,
		"dimension": strings.Join(keysOf(decision.Allowed), ","),
		"attempted_value": "entity=" + strconv.FormatInt(obj.EntityID, 10) +
			",dept=" + strconv.FormatInt(obj.DeptID, 10) + ",project=" + obj.Project,
		"policy_ids": string(ids), "status": "open", "created_at": now,
	}).Insert()
	detail, _ := json.Marshal(g.Map{
		"reason": reason, "object": label + " #" + strconv.FormatInt(objectId, 10),
		"entityId": obj.EntityID, "deptId": obj.DeptID, "project": obj.Project,
		"policyIds": decision.Matched, "path": r.URL.Path,
	})
	ip := r.Header.Get("X-Forwarded-For")
	if ip == "" {
		ip = r.RemoteAddr
	}
	g.DB().Model("audit_log").Data(g.Map{
		"ip": ip, "username": username, "action": "isolation.deny", "target_type": table,
		"target_id": objectId, "detail": string(detail), "created_at": now,
	}).Insert()
	if username != "" {
		g.DB().Model("notification").Data(g.Map{
			"username": username, "type": "security", "title": "越权访问已被拦截",
			"content": label + " #" + strconv.FormatInt(objectId, 10) + "：" + reason,
			"is_read": false, "created_at": now,
		}).Insert()
	}
	body, _ := json.Marshal(g.Map{
		"code": 403, "message": "数据隔离策略拦截：" + reason, "reason": reason,
	})
	r.Response.Header().Set("Content-Type", "application/json; charset=utf-8")
	r.Response.WriteStatusExit(403, string(body))
}

func keysOf(m map[string][]string) []string {
	out := []string{}
	for k := range m {
		out = append(out, k)
	}
	return out
}
