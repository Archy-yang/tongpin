// Package contact 真人接触域：接触请求（双向同意）与一对一文本聊天。
// 与 chat 域物理隔离：真人会话没有 AI 写入通道（产品红线）。
package contact

import "time"

type Request struct {
	ID        int64     `json:"id"`
	MatchID   int64     `json:"match_id"`
	FromUser  int64     `json:"from_user"`
	ToUser    int64     `json:"to_user"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
}

type RequestView struct {
	Request
	CounterpartNickname string `json:"counterpart_nickname"`
}

type CreateRequest struct {
	MatchID int64 `json:"match_id" binding:"required"`
}

type DirectMessage struct {
	ID        int64     `json:"id"`
	SenderID  int64     `json:"sender_id"`
	IsMine    bool      `json:"is_mine"`
	Content   string    `json:"content"`
	CreatedAt time.Time `json:"created_at"`
}

type SendDirectRequest struct {
	Content string `json:"content" binding:"required,max=2000"`
}
