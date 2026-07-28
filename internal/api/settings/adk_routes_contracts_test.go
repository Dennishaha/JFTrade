package settings_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"
	apisettings "github.com/jftrade/jftrade-main/internal/api/settings"
	srvsettings "github.com/jftrade/jftrade-main/internal/settings"
	settingsfile "github.com/jftrade/jftrade-main/internal/store/settingsfile"
	jfsettings "github.com/jftrade/jftrade-main/pkg/jftsettings"
)

func TestADKRuntimeSettingsDefaultAndSave(t *testing.T) {
	store, err := settingsfile.New(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("settingsfile.New: %v", err)
	}
	service := srvsettings.NewService(store)
	router := gin.New()
	apisettings.RegisterRoutes(router.Group("/api/v1"), service)
	srv := httptest.NewServer(router)
	t.Cleanup(srv.Close)

	resp, err := http.Get(srv.URL + "/api/v1/settings/adk")
	if err != nil {
		t.Fatalf("GET adk settings: %v", err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			t.Errorf("close GET response body: %v", err)
		}
	}()
	var getEnvelope struct {
		OK   bool                          `json:"ok"`
		Data jfsettings.ADKRuntimeSettings `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&getEnvelope); err != nil {
		t.Fatalf("decode GET adk settings: %v", err)
	}
	if !getEnvelope.OK {
		t.Fatalf("GET envelope = %+v, want ok=true", getEnvelope)
	}
	if getEnvelope.Data.RunTimeoutMs != 1_800_000 || getEnvelope.Data.StreamIdleTimeoutMs != 300_000 {
		t.Fatalf("default ADK settings = %+v", getEnvelope.Data)
	}

	body, err := json.Marshal(jfsettings.ADKRuntimeSettings{
		RunTimeoutMs:        10_000,
		StreamIdleTimeoutMs: 2_000_000,
	})
	if err != nil {
		t.Fatalf("marshal ADK settings: %v", err)
	}
	req, err := http.NewRequestWithContext(t.Context(), http.MethodPut, srv.URL+"/api/v1/settings/adk", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("NewRequest adk settings: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	saveResp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PUT adk settings: %v", err)
	}
	defer func() {
		if err := saveResp.Body.Close(); err != nil {
			t.Errorf("close PUT response body: %v", err)
		}
	}()
	var saveEnvelope struct {
		OK   bool                          `json:"ok"`
		Data jfsettings.ADKRuntimeSettings `json:"data"`
	}
	if err := json.NewDecoder(saveResp.Body).Decode(&saveEnvelope); err != nil {
		t.Fatalf("decode PUT adk settings: %v", err)
	}
	if !saveEnvelope.OK {
		t.Fatalf("PUT envelope = %+v, want ok=true", saveEnvelope)
	}
	if saveEnvelope.Data.RunTimeoutMs != 60_000 || saveEnvelope.Data.StreamIdleTimeoutMs != 900_000 {
		t.Fatalf("normalized ADK settings = %+v", saveEnvelope.Data)
	}

	body, err = json.Marshal(jfsettings.ADKRuntimeSettings{
		RunTimeoutMs:        99_999_999,
		StreamIdleTimeoutMs: 300_000,
	})
	if err != nil {
		t.Fatalf("marshal max ADK settings: %v", err)
	}
	req, err = http.NewRequestWithContext(t.Context(), http.MethodPut, srv.URL+"/api/v1/settings/adk", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("NewRequest max adk settings: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	maxResp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PUT max adk settings: %v", err)
	}
	defer func() {
		if err := maxResp.Body.Close(); err != nil {
			t.Errorf("close max PUT response body: %v", err)
		}
	}()
	var maxEnvelope struct {
		OK   bool                          `json:"ok"`
		Data jfsettings.ADKRuntimeSettings `json:"data"`
	}
	if err := json.NewDecoder(maxResp.Body).Decode(&maxEnvelope); err != nil {
		t.Fatalf("decode max PUT adk settings: %v", err)
	}
	if !maxEnvelope.OK {
		t.Fatalf("max PUT envelope = %+v, want ok=true", maxEnvelope)
	}
	if maxEnvelope.Data.RunTimeoutMs != 43_200_000 || maxEnvelope.Data.StreamIdleTimeoutMs != 300_000 {
		t.Fatalf("max normalized ADK settings = %+v", maxEnvelope.Data)
	}
}
