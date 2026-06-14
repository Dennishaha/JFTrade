package settings

import jfsettings "github.com/jftrade/jftrade-main/pkg/jftsettings"

// UIAppearanceSettingsWriteRequest documents UI appearance writes.
type UIAppearanceSettingsWriteRequest struct {
	Appearance jfsettings.UIAppearanceSettings `json:"appearance"`
}

// OnboardingWriteRequest documents onboarding state writes.
type OnboardingWriteRequest struct {
	Completed    bool   `json:"completed"`
	Dismissed    bool   `json:"dismissed"`
	LastBrokerID string `json:"lastBrokerId"`
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
	SecurityFirm       string `json:"securityFirm"`
	Enabled            bool   `json:"enabled"`
}
