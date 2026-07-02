package adk

import (
	"errors"
	"os"
	"testing"
)

func TestStoreListProvidersRepairsPersistedDefaultSelection(t *testing.T) {
	ctx := t.Context()
	store := newBusinessStore(t)

	alpha, err := store.SaveProvider(ctx, ProviderWriteRequest{
		ID: "alpha", DisplayName: "Alpha", BaseURL: "https://alpha.example/v1", Model: "model-a", APIKey: "sk-alpha", Enabled: true,
	})
	if err != nil {
		t.Fatalf("SaveProvider alpha: %v", err)
	}
	beta, err := store.SaveProvider(ctx, ProviderWriteRequest{
		ID: "beta", DisplayName: "Beta", BaseURL: "https://beta.example/v1", Model: "model-b", APIKey: "sk-beta", Enabled: true,
	})
	if err != nil {
		t.Fatalf("SaveProvider beta: %v", err)
	}
	gamma, err := store.SaveProvider(ctx, ProviderWriteRequest{
		ID: "gamma", DisplayName: "Gamma", BaseURL: "https://gamma.example/v1", Model: "model-c", APIKey: "sk-gamma", Enabled: true,
	})
	if err != nil {
		t.Fatalf("SaveProvider gamma: %v", err)
	}

	if _, err := store.db.ExecContext(ctx, `UPDATE `+tableProviders+` SET payload_json = json_set(payload_json, '$.default', json('true'))`); err != nil {
		t.Fatalf("force duplicate provider defaults: %v", err)
	}

	providers, err := store.ListProviders(ctx)
	if err != nil {
		t.Fatalf("ListProviders: %v", err)
	}
	if len(providers) != 3 {
		t.Fatalf("providers = %#v", providers)
	}
	if providers[0].ID != alpha.ID || !providers[0].Default {
		t.Fatalf("first provider after repair = %#v, want alpha default", providers[0])
	}
	for _, provider := range providers {
		if provider.ID != alpha.ID && provider.Default {
			t.Fatalf("duplicate default survived repair: %#v", providers)
		}
		if !provider.HasAPIKey {
			t.Fatalf("provider lost secret marker after repair: %#v", provider)
		}
	}

	for _, provider := range []Provider{beta, gamma} {
		persisted, ok, err := store.Provider(ctx, provider.ID)
		if err != nil || !ok {
			t.Fatalf("Provider(%s) ok=%v err=%v", provider.ID, ok, err)
		}
		if persisted.Default {
			t.Fatalf("Provider(%s) persisted default=true after repair", provider.ID)
		}
	}
}

func TestStoreDefaultAgentSkipsDisabledPrimaryAndRestoresTemplate(t *testing.T) {
	ctx := t.Context()
	store := newBusinessStore(t)

	if _, err := store.db.ExecContext(ctx, `UPDATE `+tableAgents+` SET payload_json = json_set(payload_json, '$.status', ?), updated_at = ? WHERE id = ?`, AgentStatusDisabled, nowString(), DefaultBuiltinAgentID); err != nil {
		t.Fatalf("disable primary builtin agent: %v", err)
	}
	agents, err := store.ListAgents(ctx)
	if err != nil {
		t.Fatalf("ListAgents after disabling primary: %v", err)
	}
	var firstEnabled Agent
	for _, agent := range agents {
		if agent.Status == AgentStatusEnabled {
			firstEnabled = agent
			break
		}
	}
	if firstEnabled.ID == "" || firstEnabled.ID == DefaultBuiltinAgentID {
		t.Fatalf("test setup did not leave a non-primary enabled fallback: %#v", agents)
	}

	fallback, err := store.DefaultAgent(ctx)
	if err != nil {
		t.Fatalf("DefaultAgent fallback: %v", err)
	}
	if fallback.ID != firstEnabled.ID {
		t.Fatalf("DefaultAgent fallback = %#v, want first enabled %#v", fallback, firstEnabled)
	}

	if _, err := store.db.ExecContext(ctx, `UPDATE `+tableAgents+` SET payload_json = json_set(payload_json, '$.status', ?), updated_at = ?`, AgentStatusDisabled, nowString()); err != nil {
		t.Fatalf("disable all agents: %v", err)
	}
	restored, err := store.DefaultAgent(ctx)
	if err != nil {
		t.Fatalf("DefaultAgent restore: %v", err)
	}
	if restored.ID != DefaultBuiltinAgentID || restored.Status != AgentStatusEnabled || !restored.Builtin {
		t.Fatalf("restored default agent = %#v", restored)
	}
}

func TestStoreListAuditEventsReturnsCorruptPayloadError(t *testing.T) {
	ctx := t.Context()
	store := newBusinessStore(t)

	if err := store.AddAuditEvent(ctx, AuditEvent{ID: "audit-valid", Kind: "agent.saved", SubjectID: "agent", Detail: "saved", CreatedAt: "2026-01-02T03:04:05Z"}); err != nil {
		t.Fatalf("AddAuditEvent valid: %v", err)
	}
	if _, err := store.db.ExecContext(ctx, `INSERT INTO `+tableAudit+` (id, kind, subject_id, payload_json, created_at) VALUES (?, ?, ?, ?, ?)`, "audit-corrupt", "agent.saved", "agent", "{not-json", "2026-01-02T03:04:06Z"); err != nil {
		t.Fatalf("insert corrupt audit payload: %v", err)
	}

	if _, err := store.ListAuditEvents(ctx); err == nil {
		t.Fatal("ListAuditEvents err=nil, want corrupt JSON error")
	}
}

func TestStorePersistenceMethodsSurfaceClosedDatabaseErrors(t *testing.T) {
	ctx := t.Context()
	store := newBusinessStore(t)
	if err := store.Close(); err != nil {
		t.Fatalf("Close store: %v", err)
	}

	stringPtr := func(value string) *string { return &value }
	cases := []struct {
		name string
		run  func() error
	}{
		{"list providers", func() error { _, err := store.ListProviders(ctx); return err }},
		{"save provider", func() error {
			_, err := store.SaveProvider(ctx, ProviderWriteRequest{ID: "closed-provider", BaseURL: "https://example.test/v1", Model: "model"})
			return err
		}},
		{"update provider capabilities", func() error {
			_, err := store.UpdateProviderCapabilities(ctx, "closed-provider", map[string]bool{"streaming": true})
			return err
		}},
		{"get provider", func() error { _, _, err := store.Provider(ctx, "closed-provider"); return err }},
		{"default provider", func() error { _, _, err := store.DefaultProvider(ctx); return err }},
		{"set default provider", func() error { _, err := store.SetDefaultProvider(ctx, "closed-provider"); return err }},
		{"delete provider", func() error { return store.DeleteProvider(ctx, "closed-provider") }},
		{"list agents", func() error { _, err := store.ListAgents(ctx); return err }},
		{"list all agents", func() error { _, err := store.ListAllAgents(ctx); return err }},
		{"save agent", func() error {
			_, err := store.SaveAgent(ctx, AgentWriteRequest{ID: "closed-agent", Name: "Closed Agent", Status: AgentStatusEnabled})
			return err
		}},
		{"ensure agent", func() error {
			_, err := store.EnsureAgent(ctx, AgentWriteRequest{ID: "closed-agent", Name: "Closed Agent", Status: AgentStatusEnabled})
			return err
		}},
		{"get agent", func() error { _, _, err := store.Agent(ctx, "closed-agent"); return err }},
		{"default agent", func() error { _, err := store.DefaultAgent(ctx); return err }},
		{"delete agent", func() error { return store.DeleteAgent(ctx, "closed-agent") }},
		{"create session", func() error { _, err := store.CreateSession(ctx, "closed-agent", "Closed Session"); return err }},
		{"rename session", func() error { _, err := store.RenameSession(ctx, "closed-session", "Renamed"); return err }},
		{"get session", func() error { _, _, err := store.Session(ctx, "closed-session"); return err }},
		{"list sessions", func() error { _, err := store.ListSessions(ctx); return err }},
		{"list sessions page", func() error { _, _, err := store.ListSessionsPage(ctx, "", "", 20, 0); return err }},
		{"delete session", func() error { return store.DeleteSession(ctx, "closed-session") }},
		{"save run", func() error {
			return store.SaveRun(ctx, Run{ID: "closed-run", SessionID: "closed-session", AgentID: "closed-agent", Status: RunStatusRunning})
		}},
		{"save run and deny approvals", func() error {
			return store.SaveRunAndDenyPendingApprovals(ctx, Run{ID: "closed-run", SessionID: "closed-session", AgentID: "closed-agent", Status: RunStatusFailed})
		}},
		{"get run", func() error { _, _, err := store.Run(ctx, "closed-run"); return err }},
		{"list runs", func() error { _, err := store.ListRuns(ctx); return err }},
		{"list runs page", func() error { _, _, err := store.ListRunsPage(ctx, "", "", "", 20, 0); return err }},
		{"save approval", func() error {
			return store.SaveApproval(ctx, Approval{ID: "closed-approval", RunID: "closed-run", AgentID: "closed-agent", Status: ApprovalStatusPending})
		}},
		{"save approval if confirmation absent", func() error {
			_, _, err := store.SaveApprovalIfConfirmationAbsent(ctx, Approval{ID: "closed-approval", RunID: "closed-run", AgentID: "closed-agent", Status: ApprovalStatusPending, ConfirmationCallID: "call-closed"})
			return err
		}},
		{"approval by confirmation", func() error { _, _, err := store.ApprovalByConfirmationCallID(ctx, "call-closed"); return err }},
		{"resolve pending approval", func() error {
			_, _, err := store.ResolvePendingApproval(ctx, "closed-approval", ApprovalStatusApproved)
			return err
		}},
		{"get approval", func() error { _, _, err := store.Approval(ctx, "closed-approval"); return err }},
		{"list approvals", func() error { _, err := store.ListApprovals(ctx); return err }},
		{"list approvals page", func() error { _, _, err := store.ListApprovalsPage(ctx, "", "", 20, 0); return err }},
		{"list skills", func() error { _, err := store.ListSkills(ctx); return err }},
		{"save skill", func() error {
			_, err := store.SaveSkill(ctx, Skill{ID: "closed-skill", DisplayName: "Closed Skill"})
			return err
		}},
		{"get skill", func() error { _, _, err := store.Skill(ctx, "closed-skill"); return err }},
		{"delete skill", func() error { return store.DeleteSkill(ctx, "closed-skill") }},
		{"add audit", func() error {
			return store.AddAuditEvent(ctx, AuditEvent{Kind: "closed", SubjectID: "store", Detail: "closed"})
		}},
		{"list audit", func() error { _, err := store.ListAuditEvents(ctx); return err }},
		{"save optimization task", func() error {
			_, err := store.SaveOptimizationTask(ctx, OptimizationTask{ID: "closed-optimization", Status: "queued", Objective: "closed"})
			return err
		}},
		{"get optimization task", func() error { _, _, err := store.OptimizationTask(ctx, "closed-optimization"); return err }},
		{"list optimization tasks", func() error { _, err := store.ListOptimizationTasks(ctx); return err }},
		{"save task", func() error {
			_, err := store.SaveTask(ctx, TaskWriteRequest{ID: "closed-task", Title: "Closed Task"})
			return err
		}},
		{"update task", func() error {
			_, err := store.UpdateTask(ctx, "closed-task", TaskPatchRequest{Title: stringPtr("Updated")})
			return err
		}},
		{"get task", func() error { _, _, err := store.Task(ctx, "closed-task"); return err }},
		{"list tasks page", func() error { _, _, err := store.ListTasksPage(ctx, "", "", "", 20, 0); return err }},
		{"delete task", func() error { return store.DeleteTask(ctx, "closed-task") }},
		{"save memory", func() error {
			_, err := store.SaveMemory(ctx, MemoryWriteRequest{Key: "closed", Value: "value", Scope: "workspace"})
			return err
		}},
		{"get memory", func() error { _, _, err := store.Memory(ctx, "closed-memory"); return err }},
		{"list memory", func() error { _, err := store.ListMemory(ctx, "closed-agent"); return err }},
		{"list memory filtered", func() error { _, err := store.ListMemoryFiltered(ctx, "workspace", "", "closed"); return err }},
		{"delete memory", func() error { return store.DeleteMemory(ctx, "closed-memory") }},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.run(); err == nil || errors.Is(err, os.ErrNotExist) {
				t.Fatalf("%s err = %v, want closed database error", tc.name, err)
			}
		})
	}
}
