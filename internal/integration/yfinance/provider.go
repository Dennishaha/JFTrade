package yfinance

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/jftrade/jftrade-main/internal/marketdata"
	"github.com/jftrade/jftrade-main/pkg/market"
)

const (
	defaultSearchLimit    = 20
	maxSearchLimit        = 100
	defaultCandleLimit    = 200
	maxCandleLimit        = 1000
	tickerQueryWorkers    = 4
	yfinanceDefaultMarket = "US"
	yfinancePollInterval  = 15 * time.Second
)

var supportedCandlePeriods = map[string]struct{}{
	"1m": {}, "5m": {}, "15m": {}, "30m": {}, "1h": {}, "1d": {}, "1w": {}, "1mo": {},
}

// oneMinuteCandleMaxDays is the Yahoo Finance rolling window for 1m interval data.
const oneMinuteCandleMaxDays = 7

type Provider struct {
	client        *Client
	now           func() time.Time
	pollingPolicy marketdata.QuotePollingPolicy
}

var (
	_ marketdata.Provider                 = (*Provider)(nil)
	_ marketdata.QuoteSource              = (*Provider)(nil)
	_ marketdata.QuotePollingPolicySource = (*Provider)(nil)
)

// NewProvider creates a market-data provider backed by the application-owned
// local yfinance sidecar.
func NewProvider(endpoint string) (*Provider, error) {
	client, err := NewClient(endpoint, &http.Client{Timeout: defaultRequestTimeout})
	if err != nil {
		return nil, fmt.Errorf("configure yfinance provider: %w", err)
	}
	return &Provider{
		client: client,
		now:    time.Now,
		pollingPolicy: marketdata.QuotePollingPolicy{
			Interval: yfinancePollInterval,
			Timeout:  defaultRequestTimeout,
		},
	}, nil
}

// QuotePollingPolicy prevents the shared collector from applying its
// low-latency broker fallback cadence to Yahoo's delayed HTTP snapshots.
func (p *Provider) QuotePollingPolicy() marketdata.QuotePollingPolicy {
	if p == nil {
		return marketdata.QuotePollingPolicy{}
	}
	return p.pollingPolicy
}

func (p *Provider) Descriptor(context.Context) (marketdata.ProviderDescriptor, error) {
	return marketdata.ProviderDescriptor{
		ProviderID:       "yahoo-finance",
		DisplayName:      "Yahoo Finance (yfinance)",
		BrokerID:         sourceID,
		Source:           sourceID,
		DefaultMarket:    yfinanceDefaultMarket,
		SupportedMarkets: []string{"US", "HK", "SH", "SZ"},
		Transports:       []string{"http-poll"},
		Capabilities: marketdata.ProviderCapabilities{
			Snapshots: true, HistoricalCandles: true, InstrumentSearch: true, ExtendedHours: true,
			CandleIntervals: []string{"1m", "5m", "15m", "30m", "1h", "1d", "1w", "1mo"},
			Sessions:        []string{"regular", "pre", "after", "closed"},
		},
		Constraints: marketdata.ProviderConstraints{},
		Notes: []string{
			"Quotes may be delayed by 15 minutes under Yahoo Finance's free data access.",
			"US, HK, SH, and SZ snapshots and historical candles are available through delayed HTTP polling.",
			"Order book depth, streaming quotes, and trading are unavailable; Yahoo does not provide a dependable overnight session.",
		},
	}, nil
}

func (p *Provider) GetMarkets(ctx context.Context) ([]marketdata.MarketProfile, error) {
	profiles, err := p.client.markets(ctx)
	if err != nil {
		return nil, err
	}
	return convertMarkets(profiles)
}

func (p *Provider) GetSecurityDetails(
	ctx context.Context,
	marketValue string,
	symbol string,
) (marketdata.SecurityDetails, error) {
	instrument, err := normalizeIdentity(marketValue, symbol, "")
	if err != nil {
		return nil, err
	}
	response, err := p.client.security(ctx, instrument.market, instrument.symbol)
	if err != nil {
		return nil, err
	}
	return convertSecurity(response, instrument, p.currentTime())
}

func (p *Provider) LookupInstrument(
	ctx context.Context,
	marketValue string,
	code string,
) ([]marketdata.InstrumentCandidate, error) {
	instrument, err := normalizeIdentity(marketValue, code, "")
	if err != nil {
		return nil, err
	}
	response, err := p.client.security(ctx, instrument.market, instrument.symbol)
	if isNotFound(err) {
		return []marketdata.InstrumentCandidate{}, nil
	}
	if err != nil {
		return nil, err
	}
	entries, err := convertCandidates([]remoteInstrument{{
		Market: response.Market, ResolvedMarket: response.Market, InstrumentID: response.InstrumentID,
		Code: response.Symbol, Symbol: response.Symbol, Name: response.Name,
		SecurityType: response.SecurityType, Exchange: response.Exchange, Selectable: true, Source: response.Source,
		SupportedPeriods: response.SupportedPeriods,
	}})
	if err != nil {
		return nil, err
	}
	if len(entries) != 1 || entries[0].InstrumentID != instrument.id {
		return nil, fmt.Errorf("%w: exact lookup identity mismatch", ErrInvalidResponse)
	}
	return entries, nil
}

func (p *Provider) SearchInstruments(
	ctx context.Context,
	query string,
	limit int,
) ([]marketdata.InstrumentCandidate, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, fmt.Errorf("instrument search query is required")
	}
	limit = normalizeLimit(limit, defaultSearchLimit, maxSearchLimit)
	response, err := p.client.search(ctx, query, limit)
	if err != nil {
		return nil, err
	}
	return convertCandidates(response)
}

func (p *Provider) QuerySnapshot(ctx context.Context, instrumentID string) (*marketdata.Tick, error) {
	return p.queryTick(ctx, instrumentID)
}

func (p *Provider) QueryTicker(ctx context.Context, instrumentID string) (*marketdata.Tick, error) {
	return p.queryTick(ctx, instrumentID)
}

func (p *Provider) queryTick(ctx context.Context, instrumentID string) (*marketdata.Tick, error) {
	instrument, err := normalizeIdentity("", "", instrumentID)
	if err != nil {
		return nil, err
	}
	response, err := p.client.snapshot(ctx, instrument.market, instrument.symbol)
	if err != nil {
		return nil, err
	}
	return convertSnapshot(response, instrument, p.currentTime())
}

func (p *Provider) QueryTickers(
	ctx context.Context,
	instrumentIDs []string,
) (map[string]marketdata.Tick, error) {
	ids := uniqueInstrumentIDs(instrumentIDs)
	if len(ids) == 0 {
		return map[string]marketdata.Tick{}, nil
	}
	jobs := make(chan string)
	results := make(chan tickerQueryResult, len(ids))
	workers := min(tickerQueryWorkers, len(ids))
	var waitGroup sync.WaitGroup
	waitGroup.Add(workers)
	for range workers {
		go p.queryTickerWorker(ctx, jobs, results, &waitGroup)
	}
	go func() {
		defer close(jobs)
		for _, instrumentID := range ids {
			select {
			case <-ctx.Done():
				return
			case jobs <- instrumentID:
			}
		}
	}()
	go func() {
		waitGroup.Wait()
		close(results)
	}()
	return collectTickerResults(results, len(ids))
}

func (p *Provider) queryTickerWorker(
	ctx context.Context,
	jobs <-chan string,
	results chan<- tickerQueryResult,
	waitGroup *sync.WaitGroup,
) {
	defer waitGroup.Done()
	for instrumentID := range jobs {
		tick, err := p.QueryTicker(ctx, instrumentID)
		results <- tickerQueryResult{instrumentID: instrumentID, tick: tick, err: err}
	}
}

func (p *Provider) GetHistoricalCandles(
	ctx context.Context,
	marketValue string,
	symbol string,
	period string,
	limit int,
	fromTime string,
	toTime string,
) (marketdata.CandlesResponse, error) {
	instrument, err := normalizeIdentity(marketValue, symbol, "")
	if err != nil {
		return nil, err
	}
	period = strings.ToLower(strings.TrimSpace(period))
	if _, ok := supportedCandlePeriods[period]; !ok {
		return nil, fmt.Errorf("%w: candle period %q", ErrUnsupported, period)
	}
	if period == "1m" {
		if err := validateOneMinuteCandleWindow(fromTime, p.now()); err != nil {
			return nil, err
		}
	}
	limit = normalizeLimit(limit, defaultCandleLimit, maxCandleLimit)
	response, err := p.client.candles(
		ctx, instrument.market, instrument.symbol, period, limit, fromTime, toTime,
	)
	if err != nil {
		return nil, err
	}
	return convertCandles(response, instrument, period, limit, p.currentTime())
}

func (p *Provider) GetDepth(
	context.Context,
	string,
	string,
	int,
) (marketdata.DepthResponse, error) {
	return nil, fmt.Errorf("%w: order book depth", ErrUnsupported)
}

func (p *Provider) NormalizeInstrument(
	_ context.Context,
	input map[string]any,
) (map[string]any, error) {
	marketValue, err := optionalInputString(input, "market")
	if err != nil {
		return nil, err
	}
	symbol, err := optionalInputString(input, "symbol")
	if err != nil {
		return nil, err
	}
	code, err := optionalInputString(input, "code")
	if err != nil {
		return nil, err
	}
	instrumentID, err := optionalInputString(input, "instrumentId")
	if err != nil {
		return nil, err
	}
	if symbol == "" {
		symbol = code
	} else if code != "" && !symbolMatchesCode(symbol, code, marketValue) {
		return nil, fmt.Errorf("code %q does not match symbol %q", code, symbol)
	}
	instrument, err := normalizeIdentity(marketValue, symbol, instrumentID)
	if err != nil {
		return nil, err
	}
	resolvedMarket := resolvedMarketForLeaf(instrument.market)
	return map[string]any{
		"market": instrument.market, "prefix": instrument.market, "code": instrument.symbol,
		"symbol": instrument.id, "instrumentId": instrument.id, "resolvedMarket": resolvedMarket,
	}, nil
}

func (p *Provider) Health(ctx context.Context) (marketdata.HealthStatus, error) {
	response, err := p.client.health(ctx)
	if err != nil {
		return marketdata.HealthStatus{}, err
	}
	return marketdata.HealthStatus{
		Connected:   response.OK,
		StreamMode:  "snapshot-poll-delayed",
		ActiveCount: 0,
		Readiness:   marketdata.ProviderReadiness(response.RuntimeState),
		LastError:   response.WarmupError,
	}, nil
}

type normalizedInstrument struct {
	market string
	symbol string
	id     string
}

func normalizeIdentity(marketValue, symbol, instrumentID string) (normalizedInstrument, error) {
	canonical, err := canonicalMarket(marketValue)
	if err != nil {
		return normalizedInstrument{}, err
	}
	symbol = canonicalQualifiedSymbol(symbol)
	instrumentID = canonicalQualifiedSymbol(instrumentID)
	parsed, err := market.ParseInstrument(market.InstrumentInput{
		Market: canonical, Symbol: symbol, InstrumentID: instrumentID,
	})
	if err != nil {
		return normalizedInstrument{}, err
	}
	if !isSupportedLeafMarket(parsed.Prefix) {
		return normalizedInstrument{}, fmt.Errorf("%w: market %q", ErrUnsupported, parsed.Prefix)
	}
	code := canonicalInstrumentCode(parsed.Prefix, parsed.Code)
	if err := validateInstrumentCode(parsed.Prefix, code); err != nil {
		return normalizedInstrument{}, err
	}
	return normalizedInstrument{market: parsed.Prefix, symbol: code, id: parsed.Prefix + "." + code}, nil
}

func canonicalMarket(value string) (string, error) {
	switch strings.ToUpper(strings.TrimSpace(value)) {
	case "":
		return "", nil
	case "US", "USA", "NYSE", "NASDAQ", "AMEX":
		return yfinanceDefaultMarket, nil
	case "HK", "HKEX", "HKG":
		return "HK", nil
	case "SH", "CNSH", "SHH", "SSE":
		return "SH", nil
	case "SZ", "CNSZ", "SHZ", "SZSE":
		return "SZ", nil
	case "CN":
		return "CN", nil
	default:
		return "", fmt.Errorf("%w: market %q", ErrUnsupported, value)
	}
}

func canonicalQualifiedSymbol(value string) string {
	value = strings.ToUpper(strings.TrimSpace(value))
	value = strings.Replace(value, ":", ".", 1)
	for suffix, prefix := range map[string]string{".HK": "HK", ".SS": "SH", ".SZ": "SZ"} {
		if strings.HasSuffix(value, suffix) && len(value) > len(suffix) {
			return canonicalQualifiedSymbol(prefix + "." + strings.TrimSuffix(value, suffix))
		}
	}
	parts := strings.SplitN(value, ".", 2)
	if len(parts) != 2 {
		return value
	}
	if marketValue, err := canonicalMarket(parts[0]); err == nil && marketValue != "" {
		return marketValue + "." + canonicalInstrumentCode(marketValue, parts[1])
	}
	return value
}

func symbolMatchesCode(symbol, code, marketValue string) bool {
	normalized := canonicalQualifiedSymbol(symbol)
	prefix := ""
	if parts := strings.SplitN(normalized, ".", 2); len(parts) == 2 {
		prefix = parts[0]
		normalized = parts[1]
	}
	if prefix == "" {
		prefix, _ = canonicalMarket(marketValue)
	}
	return strings.EqualFold(
		strings.TrimSpace(normalized),
		strings.TrimSpace(canonicalInstrumentCode(prefix, code)),
	)
}

func canonicalInstrumentCode(prefix, code string) string {
	code = strings.ToUpper(strings.TrimSpace(code))
	if prefix == "HK" && isDigits(code) && len(code) <= 5 {
		return strings.Repeat("0", 5-len(code)) + code
	}
	return code
}

func validateInstrumentCode(prefix, code string) error {
	switch prefix {
	case "HK":
		if !isDigits(code) || len(code) != 5 {
			return fmt.Errorf("%w: HK symbols must contain one to five digits", ErrUnsupported)
		}
	case "SH", "SZ":
		if !isDigits(code) || len(code) != 6 {
			return fmt.Errorf("%w: %s symbols must contain six digits", ErrUnsupported, prefix)
		}
	}
	return nil
}

func isDigits(value string) bool {
	if value == "" {
		return false
	}
	for _, character := range value {
		if character < '0' || character > '9' {
			return false
		}
	}
	return true
}

func resolvedMarketForLeaf(leaf string) string {
	if leaf == "SH" || leaf == "SZ" {
		return "CN"
	}
	return leaf
}

func normalizeLimit(value, defaultValue, maximum int) int {
	if value <= 0 {
		return defaultValue
	}
	return min(value, maximum)
}

func optionalInputString(input map[string]any, key string) (string, error) {
	value, ok := input[key]
	if !ok || value == nil {
		return "", nil
	}
	text, ok := value.(string)
	if !ok {
		return "", fmt.Errorf("%s must be a string", key)
	}
	return strings.TrimSpace(text), nil
}

// validateOneMinuteCandleWindow returns ErrUnsupported when a 1m candle
// request's from_time falls outside the 7-day rolling window that Yahoo
// Finance makes available. A missing from_time is always accepted; the
// sidecar will apply its own default window.
func validateOneMinuteCandleWindow(fromTime string, now time.Time) error {
	fromTime = strings.TrimSpace(fromTime)
	if fromTime == "" {
		return nil
	}
	from, err := time.Parse(time.RFC3339, fromTime)
	if err != nil {
		from, err = time.Parse(time.RFC3339Nano, fromTime)
		if err != nil {
			return fmt.Errorf("%w: from_time %q is not a valid RFC3339 timestamp", ErrUnsupported, fromTime)
		}
	}
	cutoff := now.Add(-oneMinuteCandleMaxDays * 24 * time.Hour)
	if from.Before(cutoff) {
		return fmt.Errorf(
			"%w: 1m candle data is only available for the last %d days",
			ErrUnsupported, oneMinuteCandleMaxDays,
		)
	}
	return nil
}

func uniqueInstrumentIDs(values []string) []string {
	result := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		normalized := canonicalQualifiedSymbol(value)
		if _, ok := seen[normalized]; ok {
			continue
		}
		seen[normalized] = struct{}{}
		result = append(result, normalized)
	}
	return result
}

type tickerQueryResult struct {
	instrumentID string
	tick         *marketdata.Tick
	err          error
}

func collectTickerResults(
	stream <-chan tickerQueryResult,
	expected int,
) (map[string]marketdata.Tick, error) {
	ticks := make(map[string]marketdata.Tick, expected)
	failures := make([]error, 0)
	for result := range stream {
		if result.err != nil {
			failures = append(failures, fmt.Errorf("%s: %w", result.instrumentID, result.err))
			continue
		}
		if result.tick == nil {
			failures = append(failures, fmt.Errorf("%s: no snapshot returned", result.instrumentID))
			continue
		}
		ticks[result.tick.InstrumentID] = *result.tick
	}
	if len(ticks) > 0 {
		return ticks, nil
	}
	if len(failures) == 1 {
		return nil, failures[0]
	}
	if len(failures) > 1 {
		return nil, errors.Join(failures...)
	}
	return nil, context.Canceled
}

func (p *Provider) currentTime() time.Time {
	if p != nil && p.now != nil {
		return p.now().UTC()
	}
	return time.Now().UTC()
}
