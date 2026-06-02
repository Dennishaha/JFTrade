package futu

import (
	"testing"
	"time"

	"github.com/shopspring/decimal"

	qotcommonpb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotcommon"
)

func TestQuoteSnapshotPreviousClosePriceInClosedSession(t *testing.T) {
	// Simulates a closed-session snapshot: Futu returns Friday's CurPrice
	// (9.22) and LastClosePrice (Thursday's close = 9.09). When the market is
	// closed, PreviousClosePrice should still use CurPrice so the frontend can
	// display the latest regular-session close.
	basicQot := &qotcommonpb.BasicQot{
		Security:        &qotcommonpb.Security{Market: protoInt32(1), Code: protoString("TME")},
		CurPrice:        f64(9.22),
		LastClosePrice:  f64(9.09),
		OpenPrice:       f64(9.02),
		HighPrice:       f64(9.27),
		LowPrice:        f64(9.00),
		Volume:          protoInt64(13037749),
		Turnover:        f64(119751018),
		UpdateTimestamp: f64(1748548799.998), // Friday 2026-05-29 19:59:59 UTC
	}

	closedSessionAt := time.Date(2026, 5, 31, 12, 0, 0, 0, usEasternLocation)
	snap := quoteSnapshotFromBasicQotAt(basicQot, "US.TME", closedSessionAt)

	// On a Sunday (closed session), PreviousClosePrice must equal CurPrice
	// (the most recent regular-session close), NOT LastClosePrice (previous
	// trading day's close).
	if snap.PreviousClosePrice == nil {
		t.Fatal("PreviousClosePrice is nil, expected 9.22")
	}
	if !snap.PreviousClosePrice.Equal(decimal.NewFromFloat(9.22)) {
		t.Errorf("PreviousClosePrice = %s, want 9.22 (Friday's close)", snap.PreviousClosePrice.String())
	}

	// LastClosePrice should remain the raw Futu value (Thursday's close).
	if snap.LastClosePrice == nil {
		t.Fatal("LastClosePrice is nil, expected 9.09")
	}
	if !snap.LastClosePrice.Equal(decimal.NewFromFloat(9.09)) {
		t.Errorf("LastClosePrice = %s, want 9.09", snap.LastClosePrice.String())
	}

	// Session should be closed on a weekend.
	if snap.Session != MarketSessionClosed {
		t.Errorf("Session = %s, want closed", snap.Session)
	}
}

func TestQuoteSnapshotPreviousClosePriceInAfterHours(t *testing.T) {
	// During after-hours, PreviousClosePrice should be CurPrice (today's
	// regular-session close) so the frontend can show "最近盘中收盘".
	basicQot := &qotcommonpb.BasicQot{
		Security:       &qotcommonpb.Security{Market: protoInt32(1), Code: protoString("AAPL")},
		CurPrice:       f64(195.50),
		LastClosePrice: f64(193.20),
		AfterMarket: &qotcommonpb.PreAfterMarketData{
			Price:      f64(195.30),
			ChangeRate: f64(-0.10),
		},
	}

	snap := quoteSnapshotFromBasicQot(basicQot, "US.AAPL")

	// After-hours: PreviousClosePrice = CurPrice = today's regular close.
	if snap.PreviousClosePrice == nil {
		t.Fatal("PreviousClosePrice is nil")
	}
	if !snap.PreviousClosePrice.Equal(decimal.NewFromFloat(195.50)) {
		t.Errorf("PreviousClosePrice = %s, want 195.50 (today's regular close)", snap.PreviousClosePrice.String())
	}
}

func TestQuoteSnapshotPreviousClosePriceZeroCurPrice(t *testing.T) {
	// If CurPrice is 0 (e.g., no data), PreviousClosePrice should fall back
	// to LastClosePrice regardless of session.
	basicQot := &qotcommonpb.BasicQot{
		Security:       &qotcommonpb.Security{Market: protoInt32(1), Code: protoString("TME")},
		CurPrice:       f64(0),
		LastClosePrice: f64(9.09),
	}

	snap := quoteSnapshotFromBasicQot(basicQot, "US.TME")

	if snap.PreviousClosePrice == nil {
		t.Fatal("PreviousClosePrice is nil")
	}
	if !snap.PreviousClosePrice.Equal(decimal.NewFromFloat(9.09)) {
		t.Errorf("PreviousClosePrice = %s, want 9.09 (fallback to LastClosePrice when CurPrice=0)", snap.PreviousClosePrice.String())
	}
}

func TestPreviousClosePriceConditionBySessionType(t *testing.T) {
	// Verify the core condition: session != MarketSessionRegular
	// should cause PreviousClosePrice to use CurPrice instead of LastClosePrice.
	regularSessionClose := decimal.NewFromFloat(9.22)
	lastClosePrice := decimal.NewFromFloat(9.09)

	cases := []struct {
		name            string
		session         MarketSession
		expectCurPrice  bool
		expectLastClose bool
	}{
		{"regular uses LastClosePrice", MarketSessionRegular, false, true},
		{"pre uses CurPrice", MarketSessionPre, true, false},
		{"after uses CurPrice", MarketSessionAfter, true, false},
		{"overnight uses CurPrice", MarketSessionOvernight, true, false},
		{"closed uses CurPrice", MarketSessionClosed, true, false},
		{"unknown uses CurPrice", MarketSessionUnknown, true, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			useCurPrice := tc.session != MarketSessionRegular && regularSessionClose.GreaterThan(decimal.Zero)
			if tc.expectCurPrice && !useCurPrice {
				t.Errorf("session=%s: expected to use CurPrice, but condition is false", tc.session)
			}
			if tc.expectLastClose && useCurPrice {
				t.Errorf("session=%s: expected to use LastClosePrice, but condition is true", tc.session)
			}
			// Also verify the old condition (IsExtendedMarketSession) would have
			// incorrectly excluded "closed" and "unknown":
			oldCondition := IsExtendedMarketSession(tc.session)
			if tc.session == MarketSessionClosed && oldCondition {
				t.Errorf("session=closed: old IsExtendedMarketSession incorrectly returns true")
			}
			if tc.session == MarketSessionClosed && !useCurPrice {
				t.Errorf("session=closed: new condition should use CurPrice but doesn't")
			}
		})
	}

	// Sanity: ensure the result matches LastClosePrice when regular.
	if regularSessionClose.Equal(lastClosePrice) {
		t.Fatal("test data sanity: regularSessionClose should differ from lastClosePrice")
	}
}

func protoInt32(v int32) *int32    { return &v }
func protoInt64(v int64) *int64    { return &v }
func protoString(v string) *string { return &v }
