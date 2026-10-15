package icebreak

import (
	"context"
	"encoding/json"
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

// Create 新建暖场请求（每人每匹配一个会话）。
func (r *Repository) Create(ctx context.Context, matchID, fromUser, toUser int64) (Session, error) {
	var s Session
	err := r.db.QueryRow(ctx, `
		INSERT INTO ai_icebreak_sessions (match_id, from_user, to_user)
		VALUES ($1, $2, $3)
		RETURNING id, match_id, from_user, to_user, status
	`, matchID, fromUser, toUser).Scan(&s.ID, &s.MatchID, &s.FromUser, &s.ToUser, &s.Status)
	return s, err
}

func (r *Repository) GetByMatch(ctx context.Context, matchID int64) (*Session, error) {
	row := r.db.QueryRow(ctx, `
		SELECT id, match_id, from_user, to_user, status,
		       COALESCE(transcript, 'null'::jsonb), COALESCE(summary, 'null'::jsonb)
		FROM ai_icebreak_sessions WHERE match_id = $1
	`, matchID)
	return scanSession(row.Scan)
}

func (r *Repository) GetByID(ctx context.Context, sessionID int64) (*Session, error) {
	row := r.db.QueryRow(ctx, `
		SELECT id, match_id, from_user, to_user, status,
		       COALESCE(transcript, 'null'::jsonb), COALESCE(summary, 'null'::jsonb)
		FROM ai_icebreak_sessions WHERE id = $1
	`, sessionID)
	return scanSession(row.Scan)
}

// ForUser 我参与的全部暖场会话（按 match 分组给前端）。
func (r *Repository) ForUser(ctx context.Context, userID int64) ([]Session, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id, match_id, from_user, to_user, status,
		       COALESCE(transcript, 'null'::jsonb), COALESCE(summary, 'null'::jsonb)
		FROM ai_icebreak_sessions
		WHERE from_user = $1 OR to_user = $1
		ORDER BY id DESC LIMIT 50
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Session
	for rows.Next() {
		var s Session
		var transcript, summary json.RawMessage
		if err := rows.Scan(&s.ID, &s.MatchID, &s.FromUser, &s.ToUser, &s.Status, &transcript, &summary); err != nil {
			return nil, err
		}
		_ = json.Unmarshal(transcript, &s.Transcript)
		if string(summary) != "null" {
			var sum Summary
			if json.Unmarshal(summary, &sum) == nil {
				s.Summary = &sum
			}
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

func (r *Repository) UpdateStatus(ctx context.Context, sessionID int64, status string) error {
	_, err := r.db.Exec(ctx,
		`UPDATE ai_icebreak_sessions SET status = $2, updated_at = now() WHERE id = $1`, sessionID, status)
	return err
}

func (r *Repository) SaveResult(ctx context.Context, sessionID int64, transcript []Turn, summary *Summary) error {
	tJSON, err := json.Marshal(transcript)
	if err != nil {
		return err
	}
	var sJSON any
	if summary != nil {
		sJSON, err = json.Marshal(summary)
		if err != nil {
			return err
		}
	}
	_, err = r.db.Exec(ctx, `
		UPDATE ai_icebreak_sessions
		SET transcript = $2::jsonb, summary = $3::jsonb, status = $4, updated_at = now()
		WHERE id = $1
	`, sessionID, tJSON, sJSON, StatusReady)
	return err
}

type scanFunc func(dest ...any) error

func scanSession(scan scanFunc) (*Session, error) {
	var s Session
	var transcript, summary json.RawMessage
	err := scan(&s.ID, &s.MatchID, &s.FromUser, &s.ToUser, &s.Status, &transcript, &summary)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	_ = json.Unmarshal(transcript, &s.Transcript)
	if string(summary) != "null" {
		var sum Summary
		if json.Unmarshal(summary, &sum) == nil {
			s.Summary = &sum
		}
	}
	return &s, nil
}
