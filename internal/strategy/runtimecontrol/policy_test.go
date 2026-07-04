package runtimecontrol

import (
	"math"
	"testing"
	"time"

	"github.com/jftrade/jftrade-main/internal/strategy/runtimeactivity"
)

func TestEvaluateRiskAppliesRuntimeLimits(t *testing.T) {
	maxQuantity := 5.0
	maxNotional := 500.0
	settings := RiskSettings{
		Mode:             ModeEnforce,
		CloseOnly:        true,
		MaxOrderQuantity: &maxQuantity,
		MaxOrderNotional: &maxNotional,
		PauseOnReject:    true,
	}

	tests := []struct {
		name    string
		order   OrderIntent
		context RiskContext
		want    string
	}{
		{name: "buy blocked by close only", order: OrderIntent{Symbol: "US.AAPL", Side: "BUY", Quantity: 1}, want: "close_only"},
		{name: "sell exceeds position", order: OrderIntent{Symbol: "US.AAPL", Side: "SELL", Quantity: 5}, context: RiskContext{SellableQuantity: 4}, want: "close_only_insufficient_position"},
		{name: "sell exceeds notional", order: OrderIntent{Symbol: "US.AAPL", Side: "SELL", Quantity: 4, Price: new(130.0)}, context: RiskContext{SellableQuantity: 4}, want: "max_order_notional"},
		{name: "sell allowed", order: OrderIntent{Symbol: "US.AAPL", Side: "SELL", Quantity: 4}, context: RiskContext{SellableQuantity: 4, CurrentPrice: 100}, want: ""},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			decision := EvaluateRisk(settings, test.order, test.context)
			if decision.Reason != test.want {
				t.Fatalf("reason = %q, want %q", decision.Reason, test.want)
			}
			if test.want != "" && (!decision.Matched || !decision.Rejected || !decision.PauseOnReject) {
				t.Fatalf("unexpected enforce decision: %+v", decision)
			}
		})
	}

	settings.CloseOnly = false
	decision := EvaluateRisk(settings, OrderIntent{Symbol: "US.AAPL", Side: "BUY", Quantity: 6}, RiskContext{CurrentPrice: 100})
	if decision.Reason != "max_order_quantity" {
		t.Fatalf("quantity decision reason = %q, want max_order_quantity", decision.Reason)
	}
}

func TestEvaluateRiskMonitorModeRecordsButDoesNotReject(t *testing.T) {
	dailyMaxOrders := 3
	decision := EvaluateRisk(
		RiskSettings{Mode: ModeMonitor, DailyMaxOrders: &dailyMaxOrders},
		OrderIntent{Symbol: "US.AAPL", Side: "BUY", Quantity: 1},
		RiskContext{TodaySubmittedOrderCount: 3},
	)
	if !decision.Matched || decision.Rejected || decision.Reason != "daily_max_orders" {
		t.Fatalf("unexpected monitor decision: %+v", decision)
	}
	if want := "rule=daily_max_orders"; len(decision.Detail) < len(want) || decision.Detail[:len(want)] != want {
		t.Fatalf("detail = %q, want prefix %q", decision.Detail, want)
	}
}

func TestNormalizeRiskSettingsClearsOffModeLimits(t *testing.T) {
	negative := -1.0
	zero := 0
	normalized := NormalizeRiskSettings(RiskSettings{
		Mode:             "unknown",
		CloseOnly:        true,
		MaxOrderQuantity: &negative,
		DailyMaxOrders:   &zero,
		PauseOnReject:    true,
	})
	if normalized.Mode != ModeOff || normalized.CloseOnly || normalized.MaxOrderQuantity != nil || normalized.DailyMaxOrders != nil || normalized.PauseOnReject {
		t.Fatalf("unexpected normalized settings: %+v", normalized)
	}
}

func TestMarketDayStartUTCUsesOrderSymbolTimezone(t *testing.T) {
	now := time.Date(2026, time.January, 1, 2, 0, 0, 0, time.UTC)
	if got, want := MarketDayStartUTC("US.AAPL", now), time.Date(2025, time.December, 31, 5, 0, 0, 0, time.UTC); !got.Equal(want) {
		t.Fatalf("US day start = %s, want %s", got, want)
	}
	if got, want := MarketDayStartUTC("HK.00700", now), time.Date(2025, time.December, 31, 16, 0, 0, 0, time.UTC); !got.Equal(want) {
		t.Fatalf("HK day start = %s, want %s", got, want)
	}

	ny, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatal(err)
	}
	overnight := time.Date(2026, time.June, 14, 20, 30, 0, 0, ny)
	if got, want := MarketDayStartUTC("US.AAPL", overnight), time.Date(2026, time.June, 15, 0, 0, 0, 0, time.UTC); !got.Equal(want) {
		t.Fatalf("US overnight day start = %s, want %s", got, want)
	}
}

func TestObservationFromSnapshotFormatsTimesAndDefaultsStatus(t *testing.T) {
	at := time.Date(2026, time.January, 2, 3, 4, 5, 6, time.FixedZone("test", 3600))
	observation := ObservationFromSnapshot(runtimeactivity.ObservationSnapshot{
		ActiveSymbols:     []string{"US.AAPL"},
		LastClosedKLineAt: &at,
		LastError:         "  broker rejected  ",
	}, "", "STOPPED")
	if observation.ActualStatus != "STOPPED" {
		t.Fatalf("status = %q, want STOPPED", observation.ActualStatus)
	}
	if observation.LastClosedKLineAt == nil || *observation.LastClosedKLineAt != "2026-01-02T02:04:05.000000006Z" {
		t.Fatalf("last closed kline = %v", observation.LastClosedKLineAt)
	}
	if observation.LastError == nil || *observation.LastError != "broker rejected" {
		t.Fatalf("last error = %v", observation.LastError)
	}
}

func TestPositionMatchesMarketQualifiedSymbols(t *testing.T) {
	positions := []Position{{
		Market:           "US",
		Symbol:           "AAPL",
		Quantity:         5,
		SellableQuantity: 3,
	}, {
		Market:           "HK",
		Symbol:           "00700",
		Quantity:         10,
		SellableQuantity: 10,
	}}
	if got := SellableQuantity(positions, "US.AAPL"); got != 3 {
		t.Fatalf("sellable US.AAPL = %v, want 3", got)
	}
	if !PositionMatchesSymbol(Position{Symbol: "AAPL"}, "US.AAPL") {
		t.Fatalf("expected bare position symbol to match strategy market-qualified symbol")
	}
	if !PositionMatchesSymbol(Position{Market: "US", Symbol: "AAPL"}, "US:AAPL") {
		t.Fatalf("expected market+symbol position to match colon-qualified strategy symbol")
	}
	if PositionMatchesSymbol(Position{Market: "HK", Symbol: "00700"}, "US.AAPL") {
		t.Fatalf("unexpected cross-market position match")
	}
}

func TestFormatNumberNormalizesNegativeZero(t *testing.T) {
	if got := FormatNumber(math.Copysign(0, -1)); got != "0" {
		t.Fatalf("negative zero format = %q, want 0", got)
	}
}
