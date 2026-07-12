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
