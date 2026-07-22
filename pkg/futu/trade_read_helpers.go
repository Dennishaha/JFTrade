package futu

import (
	"strconv"
	"strings"

	"github.com/jftrade/jftrade-main/pkg/bbgo/fixedpoint"

	commonpb "github.com/jftrade/jftrade-main/pkg/futu/pb/common"
	trdcommonpb "github.com/jftrade/jftrade-main/pkg/futu/pb/trdcommon"
	trdflowsummarypb "github.com/jftrade/jftrade-main/pkg/futu/pb/trdflowsummary"
)

func brokerTradeFilterConditions(symbol string, startTime string, endTime string, market int32) *trdcommonpb.TrdFilterConditions {
	filter := &trdcommonpb.TrdFilterConditions{}
	canonicalSymbol := strings.TrimSpace(strings.ToUpper(symbol))
	if canonicalSymbol != "" {
		filter.CodeList = []string{canonicalSymbol}
	}
	if trimmed := normalizeTradeFilterTimeInput(startTime, canonicalSymbol, market); trimmed != "" {
		filter.BeginTime = new(trimmed)
	}
	if trimmed := normalizeTradeFilterTimeInput(endTime, canonicalSymbol, market); trimmed != "" {
		filter.EndTime = new(trimmed)
	}
	if market != 0 {
		filter.FilterMarket = new(market)
	}
	return filter
}

func brokerOrderStatusFilterValues(statuses []string) []int32 {
	if len(statuses) == 0 {
		return nil
	}
	values := make([]int32, 0, len(statuses))
	seen := make(map[int32]struct{}, len(statuses))
	for _, rawStatus := range statuses {
		normalized := normalizeRuntimeEnum(rawStatus)
		if normalized == "" {
			continue
		}
		for value := range trdcommonpb.OrderStatus_name {
			if normalizeRuntimeEnum(enumName(value, trdcommonpb.OrderStatus_name)) != normalized {
				continue
			}
			if _, exists := seen[value]; exists {
				break
			}
			seen[value] = struct{}{}
			values = append(values, value)
			break
		}
	}
	return values
}

func trdOrderTypeFromBrokerOrderType(orderType string) (trdcommonpb.OrderType, string, bool) {
	normalized := normalizeRuntimeEnum(orderType)
	switch normalized {
	case "LIMIT", "LIMIT_MAKER", "NORMAL":
		return trdcommonpb.OrderType_OrderType_Normal, "LIMIT", true
	case "MARKET":
		return trdcommonpb.OrderType_OrderType_Market, "MARKET", true
	case "STOP":
		return trdcommonpb.OrderType_OrderType_Stop, "STOP", true
	case "STOP_LIMIT", "STOPLIMIT":
		return trdcommonpb.OrderType_OrderType_StopLimit, "STOP_LIMIT", true
	case "TAKE_PROFIT_MARKET", "MARKETIFTOUCHED":
		return trdcommonpb.OrderType_OrderType_MarketifTouched, "TAKE_PROFIT_MARKET", true
	case "TAKE_PROFIT", "LIMITIFTOUCHED":
		return trdcommonpb.OrderType_OrderType_LimitifTouched, "TAKE_PROFIT", true
	default:
		return 0, "", false
	}
}

func sessionValue(session string) (int32, bool) {
	normalized := normalizeRuntimeEnum(session)
	for value := range commonpb.Session_name {
		if normalizeRuntimeEnum(enumName(value, commonpb.Session_name)) == normalized {
			return value, true
		}
	}
	return 0, false
}

func cashFlowDirectionValue(direction string) *int32 {
	normalized := normalizeRuntimeEnum(direction)
	if normalized == "" {
		return nil
	}
	for value := range trdflowsummarypb.TrdCashFlowDirection_name {
		if normalizeRuntimeEnum(enumName(value, trdflowsummarypb.TrdCashFlowDirection_name)) != normalized {
			continue
		}
		return new(value)
	}
	return nil
}

func optionalStringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func optionalUint64StringPtr(value *uint64) *string {
	if value == nil {
		return nil
	}
	return new(strconv.FormatUint(*value, 10))
}

func optionalEnumStringPtr(value *int32, names map[int32]string) *string {
	if value == nil {
		return nil
	}
	normalized := normalizeRuntimeEnum(enumName(*value, names))
	if normalized == "" || normalized == "UNKNOWN" {
		return nil
	}
	return &normalized
}

func optionalNonEmptyString(value string) *string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}

func preferredFloat64Ptr(primary *float64, fallback *float64) *float64 {
	if primary != nil {
		return cloneFloat64Ptr(primary)
	}
	return cloneFloat64Ptr(fallback)
}

func cloneFloat64Ptr(value *float64) *float64 {
	if value == nil {
		return nil
	}
	return new(*value)
}

func fixedpointFromPtr(primary *float64, fallback *float64) fixedpoint.Value {
	if primary != nil {
		return fixedpoint.NewFromFloat(*primary)
	}
	if fallback != nil {
		return fixedpoint.NewFromFloat(*fallback)
	}
	return fixedpoint.Zero
}

func fixedpointFromDifference(total *float64, available *float64, fallback *float64) fixedpoint.Value {
	if total != nil && available != nil {
		value := *total - *available
		if value < 0 {
			value = 0
		}
		return fixedpoint.NewFromFloat(value)
	}
	if fallback != nil {
		return fixedpoint.NewFromFloat(*fallback)
	}
	return fixedpoint.Zero
}

func optionalFloat64Value(value *float64) float64 {
	if value == nil {
		return 0
	}
	return *value
}

func parseUint64(value string) uint64 {
	parsed, err := strconv.ParseUint(strings.TrimSpace(value), 10, 64)
	if err != nil {
		return 0
	}
	return parsed
}
