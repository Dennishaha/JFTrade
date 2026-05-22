package jftradeapi

import (
	"net/http"
	"strings"
)

func (s *Server) serveExecutionRoutes(w http.ResponseWriter, r *http.Request) bool {
	switch {
	case r.URL.Path == "/api/v1/execution/orders" && r.Method == http.MethodGet:
		s.writeOK(w, map[string]any{"orders": []any{}})
	case strings.HasPrefix(r.URL.Path, "/api/v1/execution/orders/") && strings.HasSuffix(r.URL.Path, "/events") && r.Method == http.MethodGet:
		s.writeOK(w, map[string]any{"internalOrderId": "", "events": []any{}})
	default:
		return false
	}
	return true
}
