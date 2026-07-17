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

func (h *Handler) handleADKAgentTemplates(c *gin.Context) {
	templates, err := h.service.AgentTemplates(c.Request.Context())
	if err != nil {
		h.writeError(c, http.StatusInternalServerError, "ADK_AGENT_TEMPLATE_LIST_FAILED", err.Error())
		return
	}
	h.writeOK(c, map[string]any{"templates": templates})
}

// handleADKSnapshot godoc
// @Summary 读取 ADK 快照
// @Tags adk
// @Produce json
// @Success 200 {object} httpserver.Envelope
// @Failure 500 {object} httpserver.Envelope
// @Router /api/v1/adk [get]
func (h *Handler) handleADKSnapshot(c *gin.Context) {
	snapshot, err := h.service.Snapshot(c.Request.Context())
	if err != nil {
		h.writeError(c, http.StatusInternalServerError, "ADK_SNAPSHOT_FAILED", err.Error())
		return
	}
	h.writeOK(c, snapshot)
}

func (h *Handler) handleADKTools(c *gin.Context) {
	tools, err := h.service.Tools(c.Request.Context())
	if err != nil {
		h.writeError(c, http.StatusInternalServerError, "ADK_TOOL_LIST_FAILED", err.Error())
		return
	}
	h.writeOK(c, map[string]any{"tools": tools})
}

func (h *Handler) handleADKTasks(c *gin.Context) {
	var query adkTasksQuery
	if err := bindADKQuery(c, &query); err != nil {
		h.writeError(c, http.StatusBadRequest, "BAD_REQUEST", "invalid tasks query")
		return
	}
	limit, offset := httpserver.NormalizeBoundPage(query.Limit.Int(), query.Offset.Int(), 20, 100)
	page, err := h.service.ListTasks(c.Request.Context(), asstsvc.TaskQuery{
		Status: query.Status, AgentID: query.AgentID, RunID: query.RunID,
		Limit: limit, Offset: offset,
	})
	if err != nil {
		status := http.StatusInternalServerError
		if strings.Contains(err.Error(), "invalid task status") {
			status = http.StatusBadRequest
		}
		h.writeError(c, status, "ADK_TASK_LIST_FAILED", err.Error())
		return
	}
	h.writeOK(c, map[string]any{"tasks": page.Items, "page": pageEnvelope(page.Limit, page.Offset, page.Total, len(page.Items))})
}

func (h *Handler) handleADKTask(c *gin.Context) {
	var uri taskURI
	if err := httpserver.BindURI(c, &uri); err != nil || strings.TrimSpace(uri.TaskID) == "" {
		h.writeError(c, http.StatusBadRequest, "BAD_REQUEST", "taskId is invalid")
		return
	}
	task, err := h.service.GetTask(c.Request.Context(), uri.TaskID)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "not found") {
			h.writeError(c, http.StatusNotFound, "ADK_TASK_NOT_FOUND", "task not found")
			return
		}
		h.writeError(c, http.StatusInternalServerError, "ADK_TASK_GET_FAILED", err.Error())
		return
	}
	h.writeOK(c, task)
}

func (h *Handler) handleADKSaveTask(c *gin.Context) {
	if c.Request.Method == http.MethodPut {
		h.handleADKPatchTask(c)
		return
	}
	var payload jfadk.TaskWriteRequest
	if err := c.ShouldBindJSON(&payload); err != nil && !errors.Is(err, io.EOF) {
		h.writeError(c, http.StatusBadRequest, "BAD_REQUEST", "invalid task payload")
		return
	}
	task, err := h.service.SaveTask(c.Request.Context(), payload)
	if err != nil {
		h.writeError(c, http.StatusBadRequest, "ADK_TASK_SAVE_FAILED", err.Error())
		return
	}
	h.writeOK(c, task)
}

func (h *Handler) handleADKPatchTask(c *gin.Context) {
	var uri taskURI
	if err := httpserver.BindURI(c, &uri); err != nil || strings.TrimSpace(uri.TaskID) == "" {
		h.writeError(c, http.StatusBadRequest, "BAD_REQUEST", "taskId is invalid")
		return
	}
	var payload jfadk.TaskPatchRequest
	if err := c.ShouldBindJSON(&payload); err != nil && !errors.Is(err, io.EOF) {
		h.writeError(c, http.StatusBadRequest, "BAD_REQUEST", "invalid task payload")
		return
	}
	task, err := h.service.UpdateTask(c.Request.Context(), uri.TaskID, payload)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			h.writeError(c, http.StatusNotFound, "ADK_TASK_NOT_FOUND", "task not found")
			return
		}
		h.writeError(c, http.StatusBadRequest, "ADK_TASK_SAVE_FAILED", err.Error())
		return
	}
	h.writeOK(c, task)
}

func (h *Handler) handleADKDeleteTask(c *gin.Context) {
	var uri taskURI
	if err := httpserver.BindURI(c, &uri); err != nil || strings.TrimSpace(uri.TaskID) == "" {
		h.writeError(c, http.StatusBadRequest, "BAD_REQUEST", "taskId is invalid")
		return
	}
	if err := h.service.DeleteTask(c.Request.Context(), uri.TaskID); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			h.writeError(c, http.StatusNotFound, "ADK_TASK_NOT_FOUND", "task not found")
			return
		}
		h.writeError(c, http.StatusInternalServerError, "ADK_TASK_DELETE_FAILED", err.Error())
		return
	}
	h.writeOK(c, map[string]any{"deleted": true, "id": uri.TaskID})
}

func (h *Handler) handleADKMemory(c *gin.Context) {
	var query adkMemoryQuery
	if err := bindADKQuery(c, &query); err != nil {
		h.writeError(c, http.StatusBadRequest, "BAD_REQUEST", "invalid memory query")
		return
	}
	entries, err := h.service.ListMemory(c.Request.Context(), asstsvc.MemoryQuery{
		Scope: query.Scope, AgentID: query.AgentID, Key: query.Key,
	})
	if err != nil {
		h.writeError(c, http.StatusBadRequest, "ADK_MEMORY_LIST_FAILED", err.Error())
		return
	}
	h.writeOK(c, map[string]any{"entries": entries})
}

func (h *Handler) handleADKSaveMemory(c *gin.Context) {
	var payload jfadk.MemoryWriteRequest
	if err := c.ShouldBindJSON(&payload); err != nil && !errors.Is(err, io.EOF) {
		h.writeError(c, http.StatusBadRequest, "BAD_REQUEST", "invalid memory payload")
		return
	}
	entry, err := h.service.SaveMemory(c.Request.Context(), payload)
	if err != nil {
		h.writeError(c, http.StatusBadRequest, "ADK_MEMORY_SAVE_FAILED", err.Error())
		return
	}
	h.writeOK(c, entry)
}

func (h *Handler) handleADKDeleteMemory(c *gin.Context) {
	var uri memoryURI
	if err := httpserver.BindURI(c, &uri); err != nil || strings.TrimSpace(uri.MemoryID) == "" {
		h.writeError(c, http.StatusBadRequest, "BAD_REQUEST", "memoryId is invalid")
		return
	}
	if err := h.service.DeleteMemory(c.Request.Context(), uri.MemoryID); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			h.writeError(c, http.StatusNotFound, "ADK_MEMORY_NOT_FOUND", "memory not found")
			return
		}
		h.writeError(c, http.StatusInternalServerError, "ADK_MEMORY_DELETE_FAILED", err.Error())
		return
	}
	h.writeOK(c, map[string]any{"deleted": true, "id": uri.MemoryID})
}

// handleADKProviders godoc
// @Summary 读取 ADK Provider 列表
// @Tags adk
// @Produce json
// @Success 200 {object} httpserver.Envelope
// @Failure 500 {object} httpserver.Envelope
// @Router /api/v1/adk/providers [get]
func (h *Handler) handleADKProviders(c *gin.Context) {
	result, err := h.service.ListProviders(c.Request.Context())
	if err != nil {
		h.writeError(c, http.StatusInternalServerError, "ADK_PROVIDER_LIST_FAILED", err.Error())
		return
	}
	h.writeOK(c, map[string]any{"providers": result})
}

func (h *Handler) handleADKTestProvider(c *gin.Context) {
	var uri providerURI
	if err := httpserver.BindURI(c, &uri); err != nil || strings.TrimSpace(uri.ProviderID) == "" {
		h.writeError(c, http.StatusBadRequest, "BAD_REQUEST", "providerId is invalid")
		return
	}
	result, err := h.service.TestProvider(c.Request.Context(), uri.ProviderID)
	if err != nil {
		h.writeError(c, http.StatusBadGateway, "ADK_PROVIDER_TEST_FAILED", err.Error())
		return
	}
	h.writeOK(c, result)
}

func (h *Handler) handleADKSetDefaultProvider(c *gin.Context) {
	var uri providerURI
	if err := httpserver.BindURI(c, &uri); err != nil || strings.TrimSpace(uri.ProviderID) == "" {
		h.writeError(c, http.StatusBadRequest, "BAD_REQUEST", "providerId is invalid")
		return
	}
	result, err := h.service.SetDefaultProvider(c.Request.Context(), uri.ProviderID)
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, os.ErrNotExist) {
			status = http.StatusNotFound
		}
		h.writeError(c, status, "ADK_PROVIDER_DEFAULT_FAILED", err.Error())
		return
	}
	h.writeOK(c, result)
}

func (h *Handler) handleADKDeleteProvider(c *gin.Context) {
	var uri providerURI
	if err := httpserver.BindURI(c, &uri); err != nil || strings.TrimSpace(uri.ProviderID) == "" {
		h.writeError(c, http.StatusBadRequest, "BAD_REQUEST", "providerId is invalid")
		return
	}
	if err := h.service.DeleteProvider(c.Request.Context(), uri.ProviderID); err != nil {
		status := http.StatusInternalServerError
		if strings.Contains(err.Error(), "used by agent") {
			status = http.StatusConflict
		}
		h.writeError(c, status, "ADK_PROVIDER_DELETE_FAILED", err.Error())
		return
	}
	h.writeOK(c, map[string]any{"deleted": true, "id": uri.ProviderID})
}

// handleADKAgents godoc
// @Summary 读取 ADK Agent 列表
// @Tags adk
// @Produce json
// @Param status query string false "Agent 状态过滤"
// @Success 200 {object} httpserver.Envelope
// @Failure 400 {object} httpserver.Envelope
// @Failure 500 {object} httpserver.Envelope
// @Router /api/v1/adk/agents [get]
func (h *Handler) handleADKAgents(c *gin.Context) {
	var query adkAgentsQuery
	if err := bindADKQuery(c, &query); err != nil {
		h.writeError(c, http.StatusBadRequest, "BAD_REQUEST", "invalid agents query")
		return
	}
	agents, err := h.service.ListAgents(c.Request.Context(), asstsvc.AgentQuery{Status: query.Status})
	if err != nil {
		h.writeError(c, http.StatusInternalServerError, "ADK_AGENT_LIST_FAILED", err.Error())
		return
	}
	limit, offset := adkPageBounds(adkPageQuery{Limit: query.Limit, Offset: query.Offset})
	total := len(agents)
	if offset > total {
		offset = total
	}
	end := min(offset+limit, total)
	h.writeOK(c, map[string]any{"agents": agents[offset:end], "page": pageEnvelope(limit, offset, total, end-offset)})
}

func (h *Handler) handleADKDeleteAgent(c *gin.Context) {
	var uri agentURI
	if err := httpserver.BindURI(c, &uri); err != nil || strings.TrimSpace(uri.AgentID) == "" {
		h.writeError(c, http.StatusBadRequest, "BAD_REQUEST", "agentId is invalid")
		return
	}
	if err := h.service.DeleteAgent(c.Request.Context(), uri.AgentID); err != nil {
		if errors.Is(err, jfadk.ErrBuiltinAgentProtected) {
			h.writeError(c, http.StatusConflict, "ADK_AGENT_PROTECTED", err.Error())
			return
		}
		h.writeError(c, http.StatusInternalServerError, "ADK_AGENT_DELETE_FAILED", err.Error())
		return
	}
	h.writeOK(c, map[string]any{"deleted": true, "id": uri.AgentID})
}

func (h *Handler) handleADKSkills(c *gin.Context) {
	result, err := h.service.ListSkills(c.Request.Context())
	if err != nil {
		h.writeError(c, http.StatusInternalServerError, "ADK_SKILL_LIST_FAILED", err.Error())
		return
	}
	h.writeOK(c, map[string]any{"skills": result})
}

func (h *Handler) handleADKInstallSkill(c *gin.Context) {
	var payload struct {
		URL string `json:"url"`
	}
	if err := c.ShouldBindJSON(&payload); err != nil {
		h.writeError(c, http.StatusBadRequest, "BAD_REQUEST", "invalid skill install payload")
		return
	}
	skill, err := h.service.InstallSkill(c.Request.Context(), payload.URL)
	if err != nil {
		h.writeError(c, http.StatusBadRequest, "ADK_SKILL_INSTALL_FAILED", err.Error())
		return
	}
	h.writeOK(c, skill)
}

func (h *Handler) handleADKDeleteSkill(c *gin.Context) {
	var uri skillURI
	if err := httpserver.BindURI(c, &uri); err != nil || strings.TrimSpace(uri.SkillID) == "" {
		h.writeError(c, http.StatusBadRequest, "BAD_REQUEST", "skillId is invalid")
		return
	}
	if err := h.service.DeleteSkill(c.Request.Context(), uri.SkillID); err != nil {
		h.writeError(c, http.StatusInternalServerError, "ADK_SKILL_UNINSTALL_FAILED", err.Error())
		return
	}
	h.writeOK(c, map[string]any{"deleted": true, "id": uri.SkillID})
}

func (h *Handler) handleADKSaveProvider(c *gin.Context) {
	var payload jfadk.ProviderWriteRequest
	if err := c.ShouldBindJSON(&payload); err != nil && !errors.Is(err, io.EOF) {
		h.writeError(c, http.StatusBadRequest, "BAD_REQUEST", "invalid provider payload")
		return
	}
	if c.Request.Method == http.MethodPut {
		var uri providerURI
		if err := httpserver.BindURI(c, &uri); err != nil || strings.TrimSpace(uri.ProviderID) == "" {
			h.writeError(c, http.StatusBadRequest, "BAD_REQUEST", "providerId is invalid")
			return
		}
		payload.ID = uri.ProviderID
	}
	provider, err := h.service.SaveProvider(c.Request.Context(), payload)
	if err != nil {
		h.writeError(c, http.StatusInternalServerError, "ADK_PROVIDER_SAVE_FAILED", err.Error())
		return
	}
	h.writeOK(c, provider)
}

func (h *Handler) handleADKSaveAgent(c *gin.Context) {
	var payload jfadk.AgentWriteRequest
	if err := c.ShouldBindJSON(&payload); err != nil && !errors.Is(err, io.EOF) {
		h.writeError(c, http.StatusBadRequest, "BAD_REQUEST", "invalid agent payload")
		return
	}
	if c.Request.Method == http.MethodPut {
		var uri agentURI
		if err := httpserver.BindURI(c, &uri); err != nil || strings.TrimSpace(uri.AgentID) == "" {
			h.writeError(c, http.StatusBadRequest, "BAD_REQUEST", "agentId is invalid")
			return
		}
		payload.ID = uri.AgentID
	}
	agent, err := h.service.SaveAgent(c.Request.Context(), payload)
	if err != nil {
		if errors.Is(err, jfadk.ErrBuiltinAgentProtected) {
			h.writeError(c, http.StatusConflict, "ADK_AGENT_PROTECTED", err.Error())
			return
		}
		if isADKAgentValidationError(err) {
			h.writeError(c, http.StatusBadRequest, "BAD_REQUEST", err.Error())
			return
		}
		h.writeError(c, http.StatusInternalServerError, "ADK_AGENT_SAVE_FAILED", err.Error())
		return
	}
	h.writeOK(c, agent)
}

func isADKAgentValidationError(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "invalid agent") ||
		errors.Is(err, jfadk.ErrBuiltinAgentProtected) ||
		strings.Contains(message, "provider not found") ||
		strings.Contains(message, "provider is disabled") ||
		strings.Contains(message, "provider api key is not configured") ||
		strings.Contains(message, "unknown adk tool") ||
		strings.Contains(message, "unknown adk skill")
}
