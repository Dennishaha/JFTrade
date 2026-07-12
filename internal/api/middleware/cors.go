package middleware

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/jftrade/jftrade-main/internal/api/origin"
)

// OriginChecker 检查请求来源是否在允许列表中。
type OriginChecker interface {
	IsOriginAllowed(r *http.Request, origin string) bool
}

// CORS 返回一个 Gin 中间件，处理跨域请求。
// checker 决定哪些来源的请求被允许；nil checker 表示拒绝所有跨域请求。
func CORS(checker OriginChecker) gin.HandlerFunc {
	return func(c *gin.Context) {
		origin := requestOrigin(c.Request)
		originProvided := requestOriginProvided(c.Request)
		if origin != "" && checker != nil && checker.IsOriginAllowed(c.Request, origin) {
			c.Header("Access-Control-Allow-Origin", origin)
			c.Header("Vary", "Origin")
			c.Header("Access-Control-Allow-Credentials", "true")
		}
		c.Header("Access-Control-Allow-Headers", "Origin, Content-Type, Authorization, X-CSRF-Token, X-Request-ID")
		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
		c.Header("Access-Control-Expose-Headers", "X-Request-ID")

		if c.Request.Method == http.MethodOptions {
			if originProvided && (origin == "" || checker == nil || !checker.IsOriginAllowed(c.Request, origin)) {
				c.AbortWithStatus(http.StatusForbidden)
				return
			}
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}

// requestOrigin 从请求中提取规范化的来源。
func requestOrigin(r *http.Request) string {
	if r == nil {
		return ""
	}
	if value := strings.TrimSpace(r.Header.Get("Origin")); value != "" {
		return canonicalOrigin(value)
	}
	return canonicalOrigin(r.Header.Get("Referer"))
}

func requestOriginProvided(r *http.Request) bool {
	if r == nil {
		return false
	}
	return strings.TrimSpace(r.Header.Get("Origin")) != "" || strings.TrimSpace(r.Header.Get("Referer")) != ""
}

// canonicalOrigin 将原始来源字符串规范化为 scheme://host 格式。
func canonicalOrigin(value string) string {
	return origin.Canonical(value)
}
