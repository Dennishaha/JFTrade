package indicatorruntime

import (
	"time"

	"github.com/jftrade/jftrade-main/pkg/market"
)

func clearMap[K comparable, V any](values map[K]V) {
	for key := range values {
		delete(values, key)
	}
}

func trimFloatSeriesInPlace(values []float64, limit int) []float64 {
	if limit <= 0 || len(values) <= limit {
		return values
	}
	start := len(values) - limit
	copy(values, values[start:])
	return values[:limit]
}

func trimTimeSeriesInPlace(values []time.Time, limit int) []time.Time {
	if limit <= 0 || len(values) <= limit {
		return values
	}
	start := len(values) - limit
	copy(values, values[start:])
	return values[:limit]
}

func trimSessionSeriesInPlace(values []market.Session, limit int) []market.Session {
	if limit <= 0 || len(values) <= limit {
		return values
	}
	start := len(values) - limit
	copy(values, values[start:])
	return values[:limit]
}

func trimInt64SeriesInPlace(values []int64, limit int) []int64 {
	if limit <= 0 || len(values) <= limit {
		return values
	}
	start := len(values) - limit
	copy(values, values[start:])
	return values[:limit]
}

func reuseFloat64Slice(values []float64, length int) []float64 {
	if length <= 0 {
		return nil
	}
	if cap(values) < length {
		return make([]float64, length)
	}
	return values[:length]
}

func reuseInt64Slice(values []int64, length int) []int64 {
	if length <= 0 {
		return nil
	}
	if cap(values) < length {
		return make([]int64, length)
	}
	return values[:length]
}

func maxFloat(left, right float64) float64 {
	if left > right {
		return left
	}
	return right
}

func minFloat(left, right float64) float64 {
	if left < right {
		return left
	}
	return right
}
