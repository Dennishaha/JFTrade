package jftradeapi

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	bbgofixedpoint "github.com/c9s/bbgo/pkg/fixedpoint"
	bbgotypes "github.com/c9s/bbgo/pkg/types"
	"github.com/gorilla/websocket"

	"github.com/jftrade/jftrade-main/pkg/futu"
	"github.com/jftrade/jftrade-main/pkg/futu/opend"
)

const (
	liveTickDispatchInterval     = 250 * time.Millisecond
	liveTickFallbackPollInterval = 1 * time.Second
	liveTickFallbackPollTimeout  = 900 * time.Millisecond
	liveTickSampleFreshness      = 1500 * time.Millisecond
)

func (s *Server) handleLiveWebSocket(w http.ResponseWriter, r *http.Request) {
	limit := s.effectiveLiveWebSocketLimit()
	if !s.tryAcquireLiveWebSocketSlot(limit) {
		s.writeError(w, http.StatusServiceUnavailable, "LIVE_WS_LIMIT_REACHED", fmt.Sprintf("live websocket connection limit reached (%d)", limit))
		return
	}

	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		s.releaseLiveWebSocketSlot()
		return
	}
	defer func() {
		s.releaseLiveWebSocketSlot()
		_ = conn.Close()
	}()

	if err := writeHeartbeat(conn); err != nil {
		return
	}
	clientClosed := liveWebSocketClientClosed(conn)

	lastSentByInstrument := map[string]string{}
	if err := s.writeLiveMarketTicks(r.Context(), conn, lastSentByInstrument); err != nil {
		return
	}

	heartbeatTicker := time.NewTicker(15 * time.Second)
	defer heartbeatTicker.Stop()
	dataTicker := time.NewTicker(liveTickDispatchInterval)
	defer dataTicker.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-clientClosed:
			return
		case <-heartbeatTicker.C:
			if err := writeHeartbeat(conn); err != nil {
				return
			}
		case <-dataTicker.C:
			if err := s.writeLiveMarketTicks(r.Context(), conn, lastSentByInstrument); err != nil {
				return
			}
		}
	}
}

func liveWebSocketClientClosed(conn *websocket.Conn) <-chan struct{} {
	closed := make(chan struct{})
	go func() {
		defer close(closed)
		for {
			if _, _, err := conn.NextReader(); err != nil {
				return
			}
		}
	}()
	return closed
}

func writeHeartbeat(conn *websocket.Conn) error {
	return conn.WriteJSON(map[string]any{"type": "heartbeat", "at": time.Now().UTC().Format(time.RFC3339Nano)})
}

func (s *Server) effectiveLiveWebSocketLimit() int {
	limit := s.store.integration().Config.MaxWebSocketConnections
	if limit <= 0 {
		return defaultMaxWebSocketClients
	}
	return limit
}

func (s *Server) tryAcquireLiveWebSocketSlot(limit int) bool {
	s.liveMu.Lock()
	defer s.liveMu.Unlock()
	if s.liveWebSocketClients >= limit {
		return false
	}
	s.liveWebSocketClients++
	return true
}

func (s *Server) releaseLiveWebSocketSlot() {
	s.liveMu.Lock()
	defer s.liveMu.Unlock()
	if s.liveWebSocketClients > 0 {
		s.liveWebSocketClients--
	}
}

func (s *Server) liveWebSocketStats() (count int, limit int, atLimit bool) {
	s.liveMu.Lock()
	count = s.liveWebSocketClients
	s.liveMu.Unlock()
	limit = s.effectiveLiveWebSocketLimit()
	return count, limit, count >= limit
}

func (s *Server) handleMarketSnapshot(w http.ResponseWriter, r *http.Request) {
	response, err := s.marketSnapshotResponse(r.Context(), r.URL.Path)
	if err != nil {
		s.writeError(w, http.StatusBadGateway, "MARKET_SNAPSHOT_FAILED", err.Error())
		return
	}
	s.writeOK(w, response)
}

func (s *Server) marketSnapshotResponse(ctx context.Context, path string) (map[string]any, error) {
	market, symbol := pathTail(path, "/api/v1/market-data/snapshots/")
	market = strings.ToUpper(strings.TrimSpace(market))
	symbol = strings.ToUpper(strings.TrimSpace(symbol))
	instrumentID := market + "." + symbol
	sample := s.latestTickerSample(instrumentID, liveTickSampleFreshness)
	fromCache := sample != nil
	if sample == nil {
		ticker, err := s.futuExchange().QueryTicker(ctx, instrumentID)
		if err != nil {
			return nil, err
		}
		sample = s.recordTickerSample(instrumentID, ticker)
	}
	if sample == nil {
		return nil, fmt.Errorf("no snapshot available for %s", instrumentID)
	}
	return map[string]any{
		"request": map[string]any{"market": market, "symbol": symbol, "instrumentId": instrumentID},
		"snapshot": map[string]any{
			"price":              sample.Price,
			"bid":                sample.Bid,
			"ask":                sample.Ask,
			"openPrice":          sample.OpenPrice,
			"highPrice":          sample.HighPrice,
			"lowPrice":           sample.LowPrice,
			"previousClosePrice": nil,
			"volume":             sample.Volume,
			"turnover":           sample.Turnover,
			"at":                 sample.QuoteAt,
			"observedAt":         sample.ObservedAt,
		},
		"meta": map[string]any{"instrumentId": instrumentID, "source": sample.Source, "resolvedAt": sample.ObservedAt, "fromCache": fromCache},
	}, nil
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
		fromLiveCache := s.latestTickerSample(instrumentID, liveTickSampleFreshness) != nil
		if !fromLiveCache {
			ticker, err := s.futuExchange().QueryTicker(ctx, instrumentID)
			if err != nil {
				cachedCandles := s.cachedTickCandles(instrumentID, query, limit)
				if len(cachedCandles) == 0 {
					return nil, err
				}
				return map[string]any{
					"request":       map[string]any{"instrument": map[string]any{"market": market, "symbol": symbol, "instrumentId": instrumentID}, "period": period, "limit": limit},
					"candles":       cachedCandles,
					"totalReturned": len(cachedCandles),
					"meta":          map[string]any{"instrumentId": instrumentID, "source": "bbgo:futu", "resolvedAt": time.Now().UTC().Format(time.RFC3339Nano), "fromCache": true},
				}, nil
			}
			s.recordTickerSample(instrumentID, ticker)
		}
		candles := s.cachedTickCandles(instrumentID, query, limit)
		return map[string]any{
			"request":       map[string]any{"instrument": map[string]any{"market": market, "symbol": symbol, "instrumentId": instrumentID}, "period": period, "limit": limit},
			"candles":       candles,
			"totalReturned": len(candles),
			"meta":          map[string]any{"instrumentId": instrumentID, "source": "bbgo:futu", "resolvedAt": time.Now().UTC().Format(time.RFC3339Nano), "fromCache": fromLiveCache},
		}, nil
	}

	interval := bbgotypes.Interval(period)
	beginAt, endAt := kLineQueryWindow(query, interval.Duration(), limit)
	klines, err := s.futuExchange().QueryKLines(ctx, instrumentID, interval, bbgotypes.KLineQueryOptions{Limit: limit, StartTime: &beginAt, EndTime: &endAt})
	if err != nil {
		return nil, err
	}
	candles := make([]map[string]any, 0, len(klines))
	for _, kline := range klines {
		candles = append(candles, map[string]any{
			"period": period,
			"open":   kline.Open.Float64(),
			"high":   kline.High.Float64(),
			"low":    kline.Low.Float64(),
			"close":  kline.Close.Float64(),
			"volume": kline.Volume.Float64(),
			"at":     kline.StartTime.Time().UTC().Format(time.RFC3339Nano),
		})
	}

	return map[string]any{
		"request":       map[string]any{"instrument": map[string]any{"market": market, "symbol": symbol, "instrumentId": instrumentID}, "period": period, "limit": limit},
		"candles":       candles,
		"totalReturned": len(candles),
		"meta":          map[string]any{"instrumentId": instrumentID, "source": "bbgo:futu", "resolvedAt": time.Now().UTC().Format(time.RFC3339Nano), "fromCache": false},
	}, nil
}

func kLineQueryWindow(query map[string][]string, periodDuration time.Duration, limit int) (time.Time, time.Time) {
	endAt := parseQueryTime(firstQuery(query, "toTime", ""), time.Now())
	if queryEnd := firstQuery(query, "to", ""); queryEnd != "" {
		endAt = parseQueryTime(queryEnd, endAt)
	}
	lookback := periodDuration * time.Duration(limit) * 4
	minimumLookback := 36 * time.Hour
	if periodDuration >= 24*time.Hour {
		minimumLookback = 45 * 24 * time.Hour
	}
	if lookback < minimumLookback {
		lookback = minimumLookback
	}
	defaultBegin := endAt.Add(-lookback)
	beginAt := parseQueryTime(firstQuery(query, "fromTime", ""), defaultBegin)
	if queryBegin := firstQuery(query, "from", ""); queryBegin != "" {
		beginAt = parseQueryTime(queryBegin, beginAt)
	}
	if !beginAt.Before(endAt) {
		beginAt = defaultBegin
	}
	return beginAt, endAt
}

func parseQueryTime(value string, fallback time.Time) time.Time {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02 15:04:05", "2006-01-02"} {
		parsed, err := time.ParseInLocation(layout, value, time.Local)
		if err == nil {
			return parsed
		}
	}
	return fallback
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
	s.exchangeConfigKey = configKey
	return s.exchange
}

func (s *Server) refreshLiveMarketTicksIfNeeded(ctx context.Context) {
	instrumentIDs := s.activeMarketInstrumentIDs()
	if len(instrumentIDs) == 0 {
		return
	}
	s.ensureLiveMarketStream(ctx, instrumentIDs)
	if s.allHaveFreshTickerSamples(instrumentIDs, liveTickFallbackPollInterval) {
		return
	}

	s.liveRefreshMu.Lock()
	defer s.liveRefreshMu.Unlock()

	now := time.Now().UTC()
	if !s.liveLastQuoteRefreshAt.IsZero() && now.Sub(s.liveLastQuoteRefreshAt) < liveTickFallbackPollInterval {
		return
	}
	if s.allHaveFreshTickerSamples(instrumentIDs, liveTickFallbackPollInterval) {
		return
	}
	s.liveLastQuoteRefreshAt = now

	refreshCtx, cancel := context.WithTimeout(ctx, liveTickFallbackPollTimeout)
	defer cancel()

	tickers, err := s.futuExchange().QueryTickers(refreshCtx, instrumentIDs...)
	if err != nil {
		return
	}
	for _, instrumentID := range instrumentIDs {
		ticker, ok := tickers[instrumentID]
		if !ok {
			continue
		}
		s.recordTickerSample(instrumentID, &ticker)
	}
}

func (s *Server) ensureLiveMarketStream(ctx context.Context, instrumentIDs []string) {
	streamKey, symbols := liveMarketStreamKey(s.store.integration().Config, instrumentIDs)
	if len(symbols) == 0 {
		return
	}

	s.liveStreamMu.Lock()
	if s.liveStream != nil && s.liveStreamKey == streamKey {
		s.liveStreamMu.Unlock()
		return
	}
	if s.liveStream != nil {
		_ = s.liveStream.Close()
	}
	stream := s.futuExchange().NewStream()
	stream.SetPublicOnly()
	for _, symbol := range symbols {
		stream.Subscribe(bbgotypes.MarketTradeChannel, symbol, bbgotypes.SubscribeOptions{})
	}
	stream.OnMarketTrade(func(trade bbgotypes.Trade) {
		s.recordTradeTickSample(trade)
	})
	s.liveStream = stream
	s.liveStreamKey = streamKey
	s.liveStreamMu.Unlock()

	streamCtx, cancel := context.WithTimeout(ctx, liveTickFallbackPollTimeout)
	defer cancel()
	if err := stream.Connect(streamCtx); err != nil {
		s.liveStreamMu.Lock()
		if s.liveStream == stream {
			s.liveStream = nil
			s.liveStreamKey = ""
		}
		s.liveStreamMu.Unlock()
	}
}

func liveMarketStreamKey(config FutuIntegrationConfig, instrumentIDs []string) (string, []string) {
	seen := map[string]struct{}{}
	symbols := make([]string, 0, len(instrumentIDs))
	for _, instrumentID := range instrumentIDs {
		symbol := strings.ToUpper(strings.TrimSpace(instrumentID))
		if symbol == "" {
			continue
		}
		if _, exists := seen[symbol]; exists {
			continue
		}
		seen[symbol] = struct{}{}
		symbols = append(symbols, symbol)
	}
	sort.Strings(symbols)
	return strings.Join([]string{
		config.Host,
		strconv.Itoa(config.APIPort),
		config.WebSocketKey,
		strings.Join(symbols, ","),
	}, "|"), symbols
}

func (s *Server) writeLiveMarketTicks(ctx context.Context, conn *websocket.Conn, lastSentByInstrument map[string]string) error {
	s.refreshLiveMarketTicksIfNeeded(ctx)
	for _, sample := range s.latestTickerSamples(s.activeMarketInstrumentIDs(), liveTickSampleFreshness) {
		if sample == nil || lastSentByInstrument[sample.InstrumentID] == sample.ObservedAt {
			continue
		}
		event := liveTickEventFromSample(sample)
		if event == nil {
			continue
		}
		if err := conn.WriteJSON(event); err != nil {
			return err
		}
		lastSentByInstrument[sample.InstrumentID] = sample.ObservedAt
	}
	return nil
}

func (s *Server) latestTickerSample(instrumentID string, maxAge time.Duration) *marketTickSample {
	if maxAge <= 0 {
		return nil
	}

	s.tickCacheMu.Lock()
	defer s.tickCacheMu.Unlock()
	samples := s.tickCache[instrumentID]
	if len(samples) == 0 {
		return nil
	}
	latest := samples[len(samples)-1]
	observedAt := parseQueryTime(latest.ObservedAt, time.Time{})
	if observedAt.IsZero() || time.Since(observedAt.UTC()) > maxAge {
		return nil
	}
	copyOfLatest := latest
	return &copyOfLatest
}

func (s *Server) latestTickerSamples(instrumentIDs []string, maxAge time.Duration) []*marketTickSample {
	if maxAge <= 0 || len(instrumentIDs) == 0 {
		return nil
	}

	cutoff := time.Now().UTC().Add(-maxAge)
	s.tickCacheMu.Lock()
	defer s.tickCacheMu.Unlock()

	results := make([]*marketTickSample, 0, len(instrumentIDs))
	for _, instrumentID := range instrumentIDs {
		samples := s.tickCache[instrumentID]
		if len(samples) == 0 {
			continue
		}
		latest := samples[len(samples)-1]
		observedAt := parseQueryTime(latest.ObservedAt, time.Time{})
		if observedAt.IsZero() || observedAt.Before(cutoff) {
			continue
		}
		copyOfLatest := latest
		results = append(results, &copyOfLatest)
	}
	return results
}

func (s *Server) allHaveFreshTickerSamples(instrumentIDs []string, maxAge time.Duration) bool {
	if len(instrumentIDs) == 0 {
		return true
	}
	for _, instrumentID := range instrumentIDs {
		if s.latestTickerSample(instrumentID, maxAge) == nil {
			return false
		}
	}
	return true
}

func (s *Server) activeMarketInstrumentIDs() []string {
	s.marketMu.Lock()
	defer s.marketMu.Unlock()
	ids := make([]string, 0, len(s.marketSubscriptions))
	seen := make(map[string]struct{}, len(s.marketSubscriptions))
	for _, entry := range s.marketSubscriptions {
		if entry.Market == "" || entry.Symbol == "" {
			continue
		}
		instrumentID := entry.Market + "." + entry.Symbol
		if _, exists := seen[instrumentID]; exists {
			continue
		}
		seen[instrumentID] = struct{}{}
		ids = append(ids, instrumentID)
	}
	return ids
}

func normalizeCandlePeriod(period string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(period)) {
	case "tick", "ticker", "k_tick":
		return "tick", nil
	case "1m", "1min", "k_1m":
		return "1m", nil
	case "3m", "3min", "k_3m":
		return "3m", nil
	case "5m", "5min", "k_5m":
		return "5m", nil
	case "10m", "10min", "k_10m":
		return "10m", nil
	case "15m", "15min", "k_15m":
		return "15m", nil
	case "30m", "30min", "k_30m":
		return "30m", nil
	case "60m", "60min", "1h", "k_60m":
		return "1h", nil
	case "1d", "day", "d", "k_day":
		return "1d", nil
	case "1w", "week", "w", "k_week":
		return "1w", nil
	case "1mo", "month", "mth", "k_month":
		return "1mo", nil
	default:
		return "", fmt.Errorf("unsupported period %q", period)
	}
}

func tickerTimestamp(ticker *bbgotypes.Ticker) string {
	resolvedAt := time.Now().UTC()
	if ticker != nil && !ticker.Time.IsZero() {
		resolvedAt = ticker.Time.UTC()
	}
	return resolvedAt.Format(time.RFC3339Nano)
}

func tickerOptionalFloat(value bbgofixedpoint.Value) *float64 {
	if value.IsZero() {
		return nil
	}
	floatValue := value.Float64()
	return &floatValue
}

func (s *Server) recordTickerSample(instrumentID string, ticker *bbgotypes.Ticker) *marketTickSample {
	if ticker == nil {
		return nil
	}
	price := ticker.Last.Float64()
	if ticker.Last.IsZero() {
		price = ticker.GetValidPrice().Float64()
	}
	if price == 0 {
		return nil
	}
	parts := strings.SplitN(instrumentID, ".", 2)
	if len(parts) != 2 {
		return nil
	}
	bid := price
	if !ticker.Buy.IsZero() {
		bid = ticker.Buy.Float64()
	}
	ask := price
	if !ticker.Sell.IsZero() {
		ask = ticker.Sell.Float64()
	}
	sample := marketTickSample{
		InstrumentID: instrumentID,
		Market:       parts[0],
		Symbol:       parts[1],
		Price:        price,
		Bid:          bid,
		Ask:          ask,
		OpenPrice:    tickerOptionalFloat(ticker.Open),
		HighPrice:    tickerOptionalFloat(ticker.High),
		LowPrice:     tickerOptionalFloat(ticker.Low),
		Volume:       ticker.Volume.Float64(),
		Turnover:     0,
		QuoteAt:      tickerTimestamp(ticker),
		ObservedAt:   time.Now().UTC().Format(time.RFC3339Nano),
		Source:       "bbgo:futu",
	}
	return s.storeTickerSample(sample)
}

func (s *Server) recordTradeTickSample(trade bbgotypes.Trade) *marketTickSample {
	instrumentID := strings.ToUpper(strings.TrimSpace(trade.Symbol))
	if instrumentID == "" || trade.Price.IsZero() {
		return nil
	}
	parts := strings.SplitN(instrumentID, ".", 2)
	if len(parts) != 2 {
		return nil
	}
	price := trade.Price.Float64()
	quoteAt := time.Now().UTC()
	if !trade.Time.Time().IsZero() {
		quoteAt = trade.Time.Time().UTC()
	}
	latest := s.latestTickerSample(instrumentID, tickCacheRetention)
	sample := marketTickSample{
		InstrumentID: instrumentID,
		Market:       parts[0],
		Symbol:       parts[1],
		Price:        price,
		Bid:          price,
		Ask:          price,
		Volume:       trade.Quantity.Float64(),
		QuoteAt:      quoteAt.Format(time.RFC3339Nano),
		ObservedAt:   time.Now().UTC().Format(time.RFC3339Nano),
		Source:       "bbgo:futu:stream",
	}
	if latest != nil {
		sample.Bid = latest.Bid
		sample.Ask = latest.Ask
		sample.OpenPrice = latest.OpenPrice
		sample.HighPrice = latest.HighPrice
		sample.LowPrice = latest.LowPrice
		sample.Turnover = latest.Turnover
		if sample.Volume == 0 {
			sample.Volume = latest.Volume
		}
	}
	return s.storeTickerSample(sample)
}

func (s *Server) storeTickerSample(sample marketTickSample) *marketTickSample {
	if sample.InstrumentID == "" || sample.Price == 0 {
		return nil
	}

	s.tickCacheMu.Lock()
	defer s.tickCacheMu.Unlock()

	retentionCutoff := time.Now().UTC().Add(-tickCacheRetention)
	samples := append([]marketTickSample(nil), s.tickCache[sample.InstrumentID]...)
	writeIndex := 0
	for _, existing := range samples {
		observedAt := parseQueryTime(existing.ObservedAt, retentionCutoff)
		if observedAt.Before(retentionCutoff) {
			continue
		}
		samples[writeIndex] = existing
		writeIndex++
	}
	samples = samples[:writeIndex]
	if len(samples) > 0 && marketTickSamplesEquivalent(samples[len(samples)-1], sample) {
		latest := samples[len(samples)-1]
		s.tickCache[sample.InstrumentID] = samples
		copyOfLatest := latest
		return &copyOfLatest
	}
	samples = append(samples, sample)
	if len(samples) > maxTickCacheSamples {
		samples = samples[len(samples)-maxTickCacheSamples:]
	}
	s.tickCache[sample.InstrumentID] = samples
	return &sample
}

func marketTickSamplesEquivalent(left marketTickSample, right marketTickSample) bool {
	return left.InstrumentID == right.InstrumentID &&
		left.Price == right.Price &&
		left.Bid == right.Bid &&
		left.Ask == right.Ask &&
		left.Volume == right.Volume &&
		left.QuoteAt == right.QuoteAt &&
		optionalFloatEqual(left.OpenPrice, right.OpenPrice) &&
		optionalFloatEqual(left.HighPrice, right.HighPrice) &&
		optionalFloatEqual(left.LowPrice, right.LowPrice)
}

func optionalFloatEqual(left *float64, right *float64) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}

func (s *Server) cachedTickCandles(instrumentID string, query map[string][]string, limit int) []map[string]any {
	endAt := parseQueryTime(firstQuery(query, "toTime", ""), time.Now())
	if queryEnd := firstQuery(query, "to", ""); queryEnd != "" {
		endAt = parseQueryTime(queryEnd, endAt)
	}
	defaultBegin := endAt.Add(-15 * time.Minute)
	beginAt := parseQueryTime(firstQuery(query, "fromTime", ""), defaultBegin)
	if queryBegin := firstQuery(query, "from", ""); queryBegin != "" {
		beginAt = parseQueryTime(queryBegin, beginAt)
	}

	s.tickCacheMu.Lock()
	samples := append([]marketTickSample(nil), s.tickCache[instrumentID]...)
	s.tickCacheMu.Unlock()

	candles := make([]map[string]any, 0, len(samples))
	previousCumulativeVolume := 0.0
	hasPreviousCumulativeVolume := false
	for _, sample := range samples {
		deltaVolume := 0.0
		if hasPreviousCumulativeVolume {
			deltaVolume = sample.Volume - previousCumulativeVolume
			if deltaVolume < 0 {
				deltaVolume = 0
			}
		}
		previousCumulativeVolume = sample.Volume
		hasPreviousCumulativeVolume = true

		observedAt := parseQueryTime(sample.ObservedAt, time.Time{})
		if !observedAt.IsZero() {
			if observedAt.Before(beginAt) || observedAt.After(endAt) {
				continue
			}
		}
		candles = append(candles, map[string]any{
			"period": "tick",
			"open":   sample.Price,
			"high":   sample.Price,
			"low":    sample.Price,
			"close":  sample.Price,
			"volume": deltaVolume,
			"at":     sample.ObservedAt,
		})
	}
	if limit > 0 && len(candles) > limit {
		candles = candles[len(candles)-limit:]
	}
	return candles
}

func liveTickEventFromSample(sample *marketTickSample) map[string]any {
	if sample == nil {
		return nil
	}
	return map[string]any{
		"type":     "market-data.tick",
		"at":       sample.ObservedAt,
		"brokerId": "futu",
		"instrument": map[string]any{
			"market":       sample.Market,
			"symbol":       sample.Symbol,
			"instrumentId": sample.InstrumentID,
		},
		"snapshot": map[string]any{
			"price":              sample.Price,
			"bid":                sample.Bid,
			"ask":                sample.Ask,
			"openPrice":          sample.OpenPrice,
			"highPrice":          sample.HighPrice,
			"lowPrice":           sample.LowPrice,
			"previousClosePrice": nil,
			"volume":             sample.Volume,
			"turnover":           sample.Turnover,
			"at":                 sample.QuoteAt,
			"observedAt":         sample.ObservedAt,
		},
		"source": sample.Source,
	}
}

func pathTail(path string, prefix string) (string, string) {
	tail := strings.TrimPrefix(path, prefix)
	parts := strings.SplitN(tail, "/", 2)
	if len(parts) != 2 {
		return "", ""
	}
	return parts[0], parts[1]
}

func firstQuery(query map[string][]string, key string, fallback string) string {
	values := query[key]
	if len(values) == 0 || values[0] == "" {
		return fallback
	}
	return values[0]
}

func intQuery(query map[string][]string, key string, fallback int) int {
	value, err := strconv.Atoi(firstQuery(query, key, strconv.Itoa(fallback)))
	if err != nil {
		return fallback
	}
	return value
}
