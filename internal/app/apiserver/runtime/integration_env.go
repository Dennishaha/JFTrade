package runtime

import (
	"net"
	"os"
	"strconv"
	"strings"

	jfsettings "github.com/jftrade/jftrade-main/pkg/jftsettings"
)

const (
	futuOpenDAddrEnv            = "FUTU_OPEND_ADDR"
	futuOpenDWebSocketKeyEnv    = "FUTU_OPEND_WEBSOCKET_KEY"
	jftradeFutuWebSocketKeyEnv  = "JFTRADE_FUTU_WEBSOCKET_KEY"
	jftradeFutuAPIPortEnv       = "JFTRADE_FUTU_API_PORT"
	jftradeFutuWebSocketPortEnv = "JFTRADE_FUTU_WEBSOCKET_PORT"
	jftradeFutuHostEnv          = "JFTRADE_FUTU_HOST"
	jftradeFutuMaxClientsEnv    = "JFTRADE_FUTU_MAX_WEBSOCKET_CONNECTIONS"
	jftradeFutuTradeMarketEnv   = "JFTRADE_FUTU_TRADE_MARKET"
	jftradeFutuSecurityFirmEnv  = "JFTRADE_FUTU_SECURITY_FIRM"
)

// IntegrationWithEnvDefaults preserves legacy environment-based defaults when
// no broker integration has been persisted.
func IntegrationWithEnvDefaults(integration jfsettings.BrokerIntegration) jfsettings.BrokerIntegration {
	config := integration.Config
	host := config.Host
	apiPort := config.APIPort
	if rawAddr := strings.TrimSpace(os.Getenv(futuOpenDAddrEnv)); rawAddr != "" {
		if parsedHost, parsedPort, err := net.SplitHostPort(rawAddr); err == nil {
			host = parsedHost
			if value, convErr := strconv.Atoi(parsedPort); convErr == nil && value > 0 {
				apiPort = value
			}
		}
	}

	config.Host = envOrDefault(jftradeFutuHostEnv, host)
	config.APIPort = positiveIntEnv(jftradeFutuAPIPortEnv, apiPort)
	config.WebSocketPort = positiveIntEnv(jftradeFutuWebSocketPortEnv, config.WebSocketPort)
	config.MaxWebSocketConnections = positiveIntEnv(jftradeFutuMaxClientsEnv, config.MaxWebSocketConnections)
	config.WebSocketKey = firstNonEmpty(os.Getenv(jftradeFutuWebSocketKeyEnv), os.Getenv(futuOpenDWebSocketKeyEnv))
	config.TradeMarket = envOrDefault(jftradeFutuTradeMarketEnv, config.TradeMarket)
	config.SecurityFirm = envOrDefault(jftradeFutuSecurityFirmEnv, config.SecurityFirm)
	integration.Config = config
	return integration
}

// ApplyIntegrationEnv exposes persisted broker settings to legacy runtime consumers.
func ApplyIntegrationEnv(integration jfsettings.BrokerIntegration) {
	config := integration.Config
	_ = os.Setenv(futuOpenDAddrEnv, net.JoinHostPort(config.Host, strconv.Itoa(config.APIPort)))
	_ = os.Setenv(futuOpenDWebSocketKeyEnv, config.WebSocketKey)
	_ = os.Setenv(jftradeFutuWebSocketKeyEnv, config.WebSocketKey)
	_ = os.Setenv(jftradeFutuAPIPortEnv, strconv.Itoa(config.APIPort))
	_ = os.Setenv(jftradeFutuWebSocketPortEnv, strconv.Itoa(config.WebSocketPort))
}

func positiveIntEnv(key string, fallback int) int {
	value, err := strconv.Atoi(strings.TrimSpace(os.Getenv(key)))
	if err != nil || value <= 0 {
		return fallback
	}
	return value
}

func envOrDefault(key string, fallback string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	return value
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
