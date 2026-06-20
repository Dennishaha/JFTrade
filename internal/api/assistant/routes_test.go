package assistant

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	adkmodel "google.golang.org/adk/model"
	adksession "google.golang.org/adk/session"
	"google.golang.org/genai"
	_ "modernc.org/sqlite"

	assistantservice "github.com/jftrade/jftrade-main/internal/assistant"
	jfadk "github.com/jftrade/jftrade-main/pkg/adk"
)

func TestCatalogSessionRunAndObservabilityContracts(t *testing.T) {
	runtime, router := newAssistantTestRouter(t)
	ctx := t.Context()

	provider, err := runtime.Store().SaveProvider(ctx, jfadk.ProviderWriteRequest{
		ID: "provider-disabled", DisplayName: "Disabled", Enabled: false,
	})
	if err != nil {
		t.Fatalf("SaveProvider: %v", err)
	}
	agent, err := runtime.Store().SaveAgent(ctx, jfadk.AgentWriteRequest{
		ID: "agent-catalog", Name: "Catalog Agent", Status: jfadk.AgentStatusEnabled,
	})
	if err != nil {
		t.Fatalf("SaveAgent: %v", err)
	}
	session, err := runtime.Store().CreateSession(ctx, agent.ID, "contract")
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	run := jfadk.Run{
		ID: "run-contract", SessionID: session.ID, AgentID: agent.ID,
		Status: jfadk.RunStatusCompleted, CreatedAt: time.Now().UTC().Format(time.RFC3339Nano),
	}
	if err := runtime.Store().SaveRun(ctx, run); err != nil {
		t.Fatalf("SaveRun: %v", err)
	}
	runtime.RecordAudit(ctx, "agent.saved", agent.ID, "saved", nil)
	if _, err := runtime.Store().SaveOptimizationTask(ctx, jfadk.OptimizationTask{
		ID: "optimization-contract", Status: "queued", Objective: "return",
		CreatedAt: time.Now().UTC().Format(time.RFC3339Nano),
		UpdatedAt: time.Now().UTC().Format(time.RFC3339Nano),
	}); err != nil {
		t.Fatalf("SaveOptimizationTask: %v", err)
	}

	for _, path := range []string{
		"/api/v1/adk",
		"/api/v1/adk/providers",
		"/api/v1/adk/agents",
		"/api/v1/adk/skills",
		"/api/v1/adk/sessions",
		"/api/v1/adk/sessions/" + session.ID,
		"/api/v1/adk/runs",
		"/api/v1/adk/runs/" + run.ID,
		"/api/v1/adk/audit",
		"/api/v1/adk/metrics",
		"/api/v1/adk/optimization-tasks",
		"/api/v1/adk/optimization-tasks/optimization-contract",
	} {
		recorder := performAssistantRequest(router, http.MethodGet, path, nil)
		if recorder.Code != http.StatusOK {
			t.Fatalf("GET %s status=%d body=%s", path, recorder.Code, recorder.Body.String())
		}
		assertOKEnvelope(t, recorder)
	}

	deleteProvider := performAssistantRequest(router, http.MethodDelete, "/api/v1/adk/providers/"+provider.ID, nil)
	if deleteProvider.Code != http.StatusOK {
		t.Fatalf("DELETE provider status=%d body=%s", deleteProvider.Code, deleteProvider.Body.String())
	}
}

func TestAgentSaveErrorClassification(t *testing.T) {
	if !isADKAgentValidationError(errors.New("provider not found")) {
		t.Fatalf("provider validation error should stay client-classified")
	}
	if isADKAgentValidationError(errors.New("disk write failed")) {
		t.Fatalf("generic persistence error should not be client-classified")
	}
}

func TestChatStreamHubReplayAndCleanupBoundaries(t *testing.T) {
	hub := newADKChatStreamHub()
	record := hub.create()
	hub.publish(record, adkChatStreamEvent{Type: "run", Run: &jfadk.Run{
		ID: "run-replay", Status: jfadk.RunStatusRunning,
	}})
	replayUntil := record.currentSequence()
	hub.publish(record, adkChatStreamEvent{Type: "timeline", Timeline: &jfadk.TimelineEntry{
		ID: "live-after-reconnect", RunID: "run-replay",
	}})

	events, _, _ := record.snapshot(0)
	if len(events) != 2 {
		t.Fatalf("events len=%d, want 2", len(events))
	}
	for index := range events {
		if events[index].Sequence <= replayUntil {
			events[index].Replay = true
		}
	}
	if !events[0].Replay {
		t.Fatalf("first event should be replayed: %+v", events[0])
	}
	if events[1].Replay {
		t.Fatalf("live event should not be replayed: %+v", events[1])
	}

	oldStartedAt := time.Now().Add(-2 * jfadk.DefaultRunTimeout).Add(-2 * adkChatStreamRetention).UTC()
	hub.cleanupWithRunLookup(func(runID string) (jfadk.Run, bool) {
		if runID != "run-replay" {
			return jfadk.Run{}, false
		}
		return jfadk.Run{
			ID:            runID,
			Status:        jfadk.RunStatusRunning,
			StartedAt:     oldStartedAt.Format(time.RFC3339Nano),
			MaxDurationMs: int64(jfadk.DefaultRunTimeout / time.Millisecond),
		}, true
	})
	if _, ok := hub.get(record.id); ok {
		t.Fatalf("expired stream was not cleaned up")
	}
}

func TestSessionTimelineFailureKeepsLegacyErrorCode(t *testing.T) {
	runtime, router, dbPath, sessionService := newAssistantTestRouterWithDBPath(t)
	ctx := t.Context()
	agent, err := runtime.Store().SaveAgent(ctx, jfadk.AgentWriteRequest{
		ID: "agent-timeline-fail", Name: "Timeline Fail", Status: jfadk.AgentStatusEnabled,
	})
	if err != nil {
		t.Fatalf("SaveAgent: %v", err)
	}
	session, err := runtime.Store().CreateSession(ctx, agent.ID, "timeline")
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	created, err := sessionService.Create(ctx, &adksession.CreateRequest{
		AppName:   "jftrade-" + agent.ID,
		UserID:    "jftrade-user",
		SessionID: session.ID,
	})
	if err != nil {
		t.Fatalf("Create ADK session: %v", err)
	}
	event := adksession.NewEvent("run-timeline-fail")
	event.ID = "event-timeline-fail"
	event.Author = "user"
	event.Timestamp = time.Now().UTC()
	event.LLMResponse = adkmodel.LLMResponse{
		Content: genai.NewContentFromText("hello", genai.RoleUser),
	}
	if err := sessionService.AppendEvent(ctx, created.Session, event); err != nil {
		t.Fatalf("AppendEvent: %v", err)
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer func() { jftradeCheckTestError(t, db.Close()) }()
	if _, err := db.ExecContext(ctx, `DROP TABLE adk_runs`); err != nil {
		t.Fatalf("drop adk_runs: %v", err)
	}

	recorder := performAssistantRequest(router, http.MethodGet, "/api/v1/adk/sessions/"+session.ID, nil)
	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	var envelope struct {
		OK    bool `json:"ok"`
		Error *struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("decode envelope: %v", err)
	}
	if envelope.Error == nil || envelope.Error.Code != "ADK_MESSAGES_GET_FAILED" {
		t.Fatalf("error envelope=%s", recorder.Body.String())
	}
}

func newAssistantTestRouterWithDBPath(t *testing.T) (*jfadk.Runtime, *gin.Engine, string, adksession.Service) {
	t.Helper()
	root := t.TempDir()
	dbPath := filepath.Join(root, "adk.db")
	store, err := jfadk.NewStore(
		dbPath,
		filepath.Join(root, "secrets.json"),
		filepath.Join(root, "skills"),
	)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	sessionService := adksession.InMemoryService()
	runtime := jfadk.NewRuntimeWithSessionService(store, jfadk.NewToolRegistry(), sessionService)
	assistantTestProvider(t, runtime)
	t.Cleanup(func() {
		jftradeErr1 := runtime.Close()
		jftradeCheckTestError(t, jftradeErr1)
	})
	service := assistantservice.NewService(runtime)
	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	RegisterRoutes(router.Group("/api/v1"), service)
	return runtime, router, dbPath, sessionService
}

func TestChatAndSSEContracts(t *testing.T) {
	runtime, router := newAssistantTestRouter(t)
	agent, err := runtime.Store().SaveAgent(t.Context(), jfadk.AgentWriteRequest{
		ID: "agent-stream", Name: "Stream Agent",
		ProviderID:     "test-provider",
		PermissionMode: jfadk.PermissionModeApproval,
		Status:         jfadk.AgentStatusEnabled,
	})
	if err != nil {
		t.Fatalf("SaveAgent: %v", err)
	}

	payload := []byte(`{"agentId":"` + agent.ID + `","message":"hello"}`)
	chat := performAssistantRequest(router, http.MethodPost, "/api/v1/adk/chat", payload)
	if chat.Code != http.StatusOK {
		t.Fatalf("chat status=%d body=%s", chat.Code, chat.Body.String())
	}
	assertOKEnvelope(t, chat)

	stream := performAssistantRequest(router, http.MethodPost, "/api/v1/adk/chat/stream", payload)
	if stream.Code != http.StatusOK {
		t.Fatalf("stream status=%d body=%s", stream.Code, stream.Body.String())
	}
	if got := stream.Header().Get("Content-Type"); !strings.Contains(got, "text/event-stream") {
		t.Fatalf("stream Content-Type=%q", got)
	}
	if got := stream.Header().Get("X-ADK-Stream-Idle-Timeout-Ms"); got != "420000" {
		t.Fatalf("stream idle timeout=%q", got)
	}
	body := stream.Body.String()
	for _, eventType := range []string{`"type":"session"`, `"type":"run"`, `"type":"final"`} {
		if !strings.Contains(body, eventType) {
			t.Fatalf("stream body missing %s: %s", eventType, body)
		}
	}
}

func TestChatRequestUsesDeclaredMessageFieldOnly(t *testing.T) {
	payload, err := decodeADKChatRequest(strings.NewReader(`{"agentId":"agent","sessionId":"session","message":"hello","prompt":"legacy","text":"legacy-text"}`))
	if err != nil {
		t.Fatalf("decodeADKChatRequest: %v", err)
	}
	if payload.AgentID != "agent" || payload.SessionID != "session" || payload.Message != "hello" {
		t.Fatalf("payload=%+v, want declared ChatRequest fields", payload)
	}

	legacyOnly, err := decodeADKChatRequest(strings.NewReader(`{"agentId":"agent","prompt":"legacy","text":"legacy-text"}`))
	if err != nil {
		t.Fatalf("decodeADKChatRequest legacy-only: %v", err)
	}
	if legacyOnly.Message != "" {
		t.Fatalf("legacy alias populated message=%q, want empty", legacyOnly.Message)
	}
}

func TestApprovalContract(t *testing.T) {
	runtime, router := newAssistantTestRouter(t)
	runtime.Tools().Register(jfadk.ToolDescriptor{
		Name: "contract.write", Permission: "write_strategy",
		AllowedModes: []string{jfadk.PermissionModeApproval},
	}, func(context.Context, map[string]any) (any, error) {
		return map[string]any{"saved": true}, nil
	})
	agent, err := runtime.Store().SaveAgent(t.Context(), jfadk.AgentWriteRequest{
		ID: "agent-approval", Name: "Approval Agent",
		ProviderID:     "test-provider",
		Tools:          []string{"contract.write"},
		PermissionMode: jfadk.PermissionModeApproval,
		Status:         jfadk.AgentStatusEnabled,
	})
	if err != nil {
		t.Fatalf("SaveAgent: %v", err)
	}
	chat := performAssistantRequest(
		router,
		http.MethodPost,
		"/api/v1/adk/chat",
		[]byte(`{"agentId":"`+agent.ID+`","message":"@contract.write save"}`),
	)
	var envelope struct {
		OK   bool `json:"ok"`
		Data struct {
			PendingApprovals []jfadk.Approval `json:"pendingApprovals"`
		} `json:"data"`
	}
	if err := json.Unmarshal(chat.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("decode chat: %v", err)
	}
	if !envelope.OK || len(envelope.Data.PendingApprovals) != 1 {
		t.Fatalf("chat envelope=%s", chat.Body.String())
	}
	approvalID := envelope.Data.PendingApprovals[0].ID

	list := performAssistantRequest(router, http.MethodGet, "/api/v1/adk/approvals?status=PENDING", nil)
	if list.Code != http.StatusOK {
		t.Fatalf("approval list status=%d body=%s", list.Code, list.Body.String())
	}
	resolve := performAssistantRequest(router, http.MethodPost, "/api/v1/adk/approvals/"+approvalID+"/deny", nil)
	if resolve.Code != http.StatusOK {
		t.Fatalf("approval deny status=%d body=%s", resolve.Code, resolve.Body.String())
	}
	assertOKEnvelope(t, resolve)
}

func newAssistantTestRouter(t *testing.T) (*jfadk.Runtime, *gin.Engine) {
	t.Helper()
	root := t.TempDir()
	store, err := jfadk.NewStore(
		filepath.Join(root, "adk.db"),
		filepath.Join(root, "secrets.json"),
		filepath.Join(root, "skills"),
	)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	runtime := jfadk.NewRuntime(store, jfadk.NewToolRegistry())
	assistantTestProvider(t, runtime)
	t.Cleanup(func() {
		jftradeErr2 := runtime.Close()
		jftradeCheckTestError(t, jftradeErr2)
	})
	service := assistantservice.NewService(
		runtime,
		assistantservice.WithRuntimeSettings(func() any {
			return map[string]any{"runTimeoutMs": 720000, "streamIdleTimeoutMs": 420000}
		}),
		assistantservice.WithStreamIdleTimeout(func() int { return 420000 }),
	)
	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	api := router.Group("/api/v1")
	RegisterRoutes(api, service)
	return runtime, router
}

func assistantTestProvider(t *testing.T, runtime *jfadk.Runtime) {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() { jftradeCheckTestError(t, r.Body.Close()) }()
		var payload struct {
			Messages []struct {
				Role    string `json:"role"`
				Content string `json:"content"`
				Name    string `json:"name"`
			} `json:"messages"`
		}
		jftradeErr1 := json.NewDecoder(r.Body).Decode(&payload)
		jftradeCheckTestError(t, jftradeErr1)
		hasToolResponse := false
		var text string
		for _, message := range payload.Messages {
			if message.Role == "tool" {
				hasToolResponse = true
			}
			text += "\n" + message.Content
		}
		message := map[string]any{"role": "assistant", "content": "ok"}
		if !hasToolResponse && strings.Contains(text, "@contract.write") {
			message["content"] = ""
			message["tool_calls"] = []map[string]any{{
				"id": "call-contract-write", "type": "function",
				"function": map[string]any{"name": "contract-write", "arguments": `{}`},
			}}
		}
		w.Header().Set("Content-Type", "application/json")
		jftradeErr2 := json.NewEncoder(w).Encode(map[string]any{"choices": []map[string]any{{"message": message}}})
		jftradeCheckTestError(t, jftradeErr2)
	}))
	t.Cleanup(server.Close)
	if _, err := runtime.Store().SaveProvider(t.Context(), jfadk.ProviderWriteRequest{
		ID: "test-provider", DisplayName: "Test Provider", BaseURL: server.URL, Model: "test-model", APIKey: "sk-test", Enabled: true,
	}); err != nil {
		t.Fatalf("SaveProvider test: %v", err)
	}
}

func performAssistantRequest(router http.Handler, method string, path string, body []byte) *httptest.ResponseRecorder {
	var reader *bytes.Reader
	if body == nil {
		reader = bytes.NewReader(nil)
	} else {
		reader = bytes.NewReader(body)
	}
	request := httptest.NewRequestWithContext(context.Background(), method, path, reader)
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	return recorder
}

func assertOKEnvelope(t *testing.T, recorder *httptest.ResponseRecorder) {
	t.Helper()
	var envelope struct {
		OK bool `json:"ok"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("decode envelope: %v body=%s", err, recorder.Body.String())
	}
	if !envelope.OK {
		t.Fatalf("envelope not ok: %s", recorder.Body.String())
	}
}

func jftradeCheckTestError(t testing.TB, err error) {
	t.Helper()
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}
