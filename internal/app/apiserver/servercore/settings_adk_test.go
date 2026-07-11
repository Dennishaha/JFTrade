package servercore

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
)

func TestADKRuntimeSettingsDefaultAndSave(t *testing.T) {
	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	if _, err := store.SaveSecuritySettings(SecuritySettings{AdminAuthRequired: false}); err != nil {
		t.Fatalf("SaveSecuritySettings: %v", err)
	}
	server := NewServer(store)
	t.Cleanup(func() {
		jftradeErr1 := server.Close()
		jftradeCheckTestError(t, jftradeErr1)
	})
	srv := httptest.NewServer(server)
	t.Cleanup(srv.Close)

	resp, err := jftradeTestHTTPGet(t, srv.URL+"/api/v1/settings/adk")
	if err != nil {
		t.Fatalf("GET adk settings: %v", err)
	}
	defer func() { jftradeCheckTestError(t, resp.Body.Close()) }()
	var getEnvelope struct {
		OK   bool               `json:"ok"`
		Data ADKRuntimeSettings `json:"data"`
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

	body, jftradeErr1 := json.Marshal(ADKRuntimeSettings{
		RunTimeoutMs:        10_000,
		StreamIdleTimeoutMs: 2_000_000,
	})
	jftradeCheckTestError(t, jftradeErr1)
	req, err := http.NewRequestWithContext(t.Context(), http.MethodPut, srv.URL+"/api/v1/settings/adk", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("NewRequest adk settings: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	saveResp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PUT adk settings: %v", err)
	}
	defer func() { jftradeCheckTestError(t, saveResp.Body.Close()) }()
	var saveEnvelope struct {
		OK   bool               `json:"ok"`
		Data ADKRuntimeSettings `json:"data"`
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

	body, jftradeErr1 = json.Marshal(ADKRuntimeSettings{
		RunTimeoutMs:        99_999_999,
		StreamIdleTimeoutMs: 300_000,
	})
	jftradeCheckTestError(t, jftradeErr1)
	req, err = http.NewRequestWithContext(t.Context(), http.MethodPut, srv.URL+"/api/v1/settings/adk", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("NewRequest max adk settings: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	maxResp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PUT max adk settings: %v", err)
	}
	defer func() { jftradeCheckTestError(t, maxResp.Body.Close()) }()
	var maxEnvelope struct {
		OK   bool               `json:"ok"`
		Data ADKRuntimeSettings `json:"data"`
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
