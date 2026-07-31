package futu

import (
	"github.com/jftrade/jftrade-main/pkg/bbgo/fixedpoint"
	"github.com/shopspring/decimal"
)

var maxLegacyFixedpointValue = decimal.RequireFromString("92233720368.54775807")

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

func legacyFixedpointVolume(value decimal.Decimal) fixedpoint.Value {
	if value.Abs().GreaterThan(maxLegacyFixedpointValue) {
		return fixedpoint.Zero
	}
	converted, err := fixedpoint.NewFromString(value.String())
	if err != nil || converted.IsInf() {
		return fixedpoint.Zero
	}
	return converted
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
