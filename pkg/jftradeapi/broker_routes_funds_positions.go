package jftradeapi

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jftrade/jftrade-main/pkg/broker"
)

// handleBrokerFundsRead godoc
// @Summary 读取券商资金
// @Tags broker
// @Produce json
// @Param brokerId path string true "券商 ID"
// @Param tradingEnvironment query string false "交易环境"
// @Param accountId query string false "账户 ID"
// @Param market query string false "市场代码"
// @Success 200 {object} envelope
// @Failure 400 {object} envelope
// @Router /api/v1/brokers/{brokerId}/funds [get]
func (s *Server) handleBrokerFundsRead(c *gin.Context) {
	var query brokerBaseReadQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		s.writeError(c, http.StatusBadRequest, "BAD_REQUEST", "invalid broker funds query")
		return
	}
	s.writeOK(c, s.brokerFundsResponse(c.Request.Context(), s.brokerReadQuery(query)))
}

// handleBrokerPositionsRead godoc
// @Summary 读取券商持仓
// @Tags broker
// @Produce json
// @Param brokerId path string true "券商 ID"
// @Param tradingEnvironment query string false "交易环境"
// @Param accountId query string false "账户 ID"
// @Param market query string false "市场代码"
// @Success 200 {object} envelope
// @Failure 400 {object} envelope
// @Router /api/v1/brokers/{brokerId}/positions [get]
func (s *Server) handleBrokerPositionsRead(c *gin.Context) {
	var query brokerBaseReadQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		s.writeError(c, http.StatusBadRequest, "BAD_REQUEST", "invalid broker positions query")
		return
	}
	s.writeOK(c, s.brokerPositionsResponse(c.Request.Context(), s.brokerReadQuery(query)))
}

func (s *Server) brokerFundsResponse(ctx context.Context, query broker.ReadQuery) map[string]any {
	reader := s.brokerMarketDataReader()
	if reader == nil {
		return brokerReadErrorResponse("summary", nil, fmt.Errorf("broker market data not available"), "currencyBalances", "marketAssets")
	}
	snapshot, err := reader.QueryFunds(ctx, query)
	if err != nil {
		return brokerReadErrorResponse("summary", nil, err, "currencyBalances", "marketAssets")
	}

	currencyBalances := make([]any, 0, len(snapshot.CurrencyBalances))
	for _, balance := range snapshot.CurrencyBalances {
		currencyBalances = append(currencyBalances, map[string]any{
			"accountId":               balance.AccountID,
			"tradingEnvironment":      balance.TradingEnvironment,
			"currency":                balance.Currency,
			"cash":                    balance.Cash,
			"availableWithdrawalCash": balance.AvailableWithdrawalCash,
			"netCashPower":            balance.NetCashPower,
		})
	}

	marketAssets := make([]any, 0, len(snapshot.MarketAssets))
	for _, asset := range snapshot.MarketAssets {
		marketAssets = append(marketAssets, map[string]any{
			"accountId":          asset.AccountID,
			"tradingEnvironment": asset.TradingEnvironment,
			"market":             asset.Market,
			"assets":             asset.Assets,
		})
	}

	return map[string]any{
		"checkedAt":    time.Now().UTC().Format(time.RFC3339Nano),
		"connectivity": "connected",
		"lastError":    nil,
		"summary": map[string]any{
			"accountId":               snapshot.AccountID,
			"tradingEnvironment":      snapshot.TradingEnvironment,
			"market":                  snapshot.Market,
			"currency":                snapshot.Currency,
			"totalAssets":             snapshot.TotalAssets,
			"securitiesAssets":        snapshot.SecuritiesAssets,
			"fundAssets":              snapshot.FundAssets,
			"bondAssets":              snapshot.BondAssets,
			"cash":                    snapshot.Cash,
			"marketValue":             snapshot.MarketValue,
			"longMarketValue":         snapshot.LongMarketValue,
			"shortMarketValue":        snapshot.ShortMarketValue,
			"purchasingPower":         snapshot.PurchasingPower,
			"shortSellingPower":       snapshot.ShortSellingPower,
			"netCashPower":            snapshot.NetCashPower,
			"availableWithdrawalCash": snapshot.AvailableWithdrawalCash,
			"maxWithdrawal":           snapshot.MaxWithdrawal,
			"availableFunds":          snapshot.AvailableFunds,
			"frozenCash":              snapshot.FrozenCash,
			"pendingAsset":            snapshot.PendingAsset,
			"unrealizedPnl":           snapshot.UnrealizedPnl,
			"realizedPnl":             snapshot.RealizedPnl,
			"initialMargin":           snapshot.InitialMargin,
			"maintenanceMargin":       snapshot.MaintenanceMargin,
			"marginCallMargin":        snapshot.MarginCallMargin,
			"riskStatus":              snapshot.RiskStatus,
			// Margin & Financing 融资融券
			"debtCash":       snapshot.DebtCash,
			"isPdt":          snapshot.IsPDT,
			"pdtSeq":         snapshot.PDTSeq,
			"beginningDTBP":  snapshot.BeginningDTBP,
			"remainingDTBP":  snapshot.RemainingDTBP,
			"dtCallAmount":   snapshot.DTCallAmount,
			"dtStatus":       snapshot.DTStatus,
			"exposureLevel":  snapshot.ExposureLevel,
			"exposureLimit":  snapshot.ExposureLimit,
			"usedLimit":      snapshot.UsedLimit,
			"remainingLimit": snapshot.RemainingLimit,
		},
		"currencyBalances": currencyBalances,
		"marketAssets":     marketAssets,
	}
}

func (s *Server) brokerPositionsResponse(ctx context.Context, query broker.ReadQuery) map[string]any {
	reader := s.brokerMarketDataReader()
	if reader == nil {
		return brokerReadErrorResponse("positions", []any{}, fmt.Errorf("broker market data not available"))
	}
	positions, err := reader.QueryPositions(ctx, query)
	if err != nil {
		return brokerReadErrorResponse("positions", []any{}, err)
	}

	entries := make([]any, 0, len(positions))
	for _, position := range positions {
		entries = append(entries, map[string]any{
			"accountId":          position.AccountID,
			"tradingEnvironment": position.TradingEnvironment,
			"market":             position.Market,
			"symbol":             position.Symbol,
			"symbolName":         position.SymbolName,
			"quantity":           position.Quantity,
			"sellableQuantity":   position.SellableQuantity,
			"lastPrice":          position.LastPrice,
			"costPrice":          position.CostPrice,
			"averageCostPrice":   position.AverageCostPrice,
			"marketValue":        position.MarketValue,
			"unrealizedPnl":      position.UnrealizedPnl,
			"realizedPnl":        position.RealizedPnl,
			"pnlRatio":           position.PnlRatio,
			"currency":           position.Currency,
		})
	}

	return map[string]any{
		"checkedAt":    time.Now().UTC().Format(time.RFC3339Nano),
		"connectivity": "connected",
		"lastError":    nil,
		"positions":    entries,
	}
}

// brokerFundsResponseWithTimeout wraps brokerFundsResponse with a dedicated timeout.
// On timeout it returns a degraded result instead of blocking indefinitely.
func (s *Server) brokerFundsResponseWithTimeout(ctx context.Context, query broker.ReadQuery, timeout time.Duration) map[string]any {
	fundsCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	type result struct {
		value map[string]any
	}
	done := make(chan result, 1)
	go func() {
		done <- result{value: s.brokerFundsResponse(fundsCtx, query)}
	}()
	select {
	case <-fundsCtx.Done():
		return brokerReadErrorResponse("summary", nil, fmt.Errorf("funds query timed out after %s", timeout), "currencyBalances", "marketAssets")
	case r := <-done:
		return r.value
	}
}

// brokerPositionsResponseWithTimeout wraps brokerPositionsResponse with a dedicated timeout.
// On timeout it returns a degraded result instead of blocking indefinitely.
func (s *Server) brokerPositionsResponseWithTimeout(ctx context.Context, query broker.ReadQuery, timeout time.Duration) map[string]any {
	positionsCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	type result struct {
		value map[string]any
	}
	done := make(chan result, 1)
	go func() {
		done <- result{value: s.brokerPositionsResponse(positionsCtx, query)}
	}()
	select {
	case <-positionsCtx.Done():
		return brokerReadErrorResponse("positions", []any{}, fmt.Errorf("positions query timed out after %s", timeout))
	case r := <-done:
		return r.value
	}
}
