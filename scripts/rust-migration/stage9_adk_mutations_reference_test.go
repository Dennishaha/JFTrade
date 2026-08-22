package rustmigration

import (
	"bytes"
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
	assistantmodel "github.com/jftrade/jftrade-main/internal/assistant/model"
	assistanttestkit "github.com/jftrade/jftrade-main/internal/assistant/testkit"
	adksession "google.golang.org/adk/v2/session"
)

const (
	stage9ADKMutationsFixtureVersion = "stage9.adk-mutations.v1"
	stage9ADKMutationsTimestamp      = "2026-08-23T08:00:00Z"
)

type stage9ADKMutationsFixture struct {
	Version   string                          `json:"version"`
	Timestamp string                          `json:"timestamp"`
	Cases     []stage9ADKMutationsFixtureCase `json:"cases"`
}

type stage9ADKMutationsFixtureCase struct {
	Name        string                          `json:"name"`
	Method      string                          `json:"method"`
	RequestPath string                          `json:"requestPath"`
	Body        *string                         `json:"body,omitempty"`
	Headers     map[string]string               `json:"headers,omitempty"`
	Expected    stage9ADKMutationsFixtureResult `json:"expected"`
}

type stage9ADKMutationsFixtureResult struct {
	Status   int               `json:"status"`
	Headers  map[string]string `json:"headers"`
	PortCall bool              `json:"portCall"`
	Envelope map[string]any    `json:"envelope"`
}

type stage9ADKMutationCaseSpec struct {
	Name        string
	Method      string
	RequestPath string
	Body        *string
	Headers     map[string]string
	Setup       string
	PortCall    bool
}

// TestStage9ADKMutationsFixtureMatchesCurrentGoOwner freezes all remaining
// Assistant mutation/control route projections. Each case uses a temporary
// ADK store and in-memory session service; no production runtime, provider,
// notification, or external skill source is used.
func TestStage9ADKMutationsFixtureMatchesCurrentGoOwner(t *testing.T) {
	_, source, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve stage 9 ADK mutation fixture source")
	}
	fixturePath := filepath.Join(
		filepath.Dir(source), "../../tests/fixtures/rust-migration/stage9/adk-mutations.json",
	)
	want := stage9ADKMutationsFixture{
		Version:   stage9ADKMutationsFixtureVersion,
		Timestamp: stage9ADKMutationsTimestamp,
		Cases:     make([]stage9ADKMutationsFixtureCase, 0, len(stage9ADKMutationCaseSpecs())),
	}
	for _, spec := range stage9ADKMutationCaseSpecs() {
		want.Cases = append(want.Cases, runStage9ADKMutationCase(t, spec))
	}
	if os.Getenv("JFTRADE_UPDATE_RUST_MIGRATION_FIXTURES") == "1" {
		contents, err := json.MarshalIndent(want, "", "  ")
		if err != nil {
			t.Fatalf("encode ADK mutation fixture: %v", err)
		}
		if err := os.WriteFile(fixturePath, append(contents, '\n'), 0o644); err != nil {
			t.Fatalf("write ADK mutation fixture: %v", err)
		}
	}
	contents, err := os.ReadFile(fixturePath)
	if err != nil {
		t.Fatalf("read ADK mutation fixture: %v", err)
	}
	var got stage9ADKMutationsFixture
	if err := json.Unmarshal(contents, &got); err != nil {
		t.Fatalf("decode ADK mutation fixture: %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("stage 9 ADK mutation fixture drifted from the Go owner")
	}
}

func stage9ADKMutationCaseSpecs() []stage9ADKMutationCaseSpec {
	body := func(value string) *string { return &value }
	return []stage9ADKMutationCaseSpec{
		{Name: "agent-delete", Method: http.MethodDelete, RequestPath: "/api/v1/adk/agents/delete-agent", Setup: "agent-delete", PortCall: true},
		{Name: "memory-delete", Method: http.MethodDelete, RequestPath: "/api/v1/adk/memory/fixture-memory", Setup: "memory-delete", PortCall: true},
		{Name: "provider-delete", Method: http.MethodDelete, RequestPath: "/api/v1/adk/providers/delete-provider", Setup: "provider-delete", PortCall: true},
		{Name: "session-delete", Method: http.MethodDelete, RequestPath: "/api/v1/adk/sessions/fixture-session", Setup: "session-delete", PortCall: true},
		{Name: "skill-delete-missing", Method: http.MethodDelete, RequestPath: "/api/v1/adk/skills/missing-skill", PortCall: true},
		{Name: "task-delete", Method: http.MethodDelete, RequestPath: "/api/v1/adk/tasks/delete-task", Setup: "task-delete", PortCall: true},
		{Name: "workflow-delete", Method: http.MethodDelete, RequestPath: "/api/v1/adk/workflows/delete-workflow", Setup: "workflow-delete", PortCall: true},
		{Name: "workflow-trigger-delete", Method: http.MethodDelete, RequestPath: "/api/v1/adk/workflows/trigger-delete-workflow/triggers/trigger-delete", Setup: "trigger-delete", PortCall: true},
		{Name: "run-objective-update", Method: http.MethodPatch, RequestPath: "/api/v1/adk/runs/objective-run/objective", Body: body(`{"objective":"updated fixture objective"}`), Setup: "run-objective", PortCall: true},
		{Name: "session-composer-update", Method: http.MethodPatch, RequestPath: "/api/v1/adk/sessions/fixture-session/composer-state", Body: body(`{"chatDraft":"fixture draft","workModeOverride":"loop","permissionModeOverride":"less_approval","goalObjectiveDraft":"fixture goal","goalObjectiveTouched":true}`), Setup: "session-composer", PortCall: true},
		{Name: "agent-create", Method: http.MethodPost, RequestPath: "/api/v1/adk/agents", Body: body(`{"id":"created-agent","name":"Created Agent","instruction":"Be useful","providerId":"fixture-provider","permissionMode":"less_approval","status":"ENABLED"}`), PortCall: true},
		{Name: "approval-approve", Method: http.MethodPost, RequestPath: "/api/v1/adk/approvals/approval-approve/approve", Setup: "approval-approve", PortCall: true},
		{Name: "approval-deny", Method: http.MethodPost, RequestPath: "/api/v1/adk/approvals/approval-deny/deny", Setup: "approval-deny", PortCall: true},
		{Name: "memory-create", Method: http.MethodPost, RequestPath: "/api/v1/adk/memory", Body: body(`{"scope":"agent","agentId":"fixture-agent","key":"risk","value":"risk first"}`), PortCall: true},
		{Name: "optimization-cancel", Method: http.MethodPost, RequestPath: "/api/v1/adk/optimization-tasks/optimization-task/cancel", Setup: "optimization-cancel", PortCall: true},
		{Name: "provider-create", Method: http.MethodPost, RequestPath: "/api/v1/adk/providers", Body: body(`{"id":"created-provider","displayName":"Created Provider","baseUrl":"http://127.0.0.1:1/v1","model":"fixture-model","apiKey":"fixture-key","enabled":true}`), PortCall: true},
		{Name: "provider-set-default", Method: http.MethodPost, RequestPath: "/api/v1/adk/providers/default-provider/default", Setup: "provider-default", PortCall: true},
		{Name: "provider-test-missing", Method: http.MethodPost, RequestPath: "/api/v1/adk/providers/missing-provider/test", Body: body(`{"mode":"quick"}`), PortCall: true},
		{Name: "run-cancel", Method: http.MethodPost, RequestPath: "/api/v1/adk/runs/cancel-run/cancel", Setup: "run-cancel", PortCall: true},
		{Name: "run-input-response", Method: http.MethodPost, RequestPath: "/api/v1/adk/runs/input-run/input-response", Body: body(`{"requestId":"input-request","answers":[{"questionId":"question-1","optionId":"option-yes"}]}`), Setup: "run-input", PortCall: true},
		{Name: "run-pause", Method: http.MethodPost, RequestPath: "/api/v1/adk/runs/pause-run/pause", Setup: "run-pause", PortCall: true},
		{Name: "run-resume", Method: http.MethodPost, RequestPath: "/api/v1/adk/runs/resume-run/resume", Setup: "run-resume", PortCall: true},
		{Name: "session-create", Method: http.MethodPost, RequestPath: "/api/v1/adk/sessions", Body: body(`{"agentId":"fixture-agent","title":"Created fixture session"}`), PortCall: true},
		{Name: "session-context-compact", Method: http.MethodPost, RequestPath: "/api/v1/adk/sessions/fixture-session/context/compact", Body: body(`{"mode":"summary","reason":"fixture compaction"}`), Setup: "session-compact", PortCall: true},
		{Name: "skill-install-invalid-source", Method: http.MethodPost, RequestPath: "/api/v1/adk/skills", Body: body(`{"url":"not-a-url"}`), PortCall: true},
		{Name: "task-create", Method: http.MethodPost, RequestPath: "/api/v1/adk/tasks", Body: body(`{"id":"created-task","title":"Created task","status":"TODO","agentId":"fixture-agent","order":1}`), PortCall: true},
		{Name: "workflow-trigger-run-missing", Method: http.MethodPost, RequestPath: "/api/v1/adk/workflow-triggers/missing-trigger/run", Body: body(`{"inputs":{"source":"fixture"}}`), PortCall: true},
		{Name: "workflow-webhook-disabled", Method: http.MethodPost, RequestPath: "/api/v1/adk/workflow-webhooks/disabled-webhook", Body: body(`{"inputs":{"source":"fixture"}}`), Setup: "webhook-disabled", PortCall: true},
		{Name: "workflow-create", Method: http.MethodPost, RequestPath: "/api/v1/adk/workflows", Body: stage9ADKWorkflowBody("created-workflow", "Created workflow"), PortCall: true},
		{Name: "workflow-run-disabled", Method: http.MethodPost, RequestPath: "/api/v1/adk/workflows/disabled-workflow/run", Body: body(`{"inputs":{"source":"fixture"}}`), Setup: "workflow-run-disabled", PortCall: true},
		{Name: "workflow-trigger-create", Method: http.MethodPost, RequestPath: "/api/v1/adk/workflows/trigger-create-workflow/triggers", Body: body(`{"id":"created-trigger","type":"manual","title":"Created trigger","status":"DISABLED","config":{"source":"fixture"}}`), Setup: "trigger-create", PortCall: true},
		{Name: "agent-update", Method: http.MethodPut, RequestPath: "/api/v1/adk/agents/fixture-agent", Body: body(`{"name":"Updated Fixture Agent","instruction":"Updated instruction","providerId":"fixture-provider","permissionMode":"less_approval","status":"ENABLED"}`), Setup: "agent-update", PortCall: true},
		{Name: "provider-update", Method: http.MethodPut, RequestPath: "/api/v1/adk/providers/fixture-provider", Body: body(`{"displayName":"Updated Fixture Provider","baseUrl":"http://127.0.0.1:1/v1","model":"updated-model","enabled":true}`), Setup: "provider-update", PortCall: true},
		{Name: "session-rename", Method: http.MethodPut, RequestPath: "/api/v1/adk/sessions/fixture-session", Body: body(`{"title":"Renamed fixture session"}`), Setup: "session-rename", PortCall: true},
		{Name: "task-update", Method: http.MethodPut, RequestPath: "/api/v1/adk/tasks/update-task", Body: body(`{"status":"DONE","order":2,"plannerWarnings":["fixture warning"]}`), Setup: "task-update", PortCall: true},
		{Name: "workflow-update", Method: http.MethodPut, RequestPath: "/api/v1/adk/workflows/update-workflow", Body: stage9ADKWorkflowBody("update-workflow", "Updated workflow"), Setup: "workflow-update", PortCall: true},
		{Name: "workflow-trigger-update", Method: http.MethodPut, RequestPath: "/api/v1/adk/workflows/update-trigger-workflow/triggers/update-trigger", Body: body(`{"type":"manual","title":"Updated trigger","status":"DISABLED","config":{"source":"updated"}}`), Setup: "trigger-update", PortCall: true},
		{Name: "agent-malformed-body", Method: http.MethodPost, RequestPath: "/api/v1/adk/agents", Body: body("{"), PortCall: false},
		{Name: "session-create-empty-body", Method: http.MethodPost, RequestPath: "/api/v1/adk/sessions", PortCall: false},
		{Name: "run-objective-blank-id", Method: http.MethodPatch, RequestPath: "/api/v1/adk/runs/%20/objective", Body: body(`{"objective":"fixture"}`), PortCall: false},
	}
}

func stage9ADKWorkflowBody(id string, name string) *string {
	body := `{"id":"` + id + `","name":"` + name + `","status":"DISABLED","agentId":"fixture-agent","workMode":"chat","promptTemplate":"Review {{ .symbol }}","defaultInputs":{"symbol":"US.AAPL"}}`
	return &body
}

func runStage9ADKMutationCase(
	t *testing.T,
	spec stage9ADKMutationCaseSpec,
) stage9ADKMutationsFixtureCase {
	t.Helper()
	router, store, cleanup := stage9ADKMutationRouter(t)
	defer cleanup()
	replacements := seedStage9ADKMutationCase(t, store, spec.Setup)
	requestPath := stage9Substitute(spec.RequestPath, replacements)
	var reader *bytes.Reader
	if spec.Body == nil {
		reader = bytes.NewReader(nil)
	} else {
		reader = bytes.NewReader([]byte(stage9Substitute(*spec.Body, replacements)))
	}
	request := httptest.NewRequestWithContext(t.Context(), spec.Method, requestPath, reader)
	if spec.Body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	for key, value := range spec.Headers {
		request.Header.Set(key, value)
	}
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	var envelope map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("case %s decode response: %v body=%q", spec.Name, err, recorder.Body.String())
	}
	normalizeStage9ADKMutationValue("", envelope, replacements)
	if spec.Name == "session-create" {
		normalizeStage9ADKCreatedSessionID(t, envelope)
	}
	return stage9ADKMutationsFixtureCase{
		Name:        spec.Name,
		Method:      spec.Method,
		RequestPath: spec.RequestPath,
		Body:        spec.Body,
		Headers:     spec.Headers,
		Expected: stage9ADKMutationsFixtureResult{
			Status:   recorder.Code,
			Headers:  stage9ADKMutationHeaders(recorder.Header()),
			PortCall: spec.PortCall,
			Envelope: envelope,
		},
	}
}

func normalizeStage9ADKCreatedSessionID(t *testing.T, envelope map[string]any) {
	t.Helper()
	data, ok := envelope["data"].(map[string]any)
	if !ok {
		t.Fatalf("session-create response has no data object")
	}
	id, ok := data["id"].(string)
	if !ok || !strings.HasPrefix(id, "session-") {
		t.Fatalf("session-create response has unexpected generated id %q", data["id"])
	}
	data["id"] = "fixture-session-created"
}

func stage9ADKMutationRouter(t *testing.T) (*gin.Engine, *assistanttestkit.Store, func()) {
	t.Helper()
	directory := t.TempDir()
	store, err := assistanttestkit.NewStore(
		filepath.Join(directory, "adk.db"),
		filepath.Join(directory, "secrets", "adk-secrets.json"),
		filepath.Join(directory, "skills"),
	)
	if err != nil {
		t.Fatalf("open ADK mutation fixture store: %v", err)
	}
	if _, err := store.SaveProvider(t.Context(), assistantmodel.ProviderWriteRequest{
		ID: "fixture-provider", DisplayName: "Fixture Provider", BaseURL: "http://127.0.0.1:1/v1",
		Model: "fixture-model", APIKey: "fixture-key", Enabled: true,
	}); err != nil {
		t.Fatalf("save fixture provider: %v", err)
	}
	if _, err := store.SaveAgent(t.Context(), assistantmodel.AgentWriteRequest{
		ID: "fixture-agent", Name: "Fixture Agent", Instruction: "Be useful",
		ProviderID: "fixture-provider", PermissionMode: assistantmodel.PermissionModeLessApproval,
		Status: assistantmodel.AgentStatusEnabled, WorkMode: assistantmodel.WorkModeChat,
	}); err != nil {
		t.Fatalf("save fixture agent: %v", err)
	}
	runtime := assistanttestkit.NewRuntimeWithSessionService(
		store,
		assistanttestkit.NewToolRegistry(),
		adksession.InMemoryService(),
	)
	service := assistantservice.NewService(runtime)
	router := gin.New()
	handler := assistantapi.RegisterRoutes(router.Group("/api/v1"), service)
	cleanup := func() {
		if err := handler.Close(); err != nil {
			t.Errorf("close ADK mutation handler: %v", err)
		}
		if err := service.Close(); err != nil {
			t.Errorf("close ADK mutation service: %v", err)
		}
	}
	return router, store, cleanup
}

func seedStage9ADKMutationCase(t *testing.T, store *assistanttestkit.Store, setup string) map[string]string {
	t.Helper()
	ctx := t.Context()
	replacements := map[string]string{}
	saveAgent := func(id string) {
		_, err := store.SaveAgent(ctx, assistantmodel.AgentWriteRequest{
			ID: id, Name: "Fixture Agent " + id, Instruction: "Be useful",
			ProviderID: "fixture-provider", PermissionMode: assistantmodel.PermissionModeLessApproval,
			Status: assistantmodel.AgentStatusEnabled, WorkMode: assistantmodel.WorkModeChat,
		})
		if err != nil {
			t.Fatalf("save agent %s: %v", id, err)
		}
	}
	saveSession := func(placeholder string) {
		session, err := store.CreateSession(ctx, "fixture-agent", "Fixture session")
		if err != nil {
			t.Fatalf("create session: %v", err)
		}
		replacements[placeholder] = session.ID
	}
	saveWorkflow := func(id string, status string) {
		_, err := store.SaveWorkflowDefinition(ctx, assistantmodel.WorkflowDefinition{
			ID: id, Name: "Fixture workflow " + id, Status: status, AgentID: "fixture-agent",
			WorkMode: assistantmodel.WorkModeChat, PromptTemplate: "Review {{ .symbol }}",
			DefaultInputs: map[string]any{"symbol": "US.AAPL"},
		})
		if err != nil {
			t.Fatalf("save workflow %s: %v", id, err)
		}
	}
	saveRun := func(run assistantmodel.Run) {
		if err := store.SaveRun(ctx, run); err != nil {
			t.Fatalf("save run %s: %v", run.ID, err)
		}
	}
	now := stage9ADKMutationsTimestamp
	switch setup {
	case "agent-delete":
		saveAgent("delete-agent")
	case "memory-delete":
		memory, err := store.SaveMemory(ctx, assistantmodel.MemoryWriteRequest{
			AgentID: "fixture-agent", Key: "delete-key", Value: "delete-value", Scope: "agent",
		})
		if err != nil {
			t.Fatalf("save memory: %v", err)
		}
		replacements["fixture-memory"] = memory.ID
	case "provider-delete":
		if _, err := store.SaveProvider(ctx, assistantmodel.ProviderWriteRequest{
			ID: "delete-provider", DisplayName: "Delete Provider", BaseURL: "http://127.0.0.1:1/v1",
			Model: "fixture-model", Enabled: true,
		}); err != nil {
			t.Fatalf("save delete provider: %v", err)
		}
	case "session-delete", "session-composer", "session-compact", "session-rename":
		saveSession("fixture-session")
	case "task-delete":
		if _, err := store.SaveTask(ctx, assistantmodel.TaskWriteRequest{ID: "delete-task", Title: "Delete task", Status: "TODO"}); err != nil {
			t.Fatalf("save delete task: %v", err)
		}
	case "workflow-delete":
		saveWorkflow("delete-workflow", assistantmodel.WorkflowStatusDisabled)
	case "trigger-delete":
		saveWorkflow("trigger-delete-workflow", assistantmodel.WorkflowStatusDisabled)
		if _, err := store.SaveWorkflowTrigger(ctx, assistantmodel.WorkflowTrigger{
			ID: "trigger-delete", WorkflowID: "trigger-delete-workflow", Type: assistantmodel.WorkflowTriggerTypeManual,
			Title: "Delete trigger", Status: assistantmodel.WorkflowTriggerStatusDisabled,
		}); err != nil {
			t.Fatalf("save delete trigger: %v", err)
		}
	case "run-objective":
		saveRun(assistantmodel.Run{ID: "objective-run", SessionID: "fixture-session", AgentID: "fixture-agent", Status: assistantmodel.RunStatusRunning, WorkMode: assistantmodel.WorkModeLoop, WorkflowStatus: "RUNNING", Objective: "old objective", Message: "running", ToolCalls: []assistantmodel.ToolCall{}, PendingApprovals: []assistantmodel.Approval{}, CreatedAt: now, UpdatedAt: now})
	case "agent-update":
	case "approval-approve", "approval-deny":
		approvalID := setup
		runID := setup
		approval := assistantmodel.Approval{ID: approvalID, RunID: runID, AgentID: "fixture-agent", ToolName: "fixture.tool", Status: assistantmodel.ApprovalStatusPending, Reason: "fixture approval", FunctionCallID: "function-" + setup, ConfirmationCallID: "confirmation-" + setup, CreatedAt: now, UpdatedAt: now}
		saveRun(assistantmodel.Run{ID: runID, SessionID: "fixture-session", AgentID: "fixture-agent", Status: assistantmodel.RunStatusPending, Message: "waiting approval", PendingApprovals: []assistantmodel.Approval{approval}, ToolCalls: []assistantmodel.ToolCall{}, CreatedAt: now, UpdatedAt: now})
		if err := store.SaveApproval(ctx, approval); err != nil {
			t.Fatalf("save approval %s: %v", approvalID, err)
		}
	case "optimization-cancel":
		if _, err := store.SaveOptimizationTask(ctx, assistantmodel.OptimizationTask{ID: "optimization-task", Status: "queued", Objective: "fixture objective", Runs: []assistantmodel.OptimizationRunRef{{DefinitionID: "definition-1", RunID: "missing-run"}}}); err != nil {
			t.Fatalf("save optimization task: %v", err)
		}
	case "provider-default":
		if _, err := store.SaveProvider(ctx, assistantmodel.ProviderWriteRequest{ID: "default-provider", DisplayName: "Default Provider", BaseURL: "http://127.0.0.1:1/v1", Model: "fixture-model", Enabled: true}); err != nil {
			t.Fatalf("save default provider: %v", err)
		}
	case "run-cancel":
		saveRun(assistantmodel.Run{ID: "cancel-run", SessionID: "fixture-session", AgentID: "fixture-agent", Status: assistantmodel.RunStatusPending, Message: "pending", ToolCalls: []assistantmodel.ToolCall{}, PendingApprovals: []assistantmodel.Approval{}, CreatedAt: now, UpdatedAt: now})
	case "run-input":
		input := assistantmodel.InputRequest{ID: "input-request", RunID: "input-run", AgentID: "fixture-agent", FunctionCallID: "input-function", Title: "Fixture question", Status: assistantmodel.InputRequestStatusPending, Questions: []assistantmodel.InputQuestion{{ID: "question-1", Question: "Continue?", Options: []assistantmodel.InputOption{{ID: "option-yes", Label: "Yes"}}, AllowOther: false}}, CreatedAt: now, UpdatedAt: now}
		saveRun(assistantmodel.Run{ID: "input-run", SessionID: "fixture-session", AgentID: "fixture-agent", Status: assistantmodel.RunStatusPendingInput, Message: "waiting input", InputRequest: &input, InputRequests: []assistantmodel.InputRequest{input}, ToolCalls: []assistantmodel.ToolCall{}, PendingApprovals: []assistantmodel.Approval{}, CreatedAt: now, UpdatedAt: now})
	case "run-pause":
		saveRun(assistantmodel.Run{ID: "pause-run", SessionID: "fixture-session", AgentID: "fixture-agent", Status: assistantmodel.RunStatusRunning, Message: "goal running", WorkMode: assistantmodel.WorkModeLoop, WorkflowStatus: "RUNNING", ToolCalls: []assistantmodel.ToolCall{}, PendingApprovals: []assistantmodel.Approval{}, CreatedAt: now, UpdatedAt: now})
	case "run-resume":
		pausedAt := now
		saveRun(assistantmodel.Run{ID: "resume-run", SessionID: "fixture-session", AgentID: "fixture-agent", Status: assistantmodel.RunStatusPaused, Message: "paused", WorkMode: assistantmodel.WorkModeLoop, WorkflowStatus: "PAUSED", ResumeState: "user_paused", PausedAt: &pausedAt, PausedReason: "user", ToolCalls: []assistantmodel.ToolCall{}, PendingApprovals: []assistantmodel.Approval{}, CreatedAt: now, UpdatedAt: now})
	case "trigger-create":
		saveWorkflow("trigger-create-workflow", assistantmodel.WorkflowStatusDisabled)
	case "webhook-disabled":
		saveWorkflow("disabled-webhook-workflow", assistantmodel.WorkflowStatusDisabled)
		if _, err := store.SaveWorkflowTrigger(ctx, assistantmodel.WorkflowTrigger{ID: "disabled-webhook", WorkflowID: "disabled-webhook-workflow", Type: assistantmodel.WorkflowTriggerTypeWebhook, Title: "Disabled webhook", Status: assistantmodel.WorkflowTriggerStatusDisabled}); err != nil {
			t.Fatalf("save disabled webhook: %v", err)
		}
	case "workflow-run-disabled":
		saveWorkflow("disabled-workflow", assistantmodel.WorkflowStatusDisabled)
	case "trigger-update":
		saveWorkflow("update-trigger-workflow", assistantmodel.WorkflowStatusDisabled)
		if _, err := store.SaveWorkflowTrigger(ctx, assistantmodel.WorkflowTrigger{ID: "update-trigger", WorkflowID: "update-trigger-workflow", Type: assistantmodel.WorkflowTriggerTypeManual, Title: "Existing trigger", Status: assistantmodel.WorkflowTriggerStatusDisabled}); err != nil {
			t.Fatalf("save update trigger: %v", err)
		}
	case "task-update":
		if _, err := store.SaveTask(ctx, assistantmodel.TaskWriteRequest{ID: "update-task", Title: "Original task", Status: "TODO"}); err != nil {
			t.Fatalf("save update task: %v", err)
		}
	case "workflow-update":
		saveWorkflow("update-workflow", assistantmodel.WorkflowStatusDisabled)
	}
	return replacements
}

func stage9Substitute(value string, replacements map[string]string) string {
	for placeholder, actual := range replacements {
		value = strings.ReplaceAll(value, placeholder, actual)
	}
	return value
}

func normalizeStage9ADKMutationValue(key string, value any, replacements map[string]string) {
	switch typed := value.(type) {
	case map[string]any:
		for childKey, childValue := range typed {
			if childKey == "timestamp" || stage9ADKMutationTimestampKey(childKey) {
				typed[childKey] = stage9ADKMutationsTimestamp
				continue
			}
			if childKey == "contextRevisionId" {
				typed[childKey] = "fixture-context-revision"
				continue
			}
			if childKey == "secret" {
				typed[childKey] = "fixture-secret"
				continue
			}
			normalizeStage9ADKMutationValue(childKey, childValue, replacements)
		}
	case []any:
		for _, childValue := range typed {
			normalizeStage9ADKMutationValue(key, childValue, replacements)
		}
	case string:
		for placeholder, actual := range replacements {
			if typed == actual {
				_ = placeholder
				break
			}
		}
	}
	if object, ok := value.(map[string]any); ok {
		for placeholder, actual := range replacements {
			replaceStage9ADKMutationString(object, placeholder, actual)
		}
	}
}

func replaceStage9ADKMutationString(value any, placeholder string, actual string) {
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			if text, ok := child.(string); ok && text == actual {
				typed[key] = placeholder
				continue
			}
			replaceStage9ADKMutationString(child, placeholder, actual)
		}
	case []any:
		for _, child := range typed {
			replaceStage9ADKMutationString(child, placeholder, actual)
		}
	}
}

func stage9ADKMutationTimestampKey(key string) bool {
	switch key {
	case "createdAt", "updatedAt", "deletedAt", "startedAt", "finishedAt", "completedAt", "cancelledAt", "pauseRequestedAt", "pausedAt", "answeredAt", "nextRunAt", "lastRunAt", "checkedAt", "contextRevisionCreatedAt", "lastCompactedAt":
		return true
	default:
		return false
	}
}

func stage9ADKMutationHeaders(header http.Header) map[string]string {
	result := map[string]string{}
	if value := header.Get("Content-Type"); value != "" {
		result["Content-Type"] = value
	}
	return result
}
