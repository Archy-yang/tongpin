package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/Archy-yang/tongpin/backend/internal/platform/jwt"
)

type Service struct {
	repo   *Repository
	tokens *jwt.Issuer
	mailer Mailer
}

func NewService(repo *Repository, tokens *jwt.Issuer, mailer Mailer) *Service {
	return &Service{repo: repo, tokens: tokens, mailer: mailer}
}

// DeviceLogin 设备匿名登录：不存在则注册，存在则直接登录。
func (s *Service) DeviceLogin(ctx context.Context, deviceID string) (LoginResponse, error) {
	user, isNew, err := s.repo.UpsertByDeviceID(ctx, deviceID)
	if err != nil {
		return LoginResponse{}, fmt.Errorf("upsert user: %w", err)
	}
	token, err := s.tokens.Issue(user.ID)
	if err != nil {
		return LoginResponse{}, fmt.Errorf("issue token: %w", err)
	}
	return LoginResponse{
		Token: token,
		User:  UserDTO{ID: user.ID, Nickname: user.Nickname},
		IsNew: isNew,
	}, nil
}

// GetProfile 当前用户信息。
func (s *Service) GetProfile(ctx context.Context, userID int64) (UserDTO, error) {
	user, err := s.repo.GetByID(ctx, userID)
	if err != nil {
		return UserDTO{}, err
	}
	return UserDTO{ID: user.ID, Nickname: user.Nickname}, nil
}

// Nickname 供其他域（contact）通过接口调用。
func (s *Service) Nickname(ctx context.Context, userID int64) (string, error) {
	return s.repo.Nickname(ctx, userID)
}

// UpdateBasics 更新基础设置。
func (s *Service) UpdateBasics(ctx context.Context, userID int64, req UpdateBasicsRequest) error {
	return s.repo.UpdateBasics(ctx, userID, req)
}

// Basic 供 match 域硬条件过滤（消费方接口适配）。
func (s *Service) Basic(ctx context.Context, userID int64) (BasicUser, error) {
	return s.repo.GetBasics(ctx, userID)
}

// ---- 邮箱 + 密码 ----

var (
	ErrEmailTaken        = errors.New("email taken")
	ErrBadCredentials    = errors.New("bad credentials")
	ErrCodeInvalid       = errors.New("code invalid or expired")
	ErrCodeRateLimited   = errors.New("code request too frequent")
	ErrWrongOldPassword  = errors.New("wrong old password")
	ErrNoPasswordSet     = errors.New("account has no password")
)

// Mailer 本域对邮件能力的最小依赖（消费方接口，装配层注入）。
type Mailer interface {
	Send(ctx context.Context, to, subject, body string) error
}

const resetCodeSubject = "同频 · 密码找回验证码"

func (s *Service) sendCode(ctx context.Context, email, code string) error {
	return s.mailer.Send(ctx, email, resetCodeSubject,
		fmt.Sprintf("你的验证码是：%s\n15 分钟内有效。如果这不是你的操作，请忽略本邮件。", code))
}

// EmailRegister 邮箱注册：即注册即登录。
func (s *Service) EmailRegister(ctx context.Context, email, password string) (LoginResponse, error) {
	if _, err := s.repo.GetByEmail(ctx, email); err == nil {
		return LoginResponse{}, ErrEmailTaken
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return LoginResponse{}, err
	}
	user, err := s.repo.CreateWithEmail(ctx, email, string(hash))
	if err != nil {
		return LoginResponse{}, err
	}
	token, err := s.tokens.Issue(user.ID)
	if err != nil {
		return LoginResponse{}, err
	}
	return LoginResponse{Token: token, User: UserDTO{ID: user.ID, Nickname: user.Nickname}, IsNew: true}, nil
}

// EmailLogin 邮箱密码登录。
func (s *Service) EmailLogin(ctx context.Context, email, password string) (LoginResponse, error) {
	user, err := s.repo.GetByEmail(ctx, email)
	if err != nil {
		return LoginResponse{}, ErrBadCredentials
	}
	if user.Status != "active" {
		return LoginResponse{}, ErrUserBanned
	}
	if user.PasswordHash == nil ||
		bcrypt.CompareHashAndPassword([]byte(*user.PasswordHash), []byte(password)) != nil {
		return LoginResponse{}, ErrBadCredentials
	}
	token, err := s.tokens.Issue(user.ID)
	if err != nil {
		return LoginResponse{}, err
	}
	return LoginResponse{Token: token, User: UserDTO{ID: user.ID, Nickname: user.Nickname}}, nil
}

// BindEmail 设备匿名账号绑定邮箱（升级为正式账号，数据全保留）。
func (s *Service) BindEmail(ctx context.Context, userID int64, email, password string) (AccountDTO, error) {
	if _, err := s.repo.GetByEmail(ctx, email); err == nil {
		return AccountDTO{}, ErrEmailTaken
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return AccountDTO{}, err
	}
	if err := s.repo.BindEmail(ctx, userID, email, string(hash)); err != nil {
		return AccountDTO{}, err
	}
	return s.Account(ctx, userID)
}

// ChangePassword 登录态修改密码（需旧密码）。
func (s *Service) ChangePassword(ctx context.Context, userID int64, oldPassword, newPassword string) error {
	user, err := s.repo.GetByID(ctx, userID)
	if err != nil {
		return err
	}
	if user.PasswordHash == nil {
		return ErrNoPasswordSet
	}
	if bcrypt.CompareHashAndPassword([]byte(*user.PasswordHash), []byte(oldPassword)) != nil {
		return ErrWrongOldPassword
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	return s.repo.SetPassword(ctx, userID, string(hash))
}

// RequestReset 发送找回密码验证码（限流：同邮箱 60 秒一封）。
// 邮箱不存在时不报错也不发信——不泄露注册状态。
func (s *Service) RequestReset(ctx context.Context, email string) error {
	if last, ok, err := s.repo.LastCodeAt(ctx, email, "reset"); err != nil {
		return err
	} else if ok && time.Since(last) < time.Minute {
		return ErrCodeRateLimited
	}
	user, err := s.repo.GetByEmail(ctx, email)
	if err != nil {
		return nil // 静默：不泄露邮箱是否已注册
	}
	_ = user
	code := generateCode()
	hash := codeHash(code)
	if err := s.repo.SaveCode(ctx, email, hash, "reset"); err != nil {
		return err
	}
	return s.sendCode(ctx, email, code)
}

// ConfirmReset 验证码 + 新密码完成找回。
func (s *Service) ConfirmReset(ctx context.Context, email, code, newPassword string) error {
	ok, err := s.repo.ConsumeCode(ctx, email, codeHash(code), "reset")
	if err != nil {
		return err
	}
	if !ok {
		return ErrCodeInvalid
	}
	user, err := s.repo.GetByEmail(ctx, email)
	if err != nil {
		return ErrCodeInvalid // 不泄露邮箱是否存在
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	return s.repo.SetPassword(ctx, user.ID, string(hash))
}

// Account 当前账号形态（含邮箱，供前端判断）。
func (s *Service) Account(ctx context.Context, userID int64) (AccountDTO, error) {
	u, err := s.repo.GetByID(ctx, userID)
	if err != nil {
		return AccountDTO{}, err
	}
	return AccountDTO{ID: u.ID, Nickname: u.Nickname, Email: u.Email}, nil
}

func generateCode() string {
	n, _ := rand.Int(rand.Reader, big.NewInt(1000000))
	return fmt.Sprintf("%06d", n.Int64())
}

func codeHash(code string) string {
	sum := sha256.Sum256([]byte("tongpin:" + code))
	return hex.EncodeToString(sum[:])
}
