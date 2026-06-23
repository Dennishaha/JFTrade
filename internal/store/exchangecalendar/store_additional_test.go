package exchangecalendar

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	marketcalendar "github.com/jftrade/jftrade-main/pkg/market/calendar"
)

func TestStoreRootAndNilSafety(t *testing.T) {
	root := filepath.Join(t.TempDir(), "calendars")
	store := New("  " + root + "  ")
	if got := store.Root(); got != root {
		t.Fatalf("Root = %q, want trimmed path", got)
	}

	var nilStore *Store
	if got := nilStore.Root(); got != "" {
		t.Fatalf("nil Root = %q, want empty", got)
	}
	if snapshots, errs := nilStore.LoadSnapshots(); snapshots != nil || errs != nil {
		t.Fatalf("nil LoadSnapshots = (%#v, %#v), want (nil, nil)", snapshots, errs)
	}
	if err := nilStore.DeleteSnapshot(marketcalendar.CalendarSnapshot{}); err != nil {
		t.Fatalf("nil DeleteSnapshot err = %v", err)
	}
	if err := nilStore.SaveSnapshot(marketcalendar.CalendarSnapshot{}); err == nil {
		t.Fatal("nil SaveSnapshot succeeded")
	}
}

func TestSaveSnapshotValidatesInputsAndResolvesYearFallbacks(t *testing.T) {
	store := New(t.TempDir())

	if err := store.SaveSnapshot(marketcalendar.CalendarSnapshot{}); err == nil {
		t.Fatal("SaveSnapshot without market/source/year succeeded")
	}
	if err := New(" ").SaveSnapshot(marketcalendar.CalendarSnapshot{
		MarketCode: "US",
		SourceID:   "nyse_official",
		From:       time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
	}); err == nil {
		t.Fatal("SaveSnapshot with empty root succeeded")
	}

	toOnly := marketcalendar.CalendarSnapshot{
		MarketCode: "US",
		SourceID:   "nyse_official",
		To:         time.Date(2027, 12, 31, 0, 0, 0, 0, time.UTC),
	}
	if err := store.SaveSnapshot(toOnly); err != nil {
		t.Fatalf("SaveSnapshot to-only: %v", err)
	}
	if _, err := os.Stat(filepath.Join(store.Root(), "US", "2027", "nyse_official.json")); err != nil {
		t.Fatalf("expected to-only snapshot file: %v", err)
	}

	scheduleOnly := marketcalendar.CalendarSnapshot{
		MarketCode: "HK",
		SourceID:   "hk_gov_1823_ical",
		Schedules: []marketcalendar.TradingDaySchedule{
			{MarketCode: "HK", Date: time.Date(2028, 5, 1, 0, 0, 0, 0, time.UTC)},
		},
	}
	if err := store.SaveSnapshot(scheduleOnly); err != nil {
		t.Fatalf("SaveSnapshot schedule-only: %v", err)
	}
	if _, err := os.Stat(filepath.Join(store.Root(), "HK", "2028", "hk_gov_1823_ical.json")); err != nil {
		t.Fatalf("expected schedule-only snapshot file: %v", err)
	}
}

func TestDeleteSnapshotIgnoresMissingFilesAndReturnsRealRemoveErrors(t *testing.T) {
	root := t.TempDir()
	store := New(root)
	snapshot := marketcalendar.CalendarSnapshot{
		MarketCode: "US",
		SourceID:   "nyse_official",
		From:       time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
	}

	if err := store.DeleteSnapshot(snapshot); err != nil {
		t.Fatalf("DeleteSnapshot missing file: %v", err)
	}
	if err := store.DeleteSnapshot(marketcalendar.CalendarSnapshot{}); err != nil {
		t.Fatalf("DeleteSnapshot missing year: %v", err)
	}

	path := filepath.Join(root, "US", "2026", "nyse_official.json")
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatalf("MkdirAll snapshot path dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(path, "nested"), []byte("x"), 0o644); err != nil {
		t.Fatalf("WriteFile nested: %v", err)
	}

	err := store.DeleteSnapshot(snapshot)
	if err == nil {
		t.Fatal("DeleteSnapshot directory path succeeded")
	}
	if !strings.Contains(err.Error(), "delete exchange calendar snapshot") {
		t.Fatalf("DeleteSnapshot error = %v", err)
	}
}
