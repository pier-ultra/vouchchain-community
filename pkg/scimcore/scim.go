// Package scimcore 收口 SCIM 2.0（RFC 7643/7644）文档解析的纯逻辑：
// User/Group 属性归一化、PatchOp 应用、filter 匹配。
// 不碰数据库和 HTTP，便于单测；协议响应与持久化落在 internal/controller/scim_*.go。
package scimcore

import (
	"fmt"
	"strconv"
	"strings"
)

const (
	SchemaUser            = "urn:ietf:params:scim:schemas:core:2.0:User"
	SchemaGroup           = "urn:ietf:params:scim:schemas:core:2.0:Group"
	SchemaEnterpriseUser  = "urn:ietf:params:scim:schemas:extension:enterprise:2.0:User"
	SchemaListResponse    = "urn:ietf:params:scim:api:messages:2.0:ListResponse"
	SchemaPatchOp         = "urn:ietf:params:scim:api:messages:2.0:PatchOp"
	SchemaError           = "urn:ietf:params:scim:api:messages:2.0:Error"
	SchemaServiceProvider = "urn:ietf:params:scim:schemas:core:2.0:ServiceProviderConfig"
	SchemaResourceType    = "urn:ietf:params:scim:api:messages:2.0:ResourceType"
	SchemaSchema          = "urn:ietf:params:scim:api:messages:2.0:Schema"
)

// UserAttrs 从 SCIM User 文档中归一化出来的本系统关心的字段。
// Active 用指针区分「文档没带」（沿用原值）与「明确给了 false」（禁用）。
type UserAttrs struct {
	ExternalId  string
	UserName    string
	DisplayName string
	Email       string
	Title       string
	Department  string
	Active      *bool
	Groups      []string
	Raw         map[string]any
}

type GroupAttrs struct {
	ExternalId  string
	DisplayName string
	Members     []string
	Raw         map[string]any
}

// ParseUser 兼容各 IdP 的常见写法：userName 必填；name.formatted / displayName 互兜；
// emails 支持数组对象、单对象、纯字符串；active 缺省为 nil；企业扩展里的 department/title 也取。
func ParseUser(doc map[string]any) (UserAttrs, error) {
	u := UserAttrs{Raw: doc, Groups: []string{}}
	u.ExternalId = strVal(doc["externalId"])
	u.UserName = strings.TrimSpace(strVal(doc["userName"]))
	u.DisplayName = strVal(doc["displayName"])
	if u.DisplayName == "" {
		u.DisplayName = strVal(nested(doc, "name", "formatted"))
	}
	u.Email = FirstEmail(doc["emails"])
	if v, ok := doc["active"]; ok {
		b, err := boolVal(v)
		if err != nil {
			return u, fmt.Errorf("active 不是布尔值：%v", v)
		}
		u.Active = &b
	}
	u.Groups = GroupNames(doc["groups"])
	u.Title = strVal(doc["title"])
	u.Department = strVal(doc["department"])
	if ext, ok := doc[SchemaEnterpriseUser].(map[string]any); ok {
		if u.Department == "" {
			u.Department = strVal(ext["department"])
		}
		if u.Title == "" {
			u.Title = strVal(ext["title"])
		}
		if u.Department == "" {
			u.Department = strVal(nested(ext, "manager", "displayName"))
		}
	}
	if u.UserName == "" {
		return u, fmt.Errorf("userName 不能为空")
	}
	return u, nil
}

func ParseGroup(doc map[string]any) (GroupAttrs, error) {
	g := GroupAttrs{Raw: doc, Members: []string{}}
	g.ExternalId = strVal(doc["externalId"])
	g.DisplayName = strings.TrimSpace(strVal(doc["displayName"]))
	g.Members = memberNames(doc["members"])
	if g.DisplayName == "" {
		return g, fmt.Errorf("displayName 不能为空")
	}
	return g, nil
}

// FirstEmail 取 primary 的邮箱，否则取第一个；兼容对象、数组、纯字符串三种写法。
func FirstEmail(v any) string {
	switch x := v.(type) {
	case string:
		return strings.TrimSpace(x)
	case map[string]any:
		if primary, ok := x["primary"].(bool); ok && primary {
			return strVal(x["value"])
		}
		return strVal(x["value"])
	case []any:
		fallback := ""
		for _, item := range x {
			switch e := item.(type) {
			case string:
				if fallback == "" {
					fallback = strings.TrimSpace(e)
				}
			case map[string]any:
				val := strVal(e["value"])
				if val == "" {
					continue
				}
				if p, ok := e["primary"].(bool); ok && p {
					return val
				}
				if fallback == "" {
					fallback = val
				}
			}
		}
		return fallback
	}
	return ""
}

// GroupNames 从 User.groups / PATCH value 中提取组标识（display 优先，value 兜底）。
func GroupNames(v any) []string {
	out := []string{}
	appendUnique := func(s string) {
		s = strings.TrimSpace(s)
		if s == "" {
			return
		}
		for _, old := range out {
			if old == s {
				return
			}
		}
		out = append(out, s)
	}
	switch x := v.(type) {
	case string:
		appendUnique(x)
	case map[string]any:
		appendUnique(firstNonEmpty(strVal(x["display"]), strVal(x["displayName"]), strVal(x["value"])))
	case []any:
		for _, item := range x {
			switch g := item.(type) {
			case string:
				appendUnique(g)
			case map[string]any:
				appendUnique(firstNonEmpty(strVal(g["display"]), strVal(g["displayName"]), strVal(g["value"])))
			}
		}
	}
	return out
}

func memberNames(v any) []string {
	out := []string{}
	switch x := v.(type) {
	case []any:
		for _, item := range x {
			switch m := item.(type) {
			case string:
				out = appendUnique(out, m)
			case map[string]any:
				out = appendUnique(out, strVal(m["value"]))
			}
		}
	case string:
		out = appendUnique(out, x)
	}
	return out
}

// ---------- PATCH（RFC 7644 §3.5.2） ----------

type PatchOp struct {
	Op    string `json:"op"`
	Path  string `json:"path"`
	Value any    `json:"value"`
}

// ApplyUserPatch 支持 add / replace / remove 与「无 path、value 是对象」两种常见推送形态。
func ApplyUserPatch(base UserAttrs, ops []PatchOp) (UserAttrs, error) {
	out := base
	out.Groups = append([]string{}, base.Groups...)
	for i, op := range ops {
		paths := []string{strings.TrimSpace(op.Path)}
		values := []any{op.Value}
		if paths[0] == "" {
			// 无 path：value 必须是对象，逐字段按 add/replace 处理
			m, ok := op.Value.(map[string]any)
			if !ok {
				return out, fmt.Errorf("Operations[%d] 未指定 path 时 value 必须是对象", i)
			}
			paths, values = nil, nil
			for k, v := range m {
				paths = append(paths, k)
				values = append(values, v)
			}
		}
		for j, path := range paths {
			if err := applyUserField(&out, strings.ToLower(strings.TrimSpace(op.Op)), path, values[j]); err != nil {
				return out, fmt.Errorf("Operations[%d] %s：%w", i, path, err)
			}
		}
	}
	return out, nil
}

func applyUserField(u *UserAttrs, op, path string, value any) error {
	key := normUserPath(path)
	if key == "" {
		return fmt.Errorf("不支持的属性路径 %q", path)
	}
	switch op {
	case "add", "replace":
	case "remove":
		switch key {
		case "groups":
			u.Groups = []string{}
		case "active":
			b := false
			u.Active = &b // remove active 保守视为禁用
		case "username":
			return fmt.Errorf("userName 不可删除")
		default:
			setUserString(u, key, "")
		}
		return nil
	default:
		return fmt.Errorf("不支持的操作 %q", op)
	}
	switch key {
	case "active":
		b, err := boolVal(value)
		if err != nil {
			return fmt.Errorf("active 不是布尔值：%v", value)
		}
		u.Active = &b
	case "groups":
		// replace 全量替换；add 追加合并（IdP 常见写法）
		names := GroupNames(value)
		if op == "replace" {
			u.Groups = names
		} else {
			for _, n := range names {
				u.Groups = appendUnique(u.Groups, n)
			}
		}
	case "email":
		u.Email = FirstEmail(value)
	default:
		setUserString(u, key, strVal(value))
	}
	return nil
}

func normUserPath(path string) string {
	p := strings.ToLower(strings.ReplaceAll(path, " ", ""))
	// 过滤路径：[type eq "work"] / [value eq "x"] 这类带筛选的子属性
	if i := strings.Index(p, "["); i >= 0 {
		if j := strings.Index(p[i:], "]"); j >= 0 {
			p = p[:i] + p[i+j+1:]
		}
	}
	p = strings.TrimPrefix(p, "urn:ietf:params:scim:schemas:extension:enterprise:2.0:user:")
	switch p {
	case "username", "externalid", "displayname", "name.formatted", "title", "department":
		return map[string]string{
			"username": "username", "externalid": "externalid", "displayname": "displayname",
			"name.formatted": "displayname", "title": "title", "department": "department",
		}[p]
	case "active", "groups":
		return p
	case "emails", "emails.value", "email", "emails.value.value":
		return "email"
	}
	// Casdoor 等 IdP 可能直接把扩展字段放在顶层的 path 里
	if strings.HasSuffix(p, ".department") || p == "department" {
		return "department"
	}
	return ""
}

func setUserString(u *UserAttrs, key, val string) {
	switch key {
	case "username":
		u.UserName = strings.TrimSpace(val)
	case "externalid":
		u.ExternalId = val
	case "displayname":
		u.DisplayName = val
	case "title":
		u.Title = val
	case "department":
		u.Department = val
	case "email":
		u.Email = val
	}
}

func ApplyGroupPatch(base GroupAttrs, ops []PatchOp) (GroupAttrs, error) {
	out := base
	out.Members = append([]string{}, base.Members...)
	for i, op := range ops {
		key := strings.ToLower(strings.TrimSpace(op.Path))
		if bracket := strings.Index(key, "["); bracket >= 0 {
			key = key[:bracket] // members[value eq "xxx"] → members
		}
		if key == "" {
			m, ok := op.Value.(map[string]any)
			if !ok {
				return out, fmt.Errorf("Operations[%d] 未指定 path 时 value 必须是对象", i)
			}
			if name, ok := m["displayName"]; ok {
				out.DisplayName = strVal(name)
			}
			if ext, ok := m["externalId"]; ok {
				out.ExternalId = strVal(ext)
			}
			if ms, ok := m["members"]; ok {
				out.Members = memberNames(ms)
			}
			continue
		}
		switch strings.ToLower(strings.TrimSpace(op.Op)) {
		case "add", "replace":
			switch key {
			case "displayname":
				out.DisplayName = strVal(op.Value)
			case "externalid":
				out.ExternalId = strVal(op.Value)
			case "members":
				names := memberNames(op.Value)
				if strings.EqualFold(strings.TrimSpace(op.Op), "replace") {
					out.Members = names
				} else {
					for _, n := range names {
						out.Members = appendUnique(out.Members, n)
					}
				}
			default:
				return out, fmt.Errorf("Operations[%d] 不支持的属性 %q", i, op.Path)
			}
		case "remove":
			switch key {
			case "members":
				drop := memberNames(op.Value)
				// 带筛选路径的 remove：members[value eq "xxx"]
				if hit := bracketEqValue(op.Path); hit != "" {
					drop = append(drop, hit)
				}
				if len(drop) == 0 {
					out.Members = []string{} // 无 value 的 remove members 视为清空
					break
				}
				kept := []string{}
				for _, m := range out.Members {
					if !contains(drop, m) {
						kept = append(kept, m)
					}
				}
				out.Members = kept
			case "displayname":
				return out, fmt.Errorf("displayName 不可删除")
			default:
				return out, fmt.Errorf("Operations[%d] 不支持的属性 %q", i, op.Path)
			}
		default:
			return out, fmt.Errorf("Operations[%d] 不支持的操作 %q", i, op.Op)
		}
	}
	return out, nil
}

// ---------- filter（RFC 7644 §3.4.2.2） ----------

// MatchUserFilter 支持 eq / ne / co / sw / ew 与 and 连接；不认识的表达式直接报错，
// 避免 IdP 传了复杂 filter 时静默退化成「返回全部用户」。
func MatchUserFilter(filter string, u UserAttrs, id int64) (bool, error) {
	return matchFilter(filter, func(attr string) (string, bool) {
		switch attr {
		case "id":
			return strconv.FormatInt(id, 10), true
		case "username":
			return u.UserName, true
		case "externalid":
			return u.ExternalId, true
		case "displayname", "name.formatted":
			return u.DisplayName, true
		case "email", "emails", "emails.value":
			return u.Email, true
		case "active":
			if u.Active == nil {
				return "true", true
			}
			return strconv.FormatBool(*u.Active), true
		case "title":
			return u.Title, true
		case "department":
			return u.Department, true
		case "groups":
			return strings.Join(u.Groups, ","), true
		}
		return "", false
	})
}

func MatchGroupFilter(filter string, g GroupAttrs, id int64) (bool, error) {
	return matchFilter(filter, func(attr string) (string, bool) {
		switch attr {
		case "id":
			return strconv.FormatInt(id, 10), true
		case "displayname":
			return g.DisplayName, true
		case "externalid":
			return g.ExternalId, true
		case "members":
			return strings.Join(g.Members, ","), true
		}
		return "", false
	})
}

func matchFilter(filter string, valueOf func(string) (string, bool)) (bool, error) {
	filter = strings.TrimSpace(filter)
	if filter == "" {
		return true, nil
	}
	// 仅支持 and；or / not / 括号显式报错，避免误判
	low := strings.ToLower(filter)
	if strings.Contains(low, " or ") || strings.Contains(low, " not ") || strings.Contains(filter, "(") {
		return false, fmt.Errorf("暂不支持的 filter 逻辑：%s", filter)
	}
	parts := splitAnd(filter)
	for _, part := range parts {
		attr, op, want, err := parseClause(part)
		if err != nil {
			return false, err
		}
		got, supported := valueOf(attr)
		if !supported {
			return false, fmt.Errorf("不支持的 filter 属性：%s", attr)
		}
		switch op {
		case "eq":
			if !strings.EqualFold(got, want) {
				return false, nil
			}
		case "ne":
			if strings.EqualFold(got, want) {
				return false, nil
			}
		case "co":
			if !strings.Contains(strings.ToLower(got), strings.ToLower(want)) {
				return false, nil
			}
		case "sw":
			if !strings.HasPrefix(strings.ToLower(got), strings.ToLower(want)) {
				return false, nil
			}
		case "ew":
			if !strings.HasSuffix(strings.ToLower(got), strings.ToLower(want)) {
				return false, nil
			}
		}
	}
	return true, nil
}

func splitAnd(filter string) []string {
	out := []string{}
	start := 0
	low := strings.ToLower(filter)
	for i := 0; i <= len(filter)-5; i++ {
		if low[i:i+5] == " and " {
			out = append(out, filter[start:i])
			start = i + 5
			i += 4
		}
	}
	out = append(out, filter[start:])
	return out
}

func parseClause(part string) (string, string, string, error) {
	part = strings.TrimSpace(part)
	// 属性名在前，比较符在后，值可带双引号
	for _, op := range []string{"eq", "ne", "co", "sw", "ew"} {
		idx := indexOp(part, op)
		if idx < 0 {
			continue
		}
		attr := strings.ToLower(strings.ReplaceAll(strings.TrimSpace(part[:idx]), " ", ""))
		val := strings.TrimSpace(part[idx+len(op):])
		val = strings.Trim(val, `"'`)
		if attr == "" {
			return "", "", "", fmt.Errorf("filter 缺少属性名：%s", part)
		}
		// 带筛选的子属性路径归一到本系统字段：emails[type eq "work"].value → email
		if strings.HasPrefix(attr, "emails[") {
			attr = "email"
		} else if strings.HasPrefix(attr, "members[") {
			attr = "members"
		} else if dot := strings.LastIndex(attr, "."); dot >= 0 && strings.HasPrefix(attr, "name.") {
			attr = "displayname"
		}
		return attr, op, val, nil
	}
	return "", "", "", fmt.Errorf("无法解析 filter 表达式：%s", part)
}

// indexOp 从右往左找比较符：emails[type eq "work"].value eq "a@b" 里
// 第一个 eq 属于方括号内的筛选条件，最后一个才是真正的比较符。
func indexOp(part, op string) int {
	low := strings.ToLower(part)
	for i := len(low) - len(op) - 2; i >= 0; i-- {
		if low[i] != ' ' || low[i+len(op)+1] != ' ' {
			continue
		}
		if low[i+1:i+1+len(op)] == op {
			return i + 1
		}
	}
	return -1
}

func bracketEqValue(path string) string {
	i := strings.Index(strings.ToLower(path), "[value eq ")
	if i < 0 {
		return ""
	}
	rest := path[i+len("[value eq "):]
	if j := strings.Index(rest, "]"); j >= 0 {
		return strings.Trim(rest[:j], `"' `)
	}
	return ""
}

// ---------- 小工具 ----------

func strVal(v any) string {
	switch x := v.(type) {
	case string:
		return strings.TrimSpace(x)
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64)
	case bool:
		return strconv.FormatBool(x)
	case nil:
		return ""
	default:
		return strings.TrimSpace(fmt.Sprint(x))
	}
}

func boolVal(v any) (bool, error) {
	switch x := v.(type) {
	case bool:
		return x, nil
	case string:
		b, err := strconv.ParseBool(strings.TrimSpace(x))
		return b, err
	case float64:
		return x != 0, nil
	}
	return false, fmt.Errorf("无法转成布尔值")
}

func nested(m map[string]any, keys ...string) any {
	var cur any = m
	for _, k := range keys {
		obj, ok := cur.(map[string]any)
		if !ok {
			return nil
		}
		cur = obj[k]
	}
	return cur
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func appendUnique(list []string, val string) []string {
	val = strings.TrimSpace(val)
	if val == "" {
		return list
	}
	for _, v := range list {
		if v == val {
			return list
		}
	}
	return append(list, val)
}

func contains(list []string, val string) bool {
	for _, v := range list {
		if v == val {
			return true
		}
	}
	return false
}
