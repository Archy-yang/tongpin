package chat

import (
	"log"
	"strconv"

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
	rg.POST("/chat/messages", h.send)
	rg.GET("/chat/messages", h.history)
	rg.GET("/chat/proactive", h.proactiveState)
	rg.PUT("/chat/proactive", h.setProactive)
}

func (h *Handler) proactiveState(c *gin.Context) {
	enabled, err := h.svc.GetProactiveEnabled(c.Request.Context(), middleware.UserIDFrom(c))
	if err != nil {
		httpx.FailInternal(c, "查询设置失败")
		return
	}
	httpx.OK(c, gin.H{"enabled": enabled})
}

func (h *Handler) setProactive(c *gin.Context) {
	var req struct {
		Enabled *bool `json:"enabled" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.Enabled == nil {
		httpx.FailBusiness(c, httpx.CodeInvalidParam, "参数错误")
		return
	}
	if err := h.svc.SetProactiveEnabled(c.Request.Context(), middleware.UserIDFrom(c), *req.Enabled); err != nil {
		httpx.FailInternal(c, "保存设置失败")
		return
	}
	httpx.OK(c, nil)
}

func (h *Handler) send(c *gin.Context) {
	var req SendMessageRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.FailBusiness(c, httpx.CodeInvalidParam, "消息不能为空且不超过 2000 字")
		return
	}
	resp, err := h.svc.SendMessage(c.Request.Context(), middleware.UserIDFrom(c), req.Content)
	if err != nil {
		log.Printf("[chat] send user %d: %v", middleware.UserIDFrom(c), err)
		httpx.FailInternal(c, "AI 走神了，请稍后再试")
		return
	}
	httpx.OK(c, resp)
}

func (h *Handler) history(c *gin.Context) {
	limit := 20
	if v, err := strconv.Atoi(c.Query("limit")); err == nil && v > 0 {
		limit = min(v, 50)
	}
	var before int64
	if v, err := strconv.ParseInt(c.Query("before"), 10, 64); err == nil && v > 0 {
		before = v
	}

	msgs, err := h.svc.History(c.Request.Context(), middleware.UserIDFrom(c), before, limit)
	if err != nil {
		httpx.FailInternal(c, "获取历史消息失败")
		return
	}
	httpx.OK(c, gin.H{"messages": msgs})
}
