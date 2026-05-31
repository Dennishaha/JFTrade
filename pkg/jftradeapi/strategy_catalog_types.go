package jftradeapi

import (
	"errors"
	"sync"

	"github.com/jmoiron/sqlx"
)

const (
	defaultStrategyCatalogFilename  = "strategy-catalog.json"
	defaultStrategyPluginDirName    = "plugins"
	strategyCatalogMetaTable        = "strategy_catalog_meta"
	strategyCatalogPluginTable      = "strategy_catalog_plugins"
	strategyCatalogStrategyTable    = "strategy_catalog_strategies"
	strategyCatalogOperationTable   = "strategy_catalog_operations"
	pluginTypeGoStrategy            = "strategy-go-plugin"
	pluginBuildMode                 = "plugin"
	strategyStatusRunning           = "RUNNING"
	strategyStatusPaused            = "PAUSED"
	strategyStatusStopped           = "STOPPED"
	strategyExecutionModeLive       = "live"
	strategyExecutionModeNotifyOnly = "notify_only"
)

var errStrategyInstanceBusy = errors.New("strategy instance must be stopped before modification")

type strategyPluginBuildTuple struct {
	JFTradeVersion string   `json:"jftradeVersion"`
	GoVersion      string   `json:"goVersion"`
	GOOS           string   `json:"goos"`
	GOARCH         string   `json:"goarch"`
	BuildMode      string   `json:"buildMode"`
	BuildTags      []string `json:"buildTags,omitempty"`
}

type strategyPluginArtifact struct {
	Path  string                   `json:"path"`
	Build strategyPluginBuildTuple `json:"build"`
}

type strategyPluginCompatibility struct {
	Mode            string                    `json:"mode"`
	Supported       bool                      `json:"supported"`
	RequiresRebuild bool                      `json:"requiresRebuild"`
	Reason          *string                   `json:"reason,omitempty"`
	Host            strategyPluginBuildTuple  `json:"host"`
	Artifact        *strategyPluginBuildTuple `json:"artifact,omitempty"`
}

type strategyPluginDescriptor struct {
	ID          string   `json:"id"`
	Type        string   `json:"type"`
	DisplayName string   `json:"displayName"`
	Version     string   `json:"version"`
	Description string   `json:"description"`
	Keywords    []string `json:"keywords"`
}

type strategyPluginOperation struct {
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

type strategyPluginUninstallGuidance struct {
	PluginID string `json:"pluginId"`
	Path     string `json:"path"`
	Exists   bool   `json:"exists"`
	Commands struct {
		Posix      string `json:"posix"`
		PowerShell string `json:"powershell"`
	} `json:"commands"`
}

type strategyPluginInstallation struct {
	Status            string                          `json:"status"`
	Installed         bool                            `json:"installed"`
	InstallPath       string                          `json:"installPath"`
	TargetDir         string                          `json:"targetDir"`
	MarkerPath        string                          `json:"markerPath"`
	CurrentOperation  *strategyPluginOperation        `json:"currentOperation"`
	LastOperation     *strategyPluginOperation        `json:"lastOperation"`
	UninstallGuidance strategyPluginUninstallGuidance `json:"uninstallGuidance"`
}

type strategyPluginCatalogItem struct {
	Descriptor    strategyPluginDescriptor    `json:"descriptor"`
	Installation  strategyPluginInstallation  `json:"installation"`
	Compatibility strategyPluginCompatibility `json:"compatibility"`
}

type strategyPluginCatalogResponse struct {
	TargetDir string                      `json:"targetDir"`
	Plugins   []strategyPluginCatalogItem `json:"plugins"`
}

type strategyDefinitionSummary struct {
	StrategyID string `json:"strategyId"`
	Name       string `json:"name"`
	Version    string `json:"version"`
}

type strategyAuditEntry struct {
	InstanceID string `json:"instanceId"`
	Kind       string `json:"kind"`
	Detail     string `json:"detail,omitempty"`
	At         string `json:"at"`
}

type strategyBrokerAccountBinding struct {
	BrokerID           string `json:"brokerId"`
	AccountID          string `json:"accountId"`
	TradingEnvironment string `json:"tradingEnvironment"`
	Market             string `json:"market"`
}

type strategyBindingInstrument struct {
	Market string `json:"market"`
	Code   string `json:"code"`
}

type strategyInstanceBinding struct {
	Instruments   []strategyBindingInstrument   `json:"instruments,omitempty"`
	Symbols       []string                      `json:"symbols"`
	Interval      string                        `json:"interval"`
	ExecutionMode string                        `json:"executionMode"`
	BrokerAccount *strategyBrokerAccountBinding `json:"brokerAccount,omitempty"`
}

type strategyRuntimeObservation struct {
	ActualStatus      string   `json:"actualStatus"`
	ActiveSymbols     []string `json:"activeSymbols"`
	LastClosedKLineAt *string  `json:"lastClosedKlineAt,omitempty"`
	LastSignalAt      *string  `json:"lastSignalAt,omitempty"`
	LastOrderAt       *string  `json:"lastOrderAt,omitempty"`
	LastErrorAt       *string  `json:"lastErrorAt,omitempty"`
	LastError         *string  `json:"lastError,omitempty"`
	UpdatedAt         *string  `json:"updatedAt,omitempty"`
}

type strategyRuntimeActiveInstanceSummary struct {
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

type strategyDefinitionSyncStatus struct {
	DefinitionID   string  `json:"definitionId"`
	AppliedVersion string  `json:"appliedVersion"`
	LatestVersion  string  `json:"latestVersion"`
	IsLatest       bool    `json:"isLatest"`
	CanApplyLatest bool    `json:"canApplyLatest"`
	BlockedReason  *string `json:"blockedReason,omitempty"`
}

type strategyApplyLinkedInstancesResponse struct {
	DefinitionID  string   `json:"definitionId"`
	LatestVersion string   `json:"latestVersion"`
	TotalLinked   int      `json:"totalLinked"`
	Applied       []string `json:"applied"`
	AlreadyLatest []string `json:"alreadyLatest"`
	SkippedBusy   []string `json:"skippedBusy"`
}

type strategyListItem struct {
	ID                 string                        `json:"id"`
	PluginID           string                        `json:"pluginId,omitempty"`
	Definition         strategyDefinitionSummary     `json:"definition"`
	Runtime            string                        `json:"runtime"`
	SourceFormat       string                        `json:"sourceFormat"`
	Startable          bool                          `json:"startable"`
	Binding            strategyInstanceBinding       `json:"binding"`
	Params             map[string]any                `json:"params"`
	Status             string                        `json:"status"`
	CreatedAt          string                        `json:"createdAt"`
	Logs               []string                      `json:"logs"`
	DefinitionSync     *strategyDefinitionSyncStatus `json:"definitionSync,omitempty"`
	RuntimeObservation *strategyRuntimeObservation   `json:"runtimeObservation,omitempty"`
}

type strategyLogsResponse struct {
	InstanceID string               `json:"instanceId"`
	Logs       []string             `json:"logs"`
	Page       strategyActivityPage `json:"page"`
}

type strategyAuditResponse struct {
	InstanceID string               `json:"instanceId"`
	Entries    []strategyAuditEntry `json:"entries"`
	Page       strategyActivityPage `json:"page"`
}

type strategyActivityPage struct {
	Limit    int  `json:"limit"`
	Offset   int  `json:"offset"`
	Total    int  `json:"total"`
	Returned int  `json:"returned"`
	HasMore  bool `json:"hasMore"`
}

type managedStrategyPlugin struct {
	Descriptor   strategyPluginDescriptor   `json:"descriptor"`
	Artifact     *strategyPluginArtifact    `json:"artifact,omitempty"`
	Installation strategyPluginInstallation `json:"installation"`
}

type managedStrategyInstance struct {
	ID           string                    `json:"id"`
	PluginID     string                    `json:"pluginId,omitempty"`
	Definition   strategyDefinitionSummary `json:"definition"`
	Binding      strategyInstanceBinding   `json:"binding"`
	Params       map[string]any            `json:"params"`
	Status       string                    `json:"status"`
	CreatedAt    string                    `json:"createdAt"`
	Logs         []string                  `json:"logs,omitempty"`
	AuditEntries []strategyAuditEntry      `json:"auditEntries,omitempty"`
}

type strategyCatalogFile struct {
	TargetDir  string                    `json:"targetDir,omitempty"`
	Plugins    []managedStrategyPlugin   `json:"plugins,omitempty"`
	Strategies []managedStrategyInstance `json:"strategies,omitempty"`
	Operations []strategyPluginOperation `json:"operations,omitempty"`
}

type strategyCatalogStore struct {
	path         string
	dbPath       string
	db           *sqlx.DB
	targetDir    string
	runtimeStore *strategyRuntimeStore
	mu           sync.RWMutex
	data         strategyCatalogFile
}
