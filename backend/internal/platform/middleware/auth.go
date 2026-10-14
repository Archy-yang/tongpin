// Package middleware 通用 HTTP 中间件。
package middleware

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/Archy-yang/tongpin/backend/internal/platform/httpx"
	"github.com/Archy-yang/tongpin/backend/internal/platform/jwt"
)

const CtxUserID = "user_id"

// Auth 校验 Bearer Token，将 user_id 注入请求上下文。
func Auth(issuer *jwt.Issuer) gin.HandlerFunc {
	return func(c *gin.Context) {
		header := c.GetHeader("Authorization")
		token, ok := strings.CutPrefix(header, "Bearer ")
		if !ok || token == "" {
			httpx.Fail(c, http.StatusUnauthorized, httpx.CodeUnauthorized, "未登录")
			c.Abort()
			return
		}
		userID, err := issuer.Parse(token)
		if err != nil {
			httpx.Fail(c, http.StatusUnauthorized, httpx.CodeUnauthorized, "登录已失效")
			c.Abort()
			return
		}
		c.Set(CtxUserID, userID)
		c.Next()
	}
}

// UserIDFrom 从上下文取出当前用户 ID（仅在 Auth 中间件之后可用）。
func UserIDFrom(c *gin.Context) int64 {
	id, _ := c.Get(CtxUserID)
	uid, _ := id.(int64)
	return uid
}
