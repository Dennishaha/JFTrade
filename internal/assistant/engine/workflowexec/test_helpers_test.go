package workflowexec

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	jfadk "github.com/jftrade/jftrade-main/internal/assistant/engine"
	enginepersistence "github.com/jftrade/jftrade-main/internal/assistant/engine/persistence"
	"github.com/jftrade/jftrade-main/internal/assistant/engine/providers"
	adksession "google.golang.org/adk/v2/session"
	"google.golang.org/genai"
)

const testProviderID = "test-openai-compatible"

type fakeWorkflowExecutionHandle struct {
	jfadk.WorkflowExecutionHandle
	calls             []ToolCall
	summaries         []string
	reply             string
	runErr            error
	postTool          bool
	finalSynthesisErr error
}

func (f *fakeWorkflowExecutionHandle) ToolContextForRun(runID string) jfadk.ToolExecutionContext {
	return jfadk.ToolExecutionContext{Calls: f.calls, Summaries: f.summaries}
}

func (f *fakeWorkflowExecutionHandle) ResultForRun(runID string) jfadk.AssistantExecutionResult {
	return jfadk.AssistantExecutionResult{Reply: f.reply}
}

func (f *fakeWorkflowExecutionHandle) HasToolCallsForRun(runID string) bool {
	return len(f.calls) > 0
}

func (f *fakeWorkflowExecutionHandle) Run(ctx context.Context, content *genai.Content) error {
	return f.runErr
}

func (f *fakeWorkflowExecutionHandle) RunNeedsFinalSynthesis(runID string) bool {
	return len(f.calls) > 0 && !f.postTool
}

func (f *fakeWorkflowExecutionHandle) RunHasPostToolText(runID string) bool {
	return f.postTool
}

func (f *fakeWorkflowExecutionHandle) HasFinalReplyForRun(runID string, visibleReply string) bool {
	return f.postTool && len(f.reply) > 0
}

func (f *fakeWorkflowExecutionHandle) WorkflowRunObserved(runID string) bool {
	return false
}

func (f *fakeWorkflowExecutionHandle) RunGoogleADKWorkflowChildFinalSynthesis(
	ctx context.Context,
	_ jfadk.Agent,
	_ jfadk.Session,
	_ jfadk.Run,
) error {
	if f.finalSynthesisErr != nil {
		return f.finalSynthesisErr
	}
	f.postTool = true
	return nil
}

func mustSaveRun(t *testing.T, runtime *jfadk.Runtime, run Run) Run {
	t.Helper()
	if err := runtime.Store().SaveRun(context.Background(), run); err != nil {
		t.Fatalf("SaveRun: %v", err)
	}
	return run
}

func mustCreateSession(t *testing.T, runtime *jfadk.Runtime, agentID string, title string) Session {
	t.Helper()
	session, err := runtime.Store().CreateSession(context.Background(), agentID, title)
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	return session
}

func mustSaveAgent(t *testing.T, runtime *jfadk.Runtime, req jfadk.AgentWriteRequest) Agent {
	t.Helper()
	if strings.TrimSpace(req.ProviderID) == "" {
		req.ProviderID = testProviderID
	}
	agent, err := runtime.Store().SaveAgent(context.Background(), req)
	if err != nil {
		t.Fatalf("SaveAgent: %v", err)
	}
	return agent
}

func mustSaveProvider(t *testing.T, runtime *jfadk.Runtime, req jfadk.ProviderWriteRequest) Provider {
	t.Helper()
	provider, err := runtime.Store().SaveProvider(context.Background(), req)
	if err != nil {
		t.Fatalf("SaveProvider: %v", err)
	}
	return provider
}

func saveGoalWorkflowProvider(t *testing.T, runtime *jfadk.Runtime, providerID string, responder func(providers.OpenAIChatRequest) providers.OpenAIChatMessage) string {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || !strings.HasSuffix(r.URL.Path, "/chat/completions") {
			http.NotFound(w, r)
			return
		}
		defer func() { _ = r.Body.Close() }()
		var req providers.OpenAIChatRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(providers.OpenAIChatResponse{Choices: []struct {
			Message providers.OpenAIChatMessage `json:"message"`
		}{{Message: responder(req)}}})
	}))
	t.Cleanup(server.Close)
	mustSaveProvider(t, runtime, ProviderWriteRequest{
		ID: providerID, DisplayName: providerID, BaseURL: server.URL, Model: "test-model", APIKey: "sk-test", Enabled: true,
	})
	return providerID
}

func newRuntimeWithRegistry(t *testing.T, store *jfadk.Store, registry *jfadk.ToolRegistry) *jfadk.Runtime {
	t.Helper()
	sessionService, err := enginepersistence.NewSQLiteSessionService(filepath.Join(filepath.Dir(store.SkillsPath()), "adk-session.db"))
	if err != nil {
		t.Fatalf("NewSQLiteSessionService: %v", err)
	}
	if err := enginepersistence.ValidateSQLiteSessionService(sessionService); err != nil {
		t.Fatalf("ValidateSQLiteSessionService: %v", err)
	}
	runtime := jfadk.NewRuntimeWithSessionService(store, registry, sessionService)
	runtime.SetWorkflowExecutor(NewWorkflowExecutor(runtime))
	t.Cleanup(func() {
		_ = enginepersistence.CloseSessionService(sessionService)
	})
	t.Cleanup(func() { _ = runtime.Close() })
	return runtime
}

func newWorkflowApprovalRuntime(t *testing.T, mode string) (*jfadk.Runtime, *atomic.Int64) {
	t.Helper()
	base := newTestRuntime(t)
	registry := jfadk.NewToolRegistry()
	executions := &atomic.Int64{}
	registry.Register(jfadk.ToolDescriptor{
		Name:               "approval.required",
		DisplayName:        "Approval Required",
		Description:        "test approval tool",
		Category:           "strategy",
		Permission:         "write_strategy",
		AllowedModes:       []string{PermissionModeApproval, PermissionModeLessApproval, PermissionModeAll},
		RequiresApprovalIn: []string{PermissionModeApproval},
	}, func(context.Context, map[string]any) (any, error) {
		executions.Add(1)
		return map[string]any{"saved": true, "mode": mode}, nil
	})
	runtime := newRuntimeWithRegistry(t, base.Store(), registry)
	return runtime, executions
}

func installRunUpdateRejectTrigger(t *testing.T, runtime *jfadk.Runtime, runID string, triggerName string) {
	t.Helper()
	if _, err := runtime.Store().DB().ExecContext(context.Background(), `
		CREATE TRIGGER `+triggerName+`
		BEFORE UPDATE ON `+enginepersistence.TableRuns+`
		WHEN OLD.id = '`+runID+`'
		BEGIN SELECT RAISE(FAIL, '`+triggerName+`'); END
	`); err != nil {
		t.Fatalf("create %s trigger: %v", triggerName, err)
	}
}

func installFailTrigger(t *testing.T, runtime *jfadk.Runtime, name string, tableName string, op string, message string) {
	t.Helper()
	sql := `CREATE TRIGGER ` + name + ` BEFORE ` + op + ` ON ` + tableName + ` BEGIN SELECT RAISE(FAIL, '` + message + `'); END`
	if _, err := runtime.Store().DB().ExecContext(t.Context(), sql); err != nil {
		t.Fatalf("create trigger %s: %v", name, err)
	}
}

type failGetSessionService struct {
	adksession.Service
	err error
}

func (service failGetSessionService) Get(context.Context, *adksession.GetRequest) (*adksession.GetResponse, error) {
	return nil, service.err
}

func (service failGetSessionService) Create(context.Context, *adksession.CreateRequest) (*adksession.CreateResponse, error) {
	return nil, service.err
}

type failAfterSessionService struct {
	adksession.Service
	mu   sync.Mutex
	fail bool
}

func (service *failAfterSessionService) Get(ctx context.Context, req *adksession.GetRequest) (*adksession.GetResponse, error) {
	service.mu.Lock()
	fail := service.fail
	service.mu.Unlock()
	if fail {
		return nil, errors.New("assistant message storage unavailable")
	}
	return service.Service.Get(ctx, req)
}

func (service *failAfterSessionService) Create(ctx context.Context, req *adksession.CreateRequest) (*adksession.CreateResponse, error) {
	service.mu.Lock()
	fail := service.fail
	service.mu.Unlock()
	if fail {
		return nil, errors.New("assistant message storage unavailable")
	}
	return service.Service.Create(ctx, req)
}
