package rustmigration

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	researchapi "github.com/jftrade/jftrade-main/internal/api/research"
	domain "github.com/jftrade/jftrade-main/internal/research"
	"github.com/jftrade/jftrade-main/pkg/broker"
	"github.com/jftrade/jftrade-main/pkg/researchscreen"
)

const stage9ResearchPresetReadFixtureVersion = "stage9.research-preset-read.v1"

type stage9ResearchPresetReadCase struct {
	Name           string          `json:"name"`
	Method         string          `json:"method"`
	RequestPath    string          `json:"requestPath"`
	ExpectedStatus int             `json:"expectedStatus"`
	Data           json.RawMessage `json:"data,omitempty"`
	ErrorCode      string          `json:"errorCode,omitempty"`
	ErrorMessage   string          `json:"errorMessage,omitempty"`
}

type stage9ResearchPresetReadFixture struct {
	Version string                         `json:"version"`
	Cases   []stage9ResearchPresetReadCase `json:"cases"`
}

// TestStage9ResearchPresetReadFixtureMatchesCurrentGoOwner freezes both
// read-only preset projections without opening the production SQLite store.
func TestStage9ResearchPresetReadFixtureMatchesCurrentGoOwner(t *testing.T) {
	_, source, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve stage 9 research preset fixture source")
	}
	fixturePath := filepath.Join(filepath.Dir(source), "../../tests/fixtures/rust-migration/stage9/research-preset-read.json")
	cases := []struct {
		name string
		path string
	}{
		{name: "list", path: "/api/v1/research/screens/presets"},
		{name: "get", path: "/api/v1/research/screens/presets/preset-value"},
		{name: "missing", path: "/api/v1/research/screens/presets/missing"},
	}
	want := stage9ResearchPresetReadFixture{
		Version: stage9ResearchPresetReadFixtureVersion,
		Cases:   make([]stage9ResearchPresetReadCase, 0, len(cases)),
	}
	for _, testCase := range cases {
		gin.SetMode(gin.TestMode)
		router := gin.New()
		researchapi.RegisterRoutes(router.Group("/api/v1"), domain.NewService(stage9ResearchPresetRepository{}))
		recorder := httptest.NewRecorder()
		request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, testCase.path, nil)
		router.ServeHTTP(recorder, request)
		entry := stage9ResearchPresetReadCase{
			Name: testCase.name, Method: http.MethodGet, RequestPath: testCase.path,
			ExpectedStatus: recorder.Code,
		}
		var envelope struct {
			Data  json.RawMessage `json:"data"`
			Error *struct {
				Code    string `json:"code"`
				Message string `json:"message"`
			} `json:"error"`
		}
		if err := json.Unmarshal(recorder.Body.Bytes(), &envelope); err != nil {
			t.Fatalf("decode %s response: %v", testCase.name, err)
		}
		if envelope.Error != nil {
			entry.ErrorCode, entry.ErrorMessage = envelope.Error.Code, envelope.Error.Message
		} else {
			entry.Data = compactResearchPresetReadJSON(envelope.Data)
		}
		want.Cases = append(want.Cases, entry)
	}
	if os.Getenv("JFTRADE_UPDATE_RUST_MIGRATION_FIXTURES") == "1" {
		contents, err := json.MarshalIndent(want, "", "  ")
		if err != nil {
			t.Fatalf("encode research preset fixture: %v", err)
		}
		if err := os.WriteFile(fixturePath, append(contents, '\n'), 0o644); err != nil {
			t.Fatalf("write research preset fixture: %v", err)
		}
	}
	contents, err := os.ReadFile(fixturePath)
	if err != nil {
		t.Fatalf("read research preset fixture: %v", err)
	}
	var got stage9ResearchPresetReadFixture
	if err := json.Unmarshal(contents, &got); err != nil {
		t.Fatalf("decode research preset fixture: %v", err)
	}
	for index := range got.Cases {
		got.Cases[index].Data = compactResearchPresetReadJSON(got.Cases[index].Data)
		want.Cases[index].Data = compactResearchPresetReadJSON(want.Cases[index].Data)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("stage 9 research preset read fixture drifted from the Go owner: got=%#v want=%#v", got, want)
	}
}

func compactResearchPresetReadJSON(data json.RawMessage) json.RawMessage {
	if len(data) == 0 {
		return data
	}
	var value any
	if err := json.Unmarshal(data, &value); err != nil {
		return data
	}
	contents, err := json.Marshal(value)
	if err != nil {
		return data
	}
	return contents
}

type stage9ResearchPresetRepository struct{}

func (stage9ResearchPresetRepository) ListScreenPresets(context.Context) ([]domain.ScreenPreset, error) {
	return []domain.ScreenPreset{stage9ResearchPreset("preset-value", "Value", 1)}, nil
}

func (stage9ResearchPresetRepository) GetScreenPreset(_ context.Context, id string) (domain.ScreenPreset, error) {
	if id != "preset-value" {
		return domain.ScreenPreset{}, domain.ErrNotFound
	}
	return stage9ResearchPreset(id, "Value", 1), nil
}

func (stage9ResearchPresetRepository) CreateScreenPreset(context.Context, string, broker.ScreenDefinitionV2, int) (domain.ScreenPreset, error) {
	return domain.ScreenPreset{}, errors.New("create is not part of the read fixture")
}

func (stage9ResearchPresetRepository) UpdateScreenPreset(context.Context, string, string, broker.ScreenDefinitionV2, int, int64) (domain.ScreenPreset, error) {
	return domain.ScreenPreset{}, errors.New("update is not part of the read fixture")
}

func (stage9ResearchPresetRepository) DeleteScreenPreset(context.Context, string) error {
	return errors.New("delete is not part of the read fixture")
}

func stage9ResearchPreset(id, name string, revision int64) domain.ScreenPreset {
	return domain.ScreenPreset{
		ID: id, Name: name, QuerySchemaVersion: broker.ScreenQuerySchemaVersionV2,
		Definition: broker.ScreenDefinitionV2{
			BrokerID: "futu", Market: "US", Pool: broker.ResearchScreenPool{},
			Columns:            []broker.ScreenColumn{{ID: "price", Factor: broker.FactorRef{InstanceID: "price", FactorKey: "simple.price"}}},
			CatalogVersion:     researchscreen.CatalogVersion,
			QuerySchemaVersion: broker.ScreenQuerySchemaVersionV2,
		},
		Revision:  revision,
		CreatedAt: time.Date(2026, 8, 15, 20, 0, 0, 0, time.UTC),
		UpdatedAt: time.Date(2026, 8, 15, 20, 1, 0, 0, time.UTC),
	}
}
