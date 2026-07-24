package futu

import (
	"context"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/jftrade/jftrade-main/pkg/broker"
	"github.com/jftrade/jftrade-main/pkg/futu/opend"
)

const (
	earningsCalendarProtocol       = "Qot_GetEarningsCalendar"
	earningsCalendarChunkDays      = 7
	earningsCalendarMaxQueryDays   = 42
	earningsCalendarStockListField = 4
)

type earningsCalendarRangeParam struct {
	minKey        string
	maxKey        string
	indicatorType int
	percentage    bool
	optionOnly    bool
}

type earningsCalendarDateChunk struct {
	begin string
	end   string
}

var earningsCalendarRangeParams = []earningsCalendarRangeParam{
	{minKey: "marketCapMin", maxKey: "marketCapMax", indicatorType: 3},
	{minKey: "optionVolumeMin", maxKey: "optionVolumeMax", indicatorType: 5, optionOnly: true},
	{minKey: "ivMin", maxKey: "ivMax", indicatorType: 6, percentage: true, optionOnly: true},
	{minKey: "ivRankMin", maxKey: "ivRankMax", indicatorType: 7, percentage: true, optionOnly: true},
	{minKey: "ivPercentileMin", maxKey: "ivPercentileMax", indicatorType: 8, percentage: true, optionOnly: true},
}

func translateEarningsCalendarParams(params map[string]any, query broker.FeatureQuery) error {
	optionMarket := strings.EqualFold(query.Market, "US") || strings.EqualFold(query.Market, "HK")

	sortValue := strings.ToLower(strings.TrimSpace(stringValue(params["sort"])))
	delete(params, "sort")
	sortTypes := map[string]int{
		"": 0, "hot": 1, "market_cap": 2, "option_volume": 3,
		"iv": 4, "iv_rank": 5, "iv_percentile": 6,
	}
	sortType, ok := sortTypes[sortValue]
	if !ok {
		return fmt.Errorf("futu: unsupported earnings calendar sort %q", sortValue)
	}
	if sortType > 2 && !optionMarket {
		return fmt.Errorf("futu: earnings calendar sort %q is available only for HK/US", sortValue)
	}
	if sortType > 0 {
		params["sortType"] = sortType
	}

	filters := make([]any, 0, len(earningsCalendarRangeParams)+1)
	scopeValue := strings.ToLower(strings.TrimSpace(stringValue(params["stockScope"])))
	delete(params, "stockScope")
	scopeTypes := map[string]int64{"": 0, "all": 0, "watchlist": 1, "position": 2, "special": 3}
	scopeType, ok := scopeTypes[scopeValue]
	if !ok {
		return fmt.Errorf("futu: unsupported earnings calendar stockScope %q", scopeValue)
	}
	if scopeType > 0 {
		filters = append(filters, earningsCalendarEnumFilter(earningsCalendarStockListField, scopeType))
	}

	for _, definition := range earningsCalendarRangeParams {
		filter, exists, err := earningsCalendarRangeFilter(params, definition, optionMarket)
		delete(params, definition.minKey)
		delete(params, definition.maxKey)
		if err != nil {
			return err
		}
		if exists {
			filters = append(filters, filter)
		}
	}
	if len(filters) > 0 {
		params["filterList"] = filters
	} else {
		delete(params, "filterList")
	}
	return nil
}

func earningsCalendarEnumFilter(indicatorType int, value int64) map[string]any {
	return map[string]any{
		"indicatorType": indicatorType,
		"indicatorValue": map[string]any{
			"valueList": []any{value},
		},
	}
}

func earningsCalendarRangeFilter(
	params map[string]any,
	definition earningsCalendarRangeParam,
	optionMarket bool,
) (map[string]any, bool, error) {
	minValue, hasMin, err := earningsCalendarNumericParam(params[definition.minKey], definition.minKey)
	if err != nil {
		return nil, false, err
	}
	maxValue, hasMax, err := earningsCalendarNumericParam(params[definition.maxKey], definition.maxKey)
	if err != nil {
		return nil, false, err
	}
	if !hasMin && !hasMax {
		return nil, false, nil
	}
	if definition.optionOnly && !optionMarket {
		return nil, false, fmt.Errorf(
			"futu: earnings calendar filters %s/%s are available only for HK/US",
			definition.minKey,
			definition.maxKey,
		)
	}
	if definition.percentage && ((hasMin && minValue > 100) || (hasMax && maxValue > 100)) {
		return nil, false, fmt.Errorf("futu: earnings calendar percentage filters must not exceed 100")
	}
	if hasMin && hasMax && minValue > maxValue {
		return nil, false, fmt.Errorf("futu: earnings calendar filter %s must not exceed %s", definition.minKey, definition.maxKey)
	}

	interval := map[string]any{}
	if hasMin {
		interval["filterMin"] = map[string]any{"value": minValue, "includes": true}
	}
	if hasMax {
		interval["filterMax"] = map[string]any{"value": maxValue, "includes": true}
	}
	return map[string]any{
		"indicatorType": definition.indicatorType,
		"indicatorValue": map[string]any{
			"valueInterval": interval,
		},
	}, true, nil
}

func earningsCalendarNumericParam(raw any, key string) (float64, bool, error) {
	if raw == nil || strings.TrimSpace(stringValue(raw)) == "" && isStringValue(raw) {
		return 0, false, nil
	}
	value, ok := researchNumber(raw)
	if !ok || math.IsNaN(value) || math.IsInf(value, 0) {
		return 0, false, fmt.Errorf("futu: earnings calendar %s must be a finite number", key)
	}
	if value < 0 {
		return 0, false, fmt.Errorf("futu: earnings calendar %s must not be negative", key)
	}
	return value, true, nil
}

func isStringValue(value any) bool {
	_, ok := value.(string)
	return ok
}

func earningsCalendarDateChunks(params map[string]any) ([]earningsCalendarDateChunk, error) {
	beginValue := strings.TrimSpace(stringValue(params["beginDate"]))
	endValue := strings.TrimSpace(stringValue(params["endDate"]))
	if beginValue == "" {
		if endValue != "" {
			return nil, fmt.Errorf("futu: earnings calendar beginDate is required when endDate is provided")
		}
		today := time.Now().Format(time.DateOnly)
		return []earningsCalendarDateChunk{{begin: today, end: today}}, nil
	}
	begin, err := time.Parse(time.DateOnly, beginValue)
	if err != nil {
		return nil, fmt.Errorf("futu: earnings calendar beginDate must use YYYY-MM-DD")
	}
	end := begin
	if endValue != "" {
		end, err = time.Parse(time.DateOnly, endValue)
		if err != nil {
			return nil, fmt.Errorf("futu: earnings calendar endDate must use YYYY-MM-DD")
		}
	}
	if end.Before(begin) {
		return nil, fmt.Errorf("futu: earnings calendar endDate must not precede beginDate")
	}
	if days := int(end.Sub(begin).Hours()/24) + 1; days > earningsCalendarMaxQueryDays {
		return nil, fmt.Errorf("futu: earnings calendar range must not exceed %d days", earningsCalendarMaxQueryDays)
	}

	chunks := make([]earningsCalendarDateChunk, 0, earningsCalendarMaxQueryDays/earningsCalendarChunkDays)
	for cursor := begin; !cursor.After(end); cursor = cursor.AddDate(0, 0, earningsCalendarChunkDays) {
		chunkEnd := cursor.AddDate(0, 0, earningsCalendarChunkDays-1)
		if chunkEnd.After(end) {
			chunkEnd = end
		}
		chunks = append(chunks, earningsCalendarDateChunk{
			begin: cursor.Format(time.DateOnly),
			end:   chunkEnd.Format(time.DateOnly),
		})
	}
	return chunks, nil
}

func (a *futuAdapter) queryEarningsCalendarFeature(
	ctx context.Context,
	query broker.FeatureQuery,
	params map[string]any,
) (*broker.FeatureResult, error) {
	chunks, err := earningsCalendarDateChunks(params)
	if err != nil {
		return nil, err
	}

	var mergedEntries []map[string]any
	if err := a.withAdvancedClient(ctx, earningsCalendarProtocol, func(client *opend.Client) error {
		var collectErr error
		// Retrying the read-only batch replaces, rather than appends to, data
		// gathered by the failed attempt.
		mergedEntries, collectErr = collectEarningsCalendarChunks(
			query,
			params,
			chunks,
			func(chunkParams map[string]any) (map[string]any, error) {
				return client.CallAdvanced(ctx, earningsCalendarProtocol, chunkParams)
			},
		)
		return collectErr
	}); err != nil {
		return nil, err
	}

	mergedEntries = deduplicateEarningsCalendarEntries(mergedEntries)
	result := featureResult(query, mergedEntries, map[string]any{
		"rangeChunks": len(chunks),
		"beginDate":   chunks[0].begin,
		"endDate":     chunks[len(chunks)-1].end,
	})
	total := len(mergedEntries)
	result.Total = &total
	return result, nil
}

func collectEarningsCalendarChunks(
	query broker.FeatureQuery,
	params map[string]any,
	chunks []earningsCalendarDateChunk,
	call func(map[string]any) (map[string]any, error),
) ([]map[string]any, error) {
	entries := make([]map[string]any, 0)
	for _, chunk := range chunks {
		chunkParams := cloneMap(params)
		chunkParams["beginDate"] = chunk.begin
		chunkParams["endDate"] = chunk.end
		payload, err := call(chunkParams)
		if err != nil {
			return nil, err
		}
		result := featureResultFromProtocolPayload(query, earningsCalendarProtocol, payload)
		entries = append(entries, result.Entries...)
	}
	return entries, nil
}

func deduplicateEarningsCalendarEntries(entries []map[string]any) []map[string]any {
	seen := make(map[string]struct{}, len(entries))
	result := make([]map[string]any, 0, len(entries))
	for _, entry := range entries {
		key := strings.Join([]string{
			stringValue(entry["eventDate"]),
			stringValue(entry["instrumentId"]),
			stringValue(entry["symbol"]),
		}, "\x00")
		if key == "\x00\x00" {
			key = fmt.Sprintf("%v", entry)
		}
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, entry)
	}
	return result
}
