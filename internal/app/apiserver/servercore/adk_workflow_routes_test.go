package servercore

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	asst "github.com/jftrade/jftrade-main/internal/assistant"
	jfadk "github.com/jftrade/jftrade-main/pkg/adk"
)

func TestADKWorkflowDefinitionTriggerAndRunRoutes(t *testing.T) {
	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	server := newTestServer(t, store)
	srv := httptest.NewServer(server)
	t.Cleanup(srv.Close)

	agent, err := server.adkRuntime.Store().SaveAgent(t.Context(), jfadk.AgentWriteRequest{
		ID: "workflow-route-agent", Name: "Workflow Route", ProviderID: testADKProviderID,
		Status: jfadk.AgentStatusEnabled, WorkMode: jfadk.WorkModeChat,
	})
	if err != nil {
		t.Fatalf("SaveAgent: %v", err)
	}

	createBody := map[string]any{
		"id":             "workflow-route",
		"name":           "Route Workflow",
		"description":    "route coverage",
		"status":         jfadk.WorkflowStatusEnabled,
		"agentId":        agent.ID,
		"workMode":       jfadk.WorkModeChat,
		"promptTemplate": "Review {{ .symbol }} from {{ .source }}",
		"defaultInputs":  map[string]any{"symbol": "US.AAPL", "source": "default"},
		"canvasGraph": map[string]any{
			"version": "1",
			"nodes": []map[string]any{{
				"id": "start", "type": "start",
				"position": map[string]any{"x": 0, "y": 0},
			}},
		},
		"tags": []string{"route", "workflow"},
	}
	createPayload, err := json.Marshal(createBody)
	if err != nil {
		t.Fatalf("marshal workflow payload: %v", err)
	}
	createResp, err := jftradeTestHTTPPost(t, srv.URL+"/api/v1/adk/workflows", "application/json", bytes.NewReader(createPayload))
	if err != nil {
		t.Fatalf("POST workflow: %v", err)
	}
	defer func() { jftradeCheckTestError(t, createResp.Body.Close()) }()
	if createResp.StatusCode != http.StatusOK {
		t.Fatalf("POST workflow status = %d", createResp.StatusCode)
	}
	var createEnvelope struct {
		OK   bool                     `json:"ok"`
		Data jfadk.WorkflowDefinition `json:"data"`
	}
	if err := json.NewDecoder(createResp.Body).Decode(&createEnvelope); err != nil {
		t.Fatalf("decode workflow create: %v", err)
	}
	if !createEnvelope.OK || createEnvelope.Data.ID != "workflow-route" || createEnvelope.Data.CanvasGraph == nil {
		t.Fatalf("workflow create envelope = %+v, want saved workflow with canvas graph", createEnvelope)
	}

	listResp, err := jftradeTestHTTPGet(t, srv.URL+"/api/v1/adk/workflows?status=ENABLED&limit=1")
	if err != nil {
		t.Fatalf("GET workflows: %v", err)
	}
	defer func() { jftradeCheckTestError(t, listResp.Body.Close()) }()
	if listResp.StatusCode != http.StatusOK {
		t.Fatalf("GET workflows status = %d", listResp.StatusCode)
	}
	var listEnvelope struct {
		OK   bool `json:"ok"`
		Data struct {
			Workflows []jfadk.WorkflowDefinition `json:"workflows"`
			Page      struct {
				Returned int  `json:"returned"`
				HasMore  bool `json:"hasMore"`
			} `json:"page"`
		} `json:"data"`
	}
	if err := json.NewDecoder(listResp.Body).Decode(&listEnvelope); err != nil {
		t.Fatalf("decode workflow list: %v", err)
	}
	if !listEnvelope.OK || listEnvelope.Data.Page.Returned == 0 || listEnvelope.Data.Workflows[0].ID != "workflow-route" {
		t.Fatalf("workflow list envelope = %+v, want created workflow", listEnvelope)
	}

	getResp, err := jftradeTestHTTPGet(t, srv.URL+"/api/v1/adk/workflows/workflow-route")
	if err != nil {
		t.Fatalf("GET workflow: %v", err)
	}
	defer func() { jftradeCheckTestError(t, getResp.Body.Close()) }()
	if getResp.StatusCode != http.StatusOK {
		t.Fatalf("GET workflow status = %d", getResp.StatusCode)
	}

	updateBody := createBody
	updateBody["name"] = "Route Workflow Updated"
	updateBody["defaultInputs"] = map[string]any{"symbol": "US.MSFT", "source": "updated"}
	updatePayload, err := json.Marshal(updateBody)
	if err != nil {
		t.Fatalf("marshal workflow update: %v", err)
	}
	updateReq, err := http.NewRequestWithContext(t.Context(), http.MethodPut, srv.URL+"/api/v1/adk/workflows/workflow-route", bytes.NewReader(updatePayload))
	if err != nil {
		t.Fatalf("NewRequest update workflow: %v", err)
	}
	updateReq.Header.Set("Content-Type", "application/json")
	updateResp, err := http.DefaultClient.Do(updateReq)
	if err != nil {
		t.Fatalf("PUT workflow: %v", err)
	}
	defer func() { jftradeCheckTestError(t, updateResp.Body.Close()) }()
	if updateResp.StatusCode != http.StatusOK {
		t.Fatalf("PUT workflow status = %d", updateResp.StatusCode)
	}

	runResp, err := jftradeTestHTTPPost(t, srv.URL+"/api/v1/adk/workflows/workflow-route/run", "application/json", strings.NewReader(`{"inputs":{"source":"manual"}}`))
	if err != nil {
		t.Fatalf("POST workflow run: %v", err)
	}
	defer func() { jftradeCheckTestError(t, runResp.Body.Close()) }()
	if runResp.StatusCode != http.StatusOK {
		t.Fatalf("POST workflow run status = %d", runResp.StatusCode)
	}
	var runEnvelope struct {
		OK   bool                          `json:"ok"`
		Data asst.WorkflowInvocationResult `json:"data"`
	}
	if err := json.NewDecoder(runResp.Body).Decode(&runEnvelope); err != nil {
		t.Fatalf("decode workflow run: %v", err)
	}
	if !runEnvelope.OK || runEnvelope.Data.Log.Status != "SUCCEEDED" || runEnvelope.Data.Log.SessionID == "" || runEnvelope.Data.Log.RunID == "" {
		t.Fatalf("workflow run envelope = %+v, want successful run with session/run", runEnvelope)
	}

	triggerResp, err := jftradeTestHTTPPost(t, srv.URL+"/api/v1/adk/workflows/workflow-route/triggers", "application/json", strings.NewReader(`{"id":"workflow-route-webhook","type":"webhook","title":"Route webhook","status":"ENABLED","config":{"source":"external","unknownLegacy":true}}`))
	if err != nil {
		t.Fatalf("POST workflow trigger: %v", err)
	}
	defer func() { jftradeCheckTestError(t, triggerResp.Body.Close()) }()
	if triggerResp.StatusCode != http.StatusOK {
		t.Fatalf("POST workflow trigger status = %d", triggerResp.StatusCode)
	}
	var triggerEnvelope struct {
		OK   bool                           `json:"ok"`
		Data asst.WorkflowTriggerSaveResult `json:"data"`
	}
	if err := json.NewDecoder(triggerResp.Body).Decode(&triggerEnvelope); err != nil {
		t.Fatalf("decode workflow trigger: %v", err)
	}
	if !triggerEnvelope.OK || triggerEnvelope.Data.Secret == "" || !triggerEnvelope.Data.Trigger.HasSecret || triggerEnvelope.Data.Trigger.SecretHash != "" {
		t.Fatalf("workflow trigger envelope = %+v, want one-time secret and sanitized trigger", triggerEnvelope)
	}

	triggersResp, err := jftradeTestHTTPGet(t, srv.URL+"/api/v1/adk/workflows/workflow-route/triggers")
	if err != nil {
		t.Fatalf("GET workflow triggers: %v", err)
	}
	defer func() { jftradeCheckTestError(t, triggersResp.Body.Close()) }()
	if triggersResp.StatusCode != http.StatusOK {
		t.Fatalf("GET workflow triggers status = %d", triggersResp.StatusCode)
	}

	badWebhookResp, err := jftradeTestHTTPPost(t, srv.URL+"/api/v1/adk/workflow-webhooks/workflow-route-webhook", "application/json", strings.NewReader(`{"inputs":{"source":"bad-secret"}}`))
	if err != nil {
		t.Fatalf("POST bad webhook: %v", err)
	}
	defer func() { jftradeCheckTestError(t, badWebhookResp.Body.Close()) }()
	status, code, _ := decodeAPIErrorEnvelope(t, badWebhookResp)
	if status != http.StatusUnauthorized || code != "ADK_WORKFLOW_WEBHOOK_FAILED" {
		t.Fatalf("bad webhook error = %d/%s, want unauthorized workflow webhook failure", status, code)
	}

	webhookReq, err := http.NewRequestWithContext(t.Context(), http.MethodPost, srv.URL+"/api/v1/adk/workflow-webhooks/workflow-route-webhook", strings.NewReader(`{"inputs":{"source":"webhook"}}`))
	if err != nil {
		t.Fatalf("NewRequest webhook: %v", err)
	}
	webhookReq.Header.Set("Content-Type", "application/json")
	webhookReq.Header.Set("Authorization", "Bearer "+triggerEnvelope.Data.Secret)
	webhookResp, err := http.DefaultClient.Do(webhookReq)
	if err != nil {
		t.Fatalf("POST webhook: %v", err)
	}
	defer func() { jftradeCheckTestError(t, webhookResp.Body.Close()) }()
	if webhookResp.StatusCode != http.StatusOK {
		t.Fatalf("POST webhook status = %d", webhookResp.StatusCode)
	}

	logsResp, err := jftradeTestHTTPGet(t, srv.URL+"/api/v1/adk/workflow-trigger-logs?workflowId=workflow-route&triggerId=workflow-route-webhook&status=SUCCEEDED&limit=2")
	if err != nil {
		t.Fatalf("GET workflow trigger logs: %v", err)
	}
	defer func() { jftradeCheckTestError(t, logsResp.Body.Close()) }()
	if logsResp.StatusCode != http.StatusOK {
		t.Fatalf("GET workflow trigger logs status = %d", logsResp.StatusCode)
	}
	var logsEnvelope struct {
		OK   bool `json:"ok"`
		Data struct {
			Logs []jfadk.WorkflowTriggerLog `json:"logs"`
		} `json:"data"`
	}
	if err := json.NewDecoder(logsResp.Body).Decode(&logsEnvelope); err != nil {
		t.Fatalf("decode workflow logs: %v", err)
	}
	if !logsEnvelope.OK || len(logsEnvelope.Data.Logs) == 0 || logsEnvelope.Data.Logs[0].TriggerID != "workflow-route-webhook" {
		t.Fatalf("workflow logs envelope = %+v, want webhook log", logsEnvelope)
	}

	deleteTriggerReq, err := http.NewRequestWithContext(t.Context(), http.MethodDelete, srv.URL+"/api/v1/adk/workflows/workflow-route/triggers/workflow-route-webhook", nil)
	if err != nil {
		t.Fatalf("NewRequest delete workflow trigger: %v", err)
	}
	deleteTriggerResp, err := http.DefaultClient.Do(deleteTriggerReq)
	if err != nil {
		t.Fatalf("DELETE workflow trigger: %v", err)
	}
	defer func() { jftradeCheckTestError(t, deleteTriggerResp.Body.Close()) }()
	if deleteTriggerResp.StatusCode != http.StatusOK {
		t.Fatalf("DELETE workflow trigger status = %d", deleteTriggerResp.StatusCode)
	}

	deleteWorkflowReq, err := http.NewRequestWithContext(t.Context(), http.MethodDelete, srv.URL+"/api/v1/adk/workflows/workflow-route", nil)
	if err != nil {
		t.Fatalf("NewRequest delete workflow: %v", err)
	}
	deleteWorkflowResp, err := http.DefaultClient.Do(deleteWorkflowReq)
	if err != nil {
		t.Fatalf("DELETE workflow: %v", err)
	}
	defer func() { jftradeCheckTestError(t, deleteWorkflowResp.Body.Close()) }()
	if deleteWorkflowResp.StatusCode != http.StatusOK {
		t.Fatalf("DELETE workflow status = %d", deleteWorkflowResp.StatusCode)
	}
}

func TestADKWorkflowRoutesRejectInvalidInputs(t *testing.T) {
	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	server := newTestServer(t, store)
	srv := httptest.NewServer(server)
	t.Cleanup(srv.Close)

	invalidSaveResp, err := jftradeTestHTTPPost(t, srv.URL+"/api/v1/adk/workflows", "application/json", strings.NewReader(`{`))
	if err != nil {
		t.Fatalf("POST invalid workflow: %v", err)
	}
	defer func() { jftradeCheckTestError(t, invalidSaveResp.Body.Close()) }()
	status, code, message := decodeAPIErrorEnvelope(t, invalidSaveResp)
	if status != http.StatusBadRequest || code != "BAD_REQUEST" || message != "invalid workflow payload" {
		t.Fatalf("invalid workflow save = %d/%s/%q", status, code, message)
	}

	missingResp, err := jftradeTestHTTPGet(t, srv.URL+"/api/v1/adk/workflows/missing-workflow")
	if err != nil {
		t.Fatalf("GET missing workflow: %v", err)
	}
	defer func() { jftradeCheckTestError(t, missingResp.Body.Close()) }()
	status, code, _ = decodeAPIErrorEnvelope(t, missingResp)
	if status != http.StatusNotFound || code != "ADK_WORKFLOW_GET_FAILED" {
		t.Fatalf("missing workflow get = %d/%s, want not found workflow get failure", status, code)
	}

	invalidRunResp, err := jftradeTestHTTPPost(t, srv.URL+"/api/v1/adk/workflows/missing-workflow/run", "application/json", strings.NewReader(`{`))
	if err != nil {
		t.Fatalf("POST invalid workflow run: %v", err)
	}
	defer func() { jftradeCheckTestError(t, invalidRunResp.Body.Close()) }()
	status, code, message = decodeAPIErrorEnvelope(t, invalidRunResp)
	if status != http.StatusBadRequest || code != "BAD_REQUEST" || message != "invalid workflow inputs" {
		t.Fatalf("invalid workflow run = %d/%s/%q", status, code, message)
	}

	invalidTriggerResp, err := jftradeTestHTTPPost(t, srv.URL+"/api/v1/adk/workflows/missing-workflow/triggers", "application/json", strings.NewReader(`{`))
	if err != nil {
		t.Fatalf("POST invalid workflow trigger: %v", err)
	}
	defer func() { jftradeCheckTestError(t, invalidTriggerResp.Body.Close()) }()
	status, code, message = decodeAPIErrorEnvelope(t, invalidTriggerResp)
	if status != http.StatusBadRequest || code != "BAD_REQUEST" || message != "invalid workflow trigger payload" {
		t.Fatalf("invalid workflow trigger = %d/%s/%q", status, code, message)
	}
}
