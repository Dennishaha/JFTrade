package assistant

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	adksession "google.golang.org/adk/v2/session"

	assistantservice "github.com/jftrade/jftrade-main/internal/assistant"
	assistantassembly "github.com/jftrade/jftrade-main/internal/assistant/assembly"
	jfadk "github.com/jftrade/jftrade-main/internal/assistant/engine/workflowruntime"
	assistanttestkit "github.com/jftrade/jftrade-main/internal/assistant/testkit"
	jfsettings "github.com/jftrade/jftrade-main/internal/jftsettings"
)

const testADKProviderID = "test-provider"

func testADKChatJSON(body string) string {
	trimmed := strings.TrimSpace(body)
	if !strings.HasPrefix(trimmed, "{") {
		return body
	}
	tail := strings.TrimPrefix(trimmed, "{")
	if tail == "}" {
		return `{"clientRequestId":"` + uuid.NewString() + `"}`
	}
	return `{"clientRequestId":"` + uuid.NewString() + `",` + tail
}

func testADKChatBody(body string) []byte {
	return []byte(testADKChatJSON(body))
}

// SettingsStore is the small settings surface needed by API transport tests.
// Persistent Assistant state belongs to the ADK store below, not this fixture.
type SettingsStore struct {
	path string

	mu     sync.RWMutex
	adk    jfsettings.ADKRuntimeSettings
	adkSet bool
}

func NewSettingsStore(path string) (*SettingsStore, error) {
	if strings.TrimSpace(path) == "" {
		return nil, errors.New("settings path is required")
	}
	return &SettingsStore{
		path: path,
		adk:  jfsettings.ADKRuntimeSettings{RunTimeoutMs: 1_800_000, StreamIdleTimeoutMs: 300_000},
	}, nil
}

func (s *SettingsStore) Path() string {
	if s == nil {
		return ""
	}
	return s.path
}

func (s *SettingsStore) ADKSettings() jfsettings.ADKRuntimeSettings {
	if s == nil {
		return jfsettings.ADKRuntimeSettings{}
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.adk
}

func (s *SettingsStore) SaveADKSettings(settings jfsettings.ADKRuntimeSettings) (jfsettings.ADKRuntimeSettings, error) {
	if s == nil {
		return jfsettings.ADKRuntimeSettings{}, errors.New("settings store is unavailable")
	}
	s.mu.Lock()
	s.adk = settings
	s.adkSet = true
	s.mu.Unlock()
	return settings, nil
}

type assistantRouteServer struct {
	runtime      *assistanttestkit.Runtime
	assistantSvc *assistantservice.Service
	originalSvc  *assistantservice.Service
	handler      *Handler
	store        *SettingsStore
	router       *gin.Engine
	openErr      error
}

// assistantRuntime exposes only the owner runtime operations used by the
// migrated tests. It deliberately does not recreate the old servercore shim.
type assistantRuntimeTestAdapter struct {
	runtime *assistanttestkit.Runtime
}

func (r *assistantRuntimeTestAdapter) RegisterTool(descriptor jfadk.ToolDescriptor, handler jfadk.ToolFunc) error {
	if r == nil || r.runtime == nil || r.runtime.Tools() == nil {
		return errors.New("ADK runtime is unavailable")
	}
	r.runtime.Tools().Register(descriptor, handler)
	return nil
}

func (r *assistantRuntimeTestAdapter) Tool(name string) (jfadk.RegisteredTool, bool) {
	if r == nil || r.runtime == nil || r.runtime.Tools() == nil {
		return jfadk.RegisteredTool{}, false
	}
	return r.runtime.Tools().Get(name)
}

func (r *assistantRuntimeTestAdapter) RecordAudit(ctx context.Context, kind string, subjectID string, detail string, metadata map[string]any) {
	if r != nil && r.runtime != nil {
		r.runtime.RecordAudit(ctx, kind, subjectID, detail, metadata)
	}
}

func assistantRuntime(server *assistantRouteServer) *assistantRuntimeTestAdapter {
	if server == nil {
		return nil
	}
	return &assistantRuntimeTestAdapter{runtime: server.runtime}
}

func newTestServer(t *testing.T, settings *SettingsStore) *assistantRouteServer {
	t.Helper()
	server := openAssistantRouteServer(settings)
	if server.openErr != nil {
		t.Fatalf("open assistant runtime: %v", server.openErr)
	}
	configureTestADKProvider(t, server)
	t.Cleanup(func() {
		if err := server.Close(); err != nil {
			t.Errorf("assistant runtime close: %v", err)
		}
	})
	return server
}

func newHTTPTestServer(t *testing.T, settings *SettingsStore) *httptest.Server {
	t.Helper()
	server := newTestServer(t, settings)
	srv := httptest.NewServer(server)
	t.Cleanup(srv.Close)
	return srv
}

// newServerWithFrontend is retained for the opt-in live integration test. It
// opens the configured ADK paths without replacing the saved provider.
func newServerWithFrontend(settings *SettingsStore, _ any) *assistantRouteServer {
	return openAssistantRouteServer(settings)
}

func openAssistantRouteServer(settings *SettingsStore) *assistantRouteServer {
	server := &assistantRouteServer{store: settings}
	if settings == nil {
		server.openErr = errors.New("settings store is unavailable")
		return server
	}
	paths := assistantTestPaths(settings.Path())
	store, err := assistanttestkit.NewStore(paths.database, paths.secrets, paths.skills)
	if err != nil {
		server.openErr = err
		return server
	}
	registry := assistanttestkit.NewToolRegistry()
	deps := assistantassembly.ToolDeps{
		ADKEnabled:   func() bool { return true },
		SystemStatus: func() map[string]any { return map[string]any{"status": "ok"} },
		SaveStrategyDraft: func(input assistantassembly.StrategyDraftInput) (any, error) {
			return map[string]any{"saved": true, "name": input.Name}, nil
		},
	}
	assistantassembly.RegisterJFTradeADKTools(store, registry, deps)
	sessionService := adksession.InMemoryService()
	runtime := assistanttestkit.NewRuntimeWithSessionService(store, registry, sessionService)
	runtime.SetRuntimeLimitsProvider(func() jfadk.RuntimeLimits {
		return jfadk.RuntimeLimits{RunTimeout: time.Duration(settings.ADKSettings().RunTimeoutMs) * time.Millisecond}
	})
	server.runtime = runtime
	server.assistantSvc = assistantservice.NewService(
		runtime,
		assistantservice.WithRuntimeSettings(func() any { return settings.ADKSettings() }),
		assistantservice.WithStreamIdleTimeout(func() int { return settings.ADKSettings().StreamIdleTimeoutMs }),
	)
	server.originalSvc = server.assistantSvc
	server.router = server.buildRouter()
	return server
}

func (s *assistantRouteServer) buildRouter() *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	s.handler = RegisterRoutes(router.Group("/api/v1"), s.assistantSvc)
	return router
}

func (s *assistantRouteServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if s == nil || s.router == nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}
	s.router.ServeHTTP(w, r)
}

func (s *assistantRouteServer) Close() error {
	if s == nil || s.assistantSvc == nil {
		return nil
	}
	var err error
	if s.handler != nil {
		err = s.handler.Close()
	}
	if closeErr := s.assistantSvc.Close(); err == nil {
		err = closeErr
	}
	if s.originalSvc != nil && s.originalSvc != s.assistantSvc {
		if closeErr := s.originalSvc.Close(); err == nil {
			err = closeErr
		}
	}
	return err
}

func serverADKTestStore(t testing.TB, server *assistantRouteServer) *assistanttestkit.Store {
	t.Helper()
	if server == nil || server.store == nil {
		t.Fatal("assistant test store requires an initialized server")
	}
	paths := assistantTestPaths(server.store.Path())
	store, err := assistanttestkit.NewStore(paths.database, paths.secrets, paths.skills)
	if err != nil {
		t.Fatalf("open assistant test store: %v", err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Errorf("assistant test store close: %v", err)
		}
	})
	return store
}

type assistantTestPathSet struct {
	database string
	secrets  string
	skills   string
	session  string
}

func assistantTestPaths(settingsPath string) assistantTestPathSet {
	directory := filepath.Dir(strings.TrimSpace(settingsPath))
	if directory == "" || directory == "." {
		directory = "."
	}
	pathOr := func(name string, fallback string) string {
		if value := strings.TrimSpace(os.Getenv(name)); value != "" {
			return value
		}
		return fallback
	}
	return assistantTestPathSet{
		database: pathOr("JFTRADE_ADK_DB", filepath.Join(directory, "adk.db")),
		secrets:  pathOr("JFTRADE_ADK_SECRETS", filepath.Join(directory, "secrets", "adk-secrets.json")),
		skills:   pathOr("JFTRADE_ADK_SKILLS_DIR", filepath.Join(directory, "adk", "skills")),
		session:  pathOr("JFTRADE_ADK_SESSION_DB", filepath.Join(directory, "adk-session.db")),
	}
}

func configureTestADKProvider(t *testing.T, server *assistantRouteServer) {
	t.Helper()
	if server == nil || server.runtime == nil {
		return
	}
	providerServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		serveAssistantResponsesFixture(t, w, r, testADKToolNameFromText)
	}))
	t.Cleanup(providerServer.Close)
	if _, err := server.runtime.Store().SaveProvider(t.Context(), jfadk.ProviderWriteRequest{
		ID: testADKProviderID, DisplayName: "Test Provider", BaseURL: providerServer.URL,
		Model: "test-model", APIKey: "sk-test", Enabled: true,
	}); err != nil {
		t.Fatalf("SaveProvider test: %v", err)
	}
	if agent, err := server.runtime.Store().DefaultAgent(t.Context()); err == nil {
		_, err := server.runtime.Store().SaveAgent(t.Context(), jfadk.AgentWriteRequest{
			ID: agent.ID, Name: agent.Name, ProviderID: testADKProviderID, Model: agent.Model,
			Instruction: agent.Instruction, Tools: agent.Tools, PermissionMode: agent.PermissionMode,
			Status: agent.Status, WorkMode: agent.WorkMode, LoopMaxIterations: agent.LoopMaxIterations,
			RecentUserWindow: agent.RecentUserWindow, MemoryEnabled: agent.MemoryEnabled,
		})
		if err != nil {
			t.Fatalf("Save default agent: %v", err)
		}
	}
}

func testADKToolNameFromText(text string) string {
	for _, name := range []string{
		"approval.required", "strategy.save_draft", "strategy.optimize", "tasks.create", "memory.remember", "contract.write",
	} {
		if strings.Contains(text, "@"+name) || strings.Contains(text, `name="`+name+`"`) {
			return name
		}
	}
	return ""
}

func serveAssistantResponsesFixture(
	t *testing.T,
	w http.ResponseWriter,
	r *http.Request,
	selectTool func(string) string,
) {
	t.Helper()
	defer func() { jftradeCheckTestError(t, r.Body.Close()) }()
	if r.Method != http.MethodPost || !strings.HasSuffix(r.URL.Path, "/responses") {
		http.NotFound(w, r)
		return
	}
	var payload map[string]any
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		t.Errorf("decode provider request: %v", err)
		return
	}
	text, hasToolResponse := assistantResponsesFixtureInput(payload["input"])
	tool := ""
	if !hasToolResponse && selectTool != nil {
		tool = selectTool(text)
	}
	stream, _ := payload["stream"].(bool)
	writeAssistantResponsesFixture(t, w, tool, stream)
}

func assistantResponsesFixtureInput(value any) (string, bool) {
	items, _ := value.([]any)
	var text strings.Builder
	hasToolResponse := false
	for _, rawItem := range items {
		item, _ := rawItem.(map[string]any)
		switch item["type"] {
		case "message":
			text.WriteString("\n" + assistantResponsesFixtureText(item["content"]))
		case "function_call_output":
			hasToolResponse = true
			text.WriteString("\n" + assistantResponsesFixtureText(item["output"]))
		}
	}
	return text.String(), hasToolResponse
}

func assistantResponsesFixtureText(value any) string {
	if text, ok := value.(string); ok {
		return text
	}
	items, _ := value.([]any)
	var text strings.Builder
	for _, rawItem := range items {
		item, _ := rawItem.(map[string]any)
		if value, _ := item["text"].(string); value != "" {
			text.WriteString(value)
		}
	}
	return text.String()
}

func writeAssistantResponsesFixture(t *testing.T, w http.ResponseWriter, tool string, stream bool) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if !stream {
		output := []map[string]any{{
			"type": "message", "role": "assistant",
			"content": []map[string]any{{"type": "output_text", "text": "ok", "annotations": []any{}}},
		}}
		if tool != "" {
			wireName := strings.ReplaceAll(tool, ".", "-")
			output = []map[string]any{{
				"type": "function_call", "call_id": "call-" + wireName,
				"name": wireName, "arguments": `{}`,
			}}
		}
		if err := json.NewEncoder(w).Encode(map[string]any{
			"id": "resp-test", "model": "test-model", "output": output,
			"usage": map[string]any{"input_tokens": 1, "output_tokens": 1, "total_tokens": 2},
		}); err != nil {
			t.Errorf("encode provider response: %v", err)
		}
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	writeEvent := func(event map[string]any) {
		raw, err := json.Marshal(event)
		if err != nil {
			t.Errorf("encode provider stream event: %v", err)
			return
		}
		_, err = w.Write(append(append([]byte("data: "), raw...), []byte("\n\n")...))
		jftradeCheckTestError(t, err)
	}
	writeEvent(map[string]any{"type": "response.created", "response": map[string]any{"id": "resp-test", "model": "test-model"}})
	if tool == "" {
		writeEvent(map[string]any{"type": "response.output_text.delta", "delta": "ok"})
	} else {
		wireName := strings.ReplaceAll(tool, ".", "-")
		writeEvent(map[string]any{"type": "response.output_item.added", "item": map[string]any{
			"type": "function_call", "id": "fc-test", "call_id": "call-" + wireName, "name": wireName,
		}})
		writeEvent(map[string]any{
			"type": "response.function_call_arguments.done", "item_id": "fc-test",
			"name": wireName, "arguments": `{}`,
		})
	}
	writeEvent(map[string]any{"type": "response.completed", "response": map[string]any{
		"id": "resp-test", "model": "test-model",
		"usage": map[string]any{"input_tokens": 1, "output_tokens": 1, "total_tokens": 2},
	}})
	_, err := w.Write([]byte("data: [DONE]\n\n"))
	jftradeCheckTestError(t, err)
}

func jftradeTestHTTPGet(t testing.TB, target string) (*http.Response, error) {
	t.Helper()
	request, err := http.NewRequestWithContext(t.Context(), http.MethodGet, target, nil)
	if err != nil {
		return nil, err
	}
	return http.DefaultClient.Do(request)
}

func jftradeTestHTTPPost(t testing.TB, target string, contentType string, body io.Reader) (*http.Response, error) {
	t.Helper()
	request, err := http.NewRequestWithContext(t.Context(), http.MethodPost, target, body)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Content-Type", contentType)
	return http.DefaultClient.Do(request)
}

func jftradeCheckedTypeAssertion[T any](value any) T {
	return value.(T)
}
