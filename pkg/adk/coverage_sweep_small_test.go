package adk

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCoverageSweepSmallBranches(t *testing.T) {
	ctx := t.Context()

	t.Run("json fallback and recent user window cover remaining helpers", func(t *testing.T) {
		if got := jsonFallbackString(map[string]any{"ok": true}); !strings.Contains(got, `"ok":true`) {
			t.Fatalf("jsonFallbackString(map) = %q, want encoded payload", got)
		}
		if got := normalizeRecentUserWindow(101); got != 100 {
			t.Fatalf("normalizeRecentUserWindow(101) = %d, want 100", got)
		}
	})

	t.Run("workflow task toolset switches to goal decision tools", func(t *testing.T) {
		decision := &workflowGoalDecision{}
		decision.beginDecision()
		tools, err := (&workflowTaskToolset{
			req: workflowRequest{Mode: WorkModeLoop, GoalDecision: decision},
		}).Tools(newGoogleADKToolTestContext())
		if err != nil {
			t.Fatalf("workflowTaskToolset.Tools: %v", err)
		}
		if len(tools) != 2 || tools[0].Name() != workflowGoalCompleteTool || tools[1].Name() != workflowGoalContinueTool {
			t.Fatalf("workflow goal decision tools = %#v, want complete/continue only", tools)
		}
	})

	t.Run("google ADK model lookup surfaces disabled providers and secret read errors", func(t *testing.T) {
		disabledRuntime := newTestRuntime(t)
		mustSaveProvider(t, disabledRuntime, ProviderWriteRequest{
			ID:          "disabled-model-provider",
			DisplayName: "Disabled Model Provider",
			BaseURL:     "https://example.test/v1",
			Model:       "test-model",
			APIKey:      "sk-test",
			Enabled:     false,
		})
		if _, err := disabledRuntime.googleADKModelForAgent(ctx, Agent{
			ID: "disabled-model-agent", Name: "Disabled Model Agent", ProviderID: "disabled-model-provider", Model: "test-model",
		}); err == nil || !strings.Contains(err.Error(), "unavailable") {
			t.Fatalf("googleADKModelForAgent disabled provider err = %v", err)
		}

		secretRuntime := newTestRuntime(t)
		mustSaveProvider(t, secretRuntime, ProviderWriteRequest{
			ID:          "broken-secret-provider",
			DisplayName: "Broken Secret Provider",
			BaseURL:     "https://example.test/v1",
			Model:       "test-model",
			Enabled:     true,
		})
		if err := os.WriteFile(secretRuntime.store.secrets.path, []byte("{"), 0o600); err != nil {
			t.Fatalf("WriteFile broken secrets: %v", err)
		}
		if _, err := secretRuntime.googleADKModelForAgent(ctx, Agent{
			ID: "broken-secret-agent", Name: "Broken Secret Agent", ProviderID: "broken-secret-provider", Model: "test-model",
		}); err == nil {
			t.Fatal("googleADKModelForAgent accepted invalid provider secret store")
		}
	})

	t.Run("skill registry source keeps non-not-found loader errors visible", func(t *testing.T) {
		registry := &SkillRegistry{skillsPath: t.TempDir()}
		writeSkillDocument(t, registry.skillsPath, "carrier", "---\nname: source-error-skill\ndescription: Source Error\n---\nBody.")
		if err := os.MkdirAll(filepath.Join(registry.skillsPath, "source-error-skill", "SKILL.md"), 0o755); err != nil {
			t.Fatalf("MkdirAll source-error-skill/SKILL.md: %v", err)
		}
		if _, err := registry.Source(ctx, []string{"source-error-skill"}); err == nil {
			t.Fatal("SkillRegistry.Source accepted a skill whose frontmatter can no longer be loaded")
		}
	})
}
