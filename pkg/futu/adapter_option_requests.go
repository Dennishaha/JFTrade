package futu

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/jftrade/jftrade-main/pkg/broker"
)

var optionEventProtocols = map[string]int{
	"Qot_GetOptionEvent":            101,
	"Qot_GetOptionZeroDteScreener":  1,
	"Qot_GetOptionEarningsScreener": 1,
	"Qot_GetOptionSellerScreener":   1,
}

func injectOptionEventDefaults(
	params map[string]any,
	protocol string,
	query broker.FeatureQuery,
) error {
	optionMarket, err := futuOptionMarket(query.Market, stringValue(params["underlyingProductClass"]))
	if err != nil {
		return err
	}
	delete(params, "underlyingProductClass")

	if protocol == "Qot_GetOptionZeroDteScreener" && !strings.EqualFold(query.Market, "US") {
		return fmt.Errorf("futu: 0DTE option research is available only in the US market")
	}
	if params["optionMarket"] == nil {
		params["optionMarket"] = optionMarket
	}
	if protocol != "Qot_GetOptionSellerScreener" && params["count"] == nil {
		count := query.PageSize
		if count <= 0 {
			count = 50
		}
		maxCount := 500
		if protocol == "Qot_GetOptionEvent" {
			maxCount = 300
		}
		params["count"] = min(count, maxCount)
	}
	if protocol == "Qot_GetOptionSellerScreener" {
		sellerType, sellerErr := futuSellerType(stringValue(params["sellerStrategy"]))
		if sellerErr != nil {
			return sellerErr
		}
		delete(params, "sellerStrategy")
		if params["sellerType"] == nil {
			params["sellerType"] = sellerType
		}
	}
	if indicatorType := optionEventProtocols[protocol]; indicatorType != 0 &&
		strings.TrimSpace(query.InstrumentID) != "" {
		security, securityErr := optionSecurityMap(query.InstrumentID)
		if securityErr != nil {
			return securityErr
		}
		appendOptionFilter(params, indicatorType, map[string]any{
			"securityList": []any{security},
		})
	}
	return nil
}

func injectZeroDteContractParams(
	params map[string]any,
	query broker.FeatureQuery,
) error {
	if !strings.EqualFold(query.Market, "US") {
		return fmt.Errorf("futu: 0DTE option research is available only in the US market")
	}
	owner, err := optionSecurityMap(query.InstrumentID)
	if err != nil {
		return err
	}
	locator, err := optionZeroDteLocator(params["chainLocator"])
	if err != nil {
		return err
	}
	expiry, ok := int64Param(params["expiryTimestamp"])
	if !ok || expiry <= 0 {
		return fmt.Errorf("futu: 0DTE contract query requires expiryTimestamp")
	}
	if locator.ProductCode == "" {
		return fmt.Errorf("futu: 0DTE contract query requires chainLocator.productCode")
	}
	params["owner"] = owner
	params["strikeDateTimestamp"] = expiry
	params["chainInfo"] = map[string]any{
		"strikeDateTimestamp": expiry,
		"productCode":         locator.ProductCode,
		"multiplier":          locator.Multiplier,
		"contractShareSize":   locator.ContractSize,
		"expirationType":      locator.ExpirationType,
		"underlying":          owner,
	}
	if sortType, sortErr := zeroDteContractSort(stringValue(params["sort"])); sortErr != nil {
		return sortErr
	} else if sortType != 0 {
		params["sortType"] = sortType
	}
	if optionType, typeErr := zeroDteOptionType(stringValue(params["optionType"])); typeErr != nil {
		return typeErr
	} else if optionType != 0 {
		appendOptionFilter(params, 1, map[string]any{"valueList": []any{optionType}})
	}
	delete(params, "chainLocator")
	delete(params, "expiryTimestamp")
	delete(params, "sort")
	delete(params, "optionType")
	delete(params, "underlyingProductClass")
	return nil
}

func futuOptionMarket(market, productClass string) (int, error) {
	isIndex := strings.EqualFold(strings.TrimSpace(productClass), string(broker.ProductClassIndex))
	switch strings.ToUpper(strings.TrimSpace(market)) {
	case "US":
		if isIndex {
			return 2, nil
		}
		return 1, nil
	case "HK":
		if isIndex {
			return 4, nil
		}
		return 3, nil
	default:
		return 0, fmt.Errorf("futu: options are unavailable for market %q", market)
	}
}

func futuSellerType(strategy string) (int, error) {
	switch strings.ToLower(strings.TrimSpace(strategy)) {
	case "", "covered_call":
		return 1, nil
	case "cash_secured_put":
		return 2, nil
	default:
		return 0, fmt.Errorf("futu: unsupported sellerStrategy %q", strategy)
	}
}

func optionSecurityMap(instrumentID string) (map[string]any, error) {
	security, _, err := futuSecurityFromSymbol(strings.ToUpper(strings.TrimSpace(instrumentID)))
	if err != nil {
		return nil, err
	}
	return map[string]any{"market": security.GetMarket(), "code": security.GetCode()}, nil
}

func appendOptionFilter(params map[string]any, indicatorType int, indicatorValue map[string]any) {
	filter := map[string]any{
		"indicatorType":  indicatorType,
		"indicatorValue": indicatorValue,
	}
	switch existing := params["filterList"].(type) {
	case []any:
		params["filterList"] = append(existing, filter)
	case nil:
		params["filterList"] = []any{filter}
	default:
		params["filterList"] = []any{existing, filter}
	}
}

func optionZeroDteLocator(value any) (broker.OptionZeroDteChainLocator, error) {
	switch typed := value.(type) {
	case broker.OptionZeroDteChainLocator:
		return typed, nil
	case map[string]any:
		return broker.OptionZeroDteChainLocator{
			ProductCode:    stringValue(typed["productCode"]),
			Multiplier:     floatParam(typed["multiplier"]),
			ContractSize:   floatParam(typed["contractSize"]),
			ExpirationType: int32Param(typed["expirationType"]),
		}, nil
	default:
		content, err := json.Marshal(value)
		if err != nil {
			return broker.OptionZeroDteChainLocator{}, fmt.Errorf("futu: invalid chainLocator")
		}
		var locator broker.OptionZeroDteChainLocator
		if err := json.Unmarshal(content, &locator); err != nil {
			return broker.OptionZeroDteChainLocator{}, fmt.Errorf("futu: invalid chainLocator")
		}
		return locator, nil
	}
}

func zeroDteContractSort(value string) (int, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "default":
		return 0, nil
	case "volume":
		return 1, nil
	case "open_interest":
		return 2, nil
	case "iv":
		return 3, nil
	case "delta":
		return 4, nil
	default:
		return 0, fmt.Errorf("futu: unsupported 0DTE contract sort %q", value)
	}
}

func zeroDteOptionType(value string) (int, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "all":
		return 0, nil
	case "call":
		return 1, nil
	case "put":
		return 2, nil
	default:
		return 0, fmt.Errorf("futu: unsupported 0DTE optionType %q", value)
	}
}

func int64Param(value any) (int64, bool) {
	switch typed := value.(type) {
	case int:
		return int64(typed), true
	case int32:
		return int64(typed), true
	case int64:
		return typed, true
	case float64:
		return int64(typed), true
	case json.Number:
		parsed, err := typed.Int64()
		return parsed, err == nil
	case string:
		parsed, err := strconv.ParseInt(strings.TrimSpace(typed), 10, 64)
		return parsed, err == nil
	default:
		return 0, false
	}
}

func floatParam(value any) float64 {
	switch typed := value.(type) {
	case float64:
		return typed
	case float32:
		return float64(typed)
	case int:
		return float64(typed)
	case int32:
		return float64(typed)
	case int64:
		return float64(typed)
	case json.Number:
		result, _ := typed.Float64()
		return result
	case string:
		result, _ := strconv.ParseFloat(strings.TrimSpace(typed), 64)
		return result
	default:
		return 0
	}
}

func int32Param(value any) int32 {
	result, _ := int64Param(value)
	return int32(result)
}
