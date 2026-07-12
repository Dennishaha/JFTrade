package adk

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	adkagent "google.golang.org/adk/v2/agent"
	adkartifact "google.golang.org/adk/v2/artifact"
	adkmodel "google.golang.org/adk/v2/model"
	adktool "google.golang.org/adk/v2/tool"
	"google.golang.org/adk/v2/tool/functiontool"
	"google.golang.org/adk/v2/tool/loadartifactstool"
	"google.golang.org/adk/v2/tool/loadmemorytool"
	"google.golang.org/adk/v2/tool/preloadmemorytool"
	"google.golang.org/adk/v2/tool/skilltoolset"
	adkskill "google.golang.org/adk/v2/tool/skilltoolset/skill"
	"google.golang.org/genai"
)

type googleADKTool struct {
	descriptor ToolDescriptor
	registered RegisteredTool
	tool       googleADKRunnableTool
	agent      Agent
}

type googleADKRunnableTool interface {
	adktool.Tool
	Run(adkagent.Context, any) (map[string]any, error)
}

type googleADKProductToolset struct {
	name  string
	tools []adktool.Tool
}

type googleADKStaticToolset struct {
	name  string
	tools []adktool.Tool
}

type googleADKArtifactToolset struct {
	name    string
	service adkartifact.Service
}

func (r *Runtime) googleADKToolsets(ctx context.Context, definition Agent) ([]adktool.Toolset, error) {
	baseToolset, err := r.googleADKProductToolset(definition)
	if err != nil {
		return nil, err
	}
	toolsets := make([]adktool.Toolset, 0, 4)
	if baseToolset != nil {
		filtered := adktool.FilterToolset(baseToolset, func(_ adkagent.ReadonlyContext, tool adktool.Tool) bool {
			if descriptor, ok := descriptorFromADKTool(tool); ok {
				return ToolAllowedInMode(descriptor, definition.PermissionMode)
			}
			return false
		})
		confirmed := adktool.WithConfirmation(filtered, false, func(toolName string, _ any) bool {
			registered, ok := r.tools.Get(toolName)
			if !ok {
				return false
			}
			return ToolRequiresApproval(registered.Descriptor, definition.PermissionMode)
		})
		toolsets = append(toolsets, newGoogleADKSkillGatedToolset(confirmed, ToolDescriptorsForAgent(definition, r.tools)))
	}
	if definition.MemoryEnabled && r.memoryService != nil {
		toolsets = append(toolsets, googleADKStaticToolset{name: "jftrade-adk-memory-tools", tools: []adktool.Tool{preloadmemorytool.New(), loadmemorytool.New()}})
	}
	if r.artifactService != nil {
		toolsets = append(toolsets, googleADKArtifactToolset{name: "jftrade-adk-artifact-tools", service: r.artifactService})
	}
	source, err := r.skills.Source(ctx, definition.Skills)
	if err != nil {
		return nil, err
	}
	if source == nil {
		return toolsets, nil
	}
	source, err = r.filteredSkillSourceForAgent(ctx, source, definition)
	if err != nil {
		return nil, err
	}
	toolset, err := skilltoolset.New(ctx, skilltoolset.Config{Source: source})
	if err != nil {
		return nil, fmt.Errorf("create GO-ADK skill toolset: %w", err)
	}
	toolsets = append(toolsets, toolset)
	return toolsets, nil
}

func (r *Runtime) filteredSkillSourceForAgent(ctx context.Context, source adkskill.Source, definition Agent) (adkskill.Source, error) {
	if source == nil {
		return nil, nil
	}
	frontmatters, err := source.ListFrontmatters(ctx)
	if err != nil {
		return nil, err
	}
	allowedTools := r.allowedToolNamesForAgent(definition)
	allowedSkills := make(map[string]struct{}, len(frontmatters))
	for _, frontmatter := range frontmatters {
		if skillAllowedForAgent(frontmatter, allowedTools, r.tools, definition.PermissionMode) {
			allowedSkills[frontmatter.Name] = struct{}{}
		}
	}
	return &agentFilteredSkillSource{base: source, allowed: allowedSkills}, nil
}

func (r *Runtime) allowedToolNamesForAgent(definition Agent) map[string]struct{} {
	descriptors := ToolDescriptorsForAgent(definition, r.tools)
	allowed := make(map[string]struct{}, len(descriptors))
	for _, descriptor := range descriptors {
		if ToolAllowedInMode(descriptor, definition.PermissionMode) {
			allowed[descriptor.Name] = struct{}{}
		}
	}
	return allowed
}

func skillAllowedForAgent(
	frontmatter *adkskill.Frontmatter,
	allowedTools map[string]struct{},
	registry *ToolRegistry,
	mode string,
) bool {
	if frontmatter == nil {
		return false
	}
	if builtinSkillAllowsAuthorizedToolSubset(frontmatter.Name) {
		return builtinSkillAllowedForAgentSubset(frontmatter, allowedTools, registry, mode)
	}
	for _, toolName := range frontmatter.AllowedTools {
		if registry == nil {
			return false
		}
		canonical, ok := registry.CanonicalName(toolName)
		if !ok {
			return false
		}
		registered, ok := registry.Get(canonical)
		if !ok {
			return false
		}
		if !ToolAllowedInMode(registered.Descriptor, mode) {
			return false
		}
		if _, ok := allowedTools[canonical]; !ok {
			return false
		}
	}
	return true
}

func builtinSkillAllowedForAgentSubset(
	frontmatter *adkskill.Frontmatter,
	allowedTools map[string]struct{},
	registry *ToolRegistry,
	mode string,
) bool {
	if frontmatter == nil || registry == nil {
		return false
	}
	for _, toolName := range frontmatter.AllowedTools {
		canonical, ok := registry.CanonicalName(toolName)
		if !ok {
			continue
		}
		registered, ok := registry.Get(canonical)
		if !ok || !ToolAllowedInMode(registered.Descriptor, mode) {
			continue
		}
		if _, ok := allowedTools[canonical]; ok {
			return true
		}
	}
	return false
}

type agentFilteredSkillSource struct {
	base    adkskill.Source
	allowed map[string]struct{}
}

func (s *agentFilteredSkillSource) ListFrontmatters(ctx context.Context) ([]*adkskill.Frontmatter, error) {
	items, err := s.base.ListFrontmatters(ctx)
	if err != nil {
		return nil, err
	}
	filtered := make([]*adkskill.Frontmatter, 0, len(items))
	for _, item := range items {
		if s.isAllowed(item.Name) {
			filtered = append(filtered, item)
		}
	}
	return filtered, nil
}

func (s *agentFilteredSkillSource) ListResources(ctx context.Context, name, subpath string) ([]string, error) {
	if !s.isAllowed(name) {
		return nil, adkskill.ErrSkillNotFound
	}
	return s.base.ListResources(ctx, name, subpath)
}

func (s *agentFilteredSkillSource) LoadFrontmatter(ctx context.Context, name string) (*adkskill.Frontmatter, error) {
	if !s.isAllowed(name) {
		return nil, adkskill.ErrSkillNotFound
	}
	return s.base.LoadFrontmatter(ctx, name)
}

func (s *agentFilteredSkillSource) LoadInstructions(ctx context.Context, name string) (string, error) {
	if !s.isAllowed(name) {
		return "", adkskill.ErrSkillNotFound
	}
	return s.base.LoadInstructions(ctx, name)
}

func (s *agentFilteredSkillSource) LoadResource(ctx context.Context, name, resourcePath string) (io.ReadCloser, error) {
	if !s.isAllowed(name) {
		return nil, adkskill.ErrSkillNotFound
	}
	return s.base.LoadResource(ctx, name, resourcePath)
}

func (s *agentFilteredSkillSource) isAllowed(name string) bool {
	if s == nil {
		return false
	}
	_, ok := s.allowed[strings.TrimSpace(name)]
	return ok
}

func (r *Runtime) googleADKProductToolset(definition Agent) (adktool.Toolset, error) {
	descriptors := ToolDescriptorsForAgent(definition, r.tools)
	if len(descriptors) == 0 {
		return nil, nil
	}
	tools := make([]adktool.Tool, 0, len(descriptors))
	for _, descriptor := range descriptors {
		registered, _ := r.tools.Get(descriptor.Name)
		tool, err := newGoogleADKTool(descriptor, registered)
		if err != nil {
			return nil, err
		}
		tool.agent = definition
		tools = append(tools, tool)
	}
	return &googleADKProductToolset{name: "jftrade-tools", tools: tools}, nil
}

func newGoogleADKTool(descriptor ToolDescriptor, registered RegisteredTool) (*googleADKTool, error) {
	schema := descriptor.InputSchema
	if schema == nil {
		schema = map[string]any{"type": "object", "properties": map[string]any{}}
	}
	if _, err := googleADKJSONSchemaFromMap(sanitizeSchemaForOpenAI(schema)); err != nil {
		return nil, fmt.Errorf("convert GO-ADK product tool schema %q: %w", descriptor.Name, err)
	}
	wrapper := &googleADKTool{descriptor: descriptor, registered: registered}
	inner, err := functiontool.New[map[string]any, map[string]any](functiontool.Config{
		Name:        descriptor.Name,
		Description: descriptor.Description,
		// Product tools preserve JFTrade's historical argument tolerance at
		// execution time; the stricter descriptor schema is still exposed by
		// Declaration for model guidance.
	}, wrapper.run)
	if err != nil {
		return nil, fmt.Errorf("create GO-ADK product function tool %q: %w", descriptor.Name, err)
	}
	wrapper.tool = inner.(googleADKRunnableTool)
	return wrapper, nil
}

func (t *googleADKTool) Name() string {
	if t == nil {
		return ""
	}
	if t.tool != nil {
		return t.tool.Name()
	}
	return t.descriptor.Name
}

func (t *googleADKTool) Description() string {
	if t == nil {
		return ""
	}
	if t.tool != nil {
		return t.tool.Description()
	}
	return t.descriptor.Description
}

func (t *googleADKTool) IsLongRunning() bool {
	return t != nil && t.tool != nil && t.tool.IsLongRunning()
}

func (t *googleADKTool) googleADKToolDescriptor() ToolDescriptor {
	return t.descriptor
}

func (t *googleADKProductToolset) Name() string { return t.name }

func (t *googleADKProductToolset) Tools(_ adkagent.ReadonlyContext) ([]adktool.Tool, error) {
	if t == nil {
		return nil, nil
	}
	tools := make([]adktool.Tool, 0, len(t.tools))
	tools = append(tools, t.tools...)
	return tools, nil
}

func (t googleADKStaticToolset) Name() string { return t.name }

func (t googleADKStaticToolset) Tools(_ adkagent.ReadonlyContext) ([]adktool.Tool, error) {
	return append([]adktool.Tool(nil), t.tools...), nil
}

func (t googleADKArtifactToolset) Name() string { return t.name }

func (t googleADKArtifactToolset) Tools(ctx adkagent.ReadonlyContext) ([]adktool.Tool, error) {
	if t.service == nil || ctx == nil || strings.TrimSpace(ctx.AppName()) == "" || strings.TrimSpace(ctx.UserID()) == "" || strings.TrimSpace(ctx.SessionID()) == "" {
		return nil, nil
	}
	response, err := t.service.List(ctx, &adkartifact.ListRequest{
		AppName: ctx.AppName(), UserID: ctx.UserID(), SessionID: ctx.SessionID(),
	})
	if err != nil {
		return nil, err
	}
	if len(response.FileNames) == 0 {
		return nil, nil
	}
	return []adktool.Tool{loadartifactstool.New()}, nil
}

func descriptorFromADKTool(tool adktool.Tool) (ToolDescriptor, bool) {
	typed, ok := tool.(interface {
		googleADKToolDescriptor() ToolDescriptor
	})
	if !ok || typed == nil {
		return ToolDescriptor{}, false
	}
	return typed.googleADKToolDescriptor(), true
}

func toolDescriptorIndex(descriptors []ToolDescriptor) map[string]ToolDescriptor {
	if len(descriptors) == 0 {
		return nil
	}
	index := make(map[string]ToolDescriptor, len(descriptors))
	for _, descriptor := range descriptors {
		index[descriptor.Name] = descriptor
	}
	return index
}

func (t *googleADKTool) Declaration() *genai.FunctionDeclaration {
	if t == nil {
		return nil
	}
	schema := t.descriptor.InputSchema
	if schema == nil {
		schema = map[string]any{"type": "object", "properties": map[string]any{}}
	}
	return &genai.FunctionDeclaration{
		Name: t.Name(), Description: t.Description(), ParametersJsonSchema: sanitizeSchemaForOpenAI(schema),
	}
}

func (t *googleADKTool) ProcessRequest(ctx adkagent.Context, req *adkmodel.LLMRequest) error {
	return packGoogleADKTool(req, t)
}

type googleADKDeclaredRunnableTool interface {
	adktool.Tool
	Declaration() *genai.FunctionDeclaration
	Run(adkagent.Context, any) (map[string]any, error)
}

func packGoogleADKTool(req *adkmodel.LLMRequest, tool googleADKDeclaredRunnableTool) error {
	if req == nil || tool == nil {
		return fmt.Errorf("GO-ADK tool request is unavailable")
	}
	if req.Tools == nil {
		req.Tools = make(map[string]any)
	}
	if _, exists := req.Tools[tool.Name()]; exists {
		return fmt.Errorf("duplicate tool: %q", tool.Name())
	}
	req.Tools[tool.Name()] = tool
	if req.Config == nil {
		req.Config = &genai.GenerateContentConfig{}
	}
	var functionTools *genai.Tool
	for _, item := range req.Config.Tools {
		if item != nil && item.FunctionDeclarations != nil {
			functionTools = item
			break
		}
	}
	if functionTools == nil {
		req.Config.Tools = append(req.Config.Tools, &genai.Tool{
			FunctionDeclarations: []*genai.FunctionDeclaration{tool.Declaration()},
		})
	} else {
		functionTools.FunctionDeclarations = append(functionTools.FunctionDeclarations, tool.Declaration())
	}
	return nil
}

func (t *googleADKTool) Run(ctx adkagent.Context, args any) (map[string]any, error) {
	if t == nil || t.tool == nil {
		return nil, fmt.Errorf("GO-ADK product tool %q is not runnable", t.Name())
	}
	return t.tool.Run(ctx, args)
}

func (t *googleADKTool) run(ctx adkagent.Context, input map[string]any) (map[string]any, error) {
	toolCtx := contextWithToolInvocationMetadata(ctx)
	toolCtx = contextWithToolAgent(toolCtx, t.agent)
	output, execErr := executeRegisteredTool(toolCtx, t.registered, input)
	if execErr != nil {
		if errors.Is(execErr, adktool.ErrConfirmationRequired) || errors.Is(execErr, adktool.ErrConfirmationRejected) {
			return nil, execErr
		}
		// Return the error as a structured message rather than a bare
		// {"error":"..."} map.  The LLM-native conversation contract uses
		// the "message" field (which the LLM reads as a natural-language
		// explanation) together with "success":false so our call-tracking
		// can detect the failure without relying on the ADK framework's
		// interpretation of an "error" key.
		return map[string]any{
			"success": false,
			"message": fmt.Sprintf("工具 %s 执行失败: %s", t.Name(), execErr.Error()),
		}, nil
	}
	if mapped, ok := output.(map[string]any); ok {
		if structuredErr, ok := structuredToolError(mapped); ok {
			return map[string]any{
				"success": false,
				"message": fmt.Sprintf("工具 %s 返回错误: %s", t.Name(), structuredErr),
			}, nil
		}
		return mapped, nil
	}
	return map[string]any{"result": output}, nil
}

func structuredToolError(result map[string]any) (string, bool) {
	if len(result) == 0 {
		return "", false
	}
	// New format: {"success":false, "message":"..."}
	if success, ok := result["success"]; ok {
		if boolVal := jftradeOptionalTypeAssertion[bool](success); !boolVal {
			if msg, ok := result["message"]; ok {
				return strings.TrimSpace(fmt.Sprint(msg)), true
			}
			return "tool execution failed", true
		}
		return "", false
	}
	// Legacy format: {"error":"..."}
	errValue, ok := result["error"]
	if !ok {
		return "", false
	}
	errText := strings.TrimSpace(fmt.Sprint(errValue))
	if errText == "" || strings.EqualFold(errText, "<nil>") {
		return "", false
	}
	return errText, true
}

// isToolResponseError reports whether the tool response map signals a failure,
// either via the new {"success":false} contract or the legacy {"error":"..."} key.
func isToolResponseError(response map[string]any) bool {
	if len(response) == 0 {
		return false
	}
	if success, ok := response["success"]; ok {
		if boolVal := jftradeOptionalTypeAssertion[bool](success); !boolVal {
			return true
		}
		return false
	}
	_, hasError := response["error"]
	return hasError
}

// toolResponseErrorMessage extracts a human-readable error message from a tool
// response map that was determined to be a failure.
func toolResponseErrorMessage(response map[string]any) string {
	if msg, ok := response["message"]; ok {
		if text := strings.TrimSpace(fmt.Sprint(msg)); text != "" {
			return text
		}
	}
	if errValue, ok := response["error"]; ok {
		return strings.TrimSpace(fmt.Sprint(errValue))
	}
	return "tool execution failed"
}
