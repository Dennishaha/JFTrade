package adk

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	adkagent "google.golang.org/adk/v2/agent"
	adkartifact "google.golang.org/adk/v2/artifact"
	adkmemory "google.golang.org/adk/v2/memory"
	adkmodel "google.golang.org/adk/v2/model"
	adksession "google.golang.org/adk/v2/session"
	adktool "google.golang.org/adk/v2/tool"
	adkskill "google.golang.org/adk/v2/tool/skilltoolset/skill"
	"google.golang.org/adk/v2/tool/toolconfirmation"
	"google.golang.org/genai"
)

func TestGoogleADKToolsetRunsRegisteredToolsAndNormalizesResponses(t *testing.T) {
	ctx := newGoogleADKToolTestContext()
	registry := NewToolRegistry()
	registry.Register(ToolDescriptor{
		Name:         "test.read",
		DisplayName:  "测试读取",
		Description:  "Read a symbol for a delegated ADK agent.",
		Category:     "test",
		Permission:   "read_internal",
		AllowedModes: allPermissionModes(),
		InputSchema: map[string]any{
			"type":                 "object",
			"additionalProperties": false,
			"properties": map[string]any{
				"symbol": map[string]any{"type": "string"},
			},
			"required": []any{"symbol"},
		},
	}, func(_ context.Context, input map[string]any) (any, error) {
		return map[string]any{"success": true, "symbol": strings.TrimSpace(toolStringValue(input, "symbol"))}, nil
	})
	registry.Register(ToolDescriptor{Name: "test.scalar", Description: "Return a scalar", Permission: "read_internal", AllowedModes: allPermissionModes()}, func(context.Context, map[string]any) (any, error) {
		return "ok", nil
	})
	registry.Register(ToolDescriptor{Name: "test.error", Description: "Fail hard", Permission: "read_internal", AllowedModes: allPermissionModes()}, func(context.Context, map[string]any) (any, error) {
		return nil, errors.New("provider failed")
	})
	registry.Register(ToolDescriptor{Name: "test.structured", Description: "Return structured failure", Permission: "read_internal", AllowedModes: allPermissionModes()}, func(context.Context, map[string]any) (any, error) {
		return map[string]any{"success": false, "message": "risk check failed"}, nil
	})
	registry.Register(ToolDescriptor{Name: "test.legacy", Description: "Return legacy failure", Permission: "read_internal", AllowedModes: allPermissionModes()}, func(context.Context, map[string]any) (any, error) {
		return map[string]any{"error": "legacy failed"}, nil
	})
	registry.Register(ToolDescriptor{Name: "test.confirm", Description: "Needs HITL confirmation", Permission: "read_internal", AllowedModes: allPermissionModes()}, func(context.Context, map[string]any) (any, error) {
		return nil, adktool.ErrConfirmationRequired
	})

	runtime := newRuntimeWithRegistry(t, newTestRuntime(t).Store(), registry)
	toolset, err := runtime.googleADKProductToolset(Agent{
		Tools:          []string{"test.read", "test.scalar", "test.error", "test.structured", "test.legacy", "test.confirm"},
		PermissionMode: PermissionModeAll,
	})
	if err != nil {
		t.Fatalf("googleADKProductToolset: %v", err)
	}
	product, ok := toolset.(*googleADKProductToolset)
	if !ok || product == nil || product.Name() != "jftrade-tools" {
		t.Fatalf("product toolset = %#v", toolset)
		return
	}
	tools, err := product.Tools(ctx)
	if err != nil || len(tools) != 6 {
		t.Fatalf("Tools len=%d err=%v", len(tools), err)
	}
	if got, err := (*googleADKProductToolset)(nil).Tools(ctx); err != nil || got != nil {
		t.Fatalf("nil product Tools = %#v err=%v", got, err)
	}

	readTool := googleToolByName(t, tools, "test.read")
	if readTool.Description() != "Read a symbol for a delegated ADK agent." || readTool.IsLongRunning() {
		t.Fatalf("read tool metadata description=%q long=%v", readTool.Description(), readTool.IsLongRunning())
	}
	if descriptor, ok := descriptorFromADKTool(readTool); !ok || descriptor.Name != "test.read" {
		t.Fatalf("descriptorFromADKTool = %+v ok=%v", descriptor, ok)
	}
	if _, ok := descriptorFromADKTool(nil); ok {
		t.Fatal("nil tool should not expose a descriptor")
	}
	declaration := readTool.Declaration()
	if declaration == nil || declaration.Name != "test.read" || declaration.ParametersJsonSchema == nil {
		t.Fatalf("Declaration = %#v", declaration)
	}

	request := &adkmodel.LLMRequest{}
	if err := readTool.ProcessRequest(ctx, request); err != nil {
		t.Fatalf("ProcessRequest read: %v", err)
	}
	if request.Config == nil || len(request.Config.Tools) != 1 || len(request.Config.Tools[0].FunctionDeclarations) != 1 {
		t.Fatalf("request after first ProcessRequest = %#v", request.Config)
	}
	if err := readTool.ProcessRequest(ctx, request); err == nil || !strings.Contains(err.Error(), "duplicate tool") {
		t.Fatalf("duplicate ProcessRequest err = %v", err)
	}
	if err := googleToolByName(t, tools, "test.scalar").ProcessRequest(ctx, request); err != nil {
		t.Fatalf("ProcessRequest scalar: %v", err)
	}
	if len(request.Config.Tools) != 1 || len(request.Config.Tools[0].FunctionDeclarations) != 2 {
		t.Fatalf("request tools after append = %#v", request.Config.Tools)
	}

	output, err := readTool.Run(ctx, map[string]any{"symbol": " AAPL "})
	if err != nil || output["symbol"] != "AAPL" || output["success"] != true {
		t.Fatalf("read Run output=%#v err=%v", output, err)
	}
	output, err = googleToolByName(t, tools, "test.scalar").Run(ctx, map[string]any{})
	if err != nil || output["result"] != "ok" {
		t.Fatalf("scalar Run output=%#v err=%v", output, err)
	}
	if _, err := readTool.Run(ctx, []string{"bad"}); err == nil || !strings.Contains(err.Error(), "unexpected args type") {
		t.Fatalf("invalid Run err = %v", err)
	}
	output, err = googleToolByName(t, tools, "test.error").Run(ctx, map[string]any{})
	if err != nil || output["success"] != false || !strings.Contains(output["message"].(string), "provider failed") {
		t.Fatalf("error Run output=%#v err=%v", output, err)
	}
	output, err = googleToolByName(t, tools, "test.structured").Run(ctx, map[string]any{})
	if err != nil || output["success"] != false || !strings.Contains(output["message"].(string), "risk check failed") {
		t.Fatalf("structured Run output=%#v err=%v", output, err)
	}
	output, err = googleToolByName(t, tools, "test.legacy").Run(ctx, map[string]any{})
	if err != nil || output["success"] != false || !strings.Contains(output["message"].(string), "legacy failed") {
		t.Fatalf("legacy Run output=%#v err=%v", output, err)
	}
	if output, err = googleToolByName(t, tools, "test.confirm").Run(ctx, map[string]any{}); !errors.Is(err, adktool.ErrConfirmationRequired) || output != nil {
		t.Fatalf("confirmation Run output=%#v err=%v", output, err)
	}
}

func TestGoogleADKProductToolsetFunctionToolBoundaries(t *testing.T) {
	ctx := newGoogleADKToolTestContext()
	registry := NewToolRegistry()
	registry.Register(ToolDescriptor{
		Name: "test.strict", Description: "Strict schema", Permission: "read_internal", AllowedModes: allPermissionModes(),
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"limit": map[string]any{"type": "integer", "minimum": 1},
			},
			"required":             []string{"limit"},
			"additionalProperties": false,
		},
	}, func(_ context.Context, input map[string]any) (any, error) {
		return map[string]any{"limit": input["limit"]}, nil
	})
	runtime := newRuntimeWithRegistry(t, newTestRuntime(t).Store(), registry)

	toolset, err := runtime.googleADKProductToolset(Agent{Tools: []string{"test.strict"}, PermissionMode: PermissionModeApproval})
	if err != nil {
		t.Fatalf("googleADKProductToolset: %v", err)
	}
	tools, err := toolset.Tools(ctx)
	if err != nil || len(tools) != 1 {
		t.Fatalf("Tools len=%d err=%v", len(tools), err)
	}
	strict := googleToolByName(t, tools, "test.strict")
	if descriptor, ok := descriptorFromADKTool(strict); !ok || descriptor.Name != "test.strict" {
		t.Fatalf("descriptorFromADKTool(strict) = %+v ok=%v", descriptor, ok)
	}
	output, err := strict.Run(ctx, map[string]any{"limit": "2", "extra": true})
	if err != nil || output["limit"] != "2" {
		t.Fatalf("strict Run output=%#v err=%v, want tolerant product args", output, err)
	}

	wrapped := adktool.WithConfirmation(toolset, false, func(string, any) bool { return false })
	wrappedTools, err := wrapped.Tools(ctx)
	if err != nil || len(wrappedTools) != 1 {
		t.Fatalf("wrapped Tools len=%d err=%v", len(wrappedTools), err)
	}
	execution := &googleADKExecution{descriptors: toolDescriptorIndex([]ToolDescriptor{{Name: "test.strict", Permission: "read_internal"}})}
	if descriptor, ok := execution.descriptorForTool(wrappedTools[0]); !ok || descriptor.Name != "test.strict" {
		t.Fatalf("descriptorForTool(wrapped) = %+v ok=%v", descriptor, ok)
	}

	defaultSchemaTool, err := newGoogleADKTool(ToolDescriptor{Name: "test.default", Description: "Default schema"}, RegisteredTool{
		Handler: func(_ context.Context, input map[string]any) (any, error) {
			return input, nil
		},
	})
	if err != nil {
		t.Fatalf("newGoogleADKTool default schema: %v", err)
	}
	output, err = defaultSchemaTool.Run(ctx, map[string]any{"arbitrary": true})
	if err != nil || output["arbitrary"] != true {
		t.Fatalf("default schema product Run output=%#v err=%v", output, err)
	}

	var nilTool *googleADKTool
	if nilTool.Name() != "" || nilTool.Description() != "" || nilTool.IsLongRunning() || nilTool.Declaration() != nil {
		t.Fatalf("nil googleADKTool metadata name=%q description=%q long=%v declaration=%#v", nilTool.Name(), nilTool.Description(), nilTool.IsLongRunning(), nilTool.Declaration())
	}
	if _, err := nilTool.Run(ctx, map[string]any{}); err == nil || !strings.Contains(err.Error(), "not runnable") {
		t.Fatalf("nil googleADKTool Run err = %v", err)
	}
	uninitialized := &googleADKTool{descriptor: ToolDescriptor{Name: "test.uninitialized", Description: "Uninitialized"}}
	if uninitialized.Name() != "test.uninitialized" || uninitialized.Description() != "Uninitialized" || uninitialized.Declaration() == nil {
		t.Fatalf("uninitialized googleADKTool metadata name=%q description=%q declaration=%#v", uninitialized.Name(), uninitialized.Description(), uninitialized.Declaration())
	}
	if _, err := uninitialized.Run(ctx, map[string]any{}); err == nil || !strings.Contains(err.Error(), "not runnable") {
		t.Fatalf("uninitialized googleADKTool Run err = %v", err)
	}
}

func TestGoogleADKProductToolsetRejectsInvalidFunctionSchema(t *testing.T) {
	registry := NewToolRegistry()
	registry.Register(ToolDescriptor{
		Name: "test.invalid_schema", Description: "Invalid schema", Permission: "read_internal", AllowedModes: allPermissionModes(),
		InputSchema: map[string]any{
			"type": make(chan int),
		},
	}, func(context.Context, map[string]any) (any, error) {
		return map[string]any{"ok": true}, nil
	})
	runtime := newRuntimeWithRegistry(t, newTestRuntime(t).Store(), registry)
	if _, err := runtime.googleADKProductToolset(Agent{Tools: []string{"test.invalid_schema"}, PermissionMode: PermissionModeAll}); err == nil || !strings.Contains(err.Error(), "convert GO-ADK product tool schema") {
		t.Fatalf("googleADKProductToolset invalid schema err = %v", err)
	}
}

func TestGoogleADKProductToolsetEmptySelectionReturnsNil(t *testing.T) {
	runtime := newRuntimeWithRegistry(t, newTestRuntime(t).Store(), NewToolRegistry())
	toolset, err := runtime.googleADKProductToolset(Agent{Tools: []string{"missing.tool"}, PermissionMode: PermissionModeAll})
	if err != nil || toolset != nil {
		t.Fatalf("googleADKProductToolset missing = %#v err=%v, want nil nil", toolset, err)
	}
}

func TestGoogleADKToolResponseErrorHelpers(t *testing.T) {
	if message, failed := structuredToolError(map[string]any{}); failed || message != "" {
		t.Fatalf("empty structuredToolError message=%q failed=%v", message, failed)
	}
	if message, failed := structuredToolError(map[string]any{"success": true, "message": "ok"}); failed || message != "" {
		t.Fatalf("success structuredToolError message=%q failed=%v", message, failed)
	}
	if message, failed := structuredToolError(map[string]any{"success": false}); !failed || message != "tool execution failed" {
		t.Fatalf("missing message structuredToolError message=%q failed=%v", message, failed)
	}
	if message, failed := structuredToolError(map[string]any{"error": "   "}); failed || message != "" {
		t.Fatalf("blank legacy structuredToolError message=%q failed=%v", message, failed)
	}
	if message, failed := structuredToolError(map[string]any{"error": "<nil>"}); failed || message != "" {
		t.Fatalf("nil legacy structuredToolError message=%q failed=%v", message, failed)
	}
	if !isToolResponseError(map[string]any{"success": false}) || !isToolResponseError(map[string]any{"error": "legacy"}) {
		t.Fatal("failure response should be detected")
	}
	if isToolResponseError(map[string]any{"success": true}) || isToolResponseError(nil) {
		t.Fatal("success or nil response should not be detected as failure")
	}
	if got := toolResponseErrorMessage(map[string]any{"message": "  blocked  ", "error": "legacy"}); got != "blocked" {
		t.Fatalf("toolResponseErrorMessage message = %q", got)
	}
	if got := toolResponseErrorMessage(map[string]any{"error": "legacy"}); got != "legacy" {
		t.Fatalf("toolResponseErrorMessage error = %q", got)
	}
	if got := toolResponseErrorMessage(map[string]any{}); got != "tool execution failed" {
		t.Fatalf("toolResponseErrorMessage default = %q", got)
	}
}

func TestGoogleADKSkillFilteringAndToolsetsRespectAgentPermissions(t *testing.T) {
	ctx := context.Background()
	registry := NewToolRegistry()
	registry.Register(ToolDescriptor{
		Name: "test.read", DisplayName: "测试读取", Description: "Read", Permission: "read_internal", AllowedModes: allPermissionModes(),
	}, func(context.Context, map[string]any) (any, error) { return map[string]any{"ok": true}, nil })
	registry.Register(ToolDescriptor{
		Name: "test.trade", Description: "Trade", Permission: "live_trading", AllowedModes: []string{PermissionModeAll},
	}, func(context.Context, map[string]any) (any, error) { return map[string]any{"ok": true}, nil })
	runtime := newRuntimeWithRegistry(t, newTestRuntime(t).Store(), registry)

	source := &googleADKFakeSkillSource{
		frontmatters: []*adkskill.Frontmatter{
			{Name: "allowed-skill", AllowedTools: []string{"test.read"}},
			{Name: "display-name-skill", AllowedTools: []string{"测试读取"}},
			{Name: "open-skill"},
			{Name: "trade-skill", AllowedTools: []string{"test.trade"}},
			{Name: "unknown-tool-skill", AllowedTools: []string{"missing.tool"}},
		},
		instructions: map[string]string{
			"allowed-skill":      "Use the read-only market guide.",
			"display-name-skill": "Display names should resolve to canonical tools.",
			"open-skill":         "No tool restriction required.",
		},
		resources: map[string]map[string]string{
			"allowed-skill": {"references/guide.md": "guide content"},
		},
	}
	filtered, err := runtime.filteredSkillSourceForAgent(ctx, source, Agent{
		Tools:          []string{"test.read"},
		PermissionMode: PermissionModeApproval,
	})
	if err != nil {
		t.Fatalf("filteredSkillSourceForAgent: %v", err)
	}
	frontmatters, err := filtered.ListFrontmatters(ctx)
	if err != nil {
		t.Fatalf("ListFrontmatters: %v", err)
	}
	names := make([]string, 0, len(frontmatters))
	for _, frontmatter := range frontmatters {
		names = append(names, frontmatter.Name)
	}
	for _, want := range []string{"allowed-skill", "display-name-skill", "open-skill"} {
		if !slices.Contains(names, want) {
			t.Fatalf("filtered names = %#v, want %q", names, want)
		}
	}
	for _, denied := range []string{"trade-skill", "unknown-tool-skill"} {
		if slices.Contains(names, denied) {
			t.Fatalf("filtered names = %#v, should not include %q", names, denied)
		}
	}

	typed := filtered.(*agentFilteredSkillSource)
	if !typed.isAllowed(" allowed-skill ") || (*agentFilteredSkillSource)(nil).isAllowed("allowed-skill") {
		t.Fatal("agentFilteredSkillSource.isAllowed did not trim or handle nil correctly")
	}
	instructions, err := typed.LoadInstructions(ctx, "allowed-skill")
	if err != nil || !strings.Contains(instructions, "read-only") {
		t.Fatalf("LoadInstructions = %q err=%v", instructions, err)
	}
	resources, err := typed.ListResources(ctx, "allowed-skill", "references")
	if err != nil || len(resources) != 1 || resources[0] != "references/guide.md" {
		t.Fatalf("ListResources = %#v err=%v", resources, err)
	}
	reader, err := typed.LoadResource(ctx, "allowed-skill", "references/guide.md")
	if err != nil {
		t.Fatalf("LoadResource: %v", err)
	}
	raw, err := io.ReadAll(reader)
	jftradeCheckTestError(t, reader.Close())
	if err != nil || string(raw) != "guide content" {
		t.Fatalf("LoadResource content=%q err=%v", string(raw), err)
	}
	if _, err := typed.LoadFrontmatter(ctx, "trade-skill"); !errors.Is(err, adkskill.ErrSkillNotFound) {
		t.Fatalf("denied LoadFrontmatter err = %v", err)
	}
	if frontmatter, err := typed.LoadFrontmatter(ctx, "allowed-skill"); err != nil || frontmatter.Name != "allowed-skill" {
		t.Fatalf("allowed LoadFrontmatter = %#v err=%v", frontmatter, err)
	}
	if _, err := typed.LoadInstructions(ctx, "trade-skill"); !errors.Is(err, adkskill.ErrSkillNotFound) {
		t.Fatalf("denied LoadInstructions err = %v", err)
	}
	if _, err := typed.ListResources(ctx, "trade-skill", "references"); !errors.Is(err, adkskill.ErrSkillNotFound) {
		t.Fatalf("denied ListResources err = %v", err)
	}
	if _, err := typed.LoadResource(ctx, "trade-skill", "references/guide.md"); !errors.Is(err, adkskill.ErrSkillNotFound) {
		t.Fatalf("denied LoadResource err = %v", err)
	}
	if skillAllowedForAgent(nil, map[string]struct{}{}, registry, PermissionModeApproval) {
		t.Fatal("nil frontmatter should never be allowed")
	}
	if skillAllowedForAgent(&adkskill.Frontmatter{Name: "nil-registry", AllowedTools: []string{"test.read"}}, map[string]struct{}{"test.read": {}}, nil, PermissionModeApproval) {
		t.Fatal("nil registry should reject tool-restricted skills")
	}
	if skillAllowedForAgent(&adkskill.Frontmatter{Name: "missing-canonical", AllowedTools: []string{"missing.tool"}}, map[string]struct{}{}, registry, PermissionModeApproval) {
		t.Fatal("unknown allowed tool should reject skill")
	}
	if skillAllowedForAgent(&adkskill.Frontmatter{Name: "allowed-map-missing", AllowedTools: []string{"test.read"}}, map[string]struct{}{}, registry, PermissionModeApproval) {
		t.Fatal("agent allowed-tools map should gate skills")
	}
	if filtered, err := runtime.filteredSkillSourceForAgent(ctx, nil, Agent{}); err != nil || filtered != nil {
		t.Fatalf("nil source filtered=%#v err=%v", filtered, err)
	}
	source.frontmatterErr = errors.New("frontmatter unavailable")
	if _, err := runtime.filteredSkillSourceForAgent(ctx, source, Agent{}); err == nil || !strings.Contains(err.Error(), "frontmatter unavailable") {
		t.Fatalf("frontmatter error = %v", err)
	}
	filteredErr := &agentFilteredSkillSource{
		base:    &googleADKFakeSkillSource{frontmatterErr: errors.New("filtered list failed")},
		allowed: map[string]struct{}{"allowed-skill": {}},
	}
	if _, err := filteredErr.ListFrontmatters(ctx); err == nil || !strings.Contains(err.Error(), "filtered list failed") {
		t.Fatalf("agentFilteredSkillSource ListFrontmatters err = %v", err)
	}

	skillDir := filepath.Join(runtime.Store().SkillsPath(), "resource-skill")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatalf("MkdirAll skill dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("---\nname: resource-skill\ndescription: Resource skill\nallowed-tools: [test.read]\n---\nUse this resource skill."), 0o644); err != nil {
		t.Fatalf("WriteFile skill: %v", err)
	}
	toolsets, err := runtime.googleADKToolsets(ctx, Agent{
		Tools:          []string{"test.read"},
		Skills:         []string{"resource-skill"},
		PermissionMode: PermissionModeApproval,
	})
	if err != nil {
		t.Fatalf("googleADKToolsets: %v", err)
	}
	if !toolsetsContainTool(t, toolsets, "test.read") {
		t.Fatalf("filtered toolsets did not include test.read")
	}
	if toolsetsContainTool(t, toolsets, "load_artifacts", newGoogleADKToolTestContext()) {
		t.Fatalf("empty artifact toolset should not expose load_artifacts")
	}
}

func TestGoogleADKToolsetsBoundaryErrorsAndStaticToolsets(t *testing.T) {
	ctx := context.Background()
	static := googleADKStaticToolset{name: "static-tools", tools: []adktool.Tool{preloadmemorytoolForBoundary{}}}
	if static.Name() != "static-tools" {
		t.Fatalf("static Name = %q", static.Name())
	}
	staticTools, err := static.Tools(newGoogleADKToolTestContext())
	if err != nil || len(staticTools) != 1 || staticTools[0].Name() != "boundary.preload" {
		t.Fatalf("static Tools = %#v err=%v", staticTools, err)
	}

	artifactSet := googleADKArtifactToolset{name: "artifact-tools"}
	if artifactSet.Name() != "artifact-tools" {
		t.Fatalf("artifact Name = %q", artifactSet.Name())
	}
	if tools, err := artifactSet.Tools(newGoogleADKToolTestContext()); err != nil || tools != nil {
		t.Fatalf("nil artifact service Tools = %#v err=%v, want nil nil", tools, err)
	}
	listErr := errors.New("artifact list failed")
	artifactSet.service = listErrorArtifactService{err: listErr}
	if _, err := artifactSet.Tools(newGoogleADKToolTestContext()); !errors.Is(err, listErr) {
		t.Fatalf("artifact List err = %v, want %v", err, listErr)
	}

	invalidRegistry := NewToolRegistry()
	invalidRegistry.Register(ToolDescriptor{
		Name:         "test.invalid_toolset_schema",
		Description:  "Invalid schema for aggregate toolset",
		Permission:   "read_internal",
		AllowedModes: allPermissionModes(),
		InputSchema:  map[string]any{"type": make(chan int)},
	}, func(context.Context, map[string]any) (any, error) {
		return map[string]any{"ok": true}, nil
	})
	invalidRuntime := newRuntimeWithRegistry(t, newTestRuntime(t).Store(), invalidRegistry)
	if _, err := invalidRuntime.googleADKToolsets(ctx, Agent{Tools: []string{"test.invalid_toolset_schema"}, PermissionMode: PermissionModeAll}); err == nil || !strings.Contains(err.Error(), "convert GO-ADK product tool schema") {
		t.Fatalf("googleADKToolsets invalid schema err = %v", err)
	}

	missingSkillRuntime := newRuntimeWithRegistry(t, newTestRuntime(t).Store(), NewToolRegistry())
	if _, err := missingSkillRuntime.googleADKToolsets(ctx, Agent{Skills: []string{"missing-skill"}}); err == nil || !strings.Contains(err.Error(), "skill not found") {
		t.Fatalf("googleADKToolsets missing skill err = %v", err)
	}
}

func TestGoogleADKToolsetsIncludeADKMemoryToolsWhenEnabled(t *testing.T) {
	ctx := context.Background()
	runtime := newRuntimeWithRegistry(t, newTestRuntime(t).Store(), NewToolRegistry())
	toolsets, err := runtime.googleADKToolsets(ctx, Agent{ID: "memory-agent", MemoryEnabled: true})
	if err != nil {
		t.Fatalf("googleADKToolsets memory enabled: %v", err)
	}
	if !toolsetsContainTool(t, toolsets, "preload_memory") {
		t.Fatalf("memory-enabled toolsets did not include preload_memory")
	}
	if !toolsetsContainTool(t, toolsets, "load_memory") {
		t.Fatalf("memory-enabled toolsets did not include load_memory")
	}

	toolsets, err = runtime.googleADKToolsets(ctx, Agent{ID: "memory-agent", MemoryEnabled: false})
	if err != nil {
		t.Fatalf("googleADKToolsets memory disabled: %v", err)
	}
	if toolsetsContainTool(t, toolsets, "preload_memory") {
		t.Fatalf("memory-disabled toolsets included preload_memory")
	}
	if toolsetsContainTool(t, toolsets, "load_memory") {
		t.Fatalf("memory-disabled toolsets included load_memory")
	}
}

func TestGoogleADKToolsetsIncludeADKArtifactTools(t *testing.T) {
	ctx := context.Background()
	runtime := newRuntimeWithRegistry(t, newTestRuntime(t).Store(), NewToolRegistry())
	toolsets, err := runtime.googleADKToolsets(ctx, Agent{ID: "artifact-agent"})
	if err != nil {
		t.Fatalf("googleADKToolsets artifact: %v", err)
	}
	requestContext := newGoogleADKToolTestContext()
	if toolsetsContainTool(t, toolsets, "load_artifacts", requestContext) {
		t.Fatalf("artifact-empty toolsets included load_artifacts")
	}
	if _, err := runtime.artifactService.Save(ctx, &adkartifact.SaveRequest{
		AppName: requestContext.AppName(), UserID: requestContext.UserID(), SessionID: requestContext.SessionID(), FileName: "report.txt",
		Part: genai.NewPartFromText("report"),
	}); err != nil {
		t.Fatalf("Save artifact: %v", err)
	}
	if !toolsetsContainTool(t, toolsets, "load_artifacts", requestContext) {
		t.Fatalf("toolsets did not include load_artifacts")
	}
}

func toolsetsContainTool(t *testing.T, toolsets []adktool.Toolset, name string, contexts ...adkagent.ReadonlyContext) bool {
	t.Helper()
	var ctx adkagent.ReadonlyContext
	if len(contexts) > 0 {
		ctx = contexts[0]
	}
	for _, toolset := range toolsets {
		tools, err := toolset.Tools(ctx)
		if err != nil {
			t.Fatalf("Tools(%s): %v", toolset.Name(), err)
		}
		for _, tool := range tools {
			if tool.Name() == name {
				return true
			}
		}
	}
	return false
}

func TestWorkflowStoreTriggerDeletionAndLogLookupBoundaries(t *testing.T) {
	ctx := context.Background()
	store := newTestRuntime(t).Store()
	workflow, err := store.SaveWorkflowDefinition(ctx, WorkflowDefinition{
		ID: " workflow-a ", Name: " Morning rebalance ", AgentID: "agent-a", WorkMode: WorkModeLoop, PromptTemplate: "Run rebalance",
	})
	if err != nil {
		t.Fatalf("SaveWorkflowDefinition: %v", err)
	}
	trigger, err := store.SaveWorkflowTrigger(ctx, WorkflowTrigger{
		ID: " trigger-a ", WorkflowID: workflow.ID, Type: " SCHEDULE ", Title: " Opening bell ", NextRunAt: "2026-07-02T09:30:00+08:00",
		Config: map[string]any{"symbol": " AAPL "},
	})
	if err != nil {
		t.Fatalf("SaveWorkflowTrigger: %v", err)
	}
	if trigger.ID != "trigger-a" || trigger.Type != WorkflowTriggerTypeSchedule || trigger.Status != WorkflowTriggerStatusEnabled || trigger.Config["symbol"] != " AAPL " {
		t.Fatalf("normalized trigger = %+v", trigger)
	}
	disabled, err := store.SaveWorkflowTrigger(ctx, WorkflowTrigger{
		ID: "disabled-trigger", WorkflowID: workflow.ID, Type: WorkflowTriggerTypeSchedule, Status: WorkflowTriggerStatusDisabled, NextRunAt: "2026-07-02T09:00:00+08:00",
	})
	if err != nil {
		t.Fatalf("Save disabled trigger: %v", err)
	}
	log, err := store.SaveWorkflowTriggerLog(ctx, WorkflowTriggerLog{
		ID:          " log-a ",
		WorkflowID:  workflow.ID,
		TriggerID:   trigger.ID,
		TriggerType: trigger.Type,
		RunID:       " run-a ",
		SessionID:   " session-a ",
		Inputs:      map[string]any{"symbol": " AAPL "},
		MatchedEvent: map[string]any{
			"type": " market.open ",
		},
		NodeRuns: []WorkflowNodeRun{{
			NodeID: " node-a ", NodeType: "agent", Status: " running ", Inputs: map[string]any{"step": " prepare "},
		}},
	})
	if err != nil {
		t.Fatalf("SaveWorkflowTriggerLog: %v", err)
	}
	if log.ID != "log-a" || log.Status != WorkflowTriggerLogStatusQueued || log.RunID != "run-a" || log.NodeRuns[0].Status != WorkflowTriggerLogStatusRunning {
		t.Fatalf("normalized log = %+v", log)
	}
	if fetched, ok, err := store.WorkflowTriggerLog(ctx, log.ID); err != nil || !ok || fetched.ID != log.ID {
		t.Fatalf("WorkflowTriggerLog fetched=%+v ok=%v err=%v", fetched, ok, err)
	}
	if missing, ok, err := store.WorkflowTriggerLog(ctx, "missing-log"); err != nil || ok || missing.ID != "" {
		t.Fatalf("missing WorkflowTriggerLog fetched=%+v ok=%v err=%v", missing, ok, err)
	}
	if _, err := store.SaveWorkflowTriggerLog(ctx, WorkflowTriggerLog{ID: "running-log", WorkflowID: workflow.ID, TriggerID: trigger.ID, TriggerType: trigger.Type, Status: WorkflowTriggerLogStatusRunning}); err != nil {
		t.Fatalf("Save running log: %v", err)
	}
	if _, err := store.SaveWorkflowTriggerLog(ctx, WorkflowTriggerLog{ID: "done-log", WorkflowID: workflow.ID, TriggerID: trigger.ID, TriggerType: trigger.Type, Status: WorkflowTriggerLogStatusSucceeded}); err != nil {
		t.Fatalf("Save done log: %v", err)
	}
	active, err := store.ListActiveWorkflowTriggerLogs(ctx, trigger.ID)
	if err != nil {
		t.Fatalf("ListActiveWorkflowTriggerLogs: %v", err)
	}
	activeIDs := make([]string, 0, len(active))
	for _, item := range active {
		activeIDs = append(activeIDs, item.ID)
	}
	if !slices.Contains(activeIDs, "log-a") || !slices.Contains(activeIDs, "running-log") || slices.Contains(activeIDs, "done-log") {
		t.Fatalf("active trigger logs = %#v", activeIDs)
	}
	running, total, err := store.ListWorkflowTriggerLogsPage(ctx, workflow.ID, trigger.ID, " running ", 10, 0)
	if err != nil || total != 1 || running[0].ID != "running-log" {
		t.Fatalf("running log page items=%+v total=%d err=%v", running, total, err)
	}

	due, err := store.ListDueWorkflowScheduleTriggers(ctx, "2026-07-02T09:30:00+08:00", 10)
	if err != nil || len(due) != 1 || due[0].ID != trigger.ID {
		t.Fatalf("due triggers = %+v err=%v", due, err)
	}
	if enabled, err := store.ListEnabledWorkflowTriggersByType(ctx, WorkflowTriggerTypeSchedule); err != nil || len(enabled) != 1 || enabled[0].ID != trigger.ID {
		t.Fatalf("enabled triggers = %+v err=%v", enabled, err)
	}

	deleted, err := store.DeleteWorkflowTrigger(ctx, trigger.ID)
	if err != nil || deleted.Status != WorkflowTriggerStatusDisabled || deleted.DeletedAt == nil {
		t.Fatalf("DeleteWorkflowTrigger deleted=%+v err=%v", deleted, err)
	}
	remaining, err := store.ListWorkflowTriggers(ctx, workflow.ID)
	if err != nil {
		t.Fatalf("ListWorkflowTriggers: %v", err)
	}
	for _, item := range remaining {
		if item.ID == trigger.ID {
			t.Fatalf("deleted trigger remained visible: %+v", remaining)
		}
	}
	if !slices.ContainsFunc(remaining, func(item WorkflowTrigger) bool { return item.ID == disabled.ID }) {
		t.Fatalf("non-deleted disabled trigger should remain visible: %+v", remaining)
	}
	if _, err := store.DeleteWorkflowTrigger(ctx, trigger.ID); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("DeleteWorkflowTrigger again err = %v", err)
	}
	if _, err := store.DeleteWorkflowTrigger(ctx, "missing-trigger"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("DeleteWorkflowTrigger missing err = %v", err)
	}
}

func TestWorkflowTaskModelsListUsesRuntimeProviderCatalog(t *testing.T) {
	ctx := context.Background()
	runtime := newTestRuntime(t)
	if _, err := runtime.Store().SaveProvider(ctx, ProviderWriteRequest{
		ID: "disabled-provider", DisplayName: "Disabled Provider", BaseURL: "https://disabled.example/v1", Model: "cold-model", Enabled: false,
	}); err != nil {
		t.Fatalf("SaveProvider disabled: %v", err)
	}
	toolset := &workflowTaskToolset{executor: runtime.workflowExecutor()}
	result, err := toolset.modelsList(map[string]any{"query": "test-model", "limit": 5})
	if err != nil {
		t.Fatalf("modelsList: %v", err)
	}
	models, ok := result["models"].([]map[string]any)
	if !ok || len(models) != 1 || models[0]["providerId"] != testProviderID || models[0]["callable"] != true {
		t.Fatalf("modelsList result = %#v", result)
	}
	if _, leaked := models[0]["apiKey"]; leaked {
		t.Fatalf("modelsList leaked api key: %#v", models[0])
	}
	result, err = toolset.modelsList(map[string]any{"providerId": "disabled-provider", "callableOnly": "false"})
	if err != nil {
		t.Fatalf("modelsList disabled: %v", err)
	}
	models, ok = result["models"].([]map[string]any)
	if !ok || len(models) != 1 || models[0]["providerId"] != "disabled-provider" || models[0]["callable"] != false {
		t.Fatalf("disabled modelsList result = %#v", result)
	}
	if _, err := (*workflowTaskToolset)(nil).modelsList(map[string]any{}); err == nil || !strings.Contains(err.Error(), "runtime is unavailable") {
		t.Fatalf("nil modelsList err = %v", err)
	}
	if _, err := (&workflowTaskToolset{}).modelsList(map[string]any{}); err == nil || !strings.Contains(err.Error(), "runtime is unavailable") {
		t.Fatalf("empty modelsList err = %v", err)
	}
}

func googleToolByName(t *testing.T, tools []adktool.Tool, name string) *googleADKTool {
	t.Helper()
	for _, tool := range tools {
		if tool.Name() == name {
			typed, ok := tool.(*googleADKTool)
			if !ok {
				t.Fatalf("tool %s has type %T", name, tool)
			}
			return typed
		}
	}
	t.Fatalf("tool %s not found in %#v", name, tools)
	return nil
}

type googleADKToolTestContext struct {
	*adkagent.StrictContextMock
}

func newGoogleADKToolTestContext() googleADKToolTestContext {
	mock := adkagent.NewStrictContextMock(context.Background())
	return googleADKToolTestContext{StrictContextMock: &mock}
}

func (c googleADKToolTestContext) Agent() adkagent.Agent { return nil }
func (c googleADKToolTestContext) Artifacts() adkagent.Artifacts {
	return nil
}
func (c googleADKToolTestContext) Memory() adkagent.Memory { return nil }
func (c googleADKToolTestContext) Session() adksession.Session {
	return nil
}
func (c googleADKToolTestContext) InvocationID() string           { return "invocation-test" }
func (c googleADKToolTestContext) Branch() string                 { return "" }
func (c googleADKToolTestContext) IsolationScope() string         { return "" }
func (c googleADKToolTestContext) UserContent() *genai.Content    { return nil }
func (c googleADKToolTestContext) RunConfig() *adkagent.RunConfig { return nil }
func (c googleADKToolTestContext) EndInvocation()                 {}
func (c googleADKToolTestContext) Ended() bool                    { return false }
func (c googleADKToolTestContext) ResumedInput(string) (any, bool) {
	return nil, false
}
func (c googleADKToolTestContext) AgentName() string { return "agent-test" }
func (c googleADKToolTestContext) ReadonlyState() adksession.ReadonlyState {
	return nil
}
func (c googleADKToolTestContext) UserID() string          { return "user-test" }
func (c googleADKToolTestContext) AppName() string         { return "jftrade-test" }
func (c googleADKToolTestContext) SessionID() string       { return "session-test" }
func (c googleADKToolTestContext) State() adksession.State { return nil }
func (c googleADKToolTestContext) FunctionCallID() string  { return "function-call-test" }
func (c googleADKToolTestContext) Actions() *adksession.EventActions {
	return &adksession.EventActions{}
}
func (c googleADKToolTestContext) SearchMemory(context.Context, string) (*adkmemory.SearchResponse, error) {
	return nil, nil
}
func (c googleADKToolTestContext) ToolConfirmation() *toolconfirmation.ToolConfirmation {
	return nil
}
func (c googleADKToolTestContext) RequestConfirmation(string, any) error { return nil }
func (c googleADKToolTestContext) WithContext(ctx context.Context) adkagent.InvocationContext {
	mock := adkagent.NewStrictContextMock(ctx)
	c.StrictContextMock = &mock
	return c
}
func (c googleADKToolTestContext) WithICDelta(*adkagent.InvocationContextDelta) adkagent.InvocationContext {
	return c
}
func (c googleADKToolTestContext) WithDelta(*adkagent.CommonContextDelta) adkagent.Context {
	return c
}
func (c googleADKToolTestContext) Path() string  { return "" }
func (c googleADKToolTestContext) RunID() string { return "" }
func (c googleADKToolTestContext) WithBranch(string) adkagent.Context {
	return c
}
func (c googleADKToolTestContext) SubScheduler() adkagent.DynamicSubScheduler {
	return nil
}
func (c googleADKToolTestContext) InvocationContext() adkagent.InvocationContext {
	return c
}
func (c googleADKToolTestContext) SetInvocationContext(adkagent.InvocationContext) {}
func (c googleADKToolTestContext) WithAgentContext(ctx context.Context) adkagent.Context {
	mock := adkagent.NewStrictContextMock(ctx)
	c.StrictContextMock = &mock
	return c
}
func (c googleADKToolTestContext) WithAgentTimeout(timeout time.Duration) (adkagent.Context, context.CancelFunc) {
	ctx, cancel := context.WithTimeout(c.Ctx, timeout)
	return c.WithAgentContext(ctx), cancel
}
func (c googleADKToolTestContext) WithAgentCancel() (adkagent.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(c.Ctx)
	return c.WithAgentContext(ctx), cancel
}
func (c googleADKToolTestContext) OutputForAncestors() []string { return nil }

type googleADKFakeSkillSource struct {
	frontmatters   []*adkskill.Frontmatter
	frontmatterErr error
	instructions   map[string]string
	resources      map[string]map[string]string
}

type preloadmemorytoolForBoundary struct{}

func (preloadmemorytoolForBoundary) Name() string        { return "boundary.preload" }
func (preloadmemorytoolForBoundary) Description() string { return "boundary preload" }
func (preloadmemorytoolForBoundary) IsLongRunning() bool { return false }

type listErrorArtifactService struct {
	adkartifact.Service
	err error
}

func (service listErrorArtifactService) List(context.Context, *adkartifact.ListRequest) (*adkartifact.ListResponse, error) {
	return nil, service.err
}

func (s *googleADKFakeSkillSource) ListFrontmatters(context.Context) ([]*adkskill.Frontmatter, error) {
	if s.frontmatterErr != nil {
		return nil, s.frontmatterErr
	}
	return s.frontmatters, nil
}

func (s *googleADKFakeSkillSource) ListResources(_ context.Context, name, subpath string) ([]string, error) {
	items := s.resources[name]
	if items == nil {
		return nil, adkskill.ErrSkillNotFound
	}
	out := make([]string, 0, len(items))
	for path := range items {
		if subpath == "" || strings.HasPrefix(path, strings.TrimRight(subpath, "/")+"/") {
			out = append(out, path)
		}
	}
	slices.Sort(out)
	return out, nil
}

func (s *googleADKFakeSkillSource) LoadFrontmatter(_ context.Context, name string) (*adkskill.Frontmatter, error) {
	for _, item := range s.frontmatters {
		if item.Name == name {
			return item, nil
		}
	}
	return nil, adkskill.ErrSkillNotFound
}

func (s *googleADKFakeSkillSource) LoadInstructions(_ context.Context, name string) (string, error) {
	if text, ok := s.instructions[name]; ok {
		return text, nil
	}
	return "", adkskill.ErrSkillNotFound
}

func (s *googleADKFakeSkillSource) LoadResource(_ context.Context, name, resourcePath string) (io.ReadCloser, error) {
	if items := s.resources[name]; items != nil {
		if content, ok := items[resourcePath]; ok {
			return io.NopCloser(bytes.NewBufferString(content)), nil
		}
	}
	return nil, adkskill.ErrResourceNotFound
}
