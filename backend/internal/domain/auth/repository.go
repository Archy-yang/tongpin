package auth

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrUserBanned = errors.New("user banned")

type Repository struct {
	db *pgxpool.Pool
}

func NewRepository(db *pgxpool.Pool) *Repository {
	return &Repository{db: db}
}

// UpsertByDeviceID 按设备号查用户，不存在则创建。返回用户及是否为新用户。
func (r *Repository) UpsertByDeviceID(ctx context.Context, deviceID string) (User, bool, error) {
	const q = `
		INSERT INTO users (device_id, nickname)
		VALUES ($1, $2)
		ON CONFLICT (device_id) DO UPDATE SET updated_at = now()
		RETURNING id, device_id, nickname, status, created_at, (xmax = 0) AS inserted
	`
	var (
		u        User
		inserted bool
	)
	err := r.db.QueryRow(ctx, q, deviceID, GenerateNickname()).
		Scan(&u.ID, &u.DeviceID, &u.Nickname, &u.Status, &u.CreatedAt, &inserted)
	if err != nil {
		return User{}, false, err
	}
	if u.Status != "active" {
		return User{}, false, ErrUserBanned
	}
	return u, inserted, nil
}

func (r *Repository) GetByID(ctx context.Context, id int64) (User, error) {
	const q = `SELECT id, device_id, nickname, status, created_at FROM users WHERE id = $1`
	var u User
	err := r.db.QueryRow(ctx, q, id).
		Scan(&u.ID, &u.DeviceID, &u.Nickname, &u.Status, &u.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, ErrUserBanned // 统一按封禁处理，不泄露用户存在性
	}
	if err != nil {
		return User{}, err
	}
	return u, nil
}

// UpdateBasics 更新用户基础设置（匹配硬条件来源）。
func (r *Repository) UpdateBasics(ctx context.Context, id int64, b UpdateBasicsRequest) error {
	_, err := r.db.Exec(ctx, `
		UPDATE users SET
			nickname   = CASE WHEN $2 <> '' THEN $2 ELSE nickname END,
			age_range  = CASE WHEN $3 <> '' THEN $3 ELSE age_range END,
			gender     = CASE WHEN $4 <> '' THEN $4 ELSE gender END,
			city       = CASE WHEN $5 <> '' THEN $5 ELSE city END,
			gender_pref = CASE WHEN $6 <> '' THEN $6 ELSE gender_pref END,
			updated_at = now()
		WHERE id = $1
	`, id, b.Nickname, b.AgeRange, b.Gender, b.City, b.GenderPref)
	return err
}

// Nickname 查询昵称（供其他域展示用）。
func (r *Repository) Nickname(ctx context.Context, id int64) (string, error) {
	var nickname string
	err := r.db.QueryRow(ctx, `SELECT nickname FROM users WHERE id = $1 AND status = 'active'`, id).Scan(&nickname)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrUserBanned
	}
	return nickname, err
}

// BasicUser 跨域提供的基础信息（昵称 + 匹配硬条件字段）。
type BasicUser struct {
	Nickname   string
	Gender     string
	GenderPref string
	AgeRange   string
	City       string
}

// GetBasics 基础信息（供 match 域硬条件过滤）。
func (r *Repository) GetBasics(ctx context.Context, id int64) (BasicUser, error) {
	var b BasicUser
	err := r.db.QueryRow(ctx, `
		SELECT nickname, COALESCE(gender, ''), COALESCE(gender_pref, ''),
		       COALESCE(age_range, ''), COALESCE(city, '')
		FROM users WHERE id = $1 AND status = 'active'
	`, id).Scan(&b.Nickname, &b.Gender, &b.GenderPref, &b.AgeRange, &b.City)
	if errors.Is(err, pgx.ErrNoRows) {
		return BasicUser{}, ErrUserBanned
	}
	return b, err
}
