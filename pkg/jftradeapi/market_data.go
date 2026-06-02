package jftradeapi

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	bbgotypes "github.com/c9s/bbgo/pkg/types"

	"github.com/jftrade/jftrade-main/pkg/broker"
	"github.com/jftrade/jftrade-main/pkg/futu"
	"github.com/jftrade/jftrade-main/pkg/futu/opend"
)

const marketSecurityDetailsStreamInterval = 1 * time.Second
const marketDepthStreamRefreshInterval = 15 * time.Second

func (s *Server) handleMarketSnapshot(w http.ResponseWriter, r *http.Request) {
	response, err := s.marketSnapshotResponse(r.Context(), r.URL.Path, r.URL.Query())
	if err != nil {
		s.writeError(w, http.StatusBadGateway, "MARKET_SNAPSHOT_FAILED", err.Error())
		return
	}
	s.writeOK(w, response)
}

func (s *Server) handleMarketSecurityDetails(w http.ResponseWriter, r *http.Request) {
	if acceptsEventStream(r) {
		s.handleMarketSecurityDetailsEventStream(w, r)
		return
	}
	response, err := s.marketSecurityDetailsResponse(r.Context(), r.URL.Path)
	if err != nil {
		s.writeError(w, http.StatusBadGateway, "MARKET_SECURITY_DETAILS_FAILED", err.Error())
		return
	}
	s.writeOK(w, response)
}

func (s *Server) handleMarketSecurityDetailsEventStream(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		s.writeError(w, http.StatusInternalServerError, "MARKET_SECURITY_DETAILS_STREAM_UNSUPPORTED", "response writer does not support streaming")
		return
	}

	response, err := s.marketSecurityDetailsResponse(r.Context(), r.URL.Path)
	if err != nil {
		s.writeError(w, http.StatusBadGateway, "MARKET_SECURITY_DETAILS_FAILED", err.Error())
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	writer := liveSSEWriter{writer: w, flusher: flusher}
	if err := writer.WriteEvent(response); err != nil {
		return
	}

	ticker := time.NewTicker(marketSecurityDetailsStreamInterval)
	defer ticker.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
			response, err := s.marketSecurityDetailsResponse(r.Context(), r.URL.Path)
			if err != nil {
				continue
			}
			if err := writer.WriteEvent(response); err != nil {
				return
			}
		}
	}
}

func (s *Server) marketSecurityDetailsResponse(ctx context.Context, path string) (map[string]any, error) {
	market, symbol := pathTail(path, "/api/v1/market-data/securities/")
	market = strings.ToUpper(strings.TrimSpace(market))
	symbol = strings.ToUpper(strings.TrimSpace(symbol))
	instrumentID := market + "." + symbol
	details, err := s.futuExchange().QuerySecurityDetails(ctx, instrumentID)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"request":  map[string]any{"market": market, "symbol": symbol, "instrumentId": instrumentID},
		"security": securityDetailsMap(details),
		"meta":     map[string]any{"instrumentId": instrumentID, "source": "bbgo:futu", "resolvedAt": time.Now().UTC().Format(time.RFC3339Nano), "fromCache": false},
	}, nil
}

func (s *Server) marketSnapshotResponse(ctx context.Context, path string, query map[string][]string) (map[string]any, error) {
	market, symbol := pathTail(path, "/api/v1/market-data/snapshots/")
	market = strings.ToUpper(strings.TrimSpace(market))
	symbol = strings.ToUpper(strings.TrimSpace(symbol))
	instrumentID := market + "." + symbol
	forceRefresh := boolQuery(query, "refresh", false)
	sample, fromCache, err := s.resolveMarketSnapshotSample(ctx, instrumentID, forceRefresh)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"request":  map[string]any{"market": market, "symbol": symbol, "instrumentId": instrumentID},
		"snapshot": snapshotMapFromSample(sample),
		"meta":     map[string]any{"instrumentId": instrumentID, "source": sample.Source, "resolvedAt": sample.ObservedAt, "fromCache": fromCache},
	}, nil
}

func (s *Server) resolveMarketSnapshotSample(ctx context.Context, instrumentID string, forceRefresh bool) (*marketTickSample, bool, error) {
	sample := (*marketTickSample)(nil)
	if !forceRefresh {
		sample = s.latestTickerSample(instrumentID, liveTickSampleFreshness)
	}
	fromCache := sample != nil
	if sample == nil {
		snapshot, err := s.futuExchange().QueryQuoteSnapshot(ctx, instrumentID)
		if err != nil {
			return nil, false, err
		}
		sample = s.recordQuoteSnapshotSample(instrumentID, snapshot)
	}
	if sample == nil {
		return nil, false, fmt.Errorf("no snapshot available for %s", instrumentID)
	}
	return sample, fromCache, nil
}

func (s *Server) handleMarketCandles(w http.ResponseWriter, r *http.Request) {
	response, err := s.marketCandlesResponse(r.Context(), r.URL.Path, r.URL.Query())
	if err != nil {
		s.writeError(w, http.StatusBadGateway, "OPEND_CANDLES_FAILED", err.Error())
		return
	}
	s.writeOK(w, response)
}

func (s *Server) marketCandlesResponse(ctx context.Context, path string, query map[string][]string) (map[string]any, error) {
	market, symbol := pathTail(path, "/api/v1/market-data/candles/")
	market = strings.ToUpper(strings.TrimSpace(market))
	symbol = strings.ToUpper(strings.TrimSpace(symbol))
	instrumentID := market + "." + symbol
	period, err := normalizeCandlePeriod(firstQuery(query, "period", "1m"))
	if err != nil {
		return nil, err
	}
	limit := intQuery(query, "limit", 200)
	if limit < 1 {
		limit = 1
	}
	if limit > 1000 {
		limit = 1000
	}
	if period == "tick" {
		return s.buildTickCandlesResponse(ctx, market, symbol, instrumentID, period, limit, query)
	}

	return s.buildKLineCandlesResponse(ctx, market, symbol, instrumentID, period, limit, query)
}

func (s *Server) buildTickCandlesResponse(ctx context.Context, market string, symbol string, instrumentID string, period string, limit int, query map[string][]string) (map[string]any, error) {
	includeSession := market == "US"
	extendedHours := includeSession
	request := marketCandlesRequest(market, symbol, instrumentID, period, limit)
	fromLiveCache := s.latestTickerSample(instrumentID, liveTickSampleFreshness) != nil
	if !fromLiveCache {
		ticker, err := s.futuExchange().QueryTicker(ctx, instrumentID)
		if err != nil {
			cachedCandles := s.cachedTickCandles(instrumentID, query, limit)
			if len(cachedCandles) == 0 {
				return nil, err
			}
			return map[string]any{
				"request":       request,
				"candles":       cachedCandles,
				"totalReturned": len(cachedCandles),
				"meta":          candleMeta(instrumentID, true, extendedHours, includeSession),
			}, nil
		}
		s.recordTickerSample(instrumentID, ticker)
	}

	candles := s.cachedTickCandles(instrumentID, query, limit)
	return map[string]any{
		"request":       request,
		"candles":       candles,
		"totalReturned": len(candles),
		"meta":          candleMeta(instrumentID, fromLiveCache, extendedHours, includeSession),
	}, nil
}

func (s *Server) buildKLineCandlesResponse(ctx context.Context, market string, symbol string, instrumentID string, period string, limit int, query map[string][]string) (map[string]any, error) {
	interval := bbgotypes.Interval(period)
	includeSession := shouldAnnotateHistoricalKLineSession(market, interval)
	beginAt, endAt := kLineQueryWindow(query, interval.Duration(), limit)
	exchange := s.futuExchange()
	klines, err := exchange.QueryKLines(ctx, instrumentID, interval, bbgotypes.KLineQueryOptions{Limit: limit, StartTime: &beginAt, EndTime: &endAt})
	if err != nil {
		return nil, err
	}
	candles := make([]map[string]any, 0, len(klines))
	for _, kline := range klines {
		candle := map[string]any{
			"period": period,
			"open":   kline.Open.String(),
			"high":   kline.High.String(),
			"low":    kline.Low.String(),
			"close":  kline.Close.String(),
			"volume": kline.Volume.Float64(),
			"at":     kline.StartTime.Time().UTC().Format(time.RFC3339Nano),
		}
		if includeSession {
			session, ok := exchange.ResolveKLineSession(kline)
			if !ok {
				session = futu.ClassifyMarketSession(instrumentID, kline.StartTime.Time().UTC())
			}
			if session != futu.MarketSessionUnknown && session != futu.MarketSessionClosed {
				candle["session"] = string(session)
			}
		}
		candles = append(candles, candle)
	}
	extendedHours := includeSession

	return map[string]any{
		"request":       marketCandlesRequest(market, symbol, instrumentID, period, limit),
		"candles":       candles,
		"totalReturned": len(candles),
		"meta":          candleMeta(instrumentID, false, extendedHours, includeSession),
	}, nil
}

func marketCandlesRequest(market string, symbol string, instrumentID string, period string, limit int) map[string]any {
	return map[string]any{
		"instrument": map[string]any{
			"market":       market,
			"symbol":       symbol,
			"instrumentId": instrumentID,
		},
		"period": period,
		"limit":  limit,
	}
}

func candleMeta(instrumentID string, fromCache bool, extendedHours bool, includeSession bool) map[string]any {
	meta := map[string]any{
		"instrumentId":  instrumentID,
		"source":        "bbgo:futu",
		"resolvedAt":    time.Now().UTC().Format(time.RFC3339Nano),
		"fromCache":     fromCache,
		"extendedHours": extendedHours,
	}
	if includeSession {
		session := "regular"
		if extendedHours {
			session = "all"
		}
		meta["session"] = session
	}
	return meta
}

func shouldAnnotateHistoricalKLineSession(market string, interval bbgotypes.Interval) bool {
	return strings.EqualFold(strings.TrimSpace(market), "US") && interval.Duration() > 0 && interval.Duration() <= time.Hour
}

func (s *Server) futuExchange() *futu.Exchange {
	integration := s.store.integration()
	config := opend.Config{
		Addr:             net.JoinHostPort(integration.Config.Host, strconv.Itoa(integration.Config.APIPort)),
		WebSocketKey:     integration.Config.WebSocketKey,
		HandshakeTimeout: 3 * time.Second,
		RequestTimeout:   8 * time.Second,
	}
	configKey := strings.Join([]string{config.Addr, config.WebSocketKey}, "|")

	s.exchangeMu.Lock()
	defer s.exchangeMu.Unlock()
	if s.exchange != nil && s.exchangeConfigKey == configKey {
		return s.exchange
	}
	s.exchange = futu.NewExchangeWithConfig(config)
	s.exchange.OnSystemNotify(s.handleFutuSystemNotify)
	s.exchangeConfigKey = configKey

	// Register the Futu broker adapter whenever a new exchange is created.
	if s.brokers != nil && s.brokers.Lookup(string(futu.Name)) == nil {
		s.brokers.Register(futu.NewBrokerAdapter(s.exchange))
	}

	return s.exchange
}

// --- Depth (Order Book) ---

func (s *Server) handleMarketDepth(w http.ResponseWriter, r *http.Request) {
	if acceptsEventStream(r) {
		s.handleMarketDepthEventStream(w, r)
		return
	}
	response, err := s.marketDepthResponse(r.Context(), r.URL.Path, r.URL.Query())
	if err != nil {
		s.writeError(w, http.StatusBadGateway, "OPEND_DEPTH_FAILED", err.Error())
		return
	}
	s.writeOK(w, response)
}

func (s *Server) handleMarketDepthEventStream(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		s.writeError(w, http.StatusInternalServerError, "MARKET_DEPTH_STREAM_UNSUPPORTED", "response writer does not support streaming")
		return
	}

	response, err := s.marketDepthResponse(r.Context(), r.URL.Path, r.URL.Query())
	if err != nil {
		s.writeError(w, http.StatusBadGateway, "OPEND_DEPTH_FAILED", err.Error())
		return
	}

	market, symbol := pathTail(r.URL.Path, "/api/v1/market-data/depth/")
	instrumentID := strings.ToUpper(strings.TrimSpace(market)) + "." + strings.ToUpper(strings.TrimSpace(symbol))
	num := int32(intQuery(r.URL.Query(), "num", 10))
	if num < 1 {
		num = 1
	}
	if num > 50 {
		num = 50
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	writer := liveSSEWriter{writer: w, flusher: flusher}
	if err := writer.WriteEvent(response); err != nil {
		return
	}

	if subscriber, ok := s.futuBroker().(broker.OrderBookSubscriber); ok {
		_ = subscriber.SubscribeOrderBook(r.Context(), broker.OrderBookSubscribeRequest{
			ReadQuery: brokerReadQuery(instrumentID),
			Symbols:   []string{instrumentID},
			Num:       num,
		})
	}

	updateCh := make(chan struct{}, 1)
	unsubscribe := s.futuExchange().OnOrderBookUpdate(func(updatedSymbol string) {
		if !strings.EqualFold(updatedSymbol, instrumentID) {
			return
		}
		select {
		case updateCh <- struct{}{}:
		default:
		}
	})
	defer unsubscribe()

	refreshTicker := time.NewTicker(marketDepthStreamRefreshInterval)
	defer refreshTicker.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-updateCh:
		case <-refreshTicker.C:
		}

		response, err := s.marketDepthResponse(r.Context(), r.URL.Path, r.URL.Query())
		if err != nil {
			continue
		}
		if err := writer.WriteEvent(response); err != nil {
			return
		}
	}
}

func (s *Server) marketDepthResponse(ctx context.Context, path string, query map[string][]string) (map[string]any, error) {
	market, symbol := pathTail(path, "/api/v1/market-data/depth/")
	market = strings.ToUpper(strings.TrimSpace(market))
	symbol = strings.ToUpper(strings.TrimSpace(symbol))
	instrumentID := market + "." + symbol
	num := int32(intQuery(query, "num", 10))
	if num < 1 {
		num = 1
	}
	if num > 50 {
		num = 50
	}

	brokerResult, err := s.futuBroker().MarketData().QueryOrderBook(ctx, broker.OrderBookQuery{
		ReadQuery: brokerReadQuery(instrumentID),
		Symbol:    instrumentID,
		Num:       num,
	})
	if err != nil {
		return nil, err
	}

	return map[string]any{
		"request": map[string]any{"market": market, "symbol": symbol, "instrumentId": instrumentID, "num": num},
		"depth":   brokerResult,
		"meta": map[string]any{
			"instrumentId": instrumentID,
			"source":       "bbgo:futu",
			"resolvedAt":   time.Now().UTC().Format(time.RFC3339Nano),
			"fromCache":    false,
		},
	}, nil
}

func (s *Server) futuBroker() broker.Broker {
	if s.brokers != nil {
		if b := s.brokers.Lookup(string(futu.Name)); b != nil {
			return b
		}
	}
	return futu.NewBrokerAdapter(s.futuExchange())
}

func brokerReadQuery(instrumentID string) broker.ReadQuery {
	parts := strings.SplitN(instrumentID, ".", 2)
	market := ""
	if len(parts) == 2 {
		market = parts[0]
	}
	return broker.ReadQuery{
		Market: market,
	}
}
