package match

import (
	"errors"
	"log"

	"github.com/gin-gonic/gin"

	"github.com/Archy-yang/tongpin/backend/internal/platform/httpx"
	"github.com/Archy-yang/tongpin/backend/internal/platform/middleware"
)

type Handler struct {
	svc *Service
}

func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

// RegisterRoutes 注册登录态路由。
func (h *Handler) RegisterRoutes(rg gin.IRouter) {
	rg.POST("/match/opt-in", h.optIn)
	rg.GET("/match/opt-in", h.myOptIn)
	rg.DELETE("/match/opt-in", h.withdraw)
	rg.GET("/match/suggestions", h.suggestions)
	rg.POST("/match/feedback", h.feedback)
}

func (h *Handler) feedback(c *gin.Context) {
	var req struct {
		MatchID   int64  `json:"match_id" binding:"required"`
		Direction string `json:"direction" binding:"required,oneof=like mismatch"`
		Note      string `json:"note" binding:"max=500"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.FailBusiness(c, httpx.CodeInvalidParam, "参数错误")
		return
	}
	if err := h.svc.SaveFeedback(c.Request.Context(), middleware.UserIDFrom(c), req.MatchID, req.Direction, req.Note); err != nil {
		if errors.Is(err, ErrNotParticipant) {
			httpx.Fail(c, 403, httpx.CodeForbidden, "无权操作")
			return
		}
		httpx.FailInternal(c, "提交反馈失败")
		return
	}
	httpx.OK(c, nil)
}

func (h *Handler) optIn(c *gin.Context) {
	var req OptInRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.FailBusiness(c, httpx.CodeInvalidParam, "参数错误")
		return
	}
	view, err := h.svc.OptIn(c.Request.Context(), middleware.UserIDFrom(c), req)
	if err != nil {
		if errors.Is(err, ErrProfileNotReady) {
			httpx.FailBusiness(c, httpx.CodeBusinessRule, "风格画像尚未生成，先和 AI 多聊聊吧")
			return
		}
		if errors.Is(err, errInvalidOptIn) {
			httpx.FailBusiness(c, httpx.CodeInvalidParam, "关系意图或年龄偏好无效")
			return
		}
		httpx.FailInternal(c, "进入匹配池失败")
		log.Printf("[match] opt-in user %d: %v", middleware.UserIDFrom(c), err)
		return
	}
	httpx.OK(c, view)
}

func (h *Handler) myOptIn(c *gin.Context) {
	view, err := h.svc.MyOptIn(c.Request.Context(), middleware.UserIDFrom(c))
	if err != nil {
		httpx.FailInternal(c, "查询匹配状态失败")
		return
	}
	httpx.OK(c, view) // nil 表示从未 opt-in
}

func (h *Handler) withdraw(c *gin.Context) {
	err := h.svc.Withdraw(c.Request.Context(), middleware.UserIDFrom(c))
	if err != nil {
		if errors.Is(err, ErrNotOptedIn) {
			httpx.FailBusiness(c, httpx.CodeBusinessRule, "当前不在匹配池中")
			return
		}
		httpx.FailInternal(c, "退出匹配池失败")
		return
	}
	httpx.OK(c, nil)
}

func (h *Handler) suggestions(c *gin.Context) {
	view, err := h.svc.Suggestions(c.Request.Context(), middleware.UserIDFrom(c))
	if err != nil {
		if errors.Is(err, ErrProfileNotReady) {
			httpx.FailBusiness(c, httpx.CodeBusinessRule, "风格画像尚未生成")
			return
		}
		httpx.FailInternal(c, "获取契合提示失败")
		log.Printf("[match] suggestions user %d: %v", middleware.UserIDFrom(c), err)
		return
	}
	httpx.OK(c, view)
}

var errInvalidOptIn = errors.New("invalid opt-in")
