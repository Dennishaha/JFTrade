package futu

import (
	"testing"
	"time"

	trdcommonpb "github.com/jftrade/jftrade-main/pkg/futu/pb/trdcommon"
)

func TestFutuQuoteTimeUsesMarketTimezoneForFallback(t *testing.T) {
	originalLocal := time.Local
	time.Local = time.FixedZone("host-local", -7*60*60)
	t.Cleanup(func() {
		time.Local = originalLocal
	})

	tests := []struct {
		name     string
		symbol   string
		fallback string
		want     time.Time
	}{
		{
			name:     "US quote",
			symbol:   "US.AAPL",
			fallback: "2026-06-20 09:30:00",
			want:     time.Date(2026, time.June, 20, 13, 30, 0, 0, time.UTC),
		},
		{
			name:     "Hong Kong quote",
			symbol:   "HK.00700",
			fallback: "2026-06-20 09:30:00",
			want:     time.Date(2026, time.June, 20, 1, 30, 0, 0, time.UTC),
		},
		{
			name:     "unknown market defaults to UTC",
			symbol:   "",
			fallback: "2026-06-20 09:30:00",
			want:     time.Date(2026, time.June, 20, 9, 30, 0, 0, time.UTC),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := futuQuoteTime(0, tc.fallback, tc.symbol)
			if !got.Equal(tc.want) {
				t.Fatalf("futuQuoteTime() = %s, want %s", got, tc.want)
			}
			if got.Location() != time.UTC {
				t.Fatalf("futuQuoteTime() location = %s, want UTC", got.Location())
			}
		})
	}
	recordedAt := time.Date(2026, time.June, 20, 12, 34, 56, 789, time.FixedZone("recorded-local", 9*60*60))
	if got := futuQuoteTimeAt(0, "not-a-time", "US.AAPL", recordedAt); !got.Equal(recordedAt) || got.Location() != time.UTC {
		t.Fatalf("invalid quote time fallback = %s (%s), want recordedAt in UTC", got, got.Location())
	}
}

func TestFormatBrokerOrderTimeUsesMarketTimezoneForFallback(t *testing.T) {
	originalLocal := time.Local
	time.Local = time.FixedZone("host-local", -7*60*60)
	t.Cleanup(func() {
		time.Local = originalLocal
	})

	if got, want := formatBrokerOrderTime(nil, "2026-06-20 09:30:00", "HK.00700"), "2026-06-20T01:30:00Z"; got != want {
		t.Fatalf("formatBrokerOrderTime() = %q, want %q", got, want)
	}
	if got, want := formatBrokerOrderTime(nil, "2026-06-20 09:30:00", "US.AAPL"), "2026-06-20T13:30:00Z"; got != want {
		t.Fatalf("formatBrokerOrderTime() = %q, want %q", got, want)
	}
	recordedAt := time.Date(2026, time.June, 20, 12, 34, 56, 789, time.FixedZone("recorded-local", 9*60*60))
	if got, want := formatBrokerOrderTimeAt(nil, "not-a-time", "US.AAPL", recordedAt), "2026-06-20T03:34:56.000000789Z"; got != want {
		t.Fatalf("invalid broker time = %q, want recordedAt %q", got, want)
	}
}

func TestBrokerOrderSnapshotUsesResolvedMarketForBareCodeFallbackTime(t *testing.T) {
	if got := brokerOrderTimeSymbol("HK", "00700"); got != "HK.00700" {
		t.Fatalf("brokerOrderTimeSymbol() = %q, want HK.00700", got)
	}
	if got := brokerOrderTimeSymbol("CN", "600519"); got != "SH.600519" {
		t.Fatalf("brokerOrderTimeSymbol() = %q, want SH.600519 timezone authority", got)
	}
	snapshot := brokerOrderSnapshotFromProto(resolvedTradeAccount{Market: "HK"}, &trdcommonpb.Order{
		Code:       new("00700"),
		CreateTime: new("2026-06-20 09:30:00"),
		UpdateTime: new("2026-06-20 09:31:00"),
	})
	if snapshot.SubmittedAt != "2026-06-20T01:30:00Z" {
		t.Fatalf("SubmittedAt = %q for market %q, want Hong Kong local time normalized to UTC", snapshot.SubmittedAt, snapshot.Market)
	}
	if snapshot.UpdatedAt != "2026-06-20T01:31:00Z" {
		t.Fatalf("UpdatedAt = %q, want Hong Kong local time normalized to UTC", snapshot.UpdatedAt)
	}

	marketValue := int32(trdcommonpb.TrdMarket_TrdMarket_HK)
	fill := BrokerOrderFillSnapshotFromPush(
		&trdcommonpb.TrdHeader{TrdMarket: &marketValue},
		&trdcommonpb.OrderFill{Code: new("00700"), CreateTime: new("2026-06-20 09:32:00")},
	)
	if fill.FilledAt != "2026-06-20T01:32:00Z" {
		t.Fatalf("FilledAt = %q, want Hong Kong local time normalized to UTC", fill.FilledAt)
	}
}
