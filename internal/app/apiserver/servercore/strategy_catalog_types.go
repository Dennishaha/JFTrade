package servercore

import (
	"errors"
	"sync"

	stratsrv "github.com/jftrade/jftrade-main/internal/strategy"
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

type strategyPluginBuildTuple = stratsrv.PluginBuildTuple

type strategyPluginArtifact struct {
	Path  string                   `json:"path"`
	Build strategyPluginBuildTuple `json:"build"`
}

type strategyPluginCompatibility = stratsrv.PluginCompatibility

type strategyPluginDescriptor = stratsrv.PluginDescriptor

type strategyPluginOperation = stratsrv.PluginOperation

type strategyPluginUninstallGuidance = stratsrv.PluginUninstallGuidance

type strategyPluginInstallation = stratsrv.PluginInstallation

type strategyPluginCatalogItem = stratsrv.PluginCatalogItem

type strategyPluginCatalogResponse = stratsrv.PluginCatalog

type strategyDefinitionSummary = stratsrv.DefinitionSummary

type strategyAuditEntry = stratsrv.AuditEntry

type strategyBrokerAccountBinding = stratsrv.BrokerAccountBinding

type strategyBindingInstrument = stratsrv.BindingInstrument

type strategyInstanceBinding = stratsrv.InstanceBinding

type strategyRuntimeRiskSettings = stratsrv.RuntimeRiskSettings

type strategyRuntimeObservation = stratsrv.RuntimeObservation

type strategyRuntimeActiveInstanceSummary = stratsrv.RuntimeActiveInstanceSummary

type strategyDefinitionSyncStatus = stratsrv.DefinitionSyncStatus

type strategyApplyLinkedInstancesResponse = stratsrv.ApplyLinkedInstancesResult

type strategyListItem = stratsrv.InstanceView

type strategyLogsResponse = stratsrv.LogsResult

type strategyAuditResponse = stratsrv.AuditResult

type strategyActivityPage = stratsrv.ActivityPage

type managedStrategyPlugin struct {
	Descriptor   strategyPluginDescriptor   `json:"descriptor"`
	Artifact     *strategyPluginArtifact    `json:"artifact,omitempty"`
	Installation strategyPluginInstallation `json:"installation"`
}

type managedStrategyInstance = stratsrv.ManagedInstance

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
