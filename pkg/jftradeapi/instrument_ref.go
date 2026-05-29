package jftradeapi

import (
	"fmt"
	"strings"
)

type normalizedInstrument struct {
	Market string
	Prefix string
	Code   string
	Symbol string
}

func normalizeInstrumentInput(market string, symbol string, code string) (normalizedInstrument, error) {
	resolvedMarket, preferredPrefix, err := normalizeInstrumentMarketInput(market)
	if err != nil {
		return normalizedInstrument{}, err
	}

	normalizedSymbol := strings.ToUpper(strings.TrimSpace(symbol))
	normalizedSymbol = strings.ReplaceAll(normalizedSymbol, ":", ".")
	normalizedCode := strings.ToUpper(strings.TrimSpace(code))

	if normalizedSymbol == "" && normalizedCode == "" {
		return normalizedInstrument{}, fmt.Errorf("symbol or code is required")
	}

	if normalizedSymbol != "" {
		parsed, err := parseQualifiedInstrumentSymbol(normalizedSymbol)
		if err == nil {
			if normalizedCode != "" && !strings.EqualFold(normalizedCode, parsed.Code) {
				return normalizedInstrument{}, fmt.Errorf("code %q does not match symbol %q", code, symbol)
			}
			if resolvedMarket != "" && !instrumentMarketInputMatchesParsedSymbol(market, parsed) {
				return normalizedInstrument{}, fmt.Errorf("market %q does not match symbol %q", market, symbol)
			}
			return parsed, nil
		}
		if strings.Contains(normalizedSymbol, ".") {
			return normalizedInstrument{}, err
		}
		if normalizedCode != "" && !strings.EqualFold(normalizedCode, normalizedSymbol) {
			return normalizedInstrument{}, fmt.Errorf("code %q does not match symbol %q", code, symbol)
		}
		normalizedCode = normalizedSymbol
	}

	if resolvedMarket == "" {
		return normalizedInstrument{}, fmt.Errorf("market is required when symbol has no market prefix")
	}
	if preferredPrefix == "" {
		return normalizedInstrument{}, fmt.Errorf("market %q requires an exchange-qualified symbol like SH.600519 or SZ.000001", market)
	}

	return normalizedInstrument{
		Market: resolvedMarket,
		Prefix: preferredPrefix,
		Code:   normalizedCode,
		Symbol: preferredPrefix + "." + normalizedCode,
	}, nil
}

func parseQualifiedInstrumentSymbol(symbol string) (normalizedInstrument, error) {
	normalized := strings.ToUpper(strings.TrimSpace(symbol))
	normalized = strings.ReplaceAll(normalized, ":", ".")
	parts := strings.SplitN(normalized, ".", 2)
	if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" || strings.TrimSpace(parts[1]) == "" {
		return normalizedInstrument{}, fmt.Errorf("symbol %q must be in MARKET.CODE form", symbol)
	}
	resolvedMarket, _, err := normalizeInstrumentMarketInput(parts[0])
	if err != nil {
		return normalizedInstrument{}, err
	}
	prefix := strings.ToUpper(strings.TrimSpace(parts[0]))
	code := strings.ToUpper(strings.TrimSpace(parts[1]))
	return normalizedInstrument{
		Market: resolvedMarket,
		Prefix: prefix,
		Code:   code,
		Symbol: prefix + "." + code,
	}, nil
}

func normalizeInstrumentMarketInput(market string) (resolvedMarket string, preferredPrefix string, err error) {
	normalized := strings.ToUpper(strings.TrimSpace(market))
	switch normalized {
	case "":
		return "", "", nil
	case "HK", "US", "SG", "JP", "AU", "MY", "CA", "CN":
		if normalized == "CN" {
			return normalized, "", nil
		}
		return normalized, normalized, nil
	case "SH", "SZ":
		return "CN", normalized, nil
	default:
		return "", "", fmt.Errorf("unsupported market %q", market)
	}
}

func instrumentMarketInputMatchesParsedSymbol(market string, parsed normalizedInstrument) bool {
	resolvedMarket, preferredPrefix, err := normalizeInstrumentMarketInput(market)
	if err != nil {
		return false
	}
	if resolvedMarket != parsed.Market {
		return false
	}
	if preferredPrefix == "" {
		return true
	}
	return preferredPrefix == parsed.Prefix
}
