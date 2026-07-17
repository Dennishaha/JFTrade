package market

import (
	"testing"
	"time"
)

func TestInstrumentAndSessionValidationCoverage(t *testing.T) {
	for _, input := range []InstrumentInput{
		{Market: "unsupported", Symbol: "AAPL"},
		{Symbol: "US.AAPL", Code: "MSFT"},
		{Symbol: "AAPL"},
		{Market: "CN", Code: "600519"},
	} {
		if _, err := ParseInstrument(input); err == nil {
			t.Fatalf("ParseInstrument(%+v) error = nil", input)
		}
	}
	if _, err := ParseQualifiedInstrumentSymbol("unsupported.AAPL"); err == nil {
		t.Fatal("ParseQualifiedInstrumentSymbol accepted unsupported market")
	}
	parsed, err := ParseQualifiedInstrumentSymbol("cnsH:600519")
	if err != nil || parsed.Symbol != "SH.600519" || parsed.Market != "CN" {
		t.Fatalf("ParseQualifiedInstrumentSymbol alias = %+v, %v", parsed, err)
	}
	if minutes, ok := tradingMinutesPerRegularDay("unknown"); ok || minutes != 0 {
		t.Fatalf("tradingMinutesPerRegularDay unknown = %d,%v", minutes, ok)
	}
}

func TestTimeBoundaryValidationCoverage(t *testing.T) {
	if _, ok := TradingDayBoundaryStart("unknown.AAPL", time.Now(), false); ok {
		t.Fatal("TradingDayBoundaryStart accepted an unknown market")
	}
	if _, ok := TradingPeriodLabelStart("US.AAPL", time.Time{}, "day", false); ok {
		t.Fatal("TradingPeriodLabelStart accepted zero time")
	}
	if _, ok := TradingPeriodLabelStartForDate("US.AAPL", time.Now(), "quarter"); ok {
		t.Fatal("TradingPeriodLabelStartForDate accepted unsupported period")
	}
	if _, _, ok := SessionAwareIntradayBucketBounds("US.AAPL", time.Time{}, time.Minute, false); ok {
		t.Fatal("SessionAwareIntradayBucketBounds accepted zero time")
	}
	if _, _, ok := sessionWindowBounds("unknown.AAPL", time.Now(), false); ok {
		t.Fatal("sessionWindowBounds accepted unknown market")
	}
}
