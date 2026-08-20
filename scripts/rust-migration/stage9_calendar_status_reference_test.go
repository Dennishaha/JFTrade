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
	"github.com/jftrade/jftrade-main/internal/system"
	marketcalendar "github.com/jftrade/jftrade-main/pkg/market/calendar"
)

const stage9CalendarStatusFixtureVersion = "stage9.calendar-status.v1"

type stage9CalendarStatusFixture struct {
	Version string         `json:"version"`
	Status  map[string]any `json:"status"`
}

type stage9CalendarStatusSource struct{}

func (stage9CalendarStatusSource) ID() string        { return "fixture_source" }
func (stage9CalendarStatusSource) Kind() string      { return "fixture" }
func (stage9CalendarStatusSource) Markets() []string { return []string{"US"} }
func (stage9CalendarStatusSource) Authority() string { return "fixture" }

func (stage9CalendarStatusSource) Fetch(context.Context, string, time.Time, time.Time) (marketcalendar.CalendarSnapshot, error) {
	fetchedAt := time.Date(2026, 1, 2, 3, 0, 0, 0, time.UTC)
	return marketcalendar.CalendarSnapshot{
		MarketCode: "US",
		SourceID:   "fixture_source",
		From:       time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		To:         time.Date(2027, 12, 31, 23, 59, 59, 0, time.UTC),
		FetchedAt:  fetchedAt,
		ValidUntil: fetchedAt.Add(7 * 24 * time.Hour),
		Checksum:   "fixture-checksum-1",
		Schedules: []marketcalendar.TradingDaySchedule{
			{
				MarketCode: "US",
				Date:       time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC),
				Status:     marketcalendar.TradingDayOpen,
				SourceID:   "fixture_source",
			},
			{
				MarketCode: "US",
				Date:       time.Date(2026, 1, 19, 0, 0, 0, 0, time.UTC),
				Status:     marketcalendar.TradingDayClosed,
				Reason:     "fixture_holiday",
				SourceID:   "fixture_source",
				Observed:   true,
			},
			{
				MarketCode: "US",
				Date:       time.Date(2026, 11, 27, 0, 0, 0, 0, time.UTC),
				Status:     marketcalendar.TradingDayEarlyClose,
				Reason:     "fixture_early_close",
				SourceID:   "fixture_source",
				Sessions: []marketcalendar.SessionWindow{
					{Kind: marketcalendar.SessionRegular, StartMinute: 570, EndMinute: 780},
				},
			},
		},
	}, nil
}

func TestStage9CalendarStatusFixtureMatchesCurrentGoOwner(t *testing.T) {
	_, source, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve stage 9 calendar status fixture source")
	}
	path := filepath.Join(
		filepath.Dir(source),
		"../../tests/fixtures/rust-migration/stage9/calendar-status.json",
	)
	now := time.Date(2026, 6, 19, 12, 0, 0, 0, time.UTC)
	settings := jfsettings.ExchangeCalendarSettings{
		AutoRefreshEnabled:   false,
		RefreshIntervalHours: 24,
		WarmupMarkets:        []string{"US"},
		SourcePolicies: []jfsettings.ExchangeCalendarSourcePolicy{
			{
				Market:             "US",
				PreferredSourceIDs: []string{"fixture_source"},
				EnabledSourceIDs:   []string{"fixture_source"},
				FallbackToBuiltin:  true,
				StaleAfterHours:    72,
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
	if result := manager.RefreshAll(context.Background()); result["updated"] != 1 || result["failures"] != 0 {
		t.Fatalf("RefreshAll result = %#v", result)
	}

	// The HTTP system route goes through servercore.systemCalendarStatus,
	// which converts the manager's map into internal/system's typed wire before
	// Gin serializes it. Keep this conversion in the fixture generator so Rust
	// is compared with the actual public response shape, including zero fields.
	encoded, err := json.Marshal(manager.Status())
	if err != nil {
		t.Fatalf("encode manager calendar status: %v", err)
	}
	var typed system.CalendarStatus
	if err := json.Unmarshal(encoded, &typed); err != nil {
		t.Fatalf("decode typed calendar status: %v", err)
	}
	typedEncoded, err := json.Marshal(typed)
	if err != nil {
		t.Fatalf("encode typed calendar status: %v", err)
	}
	var statusValue map[string]any
	if err := json.Unmarshal(typedEncoded, &statusValue); err != nil {
		t.Fatalf("decode typed calendar status: %v", err)
	}
	want := stage9CalendarStatusFixture{
		Version: stage9CalendarStatusFixtureVersion,
		Status:  statusValue,
	}
	if os.Getenv("JFTRADE_UPDATE_RUST_MIGRATION_FIXTURES") == "1" {
		contents, err := json.MarshalIndent(want, "", "  ")
		if err != nil {
			t.Fatalf("encode stage 9 calendar status fixture: %v", err)
		}
		contents = append(contents, '\n')
		if err := os.WriteFile(path, contents, 0o644); err != nil {
			t.Fatalf("write stage 9 calendar status fixture: %v", err)
		}
	}
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read stage 9 calendar status fixture: %v", err)
	}
	var got stage9CalendarStatusFixture
	if err := json.Unmarshal(contents, &got); err != nil {
		t.Fatalf("decode stage 9 calendar status fixture: %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("stage 9 calendar status fixture drifted from the Go owner")
	}
}
