package calendar

import (
	"sort"
	"strings"
	"sync"
	"time"
)

type SessionKind string

const (
	SessionUnknown   SessionKind = "unknown"
	SessionClosed    SessionKind = "closed"
	SessionPre       SessionKind = "pre"
	SessionRegular   SessionKind = "regular"
	SessionAfter     SessionKind = "after"
	SessionOvernight SessionKind = "overnight"
)

type TradingDayStatus string

const (
	TradingDayUnknown    TradingDayStatus = "unknown"
	TradingDayOpen       TradingDayStatus = "open"
	TradingDayClosed     TradingDayStatus = "closed"
	TradingDayEarlyClose TradingDayStatus = "early_close"
	TradingDaySpecial    TradingDayStatus = "special"
)

type SessionWindow struct {
	Kind        SessionKind `json:"kind"`
	StartMinute int         `json:"startMinute"`
	EndMinute   int         `json:"endMinute"`
}

type MarketTemplate struct {
	MarketCode             string          `json:"marketCode"`
	Timezone               string          `json:"timezone"`
	RegularSessions        []SessionWindow `json:"regularSessions"`
	ExtendedSessions       []SessionWindow `json:"extendedSessions,omitempty"`
	SupportsExtendedHours  bool            `json:"supportsExtendedHours"`
	OvernightCarryStartMin int             `json:"overnightCarryStartMinute,omitempty"`
}

type TradingDaySchedule struct {
	MarketCode string           `json:"marketCode"`
	Date       time.Time        `json:"date"`
	Status     TradingDayStatus `json:"status"`
	Sessions   []SessionWindow  `json:"sessions,omitempty"`
	Reason     string           `json:"reason,omitempty"`
	SourceID   string           `json:"sourceId,omitempty"`
	Observed   bool             `json:"observed,omitempty"`
	UpdatedAt  time.Time        `json:"updatedAt,omitzero"`
}

type CalendarSnapshot struct {
	MarketCode string               `json:"marketCode"`
	SourceID   string               `json:"sourceId"`
	From       time.Time            `json:"from"`
	To         time.Time            `json:"to"`
	Schedules  []TradingDaySchedule `json:"schedules,omitempty"`
	FetchedAt  time.Time            `json:"fetchedAt"`
	ValidUntil time.Time            `json:"validUntil"`
	Checksum   string               `json:"checksum,omitempty"`
}

type Resolver interface {
	Template(marketCode string) (MarketTemplate, bool)
	Schedule(marketCode string, day time.Time) (TradingDaySchedule, bool)
}

var locationCache sync.Map

func NormalizeMarketCode(code string) string {
	return strings.ToUpper(strings.TrimSpace(code))
}

func LoadLocation(template MarketTemplate) *time.Location {
	name := strings.TrimSpace(template.Timezone)
	if name == "" {
		return time.UTC
	}
	if cached, ok := locationCache.Load(name); ok {
		return jftradeCheckedTypeAssertion[*time.Location](cached)
	}
	loc, err := time.LoadLocation(name)
	if err != nil {
		return time.UTC
	}
	actual, _ := locationCache.LoadOrStore(name, loc)
	loc = jftradeCheckedTypeAssertion[*time.Location](actual)
	return loc
}

func DayStart(template MarketTemplate, at time.Time) time.Time {
	loc := LoadLocation(template)
	local := at.In(loc)
	return time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, loc)
}

func NormalizeSessions(sessions []SessionWindow) []SessionWindow {
	normalized := make([]SessionWindow, 0, len(sessions))
	for _, session := range sessions {
		if session.EndMinute <= session.StartMinute {
			continue
		}
		normalized = append(normalized, session)
	}
	sort.SliceStable(normalized, func(i, j int) bool {
		if normalized[i].StartMinute == normalized[j].StartMinute {
			return normalized[i].EndMinute < normalized[j].EndMinute
		}
		return normalized[i].StartMinute < normalized[j].StartMinute
	})
	return normalized
}

func CopySessions(sessions []SessionWindow) []SessionWindow {
	return append([]SessionWindow(nil), sessions...)
}

func TradingDayHasSessions(schedule TradingDaySchedule) bool {
	switch schedule.Status {
	case TradingDayOpen, TradingDayEarlyClose, TradingDaySpecial:
		return len(schedule.Sessions) > 0
	default:
		return false
	}
}

func SessionWindowByKind(schedule TradingDaySchedule, kind SessionKind) (SessionWindow, bool) {
	for _, session := range schedule.Sessions {
		if session.Kind == kind {
			return session, true
		}
	}
	return SessionWindow{}, false
}

func SessionForMinute(schedule TradingDaySchedule, minute int) (SessionKind, bool) {
	for _, session := range schedule.Sessions {
		if minute >= session.StartMinute && minute < session.EndMinute {
			return session.Kind, true
		}
	}
	return SessionUnknown, false
}

func ScheduleForDate(template MarketTemplate, status TradingDayStatus, day time.Time, sourceID string, reason string, observed bool, sessions []SessionWindow) TradingDaySchedule {
	return TradingDaySchedule{
		MarketCode: NormalizeMarketCode(template.MarketCode),
		Date:       DayStart(template, day),
		Status:     status,
		Sessions:   NormalizeSessions(sessions),
		Reason:     strings.TrimSpace(reason),
		SourceID:   strings.TrimSpace(sourceID),
		Observed:   observed,
	}
}

func jftradeCheckedTypeAssertion[T any](value any) T {
	typed, ok := value.(T)
	if !ok {
		panic("unexpected dynamic type")
	}
	return typed
}
