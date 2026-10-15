// Package realtime WebSocket 实时推送：Hub 按用户管理连接，业务域经接口发布事件。
package realtime

import (
	"encoding/json"
	"log"
	"sync"
)

// Frame 推送到客户端的消息帧。
type Frame struct {
	Type    string `json:"type"`              // "dm"（真人聊天新消息）
	MatchID int64  `json:"match_id,omitempty"`
	Message any    `json:"message,omitempty"`
}

// Conn 一个已鉴权的 WebSocket 连接。
type Conn struct {
	userID int64
	send   chan []byte
	closed chan struct{}
}

// Hub 连接注册表：userID → 该用户的所有活跃连接（支持多端登录）。
type Hub struct {
	mu    sync.RWMutex
	conns map[int64]map[*Conn]struct{}
}

func NewHub() *Hub {
	return &Hub{conns: make(map[int64]map[*Conn]struct{})}
}

func (h *Hub) add(c *Conn) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.conns[c.userID] == nil {
		h.conns[c.userID] = make(map[*Conn]struct{})
	}
	h.conns[c.userID][c] = struct{}{}
}

func (h *Hub) remove(c *Conn) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if set, ok := h.conns[c.userID]; ok {
		delete(set, c)
		if len(set) == 0 {
			delete(h.conns, c.userID)
		}
	}
}

// Broadcast 尽力推送：不阻塞、失败静默（客户端有轮询兜底与重连）。
func (h *Hub) Broadcast(userIDs []int64, payload any) {
	data, err := json.Marshal(payload)
	if err != nil {
		return
	}
	h.mu.RLock()
	defer h.mu.RUnlock()
	for _, uid := range userIDs {
		for c := range h.conns[uid] {
			select {
			case c.send <- data:
			default:
				// 发送缓冲满：丢弃该帧，靠兜底轮询补齐
				log.Printf("[realtime] user %d send buffer full, frame dropped", uid)
			}
		}
	}
}
