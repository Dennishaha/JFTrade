package assistant

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/jftrade/jftrade-main/internal/api/httpserver"
	assistantservice "github.com/jftrade/jftrade-main/internal/assistant"
)

// Handler owns the Assistant HTTP transport.
type Handler struct {
	service *assistantservice.Service
	streams *adkChatStreamHub
}

// RegisterRoutes registers the stable /api/v1/adk contract.
func RegisterRoutes(api *gin.RouterGroup, service *assistantservice.Service) {
	handler := &Handler{service: service, streams: newADKChatStreamHub()}
	adk := api.Group("/adk", handler.requireAvailable())
	handler.registerCatalogRoutes(adk)
	handler.registerWorkflowRoutes(adk)
	handler.registerTaskAndMemoryRoutes(adk)
	handler.registerOptimizationRoutes(adk)
	handler.registerProviderRoutes(adk)
	handler.registerAgentRoutes(adk)
	handler.registerSessionRoutes(adk)
	handler.registerChatAndRunRoutes(adk)
	handler.registerApprovalRoutes(adk)
	handler.registerSkillRoutes(adk)
}

func (h *Handler) registerCatalogRoutes(adk *gin.RouterGroup) {
	adk.GET("", h.handleADKSnapshot)
	adk.GET("/tools", h.handleADKTools)
	adk.GET("/agent-templates", h.handleADKAgentTemplates)
	adk.GET("/audit", h.handleADKAudit)
	adk.GET("/metrics", h.handleADKMetrics)
}

func (h *Handler) registerWorkflowRoutes(adk *gin.RouterGroup) {
	adk.GET("/workflows", h.handleADKWorkflows)
	adk.POST("/workflows", h.handleADKSaveWorkflow)
	adk.GET("/workflows/:workflowId", h.handleADKWorkflow)
	adk.PUT("/workflows/:workflowId", h.handleADKSaveWorkflow)
	adk.DELETE("/workflows/:workflowId", h.handleADKDeleteWorkflow)
	adk.POST("/workflows/:workflowId/run", h.handleADKRunWorkflow)
	adk.GET("/workflows/:workflowId/triggers", h.handleADKWorkflowTriggers)
	adk.POST("/workflows/:workflowId/triggers", h.handleADKSaveWorkflowTrigger)
	adk.PUT("/workflows/:workflowId/triggers/:triggerId", h.handleADKSaveWorkflowTrigger)
	adk.DELETE("/workflows/:workflowId/triggers/:triggerId", h.handleADKDeleteWorkflowTrigger)
	adk.POST("/workflow-triggers/:triggerId/run", h.handleADKRunWorkflowTrigger)
	adk.GET("/workflow-trigger-logs", h.handleADKWorkflowTriggerLogs)
	adk.POST("/workflow-webhooks/:triggerId", h.handleADKWorkflowWebhook)
}

func (h *Handler) registerTaskAndMemoryRoutes(adk *gin.RouterGroup) {
	adk.GET("/tasks", h.handleADKTasks)
	adk.POST("/tasks", h.handleADKSaveTask)
	adk.GET("/tasks/:taskId", h.handleADKTask)
	adk.PUT("/tasks/:taskId", h.handleADKSaveTask)
	adk.DELETE("/tasks/:taskId", h.handleADKDeleteTask)
	adk.GET("/memory", h.handleADKMemory)
	adk.POST("/memory", h.handleADKSaveMemory)
	adk.DELETE("/memory/:memoryId", h.handleADKDeleteMemory)
}

func (h *Handler) registerOptimizationRoutes(adk *gin.RouterGroup) {
	adk.GET("/optimization-tasks", h.handleADKOptimizationTasks)
	adk.GET("/optimization-tasks/:taskId", h.handleADKOptimizationTask)
	adk.POST("/optimization-tasks/:taskId/cancel", h.handleADKOptimizationTaskCancel)
}

func (h *Handler) registerProviderRoutes(adk *gin.RouterGroup) {
	adk.GET("/providers", h.handleADKProviders)
	adk.POST("/providers", h.handleADKSaveProvider)
	adk.PUT("/providers/:providerId", h.handleADKSaveProvider)
	adk.DELETE("/providers/:providerId", h.handleADKDeleteProvider)
	adk.POST("/providers/:providerId/default", h.handleADKSetDefaultProvider)
	adk.POST("/providers/:providerId/test", h.handleADKTestProvider)
}

func (h *Handler) registerAgentRoutes(adk *gin.RouterGroup) {
	adk.GET("/agents", h.handleADKAgents)
	adk.POST("/agents", h.handleADKSaveAgent)
	adk.PUT("/agents/:agentId", h.handleADKSaveAgent)
	adk.DELETE("/agents/:agentId", h.handleADKDeleteAgent)
}

func (h *Handler) registerSessionRoutes(adk *gin.RouterGroup) {
	adk.GET("/sessions", h.handleADKSessions)
	adk.POST("/sessions", h.handleADKCreateSession)
	adk.GET("/sessions/:sessionId", h.handleADKSession)
	adk.GET("/sessions/:sessionId/context", h.handleADKSessionContext)
	adk.POST("/sessions/:sessionId/context/compact", h.handleADKCompactSessionContext)
	adk.PATCH("/sessions/:sessionId/composer-state", h.handleADKUpdateSessionComposerState)
	adk.PUT("/sessions/:sessionId", h.handleADKRenameSession)
	adk.DELETE("/sessions/:sessionId", h.handleADKDeleteSession)
}

func (h *Handler) registerChatAndRunRoutes(adk *gin.RouterGroup) {
	adk.POST("/chat", h.handleADKChat)
	adk.POST("/chat/stream", h.handleADKChatStream)
	adk.GET("/streams/:streamId", h.handleADKChatStreamReconnect)
	adk.GET("/runs", h.handleADKRuns)
	adk.GET("/runs/:runId/stream", h.handleADKRunStreamReconnect)
	adk.GET("/runs/:runId", h.handleADKRun)
	adk.PATCH("/runs/:runId/objective", h.handleADKUpdateRunObjective)
	adk.POST("/runs/:runId/pause", h.handleADKPauseRun)
	adk.POST("/runs/:runId/resume", h.handleADKResumeRun)
	adk.POST("/runs/:runId/cancel", h.handleADKCancelRun)
}

func (h *Handler) registerApprovalRoutes(adk *gin.RouterGroup) {
	adk.GET("/approvals", h.handleADKApprovals)
	adk.POST("/approvals/:approvalId/approve", func(c *gin.Context) { h.handleADKApproval(c, true) })
	adk.POST("/approvals/:approvalId/deny", func(c *gin.Context) { h.handleADKApproval(c, false) })
}

func (h *Handler) registerSkillRoutes(adk *gin.RouterGroup) {
	adk.GET("/skills", h.handleADKSkills)
	adk.POST("/skills", h.handleADKInstallSkill)
	adk.PUT("/skills/:skillId", h.handleADKSkillUpdateRemoved)
	adk.DELETE("/skills/:skillId", h.handleADKDeleteSkill)
}

func (h *Handler) requireAvailable() gin.HandlerFunc {
	return func(c *gin.Context) {
		if h.service == nil || !h.service.Available() {
			httpserver.WriteError(c, http.StatusServiceUnavailable, "ADK_UNAVAILABLE", "ADK runtime is unavailable")
			return
		}
		c.Next()
	}
}

func (h *Handler) writeOK(c *gin.Context, data any) {
	httpserver.WriteOK(c, data)
}

func (h *Handler) writeError(c *gin.Context, status int, code string, message string) {
	httpserver.WriteError(c, status, code, message)
}

func (h *Handler) handleADKSkillUpdateRemoved(c *gin.Context) {
	h.writeError(c, http.StatusGone, "ADK_SKILL_UPDATE_REMOVED", "skill enable/disable has been removed; bind skills directly on the agent")
}
