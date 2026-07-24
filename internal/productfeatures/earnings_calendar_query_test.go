package productfeatures

import (
	"errors"
	"testing"

	"github.com/jftrade/jftrade-main/pkg/broker"
	"github.com/stretchr/testify/require"
)

func TestValidateResearchCalendarQueryAcceptsSupportedBusinessParameters(t *testing.T) {
	t.Parallel()

	err := validateResearchCalendarQuery(broker.FeatureQuery{
		FeatureID: broker.FeatureResearchCalendar,
		Market:    "US",
		Params: map[string]any{
			"operation":       "earnings",
			"beginDate":       "2026-07-01",
			"endDate":         "2026-08-08",
			"sort":            "iv_percentile",
			"stockScope":      "watchlist",
			"marketCapMin":    float64(1_000_000_000),
			"optionVolumeMax": int64(20_000),
			"ivMin":           float64(10),
			"ivMax":           float64(80),
			"ivRankMin":       float64(5),
			"ivPercentileMax": float64(100),
		},
	})

	require.NoError(t, err)
}

func TestValidateResearchCalendarQueryRejectsInvalidParameters(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		market string
		params map[string]any
	}{
		{name: "sort", market: "US", params: map[string]any{"sort": "unknown"}},
		{name: "scope", market: "US", params: map[string]any{"stockScope": "mine"}},
		{name: "end without begin", market: "US", params: map[string]any{"endDate": "2026-07-01"}},
		{name: "date format", market: "US", params: map[string]any{"beginDate": "2026/07/01"}},
		{name: "reversed date", market: "US", params: map[string]any{"beginDate": "2026-07-02", "endDate": "2026-07-01"}},
		{name: "range too long", market: "US", params: map[string]any{"beginDate": "2026-07-01", "endDate": "2026-08-12"}},
		{name: "negative", market: "US", params: map[string]any{"marketCapMin": -1}},
		{name: "not finite", market: "US", params: map[string]any{"ivMin": "not-a-number"}},
		{name: "percentage", market: "US", params: map[string]any{"ivMax": 101}},
		{name: "reversed range", market: "US", params: map[string]any{"optionVolumeMin": 20, "optionVolumeMax": 10}},
		{name: "cn sort capability", market: "SH", params: map[string]any{"sort": "iv"}},
		{name: "cn filter capability", market: "SZ", params: map[string]any{"ivRankMin": 1}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			test.params["operation"] = "earnings"
			err := validateResearchCalendarQuery(broker.FeatureQuery{
				FeatureID: broker.FeatureResearchCalendar,
				Market:    test.market,
				Params:    test.params,
			})
			require.Error(t, err)
			require.True(t, errors.Is(err, ErrInvalidQuery), "expected invalid query error, got %v", err)
		})
	}
}

func TestValidateResearchCalendarQueryIgnoresOtherOperations(t *testing.T) {
	t.Parallel()

	err := validateResearchCalendarQuery(broker.FeatureQuery{
		FeatureID: broker.FeatureResearchCalendar,
		Market:    "SH",
		Params: map[string]any{
			"operation": "dividends",
			"sort":      "iv",
		},
	})
	require.NoError(t, err)
}
