package jftradeapi

import (
	"net/http"
	"strings"
)

func (s *Server) serveADKRoutes(w http.ResponseWriter, r *http.Request) bool {
	if r.URL.Path == "/api/v1/assistant/chat" && r.Method == http.MethodPost {
		s.handleADKChat(w, r)
		return true
	}
	if !strings.HasPrefix(r.URL.Path, "/api/v1/adk") {
		return false
	}
	if s.adkRuntime == nil || s.adkRuntime.Store() == nil {
		s.writeError(w, http.StatusServiceUnavailable, "ADK_UNAVAILABLE", "ADK runtime is unavailable")
		return true
	}

	switch {
	case r.URL.Path == "/api/v1/adk/chat/stream" && r.Method == http.MethodPost:
		s.handleADKChatStream(w, r)
	case r.URL.Path == "/api/v1/adk" && r.Method == http.MethodGet:
		s.handleADKSnapshot(w, r)
	case r.URL.Path == "/api/v1/adk/tools" && r.Method == http.MethodGet:
		s.handleADKTools(w, r)
	case r.URL.Path == "/api/v1/adk/audit" && r.Method == http.MethodGet:
		s.handleADKAudit(w, r)
	case r.URL.Path == "/api/v1/adk/metrics" && r.Method == http.MethodGet:
		s.handleADKMetrics(w, r)
	case r.URL.Path == "/api/v1/adk/optimization-tasks" && r.Method == http.MethodGet:
		s.handleADKOptimizationTasks(w, r)
	case strings.HasPrefix(r.URL.Path, "/api/v1/adk/optimization-tasks/") && strings.HasSuffix(r.URL.Path, "/cancel") && r.Method == http.MethodPost:
		s.handleADKOptimizationTaskCancel(w, r)
	case strings.HasPrefix(r.URL.Path, "/api/v1/adk/optimization-tasks/") && r.Method == http.MethodGet:
		s.handleADKOptimizationTask(w, r)
	case r.URL.Path == "/api/v1/adk/providers" && r.Method == http.MethodGet:
		s.handleADKProviders(w, r)
	case r.URL.Path == "/api/v1/adk/providers" && r.Method == http.MethodPost:
		s.handleADKSaveProvider(w, r)
	case strings.HasPrefix(r.URL.Path, "/api/v1/adk/providers/") && strings.HasSuffix(r.URL.Path, "/test") && r.Method == http.MethodPost:
		s.handleADKTestProvider(w, r)
	case strings.HasPrefix(r.URL.Path, "/api/v1/adk/providers/") && r.Method == http.MethodPut:
		s.handleADKSaveProvider(w, r)
	case strings.HasPrefix(r.URL.Path, "/api/v1/adk/providers/") && r.Method == http.MethodDelete:
		s.handleADKDeleteProvider(w, r)
	case r.URL.Path == "/api/v1/adk/agents" && r.Method == http.MethodGet:
		s.handleADKAgents(w, r)
	case r.URL.Path == "/api/v1/adk/agents" && r.Method == http.MethodPost:
		s.handleADKSaveAgent(w, r)
	case strings.HasPrefix(r.URL.Path, "/api/v1/adk/agents/") && r.Method == http.MethodPut:
		s.handleADKSaveAgent(w, r)
	case strings.HasPrefix(r.URL.Path, "/api/v1/adk/agents/") && r.Method == http.MethodDelete:
		s.handleADKDeleteAgent(w, r)
	case r.URL.Path == "/api/v1/adk/sessions" && r.Method == http.MethodGet:
		s.handleADKSessions(w, r)
	case r.URL.Path == "/api/v1/adk/sessions" && r.Method == http.MethodPost:
		s.handleADKCreateSession(w, r)
	case strings.HasPrefix(r.URL.Path, "/api/v1/adk/sessions/") && r.Method == http.MethodGet:
		s.handleADKSession(w, r)
	case strings.HasPrefix(r.URL.Path, "/api/v1/adk/sessions/") && r.Method == http.MethodPut:
		s.handleADKRenameSession(w, r)
	case strings.HasPrefix(r.URL.Path, "/api/v1/adk/sessions/") && r.Method == http.MethodDelete:
		s.handleADKDeleteSession(w, r)
	case r.URL.Path == "/api/v1/adk/chat" && r.Method == http.MethodPost:
		s.handleADKChat(w, r)
	case r.URL.Path == "/api/v1/adk/runs" && r.Method == http.MethodGet:
		s.handleADKRuns(w, r)
	case strings.HasPrefix(r.URL.Path, "/api/v1/adk/runs/") && strings.HasSuffix(r.URL.Path, "/cancel") && r.Method == http.MethodPost:
		s.handleADKCancelRun(w, r)
	case strings.HasPrefix(r.URL.Path, "/api/v1/adk/runs/") && r.Method == http.MethodGet:
		s.handleADKRun(w, r)
	case r.URL.Path == "/api/v1/adk/approvals" && r.Method == http.MethodGet:
		s.handleADKApprovals(w, r)
	case strings.HasPrefix(r.URL.Path, "/api/v1/adk/approvals/") && strings.HasSuffix(r.URL.Path, "/approve") && r.Method == http.MethodPost:
		s.handleADKApproval(w, r, true)
	case strings.HasPrefix(r.URL.Path, "/api/v1/adk/approvals/") && strings.HasSuffix(r.URL.Path, "/deny") && r.Method == http.MethodPost:
		s.handleADKApproval(w, r, false)
	case r.URL.Path == "/api/v1/adk/skills" && r.Method == http.MethodGet:
		s.handleADKSkills(w, r)
	case r.URL.Path == "/api/v1/adk/skills" && r.Method == http.MethodPost:
		s.handleADKInstallSkill(w, r)
	case strings.HasPrefix(r.URL.Path, "/api/v1/adk/skills/") && r.Method == http.MethodPut:
		s.writeError(w, http.StatusGone, "ADK_SKILL_UPDATE_REMOVED", "skill enable/disable has been removed; bind skills directly on the agent")
		return true
	case strings.HasPrefix(r.URL.Path, "/api/v1/adk/skills/") && r.Method == http.MethodDelete:
		s.handleADKDeleteSkill(w, r)
	default:
		return false
	}
	return true
}
