package servercore

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	jfadk "github.com/jftrade/jftrade-main/pkg/adk"
)

func TestADKSessionsCRUDAndFilteringRoutes(t *testing.T) {
	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	server := newTestServer(t, store)
	if _, err := server.store.SaveADKSettings(ADKRuntimeSettings{RunTimeoutMs: 720_000, StreamIdleTimeoutMs: 420_000}); err != nil {
		t.Fatalf("saveADKSettings: %v", err)
	}
	srv := httptest.NewServer(server)
	t.Cleanup(srv.Close)

	agent, err := server.adkRuntime.Store().SaveAgent(t.Context(), jfadk.AgentWriteRequest{
		ID:             "session-agent",
		Name:           "Session Agent",
		ProviderID:     testADKProviderID,
		Tools:          []string{"strategy.save_draft"},
		PermissionMode: jfadk.PermissionModeApproval,
		Status:         jfadk.AgentStatusEnabled,
	})
	if err != nil {
		t.Fatalf("SaveAgent: %v", err)
	}

	createBody := []byte(`{"agentId":"` + agent.ID + `","title":"组合诊断会话"}`)
	createResp, err := jftradeTestHTTPPost(t, srv.URL+"/api/v1/adk/sessions", "application/json", bytes.NewReader(createBody))
	if err != nil {
		t.Fatalf("POST sessions: %v", err)
	}
	defer func() { jftradeCheckTestError(t, createResp.Body.Close()) }()
	var createEnvelope struct {
		OK   bool          `json:"ok"`
		Data jfadk.Session `json:"data"`
	}
	if err := json.NewDecoder(createResp.Body).Decode(&createEnvelope); err != nil {
		t.Fatalf("decode create session: %v", err)
	}
	if !createEnvelope.OK || createEnvelope.Data.AgentID != agent.ID {
		t.Fatalf("create session envelope = %+v", createEnvelope)
	}

	composerReq, err := http.NewRequestWithContext(t.Context(),
		http.MethodPatch,
		srv.URL+"/api/v1/adk/sessions/"+createEnvelope.Data.ID+"/composer-state",
		bytes.NewReader([]byte(`{"chatDraft":"未发送草稿","workModeOverride":"loop","permissionModeOverride":"less_approval","goalObjectiveDraft":"目标草稿","goalObjectiveTouched":true}`)),
	)
	if err != nil {
		t.Fatalf("NewRequest composer state: %v", err)
	}
	composerReq.Header.Set("Content-Type", "application/json")
	composerResp, err := http.DefaultClient.Do(composerReq)
	if err != nil {
		t.Fatalf("PATCH composer state: %v", err)
	}
	defer func() { jftradeCheckTestError(t, composerResp.Body.Close()) }()
	var composerEnvelope struct {
		OK   bool                       `json:"ok"`
		Data jfadk.SessionComposerState `json:"data"`
	}
	if err := json.NewDecoder(composerResp.Body).Decode(&composerEnvelope); err != nil {
		t.Fatalf("decode composer state: %v", err)
	}
	if !composerEnvelope.OK || composerEnvelope.Data.ChatDraft != "未发送草稿" || composerEnvelope.Data.WorkModeOverride != jfadk.WorkModeLoop || composerEnvelope.Data.PermissionModeOverride != jfadk.PermissionModeLessApproval {
		t.Fatalf("composer state envelope = %+v", composerEnvelope)
	}

	invalidComposerReq, err := http.NewRequestWithContext(t.Context(),
		http.MethodPatch,
		srv.URL+"/api/v1/adk/sessions/"+createEnvelope.Data.ID+"/composer-state",
		bytes.NewReader([]byte(`{"workModeOverride":"sequential"}`)),
	)
	if err != nil {
		t.Fatalf("NewRequest invalid composer state: %v", err)
	}
	invalidComposerReq.Header.Set("Content-Type", "application/json")
	invalidComposerResp, err := http.DefaultClient.Do(invalidComposerReq)
	if err != nil {
		t.Fatalf("PATCH invalid composer state: %v", err)
	}
	defer func() { jftradeCheckTestError(t, invalidComposerResp.Body.Close()) }()
	if invalidComposerResp.StatusCode != http.StatusBadRequest {
		t.Fatalf("invalid composer status = %d, want 400", invalidComposerResp.StatusCode)
	}

	invalidPermissionReq, err := http.NewRequestWithContext(t.Context(),
		http.MethodPatch,
		srv.URL+"/api/v1/adk/sessions/"+createEnvelope.Data.ID+"/composer-state",
		bytes.NewReader([]byte(`{"permissionModeOverride":"root"}`)),
	)
	if err != nil {
		t.Fatalf("NewRequest invalid permission state: %v", err)
	}
	invalidPermissionReq.Header.Set("Content-Type", "application/json")
	invalidPermissionResp, err := http.DefaultClient.Do(invalidPermissionReq)
	if err != nil {
		t.Fatalf("PATCH invalid permission state: %v", err)
	}
	defer func() { jftradeCheckTestError(t, invalidPermissionResp.Body.Close()) }()
	if invalidPermissionResp.StatusCode != http.StatusBadRequest {
		t.Fatalf("invalid permission status = %d, want 400", invalidPermissionResp.StatusCode)
	}

	chatResp, err := jftradeTestHTTPPost(t, srv.URL+"/api/v1/adk/chat", "application/json", bytes.NewReader([]byte(`{"agentId":"`+agent.ID+`","sessionId":"`+createEnvelope.Data.ID+`","message":"@strategy.save_draft 保存会话草稿"}`)))
	if err != nil {
		t.Fatalf("POST session chat: %v", err)
	}
	defer func() { jftradeCheckTestError(t, chatResp.Body.Close()) }()
	if chatResp.StatusCode != http.StatusOK {
		t.Fatalf("POST session chat status = %d", chatResp.StatusCode)
	}

	listResp, err := jftradeTestHTTPGet(t, srv.URL+"/api/v1/adk/sessions?agentId="+agent.ID+"&query=组合&limit=5")
	if err != nil {
		t.Fatalf("GET sessions: %v", err)
	}
	defer func() { jftradeCheckTestError(t, listResp.Body.Close()) }()
	var listEnvelope struct {
		OK   bool `json:"ok"`
		Data struct {
			Sessions []jfadk.Session `json:"sessions"`
			Page     struct {
				Total    int  `json:"total"`
				Returned int  `json:"returned"`
				HasMore  bool `json:"hasMore"`
			} `json:"page"`
		} `json:"data"`
	}
	if err := json.NewDecoder(listResp.Body).Decode(&listEnvelope); err != nil {
		t.Fatalf("decode sessions: %v", err)
	}
	if !listEnvelope.OK || listEnvelope.Data.Page.Total != 1 || len(listEnvelope.Data.Sessions) != 1 || listEnvelope.Data.Page.HasMore {
		t.Fatalf("list sessions envelope = %+v", listEnvelope)
	}
	listPayload, err := json.Marshal(listEnvelope.Data.Sessions)
	if err != nil {
		t.Fatalf("marshal list sessions: %v", err)
	}
	if bytes.Contains(listPayload, []byte("未发送草稿")) {
		t.Fatalf("session list leaked composer draft: %s", string(listPayload))
	}

	getResp, err := jftradeTestHTTPGet(t, srv.URL+"/api/v1/adk/sessions/"+createEnvelope.Data.ID)
	if err != nil {
		t.Fatalf("GET session detail: %v", err)
	}
	defer func() { jftradeCheckTestError(t, getResp.Body.Close()) }()
	var getEnvelope struct {
		OK   bool `json:"ok"`
		Data struct {
			Session       jfadk.Session              `json:"session"`
			Timeline      []jfadk.TimelineEntry      `json:"timeline"`
			ComposerState jfadk.SessionComposerState `json:"composerState"`
		} `json:"data"`
	}
	if err := json.NewDecoder(getResp.Body).Decode(&getEnvelope); err != nil {
		t.Fatalf("decode session detail: %v", err)
	}
	if !getEnvelope.OK || getEnvelope.Data.Session.ID != createEnvelope.Data.ID || len(getEnvelope.Data.Timeline) == 0 {
		t.Fatalf("session detail envelope = %+v", getEnvelope)
	}
	if getEnvelope.Data.ComposerState.ChatDraft != "未发送草稿" || getEnvelope.Data.ComposerState.GoalObjectiveDraft != "目标草稿" {
		t.Fatalf("session detail composer state = %+v", getEnvelope.Data.ComposerState)
	}

	renameReq, err := http.NewRequestWithContext(t.Context(), http.MethodPut, srv.URL+"/api/v1/adk/sessions/"+createEnvelope.Data.ID, bytes.NewReader([]byte(`{"title":"重命名会话"}`)))
	if err != nil {
		t.Fatalf("NewRequest rename: %v", err)
	}
	renameReq.Header.Set("Content-Type", "application/json")
	renameResp, err := http.DefaultClient.Do(renameReq)
	if err != nil {
		t.Fatalf("PUT session rename: %v", err)
	}
	defer func() { jftradeCheckTestError(t, renameResp.Body.Close()) }()
	var renameEnvelope struct {
		OK   bool          `json:"ok"`
		Data jfadk.Session `json:"data"`
	}
	if err := json.NewDecoder(renameResp.Body).Decode(&renameEnvelope); err != nil {
		t.Fatalf("decode rename session: %v", err)
	}
	if !renameEnvelope.OK || renameEnvelope.Data.Title != "重命名会话" {
		t.Fatalf("rename session envelope = %+v", renameEnvelope)
	}

	deleteReq, err := http.NewRequestWithContext(t.Context(), http.MethodDelete, srv.URL+"/api/v1/adk/sessions/"+createEnvelope.Data.ID, nil)
	if err != nil {
		t.Fatalf("NewRequest delete: %v", err)
	}
	deleteResp, err := http.DefaultClient.Do(deleteReq)
	if err != nil {
		t.Fatalf("DELETE session: %v", err)
	}
	defer func() { jftradeCheckTestError(t, deleteResp.Body.Close()) }()
	if deleteResp.StatusCode != http.StatusOK {
		t.Fatalf("delete session status = %d", deleteResp.StatusCode)
	}

	missingResp, err := jftradeTestHTTPGet(t, srv.URL+"/api/v1/adk/sessions/"+createEnvelope.Data.ID)
	if err != nil {
		t.Fatalf("GET deleted session: %v", err)
	}
	defer func() { jftradeCheckTestError(t, missingResp.Body.Close()) }()
	if missingResp.StatusCode != http.StatusNotFound {
		t.Fatalf("deleted session status = %d, want 404", missingResp.StatusCode)
	}
}
