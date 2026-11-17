package controller

import (
	"context"
	"os"

	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/net/ghttp"

	"vouchchain/pkg/longterm"
)

// A5 长期保存/国密合规封装层：
//   POST /api/longterm/archive   归档登记：{sourceType, sourceId, srcPath, remark} → {id,version,digest,tsTime}
//   GET  /api/longterm?sourceType=   登记列表
//   POST /api/longterm/{id}/verify   防篡改校验：{srcPath?} → {digestOk, decryptOk}

func LongTermArchive(r *ghttp.Request) {
	srcPath := r.Get("srcPath").String()
	if _, err := os.Stat(srcPath); err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": "srcPath 文件不存在: " + err.Error()})
		return
	}
	res, err := longterm.Archive(context.Background(),
		r.Get("sourceType").String(), r.Get("sourceId").Int64(), srcPath, r.Get("remark").String())
	if err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	audit(r, "longterm.archive", "longterm_archive", res["id"].(int64), res)
	r.Response.WriteJsonExit(g.Map{"code": 0, "data": res})
}

func LongTermList(r *ghttp.Request) {
	rows, err := longterm.List(context.Background(), r.Get("sourceType").String(), r.Get("limit").Int())
	if err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	r.Response.WriteJsonExit(g.Map{"code": 0, "data": rows})
}

func LongTermVerify(r *ghttp.Request) {
	res, err := longterm.Verify(context.Background(), r.Get("id").Int64(), r.Get("srcPath").String())
	if err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	r.Response.WriteJsonExit(g.Map{"code": 0, "data": res})
}
