package assistant

import (
	"errors"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/jftrade/jftrade-main/internal/api/httpserver"
	asstsvc "github.com/jftrade/jftrade-main/internal/assistant"
	jfadk "github.com/jftrade/jftrade-main/pkg/adk"
)

func (h *Handler) handleADKWorkflows(c *gin.Context) {
	var query adkWorkflowsQuery
	if err := bindADKQuery(c, &query); err != nil {
		h.writeError(c, http.StatusBadRequest, "BAD_REQUEST", "invalid workflows query")
		return
	}
	limit, offset := adkPageBounds(adkPageQuery{Limit: query.Limit, Offset: query.Offset})
	page, err := h.service.ListWorkflows(c.Request.Context(), asstsvc.WorkflowQuery{
		Status: query.Status, Limit: limit, Offset: offset,
	})
	if err != nil {
		h.writeError(c, http.StatusInternalServerError, "ADK_WORKFLOW_LIST_FAILED", err.Error())
		return
	}
	h.writeOK(c, map[string]any{"workflows": page.Items, "page": pageEnvelope(page.Limit, page.Offset, page.Total, len(page.Items))})
}

func (h *Handler) handleADKWorkflow(c *gin.Context) {
	var uri workflowURI
	if err := httpserver.BindURI(c, &uri); err != nil || strings.TrimSpace(uri.WorkflowID) == "" {
		h.writeError(c, http.StatusBadRequest, "BAD_REQUEST", "workflowId is invalid")
		return
	}
	workflow, err := h.service.GetWorkflow(c.Request.Context(), uri.WorkflowID)
	if err != nil {
		h.writeWorkflowError(c, err, "ADK_WORKFLOW_GET_FAILED")
		return
	}
	h.writeOK(c, workflow)
}

func (h *Handler) handleADKSaveWorkflow(c *gin.Context) {
	workflowID := ""
	if c.Request.Method == http.MethodPut {
		var uri workflowURI
		if err := httpserver.BindURI(c, &uri); err != nil || strings.TrimSpace(uri.WorkflowID) == "" {
			h.writeError(c, http.StatusBadRequest, "BAD_REQUEST", "workflowId is invalid")
			return
		}
		workflowID = uri.WorkflowID
	}
	var payload jfadk.WorkflowDefinitionWriteRequest
	if err := c.ShouldBindJSON(&payload); err != nil && !errors.Is(err, io.EOF) {
		h.writeError(c, http.StatusBadRequest, "BAD_REQUEST", "invalid workflow payload")
		return
	}
	workflow, err := h.service.SaveWorkflow(c.Request.Context(), workflowID, payload)
	if err != nil {
		h.writeWorkflowError(c, err, "ADK_WORKFLOW_SAVE_FAILED")
		return
	}
	h.writeOK(c, workflow)
}

func (h *Handler) handleADKDeleteWorkflow(c *gin.Context) {
	var uri workflowURI
	if err := httpserver.BindURI(c, &uri); err != nil || strings.TrimSpace(uri.WorkflowID) == "" {
		h.writeError(c, http.StatusBadRequest, "BAD_REQUEST", "workflowId is invalid")
		return
	}
	workflow, err := h.service.DeleteWorkflow(c.Request.Context(), uri.WorkflowID)
	if err != nil {
		h.writeWorkflowError(c, err, "ADK_WORKFLOW_DELETE_FAILED")
		return
	}
	h.writeOK(c, map[string]any{"deleted": true, "workflow": workflow})
}

func (h *Handler) handleADKRunWorkflow(c *gin.Context) {
	var uri workflowURI
	if err := httpserver.BindURI(c, &uri); err != nil || strings.TrimSpace(uri.WorkflowID) == "" {
		h.writeError(c, http.StatusBadRequest, "BAD_REQUEST", "workflowId is invalid")
		return
	}
	inputs, ok := h.decodeWorkflowInputs(c)
	if !ok {
		return
	}
	result, err := h.service.RunWorkflow(c.Request.Context(), uri.WorkflowID, inputs)
	if err != nil {
		h.writeWorkflowError(c, err, "ADK_WORKFLOW_RUN_FAILED")
		return
	}
	h.writeOK(c, result)
}

func (h *Handler) handleADKWorkflowTriggers(c *gin.Context) {
	var uri workflowURI
	if err := httpserver.BindURI(c, &uri); err != nil || strings.TrimSpace(uri.WorkflowID) == "" {
		h.writeError(c, http.StatusBadRequest, "BAD_REQUEST", "workflowId is invalid")
		return
	}
	triggers, err := h.service.ListWorkflowTriggers(c.Request.Context(), uri.WorkflowID)
	if err != nil {
		h.writeWorkflowError(c, err, "ADK_WORKFLOW_TRIGGER_LIST_FAILED")
		return
	}
	h.writeOK(c, map[string]any{"triggers": triggers})
}

func (h *Handler) handleADKSaveWorkflowTrigger(c *gin.Context) {
	var uri workflowTriggerURI
	workflowID := c.Param("workflowId")
	triggerID := ""
	if c.Request.Method == http.MethodPut {
		if err := httpserver.BindURI(c, &uri); err != nil || strings.TrimSpace(uri.WorkflowID) == "" || strings.TrimSpace(uri.TriggerID) == "" {
			h.writeError(c, http.StatusBadRequest, "BAD_REQUEST", "workflowId or triggerId is invalid")
			return
		}
		workflowID = uri.WorkflowID
		triggerID = uri.TriggerID
	}
	if strings.TrimSpace(workflowID) == "" {
		h.writeError(c, http.StatusBadRequest, "BAD_REQUEST", "workflowId is invalid")
		return
	}
	var payload jfadk.WorkflowTriggerWriteRequest
	if err := c.ShouldBindJSON(&payload); err != nil && !errors.Is(err, io.EOF) {
		h.writeError(c, http.StatusBadRequest, "BAD_REQUEST", "invalid workflow trigger payload")
		return
	}
	result, err := h.service.SaveWorkflowTrigger(c.Request.Context(), workflowID, triggerID, payload)
	if err != nil {
		h.writeWorkflowError(c, err, "ADK_WORKFLOW_TRIGGER_SAVE_FAILED")
		return
	}
	h.writeOK(c, result)
}

func (h *Handler) handleADKDeleteWorkflowTrigger(c *gin.Context) {
	var uri workflowTriggerURI
	if err := httpserver.BindURI(c, &uri); err != nil || strings.TrimSpace(uri.WorkflowID) == "" || strings.TrimSpace(uri.TriggerID) == "" {
		h.writeError(c, http.StatusBadRequest, "BAD_REQUEST", "workflowId or triggerId is invalid")
		return
	}
	trigger, err := h.service.DeleteWorkflowTrigger(c.Request.Context(), uri.WorkflowID, uri.TriggerID)
	if err != nil {
		h.writeWorkflowError(c, err, "ADK_WORKFLOW_TRIGGER_DELETE_FAILED")
		return
	}
	h.writeOK(c, map[string]any{"deleted": true, "trigger": trigger})
}

func (h *Handler) handleADKRunWorkflowTrigger(c *gin.Context) {
	var uri triggerURI
	if err := httpserver.BindURI(c, &uri); err != nil || strings.TrimSpace(uri.TriggerID) == "" {
		h.writeError(c, http.StatusBadRequest, "BAD_REQUEST", "triggerId is invalid")
		return
	}
	inputs, ok := h.decodeWorkflowInputs(c)
	if !ok {
		return
	}
	result, err := h.service.RunWorkflowTrigger(c.Request.Context(), uri.TriggerID, inputs)
	if err != nil {
		h.writeWorkflowError(c, err, "ADK_WORKFLOW_TRIGGER_RUN_FAILED")
		return
	}
	h.writeOK(c, result)
}

func (h *Handler) handleADKWorkflowTriggerLogs(c *gin.Context) {
	var query adkWorkflowTriggerLogsQuery
	if err := bindADKQuery(c, &query); err != nil {
		h.writeError(c, http.StatusBadRequest, "BAD_REQUEST", "invalid workflow trigger logs query")
		return
	}
	limit, offset := adkPageBounds(adkPageQuery{Limit: query.Limit, Offset: query.Offset})
	page, err := h.service.ListWorkflowTriggerLogs(c.Request.Context(), asstsvc.WorkflowTriggerLogQuery{
		WorkflowID: query.WorkflowID, TriggerID: query.TriggerID, Status: query.Status,
		Limit: limit, Offset: offset,
	})
	if err != nil {
		h.writeError(c, http.StatusInternalServerError, "ADK_WORKFLOW_TRIGGER_LOG_LIST_FAILED", err.Error())
		return
	}
	h.writeOK(c, map[string]any{"logs": page.Items, "page": pageEnvelope(page.Limit, page.Offset, page.Total, len(page.Items))})
}

func (h *Handler) handleADKWorkflowWebhook(c *gin.Context) {
	var uri triggerURI
	if err := httpserver.BindURI(c, &uri); err != nil || strings.TrimSpace(uri.TriggerID) == "" {
		h.writeError(c, http.StatusBadRequest, "BAD_REQUEST", "triggerId is invalid")
		return
	}
	secret := bearerToken(c.GetHeader("Authorization"))
	if secret == "" {
		secret = c.GetHeader("X-JFTrade-Workflow-Secret")
	}
	inputs, ok := h.decodeWorkflowInputs(c)
	if !ok {
		return
	}
	result, err := h.service.RunWorkflowWebhook(c.Request.Context(), uri.TriggerID, secret, inputs)
	if err != nil {
		status := http.StatusBadRequest
		if strings.Contains(strings.ToLower(err.Error()), "secret") {
			status = http.StatusUnauthorized
		}
		h.writeError(c, status, "ADK_WORKFLOW_WEBHOOK_FAILED", err.Error())
		return
	}
	h.writeOK(c, result)
}

func (h *Handler) decodeWorkflowInputs(c *gin.Context) (map[string]any, bool) {
	if c.Request.Body == nil {
		return map[string]any{}, true
	}
	var payload map[string]any
	if err := c.ShouldBindJSON(&payload); err != nil && !errors.Is(err, io.EOF) {
		h.writeError(c, http.StatusBadRequest, "BAD_REQUEST", "invalid workflow inputs")
		return nil, false
	}
	if inputs, ok := payload["inputs"].(map[string]any); ok {
		return inputs, true
	}
	if payload == nil {
		return map[string]any{}, true
	}
	return payload, true
}

func (h *Handler) writeWorkflowError(c *gin.Context, err error, code string) {
	message := strings.ToLower(err.Error())
	switch {
	case errors.Is(err, os.ErrNotExist), strings.Contains(message, "not found"):
		h.writeError(c, http.StatusNotFound, code, err.Error())
	case strings.Contains(message, "disabled"), strings.Contains(message, "active"):
		h.writeError(c, http.StatusConflict, code, err.Error())
	default:
		h.writeError(c, http.StatusBadRequest, code, err.Error())
	}
}

func bearerToken(header string) string {
	header = strings.TrimSpace(header)
	if header == "" {
		return ""
	}
	const prefix = "Bearer "
	if strings.HasPrefix(strings.ToLower(header), strings.ToLower(prefix)) {
		return strings.TrimSpace(header[len(prefix):])
	}
	return ""
}
