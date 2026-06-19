package exchangecalendar

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	marketcalendar "github.com/jftrade/jftrade-main/pkg/market/calendar"
)

func TestStoreRoundTripsSnapshotsAndIsolatesCorruption(t *testing.T) {
	root := t.TempDir()
	store := New(root)

	snapshot := marketcalendar.CalendarSnapshot{
		MarketCode: "US",
		SourceID:   "nyse_official",
		From:       time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		To:         time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC),
		Schedules: []marketcalendar.TradingDaySchedule{
			{
				MarketCode: "US",
				Date:       time.Date(2026, 6, 19, 0, 0, 0, 0, time.UTC),
				Status:     marketcalendar.TradingDayClosed,
				Reason:     "juneteenth",
			},
		},
		FetchedAt:  time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC),
		ValidUntil: time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC),
	}
	if err := store.SaveSnapshot(snapshot); err != nil {
		t.Fatalf("SaveSnapshot: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "US", "2026", "broken.json"), []byte("{"), 0o644); err != nil {
		t.Fatalf("write broken file: %v", err)
	}

	snapshots, errs := store.LoadSnapshots()
	if len(snapshots) != 1 {
		t.Fatalf("snapshots len = %d, want 1", len(snapshots))
	}
	if len(errs) != 1 {
		t.Fatalf("errs len = %d, want 1", len(errs))
	}
	if snapshots[0].SourceID != "nyse_official" {
		t.Fatalf("snapshot source = %q", snapshots[0].SourceID)
	}

	if err := store.DeleteSnapshot(snapshot); err != nil {
		t.Fatalf("DeleteSnapshot: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "US", "2026", "nyse_official.json")); !os.IsNotExist(err) {
		t.Fatalf("snapshot file should be deleted, stat err = %v", err)
	}
}

func TestStoreUsesSnapshotLocalYearForPositiveOffsetMarkets(t *testing.T) {
	root := t.TempDir()
	store := New(root)
	loc := time.FixedZone("HKT", 8*3600)
	snapshot := marketcalendar.CalendarSnapshot{
		MarketCode: "HK",
		SourceID:   "hk_gov_1823_ical",
		From:       time.Date(2026, 1, 1, 0, 0, 0, 0, loc),
		To:         time.Date(2027, 12, 31, 23, 59, 59, 0, loc),
		Schedules: []marketcalendar.TradingDaySchedule{
			{
				MarketCode: "HK",
				Date:       time.Date(2026, 6, 19, 0, 0, 0, 0, loc),
				Status:     marketcalendar.TradingDayClosed,
				Reason:     "tuen_ng_festival",
			},
		},
	}

	if err := store.SaveSnapshot(snapshot); err != nil {
		t.Fatalf("SaveSnapshot: %v", err)
	}

	if _, err := os.Stat(filepath.Join(root, "HK", "2026", "hk_gov_1823_ical.json")); err != nil {
		t.Fatalf("expected HK snapshot under local calendar year, stat err = %v", err)
	}
}
