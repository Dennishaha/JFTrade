package backtestapp

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jftrade/jftrade-main/internal/app/apiserver/marketdataapp"
	backteststore "github.com/jftrade/jftrade-main/pkg/backtest"
	marketpkg "github.com/jftrade/jftrade-main/pkg/market"
)

const instrumentRuleLookupTimeout = 15 * time.Second

func ResolveInstrumentSpec(
	ctx context.Context,
	runtime *marketdataapp.Runtime,
	providerID, _ string,
	symbol string,
) (backteststore.InstrumentSpec, error) {
	spec := defaultInstrumentSpec(symbol)
	lease, err := runtime.AcquireProvider(ctx, providerID, instrumentRulesRequireReady(providerID))
	if err != nil {
		spec.Warnings = append(spec.Warnings, fmt.Sprintf("provider %s market rules unavailable: %v", providerID, err))
		return spec, nil
	}
	defer lease.Release()
	lookupCtx, cancel := context.WithTimeout(ctx, instrumentRuleLookupTimeout)
	defer cancel()
	details, err := lease.Provider().GetSecurityDetails(lookupCtx, marketpkg.SymbolMarket(symbol), symbol)
	if err != nil {
		spec.Warnings = append(spec.Warnings, fmt.Sprintf("provider %s instrument rules unavailable: %v", providerID, err))
		return conservativeInstrumentSpec(spec), nil
	}
	security := details
	if nested, ok := details["security"].(map[string]any); ok {
		security = nested
	}
	if currency, ok := security["currency"].(string); ok && strings.TrimSpace(currency) != "" {
		spec.QuoteCurrency = strings.ToUpper(strings.TrimSpace(currency))
	}
	if lotSize, ok := positiveFloat(security["lotSize"]); ok {
		spec.LotSize = lotSize
		spec.QuantityStep = lotSize
		spec.MissingCriticalRules = false
		spec.Warnings = nil
	}
	if tickSize, ok := positiveFloat(security["priceSpread"]); ok {
		spec.TickSize = tickSize
	}
	return conservativeInstrumentSpec(spec), nil
}

func instrumentRulesRequireReady(providerID string) bool {
	switch strings.ToLower(strings.TrimSpace(providerID)) {
	case marketdataapp.ProviderYFinance, marketdataapp.ProviderAKShare:
		return true
	default:
		return false
	}
}

func NewInstrumentSpecResolver(
	runtime *marketdataapp.Runtime,
) func(context.Context, string, string, string) (backteststore.InstrumentSpec, error) {
	return func(ctx context.Context, providerID, market, symbol string) (backteststore.InstrumentSpec, error) {
		return ResolveInstrumentSpec(ctx, runtime, providerID, market, symbol)
	}
}

func defaultInstrumentSpec(symbol string) backteststore.InstrumentSpec {
	spec := backteststore.InstrumentSpec{Symbol: strings.ToUpper(strings.TrimSpace(symbol))}
	if profile, ok := marketpkg.ProfileForSymbol(symbol); ok {
		spec.QuoteCurrency = profile.QuoteCurrency
		spec.PricePrecision = profile.PricePrecision
		spec.QuotePrecision = profile.QuotePrecision
		spec.TickSize = profile.TickSize
	}
	return conservativeInstrumentSpec(spec)
}

func conservativeInstrumentSpec(spec backteststore.InstrumentSpec) backteststore.InstrumentSpec {
	if spec.TickSize <= 0 {
		spec.TickSize = 0.01
	}
	if spec.LotSize > 0 {
		if spec.QuantityStep <= 0 {
			spec.QuantityStep = spec.LotSize
		}
		return spec
	}
	switch {
	case strings.HasPrefix(spec.Symbol, "SH."), strings.HasPrefix(spec.Symbol, "SZ."):
		spec.LotSize, spec.QuantityStep = 100, 100
	case strings.HasPrefix(spec.Symbol, "HK."):
		spec.LotSize, spec.QuantityStep = 1, 1
		spec.MissingCriticalRules = true
		spec.Warnings = append(spec.Warnings, fmt.Sprintf("market rule warning for %s: lot size unavailable; conservative fallback will reject orders", spec.Symbol))
	default:
		spec.LotSize, spec.QuantityStep = 1, 1
	}
	return spec
}

func positiveFloat(value any) (float64, bool) {
	switch typed := value.(type) {
	case float64:
		return typed, typed > 0
	case float32:
		value := float64(typed)
		return value, value > 0
	case int:
		return float64(typed), typed > 0
	case int32:
		return float64(typed), typed > 0
	case int64:
		return float64(typed), typed > 0
	case string:
		parsed, err := decimalString(typed)
		if err != nil {
			return 0, false
		}
		var result float64
		_, err = fmt.Sscan(parsed, &result)
		return result, err == nil && result > 0
	default:
		return 0, false
	}
}
