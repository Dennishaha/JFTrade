package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// OriginChecker 检查请求来源是否在允许列表中。
type OriginChecker interface {
	IsOriginAllowed(origin string) bool
}

// CORS 返回一个 Gin 中间件，处理跨域请求。
// checker 决定哪些来源的请求被允许；nil checker 表示拒绝所有跨域请求。
func CORS(checker OriginChecker) gin.HandlerFunc {
	return func(c *gin.Context) {
		origin := requestOrigin(c.Request)
		if origin != "" && checker != nil && checker.IsOriginAllowed(origin) {
			c.Header("Access-Control-Allow-Origin", origin)
			c.Header("Vary", "Origin")
			c.Header("Access-Control-Allow-Credentials", "true")
		}
		c.Header("Access-Control-Allow-Headers", "Origin, Content-Type, Authorization, X-CSRF-Token, X-Request-ID")
		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		c.Header("Access-Control-Expose-Headers", "X-Request-ID")

		if c.Request.Method == http.MethodOptions {
			if origin != "" && (checker == nil || !checker.IsOriginAllowed(origin)) {
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
	// 直接使用 Origin 头
	if origin := r.Header.Get("Origin"); origin != "" {
		return canonicalOrigin(origin)
	}
	// 回退到 Referer
	if referer := r.Header.Get("Referer"); referer != "" {
		return canonicalOrigin(referer)
	}
	return ""
}

// canonicalOrigin 将原始来源字符串规范化为 scheme://host 格式。
func canonicalOrigin(value string) string {
	value = trimSpace(value)
	if value == "" {
		return ""
	}
	// 简单解析：查找 "://"
	colonIdx := indexOf(value, ':')
	if colonIdx < 0 {
		return ""
	}
	schemeEnd := indexOf(value[colonIdx:], '/')
	if schemeEnd < 0 {
		return ""
	}
	// value[colonIdx:] 应该是 "://host..."
	if len(value) <= colonIdx+3 {
		return ""
	}
	if value[colonIdx+1] != '/' || value[colonIdx+2] != '/' {
		return ""
	}
	hostStart := colonIdx + 3
	hostEnd := indexOf(value[hostStart:], '/')
	host := value[hostStart:]
	if hostEnd >= 0 {
		host = value[hostStart : hostStart+hostEnd]
	}
	if host == "" {
		return ""
	}
	// 小写化
	host = toLower(host)
	scheme := toLower(value[:colonIdx])
	if scheme != "http" && scheme != "https" {
		return ""
	}
	return scheme + "://" + host
}

// 避免导入 strings 包的简单辅助函数
func trimSpace(s string) string {
	start, end := 0, len(s)
	for start < end && (s[start] == ' ' || s[start] == '\t') {
		start++
	}
	for end > start && (s[end-1] == ' ' || s[end-1] == '\t') {
		end--
	}
	return s[start:end]
}

func indexOf(s string, c byte) int {
	for i := 0; i < len(s); i++ {
		if s[i] == c {
			return i
		}
	}
	return -1
}

func toLower(s string) string {
	b := make([]byte, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'A' && c <= 'Z' {
			c += 32
		}
		b[i] = c
	}
	return string(b)
}
