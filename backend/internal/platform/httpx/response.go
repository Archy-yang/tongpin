// Package httpx 统一 HTTP 响应格式与业务错误码。
package httpx

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// 业务错误码：0 成功；1xxx 参数错误；2xxx 认证错误；3xxx 业务规则；5xxx 服务错误。
const (
	CodeOK            = 0
	CodeInvalidParam  = 1001
	CodeUnauthorized  = 2001
	CodeForbidden     = 2003
	CodeBusinessRule  = 3001
	CodeQuotaExceeded = 3002 // 免费聊天额度用完（HTTP 429，前端引导会员升级）
	CodeInternal      = 5000
)

type Body struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
	Data any    `json:"data,omitempty"`
}

func OK(c *gin.Context, data any) {
	c.JSON(http.StatusOK, Body{Code: CodeOK, Msg: "ok", Data: data})
}

func Fail(c *gin.Context, httpStatus, code int, msg string) {
	c.JSON(httpStatus, Body{Code: code, Msg: msg})
}

func FailBusiness(c *gin.Context, code int, msg string) {
	Fail(c, http.StatusBadRequest, code, msg)
}

func FailInternal(c *gin.Context, msg string) {
	Fail(c, http.StatusInternalServerError, CodeInternal, msg)
}
