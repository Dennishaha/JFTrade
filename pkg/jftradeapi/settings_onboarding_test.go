package jftradeapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestOnboardingDefaultsAndSave(t *testing.T) {
	settingsPath := filepath.Join(t.TempDir(), "settings.json")
	store, err := NewSettingsStore(settingsPath)
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}

	initial := store.onboarding()
	if initial.Completed {
		t.Fatalf("default onboarding completed = true")
	}
	if initial.LastBrokerID != "" {
		t.Fatalf("default lastBrokerId = %q", initial.LastBrokerID)
	}

	saved, err := store.saveOnboarding(OnboardingSettings{
		Completed:    true,
		CompletedAt:  "2026-06-03T00:00:00Z",
		DismissedAt:  "2026-06-03T00:00:01Z",
		LastBrokerID: "futu",
	})
	if err != nil {
		t.Fatalf("saveOnboarding: %v", err)
	}
	if !saved.Completed || saved.CompletedAt == "" || saved.DismissedAt == "" {
		t.Fatalf("saved onboarding = %+v", saved)
	}

	raw, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("ReadFile settings: %v", err)
	}
	var decoded struct {
		Onboarding OnboardingSettings `json:"onboarding"`
	}
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("Unmarshal settings: %v", err)
	}
	if !decoded.Onboarding.Completed || decoded.Onboarding.LastBrokerID != "futu" {
		t.Fatalf("persisted onboarding = %+v", decoded.Onboarding)
	}
}

func TestOnboardingRoutesSuggestOobeUntilCompleted(t *testing.T) {
	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	api := NewServer(store)
	defer api.Close()
	srv := httptest.NewServer(api)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api/v1/settings/onboarding")
	if err != nil {
		t.Fatalf("GET onboarding: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET onboarding status = %d", resp.StatusCode)
	}

	var getEnvelope struct {
		OK   bool `json:"ok"`
		Data struct {
			State          OnboardingSettings `json:"state"`
			ShouldShowOobe bool               `json:"shouldShowOobe"`
			Reasons        []struct {
				Code string `json:"code"`
			} `json:"reasons"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&getEnvelope); err != nil {
		t.Fatalf("decode GET onboarding: %v", err)
	}
	if !getEnvelope.OK || getEnvelope.Data.State.Completed {
		t.Fatalf("unexpected GET onboarding envelope: %+v", getEnvelope)
	}
	if !getEnvelope.Data.ShouldShowOobe || len(getEnvelope.Data.Reasons) == 0 {
		t.Fatalf("expected OOBE suggestion with reasons: %+v", getEnvelope.Data)
	}
	for _, reason := range getEnvelope.Data.Reasons {
		if reason.Code == "BROKER_DISCONNECTED" {
			t.Fatalf("onboarding GET should not probe OpenD before broker selection: %+v", getEnvelope.Data.Reasons)
		}
	}

	body, _ := json.Marshal(map[string]any{
		"completed":    true,
		"dismissed":    true,
		"lastBrokerId": "futu",
	})
	req, err := http.NewRequest(http.MethodPut, srv.URL+"/api/v1/settings/onboarding", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("NewRequest PUT onboarding: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PUT onboarding: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("PUT onboarding status = %d", resp.StatusCode)
	}

	var putEnvelope struct {
		OK   bool `json:"ok"`
		Data struct {
			State          OnboardingSettings `json:"state"`
			ShouldShowOobe bool               `json:"shouldShowOobe"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&putEnvelope); err != nil {
		t.Fatalf("decode PUT onboarding: %v", err)
	}
	if !putEnvelope.OK || !putEnvelope.Data.State.Completed || putEnvelope.Data.State.DismissedAt == "" {
		t.Fatalf("unexpected PUT onboarding envelope: %+v", putEnvelope)
	}
	if putEnvelope.Data.ShouldShowOobe {
		t.Fatalf("completed onboarding should not show OOBE: %+v", putEnvelope.Data)
	}
}
