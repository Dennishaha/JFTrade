package adk

import (
	jfadkmodel "github.com/jftrade/jftrade-main/internal/assistant/model"
)

const (
	PermissionModeApproval     = jfadkmodel.PermissionModeApproval
	PermissionModeLessApproval = jfadkmodel.PermissionModeLessApproval
	PermissionModeAll          = jfadkmodel.PermissionModeAll

	WorkModeChat = jfadkmodel.WorkModeChat
	WorkModeLoop = jfadkmodel.WorkModeLoop

	WorkflowEngineADK2Loop   = jfadkmodel.WorkflowEngineADK2Loop
	WorkflowEngineADK2Canvas = jfadkmodel.WorkflowEngineADK2Canvas

	AgentStatusEnabled  = jfadkmodel.AgentStatusEnabled
	AgentStatusDisabled = jfadkmodel.AgentStatusDisabled

	WorkflowStatusEnabled  = jfadkmodel.WorkflowStatusEnabled
	WorkflowStatusDisabled = jfadkmodel.WorkflowStatusDisabled
	WorkflowStatusRunning  = jfadkmodel.WorkflowStatusRunning

	WorkflowTriggerTypeManual          = jfadkmodel.WorkflowTriggerTypeManual
	WorkflowTriggerTypeSchedule        = jfadkmodel.WorkflowTriggerTypeSchedule
	WorkflowTriggerTypeWebhook         = jfadkmodel.WorkflowTriggerTypeWebhook
	WorkflowTriggerTypeEvent           = jfadkmodel.WorkflowTriggerTypeEvent
	WorkflowTriggerTypeMarketThreshold = jfadkmodel.WorkflowTriggerTypeMarketThreshold

	WorkflowTriggerStatusEnabled  = jfadkmodel.WorkflowTriggerStatusEnabled
	WorkflowTriggerStatusDisabled = jfadkmodel.WorkflowTriggerStatusDisabled
	WorkflowTriggerStatusError    = jfadkmodel.WorkflowTriggerStatusError

	WorkflowTriggerLogStatusQueued          = jfadkmodel.WorkflowTriggerLogStatusQueued
	WorkflowTriggerLogStatusRunning         = jfadkmodel.WorkflowTriggerLogStatusRunning
	WorkflowTriggerLogStatusSucceeded       = jfadkmodel.WorkflowTriggerLogStatusSucceeded
	WorkflowTriggerLogStatusPendingApproval = jfadkmodel.WorkflowTriggerLogStatusPendingApproval
	WorkflowTriggerLogStatusFailed          = jfadkmodel.WorkflowTriggerLogStatusFailed
	WorkflowTriggerLogStatusCancelled       = jfadkmodel.WorkflowTriggerLogStatusCancelled
	WorkflowTriggerLogStatusSkipped         = jfadkmodel.WorkflowTriggerLogStatusSkipped

	RunStatusRunning      = jfadkmodel.RunStatusRunning
	RunStatusCompleted    = jfadkmodel.RunStatusCompleted
	RunStatusPending      = jfadkmodel.RunStatusPending
	RunStatusPendingInput = jfadkmodel.RunStatusPendingInput
	RunStatusFailed       = jfadkmodel.RunStatusFailed
	RunStatusDenied       = jfadkmodel.RunStatusDenied
	RunStatusCancelled    = jfadkmodel.RunStatusCancelled
	RunStatusTimedOut     = jfadkmodel.RunStatusTimedOut
	RunStatusPaused       = jfadkmodel.RunStatusPaused

	ApprovalStatusPending  = jfadkmodel.ApprovalStatusPending
	ApprovalStatusApproved = jfadkmodel.ApprovalStatusApproved
	ApprovalStatusDenied   = jfadkmodel.ApprovalStatusDenied

	InputRequestStatusPending   = jfadkmodel.InputRequestStatusPending
	InputRequestStatusAnswered  = jfadkmodel.InputRequestStatusAnswered
	InputRequestStatusCancelled = jfadkmodel.InputRequestStatusCancelled

	DefaultProviderRequestTimeout = jfadkmodel.DefaultProviderRequestTimeout
	DefaultRunTimeout             = jfadkmodel.DefaultRunTimeout
	DefaultStreamIdleTimeout      = jfadkmodel.DefaultStreamIdleTimeout
	MaxConcurrentRuns             = jfadkmodel.MaxConcurrentRuns
	DefaultLoopMaxIterations      = jfadkmodel.DefaultLoopMaxIterations
	MaxLoopIterations             = jfadkmodel.MaxLoopIterations
	MaxToolOutputBytes            = jfadkmodel.MaxToolOutputBytes
	MaxMessageLength              = jfadkmodel.MaxMessageLength

	ProviderTestModeQuick = jfadkmodel.ProviderTestModeQuick
	ProviderTestModeFull  = jfadkmodel.ProviderTestModeFull

	TimelineKindUserMessage        = jfadkmodel.TimelineKindUserMessage
	TimelineKindAssistantMessage   = jfadkmodel.TimelineKindAssistantMessage
	TimelineKindAssistantReasoning = jfadkmodel.TimelineKindAssistantReasoning
	TimelineKindToolGroup          = jfadkmodel.TimelineKindToolGroup
	TimelineKindApprovalGroup      = jfadkmodel.TimelineKindApprovalGroup
	TimelineKindInputRequest       = jfadkmodel.TimelineKindInputRequest
	TimelineKindContextNotice      = jfadkmodel.TimelineKindContextNotice

	TimelineStatusStreaming = jfadkmodel.TimelineStatusStreaming
	TimelineStatusFinal     = jfadkmodel.TimelineStatusFinal
	TimelineStatusError     = jfadkmodel.TimelineStatusError
)

type (
	ReasoningEffort                = jfadkmodel.ReasoningEffort
	ProviderReasoningMapping       = jfadkmodel.ProviderReasoningMapping
	ProviderReasoningConfig        = jfadkmodel.ProviderReasoningConfig
	ProviderReasoningTestResult    = jfadkmodel.ProviderReasoningTestResult
	ProviderReasoningTestResponse  = jfadkmodel.ProviderReasoningTestResponse
	ProviderTestMode               = jfadkmodel.ProviderTestMode
	ProviderTestResponse           = jfadkmodel.ProviderTestResponse
	Provider                       = jfadkmodel.Provider
	ProviderWriteRequest           = jfadkmodel.ProviderWriteRequest
	Agent                          = jfadkmodel.Agent
	AgentWriteRequest              = jfadkmodel.AgentWriteRequest
	Session                        = jfadkmodel.Session
	SessionComposerState           = jfadkmodel.SessionComposerState
	SessionComposerStatePatch      = jfadkmodel.SessionComposerStatePatch
	TranscriptEntry                = jfadkmodel.TranscriptEntry
	TimelineEntry                  = jfadkmodel.TimelineEntry
	SessionProjection              = jfadkmodel.SessionProjection
	Run                            = jfadkmodel.Run
	WorkflowStepState              = jfadkmodel.WorkflowStepState
	RunUsage                       = jfadkmodel.RunUsage
	ToolCall                       = jfadkmodel.ToolCall
	Approval                       = jfadkmodel.Approval
	InputOption                    = jfadkmodel.InputOption
	InputQuestion                  = jfadkmodel.InputQuestion
	InputAnswer                    = jfadkmodel.InputAnswer
	InputRequest                   = jfadkmodel.InputRequest
	InputResponseRequest           = jfadkmodel.InputResponseRequest
	InputResolution                = jfadkmodel.InputResolution
	Skill                          = jfadkmodel.Skill
	ToolDescriptor                 = jfadkmodel.ToolDescriptor
	ChatRequest                    = jfadkmodel.ChatRequest
	RunOptions                     = jfadkmodel.RunOptions
	WorkflowDefinition             = jfadkmodel.WorkflowDefinition
	WorkflowDefinitionWriteRequest = jfadkmodel.WorkflowDefinitionWriteRequest
	WorkflowCanvasGraph            = jfadkmodel.WorkflowCanvasGraph
	WorkflowCanvasPoint            = jfadkmodel.WorkflowCanvasPoint
	WorkflowCanvasNode             = jfadkmodel.WorkflowCanvasNode
	WorkflowCanvasEdge             = jfadkmodel.WorkflowCanvasEdge
	WorkflowTrigger                = jfadkmodel.WorkflowTrigger
	WorkflowTriggerWriteRequest    = jfadkmodel.WorkflowTriggerWriteRequest
	WorkflowTriggerLog             = jfadkmodel.WorkflowTriggerLog
	WorkflowResult                 = jfadkmodel.WorkflowResult
	WorkflowNodeRun                = jfadkmodel.WorkflowNodeRun
	WorkflowEvent                  = jfadkmodel.WorkflowEvent
	ChatDelta                      = jfadkmodel.ChatDelta
	ChatResponse                   = jfadkmodel.ChatResponse
	ApprovalResolution             = jfadkmodel.ApprovalResolution
	SessionsResponse               = jfadkmodel.SessionsResponse
	Snapshot                       = jfadkmodel.Snapshot
	AuditEvent                     = jfadkmodel.AuditEvent
	OptimizationRunRef             = jfadkmodel.OptimizationRunRef
	OptimizationTask               = jfadkmodel.OptimizationTask
	Task                           = jfadkmodel.Task
	TaskWriteRequest               = jfadkmodel.TaskWriteRequest
	TaskPatchRequest               = jfadkmodel.TaskPatchRequest
	MemoryEntry                    = jfadkmodel.MemoryEntry
	MemoryWriteRequest             = jfadkmodel.MemoryWriteRequest
	HandoffSegment                 = jfadkmodel.HandoffSegment
	SessionContextBreakdown        = jfadkmodel.SessionContextBreakdown
	SessionContextState            = jfadkmodel.SessionContextState
	SessionContextSnapshot         = jfadkmodel.SessionContextSnapshot
)

var (
	ErrInvalidProviderReasoning     = jfadkmodel.ErrInvalidProviderReasoning
	ErrProviderReasoningUnsupported = jfadkmodel.ErrProviderReasoningUnsupported
)

const transcriptKindMessage = "message"
