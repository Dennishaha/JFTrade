package assistant

import jfadk "github.com/jftrade/jftrade-main/internal/assistant/engine"

// Assistant-facing models are re-exported by the business boundary so
// application and transport tests do not depend on runtime assembly details.
type (
	Agent                          = jfadk.Agent
	AgentWriteRequest              = jfadk.AgentWriteRequest
	Approval                       = jfadk.Approval
	ApprovalResolution             = jfadk.ApprovalResolution
	AuditEvent                     = jfadk.AuditEvent
	ChatDelta                      = jfadk.ChatDelta
	ChatRequest                    = jfadk.ChatRequest
	ChatResponse                   = jfadk.ChatResponse
	MemoryEntry                    = jfadk.MemoryEntry
	OptimizationRunRef             = jfadk.OptimizationRunRef
	OptimizationTask               = jfadk.OptimizationTask
	Provider                       = jfadk.Provider
	ProviderWriteRequest           = jfadk.ProviderWriteRequest
	Run                            = jfadk.Run
	RunUsage                       = jfadk.RunUsage
	Session                        = jfadk.Session
	SessionComposerState           = jfadk.SessionComposerState
	SessionContextSnapshot         = jfadk.SessionContextSnapshot
	Skill                          = jfadk.Skill
	Task                           = jfadk.Task
	TimelineEntry                  = jfadk.TimelineEntry
	ToolCall                       = jfadk.ToolCall
	ToolDescriptor                 = jfadk.ToolDescriptor
	ToolFunc                       = jfadk.ToolFunc
	WorkflowCanvasEdge             = jfadk.WorkflowCanvasEdge
	WorkflowCanvasGraph            = jfadk.WorkflowCanvasGraph
	WorkflowCanvasNode             = jfadk.WorkflowCanvasNode
	WorkflowDefinition             = jfadk.WorkflowDefinition
	WorkflowDefinitionWriteRequest = jfadk.WorkflowDefinitionWriteRequest
	WorkflowTrigger                = jfadk.WorkflowTrigger
	WorkflowTriggerLog             = jfadk.WorkflowTriggerLog
	WorkflowTriggerWriteRequest    = jfadk.WorkflowTriggerWriteRequest
)

const (
	AgentStatusEnabled                = jfadk.AgentStatusEnabled
	ApprovalStatusApproved            = jfadk.ApprovalStatusApproved
	ApprovalStatusDenied              = jfadk.ApprovalStatusDenied
	ApprovalStatusPending             = jfadk.ApprovalStatusPending
	PermissionModeApproval            = jfadk.PermissionModeApproval
	PermissionModeLessApproval        = jfadk.PermissionModeLessApproval
	RunStatusCancelled                = jfadk.RunStatusCancelled
	RunStatusCompleted                = jfadk.RunStatusCompleted
	RunStatusDenied                   = jfadk.RunStatusDenied
	RunStatusFailed                   = jfadk.RunStatusFailed
	RunStatusPaused                   = jfadk.RunStatusPaused
	RunStatusPending                  = jfadk.RunStatusPending
	RunStatusRunning                  = jfadk.RunStatusRunning
	TimelineKindApprovalGroup         = jfadk.TimelineKindApprovalGroup
	TimelineKindToolGroup             = jfadk.TimelineKindToolGroup
	WorkflowStatusEnabled             = jfadk.WorkflowStatusEnabled
	WorkflowTriggerStatusEnabled      = jfadk.WorkflowTriggerStatusEnabled
	WorkflowTriggerLogStatusSucceeded = jfadk.WorkflowTriggerLogStatusSucceeded
	WorkflowTriggerTypeManual         = jfadk.WorkflowTriggerTypeManual
	WorkflowTriggerTypeWebhook        = jfadk.WorkflowTriggerTypeWebhook
	WorkModeChat                      = jfadk.WorkModeChat
	WorkModeLoop                      = jfadk.WorkModeLoop
)

var LocalMCPReadOnlyToolNames = jfadk.LocalMCPReadOnlyToolNames

func ToolRequiredSkillNames(descriptor ToolDescriptor) []string {
	return jfadk.ToolRequiredSkillNames(descriptor)
}

func ToolRequiresApproval(descriptor ToolDescriptor, mode string) bool {
	return jfadk.ToolRequiresApproval(descriptor, mode)
}
