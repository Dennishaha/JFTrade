package servercore

import (
	"context"
	"html"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

func (s *Server) applySecuritySettings(settings SecuritySettings) {
	normalized := normalizeSecuritySettings(settings)
	if s.auth != nil {
		s.auth.configure(normalized)
	}
	if s.frontend != nil {
		s.frontend.setAuthRequired(normalized.WebAccessEnabled)
	}
}

func (s *Server) webAccessMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		if s == nil || s.auth == nil || !s.auth.enforceAccess || isDesktopRequest(c.Request) {
			c.Next()
			return
		}
		if s.desktopMode && !isWebAccessSurfaceRequest(c.Request) {
			s.writeError(c, http.StatusForbidden, "DESKTOP_API_CREDENTIALS_REQUIRED", "this local API listener is reserved for the JFTrade desktop app")
			return
		}
		if s.auth.browserAccessAllowed(c.Request) {
			accessContext := s.auth.currentAccessContext()
			requestContext, cancelRequest := context.WithCancel(c.Request.Context())
			defer cancelRequest()
			go func() {
				select {
				case <-accessContext.Done():
					cancelRequest()
				case <-requestContext.Done():
				}
			}()
			c.Request = c.Request.WithContext(requestContext)
			c.Next()
			return
		}
		if !s.auth.webAccessEnabled() {
			if acceptsWebAccessStatusPage(c.Request) {
				writeWebAccessStatusPage(c, "Web 访问尚未开启", "请打开 JFTrade 桌面应用，在“设置 → Web 访问”中设置密码并主动开启。")
				return
			}
			s.writeError(c, http.StatusForbidden, "WEB_ACCESS_DISABLED", "Web access is disabled; enable it in the JFTrade desktop settings")
			return
		}
		if acceptsWebAccessStatusPage(c.Request) {
			writeWebAccessStatusPage(c, "当前仅允许本机访问", "请在运行 JFTrade 的这台电脑上打开浏览器，或从桌面设置中明确允许其他设备访问。")
			return
		}
		s.writeError(c, http.StatusForbidden, "REMOTE_WEB_ACCESS_DISABLED", "Web access is limited to this computer")
	}
}

func acceptsWebAccessStatusPage(r *http.Request) bool {
	if r == nil || (r.Method != http.MethodGet && r.Method != http.MethodHead) {
		return false
	}
	if strings.HasPrefix(r.URL.Path, "/api/") || strings.HasPrefix(r.URL.Path, "/swagger") {
		return false
	}
	return strings.Contains(strings.ToLower(r.Header.Get("Accept")), "text/html")
}

func writeWebAccessStatusPage(c *gin.Context, title string, message string) {
	c.Header("Cache-Control", "no-store")
	c.Header("Content-Type", "text/html; charset=utf-8")
	c.Header("X-Content-Type-Options", "nosniff")
	c.Status(http.StatusForbidden)
	if c.Request.Method != http.MethodHead {
		_, _ = c.Writer.WriteString(`<!doctype html><html lang="zh-CN"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>` + html.EscapeString(title) + ` · JFTrade</title><style>body{margin:0;background:#f8fafc;color:#0f172a;font-family:system-ui,-apple-system,sans-serif}.card{max-width:34rem;margin:12vh auto;padding:2rem;border:1px solid #e2e8f0;border-radius:1rem;background:white;box-shadow:0 12px 32px #0f172a12}.brand{color:#0f766e;font-weight:700;letter-spacing:.16em}h1{font-size:1.4rem;margin:1rem 0 .7rem}p{margin:0;color:#475569;line-height:1.7}</style></head><body><main class="card"><div class="brand">JFTRADE</div><h1>` + html.EscapeString(title) + `</h1><p>` + html.EscapeString(message) + `</p></main></body></html>`)
	}
	c.Abort()
}
