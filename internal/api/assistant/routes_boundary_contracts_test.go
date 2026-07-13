package assistant

import (
	"net/http"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"

	assistantservice "github.com/jftrade/jftrade-main/internal/assistant"
	jfadk "github.com/jftrade/jftrade-main/pkg/adk"
)

func TestAssistantRoutesReturnUnavailableWhenRuntimeMissing(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	RegisterRoutes(router.Group("/api/v1"), assistantservice.NewService(nil))

	cases := []struct {
		name   string
		method string
		path   string
		body   []byte
	}{
		{"snapshot", http.MethodGet, "/api/v1/adk", nil},
		{"tools", http.MethodGet, "/api/v1/adk/tools", nil},
		{"agent templates", http.MethodGet, "/api/v1/adk/agent-templates", nil},
		{"audit", http.MethodGet, "/api/v1/adk/audit", nil},
		{"metrics", http.MethodGet, "/api/v1/adk/metrics", nil},
		{"workflows", http.MethodGet, "/api/v1/adk/workflows", nil},
		{"workflow", http.MethodGet, "/api/v1/adk/workflows/workflow-1", nil},
		{"save workflow", http.MethodPost, "/api/v1/adk/workflows", []byte(`{"name":"Missing Runtime"}`)},
		{"delete workflow", http.MethodDelete, "/api/v1/adk/workflows/workflow-1", nil},
		{"run workflow", http.MethodPost, "/api/v1/adk/workflows/workflow-1/run", []byte(`{"symbol":"US.AAPL"}`)},
		{"workflow triggers", http.MethodGet, "/api/v1/adk/workflows/workflow-1/triggers", nil},
		{"save workflow trigger", http.MethodPost, "/api/v1/adk/workflows/workflow-1/triggers", []byte(`{"type":"manual","title":"Manual"}`)},
		{"delete workflow trigger", http.MethodDelete, "/api/v1/adk/workflows/workflow-1/triggers/trigger-1", nil},
		{"run workflow trigger", http.MethodPost, "/api/v1/adk/workflow-triggers/trigger-1/run", nil},
		{"workflow logs", http.MethodGet, "/api/v1/adk/workflow-trigger-logs", nil},
		{"workflow webhook", http.MethodPost, "/api/v1/adk/workflow-webhooks/trigger-1", []byte(`{"symbol":"US.AAPL"}`)},
		{"tasks", http.MethodGet, "/api/v1/adk/tasks", nil},
		{"task", http.MethodGet, "/api/v1/adk/tasks/task-1", nil},
		{"save task", http.MethodPost, "/api/v1/adk/tasks", []byte(`{"title":"Task"}`)},
		{"delete task", http.MethodDelete, "/api/v1/adk/tasks/task-1", nil},
		{"memory", http.MethodGet, "/api/v1/adk/memory", nil},
		{"save memory", http.MethodPost, "/api/v1/adk/memory", []byte(`{"key":"note","value":"v","scope":"workspace"}`)},
		{"delete memory", http.MethodDelete, "/api/v1/adk/memory/memory-1", nil},
		{"optimization tasks", http.MethodGet, "/api/v1/adk/optimization-tasks", nil},
		{"optimization task", http.MethodGet, "/api/v1/adk/optimization-tasks/task-1", nil},
		{"optimization cancel", http.MethodPost, "/api/v1/adk/optimization-tasks/task-1/cancel", nil},
		{"providers", http.MethodGet, "/api/v1/adk/providers", nil},
		{"save provider", http.MethodPost, "/api/v1/adk/providers", []byte(`{"displayName":"Provider"}`)},
		{"test provider", http.MethodPost, "/api/v1/adk/providers/provider-1/test", nil},
		{"set default provider", http.MethodPost, "/api/v1/adk/providers/provider-1/default", nil},
		{"delete provider", http.MethodDelete, "/api/v1/adk/providers/provider-1", nil},
		{"agents", http.MethodGet, "/api/v1/adk/agents", nil},
		{"save agent", http.MethodPost, "/api/v1/adk/agents", []byte(`{"name":"Agent"}`)},
		{"delete agent", http.MethodDelete, "/api/v1/adk/agents/agent-1", nil},
		{"sessions", http.MethodGet, "/api/v1/adk/sessions", nil},
		{"create session", http.MethodPost, "/api/v1/adk/sessions", []byte(`{"agentId":"agent-1"}`)},
		{"session", http.MethodGet, "/api/v1/adk/sessions/session-1", nil},
		{"session context", http.MethodGet, "/api/v1/adk/sessions/session-1/context", nil},
		{"compact context", http.MethodPost, "/api/v1/adk/sessions/session-1/context/compact", []byte(`{"mode":"normal"}`)},
		{"composer state", http.MethodPatch, "/api/v1/adk/sessions/session-1/composer-state", []byte(`{"chatDraft":"draft"}`)},
		{"rename session", http.MethodPut, "/api/v1/adk/sessions/session-1", []byte(`{"title":"Renamed"}`)},
		{"delete session", http.MethodDelete, "/api/v1/adk/sessions/session-1", nil},
		{"chat", http.MethodPost, "/api/v1/adk/chat", []byte(`{"agentId":"agent-1","message":"hi"}`)},
		{"runs", http.MethodGet, "/api/v1/adk/runs", nil},
		{"run", http.MethodGet, "/api/v1/adk/runs/run-1", nil},
		{"cancel run", http.MethodPost, "/api/v1/adk/runs/run-1/cancel", nil},
		{"pause run", http.MethodPost, "/api/v1/adk/runs/run-1/pause", nil},
		{"resume run", http.MethodPost, "/api/v1/adk/runs/run-1/resume", nil},
		{"update objective", http.MethodPatch, "/api/v1/adk/runs/run-1/objective", []byte(`{"objective":"new"}`)},
		{"input response", http.MethodPost, "/api/v1/adk/runs/run-1/input-response", []byte(`{"requestId":"input-1","answers":[]}`)},
		{"approvals", http.MethodGet, "/api/v1/adk/approvals", nil},
		{"approve", http.MethodPost, "/api/v1/adk/approvals/approval-1/approve", nil},
		{"skills", http.MethodGet, "/api/v1/adk/skills", nil},
		{"install skill", http.MethodPost, "/api/v1/adk/skills", []byte(`{"source":"local"}`)},
		{"delete skill", http.MethodDelete, "/api/v1/adk/skills/skill-1", nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			recorder := performAssistantRequest(router, tc.method, tc.path, tc.body)
			if recorder.Code != http.StatusServiceUnavailable {
				t.Fatalf("%s %s status=%d want=503 body=%s", tc.method, tc.path, recorder.Code, recorder.Body.String())
			}
		})
	}
}

func TestAssistantRoutesSurfaceStoreFailuresAfterRuntimeClose(t *testing.T) {
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
	service := assistantservice.NewService(runtime)
	gin.SetMode(gin.TestMode)
	router := gin.New()
	RegisterRoutes(router.Group("/api/v1"), service)
	if err := runtime.Close(); err != nil {
		t.Fatalf("Close runtime before requests: %v", err)
	}

	cases := []struct {
		name   string
		method string
		path   string
		body   []byte
		status int
	}{
		{"snapshot", http.MethodGet, "/api/v1/adk", nil, http.StatusInternalServerError},
		{"workflows", http.MethodGet, "/api/v1/adk/workflows", nil, http.StatusInternalServerError},
		{"workflow", http.MethodGet, "/api/v1/adk/workflows/workflow-1", nil, http.StatusBadRequest},
		{"save workflow", http.MethodPost, "/api/v1/adk/workflows", []byte(`{"name":"Closed Store Workflow"}`), http.StatusBadRequest},
		{"delete workflow", http.MethodDelete, "/api/v1/adk/workflows/workflow-1", nil, http.StatusBadRequest},
		{"run workflow", http.MethodPost, "/api/v1/adk/workflows/workflow-1/run", []byte(`{"symbol":"US.AAPL"}`), http.StatusBadRequest},
		{"workflow triggers", http.MethodGet, "/api/v1/adk/workflows/workflow-1/triggers", nil, http.StatusBadRequest},
		{"save workflow trigger", http.MethodPost, "/api/v1/adk/workflows/workflow-1/triggers", []byte(`{"type":"manual","title":"Manual"}`), http.StatusBadRequest},
		{"delete workflow trigger", http.MethodDelete, "/api/v1/adk/workflows/workflow-1/triggers/trigger-1", nil, http.StatusBadRequest},
		{"run workflow trigger", http.MethodPost, "/api/v1/adk/workflow-triggers/trigger-1/run", nil, http.StatusBadRequest},
		{"workflow logs", http.MethodGet, "/api/v1/adk/workflow-trigger-logs", nil, http.StatusInternalServerError},
		{"providers", http.MethodGet, "/api/v1/adk/providers", nil, http.StatusInternalServerError},
		{"save provider", http.MethodPost, "/api/v1/adk/providers", []byte(`{"displayName":"Provider"}`), http.StatusInternalServerError},
		{"set default provider", http.MethodPost, "/api/v1/adk/providers/provider-1/default", nil, http.StatusInternalServerError},
		{"delete provider", http.MethodDelete, "/api/v1/adk/providers/provider-1", nil, http.StatusInternalServerError},
		{"agents", http.MethodGet, "/api/v1/adk/agents", nil, http.StatusInternalServerError},
		{"save agent", http.MethodPost, "/api/v1/adk/agents", []byte(`{"name":"Agent"}`), http.StatusInternalServerError},
		{"delete agent", http.MethodDelete, "/api/v1/adk/agents/agent-1", nil, http.StatusInternalServerError},
		{"tasks", http.MethodGet, "/api/v1/adk/tasks", nil, http.StatusInternalServerError},
		{"task", http.MethodGet, "/api/v1/adk/tasks/task-1", nil, http.StatusInternalServerError},
		{"save task", http.MethodPost, "/api/v1/adk/tasks", []byte(`{"title":"Task"}`), http.StatusBadRequest},
		{"delete task", http.MethodDelete, "/api/v1/adk/tasks/task-1", nil, http.StatusInternalServerError},
		{"memory", http.MethodGet, "/api/v1/adk/memory", nil, http.StatusBadRequest},
		{"save memory", http.MethodPost, "/api/v1/adk/memory", []byte(`{"key":"note","value":"v","scope":"workspace"}`), http.StatusBadRequest},
		{"delete memory", http.MethodDelete, "/api/v1/adk/memory/memory-1", nil, http.StatusInternalServerError},
		{"sessions", http.MethodGet, "/api/v1/adk/sessions", nil, http.StatusInternalServerError},
		{"create session", http.MethodPost, "/api/v1/adk/sessions", []byte(`{"agentId":"agent-1"}`), http.StatusBadRequest},
		{"session", http.MethodGet, "/api/v1/adk/sessions/session-1", nil, http.StatusInternalServerError},
		{"rename session", http.MethodPut, "/api/v1/adk/sessions/session-1", []byte(`{"title":"Renamed"}`), http.StatusBadRequest},
		{"delete session", http.MethodDelete, "/api/v1/adk/sessions/session-1", nil, http.StatusInternalServerError},
		{"runs", http.MethodGet, "/api/v1/adk/runs", nil, http.StatusInternalServerError},
		{"run", http.MethodGet, "/api/v1/adk/runs/run-1", nil, http.StatusInternalServerError},
		{"cancel run", http.MethodPost, "/api/v1/adk/runs/run-1/cancel", nil, http.StatusNotFound},
		{"pause run", http.MethodPost, "/api/v1/adk/runs/run-1/pause", nil, http.StatusBadRequest},
		{"resume run", http.MethodPost, "/api/v1/adk/runs/run-1/resume", nil, http.StatusBadRequest},
		{"update objective", http.MethodPatch, "/api/v1/adk/runs/run-1/objective", []byte(`{"objective":"new"}`), http.StatusBadRequest},
		{"input response", http.MethodPost, "/api/v1/adk/runs/run-1/input-response", []byte(`{"requestId":"input-1","answers":[]}`), http.StatusInternalServerError},
		{"approvals", http.MethodGet, "/api/v1/adk/approvals", nil, http.StatusInternalServerError},
		{"approve", http.MethodPost, "/api/v1/adk/approvals/approval-1/approve", nil, http.StatusInternalServerError},
		{"skills", http.MethodGet, "/api/v1/adk/skills", nil, http.StatusOK},
		{"delete skill", http.MethodDelete, "/api/v1/adk/skills/skill-1", nil, http.StatusInternalServerError},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			recorder := performAssistantRequest(router, tc.method, tc.path, tc.body)
			if recorder.Code != tc.status {
				t.Fatalf("%s %s status=%d want=%d body=%s", tc.method, tc.path, recorder.Code, tc.status, recorder.Body.String())
			}
		})
	}
}

func TestAssistantCatalogSessionAndObservabilitySuccessContracts(t *testing.T) {
	runtime, router := newAssistantTestRouter(t)
	ctx := t.Context()
	store := runtime.Store()

	provider, err := store.SaveProvider(ctx, jfadk.ProviderWriteRequest{
		ID: "success-provider", DisplayName: "Success Provider", BaseURL: "https://example.test/v1",
		Model: "model", APIKey: "sk-test", Enabled: true,
	})
	if err != nil {
		t.Fatalf("SaveProvider: %v", err)
	}
	standaloneProvider, err := store.SaveProvider(ctx, jfadk.ProviderWriteRequest{
		ID: "delete-provider", DisplayName: "Delete Provider", BaseURL: "https://delete.example/v1",
		Model: "model", APIKey: "sk-delete", Enabled: true,
	})
	if err != nil {
		t.Fatalf("SaveProvider standalone: %v", err)
	}
	agent, err := store.SaveAgent(ctx, jfadk.AgentWriteRequest{
		ID: "success-agent", Name: "Success Agent", ProviderID: provider.ID, Status: jfadk.AgentStatusEnabled,
	})
	if err != nil {
		t.Fatalf("SaveAgent: %v", err)
	}
	session, err := store.CreateSession(ctx, agent.ID, "Success Session")
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	run := jfadk.Run{ID: "success-run", SessionID: session.ID, AgentID: agent.ID, Status: jfadk.RunStatusRunning, WorkMode: jfadk.WorkModeLoop}
	if err := store.SaveRun(ctx, run); err != nil {
		t.Fatalf("SaveRun: %v", err)
	}
	approval := jfadk.Approval{ID: "success-approval", RunID: run.ID, AgentID: agent.ID, Status: jfadk.ApprovalStatusPending}
	if err := store.SaveApproval(ctx, approval); err != nil {
		t.Fatalf("SaveApproval: %v", err)
	}
	task, err := store.SaveTask(ctx, jfadk.TaskWriteRequest{ID: "success-task", Title: "Do work", Status: "TODO", AgentID: agent.ID, RunID: run.ID})
	if err != nil {
		t.Fatalf("SaveTask: %v", err)
	}
	memory, err := store.SaveMemory(ctx, jfadk.MemoryWriteRequest{Key: "Success Note", Value: "remember this", Scope: "workspace"})
	if err != nil {
		t.Fatalf("SaveMemory: %v", err)
	}
	optimization, err := store.SaveOptimizationTask(ctx, jfadk.OptimizationTask{ID: "success-optimization", Status: "RUNNING", Objective: "Improve returns"})
	if err != nil {
		t.Fatalf("SaveOptimizationTask: %v", err)
	}
	if err := store.AddAuditEvent(ctx, jfadk.AuditEvent{ID: "audit-success", Kind: "provider_saved", SubjectID: provider.ID, Detail: "saved"}); err != nil {
		t.Fatalf("AddAuditEvent: %v", err)
	}

	cases := []struct {
		name   string
		method string
		path   string
		body   []byte
	}{
		{"snapshot", http.MethodGet, "/api/v1/adk", nil},
		{"tools", http.MethodGet, "/api/v1/adk/tools", nil},
		{"agent templates", http.MethodGet, "/api/v1/adk/agent-templates", nil},
		{"providers", http.MethodGet, "/api/v1/adk/providers", nil},
		{"provider update", http.MethodPut, "/api/v1/adk/providers/" + provider.ID, []byte(`{"displayName":"Updated Provider","baseUrl":"https://example.test/v1","model":"model","enabled":true}`)},
		{"agents", http.MethodGet, "/api/v1/adk/agents?status=ENABLED&limit=1&offset=0", nil},
		{"agent update", http.MethodPut, "/api/v1/adk/agents/" + agent.ID, []byte(`{"name":"Updated Agent","providerId":"` + provider.ID + `","status":"ENABLED"}`)},
		{"skills", http.MethodGet, "/api/v1/adk/skills", nil},
		{"sessions", http.MethodGet, "/api/v1/adk/sessions?agentId=" + agent.ID + "&query=success", nil},
		{"session detail", http.MethodGet, "/api/v1/adk/sessions/" + session.ID, nil},
		{"rename session", http.MethodPut, "/api/v1/adk/sessions/" + session.ID, []byte(`{"title":"Renamed Session"}`)},
		{"runs", http.MethodGet, "/api/v1/adk/runs?status=RUNNING&agentId=" + agent.ID + "&sessionId=" + session.ID, nil},
		{"run detail", http.MethodGet, "/api/v1/adk/runs/" + run.ID, nil},
		{"update objective", http.MethodPatch, "/api/v1/adk/runs/" + run.ID + "/objective", []byte(`{"objective":"new objective"}`)},
		{"approvals", http.MethodGet, "/api/v1/adk/approvals?status=PENDING&agentId=" + agent.ID, nil},
		{"tasks", http.MethodGet, "/api/v1/adk/tasks?status=TODO&agentId=" + agent.ID + "&runId=" + run.ID, nil},
		{"task detail", http.MethodGet, "/api/v1/adk/tasks/" + task.ID, nil},
		{"task create", http.MethodPost, "/api/v1/adk/tasks", []byte(`{"id":"created-task","title":"Created Task","status":"TODO","agentId":"` + agent.ID + `","runId":"` + run.ID + `"}`)},
		{"task update", http.MethodPut, "/api/v1/adk/tasks/" + task.ID, []byte(`{"title":"Updated Task","status":"IN_PROGRESS"}`)},
		{"task delete", http.MethodDelete, "/api/v1/adk/tasks/" + task.ID, nil},
		{"memory", http.MethodGet, "/api/v1/adk/memory?scope=workspace&key=success-note", nil},
		{"memory create", http.MethodPost, "/api/v1/adk/memory", []byte(`{"key":"Created Note","value":"created","scope":"workspace"}`)},
		{"delete memory", http.MethodDelete, "/api/v1/adk/memory/" + memory.ID, nil},
		{"audit", http.MethodGet, "/api/v1/adk/audit?kind=provider_saved&subjectId=" + provider.ID, nil},
		{"metrics", http.MethodGet, "/api/v1/adk/metrics", nil},
		{"optimization tasks", http.MethodGet, "/api/v1/adk/optimization-tasks", nil},
		{"optimization task", http.MethodGet, "/api/v1/adk/optimization-tasks/" + optimization.ID, nil},
		{"provider delete", http.MethodDelete, "/api/v1/adk/providers/" + standaloneProvider.ID, nil},
		{"session delete", http.MethodDelete, "/api/v1/adk/sessions/" + session.ID, nil},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			recorder := performAssistantRequest(router, tc.method, tc.path, tc.body)
			if recorder.Code != http.StatusOK {
				t.Fatalf("%s %s status=%d want=200 body=%s", tc.method, tc.path, recorder.Code, recorder.Body.String())
			}
		})
	}
}

func TestAssistantCatalogBoundaryStatusCodes(t *testing.T) {
	runtime, router := newAssistantTestRouter(t)
	ctx := t.Context()

	provider, err := runtime.Store().SaveProvider(ctx, jfadk.ProviderWriteRequest{
		ID: "provider-in-use", DisplayName: "Provider In Use", BaseURL: "https://example.test/v1",
		Model: "model", APIKey: "sk-test", Enabled: true,
	})
	if err != nil {
		t.Fatalf("SaveProvider: %v", err)
	}
	if _, err := runtime.Store().SaveAgent(ctx, jfadk.AgentWriteRequest{
		ID: "agent-uses-provider", Name: "Provider User", ProviderID: provider.ID, Status: jfadk.AgentStatusEnabled,
	}); err != nil {
		t.Fatalf("SaveAgent: %v", err)
	}

	cases := []struct {
		name   string
		method string
		path   string
		body   []byte
		status int
	}{
		{"invalid task status list", http.MethodGet, "/api/v1/adk/tasks?status=BAD", nil, http.StatusBadRequest},
		{"missing task get", http.MethodGet, "/api/v1/adk/tasks/missing-task", nil, http.StatusNotFound},
		{"empty task create", http.MethodPost, "/api/v1/adk/tasks", []byte(`{"title":" "}`), http.StatusBadRequest},
		{"missing task patch", http.MethodPut, "/api/v1/adk/tasks/missing-task", []byte(`{"title":"patched"}`), http.StatusNotFound},
		{"missing task delete", http.MethodDelete, "/api/v1/adk/tasks/missing-task", nil, http.StatusNotFound},
		{"invalid memory scope list", http.MethodGet, "/api/v1/adk/memory?scope=private", nil, http.StatusBadRequest},
		{"invalid memory save", http.MethodPost, "/api/v1/adk/memory", []byte(`{"key":"x","scope":"private"}`), http.StatusBadRequest},
		{"missing memory delete", http.MethodDelete, "/api/v1/adk/memory/missing-memory", nil, http.StatusNotFound},
		{"missing default provider", http.MethodPost, "/api/v1/adk/providers/missing-provider/default", nil, http.StatusNotFound},
		{"provider in use delete", http.MethodDelete, "/api/v1/adk/providers/" + provider.ID, nil, http.StatusConflict},
		{"builtin agent delete", http.MethodDelete, "/api/v1/adk/agents/" + jfadk.DefaultBuiltinAgentID, nil, http.StatusConflict},
		{"builtin agent disable", http.MethodPut, "/api/v1/adk/agents/" + jfadk.DefaultBuiltinAgentID, []byte(`{"status":"DISABLED"}`), http.StatusConflict},
		{"invalid provider payload", http.MethodPost, "/api/v1/adk/providers", []byte(`{`), http.StatusBadRequest},
		{"invalid agent payload", http.MethodPost, "/api/v1/adk/agents", []byte(`{`), http.StatusBadRequest},
		{"invalid skill install payload", http.MethodPost, "/api/v1/adk/skills", []byte(`{`), http.StatusBadRequest},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			recorder := performAssistantRequest(router, tc.method, tc.path, tc.body)
			if recorder.Code != tc.status {
				t.Fatalf("%s %s status=%d want=%d body=%s", tc.method, tc.path, recorder.Code, tc.status, recorder.Body.String())
			}
		})
	}
}

func TestAssistantSessionRunBoundaryStatusCodes(t *testing.T) {
	_, router := newAssistantTestRouter(t)

	cases := []struct {
		name   string
		method string
		path   string
		body   []byte
		status int
	}{
		{"create session missing agent", http.MethodPost, "/api/v1/adk/sessions", []byte(`{"agentId":"missing-agent"}`), http.StatusBadRequest},
		{"create session malformed payload", http.MethodPost, "/api/v1/adk/sessions", []byte(`{`), http.StatusBadRequest},
		{"missing session get", http.MethodGet, "/api/v1/adk/sessions/missing-session", nil, http.StatusNotFound},
		{"missing session context", http.MethodGet, "/api/v1/adk/sessions/missing-session/context", nil, http.StatusNotFound},
		{"missing session compact", http.MethodPost, "/api/v1/adk/sessions/missing-session/context/compact", []byte(`{"mode":"summary"}`), http.StatusNotFound},
		{"compact session malformed payload", http.MethodPost, "/api/v1/adk/sessions/missing-session/context/compact", []byte(`{`), http.StatusBadRequest},
		{"invalid rename payload", http.MethodPut, "/api/v1/adk/sessions/missing-session", []byte(`{`), http.StatusBadRequest},
		{"missing composer state", http.MethodPatch, "/api/v1/adk/sessions/missing-session/composer-state", []byte(`{"chatDraft":"x"}`), http.StatusNotFound},
		{"missing cancel run", http.MethodPost, "/api/v1/adk/runs/missing-run/cancel", nil, http.StatusNotFound},
		{"missing pause run", http.MethodPost, "/api/v1/adk/runs/missing-run/pause", nil, http.StatusNotFound},
		{"missing resume run", http.MethodPost, "/api/v1/adk/runs/missing-run/resume", nil, http.StatusNotFound},
		{"invalid objective payload", http.MethodPatch, "/api/v1/adk/runs/missing-run/objective", []byte(`{`), http.StatusBadRequest},
		{"missing run get", http.MethodGet, "/api/v1/adk/runs/missing-run", nil, http.StatusNotFound},
		{"missing approval approve is idempotent", http.MethodPost, "/api/v1/adk/approvals/missing-approval/approve", nil, http.StatusOK},
		{"missing stream reconnect", http.MethodGet, "/api/v1/adk/streams/missing-stream", nil, http.StatusNotFound},
		{"invalid stream after", http.MethodGet, "/api/v1/adk/streams/missing-stream?after=abc", nil, http.StatusBadRequest},
		{"missing run stream reconnect", http.MethodGet, "/api/v1/adk/runs/missing-run/stream", nil, http.StatusNotFound},
		{"invalid run stream after", http.MethodGet, "/api/v1/adk/runs/missing-run/stream?after=abc", nil, http.StatusBadRequest},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			recorder := performAssistantRequest(router, tc.method, tc.path, tc.body)
			if recorder.Code != tc.status {
				t.Fatalf("%s %s status=%d want=%d body=%s", tc.method, tc.path, recorder.Code, tc.status, recorder.Body.String())
			}
		})
	}
}

func TestAssistantMutationRoutesRejectMalformedJSON(t *testing.T) {
	_, router := newAssistantTestRouter(t)

	cases := []struct {
		method string
		path   string
	}{
		{http.MethodPut, "/api/v1/adk/tasks/task-1"},
		{http.MethodPost, "/api/v1/adk/memory"},
		{http.MethodPost, "/api/v1/adk/workflows"},
		{http.MethodPost, "/api/v1/adk/workflows/workflow-1/run"},
		{http.MethodPost, "/api/v1/adk/workflows/workflow-1/triggers"},
		{http.MethodPatch, "/api/v1/adk/sessions/session-1/composer-state"},
	}
	for _, tc := range cases {
		t.Run(tc.method+" "+tc.path, func(t *testing.T) {
			recorder := performAssistantRequest(router, tc.method, tc.path, []byte(`{`))
			if recorder.Code != http.StatusBadRequest {
				t.Fatalf("%s %s status=%d want=400 body=%s", tc.method, tc.path, recorder.Code, recorder.Body.String())
			}
		})
	}
}
