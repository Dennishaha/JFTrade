package adk

import (
	"context"
	"testing"
)

func TestCoverage98ToolInvocationMetadataPreservesSessionAndSkillAccess(t *testing.T) {
	if sessionID, ok := ToolInvocationSessionID(context.Background()); ok || sessionID != "" {
		t.Fatalf("ordinary invocation session = %q/%v", sessionID, ok)
	}
	if sessionID, ok := ToolInvocationSessionID(toolSessionTestContext{Context: context.Background(), sessionID: "  session-direct  "}); !ok || sessionID != "session-direct" {
		t.Fatalf("direct invocation session = %q/%v", sessionID, ok)
	}
	if metadata := contextWithToolInvocationMetadata(context.Background()); metadata == nil {
		t.Fatal("ordinary invocation context did not receive metadata")
	}

	state := newSkillActivationTestState()
	if err := activateSkill(state, "metadata-agent", "market-analysis"); err != nil {
		t.Fatalf("activate skill: %v", err)
	}
	source := newSkillGateTestContext(state, "metadata-agent")
	metadata := contextWithToolInvocationMetadata(source)
	if sessionID, ok := ToolInvocationSessionID(metadata); !ok || sessionID != "session-test" {
		t.Fatalf("copied invocation session = %q/%v", sessionID, ok)
	}
	if !ToolInvocationSkillActive(metadata, "market-analysis") {
		t.Fatal("metadata-wrapped invocation lost its active skill")
	}
	if !ToolInvocationSkillActive(source, "market-analysis") {
		t.Fatal("direct invocation context did not expose its active skill")
	}
	if ToolInvocationSkillActive(metadata, "missing-skill") {
		t.Fatal("unknown skill was reported active")
	}
}

func TestCoverage98ToolsSearchUsesSkillActivationAndReturnsCompanionTools(t *testing.T) {
	registry := NewToolRegistry()
	for _, descriptor := range []ToolDescriptor{
		{Name: "coverage.gated-report", DisplayName: "Coverage Gated Report", Description: "Visible only after market analysis skill activation.", Category: "analysis", Permission: "read_internal", RequiredSkill: "market-analysis", RequiredSkills: []string{"market-analysis", "alternate-analysis"}},
		{Name: "strategy.optimize", DisplayName: "Optimize", Category: "strategy", Permission: "optimize_strategy"},
		{Name: "backtest.kline_sync_status", DisplayName: "Kline sync", Category: "backtest", Permission: "read_internal"},
	} {
		registry.Register(descriptor, func(context.Context, map[string]any) (any, error) { return map[string]any{"ok": true}, nil })
	}

	search, ok := registry.Get("tools.search")
	if !ok {
		t.Fatal("tools.search is not registered")
	}
	state := newSkillActivationTestState()
	invocation := newSkillGateTestContext(state, "search-agent")
	output, err := search.Handler(invocation, map[string]any{"query": "gated report", "limit": 1})
	if err != nil {
		t.Fatalf("search without skill: %v", err)
	}
	if got := output.(map[string]any)["totalReturned"]; got != 0 {
		t.Fatalf("gated search before activation = %#v", output)
	}

	if err := activateSkill(state, "search-agent", "market-analysis"); err != nil {
		t.Fatalf("activate search skill: %v", err)
	}
	output, err = search.Handler(contextWithToolInvocationMetadata(invocation), map[string]any{"query": "gated report", "category": "analysis", "limit": 1})
	if err != nil {
		t.Fatalf("search with skill: %v", err)
	}
	result := output.(map[string]any)
	tools, ok := result["tools"].([]map[string]any)
	if !ok || len(tools) != 1 || tools[0]["name"] != "coverage.gated-report" || tools[0]["requiredSkill"] != "market-analysis" {
		t.Fatalf("gated search result = %#v", output)
	}
	if required, ok := tools[0]["requiredSkills"].([]string); !ok || len(required) != 2 {
		t.Fatalf("gated required skills = %#v", tools[0]["requiredSkills"])
	}

	if name, ok := registry.CanonicalName("   "); ok || name != "" {
		t.Fatalf("blank canonical name = %q/%v", name, ok)
	}
	descriptors := ToolDescriptorsForAgent(Agent{Tools: []string{"strategy.optimize"}}, registry)
	seen := map[string]bool{}
	for _, descriptor := range descriptors {
		seen[descriptor.Name] = true
	}
	if !seen["strategy.optimize"] || !seen["backtest.kline_sync_status"] {
		t.Fatalf("optimization tool set omitted its K-line readiness companion: %#v", seen)
	}
}
