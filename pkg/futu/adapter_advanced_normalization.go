package futu

import (
	"encoding/json"
	"maps"
	"strings"

	qotcommonpb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotcommon"
)

func normalizeOpenDMap(value map[string]any) map[string]any {
	normalized, _ := normalizeOpenDValue(value).(map[string]any)
	return normalized
}

func normalizeOpenDValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		result := make(map[string]any, len(typed))
		for key, item := range typed {
			result[key] = normalizeOpenDValue(item)
		}
		if security, ok := normalizeOpenDSecurity(result); ok {
			return security
		}
		return result
	case []any:
		result := make([]any, len(typed))
		for index, item := range typed {
			result[index] = normalizeOpenDValue(item)
		}
		return result
	case string:
		return normalizeOpenDEnum(typed)
	default:
		return value
	}
}

func normalizeOpenDSecurity(value map[string]any) (map[string]any, bool) {
	code := stringValue(value["code"])
	if code == "" {
		return nil, false
	}
	rawMarket := strings.ToLower(strings.TrimSpace(stringValue(value["market"])))
	if numericMarket, ok := value["market"].(float64); ok {
		rawMarket = strings.ToLower(qotcommonpb.QotMarket(int32(numericMarket)).String())
	}
	if rawMarket == "" {
		return nil, false
	}
	publicMarket := ""
	productClass := ""
	switch {
	case strings.Contains(rawMarket, "future"):
		publicMarket, productClass = "HK", "future"
	case strings.Contains(rawMarket, "event"), strings.Contains(rawMarket, "prediction"):
		publicMarket, productClass = "US", "event_contract"
	case strings.Contains(rawMarket, "us"):
		publicMarket = "US"
	case strings.Contains(rawMarket, "hk"):
		publicMarket = "HK"
	case strings.Contains(rawMarket, "sh"):
		publicMarket = "SH"
	case strings.Contains(rawMarket, "sz"):
		publicMarket = "SZ"
	default:
		return nil, false
	}
	result := cloneMap(value)
	result["market"] = publicMarket
	result["quoteMarket"] = publicMarket
	result["tradeMarket"] = publicMarket
	result["instrumentId"] = publicMarket + "." + strings.ToUpper(code)
	if productClass != "" {
		result["productClass"] = productClass
	}
	return result, true
}

func normalizeOpenDEnum(value string) string {
	prefixes := []string{
		"QotMarket_", "SecurityType_", "OptionType_", "IndexOptionType_",
		"ExpirationCycle_", "EC_", "PredSide_", "TrdSide_", "OrderStatus_",
		"TrdEnv_", "TrdMarket_", "KLType_", "RehabType_",
	}
	for _, prefix := range prefixes {
		if after, ok := strings.CutPrefix(value, prefix); ok {
			remainder := after
			if prefix == "EC_" {
				if index := strings.Index(remainder, "_"); index >= 0 {
					remainder = remainder[index+1:]
				}
			}
			return strings.ToLower(remainder)
		}
	}
	return value
}

func cloneMap(source map[string]any) map[string]any {
	result := make(map[string]any, len(source))
	maps.Copy(result, source)
	return result
}

func structMap(value any) (map[string]any, error) {
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

// jsonSafeStructMap converts adapter-owned DTOs whose fields are restricted to
// JSON primitives. Those concrete DTOs cannot contain unsupported JSON values.
func jsonSafeStructMap(value any) map[string]any {
	result, _ := structMap(value)
	return result
}

func stringValue(value any) string {
	text, _ := value.(string)
	return strings.TrimSpace(text)
}

func numberValue(value any, fallback float64) float64 {
	number, ok := value.(float64)
	if !ok {
		return fallback
	}
	return number
}
