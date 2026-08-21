package rustmigration

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"

	"github.com/gin-gonic/gin"
	productfeatures "github.com/jftrade/jftrade-main/internal/api/productfeatures"
	service "github.com/jftrade/jftrade-main/internal/productfeatures"
	"github.com/jftrade/jftrade-main/pkg/broker"
)

func TestStage9WatchlistsReadFixtureMatchesCurrentGoOwner(t *testing.T) {
	_, source, _, _ := runtime.Caller(0)
	fixturePath := filepath.Join(filepath.Dir(source), "../../tests/fixtures/rust-migration/stage9/watchlists-read.json")
	gin.SetMode(gin.TestMode)
	router := gin.New()
	productfeatures.RegisterRoutes(router.Group("/api/v1"), service.NewService(broker.NewRegistry(), "", nil, nil))
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/v1/watchlists/remote", nil))
	var envelope any
	if err := json.Unmarshal(recorder.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	normalizeWatchlistsReadTime(envelope)
	want := map[string]any{"version": "stage9.watchlists-read.v1", "status": float64(recorder.Code), "response": envelope}
	if os.Getenv("JFTRADE_UPDATE_RUST_MIGRATION_FIXTURES") == "1" {
		contents, _ := json.MarshalIndent(want, "", "  ")
		if err := os.WriteFile(fixturePath, append(contents, '\n'), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	contents, err := os.ReadFile(fixturePath)
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(contents, &got); err != nil {
		t.Fatal(err)
	}
	normalizeWatchlistsReadTime(got["response"])
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("watchlists read fixture drifted from Go owner")
	}
}

func normalizeWatchlistsReadTime(value any) {
	if object, ok := value.(map[string]any); ok {
		for key, child := range object {
			if key == "timestamp" {
				object[key] = "fixture-time"
			} else {
				normalizeWatchlistsReadTime(child)
			}
		}
	}
}
