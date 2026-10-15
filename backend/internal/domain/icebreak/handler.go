package icebreak

import (
	"errors"

	"github.com/gin-gonic/gin"

	"github.com/Archy-yang/tongpin/backend/internal/platform/httpx"
	"github.com/Archy-yang/tongpin/backend/internal/platform/middleware"
)

type Handler struct{ svc *Service }

func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

func (h *Handler) RegisterRoutes(rg gin.IRouter) {
	rg.POST("/icebreak/requests", h.create)
	rg.GET("/icebreak/mine", h.mine)
	rg.POST("/icebreak/sessions/:id/approve", h.approve)
	rg.POST("/icebreak/sessions/:id/decline", h.decline)
}

func (h *Handler) create(c *gin.Context) {
	var req struct {
		MatchID int64 `json:"match_id" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.FailBusiness(c, httpx.CodeInvalidParam, "参数错误")
		return
	}
	sess, err := h.svc.Create(c.Request.Context(), middleware.UserIDFrom(c), req.MatchID)
	if err != nil {
		h.fail(c, err, "发起暖场失败")
		return
	}
	httpx.OK(c, sess)
}

func (h *Handler) mine(c *gin.Context) {
	sessions, err := h.svc.ForUser(c.Request.Context(), middleware.UserIDFrom(c))
	if err != nil {
		httpx.FailInternal(c, "获取暖场会话失败")
		return
	}
	httpx.OK(c, gin.H{"sessions": sessions})
}

func (h *Handler) approve(c *gin.Context) {
	id, ok := pathID(c)
	if !ok {
		return
	}
	if err := h.svc.Approve(c.Request.Context(), middleware.UserIDFrom(c), id); err != nil {
		h.fail(c, err, "同意暖场失败")
		return
	}
	httpx.OK(c, nil)
}

func (h *Handler) decline(c *gin.Context) {
	id, ok := pathID(c)
	if !ok {
		return
	}
	if err := h.svc.Decline(c.Request.Context(), middleware.UserIDFrom(c), id); err != nil {
		h.fail(c, err, "婉拒暖场失败")
		return
	}
	httpx.OK(c, nil)
}

func (h *Handler) fail(c *gin.Context, err error, fallback string) {
	switch {
	case errors.Is(err, ErrNotParticipant), errors.Is(err, ErrNotRequestParty):
		httpx.Fail(c, 403, httpx.CodeForbidden, "无权操作")
	case errors.Is(err, ErrWrongState), errors.Is(err, ErrSessionExists):
		httpx.FailBusiness(c, httpx.CodeBusinessRule, err.Error())
	default:
		httpx.FailInternal(c, fallback)
	}
}

func pathID(c *gin.Context) (int64, bool) {
	v, ok := atoi64(c.Param("id"))
	if !ok {
		httpx.FailBusiness(c, httpx.CodeInvalidParam, "无效的 ID")
		return 0, false
	}
	return v, true
}
