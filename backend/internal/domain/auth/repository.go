package auth

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrUserBanned        = errors.New("user banned")
	ErrUserNotFound      = errors.New("user not found")
	ErrEmailAlreadyBound = errors.New("email already bound")
)

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
		RETURNING id, device_id, nickname, email, password_hash, status, created_at, (xmax = 0) AS inserted
	`
	var (
		u        User
		inserted bool
	)
	err := r.db.QueryRow(ctx, q, deviceID, GenerateNickname()).
		Scan(&u.ID, &u.DeviceID, &u.Nickname, &u.Email, &u.PasswordHash, &u.Status, &u.CreatedAt, &inserted)
	if err != nil {
		return User{}, false, err
	}
	if u.Status != "active" {
		return User{}, false, ErrUserBanned
	}
	return u, inserted, nil
}

func (r *Repository) GetByID(ctx context.Context, id int64) (User, error) {
	const q = `SELECT id, device_id, nickname, email, password_hash, status, created_at FROM users WHERE id = $1`
	var u User
	err := r.db.QueryRow(ctx, q, id).
		Scan(&u.ID, &u.DeviceID, &u.Nickname, &u.Email, &u.PasswordHash, &u.Status, &u.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, ErrUserBanned // 统一按封禁处理，不泄露用户存在性
	}
	if err != nil {
		return User{}, err
	}
	return u, nil
}

// GetByEmail 按邮箱查用户（登录 / 找回密码）。
func (r *Repository) GetByEmail(ctx context.Context, email string) (User, error) {
	const q = `SELECT id, device_id, nickname, email, password_hash, status, created_at FROM users WHERE email = $1`
	var u User
	err := r.db.QueryRow(ctx, q, email).
		Scan(&u.ID, &u.DeviceID, &u.Nickname, &u.Email, &u.PasswordHash, &u.Status, &u.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, ErrUserNotFound
	}
	return u, err
}

// CreateWithEmail 邮箱注册新用户。
func (r *Repository) CreateWithEmail(ctx context.Context, email, passwordHash string) (User, error) {
	const q = `
		INSERT INTO users (nickname, email, password_hash, email_verified)
		VALUES ($1, $2, $3, true)
		RETURNING id, device_id, nickname, email, password_hash, status, created_at
	`
	var u User
	err := r.db.QueryRow(ctx, q, GenerateNickname(), email, passwordHash).
		Scan(&u.ID, &u.DeviceID, &u.Nickname, &u.Email, &u.PasswordHash, &u.Status, &u.CreatedAt)
	return u, err
}

// BindEmail 给现有账号绑定邮箱 + 密码（设备匿名账号升级）。
func (r *Repository) BindEmail(ctx context.Context, userID int64, email, passwordHash string) error {
	tag, err := r.db.Exec(ctx, `
		UPDATE users SET email = $2, password_hash = $3, email_verified = true, updated_at = now()
		WHERE id = $1 AND email IS NULL
	`, userID, email, passwordHash)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrEmailAlreadyBound
	}
	return nil
}

// SetPassword 直接更新密码哈希。
func (r *Repository) SetPassword(ctx context.Context, userID int64, hash string) error {
	_, err := r.db.Exec(ctx,
		`UPDATE users SET password_hash = $2, email_verified = true, updated_at = now() WHERE id = $1`,
		userID, hash)
	return err
}

// SaveCode 记录邮箱验证码（哈希存储，15 分钟有效）。
func (r *Repository) SaveCode(ctx context.Context, email, codeHash, purpose string) error {
	_, err := r.db.Exec(ctx, `
		INSERT INTO email_codes (email, code_hash, purpose, expires_at)
		VALUES ($1, $2, $3, now() + interval '15 minutes')
	`, email, codeHash, purpose)
	return err
}

// ConsumeCode 校验并消费验证码：有效返回 true（同码只能用一次）。
func (r *Repository) ConsumeCode(ctx context.Context, email, codeHash, purpose string) (bool, error) {
	tag, err := r.db.Exec(ctx, `
		UPDATE email_codes SET used_at = now()
		WHERE id = (
			SELECT id FROM email_codes
			WHERE email = $1 AND purpose = $2 AND code_hash = $3
			  AND used_at IS NULL AND expires_at > now()
			ORDER BY id DESC LIMIT 1
		)
	`, email, purpose, codeHash)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

// LastCodeAt 最近的验证码创建时间（限流：同邮箱 60s 内只发一条）。
func (r *Repository) LastCodeAt(ctx context.Context, email, purpose string) (time.Time, bool, error) {
	var t time.Time
	err := r.db.QueryRow(ctx, `
		SELECT created_at FROM email_codes WHERE email = $1 AND purpose = $2
		ORDER BY id DESC LIMIT 1
	`, email, purpose).Scan(&t)
	if errors.Is(err, pgx.ErrNoRows) {
		return time.Time{}, false, nil
	}
	if err != nil {
		return time.Time{}, false, err
	}
	return t, true, nil
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

// HasEmail 账号是否已绑定邮箱（match/billing 的门槛判断用）。
func (r *Repository) HasEmail(ctx context.Context, id int64) (bool, error) {
	var bound bool
	err := r.db.QueryRow(ctx,
		`SELECT COALESCE(email, '') <> '' FROM users WHERE id = $1`, id).Scan(&bound)
	return bound, err
}

// SetFreeQuota 写用户级免费额度（绑定邮箱的激励动作；chat 域读取该列）。
func (r *Repository) SetFreeQuota(ctx context.Context, id int64, quota int) error {
	_, err := r.db.Exec(ctx, `UPDATE users SET free_quota = $2 WHERE id = $1`, id, quota)
	return err
}

// ---- 设备注册 IP 限流 ----

// CountSignupsToday 该 IP 今天已创建的新账号数。
func (r *Repository) CountSignupsToday(ctx context.Context, ip string) (int, error) {
	var n int
	err := r.db.QueryRow(ctx,
		`SELECT count FROM signup_rate WHERE ip = $1 AND day = CURRENT_DATE`, ip).Scan(&n)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, nil
	}
	return n, err
}

// RecordSignup 计一次新账号（原子自增，当日首见则建行）。
func (r *Repository) RecordSignup(ctx context.Context, ip string) error {
	_, err := r.db.Exec(ctx, `
		INSERT INTO signup_rate (ip, day, count) VALUES ($1, CURRENT_DATE, 1)
		ON CONFLICT (ip, day) DO UPDATE SET count = signup_rate.count + 1
	`, ip)
	return err
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
