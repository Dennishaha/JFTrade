package model

import (
	"context"

	adkmodel "google.golang.org/adk/v2/model"
	adksession "google.golang.org/adk/v2/session"
	adktool "google.golang.org/adk/v2/tool"
	"google.golang.org/genai"
)

// AssistantExecutionResult is the exported assistant turn result shared by the
// engine-root composition seam, provider adapters, and the workflow runtime.
type AssistantExecutionResult struct {
	Reply            string
	ReasoningContent string
	SourceEventID    string
	SyntheticKind    string
}

// WorkflowStore is the persistence surface the workflow executor and execution
// handle use. Root Store implementations satisfy it structurally.
type WorkflowStore interface {
	DeleteTask(ctx context.Context, id string) error
	ListTasksPage(ctx context.Context, status string, agentID string, runID string, limit int, offset int) ([]Task, int, error)
	Run(ctx context.Context, id string) (Run, bool, error)
	SaveRun(ctx context.Context, run Run) error
	SaveRunAndDenyPendingApprovals(ctx context.Context, run Run) error
	SaveTask(ctx context.Context, req TaskWriteRequest) (Task, error)
	Session(ctx context.Context, id string) (Session, bool, error)
	Task(ctx context.Context, id string) (Task, bool, error)
	UpdateTask(ctx context.Context, id string, req TaskPatchRequest) (Task, error)
	ListApprovals(ctx context.Context) ([]Approval, error)
	SaveApprovalIfConfirmationAbsent(ctx context.Context, approval Approval) (Approval, bool, error)
}

// WorkflowExecutionHandle is the opaque execution handle passed between
// engine-root Runtime services and the workflow executor.
type WorkflowExecutionHandle interface {
	Run(ctx context.Context, content *genai.Content) error
	PendingApprovals(ctx context.Context, store WorkflowStore) ([]Approval, error)
	SetInputRequests(requests map[string]*InputRequest)
	RunID() string
	DetachDeltaSink()
	ToolContextForRun(runID string) ToolExecutionContext
	ResultForRun(runID string) AssistantExecutionResult
	WorkflowRunObserved(runID string) bool
	HasToolCallsForRun(runID string) bool
	RunNeedsFinalSynthesis(runID string) bool
	RunHasPostToolText(runID string) bool
	SetRunIDByAgentName(agentName string, runID string)
	HasFinalReplyForRun(runID string, visibleReply string) bool
	SessionService() adksession.Service
	AppName() string
	SessionID() string
	AgentDefinition() Agent
	TrackedRunIDForFunctionCall(callID string) (string, bool)
	MarkCallWaitingForInput(callID string)
	RunGoogleADKWorkflowChildFinalSynthesis(ctx context.Context, agent Agent, session Session, child Run) error
}

// WorkflowExecutorRuntime is the engine-root service surface the workflow
// executor depends on; the root Runtime implements it.
type WorkflowExecutorRuntime interface {
	WorkflowStore() WorkflowStore
	SaveRunPreservingUserGoalPause(ctx context.Context, run Run) (Run, error)
	StartRunWithOptions(ctx context.Context, sessionID string, agent Agent, text string, options RunStartOptions) (Run, context.Context, func(), error)
	ChatResponseForExistingRun(ctx context.Context, run Run) (ChatResponse, error)
	PlanWorkflowWithADK(ctx context.Context, agent Agent, session Session, mode string, message string, objective string, options RunOptions) ([]WorkflowStep, []string, error)
	NewGoogleADKWorkflowExecution(ctx context.Context, agent Agent, session Session, parent Run, childRuns []Run, steps []WorkflowStep, mode string, options RunOptions, onDelta func(ChatDelta) error) (WorkflowExecutionHandle, error)
	MaybeAutoCompactSessionDuringWorkflow(ctx context.Context, session Session, agent Agent, pendingUserText string, onDelta func(ChatDelta) error) error
	PendingInputRequests(ctx context.Context, execution WorkflowExecutionHandle) (map[string]*InputRequest, error)
	ActiveRunExecutionContext(ctx context.Context, runID string) (context.Context, error)
	FinishPendingInputRun(ctx context.Context, session Session, run Run, request *InputRequest) (ChatResponse, error)
	AttachFinalAssistantMessage(ctx context.Context, session Session, run Run, result AssistantExecutionResult) (Run, error)
	CancelUnfinishedWorkflowChildren(ctx context.Context, parent Run)
	ProjectedChatResponse(ctx context.Context, session Session, run Run, result AssistantExecutionResult) ChatResponse
	NewGoogleADKTaskExecution(ctx context.Context, agent Agent, session Session, parent Run, req WorkflowRequest, taskTools adktool.Toolset, onDelta func(ChatDelta) error) (WorkflowExecutionHandle, error)
	TerminateParentWorkflowFromChild(ctx context.Context, parent Run, child Run) (Run, error)
	EnsureAssistantMessage(ctx context.Context, session Session, run Run, result AssistantExecutionResult) (TranscriptEntry, error)
	WorkflowChildAgentForStep(ctx context.Context, agent Agent, step WorkflowStep) (Agent, error)
	GoogleADKModelForAgent(ctx context.Context, definition Agent) (adkmodel.LLM, error)
	CompleteChatRun(ctx context.Context, session Session, run Run, text string, toolContext ToolExecutionContext, approvals []Approval, result AssistantExecutionResult, adkErr error) (ChatResponse, error)
	ExecuteGoogleADK(ctx context.Context, agent Agent, session Session, runID string, text string, onDelta func(ChatDelta) error) (ToolExecutionContext, []Approval, AssistantExecutionResult, string, string, error)
	PersistRunTerminalState(ctx context.Context, run Run) error
	AuthoritativeRunSnapshot(ctx context.Context, run Run) Run
	RegisterWorkflowExecution(parentID string, childRunIDs []string, execution WorkflowExecutionHandle)
	WithWorkflowChildLock(ctx context.Context, fn func() error) error
	RunExecutionInFlight(runID string) bool
	ModelsListTool(ctx context.Context, input map[string]any) (any, error)
}

// WorkflowExecution is the injectable workflow execution contract.
type WorkflowExecution interface {
	Run(ctx context.Context, req WorkflowRequest) (ChatResponse, error)
	FailParent(ctx context.Context, parent Run, cause error) (Run, error)
	ResumeLoopWorkflow(ctx context.Context, session Session, parent Run) (Run, error)
	ReconcileWorkflowChildren(ctx context.Context, parent Run) (Run, bool, error)
	CompleteResumedWorkflow(ctx context.Context, session Session, parent Run, reply string) (Run, error)
	ResumeADKGoalWorkflow(ctx context.Context, session Session, agent Agent, parent Run) (Run, error)
	WorkflowTasks(ctx context.Context, parent Run, known []Task) ([]Task, error)
	PersistWorkflowTasks(ctx context.Context, parent Run, agent Agent, steps []WorkflowStep) ([]Task, error)
	RunPlannedGoogleADKWorkflow(ctx context.Context, req WorkflowRequest, parent Run, steps []WorkflowStep, tasks []Task) (ChatResponse, error)
	WorkflowResponse(ctx context.Context, session Session, parent Run, replyResult AssistantExecutionResult) ChatResponse
}
