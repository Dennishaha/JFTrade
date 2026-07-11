package watchlist

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/jftrade/jftrade-main/internal/api/httpserver"
	storewatchlist "github.com/jftrade/jftrade-main/internal/store/watchlist"
	domain "github.com/jftrade/jftrade-main/internal/watchlist"
)

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

func decodeWatchlistResponse(t *testing.T, response *httptest.ResponseRecorder, target any) {
	t.Helper()
	if err := json.Unmarshal(response.Body.Bytes(), target); err != nil {
		t.Fatalf("decode response %s: %v", response.Body.String(), err)
	}
}
