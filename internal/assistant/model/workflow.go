package model

const (
	WorkflowStatusPaused   = "PAUSED"
	WorkflowStatusComplete = "COMPLETED"
	WorkflowStatusFailed   = "FAILED"

	WorkflowPlanSourcePlanner = "planner"
	WorkflowPlanSourceRuntime = "runtime"
	WorkflowPlanSourceCanvas  = "canvas"

	WorkflowTaskExecutorSelf  = "self"
	WorkflowTaskExecutorChild = "child"

	MaxRuntimeWorkflowTasks = 10
)

// WorkflowStep is the executable step projection derived from a task or a
// planner draft. It is the plan-time counterpart of WorkflowStepState.
type WorkflowStep struct {
	Order               int
	DependencyID        string
	Title               string
	Description         string
	Message             string
	DependsOn           []string
	AgentRole           string
	ChildAgentID        string
	ChildProviderID     string
	ChildModel          string
	ChildPermissionMode string
	ModeHint            string
	Objective           string
	PlanSource          string
	WorkflowMode        string
	PlannerWarnings     []string
}

type ChatRequest struct {
	ClientRequestID        string      `json:"clientRequestId"`
	AgentID                string      `json:"agentId,omitempty"`
	SessionID              string      `json:"sessionId,omitempty"`
	Message                string      `json:"message"`
	ProviderID             string      `json:"providerId,omitempty"`
	Model                  string      `json:"model,omitempty"`
	WorkModeOverride       string      `json:"workModeOverride,omitempty"`
	PermissionModeOverride string      `json:"permissionModeOverride,omitempty"`
	Objective              string      `json:"objective,omitempty"`
	RunOptions             *RunOptions `json:"runOptions,omitempty"`
}

type RunOptions struct {
	LoopMaxIterations int `json:"loopMaxIterations,omitempty"`
}

type WorkflowDefinition struct {
	ID                string               `json:"id"`
	Name              string               `json:"name"`
	Description       string               `json:"description,omitempty"`
	Status            string               `json:"status"`
	AgentID           string               `json:"agentId"`
	WorkMode          string               `json:"workMode"`
	ProviderID        string               `json:"providerId,omitempty"`
	Model             string               `json:"model,omitempty"`
	PermissionMode    string               `json:"permissionMode,omitempty"`
	PromptTemplate    string               `json:"promptTemplate"`
	ObjectiveTemplate string               `json:"objectiveTemplate,omitempty"`
	DefaultInputs     map[string]any       `json:"defaultInputs,omitempty"`
	CanvasGraph       *WorkflowCanvasGraph `json:"canvasGraph,omitempty"`
	Tags              []string             `json:"tags,omitempty"`
	BuiltinTemplate   bool                 `json:"builtinTemplate,omitempty"`
	CreatedAt         string               `json:"createdAt"`
	UpdatedAt         string               `json:"updatedAt"`
	DeletedAt         *string              `json:"deletedAt,omitempty"`
}

type WorkflowDefinitionWriteRequest struct {
	ID                string               `json:"id,omitempty"`
	Name              string               `json:"name"`
	Description       string               `json:"description,omitempty"`
	Status            string               `json:"status,omitempty"`
	AgentID           string               `json:"agentId"`
	WorkMode          string               `json:"workMode,omitempty"`
	ProviderID        string               `json:"providerId,omitempty"`
	Model             string               `json:"model,omitempty"`
	PermissionMode    string               `json:"permissionMode,omitempty"`
	PromptTemplate    string               `json:"promptTemplate"`
	ObjectiveTemplate string               `json:"objectiveTemplate,omitempty"`
	DefaultInputs     map[string]any       `json:"defaultInputs,omitempty"`
	CanvasGraph       *WorkflowCanvasGraph `json:"canvasGraph,omitempty"`
	Tags              []string             `json:"tags,omitempty"`
}

type WorkflowCanvasGraph struct {
	Version  string               `json:"version,omitempty"`
	Nodes    []WorkflowCanvasNode `json:"nodes,omitempty"`
	Edges    []WorkflowCanvasEdge `json:"edges,omitempty"`
	Viewport map[string]any       `json:"viewport,omitempty"`
}

type WorkflowCanvasPoint struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

type WorkflowCanvasNode struct {
	ID       string              `json:"id"`
	Type     string              `json:"type"`
	Position WorkflowCanvasPoint `json:"position"`
	Data     map[string]any      `json:"data,omitempty"`
}

type WorkflowCanvasEdge struct {
	ID           string         `json:"id"`
	Source       string         `json:"source"`
	Target       string         `json:"target"`
	SourceHandle string         `json:"sourceHandle,omitempty"`
	TargetHandle string         `json:"targetHandle,omitempty"`
	Type         string         `json:"type,omitempty"`
	Data         map[string]any `json:"data,omitempty"`
}

type WorkflowTrigger struct {
	ID         string         `json:"id"`
	WorkflowID string         `json:"workflowId"`
	Type       string         `json:"type"`
	Title      string         `json:"title"`
	Status     string         `json:"status"`
	Config     map[string]any `json:"config,omitempty"`
	SecretHash string         `json:"secretHash,omitempty"`
	HasSecret  bool           `json:"hasSecret,omitempty"`
	NextRunAt  string         `json:"nextRunAt,omitempty"`
	LastRunAt  string         `json:"lastRunAt,omitempty"`
	LastRunID  string         `json:"lastRunId,omitempty"`
	LastError  string         `json:"lastError,omitempty"`
	CreatedAt  string         `json:"createdAt"`
	UpdatedAt  string         `json:"updatedAt"`
	DeletedAt  *string        `json:"deletedAt,omitempty"`
}

type WorkflowTriggerWriteRequest struct {
	ID          string         `json:"id,omitempty"`
	Type        string         `json:"type"`
	Title       string         `json:"title,omitempty"`
	Status      string         `json:"status,omitempty"`
	Config      map[string]any `json:"config,omitempty"`
	ResetSecret bool           `json:"resetSecret,omitempty"`
}

type WorkflowTriggerLog struct {
	ID           string            `json:"id"`
	WorkflowID   string            `json:"workflowId"`
	TriggerID    string            `json:"triggerId,omitempty"`
	TriggerType  string            `json:"triggerType"`
	Status       string            `json:"status"`
	RunID        string            `json:"runId,omitempty"`
	SessionID    string            `json:"sessionId,omitempty"`
	Inputs       map[string]any    `json:"inputs,omitempty"`
	MatchedEvent map[string]any    `json:"matchedEvent,omitempty"`
	Result       *WorkflowResult   `json:"result,omitempty"`
	NodeRuns     []WorkflowNodeRun `json:"nodeRuns,omitempty"`
	Error        string            `json:"error,omitempty"`
	StartedAt    string            `json:"startedAt,omitempty"`
	FinishedAt   string            `json:"finishedAt,omitempty"`
	CreatedAt    string            `json:"createdAt"`
	UpdatedAt    string            `json:"updatedAt"`
}

type WorkflowResult struct {
	Format      string         `json:"format,omitempty"`
	Markdown    string         `json:"markdown,omitempty"`
	JSON        map[string]any `json:"json,omitempty"`
	RawResponse *ChatResponse  `json:"rawResponse,omitempty"`
}

type WorkflowNodeRun struct {
	NodeID     string         `json:"nodeId"`
	NodeType   string         `json:"nodeType"`
	Title      string         `json:"title,omitempty"`
	Status     string         `json:"status"`
	StartedAt  string         `json:"startedAt,omitempty"`
	FinishedAt string         `json:"finishedAt,omitempty"`
	Inputs     map[string]any `json:"inputs,omitempty"`
	Outputs    map[string]any `json:"outputs,omitempty"`
	Error      string         `json:"error,omitempty"`
}

type WorkflowEvent struct {
	ID       string         `json:"id,omitempty"`
	Type     string         `json:"type"`
	Source   string         `json:"source"`
	EntityID string         `json:"entityId,omitempty"`
	At       string         `json:"at,omitempty"`
	Payload  map[string]any `json:"payload,omitempty"`
}
