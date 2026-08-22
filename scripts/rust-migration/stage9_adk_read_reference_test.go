package rustmigration

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	assistantapi "github.com/jftrade/jftrade-main/internal/api/assistant"
	assistantservice "github.com/jftrade/jftrade-main/internal/assistant"
	assistanttestkit "github.com/jftrade/jftrade-main/internal/assistant/testkit"
	adksession "google.golang.org/adk/v2/session"
)

const stage9ADKReadFixtureVersion = "stage9.adk-read.v1"

type stage9ADKReadCase struct {
	Name           string          `json:"name"`
	Method         string          `json:"method"`
	RequestPath    string          `json:"requestPath"`
	ExpectedStatus int             `json:"expectedStatus"`
	Data           json.RawMessage `json:"data,omitempty"`
	ErrorCode      string          `json:"errorCode,omitempty"`
	ErrorMessage   string          `json:"errorMessage,omitempty"`
}

type stage9ADKReadFixture struct {
	Version string              `json:"version"`
	Cases   []stage9ADKReadCase `json:"cases"`
}

// TestStage9ADKReadFixtureMatchesCurrentGoOwner freezes the read-only ADK
// transport projection. The fixture uses a fresh local store and never starts
// a Provider, executes a run, or opens the production Assistant runtime.
func TestStage9ADKReadFixtureMatchesCurrentGoOwner(t *testing.T) {
	_, source, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve stage 9 ADK fixture source")
	}
	fixturePath := filepath.Join(
		filepath.Dir(source),
		"../../tests/fixtures/rust-migration/stage9/adk-read.json",
	)
	directory := t.TempDir()
	store, err := assistanttestkit.NewStore(
		filepath.Join(directory, "adk.db"),
		filepath.Join(directory, "secrets", "adk-secrets.json"),
		filepath.Join(directory, "skills"),
	)
	if err != nil {
		t.Fatalf("open ADK fixture store: %v", err)
	}
	runtime := assistanttestkit.NewRuntimeWithSessionService(
		store,
		assistanttestkit.NewToolRegistry(),
		adksession.InMemoryService(),
	)
	service := assistantservice.NewService(runtime)
	router := gin.New()
	handler := assistantapi.RegisterRoutes(router.Group("/api/v1"), service)
	t.Cleanup(func() {
		if err := handler.Close(); err != nil {
			t.Errorf("close ADK handler: %v", err)
		}
		if err := service.Close(); err != nil {
			t.Errorf("close ADK service: %v", err)
		}
	})

	cases := []struct {
		name string
		path string
	}{
		{name: "snapshot", path: "/api/v1/adk"},
		{name: "agents", path: "/api/v1/adk/agents"},
		{name: "agents-page", path: "/api/v1/adk/agents?limit=1&offset=0"},
		{name: "approvals", path: "/api/v1/adk/approvals"},
		{name: "audit", path: "/api/v1/adk/audit"},
		{name: "memory", path: "/api/v1/adk/memory"},
		{name: "metrics", path: "/api/v1/adk/metrics"},
		{name: "optimization-tasks", path: "/api/v1/adk/optimization-tasks"},
		{name: "optimization-tasks-page", path: "/api/v1/adk/optimization-tasks?limit=1&offset=0"},
		{name: "providers", path: "/api/v1/adk/providers"},
		{name: "runs", path: "/api/v1/adk/runs"},
		{name: "sessions", path: "/api/v1/adk/sessions"},
		{name: "skills", path: "/api/v1/adk/skills"},
		{name: "tasks", path: "/api/v1/adk/tasks"},
		{name: "tools", path: "/api/v1/adk/tools"},
		{name: "workflow-trigger-logs", path: "/api/v1/adk/workflow-trigger-logs"},
		{name: "workflows", path: "/api/v1/adk/workflows"},
		{name: "task-query-error", path: "/api/v1/adk/tasks?%zz"},
		{name: "memory-query-error", path: "/api/v1/adk/memory?%zz"},
		{name: "agents-query-error", path: "/api/v1/adk/agents?%zz"},
		{name: "workflow-query-error", path: "/api/v1/adk/workflows?%zz"},
		{name: "workflow-log-query-error", path: "/api/v1/adk/workflow-trigger-logs?%zz"},
		{name: "audit-query-error", path: "/api/v1/adk/audit?%zz"},
		{name: "optimization-query-error", path: "/api/v1/adk/optimization-tasks?%zz"},
		{name: "sessions-query-error", path: "/api/v1/adk/sessions?%zz"},
		{name: "runs-query-error", path: "/api/v1/adk/runs?%zz"},
		{name: "approvals-query-error", path: "/api/v1/adk/approvals?%zz"},
		{name: "optimization-task-missing", path: "/api/v1/adk/optimization-tasks/missing"},
		{name: "optimization-task-blank", path: "/api/v1/adk/optimization-tasks/%20"},
		{name: "run-missing", path: "/api/v1/adk/runs/missing"},
		{name: "run-blank", path: "/api/v1/adk/runs/%20"},
		{name: "run-stream-missing", path: "/api/v1/adk/runs/missing/stream"},
		{name: "run-stream-blank", path: "/api/v1/adk/runs/%20/stream"},
		{name: "session-missing", path: "/api/v1/adk/sessions/missing"},
		{name: "session-blank", path: "/api/v1/adk/sessions/%20"},
		{name: "session-context-missing", path: "/api/v1/adk/sessions/missing/context"},
		{name: "session-context-blank", path: "/api/v1/adk/sessions/%20/context"},
		{name: "stream-missing", path: "/api/v1/adk/streams/missing"},
		{name: "stream-blank", path: "/api/v1/adk/streams/%20"},
		{name: "task-missing", path: "/api/v1/adk/tasks/missing"},
		{name: "task-blank", path: "/api/v1/adk/tasks/%20"},
		{name: "workflow-missing", path: "/api/v1/adk/workflows/missing"},
		{name: "workflow-blank", path: "/api/v1/adk/workflows/%20"},
		{name: "workflow-triggers-missing", path: "/api/v1/adk/workflows/missing/triggers"},
		{name: "workflow-triggers-blank", path: "/api/v1/adk/workflows/%20/triggers"},
	}
	want := stage9ADKReadFixture{Version: stage9ADKReadFixtureVersion}
	for _, testCase := range cases {
		entry := stage9ADKReadCase{
			Name:        testCase.name,
			Method:      http.MethodGet,
			RequestPath: testCase.path,
		}
		recorder := httptest.NewRecorder()
		requestPath, rawQuery, _ := strings.Cut(testCase.path, "?")
		request := httptest.NewRequestWithContext(context.Background(), http.MethodGet, requestPath, nil)
		request.URL.RawQuery = rawQuery
		router.ServeHTTP(recorder, request)
		entry.ExpectedStatus = recorder.Code
		var envelope struct {
			Data  json.RawMessage `json:"data"`
			Error *struct {
				Code    string `json:"code"`
				Message string `json:"message"`
			} `json:"error"`
		}
		if err := json.Unmarshal(recorder.Body.Bytes(), &envelope); err != nil {
			t.Fatalf("decode %s response: %v (%s)", testCase.name, err, recorder.Body.String())
		}
		if envelope.Error != nil {
			entry.ErrorCode = envelope.Error.Code
			entry.ErrorMessage = envelope.Error.Message
		} else {
			entry.Data = normalizeADKReadData(envelope.Data)
		}
		want.Cases = append(want.Cases, entry)
	}

	if os.Getenv("JFTRADE_UPDATE_RUST_MIGRATION_FIXTURES") == "1" {
		contents, err := json.MarshalIndent(want, "", "  ")
		if err != nil {
			t.Fatalf("encode ADK fixture: %v", err)
		}
		if err := os.WriteFile(fixturePath, append(contents, '\n'), 0o644); err != nil {
			t.Fatalf("write ADK fixture: %v", err)
		}
	}
	contents, err := os.ReadFile(fixturePath)
	if err != nil {
		t.Fatalf("read ADK fixture: %v", err)
	}
	var got stage9ADKReadFixture
	if err := json.Unmarshal(contents, &got); err != nil {
		t.Fatalf("decode ADK fixture: %v", err)
	}
	for index := range got.Cases {
		got.Cases[index].Data = compactADKReadJSON(got.Cases[index].Data)
		want.Cases[index].Data = compactADKReadJSON(want.Cases[index].Data)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("stage 9 ADK read fixture drifted from the Go owner")
	}
}

func normalizeADKReadData(data json.RawMessage) json.RawMessage {
	if len(data) == 0 {
		return nil
	}
	var value any
	if err := json.Unmarshal(data, &value); err != nil {
		return data
	}
	value = normalizeADKReadValue("", value)
	contents, err := json.Marshal(value)
	if err != nil {
		return data
	}
	return contents
}

func normalizeADKReadValue(key string, value any) any {
	if key == "installPath" {
		if _, ok := value.(string); ok {
			return "fixture-skill-path"
		}
	}
	if isADKReadTimestampKey(key) {
		if _, ok := value.(string); ok {
			return "fixture-time"
		}
	}
	switch typed := value.(type) {
	case []any:
		for index := range typed {
			typed[index] = normalizeADKReadValue(key, typed[index])
		}
	case map[string]any:
		for childKey, childValue := range typed {
			typed[childKey] = normalizeADKReadValue(childKey, childValue)
		}
	}
	return value
}

func isADKReadTimestampKey(key string) bool {
	switch key {
	case "createdAt", "updatedAt", "checkedAt", "startedAt", "finishedAt", "completedAt", "cancelledAt", "nextRunAt", "lastRunAt", "contextRevisionCreatedAt", "lastCompactedAt", "since":
		return true
	default:
		return false
	}
}

func compactADKReadJSON(data json.RawMessage) json.RawMessage {
	if len(data) == 0 {
		return nil
	}
	var compacted bytes.Buffer
	if err := json.Compact(&compacted, data); err != nil {
		return data
	}
	return compacted.Bytes()
}
