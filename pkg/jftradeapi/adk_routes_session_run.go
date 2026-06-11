package jftradeapi

import (
	"context"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	jfadk "github.com/jftrade/jftrade-main/pkg/adk"
)

// handleADKSessions godoc
// @Summary 读取 ADK Session 列表
// @Tags adk
// @Produce json
// @Param limit query int false "分页大小"
// @Param offset query int false "分页偏移"
// @Param agentId query string false "Agent ID"
// @Param query query string false "搜索关键字"
// @Success 200 {object} envelope
// @Failure 400 {object} envelope
// @Failure 500 {object} envelope
// @Router /api/v1/adk/sessions [get]
func (s *Server) handleADKSessions(c *gin.Context) {
	var query adkSessionsQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		s.writeError(c, http.StatusBadRequest, "BAD_REQUEST", "invalid sessions query")
		return
	}
	limit, offset := adkPageBounds(adkPageQuery{Limit: query.Limit, Offset: query.Offset})
	items, total, err := s.adkRuntime.Store().ListSessionsPage(c.Request.Context(), query.AgentID, query.Query, limit, offset)
	writeADKPageOrError(s, c, "ADK_SESSION_LIST_FAILED", "sessions", items, total, limit, offset, err)
}

func (s *Server) handleADKCreateSession(c *gin.Context) {
	var payload struct {
		AgentID string `json:"agentId"`
		Title   string `json:"title"`
	}
	_ = c.ShouldBindJSON(&payload)
	agent, ok, agentErr := s.adkRuntime.Store().Agent(c.Request.Context(), payload.AgentID)
	if agentErr != nil || !ok || agent.Status != jfadk.AgentStatusEnabled || agent.DeletedAt != nil {
		s.writeError(c, http.StatusBadRequest, "BAD_REQUEST", "enabled agent is required")
		return
	}
	session, err := s.adkRuntime.Store().CreateSession(c.Request.Context(), payload.AgentID, payload.Title)
	if err != nil {
		s.writeError(c, http.StatusInternalServerError, "ADK_SESSION_CREATE_FAILED", err.Error())
		return
	}
	s.writeOK(c, session)
}

func (s *Server) handleADKSession(c *gin.Context) {
	var uri sessionURI
	if err := bindURI(c, &uri); err != nil || strings.TrimSpace(uri.SessionID) == "" {
		s.writeError(c, http.StatusBadRequest, "BAD_REQUEST", "sessionId is invalid")
		return
	}
	id := uri.SessionID
	session, ok, err := s.adkRuntime.Store().Session(c.Request.Context(), id)
	if err != nil {
		s.writeError(c, http.StatusInternalServerError, "ADK_SESSION_GET_FAILED", err.Error())
		return
	}
	if !ok {
		s.writeError(c, http.StatusNotFound, "NOT_FOUND", "session not found")
		return
	}
	timeline, _, err := s.adkRuntime.Store().SessionTimeline(c.Request.Context(), id)
	if err != nil {
		s.writeError(c, http.StatusInternalServerError, "ADK_MESSAGES_GET_FAILED", err.Error())
		return
	}
	if timeline == nil {
		timeline = []jfadk.TimelineEntry{}
	}
	s.writeOK(c, jfadk.NormalizeSessionsResponse(jfadk.SessionsResponse{
		Session:  session,
		Timeline: timeline,
	}))
}

func (s *Server) handleADKSessionContext(c *gin.Context) {
	var uri sessionURI
	if err := bindURI(c, &uri); err != nil || strings.TrimSpace(uri.SessionID) == "" {
		s.writeError(c, http.StatusBadRequest, "BAD_REQUEST", "sessionId is invalid")
		return
	}
	snapshot, err := s.adkRuntime.SessionContext(c.Request.Context(), uri.SessionID)
	if err != nil {
		status := http.StatusInternalServerError
		if strings.Contains(strings.ToLower(err.Error()), "not found") {
			status = http.StatusNotFound
		}
		s.writeError(c, status, "ADK_SESSION_CONTEXT_FAILED", err.Error())
		return
	}
	s.writeOK(c, snapshot)
}

func (s *Server) handleADKCompactSessionContext(c *gin.Context) {
	var uri sessionURI
	if err := bindURI(c, &uri); err != nil || strings.TrimSpace(uri.SessionID) == "" {
		s.writeError(c, http.StatusBadRequest, "BAD_REQUEST", "sessionId is invalid")
		return
	}
	var payload struct {
		Mode   string `json:"mode"`
		Reason string `json:"reason"`
	}
	_ = c.ShouldBindJSON(&payload)
	snapshot, err := s.adkRuntime.CompactSessionContext(c.Request.Context(), uri.SessionID, payload.Mode, "manual", defaultStringLocal(payload.Reason, "manual context compaction requested"))
	if err != nil {
		status := http.StatusInternalServerError
		lower := strings.ToLower(err.Error())
		switch {
		case strings.Contains(lower, "active or pending run"):
			status = http.StatusConflict
		case strings.Contains(lower, "not found"):
			status = http.StatusNotFound
		}
		s.writeError(c, status, "ADK_SESSION_CONTEXT_COMPACT_FAILED", err.Error())
		return
	}
	s.writeOK(c, snapshot)
}

func (s *Server) handleADKRenameSession(c *gin.Context) {
	var uri sessionURI
	if err := bindURI(c, &uri); err != nil || strings.TrimSpace(uri.SessionID) == "" {
		s.writeError(c, http.StatusBadRequest, "BAD_REQUEST", "sessionId is invalid")
		return
	}
	id := uri.SessionID
	var payload struct {
		Title string `json:"title"`
	}
	if err := c.ShouldBindJSON(&payload); err != nil {
		s.writeError(c, http.StatusBadRequest, "BAD_REQUEST", "invalid session payload")
		return
	}
	session, err := s.adkRuntime.Store().RenameSession(c.Request.Context(), id, payload.Title)
	if err != nil {
		s.writeError(c, http.StatusBadRequest, "ADK_SESSION_RENAME_FAILED", err.Error())
		return
	}
	s.writeOK(c, session)
}

func (s *Server) handleADKDeleteSession(c *gin.Context) {
	var uri sessionURI
	if err := bindURI(c, &uri); err != nil || strings.TrimSpace(uri.SessionID) == "" {
		s.writeError(c, http.StatusBadRequest, "BAD_REQUEST", "sessionId is invalid")
		return
	}
	id := uri.SessionID
	if err := s.adkRuntime.DeleteSession(c.Request.Context(), id); err != nil {
		s.writeError(c, http.StatusInternalServerError, "ADK_SESSION_DELETE_FAILED", err.Error())
		return
	}
	s.writeOK(c, map[string]any{"deleted": true, "id": id})
}

// handleADKRuns godoc
// @Summary 读取 ADK Run 列表
// @Tags adk
// @Produce json
// @Param limit query int false "分页大小"
// @Param offset query int false "分页偏移"
// @Param status query string false "Run 状态"
// @Param agentId query string false "Agent ID"
// @Param sessionId query string false "Session ID"
// @Success 200 {object} envelope
// @Failure 400 {object} envelope
// @Failure 500 {object} envelope
// @Router /api/v1/adk/runs [get]
func (s *Server) handleADKRuns(c *gin.Context) {
	s.adkRuntime.ReconcileExpiredRuns(c.Request.Context())
	s.adkRuntime.ReconcileResolvedApprovals(context.WithoutCancel(c.Request.Context()))
	var query adkRunsQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		s.writeError(c, http.StatusBadRequest, "BAD_REQUEST", "invalid runs query")
		return
	}
	limit, offset := adkPageBounds(adkPageQuery{Limit: query.Limit, Offset: query.Offset})
	items, total, err := s.adkRuntime.Store().ListRunsPage(c.Request.Context(), query.Status, query.AgentID, query.SessionID, limit, offset)
	for index := range items {
		items[index] = jfadk.NormalizeRun(items[index])
	}
	writeADKPageOrError(s, c, "ADK_RUN_LIST_FAILED", "runs", items, total, limit, offset, err)
}

func (s *Server) handleADKCancelRun(c *gin.Context) {
	var uri runURI
	if err := bindURI(c, &uri); err != nil || strings.TrimSpace(uri.RunID) == "" {
		s.writeError(c, http.StatusBadRequest, "BAD_REQUEST", "runId is invalid")
		return
	}
	id := uri.RunID
	run, err := s.adkRuntime.CancelRun(c.Request.Context(), id)
	if err != nil {
		s.writeError(c, http.StatusNotFound, "ADK_RUN_CANCEL_FAILED", err.Error())
		return
	}
	s.writeOK(c, jfadk.NormalizeRun(run))
}

func (s *Server) handleADKRun(c *gin.Context) {
	s.adkRuntime.ReconcileExpiredRuns(c.Request.Context())
	s.adkRuntime.ReconcileResolvedApprovals(context.WithoutCancel(c.Request.Context()))
	var uri runURI
	if err := bindURI(c, &uri); err != nil || strings.TrimSpace(uri.RunID) == "" {
		s.writeError(c, http.StatusBadRequest, "BAD_REQUEST", "runId is invalid")
		return
	}
	id := uri.RunID
	run, ok, err := s.adkRuntime.Store().Run(c.Request.Context(), id)
	if err != nil {
		s.writeError(c, http.StatusInternalServerError, "ADK_RUN_GET_FAILED", err.Error())
		return
	}
	if !ok {
		s.writeError(c, http.StatusNotFound, "NOT_FOUND", "run not found")
		return
	}
	s.writeOK(c, jfadk.NormalizeRun(run))
}

func (s *Server) handleADKApprovals(c *gin.Context) {
	s.adkRuntime.ReconcileResolvedApprovals(context.WithoutCancel(c.Request.Context()))
	var query adkApprovalsQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		s.writeError(c, http.StatusBadRequest, "BAD_REQUEST", "invalid approvals query")
		return
	}
	limit, offset := adkPageBounds(adkPageQuery{Limit: query.Limit, Offset: query.Offset})
	items, total, err := s.adkRuntime.Store().ListApprovalsPage(c.Request.Context(), query.Status, query.AgentID, limit, offset)
	writeADKPageOrError(s, c, "ADK_APPROVAL_LIST_FAILED", "approvals", items, total, limit, offset, err)
}

func (s *Server) handleADKApproval(c *gin.Context, approved bool) {
	var uri approvalURI
	if err := bindURI(c, &uri); err != nil || strings.TrimSpace(uri.ApprovalID) == "" {
		s.writeError(c, http.StatusBadRequest, "BAD_REQUEST", "approvalId is invalid")
		return
	}
	id := uri.ApprovalID
	resolution, err := s.adkRuntime.ResolveApprovalAsync(context.WithoutCancel(c.Request.Context()), id, approved)
	if err != nil {
		s.writeError(c, http.StatusInternalServerError, "ADK_APPROVAL_RESOLVE_FAILED", err.Error())
		return
	}
	s.writeOK(c, jfadk.NormalizeApprovalResolution(resolution))
}
