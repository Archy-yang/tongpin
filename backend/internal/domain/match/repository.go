package match

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Candidate 匹配候选（SQL 行的平铺）。
type Candidate struct {
	UserID            int64
	Intent            string
	AgePrefMin        *int
	AgePrefMax        *int
	AcceptRealContact bool
	Gender            string
	GenderPref        string
	AgeRange          string
	City              string
	Profile           []byte // 风格卡 JSON
	Embedding         string // pgvector ::text，形如 "[0.1,0.2,...]"
	Confidence        float64
}

type Repository struct {
	db *pgxpool.Pool
}

func NewRepository(db *pgxpool.Pool) *Repository {
	return &Repository{db: db}
}

func (r *Repository) UpsertOptIn(ctx context.Context, o OptIn) error {
	_, err := r.db.Exec(ctx, `
		INSERT INTO match_opt_ins (user_id, relationship_intent, age_pref_min, age_pref_max, accept_real_contact, status, updated_at)
		VALUES ($1, $2, $3, $4, $5, 'active', now())
		ON CONFLICT (user_id) DO UPDATE SET
			relationship_intent = $2, age_pref_min = $3, age_pref_max = $4,
			accept_real_contact = $5, status = 'active', updated_at = now()
	`, o.UserID, o.Intent, o.AgePrefMin, o.AgePrefMax, o.AcceptRealContact)
	return err
}

func (r *Repository) GetOptIn(ctx context.Context, userID int64) (*OptIn, error) {
	var o OptIn
	err := r.db.QueryRow(ctx, `
		SELECT user_id, relationship_intent, age_pref_min, age_pref_max, accept_real_contact, status
		FROM match_opt_ins WHERE user_id = $1
	`, userID).Scan(&o.UserID, &o.Intent, &o.AgePrefMin, &o.AgePrefMax, &o.AcceptRealContact, &o.Status)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &o, nil
}

// Withdraw 退出匹配池：向量移出候选集（status=withdrawn）。
func (r *Repository) Withdraw(ctx context.Context, userID int64) error {
	_, err := r.db.Exec(ctx, `
		UPDATE match_opt_ins SET status = 'withdrawn', updated_at = now() WHERE user_id = $1
	`, userID)
	return err
}

func (r *Repository) ActivePoolCount(ctx context.Context) (int, error) {
	var n int
	err := r.db.QueryRow(ctx,
		`SELECT COUNT(*) FROM match_opt_ins WHERE status = 'active'`).Scan(&n)
	return n, err
}

// Candidates 拉取全部有效候选：活跃 opt-in + 有 analysis 画像（向量）+ 未被分析抑制。
// 已与我有 match 记录的候选由 SQL 排除；拉黑在 service 层通过接口过滤。
func (r *Repository) Candidates(ctx context.Context, userID int64) ([]Candidate, error) {
	const q = `
		SELECT o.user_id, o.relationship_intent, o.age_pref_min, o.age_pref_max, o.accept_real_contact,
		       u.gender, COALESCE(u.gender_pref, ''), COALESCE(u.age_range, ''), COALESCE(u.city, ''),
		       p.profile, p.embedding::text, p.confidence
		FROM match_opt_ins o
		JOIN users u ON u.id = o.user_id AND u.status = 'active'
		JOIN style_profiles p ON p.user_id = o.user_id AND p.source = 'analysis'
		LEFT JOIN style_analysis_state s ON s.user_id = o.user_id
		WHERE o.status = 'active' AND o.user_id <> $1
		  AND COALESCE(s.suppressed, false) = false
		  AND NOT EXISTS (
		      SELECT 1 FROM matches m
		      WHERE (m.user_a = $1 AND m.user_b = o.user_id)
		         OR (m.user_a = o.user_id AND m.user_b = $1))
		ORDER BY o.user_id
	`
	rows, err := r.db.Query(ctx, q, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Candidate
	for rows.Next() {
		var c Candidate
		if err := rows.Scan(&c.UserID, &c.Intent, &c.AgePrefMin, &c.AgePrefMax, &c.AcceptRealContact,
			&c.Gender, &c.GenderPref, &c.AgeRange, &c.City, &c.Profile, &c.Embedding, &c.Confidence); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// MyProfile 候选集里也要拿到我自己的画像与向量（打分与 highlights 用）。
func (r *Repository) MyProfile(ctx context.Context, userID int64) (*Candidate, error) {
	var c Candidate
	err := r.db.QueryRow(ctx, `
		SELECT $1::bigint, COALESCE(o.relationship_intent, ''), o.age_pref_min, o.age_pref_max,
		       COALESCE(o.accept_real_contact, false),
		       u.gender, COALESCE(u.gender_pref, ''), COALESCE(u.age_range, ''), COALESCE(u.city, ''),
		       p.profile, p.embedding::text, p.confidence
		FROM users u
		LEFT JOIN match_opt_ins o ON o.user_id = u.id
		JOIN style_profiles p ON p.user_id = u.id AND p.source = 'analysis'
		WHERE u.id = $1
	`, userID).Scan(&c.UserID, &c.Intent, &c.AgePrefMin, &c.AgePrefMax, &c.AcceptRealContact,
		&c.Gender, &c.GenderPref, &c.AgeRange, &c.City, &c.Profile, &c.Embedding, &c.Confidence)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &c, nil
}

type MatchRow struct {
	ID           int64
	UserA        int64
	UserB        int64
	Score        float64
	ScoreDetail  []byte
	Status       string
	CreatedAt    time.Time
}

// InsertMatch 新建契合提示记录；同一对人已存在则忽略。
func (r *Repository) InsertMatch(ctx context.Context, userA, userB int64, score float64, detail []byte) (int64, error) {
	var id int64
	err := r.db.QueryRow(ctx, `
		INSERT INTO matches (user_a, user_b, score, score_detail)
		VALUES ($1, $2, $3, $4::jsonb)
		ON CONFLICT (user_a, user_b) DO NOTHING
		RETURNING id
	`, userA, userB, score, detail).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, nil // 已存在
	}
	return id, err
}

// MySuggested 我的全部 suggested 状态契合提示。
func (r *Repository) MySuggested(ctx context.Context, userID int64) ([]MatchRow, error) {
	return r.myMatches(ctx, userID, "suggested")
}

func (r *Repository) myMatches(ctx context.Context, userID int64, status string) ([]MatchRow, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id, user_a, user_b, score, COALESCE(score_detail, '{}'::jsonb), status, created_at
		FROM matches
		WHERE (user_a = $1 OR user_b = $1) AND status = $2
		ORDER BY score DESC
	`, userID, status)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []MatchRow
	for rows.Next() {
		var m MatchRow
		if err := rows.Scan(&m.ID, &m.UserA, &m.UserB, &m.Score, &m.ScoreDetail, &m.Status, &m.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// GetByID 查询单条匹配记录。
func (r *Repository) GetByID(ctx context.Context, matchID int64) (*MatchRow, error) {
	var m MatchRow
	err := r.db.QueryRow(ctx, `
		SELECT id, user_a, user_b, score, COALESCE(score_detail, '{}'::jsonb), status, created_at
		FROM matches WHERE id = $1
	`, matchID).Scan(&m.ID, &m.UserA, &m.UserB, &m.Score, &m.ScoreDetail, &m.Status, &m.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &m, nil
}

// UpdateStatus 状态机推进。
func (r *Repository) UpdateStatus(ctx context.Context, matchID int64, status string) error {
	_, err := r.db.Exec(ctx, `UPDATE matches SET status = $2 WHERE id = $1`, matchID, status)
	return err
}

// CloseByUser 用户撤回授权时关闭匹配（需为参与方）。
func (r *Repository) CloseByUser(ctx context.Context, matchID, userID int64) error {
	_, err := r.db.Exec(ctx, `
		UPDATE matches SET status = 'closed'
		WHERE id = $1 AND ($2 = user_a OR $2 = user_b) AND status <> 'closed'
	`, matchID, userID)
	return err
}
