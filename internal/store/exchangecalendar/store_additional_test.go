package exchangecalendar

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	marketcalendar "github.com/jftrade/jftrade-main/pkg/market/calendar"
)

type failingSnapshotTemporaryFile struct {
	path     string
	chmodErr error
	writeErr error
	syncErr  error
	closeErr error
}

func (f *failingSnapshotTemporaryFile) Name() string            { return f.path }
func (f *failingSnapshotTemporaryFile) Chmod(fs.FileMode) error { return f.chmodErr }
func (f *failingSnapshotTemporaryFile) Write(body []byte) (int, error) {
	return len(body), f.writeErr
}
func (f *failingSnapshotTemporaryFile) Sync() error  { return f.syncErr }
func (f *failingSnapshotTemporaryFile) Close() error { return f.closeErr }

func TestSaveSnapshotUsesAtomicReplacement(t *testing.T) {
	root := t.TempDir()
	store := New(root)
	original := marketcalendar.CalendarSnapshot{
		MarketCode: "US", SourceID: "nyse_official",
		From: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		Schedules: []marketcalendar.TradingDaySchedule{{
			MarketCode: "US", Date: time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC),
		}},
	}
	if err := store.SaveSnapshot(original); err != nil {
		t.Fatalf("SaveSnapshot(original): %v", err)
	}
	path := filepath.Join(root, "US", "2026", "nyse_official.json")
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(original): %v", err)
	}
	replaceErr := errors.New("atomic replacement failed")
	store.replaceFile = func(string, string) error { return replaceErr }
	replacement := original
	replacement.Schedules = append([]marketcalendar.TradingDaySchedule(nil), original.Schedules...)
	replacement.Schedules[0].Date = time.Date(2026, 1, 3, 0, 0, 0, 0, time.UTC)
	if err := store.SaveSnapshot(replacement); !errors.Is(err, replaceErr) {
		t.Fatalf("SaveSnapshot(replacement) = %v, want %v", err, replaceErr)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(after failed replacement): %v", err)
	}
	if string(after) != string(before) {
		t.Fatalf("snapshot changed after failed atomic replacement:\nbefore=%s\nafter=%s", before, after)
	}
	temporary, err := filepath.Glob(filepath.Join(root, "US", "2026", ".calendar-snapshot-*.tmp"))
	if err != nil {
		t.Fatalf("Glob temporary snapshots: %v", err)
	}
	if len(temporary) != 0 {
		t.Fatalf("temporary snapshot files were not cleaned up: %#v", temporary)
	}
}

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

func TestWriteSnapshotPropagatesTemporaryFileDurabilityFailures(t *testing.T) {
	sentinel := errors.New("temporary snapshot failure")
	tests := []struct {
		name      string
		configure func(*failingSnapshotTemporaryFile)
	}{
		{name: "chmod", configure: func(file *failingSnapshotTemporaryFile) { file.chmodErr = sentinel }},
		{name: "write", configure: func(file *failingSnapshotTemporaryFile) { file.writeErr = sentinel }},
		{name: "sync", configure: func(file *failingSnapshotTemporaryFile) { file.syncErr = sentinel }},
		{name: "close", configure: func(file *failingSnapshotTemporaryFile) { file.closeErr = sentinel }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			store := New(t.TempDir())
			file := &failingSnapshotTemporaryFile{path: filepath.Join(store.Root(), "temporary")}
			test.configure(file)
			store.createTemp = func(string, string) (snapshotTemporaryFile, error) { return file, nil }
			if err := store.writeSnapshot(filepath.Join(store.Root(), "snapshot.json"), []byte("{}\n")); !errors.Is(err, sentinel) {
				t.Fatalf("writeSnapshot() error = %v, want %v", err, sentinel)
			}
		})
	}

	store := New(t.TempDir())
	store.createTemp = func(string, string) (snapshotTemporaryFile, error) { return nil, sentinel }
	if err := store.writeSnapshot(filepath.Join(store.Root(), "snapshot.json"), []byte("{}\n")); !errors.Is(err, sentinel) {
		t.Fatalf("writeSnapshot(create) error = %v, want %v", err, sentinel)
	}
}

func TestWriteSnapshotDefaultHooksAndDirectorySyncErrors(t *testing.T) {
	store := New(t.TempDir())
	store.createTemp = nil
	store.replaceFile = nil
	path := filepath.Join(store.Root(), "snapshot.json")
	if err := store.writeSnapshot(path, []byte("{}\n")); err != nil {
		t.Fatalf("writeSnapshot(default hooks): %v", err)
	}

	missing := filepath.Join(t.TempDir(), "missing")
	if err := syncSnapshotDirectory(missing); err == nil {
		t.Fatal("syncSnapshotDirectory(missing) error = nil")
	}
}

func TestSaveSnapshotReturnsDirectoryCreationError(t *testing.T) {
	rootFile := filepath.Join(t.TempDir(), "calendar-root")
	if err := os.WriteFile(rootFile, []byte("not a directory"), 0o644); err != nil {
		t.Fatalf("WriteFile(root): %v", err)
	}
	store := New(rootFile)
	err := store.SaveSnapshot(marketcalendar.CalendarSnapshot{
		MarketCode: "US",
		SourceID:   "nyse_official",
		From:       time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
	})
	if err == nil || !strings.Contains(err.Error(), "create exchange calendar snapshot directory") {
		t.Fatalf("SaveSnapshot() error = %v, want directory creation failure", err)
	}
}
