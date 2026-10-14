package consent

import (
	"context"
	"fmt"

	"github.com/gin-gonic/gin"

	"github.com/Archy-yang/tongpin/backend/internal/platform/httpx"
	"github.com/Archy-yang/tongpin/backend/internal/platform/middleware"
)

// MatchCloser 撤回真人接触授权时需要关闭对应匹配（消费方接口，装配层注入 match 的实现）。
type MatchCloser interface {
	CloseByUser(ctx context.Context, matchID, userID int64) error
}

type Service struct {
	repo  *Repository
	match MatchCloser
}

func NewService(repo *Repository, match MatchCloser) *Service {
	return &Service{repo: repo, match: match}
}

// Record 供其他域写入审计记录。
func (s *Service) Record(ctx context.Context, userID int64, targetType string, targetID *int64, granted bool) error {
	return s.repo.Insert(ctx, userID, targetType, targetID, granted)
}

// List 我的授权历史（审计可见）。
func (s *Service) List(ctx context.Context, userID int64) ([]Record, error) {
	return s.repo.ListByUser(ctx, userID)
}

// Revoke 撤回授权。match_pool 撤回请走 DELETE /match/opt-in；
// real_contact 撤回在此处理：记录审计 + 关闭对应匹配（真人聊天随 match 关闭而终止）。
func (s *Service) Revoke(ctx context.Context, userID int64, targetType string, targetID *int64) error {
	switch targetType {
	case TargetMatchPool:
		return fmt.Errorf("match_pool 撤回请使用退出匹配池接口")
	case TargetRealContact:
		if targetID == nil || *targetID <= 0 {
			return fmt.Errorf("real_contact 撤回需提供 match_id")
		}
		if err := s.match.CloseByUser(ctx, *targetID, userID); err != nil {
			return err
		}
		return s.repo.Insert(ctx, userID, targetType, targetID, false)
	default:
		return fmt.Errorf("unknown target_type")
	}
}

type Handler struct{ svc *Service }

func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

func (h *Handler) RegisterRoutes(rg gin.IRouter) {
	rg.GET("/consents", h.list)
	rg.POST("/consents/revoke", h.revoke)
}

func (h *Handler) list(c *gin.Context) {
	records, err := h.svc.List(c.Request.Context(), middleware.UserIDFrom(c))
	if err != nil {
		httpx.FailInternal(c, "获取授权记录失败")
		return
	}
	httpx.OK(c, gin.H{"consents": records})
}

type revokeRequest struct {
	TargetType string `json:"target_type" binding:"required"`
	TargetID   *int64 `json:"target_id"`
}

func (h *Handler) revoke(c *gin.Context) {
	var req revokeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.FailBusiness(c, httpx.CodeInvalidParam, "参数错误")
		return
	}
	if err := h.svc.Revoke(c.Request.Context(), middleware.UserIDFrom(c), req.TargetType, req.TargetID); err != nil {
		httpx.FailBusiness(c, httpx.CodeBusinessRule, err.Error())
		return
	}
	httpx.OK(c, nil)
}
