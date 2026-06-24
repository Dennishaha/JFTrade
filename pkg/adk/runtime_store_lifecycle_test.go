package adk

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	adksession "google.golang.org/adk/session"
)

func sameStringSet(left []string, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	seen := make(map[string]int, len(left))
	for _, item := range left {
		seen[item]++
	}
	for _, item := range right {
		seen[item]--
		if seen[item] < 0 {
			return false
		}
	}
	return true
}

func TestStoreDefaultAgentEnsureAgentAndSessionOrdering(t *testing.T) {
	ctx := context.Background()
	runtime := newTestRuntime(t)

	defaultAgent, err := runtime.Store().DefaultAgent(ctx)
	if err != nil {
		t.Fatalf("DefaultAgent: %v", err)
	}
	if defaultAgent.ID == "" || defaultAgent.Status != AgentStatusEnabled || defaultAgent.PermissionMode != PermissionModeApproval {
		t.Fatalf("default agent = %+v, want enabled approval-mode built-in agent", defaultAgent)
	}
	if defaultAgent.ID != DefaultBuiltinAgentID || defaultAgent.Name != "默认助手" || !defaultAgent.Builtin || len(defaultAgent.Tools) != 0 {
		t.Fatalf("default agent = %+v, want primary builtin with all tools marker", defaultAgent)
	}
	if !sameStringSet(defaultAgent.Skills, BuiltinSkillIDs()) {
		t.Fatalf("default agent skills = %+v, want all builtin skills %+v", defaultAgent.Skills, BuiltinSkillIDs())
	}

	ensured, err := runtime.Store().EnsureAgent(ctx, AgentWriteRequest{
		Name:   " Momentum Scout ",
		Status: AgentStatusEnabled,
	})
	if err != nil {
		t.Fatalf("EnsureAgent create: %v", err)
	}
	if ensured.ID != "momentum-scout" {
		t.Fatalf("ensured agent id = %q, want momentum-scout", ensured.ID)
	}
	again, err := runtime.Store().EnsureAgent(ctx, AgentWriteRequest{
		Name: "Momentum Scout",
	})
	if err != nil {
		t.Fatalf("EnsureAgent lookup: %v", err)
	}
	if again.ID != ensured.ID || again.CreatedAt != ensured.CreatedAt {
		t.Fatalf("EnsureAgent idempotent result = %+v, want existing %+v", again, ensured)
	}
	agents, err := runtime.Store().ListAgents(ctx)
	if err != nil {
		t.Fatalf("ListAgents: %v", err)
	}
	if len(agents) == 0 || agents[0].ID != DefaultBuiltinAgentID {
		t.Fatalf("agent order = %+v, want primary default first", agents)
	}

	firstSession, err := runtime.Store().CreateSession(ctx, ensured.ID, "first")
	if err != nil {
		t.Fatalf("CreateSession first: %v", err)
	}
	time.Sleep(10 * time.Millisecond)
	secondSession, err := runtime.Store().CreateSession(ctx, ensured.ID, "second")
	if err != nil {
		t.Fatalf("CreateSession second: %v", err)
	}
	longTitle := strings.Repeat("名", 81)
	renamed, err := runtime.Store().RenameSession(ctx, firstSession.ID, longTitle)
	if err != nil {
		t.Fatalf("RenameSession: %v", err)
	}
	if got := len([]rune(renamed.Title)); got != 80 {
		t.Fatalf("renamed title rune len = %d, want 80", got)
	}
	if _, err := runtime.Store().RenameSession(ctx, firstSession.ID, "   "); err == nil || !strings.Contains(err.Error(), "title is required") {
		t.Fatalf("RenameSession empty err = %v, want title required", err)
	}
	if _, err := runtime.Store().RenameSession(ctx, "session-missing", "missing"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("RenameSession missing err = %v, want os.ErrNotExist", err)
	}

	sessions, err := runtime.Store().ListSessions(ctx)
	if err != nil {
		t.Fatalf("ListSessions: %v", err)
	}
	if len(sessions) < 2 {
		t.Fatalf("sessions len = %d, want at least 2", len(sessions))
	}
	if sessions[0].ID != renamed.ID || sessions[1].ID != secondSession.ID {
		t.Fatalf("session order = [%s %s], want [%s %s]", sessions[0].ID, sessions[1].ID, renamed.ID, secondSession.ID)
	}
}

func TestStoreBuiltinAgentsAreProtected(t *testing.T) {
	ctx := context.Background()
	runtime := newTestRuntime(t)

	defaultAgent, err := runtime.Store().DefaultAgent(ctx)
	if err != nil {
		t.Fatalf("DefaultAgent: %v", err)
	}
	if err := runtime.Store().DeleteAgent(ctx, defaultAgent.ID); !errors.Is(err, ErrBuiltinAgentProtected) {
		t.Fatalf("DeleteAgent default err = %v, want ErrBuiltinAgentProtected", err)
	}
	if _, err := runtime.Store().SaveAgent(ctx, AgentWriteRequest{
		ID: defaultAgent.ID, Name: defaultAgent.Name, Status: AgentStatusDisabled,
	}); !errors.Is(err, ErrBuiltinAgentProtected) {
		t.Fatalf("disable primary default err = %v, want ErrBuiltinAgentProtected", err)
	}

	secondary, err := runtime.Store().SaveAgent(ctx, AgentWriteRequest{
		ID: "investment-analyst", Name: "投资分析助手", Status: AgentStatusDisabled,
	})
	if err != nil {
		t.Fatalf("disable secondary builtin: %v", err)
	}
	if secondary.Status != AgentStatusDisabled || !secondary.Builtin {
		t.Fatalf("secondary builtin = %+v, want disabled builtin", secondary)
	}
	if err := runtime.Store().DeleteAgent(ctx, secondary.ID); !errors.Is(err, ErrBuiltinAgentProtected) {
		t.Fatalf("DeleteAgent secondary err = %v, want ErrBuiltinAgentProtected", err)
	}
}

func TestModelsListToolReturnsCallableModelsWithoutKeys(t *testing.T) {
	ctx := context.Background()
	runtime := newTestRuntime(t)
	mustSaveProvider(t, runtime, ProviderWriteRequest{
		ID: "disabled-provider", DisplayName: "Disabled Provider", BaseURL: "https://disabled.example/v1",
		Model: "disabled-model", APIKey: "sk-disabled", Enabled: false,
	})
	mustSaveProvider(t, runtime, ProviderWriteRequest{
		ID: "no-key-provider", DisplayName: "No Key Provider", BaseURL: "https://no-key.example/v1",
		Model: "no-key-model", Enabled: true,
	})
	capabilityProvider := mustSaveProvider(t, runtime, ProviderWriteRequest{
		ID: "capability-provider", DisplayName: "Capability Provider", BaseURL: "https://capability.example/v1",
		Model: "vision-model", APIKey: "sk-capability", Enabled: true,
	})
	if _, err := runtime.Store().UpdateProviderCapabilities(ctx, capabilityProvider.ID, map[string]bool{"vision": true, "disabled-capability": false}); err != nil {
		t.Fatalf("UpdateProviderCapabilities: %v", err)
	}

	raw, err := runtime.modelsListTool(ctx, map[string]any{"limit": 10})
	if err != nil {
		t.Fatalf("models.list: %v", err)
	}
	payload, ok := raw.(map[string]any)
	if !ok {
		t.Fatalf("models.list payload = %T, want map", raw)
	}
	models, ok := payload["models"].([]map[string]any)
	if !ok {
		t.Fatalf("models = %#v, want []map[string]any", payload["models"])
	}
	if len(models) != 2 {
		t.Fatalf("models = %#v, want two callable providers", models)
	}
	callableSeen := map[string]map[string]any{}
	for _, item := range models {
		callableSeen[fmt.Sprint(item["providerId"])] = item
	}
	if callableSeen[testProviderID]["model"] != "test-model" || callableSeen["capability-provider"]["model"] != "vision-model" {
		t.Fatalf("models = %#v, want callable test and capability providers", models)
	}
	if callableSeen[testProviderID]["hasApiKey"] != true || callableSeen[testProviderID]["callable"] != true {
		t.Fatalf("model flags = %#v, want hasApiKey/callable true", callableSeen[testProviderID])
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	if strings.Contains(strings.ToLower(string(encoded)), "sk-") {
		t.Fatalf("models.list leaked key material: %s", string(encoded))
	}
	rawAll, err := runtime.modelsListTool(ctx, map[string]any{"callableOnly": false, "limit": 10})
	if err != nil {
		t.Fatalf("models.list all: %v", err)
	}
	allPayload, ok := rawAll.(map[string]any)
	if !ok {
		t.Fatalf("models.list all payload = %T, want map", rawAll)
	}
	allModels, ok := allPayload["models"].([]map[string]any)
	if !ok {
		t.Fatalf("models all = %#v, want []map[string]any", allPayload["models"])
	}
	seen := map[string]map[string]any{}
	for _, item := range allModels {
		seen[fmt.Sprint(item["providerId"])] = item
	}
	if seen["disabled-provider"]["callable"] != false || seen["no-key-provider"]["hasApiKey"] != false || seen["no-key-provider"]["callable"] != false {
		t.Fatalf("all models = %#v, want disabled/no-key diagnostics", allModels)
	}
	rawDisabled, err := runtime.modelsListTool(ctx, map[string]any{"providerId": "disabled-provider", "callableOnly": false})
	if err != nil {
		t.Fatalf("models.list disabled provider: %v", err)
	}
	disabledPayload := rawDisabled.(map[string]any)
	disabledModels := disabledPayload["models"].([]map[string]any)
	if len(disabledModels) != 1 || disabledModels[0]["providerId"] != "disabled-provider" || disabledModels[0]["callable"] != false {
		t.Fatalf("disabled provider models = %#v, want disabled provider diagnostic", disabledModels)
	}
	rawVision, err := runtime.modelsListTool(ctx, map[string]any{"query": "vision"})
	if err != nil {
		t.Fatalf("models.list vision query: %v", err)
	}
	visionPayload := rawVision.(map[string]any)
	visionModels := visionPayload["models"].([]map[string]any)
	if len(visionModels) != 1 || visionModels[0]["providerId"] != "capability-provider" {
		t.Fatalf("vision query models = %#v, want capability-provider only", visionModels)
	}
}

func TestRuntimeSnapshotProviderProbeAndDeleteSession(t *testing.T) {
	ctx := context.Background()
	runtime := newTestRuntime(t)

	if runtime.Store() == nil {
		t.Fatal("Store() = nil, want store")
	}
	if runtime.Tools() == nil {
		t.Fatal("Tools() = nil, want tool registry")
	}
	if runtime.Skills() == nil {
		t.Fatal("Skills() = nil, want skill registry")
	}

	agent := mustSaveAgent(t, runtime, AgentWriteRequest{
		ID:          "runtime-lifecycle-agent",
		Name:        "Runtime Lifecycle Agent",
		ProviderID:  testProviderID,
		Status:      AgentStatusEnabled,
		Instruction: "Track lifecycle operations.",
	})
	runtime.RecordAudit(ctx, "runtime.checked", agent.ID, "runtime lifecycle smoke test", map[string]any{"kind": "lifecycle"})
	events := mustAuditEvents(t, runtime)
	if len(events) == 0 || events[0].Kind == "" {
		t.Fatalf("audit events = %+v, want recorded lifecycle event", events)
	}

	snapshot, err := runtime.Snapshot(ctx)
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	if len(snapshot.Providers) == 0 || len(snapshot.Agents) == 0 || len(snapshot.Tools) == 0 {
		t.Fatalf("snapshot = %+v, want providers/agents/tools populated", snapshot)
	}

	probe, err := runtime.TestProvider(ctx, testProviderID)
	if err != nil {
		t.Fatalf("TestProvider: %v", err)
	}
	if probe["ok"] != true || strings.TrimSpace(probe["reply"].(string)) == "" {
		t.Fatalf("provider probe = %#v, want ok reply payload", probe)
	}
	capabilities, ok := probe["capabilities"].(map[string]bool)
	if !ok || !capabilities["streaming"] || !capabilities["tools"] {
		t.Fatalf("provider capabilities = %#v, want streaming/tools true", probe["capabilities"])
	}
	if _, err := runtime.TestProvider(ctx, "provider-missing"); err == nil || !strings.Contains(err.Error(), "provider not found") {
		t.Fatalf("TestProvider missing err = %v, want provider not found", err)
	}

	session := mustCreateSession(t, runtime, agent.ID, "Runtime Delete Session")
	if _, err := runtime.rawSessionService.Create(ctx, &adksession.CreateRequest{
		AppName:   googleADKAppName(agent.ID),
		UserID:    googleADKUserID,
		SessionID: session.ID,
	}); err != nil {
		t.Fatalf("Create raw session: %v", err)
	}
	if err := runtime.DeleteSession(ctx, session.ID); err != nil {
		t.Fatalf("DeleteSession: %v", err)
	}
	if _, ok, err := runtime.Store().Session(ctx, session.ID); err != nil || ok {
		t.Fatalf("deleted session lookup ok=%v err=%v", ok, err)
	}
	if _, err := runtime.rawSessionService.Get(ctx, &adksession.GetRequest{
		AppName:   googleADKAppName(agent.ID),
		UserID:    googleADKUserID,
		SessionID: session.ID,
	}); err == nil {
		t.Fatal("raw session still exists after runtime.DeleteSession")
	}
	if err := runtime.DeleteSession(ctx, "session-missing"); err == nil || !strings.Contains(err.Error(), "session not found") {
		t.Fatalf("DeleteSession missing err = %v, want session not found", err)
	}
}

func TestRuntimeTestProviderMarksToolsUnsupportedWhenSelectionFails(t *testing.T) {
	ctx := context.Background()
	runtime := newTestRuntime(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() { _ = r.Body.Close() }()
		var req openAIChatRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if len(req.Tools) > 0 {
			http.Error(w, "tool calling unavailable", http.StatusBadGateway)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(openAIChatResponse{
			Choices: []struct {
				Message openAIChatMessage `json:"message"`
			}{{
				Message: openAIChatMessage{Role: "assistant", Content: "health check ok"},
			}},
		}); err != nil {
			t.Fatalf("Encode response: %v", err)
		}
	}))
	defer server.Close()

	provider := mustSaveProvider(t, runtime, ProviderWriteRequest{
		ID:          "provider-chat-only",
		DisplayName: "Chat Only Provider",
		BaseURL:     server.URL,
		Model:       "test-model",
		APIKey:      "sk-chat-only",
		Enabled:     true,
	})

	probe, err := runtime.TestProvider(ctx, provider.ID)
	if err != nil {
		t.Fatalf("TestProvider: %v", err)
	}
	capabilities, ok := probe["capabilities"].(map[string]bool)
	if !ok {
		t.Fatalf("provider capabilities type = %T, want map[string]bool", probe["capabilities"])
	}
	if !capabilities["streaming"] || capabilities["tools"] {
		t.Fatalf("provider capabilities = %#v, want streaming=true tools=false", capabilities)
	}
	stored, ok, err := runtime.Store().Provider(ctx, provider.ID)
	if err != nil || !ok {
		t.Fatalf("Provider lookup ok=%v err=%v", ok, err)
	}
	if stored.Capabilities["tools"] {
		t.Fatalf("stored capabilities = %#v, want tools=false after failed selection probe", stored.Capabilities)
	}
}

func TestRuntimeDeleteSessionIgnoresMissingRemoteSessionAndUnavailableRuntime(t *testing.T) {
	ctx := context.Background()
	runtime := newTestRuntime(t)
	agent := mustSaveAgent(t, runtime, AgentWriteRequest{
		ID:          "delete-fallback-agent",
		Name:        "Delete Fallback Agent",
		ProviderID:  testProviderID,
		Status:      AgentStatusEnabled,
		Instruction: "Clean up sessions even when remote state is already gone.",
	})
	session := mustCreateSession(t, runtime, agent.ID, "Delete Missing Remote Session")

	if err := runtime.DeleteSession(ctx, session.ID); err != nil {
		t.Fatalf("DeleteSession missing remote: %v", err)
	}
	if _, ok, err := runtime.Store().Session(ctx, session.ID); err != nil || ok {
		t.Fatalf("deleted session lookup ok=%v err=%v", ok, err)
	}

	var nilRuntime *Runtime
	if err := nilRuntime.DeleteSession(ctx, "any-session"); err == nil || !strings.Contains(err.Error(), "adk runtime is unavailable") {
		t.Fatalf("nil runtime DeleteSession err = %v, want unavailable", err)
	}
}

func TestNewRuntimeInitializesRegistriesAndClosesCleanly(t *testing.T) {
	dir := t.TempDir()
	store, err := NewStore(filepath.Join(dir, "adk.db"), filepath.Join(dir, "secrets", "adk.json"), filepath.Join(dir, "skills"))
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	sessionService := adksession.InMemoryService()

	runtime := NewRuntimeWithSessionService(store, nil, sessionService)
	if runtime == nil || runtime.Store() != store || runtime.Tools() == nil || runtime.Skills() == nil || runtime.contextManager == nil {
		t.Fatalf("runtime = %+v, want initialized runtime with registries and context manager", runtime)
	}
	if err := runtime.CloseSessionServices(); err != nil {
		t.Fatalf("CloseSessionServices: %v", err)
	}
	if err := runtime.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
}

func TestNewRuntimeDirectCtorAndNilSafeHelpers(t *testing.T) {
	dir := t.TempDir()
	store, err := NewStore(filepath.Join(dir, "adk.db"), filepath.Join(dir, "secrets", "adk.json"), filepath.Join(dir, "skills"))
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}

	runtime := NewRuntime(store, nil)
	if runtime == nil || runtime.Store() != store || runtime.Tools() == nil || runtime.Skills() == nil {
		t.Fatalf("runtime = %+v, want direct constructor to initialize registries", runtime)
	}
	if runtime.sessionService == nil || runtime.rawSessionService == nil {
		t.Fatalf("runtime session services = %#v/%#v, want initialized services", runtime.sessionService, runtime.rawSessionService)
	}

	var nilRuntime *Runtime
	if nilRuntime.Store() != nil || nilRuntime.Tools() != nil || nilRuntime.Skills() != nil {
		t.Fatalf("nil runtime accessors should all return nil")
	}
	if err := nilRuntime.CloseSessionServices(); err != nil {
		t.Fatalf("nil CloseSessionServices: %v", err)
	}
	if err := nilRuntime.Close(); err != nil {
		t.Fatalf("nil Close: %v", err)
	}
	if err := runtime.Close(); err != nil {
		t.Fatalf("Close direct runtime: %v", err)
	}
}

func TestApprovalResolutionSummaryAndUserFacingErrors(t *testing.T) {
	toolErr := "provider returned 500"
	run := Run{
		ToolCalls: []ToolCall{
			{ToolName: "strategy.save_draft", Status: "SUCCEEDED", Output: map[string]any{"saved": true, "strategyId": "draft-1"}},
			{ToolName: "strategy.save_draft", Status: "FAILED", Error: &toolErr},
			{ToolName: "backtest.result_view", Status: "SUCCEEDED", Output: "ignored"},
		},
	}
	approval := Approval{ToolName: "strategy.save_draft"}

	approved := approvalResolutionSummary(run, approval, true)
	if !strings.Contains(approved, "已批准并执行工具调用 `strategy.save_draft`。") {
		t.Fatalf("approved summary = %q, want approved heading", approved)
	}
	if !strings.Contains(approved, "执行结果：") || !strings.Contains(approved, `strategy.save_draft => {"saved":true,"strategyId":"draft-1"}`) {
		t.Fatalf("approved summary = %q, want summarized successful output", approved)
	}
	if !strings.Contains(approved, "执行失败："+toolErr) {
		t.Fatalf("approved summary = %q, want tool failure detail", approved)
	}

	denied := approvalResolutionSummary(run, approval, false)
	if !strings.Contains(denied, "已拒绝工具调用 `strategy.save_draft`。") || !strings.Contains(denied, "未执行该操作") {
		t.Fatalf("denied summary = %q, want rejection message", denied)
	}

	if got := userFacingADKError(nil); got != "" {
		t.Fatalf("userFacingADKError(nil) = %q, want empty", got)
	}
	if got := userFacingADKError(fmt.Errorf("wrote more than the declared content-length")); got != "模型服务响应异常，请检查模型服务配置或稍后重试。" {
		t.Fatalf("content-length error = %q", got)
	}
	if got := userFacingADKError(fmt.Errorf("database is locked")); got != "数据库繁忙，请稍后重试。" {
		t.Fatalf("database locked error = %q", got)
	}
	if got := userFacingADKError(fmt.Errorf("sqlite_busy")); got != "数据库繁忙，请稍后重试。" {
		t.Fatalf("sqlite busy error = %q", got)
	}
	if got := userFacingADKError(fmt.Errorf("plain failure")); got != "plain failure" {
		t.Fatalf("plain error = %q, want passthrough", got)
	}
}
