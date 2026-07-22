package trading

import (
	"math"
	"testing"

	"github.com/jftrade/jftrade-main/pkg/broker"
	"github.com/jftrade/jftrade-main/pkg/market"
)

func TestCommandRiskShapeRejectsSpoofedAndIncompatibleFields(t *testing.T) {
	positive := 10.0
	tests := []struct {
		name    string
		command ExecutionOrderCommand
		code    string
	}{
		{
			name: "product mismatch",
			command: ExecutionOrderCommand{ProductClass: broker.ProductClassEquity, Query: broker.PlaceOrderQuery{
				ProductClass: broker.ProductClassOption, Quantity: 1,
			}},
			code: "INVALID_ORDER_RISK_SHAPE",
		},
		{
			name: "quantity mode mismatch",
			command: ExecutionOrderCommand{QuantityMode: broker.QuantityModeUnits, Query: broker.PlaceOrderQuery{
				QuantityMode: broker.QuantityModeAmount, Quantity: 1,
			}},
			code: "INVALID_ORDER_RISK_SHAPE",
		},
		{
			name: "event contract units",
			command: ExecutionOrderCommand{ProductClass: broker.ProductClassEventContract, QuantityMode: broker.QuantityModeUnits, Query: broker.PlaceOrderQuery{
				Quantity: 1,
			}},
			code: "INVALID_ORDER_RISK_SHAPE",
		},
		{
			name: "event contract missing amount",
			command: ExecutionOrderCommand{ProductClass: broker.ProductClassEventContract, QuantityMode: broker.QuantityModeAmount, Query: broker.PlaceOrderQuery{
				QuantityMode: broker.QuantityModeAmount,
			}},
			code: "RISK_AMOUNT_UNAVAILABLE",
		},
		{
			name: "equity amount",
			command: ExecutionOrderCommand{ProductClass: broker.ProductClassEquity, Query: broker.PlaceOrderQuery{
				Quantity: 1, Amount: &positive,
			}},
			code: "INVALID_ORDER_RISK_SHAPE",
		},
		{
			name: "equity prediction side",
			command: ExecutionOrderCommand{ProductClass: broker.ProductClassEquity, Query: broker.PlaceOrderQuery{
				Quantity: 1, PredictionSide: "YES",
			}},
			code: "INVALID_ORDER_RISK_SHAPE",
		},
		{
			name: "non-finite quantity",
			command: ExecutionOrderCommand{ProductClass: broker.ProductClassEquity, Query: broker.PlaceOrderQuery{
				Quantity: math.NaN(),
			}},
			code: "RISK_QUANTITY_UNAVAILABLE",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			code, message := commandRiskShapeError(test.command)
			if code != test.code || message == "" {
				t.Fatalf("commandRiskShapeError() = %q/%q, want %q/non-empty", code, message, test.code)
			}
		})
	}
}

func TestExecutionProductAndPriceValidationBoundaries(t *testing.T) {
	negative := -1.0
	if err := validateExecutionPrices(ExecutionPlaceRequest{Price: &negative}, "MARKET"); err == nil {
		t.Fatal("negative optional price was accepted")
	}
	if err := validateExecutionPrices(ExecutionPlaceRequest{StopPrice: &negative}, "MARKET"); err == nil {
		t.Fatal("negative optional stop price was accepted")
	}
	payload := ExecutionPlaceRequest{
		Market: "US", Symbol: "PREDICTION-1", ProductClass: broker.ProductClassEventContract,
		QuantityMode: broker.QuantityModeUnits,
	}
	if _, _, _, err := normalizeExecutionProduct(&payload, market.Instrument{Market: "US", Symbol: "US.PREDICTION-1"}); err == nil {
		t.Fatal("event-contract unit quantity mode was accepted")
	}
}
