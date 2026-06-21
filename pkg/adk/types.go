package adk

import "time"

const (
	PermissionModeApproval     = "approval"
	PermissionModeLessApproval = "less_approval"
	PermissionModeAll          = "all"

	WorkModeChat = "chat"
	WorkModeTask = "task"
	WorkModeLoop = "loop"

	AgentStatusEnabled  = "ENABLED"
	AgentStatusDisabled = "DISABLED"

	RunStatusRunning   = "RUNNING"
	RunStatusCompleted = "COMPLETED"
	RunStatusPending   = "PENDING_APPROVAL"
	RunStatusFailed    = "FAILED"
	RunStatusDenied    = "DENIED"
	RunStatusCancelled = "CANCELLED"
	RunStatusTimedOut  = "TIMED_OUT"
	RunStatusPaused    = "PAUSED"

	ApprovalStatusPending  = "PENDING"
	ApprovalStatusApproved = "APPROVED"
	ApprovalStatusDenied   = "DENIED"

	// Runtime safety limits
	DefaultProviderRequestTimeout = 180 * time.Second
	DefaultRunTimeout             = 600 * time.Second
	DefaultStreamIdleTimeout      = 300 * time.Second
	MaxConcurrentRuns             = 10 // Maximum simultaneous runs
	DefaultLoopMaxIterations      = 5
	MaxLoopIterations             = 20
	MaxToolOutputBytes            = 256 << 10 // Maximum tool output size (256 KiB)
	MaxMessageLength              = 50000     // Maximum user message length in runes
)

type Provider struct {
	ID                  string            `json:"id"`
	DisplayName         string            `json:"displayName"`
	BaseURL             string            `json:"baseUrl"`
	Model               string            `json:"model"`
	ContextWindowTokens int               `json:"contextWindowTokens,omitempty"`
	RequestTimeoutMs    int               `json:"requestTimeoutMs"`
	DefaultHeaders      map[string]string `json:"defaultHeaders,omitempty"`
	Enabled             bool              `json:"enabled"`
	HasAPIKey           bool              `json:"hasApiKey"`
	Capabilities        map[string]bool   `json:"capabilities,omitempty"`
	CreatedAt           string            `json:"createdAt"`
	UpdatedAt           string            `json:"updatedAt"`
}

type ProviderWriteRequest struct {
	ID                  string            `json:"id,omitempty"`
	DisplayName         string            `json:"displayName"`
	BaseURL             string            `json:"baseUrl"`
	Model               string            `json:"model"`
	ContextWindowTokens int               `json:"contextWindowTokens,omitempty"`
	RequestTimeoutMs    int               `json:"requestTimeoutMs,omitempty"`
	DefaultHeaders      map[string]string `json:"defaultHeaders,omitempty"`
	APIKey              string            `json:"apiKey,omitempty"`
	Enabled             bool              `json:"enabled"`
}

type RuntimeLimits struct {
	RunTimeout time.Duration
}

type RuntimeLimitsProvider func() RuntimeLimits

type Agent struct {
	ID                string   `json:"id"`
	Name              string   `json:"name"`
	Instruction       string   `json:"instruction"`
	ProviderID        string   `json:"providerId"`
	Model             string   `json:"model"`
	Tools             []string `json:"tools"`
	Skills            []string `json:"skills"`
	PermissionMode    string   `json:"permissionMode"`
	MemoryEnabled     bool     `json:"memoryEnabled"`
	RecentUserWindow  int      `json:"recentUserWindow"`
	WorkMode          string   `json:"workMode"`
	LoopMaxIterations int      `json:"loopMaxIterations"`
	Status            string   `json:"status"`
	CreatedAt         string   `json:"createdAt"`
	UpdatedAt         string   `json:"updatedAt"`
	DeletedAt         *string  `json:"deletedAt,omitempty"`
}

type AgentWriteRequest struct {
	ID                string   `json:"id,omitempty"`
	Name              string   `json:"name"`
	Instruction       string   `json:"instruction"`
	ProviderID        string   `json:"providerId"`
	Model             string   `json:"model,omitempty"`
	Tools             []string `json:"tools,omitempty"`
	Skills            []string `json:"skills,omitempty"`
	PermissionMode    string   `json:"permissionMode"`
	MemoryEnabled     bool     `json:"memoryEnabled"`
	RecentUserWindow  int      `json:"recentUserWindow,omitempty"`
	WorkMode          string   `json:"workMode,omitempty"`
	LoopMaxIterations int      `json:"loopMaxIterations,omitempty"`
	Status            string   `json:"status"`
}

type Session struct {
	ID        string `json:"id"`
	AgentID   string `json:"agentId"`
	Title     string `json:"title"`
	CreatedAt string `json:"createdAt"`
	UpdatedAt string `json:"updatedAt"`
}

type SessionComposerState struct {
	SessionID              string `json:"sessionId"`
	ChatDraft              string `json:"chatDraft"`
	WorkModeOverride       string `json:"workModeOverride"`
	PermissionModeOverride string `json:"permissionModeOverride"`
	GoalObjectiveDraft     string `json:"goalObjectiveDraft"`
	GoalObjectiveTouched   bool   `json:"goalObjectiveTouched"`
	UpdatedAt              string `json:"updatedAt"`
}

type SessionComposerStatePatch struct {
	ChatDraft              *string `json:"chatDraft,omitempty"`
	WorkModeOverride       *string `json:"workModeOverride,omitempty"`
	PermissionModeOverride *string `json:"permissionModeOverride,omitempty"`
	GoalObjectiveDraft     *string `json:"goalObjectiveDraft,omitempty"`
	GoalObjectiveTouched   *bool   `json:"goalObjectiveTouched,omitempty"`
}

const transcriptKindMessage = "message"

const (
	TimelineKindUserMessage        = "user_message"
	TimelineKindAssistantMessage   = "assistant_message"
	TimelineKindAssistantReasoning = "assistant_reasoning"
	TimelineKindToolGroup          = "tool_group"
	TimelineKindApprovalGroup      = "approval_group"
	TimelineKindContextNotice      = "context_notice"

	TimelineStatusStreaming = "streaming"
	TimelineStatusFinal     = "final"
	TimelineStatusError     = "error"
)

type TranscriptEntry struct {
	ID               string `json:"id"`
	SessionID        string `json:"sessionId"`
	RunID            string `json:"runId,omitempty"`
	Role             string `json:"role"`
	Kind             string `json:"kind"`
	Content          string `json:"content"`
	ReasoningContent string `json:"reasoningContent,omitempty"`
	CreatedAt        string `json:"createdAt"`
}

type Message = TranscriptEntry

type TimelineEntry struct {
	ID            string     `json:"id"`
	SessionID     string     `json:"sessionId"`
	RunID         string     `json:"runId,omitempty"`
	Kind          string     `json:"kind"`
	CreatedAt     string     `json:"createdAt"`
	UpdatedAt     string     `json:"updatedAt,omitempty"`
	Sequence      int        `json:"sequence"`
	Status        string     `json:"status,omitempty"`
	Text          string     `json:"text,omitempty"`
	OriginalText  string     `json:"originalText,omitempty"`
	ProcessedText string     `json:"processedText,omitempty"`
	ToolCalls     []ToolCall `json:"toolCalls,omitempty"`
	Approvals     []Approval `json:"approvals,omitempty"`
}

type Run struct {
	ID                 string              `json:"id"`
	SessionID          string              `json:"sessionId"`
	AgentID            string              `json:"agentId"`
	ProviderID         string              `json:"providerId,omitempty"`
	ProviderName       string              `json:"providerName,omitempty"`
	Model              string              `json:"model,omitempty"`
	MaxDurationMs      int64               `json:"maxDurationMs"`
	Status             string              `json:"status"`
	Message            string              `json:"message"`
	UserMessage        string              `json:"userMessage,omitempty"`
	PreToolContent     string              `json:"preToolContent,omitempty"`
	PreToolReasoning   string              `json:"preToolReasoning,omitempty"`
	ToolSummaries      []string            `json:"toolSummaries,omitempty"`
	FailureReason      string              `json:"failureReason,omitempty"`
	ErrorCode          string              `json:"errorCode,omitempty"`
	Degraded           bool                `json:"degraded,omitempty"`
	OptimizationTaskID string              `json:"optimizationTaskId,omitempty"`
	WorkMode           string              `json:"workMode,omitempty"`
	PermissionMode     string              `json:"permissionMode,omitempty"`
	Objective          string              `json:"objective,omitempty"`
	ParentRunID        string              `json:"parentRunId,omitempty"`
	ChildRunIDs        []string            `json:"childRunIds,omitempty"`
	Iteration          int                 `json:"iteration,omitempty"`
	WorkflowStatus     string              `json:"workflowStatus,omitempty"`
	WorkflowCursor     int                 `json:"workflowCursor,omitempty"`
	WorkflowPlan       []WorkflowStepState `json:"workflowPlan,omitempty"`
	ToolCalls          []ToolCall          `json:"toolCalls"`
	PendingApprovals   []Approval          `json:"pendingApprovals"`
	ResumeState        string              `json:"resumeState,omitempty"`
	PauseRequestedAt   *string             `json:"pauseRequestedAt,omitempty"`
	PausedAt           *string             `json:"pausedAt,omitempty"`
	PausedReason       string              `json:"pausedReason,omitempty"`
	FinalMessageID     string              `json:"finalMessageId,omitempty"`
	Usage              *RunUsage           `json:"usage,omitempty"`
	CreatedAt          string              `json:"createdAt"`
	StartedAt          string              `json:"startedAt,omitempty"`
	UpdatedAt          string              `json:"updatedAt"`
	CompletedAt        *string             `json:"completedAt,omitempty"`
	CancelledAt        *string             `json:"cancelledAt,omitempty"`
}

type WorkflowStepState struct {
	TaskID          string   `json:"taskId,omitempty"`
	Title           string   `json:"title"`
	Description     string   `json:"description,omitempty"`
	Message         string   `json:"message,omitempty"`
	Status          string   `json:"status"`
	ChildRunID      string   `json:"childRunId,omitempty"`
	DependsOn       []string `json:"dependsOn,omitempty"`
	Iteration       int      `json:"iteration,omitempty"`
	Order           int      `json:"order,omitempty"`
	ModeHint        string   `json:"modeHint,omitempty"`
	AgentRole       string   `json:"agentRole,omitempty"`
	PlannerStepID   string   `json:"plannerStepId,omitempty"`
	PlanSource      string   `json:"planSource,omitempty"`
	WorkflowMode    string   `json:"workflowMode,omitempty"`
	Objective       string   `json:"objective,omitempty"`
	Executor        string   `json:"executor,omitempty"`
	ResultSummary   string   `json:"resultSummary,omitempty"`
	PlannerWarnings []string `json:"plannerWarnings,omitempty"`
}

type RunUsage struct {
	ModelCalls     int   `json:"modelCalls"`
	ToolCallsTotal int   `json:"toolCallsTotal"`
	DurationMs     int64 `json:"durationMs,omitempty"`
	TokensIn       int   `json:"tokensIn,omitempty"`
	TokensOut      int   `json:"tokensOut,omitempty"`
}

type ToolCall struct {
	ID             string         `json:"id"`
	RunID          string         `json:"runId"`
	ToolName       string         `json:"toolName"`
	Permission     string         `json:"permission"`
	Status         string         `json:"status"`
	Input          map[string]any `json:"input,omitempty"`
	Output         any            `json:"output,omitempty"`
	Error          *string        `json:"error,omitempty"`
	RequiresUser   bool           `json:"requiresUser"`
	IdempotencyKey string         `json:"idempotencyKey,omitempty"`
	CreatedAt      string         `json:"createdAt"`
	StartedAt      string         `json:"startedAt,omitempty"`
	UpdatedAt      string         `json:"updatedAt"`
	CompletedAt    *string        `json:"completedAt,omitempty"`
	DurationMs     int64          `json:"durationMs,omitempty"`
}

type Approval struct {
	ID                 string         `json:"id"`
	RunID              string         `json:"runId"`
	AgentID            string         `json:"agentId"`
	ToolName           string         `json:"toolName"`
	Input              map[string]any `json:"input,omitempty"`
	Status             string         `json:"status"`
	Reason             string         `json:"reason"`
	FunctionCallID     string         `json:"functionCallId,omitempty"`
	ConfirmationCallID string         `json:"confirmationCallId,omitempty"`
	CreatedAt          string         `json:"createdAt"`
	UpdatedAt          string         `json:"updatedAt"`
}

type Skill struct {
	ID               string   `json:"id"`
	DisplayName      string   `json:"displayName"`
	Description      string   `json:"description"`
	Source           string   `json:"source"`
	InstallPath      string   `json:"installPath"`
	Enabled          bool     `json:"enabled"`
	Builtin          bool     `json:"builtin"`
	Tools            []string `json:"tools"`
	Version          string   `json:"version,omitempty"`
	ContentHash      string   `json:"contentHash,omitempty"`
	ValidationStatus string   `json:"validationStatus,omitempty"`
	ValidationError  string   `json:"validationError,omitempty"`
	CreatedAt        string   `json:"createdAt"`
	UpdatedAt        string   `json:"updatedAt"`
}

type ToolDescriptor struct {
	Name               string         `json:"name"`
	DisplayName        string         `json:"displayName"`
	Description        string         `json:"description"`
	Category           string         `json:"category"`
	Permission         string         `json:"permission"`
	AllowedModes       []string       `json:"allowedModes"`
	RequiresApprovalIn []string       `json:"requiresApprovalIn"`
	InputSchema        map[string]any `json:"inputSchema,omitempty"`
	OutputSummary      string         `json:"outputSummary,omitempty"`
	RiskLevel          string         `json:"riskLevel,omitempty"`
}

type ChatRequest struct {
	AgentID                string      `json:"agentId,omitempty"`
	SessionID              string      `json:"sessionId,omitempty"`
	Message                string      `json:"message"`
	WorkModeOverride       string      `json:"workModeOverride,omitempty"`
	PermissionModeOverride string      `json:"permissionModeOverride,omitempty"`
	Objective              string      `json:"objective,omitempty"`
	RunOptions             *RunOptions `json:"runOptions,omitempty"`
}

type RunOptions struct {
	LoopMaxIterations int `json:"loopMaxIterations,omitempty"`
}

type ChatDelta struct {
	Reply            string                  `json:"reply,omitempty"`
	ReasoningContent string                  `json:"reasoningContent,omitempty"`
	ToolProgress     string                  `json:"toolProgress,omitempty"`
	Run              *Run                    `json:"run,omitempty"`
	Context          *SessionContextSnapshot `json:"context,omitempty"`
	Timeline         *TimelineEntry          `json:"timeline,omitempty"`
}

type ChatResponse struct {
	Reply            string                  `json:"reply"`
	ReasoningContent string                  `json:"reasoningContent,omitempty"`
	Session          Session                 `json:"session"`
	Run              Run                     `json:"run"`
	PendingApprovals []Approval              `json:"pendingApprovals"`
	Timeline         []TimelineEntry         `json:"timeline"`
	Context          *SessionContextSnapshot `json:"context,omitempty"`
}

type ApprovalResolution struct {
	Approval  Approval         `json:"approval"`
	Run       *Run             `json:"run,omitempty"`
	ParentRun *Run             `json:"parentRun,omitempty"`
	Message   *TranscriptEntry `json:"message,omitempty"`
}

type SessionsResponse struct {
	Session       Session              `json:"session"`
	Timeline      []TimelineEntry      `json:"timeline"`
	Runs          []Run                `json:"runs,omitempty"`
	ComposerState SessionComposerState `json:"composerState"`
}

type Snapshot struct {
	Providers []Provider       `json:"providers"`
	Agents    []Agent          `json:"agents"`
	Skills    []Skill          `json:"skills"`
	Tools     []ToolDescriptor `json:"tools"`
}

type AuditEvent struct {
	ID        string         `json:"id"`
	Kind      string         `json:"kind"`
	SubjectID string         `json:"subjectId,omitempty"`
	Detail    string         `json:"detail"`
	Metadata  map[string]any `json:"metadata,omitempty"`
	CreatedAt string         `json:"createdAt"`
}

type OptimizationRunRef struct {
	DefinitionID string `json:"definitionId"`
	RunID        string `json:"runId"`
}

type OptimizationTask struct {
	ID        string               `json:"id"`
	Status    string               `json:"status"`
	Objective string               `json:"objective"`
	Runs      []OptimizationRunRef `json:"runs"`
	CreatedAt string               `json:"createdAt"`
	UpdatedAt string               `json:"updatedAt"`
}

type Task struct {
	ID              string   `json:"id"`
	Title           string   `json:"title"`
	Description     string   `json:"description,omitempty"`
	Status          string   `json:"status"`
	AgentID         string   `json:"agentId,omitempty"`
	RunID           string   `json:"runId,omitempty"`
	DependsOn       []string `json:"dependsOn,omitempty"`
	Order           int      `json:"order,omitempty"`
	ModeHint        string   `json:"modeHint,omitempty"`
	AgentRole       string   `json:"agentRole,omitempty"`
	PlannerStepID   string   `json:"plannerStepId,omitempty"`
	PlanSource      string   `json:"planSource,omitempty"`
	WorkflowMode    string   `json:"workflowMode,omitempty"`
	Objective       string   `json:"objective,omitempty"`
	Message         string   `json:"message,omitempty"`
	Executor        string   `json:"executor,omitempty"`
	ResultSummary   string   `json:"resultSummary,omitempty"`
	PlannerWarnings []string `json:"plannerWarnings,omitempty"`
	CreatedAt       string   `json:"createdAt"`
	UpdatedAt       string   `json:"updatedAt"`
}

type TaskWriteRequest struct {
	ID              string   `json:"id,omitempty"`
	Title           string   `json:"title"`
	Description     string   `json:"description,omitempty"`
	Status          string   `json:"status,omitempty"`
	AgentID         string   `json:"agentId,omitempty"`
	RunID           string   `json:"runId,omitempty"`
	DependsOn       []string `json:"dependsOn,omitempty"`
	Order           int      `json:"order,omitempty"`
	ModeHint        string   `json:"modeHint,omitempty"`
	AgentRole       string   `json:"agentRole,omitempty"`
	PlannerStepID   string   `json:"plannerStepId,omitempty"`
	PlanSource      string   `json:"planSource,omitempty"`
	WorkflowMode    string   `json:"workflowMode,omitempty"`
	Objective       string   `json:"objective,omitempty"`
	Message         string   `json:"message,omitempty"`
	Executor        string   `json:"executor,omitempty"`
	ResultSummary   string   `json:"resultSummary,omitempty"`
	PlannerWarnings []string `json:"plannerWarnings,omitempty"`
}

type TaskPatchRequest struct {
	Title           *string  `json:"title,omitempty"`
	Description     *string  `json:"description,omitempty"`
	Status          *string  `json:"status,omitempty"`
	AgentID         *string  `json:"agentId,omitempty"`
	RunID           *string  `json:"runId,omitempty"`
	DependsOn       []string `json:"dependsOn,omitempty"`
	Order           *int     `json:"order,omitempty"`
	ModeHint        *string  `json:"modeHint,omitempty"`
	AgentRole       *string  `json:"agentRole,omitempty"`
	PlannerStepID   *string  `json:"plannerStepId,omitempty"`
	PlanSource      *string  `json:"planSource,omitempty"`
	WorkflowMode    *string  `json:"workflowMode,omitempty"`
	Objective       *string  `json:"objective,omitempty"`
	Message         *string  `json:"message,omitempty"`
	Executor        *string  `json:"executor,omitempty"`
	ResultSummary   *string  `json:"resultSummary,omitempty"`
	PlannerWarnings []string `json:"plannerWarnings,omitempty"`
}

type MemoryEntry struct {
	ID        string `json:"id"`
	AgentID   string `json:"agentId,omitempty"`
	Key       string `json:"key"`
	Value     string `json:"value"`
	Scope     string `json:"scope"`
	CreatedAt string `json:"createdAt"`
	UpdatedAt string `json:"updatedAt"`
}

type MemoryWriteRequest struct {
	AgentID string `json:"agentId,omitempty"`
	Key     string `json:"key"`
	Value   string `json:"value"`
	Scope   string `json:"scope,omitempty"`
}

type HandoffSegment struct {
	ID                string `json:"id"`
	SessionID         string `json:"sessionId"`
	ContextRevisionID string `json:"contextRevisionId,omitempty"`
	Sequence          int    `json:"sequence"`
	StartEventIndex   int    `json:"startEventIndex"`
	EndEventIndex     int    `json:"endEventIndex"`
	Summary           string `json:"summary"`
	Mode              string `json:"mode"`
	Reason            string `json:"reason,omitempty"`
	EstimatedTokens   int    `json:"estimatedTokens"`
	Active            bool   `json:"active"`
	SupersededBy      string `json:"supersededBy,omitempty"`
	CreatedAt         string `json:"createdAt"`
	UpdatedAt         string `json:"updatedAt"`
}

type SessionContextBreakdown struct {
	InstructionTokens     int `json:"instructionTokens"`
	HandoffTokens         int `json:"handoffTokens"`
	RecentUserTokens      int `json:"recentUserTokens"`
	ProtectedTailTokens   int `json:"protectedTailTokens"`
	OtherVisibleTokens    int `json:"otherVisibleTokens"`
	PendingUserTokens     int `json:"pendingUserTokens"`
	ToolDeclarationTokens int `json:"toolDeclarationTokens"`
}

type SessionContextState struct {
	SessionID                 string                  `json:"sessionId"`
	ContextRevisionID         string                  `json:"contextRevisionId,omitempty"`
	PreviousContextRevisionID string                  `json:"previousContextRevisionId,omitempty"`
	ContextRevisionCreatedAt  string                  `json:"contextRevisionCreatedAt,omitempty"`
	RecentUserWindow          int                     `json:"recentUserWindow"`
	RetainedRecentUserCount   int                     `json:"retainedRecentUserCount"`
	ActiveHandoffCount        int                     `json:"activeHandoffCount"`
	CurrentInputTokens        int                     `json:"currentInputTokens"`
	ProjectedNextTurnTokens   int                     `json:"projectedNextTurnTokens"`
	ContextWindowTokens       int                     `json:"contextWindowTokens"`
	UsageRatio                float64                 `json:"usageRatio"`
	LatestHandoffPreview      string                  `json:"latestHandoffPreview,omitempty"`
	Breakdown                 SessionContextBreakdown `json:"breakdown"`
	LastCompactedAt           string                  `json:"lastCompactedAt,omitempty"`
	LastCompactionMode        string                  `json:"lastCompactionMode,omitempty"`
	LastCompactionReason      string                  `json:"lastCompactionReason,omitempty"`
	AutoCompacted             bool                    `json:"autoCompacted,omitempty"`
	DegradedSummary           bool                    `json:"degradedSummary,omitempty"`
	CreatedAt                 string                  `json:"createdAt"`
	UpdatedAt                 string                  `json:"updatedAt"`
}

type SessionContextSnapshot struct {
	SessionID                  string                  `json:"sessionId"`
	ContextRevisionID          string                  `json:"contextRevisionId,omitempty"`
	PreviousContextRevisionID  string                  `json:"previousContextRevisionId,omitempty"`
	ContextRevisionCreatedAt   string                  `json:"contextRevisionCreatedAt,omitempty"`
	CurrentInputTokens         int                     `json:"currentInputTokens"`
	ProjectedNextTurnTokens    int                     `json:"projectedNextTurnTokens"`
	EstimatedInputTokens       int                     `json:"estimatedInputTokens,omitempty"`
	RawCurrentInputTokens      int                     `json:"rawCurrentInputTokens,omitempty"`
	RawProjectedNextTurnTokens int                     `json:"rawProjectedNextTurnTokens,omitempty"`
	ContextWindowTokens        int                     `json:"contextWindowTokens"`
	UsageRatio                 float64                 `json:"usageRatio"`
	Status                     string                  `json:"status"`
	RecentUserWindow           int                     `json:"recentUserWindow"`
	RetainedRecentUserCount    int                     `json:"retainedRecentUserCount"`
	ProtectedRecentCount       int                     `json:"protectedRecentCount,omitempty"`
	ActiveHandoffCount         int                     `json:"activeHandoffCount"`
	LatestHandoffPreview       string                  `json:"latestHandoffPreview,omitempty"`
	SummaryPreview             string                  `json:"summaryPreview,omitempty"`
	RawEventCount              int                     `json:"rawEventCount,omitempty"`
	CompactedEventCount        int                     `json:"compactedEventCount,omitempty"`
	SummaryBoundaryEventIndex  int                     `json:"summaryBoundaryEventIndex,omitempty"`
	Breakdown                  SessionContextBreakdown `json:"breakdown"`
	RawBreakdown               SessionContextBreakdown `json:"rawBreakdown,omitempty"`
	TrimmedToolResponseCount   int                     `json:"trimmedToolResponseCount,omitempty"`
	LastCompactedAt            string                  `json:"lastCompactedAt,omitempty"`
	LastCompactionMode         string                  `json:"lastCompactionMode,omitempty"`
	LastCompactionReason       string                  `json:"lastCompactionReason,omitempty"`
	AutoCompacted              bool                    `json:"autoCompacted"`
	DegradedSummary            bool                    `json:"degradedSummary"`
}

func nowString() string {
	return time.Now().UTC().Format(time.RFC3339Nano)
}
