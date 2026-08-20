package rustmigration

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"

	store "github.com/jftrade/jftrade-main/internal/store/watchlist"
	domain "github.com/jftrade/jftrade-main/internal/watchlist"
)

const stage9WatchlistMembershipsVersion = "stage9.watchlist-memberships.v1"

type stage9WatchlistMembershipCase struct {
	Name      string              `json:"name"`
	Market    string              `json:"market"`
	Symbol    string              `json:"symbol"`
	Response  *domain.Memberships `json:"response,omitempty"`
	ErrorCode string              `json:"errorCode,omitempty"`
}

type stage9WatchlistMembershipFixture struct {
	Version string                          `json:"version"`
	Cases   []stage9WatchlistMembershipCase `json:"cases"`
}

// TestStage9WatchlistMembershipsFixtureMatchesCurrentGoOwner freezes the
// read-only membership projection while keeping SQLite and the Go service as
// the only production owner. The fixture includes existing, unknown, alias,
// and invalid instrument paths so Rust cannot claim a generic success shape.
func TestStage9WatchlistMembershipsFixtureMatchesCurrentGoOwner(t *testing.T) {
	_, source, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve stage 9 watchlist membership fixture source")
	}
	fixturePath := filepath.Join(
		filepath.Dir(source),
		"../../tests/fixtures/rust-migration/stage9/watchlist-memberships.json",
	)
	repository, err := store.Open(t.Context(), filepath.Join(t.TempDir(), "watchlists.db"))
	if err != nil {
		t.Fatalf("open watchlist store: %v", err)
	}
	defer repository.Close()
	seedStage9WatchlistMemberships(t, repository)
	service := domain.NewService(repository)

	cases := []struct {
		name      string
		market    string
		symbol    string
		errorCode string
	}{
		{name: "existing-us", market: "US", symbol: "AAPL"},
		{name: "existing-sh", market: "SH", symbol: "600519"},
		{name: "unknown-us", market: "US", symbol: "MSFT"},
		{name: "cns-h-alias", market: "CNSH", symbol: "600519"},
		{name: "invalid-market", market: "BAD", symbol: "AAPL", errorCode: "WATCHLIST_INVALID"},
	}
	want := stage9WatchlistMembershipFixture{
		Version: stage9WatchlistMembershipsVersion,
		Cases:   make([]stage9WatchlistMembershipCase, 0, len(cases)),
	}
	for _, testCase := range cases {
		membership, readErr := service.GetMemberships(
			t.Context(),
			testCase.market+"."+testCase.symbol,
		)
		entry := stage9WatchlistMembershipCase{
			Name: testCase.name, Market: testCase.market, Symbol: testCase.symbol,
			ErrorCode: testCase.errorCode,
		}
		if testCase.errorCode != "" {
			if readErr == nil || !errors.Is(readErr, domain.ErrValidation) {
				t.Fatalf("case %s error = %v, want validation", testCase.name, readErr)
			}
		} else {
			if readErr != nil {
				t.Fatalf("case %s: %v", testCase.name, readErr)
			}
			entry.Response = &membership
		}
		want.Cases = append(want.Cases, entry)
	}

	if os.Getenv("JFTRADE_UPDATE_RUST_MIGRATION_FIXTURES") == "1" {
		contents, err := json.MarshalIndent(want, "", "  ")
		if err != nil {
			t.Fatalf("encode watchlist membership fixture: %v", err)
		}
		if err := os.WriteFile(fixturePath, append(contents, '\n'), 0o644); err != nil {
			t.Fatalf("write watchlist membership fixture: %v", err)
		}
	}
	contents, err := os.ReadFile(fixturePath)
	if err != nil {
		t.Fatalf("read watchlist membership fixture: %v", err)
	}
	var got stage9WatchlistMembershipFixture
	if err := json.Unmarshal(contents, &got); err != nil {
		t.Fatalf("decode watchlist membership fixture: %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("stage 9 watchlist membership fixture drifted from the Go owner")
	}
}

func seedStage9WatchlistMemberships(t *testing.T, repository *store.Store) {
	t.Helper()
	const timestamp = "2026-08-20T00:00:00Z"
	statements := []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO watchlist_groups (group_id, name, name_key, is_default, protected, revision, created_at, updated_at) VALUES ('tech', '科技', '科技', 0, 0, 1, ?, ?)`, []any{timestamp, timestamp}},
		{`INSERT INTO watchlist_instruments (instrument_id, market, symbol, name, instrument_type, membership_revision, created_at, updated_at) VALUES ('US.AAPL', 'US', 'AAPL', 'Apple', 'stock', 2, ?, ?)`, []any{timestamp, timestamp}},
		{`INSERT INTO watchlist_instruments (instrument_id, market, symbol, name, instrument_type, membership_revision, created_at, updated_at) VALUES ('SH.600519', 'CN', 'SH.600519', '贵州茅台', 'stock', 1, ?, ?)`, []any{timestamp, timestamp}},
		{`INSERT INTO watchlist_memberships (group_id, instrument_id, created_at) VALUES ('default', 'US.AAPL', ?)`, []any{timestamp}},
		{`INSERT INTO watchlist_memberships (group_id, instrument_id, created_at) VALUES ('tech', 'US.AAPL', ?)`, []any{timestamp}},
		{`INSERT INTO watchlist_memberships (group_id, instrument_id, created_at) VALUES ('tech', 'SH.600519', ?)`, []any{timestamp}},
	}
	for _, statement := range statements {
		if _, err := repository.DB().ExecContext(t.Context(), statement.sql, statement.args...); err != nil {
			t.Fatalf("seed watchlist statement %q: %v", statement.sql, err)
		}
	}
}
