package billing

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository struct{ db *pgxpool.Pool }

func NewRepository(db *pgxpool.Pool) *Repository { return &Repository{db: db} }

// ClaimEvent 原子认领事件（幂等保护）：返回 false 表示已被处理过，直接跳过。
func (r *Repository) ClaimEvent(ctx context.Context, eventID, provider string, payload []byte) (bool, error) {
	var id string
	err := r.db.QueryRow(ctx, `
		INSERT INTO billing_events (event_id, provider, payload) VALUES ($1, $2, $3)
		ON CONFLICT (event_id) DO NOTHING
		RETURNING event_id
	`, eventID, provider, payload).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

// UpsertSubscription 按三方订阅标识推进状态（存在则覆盖状态与周期）。
func (r *Repository) UpsertSubscription(ctx context.Context, s Subscription) error {
	_, err := r.db.Exec(ctx, `
		INSERT INTO subscriptions (user_id, provider, external_id, status, plan, current_period_end)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (provider, external_id) DO UPDATE
		SET status = EXCLUDED.status,
		    plan = EXCLUDED.plan,
		    current_period_end = EXCLUDED.current_period_end,
		    updated_at = now()
	`, s.UserID, s.Provider, s.ExternalID, s.Status, s.Plan, nullTime(s.CurrentPeriodEnd))
	return err
}

// UpdateStatusByExternalID 事件体没带用户时按三方订阅标识回查更新。
func (r *Repository) UpdateStatusByExternalID(ctx context.Context, provider, externalID, status string, periodEnd time.Time) error {
	_, err := r.db.Exec(ctx, `
		UPDATE subscriptions
		SET status = $3, current_period_end = $4, updated_at = now()
		WHERE provider = $1 AND external_id = $2
	`, provider, externalID, status, nullTime(periodEnd))
	return err
}

// HasActive 权益判断：存在未到期、状态 active 的订阅。
func (r *Repository) HasActive(ctx context.Context, userID int64) (bool, error) {
	var exists bool
	err := r.db.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM subscriptions
			WHERE user_id = $1 AND status = $2 AND current_period_end > now()
		)
	`, userID, StatusActive).Scan(&exists)
	return exists, err
}

// CurrentOf 返回用户最近一次的订阅（展示用），没有时返回 nil。
func (r *Repository) CurrentOf(ctx context.Context, userID int64) (*Subscription, error) {
	var s Subscription
	err := r.db.QueryRow(ctx, `
		SELECT id, user_id, provider, external_id, status, plan,
		       COALESCE(current_period_end, to_timestamp(0))
		FROM subscriptions WHERE user_id = $1
		ORDER BY updated_at DESC LIMIT 1
	`, userID).Scan(&s.ID, &s.UserID, &s.Provider, &s.ExternalID, &s.Status, &s.Plan, &s.CurrentPeriodEnd)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &s, nil
}

func nullTime(t time.Time) any {
	if t.IsZero() {
		return nil
	}
	return t
}
