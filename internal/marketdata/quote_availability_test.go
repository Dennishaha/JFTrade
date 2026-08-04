package marketdata

import (
	"testing"

	"github.com/shopspring/decimal"
)

func TestSnapshotSerializationPreservesAuthoritativeMissingQuoteFields(t *testing.T) {
	tick := &Tick{
		InstrumentID: "US.AAPL", Market: "US", Symbol: "AAPL",
		Price: decimal.NewFromInt(100), Bid: decimal.Zero, Ask: decimal.Zero,
		Volume: decimal.Zero, Turnover: decimal.Zero,
		Availability: QuoteFieldAvailability{Authoritative: true}, Source: "akshare:eastmoney",
	}
	snapshot := SnapshotJSON(tick)
	for _, field := range []string{"bid", "ask", "volume", "turnover"} {
		if snapshot[field] != nil {
			t.Fatalf("snapshot %s = %#v, want nil", field, snapshot[field])
		}
	}
	event := LiveTickJSON(tick, "")
	if event["cumulativeVolume"] != nil || event["snapshot"].(map[string]any)["volume"] != nil ||
		event["brokerId"] != "akshare" {
		t.Fatalf("nullable live event = %#v", event)
	}
	latest := LatestTicksJSON([]*Tick{tick})
	entry := latest["ticks"].([]map[string]any)[0]
	if entry["bid"] != nil || entry["ask"] != nil || entry["volume"] != nil ||
		entry["cumulativeVolume"] != nil {
		t.Fatalf("nullable latest tick = %#v", entry)
	}
}

func TestSnapshotSerializationKeepsLegacyZeroValuesAvailable(t *testing.T) {
	tick := &Tick{Price: decimal.NewFromInt(100)}
	snapshot := SnapshotJSON(tick)
	for _, field := range []string{"bid", "ask", "volume", "turnover"} {
		if snapshot[field] != "0" {
			t.Fatalf("legacy snapshot %s = %#v, want zero string", field, snapshot[field])
		}
	}
}
