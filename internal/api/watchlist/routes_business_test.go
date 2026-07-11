package watchlist

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/jftrade/jftrade-main/internal/api/httpserver"
	storewatchlist "github.com/jftrade/jftrade-main/internal/store/watchlist"
	domain "github.com/jftrade/jftrade-main/internal/watchlist"
)

type apiWatchlistSource struct {
	groups  []domain.RemoteGroup
	members map[string][]domain.RemoteMember
}

func (source *apiWatchlistSource) Source(context.Context) (domain.Source, error) {
	return domain.Source{ID: "futu:default", Broker: "futu", DisplayName: "Futu OpenD"}, nil
}

func (source *apiWatchlistSource) ListGroups(context.Context) ([]domain.RemoteGroup, error) {
	return append([]domain.RemoteGroup(nil), source.groups...), nil
}

func (source *apiWatchlistSource) ListGroupMembers(_ context.Context, groupID string) ([]domain.RemoteMember, error) {
	return append([]domain.RemoteMember(nil), source.members[groupID]...), nil
}

func (source *apiWatchlistSource) ListGroupMembersFresh(ctx context.Context, groupID string) ([]domain.RemoteMember, error) {
	return source.ListGroupMembers(ctx, groupID)
}

type apiWatchlistQuotes struct{}

func (apiWatchlistQuotes) BatchSnapshots(_ context.Context, instrumentIDs []string) ([]domain.Quote, []domain.QuoteError, error) {
	quotes := make([]domain.Quote, 0, len(instrumentIDs))
	for _, instrumentID := range instrumentIDs {
		quotes = append(quotes, domain.Quote{InstrumentID: instrumentID, Source: "test", ObservedAt: time.Date(2026, time.July, 11, 10, 0, 0, 0, time.UTC)})
	}
	return quotes, nil, nil
}

func TestWatchlistRoutesMembershipIdempotencyConflictAndPagination(t *testing.T) {
	service, router := newWatchlistAPITest(t)
	groups, err := service.ListGroups(t.Context())
	if err != nil || len(groups) != 1 {
		t.Fatalf("default groups=%#v err=%v", groups, err)
	}
	defaultGroupID := groups[0].ID

	first := performWatchlistRequest(t, router, http.MethodPut, "/api/v1/watchlist/instruments/US/AAPL/memberships", map[string]any{
		"groupIds": []string{defaultGroupID}, "newGroupNames": []string{}, "expectedRevision": 0,
	})
	if first.Code != http.StatusOK {
		t.Fatalf("first membership status=%d body=%s", first.Code, first.Body.String())
	}
	var firstEnvelope struct {
		httpserver.Envelope
		Data domain.Memberships `json:"data"`
	}
	decodeWatchlistResponse(t, first, &firstEnvelope)
	if !firstEnvelope.OK || firstEnvelope.Data.Revision != 1 || len(firstEnvelope.Data.Groups) != 1 {
		t.Fatalf("first membership envelope=%#v", firstEnvelope)
	}

	idempotent := performWatchlistRequest(t, router, http.MethodPut, "/api/v1/watchlist/instruments/US/AAPL/memberships", map[string]any{
		"groupIds": []string{defaultGroupID}, "newGroupNames": []string{}, "expectedRevision": 1,
	})
	if idempotent.Code != http.StatusOK {
		t.Fatalf("idempotent status=%d body=%s", idempotent.Code, idempotent.Body.String())
	}
	var idempotentEnvelope struct {
		httpserver.Envelope
		Data domain.Memberships `json:"data"`
	}
	decodeWatchlistResponse(t, idempotent, &idempotentEnvelope)
	if idempotentEnvelope.Data.Revision != 1 {
		t.Fatalf("idempotent revision=%d, want 1", idempotentEnvelope.Data.Revision)
	}

	stale := performWatchlistRequest(t, router, http.MethodPut, "/api/v1/watchlist/instruments/US/AAPL/memberships", map[string]any{
		"groupIds": []string{}, "newGroupNames": []string{}, "expectedRevision": 0,
	})
	if stale.Code != http.StatusConflict {
		t.Fatalf("stale status=%d body=%s", stale.Code, stale.Body.String())
	}

	for _, instrumentID := range []string{"US.MSFT", "US.NVDA"} {
		if _, err := service.ReplaceMemberships(t.Context(), domain.ReplaceMembershipsInput{
			InstrumentID: instrumentID, GroupIDs: []string{defaultGroupID}, ExpectedRevision: 0,
		}); err != nil {
			t.Fatalf("seed %s: %v", instrumentID, err)
		}
	}
	pageOne := performWatchlistRequest(t, router, http.MethodGet, "/api/v1/watchlist/items?limit=2", nil)
	if pageOne.Code != http.StatusOK {
		t.Fatalf("page one status=%d body=%s", pageOne.Code, pageOne.Body.String())
	}
	var pageOneEnvelope struct {
		httpserver.Envelope
		Data domain.ItemPage `json:"data"`
	}
	decodeWatchlistResponse(t, pageOne, &pageOneEnvelope)
	if len(pageOneEnvelope.Data.Items) != 2 || pageOneEnvelope.Data.NextCursor == "" {
		t.Fatalf("page one=%#v", pageOneEnvelope.Data)
	}
	pageTwo := performWatchlistRequest(t, router, http.MethodGet, "/api/v1/watchlist/items?limit=2&cursor="+pageOneEnvelope.Data.NextCursor, nil)
	var pageTwoEnvelope struct {
		httpserver.Envelope
		Data domain.ItemPage `json:"data"`
	}
	decodeWatchlistResponse(t, pageTwo, &pageTwoEnvelope)
	if pageTwo.Code != http.StatusOK || len(pageTwoEnvelope.Data.Items) != 1 {
		t.Fatalf("page two status=%d data=%#v", pageTwo.Code, pageTwoEnvelope.Data)
	}
}

func TestWatchlistRoutesMapValidationNotFoundAndProtectedConflicts(t *testing.T) {
	service, router := newWatchlistAPITest(t)
	groups, err := service.ListGroups(t.Context())
	if err != nil || len(groups) != 1 {
		t.Fatalf("groups=%#v err=%v", groups, err)
	}

	badInstrument := performWatchlistRequest(t, router, http.MethodGet, "/api/v1/watchlist/instruments/BAD/AAPL/memberships", nil)
	if badInstrument.Code != http.StatusBadRequest {
		t.Fatalf("bad instrument status=%d body=%s", badInstrument.Code, badInstrument.Body.String())
	}
	missing := performWatchlistRequest(t, router, http.MethodDelete, "/api/v1/watchlist/groups/missing", nil)
	if missing.Code != http.StatusNotFound {
		t.Fatalf("missing group status=%d body=%s", missing.Code, missing.Body.String())
	}
	protected := performWatchlistRequest(t, router, http.MethodDelete, "/api/v1/watchlist/groups/"+groups[0].ID, nil)
	if protected.Code != http.StatusConflict {
		t.Fatalf("protected group status=%d body=%s", protected.Code, protected.Body.String())
	}
}

func TestWatchlistRoutesCompleteImportAndGroupLifecycle(t *testing.T) {
	service, router := newWatchlistAPITest(t)
	reader := &apiWatchlistSource{
		groups: []domain.RemoteGroup{{RemoteGroupID: "remote-core", Name: "Remote Core", Type: "custom"}},
		members: map[string][]domain.RemoteMember{
			"remote-core": {{InstrumentID: "US.AAPL", Name: "Apple"}, {InstrumentID: "US.MSFT", Name: "Microsoft"}},
		},
	}
	service.RegisterSourceReader("futu:default", reader)
	service.RegisterBatchSnapshotSource(apiWatchlistQuotes{})

	createdResponse := performWatchlistRequest(t, router, http.MethodPost, "/api/v1/watchlist/groups", map[string]any{"name": "Imported"})
	if createdResponse.Code != http.StatusOK {
		t.Fatalf("create group status=%d body=%s", createdResponse.Code, createdResponse.Body.String())
	}
	var createdEnvelope struct {
		httpserver.Envelope
		Data domain.Group `json:"data"`
	}
	decodeWatchlistResponse(t, createdResponse, &createdEnvelope)
	group := createdEnvelope.Data
	if group.ID == "" || group.Name != "Imported" || group.Revision < 1 {
		t.Fatalf("created group = %#v", group)
	}

	updatedResponse := performWatchlistRequest(t, router, http.MethodPatch, "/api/v1/watchlist/groups/"+group.ID, map[string]any{
		"name": "Imported Core", "expectedRevision": group.Revision,
	})
	if updatedResponse.Code != http.StatusOK {
		t.Fatalf("update group status=%d body=%s", updatedResponse.Code, updatedResponse.Body.String())
	}
	var updatedEnvelope struct {
		httpserver.Envelope
		Data domain.Group `json:"data"`
	}
	decodeWatchlistResponse(t, updatedResponse, &updatedEnvelope)
	if updatedEnvelope.Data.Name != "Imported Core" || updatedEnvelope.Data.Revision <= group.Revision {
		t.Fatalf("updated group = %#v", updatedEnvelope.Data)
	}

	for _, endpoint := range []string{
		"/api/v1/watchlist/sources",
		"/api/v1/watchlist/sources/futu:default/groups",
	} {
		response := performWatchlistRequest(t, router, http.MethodGet, endpoint, nil)
		if response.Code != http.StatusOK {
			t.Fatalf("GET %s status=%d body=%s", endpoint, response.Code, response.Body.String())
		}
	}

	previewResponse := performWatchlistRequest(t, router, http.MethodPost, "/api/v1/watchlist/imports/preview", map[string]any{
		"sourceId": "futu:default", "remoteGroupId": "remote-core", "localGroupId": group.ID,
	})
	if previewResponse.Code != http.StatusOK {
		t.Fatalf("preview status=%d body=%s", previewResponse.Code, previewResponse.Body.String())
	}
	var previewEnvelope struct {
		httpserver.Envelope
		Data domain.ImportPreview `json:"data"`
	}
	decodeWatchlistResponse(t, previewResponse, &previewEnvelope)
	if previewEnvelope.Data.ID == "" || len(previewEnvelope.Data.Added) != 2 {
		t.Fatalf("preview = %#v", previewEnvelope.Data)
	}

	commitResponse := performWatchlistRawRequest(router, http.MethodPost, "/api/v1/watchlist/imports/"+previewEnvelope.Data.ID+"/commit", nil)
	if commitResponse.Code != http.StatusOK {
		t.Fatalf("commit status=%d body=%s", commitResponse.Code, commitResponse.Body.String())
	}
	var runEnvelope struct {
		httpserver.Envelope
		Data domain.ImportRun `json:"data"`
	}
	decodeWatchlistResponse(t, commitResponse, &runEnvelope)
	if runEnvelope.Data.ID == "" || runEnvelope.Data.AddedCount != 2 || runEnvelope.Data.LocalGroupID != group.ID {
		t.Fatalf("import run = %#v", runEnvelope.Data)
	}

	bindingsResponse := performWatchlistRequest(t, router, http.MethodGet, "/api/v1/watchlist/bindings?sourceId=futu:default", nil)
	if bindingsResponse.Code != http.StatusOK {
		t.Fatalf("bindings status=%d body=%s", bindingsResponse.Code, bindingsResponse.Body.String())
	}
	var bindingsEnvelope struct {
		httpserver.Envelope
		Data struct {
			Bindings []domain.Binding `json:"bindings"`
		} `json:"data"`
	}
	decodeWatchlistResponse(t, bindingsResponse, &bindingsEnvelope)
	if len(bindingsEnvelope.Data.Bindings) != 1 {
		t.Fatalf("bindings = %#v", bindingsEnvelope.Data.Bindings)
	}

	for _, endpoint := range []string{
		"/api/v1/watchlist/import-runs?sourceId=futu:default&limit=10",
		"/api/v1/watchlist/instruments/US/AAPL/memberships",
	} {
		response := performWatchlistRequest(t, router, http.MethodGet, endpoint, nil)
		if response.Code != http.StatusOK {
			t.Fatalf("GET %s status=%d body=%s", endpoint, response.Code, response.Body.String())
		}
	}
	quotesResponse := performWatchlistRequest(t, router, http.MethodPost, "/api/v1/watchlist/quotes/batch", map[string]any{
		"instrumentIds": []string{"US.AAPL", "US.MSFT"},
	})
	if quotesResponse.Code != http.StatusOK {
		t.Fatalf("quotes status=%d body=%s", quotesResponse.Code, quotesResponse.Body.String())
	}

	deleteBindingResponse := performWatchlistRequest(t, router, http.MethodDelete, "/api/v1/watchlist/bindings?bindingId="+bindingsEnvelope.Data.Bindings[0].ID, nil)
	if deleteBindingResponse.Code != http.StatusOK {
		t.Fatalf("delete binding status=%d body=%s", deleteBindingResponse.Code, deleteBindingResponse.Body.String())
	}
	deleteGroupResponse := performWatchlistRequest(t, router, http.MethodDelete, "/api/v1/watchlist/groups/"+group.ID, nil)
	if deleteGroupResponse.Code != http.StatusOK {
		t.Fatalf("delete group status=%d body=%s", deleteGroupResponse.Code, deleteGroupResponse.Body.String())
	}
}

func TestWatchlistRoutesRejectMalformedBodiesAndPageLimits(t *testing.T) {
	service, router := newWatchlistAPITest(t)
	groups, err := service.ListGroups(t.Context())
	if err != nil || len(groups) == 0 {
		t.Fatalf("groups=%#v err=%v", groups, err)
	}
	tests := []struct {
		method string
		path   string
		body   []byte
	}{
		{method: http.MethodPost, path: "/api/v1/watchlist/groups", body: []byte("{")},
		{method: http.MethodPatch, path: "/api/v1/watchlist/groups/" + groups[0].ID, body: []byte("{")},
		{method: http.MethodPut, path: "/api/v1/watchlist/instruments/US/AAPL/memberships", body: []byte("{")},
		{method: http.MethodPost, path: "/api/v1/watchlist/quotes/batch", body: []byte("{")},
		{method: http.MethodPost, path: "/api/v1/watchlist/imports/preview", body: []byte("{")},
		{method: http.MethodPost, path: "/api/v1/watchlist/imports/preview-id/commit", body: []byte("{")},
		{method: http.MethodGet, path: "/api/v1/watchlist/items?limit=0"},
		{method: http.MethodGet, path: "/api/v1/watchlist/import-runs?limit=0"},
	}
	for _, test := range tests {
		response := performWatchlistRawRequest(router, test.method, test.path, test.body)
		if response.Code != http.StatusBadRequest {
			t.Errorf("%s %s status=%d body=%s", test.method, test.path, response.Code, response.Body.String())
		}
	}
}

func newWatchlistAPITest(t *testing.T) (*domain.Service, *gin.Engine) {
	t.Helper()
	repository, err := storewatchlist.Open(t.Context(), filepath.Join(t.TempDir(), "watchlists.db"))
	if err != nil {
		t.Fatalf("open watchlist store: %v", err)
	}
	t.Cleanup(func() {
		if err := repository.Close(); err != nil {
			t.Errorf("close watchlist store: %v", err)
		}
	})
	service := domain.NewService(repository)
	gin.SetMode(gin.TestMode)
	router := gin.New()
	RegisterRoutes(router.Group("/api/v1"), service)
	return service, router
}

func performWatchlistRequest(t *testing.T, router http.Handler, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var raw []byte
	if body != nil {
		var err error
		raw, err = json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal request: %v", err)
		}
	}
	request := httptest.NewRequest(method, path, bytes.NewReader(raw))
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	return response
}

func performWatchlistRawRequest(router http.Handler, method, path string, body []byte) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, path, bytes.NewReader(body))
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	return response
}

func decodeWatchlistResponse(t *testing.T, response *httptest.ResponseRecorder, target any) {
	t.Helper()
	if err := json.Unmarshal(response.Body.Bytes(), target); err != nil {
		t.Fatalf("decode response %s: %v", response.Body.String(), err)
	}
}
