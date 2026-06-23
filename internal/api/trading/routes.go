// Package trading 提供券商交易 HTTP 路由。
package trading

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/jftrade/jftrade-main/internal/api/httpserver"
	srv "github.com/jftrade/jftrade-main/internal/trading"
	"github.com/jftrade/jftrade-main/pkg/broker"
)

type resourceURI struct {
	BrokerID string `uri:"brokerId" binding:"required"`
	Resource string `uri:"resource" binding:"required"`
}

type brokerURI struct {
	BrokerID string `uri:"brokerId" binding:"required"`
}

type baseReadQuery struct {
	TradingEnvironment string `form:"tradingEnvironment"`
	AccountID          string `form:"accountId"`
	Market             string `form:"market"`
}

type ordersReadQuery struct {
	baseReadQuery
	Scope     string   `form:"scope"`
	Symbol    string   `form:"symbol"`
	StartTime string   `form:"startTime"`
	EndTime   string   `form:"endTime"`
	Status    []string `form:"status"`
	Statuses  []string `form:"statuses"`
}

type fillsReadQuery struct {
	baseReadQuery
	Scope     string `form:"scope"`
	Symbol    string `form:"symbol"`
	StartTime string `form:"startTime"`
	EndTime   string `form:"endTime"`
}

type cashFlowsReadQuery struct {
	baseReadQuery
	ClearingDate string `form:"clearingDate"`
	Direction    string `form:"direction"`
}

type orderFeesReadQuery struct {
	baseReadQuery
	OrderIDEx     []string `form:"orderIdEx"`
	OrderIDExList []string `form:"orderIdExList"`
}

type symbolsReadQuery struct {
	baseReadQuery
	Symbol  []string `form:"symbol"`
	Symbols []string `form:"symbols"`
}

type maxTradeQuantityReadQuery struct {
	baseReadQuery
	Symbol             string `form:"symbol"`
	OrderType          string `form:"orderType"`
	Price              string `form:"price"`
	OrderIDEx          string `form:"orderIdEx"`
	AdjustSideAndLimit string `form:"adjustSideAndLimit"`
	Session            string `form:"session"`
	PositionID         string `form:"positionId"`
}

type kLinesReadQuery struct {
	baseReadQuery
	Symbol   string `form:"symbol"`
	Period   string `form:"period"`
	FromTime string `form:"fromTime"`
	ToTime   string `form:"toTime"`
	Limit    string `form:"limit"`
}

type placeOrderRequest struct {
	Symbol         string   `json:"symbol"`
	Side           string   `json:"side"`
	OrderType      string   `json:"orderType"`
	Price          *float64 `json:"price,omitempty"`
	Quantity       float64  `json:"quantity"`
	TimeInForce    *string  `json:"timeInForce,omitempty"`
	ClientOrderID  string   `json:"clientOrderId,omitempty"`
	Remark         *string  `json:"remark,omitempty"`
	Session        *string  `json:"session,omitempty"`
	FillOutsideRTH *bool    `json:"fillOutsideRTH,omitempty"`
}

type cancelOrdersRequest struct {
	Orders []struct {
		OrderID       uint64 `json:"orderId"`
		BrokerOrderID string `json:"brokerOrderId"`
		Symbol        string `json:"symbol"`
	} `json:"orders"`
}

type unlockTradeRequest struct {
	Unlock      bool   `json:"unlock"`
	PasswordMD5 string `json:"passwordMd5,omitempty"`
}

func RegisterRoutes(api *gin.RouterGroup, svc *srv.Service) {
	api.GET("/brokers/:brokerId/:resource", handleRead(svc))
	api.POST("/brokers/:brokerId/:resource", handleWrite(svc))
	api.DELETE("/brokers/:brokerId/:resource", handleWrite(svc))
}

func RegisterPortfolioRoutes(api *gin.RouterGroup, svc *srv.Service) {
	api.GET("/portfolio/:brokerId/cash-balances", handlePortfolioRead(svc, "cash-balances"))
	api.GET("/portfolio/:brokerId/positions", handlePortfolioRead(svc, "positions"))
	api.GET("/portfolio/:brokerId/cash-reconciliation", handlePortfolioRead(svc, "cash-reconciliation"))
	api.GET("/portfolio/:brokerId/reconciliation", handlePortfolioRead(svc, "reconciliation"))
}

func handlePortfolioRead(svc *srv.Service, resource string) gin.HandlerFunc {
	return func(c *gin.Context) {
		var uri brokerURI
		if err := httpserver.BindURI(c, &uri); err != nil || strings.TrimSpace(uri.BrokerID) == "" {
			httpserver.WriteNotFound(c)
			return
		}
		var base baseReadQuery
		if !bindQuery(c, &base, "invalid portfolio query") {
			return
		}
		query := readQuery(svc, uri.BrokerID, base)
		var (
			result map[string]any
			err    error
			code   string
		)
		switch resource {
		case "cash-balances":
			result, err = svc.PortfolioCashBalances(c.Request.Context(), query)
			code = "PORTFOLIO_CASH_BALANCES_FAILED"
		case "positions":
			result, err = svc.PortfolioPositions(c.Request.Context(), query)
			code = "PORTFOLIO_POSITIONS_FAILED"
		case "cash-reconciliation":
			result, err = svc.PortfolioCashReconciliation(c.Request.Context(), query)
			code = "PORTFOLIO_CASH_RECONCILIATION_FAILED"
		case "reconciliation":
			result, err = svc.PortfolioReconciliation(c.Request.Context(), query)
			code = "PORTFOLIO_RECONCILIATION_FAILED"
		default:
			httpserver.WriteNotFound(c)
			return
		}
		if errors.Is(err, srv.ErrBrokerNotFound) {
			httpserver.WriteNotFound(c)
			return
		}
		if err != nil {
			httpserver.WriteError(c, http.StatusInternalServerError, code, err.Error())
			return
		}
		httpserver.WriteOK(c, result)
	}
}

func handleRead(svc *srv.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		uri, ok := bindURI(c)
		if !ok {
			return
		}
		ctx := c.Request.Context()
		switch uri.Resource {
		case "runtime":
			result, err := svc.Runtime(ctx, uri.BrokerID)
			writeReadResult(c, result, err)
		case "funds":
			var query baseReadQuery
			if !bindQuery(c, &query, "invalid broker funds query") {
				return
			}
			result, err := svc.Funds(ctx, readQuery(svc, uri.BrokerID, query))
			writeReadResult(c, result, err)
		case "positions":
			var query baseReadQuery
			if !bindQuery(c, &query, "invalid broker positions query") {
				return
			}
			result, err := svc.Positions(ctx, readQuery(svc, uri.BrokerID, query))
			writeReadResult(c, result, err)
		case "orders":
			handleOrders(c, svc, uri.BrokerID)
		case "fills":
			handleFills(c, svc, uri.BrokerID)
		case "cash-flows":
			handleCashFlows(c, svc, uri.BrokerID)
		case "order-fees":
			handleOrderFees(c, svc, uri.BrokerID)
		case "margin-ratios":
			handleMarginRatios(c, svc, uri.BrokerID)
		case "max-trade-qtys":
			handleMaxTradeQuantity(c, svc, uri.BrokerID)
		case "quote":
			handleQuote(c, svc, uri.BrokerID)
		case "klines":
			handleKLines(c, svc, uri.BrokerID)
		case "securities":
			handleSecurities(c, svc, uri.BrokerID)
		default:
			httpserver.WriteNotFound(c)
		}
	}
}

func handleWrite(svc *srv.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		uri, ok := bindURI(c)
		if !ok {
			return
		}
		var base baseReadQuery
		if !bindQuery(c, &base, "invalid broker write query") {
			return
		}
		query := readQuery(svc, uri.BrokerID, base)
		switch {
		case uri.Resource == "orders" && c.Request.Method == http.MethodPost:
			var body placeOrderRequest
			if err := c.ShouldBindJSON(&body); err != nil {
				httpserver.WriteError(c, http.StatusBadRequest, "BAD_REQUEST", "invalid request body: "+err.Error())
				return
			}
			result, err := svc.PlaceBrokerOrder(c.Request.Context(), broker.PlaceOrderQuery{
				ReadQuery: query, Symbol: body.Symbol, Side: body.Side, OrderType: body.OrderType,
				Price: body.Price, Quantity: body.Quantity, TimeInForce: body.TimeInForce,
				ClientOrderID: body.ClientOrderID, Remark: body.Remark, Session: body.Session,
				FillOutsideRTH: body.FillOutsideRTH,
			})
			writeOperationResult(c, result, err, "PLACE_ORDER_FAILED")
		case uri.Resource == "orders" && c.Request.Method == http.MethodDelete:
			var body cancelOrdersRequest
			if err := c.ShouldBindJSON(&body); err != nil {
				httpserver.WriteError(c, http.StatusBadRequest, "BAD_REQUEST", "invalid request body: "+err.Error())
				return
			}
			orders := make([]broker.CancelOrder, len(body.Orders))
			for i, order := range body.Orders {
				orders[i] = broker.CancelOrder{
					OrderID: order.OrderID, BrokerOrderID: order.BrokerOrderID, Symbol: order.Symbol,
				}
			}
			result, err := svc.CancelBrokerOrders(c.Request.Context(), query, orders)
			writeOperationResult(c, result, err, "CANCEL_FAILED")
		case uri.Resource == "unlock" && c.Request.Method == http.MethodPost:
			var body unlockTradeRequest
			if err := c.ShouldBindJSON(&body); err != nil {
				httpserver.WriteError(c, http.StatusBadRequest, "BAD_REQUEST", "invalid request body: "+err.Error())
				return
			}
			result, err := svc.UnlockTrade(c.Request.Context(), broker.UnlockTradeRequest{
				ReadQuery: query, Unlock: body.Unlock, PasswordMD5: body.PasswordMD5,
			})
			writeOperationResult(c, result, err, "UNLOCK_FAILED")
		default:
			httpserver.WriteNotFound(c)
		}
	}
}

func handleOrders(c *gin.Context, svc *srv.Service, brokerID string) {
	var query ordersReadQuery
	if !bindQuery(c, &query, "invalid broker orders query") {
		return
	}
	scope, err := normalizeScope(query.Scope)
	if err != nil {
		writeBadRequest(c, err)
		return
	}
	result, err := svc.Orders(c.Request.Context(), srv.OrdersQuery{
		ReadQuery: readQuery(svc, brokerID, query.baseReadQuery), Scope: scope,
		Symbol: strings.TrimSpace(query.Symbol), StartTime: strings.TrimSpace(query.StartTime),
		EndTime: strings.TrimSpace(query.EndTime), Statuses: mergeValues(query.Status, query.Statuses),
	})
	writeReadResult(c, result, err)
}

func handleFills(c *gin.Context, svc *srv.Service, brokerID string) {
	var query fillsReadQuery
	if !bindQuery(c, &query, "invalid broker fills query") {
		return
	}
	scope, err := normalizeScope(query.Scope)
	if err != nil {
		writeBadRequest(c, err)
		return
	}
	result, err := svc.Fills(c.Request.Context(), srv.FillsQuery{
		ReadQuery: readQuery(svc, brokerID, query.baseReadQuery), Scope: scope,
		Symbol: strings.TrimSpace(query.Symbol), StartTime: strings.TrimSpace(query.StartTime),
		EndTime: strings.TrimSpace(query.EndTime),
	})
	writeReadResult(c, result, err)
}

func handleCashFlows(c *gin.Context, svc *srv.Service, brokerID string) {
	var query cashFlowsReadQuery
	if !bindQuery(c, &query, "invalid broker cash flows query") {
		return
	}
	if strings.TrimSpace(query.ClearingDate) == "" {
		writeBadRequest(c, errors.New("query parameter clearingDate is required"))
		return
	}
	result, err := svc.CashFlows(c.Request.Context(), broker.CashFlowQuery{
		ReadQuery:    readQuery(svc, brokerID, query.baseReadQuery),
		ClearingDate: strings.TrimSpace(query.ClearingDate), Direction: strings.TrimSpace(query.Direction),
	})
	writeReadResult(c, result, err)
}

func handleOrderFees(c *gin.Context, svc *srv.Service, brokerID string) {
	var query orderFeesReadQuery
	if !bindQuery(c, &query, "invalid broker order fees query") {
		return
	}
	orderIDs := mergeValues(query.OrderIDEx, query.OrderIDExList)
	if len(orderIDs) == 0 {
		writeBadRequest(c, errors.New("query parameter orderIdEx is required"))
		return
	}
	result, err := svc.OrderFees(c.Request.Context(), broker.OrderFeeQuery{
		ReadQuery: readQuery(svc, brokerID, query.baseReadQuery), OrderIDExList: orderIDs,
	})
	writeReadResult(c, result, err)
}

func handleMarginRatios(c *gin.Context, svc *srv.Service, brokerID string) {
	var query symbolsReadQuery
	if !bindQuery(c, &query, "invalid broker margin ratios query") {
		return
	}
	read := readQuery(svc, brokerID, query.baseReadQuery)
	symbols := mergeValues(query.Symbol, query.Symbols)
	if len(symbols) == 0 {
		writeBadRequest(c, errors.New("query parameter symbol is required"))
		return
	}
	symbols, err := srv.NormalizeSymbols(read.Market, symbols)
	if err != nil {
		writeBadRequest(c, err)
		return
	}
	result, err := svc.MarginRatios(c.Request.Context(), broker.MarginRatioQuery{
		ReadQuery: read, Symbols: symbols,
	})
	writeReadResult(c, result, err)
}

func handleMaxTradeQuantity(c *gin.Context, svc *srv.Service, brokerID string) {
	var query maxTradeQuantityReadQuery
	if !bindQuery(c, &query, "invalid broker max trade quantity query") {
		return
	}
	symbol := strings.TrimSpace(query.Symbol)
	orderType := strings.TrimSpace(query.OrderType)
	priceValue := strings.TrimSpace(query.Price)
	if symbol == "" || orderType == "" || priceValue == "" {
		writeBadRequest(c, errors.New("query parameters symbol, orderType, and price are required"))
		return
	}
	price, err := strconv.ParseFloat(priceValue, 64)
	if err != nil {
		writeBadRequest(c, fmt.Errorf("query parameter price is invalid: %w", err))
		return
	}
	adjust, err := optionalFloat(query.AdjustSideAndLimit, "adjustSideAndLimit")
	if err != nil {
		writeBadRequest(c, err)
		return
	}
	positionID, err := optionalUint(query.PositionID, "positionId")
	if err != nil {
		writeBadRequest(c, err)
		return
	}
	result, err := svc.MaxTradeQuantity(c.Request.Context(), broker.MaxTradeQuantityQuery{
		ReadQuery: readQuery(svc, brokerID, query.baseReadQuery), Symbol: symbol,
		OrderType: orderType, Price: price, OrderIDEx: strings.TrimSpace(query.OrderIDEx),
		AdjustSideAndLimit: adjust, Session: optionalString(query.Session), PositionID: positionID,
	})
	writeReadResult(c, result, err)
}

func handleQuote(c *gin.Context, svc *srv.Service, brokerID string) {
	var query symbolsReadQuery
	if !bindQuery(c, &query, "invalid broker quote query") {
		return
	}
	symbols := mergeValues(query.Symbol, query.Symbols)
	if len(symbols) == 0 {
		writeBadRequest(c, errors.New("query parameter symbol is required"))
		return
	}
	result, err := svc.Quote(c.Request.Context(), broker.QuoteQuery{
		ReadQuery: readQuery(svc, brokerID, query.baseReadQuery), Symbols: symbols,
	})
	writeReadResult(c, result, err)
}

func handleKLines(c *gin.Context, svc *srv.Service, brokerID string) {
	var query kLinesReadQuery
	if !bindQuery(c, &query, "invalid broker klines query") {
		return
	}
	symbol := strings.TrimSpace(query.Symbol)
	if symbol == "" {
		writeBadRequest(c, errors.New("query parameter symbol is required"))
		return
	}
	period := strings.TrimSpace(query.Period)
	if period == "" {
		period = "1d"
	}
	var limit int32
	if raw := strings.TrimSpace(query.Limit); raw != "" {
		parsed, err := strconv.ParseInt(raw, 10, 32)
		if err != nil {
			writeBadRequest(c, fmt.Errorf("query parameter limit is invalid: %w", err))
			return
		}
		limit = int32(parsed)
	}
	result, err := svc.KLines(c.Request.Context(), broker.KLineQuery{
		ReadQuery: readQuery(svc, brokerID, query.baseReadQuery), Symbol: symbol, Period: period,
		FromTime: strings.TrimSpace(query.FromTime), ToTime: strings.TrimSpace(query.ToTime), Limit: limit,
	})
	writeReadResult(c, result, err)
}

func handleSecurities(c *gin.Context, svc *srv.Service, brokerID string) {
	var query symbolsReadQuery
	if !bindQuery(c, &query, "invalid broker securities query") {
		return
	}
	symbols := mergeValues(query.Symbol, query.Symbols)
	if len(symbols) == 0 {
		writeBadRequest(c, errors.New("query parameter symbol is required"))
		return
	}
	result, err := svc.Securities(c.Request.Context(), broker.SecuritySnapshotQuery{
		ReadQuery: readQuery(svc, brokerID, query.baseReadQuery), Symbols: symbols,
	})
	writeReadResult(c, result, err)
}

func bindURI(c *gin.Context) (resourceURI, bool) {
	var uri resourceURI
	if err := httpserver.BindURI(c, &uri); err != nil ||
		strings.TrimSpace(uri.BrokerID) == "" || strings.TrimSpace(uri.Resource) == "" {
		httpserver.WriteNotFound(c)
		return resourceURI{}, false
	}
	return uri, true
}

func bindQuery(c *gin.Context, target any, message string) bool {
	if err := c.ShouldBindQuery(target); err != nil {
		httpserver.WriteError(c, http.StatusBadRequest, "BAD_REQUEST", message)
		return false
	}
	return true
}

func readQuery(svc *srv.Service, brokerID string, query baseReadQuery) broker.ReadQuery {
	return svc.ReadQuery(brokerID, query.TradingEnvironment, query.AccountID, query.Market)
}

func writeReadResult(c *gin.Context, result any, err error) {
	if errors.Is(err, srv.ErrBrokerNotFound) {
		httpserver.WriteNotFound(c)
		return
	}
	if err != nil {
		httpserver.WriteError(c, http.StatusInternalServerError, "BROKER_READ_FAILED", err.Error())
		return
	}
	httpserver.WriteOK(c, result)
}

func writeOperationResult(c *gin.Context, result any, err error, failureCode string) {
	switch {
	case errors.Is(err, srv.ErrBrokerNotFound):
		httpserver.WriteNotFound(c)
	case errors.Is(err, srv.ErrNoBroker):
		httpserver.WriteError(c, http.StatusServiceUnavailable, "NO_BROKER", err.Error())
	case errors.Is(err, srv.ErrTradingUnsupported):
		httpserver.WriteError(c, http.StatusServiceUnavailable, "NO_TRADING", err.Error())
	case errors.Is(err, srv.ErrUnlockUnsupported):
		httpserver.WriteError(c, http.StatusServiceUnavailable, "NOT_SUPPORTED", err.Error())
	case err != nil:
		httpserver.WriteError(c, http.StatusBadGateway, failureCode, err.Error())
	default:
		httpserver.WriteOK(c, result)
	}
}

func writeBadRequest(c *gin.Context, err error) {
	httpserver.WriteError(c, http.StatusBadRequest, "BAD_REQUEST", err.Error())
}

func normalizeScope(value string) (string, error) {
	switch strings.ToUpper(strings.TrimSpace(value)) {
	case "", "CURRENT":
		return "CURRENT", nil
	case "HISTORY":
		return "HISTORY", nil
	default:
		return "", errors.New("query parameter scope is invalid")
	}
}

func mergeValues(groups ...[]string) []string {
	seen := make(map[string]struct{})
	var values []string
	for _, group := range groups {
		for _, raw := range group {
			for part := range strings.SplitSeq(raw, ",") {
				value := strings.TrimSpace(part)
				key := strings.ToUpper(value)
				if value == "" {
					continue
				}
				if _, ok := seen[key]; ok {
					continue
				}
				seen[key] = struct{}{}
				values = append(values, value)
			}
		}
	}
	return values
}

func optionalFloat(value, name string) (*float64, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, nil
	}
	parsed, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return nil, fmt.Errorf("query parameter %s is invalid: %w", name, err)
	}
	return &parsed, nil
}

func optionalUint(value, name string) (*uint64, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, nil
	}
	parsed, err := strconv.ParseUint(value, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("query parameter %s is invalid: %w", name, err)
	}
	return &parsed, nil
}

func optionalString(value string) *string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return &value
}
