package model

import (
	"strings"
	"time"
)

const (
	DefaultBuiltinAgentID       = "jftrade-default"
	GoogleADKModule             = "google.golang.org/adk/v2"
	WorkflowManagementSkillName = "jftrade-workflow-management"

	PermissionModeApproval     = "approval"
	PermissionModeLessApproval = "less_approval"
	PermissionModeAll          = "all"

	WorkModeChat = "chat"
	WorkModeLoop = "loop"

	WorkflowEngineADK2Loop   = "adk2_loop"
	WorkflowEngineADK2Canvas = "adk2_canvas"

	AgentStatusEnabled  = "ENABLED"
	AgentStatusDisabled = "DISABLED"

	WorkflowStatusEnabled  = "ENABLED"
	WorkflowStatusDisabled = "DISABLED"
	WorkflowStatusRunning  = "RUNNING"

	WorkflowTriggerTypeManual          = "manual"
	WorkflowTriggerTypeSchedule        = "schedule"
	WorkflowTriggerTypeWebhook         = "webhook"
	WorkflowTriggerTypeEvent           = "event"
	WorkflowTriggerTypeMarketThreshold = "market_threshold"

	WorkflowTriggerStatusEnabled  = "ENABLED"
	WorkflowTriggerStatusDisabled = "DISABLED"
	WorkflowTriggerStatusError    = "ERROR"

	WorkflowTriggerLogStatusQueued          = "QUEUED"
	WorkflowTriggerLogStatusRunning         = "RUNNING"
	WorkflowTriggerLogStatusSucceeded       = "SUCCEEDED"
	WorkflowTriggerLogStatusPendingApproval = "PENDING_APPROVAL"
	WorkflowTriggerLogStatusFailed          = "FAILED"
	WorkflowTriggerLogStatusCancelled       = "CANCELLED"
	WorkflowTriggerLogStatusSkipped         = "SKIPPED"

	RunStatusRunning      = "RUNNING"
	RunStatusCompleted    = "COMPLETED"
	RunStatusPending      = "PENDING_APPROVAL"
	RunStatusPendingInput = "PENDING_INPUT"
	RunStatusFailed       = "FAILED"
	RunStatusDenied       = "DENIED"
	RunStatusCancelled    = "CANCELLED"
	RunStatusTimedOut     = "TIMED_OUT"
	RunStatusPaused       = "PAUSED"

	ApprovalStatusPending  = "PENDING"
	ApprovalStatusApproved = "APPROVED"
	ApprovalStatusDenied   = "DENIED"

	ContextStatusUnknown   = "unknown"
	ContextStatusHealthy   = "healthy"
	ContextStatusWarning   = "warning"
	ContextStatusNearLimit = "near_limit"
	ContextStatusCritical  = "critical"

	// Runtime safety limits
	DefaultProviderRequestTimeout = 180 * time.Second
	DefaultRunTimeout             = 30 * time.Minute
	DefaultStreamIdleTimeout      = 300 * time.Second
	MaxConcurrentRuns             = 10 // Maximum simultaneous runs
	DefaultLoopMaxIterations      = 5
	MaxLoopIterations             = 20
	MaxToolOutputBytes            = 256 << 10 // Maximum tool output size (256 KiB)
	MaxMessageLength              = 50000     // Maximum user message length in runes
)

type ReasoningEffort string // @name adk.ReasoningEffort

const (
	ReasoningEffortLow    ReasoningEffort = "low"
	ReasoningEffortMedium ReasoningEffort = "medium"
	ReasoningEffortHigh   ReasoningEffort = "high"
	ReasoningEffortXHigh  ReasoningEffort = "xhigh"
	ReasoningEffortMax    ReasoningEffort = "max"
)

// ProviderReasoningMapping maps a provider-neutral reasoning level to the
// provider's exact request enum value. Presence in the provider mapping list
// declares that the level is supported.
type ProviderReasoningMapping struct {
	Effort ReasoningEffort `json:"effort" enums:"low,medium,high,xhigh,max"`
	Value  string          `json:"value"`
} // @name adk.ProviderReasoningMapping

// ProviderReasoningConfig controls the request field and supported reasoning
// enum values for one provider. An empty mapping list means no explicit level
// is supported; an empty Agent level still uses the model default.
type ProviderReasoningConfig struct {
	RequestField string                     `json:"requestField"`
	Mappings     []ProviderReasoningMapping `json:"mappings"`
} // @name adk.ProviderReasoningConfig

type ProviderReasoningTestResult struct {
	Effort ReasoningEffort `json:"effort" enums:"low,medium,high,xhigh,max"`
	Value  string          `json:"value"`
	OK     bool            `json:"ok"`
	Error  string          `json:"error,omitempty"`
} // @name adk.ProviderReasoningTestResult

type ProviderReasoningTestResponse struct {
	Mode         ProviderTestMode              `json:"mode" enums:"quick,full"`
	RequestField string                        `json:"requestField"`
	OK           bool                          `json:"ok"`
	Results      []ProviderReasoningTestResult `json:"results"`
} // @name adk.ProviderReasoningTestResponse

type ProviderTestMode string // @name adk.ProviderTestMode

const (
	ProviderTestModeQuick ProviderTestMode = "quick"
	ProviderTestModeFull  ProviderTestMode = "full"
)

type ProviderTestResponse struct {
	OK           bool                          `json:"ok"`
	Reply        string                        `json:"reply"`
	Capabilities map[string]bool               `json:"capabilities"`
	Reasoning    ProviderReasoningTestResponse `json:"reasoning"`
	CheckedAt    string                        `json:"checkedAt"`
} // @name adk.ProviderTestResponse

// RuntimeLimits is the dynamic runtime limits snapshot consumed by ADK.
type RuntimeLimits struct {
	RunTimeout time.Duration `json:"-"`
}

type RuntimeLimitsProvider func() RuntimeLimits

type Provider struct {
	ID                  string                  `json:"id"`
	DisplayName         string                  `json:"displayName"`
	BaseURL             string                  `json:"baseUrl"`
	Model               string                  `json:"model"`
	ReasoningConfig     ProviderReasoningConfig `json:"reasoningConfig"`
	ContextWindowTokens int                     `json:"contextWindowTokens,omitempty"`
	RequestTimeoutMs    int                     `json:"requestTimeoutMs"`
	DefaultHeaders      map[string]string       `json:"defaultHeaders,omitempty"`
	Enabled             bool                    `json:"enabled"`
	Default             bool                    `json:"default"`
	HasAPIKey           bool                    `json:"hasApiKey"`
	Capabilities        map[string]bool         `json:"capabilities,omitempty"`
	CreatedAt           string                  `json:"createdAt"`
	UpdatedAt           string                  `json:"updatedAt"`
} // @name adk.Provider

// RequestTimeout returns the effective provider request timeout after
// applying the shared floor, ceiling and default normalization rules.
func (p Provider) RequestTimeout() time.Duration {
	timeoutMs := p.RequestTimeoutMs
	if timeoutMs <= 0 {
		timeoutMs = int(DefaultProviderRequestTimeout / time.Millisecond)
	}
	if timeoutMs < 15_000 {
		timeoutMs = 15_000
	}
	if timeoutMs > 600_000 {
		timeoutMs = 600_000
	}
	return time.Duration(timeoutMs) * time.Millisecond
}

// DefaultString trims value and falls back to defaultValue when blank.
func DefaultString(value string, defaultValue string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return defaultValue
	}
	return value
}

type ProviderWriteRequest struct {
	ID                  string                   `json:"id,omitempty"`
	DisplayName         string                   `json:"displayName"`
	BaseURL             string                   `json:"baseUrl"`
	Model               string                   `json:"model"`
	ReasoningConfig     *ProviderReasoningConfig `json:"reasoningConfig,omitempty"`
	ContextWindowTokens int                      `json:"contextWindowTokens,omitempty"`
	RequestTimeoutMs    int                      `json:"requestTimeoutMs,omitempty"`
	DefaultHeaders      map[string]string        `json:"defaultHeaders,omitempty"`
	APIKey              string                   `json:"apiKey,omitempty"`
	Enabled             bool                     `json:"enabled"`
}

type Agent struct {
	ID                   string          `json:"id"`
	Name                 string          `json:"name"`
	Instruction          string          `json:"instruction"`
	ProviderID           string          `json:"providerId"`
	Model                string          `json:"model"`
	ReasoningEffort      ReasoningEffort `json:"reasoningEffort,omitempty" enums:"low,medium,high,xhigh,max"`
	ReasoningEffortField string          `json:"-"`
	ReasoningEffortValue string          `json:"-"`
	Tools                []string        `json:"tools"`
	Skills               []string        `json:"skills"`
	PermissionMode       string          `json:"permissionMode"`
	MemoryEnabled        bool            `json:"memoryEnabled"`
	RecentUserWindow     int             `json:"recentUserWindow"`
	WorkMode             string          `json:"workMode"`
	LoopMaxIterations    int             `json:"loopMaxIterations"`
	Status               string          `json:"status"`
	Builtin              bool            `json:"builtin,omitempty"`
	CreatedAt            string          `json:"createdAt"`
	UpdatedAt            string          `json:"updatedAt"`
	DeletedAt            *string         `json:"deletedAt,omitempty"`
} // @name adk.Agent

type AgentWriteRequest struct {
	ID                string          `json:"id,omitempty"`
	Name              string          `json:"name"`
	Instruction       string          `json:"instruction"`
	ProviderID        string          `json:"providerId"`
	Model             string          `json:"model,omitempty"`
	ReasoningEffort   ReasoningEffort `json:"reasoningEffort,omitempty" enums:"low,medium,high,xhigh,max"`
	Tools             []string        `json:"tools,omitempty"`
	Skills            []string        `json:"skills,omitempty"`
	PermissionMode    string          `json:"permissionMode"`
	MemoryEnabled     bool            `json:"memoryEnabled"`
	RecentUserWindow  int             `json:"recentUserWindow,omitempty"`
	WorkMode          string          `json:"workMode,omitempty"`
	LoopMaxIterations int             `json:"loopMaxIterations,omitempty"`
	Status            string          `json:"status"`
} // @name adk.AgentWriteRequest

type Session struct {
	ID           string `json:"id"`
	AgentID      string `json:"agentId"`
	Title        string `json:"title"`
	WorkflowID   string `json:"workflowId,omitempty"`
	WorkflowName string `json:"workflowName,omitempty"`
	CreatedAt    string `json:"createdAt"`
	UpdatedAt    string `json:"updatedAt"`
} // @name adk.Session

type SessionComposerState struct {
	SessionID               string `json:"sessionId"`
	ChatDraft               string `json:"chatDraft"`
	ProviderIDOverride      string `json:"providerIdOverride"`
	ModelOverride           string `json:"modelOverride"`
	ReasoningEffortOverride string `json:"reasoningEffortOverride"`
	WorkModeOverride        string `json:"workModeOverride"`
	PermissionModeOverride  string `json:"permissionModeOverride"`
	GoalObjectiveDraft      string `json:"goalObjectiveDraft"`
	GoalObjectiveTouched    bool   `json:"goalObjectiveTouched"`
	UpdatedAt               string `json:"updatedAt"`
} // @name adk.SessionComposerState

type SessionComposerStatePatch struct {
	ChatDraft               *string `json:"chatDraft,omitempty"`
	ProviderIDOverride      *string `json:"providerIdOverride,omitempty"`
	ModelOverride           *string `json:"modelOverride,omitempty"`
	ReasoningEffortOverride *string `json:"reasoningEffortOverride,omitempty"`
	WorkModeOverride        *string `json:"workModeOverride,omitempty"`
	PermissionModeOverride  *string `json:"permissionModeOverride,omitempty"`
	GoalObjectiveDraft      *string `json:"goalObjectiveDraft,omitempty"`
	GoalObjectiveTouched    *bool   `json:"goalObjectiveTouched,omitempty"`
}

const (
	TimelineKindUserMessage        = "user_message"
	TimelineKindAssistantMessage   = "assistant_message"
	TimelineKindAssistantReasoning = "assistant_reasoning"
	TimelineKindToolGroup          = "tool_group"
	TimelineKindApprovalGroup      = "approval_group"
	TimelineKindInputRequest       = "input_request"
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
} // @name adk.TranscriptEntry

type TimelineEntry struct {
	ID            string        `json:"id"`
	SessionID     string        `json:"sessionId"`
	RunID         string        `json:"runId,omitempty"`
	Kind          string        `json:"kind"`
	CreatedAt     string        `json:"createdAt"`
	UpdatedAt     string        `json:"updatedAt,omitempty"`
	Sequence      int           `json:"sequence"`
	Status        string        `json:"status,omitempty"`
	Text          string        `json:"text,omitempty"`
	OriginalText  string        `json:"originalText,omitempty"`
	ProcessedText string        `json:"processedText,omitempty"`
	ToolCalls     []ToolCall    `json:"toolCalls,omitempty"`
	Approvals     []Approval    `json:"approvals,omitempty"`
	InputRequest  *InputRequest `json:"inputRequest,omitempty"`
} // @name adk.TimelineEntry

// SessionProjection is the denormalized view of one session's ADK events and
// latest run state used by timelines, transcripts and context projections.
type SessionProjection struct {
	SessionID         string
	Messages          []TranscriptEntry
	MessagesByEventID map[string]TranscriptEntry
	LatestAssistant   *TranscriptEntry
	Reply             string
	ReasoningContent  string
	ToolCalls         []ToolCall
	PendingApprovals  []Approval
	PreToolContent    string
	PreToolReasoning  string
	FinalMessageID    string
}

type Run struct {
	ID                   string              `json:"id"`
	SessionID            string              `json:"sessionId"`
	AgentID              string              `json:"agentId"`
	ProviderID           string              `json:"providerId,omitempty"`
	ProviderName         string              `json:"providerName,omitempty"`
	Model                string              `json:"model,omitempty"`
	ReasoningEffort      ReasoningEffort     `json:"reasoningEffort,omitempty" enums:"low,medium,high,xhigh,max"`
	ReasoningEffortField string              `json:"-"`
	ReasoningEffortValue string              `json:"-"`
	MaxDurationMs        int64               `json:"maxDurationMs"`
	Status               string              `json:"status"`
	Message              string              `json:"message"`
	UserMessage          string              `json:"userMessage,omitempty"`
	PreToolContent       string              `json:"preToolContent,omitempty"`
	PreToolReasoning     string              `json:"preToolReasoning,omitempty"`
	ToolSummaries        []string            `json:"toolSummaries,omitempty"`
	FailureReason        string              `json:"failureReason,omitempty"`
	ErrorCode            string              `json:"errorCode,omitempty"`
	Degraded             bool                `json:"degraded,omitempty"`
	OptimizationTaskID   string              `json:"optimizationTaskId,omitempty"`
	WorkMode             string              `json:"workMode,omitempty"`
	PermissionMode       string              `json:"permissionMode,omitempty"`
	Objective            string              `json:"objective,omitempty"`
	ParentRunID          string              `json:"parentRunId,omitempty"`
	ChildRunIDs          []string            `json:"childRunIds,omitempty"`
	Iteration            int                 `json:"iteration,omitempty"`
	WorkflowStatus       string              `json:"workflowStatus,omitempty"`
	WorkflowEngine       string              `json:"workflowEngine,omitempty"`
	WorkflowCursor       int                 `json:"workflowCursor,omitempty"`
	WorkflowPlan         []WorkflowStepState `json:"workflowPlan,omitempty"`
	ToolCalls            []ToolCall          `json:"toolCalls"`
	PendingApprovals     []Approval          `json:"pendingApprovals"`
	InputRequest         *InputRequest       `json:"inputRequest,omitempty"`
	InputRequests        []InputRequest      `json:"inputRequests,omitempty"`
	ResumeState          string              `json:"resumeState,omitempty"`
	PauseRequestedAt     *string             `json:"pauseRequestedAt,omitempty"`
	PausedAt             *string             `json:"pausedAt,omitempty"`
	PausedReason         string              `json:"pausedReason,omitempty"`
	FinalMessageID       string              `json:"finalMessageId,omitempty"`
	Usage                *RunUsage           `json:"usage,omitempty"`
	CreatedAt            string              `json:"createdAt"`
	StartedAt            string              `json:"startedAt,omitempty"`
	UpdatedAt            string              `json:"updatedAt"`
	CompletedAt          *string             `json:"completedAt,omitempty"`
	CancelledAt          *string             `json:"cancelledAt,omitempty"`
} // @name adk.Run

type WorkflowStepState struct {
	TaskID              string   `json:"taskId,omitempty"`
	Title               string   `json:"title"`
	Description         string   `json:"description,omitempty"`
	Message             string   `json:"message,omitempty"`
	Status              string   `json:"status"`
	ChildRunID          string   `json:"childRunId,omitempty"`
	ChildAgentID        string   `json:"childAgentId,omitempty"`
	ChildProviderID     string   `json:"childProviderId,omitempty"`
	ChildModel          string   `json:"childModel,omitempty"`
	ChildPermissionMode string   `json:"childPermissionMode,omitempty"`
	DependsOn           []string `json:"dependsOn,omitempty"`
	Iteration           int      `json:"iteration,omitempty"`
	Order               int      `json:"order,omitempty"`
	ModeHint            string   `json:"modeHint,omitempty"`
	AgentRole           string   `json:"agentRole,omitempty"`
	PlannerStepID       string   `json:"plannerStepId,omitempty"`
	PlanSource          string   `json:"planSource,omitempty"`
	WorkflowMode        string   `json:"workflowMode,omitempty"`
	Objective           string   `json:"objective,omitempty"`
	Executor            string   `json:"executor,omitempty"`
	ResultSummary       string   `json:"resultSummary,omitempty"`
	PlannerWarnings     []string `json:"plannerWarnings,omitempty"`
	NodeName            string   `json:"nodeName,omitempty"`
	NodeStatus          string   `json:"nodeStatus,omitempty"`
	Routes              []string `json:"routes,omitempty"`
	OutputSummary       string   `json:"outputSummary,omitempty"`
} // @name adk.WorkflowStepState

type RunUsage struct {
	ModelCalls     int   `json:"modelCalls"`
	ToolCallsTotal int   `json:"toolCallsTotal"`
	DurationMs     int64 `json:"durationMs,omitempty"`
	TokensIn       int   `json:"tokensIn,omitempty"`
	TokensOut      int   `json:"tokensOut,omitempty"`
} // @name adk.RunUsage

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
} // @name adk.ToolCall

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
} // @name adk.Approval

const (
	InputRequestStatusPending   = "PENDING"
	InputRequestStatusAnswered  = "ANSWERED"
	InputRequestStatusCancelled = "CANCELLED"
)

type InputOption struct {
	ID          string `json:"id"`
	Label       string `json:"label"`
	Description string `json:"description,omitempty"`
	Recommended bool   `json:"recommended,omitempty"`
} // @name adk.InputOption

type InputQuestion struct {
	ID         string        `json:"id"`
	Question   string        `json:"question"`
	Options    []InputOption `json:"options"`
	AllowOther bool          `json:"allowOther"`
} // @name adk.InputQuestion

type InputAnswer struct {
	QuestionID string `json:"questionId"`
	OptionID   string `json:"optionId,omitempty"`
	OtherText  string `json:"otherText,omitempty"`
} // @name adk.InputAnswer

type InputRequest struct {
	ID             string          `json:"id"`
	RunID          string          `json:"runId"`
	AgentID        string          `json:"agentId"`
	FunctionCallID string          `json:"functionCallId"`
	Title          string          `json:"title,omitempty"`
	Status         string          `json:"status"`
	Questions      []InputQuestion `json:"questions"`
	Answers        []InputAnswer   `json:"answers,omitempty"`
	CreatedAt      string          `json:"createdAt"`
	UpdatedAt      string          `json:"updatedAt"`
	AnsweredAt     *string         `json:"answeredAt,omitempty"`
} // @name adk.InputRequest

type InputResponseRequest struct {
	RequestID string        `json:"requestId"`
	Answers   []InputAnswer `json:"answers"`
}

type InputResolution struct {
	Request   InputRequest     `json:"request"`
	Run       *Run             `json:"run,omitempty"`
	ParentRun *Run             `json:"parentRun,omitempty"`
	Message   *TranscriptEntry `json:"message,omitempty"`
} // @name adk.InputResolution

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
} // @name adk.Skill

type ToolDescriptor struct {
	Name               string         `json:"name"`
	DisplayName        string         `json:"displayName"`
	Description        string         `json:"description"`
	Category           string         `json:"category"`
	Permission         string         `json:"permission"`
	IdempotencyMode    string         `json:"idempotencyMode,omitempty"`
	AllowedModes       []string       `json:"allowedModes"`
	RequiresApprovalIn []string       `json:"requiresApprovalIn"`
	InputSchema        map[string]any `json:"inputSchema,omitempty"`
	OutputSummary      string         `json:"outputSummary,omitempty"`
	RiskLevel          string         `json:"riskLevel,omitempty"`
	RequiredSkills     []string       `json:"requiredSkills,omitempty"`
} // @name adk.ToolDescriptor
