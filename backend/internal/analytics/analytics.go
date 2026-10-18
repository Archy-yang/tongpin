// Package analytics 行为埋点：只记计数不记内容，为留存/成本/转化提供数据。
// 平台层定位（同 llm/realtime）：业务域直接 import，事件名保持全局一致。
package analytics

import (
	"context"
	"encoding/json"
	"log"

	"github.com/jackc/pgx/v5/pgxpool"
)

// 事件名清单（写入此处的名字即对外契约，改动需同步 metrics 查询）。
const (
	EvAppOpen        = "app_open"         // 用户打开应用（前端每次加载报一次）
	EvChatUserMsg    = "chat_user_msg"    // 用户发一条 AI 陪伴消息
	EvLLMCall        = "llm_call"         // 一次 LLM 调用（token 用量在 props）
	EvIcebreakCreate = "icebreak_create"  // 发起一次 AI 暖场
	EvDMSent         = "dm_sent"          // 真人聊天发一条消息
)

type Recorder struct{ db *pgxpool.Pool }

func New(db *pgxpool.Pool) *Recorder { return &Recorder{db: db} }

// Record 异步尽力而为地记一条事件：绝不阻塞业务路径，失败只打日志。
// ctx 传调用方的即可，内部会解除取消关联。
func (r *Recorder) Record(ctx context.Context, userID int64, name string, props map[string]any) {
	go func() {
		defer func() {
			if p := recover(); p != nil {
				log.Printf("[analytics] panic: %v", p)
			}
		}()
		var raw any
		if props != nil {
			if b, err := json.Marshal(props); err == nil {
				raw = b
			}
		}
		if _, err := r.db.Exec(context.WithoutCancel(ctx),
			`INSERT INTO analytics_events (user_id, name, props) VALUES ($1, $2, $3)`,
			userID, name, raw); err != nil {
			log.Printf("[analytics] record %s (user %d): %v", name, userID, err)
		}
	}()
}
