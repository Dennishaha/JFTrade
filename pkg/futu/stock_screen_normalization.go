package futu

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/jftrade/jftrade-main/pkg/broker"
	"github.com/jftrade/jftrade-main/pkg/futu/opend"
	qotcommonpb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotcommon"
	"github.com/jftrade/jftrade-main/pkg/researchscreen"
)

var stockScreenResultKinds = []struct {
	Field    string
	Category string
}{
	{Field: "basicPropertyResult", Category: "basic"},
	{Field: "simplePropertyResult", Category: "simple"},
	{Field: "cumulativePropertyResult", Category: "cumulative"},
	{Field: "financialPropertyResult", Category: "financial"},
	{Field: "indicatorPropertyResult", Category: "indicator"},
	{Field: "featuredPropertyResult", Category: "featured"},
	{Field: "brokerPropertyResult", Category: "broker"},
	{Field: "optionPropertyResult", Category: "option"},
	{Field: "klineShapePropertyResult", Category: "kline_shape"},
}

const exactMainlandStockScreenTotalWarning = "OpenD reports only a combined A-share total; total is omitted for exact SH/SZ results."

func stockScreenFeatureResult(
	query broker.FeatureQuery,
	payload map[string]any,
) *broker.FeatureResult {
	entries := make([]map[string]any, 0)
	rawEntries := objectSlice(payload["dataList"])
	for _, raw := range rawEntries {
		entry := normalizeStockScreenRow(query, raw)
		if stockScreenMarketMatches(query.Market, stringValue(entry["market"])) {
			entries = append(entries, entry)
		}
	}
	metadata := cloneMap(payload)
	delete(metadata, "dataList")
	result := featureResult(query, entries, metadata)
	total := len(entries)
	if value, ok := integerValue(payload["allCount"]); ok {
		total = value
	}
	if isExactMainlandStockScreenMarket(query.Market) {
		result.Warnings = append(result.Warnings, exactMainlandStockScreenTotalWarning)
	} else {
		result.Total = &total
	}
	lastPage, _ := integerValue(payload["lastPage"])
	offset := stockScreenOffset(query.Cursor)
	hasMore := lastPage == 0 && offset+len(rawEntries) < total
	result.HasMore = &hasMore
	if hasMore {
		result.NextCursor = strconv.Itoa(offset + len(rawEntries))
	}
	return result
}

func normalizeStockScreenRow(
	query broker.FeatureQuery,
	raw map[string]any,
) map[string]any {
	stockID := uint64String(raw["stockId"])
	cells := make(map[string]broker.ScreenResultCell)
	symbol := ""
	name := ""
	industry := ""
	for _, rawResult := range objectSlice(raw["results"]) {
		for _, kind := range stockScreenResultKinds {
			value, ok := rawResult[kind.Field].(map[string]any)
			if !ok {
				continue
			}
			property, _ := value["property"].(map[string]any)
			providerID, ok := integerValue(property["name"])
			if !ok {
				continue
			}
			factor, ok := researchscreen.LookupProvider(kind.Category, int32(providerID))
			if !ok {
				continue
			}
			normalized := normalizeStockScreenValue(factor, value)
			instanceID := factor.Key
			columnID := factor.Key
			if ref, requestedColumnID, ok := stockScreenRequestedRef(query, factor, property); ok {
				if strings.TrimSpace(ref.InstanceID) != "" {
					instanceID = ref.InstanceID
				}
				if strings.TrimSpace(requestedColumnID) != "" {
					columnID = requestedColumnID
				}
			}
			cells[columnID] = broker.ScreenResultCell{
				ColumnID: columnID, InstanceID: instanceID, FactorKey: factor.Key, Value: normalized,
			}
			switch factor.Key {
			case "basic.code":
				symbol = researchScreenString(normalized)
			case "basic.name":
				name = researchScreenString(normalized)
			case "basic.industry":
				industry = researchScreenString(normalized)
			}
			break
		}
	}
	market := ""
	instrumentID := ""
	if security, ok := raw["security"].(map[string]any); ok {
		market = strings.ToUpper(stringValue(security["market"]))
		if code := strings.ToUpper(stringValue(security["code"])); code != "" {
			symbol = code
		}
		instrumentID = stringValue(security["instrumentId"])
	}
	requestMarket := strings.ToUpper(strings.TrimSpace(query.Market))
	if market == "" && requestMarket != "SH" && requestMarket != "SZ" && requestMarket != "CN" {
		market = requestMarket
	}
	if instrumentID == "" && symbol != "" && market != "" {
		instrumentID = market + "." + strings.ToUpper(symbol)
	}
	row := map[string]any{
		"stockId": stockID, "instrumentId": instrumentID,
		"market": market, "symbol": strings.ToUpper(symbol),
		"name": name, "industry": industry,
		"productClass": broker.ProductClassEquity,
		"cells":        cells,
	}
	if quoteCurrency := researchScreenQuoteCurrency(market, symbol, name); quoteCurrency != "" {
		row["quoteCurrency"] = quoteCurrency
	}
	return row
}

func stockScreenRequestedRef(
	query broker.FeatureQuery,
	factor researchscreen.FactorDescriptor,
	property map[string]any,
) (broker.FactorRef, string, bool) {
	if query.Params == nil {
		return broker.FactorRef{}, "", false
	}
	propertyRef := broker.FactorRef{FactorKey: factor.Key, Params: researchScreenParamsFromProperty(factor, property)}
	type candidate struct {
		ref      broker.FactorRef
		columnID string
	}
	candidates := make([]candidate, 0)
	definition, err := decodeResearchScreenDefinition(query.Params[researchScreenDefinitionParam])
	if err != nil {
		return broker.FactorRef{}, "", false
	}
	for _, column := range definition.Columns {
		if strings.EqualFold(strings.TrimSpace(column.Factor.FactorKey), factor.Key) {
			candidates = append(candidates, candidate{
				ref:      column.Factor,
				columnID: column.ID,
			})
		}
	}
	propertyKey := researchScreenFactorRefKey(propertyRef)
	for _, candidate := range candidates {
		if researchScreenFactorRefKey(candidate.ref) == propertyKey {
			return candidate.ref, candidate.columnID, true
		}
	}
	if len(candidates) == 1 {
		return candidates[0].ref, candidates[0].columnID, true
	}
	return broker.FactorRef{}, "", false
}

func researchScreenParamsFromProperty(
	factor researchscreen.FactorDescriptor,
	property map[string]any,
) broker.ResearchScreenFactorParams {
	integer := func(key string) int64 {
		value, _ := int64Value(property[key])
		return value
	}
	params := broker.ResearchScreenFactorParams{}
	switch factor.Category {
	case "cumulative":
		params.Days = int32(integer("days"))
		params.PeriodAverage = int32(integer("periodAverage"))
	case "financial":
		params.Term = int32(integer("term"))
		params.Duration = integer("duration")
		params.Year = int32(integer("year"))
		params.PeriodAverage = int32(integer("periodAverage"))
		params.FutureDuration = int32(integer("futureDuration"))
	case "indicator":
		params.Period = int32(integer("period"))
		params.IndicatorParams = int64Slice(property["indicatorParams"])
	case "featured":
		params.Period = int32(integer("period"))
		params.RangePeriod = int32(integer("rangePeriod"))
		params.FirstCustomParam = integer("firstCustomParam")
	case "broker":
		params.Days = int32(integer("days"))
		params.BrokerParam = stringValue(property["param"])
	case "option":
		params.OptionHVPeriod = int32(integer("period"))
	case "kline_shape":
		params.Period = int32(integer("period"))
	}
	return params
}

func researchScreenQuoteCurrency(market, symbol, name string) string {
	market = strings.ToUpper(strings.TrimSpace(market))
	symbol = strings.ToUpper(strings.TrimSpace(symbol))
	name = strings.ToUpper(strings.TrimSpace(name))
	if symbol == "" {
		return ""
	}
	switch market {
	case "US":
		return "USD"
	case "SH", "SZ":
		return "CNY"
	case "HK":
		if len(symbol) != 5 || name == "" {
			return ""
		}
		rmbCode := strings.HasPrefix(symbol, "8")
		rmbName := strings.HasSuffix(name, "-R")
		switch {
		case rmbCode && rmbName:
			return "CNY"
		case strings.HasPrefix(symbol, "0") && !rmbName:
			return "HKD"
		default:
			return ""
		}
	default:
		return ""
	}
}

func stockScreenMarketMatches(requestMarket, actualMarket string) bool {
	requestMarket = strings.ToUpper(strings.TrimSpace(requestMarket))
	actualMarket = strings.ToUpper(strings.TrimSpace(actualMarket))
	switch requestMarket {
	case "":
		return true
	case "CN":
		return actualMarket == "SH" || actualMarket == "SZ"
	default:
		return requestMarket == actualMarket
	}
}

func resolveStockScreenIdentities(
	ctx context.Context,
	client *opend.Client,
	query broker.FeatureQuery,
	payload map[string]any,
) error {
	if !isMainlandStockScreenMarket(query.Market) {
		return nil
	}
	rows := objectSlice(payload["dataList"])
	if len(rows) == 0 {
		return nil
	}
	securities := stockScreenIdentityCandidates(rows)
	if len(securities) == 0 {
		return fmt.Errorf("futu: stock screen returned mainland rows without security codes")
	}
	staticInfo, err := client.GetStaticInfo(ctx, securities)
	if err != nil {
		return fmt.Errorf("futu: resolve stock screen security identities: %w", err)
	}
	return applyStockScreenIdentities(rows, staticInfo)
}

func isMainlandStockScreenMarket(market string) bool {
	switch strings.ToUpper(strings.TrimSpace(market)) {
	case "SH", "SZ", "CN":
		return true
	default:
		return false
	}
}

func isExactMainlandStockScreenMarket(market string) bool {
	switch strings.ToUpper(strings.TrimSpace(market)) {
	case "SH", "SZ":
		return true
	default:
		return false
	}
}

func stockScreenIdentityCandidates(rows []map[string]any) []*qotcommonpb.Security {
	codes := make(map[string]struct{}, len(rows))
	result := make([]*qotcommonpb.Security, 0, len(rows)*2)
	for _, row := range rows {
		code := stockScreenRowCode(row)
		if code == "" {
			continue
		}
		if _, exists := codes[code]; exists {
			continue
		}
		codes[code] = struct{}{}
		for _, market := range []qotcommonpb.QotMarket{
			qotcommonpb.QotMarket_QotMarket_CNSH_Security,
			qotcommonpb.QotMarket_QotMarket_CNSZ_Security,
		} {
			marketValue := int32(market)
			codeValue := code
			result = append(result, &qotcommonpb.Security{
				Market: &marketValue,
				Code:   &codeValue,
			})
		}
	}
	return result
}

func stockScreenRowCode(row map[string]any) string {
	for _, rawResult := range objectSlice(row["results"]) {
		value, ok := rawResult["basicPropertyResult"].(map[string]any)
		if !ok {
			continue
		}
		property, _ := value["property"].(map[string]any)
		providerID, ok := integerValue(property["name"])
		if ok && providerID == 1101 {
			return strings.ToUpper(strings.TrimSpace(stringValue(value["sval"])))
		}
	}
	return ""
}

func applyStockScreenIdentities(
	rows []map[string]any,
	staticInfo []*qotcommonpb.SecurityStaticInfo,
) error {
	identities := make(map[string]map[string]any, len(staticInfo))
	for _, info := range staticInfo {
		if info == nil || info.GetBasic() == nil || info.GetBasic().GetId() == 0 {
			continue
		}
		security := info.GetBasic().GetSecurity()
		if security == nil {
			continue
		}
		instrumentID, err := futuSymbolFromSecurity(security)
		if err != nil {
			continue
		}
		market, code, found := strings.Cut(instrumentID, ".")
		if !found || market == "" || code == "" {
			continue
		}
		stockID := strconv.FormatUint(uint64(info.GetBasic().GetId()), 10)
		identities[stockID] = map[string]any{
			"market":       market,
			"code":         code,
			"instrumentId": instrumentID,
		}
	}

	unresolved := 0
	for _, row := range rows {
		stockID := uint64String(row["stockId"])
		identity, ok := identities[stockID]
		if !ok {
			unresolved++
			continue
		}
		row["security"] = cloneMap(identity)
	}
	if unresolved > 0 {
		return fmt.Errorf(
			"futu: could not resolve %d of %d mainland stock screen identities",
			unresolved,
			len(rows),
		)
	}
	return nil
}

func normalizeStockScreenValue(
	factor researchscreen.FactorDescriptor,
	value map[string]any,
) broker.ResearchScreenValue {
	result := broker.ResearchScreenValue{
		Type: "missing", Unit: factor.Unit,
		EnumType: stringValue(value["enumTypeName"]),
		EnumName: stringValue(value["enumName"]),
	}
	if endTime, ok := int64Value(value["endTime"]); ok {
		result.EndTime = &endTime
	}
	valueType, _ := integerValue(value["valueType"])
	switch valueType {
	case 1:
		if _, exists := value["sval"]; exists {
			text := stringValue(value["sval"])
			result.Type, result.String = "string", &text
		}
	case 2:
		if number, ok := int64Value(value["ival"]); ok {
			result.Type, result.Integer = "integer", &number
		}
	case 3:
		if numbers := int64Slice(value["aval"]); numbers != nil {
			result.Type, result.Integers = "integer_array", numbers
		}
	case 4:
		if number, ok := float64Value(value["dval"]); ok {
			result.Type, result.Number = "number", &number
		}
	}
	return result
}

func uint64String(value any) string {
	switch typed := value.(type) {
	case string:
		if _, err := strconv.ParseUint(typed, 10, 64); err == nil {
			return typed
		}
		return ""
	case float64:
		return strconv.FormatUint(uint64(typed), 10)
	case uint64:
		return strconv.FormatUint(typed, 10)
	default:
		return ""
	}
}

func int64Value(value any) (int64, bool) {
	switch typed := value.(type) {
	case string:
		parsed, err := strconv.ParseInt(typed, 10, 64)
		return parsed, err == nil
	case float64:
		return int64(typed), true
	case int:
		return int64(typed), true
	case int32:
		return int64(typed), true
	case int64:
		return typed, true
	default:
		return 0, false
	}
}

func float64Value(value any) (float64, bool) {
	switch typed := value.(type) {
	case float64:
		return typed, true
	case string:
		parsed, err := strconv.ParseFloat(typed, 64)
		return parsed, err == nil
	default:
		return 0, false
	}
}

func int64Slice(value any) []int64 {
	raw, ok := value.([]any)
	if !ok {
		return nil
	}
	result := make([]int64, 0, len(raw))
	for _, value := range raw {
		if number, ok := int64Value(value); ok {
			result = append(result, number)
		}
	}
	return result
}

func researchScreenString(value broker.ResearchScreenValue) string {
	if value.String != nil {
		return *value.String
	}
	if value.Integer != nil {
		return strconv.FormatInt(*value.Integer, 10)
	}
	if value.Number != nil {
		return strconv.FormatFloat(*value.Number, 'f', -1, 64)
	}
	content, _ := json.Marshal(value.Integers)
	return string(content)
}
