package jftradeapi

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
	server := NewServer(store)
	t.Cleanup(func() {
		_ = server.Close()
	})
	srv := httptest.NewServer(server)
	t.Cleanup(srv.Close)

	resp, err := http.Get(srv.URL + "/api/v1/settings/adk")
	if err != nil {
		t.Fatalf("GET adk settings: %v", err)
	}
	defer resp.Body.Close()
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
	if getEnvelope.Data.RunTimeoutMs != 600_000 || getEnvelope.Data.StreamIdleTimeoutMs != 300_000 {
		t.Fatalf("default ADK settings = %+v", getEnvelope.Data)
	}

	body, _ := json.Marshal(ADKRuntimeSettings{
		RunTimeoutMs:        10_000,
		StreamIdleTimeoutMs: 2_000_000,
	})
	req, err := http.NewRequest(http.MethodPut, srv.URL+"/api/v1/settings/adk", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("NewRequest adk settings: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	saveResp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PUT adk settings: %v", err)
	}
	defer saveResp.Body.Close()
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
}
