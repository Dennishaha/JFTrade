package futu

import (
	"testing"
	"time"

	"github.com/jftrade/jftrade-main/pkg/futu/opend"
	qotcommonpb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotcommon"
)

func TestExchangeQueryQuoteSnapshotUsesBasicQotPayload(t *testing.T) {
	server := startQuoteOpenDServer(t)
	server.setBasicQuotes([]*qotcommonpb.BasicQot{{
		Security:        testHKSecurity("00700"),
		IsSuspended:     new(false),
		ListTime:        new("2004-06-16"),
		PriceSpread:     new(0.1),
		UpdateTime:      new("2026-06-23 15:30:00"),
		UpdateTimestamp: new(float64(time.Date(2026, 6, 23, 7, 30, 0, 0, time.UTC).Unix())),
		CurPrice:        new(380.0),
		OpenPrice:       new(378.0),
		HighPrice:       new(382.0),
		LowPrice:        new(377.5),
		LastClosePrice:  new(379.0),
		Volume:          new(int64(1234567)),
		Turnover:        new(float64(468000000)),
		TurnoverRate:    new(0.1),
		Amplitude:       new(1.2),
	}})
	defer server.stop()

	ex := NewExchangeWithConfig(opend.Config{Addr: server.addr, RequestTimeout: 2 * time.Second})
	t.Cleanup(func() {
		jftradeCheckTestError(t, ex.Close())
	})
	if err := ex.SubscribeBasicQuote(t.Context(), "HK.00700", false); err != nil {
		t.Fatalf("SubscribeBasicQuote: %v", err)
	}

	snapshot, err := ex.QueryQuoteSnapshot(t.Context(), "HK.00700")
	if err != nil {
		t.Fatalf("QueryQuoteSnapshot: %v", err)
	}
	if snapshot == nil {
		t.Fatal("expected quote snapshot")
	}
	if snapshot.Symbol != "HK.00700" {
		t.Fatalf("Symbol = %q, want HK.00700", snapshot.Symbol)
	}
	if got := snapshot.Price.InexactFloat64(); got != 380 {
		t.Fatalf("Price = %v, want 380", got)
	}
	if snapshot.OpenPrice == nil || snapshot.OpenPrice.InexactFloat64() != 378 {
		t.Fatalf("OpenPrice = %#v, want 378", snapshot.OpenPrice)
	}
	if snapshot.HighPrice == nil || snapshot.HighPrice.InexactFloat64() != 382 {
		t.Fatalf("HighPrice = %#v, want 382", snapshot.HighPrice)
	}
	if snapshot.LowPrice == nil || snapshot.LowPrice.InexactFloat64() != 377.5 {
		t.Fatalf("LowPrice = %#v, want 377.5", snapshot.LowPrice)
	}
	if snapshot.LastClosePrice == nil || snapshot.LastClosePrice.InexactFloat64() != 379 {
		t.Fatalf("LastClosePrice = %#v, want 379", snapshot.LastClosePrice)
	}
	if snapshot.Volume != 1234567 {
		t.Fatalf("Volume = %v, want 1234567", snapshot.Volume)
	}
	if got := snapshot.Turnover.InexactFloat64(); got != 468000000 {
		t.Fatalf("Turnover = %v, want 468000000", got)
	}
	wantQuoteAt := time.Date(2026, 6, 23, 7, 30, 0, 0, time.UTC)
	if !snapshot.QuoteAt.Equal(wantQuoteAt) {
		t.Fatalf("QuoteAt = %s, want %s", snapshot.QuoteAt, wantQuoteAt)
	}
	if snapshot.PreMarket != nil || snapshot.AfterMarket != nil || snapshot.Overnight != nil {
		t.Fatalf("unexpected extended blocks: pre=%#v after=%#v overnight=%#v", snapshot.PreMarket, snapshot.AfterMarket, snapshot.Overnight)
	}
	if got := server.basicQotCalls.Load(); got != 1 {
		t.Fatalf("expected one GetBasicQot call, got %d", got)
	}
}
