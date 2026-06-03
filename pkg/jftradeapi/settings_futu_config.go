package jftradeapi

import (
	"net"
	"os"
	"strconv"
	"strings"

	"github.com/jftrade/jftrade-main/pkg/futu"
)

func (s *SettingsStore) applyRuntimeEnv() {
	integration := s.integration()
	config := integration.Config
	addr := net.JoinHostPort(config.Host, strconv.Itoa(config.APIPort))
	_ = os.Setenv(futu.EnvOpenDAddr, addr)
	_ = os.Setenv("FUTU_OPEND_WEBSOCKET_KEY", config.WebSocketKey)
	_ = os.Setenv("JFTRADE_FUTU_WEBSOCKET_KEY", config.WebSocketKey)
	_ = os.Setenv("JFTRADE_FUTU_API_PORT", strconv.Itoa(config.APIPort))
	_ = os.Setenv("JFTRADE_FUTU_WEBSOCKET_PORT", strconv.Itoa(config.WebSocketPort))
}

func defaultFutuConfig() FutuIntegrationConfig {
	host := defaultFutuHost
	apiPort := defaultFutuAPIPort
	webSocketPort := defaultFutuWebSocketPort
	if rawAddr := strings.TrimSpace(os.Getenv(futu.EnvOpenDAddr)); rawAddr != "" {
		if parsedHost, parsedPort, err := net.SplitHostPort(rawAddr); err == nil {
			host = parsedHost
			if portValue, convErr := strconv.Atoi(parsedPort); convErr == nil && portValue > 0 {
				apiPort = portValue
			}
		}
	}

	return normalizeFutuConfig(FutuIntegrationConfig{
		Type:                    "futu",
		Host:                    envOrDefault("JFTRADE_FUTU_HOST", host),
		APIPort:                 intEnv("JFTRADE_FUTU_API_PORT", apiPort),
		WebSocketPort:           intEnv("JFTRADE_FUTU_WEBSOCKET_PORT", webSocketPort),
		MaxWebSocketConnections: intEnv("JFTRADE_FUTU_MAX_WEBSOCKET_CONNECTIONS", defaultMaxWebSocketClients),
		UseEncryption:           false,
		WebSocketKey:            firstNonEmpty(os.Getenv("JFTRADE_FUTU_WEBSOCKET_KEY"), os.Getenv("FUTU_OPEND_WEBSOCKET_KEY")),
		TradeMarket:             envOrDefault("JFTRADE_FUTU_TRADE_MARKET", "HK"),
		SecurityFirm:            envOrDefault("JFTRADE_FUTU_SECURITY_FIRM", "FUTUSECURITIES"),
	})
}

func normalizeFutuConfig(config FutuIntegrationConfig) FutuIntegrationConfig {
	if config.Type == "" {
		config.Type = "futu"
	}
	if strings.TrimSpace(config.Host) == "" {
		config.Host = defaultFutuHost
	}
	if config.APIPort <= 0 {
		config.APIPort = defaultFutuAPIPort
	}
	if config.WebSocketPort <= 0 {
		config.WebSocketPort = defaultFutuWebSocketPort
	}
	if config.MaxWebSocketConnections <= 0 {
		config.MaxWebSocketConnections = defaultMaxWebSocketClients
	}
	if strings.TrimSpace(config.TradeMarket) == "" {
		config.TradeMarket = "HK"
	}
	if strings.TrimSpace(config.SecurityFirm) == "" {
		config.SecurityFirm = "FUTUSECURITIES"
	}
	config.UseEncryption = false
	return config
}

func intEnv(key string, fallback int) int {
	value, err := strconv.Atoi(strings.TrimSpace(os.Getenv(key)))
	if err != nil || value <= 0 {
		return fallback
	}
	return value
}

func boolEnv(key string, fallback bool) bool {
	value := strings.TrimSpace(strings.ToLower(os.Getenv(key)))
	switch value {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		return fallback
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
