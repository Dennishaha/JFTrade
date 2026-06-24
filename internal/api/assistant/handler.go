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
	adk.GET("", handler.handleADKSnapshot)
	adk.GET("/tools", handler.handleADKTools)
	adk.GET("/agent-templates", handler.handleADKAgentTemplates)
	adk.GET("/audit", handler.handleADKAudit)
	adk.GET("/metrics", handler.handleADKMetrics)
	adk.GET("/tasks", handler.handleADKTasks)
	adk.POST("/tasks", handler.handleADKSaveTask)
	adk.GET("/tasks/:taskId", handler.handleADKTask)
	adk.PUT("/tasks/:taskId", handler.handleADKSaveTask)
	adk.DELETE("/tasks/:taskId", handler.handleADKDeleteTask)
	adk.GET("/memory", handler.handleADKMemory)
	adk.POST("/memory", handler.handleADKSaveMemory)
	adk.DELETE("/memory/:memoryId", handler.handleADKDeleteMemory)
	adk.GET("/optimization-tasks", handler.handleADKOptimizationTasks)
	adk.GET("/optimization-tasks/:taskId", handler.handleADKOptimizationTask)
	adk.POST("/optimization-tasks/:taskId/cancel", handler.handleADKOptimizationTaskCancel)
	adk.GET("/providers", handler.handleADKProviders)
	adk.POST("/providers", handler.handleADKSaveProvider)
	adk.PUT("/providers/:providerId", handler.handleADKSaveProvider)
	adk.DELETE("/providers/:providerId", handler.handleADKDeleteProvider)
	adk.POST("/providers/:providerId/default", handler.handleADKSetDefaultProvider)
	adk.POST("/providers/:providerId/test", handler.handleADKTestProvider)
	adk.GET("/agents", handler.handleADKAgents)
	adk.POST("/agents", handler.handleADKSaveAgent)
	adk.PUT("/agents/:agentId", handler.handleADKSaveAgent)
	adk.DELETE("/agents/:agentId", handler.handleADKDeleteAgent)
	adk.GET("/sessions", handler.handleADKSessions)
	adk.POST("/sessions", handler.handleADKCreateSession)
	adk.GET("/sessions/:sessionId", handler.handleADKSession)
	adk.GET("/sessions/:sessionId/context", handler.handleADKSessionContext)
	adk.POST("/sessions/:sessionId/context/compact", handler.handleADKCompactSessionContext)
	adk.PATCH("/sessions/:sessionId/composer-state", handler.handleADKUpdateSessionComposerState)
	adk.PUT("/sessions/:sessionId", handler.handleADKRenameSession)
	adk.DELETE("/sessions/:sessionId", handler.handleADKDeleteSession)
	adk.POST("/chat", handler.handleADKChat)
	adk.POST("/chat/stream", handler.handleADKChatStream)
	adk.GET("/streams/:streamId", handler.handleADKChatStreamReconnect)
	adk.GET("/runs", handler.handleADKRuns)
	adk.GET("/runs/:runId/stream", handler.handleADKRunStreamReconnect)
	adk.GET("/runs/:runId", handler.handleADKRun)
	adk.PATCH("/runs/:runId/objective", handler.handleADKUpdateRunObjective)
	adk.POST("/runs/:runId/pause", handler.handleADKPauseRun)
	adk.POST("/runs/:runId/resume", handler.handleADKResumeRun)
	adk.POST("/runs/:runId/cancel", handler.handleADKCancelRun)
	adk.GET("/approvals", handler.handleADKApprovals)
	adk.POST("/approvals/:approvalId/approve", func(c *gin.Context) { handler.handleADKApproval(c, true) })
	adk.POST("/approvals/:approvalId/deny", func(c *gin.Context) { handler.handleADKApproval(c, false) })
	adk.GET("/skills", handler.handleADKSkills)
	adk.POST("/skills", handler.handleADKInstallSkill)
	adk.PUT("/skills/:skillId", handler.handleADKSkillUpdateRemoved)
	adk.DELETE("/skills/:skillId", handler.handleADKDeleteSkill)
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
