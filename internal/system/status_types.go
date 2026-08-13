package system

import (
	"github.com/jftrade/jftrade-main/internal/buildinfo"
	"github.com/jftrade/jftrade-main/internal/strategy"
	"github.com/jftrade/jftrade-main/internal/trading"
	"github.com/jftrade/jftrade-main/pkg/observability"
)

// Status is the domain-owned system status projection. The HTTP
// layer maps it to its compatibility DTO so the public OpenAPI schema remains
// stable while producers and consumers stay compile-time typed.
type Status struct {
	Name                      string                           `json:"name"`
	APIPort                   int                              `json:"apiPort"`
	DefaultBroker             string                           `json:"defaultBroker"`
	DefaultTradingEnvironment string                           `json:"defaultTradingEnvironment"`
	RealTradingEnabled        bool                             `json:"realTradingEnabled"`
	RealTradingKillSwitch     RealTradingKillSwitch            `json:"realTradingKillSwitch"`
	RealTradingRisk           RealTradingRisk                  `json:"realTradingRisk"`
	RealTradeAccess           RealTradeAccess                  `json:"realTradeAccess"`
	Build                     buildinfo.Information            `json:"build"`
	Persistence               Persistence                      `json:"persistence"`
	Observability             Observability                    `json:"observability"`
	RuntimeResources          RuntimeResources                 `json:"runtimeResources"`
	Broker                    *trading.BrokerRuntimeDescriptor `json:"broker,omitempty"`
	StrategyRuntime           *strategy.RuntimeSummary         `json:"strategyRuntime,omitempty"`
	Message                   string                           `json:"message"`
}

type RealTradingKillSwitch struct {
	Active            bool     `json:"active"`
	RuntimeActive     bool     `json:"runtimeActive"`
	BlockedOperations []string `json:"blockedOperations"`
	AllowsCancel      bool     `json:"allowsCancel"`
}

type RealTradingRisk struct {
	Enabled                           bool     `json:"enabled"`
	MaxOrderQuantity                  *float64 `json:"maxOrderQuantity"`
	MaxOrderNotional                  *float64 `json:"maxOrderNotional"`
	RuntimeConfiguredMaxOrderQuantity *float64 `json:"runtimeConfiguredMaxOrderQuantity"`
	RuntimeConfiguredMaxOrderNotional *float64 `json:"runtimeConfiguredMaxOrderNotional"`
	RuntimeRiskConfigured             bool     `json:"runtimeRiskConfigured"`
}

type RealTradeAccess struct {
	ApproverAllowlistEnabled bool `json:"approverAllowlistEnabled"`
	ApproverCount            int  `json:"approverCount"`
	AdminAllowlistEnabled    bool `json:"adminAllowlistEnabled"`
	AdminCount               int  `json:"adminCount"`
}

type Persistence struct {
	Engine            string   `json:"engine"`
	DatabasePath      string   `json:"databasePath"`
	Status            string   `json:"status"`
	Migrated          bool     `json:"migrated"`
	PendingMigrations []string `json:"pendingMigrations"`
	Tables            []string `json:"tables"`
	CheckedAt         string   `json:"checkedAt"`
}

type Observability struct {
	API               APIObservability                 `json:"api"`
	Live              *LiveStats                       `json:"live"`
	MarketData        *MarketDataRuntime               `json:"marketdata"`
	ExchangeCalendars *CalendarStatus                  `json:"exchangeCalendars"`
	Broker            *trading.BrokerRuntimeDescriptor `json:"broker"`
	StrategyRuntime   *strategy.RuntimeSummary         `json:"strategyRuntime"`
	Requests          observability.Snapshot           `json:"requests"`
}

type APIObservability struct {
	StartedAt string `json:"startedAt"`
	UptimeMS  int64  `json:"uptimeMs"`
}

type LiveStats struct {
	Connected         int      `json:"connected"`
	Limit             int      `json:"limit"`
	AtLimit           bool     `json:"atLimit"`
	ActiveInstruments []string `json:"activeInstruments"`
}

type MarketDataRuntime struct {
	Status          string  `json:"status"`
	Connected       bool    `json:"connected"`
	Closed          bool    `json:"closed"`
	Generation      uint64  `json:"generation"`
	ActiveCount     int     `json:"activeCount"`
	LastRefreshAt   *string `json:"lastRefreshAt"`
	QuoteRetryAt    *string `json:"quoteRetryAt"`
	QuoteFailures   int     `json:"quoteFailures"`
	QuoteLastError  *string `json:"quoteLastError"`
	StreamRetryAt   *string `json:"streamRetryAt"`
	StreamFailures  int     `json:"streamFailures"`
	StreamLastError *string `json:"streamLastError"`
}

type RuntimeResources struct {
	CheckedAt string                      `json:"checkedAt"`
	Count     int                         `json:"count"`
	Items     []RuntimeResourceDescriptor `json:"items"`
}

type RuntimeResourceDescriptor struct {
	ID                  string `json:"id"`
	Owner               string `json:"owner"`
	Kind                string `json:"kind"`
	Path                string `json:"path"`
	InitializedBy       string `json:"initializedBy"`
	SchemaOwner         string `json:"schemaOwner"`
	CloseOwner          string `json:"closeOwner"`
	HealthProvider      string `json:"healthProvider"`
	EnvironmentOverride string `json:"environmentOverride,omitempty"`
	Critical            bool   `json:"critical"`
}

type CalendarStatus struct {
	AutoRefreshEnabled   bool               `json:"autoRefreshEnabled"`
	RefreshIntervalHours int                `json:"refreshIntervalHours"`
	WarmupMarkets        []string           `json:"warmupMarkets"`
	Markets              []CalendarMarket   `json:"markets"`
	Sources              []CalendarSource   `json:"sources"`
	Snapshots            []CalendarSnapshot `json:"snapshots"`
}

type CalendarMarket struct {
	Market          string   `json:"market"`
	EffectiveSource string   `json:"effectiveSource"`
	EffectiveMode   string   `json:"effectiveMode"`
	EffectiveReason string   `json:"effectiveReason"`
	FallbackChain   []string `json:"fallbackChain"`
	CheckedAt       string   `json:"checkedAt"`
}

type CalendarSource struct {
	ID                    string   `json:"id"`
	Kind                  string   `json:"kind"`
	Authority             string   `json:"authority"`
	Markets               []string `json:"markets"`
	Enabled               bool     `json:"enabled"`
	AvailabilityNote      string   `json:"availabilityNote"`
	LastSuccessAt         string   `json:"lastSuccessAt"`
	LastFailureAt         string   `json:"lastFailureAt"`
	LastError             string   `json:"lastError"`
	ConsecutiveFailures   int      `json:"consecutiveFailures"`
	NextRefreshAt         string   `json:"nextRefreshAt"`
	LastSnapshotFetchedAt string   `json:"lastSnapshotFetchedAt"`
	LastProbeAt           string   `json:"lastProbeAt"`
	LastProbeSuccessAt    string   `json:"lastProbeSuccessAt"`
	LastProbeFailureAt    string   `json:"lastProbeFailureAt"`
	LastProbeStatus       string   `json:"lastProbeStatus"`
	LastProbeError        string   `json:"lastProbeError"`
	LastProbeMarket       string   `json:"lastProbeMarket"`
	LastProbeSchedules    int      `json:"lastProbeSchedules"`
	HealthState           string   `json:"healthState"`
	HealthFingerprint     string   `json:"healthFingerprint"`
	LastAlertAt           string   `json:"lastAlertAt"`
	LastAlertStatus       string   `json:"lastAlertStatus"`
	LastAlertFingerprint  string   `json:"lastAlertFingerprint"`
}

type CalendarSnapshot struct {
	Market          string                   `json:"market"`
	SourceID        string                   `json:"sourceId"`
	From            string                   `json:"from"`
	To              string                   `json:"to"`
	FetchedAt       string                   `json:"fetchedAt"`
	ValidUntil      string                   `json:"validUntil"`
	SchedulesParsed int                      `json:"schedulesParsed"`
	Checksum        string                   `json:"checksum"`
	SampleSchedules []CalendarSampleSchedule `json:"sampleSchedules"`
}

type CalendarSampleSchedule struct {
	Market   string                  `json:"market"`
	Date     string                  `json:"date"`
	Status   string                  `json:"status"`
	Reason   string                  `json:"reason"`
	SourceID string                  `json:"sourceId"`
	Observed bool                    `json:"observed"`
	Sessions []CalendarSessionWindow `json:"sessions"`
}

type CalendarSessionWindow struct {
	Kind        string `json:"kind"`
	StartMinute int    `json:"startMinute"`
	EndMinute   int    `json:"endMinute"`
}
