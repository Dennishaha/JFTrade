package system

import (
	"github.com/jftrade/jftrade-main/internal/strategy"
	sys "github.com/jftrade/jftrade-main/internal/system"
	"github.com/jftrade/jftrade-main/internal/trading"
	"github.com/jftrade/jftrade-main/pkg/observability"
)

type AcceptedResponse struct {
	Accepted bool `json:"accepted"`
}

type RealTradeKillSwitchRequest struct {
	TradingEnvironment string `json:"tradingEnvironment,omitempty"`
	OperatorID         string `json:"operatorId,omitempty"`
	Reason             string `json:"reason,omitempty"`
}

func (request RealTradeKillSwitchRequest) command() sys.RealTradeKillSwitchCommand {
	return sys.RealTradeKillSwitchCommand{
		TradingEnvironment: request.TradingEnvironment,
		OperatorID:         request.OperatorID,
		Reason:             request.Reason,
	}
}

type RealTradeHardStopRequest struct {
	BrokerID           string `json:"brokerId,omitempty"`
	TradingEnvironment string `json:"tradingEnvironment,omitempty"`
	AccountID          string `json:"accountId,omitempty"`
	Market             string `json:"market,omitempty"`
	Symbol             string `json:"symbol,omitempty"`
	HardStopScope      string `json:"hardStopScope,omitempty"`
	OperatorID         string `json:"operatorId,omitempty"`
	Reason             string `json:"reason,omitempty"`
}

func (request RealTradeHardStopRequest) command() sys.RealTradeHardStopCommand {
	return sys.RealTradeHardStopCommand{
		BrokerID:           request.BrokerID,
		TradingEnvironment: request.TradingEnvironment,
		AccountID:          request.AccountID,
		Market:             request.Market,
		Symbol:             request.Symbol,
		HardStopScope:      request.HardStopScope,
		OperatorID:         request.OperatorID,
		Reason:             request.Reason,
	}
}

type RealTradeRuntimeRiskRequest struct {
	TradingEnvironment string   `json:"tradingEnvironment,omitempty"`
	RealTradingEnabled bool     `json:"realTradingEnabled,omitempty"`
	MaxOrderQuantity   *float64 `json:"maxOrderQuantity,omitempty" extensions:"x-nullable"`
	MaxOrderNotional   *float64 `json:"maxOrderNotional,omitempty" extensions:"x-nullable"`
	OperatorID         string   `json:"operatorId,omitempty"`
	Reason             string   `json:"reason,omitempty"`
}

func (request RealTradeRuntimeRiskRequest) command() sys.RealTradeRuntimeRiskCommand {
	return sys.RealTradeRuntimeRiskCommand{
		TradingEnvironment: request.TradingEnvironment,
		RealTradingEnabled: request.RealTradingEnabled,
		MaxOrderQuantity:   request.MaxOrderQuantity,
		MaxOrderNotional:   request.MaxOrderNotional,
		OperatorID:         request.OperatorID,
		Reason:             request.Reason,
	}
}

type FutuOpenDHealthResponse struct {
	CheckedAt              string                     `json:"checkedAt"`
	Status                 string                     `json:"status" enums:"healthy,degraded,offline"`
	Runtime                FutuOpenDRuntime           `json:"runtime"`
	Diagnosis              FutuOpenDDiagnosis         `json:"diagnosis"`
	LocalSocketDiagnostics FutuOpenDSocketDiagnostics `json:"localSocketDiagnostics"`
	LocalInstallation      FutuOpenDLocalInstallation `json:"localInstallation"`
	LatestVersion          FutuOpenDLatestVersion     `json:"latestVersion"`
	Recommendations        []string                   `json:"recommendations"`
}

type FutuOpenDRuntime struct {
	Connectivity           string  `json:"connectivity" enums:"connected,degraded,disconnected"`
	Host                   string  `json:"host"`
	APIPort                int     `json:"apiPort"`
	WebSocketPort          int     `json:"websocketPort"`
	UseEncryption          bool    `json:"useEncryption"`
	WebSocketKeyConfigured bool    `json:"websocketKeyConfigured"`
	MarketDataTransport    string  `json:"marketDataTransport"`
	QuoteLoggedIn          *bool   `json:"quoteLoggedIn" extensions:"x-nullable"`
	TradeLoggedIn          *bool   `json:"tradeLoggedIn" extensions:"x-nullable"`
	ProgramStatus          *string `json:"programStatus" extensions:"x-nullable"`
	ServerVersion          *string `json:"serverVersion" extensions:"x-nullable"`
	MinimumVersion         string  `json:"minimumVersion"`
	LastError              *string `json:"lastError" extensions:"x-nullable"`
}

type FutuOpenDDiagnosis struct {
	Code                    string  `json:"code"`
	Summary                 *string `json:"summary" extensions:"x-nullable"`
	ManualRetryRequired     bool    `json:"manualRetryRequired"`
	RestartOpenDRecommended bool    `json:"restartOpenDRecommended"`
}

type FutuOpenDSocketDiagnostics struct {
	TransportMode                       string                   `json:"transportMode"`
	ConfiguredOpenDWebSocketLimit       int                      `json:"configuredOpenDWebSocketLimit"`
	ConfiguredOpenDWebSocketLimitActive bool                     `json:"configuredOpenDWebSocketLimitActive"`
	ConfiguredOpenDWebSocketLimitScope  string                   `json:"configuredOpenDWebSocketLimitScope"`
	WebSocketEstablishedConnections     int                      `json:"websocketEstablishedConnections"`
	JFTradeLiveWebSocketLimit           int                      `json:"jftradeLiveWebSocketLimit"`
	JFTradeLiveWebSocketAtLimit         bool                     `json:"jftradeLiveWebSocketAtLimit"`
	LikelyConnectionSaturation          bool                     `json:"likelyConnectionSaturation"`
	OpenDWebSocketPoolLikelySaturation  bool                     `json:"openDWebSocketPoolLikelySaturation"`
	LiveQuoteBackoffActive              bool                     `json:"liveQuoteBackoffActive"`
	LiveQuoteRetryAfter                 *string                  `json:"liveQuoteRetryAfter" extensions:"x-nullable"`
	LiveQuoteFailureCount               int                      `json:"liveQuoteFailureCount"`
	LiveQuoteLastError                  *string                  `json:"liveQuoteLastError" extensions:"x-nullable"`
	LiveStreamBackoffActive             bool                     `json:"liveStreamBackoffActive"`
	LiveStreamRetryAfter                *string                  `json:"liveStreamRetryAfter" extensions:"x-nullable"`
	LiveStreamFailureCount              int                      `json:"liveStreamFailureCount"`
	LiveStreamLastError                 *string                  `json:"liveStreamLastError" extensions:"x-nullable"`
	TopClientProcesses                  []FutuOpenDClientProcess `json:"topClientProcesses"`
}

type FutuOpenDClientProcess struct {
	ProcessName            string `json:"processName"`
	PID                    int    `json:"pid"`
	EstablishedConnections int    `json:"establishedConnections"`
}

type FutuOpenDLocalInstallation struct {
	Platform    string                       `json:"platform"`
	Installed   bool                         `json:"installed"`
	Version     *string                      `json:"version" extensions:"x-nullable"`
	InstallPath *string                      `json:"installPath" extensions:"x-nullable"`
	GUIDetected bool                         `json:"guiDetected"`
	Process     FutuOpenDLocalProcessDetails `json:"process"`
}

type FutuOpenDLocalProcessDetails struct {
	Running        bool    `json:"running"`
	PID            *int    `json:"pid" extensions:"x-nullable"`
	ExecutablePath *string `json:"executablePath" extensions:"x-nullable"`
}

type FutuOpenDLatestVersion struct {
	Value     *string `json:"value" extensions:"x-nullable"`
	SourceURL *string `json:"sourceUrl" extensions:"x-nullable"`
	CheckedAt *string `json:"checkedAt" extensions:"x-nullable"`
	Status    string  `json:"status" enums:"unknown,not_installed,up_to_date,outdated,ahead_of_latest"`
	Error     *string `json:"error" extensions:"x-nullable"`
}

type FutuOpenDInstallGuideResponse struct {
	BrokerID    string                   `json:"brokerId"`
	Title       string                   `json:"title"`
	Description string                   `json:"description"`
	Options     []FutuOpenDInstallOption `json:"options"`
	NextSteps   []string                 `json:"nextSteps"`
	Settings    FutuOpenDInstallSettings `json:"settings"`
}

type FutuOpenDInstallOption struct {
	ID          string `json:"id" enums:"gui,command-line"`
	Label       string `json:"label"`
	Description string `json:"description"`
	URL         string `json:"url"`
	Recommended bool   `json:"recommended"`
}

type FutuOpenDInstallSettings struct {
	Host                    string `json:"host"`
	APIPort                 int    `json:"apiPort"`
	WebSocketPort           int    `json:"websocketPort"`
	MaxWebSocketConnections int    `json:"maxWebSocketConnections"`
	UseEncryption           bool   `json:"useEncryption"`
	WebSocketKeyRequired    bool   `json:"websocketKeyRequired"`
	MarketDataTransport     string `json:"marketDataTransport"`
	MinimumVersion          string `json:"minimumVersion"`
}

type RuntimeDependenciesResponse struct {
	CheckedAt            string                  `json:"checkedAt"`
	AllRequiredSatisfied bool                    `json:"allRequiredSatisfied"`
	Dependencies         []RuntimeDependencyItem `json:"dependencies"`
}

type RuntimeDependencyItem struct {
	ID              string   `json:"id"`
	DisplayName     string   `json:"displayName"`
	Required        bool     `json:"required"`
	Status          string   `json:"status"`
	MinimumVersion  string   `json:"minimumVersion"`
	DetectedVersion string   `json:"detectedVersion"`
	ConfiguredPath  string   `json:"configuredPath"`
	EffectivePath   string   `json:"effectivePath"`
	ResolvedPath    string   `json:"resolvedPath"`
	AttemptedPaths  []string `json:"attemptedPaths"`
	Source          string   `json:"source"`
	HomepageURL     string   `json:"homepageUrl"`
	Message         string   `json:"message"`
}

type SystemStatusResponse struct {
	Name                      string                           `json:"name"`
	APIPort                   int                              `json:"apiPort"`
	DefaultBroker             string                           `json:"defaultBroker"`
	DefaultTradingEnvironment string                           `json:"defaultTradingEnvironment"`
	RealTradingEnabled        bool                             `json:"realTradingEnabled"`
	RealTradingKillSwitch     SystemRealTradingKillSwitch      `json:"realTradingKillSwitch"`
	RealTradingRisk           SystemRealTradingRisk            `json:"realTradingRisk"`
	RealTradeAccess           SystemRealTradeAccess            `json:"realTradeAccess"`
	Build                     SystemBuildInformation           `json:"build"`
	Persistence               SystemPersistence                `json:"persistence"`
	Observability             SystemObservability              `json:"observability"`
	RuntimeResources          SystemRuntimeResources           `json:"runtimeResources"`
	Broker                    *trading.BrokerRuntimeDescriptor `json:"broker,omitempty"`
	StrategyRuntime           *strategy.RuntimeSummary         `json:"strategyRuntime,omitempty"`
	Message                   string                           `json:"message"`
}

type SystemRealTradingKillSwitch struct {
	Active            bool     `json:"active"`
	RuntimeActive     bool     `json:"runtimeActive"`
	BlockedOperations []string `json:"blockedOperations"`
	AllowsCancel      bool     `json:"allowsCancel"`
}

type SystemRealTradingRisk struct {
	Enabled                           bool     `json:"enabled"`
	MaxOrderQuantity                  *float64 `json:"maxOrderQuantity" extensions:"x-nullable"`
	MaxOrderNotional                  *float64 `json:"maxOrderNotional" extensions:"x-nullable"`
	RuntimeConfiguredMaxOrderQuantity *float64 `json:"runtimeConfiguredMaxOrderQuantity" extensions:"x-nullable"`
	RuntimeConfiguredMaxOrderNotional *float64 `json:"runtimeConfiguredMaxOrderNotional" extensions:"x-nullable"`
	RuntimeRiskConfigured             bool     `json:"runtimeRiskConfigured"`
}

type SystemRealTradeAccess struct {
	ApproverAllowlistEnabled bool `json:"approverAllowlistEnabled"`
	ApproverCount            int  `json:"approverCount"`
	AdminAllowlistEnabled    bool `json:"adminAllowlistEnabled"`
	AdminCount               int  `json:"adminCount"`
}

type SystemBuildInformation struct {
	Version   string `json:"version"`
	Commit    string `json:"commit"`
	BuildTime string `json:"buildTime"`
	GOOS      string `json:"goos"`
	GOARCH    string `json:"goarch"`
}

type SystemPersistence struct {
	Engine            string   `json:"engine"`
	DatabasePath      string   `json:"databasePath"`
	Status            string   `json:"status"`
	Migrated          bool     `json:"migrated"`
	PendingMigrations []string `json:"pendingMigrations"`
	Tables            []string `json:"tables"`
	CheckedAt         string   `json:"checkedAt"`
}

type SystemObservability struct {
	API               map[string]any         `json:"api"`
	Live              map[string]any         `json:"live"`
	MarketData        map[string]any         `json:"marketdata"`
	ExchangeCalendars map[string]any         `json:"exchangeCalendars"`
	Broker            map[string]any         `json:"broker"`
	StrategyRuntime   map[string]any         `json:"strategyRuntime"`
	Requests          observability.Snapshot `json:"requests"`
}

type SystemRuntimeResources struct {
	CheckedAt string                            `json:"checkedAt"`
	Count     int                               `json:"count"`
	Items     []SystemRuntimeResourceDescriptor `json:"items"`
}

type SystemRuntimeResourceDescriptor struct {
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

type ExchangeCalendarStatusResponse struct {
	AutoRefreshEnabled   bool                       `json:"autoRefreshEnabled,omitempty"`
	RefreshIntervalHours int                        `json:"refreshIntervalHours,omitempty"`
	WarmupMarkets        []string                   `json:"warmupMarkets,omitempty"`
	Markets              []ExchangeCalendarMarket   `json:"markets,omitempty"`
	Sources              []ExchangeCalendarSource   `json:"sources,omitempty"`
	Snapshots            []ExchangeCalendarSnapshot `json:"snapshots,omitempty"`
}

type ExchangeCalendarMarket struct {
	Market          string   `json:"market"`
	EffectiveSource string   `json:"effectiveSource"`
	EffectiveMode   string   `json:"effectiveMode"`
	EffectiveReason string   `json:"effectiveReason"`
	FallbackChain   []string `json:"fallbackChain"`
	CheckedAt       string   `json:"checkedAt"`
}

type ExchangeCalendarSource struct {
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

type ExchangeCalendarSnapshot struct {
	Market          string                           `json:"market"`
	SourceID        string                           `json:"sourceId"`
	From            string                           `json:"from"`
	To              string                           `json:"to"`
	FetchedAt       string                           `json:"fetchedAt"`
	ValidUntil      string                           `json:"validUntil"`
	SchedulesParsed int                              `json:"schedulesParsed"`
	Checksum        string                           `json:"checksum"`
	SampleSchedules []ExchangeCalendarSampleSchedule `json:"sampleSchedules"`
}

type ExchangeCalendarSampleSchedule struct {
	Market   string                          `json:"market"`
	Date     string                          `json:"date"`
	Status   string                          `json:"status"`
	Reason   string                          `json:"reason"`
	SourceID string                          `json:"sourceId"`
	Observed bool                            `json:"observed"`
	Sessions []ExchangeCalendarSessionWindow `json:"sessions"`
}

type ExchangeCalendarSessionWindow struct {
	Kind        string `json:"kind"`
	StartMinute int    `json:"startMinute"`
	EndMinute   int    `json:"endMinute"`
}

type ExchangeCalendarSourcesResponse struct {
	Sources []ExchangeCalendarSource `json:"sources"`
}

type ExchangeCalendarRefreshResponse struct {
	Accepted      bool     `json:"accepted"`
	Market        string   `json:"market,omitempty"`
	Updated       int      `json:"updated,omitempty"`
	Failures      int      `json:"failures,omitempty"`
	RequestedAt   string   `json:"requestedAt,omitempty"`
	WarmupMarkets []string `json:"warmupMarkets,omitempty"`
	Reason        string   `json:"reason,omitempty"`
}

type ExchangeCalendarProbeResponse struct {
	Accepted   bool                        `json:"accepted"`
	Market     string                      `json:"market,omitempty"`
	CheckedAt  string                      `json:"checkedAt,omitempty"`
	Healthy    int                         `json:"healthy,omitempty"`
	Failures   int                         `json:"failures,omitempty"`
	Results    []ExchangeCalendarProbeItem `json:"results,omitempty"`
	ProbeScope []string                    `json:"probeScope,omitempty"`
	Reason     string                      `json:"reason,omitempty"`
}

type ExchangeCalendarProbeItem struct {
	SourceID        string `json:"sourceId"`
	Market          string `json:"market"`
	Status          string `json:"status"`
	Error           string `json:"error,omitempty"`
	FetchedAt       string `json:"fetchedAt,omitempty"`
	ValidUntil      string `json:"validUntil,omitempty"`
	SchedulesParsed int    `json:"schedulesParsed,omitempty"`
	Checksum        string `json:"checksum,omitempty"`
}

type StorageOverviewResponse struct {
	PendingOutbox           []any `json:"pendingOutbox"`
	RecentJobs              []any `json:"recentJobs"`
	RecentAuditLogs         []any `json:"recentAuditLogs"`
	RecentExecutionCommands []any `json:"recentExecutionCommands"`
}

type BrokerOrderUpdatesResponse struct {
	Subscriptions       []map[string]any `json:"subscriptions"`
	RecentInvalidations []map[string]any `json:"recentInvalidations"`
	Brokers             []map[string]any `json:"brokers"`
	Runtime             map[string]any   `json:"runtime"`
}
