// Package policycore 合规策略引擎的纯逻辑内核（产品② AG4.1–AG4.4）。
// 设计约束：不依赖数据库 / Web 框架，规则以数据描述、执行以纯函数完成，
// 便于单独开源抽取，也便于业务系统在任何写入点复用同一套判定。
package policycore

import (
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
)

// Rule 一条可执行制度规则（规则库表 compliance_rule 的行模型）
type Rule struct {
	Code       string
	Name       string
	RuleType   string // required / format / relation / threshold
	ObjectType string // contract / invoice / voucher / master_data
	Field      string
	Op         string // threshold: gt/gte/lt/lte/eq；format: regex；relation: exists
	Value      string
	Condition  string // AG4.3 规则绑定条件：key=value，空=不限
	Severity   string // block / warn
	Enabled    bool
	Version    int
}

// Result 单条规则的判定结果
type Result struct {
	RuleCode string `json:"ruleCode"`
	RuleName string `json:"ruleName"`
	Severity string `json:"severity"`
	Passed   bool   `json:"passed"`
	Message  string `json:"message"`
	Field    string `json:"field"`
}

func str(v interface{}) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return x
	case []byte:
		return string(x)
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64)
	case int:
		return strconv.Itoa(x)
	case int64:
		return strconv.FormatInt(x, 10)
	default:
		return fmt.Sprint(v)
	}
}

func num(v interface{}) (float64, error) {
	s := strings.TrimSpace(str(v))
	if s == "" {
		return 0, fmt.Errorf("空值")
	}
	return strconv.ParseFloat(strings.ReplaceAll(s, ",", ""), 64)
}

func present(v interface{}) bool {
	if v == nil {
		return false
	}
	s := strings.TrimSpace(str(v))
	return s != "" && s != "0" && s != "<nil>" && s != "null"
}

// Evaluate 对一条业务记录执行一条规则；record 的 key 为业务字段名（camelCase）
func Evaluate(rule Rule, record map[string]interface{}) Result {
	res := Result{RuleCode: rule.Code, RuleName: rule.Name, Severity: rule.Severity,
		Field: rule.Field, Passed: true}
	if !rule.Enabled {
		res.Message = "规则已停用"
		return res
	}
	raw := record[rule.Field]
	value := str(raw)

	fail := func(msg string) Result {
		res.Passed = false
		res.Message = msg
		return res
	}

	switch rule.RuleType {
	case "required":
		if !present(raw) {
			return fail(fmt.Sprintf("必填项 %s 缺失", rule.Field))
		}
	case "format":
		if !present(raw) {
			return fail(fmt.Sprintf("格式校验字段 %s 缺失", rule.Field))
		}
		re, err := regexp.Compile(rule.Value)
		if err != nil {
			return fail("规则正则无效: " + err.Error())
		}
		if !re.MatchString(value) {
			return fail(fmt.Sprintf("%s=%q 不符合格式 %s", rule.Field, value, rule.Value))
		}
	case "relation":
		// Value 为同记录内被关联的目标字段；exists 语义：目标字段有值且非 0
		target := record[rule.Value]
		if rule.Op == "exists" && !present(target) {
			return fail(fmt.Sprintf("关联校验：%s 要求已关联 %s", rule.Field, rule.Value))
		}
		if rule.Op == "equals" && str(target) != value {
			return fail(fmt.Sprintf("关联校验：%s 与 %s 不一致", rule.Field, rule.Value))
		}
	case "threshold":
		left, err := num(raw)
		if err != nil {
			return fail(fmt.Sprintf("阈值校验字段 %s 不是数值: %v", rule.Field, err))
		}
		right, err := num(rule.Value)
		if err != nil {
			return fail("规则阈值配置无效: " + err.Error())
		}
		ok := true
		switch rule.Op {
		case "gt":
			ok = left > right
		case "gte":
			ok = left >= right
		case "lt":
			ok = left < right
		case "lte":
			ok = left <= right
		case "eq":
			ok = math.Abs(left-right) < 1e-9
		default:
			return fail("阈值操作符不支持: " + rule.Op)
		}
		if !ok {
			return fail(fmt.Sprintf("%s=%s 不满足 %s %s", rule.Field,
				strconv.FormatFloat(left, 'f', -1, 64), rule.Op, rule.Value))
		}
	default:
		return fail("未知规则类型: " + rule.RuleType)
	}
	res.Message = "通过"
	return res
}

// MatchCondition AG4.3：规则可绑定到业务对象子类型 / 状态（如 mdType=supplier）
func MatchCondition(condition string, record map[string]interface{}) bool {
	if strings.TrimSpace(condition) == "" {
		return true
	}
	parts := strings.SplitN(condition, "=", 2)
	if len(parts) != 2 {
		return true // 无效条件视为不限，避免误拦截
	}
	return str(record[strings.TrimSpace(parts[0])]) == strings.TrimSpace(parts[1])
}

// EvaluateAll 执行一个对象上的全部（条件命中的）规则，返回未通过项（由调用方按 Severity 分流）
func EvaluateAll(rules []Rule, record map[string]interface{}) []Result {
	failed := make([]Result, 0)
	for _, rule := range rules {
		if !MatchCondition(rule.Condition, record) {
			continue
		}
		if r := Evaluate(rule, record); !r.Passed {
			failed = append(failed, r)
		}
	}
	return failed
}

// HasBlock 判定是否存在拦截级违规
func HasBlock(results []Result) bool {
	for _, r := range results {
		if r.Severity == "block" {
			return true
		}
	}
	return false
}
