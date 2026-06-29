// Package jftsettings defines shared settings types used by the settings
// service, API transport, persistence, and application assembly layers.
package jftsettings

import "encoding/json"

// FutuIntegrationConfig holds Futu OpenD connection parameters.
type FutuIntegrationConfig struct {
	Type                    string `json:"type"`
	Host                    string `json:"host"`
	APIPort                 int    `json:"apiPort"`
	WebSocketPort           int    `json:"websocketPort"`
	MaxWebSocketConnections int    `json:"maxWebSocketConnections"`
	UseEncryption           bool   `json:"useEncryption"`
	WebSocketKey            string `json:"websocketKey"`
	TradeMarket             string `json:"tradeMarket"`
	SecurityFirm            string `json:"securityFirm"`
}

// BrokerIntegration describes a broker integration stored in settings.
type BrokerIntegration struct {
	BrokerID  string                `json:"brokerId"`
	Enabled   bool                  `json:"enabled"`
	Config    FutuIntegrationConfig `json:"config"`
	UpdatedAt string                `json:"updatedAt"`
	CreatedAt string                `json:"createdAt"`
}

// ManagedBrokerAccount represents a managed broker account record.
type ManagedBrokerAccount struct {
	ID                 string  `json:"id"`
	BrokerID           string  `json:"brokerId"`
	AccountID          string  `json:"accountId"`
	DisplayName        string  `json:"displayName"`
	TradingEnvironment string  `json:"tradingEnvironment"`
	Market             string  `json:"market"`
	SecurityFirm       *string `json:"securityFirm"`
	Enabled            bool    `json:"enabled"`
	UpdatedAt          string  `json:"updatedAt"`
	CreatedAt          string  `json:"createdAt"`
}

// InterfaceSettings holds the network bind / base URL configuration.
type InterfaceSettings struct {
	APIBind       string `json:"apiBind"`
	GUIBind       string `json:"guiBind,omitempty"`
	GUIAPIBaseURL string `json:"guiApiBaseUrl,omitempty"`
}

// UIAppearanceSettings holds chart / UI colour preferences.
type UIAppearanceSettings struct {
	UpColor   string `json:"upColor"`
	DownColor string `json:"downColor"`
}

// OnboardingSettings tracks new-user onboarding state.
type OnboardingSettings struct {
	Completed    bool   `json:"completed"`
	CompletedAt  string `json:"completedAt,omitempty"`
	DismissedAt  string `json:"dismissedAt,omitempty"`
	LastBrokerID string `json:"lastBrokerId"`
}

// ExecutionSettings holds execution / order defaults.
type ExecutionSettings struct {
	DefaultTradingEnvironment      string `json:"defaultTradingEnvironment"`
	BrokerOrderHistoryLookbackDays int    `json:"brokerOrderHistoryLookbackDays"`
	SeenFillRetentionDays          int    `json:"seenFillRetentionDays"`
}

// SecuritySettings controls admin auth behaviour.
type SecuritySettings struct {
	AdminAuthRequired bool `json:"adminAuthRequired"`
}

// ADKRuntimeSettings holds ADK runtime tuning parameters.
type ADKRuntimeSettings struct {
	RunTimeoutMs        int `json:"runTimeoutMs"`
	StreamIdleTimeoutMs int `json:"streamIdleTimeoutMs"`
}

// PineWorkerSettings holds PineTS worker pool user-facing runtime settings.
type PineWorkerSettings struct {
	BacktestWorkerLimit int `json:"backtestWorkerLimit"`
	InstanceWorkerLimit int `json:"instanceWorkerLimit"`
}

type ExchangeCalendarSessionWindow struct {
	Kind        string `json:"kind"`
	StartMinute int    `json:"startMinute"`
	EndMinute   int    `json:"endMinute"`
}

type ExchangeCalendarManualOverride struct {
	Market   string                          `json:"market"`
	Date     string                          `json:"date"`
	Status   string                          `json:"status"`
	Sessions []ExchangeCalendarSessionWindow `json:"sessions,omitempty"`
	Reason   string                          `json:"reason,omitempty"`
	Observed bool                            `json:"observed,omitempty"`
}

type ExchangeCalendarSourcePolicy struct {
	Market             string   `json:"market"`
	PreferredSourceIDs []string `json:"preferredSourceIds,omitempty"`
	EnabledSourceIDs   []string `json:"enabledSourceIds,omitempty"`
	FallbackToBuiltin  bool     `json:"fallbackToBuiltin"`
	RequireOfficial    bool     `json:"requireOfficial,omitempty"`
	StaleAfterHours    int      `json:"staleAfterHours,omitempty"`
}

type ExchangeCalendarSettings struct {
	AutoRefreshEnabled        bool                             `json:"autoRefreshEnabled"`
	ErrorNotificationsEnabled bool                             `json:"errorNotificationsEnabled"`
	RefreshIntervalHours      int                              `json:"refreshIntervalHours"`
	WarmupMarkets             []string                         `json:"warmupMarkets,omitempty"`
	SourcePolicies            []ExchangeCalendarSourcePolicy   `json:"sourcePolicies,omitempty"`
	ManualOverrides           []ExchangeCalendarManualOverride `json:"manualOverrides,omitempty"`

	errorNotificationsEnabledSet bool
}

// UnmarshalJSON defaults error notifications on for legacy settings files that
// predate the field while preserving an explicit false from the API or disk.
func (s *ExchangeCalendarSettings) UnmarshalJSON(data []byte) error {
	type alias ExchangeCalendarSettings
	var raw struct {
		alias
		ErrorNotificationsEnabled *bool `json:"errorNotificationsEnabled"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	*s = ExchangeCalendarSettings(raw.alias)
	if raw.ErrorNotificationsEnabled != nil {
		s.ErrorNotificationsEnabled = *raw.ErrorNotificationsEnabled
		s.errorNotificationsEnabledSet = true
	} else {
		s.ErrorNotificationsEnabled = true
	}
	return nil
}

func (s ExchangeCalendarSettings) ErrorNotificationsEnabledSet() bool {
	return s.errorNotificationsEnabledSet
}

func (s ExchangeCalendarSettings) WithErrorNotificationsEnabledSet(set bool) ExchangeCalendarSettings {
	s.errorNotificationsEnabledSet = set
	return s
}

// LaunchDefaults carries startup-resolved paths and bind addresses.
// Fields are exported so the type can live in a shared package.
type LaunchDefaults struct {
	APIBind        string
	GUIBind        string
	SettingsPath   string
	BacktestDBPath string
}
