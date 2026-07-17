package assistant

import (
	"net/http"
	"strings"
	"testing"
	"time"

	jadk "github.com/jftrade/jftrade-main/pkg/adk"
)

func TestAssistantChatRoutesRejectMalformedOrUnresolvableRequests(t *testing.T) {
	_, router := newAssistantTestRouter(t)

	malformed := performAssistantRequest(router, http.MethodPost, "/api/v1/adk/chat", []byte(`{"agentId":`))
	assertAssistantErrorCode(t, malformed, http.StatusBadRequest, "BAD_REQUEST")

	missingAgent := performAssistantRequest(router, http.MethodPost, "/api/v1/adk/chat", []byte(`{"agentId":"agent-missing","message":"review US.AAPL"}`))
	assertAssistantErrorCode(t, missingAgent, http.StatusBadRequest, "ADK_CHAT_FAILED")

	stream := performAssistantRequest(router, http.MethodPost, "/api/v1/adk/chat/stream", []byte(`{"agentId":`))
	if stream.Code != http.StatusOK || !strings.Contains(stream.Header().Get("Content-Type"), "text/event-stream") || !strings.Contains(stream.Body.String(), `"type":"error"`) {
		t.Fatalf("malformed stream status=%d headers=%v body=%s", stream.Code, stream.Header(), stream.Body.String())
	}
}

func TestAssistantRoutesRejectMalformedMutationPayloads(t *testing.T) {
	_, router := newAssistantTestRouter(t)

	cases := []struct {
		name   string
		method string
		path   string
	}{
		{"create session", http.MethodPost, "/api/v1/adk/sessions"},
		{"compact session context", http.MethodPost, "/api/v1/adk/sessions/session-unknown/context/compact"},
		{"rename session", http.MethodPut, "/api/v1/adk/sessions/session-unknown"},
		{"patch composer state", http.MethodPatch, "/api/v1/adk/sessions/session-unknown/composer-state"},
		{"update run objective", http.MethodPatch, "/api/v1/adk/runs/run-unknown/objective"},
		{"respond to run input", http.MethodPost, "/api/v1/adk/runs/run-unknown/input-response"},
		{"create task", http.MethodPost, "/api/v1/adk/tasks"},
		{"update task", http.MethodPut, "/api/v1/adk/tasks/task-unknown"},
		{"create memory", http.MethodPost, "/api/v1/adk/memory"},
		{"create provider", http.MethodPost, "/api/v1/adk/providers"},
		{"update provider", http.MethodPut, "/api/v1/adk/providers/provider-unknown"},
		{"create agent", http.MethodPost, "/api/v1/adk/agents"},
		{"update agent", http.MethodPut, "/api/v1/adk/agents/agent-unknown"},
		{"install skill", http.MethodPost, "/api/v1/adk/skills"},
		{"create workflow", http.MethodPost, "/api/v1/adk/workflows"},
		{"update workflow", http.MethodPut, "/api/v1/adk/workflows/workflow-unknown"},
		{"run workflow", http.MethodPost, "/api/v1/adk/workflows/workflow-unknown/run"},
		{"create workflow trigger", http.MethodPost, "/api/v1/adk/workflows/workflow-unknown/triggers"},
		{"update workflow trigger", http.MethodPut, "/api/v1/adk/workflows/workflow-unknown/triggers/trigger-unknown"},
		{"run workflow trigger", http.MethodPost, "/api/v1/adk/workflow-triggers/trigger-unknown/run"},
		{"workflow webhook", http.MethodPost, "/api/v1/adk/workflow-webhooks/trigger-unknown"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			recorder := performAssistantRequest(router, tc.method, tc.path, []byte(`{"`))
			assertAssistantErrorCode(t, recorder, http.StatusBadRequest, "BAD_REQUEST")
		})
	}
}

func TestAssistantRoutesClampPaginationBeyondAvailableItems(t *testing.T) {
	_, router := newAssistantTestRouter(t)

	for _, path := range []string{
		"/api/v1/adk/audit?limit=1&offset=10000",
		"/api/v1/adk/agents?limit=1&offset=10000",
		"/api/v1/adk/optimization-tasks?limit=1&offset=10000",
	} {
		recorder := performAssistantRequest(router, http.MethodGet, path, nil)
		if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), `"returned":0`) {
			t.Fatalf("pagination path=%s status=%d body=%s", path, recorder.Code, recorder.Body.String())
		}
	}
}

func TestAssistantRunMutationRoutesEnforceGoalLifecycleRules(t *testing.T) {
	runtime, router := newAssistantTestRouter(t)
	ctx := t.Context()
	agent, err := runtime.Store().SaveAgent(ctx, jadk.AgentWriteRequest{
		ID: "agent-run-route-lifecycle", Name: "Run Route Lifecycle", Status: jadk.AgentStatusEnabled,
	})
	if err != nil {
		t.Fatalf("SaveAgent: %v", err)
	}

	createSession := performAssistantRequest(router, http.MethodPost, "/api/v1/adk/sessions", []byte(`{"agentId":"`+agent.ID+`","title":"Run lifecycle"}`))
	if createSession.Code != http.StatusOK {
		t.Fatalf("create session status=%d body=%s", createSession.Code, createSession.Body.String())
	}
	session, err := runtime.Store().CreateSession(ctx, agent.ID, "Goal lifecycle")
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	goalRun := jadk.Run{
		ID: "run-route-pause", SessionID: session.ID, AgentID: agent.ID,
		Status: jadk.RunStatusRunning, WorkMode: jadk.WorkModeLoop, WorkflowStatus: "running",
		CreatedAt: now, UpdatedAt: now,
	}
	if err := runtime.Store().SaveRun(ctx, goalRun); err != nil {
		t.Fatalf("SaveRun goal: %v", err)
	}
	pause := performAssistantRequest(router, http.MethodPost, "/api/v1/adk/runs/"+goalRun.ID+"/pause", nil)
	if pause.Code != http.StatusOK || !strings.Contains(pause.Body.String(), `"resumeState":"user_pause_requested"`) {
		t.Fatalf("pause goal run status=%d body=%s", pause.Code, pause.Body.String())
	}

	chatRun := jadk.Run{
		ID: "run-route-chat", SessionID: session.ID, AgentID: agent.ID,
		Status: jadk.RunStatusRunning, WorkMode: jadk.WorkModeChat,
		CreatedAt: now, UpdatedAt: now,
	}
	if err := runtime.Store().SaveRun(ctx, chatRun); err != nil {
		t.Fatalf("SaveRun chat: %v", err)
	}
	assertAssistantErrorCode(t,
		performAssistantRequest(router, http.MethodPost, "/api/v1/adk/runs/"+chatRun.ID+"/pause", nil),
		http.StatusBadRequest, "ADK_RUN_PAUSE_FAILED",
	)
	assertAssistantErrorCode(t,
		performAssistantRequest(router, http.MethodPost, "/api/v1/adk/runs/"+chatRun.ID+"/resume", nil),
		http.StatusBadRequest, "ADK_RUN_RESUME_FAILED",
	)
	assertAssistantErrorCode(t,
		performAssistantRequest(router, http.MethodPatch, "/api/v1/adk/runs/"+chatRun.ID+"/objective", []byte(`{"objective":"should be rejected"}`)),
		http.StatusBadRequest, "ADK_RUN_OBJECTIVE_UPDATE_FAILED",
	)
}

func TestAssistantRoutesClassifyMissingMutationTargets(t *testing.T) {
	runtime, router := newAssistantTestRouter(t)

	assertAssistantErrorCode(t,
		performAssistantRequest(router, http.MethodPut, "/api/v1/adk/tasks/task-missing", []byte(`{"status":"DONE"}`)),
		http.StatusNotFound, "ADK_TASK_NOT_FOUND",
	)
	assertAssistantErrorCode(t,
		performAssistantRequest(router, http.MethodPatch, "/api/v1/adk/runs/run-missing/objective", []byte(`{"objective":"new objective"}`)),
		http.StatusNotFound, "NOT_FOUND",
	)
	if _, err := runtime.Store().SaveTask(t.Context(), jadk.TaskWriteRequest{ID: "task-invalid-patch", Title: "Valid title", Status: "TODO"}); err != nil {
		t.Fatalf("SaveTask: %v", err)
	}
	assertAssistantErrorCode(t,
		performAssistantRequest(router, http.MethodPut, "/api/v1/adk/tasks/task-invalid-patch", []byte(`{"title":"   "}`)),
		http.StatusBadRequest, "ADK_TASK_SAVE_FAILED",
	)
}
