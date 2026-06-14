package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// ADKAvailable 返回一个 Gin 中间件，当 ADK 运行时不可用时中止请求。
// check 函数应返回 ADK 运行时是否已初始化并可用。
func ADKAvailable(check func() bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !check() {
			c.AbortWithStatusJSON(http.StatusServiceUnavailable, gin.H{
				"ok":    false,
				"error": gin.H{"code": "ADK_UNAVAILABLE", "message": "ADK runtime is unavailable"},
			})
			return
		}
		c.Next()
	}
}
