package safety

import (
	"errors"

	"github.com/gin-gonic/gin"

	"github.com/Archy-yang/tongpin/backend/internal/platform/httpx"
	"github.com/Archy-yang/tongpin/backend/internal/platform/middleware"
)

var errSelfBlock = errors.New("cannot block self")

type Handler struct{ svc *Service }

func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

func (h *Handler) RegisterRoutes(rg gin.IRouter) {
	rg.POST("/blocks", h.block)
	rg.POST("/reports", h.report)
}

type blockRequest struct {
	UserID int64 `json:"user_id" binding:"required"`
}

func (h *Handler) block(c *gin.Context) {
	var req blockRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.FailBusiness(c, httpx.CodeInvalidParam, "参数错误")
		return
	}
	if err := h.svc.Block(c.Request.Context(), middleware.UserIDFrom(c), req.UserID); err != nil {
		if errors.Is(err, errSelfBlock) {
			httpx.FailBusiness(c, httpx.CodeBusinessRule, "不能拉黑自己")
			return
		}
		httpx.FailInternal(c, "拉黑失败")
		return
	}
	httpx.OK(c, nil)
}

func (h *Handler) report(c *gin.Context) {
	var req Report
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.FailBusiness(c, httpx.CodeInvalidParam, "参数错误")
		return
	}
	if err := h.svc.Report(c.Request.Context(), middleware.UserIDFrom(c), req); err != nil {
		httpx.FailInternal(c, "举报失败")
		return
	}
	httpx.OK(c, nil)
}
