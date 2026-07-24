package servercore

import (
	"context"
	"database/sql"
	"errors"
	"sync"

	"github.com/jftrade/jftrade-main/internal/store/sqliteconn"
	stratsrv "github.com/jftrade/jftrade-main/internal/strategy"
	runtimeactivity "github.com/jftrade/jftrade-main/internal/strategy/runtimeactivity"
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

type strategyPluginArtifact struct {
	Path  string                    `json:"path"`
	Build stratsrv.PluginBuildTuple `json:"build"`
}

type managedStrategyPlugin struct {
	Descriptor   stratsrv.PluginDescriptor   `json:"descriptor"`
	Artifact     *strategyPluginArtifact     `json:"artifact,omitempty"`
	Installation stratsrv.PluginInstallation `json:"installation"`
}

type strategyCatalogFile struct {
	TargetDir  string                     `json:"targetDir,omitempty"`
	Plugins    []managedStrategyPlugin    `json:"plugins,omitempty"`
	Strategies []stratsrv.ManagedInstance `json:"strategies,omitempty"`
	Operations []stratsrv.PluginOperation `json:"operations,omitempty"`
}

type strategyCatalogStore struct {
	path         string
	dbPath       string
	db           *sqliteconn.DB
	targetDir    string
	runtimeStore *runtimeactivity.Store
	beginPersist func(context.Context, *sql.TxOptions) (executionMigrationTx, error)
	marshalJSON  func(any) ([]byte, error)
	mu           sync.RWMutex
	data         strategyCatalogFile
}
