package assistant

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/jftrade/jftrade-main/internal/api/httpserver"
	asstsvc "github.com/jftrade/jftrade-main/internal/assistant"
)

func (h *Handler) handleADKAudit(c *gin.Context) {
	var query adkAuditQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		h.writeError(c, http.StatusBadRequest, "BAD_REQUEST", "invalid audit query")
		return
	}
	events, err := h.service.GetAudit(c.Request.Context(), asstsvc.AuditQuery{
		Kind: query.Kind, SubjectID: query.SubjectID,
	})
	if err != nil {
		h.writeError(c, http.StatusInternalServerError, "ADK_AUDIT_LIST_FAILED", err.Error())
		return
	}
	var pageQuery adkPageQuery
	_ = c.ShouldBindQuery(&pageQuery)
	limit, offset := adkPageBounds(pageQuery)
	total := len(events)
	if offset > total {
		offset = total
	}
	end := min(offset+limit, total)
	h.writeOK(c, map[string]any{
		"events": events[offset:end],
		"page":   pageEnvelope(limit, offset, total, end-offset),
	})
}

func (h *Handler) handleADKMetrics(c *gin.Context) {
	metrics, err := h.service.GetMetrics(c.Request.Context())
	if err != nil {
		h.writeError(c, http.StatusInternalServerError, "ADK_METRICS_FAILED", err.Error())
		return
	}
	h.writeOK(c, metrics)
}

func (h *Handler) handleADKOptimizationTasks(c *gin.Context) {
	result, err := h.service.ListOptimizationTasks(c.Request.Context())
	if err != nil {
		h.writeError(c, http.StatusInternalServerError, "ADK_OPTIMIZATION_LIST_FAILED", err.Error())
		return
	}
	var pageQuery adkPageQuery
	_ = c.ShouldBindQuery(&pageQuery)
	limit, offset := adkPageBounds(pageQuery)
	tasks := result.Tasks
	total := len(tasks)
	if offset > total {
		offset = total
	}
	end := min(offset+limit, total)
	h.writeOK(c, map[string]any{
		"tasks": tasks[offset:end],
		"page":  pageEnvelope(limit, offset, total, end-offset),
	})
}

func (h *Handler) handleADKOptimizationTask(c *gin.Context) {
	var uri taskURI
	if err := httpserver.BindURI(c, &uri); err != nil || strings.TrimSpace(uri.TaskID) == "" {
		h.writeError(c, http.StatusBadRequest, "BAD_REQUEST", "taskId is invalid")
		return
	}
	task, err := h.service.GetOptimizationTask(c.Request.Context(), uri.TaskID)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "not found") {
			h.writeError(c, http.StatusNotFound, "NOT_FOUND", "optimization task not found")
			return
		}
		h.writeError(c, http.StatusInternalServerError, "ADK_OPTIMIZATION_GET_FAILED", err.Error())
		return
	}
	h.writeOK(c, task)
}

func (h *Handler) handleADKOptimizationTaskCancel(c *gin.Context) {
	var uri taskURI
	if err := httpserver.BindURI(c, &uri); err != nil || strings.TrimSpace(uri.TaskID) == "" {
		h.writeError(c, http.StatusBadRequest, "BAD_REQUEST", "taskId is invalid")
		return
	}
	task, err := h.service.CancelOptimizationTask(c.Request.Context(), uri.TaskID)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "not found") {
			h.writeError(c, http.StatusNotFound, "NOT_FOUND", "optimization task not found")
			return
		}
		h.writeError(c, http.StatusInternalServerError, "ADK_OPTIMIZATION_CANCEL_FAILED", err.Error())
		return
	}
	h.writeOK(c, task)
}
