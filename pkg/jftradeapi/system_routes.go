package jftradeapi

import "net/http"

func (s *Server) serveSystemRoutes(w http.ResponseWriter, r *http.Request) bool {
	switch {
	case r.URL.Path == "/api/v1/system/futu-opend" && r.Method == http.MethodGet:
		s.writeOK(w, s.futuOpenDHealth(r.Context()))
	case r.URL.Path == "/api/v1/system/futu-opend/manual-retry" && r.Method == http.MethodPost:
		s.resetFutuRuntime()
		s.writeOK(w, map[string]any{"accepted": true})
	case r.URL.Path == "/api/v1/system/futu-opend/install-guide" && r.Method == http.MethodGet:
		s.writeOK(w, s.futuOpenDInstallGuide())
	case r.URL.Path == "/api/v1/system/status" && r.Method == http.MethodGet:
		s.writeOK(w, s.systemStatus())
	case r.URL.Path == "/api/v1/system/storage/overview" && r.Method == http.MethodGet:
		s.writeOK(w, map[string]any{"pendingOutbox": []any{}, "recentJobs": []any{}, "recentAuditLogs": []any{}, "recentExecutionCommands": []any{}})
	case r.URL.Path == "/api/v1/system/real-trade-approvals" && r.Method == http.MethodGet:
		s.writeOK(w, s.realTradeApprovals())
	case r.URL.Path == "/api/v1/system/real-trade-hard-stops" && r.Method == http.MethodGet:
		s.writeOK(w, map[string]any{"blockedOperations": []string{"PLACE", "MODIFY"}, "allowsCancel": true, "entries": []any{}})
	case r.URL.Path == "/api/v1/system/real-trade-hard-stop-events" && r.Method == http.MethodGet:
		s.writeOK(w, map[string]any{"realTradingEnabled": false, "blockedOperations": []string{"PLACE", "MODIFY"}, "allowsCancel": true, "entries": []any{}})
	case r.URL.Path == "/api/v1/system/real-trade-kill-switch" && r.Method == http.MethodGet:
		s.writeOK(w, s.realTradeKillSwitch())
	case r.URL.Path == "/api/v1/system/real-trade-kill-switch-events" && r.Method == http.MethodGet:
		s.writeOK(w, map[string]any{"realTradingEnabled": false, "killSwitchActive": false, "envConfiguredActive": false, "controlPlaneActive": false, "blockedOperations": []string{"PLACE", "MODIFY"}, "allowsCancel": true, "entries": []any{}})
	case r.URL.Path == "/api/v1/system/real-trade-risk-limits" && r.Method == http.MethodGet:
		s.writeOK(w, s.realTradeRiskState())
	case r.URL.Path == "/api/v1/system/real-trade-risk-events" && r.Method == http.MethodGet:
		s.writeOK(w, s.realTradeRiskEvents())
	case r.URL.Path == "/api/v1/system/worker/broker-order-updates" && r.Method == http.MethodGet:
		s.writeOK(w, s.brokerOrderUpdates.snapshotResponse())
	default:
		return false
	}
	return true
}
