package futu

import (
	"encoding/json"
	"fmt"
	"maps"
	"strconv"
	"strings"

	"github.com/jftrade/jftrade-main/pkg/broker"
	"github.com/jftrade/jftrade-main/pkg/researchscreen"
)

const researchScreenDefinitionParam = "researchScreenDefinition"

func translateResearchScreenParams(params map[string]any, query broker.FeatureQuery) error {
	raw, exists := params[researchScreenDefinitionParam]
	if !exists {
		return fmt.Errorf("futu: stock screen definition v2 is required")
	}
	delete(params, researchScreenDefinitionParam)
	definition, err := decodeResearchScreenDefinition(raw)
	if err != nil {
		return err
	}
	market := strings.ToUpper(strings.TrimSpace(definition.Market))
	if market == "" {
		market = strings.ToUpper(strings.TrimSpace(query.Market))
	}
	marketValue, err := researchScreenMarketValue(market)
	if err != nil {
		return err
	}
	filters, err := translateResearchScreenFilters(definition, market, marketValue)
	if err != nil {
		return err
	}
	retrieve, err := translateResearchScreenRetrieve(definition, market)
	if err != nil {
		return err
	}
	sortList, err := translateResearchScreenSorts(definition, market)
	if err != nil {
		return err
	}
	params["filterList"] = filters
	params["retrieveList"] = retrieve
	params["watchlistStockIds"] = definition.Pool.WatchlistStockIDs
	if len(sortList) > 0 {
		params["sortList"] = sortList
	}
	return nil
}

func translateResearchScreenFilters(
	definition broker.ScreenDefinitionV2,
	market string,
	marketValue int64,
) ([]any, error) {
	filters := make([]any, 0, len(definition.Conditions)+2)
	hasMarket := false
	for index, condition := range definition.Conditions {
		if _, err := researchscreen.ValidateFactorForMarket(condition.Factor.FactorKey, market, true, false, false); err != nil {
			return nil, fmt.Errorf("futu: stock screen filter %d: %w", index, err)
		}
		translated, factor, err := translateResearchScreenCondition(condition)
		if err != nil {
			return nil, fmt.Errorf("futu: stock screen filter %d: %w", index, err)
		}
		if factor.Key == "field.market" {
			hasMarket = true
			values := researchScreenConditionValues(condition.Value)
			if len(values) != 1 || values[0] != marketValue {
				return nil, fmt.Errorf("market factor must match request market %s", market)
			}
		}
		filters = append(filters, translated)
	}
	if !hasMarket {
		filters = append([]any{map[string]any{
			"simpleFieldQuery": map[string]any{
				"simpleField": int32(1), "screenValueList": []int64{marketValue},
			},
		}}, filters...)
	}
	if len(definition.Pool.Plates) > 0 {
		plates := make([]any, 0, len(definition.Pool.Plates))
		for index, plate := range definition.Pool.Plates {
			ids := cleanStrings(plate.PlateIDs)
			if len(ids) == 0 {
				return nil, fmt.Errorf("futu: stock screen plate group %d has no plate ids", index)
			}
			plates = append(plates, map[string]any{
				"parentPlateId": strings.TrimSpace(plate.ParentPlateID),
				"plateIdList":   ids,
			})
		}
		filters = append(filters, map[string]any{
			"plateQuery": map[string]any{"plateList": plates},
		})
	}
	return filters, nil
}

func translateResearchScreenRetrieve(
	definition broker.ScreenDefinitionV2,
	market string,
) ([]any, error) {
	retrieve := make([]any, 0, len(definition.Columns))
	seen := make(map[string]struct{}, len(definition.Columns))
	for index, column := range definition.Columns {
		if _, err := researchscreen.ValidateFactorForMarket(column.Factor.FactorKey, market, false, true, false); err != nil {
			return nil, fmt.Errorf("futu: stock screen column %d: %w", index, err)
		}
		key := researchScreenFactorRefKey(column.Factor)
		if _, exists := seen[key]; exists {
			continue
		}
		translated, _, err := translateResearchScreenProperty(column.Factor, true, false)
		if err != nil {
			return nil, fmt.Errorf("futu: stock screen column %d: %w", index, err)
		}
		seen[key] = struct{}{}
		retrieve = append(retrieve, translated)
	}
	return retrieve, nil
}

func translateResearchScreenSorts(
	definition broker.ScreenDefinitionV2,
	market string,
) ([]any, error) {
	sortList := make([]any, 0, len(definition.Sorts))
	for index, value := range definition.Sorts {
		if _, err := researchscreen.ValidateFactorForMarket(value.Factor.FactorKey, market, false, false, true); err != nil {
			return nil, fmt.Errorf("futu: stock screen sort %d: %w", index, err)
		}
		property, _, err := translateResearchScreenProperty(value.Factor, false, true)
		if err != nil {
			return nil, fmt.Errorf("futu: stock screen sort %d: %w", index, err)
		}
		direction, err := researchScreenSortDirection(value.Direction)
		if err != nil {
			return nil, fmt.Errorf("futu: stock screen sort %d: %w", index, err)
		}
		sortValue := map[string]any{"direction": direction}
		maps.Copy(sortValue, property)
		sortList = append(sortList, sortValue)
	}
	return sortList, nil
}

func decodeResearchScreenDefinition(value any) (broker.ScreenDefinitionV2, error) {
	if typed, ok := value.(broker.ScreenDefinitionV2); ok {
		definition, err := researchscreen.NormalizeDefinitionV2(typed)
		if err != nil {
			return broker.ScreenDefinitionV2{}, fmt.Errorf("futu: invalid stock screen definition v2: %w", err)
		}
		return definition, nil
	}
	content, err := json.Marshal(value)
	if err != nil {
		return broker.ScreenDefinitionV2{}, fmt.Errorf("futu: encode stock screen definition v2: %w", err)
	}
	var definition broker.ScreenDefinitionV2
	if err := json.Unmarshal(content, &definition); err != nil {
		return broker.ScreenDefinitionV2{}, fmt.Errorf("futu: invalid stock screen definition v2: %w", err)
	}
	normalized, err := researchscreen.NormalizeDefinitionV2(definition)
	if err != nil {
		return broker.ScreenDefinitionV2{}, fmt.Errorf("futu: invalid stock screen definition v2: %w", err)
	}
	return normalized, nil
}

func researchScreenMarketValue(market string) (int64, error) {
	switch strings.ToUpper(strings.TrimSpace(market)) {
	case "HK":
		return 1, nil
	case "US":
		return 2, nil
	case "SH", "SZ", "CN":
		return 3, nil
	default:
		return 0, fmt.Errorf("futu: stock screen does not support market %q", market)
	}
}

func translateResearchScreenCondition(
	condition broker.ScreenCondition,
) (map[string]any, researchscreen.FactorDescriptor, error) {
	factor, err := researchscreen.ValidateFactorUse(condition.Factor.FactorKey, true, false, false)
	if err != nil {
		return nil, factor, err
	}
	value := researchScreenConditionObject(condition.Value)
	intervals := researchScreenConditionIntervals(condition.Value)
	switch factor.Category {
	case "field":
		values := researchScreenConditionValues(condition.Value)
		if len(values) == 0 {
			return nil, factor, fmt.Errorf("factor %q requires values", factor.Key)
		}
		return map[string]any{"simpleFieldQuery": map[string]any{
			"simpleField": factor.ProviderID, "screenValueList": values,
		}}, factor, nil
	case "simple":
		return map[string]any{"simplePropertyQuery": map[string]any{
			"property":  researchScreenProperty(factor, condition.Factor.Params),
			"filterMin": researchScreenConditionBoundary(value, "min"),
			"filterMax": researchScreenConditionBoundary(value, "max"),
		}}, factor, nil
	case "cumulative":
		return map[string]any{"cumulativePropertyQuery": map[string]any{
			"property":         researchScreenProperty(factor, condition.Factor.Params),
			"filterMin":        researchScreenConditionBoundary(value, "min"),
			"filterMax":        researchScreenConditionBoundary(value, "max"),
			"continuousPeriod": int32Value(value["continuousPeriod"]),
		}}, factor, nil
	case "financial":
		return map[string]any{"financialPropertyQuery": map[string]any{
			"property":         researchScreenProperty(factor, condition.Factor.Params),
			"filterMin":        researchScreenConditionBoundary(value, "min"),
			"filterMax":        researchScreenConditionBoundary(value, "max"),
			"continuousPeriod": int32Value(value["continuousPeriod"]),
		}}, factor, nil
	case "indicator":
		return translateIndicatorResearchScreenCondition(condition, factor, value, intervals)
	case "pattern":
		return translatePatternResearchScreenCondition(condition, factor, value)
	case "featured":
		return map[string]any{"featuredPropertyQuery": map[string]any{
			"property":  researchScreenProperty(factor, condition.Factor.Params),
			"intervals": intervals, "valueSet": researchScreenConditionValues(condition.Value),
		}}, factor, nil
	case "broker":
		return map[string]any{"brokerHoldingsQuery": map[string]any{
			"property": researchScreenProperty(factor, condition.Factor.Params), "intervals": intervals,
		}}, factor, nil
	case "option":
		return map[string]any{"optionQuery": map[string]any{
			"property": researchScreenProperty(factor, condition.Factor.Params), "intervals": intervals,
		}}, factor, nil
	case "kline_shape":
		values := researchScreenConditionValues(condition.Value)
		if len(values) == 0 {
			return nil, factor, fmt.Errorf("kline shape factor %q requires values", factor.Key)
		}
		return map[string]any{"klineShapeQuery": map[string]any{
			"property": researchScreenProperty(factor, condition.Factor.Params), "valueSet": values,
		}}, factor, nil
	default:
		return nil, factor, fmt.Errorf("unsupported factor category %q", factor.Category)
	}
}

func translateIndicatorResearchScreenCondition(
	condition broker.ScreenCondition,
	factor researchscreen.FactorDescriptor,
	value map[string]any,
	intervals []any,
) (map[string]any, researchscreen.FactorDescriptor, error) {
	position := int32Value(value["position"])
	if position < 1 || position > 4 {
		return nil, factor, fmt.Errorf("indicator factor %q requires position 1..4", factor.Key)
	}
	period := condition.Factor.Params.Period
	if period == 0 {
		period = 11
	}
	query := map[string]any{
		"position": position,
		"period":   period, "periodType": period,
		"firstIndicator": factor.ProviderID, "firstIndicatorName": factor.ProviderID,
		"firstIndicatorParams": condition.Factor.Params.IndicatorParams,
		"continuousPeriod":     int32Value(value["continuousPeriod"]),
		"intervals":            intervals,
	}
	if condition.SecondFactor != nil {
		second, err := researchscreen.ValidateFactorUse(condition.SecondFactor.FactorKey, true, false, false)
		if err != nil || second.Category != "indicator" {
			return nil, factor, fmt.Errorf("second factor must be an indicator")
		}
		query["secondIndicator"] = second.ProviderID
		query["secondIndicatorParams"] = condition.SecondFactor.Params.IndicatorParams
	}
	if secondValue, ok := int64Value(value["secondValue"]); ok {
		query["secondValue"] = secondValue
	}
	return map[string]any{"indicatorPositionalQuery": query}, factor, nil
}

func translatePatternResearchScreenCondition(
	condition broker.ScreenCondition,
	factor researchscreen.FactorDescriptor,
	value map[string]any,
) (map[string]any, researchscreen.FactorDescriptor, error) {
	period := condition.Factor.Params.Period
	if period == 0 {
		period = 11
	}
	match := true
	if raw, exists := value["match"]; exists {
		match, _ = raw.(bool)
	}
	return map[string]any{"indicatorPatternQuery": map[string]any{
		"pattern": factor.ProviderID, "name": factor.ProviderID,
		"period": period, "periodType": period, "isMatching": match,
		"continuousPeriod": int32Value(value["continuousPeriod"]),
		"subPatterns":      researchScreenConditionValues(value["values"]),
	}}, factor, nil
}

func translateResearchScreenProperty(
	ref broker.FactorRef,
	retrieve bool,
	sort bool,
) (map[string]any, researchscreen.FactorDescriptor, error) {
	factor, err := researchscreen.ValidateFactorUse(ref.FactorKey, false, retrieve, sort)
	if err != nil {
		return nil, factor, err
	}
	field := map[string]string{
		"basic": "basicProperty", "simple": "simpleProperty",
		"cumulative": "cumulativeProperty", "financial": "financialProperty",
		"indicator": "indicatorProperty", "featured": "featuredProperty",
		"broker": "brokerProperty", "option": "optionProperty",
		"kline_shape": "klineShapeProperty",
	}[factor.Category]
	if field == "" {
		return nil, factor, fmt.Errorf("factor %q is not a property", factor.Key)
	}
	return map[string]any{field: researchScreenProperty(factor, ref.Params)}, factor, nil
}

func researchScreenProperty(
	factor researchscreen.FactorDescriptor,
	params broker.ResearchScreenFactorParams,
) map[string]any {
	value := map[string]any{"name": factor.ProviderID}
	switch factor.Category {
	case "cumulative":
		days := params.Days
		if days == 0 {
			days = 1
		}
		value["days"] = days
		value["periodAverage"] = params.PeriodAverage
	case "financial":
		value["term"] = params.Term
		value["duration"] = params.Duration
		value["year"] = params.Year
		value["periodAverage"] = params.PeriodAverage
		value["futureDuration"] = params.FutureDuration
	case "indicator":
		period := params.Period
		if period == 0 {
			period = 11
		}
		value["period"] = period
		value["indicatorParams"] = params.IndicatorParams
	case "featured":
		value["period"] = params.Period
		value["rangePeriod"] = params.RangePeriod
		value["firstCustomParam"] = params.FirstCustomParam
	case "broker":
		value["days"] = params.Days
		value["param"] = params.BrokerParam
	case "option":
		value["period"] = params.OptionHVPeriod
		if params.OptionParamType != 0 {
			value["param"] = map[string]any{
				"type": params.OptionParamType, "sval": params.OptionParamString,
				"ival": params.OptionParamInteger, "aval": params.OptionParamIntegers,
			}
		}
	case "kline_shape":
		value["period"] = params.Period
	}
	return value
}

func researchScreenConditionObject(value any) map[string]any {
	result, _ := value.(map[string]any)
	return result
}

func researchScreenConditionBoundary(value map[string]any, key string) any {
	number, ok := float64Value(value[key])
	if !ok {
		if integer, integerOK := int64Value(value[key]); integerOK {
			number, ok = float64(integer), true
		}
	}
	if !ok {
		return nil
	}
	includes := true
	if flag, exists := value[key+"Includes"].(bool); exists {
		includes = flag
	}
	return map[string]any{"value": number, "includes": includes}
}

func researchScreenConditionIntervals(value any) []any {
	object := researchScreenConditionObject(value)
	rawIntervals, _ := object["intervals"].([]any)
	if len(rawIntervals) == 0 && (object["min"] != nil || object["max"] != nil) {
		rawIntervals = []any{object}
	}
	result := make([]any, 0, len(rawIntervals))
	for _, raw := range rawIntervals {
		interval := researchScreenConditionObject(raw)
		result = append(result, map[string]any{
			"filterMin": researchScreenConditionBoundary(interval, "min"),
			"filterMax": researchScreenConditionBoundary(interval, "max"),
			"unit":      int32Value(interval["unit"]),
		})
	}
	return result
}

func researchScreenConditionValues(value any) []int64 {
	switch typed := value.(type) {
	case []int64:
		return append([]int64(nil), typed...)
	case []any:
		return int64Slice(typed)
	default:
		return nil
	}
}

func int32Value(value any) int32 {
	number, _ := int64Value(value)
	return int32(number)
}

func researchScreenFactorRefKey(ref broker.FactorRef) string {
	content, _ := json.Marshal(ref.Params)
	return strings.ToLower(strings.TrimSpace(ref.FactorKey)) + ":" + string(content)
}

func researchScreenSortDirection(value string) (int32, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "asc":
		return 1, nil
	case "desc", "":
		return 2, nil
	case "abs_asc":
		return 3, nil
	case "abs_desc":
		return 4, nil
	default:
		return 0, fmt.Errorf("unsupported sort direction %q", value)
	}
}

func cleanStrings(values []string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			result = append(result, value)
		}
	}
	return result
}

func stockScreenOffset(cursor string) int {
	value, _ := strconv.Atoi(strings.TrimSpace(cursor))
	return max(value, 0)
}
