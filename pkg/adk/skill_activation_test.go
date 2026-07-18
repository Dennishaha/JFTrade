package adk

import (
	"context"
	"errors"
	"iter"
	"strings"
	"testing"

	adkmodel "google.golang.org/adk/v2/model"
	adksession "google.golang.org/adk/v2/session"
	adktool "google.golang.org/adk/v2/tool"
	adkskill "google.golang.org/adk/v2/tool/skilltoolset/skill"
	"google.golang.org/adk/v2/tool/toolconfirmation"

	strategypinespec "github.com/jftrade/jftrade-main/pkg/strategy/pinespec"
)

func TestSkillGatedToolIsExposedOnlyAfterLoadInCurrentInvocation(t *testing.T) {
	registered := RegisteredTool{
		Descriptor: ToolDescriptor{
			Name: "workflows.get", Description: "read workflow", RequiredSkill: WorkflowManagementSkillName,
		},
		Handler: func(_ context.Context, _ map[string]any) (any, error) {
			return map[string]any{"ok": true}, nil
		},
	}
	productTool, err := newGoogleADKTool(registered.Descriptor, registered)
	if err != nil {
		t.Fatalf("newGoogleADKTool: %v", err)
	}
	base := &googleADKProductToolset{name: "test", tools: []adktool.Tool{productTool}}
	confirmed := adktool.WithConfirmation(base, false, nil)
	toolset := newGoogleADKSkillGatedToolset(confirmed, []ToolDescriptor{registered.Descriptor})
	state := newSkillActivationTestState()
	ctx := newSkillGateTestContext(state, "agent-a")
	tools, err := toolset.Tools(ctx)
	if err != nil || len(tools) != 1 {
		t.Fatalf("gated tools = %#v, %v", tools, err)
	}
	gated := tools[0].(*googleADKSkillGatedTool)

	request := &adkmodel.LLMRequest{}
	if err := gated.ProcessRequest(ctx, request); err != nil {
		t.Fatalf("ProcessRequest before activation: %v", err)
	}
	if len(request.Tools) != 0 {
		t.Fatalf("tools before activation = %#v, want none", request.Tools)
	}
	if _, err := gated.Run(ctx, map[string]any{}); err == nil || !strings.Contains(err.Error(), "requires loading skill") {
		t.Fatalf("Run before activation err = %v", err)
	}

	if err := activateSkill(state, ctx.AgentName(), WorkflowManagementSkillName); err != nil {
		t.Fatalf("activateSkill: %v", err)
	}
	request = &adkmodel.LLMRequest{}
	if err := gated.ProcessRequest(ctx, request); err != nil {
		t.Fatalf("ProcessRequest after activation: %v", err)
	}
	if _, ok := request.Tools[registered.Descriptor.Name]; !ok {
		t.Fatalf("tools after activation = %#v", request.Tools)
	}

	otherAgent := newSkillGateTestContext(state, "agent-b")
	request = &adkmodel.LLMRequest{}
	if err := gated.ProcessRequest(otherAgent, request); err != nil || len(request.Tools) != 0 {
		t.Fatalf("different agent ProcessRequest = %#v, %v", request.Tools, err)
	}
	newInvocation := newSkillGateTestContext(newSkillActivationTestState(), "agent-a")
	request = &adkmodel.LLMRequest{}
	if err := gated.ProcessRequest(newInvocation, request); err != nil || len(request.Tools) != 0 {
		t.Fatalf("new invocation ProcessRequest = %#v, %v", request.Tools, err)
	}
}

func TestSkillGatedToolAllowsConfirmedResumeWithoutTempActivation(t *testing.T) {
	called := false
	registered := RegisteredTool{
		Descriptor: ToolDescriptor{Name: "workflows.delete", Description: "delete", RequiredSkill: WorkflowManagementSkillName},
		Handler: func(_ context.Context, _ map[string]any) (any, error) {
			called = true
			return map[string]any{"deleted": true}, nil
		},
	}
	productTool, err := newGoogleADKTool(registered.Descriptor, registered)
	if err != nil {
		t.Fatalf("newGoogleADKTool: %v", err)
	}
	base := &googleADKProductToolset{name: "test", tools: []adktool.Tool{productTool}}
	confirmed := adktool.WithConfirmation(base, true, nil)
	toolset := newGoogleADKSkillGatedToolset(confirmed, []ToolDescriptor{registered.Descriptor})
	ctx := newSkillGateTestContext(newSkillActivationTestState(), "agent-a")
	ctx.confirmation = &toolconfirmation.ToolConfirmation{Confirmed: true}
	tools, err := toolset.Tools(ctx)
	if err != nil {
		t.Fatalf("Tools: %v", err)
	}
	if _, err := tools[0].(*googleADKSkillGatedTool).Run(ctx, map[string]any{}); err != nil {
		t.Fatalf("confirmed resume: %v", err)
	}
	if !called {
		t.Fatal("confirmed resume did not execute underlying tool")
	}
}

func TestMultiSkillGatedToolIsExposedAfterEitherLoadInCurrentInvocation(t *testing.T) {
	registered := RegisteredTool{
		Descriptor: ToolDescriptor{
			Name: "strategy.validate_pine", Description: "validate Pine", RequiredSkills: []string{
				strategypinespec.ResearchBuiltinSkillName,
				strategypinespec.PublishBuiltinSkillName,
			},
		},
		Handler: func(_ context.Context, _ map[string]any) (any, error) {
			return map[string]any{"ok": true}, nil
		},
	}
	productTool, err := newGoogleADKTool(registered.Descriptor, registered)
	if err != nil {
		t.Fatalf("newGoogleADKTool: %v", err)
	}
	base := &googleADKProductToolset{name: "test", tools: []adktool.Tool{productTool}}
	toolset := newGoogleADKSkillGatedToolset(adktool.WithConfirmation(base, false, nil), []ToolDescriptor{registered.Descriptor})

	inactive := newSkillGateTestContext(newSkillActivationTestState(), "agent-a")
	tools, err := toolset.Tools(inactive)
	if err != nil || len(tools) != 1 {
		t.Fatalf("inactive gated tools = %#v, %v", tools, err)
	}
	inactiveTool := tools[0].(*googleADKSkillGatedTool)
	request := &adkmodel.LLMRequest{}
	if err := inactiveTool.ProcessRequest(inactive, request); err != nil {
		t.Fatalf("ProcessRequest before activation: %v", err)
	}
	if len(request.Tools) != 0 {
		t.Fatalf("tools before activation = %#v, want none", request.Tools)
	}
	if _, err := inactiveTool.Run(inactive, map[string]any{}); err == nil || !strings.Contains(err.Error(), "one of skills") {
		t.Fatalf("Run before activation err = %v", err)
	}

	for _, skillName := range []string{strategypinespec.ResearchBuiltinSkillName, strategypinespec.PublishBuiltinSkillName} {
		state := newSkillActivationTestState()
		ctx := newSkillGateTestContext(state, "agent-a")
		if err := activateSkill(state, ctx.AgentName(), skillName); err != nil {
			t.Fatalf("activateSkill(%q): %v", skillName, err)
		}
		tools, err := toolset.Tools(ctx)
		if err != nil || len(tools) != 1 {
			t.Fatalf("gated tools after %q = %#v, %v", skillName, tools, err)
		}
		request := &adkmodel.LLMRequest{}
		if err := tools[0].(*googleADKSkillGatedTool).ProcessRequest(ctx, request); err != nil {
			t.Fatalf("ProcessRequest after %q: %v", skillName, err)
		}
		if _, ok := request.Tools[registered.Descriptor.Name]; !ok {
			t.Fatalf("tools after %q = %#v", skillName, request.Tools)
		}
	}
}

func TestMultiSkillGatedToolRunsAfterEitherSkillActivation(t *testing.T) {
	called := 0
	registered := RegisteredTool{
		Descriptor: ToolDescriptor{
			Name: "strategy.validate_pine", RequiredSkills: []string{
				strategypinespec.ResearchBuiltinSkillName,
				strategypinespec.PublishBuiltinSkillName,
			},
		},
		Handler: func(_ context.Context, _ map[string]any) (any, error) {
			called++
			return map[string]any{"ok": true}, nil
		},
	}
	productTool, err := newGoogleADKTool(registered.Descriptor, registered)
	if err != nil {
		t.Fatalf("newGoogleADKTool: %v", err)
	}
	toolset := newGoogleADKSkillGatedToolset(
		adktool.WithConfirmation(&googleADKProductToolset{name: "test", tools: []adktool.Tool{productTool}}, false, nil),
		[]ToolDescriptor{registered.Descriptor},
	)

	for _, skillName := range []string{strategypinespec.ResearchBuiltinSkillName, strategypinespec.PublishBuiltinSkillName} {
		state := newSkillActivationTestState()
		ctx := newSkillGateTestContext(state, "agent-a")
		if err := activateSkill(state, ctx.AgentName(), skillName); err != nil {
			t.Fatalf("activateSkill(%q): %v", skillName, err)
		}
		tools, err := toolset.Tools(ctx)
		if err != nil || len(tools) != 1 {
			t.Fatalf("gated tools after %q = %#v, %v", skillName, tools, err)
		}
		result, err := tools[0].(*googleADKSkillGatedTool).Run(ctx, map[string]any{})
		if err != nil {
			t.Fatalf("Run after %q: %v", skillName, err)
		}
		if result["ok"] != true {
			t.Fatalf("Run after %q = %#v, want successful result", skillName, result)
		}
	}
	if called != 2 {
		t.Fatalf("underlying tool called %d times, want 2", called)
	}
}

func TestSkillGatedToolsetPreservesUngatedToolsAndRejectsNonRunnableGatedTools(t *testing.T) {
	registered := RegisteredTool{
		Descriptor: ToolDescriptor{Name: "workflows.get", RequiredSkill: WorkflowManagementSkillName},
		Handler:    func(context.Context, map[string]any) (any, error) { return nil, nil },
	}
	productTool, err := newGoogleADKTool(registered.Descriptor, registered)
	if err != nil {
		t.Fatalf("newGoogleADKTool: %v", err)
	}
	ungated := boundaryGoogleTool{name: "workflow.wait"}
	toolset := newGoogleADKSkillGatedToolset(
		&googleADKProductToolset{name: "test", tools: []adktool.Tool{ungated, productTool}},
		[]ToolDescriptor{registered.Descriptor},
	)
	tools, err := toolset.Tools(newSkillGateTestContext(newSkillActivationTestState(), "agent-a"))
	if err != nil || len(tools) != 2 {
		t.Fatalf("mixed tools = %#v, %v", tools, err)
	}
	if tools[0] != ungated {
		t.Fatalf("ungated tool = %#v, want original tool", tools[0])
	}
	if _, ok := tools[1].(*googleADKSkillGatedTool); !ok {
		t.Fatalf("gated tool type = %T, want googleADKSkillGatedTool", tools[1])
	}

	nonRunnable := newGoogleADKSkillGatedToolset(
		&googleADKProductToolset{name: "test", tools: []adktool.Tool{boundaryGoogleTool{name: registered.Descriptor.Name}}},
		[]ToolDescriptor{registered.Descriptor},
	)
	if _, err := nonRunnable.Tools(newSkillGateTestContext(newSkillActivationTestState(), "agent-a")); err == nil || !strings.Contains(err.Error(), "is not runnable") {
		t.Fatalf("non-runnable gated tool error = %v", err)
	}
}

func TestSkillActivationAndRequiredSkillMetadataBoundaries(t *testing.T) {
	if err := activateSkill(nil, "agent-a", "skill-a"); err == nil {
		t.Fatal("activateSkill(nil) error = nil")
	}
	state := newSkillActivationTestState()
	if err := activateSkill(state, "  agent-a  ", "  skill-a  "); err != nil {
		t.Fatalf("activateSkill with surrounding whitespace: %v", err)
	}
	if !skillActiveInState(state, "agent-a", "skill-a") {
		t.Fatal("trimmed skill activation was not readable")
	}
	if err := activateSkill(state, "", "skill-a"); err == nil {
		t.Fatal("activateSkill with empty agent error = nil")
	}
	if err := activateSkill(state, "agent-a", ""); err == nil {
		t.Fatal("activateSkill with empty skill error = nil")
	}
	state.values[skillActivationStateKey("agent-a", "wrong-type")] = "true"
	if skillActiveInState(state, "agent-a", "wrong-type") {
		t.Fatal("non-boolean skill activation should be inactive")
	}
	if skillActiveInState(nil, "agent-a", "skill-a") || ToolInvocationAnySkillActive(context.TODO(), []string{"skill-a"}) {
		t.Fatal("nil state/context should not report an active skill")
	}

	descriptor := ToolDescriptor{
		RequiredSkill:  " legacy-skill ",
		RequiredSkills: []string{"", "new-skill", " legacy-skill ", "new-skill"},
	}
	if got := ToolRequiredSkillNames(descriptor); !sameStringSet(got, []string{"legacy-skill", "new-skill"}) {
		t.Fatalf("ToolRequiredSkillNames = %v", got)
	}
	registry := NewToolRegistry()
	registry.Register(ToolDescriptor{
		Name: "test.multi-skill", RequiredSkill: " legacy-skill ",
		RequiredSkills: []string{" new-skill ", "legacy-skill", ""},
	}, func(context.Context, map[string]any) (any, error) { return nil, nil })
	registered, ok := registry.Get("test.multi-skill")
	if !ok {
		t.Fatal("normalized multi-skill tool was not registered")
	}
	if registered.Descriptor.RequiredSkill != "legacy-skill" || !sameStringSet(registered.Descriptor.RequiredSkills, []string{"legacy-skill", "new-skill"}) {
		t.Fatalf("registered required skills = %+v", registered.Descriptor)
	}
}

func TestLoadSkillCallbackActivatesToolsSearchInSameInvocation(t *testing.T) {
	state := newSkillActivationTestState()
	ctx := newSkillGateTestContext(state, "agent-a")
	execution := &googleADKExecution{}
	if result, err := execution.afterToolCallback(ctx, boundaryGoogleTool{name: "load_skill"}, nil, map[string]any{
		"skill_name": WorkflowManagementSkillName,
	}, nil); result != nil || err != nil {
		t.Fatalf("afterToolCallback load_skill = %#v, %v", result, err)
	}
	if !skillActiveInState(state, ctx.AgentName(), WorkflowManagementSkillName) {
		t.Fatal("load_skill callback did not activate skill")
	}

	registry := NewToolRegistry()
	registry.Register(ToolDescriptor{
		Name: "workflows.get", DisplayName: "读取工作流", Description: "读取工作流",
		Category: "workflow", Permission: "read_internal", RequiredSkill: WorkflowManagementSkillName,
	}, func(_ context.Context, _ map[string]any) (any, error) { return nil, nil })
	search, _ := registry.Get("tools.search")
	output, err := executeRegisteredTool(ctx, search, map[string]any{"query": "workflows.get"})
	if err != nil {
		t.Fatalf("tools.search: %v", err)
	}
	items := output.(map[string]any)["tools"].([]map[string]any)
	if len(items) != 1 || items[0]["name"] != "workflows.get" || items[0]["requiredSkill"] != WorkflowManagementSkillName {
		t.Fatalf("tools.search output = %#v", output)
	}

	inactive := newSkillGateTestContext(newSkillActivationTestState(), "agent-a")
	output, err = executeRegisteredTool(inactive, search, map[string]any{"query": "workflows.get"})
	if err != nil || len(output.(map[string]any)["tools"].([]map[string]any)) != 0 {
		t.Fatalf("inactive tools.search = %#v, %v", output, err)
	}
}

func TestToolsSearchExposesMultiSkillGatedToolAfterEitherLoad(t *testing.T) {
	registry := NewToolRegistry()
	registry.Register(ToolDescriptor{
		Name: "strategy.validate_pine", DisplayName: "校验 Pine", Description: "校验策略", Category: "strategy", Permission: "read_internal",
		RequiredSkills: []string{strategypinespec.ResearchBuiltinSkillName, strategypinespec.PublishBuiltinSkillName},
	}, func(_ context.Context, _ map[string]any) (any, error) { return nil, nil })
	search, _ := registry.Get("tools.search")

	for _, skillName := range []string{strategypinespec.ResearchBuiltinSkillName, strategypinespec.PublishBuiltinSkillName} {
		state := newSkillActivationTestState()
		ctx := newSkillGateTestContext(state, "agent-a")
		output, err := executeRegisteredTool(ctx, search, map[string]any{"query": "strategy.validate_pine"})
		if err != nil {
			t.Fatalf("tools.search before %q: %v", skillName, err)
		}
		if items := output.(map[string]any)["tools"].([]map[string]any); len(items) != 0 {
			t.Fatalf("inactive tools.search before %q = %#v", skillName, items)
		}
		if err := activateSkill(state, ctx.AgentName(), skillName); err != nil {
			t.Fatalf("activateSkill(%q): %v", skillName, err)
		}
		output, err = executeRegisteredTool(ctx, search, map[string]any{"query": "strategy.validate_pine"})
		if err != nil {
			t.Fatalf("tools.search after %q: %v", skillName, err)
		}
		items := output.(map[string]any)["tools"].([]map[string]any)
		if len(items) != 1 || items[0]["name"] != "strategy.validate_pine" {
			t.Fatalf("active tools.search after %q = %#v", skillName, items)
		}
		if got := items[0]["requiredSkills"]; !sameStringSet(got.([]string), []string{strategypinespec.ResearchBuiltinSkillName, strategypinespec.PublishBuiltinSkillName}) {
			t.Fatalf("required skills after %q = %#v", skillName, got)
		}
	}
}

func TestWorkflowManagementSkillSupportsAuthorizedSubsetAndExactBuiltinTools(t *testing.T) {
	registry := NewToolRegistry()
	for _, name := range WorkflowManagementToolNames() {
		registry.Register(ToolDescriptor{Name: name, Permission: "read_internal"}, func(_ context.Context, _ map[string]any) (any, error) {
			return nil, nil
		})
	}
	frontmatter := &adkskill.Frontmatter{Name: WorkflowManagementSkillName, AllowedTools: WorkflowManagementToolNames()}
	if !skillAllowedForAgent(frontmatter, map[string]struct{}{"workflows.get": {}}, registry, PermissionModeApproval) {
		t.Fatal("workflow skill should be available for an authorized tool subset")
	}
	if skillAllowedForAgent(frontmatter, map[string]struct{}{"tools.search": {}}, registry, PermissionModeApproval) {
		t.Fatal("workflow skill should be hidden without an authorized workflow tool")
	}

	skills := NewSkillRegistry(t.TempDir())
	skill, ok, err := skills.Get(t.Context(), WorkflowManagementSkillName)
	if err != nil || !ok {
		t.Fatalf("workflow builtin skill = %+v, %v, found=%v", skill, err, ok)
	}
	if !sameStringSet(skill.Tools, WorkflowManagementToolNames()) {
		t.Fatalf("workflow builtin allowed tools = %v, want %v", skill.Tools, WorkflowManagementToolNames())
	}
}

func TestStrategySkillsSupportAuthorizedToolSubset(t *testing.T) {
	registry := NewToolRegistry()
	for _, name := range append(strategypinespec.ResearchSkillAllowedTools(), strategypinespec.PublishSkillAllowedTools()...) {
		registry.Register(ToolDescriptor{Name: name, Permission: "read_internal"}, func(_ context.Context, _ map[string]any) (any, error) {
			return nil, nil
		})
	}
	cases := []struct {
		name         string
		allowedTools []string
		authorized   string
	}{
		{name: strategypinespec.ResearchBuiltinSkillName, allowedTools: strategypinespec.ResearchSkillAllowedTools(), authorized: "strategy.research_backtest"},
		{name: strategypinespec.PublishBuiltinSkillName, allowedTools: strategypinespec.PublishSkillAllowedTools(), authorized: "strategy.save_definition"},
	}
	for _, tc := range cases {
		frontmatter := &adkskill.Frontmatter{Name: tc.name, AllowedTools: tc.allowedTools}
		if !skillAllowedForAgent(frontmatter, map[string]struct{}{tc.authorized: {}}, registry, PermissionModeApproval) {
			t.Fatalf("skill %q should support authorized tool subset", tc.name)
		}
		if skillAllowedForAgent(frontmatter, map[string]struct{}{}, registry, PermissionModeApproval) {
			t.Fatalf("skill %q should require at least one authorized tool", tc.name)
		}
	}
	if builtinSkillAllowsAuthorizedToolSubset(" custom-skill ") {
		t.Fatal("custom skill should not use builtin authorized-subset rules")
	}
	if builtinSkillAllowedForAgentSubset(&adkskill.Frontmatter{Name: strategypinespec.ResearchBuiltinSkillName}, nil, nil, PermissionModeApproval) {
		t.Fatal("builtin skill with nil registry should not be allowed")
	}
	if builtinSkillAllowedForAgentSubset(&adkskill.Frontmatter{
		Name: strategypinespec.ResearchBuiltinSkillName, AllowedTools: []string{"missing.tool"},
	}, map[string]struct{}{}, registry, PermissionModeApproval) {
		t.Fatal("builtin skill with unknown tools should not be allowed")
	}
	registry.Register(ToolDescriptor{Name: "test.live", Permission: "live_trading"}, func(context.Context, map[string]any) (any, error) {
		return nil, nil
	})
	if !builtinSkillAllowedForAgentSubset(&adkskill.Frontmatter{
		Name: strategypinespec.ResearchBuiltinSkillName, AllowedTools: []string{"test.live"},
	}, map[string]struct{}{"test.live": {}}, registry, PermissionModeApproval) {
		t.Fatal("live trading tool should remain available behind per-call approval")
	}
}

func TestLoadSkillActivationFailureIsReturned(t *testing.T) {
	failedLoadState := newSkillActivationTestState()
	failedLoadCtx := newSkillGateTestContext(failedLoadState, "agent-a")
	if result, callbackErr := (&googleADKExecution{}).afterToolCallback(
		failedLoadCtx,
		boundaryGoogleTool{name: "load_skill"},
		nil,
		map[string]any{"skill_name": WorkflowManagementSkillName},
		errors.New("skill not found"),
	); result != nil || callbackErr != nil {
		t.Fatalf("failed load callback = %#v, %v", result, callbackErr)
	}
	if skillActiveInState(failedLoadState, failedLoadCtx.AgentName(), WorkflowManagementSkillName) {
		t.Fatal("failed load_skill activated the skill")
	}

	state := newSkillActivationTestState()
	state.setErr = errors.New("state unavailable")
	ctx := newSkillGateTestContext(state, "agent-a")
	_, err := (&googleADKExecution{}).afterToolCallback(ctx, boundaryGoogleTool{name: "load_skill"}, nil, map[string]any{
		"skill_name": WorkflowManagementSkillName,
	}, nil)
	if err == nil || !strings.Contains(err.Error(), "state unavailable") {
		t.Fatalf("activation error = %v", err)
	}
}

type skillActivationTestState struct {
	values map[string]any
	setErr error
}

func newSkillActivationTestState() *skillActivationTestState {
	return &skillActivationTestState{values: make(map[string]any)}
}

func (s *skillActivationTestState) Get(key string) (any, error) {
	value, ok := s.values[key]
	if !ok {
		return nil, adksession.ErrStateKeyNotExist
	}
	return value, nil
}

func (s *skillActivationTestState) Set(key string, value any) error {
	if s.setErr != nil {
		return s.setErr
	}
	s.values[key] = value
	return nil
}

func (s *skillActivationTestState) All() iter.Seq2[string, any] {
	return func(yield func(string, any) bool) {
		for key, value := range s.values {
			if !yield(key, value) {
				return
			}
		}
	}
}

type skillGateTestContext struct {
	googleADKToolTestContext
	state        *skillActivationTestState
	agentName    string
	confirmation *toolconfirmation.ToolConfirmation
}

func newSkillGateTestContext(state *skillActivationTestState, agentName string) *skillGateTestContext {
	return &skillGateTestContext{
		googleADKToolTestContext: newGoogleADKToolTestContext(),
		state:                    state,
		agentName:                agentName,
	}
}

func (c *skillGateTestContext) AgentName() string                       { return c.agentName }
func (c *skillGateTestContext) State() adksession.State                 { return c.state }
func (c *skillGateTestContext) ReadonlyState() adksession.ReadonlyState { return c.state }
func (c *skillGateTestContext) ToolConfirmation() *toolconfirmation.ToolConfirmation {
	return c.confirmation
}
