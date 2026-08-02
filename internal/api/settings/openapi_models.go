package settings

import (
	jfsettings "github.com/jftrade/jftrade-main/internal/jftsettings"
	"github.com/jftrade/jftrade-main/internal/live"
)

type UIAppearanceResponse struct {
	Appearance jfsettings.UIAppearanceSettings `json:"appearance"`
}

type ExchangeCalendarSettingsResponse struct {
	ExchangeCalendars jfsettings.ExchangeCalendarSettings `json:"exchangeCalendars"`
}

type MarketDataProviderSettingsResponse struct {
	ActiveProvider jfsettings.ActiveMarketDataProvider `json:"activeProvider" enums:"futu,yfinance"`
}

type OnboardingReason struct {
	Code     string `json:"code"`
	Severity string `json:"severity"`
	Message  string `json:"message"`
}

type OnboardingBroker struct {
	Descriptor map[string]any `json:"descriptor"`
	Enabled    bool           `json:"enabled"`
	Available  bool           `json:"available"`
	Configured bool           `json:"configured"`
}

type OnboardingStateResponse struct {
	State               jfsettings.OnboardingSettings `json:"state"`
	ShouldShowOOBE      bool                          `json:"shouldShowOobe"`
	Reasons             []OnboardingReason            `json:"reasons"`
	RecommendedBrokerID string                        `json:"recommendedBrokerId"`
	Brokers             []OnboardingBroker            `json:"brokers"`
}

type BrokerSettingsBroker struct {
	Descriptor  map[string]any                   `json:"descriptor"`
	Integration *jfsettings.BrokerIntegration    `json:"integration" extensions:"x-nullable"`
	Defaults    jfsettings.FutuIntegrationConfig `json:"defaults"`
}

type BrokerSettingsResponse struct {
	Brokers  []BrokerSettingsBroker            `json:"brokers"`
	Accounts []jfsettings.ManagedBrokerAccount `json:"accounts"`
}

type DeletedResourceResponse struct {
	Deleted bool   `json:"deleted"`
	ID      string `json:"id"`
}

type SystemNotificationEvent struct {
	Type     string `json:"type"`
	ID       string `json:"id"`
	At       string `json:"at"`
	Level    string `json:"level"`
	Title    string `json:"title"`
	Message  string `json:"message,omitempty"`
	Source   string `json:"source"`
	BrokerID string `json:"brokerId"`
	Category string `json:"category"`
}

type SystemNotificationTestResponse struct {
	Event    SystemNotificationEvent   `json:"event"`
	Delivery live.NotificationDelivery `json:"delivery"`
}

type DataManagementOverviewResponse struct {
	Databases []DataManagementDatabaseOverview `json:"databases"`
	Totals    DataManagementOverviewTotals     `json:"totals"`
	CheckedAt string                           `json:"checkedAt"`
}

type DataManagementDatabaseOverview struct {
	ID               string                        `json:"id"`
	Name             string                        `json:"name"`
	Path             string                        `json:"path"`
	Description      string                        `json:"description"`
	Features         []string                      `json:"features"`
	ExpectedVersion  int                           `json:"expectedVersion"`
	Status           string                        `json:"status"`
	CurrentVersion   *int                          `json:"currentVersion" extensions:"x-nullable"`
	Error            string                        `json:"error,omitempty"`
	RebuildScheduled bool                          `json:"rebuildScheduled"`
	RestartRequired  bool                          `json:"restartRequired"`
	ConfirmationText string                        `json:"confirmationText"`
	Storage          DataManagementStorageStats    `json:"storage"`
	Cleanable        []DataManagementCleanableItem `json:"cleanable"`
}

type DataManagementStorageStats struct {
	MainBytes        int64  `json:"mainBytes"`
	WALBytes         int64  `json:"walBytes"`
	SHMBytes         int64  `json:"shmBytes"`
	TotalBytes       int64  `json:"totalBytes"`
	FreePageBytes    int64  `json:"freePageBytes"`
	ReclaimableBytes int64  `json:"reclaimableBytes"`
	Error            string `json:"error,omitempty"`
}

type DataManagementCleanableItem struct {
	Kind           string `json:"kind"`
	Label          string `json:"label"`
	Count          int    `json:"count"`
	EstimatedBytes int64  `json:"estimatedBytes"`
}

type DataManagementOverviewTotals struct {
	MainBytes        int64 `json:"mainBytes"`
	WALBytes         int64 `json:"walBytes"`
	SHMBytes         int64 `json:"shmBytes"`
	TotalBytes       int64 `json:"totalBytes"`
	ReclaimableBytes int64 `json:"reclaimableBytes"`
}

type DataCleanupPreviewResponse struct {
	PreviewID        string                        `json:"previewId"`
	ExpiresAt        string                        `json:"expiresAt"`
	Kind             string                        `json:"kind"`
	DatabaseID       string                        `json:"databaseId"`
	CandidateCount   int                           `json:"candidateCount"`
	EstimatedBytes   int64                         `json:"estimatedBytes"`
	Items            []DataManagementCleanableItem `json:"items"`
	ConfirmationText string                        `json:"confirmationText"`
	WillCompact      bool                          `json:"willCompact"`
}

type DataCleanupResult struct {
	DatabaseID     string `json:"databaseId"`
	DeletedCount   int    `json:"deletedCount"`
	EstimatedBytes int64  `json:"estimatedBytes"`
	BeforeBytes    int64  `json:"beforeBytes"`
	AfterBytes     int64  `json:"afterBytes"`
	ReclaimedBytes int64  `json:"reclaimedBytes"`
	Compacted      bool   `json:"compacted"`
	Warning        string `json:"warning,omitempty"`
}

type DatabaseCompactResponse struct {
	DatabaseID     string `json:"databaseId"`
	BeforeBytes    int64  `json:"beforeBytes"`
	AfterBytes     int64  `json:"afterBytes"`
	ReclaimedBytes int64  `json:"reclaimedBytes"`
	Compacted      bool   `json:"compacted"`
}

type DatabaseRebuildResponse struct {
	DatabaseIDs     []string `json:"databaseIds"`
	RestartRequired bool     `json:"restartRequired"`
	Scheduled       bool     `json:"scheduled"`
}

// UIAppearanceSettingsWriteRequest documents UI appearance writes.
type UIAppearanceSettingsWriteRequest struct {
	Appearance jfsettings.UIAppearanceSettings `json:"appearance"`
}

// OnboardingWriteRequest documents onboarding state writes.
type OnboardingWriteRequest struct {
	Completed    bool   `json:"completed"`
	Dismissed    bool   `json:"dismissed,omitempty"`
	LastBrokerID string `json:"lastBrokerId,omitempty"`
}

// ExchangeCalendarSettingsWriteRequest documents exchange calendar writes.
type ExchangeCalendarSettingsWriteRequest struct {
	ExchangeCalendars jfsettings.ExchangeCalendarSettings `json:"exchangeCalendars"`
}

// BrokerIntegrationSaveRequest documents broker integration writes.
type BrokerIntegrationSaveRequest struct {
	Enabled bool                             `json:"enabled"`
	Config  jfsettings.FutuIntegrationConfig `json:"config"`
}

// ManagedBrokerAccountWriteRequest documents managed account writes. Server-owned
// fields such as id, createdAt, and updatedAt are intentionally omitted.
type ManagedBrokerAccountWriteRequest struct {
	BrokerID           string `json:"brokerId"`
	AccountID          string `json:"accountId"`
	DisplayName        string `json:"displayName"`
	TradingEnvironment string `json:"tradingEnvironment"`
	Market             string `json:"market"`
	SecurityFirm       string `json:"securityFirm,omitempty"`
	Enabled            bool   `json:"enabled"`
}

// ADKRuntimeSettingsWriteRequest is the independent wire contract for ADK
// runtime settings writes.
type ADKRuntimeSettingsWriteRequest struct {
	RunTimeoutMs        int `json:"runTimeoutMs"`
	StreamIdleTimeoutMs int `json:"streamIdleTimeoutMs"`
}

func (request ADKRuntimeSettingsWriteRequest) settings() jfsettings.ADKRuntimeSettings {
	return jfsettings.ADKRuntimeSettings{
		RunTimeoutMs:        request.RunTimeoutMs,
		StreamIdleTimeoutMs: request.StreamIdleTimeoutMs,
	}
}

// ExecutionSettingsWriteRequest is the independent wire contract for
// execution settings writes.
type ExecutionSettingsWriteRequest struct {
	DefaultTradingEnvironment      string `json:"defaultTradingEnvironment"`
	BrokerOrderHistoryLookbackDays int    `json:"brokerOrderHistoryLookbackDays"`
	SeenFillRetentionDays          int    `json:"seenFillRetentionDays"`
}

func (request ExecutionSettingsWriteRequest) settings() jfsettings.ExecutionSettings {
	return jfsettings.ExecutionSettings{
		DefaultTradingEnvironment:      request.DefaultTradingEnvironment,
		BrokerOrderHistoryLookbackDays: request.BrokerOrderHistoryLookbackDays,
		SeenFillRetentionDays:          request.SeenFillRetentionDays,
	}
}

// PineWorkerSettingsWriteRequest is the independent wire contract for PineTS
// worker settings writes.
type PineWorkerSettingsWriteRequest struct {
	BacktestWorkerLimit int    `json:"backtestWorkerLimit"`
	InstanceWorkerLimit int    `json:"instanceWorkerLimit"`
	NodeBinaryPath      string `json:"nodeBinaryPath"`
}

func (request PineWorkerSettingsWriteRequest) settings() jfsettings.PineWorkerSettings {
	return jfsettings.PineWorkerSettings{
		BacktestWorkerLimit: request.BacktestWorkerLimit,
		InstanceWorkerLimit: request.InstanceWorkerLimit,
		NodeBinaryPath:      request.NodeBinaryPath,
	}
}

// RuntimeDependencySettingsWriteRequest is the independent write contract
// for user-selected host runtime paths.
type RuntimeDependencySettingsWriteRequest struct {
	PythonBinaryPath string `json:"pythonBinaryPath"`
}

func (request RuntimeDependencySettingsWriteRequest) settings() jfsettings.RuntimeDependencySettings {
	return jfsettings.RuntimeDependencySettings{PythonBinaryPath: request.PythonBinaryPath}
}

// MarketDataProviderWriteRequest selects the active market-data source.
type MarketDataProviderWriteRequest struct {
	ActiveProvider jfsettings.ActiveMarketDataProvider `json:"activeProvider" enums:"futu,yfinance"`
}

// SystemNotificationSettingsWriteRequest is the independent wire contract for
// system notification settings writes.
type SystemNotificationSettingsWriteRequest struct {
	Enabled      bool     `json:"enabled"`
	Mode         string   `json:"mode"`
	Levels       []string `json:"levels,omitempty"`
	Categories   []string `json:"categories,omitempty"`
	SoundEnabled bool     `json:"soundEnabled"`
}

func (request SystemNotificationSettingsWriteRequest) settings() jfsettings.SystemNotificationSettings {
	return jfsettings.SystemNotificationSettings{
		Enabled:      request.Enabled,
		Mode:         request.Mode,
		Levels:       request.Levels,
		Categories:   request.Categories,
		SoundEnabled: request.SoundEnabled,
	}
}
