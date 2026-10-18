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
	rg.POST("/auth/email/bind", h.bindEmail)
	rg.POST("/auth/password", h.changePassword)
}

// RegisterEmailRoutes 注册邮箱体系的公开路由。
func (h *Handler) RegisterEmailRoutes(rg gin.IRouter) {
	rg.POST("/auth/email/register", h.emailRegister)
	rg.POST("/auth/email/login", h.emailLogin)
	rg.POST("/auth/email/reset-request", h.resetRequest)
	rg.POST("/auth/email/reset-confirm", h.resetConfirm)
}

func (h *Handler) emailRegister(c *gin.Context) {
	var req EmailCredentialsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.FailBusiness(c, httpx.CodeInvalidParam, "邮箱格式不正确，密码需 8-72 位")
		return
	}
	resp, err := h.svc.EmailRegister(c.Request.Context(), req.Email, req.Password)
	if err != nil {
		if errors.Is(err, ErrEmailTaken) {
			httpx.FailBusiness(c, httpx.CodeBusinessRule, "该邮箱已注册，请直接登录")
			return
		}
		httpx.FailInternal(c, "注册失败")
		return
	}
	httpx.OK(c, resp)
}

func (h *Handler) emailLogin(c *gin.Context) {
	var req EmailLoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.FailBusiness(c, httpx.CodeInvalidParam, "参数错误")
		return
	}
	resp, err := h.svc.EmailLogin(c.Request.Context(), req.Email, req.Password)
	if err != nil {
		if errors.Is(err, ErrBadCredentials) {
			httpx.Fail(c, http.StatusUnauthorized, httpx.CodeUnauthorized, "邮箱或密码不正确")
			return
		}
		if errors.Is(err, ErrUserBanned) {
			httpx.Fail(c, http.StatusForbidden, httpx.CodeForbidden, "账号不可用")
			return
		}
		httpx.FailInternal(c, "登录失败")
		return
	}
	httpx.OK(c, resp)
}

func (h *Handler) bindEmail(c *gin.Context) {
	var req EmailCredentialsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.FailBusiness(c, httpx.CodeInvalidParam, "邮箱格式不正确，密码需 8-72 位")
		return
	}
	account, err := h.svc.BindEmail(c.Request.Context(), middleware.UserIDFrom(c), req.Email, req.Password)
	if err != nil {
		if errors.Is(err, ErrEmailTaken) {
			httpx.FailBusiness(c, httpx.CodeBusinessRule, "该邮箱已被其他账号使用")
			return
		}
		if errors.Is(err, ErrEmailAlreadyBound) {
			httpx.FailBusiness(c, httpx.CodeBusinessRule, "当前账号已绑定过邮箱")
			return
		}
		httpx.FailInternal(c, "绑定失败")
		return
	}
	httpx.OK(c, account)
}

func (h *Handler) changePassword(c *gin.Context) {
	var req ChangePasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.FailBusiness(c, httpx.CodeInvalidParam, "密码需 8-72 位")
		return
	}
	if err := h.svc.ChangePassword(c.Request.Context(), middleware.UserIDFrom(c), req.OldPassword, req.NewPassword); err != nil {
		switch {
		case errors.Is(err, ErrWrongOldPassword):
			httpx.FailBusiness(c, httpx.CodeBusinessRule, "旧密码不正确")
		case errors.Is(err, ErrNoPasswordSet):
			httpx.FailBusiness(c, httpx.CodeBusinessRule, "当前账号未设置密码")
		default:
			httpx.FailInternal(c, "修改密码失败")
		}
		return
	}
	httpx.OK(c, nil)
}

func (h *Handler) resetRequest(c *gin.Context) {
	var req ResetRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.FailBusiness(c, httpx.CodeInvalidParam, "参数错误")
		return
	}
	if err := h.svc.RequestReset(c.Request.Context(), req.Email); err != nil {
		if errors.Is(err, ErrCodeRateLimited) {
			httpx.FailBusiness(c, httpx.CodeBusinessRule, "请求太频繁，一分钟后再试")
			return
		}
		httpx.FailInternal(c, "发送验证码失败")
		return
	}
	// 无论邮箱是否存在都返回成功（不泄露注册状态）
	httpx.OK(c, gin.H{"sent": true})
}

func (h *Handler) resetConfirm(c *gin.Context) {
	var req ResetConfirmRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.FailBusiness(c, httpx.CodeInvalidParam, "参数错误")
		return
	}
	if err := h.svc.ConfirmReset(c.Request.Context(), req.Email, req.Code, req.NewPassword); err != nil {
		if errors.Is(err, ErrCodeInvalid) {
			httpx.FailBusiness(c, httpx.CodeBusinessRule, "验证码无效或已过期")
			return
		}
		httpx.FailInternal(c, "重置失败")
		return
	}
	httpx.OK(c, nil)
}

func (h *Handler) deviceLogin(c *gin.Context) {
	var req DeviceLoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.FailBusiness(c, httpx.CodeInvalidParam, "参数错误")
		return
	}
	resp, err := h.svc.DeviceLogin(c.Request.Context(), req.DeviceID, c.ClientIP())
	if err != nil {
		if errors.Is(err, ErrUserBanned) {
			httpx.Fail(c, http.StatusForbidden, httpx.CodeForbidden, "账号不可用")
			return
		}
		if errors.Is(err, ErrSignupLimited) {
			httpx.FailBusiness(c, httpx.CodeBusinessRule, "创建账号太频繁，请明天再试")
			return
		}
		httpx.FailInternal(c, "登录失败")
		return
	}
	httpx.OK(c, resp)
}

func (h *Handler) me(c *gin.Context) {
	account, err := h.svc.Account(c.Request.Context(), middleware.UserIDFrom(c))
	if err != nil {
		h.failBanned(c, err, "获取用户信息失败")
		return
	}
	httpx.OK(c, account)
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
