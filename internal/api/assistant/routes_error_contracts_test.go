package assistant

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	jfadk "github.com/jftrade/jftrade-main/pkg/adk"
)

func TestAssistantRoutesRejectInvalidQueriesPayloadsAndMissingResources(t *testing.T) {
	runtime, router := newAssistantTestRouter(t)
	ctx := t.Context()

	agent, err := runtime.Store().SaveAgent(ctx, jfadk.AgentWriteRequest{
		ID: "agent-errors", Name: "Error Contracts", ProviderID: "test-provider",
		Status: jfadk.AgentStatusEnabled,
	})
	if err != nil {
		t.Fatalf("SaveAgent: %v", err)
	}
	session, err := runtime.Store().CreateSession(ctx, agent.ID, "errors")
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	run := jfadk.Run{
		ID:        "run-errors",
		SessionID: session.ID,
		AgentID:   agent.ID,
		Status:    jfadk.RunStatusCompleted,
		CreatedAt: time.Now().UTC().Format(time.RFC3339Nano),
	}
	if err := runtime.Store().SaveRun(ctx, run); err != nil {
		t.Fatalf("SaveRun: %v", err)
	}

	type testCase struct {
		name       string
		method     string
		path       string
		body       []byte
		wantStatus int
		wantCode   string
	}
	cases := []testCase{
		{name: "agent templates", method: http.MethodGet, path: "/api/v1/adk/agent-templates", wantStatus: http.StatusOK},
		{name: "snapshot", method: http.MethodGet, path: "/api/v1/adk", wantStatus: http.StatusOK},
		{name: "tools", method: http.MethodGet, path: "/api/v1/adk/tools", wantStatus: http.StatusOK},
		{name: "non numeric tasks limit falls back", method: http.MethodGet, path: "/api/v1/adk/tasks?limit=oops", wantStatus: http.StatusOK},
		{name: "invalid task status", method: http.MethodGet, path: "/api/v1/adk/tasks?status=NOT_A_STATUS", wantStatus: http.StatusBadRequest, wantCode: "ADK_TASK_LIST_FAILED"},
		{name: "invalid task create payload", method: http.MethodPost, path: "/api/v1/adk/tasks", body: []byte(`{`), wantStatus: http.StatusBadRequest, wantCode: "BAD_REQUEST"},
		{name: "invalid task patch payload", method: http.MethodPut, path: "/api/v1/adk/tasks/task-missing", body: []byte(`{`), wantStatus: http.StatusBadRequest, wantCode: "BAD_REQUEST"},
		{name: "missing task delete", method: http.MethodDelete, path: "/api/v1/adk/tasks/task-missing", wantStatus: http.StatusNotFound, wantCode: "ADK_TASK_NOT_FOUND"},
		{name: "invalid memory payload", method: http.MethodPost, path: "/api/v1/adk/memory", body: []byte(`{`), wantStatus: http.StatusBadRequest, wantCode: "BAD_REQUEST"},
		{name: "missing memory delete", method: http.MethodDelete, path: "/api/v1/adk/memory/memory-missing", wantStatus: http.StatusNotFound, wantCode: "ADK_MEMORY_NOT_FOUND"},
		{name: "invalid provider payload", method: http.MethodPost, path: "/api/v1/adk/providers", body: []byte(`{`), wantStatus: http.StatusBadRequest, wantCode: "BAD_REQUEST"},
		{name: "invalid provider update payload", method: http.MethodPut, path: "/api/v1/adk/providers/test-provider", body: []byte(`{`), wantStatus: http.StatusBadRequest, wantCode: "BAD_REQUEST"},
		{name: "invalid agent payload", method: http.MethodPost, path: "/api/v1/adk/agents", body: []byte(`{`), wantStatus: http.StatusBadRequest, wantCode: "BAD_REQUEST"},
		{name: "invalid agent update payload", method: http.MethodPut, path: "/api/v1/adk/agents/agent-errors", body: []byte(`{`), wantStatus: http.StatusBadRequest, wantCode: "BAD_REQUEST"},
		{name: "invalid skill install payload", method: http.MethodPost, path: "/api/v1/adk/skills", body: []byte(`{`), wantStatus: http.StatusBadRequest, wantCode: "BAD_REQUEST"},
		{name: "non numeric sessions limit falls back", method: http.MethodGet, path: "/api/v1/adk/sessions?limit=oops", wantStatus: http.StatusOK},
		{name: "invalid rename payload", method: http.MethodPut, path: "/api/v1/adk/sessions/" + session.ID, body: []byte(`{`), wantStatus: http.StatusBadRequest, wantCode: "BAD_REQUEST"},
		{name: "invalid composer payload", method: http.MethodPatch, path: "/api/v1/adk/sessions/" + session.ID + "/composer-state", body: []byte(`{`), wantStatus: http.StatusBadRequest, wantCode: "BAD_REQUEST"},
		{name: "non numeric runs limit falls back", method: http.MethodGet, path: "/api/v1/adk/runs?limit=oops", wantStatus: http.StatusOK},
		{name: "invalid objective payload", method: http.MethodPatch, path: "/api/v1/adk/runs/" + run.ID + "/objective", body: []byte(`{`), wantStatus: http.StatusBadRequest, wantCode: "BAD_REQUEST"},
		{name: "non numeric approvals limit falls back", method: http.MethodGet, path: "/api/v1/adk/approvals?limit=oops", wantStatus: http.StatusOK},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			recorder := performAssistantRequest(router, tc.method, tc.path, tc.body)
			if recorder.Code != tc.wantStatus {
				t.Fatalf("status=%d want=%d body=%s", recorder.Code, tc.wantStatus, recorder.Body.String())
			}
			if tc.wantCode == "" {
				assertOKEnvelope(t, recorder)
				return
			}
			var envelope struct {
				OK    bool `json:"ok"`
				Error struct {
					Code string `json:"code"`
				} `json:"error"`
			}
			if err := json.Unmarshal(recorder.Body.Bytes(), &envelope); err != nil {
				t.Fatalf("decode error envelope: %v body=%s", err, recorder.Body.String())
			}
			if envelope.OK || envelope.Error.Code != tc.wantCode {
				t.Fatalf("error envelope=%s wantCode=%s", recorder.Body.String(), tc.wantCode)
			}
		})
	}
}

func TestAssistantRoutesEnforceBusinessValidationOnUpdates(t *testing.T) {
	runtime, router := newAssistantTestRouter(t)
	ctx := t.Context()

	providerUpdate := performAssistantRequest(router, http.MethodPut, "/api/v1/adk/providers/provider-update-route", []byte(`{
		"id":"provider-body-ignored",
		"displayName":"Provider Updated In Route",
		"apiKey":"sk-route-updated",
		"enabled":true
	}`))
	if providerUpdate.Code != http.StatusOK {
		t.Fatalf("provider update status=%d body=%s", providerUpdate.Code, providerUpdate.Body.String())
	}
	var providerEnvelope struct {
		OK   bool           `json:"ok"`
		Data jfadk.Provider `json:"data"`
	}
	if err := json.Unmarshal(providerUpdate.Body.Bytes(), &providerEnvelope); err != nil {
		t.Fatalf("decode provider update: %v body=%s", err, providerUpdate.Body.String())
	}
	if !providerEnvelope.OK || providerEnvelope.Data.ID != "provider-update-route" || providerEnvelope.Data.DisplayName != "Provider Updated In Route" {
		t.Fatalf("provider update envelope=%s", providerUpdate.Body.String())
	}

	agentUpdate := performAssistantRequest(router, http.MethodPut, "/api/v1/adk/agents/agent-update-route", []byte(`{
		"id":"agent-body-ignored",
		"name":"Agent Updated In Route",
		"status":"ENABLED",
		"providerId":"provider-update-route",
		"workMode":"loop"
	}`))
	if agentUpdate.Code != http.StatusOK {
		t.Fatalf("agent update status=%d body=%s", agentUpdate.Code, agentUpdate.Body.String())
	}
	var agentEnvelope struct {
		OK   bool        `json:"ok"`
		Data jfadk.Agent `json:"data"`
	}
	if err := json.Unmarshal(agentUpdate.Body.Bytes(), &agentEnvelope); err != nil {
		t.Fatalf("decode agent update: %v body=%s", err, agentUpdate.Body.String())
	}
	if !agentEnvelope.OK || agentEnvelope.Data.ID != "agent-update-route" || agentEnvelope.Data.ProviderID != "provider-update-route" {
		t.Fatalf("agent update envelope=%s", agentUpdate.Body.String())
	}

	session, err := runtime.Store().CreateSession(ctx, agentEnvelope.Data.ID, "Validation Session")
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	renameEmpty := performAssistantRequest(router, http.MethodPut, "/api/v1/adk/sessions/"+session.ID, []byte(`{"title":"   "}`))
	assertAssistantErrorCode(t, renameEmpty, http.StatusBadRequest, "ADK_SESSION_RENAME_FAILED")

	composerInvalid := performAssistantRequest(router, http.MethodPatch, "/api/v1/adk/sessions/"+session.ID+"/composer-state", []byte(`{
		"workModeOverride":"parallel"
	}`))
	assertAssistantErrorCode(t, composerInvalid, http.StatusBadRequest, "ADK_SESSION_COMPOSER_STATE_UPDATE_FAILED")

	composerValid := performAssistantRequest(router, http.MethodPatch, "/api/v1/adk/sessions/"+session.ID+"/composer-state", []byte(`{
		"chatDraft":"复盘昨晚的挂单",
		"workModeOverride":"task",
		"permissionModeOverride":"approval",
		"goalObjectiveDraft":"检查今日开盘风险",
		"goalObjectiveTouched":true
	}`))
	if composerValid.Code != http.StatusOK {
		t.Fatalf("composer valid status=%d body=%s", composerValid.Code, composerValid.Body.String())
	}
	var composerEnvelope struct {
		OK   bool                       `json:"ok"`
		Data jfadk.SessionComposerState `json:"data"`
	}
	if err := json.Unmarshal(composerValid.Body.Bytes(), &composerEnvelope); err != nil {
		t.Fatalf("decode composer state: %v body=%s", err, composerValid.Body.String())
	}
	if !composerEnvelope.OK || composerEnvelope.Data.WorkModeOverride != jfadk.WorkModeTask || composerEnvelope.Data.PermissionModeOverride != jfadk.PermissionModeApproval || !composerEnvelope.Data.GoalObjectiveTouched {
		t.Fatalf("composer state envelope=%s", composerValid.Body.String())
	}

	missingTaskPatch := performAssistantRequest(router, http.MethodPut, "/api/v1/adk/tasks/task-missing", []byte(`{
		"status":"DONE",
		"resultSummary":"理论上不会成功"
	}`))
	assertAssistantErrorCode(t, missingTaskPatch, http.StatusNotFound, "ADK_TASK_NOT_FOUND")

	loopRun := jfadk.Run{
		ID:        "run-objective-validation",
		SessionID: session.ID,
		AgentID:   agentEnvelope.Data.ID,
		WorkMode:  jfadk.WorkModeLoop,
		Status:    jfadk.RunStatusRunning,
		Objective: "盯盘并记录异常",
		CreatedAt: time.Now().UTC().Format(time.RFC3339Nano),
		UpdatedAt: time.Now().UTC().Format(time.RFC3339Nano),
	}
	if err := runtime.Store().SaveRun(ctx, loopRun); err != nil {
		t.Fatalf("SaveRun objective validation: %v", err)
	}

	objectiveEmpty := performAssistantRequest(router, http.MethodPatch, "/api/v1/adk/runs/"+loopRun.ID+"/objective", []byte(`{"objective":"   "}`))
	assertAssistantErrorCode(t, objectiveEmpty, http.StatusBadRequest, "ADK_RUN_OBJECTIVE_UPDATE_FAILED")

	if _, err := runtime.Store().SaveOptimizationTask(ctx, jfadk.OptimizationTask{
		ID:        "optimization-cancel-route",
		Status:    "queued",
		Objective: "提高收益风险比",
		CreatedAt: time.Now().UTC().Format(time.RFC3339Nano),
		UpdatedAt: time.Now().UTC().Format(time.RFC3339Nano),
	}); err != nil {
		t.Fatalf("SaveOptimizationTask: %v", err)
	}
	cancelOptimization := performAssistantRequest(router, http.MethodPost, "/api/v1/adk/optimization-tasks/optimization-cancel-route/cancel", nil)
	if cancelOptimization.Code != http.StatusOK {
		t.Fatalf("cancel optimization status=%d body=%s", cancelOptimization.Code, cancelOptimization.Body.String())
	}
	if !strings.Contains(cancelOptimization.Body.String(), `"status":"cancelled"`) {
		t.Fatalf("cancel optimization body=%s", cancelOptimization.Body.String())
	}

	missingApproval := performAssistantRequest(router, http.MethodPost, "/api/v1/adk/approvals/approval-missing/approve", nil)
	if missingApproval.Code != http.StatusOK {
		t.Fatalf("missing approval status=%d body=%s", missingApproval.Code, missingApproval.Body.String())
	}
	assertOKEnvelope(t, missingApproval)
	if !strings.Contains(missingApproval.Body.String(), `"approval":{"id":""`) {
		t.Fatalf("missing approval body=%s", missingApproval.Body.String())
	}
}

func assertAssistantErrorCode(t *testing.T, recorder *httptest.ResponseRecorder, wantStatus int, wantCode string) {
	t.Helper()
	if recorder.Code != wantStatus {
		t.Fatalf("status=%d want=%d body=%s", recorder.Code, wantStatus, recorder.Body.String())
	}
	var envelope struct {
		OK    bool `json:"ok"`
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("decode error envelope: %v body=%s", err, recorder.Body.String())
	}
	if envelope.OK || envelope.Error.Code != wantCode {
		t.Fatalf("error envelope=%s wantCode=%s", recorder.Body.String(), wantCode)
	}
}
