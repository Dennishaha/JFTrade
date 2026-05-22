package jftradeapi

import (
	"net/http"
	"strings"
)

func (s *Server) serveStrategyRoutes(w http.ResponseWriter, r *http.Request) bool {
	switch {
	case r.URL.Path == "/api/v1/strategies" && r.Method == http.MethodGet:
		s.writeOK(w, []any{})
	case strings.HasPrefix(r.URL.Path, "/api/v1/strategies/") && strings.HasSuffix(r.URL.Path, "/logs") && r.Method == http.MethodGet:
		s.writeOK(w, map[string]any{"instanceId": pathMiddle(r.URL.Path, "/api/v1/strategies/", "/logs"), "logs": []string{}})
	case strings.HasPrefix(r.URL.Path, "/api/v1/strategies/") && strings.HasSuffix(r.URL.Path, "/audit") && r.Method == http.MethodGet:
		s.writeOK(w, map[string]any{"instanceId": pathMiddle(r.URL.Path, "/api/v1/strategies/", "/audit"), "entries": []any{}})
	default:
		return false
	}
	return true
}
