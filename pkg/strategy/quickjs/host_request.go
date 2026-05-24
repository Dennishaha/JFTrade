package quickjs

import (
	"fmt"
	"strings"

	"github.com/c9s/bbgo/pkg/fixedpoint"
	"github.com/c9s/bbgo/pkg/types"
	qjs "modernc.org/quickjs"
)

func decodeHostRequest(request *qjs.Object) (map[string]any, error) {
	if request == nil {
		return map[string]any{}, nil
	}
	decodedRequest := map[string]any{}
	if err := request.Into(&decodedRequest); err != nil {
		return nil, fmt.Errorf("decode host request: %w", err)
	}
	return decodedRequest, nil
}

func readHostString(values map[string]any, key string) string {
	if values == nil {
		return ""
	}
	value, ok := values[key]
	if !ok || value == nil {
		return ""
	}
	switch typedValue := value.(type) {
	case string:
		return typedValue
	default:
		return fmt.Sprintf("%v", value)
	}
}

func readHostFixedpoint(values map[string]any, key string) (fixedpoint.Value, error) {
	value, ok := values[key]
	if !ok || value == nil {
		return fixedpoint.Zero, fmt.Errorf("%s is required", key)
	}
	return toFixedpointValue(value, key)
}

func readOptionalHostFixedpoint(values map[string]any, key string) (*fixedpoint.Value, error) {
	if values == nil {
		return nil, nil
	}
	value, ok := values[key]
	if !ok || value == nil {
		return nil, nil
	}
	fixedValue, err := toFixedpointValue(value, key)
	if err != nil {
		return nil, err
	}
	return &fixedValue, nil
}

func toFixedpointValue(value any, key string) (fixedpoint.Value, error) {
	switch typedValue := value.(type) {
	case float64:
		return fixedpoint.NewFromFloat(typedValue), nil
	case float32:
		return fixedpoint.NewFromFloat(float64(typedValue)), nil
	case int:
		return fixedpoint.NewFromFloat(float64(typedValue)), nil
	case int64:
		return fixedpoint.NewFromFloat(float64(typedValue)), nil
	case int32:
		return fixedpoint.NewFromFloat(float64(typedValue)), nil
	case string:
		trimmedValue := strings.TrimSpace(typedValue)
		if trimmedValue == "" {
			return fixedpoint.Zero, fmt.Errorf("%s is required", key)
		}
		fixedValue, err := fixedpoint.NewFromString(trimmedValue)
		if err != nil {
			return fixedpoint.Zero, fmt.Errorf("invalid %s: %w", key, err)
		}
		return fixedValue, nil
	default:
		return fixedpoint.Zero, fmt.Errorf("invalid %s type %T", key, value)
	}
}

func readHostBool(values map[string]any, key string) (bool, bool) {
	if values == nil {
		return false, false
	}
	value, ok := values[key]
	if !ok || value == nil {
		return false, false
	}
	switch typedValue := value.(type) {
	case bool:
		return typedValue, true
	default:
		return false, false
	}
}

func parseHostSideType(value string) (types.SideType, error) {
	switch strings.ToUpper(strings.TrimSpace(value)) {
	case string(types.SideTypeBuy):
		return types.SideTypeBuy, nil
	case string(types.SideTypeSell):
		return types.SideTypeSell, nil
	default:
		return "", fmt.Errorf("invalid side %q", value)
	}
}

func parseHostOrderType(value string) (types.OrderType, error) {
	switch strings.ToUpper(strings.TrimSpace(value)) {
	case "", string(types.OrderTypeMarket):
		return types.OrderTypeMarket, nil
	case string(types.OrderTypeLimit):
		return types.OrderTypeLimit, nil
	default:
		return "", fmt.Errorf("invalid orderType %q", value)
	}
}

func parseHostTimeInForce(value string) (types.TimeInForce, error) {
	normalizedValue := strings.ToUpper(strings.TrimSpace(value))
	if normalizedValue == "" {
		return types.TimeInForceGTC, nil
	}
	timeInForce := types.TimeInForce(normalizedValue)
	switch timeInForce {
	case types.TimeInForceGTC, types.TimeInForceIOC, types.TimeInForceFOK, types.TimeInForceGTT:
		return timeInForce, nil
	default:
		return "", fmt.Errorf("invalid timeInForce %q", value)
	}
}
