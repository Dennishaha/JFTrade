// Package workflowruntime is the external ADK facade: it re-exports the
// engine-root runtime surface (Runtime/Store, constructors, normalizers and
// constants) and the workflow executor so consumers never import the engine
// root package directly. Public DTO aliases retain their legacy adk.* OpenAPI
// schema names even though runtime ownership moved into this package. The
// executor implementation lives in the leaf package workflowexec.
package workflowruntime

import (
	"context"

	jfadk "github.com/jftrade/jftrade-main/internal/assistant/engine"
	enginepersistence "github.com/jftrade/jftrade-main/internal/assistant/engine/persistence"
	workflowexec "github.com/jftrade/jftrade-main/internal/assistant/engine/workflowexec"
	jfadkmodel "github.com/jftrade/jftrade-main/internal/assistant/model"
	adksession "google.golang.org/adk/v2/session"
)

const (
	PermissionModeApproval     = jfadk.PermissionModeApproval
	PermissionModeLessApproval = jfadk.PermissionModeLessApproval
	PermissionModeAll          = jfadk.PermissionModeAll

	WorkModeChat = jfadk.WorkModeChat
	WorkModeLoop = jfadk.WorkModeLoop

	WorkflowEngineADK2Loop   = jfadk.WorkflowEngineADK2Loop
	WorkflowEngineADK2Canvas = jfadk.WorkflowEngineADK2Canvas

	AgentStatusEnabled  = jfadk.AgentStatusEnabled
	AgentStatusDisabled = jfadk.AgentStatusDisabled

	WorkflowStatusEnabled  = jfadk.WorkflowStatusEnabled
	WorkflowStatusDisabled = jfadk.WorkflowStatusDisabled

	WorkflowTriggerTypeManual          = jfadk.WorkflowTriggerTypeManual
	WorkflowTriggerTypeSchedule        = jfadk.WorkflowTriggerTypeSchedule
	WorkflowTriggerTypeWebhook         = jfadk.WorkflowTriggerTypeWebhook
	WorkflowTriggerTypeEvent           = jfadk.WorkflowTriggerTypeEvent
	WorkflowTriggerTypeMarketThreshold = jfadk.WorkflowTriggerTypeMarketThreshold

	WorkflowTriggerStatusEnabled  = jfadk.WorkflowTriggerStatusEnabled
	WorkflowTriggerStatusDisabled = jfadk.WorkflowTriggerStatusDisabled
	WorkflowTriggerStatusError    = jfadk.WorkflowTriggerStatusError

	WorkflowTriggerLogStatusQueued          = jfadk.WorkflowTriggerLogStatusQueued
	WorkflowTriggerLogStatusRunning         = jfadk.WorkflowTriggerLogStatusRunning
	WorkflowTriggerLogStatusSucceeded       = jfadk.WorkflowTriggerLogStatusSucceeded
	WorkflowTriggerLogStatusPendingApproval = jfadk.WorkflowTriggerLogStatusPendingApproval
	WorkflowTriggerLogStatusFailed          = jfadk.WorkflowTriggerLogStatusFailed
	WorkflowTriggerLogStatusCancelled       = jfadk.WorkflowTriggerLogStatusCancelled
	WorkflowTriggerLogStatusSkipped         = jfadk.WorkflowTriggerLogStatusSkipped

	RunStatusRunning      = jfadk.RunStatusRunning
	RunStatusCompleted    = jfadk.RunStatusCompleted
	RunStatusPending      = jfadk.RunStatusPending
	RunStatusPendingInput = jfadk.RunStatusPendingInput
	RunStatusFailed       = jfadk.RunStatusFailed
	RunStatusDenied       = jfadk.RunStatusDenied
	RunStatusCancelled    = jfadk.RunStatusCancelled
	RunStatusTimedOut     = jfadk.RunStatusTimedOut
	RunStatusPaused       = jfadk.RunStatusPaused

	ApprovalStatusPending  = jfadk.ApprovalStatusPending
	ApprovalStatusApproved = jfadk.ApprovalStatusApproved
	ApprovalStatusDenied   = jfadk.ApprovalStatusDenied

	InputRequestStatusPending   = jfadk.InputRequestStatusPending
	InputRequestStatusAnswered  = jfadk.InputRequestStatusAnswered
	InputRequestStatusCancelled = jfadk.InputRequestStatusCancelled

	DefaultProviderRequestTimeout = jfadk.DefaultProviderRequestTimeout
	DefaultRunTimeout             = jfadk.DefaultRunTimeout
	DefaultStreamIdleTimeout      = jfadk.DefaultStreamIdleTimeout
	MaxConcurrentRuns             = jfadk.MaxConcurrentRuns
	DefaultLoopMaxIterations      = jfadk.DefaultLoopMaxIterations
	MaxLoopIterations             = jfadk.MaxLoopIterations
	MaxToolOutputBytes            = jfadk.MaxToolOutputBytes
	MaxMessageLength              = jfadk.MaxMessageLength

	ProviderAPIProtocolChatCompletions = jfadk.ProviderAPIProtocolChatCompletions
	ProviderAPIProtocolResponses       = jfadk.ProviderAPIProtocolResponses

	TimelineKindUserMessage        = jfadk.TimelineKindUserMessage
	TimelineKindAssistantMessage   = jfadk.TimelineKindAssistantMessage
	TimelineKindAssistantReasoning = jfadk.TimelineKindAssistantReasoning
	TimelineKindToolGroup          = jfadk.TimelineKindToolGroup
	TimelineKindApprovalGroup      = jfadk.TimelineKindApprovalGroup
	TimelineKindInputRequest       = jfadk.TimelineKindInputRequest
	TimelineKindContextNotice      = jfadk.TimelineKindContextNotice

	TimelineStatusStreaming = jfadk.TimelineStatusStreaming
	TimelineStatusFinal     = jfadk.TimelineStatusFinal
	TimelineStatusError     = jfadk.TimelineStatusError

	DefaultBuiltinAgentID       = jfadk.DefaultBuiltinAgentID
	GoogleADKModule             = jfadk.GoogleADKModule
	WorkflowManagementSkillName = jfadk.WorkflowManagementSkillName

	ContextStatusUnknown   = jfadk.ContextStatusUnknown
	ContextStatusHealthy   = jfadk.ContextStatusHealthy
	ContextStatusWarning   = jfadk.ContextStatusWarning
	ContextStatusNearLimit = jfadk.ContextStatusNearLimit
	ContextStatusCritical  = jfadk.ContextStatusCritical

	ToolIdempotencyFailClosed = jfadk.ToolIdempotencyFailClosed
	ToolIdempotencyReplaySafe = jfadk.ToolIdempotencyReplaySafe
	ToolIdempotencyKeyed      = jfadk.ToolIdempotencyKeyed
)

type (
	Provider                       = jfadk.Provider // @name adk.Provider
	ProviderWriteRequest           = jfadk.ProviderWriteRequest
	Agent                          = jfadk.Agent                // @name adk.Agent
	AgentWriteRequest              = jfadk.AgentWriteRequest    // @name adk.AgentWriteRequest
	Session                        = jfadk.Session              // @name adk.Session
	SessionComposerState           = jfadk.SessionComposerState // @name adk.SessionComposerState
	SessionComposerStatePatch      = jfadk.SessionComposerStatePatch
	TranscriptEntry                = jfadk.TranscriptEntry   // @name adk.TranscriptEntry
	TimelineEntry                  = jfadk.TimelineEntry     // @name adk.TimelineEntry
	Run                            = jfadk.Run               // @name adk.Run
	WorkflowStepState              = jfadk.WorkflowStepState // @name adk.WorkflowStepState
	RunUsage                       = jfadk.RunUsage          // @name adk.RunUsage
	ToolCall                       = jfadk.ToolCall          // @name adk.ToolCall
	Approval                       = jfadk.Approval          // @name adk.Approval
	InputOption                    = jfadk.InputOption       // @name adk.InputOption
	InputQuestion                  = jfadk.InputQuestion     // @name adk.InputQuestion
	InputAnswer                    = jfadk.InputAnswer       // @name adk.InputAnswer
	InputRequest                   = jfadk.InputRequest      // @name adk.InputRequest
	InputResponseRequest           = jfadk.InputResponseRequest
	InputResolution                = jfadk.InputResolution // @name adk.InputResolution
	Skill                          = jfadk.Skill           // @name adk.Skill
	ToolDescriptor                 = jfadk.ToolDescriptor  // @name adk.ToolDescriptor
	ChatRequest                    = jfadk.ChatRequest
	RunOptions                     = jfadk.RunOptions         // @name adk.RunOptions
	WorkflowDefinition             = jfadk.WorkflowDefinition // @name adk.WorkflowDefinition
	WorkflowDefinitionWriteRequest = jfadk.WorkflowDefinitionWriteRequest
	WorkflowCanvasGraph            = jfadk.WorkflowCanvasGraph // @name adk.WorkflowCanvasGraph
	WorkflowCanvasPoint            = jfadk.WorkflowCanvasPoint // @name adk.WorkflowCanvasPoint
	WorkflowCanvasNode             = jfadk.WorkflowCanvasNode  // @name adk.WorkflowCanvasNode
	WorkflowCanvasEdge             = jfadk.WorkflowCanvasEdge  // @name adk.WorkflowCanvasEdge
	WorkflowTrigger                = jfadk.WorkflowTrigger     // @name adk.WorkflowTrigger
	WorkflowTriggerWriteRequest    = jfadk.WorkflowTriggerWriteRequest
	WorkflowTriggerLog             = jfadk.WorkflowTriggerLog // @name adk.WorkflowTriggerLog
	WorkflowResult                 = jfadk.WorkflowResult     // @name adk.WorkflowResult
	WorkflowNodeRun                = jfadk.WorkflowNodeRun    // @name adk.WorkflowNodeRun
	WorkflowEvent                  = jfadk.WorkflowEvent
	ChatDelta                      = jfadk.ChatDelta
	ChatResponse                   = jfadk.ChatResponse       // @name adk.ChatResponse
	ApprovalResolution             = jfadk.ApprovalResolution // @name adk.ApprovalResolution
	SessionsResponse               = jfadk.SessionsResponse   // @name adk.SessionsResponse
	Snapshot                       = jfadk.Snapshot
	AuditEvent                     = jfadk.AuditEvent // @name adk.AuditEvent
	OptimizationRunRef             = jfadk.OptimizationRunRef
	OptimizationTask               = jfadk.OptimizationTask
	Task                           = jfadk.Task // @name adk.Task
	TaskWriteRequest               = jfadk.TaskWriteRequest
	TaskPatchRequest               = jfadk.TaskPatchRequest
	MemoryEntry                    = jfadk.MemoryEntry // @name adk.MemoryEntry
	MemoryWriteRequest             = jfadk.MemoryWriteRequest
	HandoffSegment                 = jfadk.HandoffSegment
	SessionContextBreakdown        = jfadk.SessionContextBreakdown // @name adk.SessionContextBreakdown
	SessionContextState            = jfadk.SessionContextState
	SessionContextSnapshot         = jfadk.SessionContextSnapshot // @name adk.SessionContextSnapshot

	Runtime                  = jfadk.Runtime
	Store                    = jfadk.Store
	WorkflowExecution        = jfadk.WorkflowExecution
	WorkflowRequest          = jfadkmodel.WorkflowRequest
	WorkflowStep             = jfadk.WorkflowStep
	WorkflowGoalDecision     = jfadk.WorkflowGoalDecision
	AssistantExecutionResult = jfadk.AssistantExecutionResult
	RuntimeLimits            = jfadkmodel.RuntimeLimits
	RuntimeLimitsProvider    = jfadkmodel.RuntimeLimitsProvider
	ToolRegistry             = jfadk.ToolRegistry
	RegisteredTool           = jfadk.RegisteredTool
	ToolFunc                 = jfadk.ToolFunc
	LocalMCPHandler          = jfadk.LocalMCPHandler
	SQLiteSessionService     = enginepersistence.SQLiteSessionService
	ChatRequestConflictError = jfadk.ChatRequestConflictError
	DeletedConfigIDs         = jfadk.DeletedConfigIDs
	WorkflowCanvasRunRequest = jfadk.WorkflowCanvasRunRequest
)

var (
	ErrChatRequestConflict        = jfadk.ErrChatRequestConflict
	ErrBuiltinAgentProtected      = jfadk.ErrBuiltinAgentProtected
	ErrCleanupCandidatesChanged   = jfadk.ErrCleanupCandidatesChanged
	ErrInvalidTaskStatus          = jfadk.ErrInvalidTaskStatus
	ErrInvalidProviderAPIProtocol = jfadk.ErrInvalidProviderAPIProtocol
	ErrProviderInUse              = jfadk.ErrProviderInUse
	LocalMCPReadOnlyToolNames     = jfadk.LocalMCPReadOnlyToolNames
)

func NewRuntime(store *Store, tools *ToolRegistry) *Runtime {
	runtime := jfadk.NewRuntime(store, tools)
	runtime.SetWorkflowExecutor(workflowexec.NewWorkflowExecutor(runtime))
	return runtime
}

func NewRuntimeWithSessionService(store *Store, tools *ToolRegistry, sessionService adksession.Service) *Runtime {
	runtime := jfadk.NewRuntimeWithSessionService(store, tools, sessionService)
	runtime.SetWorkflowExecutor(workflowexec.NewWorkflowExecutor(runtime))
	return runtime
}

func NewStore(dbPath string, secretsPath string, skillsPath string) (*Store, error) {
	return jfadk.NewStore(dbPath, secretsPath, skillsPath)
}

func NewToolRegistry() *ToolRegistry {
	return jfadk.NewToolRegistry()
}

func NewSQLiteSessionService(path string) (*SQLiteSessionService, error) {
	return enginepersistence.NewSQLiteSessionService(path)
}

func CloseSessionService(service adksession.Service) error {
	return enginepersistence.CloseSessionService(service)
}

func NewLocalMCPHandler(runtime *Runtime) (*LocalMCPHandler, error) {
	return jfadk.NewLocalMCPHandler(runtime)
}

func NormalizeRun(run Run) Run {
	return jfadk.NormalizeRun(run)
}

func NormalizeAgent(agent Agent) Agent {
	return jfadk.NormalizeAgent(agent)
}

func NormalizeTimelineEntry(entry TimelineEntry) TimelineEntry {
	return jfadk.NormalizeTimelineEntry(entry)
}

func NormalizeChatResponse(response ChatResponse) ChatResponse {
	return jfadk.NormalizeChatResponse(response)
}

func NormalizeWorkflowDefinition(workflow WorkflowDefinition) WorkflowDefinition {
	return jfadk.NormalizeWorkflowDefinition(workflow)
}

func NormalizeWorkflowTrigger(trigger WorkflowTrigger) WorkflowTrigger {
	return jfadk.NormalizeWorkflowTrigger(trigger)
}

func NormalizeWorkflowTriggerLog(log WorkflowTriggerLog) WorkflowTriggerLog {
	return jfadk.NormalizeWorkflowTriggerLog(log)
}

func NormalizeApprovalResolution(resolution ApprovalResolution) ApprovalResolution {
	return jfadk.NormalizeApprovalResolution(resolution)
}

func NormalizeSessionsResponse(response SessionsResponse) SessionsResponse {
	return jfadk.NormalizeSessionsResponse(response)
}

func NormalizeChatRequestIdentity(req ChatRequest) (ChatRequest, string, error) {
	return jfadk.NormalizeChatRequestIdentity(req)
}

func ToolRequiredSkillNames(descriptor ToolDescriptor) []string {
	return jfadk.ToolRequiredSkillNames(descriptor)
}

func ToolRequiresApproval(descriptor ToolDescriptor, mode string) bool {
	return jfadk.ToolRequiresApproval(descriptor, mode)
}

func ToolDescriptorsForAgent(agent Agent, registry *ToolRegistry) []ToolDescriptor {
	return jfadk.ToolDescriptorsForAgent(agent, registry)
}

func ToolInvocationSessionID(ctx context.Context) (string, bool) {
	return jfadk.ToolInvocationSessionID(ctx)
}

func InputRequestErrorKind(err error) string {
	return jfadk.InputRequestErrorKind(err)
}

func BuiltinAgentTemplates() []AgentWriteRequest {
	return jfadk.BuiltinAgentTemplates()
}

func BuiltinAgentTemplate(id string) (AgentWriteRequest, bool) {
	return jfadk.BuiltinAgentTemplate(id)
}

func IsBuiltinAgentID(id string) bool {
	return jfadk.IsBuiltinAgentID(id)
}

func IsPrimaryBuiltinAgentID(id string) bool {
	return jfadk.IsPrimaryBuiltinAgentID(id)
}

// WorkflowExecutor is the workflow execution implementation injected into the
// engine root by the composition layer.
type WorkflowExecutor = workflowexec.WorkflowExecutor

// NewWorkflowExecutor constructs the workflow execution implementation used by
// the engine-root composition seam.
func NewWorkflowExecutor(runtime jfadkmodel.WorkflowExecutorRuntime) *WorkflowExecutor {
	return workflowexec.NewWorkflowExecutor(runtime)
}

// WorkflowTaskToolset is the ADK toolset exposed by the workflow executor.
type WorkflowTaskToolset = workflowexec.WorkflowTaskToolset

// NewWorkflowTaskToolset constructs a workflow task toolset for an executor.
func NewWorkflowTaskToolset(executor *WorkflowExecutor, parentID string, currentTaskID string) *WorkflowTaskToolset {
	return workflowexec.NewWorkflowTaskToolset(executor, parentID, currentTaskID)
}
