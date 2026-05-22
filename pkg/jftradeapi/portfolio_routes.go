package jftradeapi

import (
	"net/http"
	"strings"
)

func (s *Server) servePortfolioRoutes(w http.ResponseWriter, r *http.Request) bool {
	switch {
	case strings.HasPrefix(r.URL.Path, "/api/v1/portfolio/") && strings.Contains(r.URL.Path, "/cash-balances") && r.Method == http.MethodGet:
		s.writeOK(w, map[string]any{"balances": []any{}})
	case strings.HasPrefix(r.URL.Path, "/api/v1/portfolio/") && strings.Contains(r.URL.Path, "/positions") && r.Method == http.MethodGet:
		s.writeOK(w, map[string]any{"positions": []any{}})
	case strings.HasPrefix(r.URL.Path, "/api/v1/portfolio/") && strings.Contains(r.URL.Path, "/cash-reconciliation") && r.Method == http.MethodGet:
		s.writeOK(w, s.emptyConnectivityList("balances", []any{}))
	case strings.HasPrefix(r.URL.Path, "/api/v1/portfolio/") && strings.Contains(r.URL.Path, "/reconciliation") && r.Method == http.MethodGet:
		s.writeOK(w, s.emptyConnectivityList("positions", []any{}))
	default:
		return false
	}
	return true
}
