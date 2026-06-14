package jftradeapi

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jftrade/jftrade-main/pkg/broker"
)

func (s *Server) handleBrokerRead(c *gin.Context) {
	brokerID, resource, ok := s.brokerRouteParams(c)
	if !ok {
		s.notFound(c)
		return
	}
	// Look up the broker by ID from the registry.
	activeBroker := s.activeBroker()
	if activeBroker == nil || activeBroker.ID() != brokerID {
		// For backward compatibility, only "futu" is supported currently.
		if brokerID != "futu" {
			s.notFound(c)
			return
		}
	}

	switch resource {
	case "runtime":
		s.writeOK(c, s.brokerRuntime(c.Request.Context()))
	case "funds":
		s.handleBrokerFundsRead(c)
	case "positions":
		s.handleBrokerPositionsRead(c)
	case "orders":
		s.handleBrokerOrdersRead(c)
	case "fills":
		s.handleBrokerFillsRead(c)
	case "cash-flows":
		s.handleBrokerCashFlowsRead(c)
	case "order-fees":
		s.handleBrokerOrderFeesRead(c)
	case "margin-ratios":
		s.handleBrokerMarginRatiosRead(c)
	case "max-trade-qtys":
		s.handleBrokerMaxTradeQuantityRead(c)
	case "quote":
		s.handleBrokerQuoteRead(c)
	case "klines":
		s.handleBrokerKLinesRead(c)
	case "securities":
		s.handleBrokerSecuritiesRead(c)
	default:
		s.notFound(c)
	}
}

func (s *Server) handleBrokerWrite(c *gin.Context) {
	brokerID, resource, ok := s.brokerRouteParams(c)
	if !ok {
		s.notFound(c)
		return
	}
	activeBroker := s.activeBroker()
	if activeBroker == nil || activeBroker.ID() != brokerID {
		if brokerID != "futu" {
			s.notFound(c)
			return
		}
	}
	var query brokerBaseReadQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		s.writeError(c, http.StatusBadRequest, "BAD_REQUEST", "invalid broker write query")
		return
	}
	readQuery := s.brokerReadQuery(query)
	switch {
	case resource == "orders" && c.Request.Method == http.MethodPost:
		s.handlePlaceOrder(c, readQuery)
	case resource == "orders" && c.Request.Method == http.MethodDelete:
		s.handleCancelOrders(c, readQuery)
	case resource == "unlock" && c.Request.Method == http.MethodPost:
		s.handleUnlockTrade(c, readQuery)
	default:
		s.notFound(c)
	}
}

func (s *Server) brokerRouteParams(c *gin.Context) (brokerID string, resource string, ok bool) {
	var uri brokerResourceURI
	if err := bindURI(c, &uri); err != nil || strings.TrimSpace(uri.BrokerID) == "" || strings.TrimSpace(uri.Resource) == "" {
		return "", "", false
	}
	return uri.BrokerID, uri.Resource, true
}

type brokerOrdersReadRequest struct {
	ReadQuery broker.ReadQuery
	Scope     string
	Symbol    string
	StartTime string
	EndTime   string
	Statuses  []string
}

type brokerFillsReadRequest struct {
	ReadQuery broker.ReadQuery
	Scope     string
	Symbol    string
	StartTime string
	EndTime   string
}

func (s *Server) brokerReadQuery(query brokerBaseReadQuery) broker.ReadQuery {
	market := strings.TrimSpace(query.Market)
	if market == "" {
		market = strings.TrimSpace(s.store.integration().Config.TradeMarket)
	}
	if market == "" {
		market = "HK"
	}
	return broker.ReadQuery{
		BrokerID:           "futu",
		TradingEnvironment: strings.TrimSpace(query.TradingEnvironment),
		AccountID:          strings.TrimSpace(query.AccountID),
		Market:             market,
	}
}

// brokerMarketDataReader returns the MarketDataReader for the active broker,
// or falls back to the legacy futuExchange() path if the broker does not support market data.
func (s *Server) brokerMarketDataReader() broker.MarketDataReader {
	b := s.activeBroker()
	if b == nil {
		return nil
	}
	return b.MarketData()
}

func normalizeBrokerReadScope(value string) (string, error) {
	switch strings.ToUpper(strings.TrimSpace(value)) {
	case "", "CURRENT":
		return "CURRENT", nil
	case "HISTORY":
		return "HISTORY", nil
	default:
		return "", fmt.Errorf("query parameter scope is invalid")
	}
}

func mergeBrokerQueryValues(groups ...[]string) []string {
	seen := make(map[string]struct{})
	values := make([]string, 0)
	for _, group := range groups {
		for _, raw := range group {
			for _, part := range strings.Split(raw, ",") {
				trimmed := strings.TrimSpace(part)
				if trimmed == "" {
					continue
				}
				normalized := strings.ToUpper(trimmed)
				if _, ok := seen[normalized]; ok {
					continue
				}
				seen[normalized] = struct{}{}
				values = append(values, trimmed)
			}
		}
	}
	return values
}

func brokerReadErrorResponse(key string, value any, err error, extraKeys ...string) map[string]any {
	result := map[string]any{
		"checkedAt":    time.Now().UTC().Format(time.RFC3339Nano),
		"connectivity": connectivityFromBrokerReadError(err),
		"lastError":    err.Error(),
		key:            value,
	}
	for _, extraKey := range extraKeys {
		result[extraKey] = []any{}
	}
	return result
}

func connectivityFromBrokerReadError(err error) string {
	if err == nil {
		return "connected"
	}
	lower := strings.ToLower(err.Error())
	for _, marker := range []string{"connection refused", "dial ", "i/o timeout", "timeout", "client closed", "broken pipe", "connection reset", "eof", "unavailable"} {
		if strings.Contains(lower, marker) {
			return "disconnected"
		}
	}
	return "degraded"
}

// --- New write-side handlers ---

func (s *Server) brokerTradingService() (broker.TradingService, int, string, string) {
	activeBroker := s.activeBroker()
	if activeBroker == nil {
		return nil, http.StatusServiceUnavailable, "NO_BROKER", "no active broker"
	}
	trading := activeBroker.Trading()
	if trading == nil {
		return nil, http.StatusServiceUnavailable, "NO_TRADING", "broker does not support trading"
	}
	return trading, 0, "", ""
}

func (s *Server) brokerUnlockTrader() (broker.UnlockTrader, int, string, string) {
	activeBroker := s.activeBroker()
	if activeBroker == nil {
		return nil, http.StatusServiceUnavailable, "NO_BROKER", "no active broker"
	}
	unlocker, ok := activeBroker.(broker.UnlockTrader)
	if !ok {
		return nil, http.StatusServiceUnavailable, "NOT_SUPPORTED", "broker does not support trade unlock"
	}
	return unlocker, 0, "", ""
}
