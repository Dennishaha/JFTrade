package futu

import (
	"testing"
	"time"

	"github.com/shopspring/decimal"

	qotcommonpb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotcommon"
	"github.com/jftrade/jftrade-main/pkg/market"
)

func TestQuoteSnapshotPreviousClosePriceInClosedSession(t *testing.T) {
	// Simulates a closed-session snapshot: Futu returns Friday's CurPrice
	// (9.22) and LastClosePrice (Thursday's close = 9.09). When the market is
	// closed, PreviousClosePrice should still use CurPrice so the frontend can
	// display the latest regular-session close.
	basicQot := &qotcommonpb.BasicQot{
		Security:        &qotcommonpb.Security{Market: new(int32(1)), Code: new("TME")},
		CurPrice:        new(9.22),
		LastClosePrice:  new(9.09),
		OpenPrice:       new(9.02),
		HighPrice:       new(9.27),
		LowPrice:        new(9.00),
		Volume:          new(int64(13037749)),
		Turnover:        new(float64(119751018)),
		UpdateTimestamp: new(1748548799.998), // Friday 2026-05-29 19:59:59 UTC
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
	if snap.Session != market.SessionClosed {
		t.Errorf("Session = %s, want closed", snap.Session)
	}
}

func TestQuoteSnapshotHolidayRemainsClosedWithStaleExtendedBlocks(t *testing.T) {
	basicQot := &qotcommonpb.BasicQot{
		Security:       &qotcommonpb.Security{Market: new(int32(1)), Code: new("AAPL")},
		CurPrice:       new(195.50),
		LastClosePrice: new(193.20),
		PreMarket: &qotcommonpb.PreAfterMarketData{
			Price:      new(196.10),
			ChangeRate: new(0.31),
		},
		AfterMarket: &qotcommonpb.PreAfterMarketData{
			Price:      new(195.30),
			ChangeRate: new(-0.10),
		},
		Overnight: &qotcommonpb.PreAfterMarketData{
			Price:      new(194.90),
			ChangeRate: new(-0.31),
		},
		UpdateTimestamp: new(1781812800.0), // 2026-06-18 20:00:00 UTC
	}

	holidayAt := time.Date(2026, 6, 19, 12, 0, 0, 0, usEasternLocation)
	snap := quoteSnapshotFromBasicQotAt(basicQot, "US.AAPL", holidayAt)

	if snap.Session != market.SessionClosed {
		t.Fatalf("Session = %s, want closed", snap.Session)
	}
	if !snap.Price.Equal(decimal.NewFromFloat(195.50)) {
		t.Fatalf("Price = %s, want 195.50 regular close", snap.Price.String())
	}
	if snap.PreviousClosePrice == nil || !snap.PreviousClosePrice.Equal(decimal.NewFromFloat(195.50)) {
		t.Fatalf("PreviousClosePrice = %v, want 195.50", snap.PreviousClosePrice)
	}
	if snap.PreMarket == nil || snap.AfterMarket == nil || snap.Overnight == nil {
		t.Fatal("extended blocks should remain available for display decisions")
	}
	if snap.PreMarket.QuoteTime == "" || snap.AfterMarket.QuoteTime == "" || snap.Overnight.QuoteTime == "" {
		t.Fatal("extended blocks should preserve quoteTime metadata")
	}
}

func TestQuoteSnapshotPreviousClosePriceInAfterHours(t *testing.T) {
	// During after-hours, PreviousClosePrice should be CurPrice (today's
	// regular-session close) so the frontend can show "最近盘中收盘".
	basicQot := &qotcommonpb.BasicQot{
		Security:       &qotcommonpb.Security{Market: new(int32(1)), Code: new("AAPL")},
		CurPrice:       new(195.50),
		LastClosePrice: new(193.20),
		AfterMarket: &qotcommonpb.PreAfterMarketData{
			Price:      new(195.30),
			ChangeRate: new(-0.10),
		},
	}

	afterHoursAt := time.Date(2026, 6, 2, 17, 0, 0, 0, usEasternLocation)
	snap := quoteSnapshotFromBasicQotAt(basicQot, "US.AAPL", afterHoursAt)

	// After-hours: PreviousClosePrice = CurPrice = today's regular close.
	if snap.PreviousClosePrice == nil {
		t.Fatal("PreviousClosePrice is nil")
	}
	if !snap.PreviousClosePrice.Equal(decimal.NewFromFloat(195.50)) {
		t.Errorf("PreviousClosePrice = %s, want 195.50 (today's regular close)", snap.PreviousClosePrice.String())
	}
	if snap.Session != market.SessionAfter {
		t.Errorf("Session = %s, want after", snap.Session)
	}
	if snap.AfterMarket == nil || snap.AfterMarket.QuoteTime == "" {
		t.Fatal("AfterMarket quoteTime should be preserved")
	}
}

func TestQuoteSnapshotPreviousClosePriceZeroCurPrice(t *testing.T) {
	// If CurPrice is 0 (e.g., no data), PreviousClosePrice should fall back
	// to LastClosePrice regardless of session.
	basicQot := &qotcommonpb.BasicQot{
		Security:       &qotcommonpb.Security{Market: new(int32(1)), Code: new("TME")},
		CurPrice:       new(float64(0)),
		LastClosePrice: new(9.09),
	}

	snap := quoteSnapshotFromBasicQot(basicQot, "US.TME")

	if snap.PreviousClosePrice == nil {
		t.Fatal("PreviousClosePrice is nil")
	}
	if !snap.PreviousClosePrice.Equal(decimal.NewFromFloat(9.09)) {
		t.Errorf("PreviousClosePrice = %s, want 9.09 (fallback to LastClosePrice when CurPrice=0)", snap.PreviousClosePrice.String())
	}
}

func TestQuoteSnapshotPreviousClosePriceForHKLunchBreak(t *testing.T) {
	// HK symbols do not use the US extended-hours session model. During the
	// lunch break Futu can still return CurPrice as the latest traded price,
	// but PreviousClosePrice must remain LastClosePrice so change percent does
	// not collapse to 0%.
	basicQot := &qotcommonpb.BasicQot{
		Security:        &qotcommonpb.Security{Market: new(int32(2)), Code: new("00700")},
		CurPrice:        new(321.40),
		LastClosePrice:  new(318.90),
		OpenPrice:       new(320.00),
		HighPrice:       new(322.20),
		LowPrice:        new(319.80),
		Volume:          new(int64(12345678)),
		Turnover:        new(float64(3955555555)),
		UpdateTimestamp: new(float64(1781238600)), // 2026-06-12 12:30:00 Asia/Hong_Kong
	}

	lunchBreakAt := time.Date(2026, 6, 12, 12, 30, 0, 0, time.FixedZone("HKT", 8*60*60))
	snap := quoteSnapshotFromBasicQotAt(basicQot, "HK.00700", lunchBreakAt)

	if snap.Session != market.SessionUnknown {
		t.Errorf("Session = %s, want unknown", snap.Session)
	}
	if snap.PreviousClosePrice == nil {
		t.Fatal("PreviousClosePrice is nil, expected 318.90")
	}
	if !snap.PreviousClosePrice.Equal(decimal.NewFromFloat(318.90)) {
		t.Errorf("PreviousClosePrice = %s, want 318.90", snap.PreviousClosePrice.String())
	}
	if snap.Price.Equal(*snap.PreviousClosePrice) {
		t.Fatalf("Price and PreviousClosePrice unexpectedly match: %s", snap.Price.String())
	}
}

func TestPreviousClosePriceConditionBySessionType(t *testing.T) {
	// Verify the core condition: session != market.SessionRegular
	// should cause PreviousClosePrice to use CurPrice instead of LastClosePrice.
	regularSessionClose := decimal.NewFromFloat(9.22)
	lastClosePrice := decimal.NewFromFloat(9.09)

	cases := []struct {
		name            string
		session         market.Session
		expectCurPrice  bool
		expectLastClose bool
	}{
		{"regular uses LastClosePrice", market.SessionRegular, false, true},
		{"pre uses CurPrice", market.SessionPre, true, false},
		{"after uses CurPrice", market.SessionAfter, true, false},
		{"overnight uses CurPrice", market.SessionOvernight, true, false},
		{"closed uses CurPrice", market.SessionClosed, true, false},
		{"unknown uses CurPrice for US only", market.SessionUnknown, true, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			useCurPrice := market.ShouldUseRegularCloseAsPreviousClose("US.TME", tc.session, regularSessionClose)
			if tc.expectCurPrice && !useCurPrice {
				t.Errorf("session=%s: expected to use CurPrice, but condition is false", tc.session)
			}
			if tc.expectLastClose && useCurPrice {
				t.Errorf("session=%s: expected to use LastClosePrice, but condition is true", tc.session)
			}
			if tc.session == market.SessionClosed && !useCurPrice {
				t.Errorf("session=closed: condition should use CurPrice but doesn't")
			}
		})
	}

	// Sanity: ensure the result matches LastClosePrice when regular.
	if regularSessionClose.Equal(lastClosePrice) {
		t.Fatal("test data sanity: regularSessionClose should differ from lastClosePrice")
	}
}

func TestPreviousClosePriceConditionDoesNotRewriteNonUSUnknownSession(t *testing.T) {
	regularSessionClose := decimal.NewFromFloat(321.40)

	useCurPrice := market.ShouldUseRegularCloseAsPreviousClose("HK.00700", market.SessionUnknown, regularSessionClose)
	if useCurPrice {
		t.Fatal("HK unknown session should not rewrite PreviousClosePrice to CurPrice")
	}
}

func protoInt32(v int32) *int32    { return &v }
func protoInt64(v int64) *int64    { return &v }
func protoString(v string) *string { return &v }
