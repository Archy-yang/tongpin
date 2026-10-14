package chat

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository struct {
	db *pgxpool.Pool
}

func NewRepository(db *pgxpool.Pool) *Repository {
	return &Repository{db: db}
}

// GetOrCreateConversation 每个用户固定一个 AI 陪伴会话。
func (r *Repository) GetOrCreateConversation(ctx context.Context, userID int64) (Conversation, error) {
	const q = `
		INSERT INTO conversations (user_id) VALUES ($1)
		ON CONFLICT (user_id) DO NOTHING
		RETURNING id, user_id, created_at
	`
	var c Conversation
	err := r.db.QueryRow(ctx, q, userID).Scan(&c.ID, &c.UserID, &c.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) { // 已存在，走查询
		err = r.db.QueryRow(ctx,
			`SELECT id, user_id, created_at FROM conversations WHERE user_id = $1`, userID).
			Scan(&c.ID, &c.UserID, &c.CreatedAt)
	}
	if err != nil {
		return Conversation{}, err
	}
	return c, nil
}

func (r *Repository) InsertMessage(ctx context.Context, conversationID int64, sender, content string) (Message, error) {
	const q = `
		INSERT INTO messages (conversation_id, sender, content)
		VALUES ($1, $2, $3)
		RETURNING id, sender, content, created_at
	`
	var m Message
	err := r.db.QueryRow(ctx, q, conversationID, sender, content).
		Scan(&m.ID, &m.Sender, &m.Content, &m.CreatedAt)
	if err != nil {
		return Message{}, err
	}
	return m, nil
}

// RecentMessages 取最近 limit 条消息，按时间正序返回（供 LLM 上下文使用）。
func (r *Repository) RecentMessages(ctx context.Context, conversationID int64, limit int) ([]Message, error) {
	const q = `
		SELECT id, sender, content, created_at FROM (
			SELECT id, sender, content, created_at FROM messages
			WHERE conversation_id = $1 ORDER BY id DESC LIMIT $2
		) t ORDER BY id ASC
	`
	rows, err := r.db.Query(ctx, q, conversationID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Message
	for rows.Next() {
		var m Message
		if err := rows.Scan(&m.ID, &m.Sender, &m.Content, &m.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// MessagesSinceByUser 返回某用户 afterID 之后的消息（正序），供风格分析取数。
func (r *Repository) MessagesSinceByUser(ctx context.Context, userID, afterID int64, limit int) ([]Message, error) {
	const q = `
		SELECT m.id, m.sender, m.content, m.created_at
		FROM messages m
		JOIN conversations c ON c.id = m.conversation_id
		WHERE c.user_id = $1 AND m.id > $2
		ORDER BY m.id ASC
		LIMIT $3
	`
	rows, err := r.db.Query(ctx, q, userID, afterID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Message
	for rows.Next() {
		var m Message
		if err := rows.Scan(&m.ID, &m.Sender, &m.Content, &m.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// CountByUser 统计某用户的消息总数。
func (r *Repository) CountByUser(ctx context.Context, userID int64) (int, error) {
	var n int
	err := r.db.QueryRow(ctx, `
		SELECT COUNT(*) FROM messages m
		JOIN conversations c ON c.id = m.conversation_id
		WHERE c.user_id = $1
	`, userID).Scan(&n)
	return n, err
}

// ListBefore 游标分页：返回 id < before 的消息，倒序（新的在前）。
func (r *Repository) ListBefore(ctx context.Context, conversationID, before int64, limit int) ([]Message, error) {
	const q = `
		SELECT id, sender, content, created_at FROM messages
		WHERE conversation_id = $1 AND id < $2
		ORDER BY id DESC LIMIT $3
	`
	rows, err := r.db.Query(ctx, q, conversationID, before, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Message
	for rows.Next() {
		var m Message
		if err := rows.Scan(&m.ID, &m.Sender, &m.Content, &m.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}
