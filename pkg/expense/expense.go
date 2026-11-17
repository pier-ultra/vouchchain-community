package expense

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/gogf/gf/v2/frame/g"
)

// 费控业务：切片校验（合计不超发票额）+ 预算使用度 + 差标超标检查。
// 设计原则（用户偏好）：超支/超标是「提示与记录」，不是硬拦截——AI 只出建议、人审确认。

// Split 报销切片
type Split struct {
	ID          int64   `json:"id"`
	InvoiceID   int64   `json:"invoiceId"`
	SliceNo     int     `json:"sliceNo"`
	Amount      float64 `json:"amount"`
	ExpenseType string  `json:"expenseType"`
	DeptID      int64   `json:"deptId"`
	Bearer      string  `json:"bearer"` // company / personal
	Remark      string  `json:"remark"`
	Creator     string  `json:"creator"`
}

// invoiceTotal 发票价税合计
func invoiceTotal(ctx context.Context, invoiceID int64) (float64, error) {
	row, err := g.DB().Model("invoice").Where("id", invoiceID).One()
	if err != nil || row.IsEmpty() {
		return 0, fmt.Errorf("发票不存在")
	}
	if t := row["total"]; !t.IsNil() && t.Float64() > 0 {
		return t.Float64(), nil
	}
	return row["amount"].Float64(), nil
}

// splitSum 已切片合计
func splitSum(ctx context.Context, invoiceID int64, excludeID int64) float64 {
	m := g.DB().Model("expense_split").Where("invoice_id", invoiceID)
	if excludeID > 0 {
		m = m.WhereNot("id", excludeID)
	}
	v, err := m.Sum("amount")
	if err != nil {
		return 0
	}
	return v
}

// CreateSplitInput 创建切片入参
type CreateSplitInput struct {
	InvoiceID   int64   `json:"invoiceId"`
	Amount      float64 `json:"amount" v:"required"`
	ExpenseType string  `json:"expenseType"`
	DeptID      int64   `json:"deptId"`
	Bearer      string  `json:"bearer"`
	Remark      string  `json:"remark"`
	Creator     string  `json:"creator"`
}

// CreateSplit 创建切片：硬校验合计；软校验（预算/差标）以 warnings 返回，不阻断
func CreateSplit(ctx context.Context, in CreateSplitInput) (int64, []string, error) {
	total, err := invoiceTotal(ctx, in.InvoiceID)
	if err != nil {
		return 0, nil, err
	}
	if in.Amount <= 0 {
		return 0, nil, fmt.Errorf("切片金额必须大于 0")
	}
	already := splitSum(ctx, in.InvoiceID, 0)
	if already+in.Amount > total+0.005 {
		return 0, nil, fmt.Errorf("切片合计 %.2f + 新增 %.2f 超过发票总额 %.2f", already, in.Amount, total)
	}
	if in.Bearer == "" {
		in.Bearer = "company"
	}
	// 切片序号 = 现有条数 + 1
	cnt, _ := g.DB().Model("expense_split").Where("invoice_id", in.InvoiceID).Count()
	res, err := g.DB().Model("expense_split").Data(g.Map{
		"invoice_id": in.InvoiceID, "slice_no": cnt + 1, "amount": in.Amount,
		"expense_type": in.ExpenseType, "dept_id": in.DeptID, "bearer": in.Bearer,
		"remark": in.Remark, "creator": in.Creator, "created_at": time.Now(),
	}).Insert()
	if err != nil {
		return 0, nil, err
	}
	id, _ := res.LastInsertId()
	warnings := CheckWarnings(ctx, in.InvoiceID, in.Amount, in.ExpenseType, in.DeptID)
	return id, warnings, nil
}

// CheckWarnings 软校验：预算使用度 + 差标超标（提示级，不阻断）
func CheckWarnings(ctx context.Context, invoiceID int64, amount float64, expenseType string, deptID int64) []string {
	warnings := make([]string, 0)
	period := currentPeriod()
	// 1) 预算检查（有预算配置时）
	if deptID > 0 && expenseType != "" {
		row, err := g.DB().Model("budget").
			Where("dept_id", deptID).Where("period", period).Where("category", expenseType).One()
		if err == nil && !row.IsEmpty() {
			limit := row["amount_limit"].Float64()
			spent := deptCategorySpent(ctx, deptID, period, expenseType)
			after := spent + amount
			if after > limit {
				warnings = append(warnings, fmt.Sprintf("预算超支：%d 部门 %s 月「%s」预算 %.2f，已用 %.2f，本切片 %.2f 后累计 %.2f", deptID, period, expenseType, limit, spent, amount, after))
			} else if after > limit*0.9 {
				warnings = append(warnings, fmt.Sprintf("预算接近上限：%s 月「%s」已用 %.2f / %.2f（本次后 %.1f%%）", period, expenseType, after, limit, after/limit*100))
			}
		}
	}
	// 2) 差标检查（该分类有标准时）
	if expenseType != "" {
		rows, err := g.DB().Model("travel_standard").Where("enabled", true).Where("category", expenseType).All()
		if err == nil {
			for _, std := range rows {
				limit := std["limit_amount"].Float64()
				if limit > 0 && amount > limit {
					warnings = append(warnings, fmt.Sprintf("差旅超标：「%s」单笔 %.2f 超过标准 %.2f（%s）", expenseType, amount, limit, std["note"].String()))
				}
			}
		}
	}
	return warnings
}

func currentPeriod() string {
	return "2026-11" // 演示期固定；生产版取 time.Now().Format("2006-01")
}

// deptCategorySpent 部门+月份+分类 的切片已用
func deptCategorySpent(ctx context.Context, deptID int64, period, category string) float64 {
	v, err := g.DB().GetOne(ctx,
		`SELECT COALESCE(SUM(amount),0) AS s FROM expense_split WHERE dept_id=? AND expense_type=? AND to_char(created_at,'YYYY-MM')=?`,
		deptID, category, period)
	if err != nil || v == nil {
		return 0
	}
	return v["s"].Float64()
}

// BudgetUsage 预算使用视图
type BudgetUsage struct {
	ID       int64   `json:"id"`
	DeptID   int64   `json:"deptId"`
	Period   string  `json:"period"`
	Category string  `json:"category"`
	Limit    float64 `json:"limit"`
	Spent    float64 `json:"spent"`
	Left     float64 `json:"left"`
	Over     bool    `json:"over"`
}

// BudgetUsages 全部预算的使用情况
func BudgetUsages(ctx context.Context) ([]BudgetUsage, error) {
	rows, err := g.DB().Model("budget").Order("period desc, dept_id asc, category asc").All()
	if err != nil {
		return nil, err
	}
	out := make([]BudgetUsage, 0, len(rows))
	for _, r := range rows {
		deptID := r["dept_id"].Int64()
		period := r["period"].String()
		category := r["category"].String()
		limit := r["amount_limit"].Float64()
		spent := deptCategorySpent(ctx, deptID, period, category)
		out = append(out, BudgetUsage{
			ID: r["id"].Int64(), DeptID: deptID, Period: period, Category: category,
			Limit: limit, Spent: spent, Left: limit - spent, Over: spent > limit,
		})
	}
	return out, nil
}

// CheckInvoice 按发票整体跑差标检查（差旅类票）
func CheckInvoice(ctx context.Context, invoiceID int64) ([]string, error) {
	row, err := g.DB().Model("invoice").Where("id", invoiceID).One()
	if err != nil || row.IsEmpty() {
		return nil, fmt.Errorf("发票不存在")
	}
	category := row["category"].String()
	amount := row["amount"].Float64()
	if strings.TrimSpace(category) == "" {
		return nil, fmt.Errorf("该发票未分类，请先执行分类")
	}
	return CheckWarnings(ctx, invoiceID, amount, category, row["dept_id"].Int64()), nil
}
