package marketdata

import (
	"context"
	"errors"
	"strings"
	"testing"
)

type calendarMacroCapableProviderStub struct {
	dataProviderStub
	earnings      EarningsCalendarResponse
	earningsErr   error
	earningsBegin string
	earningsEnd   string
	dividends     DividendCalendarResponse
	dividendsErr  error
	dividendsDate string
	economic      EconomicCalendarResponse
	economicErr   error
	ipos          IpoCalendarResponse
	iposErr       error
	indicators    MacroIndicatorsResponse
	indicatorsErr error
	history       MacroIndicatorHistoryResponse
	historyErr    error
	historyID     string
	historyLimit  int
}

func (p *calendarMacroCapableProviderStub) EarningsCalendar(
	_ context.Context, beginDate, endDate string,
) (EarningsCalendarResponse, error) {
	p.earningsBegin, p.earningsEnd = beginDate, endDate
	return p.earnings, p.earningsErr
}

func (p *calendarMacroCapableProviderStub) DividendCalendar(
	_ context.Context, date string,
) (DividendCalendarResponse, error) {
	p.dividendsDate = date
	return p.dividends, p.dividendsErr
}

func (p *calendarMacroCapableProviderStub) EconomicCalendar(
	_ context.Context, beginDate, endDate string,
) (EconomicCalendarResponse, error) {
	return p.economic, p.economicErr
}

func (p *calendarMacroCapableProviderStub) IpoCalendar(context.Context) (IpoCalendarResponse, error) {
	return p.ipos, p.iposErr
}

func (p *calendarMacroCapableProviderStub) MacroIndicators(context.Context) (MacroIndicatorsResponse, error) {
	return p.indicators, p.indicatorsErr
}

func (p *calendarMacroCapableProviderStub) MacroIndicatorHistory(
	_ context.Context, indicatorID string, limit int,
) (MacroIndicatorHistoryResponse, error) {
	p.historyID, p.historyLimit = indicatorID, limit
	return p.history, p.historyErr
}

func TestServiceCalendarMacroRejectsProvidersWithoutCapability(t *testing.T) {
	service := NewService(&dataProviderStub{})
	ctx := context.Background()

	cases := []struct {
		name string
		call func() error
		want string
	}{
		{"earnings", func() error {
			_, err := service.GetEarningsCalendar(ctx, "2026-08-01", "2026-08-31")
			return err
		}, "event calendar"},
		{"dividends", func() error {
			_, err := service.GetDividendCalendar(ctx, "2026-08-15")
			return err
		}, "event calendar"},
		{"economic", func() error {
			_, err := service.GetEconomicCalendar(ctx, "", "")
			return err
		}, "event calendar"},
		{"ipos", func() error {
			_, err := service.GetIpoCalendar(ctx)
			return err
		}, "event calendar"},
		{"indicators", func() error {
			_, err := service.GetMacroIndicators(ctx)
			return err
		}, "macro indicators"},
		{"history", func() error {
			_, err := service.GetMacroIndicatorHistory(ctx, "cpi_yoy", 0)
			return err
		}, "macro indicators"},
	}
	for _, tc := range cases {
		err := tc.call()
		if !errors.Is(err, ErrCapabilityUnsupported) || !strings.Contains(err.Error(), "stub-provider") ||
			!strings.Contains(err.Error(), tc.want) {
			t.Fatalf("%s unsupported error = %v", tc.name, err)
		}
	}
}

func TestServiceCalendarValidatesDateFormats(t *testing.T) {
	provider := &calendarMacroCapableProviderStub{
		earnings: EarningsCalendarResponse{Entries: []EarningsEvent{{InstrumentID: "SH.600519"}}},
	}
	service := NewService(provider)
	ctx := context.Background()

	if _, err := service.GetEarningsCalendar(ctx, "2026/08/01", ""); err == nil ||
		!strings.Contains(err.Error(), "YYYY-MM-DD") {
		t.Fatalf("beginDate format error = %v", err)
	}
	if _, err := service.GetEconomicCalendar(ctx, "2026-08-01", "08-31"); err == nil {
		t.Fatal("invalid endDate must fail")
	}
	if _, err := service.GetDividendCalendar(ctx, "2026-13-01"); err == nil {
		t.Fatal("impossible date must fail")
	}
	if _, err := service.GetEarningsCalendar(ctx, "2026-08-01", "2026-08-31"); err != nil {
		t.Fatalf("valid range: %v", err)
	}
	if provider.earningsBegin != "2026-08-01" || provider.earningsEnd != "2026-08-31" {
		t.Fatalf("earnings range forwarded = %q/%q", provider.earningsBegin, provider.earningsEnd)
	}
	// Empty bounds pass through: the sidecar applies its default window.
	if _, err := service.GetEconomicCalendar(ctx, "", ""); err != nil {
		t.Fatalf("empty range must pass: %v", err)
	}
	if _, err := service.GetDividendCalendar(ctx, " 2026-08-15 "); err != nil {
		t.Fatalf("padded date: %v", err)
	}
	if provider.dividendsDate != " 2026-08-15 " {
		t.Fatalf("dividends date forwarded = %q", provider.dividendsDate)
	}
}

func TestServiceMacroIndicatorHistoryValidatesIDAndLimit(t *testing.T) {
	provider := &calendarMacroCapableProviderStub{}
	service := NewService(provider)
	ctx := context.Background()

	if _, err := service.GetMacroIndicatorHistory(ctx, " ", 0); err == nil {
		t.Fatal("empty indicatorId must fail")
	}
	if _, err := service.GetMacroIndicatorHistory(ctx, "cpi_yoy", 99999); err == nil {
		t.Fatal("out-of-range limit must fail")
	}
	if _, err := service.GetMacroIndicatorHistory(ctx, "cpi_yoy", 0); err != nil {
		t.Fatalf("default limit: %v", err)
	}
	if provider.historyID != "cpi_yoy" || provider.historyLimit != DefaultMacroHistoryLimit {
		t.Fatalf("history forwarded = %q/%d", provider.historyID, provider.historyLimit)
	}
}

func TestServiceCalendarMacroPassesProviderErrorsThrough(t *testing.T) {
	provider := &calendarMacroCapableProviderStub{}
	service := NewService(provider)
	want := errors.New("calendar upstream failed")
	provider.iposErr = want
	if _, err := service.GetIpoCalendar(context.Background()); !errors.Is(err, want) {
		t.Fatalf("ipo error passthrough = %v", err)
	}
	provider.indicatorsErr = want
	if _, err := service.GetMacroIndicators(context.Background()); !errors.Is(err, want) {
		t.Fatalf("indicators error passthrough = %v", err)
	}
}
