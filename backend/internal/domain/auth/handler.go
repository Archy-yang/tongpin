package auth

import (
	"errors"
	"net/http"

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

// RegisterRoutes 注册该域的公开路由（无需登录）。
func (h *Handler) RegisterPublicRoutes(rg gin.IRouter) {
	rg.POST("/auth/device", h.deviceLogin)
}

// RegisterRoutes 注册该域的登录态路由。
func (h *Handler) RegisterRoutes(rg gin.IRouter) {
	rg.GET("/me", h.me)
	rg.PUT("/me", h.updateMe)
}

func (h *Handler) deviceLogin(c *gin.Context) {
	var req DeviceLoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.FailBusiness(c, httpx.CodeInvalidParam, "参数错误")
		return
	}
	resp, err := h.svc.DeviceLogin(c.Request.Context(), req.DeviceID)
	if err != nil {
		if errors.Is(err, ErrUserBanned) {
			httpx.Fail(c, http.StatusForbidden, httpx.CodeForbidden, "账号不可用")
			return
		}
		httpx.FailInternal(c, "登录失败")
		return
	}
	httpx.OK(c, resp)
}

func (h *Handler) me(c *gin.Context) {
	profile, err := h.svc.GetProfile(c.Request.Context(), middleware.UserIDFrom(c))
	if err != nil {
		h.failBanned(c, err, "获取用户信息失败")
		return
	}
	httpx.OK(c, profile)
}

func (h *Handler) updateMe(c *gin.Context) {
	var req UpdateBasicsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.FailBusiness(c, httpx.CodeInvalidParam, "参数错误（检查枚举取值与长度）")
		return
	}
	if err := h.svc.UpdateBasics(c.Request.Context(), middleware.UserIDFrom(c), req); err != nil {
		h.failBanned(c, err, "更新设置失败")
		return
	}
	httpx.OK(c, nil)
}

func (h *Handler) failBanned(c *gin.Context, err error, fallback string) {
	if errors.Is(err, ErrUserBanned) {
		httpx.Fail(c, http.StatusForbidden, httpx.CodeForbidden, "账号不可用")
		return
	}
	httpx.FailInternal(c, fallback)
}
