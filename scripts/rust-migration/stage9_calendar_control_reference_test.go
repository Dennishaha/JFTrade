package rustmigration

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"
	"time"

	"github.com/jftrade/jftrade-main/internal/exchangecalendar"
	jfsettings "github.com/jftrade/jftrade-main/internal/jftsettings"
)

const stage9CalendarControlFixtureVersion = "stage9.calendar-control.v1"

type stage9CalendarControlFixture struct {
	Version        string         `json:"version"`
	RefreshAll     map[string]any `json:"refreshAll"`
	RefreshUnknown map[string]any `json:"refreshUnknown"`
	ProbeUS        map[string]any `json:"probeUS"`
	ProbeUnknown   map[string]any `json:"probeUnknown"`
}

func TestStage9CalendarControlFixtureMatchesCurrentGoOwner(t *testing.T) {
	_, source, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve stage 9 calendar control fixture source")
	}
	path := filepath.Join(
		filepath.Dir(source),
		"../../tests/fixtures/rust-migration/stage9/calendar-control.json",
	)
	now := time.Date(2026, 6, 19, 12, 0, 0, 0, time.UTC)
	settings := jfsettings.ExchangeCalendarSettings{
		RefreshIntervalHours: 24,
		WarmupMarkets:        []string{"US"},
		SourcePolicies: []jfsettings.ExchangeCalendarSourcePolicy{
			{
				Market:             "US",
				PreferredSourceIDs: []string{"fixture_source"},
				EnabledSourceIDs:   []string{"fixture_source"},
				FallbackToBuiltin:  true,
			},
		},
	}
	registry := exchangecalendar.NewSourceRegistry()
	registry.Register(stage9CalendarStatusSource{})
	manager := exchangecalendar.NewManager(
		nil,
		func() jfsettings.ExchangeCalendarSettings { return settings },
		exchangecalendar.WithRegistry(registry),
		exchangecalendar.WithClock(func() time.Time { return now }),
	)
	want := stage9CalendarControlFixture{
		Version:        stage9CalendarControlFixtureVersion,
		RefreshAll:     manager.RefreshAll(context.Background()),
		RefreshUnknown: manager.RefreshMarket(context.Background(), "MARS"),
		ProbeUS:        manager.ProbeMarket(context.Background(), "US"),
		ProbeUnknown:   manager.ProbeMarket(context.Background(), "MARS"),
	}
	normalized, err := json.Marshal(want)
	if err != nil {
		t.Fatalf("normalize stage 9 calendar control fixture: %v", err)
	}
	if err := json.Unmarshal(normalized, &want); err != nil {
		t.Fatalf("decode normalized stage 9 calendar control fixture: %v", err)
	}
	if os.Getenv("JFTRADE_UPDATE_RUST_MIGRATION_FIXTURES") == "1" {
		contents, err := json.MarshalIndent(want, "", "  ")
		if err != nil {
			t.Fatalf("encode stage 9 calendar control fixture: %v", err)
		}
		contents = append(contents, '\n')
		if err := os.WriteFile(path, contents, 0o644); err != nil {
			t.Fatalf("write stage 9 calendar control fixture: %v", err)
		}
	}
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read stage 9 calendar control fixture: %v", err)
	}
	var got stage9CalendarControlFixture
	if err := json.Unmarshal(contents, &got); err != nil {
		t.Fatalf("decode stage 9 calendar control fixture: %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("calendar control fixture drift\n got: %#v\nwant: %#v", got, want)
	}
}
