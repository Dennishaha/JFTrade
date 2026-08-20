package rustmigration

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"
	"time"

	"github.com/jftrade/jftrade-main/internal/exchangecalendar"
)

const stage9CalendarSourcesFixtureVersion = "stage9.calendar-sources.v1"

type stage9CalendarSourcesFixture struct {
	Version         string           `json:"version"`
	ZeroStatus      map[string]any   `json:"zeroStatus"`
	RecoveredStatus map[string]any   `json:"recoveredStatus"`
	DefaultSources  []map[string]any `json:"defaultSources"`
}

func TestStage9CalendarSourceProjectionFixtureMatchesCurrentGoOwner(t *testing.T) {
	_, source, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve stage 9 calendar source fixture source")
	}
	path := filepath.Join(
		filepath.Dir(source),
		"../../tests/fixtures/rust-migration/stage9/calendar-sources.json",
	)
	zeroStatus, err := json.Marshal(exchangecalendar.SourceStatus{
		SourceID: "nyse_official",
		Enabled:  true,
	})
	if err != nil {
		t.Fatalf("encode zero calendar source status: %v", err)
	}
	var zeroStatusValue map[string]any
	if err := json.Unmarshal(zeroStatus, &zeroStatusValue); err != nil {
		t.Fatalf("decode zero calendar source status: %v", err)
	}
	recoveredStatus, err := json.Marshal(exchangecalendar.SourceStatus{
		SourceID:             "unknown_source",
		Enabled:              true,
		LastSuccessAt:        time.Date(2026, time.June, 23, 9, 30, 0, 0, time.UTC),
		LastError:            "recovered",
		ConsecutiveFailures:  2,
		LastProbeStatus:      "recovered",
		LastProbeError:       "",
		LastProbeMarket:      "US",
		LastProbeSchedules:   8,
		HealthState:          "healthy",
		HealthFingerprint:    "fingerprint-1",
		LastAlertStatus:      "recovered",
		LastAlertFingerprint: "alert-1",
	})
	if err != nil {
		t.Fatalf("encode recovered calendar source status: %v", err)
	}
	var recoveredStatusValue map[string]any
	if err := json.Unmarshal(recoveredStatus, &recoveredStatusValue); err != nil {
		t.Fatalf("decode recovered calendar source status: %v", err)
	}
	manager := exchangecalendar.NewManager(nil, nil)
	defaultSources, err := json.Marshal(manager.Sources())
	if err != nil {
		t.Fatalf("encode default calendar source projection: %v", err)
	}
	var normalizedSources []map[string]any
	if err := json.Unmarshal(defaultSources, &normalizedSources); err != nil {
		t.Fatalf("decode default calendar source projection: %v", err)
	}
	for _, source := range normalizedSources {
		for _, field := range []string{
			"lastSuccessAt",
			"lastFailureAt",
			"nextRefreshAt",
			"lastSnapshotFetchedAt",
			"lastProbeAt",
			"lastProbeSuccessAt",
			"lastProbeFailureAt",
			"lastAlertAt",
		} {
			if source[field] == "0001-01-01T00:00:00Z" {
				delete(source, field)
			}
		}
	}
	want := stage9CalendarSourcesFixture{
		Version:         stage9CalendarSourcesFixtureVersion,
		ZeroStatus:      zeroStatusValue,
		RecoveredStatus: recoveredStatusValue,
		DefaultSources:  normalizedSources,
	}
	if os.Getenv("JFTRADE_UPDATE_RUST_MIGRATION_FIXTURES") == "1" {
		contents, err := json.MarshalIndent(want, "", "  ")
		if err != nil {
			t.Fatalf("encode stage 9 calendar source fixture: %v", err)
		}
		contents = append(contents, '\n')
		if err := os.WriteFile(path, contents, 0o644); err != nil {
			t.Fatalf("write stage 9 calendar source fixture: %v", err)
		}
	}
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read stage 9 calendar source fixture: %v", err)
	}
	var got stage9CalendarSourcesFixture
	if err := json.Unmarshal(contents, &got); err != nil {
		t.Fatalf("decode stage 9 calendar source fixture: %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("stage 9 calendar source fixture drifted from the Go owner")
	}
}
