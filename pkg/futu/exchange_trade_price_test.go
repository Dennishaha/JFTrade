package futu

import (
	"math"
	"testing"

	"github.com/jftrade/jftrade-main/pkg/bbgo/fixedpoint"
	"github.com/jftrade/jftrade-main/pkg/bbgo/types"
)

func closeFloat64(got float64, want float64) bool {
	return math.Abs(got-want) < 1e-9
}

func TestNormalizeSubmitOrderPriceForUSMarkets(t *testing.T) {
	if got := normalizeSubmitOrderPrice("US.AAPL", fixedpoint.NewFromFloat(123.456)).Float64(); got != 123.46 {
		t.Fatalf("US >= $1 normalize = %v, want 123.46", got)
	}
	if got := normalizeSubmitOrderPrice("US.SOUN", fixedpoint.NewFromFloat(0.12344)).Float64(); got != 0.1234 {
		t.Fatalf("US sub-dollar normalize = %v, want 0.1234", got)
	}
	if got := normalizeSubmitOrderPrice("US.SOUN", fixedpoint.NewFromFloat(0.12345)).Float64(); got != 0.1235 {
		t.Fatalf("US sub-dollar half-up normalize = %v, want 0.1235", got)
	}
	if got := normalizeSubmitOrderPrice("HK.00700", fixedpoint.NewFromFloat(320.123)).Float64(); got != 320.123 {
		t.Fatalf("non-US normalize = %v, want unchanged 320.123", got)
	}
}

func TestPriceStepHelpersCoverEdgeCases(t *testing.T) {
	if got := submitOrderPriceStep("US.AAPL", 150); got != 0.01 {
		t.Fatalf("submitOrderPriceStep regular US = %v, want 0.01", got)
	}
	if got := submitOrderPriceStep("US.SOUN", 0.55); got != 0.0001 {
		t.Fatalf("submitOrderPriceStep sub-dollar US = %v, want 0.0001", got)
	}
	if got := submitOrderPriceStep("HK.00700", 380); got != 0 {
		t.Fatalf("submitOrderPriceStep HK = %v, want 0", got)
	}
	if got := roundPriceToStep(0.12344, 0.0001); !closeFloat64(got, 0.1234) {
		t.Fatalf("roundPriceToStep sub-dollar = %v, want 0.1234", got)
	}
	if got := roundPriceToStep(123.456, 0.01); !closeFloat64(got, 123.46) {
		t.Fatalf("roundPriceToStep cents = %v, want 123.46", got)
	}
	if got := stepRoundedUnit(4); !closeFloat64(got, 0.0001) {
		t.Fatalf("stepRoundedUnit(4) = %v, want 0.0001", got)
	}
	if got := countStepDecimals(0.0001); got != 4 {
		t.Fatalf("countStepDecimals(0.0001) = %d, want 4", got)
	}
	if got := countStepDecimals(1); got != 0 {
		t.Fatalf("countStepDecimals(1) = %d, want 0", got)
	}
	if isFinitePositive(0) || isFinitePositive(-1) || isFinitePositive(math.NaN()) || isFinitePositive(math.Inf(1)) {
		t.Fatal("isFinitePositive should reject non-positive and non-finite values")
	}
	if !isFinitePositive(0.01) {
		t.Fatal("isFinitePositive should accept positive finite values")
	}
}

func TestPlaceOrderRequestFromSubmitOrderNormalizesUSPriceAndFlags(t *testing.T) {
	account := resolvedTradeAccount{
		AccountID:          "1002",
		TradingEnvironment: "REAL",
		Market:             "US",
		protoAccountID:     1002,
		protoTrdEnv:        1,
		protoTrdMarket:     2,
	}
	session := "RTH"
	fillOutsideRTH := true
	request, err := placeOrderRequestFromSubmitOrder(account, types.SubmitOrder{
		Symbol:      "US.SOUN",
		Side:        types.SideTypeBuy,
		Type:        types.OrderTypeStopLimit,
		Price:       fixedpoint.NewFromFloat(0.12344),
		StopPrice:   fixedpoint.NewFromFloat(0.23456),
		Quantity:    fixedpoint.NewFromFloat(100),
		TimeInForce: types.TimeInForceGTC,
	}, BrokerPlaceOrderQuery{
		Session:        &session,
		FillOutsideRTH: &fillOutsideRTH,
	})
	if err != nil {
		t.Fatalf("placeOrderRequestFromSubmitOrder: %v", err)
	}
	if got := request.GetCode(); got != "SOUN" {
		t.Fatalf("Code = %q, want SOUN", got)
	}
	if got := request.GetPrice(); !closeFloat64(got, 0.1234) {
		t.Fatalf("Price = %v, want 0.1234", got)
	}
	if got := request.GetAuxPrice(); !closeFloat64(got, 0.2346) {
		t.Fatalf("AuxPrice = %v, want 0.2346", got)
	}
	if got := request.GetSession(); got != 1 {
		t.Fatalf("Session = %d, want RTH(1)", got)
	}
	if !request.GetFillOutsideRTH() {
		t.Fatal("FillOutsideRTH should be true for US stop-limit order")
	}
}
