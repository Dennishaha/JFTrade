package rustmigration

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/jftrade/jftrade-main/internal/api/system"
	"github.com/jftrade/jftrade-main/internal/app/apiserver/futuapp"
	"github.com/jftrade/jftrade-main/internal/store/settingsfile"
	sysservice "github.com/jftrade/jftrade-main/internal/system"
)

type stage9SystemReadFixture struct {
	Version string                 `json:"version"`
	Cases   []stage9SystemReadCase `json:"cases"`
}

type stage9SystemReadCase struct {
	Name           string `json:"name"`
	Method         string `json:"method"`
	RequestPath    string `json:"requestPath"`
	ExpectedStatus int    `json:"expectedStatus"`
	Data           any    `json:"data"`
}

func TestStage9SystemReadFixtureMatchesCurrentGoOwner(t *testing.T) {
	_, source, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve stage 9 system read fixture source")
	}
	fixturePath := filepath.Join(
		filepath.Dir(source), "../../tests/fixtures/rust-migration/stage9/system-read.json",
	)
	settings, err := settingsfile.New(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("create settings fixture: %v", err)
	}
	coordinator := futuapp.New(futuapp.Options{Settings: settings})
	svc := sysservice.NewService(
		sysservice.WithBrokerRuntimeHealth(func(ctx context.Context) map[string]any {
			return coordinator.OpenDHealth(ctx)
		}),
		sysservice.WithBrokerOrderSnapshot(func() map[string]any { return map[string]any{} }),
	)
	gin.SetMode(gin.TestMode)
	router := gin.New()
	system.RegisterRoutes(router.Group("/api/v1"), svc)

	want := stage9SystemReadFixture{
		Version: "stage9.system-read.v1",
		Cases: []stage9SystemReadCase{
			{Name: "futu-opend", Method: http.MethodGet, RequestPath: "/api/v1/system/futu-opend"},
			{Name: "broker-order-updates", Method: http.MethodGet, RequestPath: "/api/v1/system/worker/broker-order-updates"},
		},
	}
	for index := range want.Cases {
		request := httptest.NewRequestWithContext(t.Context(), want.Cases[index].Method, want.Cases[index].RequestPath, nil)
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, request)
		var envelope map[string]any
		if err := json.Unmarshal(recorder.Body.Bytes(), &envelope); err != nil {
			t.Fatalf("decode %s response: %v", want.Cases[index].Name, err)
		}
		normalizeSystemReadTime(envelope)
		want.Cases[index].ExpectedStatus = recorder.Code
		want.Cases[index].Data = envelope["data"]
	}
	if os.Getenv("JFTRADE_UPDATE_RUST_MIGRATION_FIXTURES") == "1" {
		contents, err := json.MarshalIndent(want, "", "  ")
		if err != nil {
			t.Fatalf("encode system read fixture: %v", err)
		}
		if err := os.WriteFile(fixturePath, append(contents, '\n'), 0o644); err != nil {
			t.Fatalf("write system read fixture: %v", err)
		}
	}
	contents, err := os.ReadFile(fixturePath)
	if err != nil {
		t.Fatalf("read system read fixture: %v", err)
	}
	var got stage9SystemReadFixture
	if err := json.Unmarshal(contents, &got); err != nil {
		t.Fatalf("decode system read fixture: %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("system read fixture drifted from Go owner")
	}
}

func normalizeSystemReadTime(value any) {
	if object, ok := value.(map[string]any); ok {
		for key, child := range object {
			if key == "timestamp" {
				object[key] = "fixture-time"
				continue
			}
			if key == "platform" {
				object[key] = "fixture-platform"
				continue
			}
			normalizeSystemReadTime(child)
		}
		return
	}
	if values, ok := value.([]any); ok {
		for _, child := range values {
			normalizeSystemReadTime(child)
		}
	}
}
