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
	"github.com/google/uuid"
	assistantapi "github.com/jftrade/jftrade-main/internal/api/assistant"
	assistantservice "github.com/jftrade/jftrade-main/internal/assistant"
	assistantmodel "github.com/jftrade/jftrade-main/internal/assistant/model"
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

type stage9ADKReadSSECase struct {
	Name           string            `json:"name"`
	Method         string            `json:"method"`
	RequestPath    string            `json:"requestPath"`
	ExpectedStatus int               `json:"expectedStatus"`
	Headers        map[string]string `json:"headers"`
	Body           string            `json:"body"`
}

type stage9ADKReadSSEFixture struct {
	Version string                 `json:"version"`
	Cases   []stage9ADKReadSSECase `json:"cases"`
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

func TestStage9ADKReadSSEFixtureMatchesCurrentGoOwner(t *testing.T) {
	_, source, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve stage 9 ADK SSE fixture source")
	}
	fixturePath := filepath.Join(
		filepath.Dir(source),
		"../../tests/fixtures/rust-migration/stage9/adk-read-sse.json",
	)
	directory := t.TempDir()
	store, err := assistanttestkit.NewStore(
		filepath.Join(directory, "adk.db"),
		filepath.Join(directory, "secrets", "adk-secrets.json"),
		filepath.Join(directory, "skills"),
	)
	if err != nil {
		t.Fatalf("open ADK SSE fixture store: %v", err)
	}
	providerServer := httptest.NewServer(stage9ChatProviderHandler("success"))
	if _, err := store.SaveProvider(t.Context(), assistantmodel.ProviderWriteRequest{
		ID: "fixture-provider", DisplayName: "Fixture Provider", BaseURL: providerServer.URL,
		Model: "fixture-model", APIKey: "fixture-key", Enabled: true,
	}); err != nil {
		t.Fatalf("save ADK SSE fixture provider: %v", err)
	}
	if _, err := store.SaveAgent(t.Context(), assistantmodel.AgentWriteRequest{
		ID: "fixture-agent", Name: "Fixture Agent", ProviderID: "fixture-provider",
		Model: "fixture-model", PermissionMode: assistantmodel.PermissionModeLessApproval,
		Status: assistantmodel.AgentStatusEnabled,
	}); err != nil {
		t.Fatalf("save ADK SSE fixture agent: %v", err)
	}
	runtime := assistanttestkit.NewRuntimeWithSessionService(
		store,
		assistanttestkit.NewToolRegistry(),
		adksession.InMemoryService(),
	)
	service := assistantservice.NewService(
		runtime,
		assistantservice.WithStreamIdleTimeout(func() int { return 420000 }),
	)
	router := gin.New()
	handler := assistantapi.RegisterRoutes(router.Group("/api/v1"), service)
	t.Cleanup(func() {
		if err := handler.Close(); err != nil {
			t.Errorf("close ADK SSE fixture handler: %v", err)
		}
		if err := service.Close(); err != nil {
			t.Errorf("close ADK SSE fixture service: %v", err)
		}
		providerServer.Close()
	})

	streamRequest := `{"clientRequestId":"11111111-1111-4111-8111-111111111111","agentId":"fixture-agent","message":"fixture stream"}`
	stream := stage9ServeADKReadRequest(router, http.MethodPost, "/api/v1/adk/chat/stream", streamRequest)
	if stream.Code != http.StatusOK {
		t.Fatalf("seed ADK stream status=%d body=%s", stream.Code, stream.Body.String())
	}
	streamID := stream.Header().Get("X-ADK-Stream-ID")
	if streamID == "" {
		t.Fatal("seed ADK stream has no stream ID")
	}
	runs, err := store.ListRuns(t.Context())
	if err != nil || len(runs) == 0 {
		t.Fatalf("list ADK fixture runs=%v err=%v", runs, err)
	}
	runID := runs[len(runs)-1].ID
	if runID == "" {
		t.Fatal("seed ADK stream has no run ID")
	}

	paths := []struct {
		name string
		path string
	}{
		{name: "stream-after-zero", path: "/api/v1/adk/streams/" + streamID + "?after=0"},
		{name: "stream-after-one", path: "/api/v1/adk/streams/" + streamID + "?after=1"},
		{name: "run-stream-after-zero", path: "/api/v1/adk/runs/" + runID + "/stream?after=0"},
		{name: "run-stream-after-one", path: "/api/v1/adk/runs/" + runID + "/stream?after=1"},
	}
	want := stage9ADKReadSSEFixture{Version: "stage9.adk-read-sse.v1"}
	for _, testCase := range paths {
		response := stage9ServeADKReadRequest(router, http.MethodGet, testCase.path, "")
		if response.Code != http.StatusOK {
			t.Fatalf("%s status=%d body=%s", testCase.name, response.Code, response.Body.String())
		}
		want.Cases = append(want.Cases, stage9ADKReadSSECase{
			Name:           testCase.name,
			Method:         http.MethodGet,
			RequestPath:    normalizeADKReadSSEPath(testCase.path, streamID, runID),
			ExpectedStatus: response.Code,
			Headers:        normalizeADKReadSSEHeaders(response.Header()),
			Body:           normalizeADKReadSSEBody(response.Body.String(), streamID),
		})
	}

	if os.Getenv("JFTRADE_UPDATE_RUST_MIGRATION_FIXTURES") == "1" {
		contents, err := json.MarshalIndent(want, "", "  ")
		if err != nil {
			t.Fatalf("encode ADK SSE fixture: %v", err)
		}
		if err := os.WriteFile(fixturePath, append(contents, '\n'), 0o644); err != nil {
			t.Fatalf("write ADK SSE fixture: %v", err)
		}
	}
	contents, err := os.ReadFile(fixturePath)
	if err != nil {
		t.Fatalf("read ADK SSE fixture: %v", err)
	}
	var got stage9ADKReadSSEFixture
	if err := json.Unmarshal(contents, &got); err != nil {
		t.Fatalf("decode ADK SSE fixture: %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("stage 9 ADK SSE fixture drifted from the Go owner")
	}
}

func stage9ServeADKReadRequest(router http.Handler, method string, path string, body string) *httptest.ResponseRecorder {
	request := httptest.NewRequestWithContext(context.Background(), method, path, strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	return recorder
}

func normalizeADKReadSSEPath(path string, streamID string, runID string) string {
	path = strings.Replace(path, streamID, "stream-fixture", 1)
	return strings.Replace(path, runID, "run-fixture", 1)
}

func normalizeADKReadSSEHeaders(headers http.Header) map[string]string {
	want := make(map[string]string)
	for _, key := range []string{"Content-Type", "Cache-Control", "Connection", "X-ADK-Stream-ID"} {
		if value := headers.Get(key); value != "" {
			if key == "X-ADK-Stream-ID" {
				value = "stream-fixture"
			}
			want[strings.ToLower(key)] = value
		}
	}
	return want
}

func normalizeADKReadSSEBody(body string, streamID string) string {
	lines := strings.Split(body, "\n")
	for index, line := range lines {
		if strings.HasPrefix(line, "id: ") {
			lines[index] = strings.Replace(line, streamID, "stream-fixture", 1)
			continue
		}
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		var value any
		if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &value); err != nil {
			continue
		}
		value = normalizeADKReadSSEValue("", value)
		contents, err := json.Marshal(value)
		if err == nil {
			lines[index] = "data: " + string(contents)
		}
	}
	return strings.Join(lines, "\n")
}

func normalizeADKReadSSEValue(key string, value any) any {
	if key == "durationMs" {
		return float64(30)
	}
	switch key {
	case "streamId":
		return "stream-fixture"
	case "runId":
		return "run-fixture"
	case "sessionId":
		return "session-fixture"
	}
	if key == "id" {
		if stringValue, ok := value.(string); ok {
			switch {
			case strings.HasPrefix(stringValue, "stream-"):
				return "stream-fixture"
			case strings.HasPrefix(stringValue, "run-"):
				return "run-fixture"
			case strings.HasPrefix(stringValue, "session-"):
				return "session-fixture"
			}
		}
	}
	if stringValue, ok := value.(string); ok {
		if strings.HasPrefix(stringValue, "ctxrev-") {
			return "fixture-context-revision"
		}
		if _, err := uuid.Parse(stringValue); err == nil {
			return "fixture-id"
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
			typed[index] = normalizeADKReadSSEValue(key, typed[index])
		}
	case map[string]any:
		for childKey, childValue := range typed {
			typed[childKey] = normalizeADKReadSSEValue(childKey, childValue)
		}
	}
	return value
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
