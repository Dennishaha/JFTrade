package akshare

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"net/http"
	"strings"
	"time"

	"github.com/jftrade/jftrade-main/internal/marketdata"
)

const (
	defaultSearchLimit = 20
	maxSearchLimit     = 100
	defaultCandleLimit = 200
	maxCandleLimit     = 1000
	maxBatchSize       = 100
	pollInterval       = 15 * time.Second
)

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

// NewProvider creates a delayed, polling-only AKShare provider backed by the
// shared Python market-data sidecar.
func NewProvider(endpoint string) (*Provider, error) {
	client, err := NewClient(endpoint, &http.Client{Timeout: defaultRequestTimeout})
	if err != nil {
		return nil, fmt.Errorf("configure AKShare provider: %w", err)
	}
	return &Provider{
		client: client, now: time.Now,
		pollingPolicy: marketdata.QuotePollingPolicy{
			Interval: pollInterval, Timeout: defaultRequestTimeout,
		},
	}, nil
}

func (p *Provider) QuotePollingPolicy() marketdata.QuotePollingPolicy {
	if p == nil {
		return marketdata.QuotePollingPolicy{}
	}
	return p.pollingPolicy
}

func (p *Provider) Descriptor(context.Context) (marketdata.ProviderDescriptor, error) {
	return ProviderDescriptor(), nil
}

func ProviderDescriptor() marketdata.ProviderDescriptor {
	return marketdata.ProviderDescriptor{
		SelectionID: "akshare",
		ProviderID:  sourceID, DisplayName: "AKShare", BrokerID: sourceID, Source: sourceID,
		DefaultMarket: defaultMarket, SupportedMarkets: []string{"US", "HK", "SH", "SZ"},
		Transports: []string{"http-poll"},
		Capabilities: marketdata.ProviderCapabilities{
			Snapshots: true, HistoricalCandles: true, InstrumentSearch: true,
			CandleIntervals:  append([]string(nil), candlePeriodOrder...),
			Sessions:         []string{"regular", "closed"},
			PriceAdjustments: []string{"none", "forward", "backward"},
			HistoricalLookbackDays: map[string]int{
				"1m":    5,
				"US:5m": 5, "US:15m": 5, "US:30m": 5, "US:1h": 5,
			},
		},
		Constraints: marketdata.ProviderConstraints{},
		Notes: []string{
			"Quotes are best-effort and may be delayed by the upstream public data source.",
			"US, HK, SH, and SZ securities and historical candles are available through HTTP polling.",
			"Streaming quotes, order book depth, extended hours, and trading are unavailable.",
		},
	}
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
		Market: response.Market, ResolvedMarket: resolvedMarketForLeaf(response.Market),
		InstrumentID: response.InstrumentID, Code: response.Symbol, Symbol: response.Symbol,
		Name: response.Name, SecurityType: response.SecurityType, Exchange: response.Exchange,
		Selectable: true, Source: response.Source, SupportedPeriods: response.SupportedPeriods,
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
	instruments, err := uniqueInstrumentIDs(instrumentIDs)
	if err != nil {
		return nil, err
	}
	if len(instruments) == 0 {
		return map[string]marketdata.Tick{}, nil
	}
	ticks := make(map[string]marketdata.Tick, len(instruments))
	failures := make([]error, 0)
	for offset := 0; offset < len(instruments); offset += maxBatchSize {
		end := min(offset+maxBatchSize, len(instruments))
		batchTicks, batchFailures := p.queryTickerBatch(ctx, instruments[offset:end])
		maps.Copy(ticks, batchTicks)
		failures = append(failures, batchFailures...)
		if ctx.Err() != nil {
			break
		}
	}
	if len(ticks) > 0 {
		return ticks, nil
	}
	if len(failures) > 0 {
		return nil, errors.Join(failures...)
	}
	return nil, context.Canceled
}

func (p *Provider) queryTickerBatch(
	ctx context.Context,
	instruments []normalizedInstrument,
) (map[string]marketdata.Tick, []error) {
	ids := make([]string, 0, len(instruments))
	expected := make(map[string]normalizedInstrument, len(instruments))
	for _, instrument := range instruments {
		ids = append(ids, instrument.id)
		expected[instrument.id] = instrument
	}
	response, err := p.client.snapshots(ctx, ids)
	if err != nil {
		return nil, []error{err}
	}
	ticks := make(map[string]marketdata.Tick, len(instruments))
	failures := batchRemoteErrors(response.Errors)
	for _, value := range response.values() {
		identity, identityErr := normalizeIdentity(value.Market, value.Symbol, value.InstrumentID)
		instrument, ok := expected[identity.id]
		if identityErr != nil || !ok {
			failures = append(failures, fmt.Errorf("%w: batch returned an unexpected identity", ErrInvalidResponse))
			continue
		}
		tick, conversionErr := convertSnapshot(value, instrument, p.currentTime())
		if conversionErr != nil {
			failures = append(failures, fmt.Errorf("%s: %w", instrument.id, conversionErr))
			continue
		}
		if _, duplicate := ticks[instrument.id]; duplicate {
			failures = append(failures, fmt.Errorf("%w: duplicate snapshot %s", ErrInvalidResponse, instrument.id))
			continue
		}
		ticks[instrument.id] = *tick
	}
	for _, instrument := range instruments {
		if _, ok := ticks[instrument.id]; !ok {
			failures = append(failures, fmt.Errorf("%s: no snapshot returned", instrument.id))
		}
	}
	return ticks, failures
}

func batchRemoteErrors(values []remoteBatchError) []error {
	result := make([]error, 0, len(values))
	for _, value := range values {
		message := strings.TrimSpace(value.Message)
		if message == "" {
			message = "snapshot unavailable"
		}
		if code := strings.TrimSpace(value.Code); code != "" {
			message = code + ": " + message
		}
		result = append(result, fmt.Errorf("%s: %s", strings.TrimSpace(value.InstrumentID), message))
	}
	return result
}

func (p *Provider) GetHistoricalCandles(
	ctx context.Context,
	query marketdata.HistoricalCandlesQuery,
) (marketdata.CandlesResponse, error) {
	marketValue, symbol, period := query.Market, query.Symbol, query.Period
	limit, fromTime, toTime, beforeTime := query.Limit, query.FromTime, query.ToTime, query.BeforeTime
	instrument, err := normalizeIdentity(marketValue, symbol, "")
	if err != nil {
		return nil, err
	}
	period = strings.ToLower(strings.TrimSpace(period))
	if !supportedCandlePeriod(period) {
		return nil, fmt.Errorf("%w: candle period %q", ErrUnsupported, period)
	}
	adjustment := strings.ToLower(strings.TrimSpace(query.Adjustment))
	switch adjustment {
	case "", "none":
	case "forward", "backward":
		if isIntradayCandlePeriod(period) {
			return nil, fmt.Errorf("%w: %s price adjustment for %q candles", ErrUnsupported, adjustment, period)
		}
	default:
		return nil, fmt.Errorf("%w: price adjustment %q", ErrUnsupported, adjustment)
	}
	sessions, err := marketdata.ResolveCandleSessions(
		query.Sessions,
		query.SessionsSpecified,
		[]marketdata.CandleSession{marketdata.CandleSessionRegular},
	)
	if err != nil {
		return nil, err
	}
	limit = normalizeLimit(limit, defaultCandleLimit, maxCandleLimit)
	response, err := p.client.candles(
		ctx, instrument.market, instrument.symbol, period, adjustment, limit, fromTime, toTime, beforeTime,
		marketdata.CandleSessionStrings(sessions),
	)
	if err != nil {
		return nil, err
	}
	converted, err := convertCandlesForSessions(response, instrument, period, limit, sessions, p.currentTime())
	if err != nil {
		return nil, err
	}
	return validateHistoricalCandleResponse(converted, beforeTime, fromTime, toTime)
}

func isIntradayCandlePeriod(period string) bool {
	switch period {
	case "1m", "5m", "15m", "30m", "1h":
		return true
	default:
		return false
	}
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
	return map[string]any{
		"market": instrument.market, "prefix": instrument.market, "code": instrument.symbol,
		"symbol": instrument.id, "instrumentId": instrument.id,
		"resolvedMarket": resolvedMarketForLeaf(instrument.market),
	}, nil
}

func (p *Provider) Health(ctx context.Context) (marketdata.HealthStatus, error) {
	response, err := p.client.health(ctx)
	if err != nil {
		return marketdata.HealthStatus{}, err
	}
	return marketdata.HealthStatus{
		Connected: response.OK, StreamMode: "snapshot-poll-delayed", ActiveCount: 0,
		Readiness: marketdata.ProviderReadiness(response.RuntimeState), LastError: response.WarmupError,
	}, nil
}

func (p *Provider) currentTime() time.Time {
	if p != nil && p.now != nil {
		return p.now().UTC()
	}
	return time.Now().UTC()
}
