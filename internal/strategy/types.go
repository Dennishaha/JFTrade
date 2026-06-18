package strategy

import "time"

type VisualNode struct {
	ID         string         `json:"id"`
	Type       string         `json:"type"`
	X          float64        `json:"x"`
	Y          float64        `json:"y"`
	Text       string         `json:"text,omitempty"`
	Properties map[string]any `json:"properties,omitempty"`
}

type VisualEdge struct {
	ID           string         `json:"id,omitempty"`
	Type         string         `json:"type,omitempty"`
	SourceNodeID string         `json:"sourceNodeId"`
	TargetNodeID string         `json:"targetNodeId"`
	Text         string         `json:"text,omitempty"`
	Properties   map[string]any `json:"properties,omitempty"`
}

type VisualModel struct {
	Engine  string       `json:"engine,omitempty"`
	Version int          `json:"version,omitempty"`
	Nodes   []VisualNode `json:"nodes,omitempty"`
	Edges   []VisualEdge `json:"edges,omitempty"`
}

type Definition struct {
	ID           string       `json:"id"`
	Name         string       `json:"name"`
	Version      string       `json:"version"`
	Description  string       `json:"description"`
	Runtime      string       `json:"runtime"`
	SourceFormat string       `json:"sourceFormat"`
	Symbol       string       `json:"symbol,omitempty"`
	Interval     string       `json:"interval,omitempty"`
	Script       string       `json:"script"`
	VisualModel  *VisualModel `json:"visualModel,omitempty"`
	CreatedAt    string       `json:"createdAt"`
	UpdatedAt    string       `json:"updatedAt"`
}

type DefinitionView struct {
	Definition
	DerivedWarmupBars     int    `json:"derivedWarmupBars"`
	DerivedWarmupInterval string `json:"derivedWarmupInterval"`
}

type DefinitionSummary struct {
	StrategyID string `json:"strategyId"`
	Name       string `json:"name"`
	Version    string `json:"version"`
}

type BrokerAccountBinding struct {
	BrokerID           string `json:"brokerId"`
	AccountID          string `json:"accountId"`
	TradingEnvironment string `json:"tradingEnvironment"`
	Market             string `json:"market"`
}

type BindingInstrument struct {
	Market string `json:"market"`
	Code   string `json:"code"`
}

type InstanceBinding struct {
	Instruments   []BindingInstrument   `json:"instruments,omitempty"`
	Symbols       []string              `json:"symbols"`
	Interval      string                `json:"interval"`
	ExecutionMode string                `json:"executionMode"`
	BrokerAccount *BrokerAccountBinding `json:"brokerAccount,omitempty"`
	RuntimeRisk   RuntimeRiskSettings   `json:"runtimeRisk"`
}

type RuntimeRiskSettings struct {
	Mode             string   `json:"mode"`
	CloseOnly        bool     `json:"closeOnly"`
	MaxOrderQuantity *float64 `json:"maxOrderQuantity,omitempty"`
	MaxOrderNotional *float64 `json:"maxOrderNotional,omitempty"`
	DailyMaxOrders   *int     `json:"dailyMaxOrders,omitempty"`
	PauseOnReject    bool     `json:"pauseOnReject"`
}

type AuditEntry struct {
	InstanceID string `json:"instanceId"`
	Kind       string `json:"kind"`
	Detail     string `json:"detail,omitempty"`
	At         string `json:"at"`
}

type RuntimeObservation struct {
	ActualStatus      string   `json:"actualStatus"`
	ActiveSymbols     []string `json:"activeSymbols"`
	LastClosedKLineAt *string  `json:"lastClosedKlineAt,omitempty"`
	LastSignalAt      *string  `json:"lastSignalAt,omitempty"`
	LastOrderAt       *string  `json:"lastOrderAt,omitempty"`
	LastErrorAt       *string  `json:"lastErrorAt,omitempty"`
	LastError         *string  `json:"lastError,omitempty"`
	UpdatedAt         *string  `json:"updatedAt,omitempty"`
}

type DefinitionSyncStatus struct {
	DefinitionID   string  `json:"definitionId"`
	AppliedVersion string  `json:"appliedVersion"`
	LatestVersion  string  `json:"latestVersion"`
	IsLatest       bool    `json:"isLatest"`
	CanApplyLatest bool    `json:"canApplyLatest"`
	BlockedReason  *string `json:"blockedReason,omitempty"`
}

type ManagedInstance struct {
	ID           string            `json:"id"`
	PluginID     string            `json:"pluginId,omitempty"`
	Definition   DefinitionSummary `json:"definition"`
	Binding      InstanceBinding   `json:"binding"`
	Params       map[string]any    `json:"params"`
	Status       string            `json:"status"`
	CreatedAt    string            `json:"createdAt"`
	Logs         []string          `json:"logs,omitempty"`
	AuditEntries []AuditEntry      `json:"auditEntries,omitempty"`
}

type InstanceView struct {
	ID                 string                `json:"id"`
	PluginID           string                `json:"pluginId,omitempty"`
	Definition         DefinitionSummary     `json:"definition"`
	Runtime            string                `json:"runtime"`
	SourceFormat       string                `json:"sourceFormat"`
	Startable          bool                  `json:"startable"`
	Binding            InstanceBinding       `json:"binding"`
	Params             map[string]any        `json:"params"`
	Status             string                `json:"status"`
	CreatedAt          string                `json:"createdAt"`
	Logs               []string              `json:"logs"`
	DefinitionSync     *DefinitionSyncStatus `json:"definitionSync,omitempty"`
	RuntimeObservation *RuntimeObservation   `json:"runtimeObservation,omitempty"`
}

type ApplyLinkedInstancesResult struct {
	DefinitionID  string   `json:"definitionId"`
	LatestVersion string   `json:"latestVersion"`
	TotalLinked   int      `json:"totalLinked"`
	Applied       []string `json:"applied"`
	AlreadyLatest []string `json:"alreadyLatest"`
	SkippedBusy   []string `json:"skippedBusy"`
}

type LogQuery struct {
	Limit  int
	Offset int
	Level  string
	FromAt *time.Time
	ToAt   *time.Time
}

type AuditQuery struct {
	Limit  int
	Offset int
	Kind   string
	FromAt *time.Time
	ToAt   *time.Time
}

type ActivityPage struct {
	Limit    int  `json:"limit"`
	Offset   int  `json:"offset"`
	Total    int  `json:"total"`
	Returned int  `json:"returned"`
	HasMore  bool `json:"hasMore"`
}

type LogsResult struct {
	InstanceID string       `json:"instanceId"`
	Logs       []string     `json:"logs"`
	Page       ActivityPage `json:"page"`
}

type AuditResult struct {
	InstanceID string       `json:"instanceId"`
	Entries    []AuditEntry `json:"entries"`
	Page       ActivityPage `json:"page"`
}

type RuntimeActiveInstanceSummary struct {
	InstanceID        string   `json:"instanceId"`
	DefinitionName    string   `json:"definitionName"`
	ActualStatus      string   `json:"actualStatus"`
	ActiveSymbols     []string `json:"activeSymbols"`
	LastClosedKLineAt *string  `json:"lastClosedKlineAt,omitempty"`
	LastSignalAt      *string  `json:"lastSignalAt,omitempty"`
	LastOrderAt       *string  `json:"lastOrderAt,omitempty"`
	LastErrorAt       *string  `json:"lastErrorAt,omitempty"`
	LastError         *string  `json:"lastError,omitempty"`
	UpdatedAt         *string  `json:"updatedAt,omitempty"`
}

type RuntimeSummary struct {
	Status                 string                         `json:"status"`
	ActiveStrategies       int                            `json:"activeStrategies"`
	SupportsBacktestParity bool                           `json:"supportsBacktestParity"`
	ActiveInstances        []RuntimeActiveInstanceSummary `json:"activeInstances"`
}

type PluginBuildTuple struct {
	JFTradeVersion string   `json:"jftradeVersion"`
	GoVersion      string   `json:"goVersion"`
	GOOS           string   `json:"goos"`
	GOARCH         string   `json:"goarch"`
	BuildMode      string   `json:"buildMode"`
	BuildTags      []string `json:"buildTags,omitempty"`
}

type PluginCompatibility struct {
	Mode            string            `json:"mode"`
	Supported       bool              `json:"supported"`
	RequiresRebuild bool              `json:"requiresRebuild"`
	Reason          *string           `json:"reason,omitempty"`
	Host            PluginBuildTuple  `json:"host"`
	Artifact        *PluginBuildTuple `json:"artifact,omitempty"`
}

type PluginDescriptor struct {
	ID          string   `json:"id"`
	Type        string   `json:"type"`
	DisplayName string   `json:"displayName"`
	Version     string   `json:"version"`
	Description string   `json:"description"`
	Keywords    []string `json:"keywords"`
}

type PluginOperation struct {
	OperationID string  `json:"operationId"`
	PluginID    string  `json:"pluginId"`
	Status      string  `json:"status"`
	Phase       string  `json:"phase"`
	Progress    int     `json:"progress"`
	Message     string  `json:"message"`
	TargetDir   string  `json:"targetDir"`
	InstallPath string  `json:"installPath"`
	StartedAt   string  `json:"startedAt"`
	UpdatedAt   string  `json:"updatedAt"`
	CompletedAt *string `json:"completedAt"`
	Error       *string `json:"error"`
}

type PluginUninstallCommands struct {
	Posix      string `json:"posix"`
	PowerShell string `json:"powershell"`
}

type PluginUninstallGuidance struct {
	PluginID string                  `json:"pluginId"`
	Path     string                  `json:"path"`
	Exists   bool                    `json:"exists"`
	Commands PluginUninstallCommands `json:"commands"`
}

type PluginInstallation struct {
	Status            string                  `json:"status"`
	Installed         bool                    `json:"installed"`
	InstallPath       string                  `json:"installPath"`
	TargetDir         string                  `json:"targetDir"`
	MarkerPath        string                  `json:"markerPath"`
	CurrentOperation  *PluginOperation        `json:"currentOperation"`
	LastOperation     *PluginOperation        `json:"lastOperation"`
	UninstallGuidance PluginUninstallGuidance `json:"uninstallGuidance"`
}

type PluginCatalogItem struct {
	Descriptor    PluginDescriptor    `json:"descriptor"`
	Installation  PluginInstallation  `json:"installation"`
	Compatibility PluginCompatibility `json:"compatibility"`
}

type PluginCatalog struct {
	TargetDir string              `json:"targetDir"`
	Plugins   []PluginCatalogItem `json:"plugins"`
}

type PineAnalysisResult map[string]any
