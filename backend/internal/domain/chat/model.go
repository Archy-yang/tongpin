package chat

import "time"

// Conversation 一位用户对应一个 AI 陪伴会话。
type Conversation struct {
	ID              int64
	UserID          int64
	CreatedAt       time.Time
	MemorySummary   string // 长期记忆摘要（异步 worker 维护，跨会话注入 system prompt）
	LastMemoryMsgID int64  // 记忆已覆盖到的最大消息 ID
}

// Message 聊天消息。sender = "user" | "ai"，入库即确定 AI 标识。
type Message struct {
	ID          int64
	Sender      string
	Content     string
	CreatedAt   time.Time
}

type MessageDTO struct {
	ID        int64     `json:"id"`
	Sender    string    `json:"sender"`
	IsAI      bool      `json:"is_ai"` // 前端渲染 AI 消息必须展示 AI 标识
	Content   string    `json:"content"`
	CreatedAt time.Time `json:"created_at"`
}

type SendMessageRequest struct {
	Content string `json:"content" binding:"required,max=2000"`
}

type SendMessageResponse struct {
	UserMessage MessageDTO `json:"user_message"`
	AIMessage   MessageDTO `json:"ai_message"`
}

func toDTO(m Message) MessageDTO {
	return MessageDTO{
		ID:        m.ID,
		Sender:    m.Sender,
		IsAI:      m.Sender == "ai",
		Content:   m.Content,
		CreatedAt: m.CreatedAt,
	}
}
