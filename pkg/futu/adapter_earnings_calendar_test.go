package futu

import (
	"errors"
	"math"
	"testing"

	"github.com/jftrade/jftrade-main/pkg/broker"
	"github.com/stretchr/testify/require"
)

func TestTranslateEarningsCalendarParamsMapsBusinessSemantics(t *testing.T) {
	t.Parallel()

	params := map[string]any{
		"sort":            "iv_percentile",
		"stockScope":      "watchlist",
		"marketCapMin":    1_000_000_000,
		"optionVolumeMax": 20_000,
		"ivMin":           10,
		"ivMax":           80,
		"ivRankMin":       5,
		"ivPercentileMax": 95,
		"beginDate":       "2026-07-01",
		"endDate":         "2026-07-31",
	}

	err := translateEarningsCalendarParams(params, broker.FeatureQuery{Market: "US"})
	require.NoError(t, err)
	require.Equal(t, 6, params["sortType"])
	require.NotContains(t, params, "sort")
	require.NotContains(t, params, "stockScope")
	require.NotContains(t, params, "marketCapMin")

	filters := params["filterList"].([]any)
	require.Len(t, filters, 6)
	require.Equal(t, earningsCalendarEnumFilter(4, 1), filters[0])
	require.Equal(t, 3, filters[1].(map[string]any)["indicatorType"])
	require.Equal(t, 5, filters[2].(map[string]any)["indicatorType"])
	require.Equal(t, 6, filters[3].(map[string]any)["indicatorType"])
	require.Equal(t, 7, filters[4].(map[string]any)["indicatorType"])
	require.Equal(t, 8, filters[5].(map[string]any)["indicatorType"])

	ivInterval := filters[3].(map[string]any)["indicatorValue"].(map[string]any)["valueInterval"].(map[string]any)
	require.Equal(t, map[string]any{"value": float64(10), "includes": true}, ivInterval["filterMin"])
	require.Equal(t, map[string]any{"value": float64(80), "includes": true}, ivInterval["filterMax"])
	require.Equal(t, "2026-07-01", params["beginDate"])
	require.Equal(t, "2026-07-31", params["endDate"])
}

func TestTranslateEarningsCalendarParamsRejectsUnsupportedMarketConditions(t *testing.T) {
	t.Parallel()

	params := map[string]any{"sort": "iv"}
	require.Error(t, translateEarningsCalendarParams(params, broker.FeatureQuery{Market: "SH"}))

	params = map[string]any{"ivMin": 10}
	require.Error(t, translateEarningsCalendarParams(params, broker.FeatureQuery{Market: "SZ"}))
}

func TestEarningsCalendarDateChunksLimitEveryOpenDCallToSevenDays(t *testing.T) {
	t.Parallel()

	chunks, err := earningsCalendarDateChunks(map[string]any{
		"beginDate": "2026-06-28",
		"endDate":   "2026-08-08",
	})
	require.NoError(t, err)
	require.Equal(t, []earningsCalendarDateChunk{
		{begin: "2026-06-28", end: "2026-07-04"},
		{begin: "2026-07-05", end: "2026-07-11"},
		{begin: "2026-07-12", end: "2026-07-18"},
		{begin: "2026-07-19", end: "2026-07-25"},
		{begin: "2026-07-26", end: "2026-08-01"},
		{begin: "2026-08-02", end: "2026-08-08"},
	}, chunks)
}

func TestEarningsCalendarDateChunksSupportsThirtyFiveDayGridAndRejectsLongerThanFortyTwo(t *testing.T) {
	t.Parallel()

	chunks, err := earningsCalendarDateChunks(map[string]any{
		"beginDate": "2026-02-01",
		"endDate":   "2026-03-07",
	})
	require.NoError(t, err)
	require.Len(t, chunks, 5)

	_, err = earningsCalendarDateChunks(map[string]any{
		"beginDate": "2026-01-01",
		"endDate":   "2026-02-12",
	})
	require.Error(t, err)
}

func TestDeduplicateEarningsCalendarEntriesUsesDateAndSecurity(t *testing.T) {
	t.Parallel()

	entries := []map[string]any{
		{"eventDate": "2026-07-22", "instrumentId": "US.AAPL", "name": "Apple"},
		{"eventDate": "2026-07-22", "instrumentId": "US.AAPL", "name": "Apple duplicate"},
		{"eventDate": "2026-07-23", "instrumentId": "US.AAPL", "name": "Apple next day"},
	}
	result := deduplicateEarningsCalendarEntries(entries)
	require.Len(t, result, 2)
	require.Equal(t, "Apple", result[0]["name"])
	require.Equal(t, "Apple next day", result[1]["name"])
}

func TestCollectEarningsCalendarChunksFailsTheWholeRangeWhenOneChunkFails(t *testing.T) {
	t.Parallel()

	chunks := []earningsCalendarDateChunk{
		{begin: "2026-07-01", end: "2026-07-07"},
		{begin: "2026-07-08", end: "2026-07-14"},
		{begin: "2026-07-15", end: "2026-07-21"},
	}
	callCount := 0
	entries, err := collectEarningsCalendarChunks(
		broker.FeatureQuery{FeatureID: broker.FeatureResearchCalendar, Market: "US"},
		map[string]any{"sortType": 1},
		chunks,
		func(params map[string]any) (map[string]any, error) {
			callCount++
			if callCount == 2 {
				return nil, errors.New("second OpenD segment failed")
			}
			return map[string]any{"itemList": []any{}}, nil
		},
	)

	require.EqualError(t, err, "second OpenD segment failed")
	require.Nil(t, entries)
	require.Equal(t, 2, callCount)
}

func TestCollectEarningsCalendarChunksUsesEveryExactSegmentInOrder(t *testing.T) {
	t.Parallel()

	chunks := []earningsCalendarDateChunk{
		{begin: "2026-07-01", end: "2026-07-07"},
		{begin: "2026-07-08", end: "2026-07-14"},
	}
	captured := make([]earningsCalendarDateChunk, 0, len(chunks))
	entries, err := collectEarningsCalendarChunks(
		broker.FeatureQuery{FeatureID: broker.FeatureResearchCalendar, Market: "US"},
		map[string]any{"sortType": 2},
		chunks,
		func(params map[string]any) (map[string]any, error) {
			captured = append(captured, earningsCalendarDateChunk{
				begin: stringValue(params["beginDate"]),
				end:   stringValue(params["endDate"]),
			})
			require.Equal(t, 2, params["sortType"])
			return map[string]any{"itemList": []any{}}, nil
		},
	)

	require.NoError(t, err)
	require.Empty(t, entries)
	require.Equal(t, chunks, captured)
}

func TestEarningsCalendarParameterValidationEdges(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name   string
		params map[string]any
	}{
		{name: "sort", params: map[string]any{"sort": "unknown"}},
		{name: "scope", params: map[string]any{"stockScope": "unknown"}},
		{name: "invalid min", params: map[string]any{"marketCapMin": "many"}},
		{name: "invalid max", params: map[string]any{"marketCapMax": math.Inf(1)}},
		{name: "negative", params: map[string]any{"marketCapMin": -1}},
		{name: "percentage", params: map[string]any{"ivMax": 101}},
		{name: "reversed", params: map[string]any{"marketCapMin": 2, "marketCapMax": 1}},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			require.Error(t, translateEarningsCalendarParams(testCase.params, broker.FeatureQuery{Market: "US"}))
		})
	}

	params := map[string]any{"marketCapMin": "", "marketCapMax": nil, "filterList": []any{"stale"}}
	require.NoError(t, translateEarningsCalendarParams(params, broker.FeatureQuery{Market: "US"}))
	require.NotContains(t, params, "filterList")
}

func TestEarningsCalendarDateValidationEdges(t *testing.T) {
	t.Parallel()

	defaultChunks, err := earningsCalendarDateChunks(map[string]any{})
	require.NoError(t, err)
	require.Len(t, defaultChunks, 1)
	require.Equal(t, defaultChunks[0].begin, defaultChunks[0].end)

	testCases := []map[string]any{
		{"endDate": "2026-07-01"},
		{"beginDate": "07/01/2026"},
		{"beginDate": "2026-07-01", "endDate": "07/02/2026"},
		{"beginDate": "2026-07-02", "endDate": "2026-07-01"},
	}
	for _, params := range testCases {
		_, err := earningsCalendarDateChunks(params)
		require.Error(t, err)
	}

	chunks, err := earningsCalendarDateChunks(map[string]any{
		"beginDate": "2026-07-01",
		"endDate":   "2026-07-08",
	})
	require.NoError(t, err)
	require.Equal(t, "2026-07-08", chunks[1].end)
}

func TestDeduplicateEarningsCalendarEntriesFallsBackForAnonymousRows(t *testing.T) {
	t.Parallel()

	entries := []map[string]any{{"name": "anonymous"}, {"name": "anonymous"}, {"name": "other"}}
	require.Len(t, deduplicateEarningsCalendarEntries(entries), 2)
}
