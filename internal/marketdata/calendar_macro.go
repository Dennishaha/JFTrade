package marketdata

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

const (
	// DefaultMacroHistoryLimit is the default number of indicator history points
	// a provider returns for one indicator.
	DefaultMacroHistoryLimit = 100
	// MaxMacroHistoryLimit bounds the points accepted by one history request.
	MaxMacroHistoryLimit = 500
)

// CalendarSource is an optional provider capability that supplies market-wide
// event calendars (earnings, dividends, economic events, IPOs). Calendar data
// is inherently cross-market, so no market scoping is applied; region
// filtering stays in the console.
type CalendarSource interface {
	EarningsCalendar(ctx context.Context, beginDate, endDate string) (EarningsCalendarResponse, error)
	DividendCalendar(ctx context.Context, date string) (DividendCalendarResponse, error)
	EconomicCalendar(ctx context.Context, beginDate, endDate string) (EconomicCalendarResponse, error)
	IpoCalendar(ctx context.Context) (IpoCalendarResponse, error)
}

// MacroSource is an optional provider capability that supplies macro indicator
// catalogs and per-indicator history series.
type MacroSource interface {
	MacroIndicators(ctx context.Context) (MacroIndicatorsResponse, error)
	MacroIndicatorHistory(ctx context.Context, indicatorID string, limit int) (MacroIndicatorHistoryResponse, error)
}

// EarningsEvent is one provider-neutral earnings calendar entry.
type EarningsEvent struct {
	InstrumentID string       `json:"instrumentId"`
	Name         string       `json:"name"`
	Symbol       string       `json:"symbol"`
	EventDate    string       `json:"eventDate"`
	PeriodText   string       `json:"periodText"`
	MarketCap    *json.Number `json:"marketCap"`
	Price        *json.Number `json:"price"`
}

// EarningsCalendarResponse is the provider-neutral earnings calendar payload.
type EarningsCalendarResponse struct {
	BeginDate string          `json:"beginDate"`
	EndDate   string          `json:"endDate"`
	Entries   []EarningsEvent `json:"entries"`
	Source    string          `json:"source"`
}

// DividendEvent is one provider-neutral dividend calendar entry.
type DividendEvent struct {
	InstrumentID string  `json:"instrumentId"`
	Name         string  `json:"name"`
	Symbol       string  `json:"symbol"`
	Statement    string  `json:"statement"`
	ExDate       string  `json:"exDate"`
	RecordDate   string  `json:"recordDate"`
	PayableDate  *string `json:"payableDate"`
}

// DividendCalendarResponse is the provider-neutral dividend calendar payload.
type DividendCalendarResponse struct {
	Date    string          `json:"date"`
	Entries []DividendEvent `json:"entries"`
	Source  string          `json:"source"`
}

// EconomicEvent is one provider-neutral economic calendar entry; event time is
// carried as a unix timestamp because upstream feeds publish mixed timezones.
type EconomicEvent struct {
	EventID        string  `json:"eventId"`
	Title          string  `json:"title"`
	Region         string  `json:"region"`
	EventTimestamp int64   `json:"eventTimestamp"`
	Importance     *int    `json:"importance"`
	PreviousValue  *string `json:"previousValue"`
	ForecastValue  *string `json:"forecastValue"`
	ActualValue    *string `json:"actualValue"`
}

// EconomicCalendarResponse is the provider-neutral economic calendar payload.
type EconomicCalendarResponse struct {
	BeginDate string          `json:"beginDate"`
	EndDate   string          `json:"endDate"`
	Entries   []EconomicEvent `json:"entries"`
	Source    string          `json:"source"`
}

// IpoEntry is one provider-neutral IPO calendar entry; numeric pricing fields
// are nullable because pending IPOs publish incomplete terms.
type IpoEntry struct {
	InstrumentID  string       `json:"instrumentId"`
	Name          string       `json:"name"`
	Symbol        string       `json:"symbol"`
	Status        string       `json:"status" enums:"listed,pending"`
	ListingDate   *string      `json:"listingDate"`
	IssueVolume   *json.Number `json:"issueVolume"`
	IssuePrice    *json.Number `json:"issuePrice"`
	IssuePriceMin *json.Number `json:"issuePriceMin"`
	IssuePriceMax *json.Number `json:"issuePriceMax"`
}

// IpoCalendarResponse is the provider-neutral IPO calendar payload.
type IpoCalendarResponse struct {
	Entries []IpoEntry `json:"entries"`
	Source  string     `json:"source"`
}

// MacroIndicator is one provider-neutral macro indicator descriptor.
type MacroIndicator struct {
	IndicatorID string       `json:"indicatorId"`
	Name        string       `json:"name"`
	Region      string       `json:"region"`
	Unit        string       `json:"unit"`
	UnitType    *json.Number `json:"unitType"`
	Frequency   string       `json:"frequency"`
}

// MacroIndicatorCategory groups indicators under a display category.
type MacroIndicatorCategory struct {
	CategoryName string           `json:"categoryName"`
	Indicators   []MacroIndicator `json:"indicators"`
}

// MacroIndicatorsResponse is the provider-neutral macro indicator catalog.
type MacroIndicatorsResponse struct {
	Categories []MacroIndicatorCategory `json:"categories"`
	Source     string                   `json:"source"`
}

// MacroIndicatorPoint is one provider-neutral indicator history point.
type MacroIndicatorPoint struct {
	DataTime      string       `json:"dataTime"`
	Value         *json.Number `json:"value"`
	PredictValue  *json.Number `json:"predictValue"`
	PreviousValue *json.Number `json:"previousValue"`
	Unit          string       `json:"unit"`
	UnitType      *json.Number `json:"unitType"`
}

// MacroIndicatorHistoryResponse is the provider-neutral indicator history.
type MacroIndicatorHistoryResponse struct {
	IndicatorID string                `json:"indicatorId"`
	Entries     []MacroIndicatorPoint `json:"entries"`
	Source      string                `json:"source"`
}

// GetEarningsCalendar 返回当前行情提供者的财报日历（跨市场）。
func (s *Service) GetEarningsCalendar(
	ctx context.Context,
	beginDate string,
	endDate string,
) (EarningsCalendarResponse, error) {
	s.providerLifecycleMu.RLock()
	defer s.providerLifecycleMu.RUnlock()
	source, err := s.calendarSource(ctx)
	if err != nil {
		return EarningsCalendarResponse{}, err
	}
	if err := requireCalendarRange(beginDate, endDate); err != nil {
		return EarningsCalendarResponse{}, err
	}
	return source.EarningsCalendar(ctx, beginDate, endDate)
}

// GetDividendCalendar 返回当前行情提供者的单日分红日历（跨市场）。
func (s *Service) GetDividendCalendar(
	ctx context.Context,
	date string,
) (DividendCalendarResponse, error) {
	s.providerLifecycleMu.RLock()
	defer s.providerLifecycleMu.RUnlock()
	source, err := s.calendarSource(ctx)
	if err != nil {
		return DividendCalendarResponse{}, err
	}
	if err := requireCalendarDate(date, "date"); err != nil {
		return DividendCalendarResponse{}, err
	}
	return source.DividendCalendar(ctx, date)
}

// GetEconomicCalendar 返回当前行情提供者的财经事件日历（跨市场）。
func (s *Service) GetEconomicCalendar(
	ctx context.Context,
	beginDate string,
	endDate string,
) (EconomicCalendarResponse, error) {
	s.providerLifecycleMu.RLock()
	defer s.providerLifecycleMu.RUnlock()
	source, err := s.calendarSource(ctx)
	if err != nil {
		return EconomicCalendarResponse{}, err
	}
	if err := requireCalendarRange(beginDate, endDate); err != nil {
		return EconomicCalendarResponse{}, err
	}
	return source.EconomicCalendar(ctx, beginDate, endDate)
}

// GetIpoCalendar 返回当前行情提供者的新股日历（跨市场）。
func (s *Service) GetIpoCalendar(ctx context.Context) (IpoCalendarResponse, error) {
	s.providerLifecycleMu.RLock()
	defer s.providerLifecycleMu.RUnlock()
	source, err := s.calendarSource(ctx)
	if err != nil {
		return IpoCalendarResponse{}, err
	}
	return source.IpoCalendar(ctx)
}

// GetMacroIndicators 返回当前行情提供者的宏观指标目录。
func (s *Service) GetMacroIndicators(ctx context.Context) (MacroIndicatorsResponse, error) {
	s.providerLifecycleMu.RLock()
	defer s.providerLifecycleMu.RUnlock()
	source, err := s.macroSource(ctx)
	if err != nil {
		return MacroIndicatorsResponse{}, err
	}
	return source.MacroIndicators(ctx)
}

// GetMacroIndicatorHistory 返回当前行情提供者的单指标历史序列。
func (s *Service) GetMacroIndicatorHistory(
	ctx context.Context,
	indicatorID string,
	limit int,
) (MacroIndicatorHistoryResponse, error) {
	s.providerLifecycleMu.RLock()
	defer s.providerLifecycleMu.RUnlock()
	source, err := s.macroSource(ctx)
	if err != nil {
		return MacroIndicatorHistoryResponse{}, err
	}
	indicatorID = strings.TrimSpace(indicatorID)
	if indicatorID == "" {
		return MacroIndicatorHistoryResponse{}, fmt.Errorf("macro indicator history requires an indicatorId")
	}
	if limit == 0 {
		limit = DefaultMacroHistoryLimit
	}
	if limit < 1 || limit > MaxMacroHistoryLimit {
		return MacroIndicatorHistoryResponse{}, fmt.Errorf(
			"macro history limit must be between 1 and %d", MaxMacroHistoryLimit,
		)
	}
	return source.MacroIndicatorHistory(ctx, indicatorID, limit)
}

func (s *Service) calendarSource(ctx context.Context) (CalendarSource, error) {
	if source, ok := s.provider.(CalendarSource); ok {
		return source, nil
	}
	return nil, s.optionalCapabilityError(ctx, "event calendar")
}

func (s *Service) macroSource(ctx context.Context) (MacroSource, error) {
	if source, ok := s.provider.(MacroSource); ok {
		return source, nil
	}
	return nil, s.optionalCapabilityError(ctx, "macro indicators")
}

// requireCalendarRange accepts empty bounds (the sidecar applies its default
// window) but pins any provided bound to the YYYY-MM-DD wire format.
func requireCalendarRange(beginDate, endDate string) error {
	if err := requireCalendarDate(beginDate, "beginDate"); err != nil {
		return err
	}
	return requireCalendarDate(endDate, "endDate")
}

func requireCalendarDate(value, name string) error {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	if _, err := time.Parse(time.DateOnly, value); err != nil {
		return fmt.Errorf("calendar %s must use YYYY-MM-DD, got %q", name, value)
	}
	return nil
}
