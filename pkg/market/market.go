package market

import (
	"fmt"
	"github.com/jftrade/jftrade-main/pkg/market/hk"
	"github.com/jftrade/jftrade-main/pkg/market/sh"
	"github.com/jftrade/jftrade-main/pkg/market/sz"
	"github.com/jftrade/jftrade-main/pkg/market/us"
	"strings"
	"time"
)

type MarketCode string

const (
	MarketUS MarketCode = "US"
	MarketHK MarketCode = "HK"
	MarketCN MarketCode = "CN"
	MarketSH MarketCode = "SH"
	MarketSZ MarketCode = "SZ"
)

type Session string

const (
	SessionUnknown   Session = "unknown"
	SessionClosed    Session = "closed"
	SessionPre       Session = "pre"
	SessionRegular   Session = "regular"
	SessionAfter     Session = "after"
	SessionOvernight Session = "overnight"
)

type TradingWindow struct {
	StartMinute int
	EndMinute   int
}

type Precision struct {
	Price int
	Quote int
}

type MarketDescriptor struct {
	Code                   string
	ResolvedMarket         string
	PreferredPrefix        string
	DisplayName            string
	QuoteCurrency          string
	Timezone               string
	PricePrecision         int
	QuotePrecision         int
	TickSize               float64
	SupportsExtendedHours  bool
	RequiresExchangePrefix bool
	Aliases                []string
	RegularSessions        []TradingWindow
}

type Profile struct {
	Market                 string
	ResolvedMarket         string
	PreferredPrefix        string
	DisplayName            string
	QuoteCurrency          string
	PricePrecision         int
	QuotePrecision         int
	TickSize               float64
	Aliases                []string
	Location               *time.Location
	Sessions               []TradingWindow
	ExtendedHours          bool
	RequiresExchangePrefix bool
}

type Instrument struct {
	Market string
	Prefix string
	Code   string
	Symbol string
}

type InstrumentInput struct {
	Market       string
	Symbol       string
	Code         string
	InstrumentID string
}

var profiles = map[string]Profile{
	us.Code: {
		Market:          us.Code,
		ResolvedMarket:  us.ResolvedMarket,
		PreferredPrefix: us.PreferredPrefix,
		DisplayName:     "US",
		QuoteCurrency:   "USD",
		PricePrecision:  2,
		QuotePrecision:  2,
		TickSize:        0.01,
		Aliases:         []string{"NYSE", "NASDAQ"},
		Location:        us.Location(),
		Sessions:        convertWindowPairs(us.RegularWindows),
		ExtendedHours:   true,
	},
	hk.Code: {
		Market:          hk.Code,
		ResolvedMarket:  hk.ResolvedMarket,
		PreferredPrefix: hk.PreferredPrefix,
		DisplayName:     "Hong Kong",
		QuoteCurrency:   "HKD",
		PricePrecision:  3,
		QuotePrecision:  3,
		TickSize:        0.001,
		Aliases:         []string{"HKEX"},
		Location:        hk.Location(),
		Sessions:        convertWindowPairs(hk.RegularWindows),
	},
	sh.Code: {
		Market:                 sh.Code,
		ResolvedMarket:         sh.ResolvedMarket,
		PreferredPrefix:        sh.PreferredPrefix,
		DisplayName:            "Shanghai",
		QuoteCurrency:          "CNY",
		PricePrecision:         2,
		QuotePrecision:         2,
		TickSize:               0.01,
		Aliases:                []string{"CNSH"},
		Location:               sh.Location(),
		Sessions:               convertWindowPairs(sh.RegularWindows),
		RequiresExchangePrefix: true,
	},
	sz.Code: {
		Market:                 sz.Code,
		ResolvedMarket:         sz.ResolvedMarket,
		PreferredPrefix:        sz.PreferredPrefix,
		DisplayName:            "Shenzhen",
		QuoteCurrency:          "CNY",
		PricePrecision:         2,
		QuotePrecision:         2,
		TickSize:               0.01,
		Aliases:                []string{"CNSZ"},
		Location:               sz.Location(),
		Sessions:               convertWindowPairs(sz.RegularWindows),
		RequiresExchangePrefix: true,
	},
}

var marketDescriptorOrder = []string{"HK", "US", "CN", "SH", "SZ"}

var marketSubsets = map[string][]string{
	string(MarketCN): {string(MarketSH), string(MarketSZ)},
}

func convertWindowPairs(windows [][2]int) []TradingWindow {
	result := make([]TradingWindow, 0, len(windows))
	for _, window := range windows {
		result = append(result, TradingWindow{StartMinute: window[0], EndMinute: window[1]})
	}
	return result
}

func NormalizeMarketInput(market string) (resolvedMarket string, preferredPrefix string, err error) {
	normalized := strings.ToUpper(strings.TrimSpace(market))
	switch normalized {
	case "":
		return "", "", nil
	case "CN":
		return "CN", "", nil
	case "CNSH":
		return "CN", "SH", nil
	case "CNSZ":
		return "CN", "SZ", nil
	case "SG", "JP", "AU", "MY", "CA":
		return normalized, normalized, nil
	}
	if profile, ok := profiles[normalized]; ok {
		return profile.ResolvedMarket, profile.PreferredPrefix, nil
	}
	return "", "", fmt.Errorf("unsupported market %q", market)
}

func ParseInstrument(input InstrumentInput) (Instrument, error) {
	resolvedMarket, preferredPrefix, err := NormalizeMarketInput(input.Market)
	if err != nil {
		return Instrument{}, err
	}

	normalizedSymbol := strings.ToUpper(strings.TrimSpace(input.InstrumentID))
	if normalizedSymbol == "" {
		normalizedSymbol = strings.ToUpper(strings.TrimSpace(input.Symbol))
	}
	normalizedSymbol = strings.ReplaceAll(normalizedSymbol, ":", ".")
	normalizedCode := strings.ToUpper(strings.TrimSpace(input.Code))

	if normalizedSymbol == "" && normalizedCode == "" {
		return Instrument{}, fmt.Errorf("symbol or code is required")
	}

	if normalizedSymbol != "" {
		parsed, err := ParseQualifiedInstrumentSymbol(normalizedSymbol)
		if err == nil {
			if normalizedCode != "" && !strings.EqualFold(normalizedCode, parsed.Code) {
				return Instrument{}, fmt.Errorf("code %q does not match symbol %q", input.Code, input.Symbol)
			}
			if resolvedMarket != "" && !marketInputMatchesParsedSymbol(input.Market, parsed) {
				return Instrument{}, fmt.Errorf("market %q does not match symbol %q", input.Market, input.Symbol)
			}
			return parsed, nil
		}
		if strings.Contains(normalizedSymbol, ".") {
			return Instrument{}, err
		}
		if normalizedCode != "" && !strings.EqualFold(normalizedCode, normalizedSymbol) {
			return Instrument{}, fmt.Errorf("code %q does not match symbol %q", input.Code, input.Symbol)
		}
		normalizedCode = normalizedSymbol
	}

	if resolvedMarket == "" {
		return Instrument{}, fmt.Errorf("market is required when symbol has no market prefix")
	}
	if preferredPrefix == "" {
		return Instrument{}, fmt.Errorf("market %q requires an exchange-qualified symbol like SH.600519 or SZ.000001", input.Market)
	}

	return Instrument{
		Market: resolvedMarket,
		Prefix: preferredPrefix,
		Code:   normalizedCode,
		Symbol: preferredPrefix + "." + normalizedCode,
	}, nil
}

func ParseQualifiedInstrumentSymbol(symbol string) (Instrument, error) {
	normalized := strings.ToUpper(strings.TrimSpace(symbol))
	normalized = strings.ReplaceAll(normalized, ":", ".")
	parts := strings.SplitN(normalized, ".", 2)
	if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" || strings.TrimSpace(parts[1]) == "" {
		return Instrument{}, fmt.Errorf("symbol %q must be in MARKET.CODE form", symbol)
	}
	resolvedMarket, preferredPrefix, err := NormalizeMarketInput(parts[0])
	if err != nil {
		return Instrument{}, err
	}
	prefix := strings.ToUpper(strings.TrimSpace(parts[0]))
	code := strings.ToUpper(strings.TrimSpace(parts[1]))
	if preferredPrefix == "" {
		return Instrument{}, fmt.Errorf("market %q requires an exchange-qualified symbol like SH.600519 or SZ.000001", prefix)
	}
	if preferredPrefix != prefix {
		prefix = preferredPrefix
	}
	return Instrument{
		Market: resolvedMarket,
		Prefix: prefix,
		Code:   code,
		Symbol: prefix + "." + code,
	}, nil
}

func MarketInputMatchesParsedSymbol(market string, parsed Instrument) bool {
	return marketInputMatchesParsedSymbol(market, parsed)
}

func ProfileForSymbol(symbol string) (Profile, bool) {
	profile, ok := profiles[SymbolMarket(symbol)]
	return profile, ok
}

func MarketDescriptors() []MarketDescriptor {
	result := make([]MarketDescriptor, 0, len(marketDescriptorOrder))
	for _, code := range marketDescriptorOrder {
		if code == "CN" {
			result = append(result, MarketDescriptor{
				Code:                   "CN",
				ResolvedMarket:         "CN",
				DisplayName:            "沪深",
				QuoteCurrency:          "CNY",
				Timezone:               sh.LocationName,
				PricePrecision:         2,
				QuotePrecision:         2,
				TickSize:               0.01,
				RequiresExchangePrefix: true,
				Aliases:                []string{"SH", "SZ", "CNSH", "CNSZ"},
				RegularSessions:        convertWindowPairs(sh.RegularWindows),
			})
			continue
		}
		if profile, ok := profiles[code]; ok {
			result = append(result, descriptorFromProfile(profile))
		}
	}
	return result
}

// UserMarketDescriptors returns the top-level market categories exposed to
// user-facing selectors. Exchange-level children remain available through
// MarketDescriptors for routing, capabilities, calendars, and diagnostics.
func UserMarketDescriptors() []MarketDescriptor {
	descriptors := MarketDescriptors()
	result := make([]MarketDescriptor, 0, len(descriptors))
	for _, descriptor := range descriptors {
		if IsMarketSubsetChild(descriptor.Code) {
			continue
		}
		result = append(result, descriptor)
	}
	return result
}

// MarketSubsetChildren returns the configured leaf markets for a top-level
// market category in stable lookup order. The returned slice is independent
// from the package configuration and may be modified by the caller.
func MarketSubsetChildren(parent string) []string {
	children := marketSubsets[strings.ToUpper(strings.TrimSpace(parent))]
	return append([]string(nil), children...)
}

// IsMarketSubsetChild reports whether marketCode is a leaf of any configured
// top-level market category.
func IsMarketSubsetChild(marketCode string) bool {
	normalized := strings.ToUpper(strings.TrimSpace(marketCode))
	for _, children := range marketSubsets {
		for _, child := range children {
			if child == normalized {
				return true
			}
		}
	}
	return false
}

func descriptorFromProfile(profile Profile) MarketDescriptor {
	return MarketDescriptor{
		Code:                   profile.Market,
		ResolvedMarket:         profile.ResolvedMarket,
		PreferredPrefix:        profile.PreferredPrefix,
		DisplayName:            profile.DisplayName,
		QuoteCurrency:          profile.QuoteCurrency,
		Timezone:               profile.Location.String(),
		PricePrecision:         profile.PricePrecision,
		QuotePrecision:         profile.QuotePrecision,
		TickSize:               profile.TickSize,
		SupportsExtendedHours:  profile.ExtendedHours,
		RequiresExchangePrefix: profile.RequiresExchangePrefix,
		Aliases:                append([]string(nil), profile.Aliases...),
		RegularSessions:        append([]TradingWindow(nil), profile.Sessions...),
	}
}

func SymbolMarket(symbol string) string {
	trimmed := strings.ToUpper(strings.TrimSpace(symbol))
	switch {
	case strings.HasPrefix(trimmed, "US."):
		return "US"
	case strings.HasPrefix(trimmed, "HK."):
		return "HK"
	case strings.HasPrefix(trimmed, "SH."):
		return "SH"
	case strings.HasPrefix(trimmed, "SZ."):
		return "SZ"
	default:
		return ""
	}
}

func IsUSSymbol(symbol string) bool {
	return SymbolMarket(symbol) == "US"
}
