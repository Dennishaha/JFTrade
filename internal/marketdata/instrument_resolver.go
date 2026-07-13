package marketdata

import (
	"context"
	"fmt"
	"strings"

	"github.com/jftrade/jftrade-main/pkg/market"
)

// InstrumentResolutionStatus describes whether an exact-code lookup can be
// selected automatically or needs user intervention.
type InstrumentResolutionStatus string

const (
	InstrumentResolutionResolved   InstrumentResolutionStatus = "resolved"
	InstrumentResolutionAmbiguous  InstrumentResolutionStatus = "ambiguous"
	InstrumentResolutionNotFound   InstrumentResolutionStatus = "not_found"
	InstrumentResolutionIncomplete InstrumentResolutionStatus = "incomplete"
)

// InstrumentCandidate is the broker-neutral, user-selectable result of an
// exact security-code lookup. Market is always the actual routing leaf (for
// example SH/SZ); ResolvedMarket is its user-facing category (CN).
type InstrumentCandidate struct {
	Market         string `json:"market"`
	ResolvedMarket string `json:"resolvedMarket"`
	InstrumentID   string `json:"instrumentId"`
	Code           string `json:"code"`
	Symbol         string `json:"symbol"`
	Name           string `json:"name,omitempty"`
	SecurityType   string `json:"securityType,omitempty"`
	LotSize        int32  `json:"lotSize,omitempty"`
	Source         string `json:"source,omitempty"`
}

// InstrumentResolutionFailure preserves a failed leaf lookup without hiding
// successful candidates returned by other leaves.
type InstrumentResolutionFailure struct {
	Market  string `json:"market"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

// InstrumentResolution is the transport-neutral response used by the exact
// instrument resolver and the HTTP API.
type InstrumentResolution struct {
	Query            string                        `json:"query"`
	RequestedMarket  string                        `json:"requestedMarket"`
	ResolutionStatus InstrumentResolutionStatus    `json:"resolutionStatus"`
	TotalReturned    int                           `json:"totalReturned"`
	Entries          []InstrumentCandidate         `json:"entries"`
	Failures         []InstrumentResolutionFailure `json:"failures"`
}

// ExactInstrumentLookupProvider supplies leaf-market static security info.
// Implementations must not create quote subscriptions as a side effect.
type ExactInstrumentLookupProvider interface {
	LookupInstrument(ctx context.Context, market, code string) ([]InstrumentCandidate, error)
}

// MarketSubsetInstrumentResolver expands top-level market categories into
// stable leaf lookups while preserving partial results and failures.
type MarketSubsetInstrumentResolver struct {
	provider ExactInstrumentLookupProvider
}

type instrumentLeafLookupResult struct {
	index      int
	market     string
	candidates []InstrumentCandidate
	err        error
}

func NewMarketSubsetInstrumentResolver(provider ExactInstrumentLookupProvider) *MarketSubsetInstrumentResolver {
	return &MarketSubsetInstrumentResolver{provider: provider}
}

// Resolve performs an exact code lookup. Qualified input bypasses parent
// expansion; bare input under CN expands to SH and SZ concurrently.
func (r *MarketSubsetInstrumentResolver) Resolve(ctx context.Context, requestedMarket, query string) (InstrumentResolution, error) {
	result := InstrumentResolution{
		Query:            query,
		RequestedMarket:  strings.ToUpper(strings.TrimSpace(requestedMarket)),
		ResolutionStatus: InstrumentResolutionNotFound,
		Entries:          []InstrumentCandidate{},
		Failures:         []InstrumentResolutionFailure{},
	}
	normalizedQuery := strings.ToUpper(strings.TrimSpace(query))
	if normalizedQuery == "" {
		return result, nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return InstrumentResolution{}, err
	}

	leaves, code, err := resolutionLookupTargets(result.RequestedMarket, normalizedQuery)
	if err != nil {
		return InstrumentResolution{}, err
	}
	if r == nil || r.provider == nil {
		return InstrumentResolution{}, fmt.Errorf("market-data instrument lookup provider is unavailable")
	}
	entries, failures, err := r.lookupLeaves(ctx, leaves, code)
	if err != nil {
		return InstrumentResolution{}, err
	}
	result.Entries = entries
	result.Failures = failures
	result.TotalReturned = len(entries)
	result.ResolutionStatus = classifyInstrumentResolution(len(entries), len(failures))
	return result, nil
}

func (r *MarketSubsetInstrumentResolver) lookupLeaves(ctx context.Context, leaves []string, code string) ([]InstrumentCandidate, []InstrumentResolutionFailure, error) {
	responses := make(chan instrumentLeafLookupResult, len(leaves))
	for index, leaf := range leaves {
		index, leaf := index, leaf
		go func() {
			candidates, lookupErr := r.provider.LookupInstrument(ctx, leaf, code)
			responses <- instrumentLeafLookupResult{index: index, market: leaf, candidates: candidates, err: lookupErr}
		}()
	}

	slots := make([]instrumentLeafLookupResult, len(leaves))
	for received := 0; received < len(leaves); received++ {
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
			normalized, ok := normalizeInstrumentCandidate(candidate, response.market, code)
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
	qualified := strings.ReplaceAll(query, ":", ".")
	if hasRecognizedMarketPrefix(qualified) {
		instrument, err := market.ParseInstrument(market.InstrumentInput{
			Market:       requestedMarket,
			InstrumentID: qualified,
		})
		if err != nil {
			return nil, "", err
		}
		return []string{instrument.Prefix}, instrument.Code, nil
	}
	if requestedMarket == "" {
		return nil, "", fmt.Errorf("market is required when query has no market prefix")
	}
	resolvedMarket, preferredPrefix, err := market.NormalizeMarketInput(requestedMarket)
	if err != nil {
		return nil, "", err
	}
	if preferredPrefix != "" {
		return []string{preferredPrefix}, query, nil
	}
	children := market.MarketSubsetChildren(resolvedMarket)
	if len(children) == 0 {
		return nil, "", fmt.Errorf("market %q has no resolvable leaf market", requestedMarket)
	}
	return children, query, nil
}

func normalizeInstrumentCandidate(candidate InstrumentCandidate, leafMarket, code string) (InstrumentCandidate, bool) {
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
	candidate.Name = strings.TrimSpace(candidate.Name)
	candidate.SecurityType = strings.TrimSpace(candidate.SecurityType)
	candidate.Source = strings.TrimSpace(candidate.Source)
	return candidate, true
}

func hasRecognizedMarketPrefix(value string) bool {
	separator := strings.Index(value, ".")
	if separator <= 0 {
		return false
	}
	_, _, err := market.NormalizeMarketInput(value[:separator])
	return err == nil
}

func classifyInstrumentResolution(candidateCount, failureCount int) InstrumentResolutionStatus {
	if candidateCount > 1 {
		return InstrumentResolutionAmbiguous
	}
	if failureCount > 0 {
		return InstrumentResolutionIncomplete
	}
	if candidateCount == 1 {
		return InstrumentResolutionResolved
	}
	return InstrumentResolutionNotFound
}
