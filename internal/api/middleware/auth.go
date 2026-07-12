package middleware

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// Authenticator 验证请求是否已认证。
// 返回 session token、是否认证成功、是否通过可信宿主能力认证。
type Authenticator interface {
	Authenticate(r *http.Request) (session string, ok bool, trustedHost bool)
}

// CSRFValidator 验证 CSRF token 是否有效。
// session 是 Authenticator 返回的会话标识。
type CSRFValidator interface {
	ValidateCSRF(r *http.Request, session string) bool
}

// WriteMethodDetector 判断 HTTP 方法是否为写操作。
type WriteMethodDetector interface {
	IsWriteMethod(r *http.Request) bool
}

type validatedOriginContextKey struct{}
type trustedHostContextKey struct{}

// MarkRequestTrustedHost marks a request authenticated by an embedded host
// capability, such as the per-process Wails desktop token.
func MarkRequestTrustedHost(r *http.Request) *http.Request {
	if r == nil {
		return nil
	}
	return r.WithContext(context.WithValue(r.Context(), trustedHostContextKey{}, true))
}

// IsRequestTrustedHost reports whether the embedded host authenticated r.
func IsRequestTrustedHost(r *http.Request) bool {
	return r != nil && r.Context().Value(trustedHostContextKey{}) == true
}

// Auth 返回 Gin 鉴权中间件。
// 仅跳过建立或探测 Web 会话所需的 login/session 路径。
func Auth(auth Authenticator, csrf CSRFValidator, writeDetector WriteMethodDetector, originChecker OriginChecker) gin.HandlerFunc {
	return func(c *gin.Context) {
		r := c.Request
		if !requiresAuthentication(r) {
			c.Next()
			return
		}
		if !authorizeRequest(c, auth, csrf, writeDetector, originChecker) {
			return
		}
		c.Next()
	}
}

// requiresAuthentication 判断路径是否需要鉴权。
func requiresAuthentication(r *http.Request) bool {
	if r == nil {
		return false
	}
	path := r.URL.Path
	if strings.HasPrefix(path, "/swagger") {
		return true
	}
	if !strings.HasPrefix(path, "/api/") {
		return false
	}
	if path == "/api/v1/auth/login" || path == "/api/v1/auth/session" {
		return false
	}
	return true
}

// authorizeRequest 执行请求鉴权。返回 true 表示通过。
func authorizeRequest(c *gin.Context, auth Authenticator, csrf CSRFValidator, writeDetector WriteMethodDetector, originChecker OriginChecker) bool {
	r := c.Request
	origin := requestOrigin(r)
	if requestOriginProvided(r) {
		if origin == "" || originChecker == nil || !originChecker.IsOriginAllowed(r, origin) {
			writeAuthError(c, http.StatusForbidden, "ORIGIN_FORBIDDEN", "request origin is not allowed")
			return false
		}
		markRequestOriginValidated(r)
	}

	if auth == nil {
		writeAuthError(c, http.StatusUnauthorized, "WEB_AUTH_REQUIRED", "Web password authentication is required")
		return false
	}
	session, ok, trustedHost := auth.Authenticate(r)
	if !ok {
		writeAuthError(c, http.StatusUnauthorized, "WEB_AUTH_REQUIRED", "Web password authentication is required")
		return false
	}
	if trustedHost {
		return true
	}
	if !isWriteMethod(writeDetector, r) {
		return true
	}
	if origin == "" {
		writeAuthError(c, http.StatusForbidden, "ORIGIN_FORBIDDEN", "write request origin is not allowed")
		return false
	}
	if csrf == nil || !csrf.ValidateCSRF(r, session) {
		writeAuthError(c, http.StatusForbidden, "CSRF_FAILED", "valid CSRF token is required")
		return false
	}
	return true
}

func isWriteMethod(detector WriteMethodDetector, r *http.Request) bool {
	if detector != nil {
		return detector.IsWriteMethod(r)
	}
	if r == nil {
		return false
	}
	return r.Method == http.MethodPost || r.Method == http.MethodPut || r.Method == http.MethodPatch || r.Method == http.MethodDelete
}

func markRequestOriginValidated(r *http.Request) {
	if r == nil {
		return
	}
	*r = *r.WithContext(context.WithValue(r.Context(), validatedOriginContextKey{}, true))
}

// IsRequestOriginValidated reports whether Auth accepted the browser origin on this request.
func IsRequestOriginValidated(r *http.Request) bool {
	return r != nil && r.Context().Value(validatedOriginContextKey{}) == true
}

// writeAuthError 写入鉴权错误响应。使用本地 envelope 结构避免引入 httpserver 依赖。
// 注意：middleware 包不应依赖 httpserver 包。
func writeAuthError(c *gin.Context, status int, code string, message string) {
	c.AbortWithStatusJSON(status, gin.H{
		"ok":        false,
		"error":     gin.H{"code": code, "message": message},
		"timestamp": time.Now().UTC().Format(time.RFC3339Nano),
	})
}
