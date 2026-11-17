package controller

import (
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/net/ghttp"

	"vouchchain/pkg/seal"
)

// R-B11 电子签平台盖章记录回填：
//
//	GET    /api/es/config                 读外部电子签平台配置（凭据脱敏）
//	POST   /api/es/config                 保存配置（凭据传空=不改）
//	POST   /api/seal-applies/{id}/es-stamp   按用印申请回填电子章记录（未配置明确失败）
//	GET    /api/es/records?applyId=       回填记录留痕
//
// 口径：电子真章不做（CA 资质墙）；只回填外部平台结果。未配置/平台失败一律明确报错，不谎报成功。

func ESConfigGet(r *ghttp.Request) {
	cfg, err := seal.ESConfigGet(r.Context())
	if err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	r.Response.WriteJsonExit(g.Map{"code": 0, "data": g.Map{
		"platform": cfg.Platform, "endpoint": cfg.Endpoint, "token": cfg.Token,
		"enabled": cfg.Enabled, "remark": cfg.Remark,
	}})
}

func ESConfigSave(r *ghttp.Request) {
	var req struct {
		Platform string `json:"platform"`
		Endpoint string `json:"endpoint"`
		Token    string `json:"token"`
		Enabled  string `json:"enabled"`
		Remark   string `json:"remark"`
	}
	if err := r.Parse(&req); err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	by := r.GetCtxVar("user").String()
	if err := seal.ESConfigSave(r.Context(), seal.ESConfig{
		Platform: req.Platform, Endpoint: req.Endpoint, Token: req.Token,
		Enabled: req.Enabled, Remark: req.Remark,
	}, by); err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	audit(r, "seal.es.config", "seal_es_config", 1, g.Map{
		"platform": req.Platform, "enabled": req.Enabled, "hasEndpoint": req.Endpoint != "",
	})
	r.Response.WriteJsonExit(g.Map{"code": 0})
}

// SealESStamp R-B11：拉外部平台盖章记录并回填用印台账（未配置明确失败）
func SealESStamp(r *ghttp.Request) {
	id := r.Get("id").Int64()
	by := r.GetCtxVar("user").String()
	recID, err := seal.ESSync(r.Context(), id, by)
	if err != nil {
		// 审计留痕：失败也记（便于追溯"为什么没回填"）
		audit(r, "seal.es.stamp.fail", "seal_apply", id, g.Map{"error": err.Error()})
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	audit(r, "seal.es.stamp", "seal_apply", id, g.Map{"recordId": recID})
	r.Response.WriteJsonExit(g.Map{"code": 0, "data": g.Map{"recordId": recID}})
}

func ESRecordList(r *ghttp.Request) {
	rows, err := seal.ESRecordList(r.Context(), r.Get("applyId").Int64())
	if err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	out := make([]g.Map, 0, len(rows))
	for _, row := range rows {
		out = append(out, row.Map())
	}
	r.Response.WriteJsonExit(g.Map{"code": 0, "data": out})
}
