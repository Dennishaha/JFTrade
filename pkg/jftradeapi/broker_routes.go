package jftradeapi

import (
	"net/http"
	"strings"
)

func (s *Server) serveBrokerRoutes(w http.ResponseWriter, r *http.Request) bool {
	switch {
	case strings.HasPrefix(r.URL.Path, "/api/v1/brokers/") && strings.HasSuffix(r.URL.Path, "/runtime") && r.Method == http.MethodGet:
		s.writeOK(w, s.brokerRuntime(r.Context()))
	case strings.HasPrefix(r.URL.Path, "/api/v1/brokers/") && strings.HasSuffix(r.URL.Path, "/funds") && r.Method == http.MethodGet:
		s.writeOK(w, s.emptyConnectivityList("summary", nil, "currencyBalances", "marketAssets"))
	case strings.HasPrefix(r.URL.Path, "/api/v1/brokers/") && strings.HasSuffix(r.URL.Path, "/positions") && r.Method == http.MethodGet:
		s.writeOK(w, s.emptyConnectivityList("positions", []any{}))
	case strings.HasPrefix(r.URL.Path, "/api/v1/brokers/") && strings.HasSuffix(r.URL.Path, "/orders") && r.Method == http.MethodGet:
		s.writeOK(w, s.emptyConnectivityList("orders", []any{}))
	case strings.HasPrefix(r.URL.Path, "/api/v1/brokers/") && strings.HasSuffix(r.URL.Path, "/cash-flows") && r.Method == http.MethodGet:
		s.writeOK(w, s.emptyConnectivityList("cashFlows", []any{}))
	case strings.HasPrefix(r.URL.Path, "/api/v1/brokers/") && strings.HasSuffix(r.URL.Path, "/order-fees") && r.Method == http.MethodGet:
		s.writeOK(w, s.emptyConnectivityList("fees", []any{}))
	default:
		return false
	}
	return true
}
