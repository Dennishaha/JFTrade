package workflowexec

import (
	jfadkmodel "github.com/jftrade/jftrade-main/internal/assistant/model"
)

// The executor leaf package owns the workflow execution implementation. It
// aliases the shared business DTOs from internal/assistant/model so the
// package does not depend on the engine root package or the runtime facade.
type (
	Agent                    = jfadkmodel.Agent
	Approval                 = jfadkmodel.Approval
	AssistantExecutionResult = jfadkmodel.AssistantExecutionResult
	ChatDelta                = jfadkmodel.ChatDelta
	ChatResponse             = jfadkmodel.ChatResponse
	InputRequest             = jfadkmodel.InputRequest
	Run                      = jfadkmodel.Run
	RunOptions               = jfadkmodel.RunOptions
	RunUsage                 = jfadkmodel.RunUsage
	Session                  = jfadkmodel.Session
	Snapshot                 = jfadkmodel.Snapshot
	Task                     = jfadkmodel.Task
	TaskPatchRequest         = jfadkmodel.TaskPatchRequest
	TaskWriteRequest         = jfadkmodel.TaskWriteRequest
	ToolCall                 = jfadkmodel.ToolCall
	WorkflowGoalDecision     = jfadkmodel.WorkflowGoalDecision
	WorkflowRequest          = jfadkmodel.WorkflowRequest
	WorkflowStep             = jfadkmodel.WorkflowStep
)

const (
	ApprovalStatusPending = jfadkmodel.ApprovalStatusPending

	RunStatusRunning      = jfadkmodel.RunStatusRunning
	RunStatusCompleted    = jfadkmodel.RunStatusCompleted
	RunStatusPending      = jfadkmodel.RunStatusPending
	RunStatusPendingInput = jfadkmodel.RunStatusPendingInput
	RunStatusPaused       = jfadkmodel.RunStatusPaused
	RunStatusFailed       = jfadkmodel.RunStatusFailed
	RunStatusDenied       = jfadkmodel.RunStatusDenied
	RunStatusCancelled    = jfadkmodel.RunStatusCancelled
	RunStatusTimedOut     = jfadkmodel.RunStatusTimedOut

	WorkModeChat = jfadkmodel.WorkModeChat
	WorkModeLoop = jfadkmodel.WorkModeLoop

	WorkflowEngineADK2Loop = jfadkmodel.WorkflowEngineADK2Loop
)
