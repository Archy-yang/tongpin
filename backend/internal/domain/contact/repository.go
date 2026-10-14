package contact

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// 本域只持有 contact_requests 与 direct_messages 两张表；
// matches 与 users 的读写通过消费方接口（装配层注入）。

type Repository struct {
	db *pgxpool.Pool
}

func NewRepository(db *pgxpool.Pool) *Repository {
	return &Repository{db: db}
}

func (r *Repository) InsertRequest(ctx context.Context, matchID, fromUser, toUser int64) (Request, error) {
	var req Request
	err := r.db.QueryRow(ctx, `
		INSERT INTO contact_requests (match_id, from_user, to_user) VALUES ($1, $2, $3)
		RETURNING id, created_at
	`, matchID, fromUser, toUser).Scan(&req.ID, &req.CreatedAt)
	if err != nil {
		return Request{}, err
	}
	req.MatchID = matchID
	req.FromUser = fromUser
	req.ToUser = toUser
	req.Status = "pending"
	return req, nil
}

func (r *Repository) GetRequest(ctx context.Context, requestID int64) (*Request, error) {
	var req Request
	err := r.db.QueryRow(ctx, `
		SELECT id, match_id, from_user, to_user, status, created_at
		FROM contact_requests WHERE id = $1
	`, requestID).Scan(&req.ID, &req.MatchID, &req.FromUser, &req.ToUser, &req.Status, &req.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &req, nil
}

func (r *Repository) UpdateRequestStatus(ctx context.Context, requestID int64, status string) error {
	_, err := r.db.Exec(ctx, `
		UPDATE contact_requests SET status = $2, updated_at = now() WHERE id = $1
	`, requestID, status)
	return err
}

func (r *Repository) ListRequests(ctx context.Context, userID int64, inbox bool, activeOnly bool) ([]Request, error) {
	cond := "from_user = $1"
	if inbox {
		cond = "to_user = $1"
	}
	statusCond := ""
	if activeOnly {
		statusCond = " AND status IN ('pending', 'approved')"
	}
	rows, err := r.db.Query(ctx, `
		SELECT id, match_id, from_user, to_user, status, created_at
		FROM contact_requests WHERE `+cond+statusCond+`
		ORDER BY id DESC LIMIT 50
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Request
	for rows.Next() {
		var req Request
		if err := rows.Scan(&req.ID, &req.MatchID, &req.FromUser, &req.ToUser, &req.Status, &req.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, req)
	}
	return out, rows.Err()
}

func (r *Repository) InsertDirectMessage(ctx context.Context, matchID, senderID int64, content string) (DirectMessage, error) {
	var m DirectMessage
	var createdAt time.Time
	err := r.db.QueryRow(ctx, `
		INSERT INTO direct_messages (match_id, sender_id, content) VALUES ($1, $2, $3)
		RETURNING id, created_at
	`, matchID, senderID, content).Scan(&m.ID, &createdAt)
	if err != nil {
		return DirectMessage{}, err
	}
	m.SenderID = senderID
	m.Content = content
	m.CreatedAt = createdAt
	return m, nil
}

func (r *Repository) ListDirectMessages(ctx context.Context, matchID, before int64, limit int) ([]DirectMessage, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id, sender_id, content, created_at FROM direct_messages
		WHERE match_id = $1 AND id < $2
		ORDER BY id DESC LIMIT $3
	`, matchID, before, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []DirectMessage
	for rows.Next() {
		var m DirectMessage
		if err := rows.Scan(&m.ID, &m.SenderID, &m.Content, &m.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}
