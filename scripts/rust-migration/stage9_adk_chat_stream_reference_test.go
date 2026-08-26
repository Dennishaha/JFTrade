package rustmigration

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	assistantapi "github.com/jftrade/jftrade-main/internal/api/assistant"
	"github.com/jftrade/jftrade-main/internal/api/httpserver"
	"github.com/jftrade/jftrade-main/internal/api/middleware"
	assistantservice "github.com/jftrade/jftrade-main/internal/assistant"
	assistantmodel "github.com/jftrade/jftrade-main/internal/assistant/model"
	assistanttestkit "github.com/jftrade/jftrade-main/internal/assistant/testkit"
	adksession "google.golang.org/adk/v2/session"
)

const stage9ADKChatStreamFixtureVersion = "stage9.adk-chat-stream.v1"

var stage9ADKProviderURLPattern = regexp.MustCompile(`http://127\.0\.0\.1:[0-9]+`)

type stage9ADKChatStreamFixture struct {
	Version string                    `json:"version"`
	Cases   []stage9ADKChatStreamCase `json:"cases"`
}

type stage9ADKChatStreamCase struct {
	Name           string                      `json:"name"`
	Method         string                      `json:"method"`
	RequestPath    string                      `json:"requestPath"`
	Body           *string                     `json:"body,omitempty"`
	RequestHeaders map[string]string           `json:"requestHeaders,omitempty"`
	PortMode       string                      `json:"portMode"`
	Expected       stage9ADKChatStreamExpected `json:"expected"`
	Observation    map[string]any              `json:"observation,omitempty"`
}

type stage9ADKChatStreamExpected struct {
	Status  int               `json:"status"`
	Headers map[string]string `json:"headers,omitempty"`
	Body    json.RawMessage   `json:"body"`
	Frames  []stage9SSEFrame  `json:"frames,omitempty"`
}

type stage9SSEFrame struct {
	Kind         string          `json:"kind"`
	ID           string          `json:"id,omitempty"`
	Milliseconds int             `json:"milliseconds,omitempty"`
	Comment      string          `json:"comment,omitempty"`
	Data         json.RawMessage `json:"data,omitempty"`
}

type stage9ADKChatStreamRequest struct {
	Name           string
	Method         string
	RequestPath    string
	Body           *string
	RequestHeaders map[string]string
	PortMode       string
}

// TestStage9ADKChatStreamFixtureMatchesCurrentGoOwner freezes both chat POST
// routes through the real Gin transport and the local ADK runtime. The model
// is an httptest Responses endpoint; no real provider, model, SQLite file, or
// production runtime is used.
func TestStage9ADKChatStreamFixtureMatchesCurrentGoOwner(t *testing.T) {
	_, source, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve stage 9 ADK chat-stream fixture source")
	}
	fixturePath := filepath.Join(
		filepath.Dir(source),
		"../../tests/fixtures/rust-migration/stage9/adk-chat-stream.json",
	)

	want := stage9ADKChatStreamFixture{
		Version: stage9ADKChatStreamFixtureVersion,
		Cases:   make([]stage9ADKChatStreamCase, 0, len(stage9ADKChatStreamCases())),
	}
	for _, testCase := range stage9ADKChatStreamCases() {
		want.Cases = append(want.Cases, runStage9ADKChatStreamCase(t, testCase))
	}
	if os.Getenv("JFTRADE_UPDATE_RUST_MIGRATION_FIXTURES") == "1" {
		contents, err := json.MarshalIndent(want, "", "  ")
		if err != nil {
			t.Fatalf("encode ADK chat-stream fixture: %v", err)
		}
		if err := os.WriteFile(fixturePath, append(contents, '\n'), 0o644); err != nil {
			t.Fatalf("write ADK chat-stream fixture: %v", err)
		}
	}
	contents, err := os.ReadFile(fixturePath)
	if err != nil {
		t.Fatalf("read ADK chat-stream fixture: %v", err)
	}
	var got stage9ADKChatStreamFixture
	if err := json.Unmarshal(contents, &got); err != nil {
		t.Fatalf("decode ADK chat-stream fixture: %v", err)
	}
	var gotValue, wantValue any
	if err := json.Unmarshal(mustJSONMarshal(got), &gotValue); err != nil {
		t.Fatalf("normalize loaded ADK chat-stream fixture: %v", err)
	}
	if err := json.Unmarshal(mustJSONMarshal(want), &wantValue); err != nil {
		t.Fatalf("normalize current ADK chat-stream owner: %v", err)
	}
	if !reflect.DeepEqual(gotValue, wantValue) {
		t.Fatalf("stage 9 ADK chat-stream fixture drifted from the Go owner")
	}
}

func stage9ADKChatStreamCases() []stage9ADKChatStreamRequest {
	validID := "11111111-1111-4111-8111-111111111111"
	body := func(message string) *string {
		value := fmt.Sprintf(`{"clientRequestId":%q,"agentId":"fixture-agent","message":%q}`, validID, message)
		return &value
	}
	return []stage9ADKChatStreamRequest{
		{Name: "chat-success", Method: http.MethodPost, RequestPath: "/api/v1/adk/chat", Body: body("hello"), PortMode: "success"},
		{Name: "chat-empty-message", Method: http.MethodPost, RequestPath: "/api/v1/adk/chat", Body: body("   "), PortMode: "success"},
		{Name: "chat-invalid-json", Method: http.MethodPost, RequestPath: "/api/v1/adk/chat", Body: stringPtr("{"), PortMode: "success"},
		{Name: "chat-invalid-client-request-id", Method: http.MethodPost, RequestPath: "/api/v1/adk/chat", Body: stringPtr(`{"agentId":"fixture-agent","message":"hello"}`), PortMode: "success"},
		{Name: "chat-runtime-unavailable", Method: http.MethodPost, RequestPath: "/api/v1/adk/chat", Body: body("hello"), PortMode: "runtime-unavailable"},
		{Name: "chat-provider-failure", Method: http.MethodPost, RequestPath: "/api/v1/adk/chat", Body: body("provider failure"), PortMode: "provider-failure"},
		{Name: "chat-idempotency-conflict", Method: http.MethodPost, RequestPath: "/api/v1/adk/chat", Body: body("changed"), PortMode: "idempotency-conflict"},
		{Name: "chat-auth-required", Method: http.MethodPost, RequestPath: "/api/v1/adk/chat", Body: body("hello"), PortMode: "auth-required"},
		{Name: "chat-csrf-forbidden", Method: http.MethodPost, RequestPath: "/api/v1/adk/chat", Body: body("hello"), RequestHeaders: map[string]string{"Origin": "http://localhost:3003"}, PortMode: "csrf-forbidden"},
		{Name: "chat-method-not-found", Method: http.MethodGet, RequestPath: "/api/v1/adk/chat", PortMode: "not-found"},
		{Name: "stream-success", Method: http.MethodPost, RequestPath: "/api/v1/adk/chat/stream", Body: body("hello"), PortMode: "success"},
		{Name: "stream-invalid-json", Method: http.MethodPost, RequestPath: "/api/v1/adk/chat/stream", Body: stringPtr("{"), PortMode: "success"},
		{Name: "stream-invalid-client-request-id", Method: http.MethodPost, RequestPath: "/api/v1/adk/chat/stream", Body: stringPtr(`{"agentId":"fixture-agent","message":"hello"}`), PortMode: "success"},
		{Name: "stream-runtime-unavailable", Method: http.MethodPost, RequestPath: "/api/v1/adk/chat/stream", Body: body("hello"), PortMode: "runtime-unavailable"},
		{Name: "stream-provider-failure", Method: http.MethodPost, RequestPath: "/api/v1/adk/chat/stream", Body: body("provider failure"), PortMode: "provider-failure"},
		{Name: "stream-idempotency-conflict", Method: http.MethodPost, RequestPath: "/api/v1/adk/chat/stream", Body: body("changed"), PortMode: "idempotency-conflict"},
		{Name: "stream-client-disconnect", Method: http.MethodPost, RequestPath: "/api/v1/adk/chat/stream", Body: body("disconnect"), PortMode: "client-disconnect"},
	}
}

func runStage9ADKChatStreamCase(
	t *testing.T,
	spec stage9ADKChatStreamRequest,
) stage9ADKChatStreamCase {
	t.Helper()
	result := stage9ADKChatStreamCase{
		Name:           spec.Name,
		Method:         spec.Method,
		RequestPath:    spec.RequestPath,
		Body:           spec.Body,
		RequestHeaders: spec.RequestHeaders,
		PortMode:       spec.PortMode,
	}

	if spec.PortMode == "idempotency-conflict" {
		result.Expected = runStage9IdempotencyConflict(t, spec)
		return result
	}
	if spec.PortMode == "client-disconnect" {
		result.Expected, result.Observation = runStage9ClientDisconnect(t, spec)
		return result
	}

	router, cleanup := stage9ADKChatRouter(t, spec.PortMode)
	t.Cleanup(cleanup)
	response := stage9ServeADKChatRequest(router, spec.Method, spec.RequestPath, spec.Body, spec.RequestHeaders)
	result.Expected = stage9NormalizeADKChatResponse(t, response)
	return result
}

func stage9ADKChatRouter(t *testing.T, mode string) (http.Handler, func()) {
	t.Helper()
	if mode == "auth-required" || mode == "csrf-forbidden" {
		router := gin.New()
		if mode == "auth-required" {
			router.Use(middleware.Auth(nil, nil, nil, nil))
		} else {
			router.Use(middleware.Auth(
				stage9ChatAuthenticator{},
				stage9ChatCSRFValidator{},
				nil,
				stage9ChatOriginChecker{"http://localhost:3003": {}},
			))
		}
		assistantapi.RegisterRoutes(router.Group("/api/v1"), assistantservice.NewService(nil))
		return router, func() {}
	}
	if mode == "not-found" {
		router := gin.New()
		assistantapi.RegisterRoutes(router.Group("/api/v1"), assistantservice.NewService(nil))
		router.NoRoute(func(c *gin.Context) {
			httpserver.WriteError(c, http.StatusNotFound, "NOT_FOUND", "unknown endpoint "+c.Request.URL.Path)
		})
		return router, func() {}
	}
	if mode == "runtime-unavailable" {
		router := gin.New()
		assistantapi.RegisterRoutes(router.Group("/api/v1"), assistantservice.NewService(nil))
		return router, func() {}
	}

	directory := t.TempDir()
	store, err := assistanttestkit.NewStore(
		filepath.Join(directory, "adk.db"),
		filepath.Join(directory, "secrets", "adk-secrets.json"),
		filepath.Join(directory, "skills"),
	)
	if err != nil {
		t.Fatalf("open ADK chat fixture store: %v", err)
	}
	providerServer := httptest.NewServer(stage9ChatProviderHandler(mode))
	if _, err := store.SaveProvider(t.Context(), assistantmodel.ProviderWriteRequest{
		ID: "fixture-provider", DisplayName: "Fixture Provider", BaseURL: providerServer.URL,
		Model: "fixture-model", APIKey: "fixture-key", Enabled: true,
	}); err != nil {
		t.Fatalf("save ADK fixture provider: %v", err)
	}
	if _, err := store.SaveAgent(t.Context(), assistantmodel.AgentWriteRequest{
		ID: "fixture-agent", Name: "Fixture Agent", ProviderID: "fixture-provider",
		Model: "fixture-model", PermissionMode: assistantmodel.PermissionModeLessApproval,
		Status: assistantmodel.AgentStatusEnabled,
	}); err != nil {
		t.Fatalf("save ADK fixture agent: %v", err)
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
	return router, func() {
		if err := handler.Close(); err != nil {
			t.Errorf("close ADK chat fixture handler: %v", err)
		}
		if err := service.Close(); err != nil {
			t.Errorf("close ADK chat fixture service: %v", err)
		}
		providerServer.Close()
	}
}

func stage9ServeADKChatRequest(
	handler http.Handler,
	method string,
	path string,
	body *string,
	headers map[string]string,
) *httptest.ResponseRecorder {
	var reader io.Reader = strings.NewReader("")
	if body != nil {
		reader = strings.NewReader(*body)
	}
	request := httptest.NewRequestWithContext(context.Background(), method, path, reader)
	request.Header.Set("Content-Type", "application/json")
	for key, value := range headers {
		request.Header.Set(key, value)
	}
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	return recorder
}

func stage9NormalizeADKChatResponse(
	t *testing.T,
	response *httptest.ResponseRecorder,
) stage9ADKChatStreamExpected {
	t.Helper()
	expected := stage9ADKChatStreamExpected{
		Status:  response.Code,
		Headers: stage9ADKChatHeaders(response.Header()),
	}
	if strings.Contains(response.Header().Get("Content-Type"), "text/event-stream") {
		frames := stage9ParseSSEFrames(t, response.Body.String())
		expected.Frames = frames
		normalizedBody := stage9EncodeSSEFrames(frames)
		expected.Body = json.RawMessage(mustJSONMarshal(normalizedBody))
		return expected
	}
	var value any
	if err := json.Unmarshal(response.Body.Bytes(), &value); err != nil {
		t.Fatalf("decode ADK chat response status=%d body=%q: %v", response.Code, response.Body.String(), err)
	}
	expected.Body = mustJSONMarshal(stage9NormalizeADKValue("", "", value))
	return expected
}

func stage9ADKChatHeaders(headers http.Header) map[string]string {
	result := map[string]string{}
	for _, key := range []string{
		"Content-Type", "Cache-Control", "Connection", "X-ADK-Stream-Idle-Timeout-Ms", "X-ADK-Stream-ID",
	} {
		if value := headers.Get(key); value != "" {
			result[key] = value
		}
	}
	if streamID := result["X-ADK-Stream-ID"]; streamID != "" {
		result["X-ADK-Stream-ID"] = "stream-fixture"
	}
	return result
}

func stage9ParseSSEFrames(t *testing.T, body string) []stage9SSEFrame {
	t.Helper()
	frames := make([]stage9SSEFrame, 0)
	for _, segment := range strings.Split(body, "\n\n") {
		if strings.TrimSpace(segment) == "" {
			continue
		}
		frame := stage9SSEFrame{}
		var data strings.Builder
		for _, line := range strings.Split(segment, "\n") {
			switch {
			case strings.HasPrefix(line, "retry: "):
				frame.Kind = "retry"
				_, err := fmt.Sscanf(strings.TrimPrefix(line, "retry: "), "%d", &frame.Milliseconds)
				if err != nil {
					t.Fatalf("parse retry frame %q: %v", line, err)
				}
			case strings.HasPrefix(line, ": "):
				frame.Kind = "comment"
				frame.Comment = strings.TrimPrefix(line, ": ")
			case strings.HasPrefix(line, "id: "):
				frame.ID = strings.TrimPrefix(line, "id: ")
			case strings.HasPrefix(line, "data: "):
				frame.Kind = "event"
				data.WriteString(strings.TrimPrefix(line, "data: "))
			}
		}
		if frame.Kind == "event" {
			var value any
			if err := json.Unmarshal([]byte(data.String()), &value); err != nil {
				t.Fatalf("decode SSE event %q: %v", data.String(), err)
			}
			value = stage9NormalizeADKValue("", "", value)
			frame.ID = stage9NormalizeSSEID(frame.ID)
			frame.Data = mustJSONMarshal(value)
		}
		frames = append(frames, frame)
	}
	return frames
}

func stage9NormalizeSSEID(value string) string {
	if value == "" {
		return ""
	}
	parts := strings.Split(value, ":")
	if len(parts) == 2 && strings.HasPrefix(parts[0], "stream-") {
		return "stream-fixture:" + parts[1]
	}
	return value
}

func stage9EncodeSSEFrames(frames []stage9SSEFrame) string {
	var body strings.Builder
	for _, frame := range frames {
		switch frame.Kind {
		case "retry":
			fmt.Fprintf(&body, "retry: %d\n\n", frame.Milliseconds)
		case "comment":
			fmt.Fprintf(&body, ": %s\n\n", frame.Comment)
		case "event":
			if frame.ID != "" {
				body.WriteString("id: ")
				body.WriteString(frame.ID)
				body.WriteByte('\n')
			}
			body.WriteString("data: ")
			body.Write(frame.Data)
			body.WriteString("\n\n")
		}
	}
	return body.String()
}

func stage9NormalizeADKValue(parent string, key string, value any) any {
	if text, ok := value.(string); ok {
		value = stage9NormalizeADKText(text)
	}
	if timestampKey(key) {
		if _, ok := value.(string); ok {
			return "fixture-time"
		}
	}
	if key == "streamId" {
		if _, ok := value.(string); ok {
			return "stream-fixture"
		}
	}
	if key == "runId" {
		if _, ok := value.(string); ok {
			return "run-fixture"
		}
	}
	if key == "sessionId" {
		if _, ok := value.(string); ok {
			return "session-fixture"
		}
	}
	if key == "contextRevisionId" {
		if _, ok := value.(string); ok {
			return "context-revision-fixture"
		}
	}
	if key == "finalMessageId" {
		if _, ok := value.(string); ok {
			return "message-fixture"
		}
	}
	if key == "durationMs" {
		if _, ok := value.(float64); ok {
			return float64(0)
		}
	}
	if key == "id" {
		if _, ok := value.(string); ok {
			switch parent {
			case "session":
				return "session-fixture"
			case "run":
				return "run-fixture"
			case "response":
				return "response-fixture"
			case "timeline":
				return "timeline-fixture"
			case "toolCalls":
				return "tool-fixture"
			}
		}
	}
	switch typed := value.(type) {
	case []any:
		for index := range typed {
			typed[index] = stage9NormalizeADKValue(parent, key, typed[index])
		}
	case map[string]any:
		for childKey, childValue := range typed {
			typed[childKey] = stage9NormalizeADKValue(key, childKey, childValue)
		}
	}
	return value
}

func stage9NormalizeADKText(value string) string {
	return stage9ADKProviderURLPattern.ReplaceAllString(value, "http://fixture-provider")
}

func timestampKey(key string) bool {
	switch key {
	case "timestamp", "createdAt", "updatedAt", "startedAt", "completedAt", "cancelledAt", "contextRevisionCreatedAt":
		return true
	default:
		return false
	}
}

func runStage9IdempotencyConflict(
	t *testing.T,
	spec stage9ADKChatStreamRequest,
) stage9ADKChatStreamExpected {
	t.Helper()
	mode := "success"
	router, cleanup := stage9ADKChatRouter(t, mode)
	t.Cleanup(cleanup)
	validID := "11111111-1111-4111-8111-111111111111"
	firstMessage := "hello"
	if strings.Contains(spec.RequestPath, "/stream") {
		firstMessage = "stream first"
	}
	firstBody := fmt.Sprintf(`{"clientRequestId":%q,"agentId":"fixture-agent","message":%q}`, validID, firstMessage)
	first := stage9ServeADKChatRequest(router, spec.Method, spec.RequestPath, &firstBody, nil)
	if first.Code != http.StatusOK {
		t.Fatalf("idempotency seed status = %d body=%q", first.Code, first.Body.String())
	}
	second := stage9ServeADKChatRequest(router, spec.Method, spec.RequestPath, spec.Body, spec.RequestHeaders)
	return stage9NormalizeADKChatResponse(t, second)
}

func runStage9ClientDisconnect(
	t *testing.T,
	spec stage9ADKChatStreamRequest,
) (stage9ADKChatStreamExpected, map[string]any) {
	t.Helper()
	router, cleanup := stage9ADKChatRouter(t, "success")
	t.Cleanup(cleanup)
	writer := newStage9FailingSSEWriter()
	var reader io.Reader = strings.NewReader(*spec.Body)
	request := httptest.NewRequestWithContext(context.Background(), spec.Method, spec.RequestPath, reader)
	request.Header.Set("Content-Type", "application/json")
	for key, value := range spec.RequestHeaders {
		request.Header.Set(key, value)
	}
	router.ServeHTTP(writer, request)
	streamID := writer.Header().Get("X-ADK-Stream-ID")
	if streamID == "" {
		t.Fatal("disconnected stream did not allocate a stream id")
	}
	var replay *httptest.ResponseRecorder
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		candidate := stage9ServeADKChatRequest(router, http.MethodGet, "/api/v1/adk/streams/"+streamID, nil, nil)
		if strings.Contains(candidate.Body.String(), `"type":"final"`) {
			replay = candidate
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if replay == nil {
		t.Fatal("disconnected stream did not retain a terminal replay")
	}
	expected := stage9ADKChatStreamExpected{
		Status:  writer.status,
		Headers: stage9ADKChatHeaders(writer.Header()),
		Body:    mustJSONMarshal(writer.body.String()),
	}
	observation := map[string]any{
		"replay":                 stage9NormalizeADKChatResponse(t, replay),
		"writesBeforeDisconnect": float64(writer.writes),
		"writeError":             writer.writeError,
		"closedAfterTerminal":    true,
	}
	return expected, observation
}

type stage9FailingSSEWriter struct {
	header     http.Header
	status     int
	writes     int
	body       bytes.Buffer
	writeError string
}

func newStage9FailingSSEWriter() *stage9FailingSSEWriter {
	return &stage9FailingSSEWriter{header: make(http.Header)}
}

func (w *stage9FailingSSEWriter) Header() http.Header { return w.header }

func (w *stage9FailingSSEWriter) WriteHeader(status int) { w.status = status }

func (w *stage9FailingSSEWriter) Write(value []byte) (int, error) {
	w.writes++
	if w.writes == 1 {
		err := errors.New("stream client disconnected")
		w.writeError = err.Error()
		return 0, err
	}
	return w.body.Write(value)
}

func (w *stage9FailingSSEWriter) Flush() {}

type stage9ChatAuthenticator struct{}

func (stage9ChatAuthenticator) Authenticate(*http.Request) (string, bool, bool) {
	return "fixture-session", true, false
}

type stage9ChatCSRFValidator struct{}

func (stage9ChatCSRFValidator) ValidateCSRF(*http.Request, string) bool { return false }

type stage9ChatOriginChecker map[string]struct{}

func (checker stage9ChatOriginChecker) IsOriginAllowed(_ *http.Request, origin string) bool {
	_, ok := checker[origin]
	return ok
}

func stage9ChatProviderHandler(mode string) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost || !strings.HasSuffix(request.URL.Path, "/responses") {
			http.NotFound(writer, request)
			return
		}
		if mode == "provider-failure" {
			writer.Header().Set("Content-Type", "application/json")
			writer.WriteHeader(http.StatusInternalServerError)
			_, _ = io.WriteString(writer, `{"error":{"message":"fixture provider failed"}}`)
			return
		}
		var payload map[string]any
		if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
			writer.WriteHeader(http.StatusBadRequest)
			return
		}
		stream, _ := payload["stream"].(bool)
		if !stream {
			writer.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(writer, `{"id":"resp-fixture","model":"fixture-model","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"ok","annotations":[]}]}],"usage":{"input_tokens":1,"output_tokens":1,"total_tokens":2}}`)
			return
		}
		writer.Header().Set("Content-Type", "text/event-stream")
		write := func(event string) {
			_, _ = io.WriteString(writer, "data: "+event+"\n\n")
		}
		write(`{"type":"response.created","response":{"id":"resp-fixture","model":"fixture-model"}}`)
		write(`{"type":"response.output_text.delta","delta":"ok"}`)
		write(`{"type":"response.completed","response":{"id":"resp-fixture","model":"fixture-model","usage":{"input_tokens":1,"output_tokens":1,"total_tokens":2}}}`)
		write(`[DONE]`)
	})
}

func stringPtr(value string) *string { return &value }

func mustJSONMarshal(value any) json.RawMessage {
	contents, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	return contents
}

var _ = uuid.Nil
