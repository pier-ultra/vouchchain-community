package invoice_redflush

import (
	"context"
	"strings"

	"github.com/gogf/gf/v2/frame/g"
)

// A2 发票红冲预警规则层（P1）：
//   - 红字发票：发票类型含"红"，或价税合计为负（数电票/增值税红字发票特征）。
//   - 蓝字被红冲：存在指向它的红字发票（红字.original_invoice_no == 蓝字.invoice_no），
//     或同购销方存在红字发票（发票池内红字与蓝字成对出现，预警提示人工核查，非自动入账）。
// 预警是"提示"不是"自动作废"：只标记 red_flush_status，不做状态机变更。

const (
	StatusRed      = "red"      // 本张是红字发票
	StatusReversed = "reversed" // 蓝字发票，被红冲
)

// Detect 单张发票红冲判定（纯函数）：返回 "" 正常 / "red" 红字发票。
func Detect(invoiceType string, total float64) string {
	if strings.Contains(invoiceType, "红") || total < 0 {
		return StatusRed
	}
	return ""
}

// ScanAndMark 全量扫描并落 red_flush_status：
//  1. 先标记所有红字发票（red）；
//  2. 再对每张蓝字发票，若池内存在指向它的红字（同号 或 同购销方），标记被红冲（reversed）。
//
// 返回统计 {red, reversed}。
func ScanAndMark(ctx context.Context) (map[string]any, error) {
	rows, err := g.DB().Model("invoice").All()
	if err != nil {
		return nil, err
	}
	// 第一遍：标记红字，收集红字发票
	reds := []map[string]interface{}{}
	for _, r := range rows {
		st := Detect(r["invoice_type"].String(), r["total"].Float64())
		if st == StatusRed {
			g.DB().Model("invoice").Data(g.Map{"red_flush_status": StatusRed}).Where("id", r["id"].Int64()).Update()
			reds = append(reds, r.Map())
		}
	}
	// 第二遍：蓝字被红冲
	reversed := 0
	for _, r := range rows {
		if Detect(r["invoice_type"].String(), r["total"].Float64()) == StatusRed {
			continue // 红字发票不参与"被红冲"判定
		}
		for _, R := range reds {
			sameNo := R["original_invoice_no"] != nil && R["original_invoice_no"].(string) == r["invoice_no"].String()
			sameParty := R["seller_name"].(string) == r["seller_name"].String() && R["buyer_name"].(string) == r["buyer_name"].String()
			if sameNo || sameParty {
				g.DB().Model("invoice").Data(g.Map{"red_flush_status": StatusReversed}).Where("id", r["id"].Int64()).Update()
				reversed++
				break
			}
		}
	}
	return map[string]any{"red": len(reds), "reversed": reversed}, nil
}

// Alerts 红冲预警列表（red 或 reversed），按最新在前。
func Alerts(ctx context.Context, limit int) ([]g.Map, error) {
	if limit <= 0 || limit > 500 {
		limit = 200
	}
	rows, err := g.DB().Model("invoice").
		Where("red_flush_status <> ''").
		Order("id desc").Limit(limit).All()
	if err != nil {
		return nil, err
	}
	out := make([]g.Map, 0, len(rows))
	for _, r := range rows {
		out = append(out, r.Map())
	}
	return out, nil
}
