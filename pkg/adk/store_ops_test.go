package adk

import (
	"archive/zip"
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	strategypinespec "github.com/jftrade/jftrade-main/pkg/strategy/pinespec"
)

func TestStoreBuiltinSkillsSplitStrategySkillAndRemoveLegacyRecord(t *testing.T) {
	ctx := context.Background()
	runtime := newTestRuntime(t)
	store := runtime.Store()
	if _, ok, err := store.Skill(ctx, strategypinespec.LegacyBuiltinSkillName); err != nil || ok {
		t.Fatalf("legacy store skill ok=%v err=%v, want absent", ok, err)
	}
	for _, skillName := range []string{strategypinespec.ResearchBuiltinSkillName, strategypinespec.PublishBuiltinSkillName} {
		skill, ok, err := store.Skill(ctx, skillName)
		if err != nil || !ok {
			t.Fatalf("store skill %s ok=%v err=%v", skillName, ok, err)
		}
		if !skill.Builtin || !strings.EqualFold(skill.Source, "builtin") {
			t.Fatalf("store skill %s metadata = %+v, want builtin", skillName, skill)
		}
	}

	if _, err := store.SaveSkill(ctx, Skill{ID: strategypinespec.LegacyBuiltinSkillName, DisplayName: "Legacy Strategy", Source: "builtin", Builtin: true}); err != nil {
		t.Fatalf("SaveSkill legacy builtin: %v", err)
	}
	if err := store.ensureBuiltins(ctx); err != nil {
		t.Fatalf("ensureBuiltins after legacy builtin: %v", err)
	}
	if _, ok, err := store.Skill(ctx, strategypinespec.LegacyBuiltinSkillName); err != nil || ok {
		t.Fatalf("legacy builtin store skill ok=%v err=%v, want deleted", ok, err)
	}

	if _, err := store.SaveSkill(ctx, Skill{ID: strategypinespec.LegacyBuiltinSkillName, DisplayName: "External Strategy", Source: "filesystem", Builtin: false}); err != nil {
		t.Fatalf("SaveSkill legacy external: %v", err)
	}
	if err := store.ensureBuiltins(ctx); err != nil {
		t.Fatalf("ensureBuiltins after legacy external: %v", err)
	}
	if skill, ok, err := store.Skill(ctx, strategypinespec.LegacyBuiltinSkillName); err != nil || !ok || skill.Source != "filesystem" {
		t.Fatalf("legacy external store skill = %+v ok=%v err=%v, want preserved", skill, ok, err)
	}
}

func TestBuiltinStrategySkillRefreshesOutdatedBundle(t *testing.T) {
	ctx := context.Background()
	runtime := newTestRuntime(t)
	skillDir := filepath.Join(runtime.Store().SkillsPath(), strategypinespec.ResearchBuiltinSkillName)
	skillPath := filepath.Join(skillDir, "SKILL.md")
	if err := os.WriteFile(skillPath, []byte(`---
name: jftrade-strategy-research
description: outdated research builtin
allowed-tools: [strategy.validate_pine]
metadata:
  source: builtin
  version: 1
---
Old research instructions.
`), 0o644); err != nil {
		t.Fatalf("WriteFile outdated strategy skill: %v", err)
	}
	if err := os.Remove(filepath.Join(skillDir, "references", "pine-v6-spec.md")); err != nil {
		t.Fatalf("Remove spec resource: %v", err)
	}
	legacyDir := filepath.Join(runtime.Store().SkillsPath(), strategypinespec.LegacyBuiltinSkillName)
	if err := os.MkdirAll(legacyDir, 0o755); err != nil {
		t.Fatalf("MkdirAll legacyDir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(legacyDir, "SKILL.md"), []byte(`---
name: jftrade-strategy
description: legacy builtin
allowed-tools: [strategy.validate_pine]
metadata:
  source: builtin
  version: 7
---
Legacy strategy instructions.
`), 0o644); err != nil {
		t.Fatalf("WriteFile legacy strategy skill: %v", err)
	}

	if err := runtime.Skills().ensureBuiltins(); err != nil {
		t.Fatalf("ensureBuiltins: %v", err)
	}

	skill, ok, err := runtime.Skills().Get(ctx, strategypinespec.ResearchBuiltinSkillName)
	if err != nil || !ok {
		t.Fatalf("Get refreshed strategy skill ok=%v err=%v", ok, err)
	}
	if skill.Version != strategypinespec.BuiltinSkillVersion {
		t.Fatalf("refreshed strategy skill version = %q, want %q", skill.Version, strategypinespec.BuiltinSkillVersion)
	}
	raw, err := os.ReadFile(filepath.Join(skillDir, "references", "pine-v6-spec.md"))
	if err != nil {
		t.Fatalf("ReadFile restored spec: %v", err)
	}
	if !strings.Contains(string(raw), "# JFTrade Pine Script v6 规范") {
		t.Fatalf("restored spec content = %q, want DSL heading", string(raw))
	}
	if _, err := os.Stat(legacyDir); !os.IsNotExist(err) {
		t.Fatalf("legacy builtin skill dir stat err = %v, want not exist", err)
	}
	if _, ok, err := runtime.Skills().Get(ctx, strategypinespec.LegacyBuiltinSkillName); err != nil || ok {
		t.Fatalf("legacy strategy skill ok=%v err=%v, want absent", ok, err)
	}
}

func TestBuiltinStrategyAgentTemplatesExposeExplicitStrategyTools(t *testing.T) {
	defaultAgent, ok := BuiltinAgentTemplate(DefaultBuiltinAgentID)
	if !ok {
		t.Fatalf("BuiltinAgentTemplate(%q) not found", DefaultBuiltinAgentID)
	}
	if !sameStringSet(defaultAgent.Skills, BuiltinSkillIDs()) {
		t.Fatalf("default agent skills = %+v, want all builtin skills %+v", defaultAgent.Skills, BuiltinSkillIDs())
	}

	for _, agentID := range []string{"investment-analyst", "strategy-researcher", "risk-reviewer"} {
		template, ok := BuiltinAgentTemplate(agentID)
		if !ok {
			t.Fatalf("BuiltinAgentTemplate(%q) not found", agentID)
		}
		if containsString(template.Skills, strategypinespec.LegacyBuiltinSkillName) {
			t.Fatalf("template %q still references legacy strategy skill: %+v", agentID, template.Skills)
		}
	}
	investment, _ := BuiltinAgentTemplate("investment-analyst")
	if !containsString(investment.Skills, strategypinespec.ResearchBuiltinSkillName) || containsString(investment.Skills, strategypinespec.PublishBuiltinSkillName) {
		t.Fatalf("investment skills = %+v, want research only", investment.Skills)
	}
	for _, toolName := range strategypinespec.ResearchSkillAllowedTools() {
		if !containsString(investment.Tools, toolName) {
			t.Fatalf("investment tools = %+v, want research tool %s", investment.Tools, toolName)
		}
	}
	for _, toolName := range []string{"strategy.save_definition", "strategy.update_instance_mode"} {
		if containsString(investment.Tools, toolName) {
			t.Fatalf("investment tools unexpectedly include publish tool %s: %+v", toolName, investment.Tools)
		}
	}

	researcher, _ := BuiltinAgentTemplate("strategy-researcher")
	for _, skillName := range []string{strategypinespec.ResearchBuiltinSkillName, strategypinespec.PublishBuiltinSkillName} {
		if !containsString(researcher.Skills, skillName) {
			t.Fatalf("strategy-researcher skills = %+v, want %s", researcher.Skills, skillName)
		}
	}
	for _, toolName := range append(strategypinespec.ResearchSkillAllowedTools(), strategypinespec.PublishSkillAllowedTools()...) {
		if !containsString(researcher.Tools, toolName) {
			t.Fatalf("strategy-researcher tools = %+v, want %s", researcher.Tools, toolName)
		}
	}

	risk, _ := BuiltinAgentTemplate("risk-reviewer")
	if !containsString(risk.Skills, strategypinespec.PublishBuiltinSkillName) || containsString(risk.Skills, strategypinespec.ResearchBuiltinSkillName) {
		t.Fatalf("risk skills = %+v, want publish only", risk.Skills)
	}
	for _, toolName := range strategypinespec.PublishSkillAllowedTools() {
		if !containsString(risk.Tools, toolName) {
			t.Fatalf("risk tools = %+v, want publish tool %s", risk.Tools, toolName)
		}
	}
}

func TestBuiltinRefreshDoesNotOverrideNonBuiltinSkill(t *testing.T) {
	runtime := newTestRuntime(t)
	skillDir := filepath.Join(runtime.Store().SkillsPath(), strategypinespec.LegacyBuiltinSkillName)
	if err := os.RemoveAll(skillDir); err != nil {
		t.Fatalf("RemoveAll skillDir: %v", err)
	}
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatalf("MkdirAll skillDir: %v", err)
	}
	skillPath := filepath.Join(skillDir, "SKILL.md")
	externalSkill := `---
name: jftrade-strategy
description: custom external strategy skill
allowed-tools: [strategy.definitions]
metadata:
  source: https://example.com/jftrade-strategy/SKILL.md
  version: custom
---
Use the custom external strategy instructions.
`
	if err := os.WriteFile(skillPath, []byte(externalSkill), 0o644); err != nil {
		t.Fatalf("WriteFile external strategy skill: %v", err)
	}

	if err := runtime.Skills().ensureBuiltins(); err != nil {
		t.Fatalf("ensureBuiltins: %v", err)
	}

	raw, err := os.ReadFile(skillPath)
	if err != nil {
		t.Fatalf("ReadFile strategy skill: %v", err)
	}
	if string(raw) != externalSkill {
		t.Fatalf("external skill was overwritten:\n%s", string(raw))
	}
}

func TestInstallSkillArchivePreservesResources(t *testing.T) {
	ctx := context.Background()
	runtime := newTestRuntime(t)
	var archive bytes.Buffer
	writer := zip.NewWriter(&archive)
	skillFile, err := writer.Create("research-pack/SKILL.md")
	if err != nil {
		t.Fatalf("Create SKILL.md: %v", err)
	}
	if _, err := skillFile.Write([]byte("---\nname: research-pack\ndescription: Research pack\nallowed-tools: [http.fetch]\n---\nUse bundled resources.\n")); err != nil {
		t.Fatalf("Write SKILL.md: %v", err)
	}
	resourceFile, err := writer.Create("research-pack/references/playbook.md")
	if err != nil {
		t.Fatalf("Create resource: %v", err)
	}
	if _, err := resourceFile.Write([]byte("playbook content")); err != nil {
		t.Fatalf("Write resource: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("Close zip writer: %v", err)
	}
	sourceURL := "https://example.com/research-pack.zip"
	skill, err := runtime.Skills().installArchive(ctx, sourceURL, archive.Bytes())
	if err != nil {
		t.Fatalf("installArchive: %v", err)
	}
	if skill.Source != sourceURL {
		t.Fatalf("skill source = %q, want %q", skill.Source, sourceURL)
	}
	resourcePath := filepath.Join(runtime.Store().SkillsPath(), "research-pack", "references", "playbook.md")
	raw, err := os.ReadFile(resourcePath)
	if err != nil {
		t.Fatalf("ReadFile resource: %v", err)
	}
	if string(raw) != "playbook content" {
		t.Fatalf("resource content = %q", string(raw))
	}
}

func TestInstallSkillURLInstallsNeodataFinancialSearch(t *testing.T) {
	ctx := context.Background()
	runtime := newTestRuntime(t)
	originalValidator := skillInstallHostValidator
	skillInstallHostValidator = func(context.Context, string) error { return nil }
	t.Cleanup(func() { skillInstallHostValidator = originalValidator })
	skillServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/skills/neodata-financial-search/SKILL.md" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
		_, jftradeErr4 := w.Write([]byte(`---
name: neodata-financial-search
description: Search NeoData financial filings and earnings materials.
allowed-tools: [http.fetch]
metadata:
  version: 2026.06
---
Use NeoData search results as reference material and cite the source URL.`))
		jftradeCheckTestError(t, jftradeErr4)
	}))
	t.Cleanup(skillServer.Close)

	skillURL := skillServer.URL + "/skills/neodata-financial-search/SKILL.md"
	skill, err := runtime.Skills().InstallURL(ctx, skillURL)
	if err != nil {
		t.Fatalf("InstallURL: %v", err)
	}
	if skill.ID != "neodata-financial-search" {
		t.Fatalf("skill ID = %q, want neodata-financial-search", skill.ID)
	}
	if skill.Source != skillURL {
		t.Fatalf("skill source = %q, want %q", skill.Source, skillURL)
	}
	if skill.Version != "2026.06" {
		t.Fatalf("skill version = %q, want 2026.06", skill.Version)
	}
	if len(skill.Tools) != 1 || skill.Tools[0] != "http.fetch" {
		t.Fatalf("skill tools = %+v, want [http.fetch]", skill.Tools)
	}
	installedPath := filepath.Join(runtime.Store().SkillsPath(), "neodata-financial-search", "SKILL.md")
	if _, err := os.Stat(installedPath); err != nil {
		t.Fatalf("installed skill path stat: %v", err)
	}
	stored, ok, err := runtime.Skills().Get(ctx, "neodata-financial-search")
	if err != nil {
		t.Fatalf("Get installed skill: %v", err)
	}
	if !ok {
		t.Fatal("installed skill not found in registry")
	}
	if stored.ContentHash == "" || stored.ValidationStatus != "VALID" {
		t.Fatalf("stored skill metadata = %+v", stored)
	}
}

func TestResolveSessionRejectsDifferentAgent(t *testing.T) {
	ctx := context.Background()
	runtime := newTestRuntime(t)
	session, err := runtime.Store().CreateSession(ctx, "agent-a", "test")
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	_, err = runtime.resolveSession(ctx, session.ID, Agent{ID: "agent-b"}, "hello")
	if err == nil || !strings.Contains(err.Error(), "different agent") {
		t.Fatalf("resolveSession error = %v, want different agent", err)
	}
}

func TestDeleteAgentSoftDeletesHistoricalRecord(t *testing.T) {
	ctx := context.Background()
	runtime := newTestRuntime(t)
	agent := mustSaveAgent(t, runtime, AgentWriteRequest{
		ID: "agent", Name: "Agent", Status: AgentStatusEnabled,
	})
	if err := runtime.Store().DeleteAgent(ctx, agent.ID); err != nil {
		t.Fatalf("DeleteAgent: %v", err)
	}
	stored, ok, err := runtime.Store().Agent(ctx, agent.ID)
	if err != nil || !ok {
		t.Fatalf("Agent after delete: ok=%v err=%v", ok, err)
	}
	if stored.DeletedAt == nil || stored.Status != AgentStatusDisabled {
		t.Fatalf("soft deleted agent = %+v", stored)
	}
	agents, err := runtime.Store().ListAgents(ctx)
	if err != nil {
		t.Fatalf("ListAgents: %v", err)
	}
	userAgents := nonBuiltinAgents(agents)
	if len(userAgents) != 0 {
		t.Fatalf("active user agents = %+v, want none", userAgents)
	}
}

func TestListAgentsExcludesSoftDeletedWhileListAllIncludesThem(t *testing.T) {
	ctx := context.Background()
	runtime := newTestRuntime(t)

	mustSaveAgent(t, runtime, AgentWriteRequest{
		ID: "agent-older", Name: "Older Agent", Status: AgentStatusEnabled,
	})
	time.Sleep(10 * time.Millisecond)
	mustSaveAgent(t, runtime, AgentWriteRequest{
		ID: "agent-newer", Name: "Newer Agent", Status: AgentStatusEnabled,
	})
	if err := runtime.Store().DeleteAgent(ctx, "agent-older"); err != nil {
		t.Fatalf("DeleteAgent older: %v", err)
	}

	active, err := runtime.Store().ListAgents(ctx)
	if err != nil {
		t.Fatalf("ListAgents: %v", err)
	}
	activeUserAgents := nonBuiltinAgents(active)
	if len(activeUserAgents) != 1 || activeUserAgents[0].ID != "agent-newer" {
		t.Fatalf("active user agents = %+v, want only agent-newer", activeUserAgents)
	}

	all, err := runtime.Store().ListAllAgents(ctx)
	if err != nil {
		t.Fatalf("ListAllAgents: %v", err)
	}
	allUserAgents := nonBuiltinAgents(all)
	if len(allUserAgents) != 2 {
		t.Fatalf("all user agents len = %d, want 2", len(allUserAgents))
	}
	var deletedFound bool
	for _, agent := range allUserAgents {
		if agent.ID == "agent-older" {
			deletedFound = agent.DeletedAt != nil && agent.Status == AgentStatusDisabled
		}
	}
	if !deletedFound {
		t.Fatalf("all agents = %+v, want deleted agent-older preserved historically", all)
	}
}

func TestSaveAgentRestoresDeletedAgentRecord(t *testing.T) {
	ctx := context.Background()
	runtime := newTestRuntime(t)
	agent := mustSaveAgent(t, runtime, AgentWriteRequest{
		ID: "agent", Name: "Agent", Status: AgentStatusEnabled,
	})
	if err := runtime.Store().DeleteAgent(ctx, agent.ID); err != nil {
		t.Fatalf("DeleteAgent: %v", err)
	}
	restored, err := runtime.Store().SaveAgent(ctx, AgentWriteRequest{
		ID: "agent", Name: "Agent Restored", Status: AgentStatusEnabled,
	})
	if err != nil {
		t.Fatalf("SaveAgent restore: %v", err)
	}
	if restored.DeletedAt != nil {
		t.Fatalf("restored agent deleted_at = %v, want nil", restored.DeletedAt)
	}
	agents, err := runtime.Store().ListAgents(ctx)
	if err != nil {
		t.Fatalf("ListAgents: %v", err)
	}
	userAgents := nonBuiltinAgents(agents)
	if len(userAgents) != 1 || userAgents[0].Name != "Agent Restored" {
		t.Fatalf("active user agents = %+v, want restored visible agent", userAgents)
	}
}

func nonBuiltinAgents(agents []Agent) []Agent {
	filtered := make([]Agent, 0, len(agents))
	for _, agent := range agents {
		if agent.Builtin {
			continue
		}
		filtered = append(filtered, agent)
	}
	return filtered
}

func TestCancelPendingRunDeniesApprovals(t *testing.T) {
	ctx := context.Background()
	runtime := newTestRuntime(t)
	run := mustSaveRun(t, runtime, Run{
		ID: "run-cancel", SessionID: "session", AgentID: "agent", Status: RunStatusPending,
		PendingApprovals: []Approval{{ID: "approval-cancel", RunID: "run-cancel", AgentID: "agent", Status: ApprovalStatusPending}},
		CreatedAt:        nowString(), UpdatedAt: nowString(),
	})
	if err := runtime.Store().SaveApproval(ctx, run.PendingApprovals[0]); err != nil {
		t.Fatalf("SaveApproval: %v", err)
	}
	cancelled, err := runtime.CancelRun(ctx, run.ID)
	if err != nil {
		t.Fatalf("CancelRun: %v", err)
	}
	if cancelled.Status != RunStatusCancelled || cancelled.CancelledAt == nil {
		t.Fatalf("cancelled run = %+v", cancelled)
	}
	approval, ok, err := runtime.Store().Approval(ctx, "approval-cancel")
	if err != nil || !ok || approval.Status != ApprovalStatusDenied {
		t.Fatalf("cancelled approval = %+v ok=%v err=%v", approval, ok, err)
	}
}

func TestCancelRunMissingReturnsNotFound(t *testing.T) {
	ctx := context.Background()
	runtime := newTestRuntime(t)

	_, err := runtime.CancelRun(ctx, "run-missing")
	if err == nil || err.Error() != "run not found" {
		t.Fatalf("CancelRun missing error = %v, want run not found", err)
	}
}

func TestResolveApprovalMissingReturnsIdempotentEmptyResult(t *testing.T) {
	ctx := context.Background()
	runtime := newTestRuntime(t)

	resolution, err := runtime.ResolveApproval(ctx, "approval-missing", true)
	if err != nil {
		t.Fatalf("ResolveApproval missing: %v", err)
	}
	if resolution.Run != nil || resolution.Message != nil {
		t.Fatalf("missing approval resolution = %+v, want no run/message", resolution)
	}
	if resolution.Approval.ID != "" || resolution.Approval.Status != "" {
		t.Fatalf("missing approval = %+v, want zero-value approval", resolution.Approval)
	}
}

func TestStoreResolvePendingApprovalMissingAndIdempotent(t *testing.T) {
	ctx := context.Background()
	runtime := newTestRuntime(t)

	approval, changed, err := runtime.Store().ResolvePendingApproval(ctx, "approval-missing", ApprovalStatusApproved)
	if err != nil {
		t.Fatalf("ResolvePendingApproval missing: %v", err)
	}
	if changed {
		t.Fatal("missing approval unexpectedly reported changed=true")
	}
	if approval.ID != "" || approval.Status != "" {
		t.Fatalf("missing approval = %+v, want zero value", approval)
	}

	stored := Approval{
		ID: "approval-approved", RunID: "run-1", AgentID: "agent-1", ToolName: "strategy.save_draft",
		Status: ApprovalStatusApproved, CreatedAt: nowString(), UpdatedAt: nowString(),
	}
	if err := runtime.Store().SaveApproval(ctx, stored); err != nil {
		t.Fatalf("SaveApproval: %v", err)
	}
	resolved, changed, err := runtime.Store().ResolvePendingApproval(ctx, stored.ID, ApprovalStatusDenied)
	if err != nil {
		t.Fatalf("ResolvePendingApproval approved: %v", err)
	}
	if changed {
		t.Fatal("non-pending approval unexpectedly changed")
	}
	if resolved.Status != ApprovalStatusApproved {
		t.Fatalf("resolved status = %q, want approved", resolved.Status)
	}
}

func TestListRunsPageFiltersAndSortsNewestFirst(t *testing.T) {
	ctx := context.Background()
	runtime := newTestRuntime(t)

	runs := []Run{
		{
			ID: "run-older", SessionID: "session-a", AgentID: "agent-a", Status: RunStatusFailed,
			CreatedAt: "2024-01-01T00:00:00Z", UpdatedAt: "2024-01-01T00:00:00Z",
		},
		{
			ID: "run-newer", SessionID: "session-a", AgentID: "agent-a", Status: RunStatusFailed,
			CreatedAt: "2024-01-02T00:00:00Z", UpdatedAt: "2024-01-02T00:00:00Z",
		},
		{
			ID: "run-other-session", SessionID: "session-b", AgentID: "agent-a", Status: RunStatusFailed,
			CreatedAt: "2024-01-03T00:00:00Z", UpdatedAt: "2024-01-03T00:00:00Z",
		},
		{
			ID: "run-other-status", SessionID: "session-a", AgentID: "agent-a", Status: RunStatusCompleted,
			CreatedAt: "2024-01-04T00:00:00Z", UpdatedAt: "2024-01-04T00:00:00Z",
		},
	}
	for _, run := range runs {
		mustSaveRun(t, runtime, run)
	}

	page, total, err := runtime.Store().ListRunsPage(ctx, RunStatusFailed, "agent-a", "session-a", 10, 0)
	if err != nil {
		t.Fatalf("ListRunsPage: %v", err)
	}
	if total != 2 {
		t.Fatalf("total = %d, want 2", total)
	}
	if len(page) != 2 {
		t.Fatalf("page len = %d, want 2", len(page))
	}
	if page[0].ID != "run-newer" || page[1].ID != "run-older" {
		t.Fatalf("page order = [%s %s], want [run-newer run-older]", page[0].ID, page[1].ID)
	}
}

func TestDuplicateApprovalResolutionDoesNotExecuteTwice(t *testing.T) {
	ctx := context.Background()
	runtime := newTestRuntime(t)
	executions := 0
	registry := NewToolRegistry()
	registry.Register(ToolDescriptor{
		Name:               "approval.required",
		Permission:         "write_strategy",
		AllowedModes:       []string{PermissionModeApproval},
		RequiresApprovalIn: []string{PermissionModeApproval},
	}, func(context.Context, map[string]any) (any, error) {
		executions++
		return map[string]any{"saved": true}, nil
	})
	runtime = newRuntimeWithRegistry(t, runtime.Store(), registry)
	agent, err := runtime.Store().SaveAgent(ctx, AgentWriteRequest{
		ID: "agent", Name: "Agent", ProviderID: testProviderID, Tools: []string{"approval.required"},
		PermissionMode: PermissionModeApproval, Status: AgentStatusEnabled,
	})
	if err != nil {
		t.Fatalf("SaveAgent: %v", err)
	}
	response, err := runtime.Chat(ctx, ChatRequest{AgentID: agent.ID, Message: "@approval.required save"})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	id := response.PendingApprovals[0].ID
	if _, err := runtime.ResolveApproval(ctx, id, true); err != nil {
		t.Fatalf("first ResolveApproval: %v", err)
	}
	if _, err := runtime.ResolveApproval(ctx, id, true); err != nil {
		t.Fatalf("second ResolveApproval: %v", err)
	}
	if executions != 1 {
		t.Fatalf("executions = %d, want 1", executions)
	}
}

func TestPendingApprovalResumesThroughGoogleADKAfterRuntimeRestart(t *testing.T) {
	ctx := context.Background()
	runtime := newTestRuntime(t)
	executions := 0
	registry := NewToolRegistry()
	registry.Register(ToolDescriptor{
		Name:               "approval.required",
		Permission:         "write_strategy",
		AllowedModes:       []string{PermissionModeApproval},
		RequiresApprovalIn: []string{PermissionModeApproval},
	}, func(context.Context, map[string]any) (any, error) {
		executions++
		return map[string]any{"saved": true}, nil
	})
	runtime = newRuntimeWithRegistry(t, runtime.Store(), registry)
	agent, err := runtime.Store().SaveAgent(ctx, AgentWriteRequest{
		ID: "agent", Name: "Agent", ProviderID: testProviderID, Tools: []string{"approval.required"},
		PermissionMode: PermissionModeApproval, Status: AgentStatusEnabled,
	})
	if err != nil {
		t.Fatalf("SaveAgent: %v", err)
	}
	response, err := runtime.Chat(ctx, ChatRequest{
		AgentID: agent.ID, Message: "@approval.required save",
	})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	approval := response.PendingApprovals[0]
	if approval.FunctionCallID == "" || approval.ConfirmationCallID == "" {
		t.Fatalf("approval lacks GO-ADK confirmation identifiers: %+v", approval)
	}

	restarted := newRuntimeWithRegistry(t, runtime.Store(), registry)
	restartedRun, ok, err := restarted.Store().Run(ctx, response.Run.ID)
	if err != nil || !ok {
		t.Fatalf("restarted run lookup err=%v ok=%v", err, ok)
	}
	if restartedRun.Status != RunStatusPending {
		t.Fatalf("restarted run status = %q, want %q", restartedRun.Status, RunStatusPending)
	}
	resolution, err := restarted.ResolveApproval(ctx, approval.ID, true)
	if err != nil {
		t.Fatalf("ResolveApproval after restart: %v", err)
	}
	if executions != 1 {
		t.Fatalf("executions = %d, want 1", executions)
	}
	if resolution.Run == nil || resolution.Run.Status != RunStatusCompleted {
		t.Fatalf("resolution run = %+v, want completed", resolution.Run)
	}
	if resolution.Run.ResumeState != "adk_confirmation_resolved" {
		t.Fatalf("resume state = %q, want GO-ADK resume", resolution.Run.ResumeState)
	}
}

func TestApprovalResumingRunIsRecoveredAfterRuntimeRestart(t *testing.T) {
	ctx := context.Background()
	runtime := newTestRuntime(t)
	releaseTool := make(chan struct{})
	toolStarted := make(chan struct{}, 1)
	executions := 0
	registry := NewToolRegistry()
	registry.Register(ToolDescriptor{
		Name:               "approval.required",
		Permission:         "write_strategy",
		AllowedModes:       []string{PermissionModeApproval},
		RequiresApprovalIn: []string{PermissionModeApproval},
	}, func(ctx context.Context, _ map[string]any) (any, error) {
		executions++
		select {
		case toolStarted <- struct{}{}:
		default:
		}
		select {
		case <-releaseTool:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
		return map[string]any{"saved": true}, nil
	})
	runtime = newRuntimeWithRegistry(t, runtime.Store(), registry)
	agent, err := runtime.Store().SaveAgent(ctx, AgentWriteRequest{
		ID: "agent", Name: "Agent", ProviderID: testProviderID, Tools: []string{"approval.required"},
		PermissionMode: PermissionModeApproval, Status: AgentStatusEnabled,
	})
	if err != nil {
		t.Fatalf("SaveAgent: %v", err)
	}
	response, err := runtime.Chat(ctx, ChatRequest{
		AgentID: agent.ID, Message: "@approval.required save",
	})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	approval := response.PendingApprovals[0]
	approvedApproval, changed, err := runtime.Store().ResolvePendingApproval(ctx, approval.ID, ApprovalStatusApproved)
	if err != nil || !changed {
		t.Fatalf("ResolvePendingApproval changed=%v err=%v", changed, err)
	}
	run, ok, err := runtime.Store().Run(ctx, response.Run.ID)
	if err != nil || !ok {
		t.Fatalf("Run lookup err=%v ok=%v", err, ok)
	}
	run.Status = RunStatusRunning
	run.ResumeState = "approval_resuming"
	for index := range run.PendingApprovals {
		if run.PendingApprovals[index].ID == approvedApproval.ID {
			run.PendingApprovals[index] = approvedApproval
		}
	}
	for index := range run.ToolCalls {
		if run.ToolCalls[index].Status != "PENDING_APPROVAL" {
			continue
		}
		run.ToolCalls[index].Status = "RUNNING"
		run.ToolCalls[index].RequiresUser = false
		run.ToolCalls[index].UpdatedAt = nowString()
	}
	if err := runtime.Store().SaveRun(ctx, run); err != nil {
		t.Fatalf("SaveRun: %v", err)
	}

	restarted := newRuntimeWithRegistry(t, runtime.Store(), registry)
	restartedRun, ok, err := restarted.Store().Run(ctx, response.Run.ID)
	if err != nil || !ok {
		t.Fatalf("restarted run lookup err=%v ok=%v", err, ok)
	}
	if restartedRun.Status != RunStatusRunning || restartedRun.ResumeState != "approval_resuming" {
		t.Fatalf("restarted run = %+v, want running approval_resuming", restartedRun)
	}

	restarted.ReconcileResolvedApprovals(ctx)

	select {
	case <-toolStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for recovered approval continuation to resume")
	}

	timeline, ok, err := restarted.Store().SessionTimeline(ctx, response.Run.SessionID)
	if err != nil || !ok {
		t.Fatalf("SessionTimeline ok=%v err=%v", ok, err)
	}
	toolGroupSeen := false
	for _, entry := range timeline {
		if entry.Kind == TimelineKindApprovalGroup {
			t.Fatalf("timeline approval group = %+v, want resolved approval omitted", entry)
		}
		if entry.Kind == TimelineKindToolGroup && entry.RunID == response.Run.ID {
			toolGroupSeen = true
			if len(entry.ToolCalls) != 1 || entry.ToolCalls[0].Status != "RUNNING" {
				t.Fatalf("timeline tool group = %+v, want running tool call", entry)
			}
		}
	}
	if !toolGroupSeen {
		t.Fatalf("timeline = %+v, want tool group for run %s", timeline, response.Run.ID)
	}

	close(releaseTool)

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		stored, ok, err := restarted.Store().Run(ctx, response.Run.ID)
		if err != nil || !ok {
			t.Fatalf("stored run lookup err=%v ok=%v", err, ok)
		}
		if stored.Status == RunStatusCompleted {
			if executions != 1 {
				t.Fatalf("executions = %d, want 1", executions)
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	stored, ok, err := restarted.Store().Run(ctx, response.Run.ID)
	t.Fatalf("stored run after recovered continuation = %+v ok=%v err=%v, want completed", stored, ok, err)
}

func TestUnrecoverablePendingApprovalRunIsMarkedOrphanedOnRestart(t *testing.T) {
	ctx := context.Background()
	runtime := newTestRuntime(t)
	run := Run{
		ID:        "run-orphaned-pending",
		SessionID: "session-1",
		AgentID:   "agent-1",
		Status:    RunStatusPending,
		Message:   "waiting approval",
		CreatedAt: nowString(),
		StartedAt: nowString(),
		UpdatedAt: nowString(),
		PendingApprovals: []Approval{{
			ID:        "approval-1",
			RunID:     "run-orphaned-pending",
			AgentID:   "agent-1",
			ToolName:  "strategy.save_draft",
			Status:    ApprovalStatusPending,
			Reason:    "needs approval",
			CreatedAt: nowString(),
			UpdatedAt: nowString(),
		}},
	}
	if err := runtime.Store().SaveRun(ctx, run); err != nil {
		t.Fatalf("SaveRun: %v", err)
	}

	restarted := newRuntimeWithRegistry(t, runtime.Store(), NewToolRegistry())
	stored, ok, err := restarted.Store().Run(ctx, run.ID)
	if err != nil || !ok {
		t.Fatalf("Run lookup err=%v ok=%v", err, ok)
	}
	if stored.Status != RunStatusFailed {
		t.Fatalf("stored status = %q, want %q", stored.Status, RunStatusFailed)
	}
	if stored.ErrorCode != "RUN_ORPHANED" {
		t.Fatalf("stored error code = %q, want RUN_ORPHANED", stored.ErrorCode)
	}
	if stored.ResumeState != "approval_context_missing" {
		t.Fatalf("stored resume state = %q, want approval_context_missing", stored.ResumeState)
	}
}

func TestMultipleApprovalsExecuteOnlyAfterAllApproved(t *testing.T) {
	ctx := context.Background()
	runtime := newTestRuntime(t)
	var executions atomic.Int64
	registry := NewToolRegistry()
	for _, name := range []string{"approval.required.one", "approval.required.two"} {
		registry.Register(ToolDescriptor{
			Name:               name,
			Permission:         "write_strategy",
			AllowedModes:       []string{PermissionModeApproval},
			RequiresApprovalIn: []string{PermissionModeApproval},
		}, func(context.Context, map[string]any) (any, error) {
			executions.Add(1)
			return map[string]any{"ok": true}, nil
		})
	}
	runtime = newRuntimeWithRegistry(t, runtime.Store(), registry)
	agent, err := runtime.Store().SaveAgent(ctx, AgentWriteRequest{
		ID: "agent", Name: "Agent", ProviderID: testProviderID, Tools: []string{"approval.required.one", "approval.required.two"},
		PermissionMode: PermissionModeApproval, Status: AgentStatusEnabled,
	})
	if err != nil {
		t.Fatalf("SaveAgent: %v", err)
	}
	response, err := runtime.Chat(ctx, ChatRequest{
		AgentID: agent.ID,
		Message: `<execute-tool name="approval.required.one" /><execute-tool name="approval.required.two" />`,
	})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if len(response.PendingApprovals) != 2 {
		t.Fatalf("pending approvals = %d, want 2", len(response.PendingApprovals))
	}
	first, err := runtime.ResolveApproval(ctx, response.PendingApprovals[0].ID, true)
	if err != nil {
		t.Fatalf("first approval: %v", err)
	}
	if got := executions.Load(); got != 0 {
		t.Fatalf("executions after first approval = %d, want 0", got)
	}
	if first.Run == nil || first.Run.Status != RunStatusPending {
		t.Fatalf("first approval run = %+v, want pending", first.Run)
	}
	second, err := runtime.ResolveApproval(ctx, response.PendingApprovals[1].ID, true)
	if err != nil {
		t.Fatalf("second approval: %v", err)
	}
	if got := executions.Load(); got != 2 {
		t.Fatalf("executions after all approvals = %d, want 2", got)
	}
	if second.Run == nil || second.Run.Status != RunStatusCompleted {
		t.Fatalf("second approval run = %+v, want completed", second.Run)
	}
}

func TestADKTaskUpdateDeleteAndValidation(t *testing.T) {
	ctx := context.Background()
	runtime := newTestRuntime(t)
	task, err := runtime.Store().SaveTask(ctx, TaskWriteRequest{
		ID: "task-a", Title: "Original", Status: "TODO", AgentID: "agent-a", DependsOn: []string{"task-b", "task-b"},
		Order: 2, ModeHint: WorkModeTask, AgentRole: "实现 Agent", PlannerStepID: "__planner_step_2",
		PlanSource: workflowPlanSourcePlanner, WorkflowMode: WorkModeTask, Objective: "完成目标",
		PlannerWarnings: []string{"裁剪了多余步骤", "裁剪了多余步骤"},
	})
	if err != nil {
		t.Fatalf("SaveTask: %v", err)
	}
	if len(task.DependsOn) != 1 || task.DependsOn[0] != "task-b" {
		t.Fatalf("dependsOn = %+v, want deduplicated task-b", task.DependsOn)
	}
	if task.Order != 2 || task.ModeHint != WorkModeTask || task.AgentRole != "实现 Agent" || task.PlannerStepID != "__planner_step_2" || task.PlanSource != workflowPlanSourcePlanner || task.WorkflowMode != WorkModeTask || task.Objective != "完成目标" {
		t.Fatalf("task planner metadata = %+v, want saved metadata", task)
	}
	if len(task.PlannerWarnings) != 1 || task.PlannerWarnings[0] != "裁剪了多余步骤" {
		t.Fatalf("planner warnings = %+v, want deduplicated warning", task.PlannerWarnings)
	}
	description := "kept details"
	status := "IN_PROGRESS"
	warnings := []string{"planner warning"}
	updated, err := runtime.Store().UpdateTask(ctx, task.ID, TaskPatchRequest{Description: &description, Status: &status, Order: new(3), AgentRole: new("验证 Agent"), PlannerWarnings: warnings})
	if err != nil {
		t.Fatalf("UpdateTask: %v", err)
	}
	if updated.Title != "Original" || updated.Description != description || updated.Status != status {
		t.Fatalf("updated task = %+v, want partial update preserving title", updated)
	}
	if updated.Order != 3 || updated.AgentRole != "验证 Agent" || len(updated.PlannerWarnings) != 1 || updated.PlannerWarnings[0] != "planner warning" {
		t.Fatalf("updated planner metadata = %+v, want patched metadata", updated)
	}
	if _, err := runtime.Store().SaveTask(ctx, TaskWriteRequest{ID: "bad-status", Title: "Bad", Status: "NOPE"}); err == nil {
		t.Fatalf("SaveTask invalid status err = nil, want error")
	}
	if _, err := runtime.Store().SaveTask(ctx, TaskWriteRequest{ID: "self", Title: "Self", DependsOn: []string{"self"}}); err == nil {
		t.Fatalf("SaveTask self dependency err = nil, want error")
	}
	tasks, total, err := runtime.Store().ListTasksPage(ctx, "IN_PROGRESS", "agent-a", "", 20, 0)
	if err != nil {
		t.Fatalf("ListTasksPage: %v", err)
	}
	if total != 1 || len(tasks) != 1 || tasks[0].ID != task.ID {
		t.Fatalf("filtered tasks total=%d tasks=%+v, want task-a", total, tasks)
	}
	if err := runtime.Store().DeleteTask(ctx, task.ID); err != nil {
		t.Fatalf("DeleteTask: %v", err)
	}
	if _, ok, err := runtime.Store().Task(ctx, task.ID); err != nil || ok {
		t.Fatalf("Task after delete ok=%v err=%v, want missing", ok, err)
	}
}

func TestADKMemoryFiltersDeleteAndAgentValidation(t *testing.T) {
	ctx := context.Background()
	runtime := newTestRuntime(t)
	agent, err := runtime.Store().SaveAgent(ctx, AgentWriteRequest{ID: "agent-memory", Name: "Agent", ProviderID: testProviderID, Status: AgentStatusEnabled})
	if err != nil {
		t.Fatalf("SaveAgent: %v", err)
	}
	workspace, err := runtime.Store().SaveMemory(ctx, MemoryWriteRequest{Scope: "workspace", Key: "Market", Value: "HK"})
	if err != nil {
		t.Fatalf("SaveMemory workspace: %v", err)
	}
	agentEntry, err := runtime.Store().SaveMemory(ctx, MemoryWriteRequest{Scope: "agent", AgentID: agent.ID, Key: "Style", Value: "risk first"})
	if err != nil {
		t.Fatalf("SaveMemory agent: %v", err)
	}
	if _, err := runtime.Store().SaveMemory(ctx, MemoryWriteRequest{Scope: "agent", Key: "missing", Value: "bad"}); err == nil {
		t.Fatalf("SaveMemory agent without agentId err = nil, want error")
	}
	entries, err := runtime.Store().ListMemoryFiltered(ctx, "agent", agent.ID, "style")
	if err != nil {
		t.Fatalf("ListMemoryFiltered: %v", err)
	}
	if len(entries) != 1 || entries[0].ID != agentEntry.ID {
		t.Fatalf("agent memory entries = %+v, want style entry", entries)
	}
	promptEntries, err := runtime.Store().ListMemory(ctx, agent.ID)
	if err != nil {
		t.Fatalf("ListMemory: %v", err)
	}
	if len(promptEntries) != 2 {
		t.Fatalf("prompt memory len=%d entries=%+v, want workspace + agent", len(promptEntries), promptEntries)
	}
	if err := runtime.Store().DeleteMemory(ctx, workspace.ID); err != nil {
		t.Fatalf("DeleteMemory: %v", err)
	}
	if _, ok, err := runtime.Store().Memory(ctx, workspace.ID); err != nil || ok {
		t.Fatalf("Memory after delete ok=%v err=%v, want missing", ok, err)
	}
}

func TestPrepareAgentInjectsMemoryOnlyWhenEnabled(t *testing.T) {
	ctx := context.Background()
	runtime := newTestRuntime(t)
	if _, err := runtime.Store().SaveMemory(ctx, MemoryWriteRequest{Scope: "workspace", Key: "preference", Value: "use HK market"}); err != nil {
		t.Fatalf("SaveMemory: %v", err)
	}
	disabled, err := runtime.prepareAgent(ctx, Agent{ID: "agent", Instruction: "base", MemoryEnabled: false})
	if err != nil {
		t.Fatalf("prepareAgent disabled: %v", err)
	}
	if strings.Contains(disabled.Instruction, "JFTrade memory") {
		t.Fatalf("disabled instruction = %q, want no memory", disabled.Instruction)
	}
	enabled, err := runtime.prepareAgent(ctx, Agent{ID: "agent", Instruction: "base", MemoryEnabled: true})
	if err != nil {
		t.Fatalf("prepareAgent enabled: %v", err)
	}
	if !strings.Contains(enabled.Instruction, "use HK market") {
		t.Fatalf("enabled instruction = %q, want memory", enabled.Instruction)
	}
}

func TestToolsSearchReturnsOnlyCurrentAgentTools(t *testing.T) {
	ctx := context.Background()
	registry := NewToolRegistry()
	registry.Register(ToolDescriptor{Name: "visible.read", DisplayName: "Visible", Category: "test", Permission: "read_internal"}, func(context.Context, map[string]any) (any, error) {
		return nil, nil
	})
	registry.Register(ToolDescriptor{Name: "hidden.read", DisplayName: "Hidden", Category: "test", Permission: "read_internal"}, func(context.Context, map[string]any) (any, error) {
		return nil, nil
	})
	registered, ok := registry.Get("tools.search")
	if !ok {
		t.Fatalf("tools.search not registered")
	}
	output, err := executeRegisteredTool(contextWithToolAgent(ctx, Agent{ID: "agent", Tools: []string{"tools.search", "visible.read"}}), registered, map[string]any{"query": "read"})
	if err != nil {
		t.Fatalf("execute tools.search: %v", err)
	}
	payload, ok := output.(map[string]any)
	if !ok {
		t.Fatalf("output = %T, want map", output)
	}
	tools, ok := payload["tools"].([]map[string]any)
	if !ok {
		t.Fatalf("tools payload = %T, want []map[string]any", payload["tools"])
	}
	if len(tools) != 1 || tools[0]["name"] != "visible.read" {
		t.Fatalf("tools.search tools = %+v, want only visible.read", tools)
	}
	if _, ok := tools[0]["requiresApprovalIn"]; !ok {
		t.Fatalf("tools.search result lacks requiresApprovalIn: %+v", tools[0])
	}
}

func containsString(values []string, target string) bool {
	return slices.Contains(values, target)
}

func TestWorkflowWriteToolsRequireApprovalExceptLowRiskTaskWrites(t *testing.T) {
	ctx := context.Background()
	runtime := newTestRuntime(t)
	registry := NewToolRegistry()
	var taskCreates int
	var taskUpdates int
	var taskDeletes int
	var remembers int
	var forgets int
	var draftSaves int
	registry.Register(ToolDescriptor{Name: "tasks.create", Permission: "write_task", AllowedModes: []string{PermissionModeApproval}}, func(context.Context, map[string]any) (any, error) {
		taskCreates++
		return map[string]any{"created": true}, nil
	})
	registry.Register(ToolDescriptor{Name: "tasks.update", Permission: "write_task", AllowedModes: []string{PermissionModeApproval}}, func(context.Context, map[string]any) (any, error) {
		taskUpdates++
		return map[string]any{"updated": true}, nil
	})
	registry.Register(ToolDescriptor{Name: "tasks.delete", Permission: "write_task", AllowedModes: []string{PermissionModeApproval}}, func(context.Context, map[string]any) (any, error) {
		taskDeletes++
		return map[string]any{"deleted": true}, nil
	})
	registry.Register(ToolDescriptor{Name: "memory.remember", Permission: "write_memory", AllowedModes: []string{PermissionModeApproval}}, func(context.Context, map[string]any) (any, error) {
		remembers++
		return map[string]any{"remembered": true}, nil
	})
	registry.Register(ToolDescriptor{Name: "memory.forget", Permission: "write_memory", AllowedModes: []string{PermissionModeApproval}}, func(context.Context, map[string]any) (any, error) {
		forgets++
		return map[string]any{"forgotten": true}, nil
	})
	registry.Register(ToolDescriptor{Name: "strategy.save_draft", Permission: "write_strategy", AllowedModes: []string{PermissionModeApproval}}, func(context.Context, map[string]any) (any, error) {
		draftSaves++
		return map[string]any{"saved": true}, nil
	})
	runtime = newRuntimeWithRegistry(t, runtime.Store(), registry)
	agent, err := runtime.Store().SaveAgent(ctx, AgentWriteRequest{
		ID: "workflow-agent", Name: "Workflow", ProviderID: testProviderID, Tools: []string{"tasks.create", "tasks.update", "tasks.delete", "memory.remember", "memory.forget", "strategy.save_draft"},
		PermissionMode: PermissionModeApproval, Status: AgentStatusEnabled,
	})
	if err != nil {
		t.Fatalf("SaveAgent: %v", err)
	}
	response, err := runtime.Chat(ctx, ChatRequest{
		AgentID: agent.ID,
		Message: `<execute-tool name="tasks.create" title="Follow up" /><execute-tool name="tasks.update" id="task-1" status="DONE" /><execute-tool name="tasks.delete" id="task-2" /><execute-tool name="memory.remember" key="market" value="HK" /><execute-tool name="memory.forget" id="memory-1" /><execute-tool name="strategy.save_draft" name="Draft" script="strategy('x')" />`,
	})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if taskCreates != 1 {
		t.Fatalf("task creates = %d, want executed without approval", taskCreates)
	}
	if taskUpdates != 1 {
		t.Fatalf("task updates = %d, want executed without approval", taskUpdates)
	}
	if taskDeletes != 1 {
		t.Fatalf("task deletes = %d, want executed without approval", taskDeletes)
	}
	if remembers != 1 {
		t.Fatalf("memory remembers = %d, want executed without approval", remembers)
	}
	if forgets != 1 {
		t.Fatalf("memory forgets = %d, want executed without approval", forgets)
	}
	if draftSaves != 1 {
		t.Fatalf("draft saves = %d, want executed without approval", draftSaves)
	}
	if len(response.PendingApprovals) != 0 {
		t.Fatalf("pending approvals = %d, want 0", len(response.PendingApprovals))
	}
}
