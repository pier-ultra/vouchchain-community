package controller

import (
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/net/ghttp"

	"vouchchain/pkg/duban"
)

// 公文督办接口（v23 线D R-D1/R-D2）：
//   POST   /api/duban/tasks                  建督办任务（会议决议/领导批示/其他）
//   GET    /api/duban/tasks?status=&priority=&sourceType=&owner=  列表（按条件过滤）
//   GET    /api/duban/board                  督办看板（待办/逾期/已完成）
//   POST   /api/duban/tasks/{id}/remind      催办
//   POST   /api/duban/tasks/{id}/done        完成督办
//   GET    /api/duban/tasks/{id}/log         督办留痕链
//   GET    /api/duban/stats                  督办统计

func DubanCreate(r *ghttp.Request) {
	var req struct {
		Title      string `json:"title" v:"required"`
		Content    string `json:"content"`
		Owner      string `json:"owner"`
		SourceType string `json:"sourceType" v:"required"`
		Priority   string `json:"priority"`
		Deadline   string `json:"deadline"`
		ContractID int64  `json:"contractFileId"`
		Remark     string `json:"remark"`
	}
	if err := r.Parse(&req); err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	if req.Priority == "" {
		req.Priority = duban.PriorityMid
	}
	user := r.GetCtxVar("user").String()
	id, err := duban.CreateTask(r.Context(), req.Title, req.Content, req.Owner, req.SourceType, req.Priority, req.Deadline, req.ContractID, req.Remark, user)
	if err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	audit(r, "duban.create", "duban_task", id, g.Map{"sourceType": req.SourceType, "title": req.Title})
	r.Response.WriteJsonExit(g.Map{"code": 0, "data": g.Map{"id": id}})
}

func DubanList(r *ghttp.Request) {
	f := duban.BoardFilter{
		Status:     r.Get("status").String(),
		Priority:   r.Get("priority").String(),
		SourceType: r.Get("sourceType").String(),
		OnlyOwner:  r.Get("owner").String(),
	}
	rows, err := duban.ListTasks(r.Context(), f, r.Get("limit").Int())
	if err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	r.Response.WriteJsonExit(g.Map{"code": 0, "data": rows})
}

func DubanBoard(r *ghttp.Request) {
	f := duban.BoardFilter{
		Status:     r.Get("status").String(),
		Priority:   r.Get("priority").String(),
		SourceType: r.Get("sourceType").String(),
		OnlyOwner:  r.Get("owner").String(),
	}
	board, err := duban.Board(r.Context(), f)
	if err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	r.Response.WriteJsonExit(g.Map{"code": 0, "data": board})
}

func DubanRemind(r *ghttp.Request) {
	id := r.Get("id").Int64()
	user := r.GetCtxVar("user").String()
	if err := duban.Remind(r.Context(), id, user, r.Get("note").String()); err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	audit(r, "duban.remind", "duban_task", id, nil)
	r.Response.WriteJsonExit(g.Map{"code": 0})
}

func DubanDone(r *ghttp.Request) {
	id := r.Get("id").Int64()
	user := r.GetCtxVar("user").String()
	if err := duban.Done(r.Context(), id, user, r.Get("note").String()); err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	audit(r, "duban.done", "duban_task", id, nil)
	r.Response.WriteJsonExit(g.Map{"code": 0})
}

func DubanLog(r *ghttp.Request) {
	rows, err := duban.LogList(r.Context(), r.Get("id").Int64(), r.Get("limit").Int())
	if err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	r.Response.WriteJsonExit(g.Map{"code": 0, "data": rows})
}

func DubanStats(r *ghttp.Request) {
	s, err := duban.Stats(r.Context())
	if err != nil {
		r.Response.WriteJsonExit(g.Map{"code": 1, "message": err.Error()})
		return
	}
	r.Response.WriteJsonExit(g.Map{"code": 0, "data": s})
}
