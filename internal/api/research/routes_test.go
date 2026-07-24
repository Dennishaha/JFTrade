package research

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/jftrade/jftrade-main/internal/api/httpserver"
	domain "github.com/jftrade/jftrade-main/internal/research"
	researchstore "github.com/jftrade/jftrade-main/internal/store/research"
)

func TestScreenPresetRoutesCRUDAndConflict(t *testing.T) {
	gin.SetMode(gin.TestMode)
	store, err := researchstore.Open(t.Context(), filepath.Join(t.TempDir(), "research.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	router := gin.New()
	RegisterRoutes(router.Group("/api/v1"), domain.NewService(store))

	created := performRequest(t, router, http.MethodPost, "/api/v1/research/screens/presets",
		`{"name":"价值筛选","definition":{"brokerId":"futu","market":"US","catalogVersion":"futu-stock-screen-v1","querySchemaVersion":2,"columns":[{"columnId":"price","factor":{"instanceId":"price","factorKey":"simple.price"}}]}}`)
	if created.Code != http.StatusOK {
		t.Fatalf("POST status=%d body=%s", created.Code, created.Body.String())
	}
	var createEnvelope struct {
		Data domain.ScreenPreset `json:"data"`
	}
	if err := json.Unmarshal(created.Body.Bytes(), &createEnvelope); err != nil {
		t.Fatalf("decode create: %v", err)
	}
	preset := createEnvelope.Data
	if preset.ID == "" || preset.Revision != 1 {
		t.Fatalf("created preset = %#v", preset)
	}

	list := performRequest(t, router, http.MethodGet, "/api/v1/research/screens/presets", "")
	if list.Code != http.StatusOK || !bytes.Contains(list.Body.Bytes(), []byte(preset.ID)) {
		t.Fatalf("GET list status=%d body=%s", list.Code, list.Body.String())
	}
	get := performRequest(t, router, http.MethodGet, "/api/v1/research/screens/presets/"+preset.ID, "")
	if get.Code != http.StatusOK {
		t.Fatalf("GET preset status=%d body=%s", get.Code, get.Body.String())
	}

	updated := performRequest(t, router, http.MethodPatch, "/api/v1/research/screens/presets/"+preset.ID,
		`{"name":"港股价值","definition":{"brokerId":"futu","market":"HK","catalogVersion":"futu-stock-screen-v1","querySchemaVersion":2,"columns":[{"columnId":"price","factor":{"instanceId":"price","factorKey":"simple.price"}}]},"expectedRevision":1}`)
	if updated.Code != http.StatusOK || !bytes.Contains(updated.Body.Bytes(), []byte(`"revision":2`)) {
		t.Fatalf("PATCH status=%d body=%s", updated.Code, updated.Body.String())
	}
	stale := performRequest(t, router, http.MethodPatch, "/api/v1/research/screens/presets/"+preset.ID,
		`{"name":"过期","expectedRevision":1}`)
	if stale.Code != http.StatusConflict {
		t.Fatalf("stale PATCH status=%d body=%s", stale.Code, stale.Body.String())
	}

	deleted := performRequest(t, router, http.MethodDelete, "/api/v1/research/screens/presets/"+preset.ID, "")
	if deleted.Code != http.StatusOK {
		t.Fatalf("DELETE status=%d body=%s", deleted.Code, deleted.Body.String())
	}
	missing := performRequest(t, router, http.MethodGet, "/api/v1/research/screens/presets/"+preset.ID, "")
	if missing.Code != http.StatusNotFound {
		t.Fatalf("missing GET status=%d body=%s", missing.Code, missing.Body.String())
	}
}

func TestScreenPresetRoutesValidatePayloadAndUnavailableStore(t *testing.T) {
	gin.SetMode(gin.TestMode)
	unavailableRouter := gin.New()
	RegisterRoutes(unavailableRouter.Group("/api/v1"), nil)
	unavailable := performRequest(t, unavailableRouter, http.MethodGet, "/api/v1/research/screens/presets", "")
	if unavailable.Code != http.StatusServiceUnavailable {
		t.Fatalf("unavailable status=%d body=%s", unavailable.Code, unavailable.Body.String())
	}
	store, err := researchstore.Open(t.Context(), filepath.Join(t.TempDir(), "research.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	router := gin.New()
	RegisterRoutes(router.Group("/api/v1"), domain.NewService(store))
	invalid := performRequest(t, router, http.MethodPost, "/api/v1/research/screens/presets", `{"name":"missing query"}`)
	if invalid.Code != http.StatusBadRequest {
		t.Fatalf("invalid status=%d body=%s", invalid.Code, invalid.Body.String())
	}
	var envelope httpserver.Envelope
	if err := json.Unmarshal(invalid.Body.Bytes(), &envelope); err != nil || envelope.Error == nil || envelope.Error.Code != "RESEARCH_PRESET_INVALID" {
		t.Fatalf("invalid envelope=%#v err=%v", envelope, err)
	}
	v1 := performRequest(
		t, router, http.MethodPost, "/api/v1/research/screens/presets",
		`{"name":"V1","query":{"brokerId":"futu","market":"US"}}`,
	)
	if v1.Code != http.StatusBadRequest {
		t.Fatalf("V1 payload status=%d body=%s", v1.Code, v1.Body.String())
	}
}

func TestResearchOpenAPIDocumentationStubs(t *testing.T) {
	t.Parallel()

	documentListScreenPresets()
	documentCreateScreenPreset()
	documentGetScreenPreset()
	documentUpdateScreenPreset()
	documentDeleteScreenPreset()
}

func performRequest(t *testing.T, router http.Handler, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(method, path, bytes.NewBufferString(body))
	if body != "" {
		request.Header.Set("Content-Type", "application/json")
	}
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	return response
}
