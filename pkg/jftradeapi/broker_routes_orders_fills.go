package jftradeapi

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jftrade/jftrade-main/pkg/broker"
)

// handleBrokerOrdersRead godoc
// @Summary 读取券商订单
// @Tags broker
// @Produce json
// @Param brokerId path string true "券商 ID"
// @Param tradingEnvironment query string false "交易环境"
// @Param accountId query string false "账户 ID"
// @Param market query string false "市场代码"
// @Param scope query string false "CURRENT 或 HISTORY"
// @Param symbol query string false "证券代码"
// @Param startTime query string false "历史查询起始时间"
// @Param endTime query string false "历史查询结束时间"
// @Param status query []string false "订单状态"
// @Param statuses query []string false "订单状态，逗号分隔或重复参数"
// @Success 200 {object} envelope
// @Failure 400 {object} envelope
// @Router /api/v1/brokers/{brokerId}/orders [get]
func (s *Server) handleBrokerOrdersRead(c *gin.Context) {
	var query brokerOrdersReadQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		s.writeError(c, http.StatusBadRequest, "BAD_REQUEST", "invalid broker orders query")
		return
	}
	request, err := s.brokerOrdersRequest(query)
	if err != nil {
		s.writeError(c, http.StatusBadRequest, "BAD_REQUEST", err.Error())
		return
	}
	s.writeOK(c, s.brokerOrdersResponse(c.Request.Context(), request))
}

// handleBrokerFillsRead godoc
// @Summary 读取券商成交
// @Tags broker
// @Produce json
// @Param brokerId path string true "券商 ID"
// @Param tradingEnvironment query string false "交易环境"
// @Param accountId query string false "账户 ID"
// @Param market query string false "市场代码"
// @Param scope query string false "CURRENT 或 HISTORY"
// @Param symbol query string false "证券代码"
// @Param startTime query string false "历史查询起始时间"
// @Param endTime query string false "历史查询结束时间"
// @Success 200 {object} envelope
// @Failure 400 {object} envelope
// @Router /api/v1/brokers/{brokerId}/fills [get]
func (s *Server) handleBrokerFillsRead(c *gin.Context) {
	var query brokerFillsReadQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		s.writeError(c, http.StatusBadRequest, "BAD_REQUEST", "invalid broker fills query")
		return
	}
	request, err := s.brokerFillsRequest(query)
	if err != nil {
		s.writeError(c, http.StatusBadRequest, "BAD_REQUEST", err.Error())
		return
	}
	s.writeOK(c, s.brokerFillsResponse(c.Request.Context(), request))
}

func (s *Server) brokerOrdersRequest(query brokerOrdersReadQuery) (brokerOrdersReadRequest, error) {
	scope, err := normalizeBrokerReadScope(query.Scope)
	if err != nil {
		return brokerOrdersReadRequest{}, err
	}
	return brokerOrdersReadRequest{
		ReadQuery: s.brokerReadQuery(query.brokerBaseReadQuery),
		Scope:     scope,
		Symbol:    strings.TrimSpace(query.Symbol),
		StartTime: strings.TrimSpace(query.StartTime),
		EndTime:   strings.TrimSpace(query.EndTime),
		Statuses:  mergeBrokerQueryValues(query.Status, query.Statuses),
	}, nil
}

func (s *Server) brokerOrdersResponse(ctx context.Context, request brokerOrdersReadRequest) map[string]any {
	reader := s.brokerMarketDataReader()
	if reader == nil {
		return brokerReadErrorResponse("orders", []any{}, fmt.Errorf("broker market data not available"))
	}

	var (
		orders []broker.OrderSnapshot
		err    error
	)
	if request.Scope == "HISTORY" {
		orders, err = reader.QueryHistoryOrders(ctx, broker.OrderHistoryQuery{
			ReadQuery: request.ReadQuery,
			Symbol:    request.Symbol,
			StartTime: request.StartTime,
			EndTime:   request.EndTime,
			Statuses:  request.Statuses,
		})
	} else {
		orders, err = reader.QueryOrders(ctx, request.ReadQuery, request.Symbol)
	}
	if err != nil {
		return brokerReadErrorResponse("orders", []any{}, err)
	}

	entries := make([]any, 0, len(orders))
	for _, order := range orders {
		accountID := firstNonEmptyString(order.AccountID, request.ReadQuery.AccountID)
		tradingEnvironment := firstNonEmptyString(order.TradingEnvironment, request.ReadQuery.TradingEnvironment)
		market := firstNonEmptyString(order.Market, request.ReadQuery.Market)
		entries = append(entries, map[string]any{
			"accountId":          accountID,
			"tradingEnvironment": tradingEnvironment,
			"market":             market,
			"brokerOrderId":      order.BrokerOrderID,
			"brokerOrderIdEx":    order.BrokerOrderIDEx,
			"symbol":             order.Symbol,
			"symbolName":         order.SymbolName,
			"side":               order.Side,
			"orderType":          order.OrderType,
			"status":             order.Status,
			"quantity":           order.Quantity,
			"filledQuantity":     order.FilledQuantity,
			"price":              order.Price,
			"filledAveragePrice": order.FilledAveragePrice,
			"submittedAt":        order.SubmittedAt,
			"updatedAt":          order.UpdatedAt,
			"remark":             order.Remark,
			"lastError":          order.LastError,
			"timeInForce":        order.TimeInForce,
			"currency":           order.Currency,
		})
	}

	return map[string]any{
		"checkedAt":    time.Now().UTC().Format(time.RFC3339Nano),
		"connectivity": "connected",
		"lastError":    nil,
		"orders":       entries,
	}
}

func (s *Server) brokerFillsRequest(query brokerFillsReadQuery) (brokerFillsReadRequest, error) {
	scope, err := normalizeBrokerReadScope(query.Scope)
	if err != nil {
		return brokerFillsReadRequest{}, err
	}
	return brokerFillsReadRequest{
		ReadQuery: s.brokerReadQuery(query.brokerBaseReadQuery),
		Scope:     scope,
		Symbol:    strings.TrimSpace(query.Symbol),
		StartTime: strings.TrimSpace(query.StartTime),
		EndTime:   strings.TrimSpace(query.EndTime),
	}, nil
}

func (s *Server) brokerFillsResponse(ctx context.Context, request brokerFillsReadRequest) map[string]any {
	reader := s.brokerMarketDataReader()
	if reader == nil {
		return brokerReadErrorResponse("fills", []any{}, fmt.Errorf("broker market data not available"))
	}

	fillQuery := broker.OrderFillQuery{
		ReadQuery: request.ReadQuery,
		Symbol:    request.Symbol,
		StartTime: request.StartTime,
		EndTime:   request.EndTime,
	}
	var (
		fills []broker.OrderFillSnapshot
		err   error
	)
	if request.Scope == "HISTORY" {
		fills, err = reader.QueryHistoryOrderFills(ctx, broker.OrderFillHistoryQuery(fillQuery))
	} else {
		fills, err = reader.QueryOrderFills(ctx, fillQuery)
	}
	if err != nil {
		return brokerReadErrorResponse("fills", []any{}, err)
	}

	entries := make([]any, 0, len(fills))
	for _, fill := range fills {
		entries = append(entries, map[string]any{
			"accountId":          fill.AccountID,
			"tradingEnvironment": fill.TradingEnvironment,
			"market":             fill.Market,
			"brokerOrderId":      fill.BrokerOrderID,
			"brokerOrderIdEx":    fill.BrokerOrderIDEx,
			"brokerFillId":       fill.BrokerFillID,
			"brokerFillIdEx":     fill.BrokerFillIDEx,
			"symbol":             fill.Symbol,
			"symbolName":         fill.SymbolName,
			"side":               fill.Side,
			"filledQuantity":     fill.FilledQuantity,
			"fillPrice":          fill.FillPrice,
			"filledAt":           fill.FilledAt,
			"status":             fill.Status,
		})
	}

	return map[string]any{
		"checkedAt":    time.Now().UTC().Format(time.RFC3339Nano),
		"connectivity": "connected",
		"lastError":    nil,
		"fills":        entries,
	}
}
