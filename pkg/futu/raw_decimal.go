package futu

import (
	"github.com/jftrade/jftrade-main/pkg/bbgo/fixedpoint"
	"github.com/shopspring/decimal"
)

func decimalFromFloat64(value float64) decimal.Decimal {
	return decimal.NewFromFloat(value)
}

func decimalPtrFromFloat64(value *float64) *decimal.Decimal {
	if value == nil {
		return nil
	}
	return new(decimal.NewFromFloat(*value))
}

func decimalPositive(value *decimal.Decimal) bool {
	return value != nil && value.GreaterThan(decimal.Zero)
}

func fixedpointFromDecimal(value decimal.Decimal) fixedpoint.Value {
	return fixedpoint.MustNewFromString(value.String())
}

func fixedpointFromDecimalPtr(value *decimal.Decimal) fixedpoint.Value {
	if value == nil {
		return fixedpoint.Zero
	}
	return fixedpointFromDecimal(*value)
}

func fixedpointFromFloat64(value float64) fixedpoint.Value {
	return fixedpointFromDecimal(decimalFromFloat64(value))
}

func cloneFloat64(value *float64) *float64 {
	if value == nil {
		return nil
	}
	return new(*value)
}
