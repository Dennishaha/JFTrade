package exchangecalendar

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	marketcalendar "github.com/jftrade/jftrade-main/pkg/market/calendar"
)

func TestCalendarStoreRejectsInvalidSnapshotPersistence(t *testing.T) {
	var nilStore *Store
	if err := nilStore.SaveSnapshot(marketcalendar.CalendarSnapshot{}); err == nil || !strings.Contains(err.Error(), "store is nil") {
		t.Fatalf("nil store SaveSnapshot err = %v", err)
	}
	if err := New(" ").SaveSnapshot(marketcalendar.CalendarSnapshot{}); err == nil || !strings.Contains(err.Error(), "root is empty") {
		t.Fatalf("empty root SaveSnapshot err = %v", err)
	}

	store := New(t.TempDir())
	if err := store.SaveSnapshot(marketcalendar.CalendarSnapshot{MarketCode: "US"}); err == nil || !strings.Contains(err.Error(), "marketCode and sourceId") {
		t.Fatalf("missing source SaveSnapshot err = %v", err)
	}
	if err := store.SaveSnapshot(marketcalendar.CalendarSnapshot{MarketCode: "US", SourceID: "source"}); err == nil || !strings.Contains(err.Error(), "snapshot year") {
		t.Fatalf("missing year SaveSnapshot err = %v", err)
	}
}

func TestCalendarStoreReportsUnavailableSnapshotDirectory(t *testing.T) {
	root := filepath.Join(t.TempDir(), "occupied")
	if err := os.WriteFile(root, []byte("file"), 0o600); err != nil {
		t.Fatalf("prepare occupied root: %v", err)
	}
	snapshot := marketcalendar.CalendarSnapshot{
		MarketCode: "US", SourceID: "source",
		Schedules: []marketcalendar.TradingDaySchedule{{MarketCode: "US", Date: mustCalendarBoundaryDate(t, "2026-07-02")}},
	}
	if err := New(root).SaveSnapshot(snapshot); err == nil || !strings.Contains(err.Error(), "create exchange calendar snapshot directory") {
		t.Fatalf("occupied root SaveSnapshot err = %v", err)
	}
}

func TestCalendarStoreEmptyLoadAndDeleteAreIdempotent(t *testing.T) {
	var nilStore *Store
	if snapshots, errs := nilStore.LoadSnapshots(); snapshots != nil || errs != nil {
		t.Fatalf("nil store load = %#v/%#v", snapshots, errs)
	}
	empty := New(" ")
	if snapshots, errs := empty.LoadSnapshots(); snapshots != nil || errs != nil {
		t.Fatalf("empty root load = %#v/%#v", snapshots, errs)
	}
	if err := nilStore.DeleteSnapshot(marketcalendar.CalendarSnapshot{}); err != nil {
		t.Fatalf("nil store delete: %v", err)
	}
	if err := New(t.TempDir()).DeleteSnapshot(marketcalendar.CalendarSnapshot{MarketCode: "US", SourceID: "source"}); err != nil {
		t.Fatalf("yearless snapshot delete: %v", err)
	}
}

func mustCalendarBoundaryDate(t *testing.T, value string) time.Time {
	t.Helper()
	parsed, err := time.Parse("2006-01-02", value)
	if err != nil {
		t.Fatalf("parse date: %v", err)
	}
	return parsed
}
