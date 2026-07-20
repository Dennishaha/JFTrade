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
	"github.com/jftrade/jftrade-main/pkg/besteffort"
	"github.com/jftrade/jftrade-main/pkg/broker"
)

type subscriptionRequest struct {
	ConsumerID       string              `json:"consumerId"`
	ProviderBrokerID string              `json:"providerBrokerId,omitempty"`
	Instruments      []srv.InstrumentRef `json:"instruments"`
}

type BrokerMarketDataReader interface {
	ReadMarketSnapshot(context.Context, string, string, string, bool) (map[string]any, error)
	ReadMarketSecurityDetails(context.Context, string, string, string) (map[string]any, error)
	ReadMarketCandles(context.Context, string, string, string, string, int, string, string, string) (map[string]any, error)
	ReadMarketDepth(context.Context, string, string, string, int) (map[string]any, error)
}

func firstBrokerMarketDataReader(readers []BrokerMarketDataReader) BrokerMarketDataReader {
	if len(readers) == 0 {
		return nil
	}
	return readers[0]
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
// @Success 200 {object} httpserver.Envelope
// @Failure 502 {object} httpserver.Envelope
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

// handleMarkets 返回可用市场列表。
func handleMarkets(svc *srv.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		markets, err := svc.GetMarkets(c.Request.Context())
		if err != nil {
			httpserver.WriteError(c, 500, "MARKET_DATA_FAILED", err.Error())
			return
		}
		httpserver.WriteOK(c, map[string]any{
			"defaultMarket": "HK",
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
// @Success 200 {object} httpserver.Envelope
// @Failure 409 {object} httpserver.Envelope
// @Failure 429 {object} httpserver.Envelope
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
		if brokerID := strings.TrimSpace(c.Query("brokerId")); brokerID != "" {
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
// @Success 200 {object} httpserver.Envelope
// @Failure 400 {object} httpserver.Envelope
// @Failure 409 {object} httpserver.Envelope
// @Failure 502 {object} httpserver.Envelope
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
			jftradeErr3 := refreshValue.UnmarshalText([]byte(raw))
			besteffort.LogError(jftradeErr3)
		}
		refresh := refreshValue.Bool()

		var snapshot map[string]any
		var err error
		if brokerID := strings.TrimSpace(c.Query("brokerId")); brokerID != "" {
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
// @Param brokerId query string false "行情提供者；省略时使用服务端默认"
// @Success 200 {object} httpserver.Envelope
// @Failure 409 {object} httpserver.Envelope
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
			result, err = svc.GetCandles(
				c.Request.Context(), uri.Market, uri.Symbol,
				query.period, query.limit, query.fromTime, query.toTime,
			)
			if err == nil {
				result["pagination"] = map[string]any{"hasMore": false}
			}
		} else if brokerID := strings.TrimSpace(c.Query("brokerId")); brokerID != "" {
			if brokerReader == nil {
				err = productfeatures.ErrCapabilityUnavailable
			} else {
				result, err = brokerReader.ReadMarketCandles(
					c.Request.Context(), brokerID, uri.Market, uri.Symbol,
					query.period, query.limit, query.fromTime, query.toTime, query.beforeTime,
				)
			}
		} else {
			defaultToTime := query.toTime
			if query.beforeTime != "" {
				beforeAt, _ := time.Parse(time.RFC3339Nano, query.beforeTime)
				defaultToTime = beforeAt.Add(-time.Nanosecond).Format(time.RFC3339Nano)
			}
			result, err = svc.GetCandles(
				c.Request.Context(), uri.Market, uri.Symbol,
				query.period, query.limit, query.fromTime, defaultToTime,
			)
			if err == nil {
				result["pagination"] = defaultCandlePagination(result, query.limit)
			}
		}
		if err != nil {
			writeBrokerMarketDataReadError(c, "OPEND_CANDLES_FAILED", err)
			return
		}
		httpserver.WriteOK(c, result)
	}
}

type candleRouteQuery struct {
	period     string
	limit      int
	fromTime   string
	toTime     string
	beforeTime string
}

func parseCandleRouteQuery(c *gin.Context) (candleRouteQuery, error) {
	query := candleRouteQuery{period: "1m"}
	if raw := c.Query("period"); raw != "" {
		period, err := httpserver.NormalizeCandlePeriod(raw)
		if err != nil {
			return candleRouteQuery{}, errors.New("invalid candle query")
		}
		query.period = period
	}
	if rawLimit := c.Query("limit"); rawLimit != "" {
		parsed := httpserver.OptionalIntValue{}
		besteffort.LogError(parsed.UnmarshalText([]byte(rawLimit)))
		if parsed.Valid {
			query.limit = parsed.Int()
		}
	}
	query.fromTime = normalizeOptionalQueryTime(c.Query("fromTime"))
	if query.fromTime == "" {
		query.fromTime = normalizeOptionalQueryTime(c.Query("from"))
	}
	query.toTime = normalizeOptionalQueryTime(c.Query("toTime"))
	if query.toTime == "" {
		query.toTime = normalizeOptionalQueryTime(c.Query("to"))
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

func defaultCandlePagination(result map[string]any, limit int) map[string]any {
	candles, _ := result["candles"].([]map[string]any)
	hasMore := limit > 0 && len(candles) >= limit
	pagination := map[string]any{"hasMore": hasMore}
	if hasMore && len(candles) > 0 {
		if at, ok := candles[0]["at"].(string); ok && at != "" {
			pagination["nextBefore"] = at
		}
	}
	return pagination
}

func writeMarketDataReadError(c *gin.Context, fallbackCode string, err error) {
	if errors.Is(err, srv.ErrSubscriptionRequired) {
		httpserver.WriteError(c, http.StatusConflict, "MARKET_DATA_SUBSCRIPTION_REQUIRED", err.Error())
		return
	}
	httpserver.WriteError(c, http.StatusBadGateway, fallbackCode, err.Error())
}

func writeBrokerMarketDataReadError(c *gin.Context, fallbackCode string, err error) {
	switch {
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

func normalizeOptionalQueryTime(value string) string {
	parsed := httpserver.ParseQueryTime(value, time.Time{})
	if parsed.IsZero() {
		return ""
	}
	return parsed.UTC().Format(time.RFC3339Nano)
}

// handleDepth godoc
// @Summary 读取盘口深度
// @Tags market-data
// @Produce json
// @Param market path string true "市场代码"
// @Param symbol path string true "证券代码"
// @Param num query int false "档数，默认 10，最大 50"
// @Param brokerId query string false "行情提供者；省略时使用服务端默认"
// @Success 200 {object} httpserver.Envelope
// @Failure 400 {object} httpserver.Envelope
// @Failure 409 {object} httpserver.Envelope
// @Failure 502 {object} httpserver.Envelope
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
			jftradeErr1 := parsed.UnmarshalText([]byte(n))
			besteffort.LogError(jftradeErr1)
			if parsed.Valid {
				num = parsed.Int()
			}
		}
		var result map[string]any
		var err error
		if brokerID := strings.TrimSpace(c.Query("brokerId")); brokerID != "" {
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
			writeBrokerMarketDataReadError(c, "OPEND_DEPTH_FAILED", err)
			return
		}
		httpserver.WriteOK(c, result)
	}
}

// handleGetSubscriptions 查询当前订阅。
func handleGetSubscriptions(svc *srv.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		result, err := svc.GetSubscriptions(c.Request.Context())
		if err != nil {
			httpserver.WriteError(c, 500, "SUBSCRIPTION_FAILED", err.Error())
			return
		}
		httpserver.WriteOK(c, result)
	}
}

// handleAcquireSubscription godoc
// @Summary 申请行情订阅
// @Tags market-data
// @Accept json
// @Produce json
// @Param request body SubscriptionRequest true "订阅请求"
// @Success 200 {object} httpserver.Envelope
// @Failure 400 {object} httpserver.Envelope
// @Router /api/v1/market-data/subscriptions [post]
//
// 请求格式：
//
//	{"consumerId":"...", "instruments":[{"market":"...", "symbol":"...", "channel":"...", "interval":"..."}]}
func handleAcquireSubscription(svc *srv.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req subscriptionRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			httpserver.WriteError(c, 400, "BAD_REQUEST", "invalid subscription request")
			return
		}
		consumerID := req.ConsumerID
		instruments := subscriptionInstruments(req)
		if consumerID == "" || len(instruments) == 0 {
			httpserver.WriteError(c, 400, "BAD_REQUEST", "consumerId and instruments are required")
			return
		}
		if err := srv.ValidateSubscriptionRefs(instruments); err != nil {
			httpserver.WriteError(c, 400, "BAD_REQUEST", err.Error())
			return
		}
		if usesBrokerPolling(req.ProviderBrokerID) {
			httpserver.WriteOK(c, brokerPollingSubscriptionResponse(
				req.ConsumerID, req.ProviderBrokerID, instruments, "acquired",
			))
			return
		}
		result, err := svc.AcquireSubscription(c.Request.Context(), consumerID, instruments)
		if err != nil {
			httpserver.WriteError(c, 500, "SUBSCRIPTION_FAILED", err.Error())
			return
		}
		httpserver.WriteOK(c, result)
	}
}

// handleReleaseSubscription godoc
// @Summary 释放行情订阅
// @Tags market-data
// @Accept json
// @Produce json
// @Param request body SubscriptionRequest true "释放请求"
// @Success 200 {object} httpserver.Envelope
// @Failure 400 {object} httpserver.Envelope
// @Router /api/v1/market-data/subscriptions/release [post]
//
// 请求格式：
//
//	释放指定目标：{"consumerId":"...", "instruments":[{"market":"...", "symbol":"...", "channel":"...", "interval":"..."}]}
//	释放消费者全部订阅：{"consumerId":"..."}
func handleReleaseSubscription(svc *srv.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req subscriptionRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			httpserver.WriteError(c, 400, "BAD_REQUEST", "invalid release request")
			return
		}
		consumerID := req.ConsumerID
		if consumerID == "" {
			httpserver.WriteError(c, 400, "BAD_REQUEST", "consumerId is required")
			return
		}
		target, hasTarget, validTarget := subscriptionReleaseTarget(req)
		if !validTarget {
			httpserver.WriteError(c, 400, "BAD_REQUEST", "release target market and symbol are required")
			return
		}
		if hasTarget {
			if err := srv.ValidateSubscriptionRefs([]srv.InstrumentRef{target}); err != nil {
				httpserver.WriteError(c, 400, "BAD_REQUEST", err.Error())
				return
			}
		}
		if usesBrokerPolling(req.ProviderBrokerID) {
			httpserver.WriteOK(c, brokerPollingSubscriptionResponse(
				req.ConsumerID, req.ProviderBrokerID, req.Instruments, "released",
			))
			return
		}
		var err error
		if hasTarget {
			err = svc.ReleaseSubscription(c.Request.Context(), consumerID, target)
		} else {
			err = svc.ReleaseSubscription(c.Request.Context(), consumerID)
		}
		if err != nil {
			httpserver.WriteError(c, 500, "SUBSCRIPTION_FAILED", err.Error())
			return
		}
		result, err := svc.GetSubscriptions(c.Request.Context())
		if err != nil {
			httpserver.WriteError(c, 500, "SUBSCRIPTION_FAILED", err.Error())
			return
		}
		result["released"] = true
		httpserver.WriteOK(c, result)
	}
}

// handleClearSubscriptions godoc
// @Summary 清空行情订阅
// @Tags market-data
// @Produce json
// @Param consumerId query string false "消费者 ID；为空时清空全部"
// @Success 200 {object} httpserver.Envelope
// @Failure 400 {object} httpserver.Envelope
// @Router /api/v1/market-data/subscriptions [delete]
func handleClearSubscriptions(svc *srv.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		if err := svc.ClearSubscriptions(c.Request.Context(), c.Query("consumerId")); err != nil {
			httpserver.WriteError(c, 500, "SUBSCRIPTION_FAILED", err.Error())
			return
		}
		result, err := svc.GetSubscriptions(c.Request.Context())
		if err != nil {
			httpserver.WriteError(c, 500, "SUBSCRIPTION_FAILED", err.Error())
			return
		}
		result["cleared"] = true
		httpserver.WriteOK(c, result)
	}
}

func subscriptionInstruments(req subscriptionRequest) []srv.InstrumentRef {
	instruments := make([]srv.InstrumentRef, 0, len(req.Instruments))
	for _, instrument := range req.Instruments {
		if strings.TrimSpace(instrument.Market) == "" || strings.TrimSpace(instrument.Symbol) == "" {
			continue
		}
		instruments = append(instruments, instrument)
	}
	if len(instruments) > 0 {
		return instruments
	}
	return nil
}

func subscriptionReleaseTarget(req subscriptionRequest) (srv.InstrumentRef, bool, bool) {
	if len(req.Instruments) == 0 {
		return srv.InstrumentRef{}, false, true
	}
	target := req.Instruments[0]
	if strings.TrimSpace(target.Market) == "" || strings.TrimSpace(target.Symbol) == "" {
		return srv.InstrumentRef{}, false, false
	}
	return target, true, true
}

// handleHeartbeat godoc
// @Summary 刷新订阅心跳
// @Tags market-data
// @Accept json
// @Produce json
// @Param request body SubscriptionHeartbeatRequest true "心跳请求"
// @Success 200 {object} httpserver.Envelope
// @Failure 400 {object} httpserver.Envelope
// @Router /api/v1/market-data/subscriptions/heartbeat [post]
func handleHeartbeat(svc *srv.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req struct {
			ConsumerID       string `json:"consumerId"`
			ProviderBrokerID string `json:"providerBrokerId,omitempty"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			httpserver.WriteError(c, 400, "BAD_REQUEST", "invalid heartbeat request")
			return
		}
		if strings.TrimSpace(req.ConsumerID) == "" {
			httpserver.WriteError(c, 400, "BAD_REQUEST", "consumerId is required")
			return
		}
		if usesBrokerPolling(req.ProviderBrokerID) {
			httpserver.WriteOK(c, brokerPollingSubscriptionResponse(
				req.ConsumerID, req.ProviderBrokerID, nil, "heartbeat",
			))
			return
		}
		result, err := svc.Heartbeat(c.Request.Context(), req.ConsumerID)
		if err != nil {
			httpserver.WriteError(c, 500, "SUBSCRIPTION_FAILED", err.Error())
			return
		}
		httpserver.WriteOK(c, result)
	}
}

func usesBrokerPolling(brokerID string) bool {
	brokerID = strings.TrimSpace(brokerID)
	return brokerID != "" && !strings.EqualFold(brokerID, "futu")
}

func brokerPollingSubscriptionResponse(
	consumerID string,
	brokerID string,
	instruments []srv.InstrumentRef,
	action string,
) map[string]any {
	return map[string]any{
		"consumerId":               consumerID,
		"providerBrokerId":         strings.ToLower(strings.TrimSpace(brokerID)),
		"instruments":              instruments,
		"action":                   action,
		"totalActiveSubscriptions": 0,
		"desiredCount":             0,
		"ownActiveCount":           0,
		"pendingReleaseCount":      0,
		"entries":                  []any{},
		"quota": map[string]any{
			"totalUsed":      0,
			"totalLimit":     nil,
			"totalRemaining": nil,
			"byMarket":       []any{},
		},
		"transport": map[string]any{
			"mode": "snapshot-poll-fallback",
		},
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
// @Failure 400 {object} httpserver.Envelope
// @Failure 502 {object} httpserver.Envelope
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
			httpserver.WriteError(c, 502, "MARKET_INSTRUMENT_SEARCH_FAILED", err.Error())
			return
		}
		httpserver.WriteOK(c, result)
	}
}

// handleNormalizeInstrument 规范化为标的。
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
