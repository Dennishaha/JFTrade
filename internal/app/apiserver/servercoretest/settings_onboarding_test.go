package servercoretest

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/jftrade/jftrade-main/internal/app/apiserver/marketdataapp"
	apiruntime "github.com/jftrade/jftrade-main/internal/app/apiserver/runtime"
	"github.com/jftrade/jftrade-main/internal/app/apiserver/servercore"
	jfsettings "github.com/jftrade/jftrade-main/internal/jftsettings"
)

func TestOnboardingDefaultsAndSave(t *testing.T) {
	settingsPath := filepath.Join(t.TempDir(), "settings.json")
	store, err := servercore.NewSettingsStore(settingsPath)
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}

	initial := store.Onboarding()
	if initial.Completed {
		t.Fatalf("default onboarding completed = true")
	}
	if initial.LastBrokerID != "" {
		t.Fatalf("default lastBrokerId = %q", initial.LastBrokerID)
	}

	saved, err := store.SaveOnboarding(jfsettings.OnboardingSettings{
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
		Onboarding jfsettings.OnboardingSettings `json:"onboarding"`
	}
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("Unmarshal settings: %v", err)
	}
	if !decoded.Onboarding.Completed || decoded.Onboarding.LastBrokerID != "futu" {
		t.Fatalf("persisted onboarding = %+v", decoded.Onboarding)
	}
}

func TestOnboardingRoutesSuggestOobeUntilCompleted(t *testing.T) {
	t.Cleanup(apiruntime.OverrideDependencyProbe(
		func(path string) (string, error) { return path, nil },
		func(context.Context, string, ...string) ([]byte, error) { return []byte("v22.0.0"), nil },
	))
	helper := filepath.Join(t.TempDir(), "yfinance-helper")
	if err := os.WriteFile(helper, []byte("helper"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv(marketdataapp.EnvYFinanceSidecar, helper)
	store, err := servercore.NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	srv := newHTTPTestServer(t, store)

	resp, err := jftradeTestHTTPGet(t, srv.URL+"/api/v1/settings/onboarding")
	if err != nil {
		t.Fatalf("GET onboarding: %v", err)
	}
	defer func() { jftradeCheckTestError(t, resp.Body.Close()) }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET onboarding status = %d", resp.StatusCode)
	}

	var getEnvelope struct {
		OK   bool `json:"ok"`
		Data struct {
			State          jfsettings.OnboardingSettings `json:"state"`
			ShouldShowOobe bool                          `json:"shouldShowOobe"`
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

	body, jftradeErr1 := json.Marshal(map[string]any{
		"completed":    true,
		"dismissed":    true,
		"lastBrokerId": "futu",
	})
	jftradeCheckTestError(t, jftradeErr1)
	req, err := http.NewRequestWithContext(t.Context(), http.MethodPut, srv.URL+"/api/v1/settings/onboarding", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("NewRequest PUT onboarding: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PUT onboarding: %v", err)
	}
	defer func() { jftradeCheckTestError(t, resp.Body.Close()) }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("PUT onboarding status = %d", resp.StatusCode)
	}

	var putEnvelope struct {
		OK   bool `json:"ok"`
		Data struct {
			State          jfsettings.OnboardingSettings `json:"state"`
			ShouldShowOobe bool                          `json:"shouldShowOobe"`
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

func TestOnboardingReopensWhenRuntimeDependencyFailsAfterCompletion(t *testing.T) {
	t.Cleanup(apiruntime.OverrideDependencyProbe(
		func(path string) (string, error) { return path, nil },
		func(context.Context, string, ...string) ([]byte, error) { return []byte("v20.0.0"), nil },
	))
	store, err := servercore.NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	if _, err := store.SaveOnboarding(jfsettings.OnboardingSettings{Completed: true, LastBrokerID: "futu"}); err != nil {
		t.Fatalf("SaveOnboarding: %v", err)
	}
	srv := newHTTPTestServer(t, store)

	resp, err := jftradeTestHTTPGet(t, srv.URL+"/api/v1/settings/onboarding")
	if err != nil {
		t.Fatalf("GET onboarding: %v", err)
	}
	defer func() { jftradeCheckTestError(t, resp.Body.Close()) }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET onboarding status = %d", resp.StatusCode)
	}
	var envelope struct {
		OK   bool `json:"ok"`
		Data struct {
			ShouldShowOobe bool `json:"shouldShowOobe"`
			Reasons        []struct {
				Code string `json:"code"`
			} `json:"reasons"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		t.Fatalf("decode GET onboarding: %v", err)
	}
	if !envelope.OK || !envelope.Data.ShouldShowOobe {
		t.Fatalf("expected OOBE suggestion after dependency failure: %+v", envelope.Data)
	}
	found := false
	for _, reason := range envelope.Data.Reasons {
		if reason.Code == "RUNTIME_DEPENDENCY_UNSATISFIED" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("dependency reason missing: %+v", envelope.Data.Reasons)
	}
}
