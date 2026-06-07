package jftradeapi

import (
	"encoding/json"
	"net/http"
	"strings"

	jfadk "github.com/jftrade/jftrade-main/pkg/adk"
)

func (s *Server) handleADKSessions(w http.ResponseWriter, r *http.Request) {
	limit, offset := adkPageBounds(r)
	items, total, err := s.adkRuntime.Store().ListSessionsPage(r.Context(), r.URL.Query().Get("agentId"), r.URL.Query().Get("query"), limit, offset)
	writeADKPageOrError(s, w, "ADK_SESSION_LIST_FAILED", "sessions", items, total, limit, offset, err)
}

func (s *Server) handleADKCreateSession(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		AgentID string `json:"agentId"`
		Title   string `json:"title"`
	}
	_ = json.NewDecoder(r.Body).Decode(&payload)
	agent, ok, agentErr := s.adkRuntime.Store().Agent(r.Context(), payload.AgentID)
	if agentErr != nil || !ok || agent.Status != jfadk.AgentStatusEnabled || agent.DeletedAt != nil {
		s.writeError(w, http.StatusBadRequest, "BAD_REQUEST", "enabled agent is required")
		return
	}
	session, err := s.adkRuntime.Store().CreateSession(r.Context(), payload.AgentID, payload.Title)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "ADK_SESSION_CREATE_FAILED", err.Error())
		return
	}
	s.writeOK(w, session)
}

func (s *Server) handleADKSession(w http.ResponseWriter, r *http.Request) {
	id, err := decodePathSegment(strings.TrimPrefix(r.URL.Path, "/api/v1/adk/sessions/"))
	if err != nil || strings.TrimSpace(id) == "" {
		s.writeError(w, http.StatusBadRequest, "BAD_REQUEST", "sessionId is invalid")
		return
	}
	session, ok, err := s.adkRuntime.Store().Session(r.Context(), id)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "ADK_SESSION_GET_FAILED", err.Error())
		return
	}
	if !ok {
		s.writeError(w, http.StatusNotFound, "NOT_FOUND", "session not found")
		return
	}
	messages, err := s.adkRuntime.Store().Messages(r.Context(), id)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "ADK_MESSAGES_GET_FAILED", err.Error())
		return
	}
	s.writeOK(w, jfadk.SessionsResponse{Session: session, Messages: messages})
}

func (s *Server) handleADKRenameSession(w http.ResponseWriter, r *http.Request) {
	id, err := decodePathSegment(strings.TrimPrefix(r.URL.Path, "/api/v1/adk/sessions/"))
	if err != nil || strings.TrimSpace(id) == "" {
		s.writeError(w, http.StatusBadRequest, "BAD_REQUEST", "sessionId is invalid")
		return
	}
	var payload struct {
		Title string `json:"title"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		s.writeError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid session payload")
		return
	}
	session, err := s.adkRuntime.Store().RenameSession(r.Context(), id, payload.Title)
	if err != nil {
		s.writeError(w, http.StatusBadRequest, "ADK_SESSION_RENAME_FAILED", err.Error())
		return
	}
	s.writeOK(w, session)
}

func (s *Server) handleADKDeleteSession(w http.ResponseWriter, r *http.Request) {
	id, err := decodePathSegment(strings.TrimPrefix(r.URL.Path, "/api/v1/adk/sessions/"))
	if err != nil || strings.TrimSpace(id) == "" {
		s.writeError(w, http.StatusBadRequest, "BAD_REQUEST", "sessionId is invalid")
		return
	}
	if err := s.adkRuntime.DeleteSession(r.Context(), id); err != nil {
		s.writeError(w, http.StatusInternalServerError, "ADK_SESSION_DELETE_FAILED", err.Error())
		return
	}
	s.writeOK(w, map[string]any{"deleted": true, "id": id})
}

func (s *Server) handleADKRuns(w http.ResponseWriter, r *http.Request) {
	s.adkRuntime.ReconcileExpiredRuns(r.Context())
	limit, offset := adkPageBounds(r)
	items, total, err := s.adkRuntime.Store().ListRunsPage(r.Context(), r.URL.Query().Get("status"), r.URL.Query().Get("agentId"), r.URL.Query().Get("sessionId"), limit, offset)
	writeADKPageOrError(s, w, "ADK_RUN_LIST_FAILED", "runs", items, total, limit, offset, err)
}

func (s *Server) handleADKCancelRun(w http.ResponseWriter, r *http.Request) {
	id, err := decodePathSegment(pathMiddle(r.URL.Path, "/api/v1/adk/runs/", "/cancel"))
	if err != nil || strings.TrimSpace(id) == "" {
		s.writeError(w, http.StatusBadRequest, "BAD_REQUEST", "runId is invalid")
		return
	}
	run, err := s.adkRuntime.CancelRun(r.Context(), id)
	if err != nil {
		s.writeError(w, http.StatusNotFound, "ADK_RUN_CANCEL_FAILED", err.Error())
		return
	}
	s.writeOK(w, run)
}

func (s *Server) handleADKRun(w http.ResponseWriter, r *http.Request) {
	s.adkRuntime.ReconcileExpiredRuns(r.Context())
	id, err := decodePathSegment(strings.TrimPrefix(r.URL.Path, "/api/v1/adk/runs/"))
	if err != nil || strings.TrimSpace(id) == "" {
		s.writeError(w, http.StatusBadRequest, "BAD_REQUEST", "runId is invalid")
		return
	}
	run, ok, err := s.adkRuntime.Store().Run(r.Context(), id)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "ADK_RUN_GET_FAILED", err.Error())
		return
	}
	if !ok {
		s.writeError(w, http.StatusNotFound, "NOT_FOUND", "run not found")
		return
	}
	s.writeOK(w, run)
}

func (s *Server) handleADKApprovals(w http.ResponseWriter, r *http.Request) {
	limit, offset := adkPageBounds(r)
	items, total, err := s.adkRuntime.Store().ListApprovalsPage(r.Context(), r.URL.Query().Get("status"), r.URL.Query().Get("agentId"), limit, offset)
	writeADKPageOrError(s, w, "ADK_APPROVAL_LIST_FAILED", "approvals", items, total, limit, offset, err)
}

func (s *Server) handleADKApproval(w http.ResponseWriter, r *http.Request, approved bool) {
	suffix := "/deny"
	if approved {
		suffix = "/approve"
	}
	id, err := decodePathSegment(pathMiddle(r.URL.Path, "/api/v1/adk/approvals/", suffix))
	if err != nil || strings.TrimSpace(id) == "" {
		s.writeError(w, http.StatusBadRequest, "BAD_REQUEST", "approvalId is invalid")
		return
	}
	resolution, err := s.adkRuntime.ResolveApproval(r.Context(), id, approved)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "ADK_APPROVAL_RESOLVE_FAILED", err.Error())
		return
	}
	s.writeOK(w, resolution)
}
