package adk

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	adksession "google.golang.org/adk/v2/session"
)

type deleteErrorSessionService struct {
	adksession.Service
	err error
}

func (service deleteErrorSessionService) Delete(context.Context, *adksession.DeleteRequest) error {
	return service.err
}

func TestRunnerAdditionalBoundaryCoverageBranches(t *testing.T) {
	ctx := t.Context()

	t.Run("CompactSessionContext surfaces active-run lookup resolve-agent and prepareAgent errors", func(t *testing.T) {
		activeRuntime := newTestRuntime(t)
		activeAgent := mustSaveAgent(t, activeRuntime, AgentWriteRequest{
			ID: "compact-active-lookup-agent", Name: "Compact Active Lookup", ProviderID: testProviderID, Status: AgentStatusEnabled,
		})
		activeSession := mustCreateSession(t, activeRuntime, activeAgent.ID, "compact active lookup")
		if _, err := activeRuntime.Store().db.ExecContext(ctx, `DROP TABLE `+tableRuns); err != nil {
			t.Fatalf("drop runs table(active lookup): %v", err)
		}
		if _, err := activeRuntime.CompactSessionContext(ctx, activeSession.ID, "normal", "manual", "lookup error"); err == nil || !strings.Contains(err.Error(), tableRuns) {
			t.Fatalf("CompactSessionContext active lookup err = %v, want %s failure", err, tableRuns)
		}

		resolveRuntime := newTestRuntime(t)
		resolveAgent, err := resolveRuntime.Store().SaveAgent(ctx, AgentWriteRequest{
			ID: "compact-resolve-agent", Name: "Compact Resolve Agent", ProviderID: "", Status: AgentStatusEnabled,
		})
		if err != nil {
			t.Fatalf("SaveAgent compact resolve: %v", err)
		}
		resolveSession := mustCreateSession(t, resolveRuntime, resolveAgent.ID, "compact resolve")
		overrideProvider := "missing-provider"
		if _, err := resolveRuntime.Store().SaveSessionComposerState(ctx, resolveSession.ID, SessionComposerStatePatch{
			ProviderIDOverride: &overrideProvider,
		}); err != nil {
			t.Fatalf("SaveSessionComposerState override: %v", err)
		}
		if _, err := resolveRuntime.CompactSessionContext(ctx, resolveSession.ID, "normal", "manual", "resolve error"); err == nil || !strings.Contains(err.Error(), "provider") {
			t.Fatalf("CompactSessionContext resolve err = %v, want provider error", err)
		}

		skillRuntime := newTestRuntime(t)
		skillAgent := mustSaveAgent(t, skillRuntime, AgentWriteRequest{
			ID: "compact-skill-agent", Name: "Compact Skill Agent", ProviderID: testProviderID, Status: AgentStatusEnabled,
			Skills: []string{"missing-skill"},
		})
		skillSession := mustCreateSession(t, skillRuntime, skillAgent.ID, "compact skill")
		if _, err := skillRuntime.CompactSessionContext(ctx, skillSession.ID, "normal", "manual", "prepare agent error"); err == nil || !strings.Contains(err.Error(), "skill not found") {
			t.Fatalf("CompactSessionContext prepareAgent err = %v, want skill not found", err)
		}
	})

	t.Run("TestProvider surfaces API key and capability update errors", func(t *testing.T) {
		keyRuntime := newTestRuntime(t)
		keyProvider := mustSaveProvider(t, keyRuntime, ProviderWriteRequest{
			ID: "test-provider-bad-secrets", DisplayName: "Bad Secrets", BaseURL: "http://127.0.0.1:1/v1", Model: "model", APIKey: "sk-test", Enabled: true,
		})
		if err := os.WriteFile(keyRuntime.Store().secrets.path, []byte("{"), 0o600); err != nil {
			t.Fatalf("write bad secrets file: %v", err)
		}
		if _, err := keyRuntime.TestProvider(ctx, keyProvider.ID); err == nil {
			t.Fatal("TestProvider accepted malformed secrets file")
		}
	})

	t.Run("TestProvider capability update and runtime delete session surface storage/session-service errors", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() { _ = r.Body.Close() }()
			var req openAIChatRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
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

		updateRuntime := newTestRuntime(t)
		updateProvider := mustSaveProvider(t, updateRuntime, ProviderWriteRequest{
			ID: "test-provider-capability-update", DisplayName: "Capability Update", BaseURL: server.URL, Model: "model", APIKey: "sk-test", Enabled: true,
		})
		installFailTrigger(t, updateRuntime, "fail_provider_capability_update", tableProviders, "UPDATE", "provider capability update failed")
		if _, err := updateRuntime.TestProvider(ctx, updateProvider.ID); err == nil || !strings.Contains(err.Error(), "provider capability update failed") {
			t.Fatalf("TestProvider update err = %v, want capability update failure", err)
		}

		deleteLookupRuntime := newTestRuntime(t)
		if _, err := deleteLookupRuntime.Store().db.ExecContext(ctx, `DROP TABLE `+tableSessions); err != nil {
			t.Fatalf("drop sessions table(delete lookup): %v", err)
		}
		if err := deleteLookupRuntime.DeleteSession(ctx, "session"); err == nil || !strings.Contains(err.Error(), tableSessions) {
			t.Fatalf("DeleteSession lookup err = %v, want %s failure", err, tableSessions)
		}

		deleteRuntime := newTestRuntime(t)
		deleteAgent := mustSaveAgent(t, deleteRuntime, AgentWriteRequest{
			ID: "delete-error-agent", Name: "Delete Error Agent", ProviderID: testProviderID, Status: AgentStatusEnabled,
		})
		deleteSession := mustCreateSession(t, deleteRuntime, deleteAgent.ID, "delete session service error")
		deleteRuntime.sessionService = deleteErrorSessionService{Service: deleteRuntime.sessionService, err: errors.New("remote delete failed")}
		if err := deleteRuntime.DeleteSession(ctx, deleteSession.ID); err == nil || !strings.Contains(err.Error(), "remote delete failed") {
			t.Fatalf("DeleteSession remote err = %v, want remote delete failed", err)
		}

		notFoundRuntime := newTestRuntime(t)
		notFoundAgent := mustSaveAgent(t, notFoundRuntime, AgentWriteRequest{
			ID: "delete-not-found-agent", Name: "Delete Not Found Agent", ProviderID: testProviderID, Status: AgentStatusEnabled,
		})
		notFoundSession := mustCreateSession(t, notFoundRuntime, notFoundAgent.ID, "delete session service not found")
		notFoundRuntime.sessionService = deleteErrorSessionService{Service: notFoundRuntime.sessionService, err: errors.New("remote session not found")}
		if err := notFoundRuntime.DeleteSession(ctx, notFoundSession.ID); err != nil {
			t.Fatalf("DeleteSession remote not found err = %v, want nil", err)
		}
		if _, ok, err := notFoundRuntime.Store().Session(ctx, notFoundSession.ID); err != nil || ok {
			t.Fatalf("session after DeleteSession not found = ok:%v err:%v, want deleted", ok, err)
		}
	})

	t.Run("resolveAgentDefinition and prepareAgent surface default and skill lookup failures", func(t *testing.T) {
		defaultLookupRuntime := newTestRuntime(t)
		if _, err := defaultLookupRuntime.Store().db.ExecContext(ctx, `DROP TABLE `+tableAgents); err != nil {
			t.Fatalf("drop agents table(default lookup): %v", err)
		}
		if _, err := defaultLookupRuntime.resolveAgentDefinition(ctx, ""); err == nil || !strings.Contains(err.Error(), tableAgents) {
			t.Fatalf("resolveAgentDefinition default lookup err = %v, want %s failure", err, tableAgents)
		}

		errorRuntime := newTestRuntime(t)
		writeSkillDocument(t, errorRuntime.Store().SkillsPath(), "carrier", "---\nname: malformed-skill\n---\nCarrier.")
		if err := os.MkdirAll(errorRuntime.Store().SkillsPath()+"/malformed-skill/SKILL.md", 0o755); err != nil {
			t.Fatalf("MkdirAll malformed-skill/SKILL.md: %v", err)
		}
		if _, err := errorRuntime.prepareAgent(ctx, Agent{ID: "skill-lookup-error", Skills: []string{"malformed-skill"}}); err == nil {
			t.Fatal("prepareAgent accepted malformed skill install path")
		}
	})
}
