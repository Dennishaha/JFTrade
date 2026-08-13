package assistant

import (
	jfadkruntime "github.com/jftrade/jftrade-main/internal/assistant/engine/workflowruntime"
	assistantmodel "github.com/jftrade/jftrade-main/internal/assistant/model"
)

// Assistant-facing models are re-exported by the business boundary so
// application and transport tests do not depend on runtime assembly details.
type (
	Agent                          = assistantmodel.Agent
	AgentWriteRequest              = assistantmodel.AgentWriteRequest
	Approval                       = assistantmodel.Approval
	ApprovalResolution             = assistantmodel.ApprovalResolution
	AuditEvent                     = assistantmodel.AuditEvent
	ChatDelta                      = assistantmodel.ChatDelta
	ChatRequest                    = assistantmodel.ChatRequest
	ChatResponse                   = assistantmodel.ChatResponse
	MemoryEntry                    = assistantmodel.MemoryEntry
	OptimizationRunRef             = assistantmodel.OptimizationRunRef
	OptimizationTask               = assistantmodel.OptimizationTask
	Provider                       = assistantmodel.Provider
	ProviderReasoningConfig        = assistantmodel.ProviderReasoningConfig
	ProviderReasoningMapping       = assistantmodel.ProviderReasoningMapping
	ProviderReasoningTestResponse  = assistantmodel.ProviderReasoningTestResponse
	ProviderTestMode               = assistantmodel.ProviderTestMode
	ProviderTestResponse           = assistantmodel.ProviderTestResponse
	ProviderWriteRequest           = assistantmodel.ProviderWriteRequest
	Run                            = assistantmodel.Run
	RunUsage                       = assistantmodel.RunUsage
	Session                        = assistantmodel.Session
	SessionComposerState           = assistantmodel.SessionComposerState
	SessionContextSnapshot         = assistantmodel.SessionContextSnapshot
	Skill                          = assistantmodel.Skill
	Task                           = assistantmodel.Task
	TimelineEntry                  = assistantmodel.TimelineEntry
	ToolCall                       = assistantmodel.ToolCall
	ToolDescriptor                 = assistantmodel.ToolDescriptor
	ToolFunc                       = jfadkruntime.ToolFunc
	WorkflowCanvasEdge             = assistantmodel.WorkflowCanvasEdge
	WorkflowCanvasGraph            = assistantmodel.WorkflowCanvasGraph
	WorkflowCanvasNode             = assistantmodel.WorkflowCanvasNode
	WorkflowDefinition             = assistantmodel.WorkflowDefinition
	WorkflowDefinitionWriteRequest = assistantmodel.WorkflowDefinitionWriteRequest
	WorkflowTrigger                = assistantmodel.WorkflowTrigger
	WorkflowTriggerLog             = assistantmodel.WorkflowTriggerLog
	WorkflowTriggerWriteRequest    = assistantmodel.WorkflowTriggerWriteRequest
)

const (
	AgentStatusEnabled                = assistantmodel.AgentStatusEnabled
	ApprovalStatusApproved            = assistantmodel.ApprovalStatusApproved
	ApprovalStatusDenied              = assistantmodel.ApprovalStatusDenied
	ApprovalStatusPending             = assistantmodel.ApprovalStatusPending
	PermissionModeApproval            = assistantmodel.PermissionModeApproval
	PermissionModeLessApproval        = assistantmodel.PermissionModeLessApproval
	RunStatusCancelled                = assistantmodel.RunStatusCancelled
	RunStatusCompleted                = assistantmodel.RunStatusCompleted
	RunStatusDenied                   = assistantmodel.RunStatusDenied
	RunStatusFailed                   = assistantmodel.RunStatusFailed
	RunStatusPaused                   = assistantmodel.RunStatusPaused
	RunStatusPending                  = assistantmodel.RunStatusPending
	RunStatusRunning                  = assistantmodel.RunStatusRunning
	TimelineKindApprovalGroup         = assistantmodel.TimelineKindApprovalGroup
	TimelineKindToolGroup             = assistantmodel.TimelineKindToolGroup
	WorkflowStatusEnabled             = assistantmodel.WorkflowStatusEnabled
	WorkflowTriggerStatusEnabled      = assistantmodel.WorkflowTriggerStatusEnabled
	WorkflowTriggerLogStatusSucceeded = assistantmodel.WorkflowTriggerLogStatusSucceeded
	WorkflowTriggerTypeManual         = assistantmodel.WorkflowTriggerTypeManual
	WorkflowTriggerTypeWebhook        = assistantmodel.WorkflowTriggerTypeWebhook
	WorkModeChat                      = assistantmodel.WorkModeChat
	WorkModeLoop                      = assistantmodel.WorkModeLoop
)

var LocalMCPReadOnlyToolNames = jfadkruntime.LocalMCPReadOnlyToolNames

func ToolRequiredSkillNames(descriptor ToolDescriptor) []string {
	return assistantmodel.ToolRequiredSkillNames(descriptor)
}

func ToolRequiresApproval(descriptor ToolDescriptor, mode string) bool {
	return assistantmodel.ToolRequiresApproval(descriptor, mode)
}
