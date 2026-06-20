package assistant

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/jftrade/jftrade-main/internal/api/httpserver"
	asstsvc "github.com/jftrade/jftrade-main/internal/assistant"
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
// @Success 200 {object} httpserver.Envelope
// @Failure 400 {object} httpserver.Envelope
// @Failure 500 {object} httpserver.Envelope
// @Router /api/v1/adk/sessions [get]
func (h *Handler) handleADKSessions(c *gin.Context) {
	var query adkSessionsQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		h.writeError(c, http.StatusBadRequest, "BAD_REQUEST", "invalid sessions query")
		return
	}
	limit, offset := adkPageBounds(adkPageQuery{Limit: query.Limit, Offset: query.Offset})
	page, err := h.service.ListSessions(c.Request.Context(), asstsvc.SessionQuery{
		AgentID: query.AgentID, Query: query.Query, Limit: limit, Offset: offset,
	})
	if err != nil {
		h.writeError(c, http.StatusInternalServerError, "ADK_SESSION_LIST_FAILED", err.Error())
		return
	}
	h.writeOK(c, map[string]any{
		"sessions": page.Items,
		"page":     pageEnvelope(page.Limit, page.Offset, page.Total, len(page.Items)),
	})
}

func (h *Handler) handleADKCreateSession(c *gin.Context) {
	var payload struct {
		AgentID string `json:"agentId"`
		Title   string `json:"title"`
	}
	jftradeErr1 := c.ShouldBindJSON(&payload)
	jftradeLogError(jftradeErr1)
	session, err := h.service.CreateSession(c.Request.Context(), asstsvc.CreateSessionRequest{
		AgentID: payload.AgentID,
		Title:   payload.Title,
	})
	if err != nil {
		message := err.Error()
		if strings.Contains(strings.ToLower(message), "agent") {
			h.writeError(c, http.StatusBadRequest, "BAD_REQUEST", "enabled agent is required")
			return
		}
		h.writeError(c, http.StatusInternalServerError, "ADK_SESSION_CREATE_FAILED", message)
		return
	}
	h.writeOK(c, session)
}

func (h *Handler) handleADKSession(c *gin.Context) {
	var uri sessionURI
	if err := httpserver.BindURI(c, &uri); err != nil || strings.TrimSpace(uri.SessionID) == "" {
		h.writeError(c, http.StatusBadRequest, "BAD_REQUEST", "sessionId is invalid")
		return
	}
	session, err := h.service.GetSessionDetail(c.Request.Context(), uri.SessionID)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "not found") {
			h.writeError(c, http.StatusNotFound, "NOT_FOUND", "session not found")
			return
		}
		if errors.Is(err, asstsvc.ErrSessionTimelineFailed) {
			h.writeError(c, http.StatusInternalServerError, "ADK_MESSAGES_GET_FAILED", err.Error())
			return
		}
		h.writeError(c, http.StatusInternalServerError, "ADK_SESSION_GET_FAILED", err.Error())
		return
	}
	h.writeOK(c, session)
}

func (h *Handler) handleADKSessionContext(c *gin.Context) {
	var uri sessionURI
	if err := httpserver.BindURI(c, &uri); err != nil || strings.TrimSpace(uri.SessionID) == "" {
		h.writeError(c, http.StatusBadRequest, "BAD_REQUEST", "sessionId is invalid")
		return
	}
	snapshot, err := h.service.GetSessionContext(c.Request.Context(), uri.SessionID)
	if err != nil {
		status := http.StatusInternalServerError
		if strings.Contains(strings.ToLower(err.Error()), "not found") {
			status = http.StatusNotFound
		}
		h.writeError(c, status, "ADK_SESSION_CONTEXT_FAILED", err.Error())
		return
	}
	h.writeOK(c, snapshot)
}

func (h *Handler) handleADKCompactSessionContext(c *gin.Context) {
	var uri sessionURI
	if err := httpserver.BindURI(c, &uri); err != nil || strings.TrimSpace(uri.SessionID) == "" {
		h.writeError(c, http.StatusBadRequest, "BAD_REQUEST", "sessionId is invalid")
		return
	}
	var payload struct {
		Mode   string `json:"mode"`
		Reason string `json:"reason"`
	}
	jftradeErr2 := c.ShouldBindJSON(&payload)
	jftradeLogError(jftradeErr2)
	snapshot, err := h.service.CompactSessionContext(
		c.Request.Context(),
		uri.SessionID,
		payload.Mode,
		"manual",
		defaultString(payload.Reason, "manual context compaction requested"),
	)
	if err != nil {
		status := http.StatusInternalServerError
		lower := strings.ToLower(err.Error())
		switch {
		case strings.Contains(lower, "active run"):
			status = http.StatusConflict
		case strings.Contains(lower, "not found"):
			status = http.StatusNotFound
		}
		h.writeError(c, status, "ADK_SESSION_CONTEXT_COMPACT_FAILED", err.Error())
		return
	}
	h.writeOK(c, snapshot)
}

func (h *Handler) handleADKRenameSession(c *gin.Context) {
	var uri sessionURI
	if err := httpserver.BindURI(c, &uri); err != nil || strings.TrimSpace(uri.SessionID) == "" {
		h.writeError(c, http.StatusBadRequest, "BAD_REQUEST", "sessionId is invalid")
		return
	}
	var payload struct {
		Title string `json:"title"`
	}
	if err := c.ShouldBindJSON(&payload); err != nil {
		h.writeError(c, http.StatusBadRequest, "BAD_REQUEST", "invalid session payload")
		return
	}
	session, err := h.service.RenameSession(c.Request.Context(), uri.SessionID, payload.Title)
	if err != nil {
		h.writeError(c, http.StatusBadRequest, "ADK_SESSION_RENAME_FAILED", err.Error())
		return
	}
	h.writeOK(c, session)
}

func (h *Handler) handleADKUpdateSessionComposerState(c *gin.Context) {
	var uri sessionURI
	if err := httpserver.BindURI(c, &uri); err != nil || strings.TrimSpace(uri.SessionID) == "" {
		h.writeError(c, http.StatusBadRequest, "BAD_REQUEST", "sessionId is invalid")
		return
	}
	var payload jfadk.SessionComposerStatePatch
	if err := c.ShouldBindJSON(&payload); err != nil {
		h.writeError(c, http.StatusBadRequest, "BAD_REQUEST", "invalid composer state payload")
		return
	}
	state, err := h.service.UpdateSessionComposerState(c.Request.Context(), uri.SessionID, payload)
	if err != nil {
		lower := strings.ToLower(err.Error())
		if strings.Contains(lower, "not exist") || strings.Contains(lower, "not found") {
			h.writeError(c, http.StatusNotFound, "NOT_FOUND", "session not found")
			return
		}
		h.writeError(c, http.StatusBadRequest, "ADK_SESSION_COMPOSER_STATE_UPDATE_FAILED", err.Error())
		return
	}
	h.writeOK(c, state)
}

func (h *Handler) handleADKDeleteSession(c *gin.Context) {
	var uri sessionURI
	if err := httpserver.BindURI(c, &uri); err != nil || strings.TrimSpace(uri.SessionID) == "" {
		h.writeError(c, http.StatusBadRequest, "BAD_REQUEST", "sessionId is invalid")
		return
	}
	if err := h.service.DeleteSession(c.Request.Context(), uri.SessionID); err != nil {
		h.writeError(c, http.StatusInternalServerError, "ADK_SESSION_DELETE_FAILED", err.Error())
		return
	}
	h.writeOK(c, map[string]any{"deleted": true, "id": uri.SessionID})
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
// @Success 200 {object} httpserver.Envelope
// @Failure 400 {object} httpserver.Envelope
// @Failure 500 {object} httpserver.Envelope
// @Router /api/v1/adk/runs [get]
func (h *Handler) handleADKRuns(c *gin.Context) {
	h.service.ReconcileExpiredRuns(c.Request.Context())
	h.service.ReconcileResolvedApprovals(context.WithoutCancel(c.Request.Context()))
	var query adkRunsQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		h.writeError(c, http.StatusBadRequest, "BAD_REQUEST", "invalid runs query")
		return
	}
	limit, offset := adkPageBounds(adkPageQuery{Limit: query.Limit, Offset: query.Offset})
	page, err := h.service.ListRuns(c.Request.Context(), asstsvc.RunQuery{
		Status: query.Status, AgentID: query.AgentID, SessionID: query.SessionID,
		Limit: limit, Offset: offset,
	})
	if err != nil {
		h.writeError(c, http.StatusInternalServerError, "ADK_RUN_LIST_FAILED", err.Error())
		return
	}
	for index := range page.Items {
		page.Items[index] = jfadk.NormalizeRun(page.Items[index])
	}
	h.writeOK(c, map[string]any{"runs": page.Items, "page": pageEnvelope(page.Limit, page.Offset, page.Total, len(page.Items))})
}

func (h *Handler) handleADKCancelRun(c *gin.Context) {
	var uri runURI
	if err := httpserver.BindURI(c, &uri); err != nil || strings.TrimSpace(uri.RunID) == "" {
		h.writeError(c, http.StatusBadRequest, "BAD_REQUEST", "runId is invalid")
		return
	}
	run, err := h.service.CancelRun(c.Request.Context(), uri.RunID)
	if err != nil {
		h.writeError(c, http.StatusNotFound, "ADK_RUN_CANCEL_FAILED", err.Error())
		return
	}
	h.writeOK(c, jfadk.NormalizeRun(run))
}

func (h *Handler) handleADKPauseRun(c *gin.Context) {
	var uri runURI
	if err := httpserver.BindURI(c, &uri); err != nil || strings.TrimSpace(uri.RunID) == "" {
		h.writeError(c, http.StatusBadRequest, "BAD_REQUEST", "runId is invalid")
		return
	}
	run, err := h.service.PauseGoalRun(c.Request.Context(), uri.RunID)
	if err != nil {
		message := err.Error()
		if strings.Contains(strings.ToLower(message), "not found") {
			h.writeError(c, http.StatusNotFound, "NOT_FOUND", "run not found")
			return
		}
		h.writeError(c, http.StatusBadRequest, "ADK_RUN_PAUSE_FAILED", message)
		return
	}
	h.writeOK(c, jfadk.NormalizeRun(run))
}

func (h *Handler) handleADKResumeRun(c *gin.Context) {
	var uri runURI
	if err := httpserver.BindURI(c, &uri); err != nil || strings.TrimSpace(uri.RunID) == "" {
		h.writeError(c, http.StatusBadRequest, "BAD_REQUEST", "runId is invalid")
		return
	}
	run, err := h.service.ResumeGoalRun(c.Request.Context(), uri.RunID)
	if err != nil {
		message := err.Error()
		if strings.Contains(strings.ToLower(message), "not found") {
			h.writeError(c, http.StatusNotFound, "NOT_FOUND", "run not found")
			return
		}
		h.writeError(c, http.StatusBadRequest, "ADK_RUN_RESUME_FAILED", message)
		return
	}
	h.writeOK(c, jfadk.NormalizeRun(run))
}

func (h *Handler) handleADKUpdateRunObjective(c *gin.Context) {
	var uri runURI
	if err := httpserver.BindURI(c, &uri); err != nil || strings.TrimSpace(uri.RunID) == "" {
		h.writeError(c, http.StatusBadRequest, "BAD_REQUEST", "runId is invalid")
		return
	}
	var payload struct {
		Objective string `json:"objective"`
	}
	if err := c.ShouldBindJSON(&payload); err != nil {
		h.writeError(c, http.StatusBadRequest, "BAD_REQUEST", "invalid objective payload")
		return
	}
	run, err := h.service.UpdateRunObjective(c.Request.Context(), uri.RunID, payload.Objective)
	if err != nil {
		message := err.Error()
		if strings.Contains(strings.ToLower(message), "not found") {
			h.writeError(c, http.StatusNotFound, "NOT_FOUND", "run not found")
			return
		}
		h.writeError(c, http.StatusBadRequest, "ADK_RUN_OBJECTIVE_UPDATE_FAILED", message)
		return
	}
	h.writeOK(c, jfadk.NormalizeRun(run))
}

func (h *Handler) handleADKRun(c *gin.Context) {
	h.service.ReconcileExpiredRuns(c.Request.Context())
	h.service.ReconcileResolvedApprovals(context.WithoutCancel(c.Request.Context()))
	var uri runURI
	if err := httpserver.BindURI(c, &uri); err != nil || strings.TrimSpace(uri.RunID) == "" {
		h.writeError(c, http.StatusBadRequest, "BAD_REQUEST", "runId is invalid")
		return
	}
	run, err := h.service.GetRun(c.Request.Context(), uri.RunID)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "not found") {
			h.writeError(c, http.StatusNotFound, "NOT_FOUND", "run not found")
			return
		}
		h.writeError(c, http.StatusInternalServerError, "ADK_RUN_GET_FAILED", err.Error())
		return
	}
	h.writeOK(c, jfadk.NormalizeRun(run))
}

func (h *Handler) handleADKApprovals(c *gin.Context) {
	h.service.ReconcileResolvedApprovals(context.WithoutCancel(c.Request.Context()))
	var query adkApprovalsQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		h.writeError(c, http.StatusBadRequest, "BAD_REQUEST", "invalid approvals query")
		return
	}
	limit, offset := adkPageBounds(adkPageQuery{Limit: query.Limit, Offset: query.Offset})
	page, err := h.service.ListApprovals(c.Request.Context(), asstsvc.ApprovalQuery{
		Status: query.Status, AgentID: query.AgentID, Limit: limit, Offset: offset,
	})
	if err != nil {
		h.writeError(c, http.StatusInternalServerError, "ADK_APPROVAL_LIST_FAILED", err.Error())
		return
	}
	h.writeOK(c, map[string]any{
		"approvals": page.Items,
		"page":      pageEnvelope(page.Limit, page.Offset, page.Total, len(page.Items)),
	})
}

func (h *Handler) handleADKApproval(c *gin.Context, approved bool) {
	var uri approvalURI
	if err := httpserver.BindURI(c, &uri); err != nil || strings.TrimSpace(uri.ApprovalID) == "" {
		h.writeError(c, http.StatusBadRequest, "BAD_REQUEST", "approvalId is invalid")
		return
	}
	resolution, err := h.service.ResolveApprovalAsync(context.WithoutCancel(c.Request.Context()), uri.ApprovalID, approved)
	if err != nil {
		h.writeError(c, http.StatusInternalServerError, "ADK_APPROVAL_RESOLVE_FAILED", err.Error())
		return
	}
	h.writeOK(c, jfadk.NormalizeApprovalResolution(resolution))
}
