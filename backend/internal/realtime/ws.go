package realtime

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"

	"github.com/Archy-yang/tongpin/backend/internal/platform/jwt"
)

const (
	writeWait  = 10 * time.Second
	pongWait   = 60 * time.Second
	pingPeriod = 30 * time.Second
	sendBuffer = 32
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	// MVP 部署经同源反代；后续收紧为白名单
	CheckOrigin: func(r *http.Request) bool { return true },
}

// Serve 处理 WS 升级与连接生命周期。token 走查询参数
// （浏览器 WebSocket 无法自定义 Authorization 头）。
func Serve(hub *Hub, issuer *jwt.Issuer) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID, err := issuer.Parse(c.Query("token"))
		if err != nil {
			c.AbortWithStatus(http.StatusUnauthorized)
			return
		}

		ws, err := upgrader.Upgrade(c.Writer, c.Request, nil)
		if err != nil {
			return
		}
		conn := &Conn{
			userID: userID,
			send:   make(chan []byte, sendBuffer),
			closed: make(chan struct{}),
		}
		hub.add(conn)
		go conn.writePump(ws)
		go conn.readPump(hub, ws)
	}
}

// readPump 只为保活：业务数据经 REST 发送，WS 是单向推送通道。
// 收到任何客户端消息或错误都视为连接异常，清理退出。
func (c *Conn) readPump(hub *Hub, ws *websocket.Conn) {
	defer func() {
		hub.remove(c)
		close(c.closed)
		_ = ws.Close()
	}()
	ws.SetReadLimit(1024)
	_ = ws.SetReadDeadline(time.Now().Add(pongWait))
	ws.SetPongHandler(func(string) error {
		return ws.SetReadDeadline(time.Now().Add(pongWait))
	})
	for {
		if _, _, err := ws.ReadMessage(); err != nil {
			return
		}
	}
}

func (c *Conn) writePump(ws *websocket.Conn) {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		_ = ws.Close()
	}()
	for {
		select {
		case data, ok := <-c.send:
			_ = ws.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				_ = ws.WriteMessage(websocket.CloseMessage, nil)
				return
			}
			if err := ws.WriteMessage(websocket.TextMessage, data); err != nil {
				return
			}
		case <-ticker.C:
			_ = ws.SetWriteDeadline(time.Now().Add(writeWait))
			if err := ws.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		case <-c.closed:
			return
		}
	}
}
