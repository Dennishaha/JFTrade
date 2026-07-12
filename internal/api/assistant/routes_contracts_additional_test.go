package assistant

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	jfadk "github.com/jftrade/jftrade-main/pkg/adk"
)

func TestTaskAndMemoryCRUDContracts(t *testing.T) {
	runtime, router := newAssistantTestRouter(t)
	ctx := t.Context()

	agent, err := runtime.Store().SaveAgent(ctx, jfadk.AgentWriteRequest{
		ID: "agent-contract-crud", Name: "CRUD Agent", Status: jfadk.AgentStatusEnabled,
	})
	if err != nil {
		t.Fatalf("SaveAgent: %v", err)
	}

	taskResponse := performAssistantRequest(router, http.MethodPost, "/api/v1/adk/tasks", []byte(`{
		"title":"检查盘前准备",
		"status":"IN_PROGRESS",
		"agentId":"`+agent.ID+`",
		"message":"补齐观察清单",
		"childProviderId":"provider-child",
		"childModel":"model-child-a"
	}`))
	if taskResponse.Code != http.StatusOK {
		t.Fatalf("create task status=%d body=%s", taskResponse.Code, taskResponse.Body.String())
	}
	var taskEnvelope struct {
		OK   bool       `json:"ok"`
		Data jfadk.Task `json:"data"`
	}
	if err := json.Unmarshal(taskResponse.Body.Bytes(), &taskEnvelope); err != nil {
		t.Fatalf("decode task create: %v", err)
	}
	if !taskEnvelope.OK || taskEnvelope.Data.ID == "" {
		t.Fatalf("task create envelope=%s", taskResponse.Body.String())
	}
	if taskEnvelope.Data.ChildProviderID != "provider-child" || taskEnvelope.Data.ChildModel != "model-child-a" {
		t.Fatalf("task child model fields = %+v, want provider/model persisted", taskEnvelope.Data)
	}

	taskList := performAssistantRequest(router, http.MethodGet, "/api/v1/adk/tasks?status=IN_PROGRESS", nil)
	if taskList.Code != http.StatusOK || !strings.Contains(taskList.Body.String(), "检查盘前准备") {
		t.Fatalf("list tasks status=%d body=%s", taskList.Code, taskList.Body.String())
	}

	taskPatch := performAssistantRequest(router, http.MethodPut, "/api/v1/adk/tasks/"+taskEnvelope.Data.ID, []byte(`{
		"status":"DONE",
		"resultSummary":"已完成",
		"childProviderId":"provider-child-updated",
		"childModel":"model-child-b"
	}`))
	if taskPatch.Code != http.StatusOK || !strings.Contains(taskPatch.Body.String(), `"status":"DONE"`) || !strings.Contains(taskPatch.Body.String(), `"childModel":"model-child-b"`) {
		t.Fatalf("patch task status=%d body=%s", taskPatch.Code, taskPatch.Body.String())
	}

	taskGet := performAssistantRequest(router, http.MethodGet, "/api/v1/adk/tasks/"+taskEnvelope.Data.ID, nil)
	if taskGet.Code != http.StatusOK || !strings.Contains(taskGet.Body.String(), `"resultSummary":"已完成"`) {
		t.Fatalf("get task status=%d body=%s", taskGet.Code, taskGet.Body.String())
	}

	taskDelete := performAssistantRequest(router, http.MethodDelete, "/api/v1/adk/tasks/"+taskEnvelope.Data.ID, nil)
	if taskDelete.Code != http.StatusOK {
		t.Fatalf("delete task status=%d body=%s", taskDelete.Code, taskDelete.Body.String())
	}
	missingTask := performAssistantRequest(router, http.MethodGet, "/api/v1/adk/tasks/"+taskEnvelope.Data.ID, nil)
	if missingTask.Code != http.StatusNotFound {
		t.Fatalf("missing task status=%d body=%s", missingTask.Code, missingTask.Body.String())
	}

	memoryResponse := performAssistantRequest(router, http.MethodPost, "/api/v1/adk/memory", []byte(`{
		"agentId":"`+agent.ID+`",
		"key":"watch-note",
		"value":"关注开盘波动",
		"scope":"agent"
	}`))
	if memoryResponse.Code != http.StatusOK {
		t.Fatalf("save memory status=%d body=%s", memoryResponse.Code, memoryResponse.Body.String())
	}
	var memoryEnvelope struct {
		OK   bool              `json:"ok"`
		Data jfadk.MemoryEntry `json:"data"`
	}
	if err := json.Unmarshal(memoryResponse.Body.Bytes(), &memoryEnvelope); err != nil {
		t.Fatalf("decode memory create: %v", err)
	}
	if !memoryEnvelope.OK || memoryEnvelope.Data.ID == "" {
		t.Fatalf("memory create envelope=%s", memoryResponse.Body.String())
	}

	memoryList := performAssistantRequest(router, http.MethodGet, "/api/v1/adk/memory?agentId="+agent.ID+"&key=watch-note", nil)
	if memoryList.Code != http.StatusOK || !strings.Contains(memoryList.Body.String(), "关注开盘波动") {
		t.Fatalf("list memory status=%d body=%s", memoryList.Code, memoryList.Body.String())
	}

	memoryDelete := performAssistantRequest(router, http.MethodDelete, "/api/v1/adk/memory/"+memoryEnvelope.Data.ID, nil)
	if memoryDelete.Code != http.StatusOK {
		t.Fatalf("delete memory status=%d body=%s", memoryDelete.Code, memoryDelete.Body.String())
	}
	missingMemory := performAssistantRequest(router, http.MethodDelete, "/api/v1/adk/memory/"+memoryEnvelope.Data.ID, nil)
	if missingMemory.Code != http.StatusNotFound {
		t.Fatalf("missing memory status=%d body=%s", missingMemory.Code, missingMemory.Body.String())
	}
}

func TestCatalogSnapshotToolsTemplatesAndDeleteAgentContracts(t *testing.T) {
	runtime, router := newAssistantTestRouter(t)
	ctx := t.Context()

	snapshot := performAssistantRequest(router, http.MethodGet, "/api/v1/adk", nil)
	if snapshot.Code != http.StatusOK || !strings.Contains(snapshot.Body.String(), `"runtimeSettings"`) {
		t.Fatalf("snapshot status=%d body=%s", snapshot.Code, snapshot.Body.String())
	}
	tools := performAssistantRequest(router, http.MethodGet, "/api/v1/adk/tools", nil)
	if tools.Code != http.StatusOK || !strings.Contains(tools.Body.String(), "tools.search") {
		t.Fatalf("tools status=%d body=%s", tools.Code, tools.Body.String())
	}
	templates := performAssistantRequest(router, http.MethodGet, "/api/v1/adk/agent-templates", nil)
	if templates.Code != http.StatusOK || !strings.Contains(templates.Body.String(), "默认助手") || strings.Contains(templates.Body.String(), "investment-analyst") {
		t.Fatalf("templates status=%d body=%s", templates.Code, templates.Body.String())
	}
	editDefault := performAssistantRequest(router, http.MethodPut, "/api/v1/adk/agents/"+jfadk.DefaultBuiltinAgentID, []byte(`{
		"name":"Edited Default",
		"status":"ENABLED"
	}`))
	if editDefault.Code != http.StatusConflict || !strings.Contains(editDefault.Body.String(), "ADK_AGENT_PROTECTED") {
		t.Fatalf("edit default agent status=%d body=%s", editDefault.Code, editDefault.Body.String())
	}
	deleteDefault := performAssistantRequest(router, http.MethodDelete, "/api/v1/adk/agents/"+jfadk.DefaultBuiltinAgentID, nil)
	if deleteDefault.Code != http.StatusConflict || !strings.Contains(deleteDefault.Body.String(), "ADK_AGENT_PROTECTED") {
		t.Fatalf("delete default agent status=%d body=%s", deleteDefault.Code, deleteDefault.Body.String())
	}

	agent, err := runtime.Store().SaveAgent(ctx, jfadk.AgentWriteRequest{
		ID: "agent-delete-route", Name: "Delete Route", Status: jfadk.AgentStatusEnabled,
	})
	if err != nil {
		t.Fatalf("SaveAgent: %v", err)
	}
	deleteResponse := performAssistantRequest(router, http.MethodDelete, "/api/v1/adk/agents/"+agent.ID, nil)
	if deleteResponse.Code != http.StatusOK {
		t.Fatalf("delete agent status=%d body=%s", deleteResponse.Code, deleteResponse.Body.String())
	}
	agents := performAssistantRequest(router, http.MethodGet, "/api/v1/adk/agents", nil)
	if agents.Code != http.StatusOK || strings.Contains(agents.Body.String(), agent.ID) {
		t.Fatalf("agents after delete status=%d body=%s", agents.Code, agents.Body.String())
	}
}

func TestProviderAndAgentValidationContracts(t *testing.T) {
	runtime, router := newAssistantTestRouter(t)
	ctx := t.Context()

	providerCreate := performAssistantRequest(router, http.MethodPost, "/api/v1/adk/providers", []byte(`{
		"id":"provider-route-ok",
		"displayName":"Route OK",
		"apiKey":"sk-route",
		"enabled":true
	}`))
	if providerCreate.Code != http.StatusOK {
		t.Fatalf("create provider status=%d body=%s", providerCreate.Code, providerCreate.Body.String())
	}

	if _, err := runtime.Store().SaveProvider(ctx, jfadk.ProviderWriteRequest{
		ID: "provider-route-disabled", DisplayName: "Route Disabled", Enabled: false,
	}); err != nil {
		t.Fatalf("SaveProvider disabled: %v", err)
	}
	if _, err := runtime.Store().SaveProvider(ctx, jfadk.ProviderWriteRequest{
		ID: "provider-route-no-key", DisplayName: "Route No Key", Enabled: true,
	}); err != nil {
		t.Fatalf("SaveProvider no key: %v", err)
	}

	for _, tc := range []struct {
		name       string
		body       string
		wantStatus int
		wantText   string
	}{
		{
			name:       "missing provider",
			body:       `{"id":"agent-missing-provider","name":"Missing Provider","status":"ENABLED","providerId":"provider-404"}`,
			wantStatus: http.StatusBadRequest,
			wantText:   "provider not found",
		},
		{
			name:       "disabled provider",
			body:       `{"id":"agent-disabled-provider","name":"Disabled Provider","status":"ENABLED","providerId":"provider-route-disabled"}`,
			wantStatus: http.StatusBadRequest,
			wantText:   "provider is disabled",
		},
		{
			name:       "provider without api key",
			body:       `{"id":"agent-no-key","name":"No Key","status":"ENABLED","providerId":"provider-route-no-key"}`,
			wantStatus: http.StatusBadRequest,
			wantText:   "provider API key is not configured",
		},
		{
			name:       "unknown tool",
			body:       `{"id":"agent-unknown-tool","name":"Unknown Tool","status":"ENABLED","providerId":"provider-route-ok","tools":["does.not.exist"]}`,
			wantStatus: http.StatusBadRequest,
			wantText:   "unknown ADK tool",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			response := performAssistantRequest(router, http.MethodPost, "/api/v1/adk/agents", []byte(tc.body))
			if response.Code != tc.wantStatus || !strings.Contains(response.Body.String(), tc.wantText) {
				t.Fatalf("status=%d body=%s wantStatus=%d wantText=%q", response.Code, response.Body.String(), tc.wantStatus, tc.wantText)
			}
		})
	}

	validAgent := performAssistantRequest(router, http.MethodPost, "/api/v1/adk/agents", []byte(`{
		"id":"agent-route-valid",
		"name":"Route Valid",
		"status":"ENABLED",
		"providerId":"provider-route-ok"
	}`))
	if validAgent.Code != http.StatusOK {
		t.Fatalf("save valid agent status=%d body=%s", validAgent.Code, validAgent.Body.String())
	}

	deleteProvider := performAssistantRequest(router, http.MethodDelete, "/api/v1/adk/providers/provider-route-ok", nil)
	if deleteProvider.Code != http.StatusConflict || !strings.Contains(deleteProvider.Body.String(), "used by agent") {
		t.Fatalf("delete used provider status=%d body=%s", deleteProvider.Code, deleteProvider.Body.String())
	}

	providerTest := performAssistantRequest(router, http.MethodPost, "/api/v1/adk/providers/test-provider/test", nil)
	if providerTest.Code != http.StatusOK || !strings.Contains(providerTest.Body.String(), `"ok":true`) {
		t.Fatalf("provider test status=%d body=%s", providerTest.Code, providerTest.Body.String())
	}
	missingProviderTest := performAssistantRequest(router, http.MethodPost, "/api/v1/adk/providers/provider-missing/test", nil)
	if missingProviderTest.Code != http.StatusBadGateway || !strings.Contains(missingProviderTest.Body.String(), "provider not found") {
		t.Fatalf("missing provider test status=%d body=%s", missingProviderTest.Code, missingProviderTest.Body.String())
	}
}

func TestProviderDefaultContract(t *testing.T) {
	_, router := newAssistantTestRouter(t)

	for _, body := range []string{
		`{"id":"provider-default-a","displayName":"Provider A","apiKey":"sk-a","enabled":true}`,
		`{"id":"provider-default-b","displayName":"Provider B","apiKey":"sk-b","enabled":true}`,
	} {
		response := performAssistantRequest(router, http.MethodPost, "/api/v1/adk/providers", []byte(body))
		if response.Code != http.StatusOK {
			t.Fatalf("create provider status=%d body=%s", response.Code, response.Body.String())
		}
	}

	setDefault := performAssistantRequest(router, http.MethodPost, "/api/v1/adk/providers/provider-default-b/default", nil)
	if setDefault.Code != http.StatusOK || !strings.Contains(setDefault.Body.String(), `"default":true`) {
		t.Fatalf("set default status=%d body=%s", setDefault.Code, setDefault.Body.String())
	}
	list := performAssistantRequest(router, http.MethodGet, "/api/v1/adk/providers", nil)
	if list.Code != http.StatusOK {
		t.Fatalf("list providers status=%d body=%s", list.Code, list.Body.String())
	}
	body := list.Body.String()
	if !strings.Contains(body, "provider-default-b") ||
		!strings.Contains(body, "provider-default-a") ||
		strings.Index(body, "provider-default-b") > strings.Index(body, "provider-default-a") {
		t.Fatalf("providers body = %s, want default provider first", body)
	}

	missing := performAssistantRequest(router, http.MethodPost, "/api/v1/adk/providers/provider-default-missing/default", nil)
	if missing.Code != http.StatusNotFound {
		t.Fatalf("missing default status=%d body=%s, want 404", missing.Code, missing.Body.String())
	}
}

func TestSessionRunAndOptimizationRouteContracts(t *testing.T) {
	runtime, router := newAssistantTestRouter(t)
	ctx := t.Context()

	disabledAgent, err := runtime.Store().SaveAgent(ctx, jfadk.AgentWriteRequest{
		ID: "agent-session-disabled", Name: "Disabled Agent", Status: jfadk.AgentStatusDisabled,
	})
	if err != nil {
		t.Fatalf("SaveAgent disabled: %v", err)
	}
	createDisabled := performAssistantRequest(router, http.MethodPost, "/api/v1/adk/sessions", []byte(`{
		"agentId":"`+disabledAgent.ID+`",
		"title":"should fail"
	}`))
	if createDisabled.Code != http.StatusBadRequest || !strings.Contains(createDisabled.Body.String(), "enabled agent is required") {
		t.Fatalf("create disabled agent session status=%d body=%s", createDisabled.Code, createDisabled.Body.String())
	}

	enabledAgent, err := runtime.Store().SaveAgent(ctx, jfadk.AgentWriteRequest{
		ID: "agent-session-enabled", Name: "Enabled Agent", Status: jfadk.AgentStatusEnabled, WorkMode: jfadk.WorkModeLoop,
	})
	if err != nil {
		t.Fatalf("SaveAgent enabled: %v", err)
	}
	session, err := runtime.Store().CreateSession(ctx, enabledAgent.ID, "Session Contract")
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	if response := performAssistantRequest(router, http.MethodGet, "/api/v1/adk/sessions/session-missing", nil); response.Code != http.StatusNotFound {
		t.Fatalf("missing session status=%d body=%s", response.Code, response.Body.String())
	}
	if response := performAssistantRequest(router, http.MethodGet, "/api/v1/adk/sessions/session-missing/context", nil); response.Code != http.StatusNotFound {
		t.Fatalf("missing session context status=%d body=%s", response.Code, response.Body.String())
	}
	if response := performAssistantRequest(router, http.MethodPatch, "/api/v1/adk/sessions/session-missing/composer-state", []byte(`{"workModeOverride":"loop"}`)); response.Code != http.StatusNotFound {
		t.Fatalf("missing composer state status=%d body=%s", response.Code, response.Body.String())
	}
	if response := performAssistantRequest(router, http.MethodPost, "/api/v1/adk/sessions/session-missing/context/compact", []byte(`{"mode":"normal"}`)); response.Code != http.StatusNotFound {
		t.Fatalf("missing compact status=%d body=%s", response.Code, response.Body.String())
	}
	renameSession := performAssistantRequest(router, http.MethodPut, "/api/v1/adk/sessions/"+session.ID, []byte(`{"title":"Renamed Session Contract"}`))
	if renameSession.Code != http.StatusOK || !strings.Contains(renameSession.Body.String(), "Renamed Session Contract") {
		t.Fatalf("rename session status=%d body=%s", renameSession.Code, renameSession.Body.String())
	}

	activeRun := jfadk.Run{
		ID: "run-active-compact", SessionID: session.ID, AgentID: enabledAgent.ID, WorkMode: jfadk.WorkModeLoop, Status: jfadk.RunStatusRunning,
		Objective: "monitor market", CreatedAt: time.Now().UTC().Format(time.RFC3339Nano), UpdatedAt: time.Now().UTC().Format(time.RFC3339Nano),
	}
	if err := runtime.Store().SaveRun(ctx, activeRun); err != nil {
		t.Fatalf("SaveRun active: %v", err)
	}
	compactActive := performAssistantRequest(router, http.MethodPost, "/api/v1/adk/sessions/"+session.ID+"/context/compact", []byte(`{"mode":"normal","reason":"manual"}`))
	if compactActive.Code != http.StatusConflict || !strings.Contains(compactActive.Body.String(), "active run") {
		t.Fatalf("active compact status=%d body=%s", compactActive.Code, compactActive.Body.String())
	}

	updateObjective := performAssistantRequest(router, http.MethodPatch, "/api/v1/adk/runs/"+activeRun.ID+"/objective", []byte(`{"objective":"watch premarket liquidity"}`))
	if updateObjective.Code != http.StatusOK || !strings.Contains(updateObjective.Body.String(), "watch premarket liquidity") {
		t.Fatalf("update objective status=%d body=%s", updateObjective.Code, updateObjective.Body.String())
	}

	if response := performAssistantRequest(router, http.MethodGet, "/api/v1/adk/runs/run-missing", nil); response.Code != http.StatusNotFound {
		t.Fatalf("missing run status=%d body=%s", response.Code, response.Body.String())
	}
	if response := performAssistantRequest(router, http.MethodPost, "/api/v1/adk/runs/run-missing/pause", nil); response.Code != http.StatusNotFound {
		t.Fatalf("missing pause status=%d body=%s", response.Code, response.Body.String())
	}
	if response := performAssistantRequest(router, http.MethodPost, "/api/v1/adk/runs/run-missing/resume", nil); response.Code != http.StatusNotFound {
		t.Fatalf("missing resume status=%d body=%s", response.Code, response.Body.String())
	}

	cancelRun := jfadk.Run{
		ID:        "run-cancel-route",
		SessionID: session.ID,
		AgentID:   enabledAgent.ID,
		Status:    jfadk.RunStatusPending,
		CreatedAt: time.Now().UTC().Format(time.RFC3339Nano),
		UpdatedAt: time.Now().UTC().Format(time.RFC3339Nano),
	}
	if err := runtime.Store().SaveRun(ctx, cancelRun); err != nil {
		t.Fatalf("SaveRun cancel route: %v", err)
	}
	cancelResponse := performAssistantRequest(router, http.MethodPost, "/api/v1/adk/runs/"+cancelRun.ID+"/cancel", nil)
	if cancelResponse.Code != http.StatusOK || !strings.Contains(cancelResponse.Body.String(), jfadk.RunStatusCancelled) {
		t.Fatalf("cancel run status=%d body=%s", cancelResponse.Code, cancelResponse.Body.String())
	}

	if _, err := runtime.Store().SaveOptimizationTask(ctx, jfadk.OptimizationTask{
		ID: "optimization-route", Status: "queued", Objective: "maximize sharpe",
		CreatedAt: time.Now().UTC().Format(time.RFC3339Nano), UpdatedAt: time.Now().UTC().Format(time.RFC3339Nano),
	}); err != nil {
		t.Fatalf("SaveOptimizationTask: %v", err)
	}
	optList := performAssistantRequest(router, http.MethodGet, "/api/v1/adk/optimization-tasks?limit=1&offset=0", nil)
	if optList.Code != http.StatusOK || !strings.Contains(optList.Body.String(), "optimization-route") {
		t.Fatalf("list optimization tasks status=%d body=%s", optList.Code, optList.Body.String())
	}
	if response := performAssistantRequest(router, http.MethodGet, "/api/v1/adk/optimization-tasks/task-missing", nil); response.Code != http.StatusNotFound {
		t.Fatalf("missing optimization task status=%d body=%s", response.Code, response.Body.String())
	}
	if response := performAssistantRequest(router, http.MethodPost, "/api/v1/adk/optimization-tasks/task-missing/cancel", nil); response.Code != http.StatusNotFound {
		t.Fatalf("missing optimization cancel status=%d body=%s", response.Code, response.Body.String())
	}

	deleteSession := performAssistantRequest(router, http.MethodDelete, "/api/v1/adk/sessions/"+session.ID, nil)
	if deleteSession.Code != http.StatusOK {
		t.Fatalf("delete session status=%d body=%s", deleteSession.Code, deleteSession.Body.String())
	}
	if response := performAssistantRequest(router, http.MethodGet, "/api/v1/adk/sessions/"+session.ID, nil); response.Code != http.StatusNotFound {
		t.Fatalf("deleted session lookup status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestStreamReconnectAndSkillContracts(t *testing.T) {
	runtime, router := newAssistantTestRouter(t)
	ctx := t.Context()

	agent, err := runtime.Store().SaveAgent(ctx, jfadk.AgentWriteRequest{
		ID: "agent-stream-reconnect", Name: "Stream Reconnect", ProviderID: "test-provider", Status: jfadk.AgentStatusEnabled,
	})
	if err != nil {
		t.Fatalf("SaveAgent: %v", err)
	}

	stream := performAssistantRequest(router, http.MethodPost, "/api/v1/adk/chat/stream", []byte(`{"agentId":"`+agent.ID+`","message":"hello reconnect"}`))
	if stream.Code != http.StatusOK {
		t.Fatalf("stream status=%d body=%s", stream.Code, stream.Body.String())
	}
	streamID := stream.Header().Get("X-ADK-Stream-ID")
	if streamID == "" {
		t.Fatalf("missing stream id header, headers=%v", stream.Header())
	}
	if !strings.Contains(stream.Body.String(), `"type":"final"`) {
		t.Fatalf("stream body missing final event: %s", stream.Body.String())
	}

	runs, err := runtime.Store().ListRuns(ctx)
	if err != nil || len(runs) == 0 {
		t.Fatalf("ListRuns() runs=%v err=%v", runs, err)
	}
	runID := runs[0].ID

	if response := performAssistantRequest(router, http.MethodGet, "/api/v1/adk/streams/"+streamID+"?after=-1", nil); response.Code != http.StatusBadRequest {
		t.Fatalf("invalid after status=%d body=%s", response.Code, response.Body.String())
	}
	streamReplay := performAssistantRequest(router, http.MethodGet, "/api/v1/adk/streams/"+streamID+"?after=1", nil)
	if streamReplay.Code != http.StatusOK || !strings.Contains(streamReplay.Body.String(), `"replay":true`) {
		t.Fatalf("stream replay status=%d body=%s", streamReplay.Code, streamReplay.Body.String())
	}
	runReplay := performAssistantRequest(router, http.MethodGet, "/api/v1/adk/runs/"+runID+"/stream?after=1", nil)
	if runReplay.Code != http.StatusOK || !strings.Contains(runReplay.Body.String(), `"replay":true`) {
		t.Fatalf("run replay status=%d body=%s", runReplay.Code, runReplay.Body.String())
	}
	if response := performAssistantRequest(router, http.MethodGet, "/api/v1/adk/streams/stream-missing", nil); response.Code != http.StatusNotFound {
		t.Fatalf("missing stream status=%d body=%s", response.Code, response.Body.String())
	}
	if response := performAssistantRequest(router, http.MethodGet, "/api/v1/adk/runs/run-missing/stream", nil); response.Code != http.StatusNotFound {
		t.Fatalf("missing run stream status=%d body=%s", response.Code, response.Body.String())
	}

	skillList := performAssistantRequest(router, http.MethodGet, "/api/v1/adk/skills", nil)
	if skillList.Code != http.StatusOK || !strings.Contains(skillList.Body.String(), "jftrade-market") {
		t.Fatalf("skill list status=%d body=%s", skillList.Code, skillList.Body.String())
	}
	skillRemoved := performAssistantRequest(router, http.MethodPut, "/api/v1/adk/skills/jftrade-market", nil)
	if skillRemoved.Code != http.StatusGone || !strings.Contains(skillRemoved.Body.String(), "ADK_SKILL_UPDATE_REMOVED") {
		t.Fatalf("skill removed status=%d body=%s", skillRemoved.Code, skillRemoved.Body.String())
	}
	skillInstallInvalid := performAssistantRequest(router, http.MethodPost, "/api/v1/adk/skills", []byte(`{"url":"ftp://invalid-skill"}`))
	if skillInstallInvalid.Code != http.StatusBadRequest || !strings.Contains(skillInstallInvalid.Body.String(), "valid http/https skill URL is required") {
		t.Fatalf("skill install invalid status=%d body=%s", skillInstallInvalid.Code, skillInstallInvalid.Body.String())
	}

	externalDir := filepath.Join(runtime.Store().SkillsPath(), "external-skill")
	if err := os.MkdirAll(externalDir, 0o755); err != nil {
		t.Fatalf("MkdirAll external skill: %v", err)
	}
	if err := os.WriteFile(filepath.Join(externalDir, "SKILL.md"), []byte("---\nname: external-skill\ndescription: external skill\nmetadata:\n  source: https://example.com/SKILL.md\n---\nUse external references carefully.\n"), 0o644); err != nil {
		t.Fatalf("WriteFile external skill: %v", err)
	}
	skillList = performAssistantRequest(router, http.MethodGet, "/api/v1/adk/skills", nil)
	if skillList.Code != http.StatusOK || !strings.Contains(skillList.Body.String(), "external-skill") {
		t.Fatalf("skill list with external status=%d body=%s", skillList.Code, skillList.Body.String())
	}
	deleteExternal := performAssistantRequest(router, http.MethodDelete, "/api/v1/adk/skills/external-skill", nil)
	if deleteExternal.Code != http.StatusOK {
		t.Fatalf("delete external skill status=%d body=%s", deleteExternal.Code, deleteExternal.Body.String())
	}
	deleteBuiltin := performAssistantRequest(router, http.MethodDelete, "/api/v1/adk/skills/jftrade-market", nil)
	if deleteBuiltin.Code != http.StatusInternalServerError || !strings.Contains(deleteBuiltin.Body.String(), "cannot be uninstalled") {
		t.Fatalf("delete builtin skill status=%d body=%s", deleteBuiltin.Code, deleteBuiltin.Body.String())
	}
}
