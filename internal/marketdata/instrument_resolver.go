package marketdata

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/jftrade/jftrade-main/pkg/market"
	"golang.org/x/sync/singleflight"
)

const (
	defaultInstrumentSearchLimit = 20
	maxInstrumentSearchLimit     = 100
	instrumentSearchCacheTTL     = 30 * time.Second
	instrumentSearchTimeout      = 15 * time.Second
)

// InstrumentResolutionStatus describes whether a lookup can be selected
// automatically or needs user intervention.
type InstrumentResolutionStatus string

const (
	InstrumentResolutionResolved    InstrumentResolutionStatus = "resolved"
	InstrumentResolutionAmbiguous   InstrumentResolutionStatus = "ambiguous"
	InstrumentResolutionNotFound    InstrumentResolutionStatus = "not_found"
	InstrumentResolutionIncomplete  InstrumentResolutionStatus = "incomplete"
	InstrumentResolutionUnavailable InstrumentResolutionStatus = "unavailable"
)

// InstrumentCandidate is the broker-neutral result of an instrument lookup.
// Market is the actual routing market. Only HK/US/SH/SZ candidates may enter
// downstream trading, backtest, or workspace state.
type InstrumentCandidate struct {
	Market            string `json:"market"`
	ResolvedMarket    string `json:"resolvedMarket"`
	InstrumentID      string `json:"instrumentId"`
	Code              string `json:"code"`
	Symbol            string `json:"symbol"`
	Name              string `json:"name,omitempty"`
	SecurityType      string `json:"securityType,omitempty"`
	LotSize           int32  `json:"lotSize,omitempty"`
	Source            string `json:"source,omitempty"`
	IsWatched         bool   `json:"isWatched,omitempty"`
	Selectable        bool   `json:"selectable"`
	UnavailableReason string `json:"unavailableReason,omitempty"`
}

// InstrumentResolutionFailure preserves a failed leaf lookup without hiding
// successful candidates returned by other leaves.
type InstrumentResolutionFailure struct {
	Market  string `json:"market"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

// InstrumentResolution is the transport-neutral response used by the
// instrument resolver and the HTTP API.
type InstrumentResolution struct {
	Query            string                        `json:"query"`
	RequestedMarket  string                        `json:"requestedMarket"`
	ResolutionStatus InstrumentResolutionStatus    `json:"resolutionStatus"`
	TotalReturned    int                           `json:"totalReturned"`
	Entries          []InstrumentCandidate         `json:"entries"`
	Failures         []InstrumentResolutionFailure `json:"failures"`
}

// InstrumentSearchInputError identifies caller input errors so the transport
// can distinguish them from OpenD/provider failures.
type InstrumentSearchInputError struct {
	message string
}

func (e *InstrumentSearchInputError) Error() string { return e.message }

func instrumentSearchInputErrorf(format string, args ...any) error {
	return &InstrumentSearchInputError{message: fmt.Sprintf(format, args...)}
}

func IsInstrumentSearchInputError(err error) bool {
	var target *InstrumentSearchInputError
	return errors.As(err, &target)
}

// ExactInstrumentLookupProvider supplies leaf-market static security info.
// Implementations must not create quote subscriptions as a side effect.
type ExactInstrumentLookupProvider interface {
	LookupInstrument(ctx context.Context, market, code string) ([]InstrumentCandidate, error)
}

// InstrumentSearchProvider supplies ranked cross-market matches. The resolver
// always asks for the OpenD maximum and applies market filtering and limits
// after preserving provider relevance order.
type InstrumentSearchProvider interface {
	SearchInstruments(ctx context.Context, query string, limit int) ([]InstrumentCandidate, error)
}

type cachedInstrumentSearch struct {
	expiresAt time.Time
	entries   []InstrumentCandidate
}

// MarketSubsetInstrumentResolver keeps qualified exact lookups while routing
// every unqualified code or name through the broker's cross-market search.
type MarketSubsetInstrumentResolver struct {
	provider       ExactInstrumentLookupProvider
	searchProvider InstrumentSearchProvider
	cacheMu        sync.Mutex
	searchCache    map[string]cachedInstrumentSearch
	searchGroup    singleflight.Group
	now            func() time.Time
}

type instrumentLeafLookupResult struct {
	index      int
	market     string
	candidates []InstrumentCandidate
	err        error
}

func NewMarketSubsetInstrumentResolver(provider ExactInstrumentLookupProvider) *MarketSubsetInstrumentResolver {
	searchProvider, _ := provider.(InstrumentSearchProvider)
	return &MarketSubsetInstrumentResolver{
		provider:       provider,
		searchProvider: searchProvider,
		searchCache:    make(map[string]cachedInstrumentSearch),
		now:            time.Now,
	}
}

// Resolve performs a qualified exact lookup or an unqualified cross-market
// search. limit defaults to 20 and is constrained to OpenD's 1..100 range.
func (r *MarketSubsetInstrumentResolver) Resolve(ctx context.Context, requestedMarket, query string, limit int) (InstrumentResolution, error) {
	trimmedQuery := strings.TrimSpace(query)
	result := InstrumentResolution{
		Query:            trimmedQuery,
		RequestedMarket:  strings.ToUpper(strings.TrimSpace(requestedMarket)),
		ResolutionStatus: InstrumentResolutionNotFound,
		Entries:          []InstrumentCandidate{},
		Failures:         []InstrumentResolutionFailure{},
	}
	if trimmedQuery == "" {
		return InstrumentResolution{}, instrumentSearchInputErrorf("query is required")
	}
	if limit == 0 {
		limit = defaultInstrumentSearchLimit
	}
	if limit < 1 || limit > maxInstrumentSearchLimit {
		return InstrumentResolution{}, instrumentSearchInputErrorf("limit must be between 1 and %d", maxInstrumentSearchLimit)
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return InstrumentResolution{}, err
	}
	if r == nil || r.provider == nil {
		return InstrumentResolution{}, fmt.Errorf("market-data instrument lookup provider is unavailable")
	}

	normalizedQuery := strings.ToUpper(strings.ReplaceAll(trimmedQuery, ":", "."))
	if hasRecognizedMarketPrefix(normalizedQuery) {
		return r.resolveQualified(ctx, result, normalizedQuery)
	}
	return r.resolveSearch(ctx, result, trimmedQuery, normalizedQuery, limit)
}

func (r *MarketSubsetInstrumentResolver) resolveQualified(ctx context.Context, result InstrumentResolution, query string) (InstrumentResolution, error) {
	leaves, code, err := resolutionLookupTargets(result.RequestedMarket, query)
	if err != nil {
		return InstrumentResolution{}, instrumentSearchInputErrorf("%v", err)
	}
	entries, failures, err := r.lookupLeaves(ctx, leaves, code)
	if err != nil {
		return InstrumentResolution{}, err
	}
	result.Entries = entries
	result.Failures = failures
	result.TotalReturned = len(entries)
	result.ResolutionStatus = classifyInstrumentResolution(entries, len(failures))
	return result, nil
}

func (r *MarketSubsetInstrumentResolver) resolveSearch(ctx context.Context, result InstrumentResolution, query, normalizedQuery string, limit int) (InstrumentResolution, error) {
	filter, err := normalizeInstrumentSearchMarket(result.RequestedMarket)
	if err != nil {
		return InstrumentResolution{}, err
	}
	result.RequestedMarket = filter
	if r.searchProvider == nil {
		return InstrumentResolution{}, fmt.Errorf("market-data instrument search provider is unavailable")
	}

	searched, err := r.search(ctx, query)
	if err != nil {
		return InstrumentResolution{}, err
	}
	entries := filterAndNormalizeSearchCandidates(searched, filter)
	if exact := exactCodeMatches(entries, normalizedQuery); len(exact) > 0 {
		entries = exact
	}
	allEntries := entries
	if len(entries) > limit {
		entries = entries[:limit]
	}
	result.Entries = entries
	result.TotalReturned = len(entries)
	result.ResolutionStatus = classifyInstrumentResolution(allEntries, 0)
	return result, nil
}

func (r *MarketSubsetInstrumentResolver) search(ctx context.Context, query string) ([]InstrumentCandidate, error) {
	key := strings.ToUpper(strings.TrimSpace(query))
	if cached, ok := r.cachedSearch(key); ok {
		return cached, nil
	}

	resultCh := r.searchGroup.DoChan(key, func() (any, error) {
		sharedCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), instrumentSearchTimeout)
		defer cancel()
		entries, err := r.searchProvider.SearchInstruments(sharedCtx, strings.TrimSpace(query), maxInstrumentSearchLimit)
		if err != nil {
			return nil, err
		}
		entries = append([]InstrumentCandidate(nil), entries...)
		now := r.now()
		r.cacheMu.Lock()
		for cachedKey, cached := range r.searchCache {
			if !now.Before(cached.expiresAt) {
				delete(r.searchCache, cachedKey)
			}
		}
		r.searchCache[key] = cachedInstrumentSearch{
			expiresAt: now.Add(instrumentSearchCacheTTL),
			entries:   entries,
		}
		r.cacheMu.Unlock()
		return entries, nil
	})
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case searchResult := <-resultCh:
		if searchResult.Err != nil {
			return nil, searchResult.Err
		}
		entries := searchResult.Val.([]InstrumentCandidate)
		return append([]InstrumentCandidate(nil), entries...), nil
	}
}

func (r *MarketSubsetInstrumentResolver) cachedSearch(key string) ([]InstrumentCandidate, bool) {
	r.cacheMu.Lock()
	defer r.cacheMu.Unlock()
	cached, ok := r.searchCache[key]
	if !ok {
		return nil, false
	}
	if !r.now().Before(cached.expiresAt) {
		delete(r.searchCache, key)
		return nil, false
	}
	return append([]InstrumentCandidate(nil), cached.entries...), true
}

func (r *MarketSubsetInstrumentResolver) lookupLeaves(ctx context.Context, leaves []string, code string) ([]InstrumentCandidate, []InstrumentResolutionFailure, error) {
	responses := make(chan instrumentLeafLookupResult, len(leaves))
	for index, leaf := range leaves {
		go func() {
			candidates, lookupErr := r.provider.LookupInstrument(ctx, leaf, code)
			responses <- instrumentLeafLookupResult{index: index, market: leaf, candidates: candidates, err: lookupErr}
		}()
	}

	slots := make([]instrumentLeafLookupResult, len(leaves))
	for range leaves {
		select {
		case <-ctx.Done():
			return nil, nil, ctx.Err()
		case response := <-responses:
			slots[response.index] = response
		}
	}
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}

	entries := make([]InstrumentCandidate, 0, len(leaves))
	failures := make([]InstrumentResolutionFailure, 0)
	seen := make(map[string]struct{})
	for _, response := range slots {
		if response.err != nil {
			failures = append(failures, InstrumentResolutionFailure{
				Market:  response.market,
				Code:    code,
				Message: response.err.Error(),
			})
			continue
		}
		for _, candidate := range response.candidates {
			normalized, ok := normalizeExactInstrumentCandidate(candidate, response.market, code)
			if !ok {
				continue
			}
			key := strings.ToUpper(normalized.InstrumentID)
			if _, exists := seen[key]; exists {
				continue
			}
			seen[key] = struct{}{}
			entries = append(entries, normalized)
		}
	}
	return entries, failures, nil
}

func resolutionLookupTargets(requestedMarket, query string) ([]string, string, error) {
	instrument, err := market.ParseInstrument(market.InstrumentInput{
		Market:       requestedMarket,
		InstrumentID: query,
	})
	if err != nil {
		return nil, "", err
	}
	return []string{instrument.Prefix}, instrument.Code, nil
}

func normalizeExactInstrumentCandidate(candidate InstrumentCandidate, leafMarket, code string) (InstrumentCandidate, bool) {
	instrumentID := strings.ToUpper(strings.TrimSpace(candidate.InstrumentID))
	if instrumentID == "" {
		candidateCode := strings.ToUpper(strings.TrimSpace(candidate.Code))
		if candidateCode == "" {
			candidateCode = strings.ToUpper(strings.TrimSpace(candidate.Symbol))
		}
		if hasRecognizedMarketPrefix(strings.ReplaceAll(candidateCode, ":", ".")) {
			instrumentID = strings.ReplaceAll(candidateCode, ":", ".")
		} else {
			if candidateCode == "" {
				candidateCode = code
			}
			instrumentID = leafMarket + "." + candidateCode
		}
	}
	instrument, err := market.ParseQualifiedInstrumentSymbol(instrumentID)
	if err != nil || instrument.Prefix != leafMarket || !strings.EqualFold(instrument.Code, code) {
		return InstrumentCandidate{}, false
	}
	candidate.Market = instrument.Prefix
	candidate.ResolvedMarket = instrument.Market
	candidate.InstrumentID = instrument.Symbol
	candidate.Code = instrument.Code
	candidate.Symbol = instrument.Code
	normalizeCandidateText(&candidate)
	applyInstrumentAvailability(&candidate)
	return candidate, true
}

func filterAndNormalizeSearchCandidates(candidates []InstrumentCandidate, filter string) []InstrumentCandidate {
	entries := make([]InstrumentCandidate, 0, len(candidates))
	seen := make(map[string]struct{}, len(candidates))
	for _, candidate := range candidates {
		normalized, ok := normalizeSearchInstrumentCandidate(candidate)
		if !ok || !instrumentSearchMarketMatches(filter, normalized.Market) {
			continue
		}
		key := strings.ToUpper(normalized.InstrumentID)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		entries = append(entries, normalized)
	}
	return entries
}

func normalizeSearchInstrumentCandidate(candidate InstrumentCandidate) (InstrumentCandidate, bool) {
	marketCode := stableSearchMarketCode(candidate.Market)
	instrumentID := strings.ToUpper(strings.TrimSpace(candidate.InstrumentID))
	if instrumentID == "" {
		instrumentID = strings.ToUpper(strings.TrimSpace(candidate.Symbol))
	}
	if prefix, bareCode, ok := splitStableSearchInstrument(instrumentID); ok {
		if marketCode == "UNKNOWN" || marketCode == "" {
			marketCode = prefix
		}
		if prefix == marketCode {
			instrumentID = bareCode
		}
	}
	code := strings.ToUpper(strings.TrimSpace(candidate.Code))
	if code == "" {
		code = strings.ToUpper(strings.TrimSpace(instrumentID))
	}
	if prefix, bareCode, ok := splitStableSearchInstrument(code); ok && prefix == marketCode {
		code = bareCode
	}
	if code == "" {
		return InstrumentCandidate{}, false
	}
	candidate.Market = marketCode
	candidate.ResolvedMarket = resolvedSearchMarket(marketCode)
	candidate.InstrumentID = marketCode + "." + code
	candidate.Code = code
	candidate.Symbol = code
	normalizeCandidateText(&candidate)
	applyInstrumentAvailability(&candidate)
	return candidate, true
}

func splitStableSearchInstrument(value string) (string, string, bool) {
	separator := strings.Index(value, ".")
	if separator <= 0 || separator == len(value)-1 {
		return "", "", false
	}
	rawPrefix := strings.TrimSpace(value[:separator])
	prefix := stableSearchMarketCode(rawPrefix)
	if prefix == "UNKNOWN" && !strings.EqualFold(rawPrefix, "UNKNOWN") {
		return "", "", false
	}
	code := strings.TrimSpace(value[separator+1:])
	if code == "" {
		return "", "", false
	}
	return prefix, code, true
}

func normalizeCandidateText(candidate *InstrumentCandidate) {
	candidate.Name = strings.TrimSpace(candidate.Name)
	candidate.SecurityType = strings.TrimSpace(candidate.SecurityType)
	candidate.Source = strings.TrimSpace(candidate.Source)
	candidate.UnavailableReason = strings.TrimSpace(candidate.UnavailableReason)
}

func stableSearchMarketCode(value string) string {
	normalized := strings.ToUpper(strings.TrimSpace(value))
	normalized = strings.ReplaceAll(normalized, "-", "_")
	switch normalized {
	case "CNSH":
		return "SH"
	case "CNSZ":
		return "SZ"
	case "HKFUTURE", "HK_FUTURES", "HK_FUTURE":
		return "HK_FUTURE"
	case "CC", "CRYPTO":
		return "CRYPTO"
	case "", "UNKNOWN":
		return "UNKNOWN"
	case "HK", "US", "SH", "SZ", "SG", "JP", "AU", "MY", "CA", "FX":
		return normalized
	default:
		return "UNKNOWN"
	}
}

func resolvedSearchMarket(marketCode string) string {
	if marketCode == "SH" || marketCode == "SZ" {
		return "CN"
	}
	return marketCode
}

func applyInstrumentAvailability(candidate *InstrumentCandidate) {
	candidate.Selectable = isSelectableInstrumentMarket(candidate.Market)
	if candidate.Selectable {
		candidate.UnavailableReason = ""
		return
	}
	if candidate.UnavailableReason == "" {
		candidate.UnavailableReason = fmt.Sprintf("当前版本暂不支持 %s 市场", candidate.Market)
	}
}

func isSelectableInstrumentMarket(marketCode string) bool {
	switch strings.ToUpper(strings.TrimSpace(marketCode)) {
	case "HK", "US", "SH", "SZ":
		return true
	default:
		return false
	}
}

func normalizeInstrumentSearchMarket(value string) (string, error) {
	normalized := strings.ToUpper(strings.TrimSpace(value))
	if normalized == "" {
		return "", nil
	}
	resolved, preferred, err := market.NormalizeMarketInput(normalized)
	if err != nil {
		return "", instrumentSearchInputErrorf("%v", err)
	}
	if preferred == "SH" || preferred == "SZ" {
		return preferred, nil
	}
	if resolved == "HK" || resolved == "US" || resolved == "CN" {
		return resolved, nil
	}
	return "", instrumentSearchInputErrorf("market filter must be HK, US, CN, SH, or SZ")
}

func instrumentSearchMarketMatches(filter, marketCode string) bool {
	if filter == "" {
		return true
	}
	if filter == "CN" {
		return marketCode == "SH" || marketCode == "SZ"
	}
	return filter == marketCode
}

func exactCodeMatches(entries []InstrumentCandidate, normalizedQuery string) []InstrumentCandidate {
	query := strings.ToUpper(strings.TrimSpace(normalizedQuery))
	matches := make([]InstrumentCandidate, 0)
	for _, candidate := range entries {
		if strings.EqualFold(candidate.Code, query) {
			matches = append(matches, candidate)
		}
	}
	return matches
}

func hasRecognizedMarketPrefix(value string) bool {
	separator := strings.Index(value, ".")
	if separator <= 0 {
		return false
	}
	_, _, err := market.NormalizeMarketInput(value[:separator])
	return err == nil
}

func classifyInstrumentResolution(candidates []InstrumentCandidate, failureCount int) InstrumentResolutionStatus {
	if len(candidates) > 0 {
		selectable := false
		for _, candidate := range candidates {
			if candidate.Selectable {
				selectable = true
				break
			}
		}
		if !selectable {
			return InstrumentResolutionUnavailable
		}
	}
	if len(candidates) > 1 {
		return InstrumentResolutionAmbiguous
	}
	if failureCount > 0 {
		return InstrumentResolutionIncomplete
	}
	if len(candidates) == 0 {
		return InstrumentResolutionNotFound
	}
	return InstrumentResolutionResolved
}
