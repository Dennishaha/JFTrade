package servercore

import (
	"fmt"
	"strings"
	"time"

	"github.com/jftrade/jftrade-main/internal/exchangecalendar"
)

func (s *Server) recordExchangeCalendarAlert(alert exchangecalendar.SourceAlert) {
	if s == nil || s.store == nil {
		return
	}
	if !persistenceOnlySettingsStore(s.store).ExchangeCalendarSettings().ErrorNotificationsEnabled {
		return
	}
	note := liveNotificationFromExchangeCalendarAlert(alert)
	if note == nil {
		return
	}
	s.recordLiveNotification(*note)
}

func liveNotificationFromExchangeCalendarAlert(alert exchangecalendar.SourceAlert) *liveNotification {
	if strings.TrimSpace(alert.Title) == "" {
		return nil
	}
	note := &liveNotification{
		At:       time.Now().UTC().Format(time.RFC3339Nano),
		Level:    normalizeExchangeCalendarNotificationLevel(alert.Level),
		Title:    strings.TrimSpace(alert.Title),
		Message:  strings.TrimSpace(alert.Message),
		Source:   "exchange-calendars",
		Category: "market.calendar.source",
	}
	if strings.TrimSpace(note.Message) == "" {
		note.Message = fmt.Sprintf("%s 市场日历源 %s 状态发生变化。", strings.TrimSpace(alert.Market), strings.TrimSpace(alert.SourceID))
	}
	return note
}

func normalizeExchangeCalendarNotificationLevel(level string) string {
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "success":
		return "success"
	case "error":
		return "error"
	case "warn":
		return "warn"
	default:
		return "info"
	}
}
