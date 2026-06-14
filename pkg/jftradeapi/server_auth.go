package jftradeapi

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

func (s *Server) applySecuritySettings(settings SecuritySettings) {
	required := normalizeSecuritySettings(settings).AdminAuthRequired
	if s.auth != nil {
		s.auth.enabled = required
	}
	if s.frontend != nil {
		s.frontend.setAuthRequired(required)
	}
}

func (s *Server) requiresAuthentication(r *http.Request) bool {
	if !strings.HasPrefix(r.URL.Path, "/api/") {
		return false
	}
	if strings.HasPrefix(r.URL.Path, "/api/v1/auth/") {
		return false
	}
	return r.URL.Path != "/api/v1/system/status"
}

func (s *Server) authorizeRequest(c *gin.Context) bool {
	r := c.Request
	if s.auth == nil || !s.auth.enabled {
		return true
	}
	session, ok, bearer := s.auth.authenticate(r)
	if !ok {
		s.writeError(c, http.StatusUnauthorized, "UNAUTHORIZED", "administrator authentication is required")
		return false
	}
	if bearer {
		return true
	}
	origin := requestOrigin(r)
	if origin != "" && !s.auth.originAllowed(origin) {
		s.writeError(c, http.StatusForbidden, "ORIGIN_FORBIDDEN", "request origin is not allowed")
		return false
	}
	if !s.isWriteMethod(r) {
		return true
	}
	if origin == "" {
		s.writeError(c, http.StatusForbidden, "ORIGIN_FORBIDDEN", "write request origin is not allowed")
		return false
	}
	if !constantTimeEqual(strings.TrimSpace(r.Header.Get("X-CSRF-Token")), session.CSRF) {
		s.writeError(c, http.StatusForbidden, "CSRF_FAILED", "valid CSRF token is required")
		return false
	}
	return true
}
