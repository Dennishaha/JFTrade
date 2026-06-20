package market

import (
	"strings"
	"testing"
	"time"

	"github.com/shopspring/decimal"
)

func TestClassifySessionForUS(t *testing.T) {
	loc := mustLocation(t, "America/New_York")
	cases := []struct {
		name string
		at   time.Time
		want Session
	}{
		{"overnight before premarket", time.Date(2026, 6, 12, 3, 59, 0, 0, loc), SessionOvernight},
		{"pre", time.Date(2026, 6, 12, 4, 0, 0, 0, loc), SessionPre},
		{"regular", time.Date(2026, 6, 12, 9, 30, 0, 0, loc), SessionRegular},
		{"after", time.Date(2026, 6, 12, 16, 0, 0, 0, loc), SessionAfter},
		{"friday closed", time.Date(2026, 6, 12, 20, 0, 0, 0, loc), SessionClosed},
		{"saturday closed", time.Date(2026, 6, 13, 10, 0, 0, 0, loc), SessionClosed},
		{"sunday overnight", time.Date(2026, 6, 14, 20, 0, 0, 0, loc), SessionOvernight},
		{"juneteenth holiday closed", time.Date(2026, 6, 19, 12, 0, 0, 0, loc), SessionClosed},
		{"before holiday does not enter overnight", time.Date(2026, 6, 18, 20, 30, 0, 0, loc), SessionClosed},
		{"black friday early close after-hours", time.Date(2026, 11, 27, 13, 30, 0, 0, loc), SessionAfter},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ClassifySession("US.AAPL", tc.at); got != tc.want {
				t.Fatalf("ClassifySession = %s, want %s", got, tc.want)
			}
		})
	}
}

func TestIsRegularTradingTimeUsesHolidayCalendarForUS(t *testing.T) {
	loc := mustLocation(t, "America/New_York")
	if IsRegularTradingTime("US.AAPL", time.Date(2026, 6, 19, 10, 0, 0, 0, loc)) {
		t.Fatal("Juneteenth should not be a regular trading session")
	}
	if IsRegularTradingTime("US.AAPL", time.Date(2026, 11, 27, 14, 0, 0, 0, loc)) {
		t.Fatal("Black Friday 14:00 ET should be after-hours on an early close day")
	}
	if !IsRegularTradingTime("US.AAPL", time.Date(2026, 11, 27, 12, 30, 0, 0, loc)) {
		t.Fatal("Black Friday 12:30 ET should still be regular trading time")
	}
}

func TestRegularTradingTimeForHKAndChinaLunchBreaks(t *testing.T) {
	hkLoc := mustLocation(t, "Asia/Hong_Kong")
	shLoc := mustLocation(t, "Asia/Shanghai")
	cases := []struct {
		name   string
		symbol string
		at     time.Time
		want   bool
	}{
		{"hk morning open", "HK.00700", time.Date(2026, 6, 12, 9, 30, 0, 0, hkLoc), true},
		{"hk lunch break", "HK.00700", time.Date(2026, 6, 12, 12, 30, 0, 0, hkLoc), false},
		{"hk afternoon open", "HK.00700", time.Date(2026, 6, 12, 13, 0, 0, 0, hkLoc), true},
		{"hk weekend", "HK.00700", time.Date(2026, 6, 13, 10, 0, 0, 0, hkLoc), false},
		{"sh lunch break", "SH.600519", time.Date(2026, 6, 12, 12, 30, 0, 0, shLoc), false},
		{"sz lunch break", "SZ.000001", time.Date(2026, 6, 12, 12, 30, 0, 0, shLoc), false},
		{"sh afternoon open", "SH.600519", time.Date(2026, 6, 12, 13, 0, 0, 0, shLoc), true},
		{"sz afternoon open", "SZ.000001", time.Date(2026, 6, 12, 13, 0, 0, 0, shLoc), true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := IsRegularTradingTime(tc.symbol, tc.at); got != tc.want {
				t.Fatalf("IsRegularTradingTime = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestNormalizeMarketInputAndParseInstrument(t *testing.T) {
	cases := []struct {
		name            string
		input           InstrumentInput
		wantMarket      string
		wantPrefix      string
		wantCode        string
		wantSymbol      string
		wantErrContains string
	}{
		{"us bare", InstrumentInput{Market: "us", Symbol: "aapl"}, "US", "US", "AAPL", "US.AAPL", ""},
		{"hk colon", InstrumentInput{Symbol: "hk:00700"}, "HK", "HK", "00700", "HK.00700", ""},
		{"sh maps to cn", InstrumentInput{Market: "SH", Symbol: "600519"}, "CN", "SH", "600519", "SH.600519", ""},
		{"sz maps to cn", InstrumentInput{Symbol: "SZ.000001"}, "CN", "SZ", "000001", "SZ.000001", ""},
		{"instrument id normalizes", InstrumentInput{InstrumentID: "us:aapl"}, "US", "US", "AAPL", "US.AAPL", ""},
		{"cn bare rejected", InstrumentInput{Market: "CN", Symbol: "600519"}, "", "", "", "", "requires an exchange-qualified symbol"},
		{"cn qualified rejected", InstrumentInput{Symbol: "CN.600519"}, "", "", "", "", "requires an exchange-qualified symbol"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ParseInstrument(tc.input)
			if tc.wantErrContains != "" {
				if err == nil || !contains(err.Error(), tc.wantErrContains) {
					t.Fatalf("ParseInstrument error = %v, want containing %q", err, tc.wantErrContains)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseInstrument error = %v", err)
			}
			if got.Market != tc.wantMarket || got.Prefix != tc.wantPrefix || got.Code != tc.wantCode || got.Symbol != tc.wantSymbol {
				t.Fatalf("ParseInstrument = %#v, want market=%s prefix=%s code=%s symbol=%s", got, tc.wantMarket, tc.wantPrefix, tc.wantCode, tc.wantSymbol)
			}
		})
	}
}

func TestMarketDescriptorsExposeFrontendMetadata(t *testing.T) {
	descriptors := MarketDescriptors()
	if len(descriptors) < 5 {
		t.Fatalf("MarketDescriptors len = %d, want at least 5", len(descriptors))
	}
	byCode := make(map[string]MarketDescriptor, len(descriptors))
	for _, descriptor := range descriptors {
		byCode[descriptor.Code] = descriptor
	}
	us := byCode["US"]
	if us.QuoteCurrency != "USD" || us.PricePrecision != 2 || us.TickSize != 0.01 || !us.SupportsExtendedHours {
		t.Fatalf("US descriptor = %#v", us)
	}
	hk := byCode["HK"]
	if hk.QuoteCurrency != "HKD" || hk.PricePrecision != 3 || hk.SupportsExtendedHours {
		t.Fatalf("HK descriptor = %#v", hk)
	}
	cn := byCode["CN"]
	if !cn.RequiresExchangePrefix || cn.PreferredPrefix != "" || !contains(strings.Join(cn.Aliases, ","), "SH") {
		t.Fatalf("CN descriptor = %#v", cn)
	}
	sh := byCode["SH"]
	if sh.ResolvedMarket != "CN" || sh.PreferredPrefix != "SH" || sh.QuoteCurrency != "CNY" {
		t.Fatalf("SH descriptor = %#v", sh)
	}
}

func TestShouldUseRegularCloseAsPreviousClose(t *testing.T) {
	regularClose := decimal.NewFromFloat(321.40)
	if !ShouldUseRegularCloseAsPreviousClose("US.AAPL", SessionAfter, regularClose) {
		t.Fatal("US after-hours should use regular close as previous close")
	}
	if ShouldUseRegularCloseAsPreviousClose("US.AAPL", SessionRegular, regularClose) {
		t.Fatal("US regular session should not rewrite previous close")
	}
	if ShouldUseRegularCloseAsPreviousClose("HK.00700", SessionUnknown, regularClose) {
		t.Fatal("HK unknown/lunch session should not rewrite previous close")
	}
	if ShouldUseRegularCloseAsPreviousClose("SH.600519", SessionUnknown, regularClose) {
		t.Fatal("SH unknown/lunch session should not rewrite previous close")
	}
	if ShouldUseRegularCloseAsPreviousClose("SZ.000001", SessionUnknown, regularClose) {
		t.Fatal("SZ unknown/lunch session should not rewrite previous close")
	}
}

func TestTradingPeriodKeys(t *testing.T) {
	nyLoc := mustLocation(t, "America/New_York")
	hkLoc := mustLocation(t, "Asia/Hong_Kong")
	shLoc := mustLocation(t, "Asia/Shanghai")

	cases := []struct {
		name     string
		symbol   string
		at       time.Time
		extended bool
		want     string
		wantOK   bool
	}{
		{"us regular day", "US.AAPL", time.Date(2026, 6, 12, 10, 0, 0, 0, nyLoc), false, "2026-06-12", true},
		{"us overnight rolls forward", "US.AAPL", time.Date(2026, 6, 14, 20, 30, 0, 0, nyLoc), true, "2026-06-15", true},
		{"us premarket extended day", "US.AAPL", time.Date(2026, 6, 12, 5, 0, 0, 0, nyLoc), true, "2026-06-12", true},
		{"us premarket regular rejected", "US.AAPL", time.Date(2026, 6, 12, 5, 0, 0, 0, nyLoc), false, "", false},
		{"hk lunch rejected", "HK.00700", time.Date(2026, 6, 12, 12, 30, 0, 0, hkLoc), true, "", false},
		{"hk afternoon day", "HK.00700", time.Date(2026, 6, 12, 13, 30, 0, 0, hkLoc), true, "2026-06-12", true},
		{"sh lunch rejected", "SH.600519", time.Date(2026, 6, 12, 12, 30, 0, 0, shLoc), true, "", false},
		{"sz afternoon day", "SZ.000001", time.Date(2026, 6, 12, 13, 30, 0, 0, shLoc), true, "2026-06-12", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := TradingPeriodKey(tc.symbol, tc.at, "day", tc.extended)
			if ok != tc.wantOK || got != tc.want {
				t.Fatalf("TradingPeriodKey = %q, %v; want %q, %v", got, ok, tc.want, tc.wantOK)
			}
		})
	}
}

func TestTradingPeriodLabelStart(t *testing.T) {
	nyLoc := mustLocation(t, "America/New_York")
	at := time.Date(2026, 6, 14, 20, 30, 0, 0, nyLoc)

	got, ok := TradingPeriodLabelStart("US.AAPL", at, "day", true)
	if !ok {
		t.Fatal("expected label start for US overnight extended session")
	}
	want := time.Date(2026, 6, 15, 0, 0, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Fatalf("TradingPeriodLabelStart = %s, want %s", got, want)
	}
}

func TestTradingDayBoundaryStart(t *testing.T) {
	nyLoc := mustLocation(t, "America/New_York")
	hkLoc := mustLocation(t, "Asia/Hong_Kong")

	usStart, ok := TradingDayBoundaryStart("US.AAPL", time.Date(2026, 6, 14, 20, 30, 0, 0, nyLoc), true)
	if !ok {
		t.Fatal("expected US overnight trading-day boundary")
	}
	if want := time.Date(2026, 6, 15, 0, 0, 0, 0, time.UTC); !usStart.Equal(want) {
		t.Fatalf("US overnight boundary = %s, want %s", usStart, want)
	}

	hkStart, ok := TradingDayBoundaryStart("HK.00700", time.Date(2026, 6, 15, 10, 0, 0, 0, hkLoc), true)
	if !ok {
		t.Fatal("expected HK trading-day boundary")
	}
	if want := time.Date(2026, 6, 14, 16, 0, 0, 0, time.UTC); !hkStart.Equal(want) {
		t.Fatalf("HK boundary = %s, want %s", hkStart, want)
	}
}

func TestSessionAwareIntradayBucketBounds(t *testing.T) {
	nyLoc := mustLocation(t, "America/New_York")
	hkLoc := mustLocation(t, "Asia/Hong_Kong")
	shLoc := mustLocation(t, "Asia/Shanghai")

	cases := []struct {
		name      string
		symbol    string
		at        time.Time
		extended  bool
		wantStart time.Time
		wantEnd   time.Time
		wantOK    bool
	}{
		{
			name:      "us overnight extended bucket",
			symbol:    "US.AAPL",
			at:        time.Date(2026, 6, 12, 3, 30, 0, 0, nyLoc),
			extended:  true,
			wantStart: time.Date(2026, 6, 12, 6, 0, 0, 0, time.UTC),
			wantEnd:   time.Date(2026, 6, 12, 7, 59, 59, int(999*time.Millisecond), time.UTC),
			wantOK:    true,
		},
		{
			name:      "us premarket extended bucket",
			symbol:    "US.AAPL",
			at:        time.Date(2026, 6, 12, 5, 0, 0, 0, nyLoc),
			extended:  true,
			wantStart: time.Date(2026, 6, 12, 8, 0, 0, 0, time.UTC),
			wantEnd:   time.Date(2026, 6, 12, 9, 59, 59, int(999*time.Millisecond), time.UTC),
			wantOK:    true,
		},
		{
			name:      "us regular bucket starts from open",
			symbol:    "US.AAPL",
			at:        time.Date(2026, 6, 12, 10, 0, 0, 0, nyLoc),
			extended:  true,
			wantStart: time.Date(2026, 6, 12, 13, 30, 0, 0, time.UTC),
			wantEnd:   time.Date(2026, 6, 12, 15, 29, 59, int(999*time.Millisecond), time.UTC),
			wantOK:    true,
		},
		{
			name:     "hk lunch rejected",
			symbol:   "HK.00700",
			at:       time.Date(2026, 6, 12, 12, 30, 0, 0, hkLoc),
			extended: true,
			wantOK:   false,
		},
		{
			name:      "hk afternoon bucket starts from afternoon open",
			symbol:    "HK.00700",
			at:        time.Date(2026, 6, 12, 13, 15, 0, 0, hkLoc),
			extended:  true,
			wantStart: time.Date(2026, 6, 12, 5, 0, 0, 0, time.UTC),
			wantEnd:   time.Date(2026, 6, 12, 6, 59, 59, int(999*time.Millisecond), time.UTC),
			wantOK:    true,
		},
		{
			name:     "sh lunch rejected",
			symbol:   "SH.600519",
			at:       time.Date(2026, 6, 12, 12, 30, 0, 0, shLoc),
			extended: true,
			wantOK:   false,
		},
		{
			name:      "sz afternoon bucket starts from afternoon open",
			symbol:    "SZ.000001",
			at:        time.Date(2026, 6, 12, 13, 15, 0, 0, shLoc),
			extended:  true,
			wantStart: time.Date(2026, 6, 12, 5, 0, 0, 0, time.UTC),
			wantEnd:   time.Date(2026, 6, 12, 6, 59, 59, int(999*time.Millisecond), time.UTC),
			wantOK:    true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gotStart, gotEnd, ok := SessionAwareIntradayBucketBounds(tc.symbol, tc.at, 2*time.Hour, tc.extended)
			if ok != tc.wantOK {
				t.Fatalf("SessionAwareIntradayBucketBounds ok = %v, want %v", ok, tc.wantOK)
			}
			if !ok {
				return
			}
			if !gotStart.Equal(tc.wantStart) || !gotEnd.Equal(tc.wantEnd) {
				t.Fatalf("SessionAwareIntradayBucketBounds = %s, %s; want %s, %s", gotStart, gotEnd, tc.wantStart, tc.wantEnd)
			}
		})
	}
}

func mustLocation(t *testing.T, name string) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation(name)
	if err != nil {
		t.Fatalf("LoadLocation(%q): %v", name, err)
	}
	return loc
}

func contains(value string, part string) bool {
	return strings.Contains(value, part)
}
