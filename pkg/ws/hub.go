package ws

import (
	"encoding/json"
	"sync"

	"github.com/gogf/gf/v2/net/ghttp"
)

// 进度推送中心：抽取任务状态/进度变化时广播给所有已连接页面（任务中心实时可见）。
// 自托管单点工具场景，进程内 hub 即可，无需外置 MQ。
// 连接类型用 gf 封装的 *ghttp.WebSocket（升级由 r.WebSocket() 完成）；
// 消息类型常量 1 = 文本帧（gorilla websocket.TextMessage 的值，避免直接依赖其类型别名差异）。

const textMessage = 1

var (
	mu      sync.Mutex
	clients = make(map[*ghttp.WebSocket]struct{})
)

// Event 推送事件结构
type Event struct {
	Type string      `json:"type"` // "task"
	Data interface{} `json:"data"`
}

// HandlerTask WS 端点：升级连接 → 注册 → 读循环保活，断开即注销
func HandlerTask(r *ghttp.Request) {
	conn, err := r.WebSocket()
	if err != nil {
		r.Response.WriteStatus(400, "websocket upgrade failed")
		return
	}
	mu.Lock()
	clients[conn] = struct{}{}
	mu.Unlock()
	defer func() {
		mu.Lock()
		delete(clients, conn)
		mu.Unlock()
		conn.Close()
	}()
	// 读循环：仅保活，忽略业务消息
	for {
		if _, _, err := conn.ReadMessage(); err != nil {
			break
		}
	}
}

// Broadcast 向所有在线客户端广播事件；发送失败的连接顺手清理
func Broadcast(evt Event) {
	data, err := json.Marshal(evt)
	if err != nil {
		return
	}
	mu.Lock()
	defer mu.Unlock()
	for conn := range clients {
		if err := conn.WriteMessage(textMessage, data); err != nil {
			delete(clients, conn)
			conn.Close()
		}
	}
}

// ClientCount 在线订阅数（诊断用）
func ClientCount() int {
	mu.Lock()
	defer mu.Unlock()
	return len(clients)
}
