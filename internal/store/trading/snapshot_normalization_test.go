package trading

import (
	"testing"
	"time"

	trdsrv "github.com/jftrade/jftrade-main/internal/trading"
	"github.com/jftrade/jftrade-main/pkg/broker"
)

func TestBrokerSnapshotNormalizesIdentityAndQuantities(t *testing.T) {
	summary := trdsrv.ExecutionOrder{
		ProductClass: broker.ProductClassUnknown,
		QuantityMode: broker.QuantityModeUnits,
	}
	amount := 25.0
	price := 0.42
	filled := 2.0
	average := 0.4
	if !applyBrokerOrderSnapshotIdentity(&summary, broker.OrderSnapshot{
		OrderKind: broker.OrderKindEventParlay, ProductClass: broker.ProductClassEventContract,
		QuantityMode: broker.QuantityModeAmount, Market: "US", AccountID: "account-1",
		TradingEnvironment: "SIMULATE", Symbol: "US.EVENT", Side: "BUY", OrderType: "LIMIT",
	}) {
		t.Fatal("broker identity update was ignored")
	}
	if summary.OrderKind != broker.OrderKindEventParlay ||
		summary.ProductClass != broker.ProductClassEventContract ||
		summary.QuantityMode != broker.QuantityModeAmount {
		t.Fatalf("normalized broker identity = %#v", summary)
	}
	if !applyBrokerOrderSnapshotQuantities(&summary, broker.OrderSnapshot{
		Quantity: 2, Price: &price, Amount: &amount,
		FilledQuantity: &filled, FilledAveragePrice: &average,
	}) {
		t.Fatal("broker quantity update was ignored")
	}
	if summary.RequestedAmount == nil || *summary.RequestedAmount != amount ||
		summary.FilledAveragePrice == nil || *summary.FilledAveragePrice != average {
		t.Fatalf("normalized broker quantities = %#v", summary)
	}
}

func TestExecutionTimestampCutoffBoundaries(t *testing.T) {
	cutoff := time.Date(2026, time.June, 20, 0, 0, 0, 0, time.UTC)
	if !executionTimestampBefore("2026-06-19T23:59:59Z", cutoff) {
		t.Fatal("timestamp before cutoff not detected")
	}
	if executionTimestampBefore("", cutoff) || executionTimestampBefore("bad", cutoff) ||
		executionTimestampBefore("2026-06-20T00:00:00Z", cutoff) {
		t.Fatal("cutoff accepted empty, malformed, or equal timestamp")
	}
}
