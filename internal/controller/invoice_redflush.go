package controller

import (
	"context"

	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/net/ghttp"

	"vouchchain/pkg/invoice_redflush"
)

// A2 发票红冲预警：
//   POST /api/invoices/red-flush-scan    全量扫描红冲并落库，返回 {red, reversed}
//   GET  /api/invoices/red-flush-alerts  红冲预警列表（red / reversed）

func InvoiceRedFlushScan(r *ghttp.Request) {
	ctx := context.Background()
	res, err := invoice_redflush.ScanAndMark(ctx)
	if err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	audit(r, "invoice.red_flush_scan", "invoice", 0, res)
	r.Response.WriteJsonExit(g.Map{"code": 0, "data": res})
}

func InvoiceRedFlushAlerts(r *ghttp.Request) {
	rows, err := invoice_redflush.Alerts(context.Background(), r.Get("limit").Int())
	if err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	r.Response.WriteJsonExit(g.Map{"code": 0, "data": rows})
}
