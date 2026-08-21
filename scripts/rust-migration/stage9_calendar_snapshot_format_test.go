package rustmigration

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"
	"time"

	marketcalendar "github.com/jftrade/jftrade-main/pkg/market/calendar"
)

const stage9CalendarSnapshotFormatVersion = "stage9.calendar-snapshot-format.v1"

type stage9CalendarSnapshotFormatFixture struct {
	Version       string         `json:"version"`
	Platforms     []string       `json:"platforms"`
	RelativePath  string         `json:"relativePath"`
	DirectoryMode string         `json:"directoryMode"`
	FileMode      string         `json:"fileMode"`
	FileContents  string         `json:"fileContents"`
	Snapshot      map[string]any `json:"snapshot"`
}

func TestStage9CalendarSnapshotFormatMatchesCurrentGoOwner(t *testing.T) {
	location := time.FixedZone("HKT", 8*60*60)
	snapshot := marketcalendar.CalendarSnapshot{
		MarketCode: "HK",
		SourceID:   "hk_gov_1823_ical",
		From:       time.Date(2026, time.January, 1, 0, 0, 0, 0, location),
		To:         time.Date(2026, time.December, 31, 23, 59, 59, 123456000, location),
		Schedules: []marketcalendar.TradingDaySchedule{
			{
				MarketCode: "HK",
				Date:       time.Date(2026, time.June, 19, 0, 0, 0, 0, location),
				Status:     marketcalendar.TradingDayEarlyClose,
				Sessions: []marketcalendar.SessionWindow{
					{Kind: marketcalendar.SessionRegular, StartMinute: 570, EndMinute: 720},
				},
				Reason:    "tuen_ng_festival",
				SourceID:  "hk_gov_1823_ical",
				Observed:  true,
				UpdatedAt: time.Date(2026, time.January, 2, 9, 30, 0, 0, location),
			},
		},
		FetchedAt:  time.Date(2026, time.January, 2, 9, 30, 0, 0, location),
		ValidUntil: time.Date(2027, time.January, 3, 9, 30, 0, 0, location),
		Checksum:   "sha256:calendar-fixture",
	}
	body, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		t.Fatalf("encode calendar snapshot: %v", err)
	}
	body = append(body, '\n')
	var snapshotValue map[string]any
	if err := json.Unmarshal(body, &snapshotValue); err != nil {
		t.Fatalf("decode calendar snapshot fixture value: %v", err)
	}
	want := stage9CalendarSnapshotFormatFixture{
		Version:       stage9CalendarSnapshotFormatVersion,
		Platforms:     []string{"unix", "windows"},
		RelativePath:  "HK/2026/hk_gov_1823_ical.json",
		DirectoryMode: "0755",
		FileMode:      "0644",
		FileContents:  string(body),
		Snapshot:      snapshotValue,
	}
	_, source, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve calendar snapshot fixture source")
	}
	path := filepath.Join(
		filepath.Dir(source),
		"../../tests/fixtures/rust-migration/stage9/calendar-snapshot-format.json",
	)
	if os.Getenv("JFTRADE_UPDATE_RUST_MIGRATION_FIXTURES") == "1" {
		contents, err := json.MarshalIndent(want, "", "  ")
		if err != nil {
			t.Fatalf("encode calendar snapshot format fixture: %v", err)
		}
		if err := os.WriteFile(path, append(contents, '\n'), 0o644); err != nil {
			t.Fatalf("write calendar snapshot format fixture: %v", err)
		}
	}
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read calendar snapshot format fixture: %v", err)
	}
	var got stage9CalendarSnapshotFormatFixture
	if err := json.Unmarshal(contents, &got); err != nil {
		t.Fatalf("decode calendar snapshot format fixture: %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatal("calendar snapshot format fixture drifted from the Go owner")
	}
}
