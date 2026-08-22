package rustmigration

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	watchlistapi "github.com/jftrade/jftrade-main/internal/api/watchlist"
	domain "github.com/jftrade/jftrade-main/internal/watchlist"
)

const (
	stage9WatchlistWriteFixtureVersion = "stage9.watchlist-write.v1"
	stage9WatchlistWriteTimestamp      = "2026-08-23T00:00:00Z"
)

var stage9WatchlistWriteNow = time.Date(2026, 8, 23, 0, 0, 0, 0, time.UTC)

type stage9WatchlistWriteFixture struct {
	Version   string                            `json:"version"`
	Timestamp string                            `json:"timestamp"`
	Cases     []stage9WatchlistWriteFixtureCase `json:"cases"`
}

type stage9WatchlistWriteFixtureCase struct {
	Name                string                                `json:"name"`
	Requests            []stage9WatchlistWriteFixtureRequest  `json:"requests"`
	Expected            []stage9WatchlistWriteFixtureExpected `json:"expected"`
	Calls               []map[string]any                      `json:"calls,omitempty"`
	ExpectedObservation map[string]any                        `json:"expectedObservation"`
	PortMode            string                                `json:"portMode,omitempty"`
	Concurrent          bool                                  `json:"concurrent,omitempty"`
}

type stage9WatchlistWriteFixtureRequest struct {
	Method  string  `json:"method"`
	Path    string  `json:"path"`
	Body    *string `json:"body,omitempty"`
	Context string  `json:"context,omitempty"`
}

type stage9WatchlistWriteFixtureExpected struct {
	Status   int               `json:"status"`
	Headers  map[string]string `json:"headers"`
	Envelope map[string]any    `json:"envelope"`
	PortCall bool              `json:"portCall"`
}

type stage9WatchlistWriteCaseSpec struct {
	Name       string
	Requests   []stage9WatchlistWriteFixtureRequest
	Mode       string
	Concurrent bool
	Setup      func(*stage9WatchlistWriteRepository, *stage9WatchlistWriteReader, *stage9WatchlistWriteQuotes)
}

type stage9WatchlistWriteRepository struct {
	mu              sync.Mutex
	mode            string
	now             time.Time
	nextGroup       int
	nextPreview     int
	nextRun         int
	groups          map[string]domain.Group
	instruments     map[string]domain.Instrument
	memberships     map[string]map[string]struct{}
	bindings        map[string]domain.Binding
	previews        map[string]domain.ImportPreview
	committed       map[string]bool
	runs            []domain.ImportRun
	remoteGroups    map[string][]domain.RemoteGroup
	sources         map[string]domain.Source
	mutationCalls   int
	operationCounts map[string]int
}

type stage9WatchlistWriteReader struct {
	mu           sync.Mutex
	groups       []domain.RemoteGroup
	members      []domain.RemoteMember
	freshMembers []domain.RemoteMember
	groupsErr    error
	membersErr   error
	freshErr     error
	freshCalls   int
}

type stage9WatchlistWriteQuotes struct {
	mu         sync.Mutex
	quotes     []domain.Quote
	itemErrors []domain.QuoteError
	err        error
	requested  [][]string
}

// TestStage9WatchlistWriteFixtureMatchesCurrentGoOwner freezes all eight
// watchlist mutation routes through the real Go service and Gin transport.
// The repository, remote reader, and quote source are in-memory doubles; no
// production SQLite file, provider, OpenD connection, or quote worker is used.
func TestStage9WatchlistWriteFixtureMatchesCurrentGoOwner(t *testing.T) {
	_, source, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve stage 9 watchlist-write fixture source")
	}
	fixturePath := filepath.Join(
		filepath.Dir(source), "../../tests/fixtures/rust-migration/stage9/watchlist-write.json",
	)
	want := stage9WatchlistWriteFixture{
		Version:   stage9WatchlistWriteFixtureVersion,
		Timestamp: stage9WatchlistWriteTimestamp,
		Cases:     make([]stage9WatchlistWriteFixtureCase, 0),
	}
	for _, spec := range stage9WatchlistWriteCaseSpecs() {
		want.Cases = append(want.Cases, runStage9WatchlistWriteCase(t, spec))
	}
	if os.Getenv("JFTRADE_UPDATE_RUST_MIGRATION_FIXTURES") == "1" {
		contents, err := json.MarshalIndent(want, "", "  ")
		if err != nil {
			t.Fatalf("encode watchlist-write fixture: %v", err)
		}
		if err := os.WriteFile(fixturePath, append(contents, '\n'), 0o644); err != nil {
			t.Fatalf("write watchlist-write fixture: %v", err)
		}
	}
	contents, err := os.ReadFile(fixturePath)
	if err != nil {
		t.Fatalf("read watchlist-write fixture: %v", err)
	}
	var got stage9WatchlistWriteFixture
	if err := json.Unmarshal(contents, &got); err != nil {
		t.Fatalf("decode watchlist-write fixture: %v", err)
	}
	gotJSON, _ := json.Marshal(got)
	wantJSON, _ := json.Marshal(want)
	var gotValue, wantValue any
	_ = json.Unmarshal(gotJSON, &gotValue)
	_ = json.Unmarshal(wantJSON, &wantValue)
	gotCanonical, _ := json.Marshal(gotValue)
	wantCanonical, _ := json.Marshal(wantValue)
	if string(gotCanonical) != string(wantCanonical) {
		t.Fatalf("stage 9 watchlist-write fixture drifted from the Go owner")
	}
}

func stage9WatchlistWriteCaseSpecs() []stage9WatchlistWriteCaseSpec {
	body := func(value string) *string { return &value }
	request := func(method, path string, value *string) stage9WatchlistWriteFixtureRequest {
		return stage9WatchlistWriteFixtureRequest{Method: method, Path: path, Body: value}
	}
	groupBody := `{"name":"  Growth  "}`
	updateBody := `{"name":"  Updated  ","expectedRevision":1}`
	membershipBody := `{"groupIds":["tech"],"newGroupNames":[],"expectedRevision":0}`
	return []stage9WatchlistWriteCaseSpec{
		{
			Name:     "create-success",
			Requests: []stage9WatchlistWriteFixtureRequest{request(http.MethodPost, "/api/v1/watchlist/groups", body(groupBody))},
		},
		{
			Name:     "create-empty-object",
			Requests: []stage9WatchlistWriteFixtureRequest{request(http.MethodPost, "/api/v1/watchlist/groups", body(`{}`))},
		},
		{
			Name:     "create-whitespace-name",
			Requests: []stage9WatchlistWriteFixtureRequest{request(http.MethodPost, "/api/v1/watchlist/groups", body(`{"name":"   "}`))},
		},
		{
			Name:     "create-unknown-field-and-trailing-value",
			Requests: []stage9WatchlistWriteFixtureRequest{request(http.MethodPost, "/api/v1/watchlist/groups", body(`{"name":"Trailing","unknown":true}{"ignored":true}`))},
		},
		{
			Name:     "create-duplicate-name",
			Requests: []stage9WatchlistWriteFixtureRequest{request(http.MethodPost, "/api/v1/watchlist/groups", body(`{"name":" Growth "}`))},
			Setup: func(repository *stage9WatchlistWriteRepository, _ *stage9WatchlistWriteReader, _ *stage9WatchlistWriteQuotes) {
				repository.seedGroup("existing", "Growth", 1, false, false)
			},
		},
		{
			Name:     "create-repository-unavailable",
			Mode:     "repo-unavailable",
			Requests: []stage9WatchlistWriteFixtureRequest{request(http.MethodPost, "/api/v1/watchlist/groups", body(groupBody))},
		},
		{
			Name: "create-failure-recovery",
			Mode: "create-failure-once",
			Requests: []stage9WatchlistWriteFixtureRequest{
				request(http.MethodPost, "/api/v1/watchlist/groups", body(groupBody)),
				request(http.MethodPost, "/api/v1/watchlist/groups", body(`{"name":"Recovered"}`)),
			},
		},
		{
			Name:     "create-cancelled",
			Requests: []stage9WatchlistWriteFixtureRequest{{Method: http.MethodPost, Path: "/api/v1/watchlist/groups", Body: body(groupBody), Context: "canceled"}},
		},
		{
			Name:     "create-port-unavailable",
			Mode:     "no-port",
			Requests: []stage9WatchlistWriteFixtureRequest{request(http.MethodPost, "/api/v1/watchlist/groups", body(groupBody))},
		},
		{
			Name:     "update-success",
			Requests: []stage9WatchlistWriteFixtureRequest{request(http.MethodPatch, "/api/v1/watchlist/groups/group-1", body(updateBody))},
			Setup: func(repository *stage9WatchlistWriteRepository, _ *stage9WatchlistWriteReader, _ *stage9WatchlistWriteQuotes) {
				repository.seedGroup("group-1", "Original", 1, false, false)
			},
		},
		{
			Name:     "update-malformed-body",
			Requests: []stage9WatchlistWriteFixtureRequest{request(http.MethodPatch, "/api/v1/watchlist/groups/group-1", body("{"))},
			Setup: func(repository *stage9WatchlistWriteRepository, _ *stage9WatchlistWriteReader, _ *stage9WatchlistWriteQuotes) {
				repository.seedGroup("group-1", "Original", 1, false, false)
			},
		},
		{
			Name:     "update-not-found",
			Requests: []stage9WatchlistWriteFixtureRequest{request(http.MethodPatch, "/api/v1/watchlist/groups/missing", body(updateBody))},
		},
		{
			Name:     "update-protected",
			Requests: []stage9WatchlistWriteFixtureRequest{request(http.MethodPatch, "/api/v1/watchlist/groups/default", body(updateBody))},
			Setup: func(repository *stage9WatchlistWriteRepository, _ *stage9WatchlistWriteReader, _ *stage9WatchlistWriteQuotes) {
				repository.seedGroup("default", domain.DefaultGroupName, 1, true, true)
			},
		},
		{
			Name:     "update-stale-revision",
			Requests: []stage9WatchlistWriteFixtureRequest{request(http.MethodPatch, "/api/v1/watchlist/groups/group-1", body(updateBody))},
			Setup: func(repository *stage9WatchlistWriteRepository, _ *stage9WatchlistWriteReader, _ *stage9WatchlistWriteQuotes) {
				repository.seedGroup("group-1", "Original", 2, false, false)
			},
		},
		{
			Name: "update-failure-recovery",
			Mode: "update-failure-once",
			Requests: []stage9WatchlistWriteFixtureRequest{
				request(http.MethodPatch, "/api/v1/watchlist/groups/group-1", body(updateBody)),
				request(http.MethodPatch, "/api/v1/watchlist/groups/group-1", body(updateBody)),
			},
			Setup: func(repository *stage9WatchlistWriteRepository, _ *stage9WatchlistWriteReader, _ *stage9WatchlistWriteQuotes) {
				repository.seedGroup("group-1", "Original", 1, false, false)
			},
		},
		{
			Name: "delete-group-success-and-repeat",
			Requests: []stage9WatchlistWriteFixtureRequest{
				request(http.MethodDelete, "/api/v1/watchlist/groups/group-1", body("{")),
				request(http.MethodDelete, "/api/v1/watchlist/groups/group-1", nil),
			},
			Setup: func(repository *stage9WatchlistWriteRepository, _ *stage9WatchlistWriteReader, _ *stage9WatchlistWriteQuotes) {
				repository.seedGroup("group-1", "Delete me", 1, false, false)
			},
		},
		{
			Name:     "delete-group-protected",
			Requests: []stage9WatchlistWriteFixtureRequest{request(http.MethodDelete, "/api/v1/watchlist/groups/default", nil)},
			Setup: func(repository *stage9WatchlistWriteRepository, _ *stage9WatchlistWriteReader, _ *stage9WatchlistWriteQuotes) {
				repository.seedGroup("default", domain.DefaultGroupName, 1, true, true)
			},
		},
		{
			Name:     "delete-group-not-found",
			Requests: []stage9WatchlistWriteFixtureRequest{request(http.MethodDelete, "/api/v1/watchlist/groups/missing", nil)},
		},
		{
			Name: "delete-binding-success-and-repeat",
			Requests: []stage9WatchlistWriteFixtureRequest{
				request(http.MethodDelete, "/api/v1/watchlist/bindings?bindingId=binding-1", body("{")),
				request(http.MethodDelete, "/api/v1/watchlist/bindings?bindingId=binding-1", nil),
			},
			Setup: func(repository *stage9WatchlistWriteRepository, _ *stage9WatchlistWriteReader, _ *stage9WatchlistWriteQuotes) {
				repository.seedBinding("binding-1")
			},
		},
		{
			Name:     "delete-binding-missing-id",
			Requests: []stage9WatchlistWriteFixtureRequest{request(http.MethodDelete, "/api/v1/watchlist/bindings?sourceId=futu:default", nil)},
		},
		{
			Name:     "delete-binding-malformed-query",
			Requests: []stage9WatchlistWriteFixtureRequest{request(http.MethodDelete, "/api/v1/watchlist/bindings?%zz", nil)},
		},
		{
			Name: "membership-success-idempotent",
			Requests: []stage9WatchlistWriteFixtureRequest{
				request(http.MethodPut, "/api/v1/watchlist/instruments/US/AAPL/memberships", body(membershipBody)),
				request(http.MethodPut, "/api/v1/watchlist/instruments/US/AAPL/memberships", body(`{"groupIds":["tech"],"newGroupNames":[],"expectedRevision":1}`)),
			},
			Setup: func(repository *stage9WatchlistWriteRepository, _ *stage9WatchlistWriteReader, _ *stage9WatchlistWriteQuotes) {
				repository.seedGroup("tech", "Tech", 1, false, false)
			},
		},
		{
			Name:     "membership-new-groups-and-alias",
			Requests: []stage9WatchlistWriteFixtureRequest{request(http.MethodPut, "/api/v1/watchlist/instruments/CNSH/600519/memberships", body(`{"groupIds":["tech","tech"],"newGroupNames":[" Growth ","growth"],"expectedRevision":0}`))},
			Setup: func(repository *stage9WatchlistWriteRepository, _ *stage9WatchlistWriteReader, _ *stage9WatchlistWriteQuotes) {
				repository.seedGroup("tech", "科技", 1, false, false)
			},
		},
		{
			Name:     "membership-unknown-group",
			Requests: []stage9WatchlistWriteFixtureRequest{request(http.MethodPut, "/api/v1/watchlist/instruments/US/AAPL/memberships", body(`{"groupIds":["missing"],"expectedRevision":0}`))},
		},
		{
			Name:     "membership-conflict",
			Requests: []stage9WatchlistWriteFixtureRequest{request(http.MethodPut, "/api/v1/watchlist/instruments/US/AAPL/memberships", body(`{"groupIds":[],"expectedRevision":1}`))},
			Setup: func(repository *stage9WatchlistWriteRepository, _ *stage9WatchlistWriteReader, _ *stage9WatchlistWriteQuotes) {
				repository.seedInstrument("US.AAPL", 2, nil)
			},
		},
		{
			Name:     "membership-invalid-market",
			Requests: []stage9WatchlistWriteFixtureRequest{request(http.MethodPut, "/api/v1/watchlist/instruments/BAD/AAPL/memberships", body(`{"groupIds":[]}`))},
		},
		{
			Name:     "membership-failure-rolls-back",
			Mode:     "membership-failure",
			Requests: []stage9WatchlistWriteFixtureRequest{request(http.MethodPut, "/api/v1/watchlist/instruments/US/AAPL/memberships", body(membershipBody))},
			Setup: func(repository *stage9WatchlistWriteRepository, _ *stage9WatchlistWriteReader, _ *stage9WatchlistWriteQuotes) {
				repository.seedGroup("tech", "Tech", 1, false, false)
				repository.seedInstrument("US.AAPL", 1, []string{"tech"})
			},
		},
		{
			Name:     "preview-success-local-diff",
			Requests: []stage9WatchlistWriteFixtureRequest{request(http.MethodPost, "/api/v1/watchlist/imports/preview", body(`{"sourceId":"futu:default","remoteGroupId":"remote-tech","localGroupId":"local"}`))},
			Setup: func(repository *stage9WatchlistWriteRepository, reader *stage9WatchlistWriteReader, _ *stage9WatchlistWriteQuotes) {
				repository.seedGroup("local", "Local", 3, false, false)
				repository.seedInstrument("US.AAPL", 1, []string{"local"})
				repository.seedRemoteGroup("futu:default", domain.RemoteGroup{RemoteGroupID: "remote-tech", Name: "Tech", Type: "stock"})
				reader.groups = []domain.RemoteGroup{{RemoteGroupID: "remote-tech", Name: "Tech", Type: "stock"}}
				reader.members = []domain.RemoteMember{{InstrumentID: "US.AAPL", Name: "Apple", Type: "stock"}, {InstrumentID: "US.MSFT", Name: "Microsoft", Type: "stock"}}
			},
		},
		{
			Name:     "preview-success-default-new-group",
			Requests: []stage9WatchlistWriteFixtureRequest{request(http.MethodPost, "/api/v1/watchlist/imports/preview", body(`{"sourceId":"futu:default","remoteGroupId":"remote-tech"}`))},
			Setup: func(repository *stage9WatchlistWriteRepository, reader *stage9WatchlistWriteReader, _ *stage9WatchlistWriteQuotes) {
				repository.seedRemoteGroup("futu:default", domain.RemoteGroup{RemoteGroupID: "remote-tech", Name: "Tech", Type: "stock"})
				reader.groups = []domain.RemoteGroup{{RemoteGroupID: "remote-tech", Name: "Tech", Type: "stock"}}
				reader.members = []domain.RemoteMember{{InstrumentID: "US.AAPL", Name: "Apple", Type: "stock"}}
			},
		},
		{
			Name:     "preview-malformed-body",
			Requests: []stage9WatchlistWriteFixtureRequest{request(http.MethodPost, "/api/v1/watchlist/imports/preview", body("{"))},
		},
		{
			Name:     "preview-missing-source",
			Requests: []stage9WatchlistWriteFixtureRequest{request(http.MethodPost, "/api/v1/watchlist/imports/preview", body(`{"sourceId":"missing","remoteGroupId":"remote-tech"}`))},
		},
		{
			Name:     "preview-ambiguous-remote-group",
			Requests: []stage9WatchlistWriteFixtureRequest{request(http.MethodPost, "/api/v1/watchlist/imports/preview", body(`{"sourceId":"futu:default","remoteGroupId":"remote-one"}`))},
			Setup: func(_ *stage9WatchlistWriteRepository, reader *stage9WatchlistWriteReader, _ *stage9WatchlistWriteQuotes) {
				reader.groups = []domain.RemoteGroup{{RemoteGroupID: "remote-one", Name: "Same"}, {RemoteGroupID: "remote-two", Name: "Same"}}
			},
		},
		{
			Name:     "preview-repository-unavailable",
			Mode:     "no-port",
			Requests: []stage9WatchlistWriteFixtureRequest{request(http.MethodPost, "/api/v1/watchlist/imports/preview", body(`{"sourceId":"futu:default","remoteGroupId":"remote-tech"}`))},
		},
		{
			Name: "commit-success-and-repeat",
			Requests: []stage9WatchlistWriteFixtureRequest{
				request(http.MethodPost, "/api/v1/watchlist/imports/preview-1/commit", nil),
				request(http.MethodPost, "/api/v1/watchlist/imports/preview-1/commit", body(`{}`)),
			},
			Setup: func(repository *stage9WatchlistWriteRepository, reader *stage9WatchlistWriteReader, _ *stage9WatchlistWriteQuotes) {
				repository.seedGroup("local", "Local", 1, false, false)
				repository.seedPreview("preview-1", "local", 1, []domain.ImportDiffItem{{InstrumentID: "US.MSFT", Selected: true}}, []domain.ImportDiffItem{{InstrumentID: "US.AAPL", Selected: false}}, nil)
				reader.groups = []domain.RemoteGroup{{RemoteGroupID: "remote-tech", Name: "Tech", Type: "stock"}}
				reader.members = []domain.RemoteMember{{InstrumentID: "US.AAPL", Name: "Apple", Type: "stock"}, {InstrumentID: "US.MSFT", Name: "Microsoft", Type: "stock"}}
				reader.freshMembers = append([]domain.RemoteMember(nil), reader.members...)
			},
		},
		{
			Name:     "commit-not-found",
			Requests: []stage9WatchlistWriteFixtureRequest{request(http.MethodPost, "/api/v1/watchlist/imports/missing/commit", nil)},
			Setup: func(_ *stage9WatchlistWriteRepository, reader *stage9WatchlistWriteReader, _ *stage9WatchlistWriteQuotes) {
				reader.freshMembers = []domain.RemoteMember{}
			},
		},
		{
			Name:     "commit-expired",
			Requests: []stage9WatchlistWriteFixtureRequest{request(http.MethodPost, "/api/v1/watchlist/imports/preview-1/commit", nil)},
			Setup: func(repository *stage9WatchlistWriteRepository, _ *stage9WatchlistWriteReader, _ *stage9WatchlistWriteQuotes) {
				repository.seedPreviewWithExpiry("preview-1", "local", 1, stage9WatchlistWriteNow.Add(-time.Minute))
			},
		},
		{
			Name:     "commit-stale-remote",
			Requests: []stage9WatchlistWriteFixtureRequest{request(http.MethodPost, "/api/v1/watchlist/imports/preview-1/commit", nil)},
			Setup: func(repository *stage9WatchlistWriteRepository, reader *stage9WatchlistWriteReader, _ *stage9WatchlistWriteQuotes) {
				repository.seedPreview("preview-1", "local", 1, nil, nil, nil)
				reader.freshMembers = []domain.RemoteMember{{InstrumentID: "US.NVDA"}}
			},
		},
		{
			Name:     "commit-invalid-delete",
			Requests: []stage9WatchlistWriteFixtureRequest{request(http.MethodPost, "/api/v1/watchlist/imports/preview-1/commit", body(`{"deleteInstrumentIds":["US.AAPL"]}`))},
			Setup: func(repository *stage9WatchlistWriteRepository, reader *stage9WatchlistWriteReader, _ *stage9WatchlistWriteQuotes) {
				repository.seedGroup("local", "Local", 1, false, false)
				repository.seedPreview("preview-1", "local", 1, nil, nil, []domain.ImportDiffItem{{InstrumentID: "US.MSFT", Selected: false}})
				reader.freshMembers = []domain.RemoteMember{{InstrumentID: "US.AAPL", Name: "Apple", Type: "stock"}, {InstrumentID: "US.MSFT", Name: "Microsoft", Type: "stock"}}
			},
		},
		{
			Name:     "commit-failure-rolls-back",
			Mode:     "commit-failure",
			Requests: []stage9WatchlistWriteFixtureRequest{request(http.MethodPost, "/api/v1/watchlist/imports/preview-1/commit", nil)},
			Setup: func(repository *stage9WatchlistWriteRepository, reader *stage9WatchlistWriteReader, _ *stage9WatchlistWriteQuotes) {
				repository.seedGroup("local", "Local", 1, false, false)
				repository.seedPreview("preview-1", "local", 1, nil, nil, nil)
				reader.freshMembers = []domain.RemoteMember{{InstrumentID: "US.AAPL", Name: "Apple", Type: "stock"}, {InstrumentID: "US.MSFT", Name: "Microsoft", Type: "stock"}}
			},
		},
		{
			Name:     "quotes-success-deduplicates",
			Requests: []stage9WatchlistWriteFixtureRequest{request(http.MethodPost, "/api/v1/watchlist/quotes/batch", body(`{"instrumentIds":["us:aapl","US.AAPL","SH.600519"]}`))},
			Setup: func(_ *stage9WatchlistWriteRepository, _ *stage9WatchlistWriteReader, quotes *stage9WatchlistWriteQuotes) {
				priceOne, priceTwo := 100.5, 88.25
				quotes.quotes = []domain.Quote{
					{InstrumentID: "US.AAPL", Name: "Apple", Type: "stock", Source: "fixture", Price: &priceOne, ObservedAt: stage9WatchlistWriteNow},
					{InstrumentID: "SH.600519", Name: "贵州茅台", Type: "stock", Source: "fixture", Price: &priceTwo, ObservedAt: stage9WatchlistWriteNow},
				}
			},
		},
		{
			Name:     "quotes-item-errors",
			Requests: []stage9WatchlistWriteFixtureRequest{request(http.MethodPost, "/api/v1/watchlist/quotes/batch", body(`{"instrumentIds":["US.AAPL","US.MSFT"]}`))},
			Setup: func(_ *stage9WatchlistWriteRepository, _ *stage9WatchlistWriteReader, quotes *stage9WatchlistWriteQuotes) {
				quotes.itemErrors = []domain.QuoteError{{InstrumentID: "US.MSFT", Code: "NO_PERMISSION", Message: "quote permission denied"}}
			},
		},
		{
			Name:     "quotes-source-unavailable",
			Requests: []stage9WatchlistWriteFixtureRequest{request(http.MethodPost, "/api/v1/watchlist/quotes/batch", body(`{"instrumentIds":["US.AAPL"]}`))},
		},
		{
			Name:     "quotes-malformed-body",
			Requests: []stage9WatchlistWriteFixtureRequest{request(http.MethodPost, "/api/v1/watchlist/quotes/batch", body("{"))},
			Setup: func(_ *stage9WatchlistWriteRepository, _ *stage9WatchlistWriteReader, quotes *stage9WatchlistWriteQuotes) {
				quotes.quotes = []domain.Quote{}
			},
		},
		{
			Name:     "quotes-cancelled-source",
			Requests: []stage9WatchlistWriteFixtureRequest{{Method: http.MethodPost, Path: "/api/v1/watchlist/quotes/batch", Body: body(`{"instrumentIds":["US.AAPL"]}`), Context: "canceled"}},
			Setup: func(_ *stage9WatchlistWriteRepository, _ *stage9WatchlistWriteReader, quotes *stage9WatchlistWriteQuotes) {
				quotes.err = errors.New("quote provider unavailable")
			},
		},
		{
			Name:       "update-concurrent-revision-fence",
			Concurrent: true,
			Requests: []stage9WatchlistWriteFixtureRequest{
				request(http.MethodPatch, "/api/v1/watchlist/groups/group-1", body(`{"name":"Concurrent","expectedRevision":1}`)),
				request(http.MethodPatch, "/api/v1/watchlist/groups/group-1", body(`{"name":"Concurrent","expectedRevision":1}`)),
				request(http.MethodPatch, "/api/v1/watchlist/groups/group-1", body(`{"name":"Concurrent","expectedRevision":1}`)),
			},
			Setup: func(repository *stage9WatchlistWriteRepository, _ *stage9WatchlistWriteReader, _ *stage9WatchlistWriteQuotes) {
				repository.seedGroup("group-1", "Original", 1, false, false)
			},
		},
	}
}

func runStage9WatchlistWriteCase(t *testing.T, spec stage9WatchlistWriteCaseSpec) stage9WatchlistWriteFixtureCase {
	t.Helper()
	repository := newStage9WatchlistWriteRepository(spec.Mode)
	reader := &stage9WatchlistWriteReader{}
	quotes := &stage9WatchlistWriteQuotes{}
	if spec.Setup != nil {
		spec.Setup(repository, reader, quotes)
	}
	var service *domain.Service
	if spec.Mode != "no-port" {
		options := []domain.Option{domain.WithClock(func() time.Time { return stage9WatchlistWriteNow })}
		if len(reader.groups) > 0 || len(reader.members) > 0 || len(reader.freshMembers) > 0 || spec.Name == "preview-missing-source" || strings.HasPrefix(spec.Name, "commit-") {
			options = append(options, domain.WithSourceReader("futu:default", reader))
		}
		if spec.Name != "quotes-source-unavailable" && strings.HasPrefix(spec.Name, "quotes-") {
			options = append(options, domain.WithBatchSnapshotSource(quotes))
		}
		service = domain.NewService(repository, options...)
	}
	gin.SetMode(gin.TestMode)
	router := gin.New()
	watchlistapi.RegisterRoutes(router.Group("/api/v1"), service)

	type observed struct {
		request  stage9WatchlistWriteFixtureRequest
		expected stage9WatchlistWriteFixtureExpected
		action   map[string]any
		orderKey string
	}
	results := make([]observed, len(spec.Requests))
	serve := func(index int) {
		request := spec.Requests[index]
		response := serveStage9WatchlistWriteRequest(t, router, request)
		var envelope map[string]any
		if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil {
			t.Fatalf("case %s decode response: %v; body=%s", spec.Name, err, response.Body.String())
		}
		normalizeStage9WatchlistWriteJSON(envelope, nil)
		headers := map[string]string{"Content-Type": response.Header().Get("Content-Type")}
		expected := stage9WatchlistWriteFixtureExpected{
			Status: response.Code, Headers: headers, Envelope: envelope,
			PortCall: stage9WatchlistWriteMutationPortDispatchable(request) && spec.Mode != "no-port",
		}
		action, _ := stage9WatchlistWriteAction(request)
		results[index] = observed{request: request, expected: expected, action: action, orderKey: fmt.Sprintf("%03d:%s", response.Code, compactStage9WatchlistWriteJSON(envelope))}
	}
	if spec.Concurrent {
		var waitGroup sync.WaitGroup
		for index := range spec.Requests {
			waitGroup.Add(1)
			go func(index int) {
				defer waitGroup.Done()
				serve(index)
			}(index)
		}
		waitGroup.Wait()
	} else {
		for index := range spec.Requests {
			serve(index)
		}
	}
	sort.SliceStable(results, func(left, right int) bool { return results[left].orderKey < results[right].orderKey })
	caseFixture := stage9WatchlistWriteFixtureCase{
		Name: spec.Name, PortMode: spec.Mode, Concurrent: spec.Concurrent,
		Requests:            make([]stage9WatchlistWriteFixtureRequest, 0, len(results)),
		Expected:            make([]stage9WatchlistWriteFixtureExpected, 0, len(results)),
		Calls:               make([]map[string]any, 0, len(results)),
		ExpectedObservation: repository.observation(),
	}
	for _, item := range results {
		caseFixture.Requests = append(caseFixture.Requests, item.request)
		caseFixture.Expected = append(caseFixture.Expected, item.expected)
		if item.expected.PortCall && item.action != nil {
			caseFixture.Calls = append(caseFixture.Calls, item.action)
		}
	}
	canonicalizeStage9WatchlistWriteFixture(&caseFixture)
	return caseFixture
}

func serveStage9WatchlistWriteRequest(t *testing.T, router http.Handler, request stage9WatchlistWriteFixtureRequest) *httptest.ResponseRecorder {
	t.Helper()
	var body []byte
	if request.Body != nil {
		body = []byte(*request.Body)
	}
	httpRequest := httptest.NewRequest(request.Method, request.Path, bytes.NewReader(body))
	if request.Body != nil {
		httpRequest.Header.Set("Content-Type", "application/json")
	}
	if request.Context == "canceled" {
		ctx, cancel := context.WithCancel(httpRequest.Context())
		cancel()
		httpRequest = httpRequest.WithContext(ctx)
	}
	if request.Context == "deadline-exceeded" {
		ctx, cancel := context.WithDeadline(httpRequest.Context(), time.Now().Add(-time.Second))
		defer cancel()
		httpRequest = httpRequest.WithContext(ctx)
	}
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httpRequest)
	return response
}

func newStage9WatchlistWriteRepository(mode string) *stage9WatchlistWriteRepository {
	return &stage9WatchlistWriteRepository{
		mode: mode, now: stage9WatchlistWriteNow, nextGroup: 1, nextPreview: 1, nextRun: 1,
		groups: make(map[string]domain.Group), instruments: make(map[string]domain.Instrument),
		memberships: make(map[string]map[string]struct{}), bindings: make(map[string]domain.Binding),
		previews: make(map[string]domain.ImportPreview), committed: make(map[string]bool),
		remoteGroups: make(map[string][]domain.RemoteGroup), sources: make(map[string]domain.Source),
		operationCounts: make(map[string]int),
	}
}

func (r *stage9WatchlistWriteRepository) contextError(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
		return nil
	}
}

func (r *stage9WatchlistWriteRepository) repositoryError(operation string) error {
	r.operationCounts[operation]++
	if r.mode == "repo-unavailable" {
		return domain.ErrUnavailable
	}
	switch r.mode {
	case "create-failure-once":
		if operation == "create" && r.operationCounts[operation] == 1 {
			return errors.New("database write failed")
		}
	case "update-failure-once":
		if operation == "update" && r.operationCounts[operation] == 1 {
			return errors.New("database write failed")
		}
	case "membership-failure":
		if operation == "membership" {
			return errors.New("database write failed")
		}
	case "commit-failure":
		if operation == "commit" {
			return errors.New("database write failed")
		}
	}
	return nil
}

func (r *stage9WatchlistWriteRepository) seedGroup(id, name string, revision int64, protected, isDefault bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.groups[id] = domain.Group{ID: id, Name: name, IsDefault: isDefault, Protected: protected, Revision: revision, CreatedAt: r.now, UpdatedAt: r.now}
}

func (r *stage9WatchlistWriteRepository) seedInstrument(id string, revision int64, groups []string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.instruments[id] = domain.Instrument{ID: id, Market: strings.SplitN(id, ".", 2)[0], Symbol: strings.SplitN(id, ".", 2)[1], Revision: revision}
	set := make(map[string]struct{}, len(groups))
	for _, groupID := range groups {
		set[groupID] = struct{}{}
	}
	r.memberships[id] = set
}

func (r *stage9WatchlistWriteRepository) seedBinding(id string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.bindings[id] = domain.Binding{ID: id, SourceID: "futu:default", RemoteGroupID: "remote-tech", RemoteName: "Tech", LocalGroupID: "local", CreatedAt: r.now, UpdatedAt: r.now}
}

func (r *stage9WatchlistWriteRepository) seedRemoteGroup(sourceID string, group domain.RemoteGroup) {
	r.mu.Lock()
	defer r.mu.Unlock()
	group.SourceID = sourceID
	group.ObservedAt = r.now
	r.remoteGroups[sourceID] = append(r.remoteGroups[sourceID], group)
}

func (r *stage9WatchlistWriteRepository) seedPreview(id, localGroupID string, localRevision int64, added, unchanged, localOnly []domain.ImportDiffItem) {
	r.seedPreviewWithExpiry(id, localGroupID, localRevision, r.now.Add(10*time.Minute))
	r.mu.Lock()
	preview := r.previews[id]
	preview.Added = append([]domain.ImportDiffItem(nil), added...)
	preview.Unchanged = append([]domain.ImportDiffItem(nil), unchanged...)
	preview.LocalOnly = append([]domain.ImportDiffItem(nil), localOnly...)
	preview.RemoteHash = stage9WatchlistWriteHashMembers([]domain.RemoteMember{{InstrumentID: "US.AAPL", Name: "Apple", Type: "stock"}, {InstrumentID: "US.MSFT", Name: "Microsoft", Type: "stock"}})
	r.previews[id] = preview
	r.mu.Unlock()
}

func (r *stage9WatchlistWriteRepository) seedPreviewWithExpiry(id, localGroupID string, localRevision int64, expiresAt time.Time) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.previews[id] = domain.ImportPreview{
		ID: id, SourceID: "futu:default", RemoteGroupID: "remote-tech", RemoteGroupName: "Tech",
		LocalGroupID: localGroupID, LocalGroupRevision: localRevision, RemoteHash: stage9WatchlistWriteHashMembers(nil),
		CreatedAt: r.now, ExpiresAt: expiresAt, Added: []domain.ImportDiffItem{}, Unchanged: []domain.ImportDiffItem{}, LocalOnly: []domain.ImportDiffItem{},
	}
}

func stage9WatchlistWriteHashMembers(members []domain.RemoteMember) string {
	ids := make([]string, 0, len(members))
	for _, member := range members {
		ids = append(ids, member.InstrumentID)
	}
	sort.Strings(ids)
	digest := sha256.Sum256([]byte(strings.Join(ids, "\n")))
	return hex.EncodeToString(digest[:])
}

func (r *stage9WatchlistWriteRepository) ListGroups(ctx context.Context) ([]domain.Group, error) {
	if err := r.contextError(ctx); err != nil {
		return nil, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	groups := make([]domain.Group, 0, len(r.groups))
	for _, group := range r.groups {
		group.ItemCount = len(r.membershipsForGroupLocked(group.ID))
		groups = append(groups, group)
	}
	sort.Slice(groups, func(i, j int) bool {
		if groups[i].IsDefault != groups[j].IsDefault {
			return groups[i].IsDefault
		}
		return groups[i].ID < groups[j].ID
	})
	return groups, nil
}

func (r *stage9WatchlistWriteRepository) GetGroup(ctx context.Context, id string) (domain.Group, error) {
	if err := r.contextError(ctx); err != nil {
		return domain.Group{}, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	group, ok := r.groups[strings.TrimSpace(id)]
	if !ok {
		return domain.Group{}, domain.ErrNotFound
	}
	group.ItemCount = len(r.membershipsForGroupLocked(group.ID))
	return group, nil
}

func (r *stage9WatchlistWriteRepository) CreateGroup(ctx context.Context, name string) (domain.Group, error) {
	if err := r.contextError(ctx); err != nil {
		return domain.Group{}, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.mutationCalls++
	if err := r.repositoryError("create"); err != nil {
		return domain.Group{}, err
	}
	for _, group := range r.groups {
		if domain.GroupNameKey(group.Name) == domain.GroupNameKey(name) {
			return domain.Group{}, fmt.Errorf("%w: UNIQUE constraint failed: watchlist_groups.name_key", domain.ErrConflict)
		}
	}
	id := fmt.Sprintf("wlgrp_generated-%d", r.nextGroup)
	r.nextGroup++
	group := domain.Group{ID: id, Name: strings.TrimSpace(name), Revision: 1, CreatedAt: r.now, UpdatedAt: r.now}
	r.groups[id] = group
	return group, nil
}

func (r *stage9WatchlistWriteRepository) UpdateGroup(ctx context.Context, id, name string, expected int64) (domain.Group, error) {
	if err := r.contextError(ctx); err != nil {
		return domain.Group{}, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.mutationCalls++
	if err := r.repositoryError("update"); err != nil {
		return domain.Group{}, err
	}
	group, ok := r.groups[strings.TrimSpace(id)]
	if !ok {
		return domain.Group{}, domain.ErrNotFound
	}
	if group.Protected {
		return domain.Group{}, domain.ErrProtectedGroup
	}
	if group.Revision != expected {
		return domain.Group{}, domain.ErrConflict
	}
	for otherID, other := range r.groups {
		if otherID != group.ID && domain.GroupNameKey(other.Name) == domain.GroupNameKey(name) {
			return domain.Group{}, fmt.Errorf("%w: UNIQUE constraint failed: watchlist_groups.name_key", domain.ErrConflict)
		}
	}
	group.Name, group.Revision, group.UpdatedAt = strings.TrimSpace(name), group.Revision+1, r.now
	r.groups[group.ID] = group
	return group, nil
}

func (r *stage9WatchlistWriteRepository) DeleteGroup(ctx context.Context, id string) error {
	if err := r.contextError(ctx); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.mutationCalls++
	if err := r.repositoryError("delete-group"); err != nil {
		return err
	}
	group, ok := r.groups[strings.TrimSpace(id)]
	if !ok {
		return domain.ErrNotFound
	}
	if group.Protected {
		return domain.ErrProtectedGroup
	}
	delete(r.groups, group.ID)
	for instrumentID, memberships := range r.memberships {
		if _, ok := memberships[group.ID]; ok {
			delete(memberships, group.ID)
			instrument := r.instruments[instrumentID]
			instrument.Revision++
			r.instruments[instrumentID] = instrument
		}
	}
	for bindingID, binding := range r.bindings {
		if binding.LocalGroupID == group.ID {
			delete(r.bindings, bindingID)
		}
	}
	return nil
}

func (r *stage9WatchlistWriteRepository) ListItems(context.Context, domain.ListItemsOptions) (domain.ItemPage, error) {
	return domain.ItemPage{Items: []domain.Item{}}, nil
}

func (r *stage9WatchlistWriteRepository) GetMemberships(ctx context.Context, instrumentID string) (domain.Memberships, error) {
	if err := r.contextError(ctx); err != nil {
		return domain.Memberships{}, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	instrument, ok := r.instruments[instrumentID]
	if !ok {
		return domain.Memberships{InstrumentID: instrumentID, Groups: []domain.GroupRef{}}, nil
	}
	return domain.Memberships{InstrumentID: instrumentID, Revision: instrument.Revision, Groups: r.groupRefsLocked(r.memberships[instrumentID])}, nil
}

func (r *stage9WatchlistWriteRepository) ReplaceMemberships(ctx context.Context, input domain.ReplaceMembershipsInput) (domain.Memberships, error) {
	if err := r.contextError(ctx); err != nil {
		return domain.Memberships{}, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.mutationCalls++
	if err := r.repositoryError("membership"); err != nil {
		return domain.Memberships{}, err
	}
	instrument, exists := r.instruments[input.InstrumentID]
	if exists && instrument.Revision != input.ExpectedRevision {
		return domain.Memberships{}, domain.ErrConflict
	}
	if !exists && input.ExpectedRevision != 0 {
		return domain.Memberships{}, domain.ErrConflict
	}
	desired := make(map[string]struct{}, len(input.GroupIDs)+len(input.NewGroupNames))
	for _, groupID := range input.GroupIDs {
		if _, ok := r.groups[groupID]; !ok {
			return domain.Memberships{}, domain.ErrNotFound
		}
		desired[groupID] = struct{}{}
	}
	for _, name := range input.NewGroupNames {
		for _, group := range r.groups {
			if domain.GroupNameKey(group.Name) == domain.GroupNameKey(name) {
				return domain.Memberships{}, fmt.Errorf("%w: UNIQUE constraint failed: watchlist_groups.name_key", domain.ErrConflict)
			}
		}
		id := fmt.Sprintf("wlgrp_generated-%d", r.nextGroup)
		r.nextGroup++
		r.groups[id] = domain.Group{ID: id, Name: name, Revision: 1, CreatedAt: r.now, UpdatedAt: r.now}
		desired[id] = struct{}{}
	}
	current := r.memberships[input.InstrumentID]
	if current == nil {
		current = map[string]struct{}{}
	}
	changed := !sameStage9WatchlistWriteSet(current, desired)
	if !exists {
		instrument = domain.Instrument{ID: input.InstrumentID, Market: strings.SplitN(input.InstrumentID, ".", 2)[0], Symbol: strings.SplitN(input.InstrumentID, ".", 2)[1]}
		r.instruments[input.InstrumentID] = instrument
	}
	if changed {
		instrument.Revision++
		r.instruments[input.InstrumentID] = instrument
		r.memberships[input.InstrumentID] = desired
	}
	return domain.Memberships{InstrumentID: input.InstrumentID, Revision: instrument.Revision, Groups: r.groupRefsLocked(r.memberships[input.InstrumentID])}, nil
}

func sameStage9WatchlistWriteSet(left, right map[string]struct{}) bool {
	if len(left) != len(right) {
		return false
	}
	for key := range left {
		if _, ok := right[key]; !ok {
			return false
		}
	}
	return true
}

func (r *stage9WatchlistWriteRepository) UpsertSource(context.Context, domain.Source) error {
	return nil
}
func (r *stage9WatchlistWriteRepository) ListSources(context.Context) ([]domain.Source, error) {
	return []domain.Source{}, nil
}
func (r *stage9WatchlistWriteRepository) ReplaceRemoteGroups(_ context.Context, sourceID string, groups []domain.RemoteGroup) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.remoteGroups[sourceID] = append([]domain.RemoteGroup(nil), groups...)
	return nil
}
func (r *stage9WatchlistWriteRepository) ListRemoteGroups(_ context.Context, sourceID string) ([]domain.RemoteGroup, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]domain.RemoteGroup(nil), r.remoteGroups[sourceID]...), nil
}

func (r *stage9WatchlistWriteRepository) ListBindings(ctx context.Context, sourceID string) ([]domain.Binding, error) {
	if err := r.contextError(ctx); err != nil {
		return nil, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	result := make([]domain.Binding, 0, len(r.bindings))
	for _, binding := range r.bindings {
		if sourceID == "" || binding.SourceID == sourceID {
			result = append(result, binding)
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	return result, nil
}

func (r *stage9WatchlistWriteRepository) DeleteBinding(ctx context.Context, id string) error {
	if err := r.contextError(ctx); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.mutationCalls++
	if err := r.repositoryError("delete-binding"); err != nil {
		return err
	}
	if _, ok := r.bindings[id]; !ok {
		return domain.ErrNotFound
	}
	delete(r.bindings, id)
	return nil
}

func (r *stage9WatchlistWriteRepository) GroupInstrumentIDs(ctx context.Context, groupID string) ([]string, error) {
	if err := r.contextError(ctx); err != nil {
		return nil, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.groups[groupID]; !ok {
		return nil, domain.ErrNotFound
	}
	result := []string{}
	for instrumentID, memberships := range r.memberships {
		if _, ok := memberships[groupID]; ok {
			result = append(result, instrumentID)
		}
	}
	sort.Strings(result)
	return result, nil
}

func (r *stage9WatchlistWriteRepository) SaveImportPreview(ctx context.Context, preview domain.ImportPreview) error {
	if err := r.contextError(ctx); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.mutationCalls++
	if err := r.repositoryError("preview"); err != nil {
		return err
	}
	r.previews[preview.ID] = preview
	return nil
}

func (r *stage9WatchlistWriteRepository) GetImportPreview(ctx context.Context, id string) (domain.ImportPreview, error) {
	if err := r.contextError(ctx); err != nil {
		return domain.ImportPreview{}, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	preview, ok := r.previews[id]
	if !ok {
		return domain.ImportPreview{}, domain.ErrNotFound
	}
	if r.committed[id] {
		return domain.ImportPreview{}, domain.ErrStalePreview
	}
	return preview, nil
}

func (r *stage9WatchlistWriteRepository) CommitImport(ctx context.Context, input domain.CommitImportStoreInput) (domain.ImportRun, error) {
	if err := r.contextError(ctx); err != nil {
		return domain.ImportRun{}, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.mutationCalls++
	if err := r.repositoryError("commit"); err != nil {
		return domain.ImportRun{}, err
	}
	if r.committed[input.Preview.ID] {
		return domain.ImportRun{}, domain.ErrStalePreview
	}
	groupID := input.Preview.LocalGroupID
	if groupID == "" {
		groupID = fmt.Sprintf("wlgrp_generated-%d", r.nextGroup)
		r.nextGroup++
		r.groups[groupID] = domain.Group{ID: groupID, Name: input.Preview.NewGroupName, Revision: 1, CreatedAt: r.now, UpdatedAt: r.now}
	}
	for _, member := range input.RemoteMembers {
		if _, ok := r.instruments[member.InstrumentID]; !ok {
			r.instruments[member.InstrumentID] = domain.Instrument{ID: member.InstrumentID, Market: strings.SplitN(member.InstrumentID, ".", 2)[0], Symbol: strings.SplitN(member.InstrumentID, ".", 2)[1]}
		}
		if r.memberships[member.InstrumentID] == nil {
			r.memberships[member.InstrumentID] = map[string]struct{}{}
		}
		r.memberships[member.InstrumentID][groupID] = struct{}{}
	}
	r.nextRun++
	run := domain.ImportRun{ID: fmt.Sprintf("wlrun_generated-%d", r.nextRun-1), PreviewID: input.Preview.ID, SourceID: input.Preview.SourceID, RemoteGroupID: input.Preview.RemoteGroupID, RemoteGroupName: input.Preview.RemoteGroupName, LocalGroupID: groupID, Status: "completed", AddedCount: len(input.RemoteMembers), RemoteHash: input.Preview.RemoteHash, CreatedAt: r.now, CompletedAt: r.now}
	r.runs = append(r.runs, run)
	r.committed[input.Preview.ID] = true
	return run, nil
}

func (r *stage9WatchlistWriteRepository) ListImportRuns(context.Context, string, string, int) (domain.ImportRunPage, error) {
	return domain.ImportRunPage{Items: append([]domain.ImportRun(nil), r.runs...)}, nil
}

func (r *stage9WatchlistWriteRepository) membershipsForGroupLocked(groupID string) map[string]struct{} {
	result := map[string]struct{}{}
	for instrumentID, groups := range r.memberships {
		if _, ok := groups[groupID]; ok {
			result[instrumentID] = struct{}{}
		}
	}
	return result
}
func (r *stage9WatchlistWriteRepository) groupRefsLocked(groupIDs map[string]struct{}) []domain.GroupRef {
	result := make([]domain.GroupRef, 0, len(groupIDs))
	for id := range groupIDs {
		if group, ok := r.groups[id]; ok {
			result = append(result, domain.GroupRef{ID: id, Name: group.Name})
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	return result
}

func (r *stage9WatchlistWriteRepository) observation() map[string]any {
	r.mu.Lock()
	defer r.mu.Unlock()
	groups := make([]domain.Group, 0, len(r.groups))
	for _, group := range r.groups {
		group.ItemCount = len(r.membershipsForGroupLocked(group.ID))
		groups = append(groups, group)
	}
	sort.Slice(groups, func(i, j int) bool { return groups[i].ID < groups[j].ID })
	memberships := make([]domain.Memberships, 0, len(r.instruments))
	for id, instrument := range r.instruments {
		memberships = append(memberships, domain.Memberships{InstrumentID: id, Revision: instrument.Revision, Groups: r.groupRefsLocked(r.memberships[id])})
	}
	sort.Slice(memberships, func(i, j int) bool { return memberships[i].InstrumentID < memberships[j].InstrumentID })
	bindings := make([]domain.Binding, 0, len(r.bindings))
	for _, binding := range r.bindings {
		bindings = append(bindings, binding)
	}
	sort.Slice(bindings, func(i, j int) bool { return bindings[i].ID < bindings[j].ID })
	return map[string]any{"groups": groups, "memberships": memberships, "bindings": bindings, "previewCount": len(r.previews), "committedPreviewCount": len(r.committed), "runCount": len(r.runs), "mutationCalls": r.mutationCalls}
}

func (r *stage9WatchlistWriteReader) Source(ctx context.Context) (domain.Source, error) {
	if err := stage9WatchlistWriteContextError(ctx); err != nil {
		return domain.Source{}, err
	}
	return domain.Source{ID: "futu:default", Broker: "futu", DisplayName: "Futu", Status: "ready"}, nil
}
func (r *stage9WatchlistWriteReader) ListGroups(ctx context.Context) ([]domain.RemoteGroup, error) {
	if err := stage9WatchlistWriteContextError(ctx); err != nil {
		return nil, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.groupsErr != nil {
		return nil, r.groupsErr
	}
	return append([]domain.RemoteGroup(nil), r.groups...), nil
}
func (r *stage9WatchlistWriteReader) ListGroupMembers(ctx context.Context, _ string) ([]domain.RemoteMember, error) {
	if err := stage9WatchlistWriteContextError(ctx); err != nil {
		return nil, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.membersErr != nil {
		return nil, r.membersErr
	}
	return append([]domain.RemoteMember(nil), r.members...), nil
}
func (r *stage9WatchlistWriteReader) ListGroupMembersFresh(ctx context.Context, _ string) ([]domain.RemoteMember, error) {
	if err := stage9WatchlistWriteContextError(ctx); err != nil {
		return nil, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.freshCalls++
	if r.freshErr != nil {
		return nil, r.freshErr
	}
	return append([]domain.RemoteMember(nil), r.freshMembers...), nil
}
func stage9WatchlistWriteContextError(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
		return nil
	}
}

func (q *stage9WatchlistWriteQuotes) BatchSnapshots(ctx context.Context, ids []string) ([]domain.Quote, []domain.QuoteError, error) {
	if err := stage9WatchlistWriteContextError(ctx); err != nil {
		return nil, nil, err
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	q.requested = append(q.requested, append([]string(nil), ids...))
	if q.err != nil {
		return nil, nil, q.err
	}
	return append([]domain.Quote(nil), q.quotes...), append([]domain.QuoteError(nil), q.itemErrors...), nil
}

func stage9WatchlistWriteMutationPortDispatchable(request stage9WatchlistWriteFixtureRequest) bool {
	_, ok := stage9WatchlistWriteAction(request)
	return ok
}

func stage9WatchlistWriteAction(request stage9WatchlistWriteFixtureRequest) (map[string]any, bool) {
	path, rawQuery, ok := stage9WatchlistWriteSplitPath(request.Path)
	if !ok {
		return nil, false
	}
	if request.Method == http.MethodDelete && path == "/api/v1/watchlist/bindings" {
		values, err := url.ParseQuery(rawQuery)
		if err != nil {
			return nil, false
		}
		return map[string]any{"route": "delete-binding", "bindingId": values.Get("bindingId")}, true
	}
	if (request.Method == http.MethodDelete || request.Method == http.MethodPatch) && strings.HasPrefix(path, "/api/v1/watchlist/groups/") {
		id := strings.TrimPrefix(path, "/api/v1/watchlist/groups/")
		if id == "" || strings.Contains(id, "/") {
			return nil, false
		}
		if request.Method == http.MethodDelete {
			return map[string]any{"route": "delete-group", "groupId": id}, true
		}
		var input struct {
			Name             string `json:"name"`
			ExpectedRevision int64  `json:"expectedRevision"`
		}
		if !stage9WatchlistWriteDecodeBody(request.Body, &input) || input.Name == "" || input.ExpectedRevision < 1 {
			return nil, false
		}
		return map[string]any{"route": "update-group", "groupId": id, "name": input.Name, "expectedRevision": input.ExpectedRevision}, true
	}
	if request.Method == http.MethodPost && path == "/api/v1/watchlist/groups" {
		var input struct {
			Name string `json:"name"`
		}
		if !stage9WatchlistWriteDecodeBody(request.Body, &input) || input.Name == "" {
			return nil, false
		}
		return map[string]any{"route": "create-group", "name": input.Name}, true
	}
	if request.Method == http.MethodPost && path == "/api/v1/watchlist/imports/preview" {
		var input domain.ImportPreviewRequest
		if !stage9WatchlistWriteDecodeBody(request.Body, &input) || input.SourceID == "" || input.RemoteGroupID == "" {
			return nil, false
		}
		return map[string]any{"route": "preview-import", "sourceId": input.SourceID, "remoteGroupId": input.RemoteGroupID, "localGroupId": input.LocalGroupID, "newGroupName": input.NewGroupName}, true
	}
	if request.Method == http.MethodPost && strings.HasPrefix(path, "/api/v1/watchlist/imports/") && strings.HasSuffix(path, "/commit") {
		id := strings.TrimSuffix(strings.TrimPrefix(path, "/api/v1/watchlist/imports/"), "/commit")
		if id == "" || strings.Contains(id, "/") {
			return nil, false
		}
		var input domain.CommitImportInput
		if request.Body != nil && *request.Body != "" && !stage9WatchlistWriteDecodeBody(request.Body, &input) {
			return nil, false
		}
		return map[string]any{"route": "commit-import", "previewId": id, "deleteInstrumentIds": input.DeleteInstrumentIDs}, true
	}
	if request.Method == http.MethodPost && path == "/api/v1/watchlist/quotes/batch" {
		var input struct {
			InstrumentIDs []string `json:"instrumentIds"`
		}
		if !stage9WatchlistWriteDecodeBody(request.Body, &input) || len(input.InstrumentIDs) == 0 {
			return nil, false
		}
		return map[string]any{"route": "batch-quotes", "instrumentIds": input.InstrumentIDs}, true
	}
	if request.Method == http.MethodPut && strings.HasPrefix(path, "/api/v1/watchlist/instruments/") && strings.HasSuffix(path, "/memberships") {
		value := strings.TrimSuffix(strings.TrimPrefix(path, "/api/v1/watchlist/instruments/"), "/memberships")
		parts := strings.Split(value, "/")
		if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
			return nil, false
		}
		var input domain.ReplaceMembershipsInput
		if !stage9WatchlistWriteDecodeBody(request.Body, &input) {
			return nil, false
		}
		return map[string]any{"route": "replace-memberships", "instrumentId": strings.TrimSpace(parts[0]) + "." + strings.TrimSpace(parts[1]), "groupIds": input.GroupIDs, "newGroupNames": input.NewGroupNames, "expectedRevision": input.ExpectedRevision}, true
	}
	return nil, false
}

func stage9WatchlistWriteSplitPath(value string) (string, string, bool) {
	parts := strings.SplitN(value, "?", 2)
	if len(parts) == 1 {
		return parts[0], "", true
	}
	return parts[0], parts[1], true
}

func stage9WatchlistWriteDecodeBody(body *string, target any) bool {
	if body == nil || *body == "" {
		return false
	}
	decoder := json.NewDecoder(strings.NewReader(*body))
	return decoder.Decode(target) == nil
}

func canonicalizeStage9WatchlistWriteFixture(caseFixture *stage9WatchlistWriteFixtureCase) {
	ids := map[string]string{}
	for _, expected := range caseFixture.Expected {
		normalizeStage9WatchlistWriteJSON(expected.Envelope, ids)
	}
	observationJSON, _ := json.Marshal(caseFixture.ExpectedObservation)
	var observation any
	_ = json.Unmarshal(observationJSON, &observation)
	normalizeStage9WatchlistWriteJSON(observation, ids)
	if normalized, ok := observation.(map[string]any); ok {
		caseFixture.ExpectedObservation = normalized
	}
	for _, call := range caseFixture.Calls {
		normalizeStage9WatchlistWriteJSON(call, ids)
	}
}

func normalizeStage9WatchlistWriteJSON(value any, ids map[string]string) any {
	if ids == nil {
		ids = map[string]string{}
	}
	switch current := value.(type) {
	case map[string]any:
		for key, item := range current {
			if key == "timestamp" {
				current[key] = stage9WatchlistWriteTimestamp
				continue
			}
			current[key] = normalizeStage9WatchlistWriteJSON(item, ids)
		}
	case []any:
		for index, item := range current {
			current[index] = normalizeStage9WatchlistWriteJSON(item, ids)
		}
	case string:
		for _, prefix := range []string{"wlgrp_generated-", "wlpreview_", "wlrun_generated-", "wlbind_"} {
			if strings.HasPrefix(current, prefix) {
				if replacement, ok := ids[current]; ok {
					return replacement
				}
				kind := "id"
				switch {
				case strings.HasPrefix(prefix, "wlgrp"):
					kind = "group"
				case strings.HasPrefix(prefix, "wlpreview"):
					kind = "preview"
				case strings.HasPrefix(prefix, "wlrun"):
					kind = "run"
				case strings.HasPrefix(prefix, "wlbind"):
					kind = "binding"
				}
				replacement := fmt.Sprintf("%s-%d", kind, len(ids)+1)
				ids[current] = replacement
				return replacement
			}
		}
	}
	return value
}

func compactStage9WatchlistWriteJSON(value any) string {
	body, _ := json.Marshal(value)
	return string(body)
}
