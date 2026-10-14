package contact

import (
	"errors"
	"log"
	"math"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/Archy-yang/tongpin/backend/internal/platform/httpx"
	"github.com/Archy-yang/tongpin/backend/internal/platform/middleware"
)

type Handler struct{ svc *Service }

func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

func (h *Handler) RegisterRoutes(rg gin.IRouter) {
	rg.POST("/contact/requests", h.create)
	rg.GET("/contact/requests", h.list)
	rg.POST("/contact/requests/:id/approve", h.approve)
	rg.POST("/contact/requests/:id/reject", h.reject)
	rg.POST("/contact/requests/:id/revoke", h.revoke)
	rg.GET("/dm/:match_id/messages", h.listDirect)
	rg.POST("/dm/:match_id/messages", h.sendDirect)
}

func (h *Handler) create(c *gin.Context) {
	var req CreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.FailBusiness(c, httpx.CodeInvalidParam, "参数错误")
		return
	}
	view, err := h.svc.Create(c.Request.Context(), middleware.UserIDFrom(c), req.MatchID)
	if err != nil {
		h.fail(c, err, "发起接触请求失败")
		return
	}
	httpx.OK(c, view)
}

func (h *Handler) list(c *gin.Context) {
	inbox := c.Query("box") != "outbox"
	reqs, err := h.svc.List(c.Request.Context(), middleware.UserIDFrom(c), inbox)
	if err != nil {
		httpx.FailInternal(c, "获取请求列表失败")
		return
	}
	httpx.OK(c, gin.H{"requests": reqs})
}

func (h *Handler) approve(c *gin.Context) {
	requestID, ok := pathID(c)
	if !ok {
		return
	}
	if err := h.svc.Approve(c.Request.Context(), middleware.UserIDFrom(c), requestID); err != nil {
		h.fail(c, err, "同意请求失败")
		return
	}
	httpx.OK(c, nil)
}

func (h *Handler) reject(c *gin.Context) {
	requestID, ok := pathID(c)
	if !ok {
		return
	}
	if err := h.svc.Reject(c.Request.Context(), middleware.UserIDFrom(c), requestID); err != nil {
		h.fail(c, err, "拒绝请求失败")
		return
	}
	httpx.OK(c, nil)
}

func (h *Handler) revoke(c *gin.Context) {
	requestID, ok := pathID(c)
	if !ok {
		return
	}
	if err := h.svc.Revoke(c.Request.Context(), middleware.UserIDFrom(c), requestID); err != nil {
		h.fail(c, err, "撤回请求失败")
		return
	}
	httpx.OK(c, nil)
}

func (h *Handler) listDirect(c *gin.Context) {
	matchID, ok := pathID(c)
	if !ok {
		return
	}
	limit := 30
	if v, err := strconv.Atoi(c.Query("limit")); err == nil && v > 0 {
		limit = min(v, 50)
	}
	before := int64(math.MaxInt64)
	if v, err := strconv.ParseInt(c.Query("before"), 10, 64); err == nil && v > 0 {
		before = v
	}
	msgs, err := h.svc.ListDirect(c.Request.Context(), middleware.UserIDFrom(c), matchID, before, limit)
	if err != nil {
		h.fail(c, err, "获取聊天记录失败")
		return
	}
	httpx.OK(c, gin.H{"messages": msgs})
}

func (h *Handler) sendDirect(c *gin.Context) {
	matchID, ok := pathID(c)
	if !ok {
		return
	}
	var req SendDirectRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.FailBusiness(c, httpx.CodeInvalidParam, "消息不能为空且不超过 2000 字")
		return
	}
	msg, err := h.svc.SendDirect(c.Request.Context(), middleware.UserIDFrom(c), matchID, req.Content)
	if err != nil {
		h.fail(c, err, "发送失败")
		return
	}
	httpx.OK(c, msg)
}

func (h *Handler) fail(c *gin.Context, err error, fallback string) {
	switch {
	case errors.Is(err, ErrNotParticipant), errors.Is(err, ErrNotRequestParty):
		httpx.Fail(c, 403, httpx.CodeForbidden, "无权操作")
	case errors.Is(err, ErrWrongState), errors.Is(err, ErrChatNotOpen):
		httpx.FailBusiness(c, httpx.CodeBusinessRule, "当前状态不支持该操作")
	case errors.Is(err, ErrBlocked):
		httpx.FailBusiness(c, httpx.CodeBusinessRule, "无法与该用户互动")
	default:
		log.Printf("[contact] user %d: %v", middleware.UserIDFrom(c), err)
		httpx.FailInternal(c, fallback)
	}
}

func pathID(c *gin.Context) (int64, bool) {
	if s := c.Param("match_id"); s != "" {
		v, err := strconv.ParseInt(s, 10, 64)
		if err != nil || v <= 0 {
			httpx.FailBusiness(c, httpx.CodeInvalidParam, "无效的 ID")
			return 0, false
		}
		return v, true
	}
	v, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || v <= 0 {
		httpx.FailBusiness(c, httpx.CodeInvalidParam, "无效的 ID")
		return 0, false
	}
	return v, true
}
