package rustmigration

import (
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"

	"github.com/jftrade/jftrade-main/internal/app/apiserver/servercore"
)

type stage9SettingsUIReadFixture struct {
	Version string                     `json:"version"`
	Cases   []stage9SettingsUIReadCase `json:"cases"`
}

type stage9SettingsUIReadCase struct {
	Name            string                      `json:"name"`
	Method          string                      `json:"method"`
	RequestPath     string                      `json:"requestPath"`
	RequestID       string                      `json:"requestId"`
	SeedDocument    map[string]any              `json:"seedDocument"`
	ExpectedStatus  int                         `json:"expectedStatus"`
	ExpectedHeaders stage9SettingsUIReadHeaders `json:"expectedHeaders"`
	Response        map[string]any              `json:"response"`
}

type stage9SettingsUIReadHeaders struct {
	ContentType  string `json:"contentType"`
	CacheControl string `json:"cacheControl"`
	RequestID    string `json:"requestId"`
}

func TestStage9SettingsUIReadFixtureMatchesCurrentGoOwner(t *testing.T) {
	_, source, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve stage 9 settings UI read fixture source")
	}
	fixturePath := filepath.Join(
		filepath.Dir(source), "../../tests/fixtures/rust-migration/stage9/settings-ui-read.json",
	)
	contents, err := os.ReadFile(fixturePath)
	if err != nil {
		t.Fatalf("read settings UI read fixture: %v", err)
	}
	var fixture stage9SettingsUIReadFixture
	if err := json.Unmarshal(contents, &fixture); err != nil {
		t.Fatalf("decode settings UI read fixture: %v", err)
	}
	if fixture.Version != "stage9.settings-ui-read.v1" || len(fixture.Cases) < 4 {
		t.Fatalf("settings UI read fixture is incomplete: version=%q cases=%d", fixture.Version, len(fixture.Cases))
	}

	for _, testCase := range fixture.Cases {
		t.Run(testCase.Name, func(t *testing.T) {
			settingsPath := filepath.Join(t.TempDir(), "settings.json")
			seed, err := json.Marshal(testCase.SeedDocument)
			if err != nil {
				t.Fatalf("encode settings seed: %v", err)
			}
			if err := os.WriteFile(settingsPath, seed, 0o600); err != nil {
				t.Fatalf("write settings seed: %v", err)
			}
			t.Setenv("JFTRADE_BACKTEST_DB", filepath.Join(filepath.Dir(settingsPath), "backtest.db"))

			store, err := servercore.NewSettingsStore(settingsPath)
			if err != nil {
				t.Fatalf("open Go settings owner: %v", err)
			}
			handler := servercore.NewSidecarHandlerWithOptions(store, servercore.SidecarOptions{
				DesktopMode: true,
			})
			t.Cleanup(func() {
				if err := handler.Close(); err != nil {
					t.Errorf("close Go settings owner: %v", err)
				}
			})

			request := httptest.NewRequestWithContext(t.Context(), testCase.Method, testCase.RequestPath, nil)
			request.Header.Set("X-Request-ID", testCase.RequestID)
			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, request)

			if recorder.Code != testCase.ExpectedStatus {
				t.Fatalf("status = %d, want %d; body=%s", recorder.Code, testCase.ExpectedStatus, recorder.Body.String())
			}
			if got := recorder.Header().Get("Content-Type"); got != testCase.ExpectedHeaders.ContentType {
				t.Fatalf("Content-Type = %q, want %q", got, testCase.ExpectedHeaders.ContentType)
			}
			if got := recorder.Header().Get("Cache-Control"); got != testCase.ExpectedHeaders.CacheControl {
				t.Fatalf("Cache-Control = %q, want %q", got, testCase.ExpectedHeaders.CacheControl)
			}
			if got := recorder.Header().Get("X-Request-ID"); got != testCase.ExpectedHeaders.RequestID {
				t.Fatalf("X-Request-ID = %q, want %q", got, testCase.ExpectedHeaders.RequestID)
			}

			var response map[string]any
			if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
				t.Fatalf("decode Go settings UI response: %v; body=%s", err, recorder.Body.String())
			}
			response["timestamp"] = "fixture-time"
			if !reflect.DeepEqual(response, testCase.Response) {
				got, _ := json.Marshal(response)
				want, _ := json.Marshal(testCase.Response)
				t.Fatalf("response = %s, want %s", got, want)
			}
		})
	}
}
