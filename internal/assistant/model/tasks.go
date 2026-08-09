package model

type ChatDelta struct {
	Reply            string                  `json:"reply,omitempty"`
	ReasoningContent string                  `json:"reasoningContent,omitempty"`
	ToolProgress     string                  `json:"toolProgress,omitempty"`
	Run              *Run                    `json:"run,omitempty"`
	Context          *SessionContextSnapshot `json:"context,omitempty"`
	Timeline         *TimelineEntry          `json:"timeline,omitempty"`
}

// ToolInvocation is the provider-agnostic shape of one tool call captured from
// an upstream protocol stream. It is shared by provider adapters and the tool
// execution layer and intentionally carries no ADK SDK types.
type ToolInvocation struct {
	Name  string
	Input map[string]any
}

type ChatResponse struct {
	Reply            string                  `json:"reply"`
	ReasoningContent string                  `json:"reasoningContent,omitempty"`
	Session          Session                 `json:"session"`
	Run              Run                     `json:"run"`
	PendingApprovals []Approval              `json:"pendingApprovals"`
	InputRequest     *InputRequest           `json:"inputRequest,omitempty"`
	Timeline         []TimelineEntry         `json:"timeline"`
	Context          *SessionContextSnapshot `json:"context,omitempty"`
} // @name adk.ChatResponse

type ApprovalResolution struct {
	Approval  Approval         `json:"approval"`
	Run       *Run             `json:"run,omitempty"`
	ParentRun *Run             `json:"parentRun,omitempty"`
	Message   *TranscriptEntry `json:"message,omitempty"`
} // @name adk.ApprovalResolution

type SessionsResponse struct {
	Session       Session              `json:"session"`
	Timeline      []TimelineEntry      `json:"timeline"`
	Runs          []Run                `json:"runs,omitempty"`
	ComposerState SessionComposerState `json:"composerState"`
} // @name adk.SessionsResponse

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
} // @name adk.AuditEvent

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
	ID                  string   `json:"id"`
	Title               string   `json:"title"`
	Description         string   `json:"description,omitempty"`
	Status              string   `json:"status"`
	AgentID             string   `json:"agentId,omitempty"`
	RunID               string   `json:"runId,omitempty"`
	DependsOn           []string `json:"dependsOn,omitempty"`
	Order               int      `json:"order,omitempty"`
	ModeHint            string   `json:"modeHint,omitempty"`
	AgentRole           string   `json:"agentRole,omitempty"`
	PlannerStepID       string   `json:"plannerStepId,omitempty"`
	PlanSource          string   `json:"planSource,omitempty"`
	WorkflowMode        string   `json:"workflowMode,omitempty"`
	Objective           string   `json:"objective,omitempty"`
	Message             string   `json:"message,omitempty"`
	Executor            string   `json:"executor,omitempty"`
	ChildAgentID        string   `json:"childAgentId,omitempty"`
	ChildProviderID     string   `json:"childProviderId,omitempty"`
	ChildModel          string   `json:"childModel,omitempty"`
	ChildPermissionMode string   `json:"childPermissionMode,omitempty"`
	ResultSummary       string   `json:"resultSummary,omitempty"`
	PlannerWarnings     []string `json:"plannerWarnings,omitempty"`
	CreatedAt           string   `json:"createdAt"`
	UpdatedAt           string   `json:"updatedAt"`
} // @name adk.Task

type TaskWriteRequest struct {
	ID                  string   `json:"id,omitempty"`
	Title               string   `json:"title"`
	Description         string   `json:"description,omitempty"`
	Status              string   `json:"status,omitempty"`
	AgentID             string   `json:"agentId,omitempty"`
	RunID               string   `json:"runId,omitempty"`
	DependsOn           []string `json:"dependsOn,omitempty"`
	Order               int      `json:"order,omitempty"`
	ModeHint            string   `json:"modeHint,omitempty"`
	AgentRole           string   `json:"agentRole,omitempty"`
	PlannerStepID       string   `json:"plannerStepId,omitempty"`
	PlanSource          string   `json:"planSource,omitempty"`
	WorkflowMode        string   `json:"workflowMode,omitempty"`
	Objective           string   `json:"objective,omitempty"`
	Message             string   `json:"message,omitempty"`
	Executor            string   `json:"executor,omitempty"`
	ChildAgentID        string   `json:"childAgentId,omitempty"`
	ChildProviderID     string   `json:"childProviderId,omitempty"`
	ChildModel          string   `json:"childModel,omitempty"`
	ChildPermissionMode string   `json:"childPermissionMode,omitempty"`
	ResultSummary       string   `json:"resultSummary,omitempty"`
	PlannerWarnings     []string `json:"plannerWarnings,omitempty"`
}

type TaskPatchRequest struct {
	Title               *string  `json:"title,omitempty"`
	Description         *string  `json:"description,omitempty"`
	Status              *string  `json:"status,omitempty"`
	AgentID             *string  `json:"agentId,omitempty"`
	RunID               *string  `json:"runId,omitempty"`
	DependsOn           []string `json:"dependsOn,omitempty"`
	Order               *int     `json:"order,omitempty"`
	ModeHint            *string  `json:"modeHint,omitempty"`
	AgentRole           *string  `json:"agentRole,omitempty"`
	PlannerStepID       *string  `json:"plannerStepId,omitempty"`
	PlanSource          *string  `json:"planSource,omitempty"`
	WorkflowMode        *string  `json:"workflowMode,omitempty"`
	Objective           *string  `json:"objective,omitempty"`
	Message             *string  `json:"message,omitempty"`
	Executor            *string  `json:"executor,omitempty"`
	ChildAgentID        *string  `json:"childAgentId,omitempty"`
	ChildProviderID     *string  `json:"childProviderId,omitempty"`
	ChildModel          *string  `json:"childModel,omitempty"`
	ChildPermissionMode *string  `json:"childPermissionMode,omitempty"`
	ResultSummary       *string  `json:"resultSummary,omitempty"`
	PlannerWarnings     []string `json:"plannerWarnings,omitempty"`
}

type MemoryEntry struct {
	ID        string `json:"id"`
	AgentID   string `json:"agentId,omitempty"`
	Key       string `json:"key"`
	Value     string `json:"value"`
	Scope     string `json:"scope"`
	CreatedAt string `json:"createdAt"`
	UpdatedAt string `json:"updatedAt"`
} // @name adk.MemoryEntry

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
} // @name adk.SessionContextBreakdown

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
	RawBreakdown               SessionContextBreakdown `json:"rawBreakdown,omitzero"`
	TrimmedToolResponseCount   int                     `json:"trimmedToolResponseCount,omitempty"`
	LastCompactedAt            string                  `json:"lastCompactedAt,omitempty"`
	LastCompactionMode         string                  `json:"lastCompactionMode,omitempty"`
	LastCompactionReason       string                  `json:"lastCompactionReason,omitempty"`
	AutoCompacted              bool                    `json:"autoCompacted"`
	DegradedSummary            bool                    `json:"degradedSummary"`
} // @name adk.SessionContextSnapshot
