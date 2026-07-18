package futu

import (
	"strconv"
	"strings"

	"github.com/jftrade/jftrade-main/pkg/broker"
)

func featureResultFromProtocolPayload(
	query broker.FeatureQuery,
	protocol string,
	payload map[string]any,
) *broker.FeatureResult {
	normalized := normalizeOpenDMap(payload)
	if protocol == "Qot_GetOptionZeroDteScreener" {
		return optionZeroDteFeatureResult(query, normalized)
	}
	if !isPredictionProtocol(protocol) {
		return featureResultFromNormalizedPayload(query, normalized)
	}
	return predictionFeatureResult(query, protocol, normalized)
}

func optionZeroDteFeatureResult(
	query broker.FeatureQuery,
	payload map[string]any,
) *broker.FeatureResult {
	result := featureResultFromNormalizedPayload(query, payload)
	for _, entry := range result.Entries {
		chain, ok := entry["chainInfo"].(map[string]any)
		if !ok {
			continue
		}
		underlyingID := securityInstrumentID(entry["owner"])
		if underlyingID == "" {
			underlyingID = securityInstrumentID(chain["underlying"])
		}
		expiryTimestamp, _ := integerValue(chain["strikeDateTimestamp"])
		expirationType, _ := integerValue(chain["expirationType"])
		entry["drilldownContext"] = map[string]any{
			"underlyingInstrumentId": underlyingID,
			"expiryTimestamp":        expiryTimestamp,
			"chain": map[string]any{
				"productCode":    stringValue(chain["productCode"]),
				"multiplier":     floatParam(chain["multiplier"]),
				"contractSize":   floatParam(chain["contractShareSize"]),
				"expirationType": expirationType,
			},
		}
		delete(entry, "chainInfo")
	}
	return result
}

func featureResultFromNormalizedPayload(
	query broker.FeatureQuery,
	payload map[string]any,
) *broker.FeatureResult {
	entries, metadata := payloadEntries(payload)
	result := featureResult(query, entries, metadata)
	setPagination(result, payload, len(entries))
	return result
}

func predictionFeatureResult(
	query broker.FeatureQuery,
	protocol string,
	payload map[string]any,
) *broker.FeatureResult {
	listKey := predictionListKey(protocol)
	entries := objectSlice(payload[listKey])
	metadata := cloneMap(payload)
	delete(metadata, listKey)
	result := featureResult(query, entries, metadata)
	setPagination(result, payload, len(entries))

	if protocol == "Qot_GetEventContractComboRfq" {
		result.Entries = objectSlice(payload["comboLegList"])
		result.Metadata = map[string]any{
			"quoteId":     stringValue(payload["quoteId"]),
			"bidPrice":    payload["bidPrice"],
			"askPrice":    payload["askPrice"],
			"shouldRetry": payload["shouldRetry"],
			"mvc":         stringValue(query.Params["mvc"]),
		}
	}
	if instrument := resolvedPredictionInstrument(query, protocol, result.Entries); instrument != nil {
		result.ResolvedInstrument = instrument
	}
	return result
}

func isPredictionProtocol(protocol string) bool {
	return strings.Contains(protocol, "EventContract") ||
		protocol == "Qot_FilterCompetition"
}

func predictionListKey(protocol string) string {
	switch protocol {
	case "Qot_GetEventContractCategory":
		return "categoryList"
	case "Qot_FilterCompetition":
		return "competitionList"
	case "Qot_GetEventContractSeriesList":
		return "seriesList"
	case "Qot_GetEventContractEventList":
		return "eventList"
	case "Qot_GetEventContract":
		return "contractList"
	case "Qot_GetEventContractMilestoneList":
		return "milestoneList"
	case "Qot_GetEventContractSnapshot":
		return "snapshotList"
	case "Qot_GetEventContractOrderBook":
		return "orderBookList"
	case "Qot_GetEventContractKline", "Qot_RequestHistoryEventContractKL":
		return "klineList"
	case "Qot_GetEventContractTicker":
		return "tickerList"
	case "Qot_GetEventContractComboList":
		return "eventList"
	case "Qot_GetEventContractComboRfq":
		return "comboLegList"
	default:
		return ""
	}
}

func objectSlice(value any) []map[string]any {
	values, ok := value.([]any)
	if !ok {
		return []map[string]any{}
	}
	result := make([]map[string]any, 0, len(values))
	for _, value := range values {
		if entry, ok := value.(map[string]any); ok {
			result = append(result, entry)
		}
	}
	return result
}

func setPagination(result *broker.FeatureResult, payload map[string]any, count int) {
	result.NextCursor = firstString(payload, "nextPage", "nextKey")
	hasMore := result.NextCursor != ""
	result.HasMore = &hasMore
	total := count
	for _, key := range []string{"total", "totalCount", "allCount"} {
		if value, ok := integerValue(payload[key]); ok {
			total = value
			break
		}
	}
	result.Total = &total
}

func firstString(values map[string]any, keys ...string) string {
	for _, key := range keys {
		if value := stringValue(values[key]); value != "" {
			return value
		}
	}
	return ""
}

func integerValue(value any) (int, bool) {
	switch number := value.(type) {
	case float64:
		return int(number), true
	case int:
		return number, true
	case int32:
		return int(number), true
	case int64:
		return int(number), true
	case string:
		parsed, err := strconv.Atoi(number)
		return parsed, err == nil
	default:
		return 0, false
	}
}

func resolvedPredictionInstrument(
	query broker.FeatureQuery,
	protocol string,
	entries []map[string]any,
) *broker.Instrument {
	if query.ProductClass != broker.ProductClassEventContract &&
		query.MarketSegment != broker.MarketSegmentPrediction {
		return nil
	}
	code := strings.TrimPrefix(strings.ToUpper(strings.TrimSpace(query.InstrumentID)), "US.")
	name := ""
	tickSize := 0.01
	var eventID, seriesID, status string
	if len(entries) > 0 {
		entry := entries[0]
		for _, key := range []string{"code", "contractSecurity"} {
			if security, ok := entry[key].(map[string]any); ok {
				if value := stringValue(security["code"]); value != "" {
					code = strings.ToUpper(value)
				}
			}
		}
		name = firstString(entry, "name", "title")
		eventID = securityInstrumentID(entry["eventCode"])
		if eventID == "" {
			eventID = securityInstrumentID(entry["event_code"])
		}
		if eventID == "" {
			eventID = securityInstrumentID(entry["eventSecurity"])
		}
		seriesID = securityInstrumentID(entry["seriesSecurity"])
		status = stringValue(entry["status"])
		if value, err := strconv.ParseFloat(stringValue(entry["tickSize"]), 64); err == nil && value > 0 {
			tickSize = value
		}
	}
	if code == "" || protocol == "Qot_GetEventContractCategory" ||
		protocol == "Qot_FilterCompetition" ||
		protocol == "Qot_GetEventContractSeriesList" ||
		protocol == "Qot_GetEventContractEventList" {
		return nil
	}
	return &broker.Instrument{
		InstrumentID:  "US." + code,
		Code:          code,
		Name:          name,
		ProductClass:  broker.ProductClassEventContract,
		MarketSegment: broker.MarketSegmentPrediction,
		QuoteMarket:   "US",
		TradeMarket:   "US",
		Venue:         "event_contract",
		PriceTick:     &tickSize,
		QuantityMode:  broker.QuantityModeAmount,
		Event: &broker.EventProduct{
			EventID: eventID, SeriesID: seriesID, ContractID: "US." + code,
			Status: status,
		},
	}
}

func securityInstrumentID(value any) string {
	security, ok := value.(map[string]any)
	if !ok {
		return ""
	}
	return stringValue(security["instrumentId"])
}
