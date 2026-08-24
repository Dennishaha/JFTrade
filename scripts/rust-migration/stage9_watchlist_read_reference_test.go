package rustmigration

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	apiwatchlist "github.com/jftrade/jftrade-main/internal/api/watchlist"
	store "github.com/jftrade/jftrade-main/internal/store/watchlist"
	domain "github.com/jftrade/jftrade-main/internal/watchlist"
)

const stage9WatchlistReadFixtureVersion = "stage9.watchlist-read.v1"

type stage9WatchlistReadCase struct {
	Name           string          `json:"name"`
	Method         string          `json:"method"`
	RequestPath    string          `json:"requestPath"`
	ExpectedStatus int             `json:"expectedStatus"`
	Data           json.RawMessage `json:"data,omitempty"`
	ErrorCode      string          `json:"errorCode,omitempty"`
	ErrorMessage   string          `json:"errorMessage,omitempty"`
}

type stage9WatchlistReadFixture struct {
	Version string                    `json:"version"`
	Cases   []stage9WatchlistReadCase `json:"cases"`
}

// TestStage9WatchlistReadFixtureMatchesCurrentGoOwner freezes all read-only
// watchlist projections together. It uses the existing Go store only as a
// fixture producer; Rust never opens this database in product mode.
func TestStage9WatchlistReadFixtureMatchesCurrentGoOwner(t *testing.T) {
	_, source, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve stage 9 watchlist fixture source")
	}
	fixturePath := filepath.Join(filepath.Dir(source), "../../tests/fixtures/rust-migration/stage9/watchlist-read.json")
	repository, err := store.Open(t.Context(), filepath.Join(t.TempDir(), "watchlists.db"))
	if err != nil {
		t.Fatalf("open watchlist store: %v", err)
	}
	defer func() { _ = repository.Close() }()
	seedStage9WatchlistRead(t, repository)
	clock := func() time.Time { return time.Date(2026, 8, 21, 4, 0, 0, 0, time.UTC) }
	service := domain.NewService(repository, domain.WithClock(clock), domain.WithSourceReader("futu:default", stage9WatchlistReader{}))
	router := gin.New()
	apiwatchlist.RegisterRoutes(router.Group("/api/v1"), service)

	cases := []struct {
		name string
		path string
	}{
		{name: "groups", path: "/api/v1/watchlist/groups"},
		{name: "items", path: "/api/v1/watchlist/items"},
		{name: "items-empty", path: "/api/v1/watchlist/items?query=missing"},
		{name: "items-invalid-limit", path: "/api/v1/watchlist/items?limit=0"},
		{name: "sources", path: "/api/v1/watchlist/sources"},
		{name: "source-groups", path: "/api/v1/watchlist/sources/futu:default/groups"},
		{name: "source-groups-missing", path: "/api/v1/watchlist/sources/missing/groups"},
		{name: "bindings", path: "/api/v1/watchlist/bindings"},
		{name: "bindings-empty", path: "/api/v1/watchlist/bindings?sourceId=missing"},
		{name: "import-runs", path: "/api/v1/watchlist/import-runs"},
		{name: "import-runs-invalid-limit", path: "/api/v1/watchlist/import-runs?limit=0"},
	}
	want := stage9WatchlistReadFixture{Version: stage9WatchlistReadFixtureVersion, Cases: make([]stage9WatchlistReadCase, 0, len(cases))}
	for _, testCase := range cases {
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, stage9WatchlistReadRequest(testCase.path))
		entry := stage9WatchlistReadCase{
			Name: testCase.name, Method: http.MethodGet, RequestPath: testCase.path,
			ExpectedStatus: recorder.Code,
		}
		var envelope struct {
			Data  json.RawMessage `json:"data"`
			Error *struct {
				Code    string `json:"code"`
				Message string `json:"message"`
			} `json:"error"`
		}
		if err := json.Unmarshal(recorder.Body.Bytes(), &envelope); err != nil {
			t.Fatalf("decode %s response: %v (%s)", testCase.name, err, recorder.Body.String())
		}
		if envelope.Error != nil {
			entry.ErrorCode = envelope.Error.Code
			entry.ErrorMessage = envelope.Error.Message
		} else {
			entry.Data = envelope.Data
		}
		want.Cases = append(want.Cases, entry)
	}
	if os.Getenv("JFTRADE_UPDATE_RUST_MIGRATION_FIXTURES") == "1" {
		contents, err := json.MarshalIndent(want, "", "  ")
		if err != nil {
			t.Fatalf("encode watchlist fixture: %v", err)
		}
		if err := os.WriteFile(fixturePath, append(contents, '\n'), 0o644); err != nil {
			t.Fatalf("write watchlist fixture: %v", err)
		}
	}
	contents, err := os.ReadFile(fixturePath)
	if err != nil {
		t.Fatalf("read watchlist fixture: %v", err)
	}
	var got stage9WatchlistReadFixture
	if err := json.Unmarshal(contents, &got); err != nil {
		t.Fatalf("decode watchlist fixture: %v", err)
	}
	for index := range got.Cases {
		got.Cases[index].Data = compactPluginJSON(got.Cases[index].Data)
		want.Cases[index].Data = compactPluginJSON(want.Cases[index].Data)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("stage 9 watchlist read fixture drifted from the Go owner")
	}
}

func stage9WatchlistReadRequest(requestPath string) *http.Request {
	path, rawQuery, _ := strings.Cut(requestPath, "?")
	request := httptest.NewRequest(http.MethodGet, path, nil)
	request.URL.RawQuery = rawQuery
	request.RequestURI = requestPath
	return request
}

type stage9WatchlistReader struct{}

func (stage9WatchlistReader) Source(context.Context) (domain.Source, error) {
	return domain.Source{ID: "futu:default", Broker: "futu", DisplayName: "Futu", Status: "ready"}, nil
}
func (stage9WatchlistReader) ListGroups(context.Context) ([]domain.RemoteGroup, error) {
	return []domain.RemoteGroup{{RemoteGroupID: "remote-tech", Name: "Tech", Type: "stock", MemberCount: 1}}, nil
}
func (stage9WatchlistReader) ListGroupMembers(context.Context, string) ([]domain.RemoteMember, error) {
	return nil, nil
}

func seedStage9WatchlistRead(t *testing.T, repository *store.Store) {
	t.Helper()
	const timestamp = "2026-08-21T04:00:00Z"
	statements := []struct {
		sql  string
		args []any
	}{
		{`UPDATE watchlist_groups SET created_at = ?, updated_at = ? WHERE group_id = 'default'`, []any{timestamp, timestamp}},
		{`INSERT INTO watchlist_groups (group_id, name, name_key, is_default, protected, revision, created_at, updated_at) VALUES ('tech', 'Tech', 'tech', 0, 0, 2, ?, ?)`, []any{timestamp, timestamp}},
		{`INSERT INTO watchlist_instruments (instrument_id, market, symbol, name, instrument_type, membership_revision, created_at, updated_at) VALUES ('US.AAPL', 'US', 'AAPL', 'Apple', 'stock', 2, ?, ?)`, []any{timestamp, timestamp}},
		{`INSERT INTO watchlist_memberships (group_id, instrument_id, created_at) VALUES ('default', 'US.AAPL', ?)`, []any{timestamp}},
		{`INSERT INTO watchlist_memberships (group_id, instrument_id, created_at) VALUES ('tech', 'US.AAPL', ?)`, []any{timestamp}},
		{`INSERT INTO watchlist_sources (source_id, broker, display_name, status, last_error, updated_at) VALUES ('futu:default', 'futu', 'Futu', 'ready', '', ?)`, []any{timestamp}},
		{`INSERT INTO watchlist_bindings (binding_id, source_id, remote_group_id, remote_name, local_group_id, created_at, updated_at) VALUES ('binding-1', 'futu:default', 'remote-tech', 'Tech', 'tech', ?, ?)`, []any{timestamp, timestamp}},
	}
	for _, statement := range statements {
		if _, err := repository.DB().ExecContext(t.Context(), statement.sql, statement.args...); err != nil {
			t.Fatalf("seed watchlist statement %q: %v", statement.sql, err)
		}
	}
}
