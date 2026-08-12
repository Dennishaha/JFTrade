package assistant

import (
	jfadk "github.com/jftrade/jftrade-main/internal/assistant/engine/workflowruntime"
	assistantmodel "github.com/jftrade/jftrade-main/internal/assistant/model"
	"github.com/jftrade/jftrade-main/internal/jftsettings"
)

type ADKPageData struct {
	Limit    int  `json:"limit"`
	Offset   int  `json:"offset"`
	Total    int  `json:"total"`
	Returned int  `json:"returned"`
	HasMore  bool `json:"hasMore"`
}

type ADKSnapshotData struct {
	Providers       []jfadk.Provider                `json:"providers"`
	Agents          []jfadk.Agent                   `json:"agents"`
	Skills          []jfadk.Skill                   `json:"skills"`
	Tools           []jfadk.ToolDescriptor          `json:"tools"`
	RuntimeSettings *jftsettings.ADKRuntimeSettings `json:"runtimeSettings,omitempty"`
}

type ADKToolsData struct {
	Tools []jfadk.ToolDescriptor `json:"tools"`
}

type ADKAgentTemplatesData struct {
	Templates []jfadk.AgentWriteRequest `json:"templates"`
}

type ADKTasksData struct {
	Tasks []jfadk.Task `json:"tasks"`
	Page  ADKPageData  `json:"page"`
}

type ADKMemoryData struct {
	Entries []jfadk.MemoryEntry `json:"entries"`
}

type ADKProvidersData struct {
	Providers []jfadk.Provider `json:"providers"`
}

type ADKProviderTestRequest struct {
	Mode assistantmodel.ProviderTestMode `json:"mode,omitempty" enums:"quick,full"`
}

type ADKProviderTestData struct {
	OK           bool                                         `json:"ok"`
	Reply        string                                       `json:"reply"`
	Capabilities map[string]bool                              `json:"capabilities"`
	Reasoning    assistantmodel.ProviderReasoningTestResponse `json:"reasoning"`
	CheckedAt    string                                       `json:"checkedAt"`
}

type ADKAgentsData struct {
	Agents []jfadk.Agent `json:"agents"`
	Page   ADKPageData   `json:"page"`
}

type ADKSkillsData struct {
	Skills []jfadk.Skill `json:"skills"`
}

type ADKDeletedIDData struct {
	Deleted bool   `json:"deleted"`
	ID      string `json:"id"`
}

type ADKSessionsData struct {
	Sessions []jfadk.Session `json:"sessions"`
	Page     ADKPageData     `json:"page"`
}

type ADKRunsData struct {
	Runs []jfadk.Run `json:"runs"`
	Page ADKPageData `json:"page"`
}

type ADKApprovalsData struct {
	Approvals []jfadk.Approval `json:"approvals"`
	Page      ADKPageData      `json:"page"`
}

type ADKAuditData struct {
	Events []jfadk.AuditEvent `json:"events"`
	Page   ADKPageData        `json:"page"`
}

type ADKMetricsData struct {
	Runs              ADKRunMetricsData        `json:"runs"`
	Tools             ADKToolMetricsData       `json:"tools"`
	Approvals         ADKApprovalMetricsData   `json:"approvals"`
	Usage             ADKUsageMetricsData      `json:"usage"`
	Sessions          ADKActivityCountData     `json:"sessions"`
	Workflows         ADKWorkflowMetricsData   `json:"workflows"`
	MeasurementWindow ADKMeasurementWindowData `json:"measurementWindow"`
	CheckedAt         string                   `json:"checkedAt"`
}

type ADKRunMetricsData struct {
	Total      int            `json:"total"`
	Last7Days  int            `json:"last7Days"`
	ByStatus   map[string]int `json:"byStatus"`
	ByAgent    map[string]int `json:"byAgent"`
	ByProvider map[string]int `json:"byProvider"`
	Lifecycle  map[string]int `json:"lifecycle"`
}

type ADKToolMetricsData struct {
	Total             int            `json:"total"`
	Successful        int            `json:"successful"`
	AverageDurationMs int64          `json:"averageDurationMs"`
	ByName            map[string]int `json:"byName"`
	ByStatus          map[string]int `json:"byStatus"`
}

type ADKWaitMetricsData struct {
	Average int64 `json:"average"`
	Max     int64 `json:"max"`
	Count   int64 `json:"count,omitempty"`
}

type ADKApprovalMetricsData struct {
	Pending            int                `json:"pending"`
	Total              int                `json:"total"`
	Last7Days          int                `json:"last7Days"`
	Approved           int                `json:"approved"`
	Denied             int                `json:"denied"`
	RecoverablePending int                `json:"recoverablePending"`
	PendingWaitMs      ADKWaitMetricsData `json:"pendingWaitMs"`
	ResolutionWaitMs   ADKWaitMetricsData `json:"resolutionWaitMs"`
}

type ADKActivityCountData struct {
	Total     int `json:"total"`
	Last7Days int `json:"last7Days"`
}

type ADKWorkflowMetricsData struct {
	Definitions          int            `json:"definitions"`
	EnabledDefinitions   int            `json:"enabledDefinitions"`
	Triggers             int            `json:"triggers"`
	EnabledTriggers      int            `json:"enabledTriggers"`
	Invocations          int            `json:"invocations"`
	InvocationsLast7Days int            `json:"invocationsLast7Days"`
	ByStatus             map[string]int `json:"byStatus"`
	ByTriggerType        map[string]int `json:"byTriggerType"`
}

type ADKMeasurementWindowData struct {
	Days  int    `json:"days"`
	Since string `json:"since"`
}

type ADKUsageMetricsData struct {
	Samples          int `json:"samples"`
	TokensInTotal    int `json:"tokensInTotal"`
	TokensOutTotal   int `json:"tokensOutTotal"`
	TokensInAverage  int `json:"tokensInAverage"`
	TokensOutAverage int `json:"tokensOutAverage"`
}

type ADKOptimizationTasksData struct {
	Tasks []ADKOptimizationTaskData `json:"tasks"`
	Page  ADKPageData               `json:"page"`
}

type ADKOptimizationTaskData struct {
	ID        string                   `json:"id"`
	Status    string                   `json:"status"`
	Objective string                   `json:"objective"`
	Runs      []ADKOptimizationRunData `json:"runs"`
	Progress  ADKOptimizationProgress  `json:"progress"`
	CreatedAt string                   `json:"createdAt"`
	UpdatedAt string                   `json:"updatedAt"`
}

type ADKOptimizationRunData struct {
	DefinitionID string `json:"definitionId"`
	RunID        string `json:"runId"`
	Status       string `json:"status"`
	Result       any    `json:"result,omitempty"`
}

type ADKOptimizationProgress struct {
	Total     int `json:"total"`
	Running   int `json:"running"`
	Completed int `json:"completed"`
	Failed    int `json:"failed"`
	Cancelled int `json:"cancelled"`
}

type ADKWorkflowsData struct {
	Workflows []jfadk.WorkflowDefinition `json:"workflows"`
	Page      ADKPageData                `json:"page"`
}

type ADKWorkflowDeleteData struct {
	Deleted  bool                     `json:"deleted"`
	Workflow jfadk.WorkflowDefinition `json:"workflow"`
}

type ADKWorkflowTriggersData struct {
	Triggers []jfadk.WorkflowTrigger `json:"triggers"`
}

type ADKWorkflowTriggerSaveData struct {
	Trigger jfadk.WorkflowTrigger `json:"trigger"`
	Secret  string                `json:"secret,omitempty"`
}

type ADKWorkflowTriggerDeleteData struct {
	Deleted bool                  `json:"deleted"`
	Trigger jfadk.WorkflowTrigger `json:"trigger"`
}

type ADKWorkflowInvocationData struct {
	Workflow jfadk.WorkflowDefinition `json:"workflow"`
	Trigger  *jfadk.WorkflowTrigger   `json:"trigger,omitempty"`
	Log      jfadk.WorkflowTriggerLog `json:"log"`
	Response *jfadk.ChatResponse      `json:"response,omitempty"`
}

type ADKWorkflowTriggerLogsData struct {
	Logs []jfadk.WorkflowTriggerLog `json:"logs"`
	Page ADKPageData                `json:"page"`
}

type ADKCreateSessionRequest struct {
	AgentID string `json:"agentId"`
	Title   string `json:"title"`
}

type ADKRenameSessionRequest struct {
	Title string `json:"title"`
}

type ADKCompactContextRequest struct {
	Mode   string `json:"mode"`
	Reason string `json:"reason,omitempty"`
}

type ADKUpdateRunObjectiveRequest struct {
	Objective string `json:"objective"`
}

type ADKInstallSkillRequest struct {
	URL string `json:"url"`
}

type ADKWorkflowInputsRequest struct {
	Inputs map[string]any `json:"inputs,omitempty"`
}

type ADKTaskWriteRequest struct {
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

type ADKTaskPatchRequest struct {
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

type ADKMemoryWriteRequest struct {
	AgentID string `json:"agentId,omitempty"`
	Key     string `json:"key"`
	Value   string `json:"value"`
	Scope   string `json:"scope,omitempty"`
}

type ADKProviderWriteRequest struct {
	ID                  string                                  `json:"id,omitempty"`
	DisplayName         string                                  `json:"displayName"`
	BaseURL             string                                  `json:"baseUrl"`
	Model               string                                  `json:"model"`
	ReasoningConfig     *assistantmodel.ProviderReasoningConfig `json:"reasoningConfig,omitempty"`
	ContextWindowTokens int                                     `json:"contextWindowTokens,omitempty"`
	RequestTimeoutMs    int                                     `json:"requestTimeoutMs,omitempty"`
	DefaultHeaders      map[string]string                       `json:"defaultHeaders,omitempty"`
	APIKey              string                                  `json:"apiKey,omitempty"`
	Enabled             bool                                    `json:"enabled"`
}

type ADKAgentWriteRequest struct {
	ID                string                         `json:"id,omitempty"`
	Name              string                         `json:"name"`
	Instruction       string                         `json:"instruction"`
	ProviderID        string                         `json:"providerId"`
	Model             string                         `json:"model,omitempty"`
	ReasoningEffort   assistantmodel.ReasoningEffort `json:"reasoningEffort,omitempty" enums:"low,medium,high,xhigh,max"`
	Tools             []string                       `json:"tools,omitempty"`
	Skills            []string                       `json:"skills,omitempty"`
	PermissionMode    string                         `json:"permissionMode"`
	MemoryEnabled     bool                           `json:"memoryEnabled"`
	RecentUserWindow  int                            `json:"recentUserWindow,omitempty"`
	WorkMode          string                         `json:"workMode,omitempty"`
	LoopMaxIterations int                            `json:"loopMaxIterations,omitempty"`
	Status            string                         `json:"status"`
}

type ADKSessionComposerStatePatch struct {
	ChatDraft               *string `json:"chatDraft,omitempty"`
	ProviderIDOverride      *string `json:"providerIdOverride,omitempty"`
	ModelOverride           *string `json:"modelOverride,omitempty"`
	ReasoningEffortOverride *string `json:"reasoningEffortOverride,omitempty"`
	WorkModeOverride        *string `json:"workModeOverride,omitempty"`
	PermissionModeOverride  *string `json:"permissionModeOverride,omitempty"`
	GoalObjectiveDraft      *string `json:"goalObjectiveDraft,omitempty"`
	GoalObjectiveTouched    *bool   `json:"goalObjectiveTouched,omitempty"`
}

type ADKChatRequest struct {
	ClientRequestID         string                         `json:"clientRequestId" format:"uuid"`
	AgentID                 string                         `json:"agentId,omitempty"`
	SessionID               string                         `json:"sessionId,omitempty"`
	Message                 string                         `json:"message"`
	ProviderID              string                         `json:"providerId,omitempty"`
	Model                   string                         `json:"model,omitempty"`
	ReasoningEffortOverride assistantmodel.ReasoningEffort `json:"reasoningEffortOverride,omitempty" enums:"low,medium,high,xhigh,max"`
	WorkModeOverride        string                         `json:"workModeOverride,omitempty"`
	PermissionModeOverride  string                         `json:"permissionModeOverride,omitempty"`
	Objective               string                         `json:"objective,omitempty"`
	RunOptions              *jfadk.RunOptions              `json:"runOptions,omitempty"`
}

type ADKInputResponseRequest struct {
	RequestID string              `json:"requestId"`
	Answers   []jfadk.InputAnswer `json:"answers"`
}

type ADKWorkflowDefinitionWriteRequest struct {
	ID                string                     `json:"id,omitempty"`
	Name              string                     `json:"name"`
	Description       string                     `json:"description,omitempty"`
	Status            string                     `json:"status,omitempty"`
	AgentID           string                     `json:"agentId"`
	WorkMode          string                     `json:"workMode,omitempty"`
	ProviderID        string                     `json:"providerId,omitempty"`
	Model             string                     `json:"model,omitempty"`
	PermissionMode    string                     `json:"permissionMode,omitempty"`
	PromptTemplate    string                     `json:"promptTemplate"`
	ObjectiveTemplate string                     `json:"objectiveTemplate,omitempty"`
	DefaultInputs     map[string]any             `json:"defaultInputs,omitempty"`
	CanvasGraph       *jfadk.WorkflowCanvasGraph `json:"canvasGraph,omitempty"`
	Tags              []string                   `json:"tags,omitempty"`
}

type ADKWorkflowTriggerWriteRequest struct {
	ID          string         `json:"id,omitempty"`
	Type        string         `json:"type"`
	Title       string         `json:"title,omitempty"`
	Status      string         `json:"status,omitempty"`
	Config      map[string]any `json:"config,omitempty"`
	ResetSecret bool           `json:"resetSecret,omitempty"`
}
