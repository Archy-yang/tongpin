// Package safety 安全域：拉黑与举报（MVP 只做基础能力；内容审核管道接入为后续阶段）。
package safety

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository struct {
	db *pgxpool.Pool
}

func NewRepository(db *pgxpool.Pool) *Repository {
	return &Repository{db: db}
}

func (r *Repository) Block(ctx context.Context, blocker, blocked int64) error {
	_, err := r.db.Exec(ctx, `
		INSERT INTO blocks (blocker_id, blocked_id) VALUES ($1, $2)
		ON CONFLICT (blocker_id, blocked_id) DO NOTHING
	`, blocker, blocked)
	return err
}

// BlockedBetween 双向拉黑检查。
func (r *Repository) BlockedBetween(ctx context.Context, a, b int64) (bool, error) {
	var n int
	err := r.db.QueryRow(ctx, `
		SELECT COUNT(*) FROM blocks
		WHERE (blocker_id = $1 AND blocked_id = $2) OR (blocker_id = $2 AND blocked_id = $1)
	`, a, b).Scan(&n)
	return n > 0, err
}

type Report struct {
	TargetID int64  `json:"target_id" binding:"required"`
	Reason   string `json:"reason" binding:"required,max=32"`
	Detail   string `json:"detail" binding:"max=2000"`
}

func (r *Repository) InsertReport(ctx context.Context, reporterID int64, rep Report) error {
	_, err := r.db.Exec(ctx, `
		INSERT INTO reports (reporter_id, target_id, reason, detail) VALUES ($1, $2, $3, $4)
	`, reporterID, rep.TargetID, rep.Reason, rep.Detail)
	return err
}

type Service struct{ repo *Repository }

func NewService(repo *Repository) *Service { return &Service{repo: repo} }

func (s *Service) Block(ctx context.Context, blocker, blocked int64) error {
	if blocker == blocked {
		return errSelfBlock
	}
	return s.repo.Block(ctx, blocker, blocked)
}

// BlockedBetween 实现 match/contact 等域消费的 BlockChecker 接口。
func (s *Service) BlockedBetween(ctx context.Context, a, b int64) (bool, error) {
	return s.repo.BlockedBetween(ctx, a, b)
}

func (s *Service) Report(ctx context.Context, reporterID int64, rep Report) error {
	return s.repo.InsertReport(ctx, reporterID, rep)
}
