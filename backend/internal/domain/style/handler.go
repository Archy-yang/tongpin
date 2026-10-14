package style

import (
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
	rg.GET("/style-profile", h.view)
	rg.PUT("/style-profile", h.updateDisplay)
	rg.DELETE("/style-profile", h.delete)
	rg.POST("/style-profile/resume", h.resume)
}

func (h *Handler) view(c *gin.Context) {
	view, err := h.svc.View(c.Request.Context(), middleware.UserIDFrom(c))
	if err != nil {
		httpx.FailInternal(c, "获取风格卡失败")
		return
	}
	httpx.OK(c, view)
}

func (h *Handler) updateDisplay(c *gin.Context) {
	var req UpdateDisplayRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.FailBusiness(c, httpx.CodeInvalidParam, "display_overrides 不能为空")
		return
	}
	if err := h.svc.UpdateDisplay(c.Request.Context(), middleware.UserIDFrom(c), req.DisplayOverrides); err != nil {
		httpx.FailBusiness(c, httpx.CodeBusinessRule, "编辑内容无效（需为合法 JSON 且不超过 8KB）")
		return
	}
	httpx.OK(c, nil)
}

func (h *Handler) delete(c *gin.Context) {
	if err := h.svc.Delete(c.Request.Context(), middleware.UserIDFrom(c)); err != nil {
		httpx.FailInternal(c, "删除风格卡失败")
		return
	}
	httpx.OK(c, nil)
}

func (h *Handler) resume(c *gin.Context) {
	if err := h.svc.Resume(c.Request.Context(), middleware.UserIDFrom(c)); err != nil {
		httpx.FailInternal(c, "重新开启分析失败")
		return
	}
	httpx.OK(c, nil)
}
