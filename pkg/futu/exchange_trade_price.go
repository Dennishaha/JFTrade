package futu

import (
	"math"
	"strconv"
	"strings"

	"github.com/c9s/bbgo/pkg/fixedpoint"
)

// --- Price-step normalization for order placement ---

// normalizeSubmitOrderPrice rounds a price to the market-specific tick step.
func normalizeSubmitOrderPrice(symbol string, price fixedpoint.Value) fixedpoint.Value {
	if price.Sign() <= 0 {
		return price
	}
	step := submitOrderPriceStep(symbol, price.Float64())
	if step <= 0 {
		return price
	}
	return fixedpoint.NewFromFloat(roundPriceToStep(price.Float64(), step))
}

// submitOrderPriceStep returns the minimum price increment for the symbol's market.
func submitOrderPriceStep(symbol string, price float64) float64 {
	switch strings.ToUpper(strings.TrimSpace(marketFromSymbol(symbol, ""))) {
	case "US":
		if price > 0 && price < 1 {
			return 0.0001
		}
		return 0.01
	default:
		return 0
	}
}

// roundPriceToStep rounds value to the nearest multiple of step.
func roundPriceToStep(value float64, step float64) float64 {
	if step <= 0 || !isFinitePositive(value) {
		return value
	}
	decimals := countStepDecimals(step)
	return math.Round(value/step) * stepRoundedUnit(decimals)
}

// stepRoundedUnit returns 1 / 10^decimals, clamped to ≥1.
func stepRoundedUnit(decimals int) float64 {
	factor := math.Pow10(decimals)
	if factor <= 0 {
		return 1
	}
	return 1 / factor
}

// countStepDecimals counts digits after the decimal point in step.
func countStepDecimals(step float64) int {
	text := strconv.FormatFloat(step, 'f', -1, 64)
	idx := strings.IndexByte(text, '.')
	if idx < 0 {
		return 0
	}
	return len(text) - idx - 1
}

// isFinitePositive returns true when value is a finite positive number (not NaN/Inf/≤0).
func isFinitePositive(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0) && value > 0
}
