package productfeatures

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/jftrade/jftrade-main/pkg/broker"
)

const earningsCalendarMaxRangeDays = 42

type earningsCalendarRangeRule struct {
	minKey     string
	maxKey     string
	percentage bool
	optionOnly bool
}

var earningsCalendarRangeRules = []earningsCalendarRangeRule{
	{minKey: "marketCapMin", maxKey: "marketCapMax"},
	{minKey: "optionVolumeMin", maxKey: "optionVolumeMax", optionOnly: true},
	{minKey: "ivMin", maxKey: "ivMax", percentage: true, optionOnly: true},
	{minKey: "ivRankMin", maxKey: "ivRankMax", percentage: true, optionOnly: true},
	{minKey: "ivPercentileMin", maxKey: "ivPercentileMax", percentage: true, optionOnly: true},
}

func validateResearchCalendarQuery(query broker.FeatureQuery) error {
	if query.FeatureID != broker.FeatureResearchCalendar {
		return nil
	}

	operation := strings.ToLower(strings.TrimSpace(stringParam(query.Params, "operation")))
	if operation != "" && operation != "earnings" {
		return nil
	}

	market := strings.ToUpper(strings.TrimSpace(query.Market))
	optionMarket := market == "" || market == "US" || market == "HK"
	sortValue := strings.ToLower(strings.TrimSpace(stringParam(query.Params, "sort")))
	switch sortValue {
	case "", "hot", "market_cap":
	case "option_volume", "iv", "iv_rank", "iv_percentile":
		if !optionMarket {
			return fmt.Errorf("%w: sort %q is not supported for market %s", ErrInvalidQuery, sortValue, market)
		}
	default:
		return fmt.Errorf("%w: unsupported earnings calendar sort %q", ErrInvalidQuery, sortValue)
	}

	scope := strings.ToLower(strings.TrimSpace(stringParam(query.Params, "stockScope")))
	switch scope {
	case "", "all", "watchlist", "position", "special":
	default:
		return fmt.Errorf("%w: unsupported earnings calendar stockScope %q", ErrInvalidQuery, scope)
	}

	if err := validateEarningsCalendarDates(query.Params); err != nil {
		return err
	}

	for _, rule := range earningsCalendarRangeRules {
		if rule.optionOnly && !optionMarket && (hasNonEmptyParam(query.Params, rule.minKey) || hasNonEmptyParam(query.Params, rule.maxKey)) {
			return fmt.Errorf("%w: %s/%s are not supported for market %s", ErrInvalidQuery, rule.minKey, rule.maxKey, market)
		}
		minValue, hasMin, err := earningsCalendarNumberParam(query.Params, rule.minKey)
		if err != nil {
			return err
		}
		maxValue, hasMax, err := earningsCalendarNumberParam(query.Params, rule.maxKey)
		if err != nil {
			return err
		}
		if rule.percentage {
			if hasMin && minValue > 100 {
				return fmt.Errorf("%w: %s must not exceed 100", ErrInvalidQuery, rule.minKey)
			}
			if hasMax && maxValue > 100 {
				return fmt.Errorf("%w: %s must not exceed 100", ErrInvalidQuery, rule.maxKey)
			}
		}
		if hasMin && hasMax && minValue > maxValue {
			return fmt.Errorf("%w: %s must not exceed %s", ErrInvalidQuery, rule.minKey, rule.maxKey)
		}
	}

	return nil
}

func validateEarningsCalendarDates(params map[string]any) error {
	beginValue := strings.TrimSpace(stringParam(params, "beginDate"))
	endValue := strings.TrimSpace(stringParam(params, "endDate"))
	if beginValue == "" && endValue == "" {
		return nil
	}
	if beginValue == "" {
		return fmt.Errorf("%w: beginDate is required when endDate is provided", ErrInvalidQuery)
	}

	begin, err := time.Parse(time.DateOnly, beginValue)
	if err != nil {
		return fmt.Errorf("%w: beginDate must use YYYY-MM-DD", ErrInvalidQuery)
	}
	end := begin
	if endValue != "" {
		end, err = time.Parse(time.DateOnly, endValue)
		if err != nil {
			return fmt.Errorf("%w: endDate must use YYYY-MM-DD", ErrInvalidQuery)
		}
	}
	if end.Before(begin) {
		return fmt.Errorf("%w: endDate must not precede beginDate", ErrInvalidQuery)
	}
	if days := int(end.Sub(begin).Hours()/24) + 1; days > earningsCalendarMaxRangeDays {
		return fmt.Errorf("%w: earnings calendar range must not exceed %d days", ErrInvalidQuery, earningsCalendarMaxRangeDays)
	}
	return nil
}

func earningsCalendarNumberParam(params map[string]any, key string) (float64, bool, error) {
	if params == nil {
		return 0, false, nil
	}
	raw, ok := params[key]
	if !ok || raw == nil {
		return 0, false, nil
	}
	if text, stringValue := raw.(string); stringValue && strings.TrimSpace(text) == "" {
		return 0, false, nil
	}

	var (
		value float64
		err   error
	)
	switch typed := raw.(type) {
	case float64:
		value = typed
	case float32:
		value = float64(typed)
	case int:
		value = float64(typed)
	case int8:
		value = float64(typed)
	case int16:
		value = float64(typed)
	case int32:
		value = float64(typed)
	case int64:
		value = float64(typed)
	case uint:
		value = float64(typed)
	case uint8:
		value = float64(typed)
	case uint16:
		value = float64(typed)
	case uint32:
		value = float64(typed)
	case uint64:
		value = float64(typed)
	default:
		value, err = strconv.ParseFloat(strings.TrimSpace(fmt.Sprint(raw)), 64)
	}
	if err != nil || math.IsNaN(value) || math.IsInf(value, 0) {
		return 0, false, fmt.Errorf("%w: %s must be a finite number", ErrInvalidQuery, key)
	}
	if value < 0 {
		return 0, false, fmt.Errorf("%w: %s must not be negative", ErrInvalidQuery, key)
	}
	return value, true, nil
}

func hasNonEmptyParam(params map[string]any, key string) bool {
	if params == nil {
		return false
	}
	value, ok := params[key]
	if !ok || value == nil {
		return false
	}
	text, stringValue := value.(string)
	return !stringValue || strings.TrimSpace(text) != ""
}
