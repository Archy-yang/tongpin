// Package consent 授权审计域：授权记录只追加，撤回即新增 granted=false 记录。
package consent

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	TargetMatchPool   = "match_pool"
	TargetRealContact = "real_contact"
)

type Record struct {
	ID         int64     `json:"id"`
	TargetType string    `json:"target_type"`
	TargetID   *int64    `json:"target_id,omitempty"`
	Granted    bool      `json:"granted"`
	CreatedAt  time.Time `json:"created_at"`
}

type Repository struct {
	db *pgxpool.Pool
}

func NewRepository(db *pgxpool.Pool) *Repository {
	return &Repository{db: db}
}

func (r *Repository) Insert(ctx context.Context, userID int64, targetType string, targetID *int64, granted bool) error {
	_, err := r.db.Exec(ctx, `
		INSERT INTO consents (user_id, target_type, target_id, granted) VALUES ($1, $2, $3, $4)
	`, userID, targetType, targetID, granted)
	return err
}

func (r *Repository) ListByUser(ctx context.Context, userID int64) ([]Record, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id, target_type, target_id, granted, created_at
		FROM consents WHERE user_id = $1 ORDER BY id DESC LIMIT 100
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Record
	for rows.Next() {
		var rec Record
		if err := rows.Scan(&rec.ID, &rec.TargetType, &rec.TargetID, &rec.Granted, &rec.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, rec)
	}
	return out, rows.Err()
}
