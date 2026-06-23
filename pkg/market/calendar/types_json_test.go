package calendar

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestTradingDayScheduleJSONOmitsZeroUpdatedAt(t *testing.T) {
	raw, err := json.Marshal(TradingDaySchedule{MarketCode: "US", Status: TradingDayClosed})
	if err != nil {
		t.Fatalf("Marshal zero schedule: %v", err)
	}
	if strings.Contains(string(raw), `"updatedAt"`) {
		t.Fatalf("zero schedule JSON = %s, unexpectedly contains updatedAt", raw)
	}

	raw, err = json.Marshal(TradingDaySchedule{
		MarketCode: "US",
		Status:     TradingDayClosed,
		UpdatedAt:  time.Date(2026, time.June, 23, 9, 30, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("Marshal nonzero schedule: %v", err)
	}
	if !strings.Contains(string(raw), `"updatedAt":"2026-06-23T09:30:00Z"`) {
		t.Fatalf("nonzero schedule JSON = %s, missing updatedAt", raw)
	}
}
