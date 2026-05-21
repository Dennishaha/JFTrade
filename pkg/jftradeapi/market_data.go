package jftradeapi

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/shopspring/decimal"

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
	liveStreamConnectTimeout     = 8 * time.Second
	liveStreamRetryBaseDelay     = 5 * time.Second
	liveStreamRetryMaxDelay      = 30 * time.Second
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
	response, err := s.marketSnapshotResponse(r.Context(), r.URL.Path, r.URL.Query())
	if err != nil {
		s.writeError(w, http.StatusBadGateway, "MARKET_SNAPSHOT_FAILED", err.Error())
		return
	}
	s.writeOK(w, response)
}

func (s *Server) marketSnapshotResponse(ctx context.Context, path string, query map[string][]string) (map[string]any, error) {
	market, symbol := pathTail(path, "/api/v1/market-data/snapshots/")
	market = strings.ToUpper(strings.TrimSpace(market))
	symbol = strings.ToUpper(strings.TrimSpace(symbol))
	instrumentID := market + "." + symbol
	forceRefresh := boolQuery(query, "refresh", false)
	sample := (*marketTickSample)(nil)
	if !forceRefresh {
		sample = s.latestTickerSample(instrumentID, liveTickSampleFreshness)
	}
	fromCache := sample != nil
	if sample == nil {
		snapshot, err := s.futuExchange().QueryQuoteSnapshot(ctx, instrumentID)
		if err != nil {
			return nil, err
		}
		sample = s.recordQuoteSnapshotSample(instrumentID, snapshot)
	}
	if sample == nil {
		return nil, fmt.Errorf("no snapshot available for %s", instrumentID)
	}
	return map[string]any{
		"request":  map[string]any{"market": market, "symbol": symbol, "instrumentId": instrumentID},
		"snapshot": snapshotMapFromSample(sample),
		"meta":     map[string]any{"instrumentId": instrumentID, "source": sample.Source, "resolvedAt": sample.ObservedAt, "fromCache": fromCache},
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
		extendedHours := market == "US"
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
					"meta":          candleMeta(instrumentID, true, extendedHours),
				}, nil
			}
			s.recordTickerSample(instrumentID, ticker)
		}
		candles := s.cachedTickCandles(instrumentID, query, limit)
		return map[string]any{
			"request":       map[string]any{"instrument": map[string]any{"market": market, "symbol": symbol, "instrumentId": instrumentID}, "period": period, "limit": limit},
			"candles":       candles,
			"totalReturned": len(candles),
			"meta":          candleMeta(instrumentID, fromLiveCache, extendedHours),
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
		session := futu.ClassifyMarketSession(instrumentID, kline.StartTime.Time().UTC())
		candles = append(candles, map[string]any{
			"period":  period,
			"open":    json.Number(kline.Open.String()),
			"high":    json.Number(kline.High.String()),
			"low":     json.Number(kline.Low.String()),
			"close":   json.Number(kline.Close.String()),
			"volume":  kline.Volume.Float64(),
			"at":      kline.StartTime.Time().UTC().Format(time.RFC3339Nano),
			"session": string(session),
		})
	}
	extendedHours := market == "US" && interval.Duration() <= time.Hour

	return map[string]any{
		"request":       map[string]any{"instrument": map[string]any{"market": market, "symbol": symbol, "instrumentId": instrumentID}, "period": period, "limit": limit},
		"candles":       candles,
		"totalReturned": len(candles),
		"meta":          candleMeta(instrumentID, false, extendedHours),
	}, nil
}

func candleMeta(instrumentID string, fromCache bool, extendedHours bool) map[string]any {
	session := "regular"
	if extendedHours {
		session = "all"
	}
	return map[string]any{
		"instrumentId":  instrumentID,
		"source":        "bbgo:futu",
		"resolvedAt":    time.Now().UTC().Format(time.RFC3339Nano),
		"fromCache":     fromCache,
		"extendedHours": extendedHours,
		"session":       session,
	}
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
	if !s.liveQuoteRetryAfter.IsZero() && now.Before(s.liveQuoteRetryAfter) {
		return
	}
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
		retryDelay := liveRetryDelay(s.liveQuoteFailureCount)
		s.liveQuoteFailureCount++
		s.liveQuoteRetryAfter = time.Now().UTC().Add(retryDelay)
		s.liveQuoteLastError = err.Error()
		log.Printf("JFTrade live quote refresh failed; retrying in %s: %v", retryDelay, err)
		return
	}
	s.liveQuoteFailureCount = 0
	s.liveQuoteRetryAfter = time.Time{}
	s.liveQuoteLastError = ""
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
	if !s.liveStreamRetryAfter.IsZero() && time.Now().UTC().Before(s.liveStreamRetryAfter) {
		s.liveStreamMu.Unlock()
		return
	}
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

	// Run the OpenD push subscription handshake off the websocket dispatch
	// goroutine so a slow handshake cannot block live tick fan-out, and use a
	// background context with a generous timeout so the connect is not bound
	// to the 900ms fallback poll budget.
	go func() {
		connectCtx, cancel := context.WithTimeout(context.Background(), liveStreamConnectTimeout)
		defer cancel()
		if err := stream.Connect(connectCtx); err != nil {
			retryDelay := s.nextLiveStreamRetryDelay()
			s.liveStreamMu.Lock()
			if s.liveStream == stream {
				s.liveStream = nil
				s.liveStreamKey = ""
			}
			s.liveStreamFailureCount++
			s.liveStreamRetryAfter = time.Now().UTC().Add(retryDelay)
			s.liveStreamLastError = err.Error()
			s.liveStreamMu.Unlock()
			_ = stream.Close()
			log.Printf("JFTrade live market stream connect failed; retrying in %s: %v", retryDelay, err)
			return
		}

		s.liveStreamMu.Lock()
		if s.liveStream == stream {
			s.liveStreamFailureCount = 0
			s.liveStreamRetryAfter = time.Time{}
			s.liveStreamLastError = ""
		}
		s.liveStreamMu.Unlock()
	}()
}

func (s *Server) nextLiveStreamRetryDelay() time.Duration {
	s.liveStreamMu.Lock()
	failures := s.liveStreamFailureCount
	s.liveStreamMu.Unlock()
	return liveRetryDelay(failures)
}

func liveRetryDelay(failures int) time.Duration {
	delay := liveStreamRetryBaseDelay
	for i := 0; i < failures && delay < liveStreamRetryMaxDelay; i++ {
		delay *= 2
	}
	if delay > liveStreamRetryMaxDelay {
		return liveStreamRetryMaxDelay
	}
	return delay
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

// tickerOptionalDecimal converts a bbgo fixedpoint.Value to *decimal.Decimal,
// returning nil when the value is zero (meaning "not available").
func tickerOptionalDecimal(value bbgofixedpoint.Value) *decimal.Decimal {
	if value.IsZero() {
		return nil
	}
	d := decimal.RequireFromString(value.String())
	return &d
}

// decimalPtr converts a *float64 from a Futu proto adapter into *decimal.Decimal.
// decimal.NewFromFloat uses the shortest float64 string representation, so
// proto values like 23.65 arrive as exactly "23.65" without IEEE-754 noise.
func decimalPtr(v *float64) *decimal.Decimal {
	if v == nil {
		return nil
	}
	d := decimal.NewFromFloat(*v)
	return &d
}

// priceJSON serialises a decimal.Decimal as a json.Number so the JSON
// encoder emits a literal numeric token (no quotes) with exact decimal digits.
func priceJSON(d decimal.Decimal) json.Number {
	return json.Number(d.String())
}

// optionalPriceJSON serialises a *decimal.Decimal as either null or a
// json.Number literal numeric token.
func optionalPriceJSON(d *decimal.Decimal) any {
	if d == nil {
		return nil
	}
	return json.Number(d.String())
}

// floatPtrToJSONNumber converts a *float64 from a Futu proto adapter struct
// (e.g. ExtendedMarketQuote) to a json.Number for serialisation.
func floatPtrToJSONNumber(v *float64) any {
	if v == nil {
		return nil
	}
	return json.Number(decimal.NewFromFloat(*v).String())
}

func snapshotMapFromSample(sample *marketTickSample) map[string]any {
	return map[string]any{
		"price":              priceJSON(sample.Price),
		"bid":                priceJSON(sample.Bid),
		"ask":                priceJSON(sample.Ask),
		"openPrice":          optionalPriceJSON(sample.OpenPrice),
		"highPrice":          optionalPriceJSON(sample.HighPrice),
		"lowPrice":           optionalPriceJSON(sample.LowPrice),
		"previousClosePrice": optionalPriceJSON(sample.PreviousClosePrice),
		"lastClosePrice":     optionalPriceJSON(sample.LastClosePrice),
		"volume":             sample.Volume,
		"turnover":           sample.Turnover,
		"at":                 sample.QuoteAt,
		"observedAt":         sample.ObservedAt,
		"session":            sample.Session,
		"extendedHours":      sample.ExtendedHours,
		"extended": map[string]any{
			"preMarket":   extendedMarketQuoteMap(sample.PreMarket),
			"afterMarket": extendedMarketQuoteMap(sample.AfterMarket),
			"overnight":   extendedMarketQuoteMap(sample.Overnight),
		},
	}
}

func extendedMarketQuoteMap(quote *futu.ExtendedMarketQuote) map[string]any {
	if quote == nil {
		return nil
	}
	return map[string]any{
		"price":      floatPtrToJSONNumber(quote.Price),
		"highPrice":  floatPtrToJSONNumber(quote.HighPrice),
		"lowPrice":   floatPtrToJSONNumber(quote.LowPrice),
		"volume":     quote.Volume,
		"turnover":   floatPtrToJSONNumber(quote.Turnover),
		"changeVal":  floatPtrToJSONNumber(quote.ChangeVal),
		"changeRate": quote.ChangeRate,
		"amplitude":  quote.Amplitude,
	}
}

func (s *Server) recordQuoteSnapshotSample(instrumentID string, snapshot *futu.QuoteSnapshot) *marketTickSample {
	if snapshot == nil || snapshot.Price == 0 {
		return nil
	}
	parts := strings.SplitN(instrumentID, ".", 2)
	if len(parts) != 2 {
		return nil
	}
	sample := marketTickSample{
		InstrumentID:       instrumentID,
		Market:             parts[0],
		Symbol:             parts[1],
		Price:              decimal.NewFromFloat(snapshot.Price),
		Bid:                decimal.NewFromFloat(snapshot.Bid),
		Ask:                decimal.NewFromFloat(snapshot.Ask),
		OpenPrice:          decimalPtr(snapshot.OpenPrice),
		HighPrice:          decimalPtr(snapshot.HighPrice),
		LowPrice:           decimalPtr(snapshot.LowPrice),
		PreviousClosePrice: decimalPtr(snapshot.PreviousClosePrice),
		LastClosePrice:     decimalPtr(snapshot.LastClosePrice),
		Volume:             snapshot.Volume,
		Turnover:           snapshot.Turnover,
		QuoteAt:            snapshot.QuoteAt.UTC().Format(time.RFC3339Nano),
		ObservedAt:         time.Now().UTC().Format(time.RFC3339Nano),
		Source:             "bbgo:futu",
		Session:            string(snapshot.Session),
		ExtendedHours:      snapshot.ExtendedHours,
		PreMarket:          snapshot.PreMarket,
		AfterMarket:        snapshot.AfterMarket,
		Overnight:          snapshot.Overnight,
	}
	return s.storeTickerSample(sample)
}

func (s *Server) recordTickerSample(instrumentID string, ticker *bbgotypes.Ticker) *marketTickSample {
	if ticker == nil {
		return nil
	}
	priceFixed := ticker.Last
	if priceFixed.IsZero() {
		priceFixed = ticker.GetValidPrice()
	}
	if priceFixed.IsZero() {
		return nil
	}
	parts := strings.SplitN(instrumentID, ".", 2)
	if len(parts) != 2 {
		return nil
	}
	price := decimal.RequireFromString(priceFixed.String())
	bid := price
	if !ticker.Buy.IsZero() {
		bid = decimal.RequireFromString(ticker.Buy.String())
	}
	ask := price
	if !ticker.Sell.IsZero() {
		ask = decimal.RequireFromString(ticker.Sell.String())
	}
	session := futu.ClassifyMarketSession(instrumentID, time.Now().UTC())
	sample := marketTickSample{
		InstrumentID:  instrumentID,
		Market:        parts[0],
		Symbol:        parts[1],
		Price:         price,
		Bid:           bid,
		Ask:           ask,
		OpenPrice:     tickerOptionalDecimal(ticker.Open),
		HighPrice:     tickerOptionalDecimal(ticker.High),
		LowPrice:      tickerOptionalDecimal(ticker.Low),
		Volume:        ticker.Volume.Float64(),
		Turnover:      0,
		QuoteAt:       tickerTimestamp(ticker),
		ObservedAt:    time.Now().UTC().Format(time.RFC3339Nano),
		Source:        "bbgo:futu",
		Session:       string(session),
		ExtendedHours: futu.IsExtendedMarketSession(session),
	}
	// The ticker stream does not carry PreviousClosePrice or extended-session
	// quote blocks (PreMarket/AfterMarket/Overnight). Inherit those from the
	// most-recent cached sample so they are not silently dropped when a live
	// ticker event overwrites the snapshot used by the frontend.
	if latest := s.latestTickerSample(instrumentID, tickCacheRetention); latest != nil {
		if sample.PreviousClosePrice == nil {
			sample.PreviousClosePrice = latest.PreviousClosePrice
		}
		if sample.LastClosePrice == nil {
			sample.LastClosePrice = latest.LastClosePrice
		}
		if sample.PreMarket == nil {
			sample.PreMarket = latest.PreMarket
		}
		if sample.AfterMarket == nil {
			sample.AfterMarket = latest.AfterMarket
		}
		if sample.Overnight == nil {
			sample.Overnight = latest.Overnight
		}
		if sample.Turnover == 0 {
			sample.Turnover = latest.Turnover
		}
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
	price := decimal.RequireFromString(trade.Price.String())
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
		sample.PreviousClosePrice = latest.PreviousClosePrice
		sample.LastClosePrice = latest.LastClosePrice
		sample.Turnover = latest.Turnover
		sample.Session = latest.Session
		sample.ExtendedHours = latest.ExtendedHours
		sample.PreMarket = latest.PreMarket
		sample.AfterMarket = latest.AfterMarket
		sample.Overnight = latest.Overnight
		if sample.Volume == 0 {
			sample.Volume = latest.Volume
		}
	} else {
		session := futu.ClassifyMarketSession(instrumentID, time.Now().UTC())
		sample.Session = string(session)
		sample.ExtendedHours = futu.IsExtendedMarketSession(session)
	}
	return s.storeTickerSample(sample)
}

func (s *Server) storeTickerSample(sample marketTickSample) *marketTickSample {
	if sample.InstrumentID == "" || sample.Price.IsZero() {
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
		// Promote the cached sample's Source/ObservedAt when a more
		// authoritative event (an OpenD push trade) arrives carrying the same
		// numeric payload as a fallback poll that was recorded first. Without
		// this promotion the stream event is silently deduped, the WS write
		// loop's per-instrument ObservedAt dedupe never advances, and the
		// frontend keeps seeing only `source=bbgo:futu` fallback events.
		if shouldPromoteTickSampleSource(latest.Source, sample.Source) {
			latest.Source = sample.Source
			latest.ObservedAt = sample.ObservedAt
			samples[len(samples)-1] = latest
		}
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
		left.Price.Equal(right.Price) &&
		left.Bid.Equal(right.Bid) &&
		left.Ask.Equal(right.Ask) &&
		left.Volume == right.Volume &&
		left.QuoteAt == right.QuoteAt &&
		left.Session == right.Session &&
		left.ExtendedHours == right.ExtendedHours &&
		optionalDecimalEqual(left.OpenPrice, right.OpenPrice) &&
		optionalDecimalEqual(left.HighPrice, right.HighPrice) &&
		optionalDecimalEqual(left.LowPrice, right.LowPrice) &&
		optionalDecimalEqual(left.PreviousClosePrice, right.PreviousClosePrice)
}

func optionalDecimalEqual(left *decimal.Decimal, right *decimal.Decimal) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return left.Equal(*right)
}

// shouldPromoteTickSampleSource reports whether a newly-recorded sample's
// source is strictly more authoritative than the cached sample's source and
// should replace it. Today the only promotion is fallback -> stream so that
// OpenD push trades surface to the websocket even when a 1s fallback poll
// recorded an equivalent sample microseconds earlier.
func shouldPromoteTickSampleSource(cachedSource string, incomingSource string) bool {
	return incomingSource == "bbgo:futu:stream" && cachedSource != "bbgo:futu:stream"
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
			"period":  "tick",
			"open":    priceJSON(sample.Price),
			"high":    priceJSON(sample.Price),
			"low":     priceJSON(sample.Price),
			"close":   priceJSON(sample.Price),
			"volume":  deltaVolume,
			"at":      sample.ObservedAt,
			"session": sample.Session,
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
			"price":              priceJSON(sample.Price),
			"bid":                priceJSON(sample.Bid),
			"ask":                priceJSON(sample.Ask),
			"openPrice":          optionalPriceJSON(sample.OpenPrice),
			"highPrice":          optionalPriceJSON(sample.HighPrice),
			"lowPrice":           optionalPriceJSON(sample.LowPrice),
			"previousClosePrice": optionalPriceJSON(sample.PreviousClosePrice),
			"lastClosePrice":     optionalPriceJSON(sample.LastClosePrice),
			"volume":             sample.Volume,
			"turnover":           sample.Turnover,
			"at":                 sample.QuoteAt,
			"observedAt":         sample.ObservedAt,
			"session":            sample.Session,
			"extendedHours":      sample.ExtendedHours,
			"extended": map[string]any{
				"preMarket":   extendedMarketQuoteMap(sample.PreMarket),
				"afterMarket": extendedMarketQuoteMap(sample.AfterMarket),
				"overnight":   extendedMarketQuoteMap(sample.Overnight),
			},
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

func boolQuery(query map[string][]string, key string, fallback bool) bool {
	value := strings.TrimSpace(strings.ToLower(firstQuery(query, key, "")))
	if value == "" {
		return fallback
	}
	switch value {
	case "1", "true", "yes", "y", "on":
		return true
	case "0", "false", "no", "n", "off":
		return false
	default:
		return fallback
	}
}
