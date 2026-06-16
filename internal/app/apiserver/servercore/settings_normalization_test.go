package servercore

import "testing"

func TestNormalizeManagedBrokerAccountAppliesDefaults(t *testing.T) {
	account := normalizeManagedBrokerAccount(ManagedBrokerAccount{
		BrokerID:           "  ",
		AccountID:          "  12345678  ",
		DisplayName:        "  ",
		TradingEnvironment: "  ",
		Market:             " us ",
		SecurityFirm:       new("  "),
	})

	if account.BrokerID != "futu" {
		t.Fatalf("brokerId = %q", account.BrokerID)
	}
	if account.AccountID != "12345678" {
		t.Fatalf("accountId = %q", account.AccountID)
	}
	if account.DisplayName != "12345678" {
		t.Fatalf("displayName = %q", account.DisplayName)
	}
	if account.TradingEnvironment != "SIMULATE" {
		t.Fatalf("tradingEnvironment = %q", account.TradingEnvironment)
	}
	if account.Market != "US" {
		t.Fatalf("market = %q", account.Market)
	}
	if account.SecurityFirm != nil {
		t.Fatalf("securityFirm = %#v", account.SecurityFirm)
	}
}

func TestNormalizeFutuConfigAppliesDefaults(t *testing.T) {
	config := normalizeFutuConfig(FutuIntegrationConfig{UseEncryption: true})

	if config.Type != "futu" {
		t.Fatalf("type = %q", config.Type)
	}
	if config.Host != defaultFutuHost {
		t.Fatalf("host = %q", config.Host)
	}
	if config.APIPort != defaultFutuAPIPort {
		t.Fatalf("apiPort = %d", config.APIPort)
	}
	if config.WebSocketPort != defaultFutuWebSocketPort {
		t.Fatalf("webSocketPort = %d", config.WebSocketPort)
	}
	if config.MaxWebSocketConnections != defaultMaxWebSocketClients {
		t.Fatalf("maxWebSocketConnections = %d", config.MaxWebSocketConnections)
	}
	if config.TradeMarket != "HK" {
		t.Fatalf("tradeMarket = %q", config.TradeMarket)
	}
	if config.SecurityFirm != "FUTUSECURITIES" {
		t.Fatalf("securityFirm = %q", config.SecurityFirm)
	}
	if config.UseEncryption {
		t.Fatalf("useEncryption should be forced false")
	}
}

func TestNormalizeExecutionSettingsAppliesDefaultsAndBounds(t *testing.T) {
	settings := normalizeExecutionSettings(ExecutionSettings{
		DefaultTradingEnvironment:      " real ",
		BrokerOrderHistoryLookbackDays: 500,
		SeenFillRetentionDays:          0,
	})

	if settings.DefaultTradingEnvironment != "REAL" {
		t.Fatalf("defaultTradingEnvironment = %q", settings.DefaultTradingEnvironment)
	}
	if settings.BrokerOrderHistoryLookbackDays != 365 {
		t.Fatalf("brokerOrderHistoryLookbackDays = %d", settings.BrokerOrderHistoryLookbackDays)
	}
	if settings.SeenFillRetentionDays != 90 {
		t.Fatalf("seenFillRetentionDays = %d", settings.SeenFillRetentionDays)
	}

	settings = normalizeExecutionSettings(ExecutionSettings{DefaultTradingEnvironment: "paper"})
	if settings.DefaultTradingEnvironment != "SIMULATE" {
		t.Fatalf("invalid defaultTradingEnvironment = %q", settings.DefaultTradingEnvironment)
	}
}
