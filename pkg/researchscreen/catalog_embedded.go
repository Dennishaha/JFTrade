package researchscreen

import (
	"fmt"
	"slices"
	"strings"
)

// EmbeddedCatalogVersion identifies the hand-written factor catalog served by
// the embedded market-data providers (yfinance/akshare). Factor keys reuse the
// Futu naming so presets and result cells stay comparable across providers;
// the two simple.* keys without a Futu counterpart (change_pct, volume) are
// catalog additions, not renames.
const EmbeddedCatalogVersion = "embedded-stock-screen-v1"

// embeddedScreenMarkets lists the markets each embedded provider screens.
// yfinance covers US only; akshare covers the CN aggregate, its SH/SZ leaves,
// HK, and US (US rows come from the Eastmoney clist feed with PB/PE TTM
// filled in by the sidecar).
var embeddedScreenMarkets = map[string][]string{
	"yfinance": {"US"},
	"akshare":  {"SH", "SZ", "CN", "HK", "US"},
}

// embeddedFactors is the provider intersection both sidecars can compute.
// Filter semantics are interval-only (operator between); basic.* identity
// factors are retrieve/sort columns, never filters. basic.name sorts on the
// provider intersection: akshare sorts locally by name, but Yahoo has no
// name sortField, so the shared catalog keeps name retrieve-only.
var embeddedFactors = []FactorDescriptor{
	{
		Key: "basic.code", Category: "basic", Label: "代码", ValueType: "string",
		Retrieve: true, Sort: true,
	},
	{
		Key: "basic.name", Category: "basic", Label: "名称", ValueType: "string",
		Retrieve: true,
	},
	{
		Key: "basic.industry", Category: "basic", Label: "所属行业", ValueType: "string",
		Retrieve: true,
	},
	{
		Key: "simple.price", Category: "simple", Label: "最新价", ValueType: "number",
		Unit: "currency", DisplayFormat: "price", CurrencyBasis: "quote",
		Filter: true, Retrieve: true, Sort: true,
	},
	{
		Key: "simple.change_pct", Category: "simple", Label: "当日涨跌幅", ValueType: "number",
		Unit: "percent", DisplayFormat: "percent",
		Filter: true, Retrieve: true, Sort: true,
	},
	{
		Key: "simple.volume", Category: "simple", Label: "当日成交量", ValueType: "integer",
		Unit: "shares", DisplayFormat: "integer",
		Filter: true, Retrieve: true, Sort: true,
	},
	{
		Key: "simple.market_cap", Category: "simple", Label: "总市值", ValueType: "number",
		Unit: "currency", DisplayFormat: "compact_amount", CurrencyBasis: "quote",
		Filter: true, Retrieve: true, Sort: true,
	},
	{
		Key: "simple.pe_ttm", Category: "simple", Label: "市盈率(TTM)", ValueType: "number",
		DisplayFormat: "number",
		Filter:        true, Retrieve: true, Sort: true,
	},
	{
		Key: "simple.pb", Category: "simple", Label: "市净率", ValueType: "number",
		DisplayFormat: "number",
		Filter:        true, Retrieve: true, Sort: true,
	},
}

var embeddedFactorByKey = func() map[string]FactorDescriptor {
	result := make(map[string]FactorDescriptor, len(embeddedFactors))
	for index, factor := range embeddedFactors {
		factor.Availability = "available"
		roles := []string{}
		if factor.Filter {
			factor.FilterKind = "interval"
			factor.ConditionEditor = "range"
			factor.Operators = []string{"between"}
			roles = append(roles, "condition")
		}
		if factor.Retrieve {
			roles = append(roles, "column")
		}
		if factor.Sort {
			roles = append(roles, "sort")
		}
		factor.Roles = roles
		if factor.Help == "" {
			factor.Help = factor.Label
		}
		factor.SearchKeywords = factorSearchKeywords(factor)
		embeddedFactors[index] = factor
		result[factor.Key] = factor
	}
	return result
}()

// IsEmbeddedCatalogVersion reports whether a catalog version names the
// embedded provider catalog instead of the Futu generated catalog.
func IsEmbeddedCatalogVersion(version string) bool {
	return strings.EqualFold(strings.TrimSpace(version), EmbeddedCatalogVersion)
}

// EmbeddedScreenMarkets returns the markets an embedded provider screens;
// unknown providers get no markets.
func EmbeddedScreenMarkets(brokerID string) []string {
	markets, ok := embeddedScreenMarkets[strings.ToLower(strings.TrimSpace(brokerID))]
	if !ok {
		return nil
	}
	return append([]string(nil), markets...)
}

// EmbeddedCatalog builds the catalog document for one embedded provider. An
// empty market returns the unfiltered catalog; a market outside the provider's
// coverage is rejected by the caller before this is invoked.
func EmbeddedCatalog(brokerID, market string) Catalog {
	brokerID = strings.ToLower(strings.TrimSpace(brokerID))
	market = strings.ToUpper(strings.TrimSpace(market))
	factors := append([]FactorDescriptor(nil), embeddedFactors...)
	counts := make(map[string]int)
	for _, factor := range factors {
		counts[factor.Category]++
	}
	categories := make([]Category, 0, len(counts))
	for key, count := range counts {
		categories = append(categories, Category{Key: key, Label: categoryLabels[key], Count: count})
	}
	slices.SortFunc(categories, func(a, b Category) int {
		return strings.Compare(a.Key, b.Key)
	})
	return Catalog{
		Version:            EmbeddedCatalogVersion,
		SchemaVersion:      CatalogSchemaVersion,
		QuerySchemaVersion: QuerySchemaVersion,
		Provider:           brokerID,
		Market:             market,
		Markets:            EmbeddedScreenMarkets(brokerID),
		Categories:         categories,
		Factors:            factors,
		Enums:              map[string][]EnumOption{},
		RateLimit:          RateLimit{Requests: 10, Window: 30},
	}
}

// LookupEmbedded resolves one factor from the embedded catalog.
func LookupEmbedded(key string) (FactorDescriptor, bool) {
	factor, ok := embeddedFactorByKey[strings.ToLower(strings.TrimSpace(key))]
	return factor, ok
}

// ValidateEmbeddedFactorUse mirrors ValidateFactorUse for the embedded
// catalog; market coverage is broker-level, so no per-factor market check
// applies here.
func ValidateEmbeddedFactorUse(key string, filter, retrieve, sort bool) (FactorDescriptor, error) {
	factor, ok := LookupEmbedded(key)
	if !ok {
		return FactorDescriptor{}, fmt.Errorf("unknown embedded research screen factor %q", key)
	}
	switch {
	case filter && !factor.Filter:
		return FactorDescriptor{}, fmt.Errorf("embedded research screen factor %q cannot be filtered", key)
	case retrieve && !factor.Retrieve:
		return FactorDescriptor{}, fmt.Errorf("embedded research screen factor %q cannot be retrieved", key)
	case sort && !factor.Sort:
		return FactorDescriptor{}, fmt.Errorf("embedded research screen factor %q cannot be sorted", key)
	default:
		return factor, nil
	}
}
