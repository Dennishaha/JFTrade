package assistant

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	jfadk "github.com/jftrade/jftrade-main/pkg/adk"
)

func TestWorkflowRoutesCoverDefinitionTriggerRunAndWebhookContracts(t *testing.T) {
	runtime, router := newAssistantTestRouter(t)
	ctx := t.Context()
	if _, err := runtime.Store().SaveAgent(ctx, jfadk.AgentWriteRequest{
		ID: "agent-workflow-route", Name: "Workflow Route Agent", ProviderID: "test-provider",
		Status: jfadk.AgentStatusEnabled, WorkMode: jfadk.WorkModeChat, PermissionMode: jfadk.PermissionModeAll,
	}); err != nil {
		t.Fatalf("SaveAgent: %v", err)
	}

	createWorkflow := performAssistantRequest(router, http.MethodPost, "/api/v1/adk/workflows", []byte(`{
		"id":"workflow-route-review",
		"name":"Route Review Workflow",
		"description":"复盘单只股票的盘前风险",
		"status":"ENABLED",
		"agentId":"agent-workflow-route",
		"workMode":"chat",
		"permissionMode":"all",
		"promptTemplate":"复盘 {{.symbol}} 在 {{.session}} 的交易风险",
		"objectiveTemplate":"形成 {{.symbol}} 的风险摘要",
		"defaultInputs":{"symbol":"US.AAPL","session":"盘前"},
		"tags":["route","workflow"],
		"canvasGraph":{
			"version":"adk-workflow-canvas/v1",
			"nodes":[
				{"id":"start","type":"start","position":{"x":0,"y":0}},
				{"id":"agent:primary","type":"agent","position":{"x":160,"y":0}},
				{"id":"monitor","type":"monitor","position":{"x":320,"y":0}}
			],
			"edges":[
				{"id":"start-agent","source":"start","target":"agent:primary"},
				{"id":"agent-monitor","source":"agent:primary","target":"monitor"}
			]
		}
	}`))
	if createWorkflow.Code != http.StatusOK {
		t.Fatalf("create workflow status=%d body=%s", createWorkflow.Code, createWorkflow.Body.String())
	}
	var workflowEnvelope struct {
		OK   bool                     `json:"ok"`
		Data jfadk.WorkflowDefinition `json:"data"`
	}
	if err := json.Unmarshal(createWorkflow.Body.Bytes(), &workflowEnvelope); err != nil {
		t.Fatalf("decode workflow create: %v", err)
	}
	if !workflowEnvelope.OK || workflowEnvelope.Data.ID != "workflow-route-review" || workflowEnvelope.Data.CanvasGraph == nil {
		t.Fatalf("workflow create envelope=%s", createWorkflow.Body.String())
	}

	updateWorkflow := performAssistantRequest(router, http.MethodPut, "/api/v1/adk/workflows/workflow-route-review", []byte(`{
		"name":"Route Review Workflow Updated",
		"status":"ENABLED",
		"agentId":"agent-workflow-route",
		"workMode":"chat",
		"permissionMode":"all",
		"promptTemplate":"复盘 {{.symbol}} 在 {{.session}} 的交易风险",
		"defaultInputs":{"symbol":"US.AAPL","session":"盘中"},
		"canvasGraph":{
			"version":"adk-workflow-canvas/v1",
			"nodes":[
				{"id":"start","type":"start","position":{"x":0,"y":0}},
				{"id":"agent:primary","type":"agent","position":{"x":160,"y":0}},
				{"id":"monitor","type":"monitor","position":{"x":320,"y":0}}
			],
			"edges":[
				{"id":"start-agent","source":"start","target":"agent:primary"},
				{"id":"agent-monitor","source":"agent:primary","target":"monitor"}
			]
		}
	}`))
	if updateWorkflow.Code != http.StatusOK || !strings.Contains(updateWorkflow.Body.String(), "Route Review Workflow Updated") {
		t.Fatalf("update workflow status=%d body=%s", updateWorkflow.Code, updateWorkflow.Body.String())
	}

	listWorkflow := performAssistantRequest(router, http.MethodGet, "/api/v1/adk/workflows?status=ENABLED&limit=1&offset=0", nil)
	if listWorkflow.Code != http.StatusOK || !strings.Contains(listWorkflow.Body.String(), `"returned":1`) {
		t.Fatalf("list workflow status=%d body=%s", listWorkflow.Code, listWorkflow.Body.String())
	}
	getWorkflow := performAssistantRequest(router, http.MethodGet, "/api/v1/adk/workflows/workflow-route-review", nil)
	if getWorkflow.Code != http.StatusOK || !strings.Contains(getWorkflow.Body.String(), `"session":"盘中"`) {
		t.Fatalf("get workflow status=%d body=%s", getWorkflow.Code, getWorkflow.Body.String())
	}

	runWorkflow := performAssistantRequest(router, http.MethodPost, "/api/v1/adk/workflows/workflow-route-review/run", []byte(`{"inputs":{"symbol":"US.MSFT","session":"盘后"}}`))
	if runWorkflow.Code != http.StatusOK {
		t.Fatalf("run workflow status=%d body=%s", runWorkflow.Code, runWorkflow.Body.String())
	}
	var runEnvelope struct {
		OK   bool `json:"ok"`
		Data struct {
			Log      jfadk.WorkflowTriggerLog `json:"log"`
			Response *jfadk.ChatResponse      `json:"response"`
		} `json:"data"`
	}
	if err := json.Unmarshal(runWorkflow.Body.Bytes(), &runEnvelope); err != nil {
		t.Fatalf("decode workflow run: %v", err)
	}
	if !runEnvelope.OK || runEnvelope.Data.Log.Status != jfadk.WorkflowTriggerLogStatusSucceeded || runEnvelope.Data.Response == nil {
		t.Fatalf("run workflow envelope=%s", runWorkflow.Body.String())
	}

	manualTrigger := performAssistantRequest(router, http.MethodPost, "/api/v1/adk/workflows/workflow-route-review/triggers", []byte(`{
		"id":"workflow-route-manual",
		"type":"manual",
		"title":"手动复盘",
		"status":"ENABLED"
	}`))
	if manualTrigger.Code != http.StatusOK {
		t.Fatalf("create manual trigger status=%d body=%s", manualTrigger.Code, manualTrigger.Body.String())
	}
	runTrigger := performAssistantRequest(router, http.MethodPost, "/api/v1/adk/workflow-triggers/workflow-route-manual/run", []byte(`{"symbol":"HK.00700","session":"午盘"}`))
	if runTrigger.Code != http.StatusOK || !strings.Contains(runTrigger.Body.String(), `"triggerId":"workflow-route-manual"`) {
		t.Fatalf("run manual trigger status=%d body=%s", runTrigger.Code, runTrigger.Body.String())
	}

	webhookTrigger := performAssistantRequest(router, http.MethodPost, "/api/v1/adk/workflows/workflow-route-review/triggers", []byte(`{
		"id":"workflow-route-webhook",
		"type":"webhook",
		"title":"外部风险事件",
		"status":"ENABLED"
	}`))
	if webhookTrigger.Code != http.StatusOK {
		t.Fatalf("create webhook trigger status=%d body=%s", webhookTrigger.Code, webhookTrigger.Body.String())
	}
	var webhookEnvelope struct {
		OK   bool `json:"ok"`
		Data struct {
			Trigger jfadk.WorkflowTrigger `json:"trigger"`
			Secret  string                `json:"secret"`
		} `json:"data"`
	}
	if err := json.Unmarshal(webhookTrigger.Body.Bytes(), &webhookEnvelope); err != nil {
		t.Fatalf("decode webhook trigger: %v", err)
	}
	if !webhookEnvelope.OK || webhookEnvelope.Data.Secret == "" || !webhookEnvelope.Data.Trigger.HasSecret || webhookEnvelope.Data.Trigger.SecretHash != "" {
		t.Fatalf("webhook trigger envelope=%s", webhookTrigger.Body.String())
	}

	badWebhook := performAssistantRequestWithHeaders(router, http.MethodPost, "/api/v1/adk/workflow-webhooks/workflow-route-webhook", []byte(`{"inputs":{"symbol":"US.TSLA"}}`), map[string]string{
		"Authorization": "Bearer wrong-secret",
	})
	assertAssistantErrorCode(t, badWebhook, http.StatusUnauthorized, "ADK_WORKFLOW_WEBHOOK_FAILED")

	goodWebhook := performAssistantRequestWithHeaders(router, http.MethodPost, "/api/v1/adk/workflow-webhooks/workflow-route-webhook", []byte(`{"inputs":{"symbol":"US.TSLA","session":"新闻后"}}`), map[string]string{
		"X-JFTrade-Workflow-Secret": webhookEnvelope.Data.Secret,
	})
	if goodWebhook.Code != http.StatusOK || !strings.Contains(goodWebhook.Body.String(), `"type":"workflow.webhook"`) {
		t.Fatalf("good webhook status=%d body=%s", goodWebhook.Code, goodWebhook.Body.String())
	}

	logs := performAssistantRequest(router, http.MethodGet, "/api/v1/adk/workflow-trigger-logs?workflowId=workflow-route-review&triggerId=workflow-route-webhook&status=SUCCEEDED&limit=5", nil)
	if logs.Code != http.StatusOK || !strings.Contains(logs.Body.String(), `"triggerId":"workflow-route-webhook"`) {
		t.Fatalf("workflow logs status=%d body=%s", logs.Code, logs.Body.String())
	}
	triggers := performAssistantRequest(router, http.MethodGet, "/api/v1/adk/workflows/workflow-route-review/triggers", nil)
	if triggers.Code != http.StatusOK || !strings.Contains(triggers.Body.String(), `"hasSecret":true`) {
		t.Fatalf("list triggers status=%d body=%s", triggers.Code, triggers.Body.String())
	}

	deleteTrigger := performAssistantRequest(router, http.MethodDelete, "/api/v1/adk/workflows/workflow-route-review/triggers/workflow-route-manual", nil)
	if deleteTrigger.Code != http.StatusOK || !strings.Contains(deleteTrigger.Body.String(), `"deleted":true`) {
		t.Fatalf("delete trigger status=%d body=%s", deleteTrigger.Code, deleteTrigger.Body.String())
	}
	deleteWorkflow := performAssistantRequest(router, http.MethodDelete, "/api/v1/adk/workflows/workflow-route-review", nil)
	if deleteWorkflow.Code != http.StatusOK || !strings.Contains(deleteWorkflow.Body.String(), `"deleted":true`) {
		t.Fatalf("delete workflow status=%d body=%s", deleteWorkflow.Code, deleteWorkflow.Body.String())
	}
	missingAfterDelete := performAssistantRequest(router, http.MethodGet, "/api/v1/adk/workflows/workflow-route-review", nil)
	assertAssistantErrorCode(t, missingAfterDelete, http.StatusNotFound, "ADK_WORKFLOW_GET_FAILED")
}

func TestWorkflowRoutesClassifyInvalidPayloadsAndUnavailableRuns(t *testing.T) {
	runtime, router := newAssistantTestRouter(t)
	ctx := t.Context()
	if _, err := runtime.Store().SaveAgent(ctx, jfadk.AgentWriteRequest{
		ID: "agent-workflow-errors", Name: "Workflow Error Agent", ProviderID: "test-provider",
		Status: jfadk.AgentStatusEnabled, WorkMode: jfadk.WorkModeChat, PermissionMode: jfadk.PermissionModeAll,
	}); err != nil {
		t.Fatalf("SaveAgent: %v", err)
	}

	invalidSave := performAssistantRequest(router, http.MethodPost, "/api/v1/adk/workflows", []byte(`{`))
	assertAssistantErrorCode(t, invalidSave, http.StatusBadRequest, "BAD_REQUEST")

	missingWorkflow := performAssistantRequest(router, http.MethodGet, "/api/v1/adk/workflows/workflow-missing", nil)
	assertAssistantErrorCode(t, missingWorkflow, http.StatusNotFound, "ADK_WORKFLOW_GET_FAILED")

	disabled := performAssistantRequest(router, http.MethodPost, "/api/v1/adk/workflows", []byte(`{
		"id":"workflow-disabled-route",
		"name":"Disabled Workflow",
		"status":"DISABLED",
		"agentId":"agent-workflow-errors",
		"workMode":"chat",
		"permissionMode":"all",
		"promptTemplate":"这条 workflow 不应运行"
	}`))
	if disabled.Code != http.StatusOK {
		t.Fatalf("create disabled workflow status=%d body=%s", disabled.Code, disabled.Body.String())
	}
	runDisabled := performAssistantRequest(router, http.MethodPost, "/api/v1/adk/workflows/workflow-disabled-route/run", []byte(`{"inputs":{"symbol":"US.AAPL"}}`))
	assertAssistantErrorCode(t, runDisabled, http.StatusConflict, "ADK_WORKFLOW_RUN_FAILED")

	invalidRunInputs := performAssistantRequest(router, http.MethodPost, "/api/v1/adk/workflows/workflow-disabled-route/run", []byte(`{`))
	assertAssistantErrorCode(t, invalidRunInputs, http.StatusBadRequest, "BAD_REQUEST")

	invalidTriggerPayload := performAssistantRequest(router, http.MethodPost, "/api/v1/adk/workflows/workflow-disabled-route/triggers", []byte(`{`))
	assertAssistantErrorCode(t, invalidTriggerPayload, http.StatusBadRequest, "BAD_REQUEST")

	missingTriggerList := performAssistantRequest(router, http.MethodGet, "/api/v1/adk/workflows/workflow-missing/triggers", nil)
	assertAssistantErrorCode(t, missingTriggerList, http.StatusNotFound, "ADK_WORKFLOW_TRIGGER_LIST_FAILED")

	missingTriggerRun := performAssistantRequest(router, http.MethodPost, "/api/v1/adk/workflow-triggers/trigger-missing/run", nil)
	assertAssistantErrorCode(t, missingTriggerRun, http.StatusNotFound, "ADK_WORKFLOW_TRIGGER_RUN_FAILED")

	missingWebhook := performAssistantRequestWithHeaders(router, http.MethodPost, "/api/v1/adk/workflow-webhooks/trigger-missing", nil, map[string]string{
		"Authorization": "Bearer secret",
	})
	assertAssistantErrorCode(t, missingWebhook, http.StatusBadRequest, "ADK_WORKFLOW_WEBHOOK_FAILED")
}

func performAssistantRequestWithHeaders(router http.Handler, method string, path string, body []byte, headers map[string]string) *httptest.ResponseRecorder {
	reader := bytes.NewReader(body)
	request := httptest.NewRequest(method, path, reader)
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	for key, value := range headers {
		request.Header.Set(key, value)
	}
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	return recorder
}
