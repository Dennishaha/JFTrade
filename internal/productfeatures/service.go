// Package productfeatures owns broker-neutral product, research, prediction,
// and customization queries. HTTP, ADK tools, and MCP all use this service.
package productfeatures

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jftrade/jftrade-main/pkg/broker"
)

var usOptionContractPattern = regexp.MustCompile(`^[A-Z][A-Z0-9.-]*\d{6}[CP]\d+$`)

var (
	ErrCapabilityUnavailable = errors.New("broker feature capability is unavailable")
	ErrPredictionIneligible  = errors.New("prediction market requires an eligible Moomoo US account")
	ErrInvalidQuery          = errors.New("invalid product feature query")
	ErrOptionChainContext    = errors.New("0DTE option chain context is required")
	ErrOptionZeroDTEMarket   = errors.New("0DTE option research is available only in the US market")
)

type Service struct {
	registry         *broker.Registry
	router           *broker.BrokerFeatureRouter
	ensure           func()
	now              func() time.Time
	predictionQuotes broker.PredictionQuoteStore

	cacheMu sync.Mutex
	cache   map[string]cacheEntry

	predictionSubscriptionMu     sync.Mutex
	predictionSubscriptionSeq    uint64
	predictionSubscriptionCounts map[string]int
	predictionSubscriptionLeases map[string]predictionSubscriptionLease
	predictionPushMu             sync.Mutex
	predictionPushCache          map[string]broker.PredictionMarketUpdate
	predictionPushUnsubscribe    map[string]func()
}

type PredictionComboQuoteRequest struct {
	BrokerID           string                  `json:"brokerId"`
	AccountID          string                  `json:"accountId"`
	TradingEnvironment string                  `json:"tradingEnvironment"`
	MVC                string                  `json:"mvc"`
	Legs               []broker.OrderLegIntent `json:"legs"`
}

type cacheEntry struct {
	expiresAt time.Time
	result    *broker.FeatureResult
}

type predictionSubscriptionLease struct {
	Key          string
	BrokerID     string
	AccountID    string
	InstrumentID string
	DataTypes    []string
	Provider     broker.ProviderAttribution
}

type PredictionSubscriptionLease struct {
	LeaseID      string                     `json:"leaseId"`
	InstrumentID string                     `json:"instrumentId"`
	DataTypes    []string                   `json:"dataTypes"`
	Provider     broker.ProviderAttribution `json:"provider"`
}

func NewService(registry *broker.Registry, defaultBroker string, fallbackOrder []string, ensure func()) *Service {
	return &Service{
		registry:                     registry,
		router:                       broker.NewBrokerFeatureRouter(registry, defaultBroker, fallbackOrder),
		ensure:                       ensure,
		now:                          time.Now,
		cache:                        make(map[string]cacheEntry),
		predictionSubscriptionCounts: make(map[string]int),
		predictionSubscriptionLeases: make(map[string]predictionSubscriptionLease),
		predictionPushCache:          make(map[string]broker.PredictionMarketUpdate),
		predictionPushUnsubscribe:    make(map[string]func()),
	}
}

func stringParam(values map[string]any, key string) string {
	if values == nil || values[key] == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(values[key]))
}

func (s *Service) staticCapabilityDescriptors() []broker.Descriptor {
	if s.ensure != nil {
		s.ensure()
	}
	descriptors := []broker.Descriptor{}
	if s.registry != nil {
		for _, value := range s.registry.All() {
			descriptors = append(descriptors, value.Descriptor())
		}
		slices.SortFunc(descriptors, func(a, b broker.Descriptor) int {
			return strings.Compare(a.ID, b.ID)
		})
	}
	return descriptors
}

func (s *Service) Query(ctx context.Context, query broker.FeatureQuery) (*broker.FeatureResult, error) {
	if s == nil || s.router == nil {
		return nil, fmt.Errorf("product feature service is unavailable")
	}
	if s.ensure != nil {
		s.ensure()
	}
	definition, err := prepareReadQuery(&query)
	if err != nil {
		return nil, err
	}
	resolution, err := s.router.ResolveContext(ctx, broker.FeatureRouteRequest{
		BrokerID:           query.BrokerID,
		AccountID:          query.AccountID,
		FeatureID:          query.FeatureID,
		Market:             query.Market,
		MarketSegment:      query.MarketSegment,
		ProductClass:       query.ProductClass,
		TradingEnvironment: query.TradingEnvironment,
	})
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrCapabilityUnavailable, err)
	}
	query.BrokerID = resolution.BrokerID
	if strings.HasPrefix(string(query.FeatureID), "prediction.") {
		securityFirm, eligibilityErr := predictionEligibility(ctx, resolution.Broker, query.AccountID)
		if eligibilityErr != nil {
			return nil, eligibilityErr
		}
		resolution.SecurityFirm = securityFirm
	}
	if pushed := s.predictionPushResult(query); pushed != nil {
		pushed.Provider = broker.ProviderAttribution{
			BrokerID:     resolution.BrokerID,
			SecurityFirm: firstNonEmpty(resolution.SecurityFirm, resolution.Broker.Descriptor().SecurityFirm),
			FeatureID:    query.FeatureID, Capability: resolution.Capability.State,
			SelectionReason: resolution.SelectionReason + ":push",
			ResolvedAt:      resolution.ResolvedAt, AsOf: pushed.AsOf,
		}
		return pushed, nil
	}

	cacheKey := queryCacheKey(query, resolution.CapabilityVersion)
	if ttl := featureTTL(query); ttl > 0 && !boolParam(query.Params, "refresh") {
		if cached := s.cached(cacheKey); cached != nil {
			return cached, nil
		}
	}
	delete(query.Params, "refresh")
	result, err := queryResolvedFeature(ctx, resolution.Broker, definition.AdapterInterface, query)
	if err != nil {
		return nil, err
	}
	if result == nil {
		result = &broker.FeatureResult{}
	}
	if result.Entries == nil {
		result.Entries = []map[string]any{}
	}
	if result.AsOf.IsZero() {
		result.AsOf = s.now().UTC()
	}
	result.Provider = broker.ProviderAttribution{
		BrokerID:        resolution.BrokerID,
		SecurityFirm:    firstNonEmpty(resolution.SecurityFirm, resolution.Broker.Descriptor().SecurityFirm),
		FeatureID:       query.FeatureID,
		Capability:      resolution.Capability.State,
		SelectionReason: resolution.SelectionReason,
		ResolvedAt:      resolution.ResolvedAt,
		AsOf:            result.AsOf,
	}
	if ttl := featureTTL(query); ttl > 0 {
		s.putCache(cacheKey, result, ttl)
	}
	return cloneResult(result), nil
}

func prepareReadQuery(query *broker.FeatureQuery) (broker.CapabilityDefinition, error) {
	definition, ok := broker.BuiltinCapabilityCatalog.Definition(query.FeatureID)
	if !ok {
		return broker.CapabilityDefinition{}, fmt.Errorf("unknown product feature %q", query.FeatureID)
	}
	if definition.Access != broker.FeatureAccessRead {
		return broker.CapabilityDefinition{}, fmt.Errorf("feature %q is not a read operation", query.FeatureID)
	}
	normalizeQuery(query)
	if err := validateOptionFeatureQuery(*query); err != nil {
		return broker.CapabilityDefinition{}, err
	}
	if err := validateResearchInstitutionQuery(*query); err != nil {
		return broker.CapabilityDefinition{}, err
	}
	return definition, nil
}

func validateResearchInstitutionQuery(query broker.FeatureQuery) error {
	if query.FeatureID != broker.FeatureResearchInstitutions {
		return nil
	}
	operation := strings.ToLower(stringParam(query.Params, "operation"))
	switch operation {
	case "profile", "distribution", "holding_changes", "holdings":
		institutionID, err := strconv.ParseInt(
			stringParam(query.Params, "institutionId"),
			10,
			32,
		)
		if err != nil || institutionID <= 0 {
			return fmt.Errorf(
				"%w: operation %s requires a positive integer institutionId",
				ErrInvalidQuery,
				operation,
			)
		}
	}
	return nil
}

func validateOptionFeatureQuery(query broker.FeatureQuery) error {
	if query.FeatureID == broker.FeatureOptionAnalysis {
		operation := strings.ToLower(stringParam(query.Params, "operation"))
		isUSOptionContract := strings.EqualFold(query.Market, "US") &&
			usOptionContractPattern.MatchString(strings.TrimPrefix(query.InstrumentID, "US."))
		switch operation {
		case "quote", "volatility", "exercise_probability":
			if strings.EqualFold(query.Market, "US") && !isUSOptionContract {
				return fmt.Errorf(
					"%w: operation %s requires a concrete option contract instrumentId",
					ErrInvalidQuery,
					operation,
				)
			}
		case "underlying_overview", "market_statistics", "historical_statistics",
			"historical_volatility":
			if isUSOptionContract {
				return fmt.Errorf(
					"%w: operation %s requires the underlying instrumentId",
					ErrInvalidQuery,
					operation,
				)
			}
		}
		return nil
	}
	if query.FeatureID != broker.FeatureOptionEvents {
		return nil
	}
	operation := strings.ToLower(stringParam(query.Params, "operation"))
	if (operation == "zero_dte" || operation == "zero_dte_contract") &&
		!strings.EqualFold(query.Market, "US") {
		return ErrOptionZeroDTEMarket
	}
	if operation == "zero_dte_contract" {
		locator, ok := query.Params["chainLocator"]
		expiryTimestamp := int64(0)
		_, expiryErr := fmt.Sscan(stringParam(query.Params, "expiryTimestamp"), &expiryTimestamp)
		if !ok || locator == nil || expiryErr != nil || expiryTimestamp <= 0 ||
			strings.TrimSpace(query.InstrumentID) == "" {
			return ErrOptionChainContext
		}
		content, err := json.Marshal(locator)
		if err != nil {
			return ErrOptionChainContext
		}
		var parsed broker.OptionZeroDteChainLocator
		if err := json.Unmarshal(content, &parsed); err != nil ||
			strings.TrimSpace(parsed.ProductCode) == "" {
			return ErrOptionChainContext
		}
		switch strings.ToLower(stringParam(query.Params, "sort")) {
		case "", "default", "volume", "open_interest", "iv", "delta":
		default:
			return fmt.Errorf("%w: unsupported 0DTE contract sort", ErrInvalidQuery)
		}
		switch strings.ToLower(stringParam(query.Params, "optionType")) {
		case "", "all", "call", "put":
		default:
			return fmt.Errorf("%w: optionType must be all, call, or put", ErrInvalidQuery)
		}
	}
	if operation == "seller" {
		switch strings.ToLower(stringParam(query.Params, "sellerStrategy")) {
		case "", "covered_call", "cash_secured_put":
		default:
			return fmt.Errorf("%w: sellerStrategy must be covered_call or cash_secured_put", ErrInvalidQuery)
		}
	}
	return nil
}

// BatchSnapshots reads a bounded group of non-streaming snapshots through the
// optional broker capability. The broker is resolved once and retained for the
// whole request; this method never creates quote subscriptions.
func (s *Service) BatchSnapshots(
	ctx context.Context,
	query broker.FeatureQuery,
	symbols []string,
) (*broker.FeatureResult, error) {
	if s == nil || s.router == nil {
		return nil, fmt.Errorf("product feature service is unavailable")
	}
	if s.ensure != nil {
		s.ensure()
	}
	if query.FeatureID != broker.FeatureMarketSnapshot {
		query.FeatureID = broker.FeatureMarketSnapshots
	}
	normalizeQuery(&query)
	normalizedSymbols, err := normalizeSnapshotSymbols(symbols)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidQuery, err)
	}
	if query.Market == "" {
		query.Market = marketFromSymbol(normalizedSymbols[0])
	}
	resolution, source, err := s.resolveBatchSnapshotSourceContext(ctx, query, normalizedSymbols)
	if err != nil {
		return nil, err
	}

	query.BrokerID = resolution.BrokerID
	cacheQuery := query
	cacheQuery.Params["symbols"] = append([]string(nil), normalizedSymbols...)
	cacheKey := queryCacheKey(cacheQuery, resolution.CapabilityVersion)
	if !boolParam(query.Params, "refresh") {
		if cached := s.cached(cacheKey); cached != nil {
			return cached, nil
		}
	}
	snapshot, err := source.QuerySecuritySnapshot(ctx, broker.SecuritySnapshotQuery{
		ReadQuery: broker.ReadQuery{
			BrokerID:  resolution.BrokerID,
			AccountID: query.AccountID,
			Market:    query.Market,
		},
		Symbols: normalizedSymbols,
	})
	if err != nil {
		return nil, err
	}
	result := s.batchSnapshotResult(query, normalizedSymbols, resolution, snapshot)
	s.putCache(cacheKey, result, 3*time.Second)
	return cloneResult(result), nil
}

func (s *Service) resolveBatchSnapshotSource(
	query broker.FeatureQuery,
	symbols []string,
) (broker.FeatureResolution, broker.BatchSnapshotSource, error) {
	return s.resolveBatchSnapshotSourceContext(context.Background(), query, symbols)
}

func (s *Service) resolveBatchSnapshotSourceContext(
	ctx context.Context,
	query broker.FeatureQuery,
	symbols []string,
) (broker.FeatureResolution, broker.BatchSnapshotSource, error) {
	resolution, err := s.router.ResolveContext(ctx, broker.FeatureRouteRequest{
		BrokerID:      query.BrokerID,
		AccountID:     query.AccountID,
		FeatureID:     query.FeatureID,
		Market:        query.Market,
		MarketSegment: query.MarketSegment,
		ProductClass:  query.ProductClass,
	})
	if err != nil {
		return broker.FeatureResolution{}, nil, fmt.Errorf("%w: %v", ErrCapabilityUnavailable, err)
	}
	// BrokerFeatureRouter verifies the catalog adapter interface before it
	// returns a resolution.
	source := resolution.Broker.(broker.BatchSnapshotSource)
	if err := validateSnapshotMarkets(resolution.Broker.Descriptor(), symbols); err != nil {
		return broker.FeatureResolution{}, nil, fmt.Errorf("%w: %v", ErrCapabilityUnavailable, err)
	}
	return resolution, source, nil
}

func (s *Service) batchSnapshotResult(
	query broker.FeatureQuery,
	symbols []string,
	resolution broker.FeatureResolution,
	snapshot *broker.SecuritySnapshotResult,
) *broker.FeatureResult {
	result := &broker.FeatureResult{
		Entries:  make([]map[string]any, 0, len(symbols)),
		AsOf:     s.now().UTC(),
		Metadata: map[string]any{"requestedSymbols": symbols, "subscriptionCreated": false},
	}
	if snapshot != nil {
		for _, item := range snapshot.Snapshots {
			// SecuritySnapshotItem is a concrete JSON-safe broker contract.
			entry, _ := jsonObject(item)
			result.Entries = append(result.Entries, entry)
			if item.ObservedAt.After(result.AsOf) {
				result.AsOf = item.ObservedAt
			}
		}
	}
	result.Provider = broker.ProviderAttribution{
		BrokerID:        resolution.BrokerID,
		SecurityFirm:    resolution.Broker.Descriptor().SecurityFirm,
		FeatureID:       query.FeatureID,
		Capability:      resolution.Capability.State,
		SelectionReason: resolution.SelectionReason,
		ResolvedAt:      resolution.ResolvedAt,
		AsOf:            result.AsOf,
	}
	return result
}

func queryResolvedFeature(
	ctx context.Context,
	selected broker.Broker,
	adapterInterface string,
	query broker.FeatureQuery,
) (*broker.FeatureResult, error) {
	switch adapterInterface {
	case "MarketDataReader":
		return queryCoreMarketDataFeature(ctx, selected, query)
	case "BatchSnapshotSource":
		return nil, fmt.Errorf("feature %q is served by the snapshot service", query.FeatureID)
	case "MarketMicrostructureReader":
		reader, ok := selected.(broker.MarketMicrostructureReader)
		if !ok {
			return nil, missingInterface(query.FeatureID, adapterInterface)
		}
		return reader.QueryMarketMicrostructure(ctx, query)
	case "InstrumentProfileReader":
		reader, ok := selected.(broker.InstrumentProfileReader)
		if !ok {
			return nil, missingInterface(query.FeatureID, adapterInterface)
		}
		return reader.QueryInstrumentProfile(ctx, query)
	case "DerivativeCatalogReader":
		reader, ok := selected.(broker.DerivativeCatalogReader)
		if !ok {
			return nil, missingInterface(query.FeatureID, adapterInterface)
		}
		return reader.QueryDerivativeCatalog(ctx, query)
	case "OptionAnalyticsReader":
		reader, ok := selected.(broker.OptionAnalyticsReader)
		if !ok {
			return nil, missingInterface(query.FeatureID, adapterInterface)
		}
		return reader.QueryOptionAnalytics(ctx, query)
	case "InstrumentResearchReader":
		reader, ok := selected.(broker.InstrumentResearchReader)
		if !ok {
			return nil, missingInterface(query.FeatureID, adapterInterface)
		}
		return reader.QueryInstrumentResearch(ctx, query)
	case "MarketResearchReader":
		reader, ok := selected.(broker.MarketResearchReader)
		if !ok {
			return nil, missingInterface(query.FeatureID, adapterInterface)
		}
		return reader.QueryMarketResearch(ctx, query)
	case "PredictionMarketReader":
		reader, ok := selected.(broker.PredictionMarketReader)
		if !ok {
			return nil, missingInterface(query.FeatureID, adapterInterface)
		}
		return reader.QueryPredictionMarket(ctx, query)
	case "TechnicalIndicatorReader":
		reader, ok := selected.(broker.TechnicalIndicatorReader)
		if !ok {
			return nil, missingInterface(query.FeatureID, adapterInterface)
		}
		return reader.QueryTechnicalIndicator(ctx, query)
	case "CustomizationService":
		reader, ok := selected.(broker.CustomizationService)
		if !ok {
			return nil, missingInterface(query.FeatureID, adapterInterface)
		}
		return reader.QueryCustomization(ctx, query)
	default:
		return nil, fmt.Errorf("feature %q has unsupported adapter interface %q", query.FeatureID, adapterInterface)
	}
}

func queryCoreMarketDataFeature(
	ctx context.Context,
	selected broker.Broker,
	query broker.FeatureQuery,
) (*broker.FeatureResult, error) {
	reader := selected.MarketData()
	if reader == nil {
		return nil, missingInterface(query.FeatureID, "MarketDataReader")
	}
	if query.FeatureID != broker.FeatureMarketCandles {
		return nil, fmt.Errorf("feature %q is not supported by the normalized core market-data bridge", query.FeatureID)
	}
	candleQuery, operation, err := normalizeCoreCandleQuery(selected, query)
	if err != nil {
		return nil, err
	}
	snapshot, err := reader.QueryKLines(ctx, candleQuery)
	if err != nil {
		return nil, err
	}
	return normalizedCoreCandleResult(query, candleQuery, operation, snapshot)
}

func normalizeCoreCandleQuery(
	selected broker.Broker,
	query broker.FeatureQuery,
) (broker.KLineQuery, string, error) {
	instrumentID := strings.ToUpper(strings.TrimSpace(query.InstrumentID))
	if instrumentID == "" {
		return broker.KLineQuery{}, "", fmt.Errorf("%w: instrumentId is required", ErrInvalidQuery)
	}
	market := query.Market
	if market == "" {
		market = marketFromSymbol(instrumentID)
	}
	if market == "" {
		return broker.KLineQuery{}, "", fmt.Errorf(
			"%w: instrumentId must use HK, US, SH, or SZ prefix",
			ErrInvalidQuery,
		)
	}
	operation := strings.ToLower(stringParam(query.Params, "operation"))
	if operation == "" {
		operation = "historical"
	}
	if operation != "current" && operation != "historical" {
		return broker.KLineQuery{}, "", fmt.Errorf(
			"%w: unsupported market.candles operation %q",
			ErrInvalidQuery,
			operation,
		)
	}
	period := stringParam(query.Params, "period")
	if period == "" {
		period = "1m"
	}
	limit := int32Param(query.Params, "limit", int32(query.PageSize))
	if limit < 1 {
		limit = 500
	}
	if limit > 500 {
		return broker.KLineQuery{}, "", fmt.Errorf(
			"%w: limit must be between 1 and 500",
			ErrInvalidQuery,
		)
	}
	beforeTime := stringParam(query.Params, "beforeTime")
	fromTime := firstNonEmpty(
		stringParam(query.Params, "startTime"),
		stringParam(query.Params, "fromTime"),
	)
	toTime := firstNonEmpty(
		stringParam(query.Params, "endTime"),
		stringParam(query.Params, "toTime"),
	)
	if beforeTime != "" && (fromTime != "" || toTime != "") {
		return broker.KLineQuery{}, "", fmt.Errorf(
			"%w: before cannot be combined with fromTime or toTime",
			ErrInvalidQuery,
		)
	}
	return broker.KLineQuery{
		ReadQuery: broker.ReadQuery{
			BrokerID: resolutionBrokerID(selected, query.BrokerID), AccountID: query.AccountID,
			TradingEnvironment: query.TradingEnvironment, Market: market,
		},
		Symbol: instrumentID, Period: period,
		FromTime: fromTime, ToTime: toTime, BeforeTime: beforeTime, Limit: limit,
	}, operation, nil
}

func normalizedCoreCandleResult(
	query broker.FeatureQuery,
	candleQuery broker.KLineQuery,
	operation string,
	snapshot *broker.KLineSnapshot,
) (*broker.FeatureResult, error) {
	entries := make([]map[string]any, 0)
	if snapshot != nil {
		entries = make([]map[string]any, 0, len(snapshot.KLines))
		for _, candle := range snapshot.KLines {
			// KLineItem is a concrete JSON-safe broker contract.
			entry, _ := jsonObject(candle)
			entries = append(entries, entry)
		}
	}
	code := candleQuery.Symbol
	if _, value, ok := strings.Cut(candleQuery.Symbol, "."); ok {
		code = value
	}
	productClass := query.ProductClass
	if productClass == "" {
		productClass = broker.ProductClassUnknown
	}
	segment := query.MarketSegment
	if segment == "" {
		segment = broker.MarketSegmentSecurities
	}
	quantityMode := broker.QuantityModeUnits
	if productClass == broker.ProductClassOption || productClass == broker.ProductClassFuture {
		quantityMode = broker.QuantityModeContracts
	}
	total := len(entries)
	hasMore := false
	nextCursor := ""
	metadata := map[string]any{"operation": operation, "period": candleQuery.Period}
	if snapshot != nil {
		hasMore = snapshot.Pagination.HasMore
		nextCursor = snapshot.Pagination.NextBefore
		metadata["extendedHours"] = snapshot.ExtendedHours
		metadata["session"] = snapshot.Session
	}
	return &broker.FeatureResult{
		ResolvedInstrument: &broker.Instrument{
			InstrumentID: candleQuery.Symbol, Code: code, ProductClass: productClass,
			MarketSegment: segment, QuoteMarket: candleQuery.Market, TradeMarket: candleQuery.Market,
			QuantityMode: quantityMode,
		},
		Entries: entries, Total: &total, HasMore: &hasMore, NextCursor: nextCursor,
		Metadata: metadata,
	}, nil
}

func int32Param(values map[string]any, key string, fallback int32) int32 {
	value := strings.TrimSpace(fmt.Sprint(values[key]))
	if value == "" || value == "<nil>" {
		return fallback
	}
	var result int32
	if _, err := fmt.Sscan(value, &result); err != nil {
		return fallback
	}
	return result
}

func resolutionBrokerID(selected broker.Broker, fallback string) string {
	if selected != nil && strings.TrimSpace(selected.ID()) != "" {
		return strings.TrimSpace(selected.ID())
	}
	return strings.TrimSpace(fallback)
}

func normalizeSnapshotSymbols(symbols []string) ([]string, error) {
	if len(symbols) == 0 {
		return nil, fmt.Errorf("at least one instrumentId is required")
	}
	if len(symbols) > 200 {
		return nil, fmt.Errorf("at most 200 instrumentIds are allowed")
	}
	result := make([]string, 0, len(symbols))
	seen := make(map[string]struct{}, len(symbols))
	for _, value := range symbols {
		symbol := strings.ToUpper(strings.TrimSpace(value))
		market := marketFromSymbol(symbol)
		if market == "" {
			return nil, fmt.Errorf("instrumentId %q must use HK, US, SH, or SZ prefix", value)
		}
		if _, ok := seen[symbol]; ok {
			continue
		}
		seen[symbol] = struct{}{}
		result = append(result, symbol)
	}
	return result, nil
}

func marketFromSymbol(symbol string) string {
	market, _, ok := strings.Cut(strings.ToUpper(strings.TrimSpace(symbol)), ".")
	if !ok {
		return ""
	}
	switch market {
	case "HK", "US", "SH", "SZ":
		return market
	default:
		return ""
	}
}

func validateSnapshotMarkets(descriptor broker.Descriptor, symbols []string) error {
	required := make(map[string]struct{}, 4)
	for _, symbol := range symbols {
		required[marketFromSymbol(symbol)] = struct{}{}
	}
	for market := range required {
		supported := false
		for _, capability := range descriptor.Capabilities {
			if !strings.EqualFold(capability.Market, market) {
				continue
			}
			for _, feature := range capability.Features {
				if feature.ID == broker.FeatureMarketSnapshots && feature.State != broker.CapabilityUnavailable {
					supported = true
					break
				}
			}
		}
		if !supported {
			return fmt.Errorf("broker %q does not support batch snapshots for market %q", descriptor.ID, market)
		}
	}
	return nil
}

func jsonObject(value any) (map[string]any, error) {
	content, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	var result map[string]any
	if err := json.Unmarshal(content, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func (s *Service) ApplyCustomization(
	ctx context.Context,
	action broker.CustomizationAction,
) (*broker.CustomizationResult, error) {
	if s == nil || s.router == nil {
		return nil, fmt.Errorf("product feature service is unavailable")
	}
	if s.ensure != nil {
		s.ensure()
	}
	definition, ok := broker.BuiltinCapabilityCatalog.Definition(action.FeatureID)
	if !ok || definition.Access != broker.FeatureAccessWrite {
		return nil, fmt.Errorf("feature %q is not an external write operation", action.FeatureID)
	}
	resolution, err := s.router.ResolveContext(ctx, broker.FeatureRouteRequest{
		BrokerID:  action.BrokerID,
		AccountID: action.AccountID,
		FeatureID: action.FeatureID,
	})
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrCapabilityUnavailable, err)
	}
	// BrokerFeatureRouter verifies the catalog adapter interface before it
	// returns a resolution.
	service := resolution.Broker.(broker.CustomizationService)
	action.BrokerID = resolution.BrokerID
	result, err := service.ApplyCustomization(ctx, action)
	if err != nil {
		return nil, err
	}
	if result == nil {
		result = &broker.CustomizationResult{}
	}
	result.Provider = broker.ProviderAttribution{
		BrokerID:        resolution.BrokerID,
		SecurityFirm:    resolution.Broker.Descriptor().SecurityFirm,
		FeatureID:       action.FeatureID,
		Capability:      resolution.Capability.State,
		SelectionReason: resolution.SelectionReason,
		ResolvedAt:      resolution.ResolvedAt,
		AsOf:            s.now().UTC(),
	}
	return result, nil
}
