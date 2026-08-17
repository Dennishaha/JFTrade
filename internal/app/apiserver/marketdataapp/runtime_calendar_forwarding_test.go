package marketdataapp

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jftrade/jftrade-main/internal/marketdata"
)

// calendarMacroStub 覆盖日历与宏观六项读取，供运行时转发测试使用。
type calendarMacroStub struct {
	forwardingProviderStub
	lastRange   [2]string
	lastDate    string
	lastID      string
	lastLimit   int
	calendarErr error
}

func (p *calendarMacroStub) EarningsCalendar(_ context.Context, beginDate, endDate string) (marketdata.EarningsCalendarResponse, error) {
	p.record("earnings-calendar")
	p.lastRange = [2]string{beginDate, endDate}
	if p.calendarErr != nil {
		return marketdata.EarningsCalendarResponse{}, p.calendarErr
	}
	return marketdata.EarningsCalendarResponse{Source: "stub"}, nil
}

func (p *calendarMacroStub) DividendCalendar(_ context.Context, date string) (marketdata.DividendCalendarResponse, error) {
	p.record("dividend-calendar")
	p.lastDate = date
	if p.calendarErr != nil {
		return marketdata.DividendCalendarResponse{}, p.calendarErr
	}
	return marketdata.DividendCalendarResponse{Source: "stub"}, nil
}

func (p *calendarMacroStub) EconomicCalendar(_ context.Context, beginDate, endDate string) (marketdata.EconomicCalendarResponse, error) {
	p.record("economic-calendar")
	p.lastRange = [2]string{beginDate, endDate}
	if p.calendarErr != nil {
		return marketdata.EconomicCalendarResponse{}, p.calendarErr
	}
	return marketdata.EconomicCalendarResponse{Source: "stub"}, nil
}

func (p *calendarMacroStub) IpoCalendar(context.Context) (marketdata.IpoCalendarResponse, error) {
	p.record("ipo-calendar")
	if p.calendarErr != nil {
		return marketdata.IpoCalendarResponse{}, p.calendarErr
	}
	return marketdata.IpoCalendarResponse{Source: "stub"}, nil
}

func (p *calendarMacroStub) MacroIndicators(context.Context) (marketdata.MacroIndicatorsResponse, error) {
	p.record("macro-indicators")
	if p.calendarErr != nil {
		return marketdata.MacroIndicatorsResponse{}, p.calendarErr
	}
	return marketdata.MacroIndicatorsResponse{Source: "stub"}, nil
}

func (p *calendarMacroStub) MacroIndicatorHistory(_ context.Context, indicatorID string, limit int) (marketdata.MacroIndicatorHistoryResponse, error) {
	p.record("macro-history")
	p.lastID, p.lastLimit = indicatorID, limit
	if p.calendarErr != nil {
		return marketdata.MacroIndicatorHistoryResponse{}, p.calendarErr
	}
	return marketdata.MacroIndicatorHistoryResponse{Source: "stub"}, nil
}

func TestRuntimeCalendarMacroForwarding(t *testing.T) {
	provider := &calendarMacroStub{}
	runtime, err := NewRuntime(RuntimeOptions{FutuProvider: provider})
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}
	ctx := context.Background()

	if _, err := runtime.EarningsCalendar(ctx, "2026-08-01", "2026-08-31"); err != nil {
		t.Fatalf("EarningsCalendar: %v", err)
	}
	if provider.lastRange != [2]string{"2026-08-01", "2026-08-31"} {
		t.Fatalf("earnings range forwarded = %v", provider.lastRange)
	}
	if _, err := runtime.DividendCalendar(ctx, "2026-08-15"); err != nil {
		t.Fatalf("DividendCalendar: %v", err)
	}
	if provider.lastDate != "2026-08-15" {
		t.Fatalf("dividend date forwarded = %q", provider.lastDate)
	}
	if _, err := runtime.EconomicCalendar(ctx, "", ""); err != nil {
		t.Fatalf("EconomicCalendar: %v", err)
	}
	if _, err := runtime.IpoCalendar(ctx); err != nil {
		t.Fatalf("IpoCalendar: %v", err)
	}
	if _, err := runtime.MacroIndicators(ctx); err != nil {
		t.Fatalf("MacroIndicators: %v", err)
	}
	if _, err := runtime.MacroIndicatorHistory(ctx, "cpi_yoy", 120); err != nil {
		t.Fatalf("MacroIndicatorHistory: %v", err)
	}
	if provider.lastID != "cpi_yoy" || provider.lastLimit != 120 {
		t.Fatalf("history forwarded = %q/%d", provider.lastID, provider.lastLimit)
	}
	for _, method := range []string{
		"earnings-calendar", "dividend-calendar", "economic-calendar",
		"ipo-calendar", "macro-indicators", "macro-history",
	} {
		if got := provider.calls[method]; got != 1 {
			t.Fatalf("expected %s call, got %d", method, got)
		}
	}
}

func TestRuntimeCalendarMacroPropagatesError(t *testing.T) {
	want := errors.New("calendar failed")
	provider := &calendarMacroStub{calendarErr: want}
	runtime, err := NewRuntime(RuntimeOptions{FutuProvider: provider})
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}
	if _, err := runtime.IpoCalendar(context.Background()); !errors.Is(err, want) {
		t.Fatalf("expected provider error, got %v", err)
	}
}

func TestRuntimeCalendarMacroCapabilityUnsupported(t *testing.T) {
	runtime, err := NewRuntime(RuntimeOptions{FutuProvider: &forwardingProviderStub{}})
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}
	ctx := context.Background()

	cases := []struct {
		name string
		call func() error
		want string
	}{
		{"earnings", func() error {
			_, err := runtime.EarningsCalendar(ctx, "", "")
			return err
		}, "event calendars"},
		{"dividends", func() error {
			_, err := runtime.DividendCalendar(ctx, "2026-08-15")
			return err
		}, "event calendars"},
		{"economic", func() error {
			_, err := runtime.EconomicCalendar(ctx, "", "")
			return err
		}, "event calendars"},
		{"ipos", func() error {
			_, err := runtime.IpoCalendar(ctx)
			return err
		}, "event calendars"},
		{"indicators", func() error {
			_, err := runtime.MacroIndicators(ctx)
			return err
		}, "macro indicators"},
		{"history", func() error {
			_, err := runtime.MacroIndicatorHistory(ctx, "cpi_yoy", 0)
			return err
		}, "macro indicators"},
	}
	for _, tc := range cases {
		err := tc.call()
		if err == nil {
			t.Fatalf("%s: expected capability error", tc.name)
		}
		if !errors.Is(err, marketdata.ErrCapabilityUnsupported) {
			t.Fatalf("%s: expected ErrCapabilityUnsupported, got %v", tc.name, err)
		}
		if got := err.Error(); !strings.Contains(got, ProviderFutu) || !strings.Contains(got, tc.want) {
			t.Fatalf("%s: unexpected error message: %q", tc.name, got)
		}
	}
}
