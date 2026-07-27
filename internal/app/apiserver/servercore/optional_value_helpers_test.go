package servercore

import (
	"encoding/json"

	"github.com/shopspring/decimal"
)

func optionalFloat64JSON(value *float64) any {
	if value == nil {
		return nil
	}
	return json.Number(decimal.NewFromFloat(*value).String())
}

func optionalInt64(value *int64) any {
	if value == nil {
		return nil
	}
	return *value
}

func optionalBool(value *bool) any {
	if value == nil {
		return nil
	}
	return *value
}

func optionalString(value *string) any {
	if value == nil {
		return nil
	}
	return *value
}
