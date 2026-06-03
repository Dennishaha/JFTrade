package jftradeapi

import (
	"net/http"
	"strings"
)

func (s *Server) serveSettingsRoutes(w http.ResponseWriter, r *http.Request) bool {
	switch {
	case r.URL.Path == "/api/v1/settings/ui" && r.Method == http.MethodGet:
		s.writeOK(w, map[string]any{"appearance": s.store.appearance()})
	case r.URL.Path == "/api/v1/settings/ui" && r.Method == http.MethodPut:
		s.handleSaveUIAppearance(w, r)
	case r.URL.Path == "/api/v1/settings/onboarding" && r.Method == http.MethodGet:
		s.writeOK(w, s.onboardingState(r.Context()))
	case r.URL.Path == "/api/v1/settings/onboarding" && r.Method == http.MethodPut:
		s.handleSaveOnboarding(w, r)
	case r.URL.Path == "/api/v1/settings/brokers" && r.Method == http.MethodGet:
		s.writeOK(w, s.brokerSettings())
	case strings.HasPrefix(r.URL.Path, "/api/v1/settings/brokers/") && strings.HasSuffix(r.URL.Path, "/integration") && r.Method == http.MethodPut:
		s.handleSaveBrokerIntegration(w, r)
	case r.URL.Path == "/api/v1/settings/broker-accounts" && r.Method == http.MethodPost:
		s.handleCreateManagedBrokerAccount(w, r)
	case strings.HasPrefix(r.URL.Path, "/api/v1/settings/broker-accounts/") && r.Method == http.MethodPut:
		s.handleUpdateManagedBrokerAccount(w, r)
	case strings.HasPrefix(r.URL.Path, "/api/v1/settings/broker-accounts/") && r.Method == http.MethodDelete:
		s.handleDeleteManagedBrokerAccount(w, r)
	default:
		return false
	}
	return true
}
