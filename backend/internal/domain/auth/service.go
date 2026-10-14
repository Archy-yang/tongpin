package auth

import (
	"context"
	"fmt"

	"github.com/Archy-yang/tongpin/backend/internal/platform/jwt"
)

type Service struct {
	repo   *Repository
	tokens *jwt.Issuer
}

func NewService(repo *Repository, tokens *jwt.Issuer) *Service {
	return &Service{repo: repo, tokens: tokens}
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
