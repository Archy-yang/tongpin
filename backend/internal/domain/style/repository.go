package style

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type ProfileRecord struct {
	Version    int
	Profile    []byte // 风格卡原始 JSON
	Confidence float64
}

type Repository struct {
	db *pgxpool.Pool
}

func NewRepository(db *pgxpool.Pool) *Repository {
	return &Repository{db: db}
}

// CandidateUsers 发现需要分析的用户：自上次分析后有足够多新消息、且未被抑制。
func (r *Repository) CandidateUsers(ctx context.Context, minNewMessages int) ([]int64, error) {
	const q = `
		SELECT c.user_id
		FROM messages m
		JOIN conversations c ON c.id = m.conversation_id
		LEFT JOIN style_analysis_state s ON s.user_id = c.user_id
		WHERE m.id > COALESCE(s.last_message_id, 0)
		  AND COALESCE(s.suppressed, false) = false
		GROUP BY c.user_id
		HAVING COUNT(*) >= $1
		ORDER BY c.user_id
	`
	rows, err := r.db.Query(ctx, q, minNewMessages)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// LatestProfile 返回最新 analysis 版本风格卡；无则返回 nil。
func (r *Repository) LatestProfile(ctx context.Context, userID int64) (*ProfileRecord, error) {
	const q = `
		SELECT version, profile, confidence FROM style_profiles
		WHERE user_id = $1 AND source = 'analysis'
		ORDER BY version DESC LIMIT 1
	`
	var rec ProfileRecord
	err := r.db.QueryRow(ctx, q, userID).
		Scan(&rec.Version, &rec.Profile, &rec.Confidence)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &rec, nil
}

func (r *Repository) InsertProfile(ctx context.Context, userID int64, version int, profile []byte, confidence float64, embedding [embeddingDim]float32) error {
	const q = `
		INSERT INTO style_profiles (user_id, version, profile, confidence, embedding, source)
		VALUES ($1, $2, $3::jsonb, $4, $5::vector, 'analysis')
	`
	_, err := r.db.Exec(ctx, q, userID, version, profile, confidence, embeddingLiteral(embedding))
	return err
}

// DeleteAllProfiles 物理删除该用户全部风格卡版本与展示覆盖（隐私承诺：删除即删除）。
func (r *Repository) DeleteAllProfiles(ctx context.Context, userID int64) error {
	if _, err := r.db.Exec(ctx, `DELETE FROM style_profiles WHERE user_id = $1`, userID); err != nil {
		return err
	}
	_, err := r.db.Exec(ctx, `DELETE FROM style_display_overrides WHERE user_id = $1`, userID)
	return err
}

// GetState 返回分析状态：上次分析位置 + 是否被抑制。
func (r *Repository) GetState(ctx context.Context, userID int64) (lastMessageID int64, suppressed bool, err error) {
	err = r.db.QueryRow(ctx,
		`SELECT last_message_id, suppressed FROM style_analysis_state WHERE user_id = $1`, userID).
		Scan(&lastMessageID, &suppressed)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, false, nil
	}
	return
}

func (r *Repository) UpsertState(ctx context.Context, userID, lastMessageID int64) error {
	_, err := r.db.Exec(ctx, `
		INSERT INTO style_analysis_state (user_id, last_message_id, updated_at)
		VALUES ($1, $2, now())
		ON CONFLICT (user_id) DO UPDATE SET last_message_id = $2, updated_at = now()
	`, userID, lastMessageID)
	return err
}

func (r *Repository) SetSuppressed(ctx context.Context, userID int64, suppressed bool) error {
	_, err := r.db.Exec(ctx, `
		INSERT INTO style_analysis_state (user_id, suppressed, updated_at)
		VALUES ($1, $2, now())
		ON CONFLICT (user_id) DO UPDATE SET suppressed = $2, updated_at = now()
	`, userID, suppressed)
	return err
}

func (r *Repository) GetOverrides(ctx context.Context, userID int64) ([]byte, error) {
	var overrides []byte
	err := r.db.QueryRow(ctx,
		`SELECT overrides FROM style_display_overrides WHERE user_id = $1`, userID).
		Scan(&overrides)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return overrides, err
}

func (r *Repository) UpsertOverrides(ctx context.Context, userID int64, overrides []byte) error {
	_, err := r.db.Exec(ctx, `
		INSERT INTO style_display_overrides (user_id, overrides, updated_at)
		VALUES ($1, $2::jsonb, now())
		ON CONFLICT (user_id) DO UPDATE SET overrides = $2::jsonb, updated_at = now()
	`, userID, overrides)
	return err
}
