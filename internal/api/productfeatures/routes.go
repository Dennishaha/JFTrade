package productfeatures

import (
	"errors"
	"maps"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/jftrade/jftrade-main/internal/api/httpserver"
	service "github.com/jftrade/jftrade-main/internal/productfeatures"
	"github.com/jftrade/jftrade-main/pkg/broker"
)

type queryRoute struct {
	feature       broker.FeatureID
	operation     string
	defaultMarket string
	productClass  broker.ProductClass
	segment       broker.MarketSegment
}

func RegisterRoutes(api *gin.RouterGroup, svc *service.Service) {
	registerSwaggerDocs()
	api.GET("/brokers/capabilities", handleCapabilities(svc))
	api.GET("/alerts/price", handleQuery(svc, queryRoute{feature: broker.FeaturePriceAlertList}))
	api.POST("/alerts/price", handleCustomization(svc, broker.FeaturePriceAlertSet, "set"))
	api.GET("/alerts/option-events", handleQuery(svc, queryRoute{feature: broker.FeatureOptionEventAlertList}))
	api.POST("/alerts/option-events", handleCustomization(svc, broker.FeatureOptionEventAlertSet, "set"))
	api.GET("/watchlists/remote", handleQuery(svc, queryRoute{feature: broker.FeatureRemoteWatchlistList}))
	api.POST("/watchlists/remote", handleCustomization(svc, broker.FeatureRemoteWatchlistModify, "modify"))

	market := api.Group("/market-data")
	market.POST("/snapshots", handleBatchSnapshots(svc))
	market.GET("/instruments/:instrumentId/profile", handleQuery(svc, queryRoute{feature: broker.FeatureInstrumentProfile}))
	market.GET("/intraday/:instrumentId", handleQuery(svc, queryRoute{feature: broker.FeatureMarketIntraday}))
	market.GET("/ticks/:instrumentId", handleQuery(svc, queryRoute{feature: broker.FeatureMarketTicks}))
	market.GET("/broker-queue/:instrumentId", handleQuery(svc, queryRoute{feature: broker.FeatureMarketBrokerQueue}))
	market.GET("/capital-flow/:instrumentId", handleQuery(svc, queryRoute{feature: broker.FeatureMarketCapitalFlow, operation: "flow"}))
	market.GET("/options/chains/:instrumentId", handleQuery(svc, queryRoute{feature: broker.FeatureOptionChain, operation: "chain", productClass: broker.ProductClassOption, segment: broker.MarketSegmentDerivatives}))
	market.GET("/options/expirations/:instrumentId", handleQuery(svc, queryRoute{feature: broker.FeatureOptionChain, operation: "expirations", productClass: broker.ProductClassOption, segment: broker.MarketSegmentDerivatives}))
	market.GET("/options/screens", handleQuery(svc, queryRoute{feature: broker.FeatureOptionScreen, operation: "screen", productClass: broker.ProductClassOption, segment: broker.MarketSegmentDerivatives}))
	market.GET("/options/analysis/:instrumentId", handleQuery(svc, queryRoute{feature: broker.FeatureOptionAnalysis, productClass: broker.ProductClassOption, segment: broker.MarketSegmentDerivatives}))
	market.POST("/options/analysis/:instrumentId", handlePostQuery(svc, queryRoute{feature: broker.FeatureOptionAnalysis, productClass: broker.ProductClassOption, segment: broker.MarketSegmentDerivatives}))
	market.GET("/options/events", handleQuery(svc, queryRoute{feature: broker.FeatureOptionEvents, productClass: broker.ProductClassOption, segment: broker.MarketSegmentDerivatives}))
	market.POST("/options/events/zero-dte-contracts", handleZeroDteContracts(svc))
	market.GET("/warrants", handleQuery(svc, queryRoute{feature: broker.FeatureWarrants, productClass: broker.ProductClassWarrant, segment: broker.MarketSegmentDerivatives}))
	market.GET("/futures", handleQuery(svc, queryRoute{feature: broker.FeatureFutures, productClass: broker.ProductClassFuture, segment: broker.MarketSegmentDerivatives}))
	market.GET("/news", handleQuery(svc, queryRoute{feature: broker.FeatureResearchNews, operation: "search"}))

	research := api.Group("/research")
	research.GET("/instruments/:instrumentId", handleQuery(svc, queryRoute{feature: broker.FeatureResearchInstrument}))
	research.GET("/financials/:instrumentId", handleQuery(svc, queryRoute{feature: broker.FeatureResearchFinancials}))
	research.GET("/valuation/:instrumentId", handleQuery(svc, queryRoute{feature: broker.FeatureResearchValuation}))
	research.GET("/analyst/:instrumentId", handleQuery(svc, queryRoute{feature: broker.FeatureResearchAnalyst}))
	research.GET("/ownership/:instrumentId", handleQuery(svc, queryRoute{feature: broker.FeatureResearchOwnership}))
	research.GET("/corporate-actions/:instrumentId", handleQuery(svc, queryRoute{feature: broker.FeatureResearchCorporateAction}))
	research.GET("/short-interest/:instrumentId", handleQuery(svc, queryRoute{feature: broker.FeatureResearchShortInterest}))
	research.GET("/screens", handleQuery(svc, queryRoute{feature: broker.FeatureResearchScreen}))
	research.GET("/calendars", handleQuery(svc, queryRoute{feature: broker.FeatureResearchCalendar}))
	research.GET("/macro", handleQuery(svc, queryRoute{feature: broker.FeatureResearchMacro}))
	research.GET("/rankings", handleQuery(svc, queryRoute{feature: broker.FeatureResearchRankings}))
	research.GET("/institutions", handleQuery(svc, queryRoute{feature: broker.FeatureResearchInstitutions}))
	research.GET("/industries", handleQuery(svc, queryRoute{feature: broker.FeatureResearchIndustry}))
	research.GET("/technical-indicators/:instrumentId", handleQuery(svc, queryRoute{feature: broker.FeatureTechnicalIndicator}))

	prediction := market.Group("/prediction")
	prediction.GET("/categories", handleQuery(svc, predictionRoute("categories")))
	prediction.GET("/competitions", handleQuery(svc, predictionRoute("competitions")))
	prediction.GET("/series", handleQuery(svc, predictionRoute("series")))
	prediction.GET("/events", handleQuery(svc, predictionRoute("events")))
	prediction.GET("/events/:eventId/contracts", handleQuery(svc, predictionRoute("contracts")))
	prediction.GET("/contracts/:code/snapshot", handleQuery(svc, queryRoute{feature: broker.FeaturePredictionSnapshot, operation: "snapshot", defaultMarket: "US", productClass: broker.ProductClassEventContract, segment: broker.MarketSegmentPrediction}))
	prediction.GET("/contracts/:code/order-book", handleQuery(svc, queryRoute{feature: broker.FeaturePredictionDepth, operation: "order_book", defaultMarket: "US", productClass: broker.ProductClassEventContract, segment: broker.MarketSegmentPrediction}))
	prediction.GET("/contracts/:code/candles", handleQuery(svc, queryRoute{feature: broker.FeaturePredictionHistory, operation: "candles", defaultMarket: "US", productClass: broker.ProductClassEventContract, segment: broker.MarketSegmentPrediction}))
	prediction.GET("/contracts/:code/candles/history", handleQuery(svc, queryRoute{feature: broker.FeaturePredictionHistory, operation: "historical", defaultMarket: "US", productClass: broker.ProductClassEventContract, segment: broker.MarketSegmentPrediction}))
	prediction.GET("/contracts/:code/ticks", handleQuery(svc, queryRoute{feature: broker.FeaturePredictionHistory, operation: "ticks", defaultMarket: "US", productClass: broker.ProductClassEventContract, segment: broker.MarketSegmentPrediction}))
	prediction.GET("/contracts/:code/milestones", handleQuery(svc, predictionRoute("milestones")))
	prediction.POST("/contracts/:code/subscriptions", handlePredictionSubscriptionAcquire(svc))
	prediction.DELETE("/contracts/:code/subscriptions/:leaseId", handlePredictionSubscriptionRelease(svc))
	prediction.GET("/combos/eligible-events", handleQuery(svc, queryRoute{feature: broker.FeaturePredictionComboEligible, operation: "eligible_events", defaultMarket: "US", productClass: broker.ProductClassEventContract, segment: broker.MarketSegmentPrediction}))
	prediction.POST("/combos/quotes", handlePredictionComboQuote(svc))
}

func registerSwaggerDocs() {
	for _, register := range []func() string{
		marketProductDocs,
		batchSnapshotDocs,
		zeroDteContractDocs,
		researchDocs,
		predictionReadDocs,
		predictionSubscriptionAcquireDocs,
		predictionSubscriptionReleaseDocs,
		predictionQuoteDocs,
		brokerCapabilityDocs,
		customizationDocs,
	} {
		if register() == "" {
			panic("product feature Swagger documentation marker is empty")
		}
	}
}

type predictionSubscriptionRequest struct {
	DataTypes []string `json:"dataTypes"`
}

func handlePredictionComboQuote(svc *service.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		var body service.PredictionComboQuoteRequest
		if err := c.ShouldBindJSON(&body); err != nil {
			httpserver.WriteError(c, http.StatusBadRequest, "BAD_REQUEST", "invalid prediction combo quote payload")
			return
		}
		if body.BrokerID == "" {
			body.BrokerID = c.Query("brokerId")
		}
		if body.AccountID == "" {
			body.AccountID = c.Query("accountId")
		}
		if body.TradingEnvironment == "" {
			body.TradingEnvironment = c.Query("tradingEnvironment")
		}
		result, err := svc.QuotePredictionCombo(c.Request.Context(), body)
		if err != nil {
			writeQueryError(c, err)
			return
		}
		httpserver.WriteOK(c, result)
	}
}

func handlePredictionSubscriptionAcquire(svc *service.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		var body predictionSubscriptionRequest
		if err := c.ShouldBindJSON(&body); err != nil {
			httpserver.WriteError(c, http.StatusBadRequest, "BAD_REQUEST", "invalid prediction subscription payload")
			return
		}
		lease, err := svc.AcquirePredictionSubscription(
			c.Request.Context(), c.Query("brokerId"), c.Query("accountId"),
			c.Param("code"), body.DataTypes,
		)
		if err != nil {
			writeQueryError(c, err)
			return
		}
		httpserver.WriteOK(c, lease)
	}
}

func handlePredictionSubscriptionRelease(svc *service.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		if err := svc.ReleasePredictionSubscription(c.Request.Context(), c.Param("leaseId")); err != nil {
			writeQueryError(c, err)
			return
		}
		httpserver.WriteOK(c, map[string]any{"released": true})
	}
}

type batchSnapshotsRequest struct {
	InstrumentIDs []string `json:"instrumentIds"`
	Symbols       []string `json:"symbols,omitempty"`
}

type zeroDteContractsRequest struct {
	BrokerID               string                           `json:"brokerId,omitempty"`
	AccountID              string                           `json:"accountId,omitempty"`
	TradingEnvironment     string                           `json:"tradingEnvironment,omitempty"`
	Market                 string                           `json:"market" binding:"required" enums:"US"`
	UnderlyingInstrumentID string                           `json:"underlyingInstrumentId" binding:"required"`
	UnderlyingProductClass broker.ProductClass              `json:"underlyingProductClass,omitempty" enums:"equity,index"`
	ExpiryTimestamp        int64                            `json:"expiryTimestamp" binding:"required"`
	Chain                  broker.OptionZeroDteChainLocator `json:"chain" binding:"required"`
	Sort                   string                           `json:"sort,omitempty" enums:"volume,open_interest,iv,delta"`
	OptionType             string                           `json:"optionType,omitempty" enums:"all,call,put"`
}

func handleZeroDteContracts(svc *service.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		var body zeroDteContractsRequest
		if err := c.ShouldBindJSON(&body); err != nil {
			httpserver.WriteError(c, http.StatusBadRequest, "OPTION_CHAIN_CONTEXT_REQUIRED", "invalid 0DTE chain context")
			return
		}
		if body.BrokerID == "" {
			body.BrokerID = c.Query("brokerId")
		}
		if body.AccountID == "" {
			body.AccountID = c.Query("accountId")
		}
		if body.TradingEnvironment == "" {
			body.TradingEnvironment = c.Query("tradingEnvironment")
		}
		result, err := svc.Query(c.Request.Context(), broker.FeatureQuery{
			BrokerID: body.BrokerID, AccountID: body.AccountID,
			TradingEnvironment: body.TradingEnvironment,
			Market:             strings.ToUpper(strings.TrimSpace(body.Market)),
			MarketSegment:      broker.MarketSegmentDerivatives,
			ProductClass:       broker.ProductClassOption,
			InstrumentID:       strings.ToUpper(strings.TrimSpace(body.UnderlyingInstrumentID)),
			FeatureID:          broker.FeatureOptionEvents,
			Params: map[string]any{
				"operation":              "zero_dte_contract",
				"underlyingProductClass": body.UnderlyingProductClass,
				"expiryTimestamp":        body.ExpiryTimestamp,
				"chainLocator":           body.Chain,
				"sort":                   body.Sort,
				"optionType":             body.OptionType,
			},
		})
		if err != nil {
			writeQueryError(c, err)
			return
		}
		httpserver.WriteOK(c, result)
	}
}

func handleBatchSnapshots(svc *service.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		var body batchSnapshotsRequest
		if err := c.ShouldBindJSON(&body); err != nil {
			httpserver.WriteError(c, http.StatusBadRequest, "BAD_REQUEST", "invalid request body")
			return
		}
		symbols := append(append([]string(nil), body.InstrumentIDs...), body.Symbols...)
		result, err := svc.BatchSnapshots(c.Request.Context(), broker.FeatureQuery{
			BrokerID:  c.Query("brokerId"),
			AccountID: c.Query("accountId"),
			Market:    c.Query("market"),
			Params:    queryParameters(c),
		}, symbols)
		if err != nil {
			writeQueryError(c, err)
			return
		}
		httpserver.WriteOK(c, result)
	}
}

func handleCustomization(svc *service.Service, featureID broker.FeatureID, action string) gin.HandlerFunc {
	return func(c *gin.Context) {
		var body map[string]any
		if err := c.ShouldBindJSON(&body); err != nil {
			httpserver.WriteError(c, http.StatusBadRequest, "BAD_REQUEST", "invalid request body")
			return
		}
		result, err := svc.ApplyCustomization(c.Request.Context(), broker.CustomizationAction{
			FeatureID: featureID, BrokerID: c.Query("brokerId"), AccountID: c.Query("accountId"),
			Action: action, Payload: body,
		})
		if err != nil {
			writeQueryError(c, err)
			return
		}
		httpserver.WriteOK(c, result)
	}
}

func predictionRoute(operation string) queryRoute {
	return queryRoute{
		feature:       broker.FeaturePredictionDiscover,
		operation:     operation,
		defaultMarket: "US",
		productClass:  broker.ProductClassEventContract,
		segment:       broker.MarketSegmentPrediction,
	}
}

func handleCapabilities(svc *service.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		httpserver.WriteOK(c, svc.CapabilitiesContext(c.Request.Context(), service.CapabilityQuery{
			BrokerID: c.Query("brokerId"), AccountID: c.Query("accountId"),
			TradingEnvironment: c.Query("tradingEnvironment"),
			Market:             strings.ToUpper(strings.TrimSpace(c.Query("market"))),
			FeatureID:          broker.FeatureID(strings.TrimSpace(c.Query("featureId"))),
			ProductClass:       broker.ProductClass(strings.TrimSpace(c.Query("productClass"))),
			MarketSegment:      broker.MarketSegment(strings.TrimSpace(c.Query("marketSegment"))),
		}))
	}
}

func handleQuery(svc *service.Service, route queryRoute) gin.HandlerFunc {
	return func(c *gin.Context) {
		query := routeQuery(c, route)
		result, err := svc.Query(c.Request.Context(), query)
		if err != nil {
			writeQueryError(c, err)
			return
		}
		httpserver.WriteOK(c, result)
	}
}

func handlePostQuery(svc *service.Service, route queryRoute) gin.HandlerFunc {
	return func(c *gin.Context) {
		query := routeQuery(c, route)
		var body map[string]any
		if err := c.ShouldBindJSON(&body); err != nil {
			httpserver.WriteError(c, http.StatusBadRequest, "BAD_REQUEST", "invalid request body")
			return
		}
		maps.Copy(query.Params, body)
		result, err := svc.Query(c.Request.Context(), query)
		if err != nil {
			writeQueryError(c, err)
			return
		}
		httpserver.WriteOK(c, result)
	}
}

func routeQuery(c *gin.Context, route queryRoute) broker.FeatureQuery {
	pageSize, _ := strconv.Atoi(c.Query("pageSize"))
	market := strings.ToUpper(strings.TrimSpace(c.Query("market")))
	instrumentID := firstPathValue(c, "instrumentId", "code", "eventId", "seriesId")
	if instrumentID == "" {
		instrumentID = firstQueryValue(c, "instrumentId", "underlying", "code", "eventId", "seriesId")
	}
	if market == "" {
		market = marketFromInstrument(instrumentID)
	}
	if market == "" {
		market = route.defaultMarket
	}
	params := queryParameters(c)
	if operation := strings.TrimSpace(c.Query("operation")); operation != "" {
		params["operation"] = operation
	} else if route.operation != "" {
		params["operation"] = route.operation
	}
	return broker.FeatureQuery{
		BrokerID:           c.Query("brokerId"),
		AccountID:          c.Query("accountId"),
		TradingEnvironment: c.Query("tradingEnvironment"),
		Market:             market,
		MarketSegment:      firstMarketSegment(c.Query("marketSegment"), route.segment),
		ProductClass:       firstProductClass(c.Query("productClass"), route.productClass),
		InstrumentID:       instrumentID,
		FeatureID:          route.feature,
		Cursor:             c.Query("cursor"),
		PageSize:           pageSize,
		Params:             params,
	}
}

func firstMarketSegment(value string, fallback broker.MarketSegment) broker.MarketSegment {
	switch normalized := broker.MarketSegment(strings.ToLower(strings.TrimSpace(value))); normalized {
	case broker.MarketSegmentSecurities, broker.MarketSegmentDerivatives, broker.MarketSegmentPrediction:
		return normalized
	default:
		return fallback
	}
}

func firstProductClass(value string, fallback broker.ProductClass) broker.ProductClass {
	normalized := broker.ProductClass(strings.ToLower(strings.TrimSpace(value)))
	switch normalized {
	case broker.ProductClassEquity, broker.ProductClassFund, broker.ProductClassOption,
		broker.ProductClassWarrant, broker.ProductClassCBBC, broker.ProductClassFuture,
		broker.ProductClassEventContract, broker.ProductClassIndex, broker.ProductClassBond,
		broker.ProductClassPlate:
		return normalized
	default:
		return fallback
	}
}

func queryParameters(c *gin.Context) map[string]any {
	excluded := map[string]struct{}{
		"brokerId": {}, "accountId": {}, "market": {}, "cursor": {}, "pageSize": {}, "operation": {},
		"instrumentId": {}, "underlying": {}, "code": {},
		"eventId": {}, "seriesId": {}, "marketSegment": {}, "productClass": {},
	}
	result := make(map[string]any)
	for key, values := range c.Request.URL.Query() {
		if _, skip := excluded[key]; skip || len(values) == 0 {
			continue
		}
		if len(values) == 1 {
			result[key] = parseQueryScalar(values[0])
			continue
		}
		items := make([]any, len(values))
		for index, value := range values {
			items[index] = parseQueryScalar(value)
		}
		result[key] = items
	}
	return result
}

func firstQueryValue(c *gin.Context, names ...string) string {
	for _, name := range names {
		if value := strings.TrimSpace(c.Query(name)); value != "" {
			return strings.ToUpper(value)
		}
	}
	return ""
}

func parseQueryScalar(value string) any {
	value = strings.TrimSpace(value)
	if value == "true" {
		return true
	}
	if value == "false" {
		return false
	}
	if integer, err := strconv.ParseInt(value, 10, 64); err == nil {
		return integer
	}
	if number, err := strconv.ParseFloat(value, 64); err == nil {
		return number
	}
	return value
}

func firstPathValue(c *gin.Context, names ...string) string {
	for _, name := range names {
		if value := strings.TrimSpace(c.Param(name)); value != "" {
			return strings.ToUpper(value)
		}
	}
	return ""
}

func marketFromInstrument(instrumentID string) string {
	if before, _, ok := strings.Cut(instrumentID, "."); ok {
		switch strings.ToUpper(before) {
		case "HK", "US", "SH", "SZ":
			return strings.ToUpper(before)
		}
	}
	return ""
}

func writeQueryError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, broker.ErrSnapshotRateLimited):
		retryAfter, ok := broker.SnapshotRetryAfter(err)
		if !ok {
			retryAfter = time.Second
		}
		seconds := max(int64((retryAfter+time.Second-1)/time.Second), 1)
		c.Header("Retry-After", strconv.FormatInt(seconds, 10))
		httpserver.WriteError(c, http.StatusTooManyRequests, "MARKET_SNAPSHOT_RATE_LIMITED", err.Error())
	case errors.Is(err, service.ErrOptionChainContext):
		httpserver.WriteError(c, http.StatusBadRequest, "OPTION_CHAIN_CONTEXT_REQUIRED", err.Error())
	case errors.Is(err, service.ErrOptionZeroDTEMarket):
		httpserver.WriteError(c, http.StatusUnprocessableEntity, "OPTION_ZERO_DTE_UNAVAILABLE", err.Error())
	case errors.Is(err, service.ErrInvalidQuery):
		httpserver.WriteError(c, http.StatusBadRequest, "BAD_REQUEST", err.Error())
	case errors.Is(err, service.ErrPredictionIneligible):
		httpserver.WriteError(c, http.StatusForbidden, "PREDICTION_MARKET_INELIGIBLE", err.Error())
	case errors.Is(err, service.ErrCapabilityUnavailable):
		httpserver.WriteError(c, http.StatusConflict, "BROKER_CAPABILITY_UNAVAILABLE", err.Error())
	default:
		httpserver.WriteError(c, http.StatusBadGateway, "BROKER_FEATURE_FAILED", err.Error())
	}
}

// marketProductDocs godoc
// @Summary 查询全产品行情、衍生品和资讯
// @Tags market-data-products
// @Produce json
// @Success 200 {object} httpserver.Envelope
// @Failure 409 {object} httpserver.Envelope
// @Failure 502 {object} httpserver.Envelope
// @Router /api/v1/market-data/instruments/{instrumentId}/profile [get]
// @Router /api/v1/market-data/intraday/{instrumentId} [get]
// @Router /api/v1/market-data/ticks/{instrumentId} [get]
// @Router /api/v1/market-data/broker-queue/{instrumentId} [get]
// @Router /api/v1/market-data/capital-flow/{instrumentId} [get]
// @Router /api/v1/market-data/options/chains/{instrumentId} [get]
// @Router /api/v1/market-data/options/expirations/{instrumentId} [get]
// @Router /api/v1/market-data/options/screens [get]
// @Router /api/v1/market-data/options/analysis/{instrumentId} [get]
// @Router /api/v1/market-data/options/analysis/{instrumentId} [post]
// @Router /api/v1/market-data/options/events [get]
// @Router /api/v1/market-data/warrants [get]
// @Router /api/v1/market-data/futures [get]
// @Router /api/v1/market-data/news [get]
//
//go:noinline
func marketProductDocs() string { return "market-products" }

// batchSnapshotDocs godoc
// @Summary 批量查询非订阅证券快照
// @Tags market-data-products
// @Accept json
// @Produce json
// @Param request body batchSnapshotsRequest true "批量标的"
// @Success 200 {object} httpserver.Envelope
// @Failure 400 {object} httpserver.Envelope
// @Failure 409 {object} httpserver.Envelope
// @Failure 429 {object} httpserver.Envelope
// @Failure 502 {object} httpserver.Envelope
// @Router /api/v1/market-data/snapshots [post]
//
//go:noinline
func batchSnapshotDocs() string { return "batch-snapshots" }

// zeroDteContractDocs godoc
// @Summary 使用标的筛选返回的链上下文查询 0DTE 合约
// @Tags market-data-products
// @Accept json
// @Produce json
// @Param request body zeroDteContractsRequest true "0DTE 标的与期权链上下文"
// @Success 200 {object} httpserver.Envelope
// @Failure 400 {object} httpserver.Envelope
// @Failure 409 {object} httpserver.Envelope
// @Failure 422 {object} httpserver.Envelope
// @Failure 502 {object} httpserver.Envelope
// @Router /api/v1/market-data/options/events/zero-dte-contracts [post]
//
//go:noinline
func zeroDteContractDocs() string { return "zero-dte-contracts" }

// researchDocs godoc
// @Summary 查询公司、市场和宏观研究
// @Tags research
// @Produce json
// @Success 200 {object} httpserver.Envelope
// @Failure 409 {object} httpserver.Envelope
// @Failure 502 {object} httpserver.Envelope
// @Router /api/v1/research/instruments/{instrumentId} [get]
// @Router /api/v1/research/financials/{instrumentId} [get]
// @Router /api/v1/research/valuation/{instrumentId} [get]
// @Router /api/v1/research/analyst/{instrumentId} [get]
// @Router /api/v1/research/ownership/{instrumentId} [get]
// @Router /api/v1/research/corporate-actions/{instrumentId} [get]
// @Router /api/v1/research/short-interest/{instrumentId} [get]
// @Router /api/v1/research/screens [get]
// @Router /api/v1/research/calendars [get]
// @Router /api/v1/research/macro [get]
// @Router /api/v1/research/rankings [get]
// @Router /api/v1/research/institutions [get]
// @Router /api/v1/research/industries [get]
// @Router /api/v1/research/technical-indicators/{instrumentId} [get]
//
//go:noinline
func researchDocs() string { return "research" }

// predictionReadDocs godoc
// @Summary 查询预测市场目录、合约和 Parlay 资格
// @Tags prediction-market
// @Produce json
// @Success 200 {object} httpserver.Envelope
// @Failure 403 {object} httpserver.Envelope
// @Failure 409 {object} httpserver.Envelope
// @Failure 502 {object} httpserver.Envelope
// @Router /api/v1/market-data/prediction/categories [get]
// @Router /api/v1/market-data/prediction/competitions [get]
// @Router /api/v1/market-data/prediction/series [get]
// @Router /api/v1/market-data/prediction/events [get]
// @Router /api/v1/market-data/prediction/events/{eventId}/contracts [get]
// @Router /api/v1/market-data/prediction/contracts/{code}/snapshot [get]
// @Router /api/v1/market-data/prediction/contracts/{code}/order-book [get]
// @Router /api/v1/market-data/prediction/contracts/{code}/candles [get]
// @Router /api/v1/market-data/prediction/contracts/{code}/candles/history [get]
// @Router /api/v1/market-data/prediction/contracts/{code}/ticks [get]
// @Router /api/v1/market-data/prediction/contracts/{code}/milestones [get]
// @Router /api/v1/market-data/prediction/combos/eligible-events [get]
//
//go:noinline
func predictionReadDocs() string { return "prediction-read" }

// predictionSubscriptionAcquireDocs godoc
// @Summary 为可见预测合约申请实时数据订阅租约
// @Tags prediction-market
// @Accept json
// @Produce json
// @Param code path string true "预测合约代码"
// @Param request body predictionSubscriptionRequest true "盘口、K 线或逐笔类型"
// @Success 200 {object} httpserver.Envelope
// @Failure 400 {object} httpserver.Envelope
// @Failure 403 {object} httpserver.Envelope
// @Router /api/v1/market-data/prediction/contracts/{code}/subscriptions [post]
//
//go:noinline
func predictionSubscriptionAcquireDocs() string { return "prediction-subscription-acquire" }

// predictionSubscriptionReleaseDocs godoc
// @Summary 释放预测合约实时数据订阅租约
// @Tags prediction-market
// @Produce json
// @Param code path string true "预测合约代码"
// @Param leaseId path string true "订阅租约 ID"
// @Success 200 {object} httpserver.Envelope
// @Router /api/v1/market-data/prediction/contracts/{code}/subscriptions/{leaseId} [delete]
//
//go:noinline
func predictionSubscriptionReleaseDocs() string { return "prediction-subscription-release" }

// predictionQuoteDocs godoc
// @Summary 预测市场 Parlay RFQ
// @Tags prediction-market
// @Accept json
// @Produce json
// @Param request body map[string]any true "Parlay legs and MVC"
// @Success 200 {object} httpserver.Envelope
// @Failure 400 {object} httpserver.Envelope
// @Failure 403 {object} httpserver.Envelope
// @Router /api/v1/market-data/prediction/combos/quotes [post]
//
//go:noinline
func predictionQuoteDocs() string { return "prediction-quote" }

// brokerCapabilityDocs godoc
// @Summary 查询机器可检查的券商能力目录
// @Tags brokers
// @Produce json
// @Success 200 {object} httpserver.Envelope
// @Router /api/v1/brokers/capabilities [get]
//
//go:noinline
func brokerCapabilityDocs() string { return "broker-capabilities" }

// customizationDocs godoc
// @Summary 查询或修改券商提醒与远程自选
// @Tags customization
// @Accept json
// @Produce json
// @Success 200 {object} httpserver.Envelope
// @Failure 400 {object} httpserver.Envelope
// @Failure 502 {object} httpserver.Envelope
// @Router /api/v1/alerts/price [get]
// @Router /api/v1/alerts/price [post]
// @Router /api/v1/alerts/option-events [get]
// @Router /api/v1/alerts/option-events [post]
// @Router /api/v1/watchlists/remote [get]
// @Router /api/v1/watchlists/remote [post]
//
//go:noinline
func customizationDocs() string { return "customization" }
