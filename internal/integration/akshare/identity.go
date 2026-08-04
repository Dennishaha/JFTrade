package akshare

import (
	"fmt"
	"strings"

	"github.com/jftrade/jftrade-main/pkg/market"
)

const defaultMarket = "US"

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
	input := market.InstrumentInput{Market: canonical, Symbol: symbol, InstrumentID: instrumentID}
	if instrumentID == "" && canonical != "" && strings.HasPrefix(symbol, ".") {
		input.Symbol = ""
		input.Code = symbol
	}
	parsed, err := market.ParseInstrument(input)
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
		return defaultMarket, nil
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
	if code == "" {
		return fmt.Errorf("%w: instrument code is required", ErrUnsupported)
	}
	switch prefix {
	case "US":
		if !validCatalogToken(code) {
			return fmt.Errorf("%w: invalid US symbol", ErrUnsupported)
		}
	case "HK":
		if isDigits(code) && len(code) == 5 {
			return nil
		}
		if !validCatalogToken(code) {
			return fmt.Errorf("%w: invalid HK symbol", ErrUnsupported)
		}
	case "SH", "SZ":
		if !isDigits(code) || len(code) != 6 {
			return fmt.Errorf("%w: %s symbols must contain six digits", ErrUnsupported, prefix)
		}
	}
	return nil
}

func validCatalogToken(value string) bool {
	if value == "" || len(value) > 64 {
		return false
	}
	for _, character := range value {
		if (character >= 'A' && character <= 'Z') || (character >= '0' && character <= '9') ||
			strings.ContainsRune(".^=_-", character) {
			continue
		}
		return false
	}
	return true
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

func isSupportedLeafMarket(value string) bool {
	switch strings.ToUpper(strings.TrimSpace(value)) {
	case "US", "HK", "SH", "SZ":
		return true
	default:
		return false
	}
}

func validResolvedMarket(leaf, resolved string) bool {
	leaf = strings.ToUpper(strings.TrimSpace(leaf))
	resolved = strings.ToUpper(strings.TrimSpace(resolved))
	if leaf == "SH" || leaf == "SZ" {
		return resolved == "CN" || resolved == leaf
	}
	return resolved == leaf
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

func uniqueInstrumentIDs(values []string) ([]normalizedInstrument, error) {
	result := make([]normalizedInstrument, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		instrument, err := normalizeIdentity("", "", value)
		if err != nil {
			return nil, err
		}
		if _, ok := seen[instrument.id]; ok {
			continue
		}
		seen[instrument.id] = struct{}{}
		result = append(result, instrument)
	}
	return result, nil
}
