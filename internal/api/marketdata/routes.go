package marketdata

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/jftrade/jftrade-main/internal/api/httpserver"
	srv "github.com/jftrade/jftrade-main/internal/marketdata"
	productfeatures "github.com/jftrade/jftrade-main/internal/productfeatures"
	"github.com/jftrade/jftrade-main/pkg/broker"
)

type BrokerMarketDataReader interface {
	ReadMarketSnapshot(context.Context, string, string, string, bool) (map[string]any, error)
	ReadMarketSecurityDetails(context.Context, string, string, string) (map[string]any, error)
	ReadMarketCandles(context.Context, string, string, string, string, int, string, string, string, []string) (map[string]any, error)
	ReadMarketDepth(context.Context, string, string, string, int) (map[string]any, error)
}

func firstBrokerMarketDataReader(readers []BrokerMarketDataReader) BrokerMarketDataReader {
	if len(readers) == 0 {
		return nil
	}
	return readers[0]
}

func usesActiveNonBrokerProvider(
	ctx context.Context,
	svc *srv.Service,
	providerID string,
) bool {
	providerID = strings.TrimSpace(providerID)
	if providerID == "" || svc == nil {
		return false
	}
	descriptor, err := svc.ProviderDescriptor(ctx)
	if err != nil || strings.EqualFold(descriptor.BrokerID, "futu") {
		return false
	}
	return strings.EqualFold(providerID, descriptor.BrokerID) ||
		strings.EqualFold(providerID, descriptor.ProviderID)
}

// RegisterRoutes 注册所有 /api/v1 下的行情路由。
// WebSocket /ws/live 由应用装配层单独注册。
func RegisterRoutes(api *gin.RouterGroup, svc *srv.Service, brokerReaders ...BrokerMarketDataReader) {
	var brokerReader BrokerMarketDataReader
	if len(brokerReaders) > 0 {
		brokerReader = brokerReaders[0]
	}
	market := api.Group("/market-data")
	market.GET("/provider", handleProvider(svc))
	market.GET("/markets", handleMarkets(svc))
	market.GET("/instruments", handleInstrumentSearch(svc))
	market.POST("/instruments/normalize", handleNormalizeInstrument(svc))
	market.GET("/subscriptions", handleGetSubscriptions(svc))
	market.POST("/subscriptions", handleAcquireSubscription(svc))
	market.DELETE("/subscriptions", handleClearSubscriptions(svc))
	market.POST("/subscriptions/release", handleReleaseSubscription(svc))
	market.POST("/subscriptions/heartbeat", handleHeartbeat(svc))
	market.GET("/securities/:market/:symbol", handleSecurityDetails(svc, brokerReader))
	market.GET("/snapshots/:market/:symbol", handleSnapshot(svc, brokerReader))
	market.GET("/candles/:market/:symbol", handleCandles(svc, brokerReader))
	market.GET("/depth/:market/:symbol", handleDepth(svc, brokerReader))
}

// handleProvider godoc
// @Summary 查询行情 Provider 能力与运行状态
// @Tags market-data
// @Produce json
// @Success 200 {object} httpserver.Envelope{data=srv.ProviderStatusResponse}
// @Failure 502 {object} httpserver.ErrorEnvelope
// @Router /api/v1/market-data/provider [get]
func handleProvider(svc *srv.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		status, err := svc.ProviderStatus(c.Request.Context())
		if err != nil {
			httpserver.WriteError(c, 502, "MARKET_DATA_PROVIDER_FAILED", err.Error())
			return
		}
		httpserver.WriteOK(c, status)
	}
}

// handleMarkets godoc
// @Summary 返回可用市场列表
// @Tags market-data
// @Produce json
// @Success 200 {object} httpserver.Envelope{data=MarketsData}
// @Failure 500 {object} httpserver.ErrorEnvelope
// @Router /api/v1/market-data/markets [get]
func handleMarkets(svc *srv.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		markets, err := svc.GetMarkets(c.Request.Context())
		if err != nil {
			httpserver.WriteError(c, 500, "MARKET_DATA_FAILED", err.Error())
			return
		}
		descriptor, err := svc.ProviderDescriptor(c.Request.Context())
		if err != nil {
			httpserver.WriteError(c, 500, "MARKET_DATA_FAILED", err.Error())
			return
		}
		httpserver.WriteOK(c, map[string]any{
			"defaultMarket": descriptor.DefaultMarket,
			"markets":       markets,
		})
	}
}

// handleSecurityDetails godoc
// @Summary 查询证券详情
// @Tags market-data
// @Produce json
// @Param market path string true "市场"
// @Param symbol path string true "标的"
// @Param brokerId query string false "行情提供者；省略时使用服务端默认"
// @Success 200 {object} httpserver.Envelope{data=SecurityDetailsData}
// @Failure 400 {object} httpserver.ErrorEnvelope
// @Failure 409 {object} httpserver.ErrorEnvelope
// @Failure 429 {object} httpserver.ErrorEnvelope
// @Failure 503 {object} httpserver.ErrorEnvelope
// @Failure 502 {object} httpserver.ErrorEnvelope
// @Router /api/v1/market-data/securities/{market}/{symbol} [get]
func handleSecurityDetails(svc *srv.Service, brokerReaders ...BrokerMarketDataReader) gin.HandlerFunc {
	brokerReader := firstBrokerMarketDataReader(brokerReaders)
	return func(c *gin.Context) {
		var uri struct {
			Market string `uri:"market" binding:"required"`
			Symbol string `uri:"symbol" binding:"required"`
		}
		if err := httpserver.BindURI(c, &uri); err != nil {
			httpserver.WriteError(c, 400, "BAD_REQUEST", "invalid instrument")
			return
		}
		var details map[string]any
		var err error
		if brokerID := strings.TrimSpace(c.Query("brokerId")); brokerID != "" &&
			!usesActiveNonBrokerProvider(c.Request.Context(), svc, brokerID) {
			if brokerReader == nil {
				err = productfeatures.ErrCapabilityUnavailable
			} else {
				details, err = brokerReader.ReadMarketSecurityDetails(
					c.Request.Context(), brokerID, uri.Market, uri.Symbol,
				)
			}
		} else {
			details, err = svc.GetSecurityDetails(c.Request.Context(), uri.Market, uri.Symbol)
		}
		if err != nil {
			writeBrokerMarketDataReadError(c, "MARKET_SECURITY_DETAILS_FAILED", err)
			return
		}
		httpserver.WriteOK(c, details)
	}
}

// handleSnapshot godoc
// @Summary 读取行情快照
// @Tags market-data
// @Produce json
// @Param market path string true "市场代码"
// @Param symbol path string true "证券代码"
// @Param refresh query bool false "是否绕过缓存强制刷新"
// @Param brokerId query string false "行情提供者；省略时使用服务端默认"
// @Success 200 {object} httpserver.Envelope{data=SnapshotData}
// @Failure 400 {object} httpserver.ErrorEnvelope
// @Failure 409 {object} httpserver.ErrorEnvelope
// @Failure 429 {object} httpserver.ErrorEnvelope
// @Failure 503 {object} httpserver.ErrorEnvelope
// @Failure 502 {object} httpserver.ErrorEnvelope
// @Router /api/v1/market-data/snapshots/{market}/{symbol} [get]
func handleSnapshot(svc *srv.Service, brokerReaders ...BrokerMarketDataReader) gin.HandlerFunc {
	brokerReader := firstBrokerMarketDataReader(brokerReaders)
	return func(c *gin.Context) {
		var uri struct {
			Market string `uri:"market" binding:"required"`
			Symbol string `uri:"symbol" binding:"required"`
		}
		if err := httpserver.BindURI(c, &uri); err != nil {
			httpserver.WriteError(c, 400, "BAD_REQUEST", "invalid instrument")
			return
		}
		var refreshValue httpserver.OptionalBoolValue
		if raw := c.Query("refresh"); raw != "" {
			if err := refreshValue.UnmarshalText([]byte(raw)); err != nil {
				httpserver.WriteError(c, http.StatusBadRequest, "BAD_REQUEST", "invalid refresh query")
				return
			}
		}
		refresh := refreshValue.Bool()

		var snapshot map[string]any
		var err error
		if brokerID := strings.TrimSpace(c.Query("brokerId")); brokerID != "" &&
			!usesActiveNonBrokerProvider(c.Request.Context(), svc, brokerID) {
			if brokerReader == nil {
				err = productfeatures.ErrCapabilityUnavailable
			} else {
				snapshot, err = brokerReader.ReadMarketSnapshot(
					c.Request.Context(), brokerID, uri.Market, uri.Symbol, refresh,
				)
			}
		} else {
			snapshot, err = svc.GetSnapshot(c.Request.Context(), uri.Market, uri.Symbol, refresh)
		}
		if err != nil {
			writeBrokerMarketDataReadError(c, "MARKET_SNAPSHOT_FAILED", err)
			return
		}
		httpserver.WriteOK(c, snapshot)
	}
}

// handleCandles godoc
// @Summary 查询 K 线
// @Tags market-data
// @Produce json
// @Param market path string true "市场"
// @Param symbol path string true "标的"
// @Param period query string false "周期"
// @Param limit query int false "数量"
// @Param fromTime query string false "起始时间"
// @Param toTime query string false "结束时间"
// @Param before query string false "严格早于该 RFC3339 时间的历史分页游标"
// @Param sessions query []string false "交易时段：regular,extended,overnight" collectionFormat(csv)
// @Param brokerId query string false "行情提供者；省略时使用服务端默认"
// @Success 200 {object} httpserver.Envelope{data=CandlesData}
// @Failure 400 {object} httpserver.ErrorEnvelope
// @Failure 409 {object} httpserver.ErrorEnvelope
// @Failure 429 {object} httpserver.ErrorEnvelope
// @Failure 503 {object} httpserver.ErrorEnvelope
// @Failure 502 {object} httpserver.ErrorEnvelope
// @Router /api/v1/market-data/candles/{market}/{symbol} [get]
func handleCandles(svc *srv.Service, brokerReaders ...BrokerMarketDataReader) gin.HandlerFunc {
	brokerReader := firstBrokerMarketDataReader(brokerReaders)
	return func(c *gin.Context) {
		var uri struct {
			Market string `uri:"market" binding:"required"`
			Symbol string `uri:"symbol" binding:"required"`
		}
		if err := httpserver.BindURI(c, &uri); err != nil {
			httpserver.WriteError(c, 400, "BAD_REQUEST", "invalid instrument")
			return
		}
		query, parseErr := parseCandleRouteQuery(c)
		if parseErr != nil {
			httpserver.WriteError(c, 400, "BAD_REQUEST", parseErr.Error())
			return
		}

		var result map[string]any
		var err error
		if query.period == "tick" {
			result, err = svc.GetCandles(c.Request.Context(), srv.HistoricalCandlesQuery{
				Market: uri.Market, Symbol: uri.Symbol, Period: query.period,
				Limit: query.limit, FromTime: query.fromTime, ToTime: query.toTime,
				Sessions: query.sessions, SessionsSpecified: query.sessionsSpecified,
			})
			if err == nil {
				result["pagination"] = map[string]any{"hasMore": false}
			}
		} else if brokerID := strings.TrimSpace(c.Query("brokerId")); brokerID != "" &&
			!usesActiveNonBrokerProvider(c.Request.Context(), svc, brokerID) {
			if brokerReader == nil {
				err = productfeatures.ErrCapabilityUnavailable
			} else {
				result, err = brokerReader.ReadMarketCandles(
					c.Request.Context(), brokerID, uri.Market, uri.Symbol,
					query.period, query.limit, query.fromTime, query.toTime, query.beforeTime,
					srv.CandleSessionStrings(query.sessions),
				)
			}
		} else {
			result, err = svc.GetCandles(c.Request.Context(), srv.HistoricalCandlesQuery{
				Market: uri.Market, Symbol: uri.Symbol, Period: query.period,
				Limit: query.limit, FromTime: query.fromTime, ToTime: query.toTime,
				BeforeTime: query.beforeTime,
				Sessions: query.sessions, SessionsSpecified: query.sessionsSpecified,
			})
		}
		if err == nil && (query.fromTime != "" || query.toTime != "") {
			// Explicit ranges cannot be continued with a bare before cursor.
			result["pagination"] = map[string]any{"hasMore": false}
		}
		if err != nil {
			writeBrokerMarketDataReadError(
				c,
				providerFailureCode(
					c.Request.Context(),
					svc,
					c.Query("brokerId"),
					"OPEND_CANDLES_FAILED",
					"MARKET_CANDLES_FAILED",
				),
				err,
			)
			return
		}
		httpserver.WriteOK(c, result)
	}
}

type candleRouteQuery struct {
	period            string
	limit             int
	fromTime          string
	toTime            string
	beforeTime        string
	sessions          []srv.CandleSession
	sessionsSpecified bool
}

func parseCandleRouteQuery(c *gin.Context) (candleRouteQuery, error) {
	query := candleRouteQuery{period: "1m"}
	if values, ok := c.Request.URL.Query()["sessions"]; ok {
		sessions, err := srv.ParseCandleSessions(values)
		if err != nil {
			return candleRouteQuery{}, err
		}
		query.sessions = sessions
		query.sessionsSpecified = true
	}
	if raw := c.Query("period"); raw != "" {
		period, err := httpserver.NormalizeCandlePeriod(raw)
		if err != nil {
			return candleRouteQuery{}, errors.New("invalid candle query")
		}
		query.period = period
	}
	if rawLimit := c.Query("limit"); rawLimit != "" {
		parsed := httpserver.OptionalIntValue{}
		if err := parsed.UnmarshalText([]byte(rawLimit)); err != nil {
			return candleRouteQuery{}, errors.New("limit must be an integer")
		}
		query.limit = parsed.Int()
	}
	fromTime, err := normalizeOptionalQueryTime(c.Query("fromTime"))
	if err != nil {
		return candleRouteQuery{}, err
	}
	query.fromTime = fromTime
	if query.fromTime == "" {
		from, err := normalizeOptionalQueryTime(c.Query("from"))
		if err != nil {
			return candleRouteQuery{}, err
		}
		query.fromTime = from
	}
	toTime, err := normalizeOptionalQueryTime(c.Query("toTime"))
	if err != nil {
		return candleRouteQuery{}, err
	}
	query.toTime = toTime
	if query.toTime == "" {
		to, err := normalizeOptionalQueryTime(c.Query("to"))
		if err != nil {
			return candleRouteQuery{}, err
		}
		query.toTime = to
	}
	if rawBefore := strings.TrimSpace(c.Query("before")); rawBefore != "" {
		beforeAt, err := time.Parse(time.RFC3339Nano, rawBefore)
		if err != nil {
			return candleRouteQuery{}, errors.New("before must be an RFC3339 timestamp")
		}
		query.beforeTime = beforeAt.UTC().Format(time.RFC3339Nano)
	}
	if query.beforeTime != "" && (query.fromTime != "" || query.toTime != "") {
		return candleRouteQuery{}, errors.New("before cannot be combined with from or to")
	}
	if query.period == "tick" && query.beforeTime != "" {
		return candleRouteQuery{}, errors.New("tick candles do not support historical pagination")
	}
	return query, nil
}

func writeMarketDataReadError(c *gin.Context, fallbackCode string, err error) {
	switch {
	case errors.Is(err, srv.ErrInvalidCandleSessions):
		httpserver.WriteError(c, http.StatusBadRequest, "MARKET_CANDLE_SESSIONS_INVALID", err.Error())
	case errors.Is(err, srv.ErrSubscriptionRequired):
		httpserver.WriteError(c, http.StatusConflict, "MARKET_DATA_SUBSCRIPTION_REQUIRED", err.Error())
	case errors.Is(err, srv.ErrCapabilityUnsupported):
		httpserver.WriteError(c, http.StatusConflict, "MARKET_DATA_CAPABILITY_UNSUPPORTED", err.Error())
	case errors.Is(err, srv.ErrProviderChanged):
		httpserver.WriteError(c, http.StatusConflict, "MARKET_DATA_PROVIDER_CHANGED", err.Error())
	case errors.Is(err, srv.ErrProviderWarming):
		c.Header("Retry-After", "1")
		httpserver.WriteError(
			c,
			http.StatusServiceUnavailable,
			"MARKET_DATA_PROVIDER_WARMING",
			"行情服务正在预热，请稍后重试",
		)
	case errors.Is(err, srv.ErrProviderBusy):
		c.Header("Retry-After", "2")
		httpserver.WriteError(
			c,
			http.StatusServiceUnavailable,
			"MARKET_DATA_PROVIDER_BUSY",
			"行情服务当前繁忙，请稍后重试",
		)
	default:
		httpserver.WriteError(c, http.StatusBadGateway, fallbackCode, err.Error())
	}
}

func providerFailureCode(
	ctx context.Context,
	svc *srv.Service,
	explicitBrokerID string,
	futuCode string,
	genericCode string,
) string {
	brokerID := strings.ToLower(strings.TrimSpace(explicitBrokerID))
	if brokerID != "" {
		if brokerID == "futu" {
			return futuCode
		}
		return genericCode
	}
	descriptor, err := svc.ProviderDescriptor(ctx)
	if err == nil && strings.EqualFold(descriptor.BrokerID, "futu") {
		return futuCode
	}
	return genericCode
}

func writeBrokerMarketDataReadError(c *gin.Context, fallbackCode string, err error) {
	switch {
	case errors.Is(err, broker.ErrInvalidCandleSessions):
		httpserver.WriteError(c, http.StatusBadRequest, "MARKET_CANDLE_SESSIONS_INVALID", err.Error())
	case errors.Is(err, broker.ErrSnapshotRateLimited):
		retryAfter, ok := broker.SnapshotRetryAfter(err)
		if !ok {
			retryAfter = time.Second
		}
		seconds := max(int64((retryAfter+time.Second-1)/time.Second), 1)
		c.Header("Retry-After", strconv.FormatInt(seconds, 10))
		httpserver.WriteError(c, http.StatusTooManyRequests, "MARKET_SNAPSHOT_RATE_LIMITED", err.Error())
	case errors.Is(err, productfeatures.ErrInvalidQuery):
		httpserver.WriteError(c, http.StatusBadRequest, "MARKET_DATA_QUERY_INVALID", err.Error())
	case errors.Is(err, productfeatures.ErrCapabilityUnavailable):
		httpserver.WriteError(c, http.StatusConflict, "BROKER_CAPABILITY_UNAVAILABLE", err.Error())
	default:
		writeMarketDataReadError(c, fallbackCode, err)
	}
}

func normalizeOptionalQueryTime(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", nil
	}
	parsed := httpserver.OptionalTimeValue{}
	if err := parsed.UnmarshalText([]byte(value)); err != nil {
		return "", errors.New("time must be a valid timestamp")
	}
	return parsed.UTC().Format(time.RFC3339Nano), nil
}

// handleDepth godoc
// @Summary 读取盘口深度
// @Tags market-data
// @Produce json
// @Param market path string true "市场代码"
// @Param symbol path string true "证券代码"
// @Param num query int false "档数，默认 10，最大 50"
// @Param brokerId query string false "行情提供者；省略时使用服务端默认"
// @Success 200 {object} httpserver.Envelope{data=DepthData}
// @Failure 400 {object} httpserver.ErrorEnvelope
// @Failure 409 {object} httpserver.ErrorEnvelope
// @Failure 429 {object} httpserver.ErrorEnvelope
// @Failure 502 {object} httpserver.ErrorEnvelope
// @Router /api/v1/market-data/depth/{market}/{symbol} [get]
func handleDepth(svc *srv.Service, brokerReaders ...BrokerMarketDataReader) gin.HandlerFunc {
	brokerReader := firstBrokerMarketDataReader(brokerReaders)
	return func(c *gin.Context) {
		var uri struct {
			Market string `uri:"market" binding:"required"`
			Symbol string `uri:"symbol" binding:"required"`
		}
		if err := httpserver.BindURI(c, &uri); err != nil {
			httpserver.WriteError(c, 400, "BAD_REQUEST", "invalid instrument")
			return
		}
		num := 10
		if n := c.Query("num"); n != "" {
			parsed := httpserver.OptionalIntValue{}
			if err := parsed.UnmarshalText([]byte(n)); err != nil {
				httpserver.WriteError(c, http.StatusBadRequest, "BAD_REQUEST", "num must be an integer")
				return
			}
			num = parsed.Int()
		}
		var result map[string]any
		var err error
		if brokerID := strings.TrimSpace(c.Query("brokerId")); brokerID != "" &&
			!usesActiveNonBrokerProvider(c.Request.Context(), svc, brokerID) {
			if brokerReader == nil {
				err = productfeatures.ErrCapabilityUnavailable
			} else {
				result, err = brokerReader.ReadMarketDepth(
					c.Request.Context(), brokerID, uri.Market, uri.Symbol, num,
				)
			}
		} else {
			result, err = svc.GetDepth(c.Request.Context(), uri.Market, uri.Symbol, num)
		}
		if err != nil {
			writeBrokerMarketDataReadError(
				c,
				providerFailureCode(
					c.Request.Context(),
					svc,
					c.Query("brokerId"),
					"OPEND_DEPTH_FAILED",
					"MARKET_DEPTH_FAILED",
				),
				err,
			)
			return
		}
		httpserver.WriteOK(c, result)
	}
}

// handleInstrumentSearch godoc
// @Summary 按代码或名称搜索行情标的
// @Tags market-data
// @Produce json
// @Param market query string false "市场筛选：HK、US、CN、SH 或 SZ；省略时搜索全部市场"
// @Param query query string true "证券代码、名称或完整 MARKET.CODE"
// @Param limit query int false "返回数量，默认 20，范围 1..100"
// @Success 200 {object} httpserver.Envelope{data=marketdata.InstrumentResolution}
// @Failure 400 {object} httpserver.ErrorEnvelope
// @Failure 503 {object} httpserver.ErrorEnvelope
// @Failure 502 {object} httpserver.ErrorEnvelope
// @Router /api/v1/market-data/instruments [get]
func handleInstrumentSearch(svc *srv.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		query := strings.TrimSpace(c.Query("query"))
		if query == "" {
			httpserver.WriteError(c, 400, "MARKET_INSTRUMENT_INVALID", "query is required")
			return
		}
		limit := 20
		if rawLimit := strings.TrimSpace(c.Query("limit")); rawLimit != "" {
			parsed, err := strconv.Atoi(rawLimit)
			if err != nil || parsed < 1 || parsed > 100 {
				httpserver.WriteError(c, 400, "MARKET_INSTRUMENT_INVALID", "limit must be between 1 and 100")
				return
			}
			limit = parsed
		}
		result, err := svc.ResolveInstrument(c.Request.Context(), c.Query("market"), query, limit)
		if err != nil {
			if srv.IsInstrumentSearchInputError(err) {
				httpserver.WriteError(c, 400, "MARKET_INSTRUMENT_INVALID", err.Error())
				return
			}
			writeMarketDataReadError(c, "MARKET_INSTRUMENT_SEARCH_FAILED", err)
			return
		}
		httpserver.WriteOK(c, result)
	}
}

// handleNormalizeInstrument godoc
// @Summary 规范化行情标的
// @Tags market-data
// @Accept json
// @Produce json
// @Param request body NormalizeInstrumentRequest true "标的别名"
// @Success 200 {object} httpserver.Envelope{data=NormalizeInstrumentData}
// @Failure 400 {object} httpserver.ErrorEnvelope
// @Router /api/v1/market-data/instruments/normalize [post]
func handleNormalizeInstrument(svc *srv.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req map[string]any
		if err := c.ShouldBindJSON(&req); err != nil {
			httpserver.WriteError(c, 400, "BAD_REQUEST", "invalid normalize request")
			return
		}
		result, err := svc.NormalizeInstrument(c.Request.Context(), req)
		if err != nil {
			httpserver.WriteError(c, 400, "MARKET_INSTRUMENT_INVALID", err.Error())
			return
		}
		httpserver.WriteOK(c, result)
	}
}
