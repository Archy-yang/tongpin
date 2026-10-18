package billing

import (
	"errors"
	"log"

	"github.com/gin-gonic/gin"

	"github.com/Archy-yang/tongpin/backend/internal/platform/httpx"
	"github.com/Archy-yang/tongpin/backend/internal/platform/middleware"
)

type Handler struct{ svc *Service }

func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

// RegisterRoutes 注册登录态路由（发起购买、查询会员状态）。
func (h *Handler) RegisterRoutes(rg gin.IRouter) {
	rg.POST("/billing/checkout", h.checkout)
	rg.GET("/billing/subscription", h.subscription)
}

// RegisterWebhookRoutes 注册公开的 webhook 路由——鉴权靠渠道验签，不走登录态。
func (h *Handler) RegisterWebhookRoutes(rg gin.IRouter) {
	rg.POST("/billing/webhook/:provider", h.webhook)
}

func (h *Handler) checkout(c *gin.Context) {
	url, err := h.svc.StartCheckout(c.Request.Context(), middleware.UserIDFrom(c), "")
	if err != nil {
		if errors.Is(err, ErrEmailRequired) {
			httpx.FailBusiness(c, httpx.CodeBusinessRule, "购买前先绑定邮箱（我的 → 绑定邮箱）")
			return
		}
		if errors.Is(err, ErrProviderUnavailable) {
			httpx.FailBusiness(c, httpx.CodeBusinessRule, "支付暂未开放，敬请期待")
			return
		}
		log.Printf("[billing] checkout user %d: %v", middleware.UserIDFrom(c), err)
		httpx.FailInternal(c, "发起支付失败，请稍后再试")
		return
	}
	httpx.OK(c, gin.H{"url": url})
}

func (h *Handler) subscription(c *gin.Context) {
	sub, err := h.svc.Current(c.Request.Context(), middleware.UserIDFrom(c))
	if err != nil {
		httpx.FailInternal(c, "查询会员状态失败")
		return
	}
	httpx.OK(c, sub)
}

// webhook 三方支付回调：验签 → 归一化 → 状态机。验签失败拒绝（三方会重试）。
func (h *Handler) webhook(c *gin.Context) {
	body, err := c.GetRawData()
	if err != nil {
		c.Status(400)
		return
	}
	ev, err := h.svc.ParseWebhook(c.Param("provider"), c.Request.Header, body)
	if err != nil {
		if errors.Is(err, ErrEventIgnored) {
			c.JSON(200, gin.H{"received": true, "ignored": true})
			return
		}
		// 验签失败/未知渠道：不打错误日志（公网会收到大量无效探测）
		log.Printf("[billing] webhook rejected: %v", err)
		c.JSON(400, gin.H{"error": "invalid webhook"})
		return
	}
	if err := h.svc.HandleEvent(c.Request.Context(), ev, body); err != nil {
		log.Printf("[billing] event %s failed: %v", ev.ID, err)
		c.JSON(500, gin.H{"error": "process failed"}) // 让三方重试
		return
	}
	c.JSON(200, gin.H{"received": true})
}
