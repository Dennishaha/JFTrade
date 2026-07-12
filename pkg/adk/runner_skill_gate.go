package adk

import (
	"fmt"
	"strings"

	adkagent "google.golang.org/adk/v2/agent"
	adkmodel "google.golang.org/adk/v2/model"
	adktool "google.golang.org/adk/v2/tool"
	"google.golang.org/genai"
)

type googleADKSkillGatedToolset struct {
	toolset        adktool.Toolset
	requiredSkills map[string][]string
}

func newGoogleADKSkillGatedToolset(toolset adktool.Toolset, descriptors []ToolDescriptor) adktool.Toolset {
	required := make(map[string][]string)
	for _, descriptor := range descriptors {
		if skillNames := ToolRequiredSkillNames(descriptor); len(skillNames) > 0 {
			required[descriptor.Name] = skillNames
		}
	}
	return &googleADKSkillGatedToolset{toolset: toolset, requiredSkills: required}
}

func (t *googleADKSkillGatedToolset) Name() string {
	if t == nil || t.toolset == nil {
		return ""
	}
	return t.toolset.Name()
}

func (t *googleADKSkillGatedToolset) Tools(ctx adkagent.ReadonlyContext) ([]adktool.Tool, error) {
	if t == nil || t.toolset == nil {
		return nil, nil
	}
	tools, err := t.toolset.Tools(ctx)
	if err != nil {
		return nil, err
	}
	wrapped := make([]adktool.Tool, 0, len(tools))
	for _, tool := range tools {
		requiredSkills := t.requiredSkills[tool.Name()]
		if len(requiredSkills) == 0 {
			wrapped = append(wrapped, tool)
			continue
		}
		runnable, ok := tool.(googleADKDeclaredRunnableTool)
		if !ok {
			return nil, fmt.Errorf("skill-gated tool %q is not runnable", tool.Name())
		}
		wrapped = append(wrapped, &googleADKSkillGatedTool{tool: runnable, requiredSkills: requiredSkills})
	}
	return wrapped, nil
}

type googleADKSkillGatedTool struct {
	tool           googleADKDeclaredRunnableTool
	requiredSkills []string
}

func (t *googleADKSkillGatedTool) Name() string        { return t.tool.Name() }
func (t *googleADKSkillGatedTool) Description() string { return t.tool.Description() }
func (t *googleADKSkillGatedTool) IsLongRunning() bool { return t.tool.IsLongRunning() }
func (t *googleADKSkillGatedTool) Declaration() *genai.FunctionDeclaration {
	return t.tool.Declaration()
}

func (t *googleADKSkillGatedTool) ProcessRequest(ctx adkagent.Context, req *adkmodel.LLMRequest) error {
	if t == nil || t.tool == nil || ctx == nil {
		return nil
	}
	if !anySkillActiveInState(ctx.ReadonlyState(), ctx.AgentName(), t.requiredSkills) {
		return nil
	}
	return packGoogleADKTool(req, t)
}

func (t *googleADKSkillGatedTool) Run(ctx adkagent.Context, args any) (map[string]any, error) {
	if t == nil || t.tool == nil || ctx == nil {
		return nil, fmt.Errorf("skill-gated tool is unavailable")
	}
	if ctx.ToolConfirmation() == nil && !anySkillActiveInState(ctx.ReadonlyState(), ctx.AgentName(), t.requiredSkills) {
		return nil, skillRequiredLoadError(t.Name(), t.requiredSkills)
	}
	return t.tool.Run(ctx, args)
}

func skillRequiredLoadError(toolName string, skillNames []string) error {
	skillNames = normalizeStringSlice(skillNames)
	if len(skillNames) == 1 {
		return fmt.Errorf("tool %q requires loading skill %q in the current invocation", toolName, skillNames[0])
	}
	return fmt.Errorf("tool %q requires loading one of skills %q in the current invocation", toolName, strings.Join(skillNames, "\", \""))
}
