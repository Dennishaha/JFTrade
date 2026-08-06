package adk

import (
	"context"
	"fmt"
	"iter"
	"strings"

	"github.com/openai/openai-go/v3/option"
	"google.golang.org/adk/v2/model"
	openaimodel "google.golang.org/adk/v2/model/openaimodel"
	"google.golang.org/genai"
)

type responsesToolNameModel struct {
	inner model.LLM
}

func newOpenAIResponsesADKModel(ctx context.Context, provider Provider, apiKey string, modelName string) (model.LLM, error) {
	inner, err := openaimodel.NewModel(ctx, defaultString(modelName, provider.Model), &openaimodel.ClientConfig{
		APIKey:     strings.TrimSpace(apiKey),
		BaseURL:    provider.BaseURL,
		HTTPClient: newProviderHTTPClient(providerRequestTimeout(provider)),
		Options:    providerResponseOptions(provider),
	})
	if err != nil {
		return nil, fmt.Errorf("create OpenAI Responses ADK model: %w", err)
	}
	return &responsesToolNameModel{inner: inner}, nil
}

func providerResponseOptions(provider Provider) []option.RequestOption {
	options := make([]option.RequestOption, 0, len(provider.DefaultHeaders))
	for key, value := range provider.DefaultHeaders {
		if strings.TrimSpace(key) != "" && strings.TrimSpace(value) != "" {
			options = append(options, option.WithHeader(key, value))
		}
	}
	return options
}

func probeOpenAIResponsesProvider(ctx context.Context, provider Provider, apiKey string, includeTool bool) (string, error) {
	llm, err := newOpenAIResponsesADKModel(ctx, provider, apiKey, provider.Model)
	if err != nil {
		return "", err
	}
	request := &model.LLMRequest{
		Config: &genai.GenerateContentConfig{
			SystemInstruction: genai.NewContentFromText("Reply with a short health check sentence.", genai.RoleUser),
		},
		Contents: []*genai.Content{genai.NewContentFromText("JFTrade ADK provider connectivity test.", genai.RoleUser)},
	}
	if includeTool {
		request.Config.Tools = []*genai.Tool{{FunctionDeclarations: []*genai.FunctionDeclaration{{
			Name: "system.health_probe", Description: "Probe provider tool support.",
		}}}}
	}
	var reply strings.Builder
	for response, responseErr := range llm.GenerateContent(ctx, request, false) {
		if responseErr != nil {
			return "", responseErr
		}
		if response == nil || response.Content == nil {
			continue
		}
		for _, part := range response.Content.Parts {
			if part != nil {
				reply.WriteString(part.Text)
			}
		}
	}
	return strings.TrimSpace(reply.String()), nil
}

func (m *responsesToolNameModel) Name() string {
	return m.inner.Name()
}

func (m *responsesToolNameModel) GenerateContent(ctx context.Context, req *model.LLMRequest, stream bool) iter.Seq2[*model.LLMResponse, error] {
	prepared, names, err := prepareResponsesRequest(req)
	if err != nil {
		return responseModelError(err)
	}
	return func(yield func(*model.LLMResponse, error) bool) {
		for response, responseErr := range m.inner.GenerateContent(ctx, prepared, stream) {
			if responseErr != nil || response == nil {
				if !yield(response, responseErr) {
					return
				}
				continue
			}
			if !yield(names.restoreResponse(response), nil) {
				return
			}
		}
	}
}

func responseModelError(err error) iter.Seq2[*model.LLMResponse, error] {
	return func(yield func(*model.LLMResponse, error) bool) {
		yield(nil, err)
	}
}

type responsesToolNames struct {
	toWire   map[string]string
	fromWire map[string]string
}

func prepareResponsesRequest(req *model.LLMRequest) (*model.LLMRequest, responsesToolNames, error) {
	if req == nil {
		return nil, responsesToolNames{}, fmt.Errorf("OpenAI Responses request is nil")
	}
	names, err := newResponsesToolNames(req)
	if err != nil {
		return nil, responsesToolNames{}, err
	}
	prepared := *req
	prepared.Config = names.cloneConfig(req.Config)
	prepared.Contents = names.cloneContents(req.Contents)
	return &prepared, names, nil
}

func newResponsesToolNames(req *model.LLMRequest) (responsesToolNames, error) {
	mapper := responsesToolNames{toWire: map[string]string{}, fromWire: map[string]string{}}
	for _, name := range responseToolNamesFromRequest(req) {
		if err := mapper.add(name); err != nil {
			return responsesToolNames{}, err
		}
	}
	return mapper, nil
}

func responseToolNamesFromRequest(req *model.LLMRequest) []string {
	var names []string
	if req.Config != nil {
		for _, tool := range req.Config.Tools {
			if tool == nil {
				continue
			}
			for _, declaration := range tool.FunctionDeclarations {
				if declaration == nil {
					continue
				}
				names = append(names, declaration.Name)
			}
		}
		if config := req.Config.ToolConfig; config != nil && config.FunctionCallingConfig != nil {
			names = append(names, config.FunctionCallingConfig.AllowedFunctionNames...)
		}
	}
	for _, content := range req.Contents {
		if content == nil {
			continue
		}
		for _, part := range content.Parts {
			if part.FunctionCall != nil {
				names = append(names, part.FunctionCall.Name)
			}
			if part.FunctionResponse != nil {
				names = append(names, part.FunctionResponse.Name)
			}
		}
	}
	return names
}

func (m responsesToolNames) add(name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil
	}
	if _, exists := m.toWire[name]; exists {
		return nil
	}
	wireName := sanitizeToolNameForOpenAI(name)
	if previous, collision := m.fromWire[wireName]; collision && previous != name {
		return fmt.Errorf("OpenAI Responses tool name collision: %q and %q both sanitize to %q", previous, name, wireName)
	}
	m.toWire[name] = wireName
	m.fromWire[wireName] = name
	return nil
}

func (m responsesToolNames) toResponseName(name string) string {
	if mapped, ok := m.toWire[name]; ok {
		return mapped
	}
	return sanitizeToolNameForOpenAI(name)
}

func (m responsesToolNames) restoreResponseName(name string) string {
	if mapped, ok := m.fromWire[name]; ok {
		return mapped
	}
	return name
}

func (m responsesToolNames) cloneConfig(config *genai.GenerateContentConfig) *genai.GenerateContentConfig {
	if config == nil {
		return nil
	}
	copyConfig := *config
	copyConfig.Tools = m.cloneTools(config.Tools)
	copyConfig.ToolConfig = m.cloneToolConfig(config.ToolConfig)
	return &copyConfig
}

func (m responsesToolNames) cloneTools(tools []*genai.Tool) []*genai.Tool {
	if len(tools) == 0 {
		return nil
	}
	cloned := make([]*genai.Tool, 0, len(tools))
	for _, tool := range tools {
		if tool == nil {
			cloned = append(cloned, nil)
			continue
		}
		copyTool := *tool
		copyTool.FunctionDeclarations = m.cloneFunctionDeclarations(tool.FunctionDeclarations)
		cloned = append(cloned, &copyTool)
	}
	return cloned
}

func (m responsesToolNames) cloneFunctionDeclarations(declarations []*genai.FunctionDeclaration) []*genai.FunctionDeclaration {
	cloned := make([]*genai.FunctionDeclaration, 0, len(declarations))
	for _, declaration := range declarations {
		if declaration == nil {
			cloned = append(cloned, nil)
			continue
		}
		copyDeclaration := *declaration
		copyDeclaration.Name = m.toResponseName(declaration.Name)
		cloned = append(cloned, &copyDeclaration)
	}
	return cloned
}

func (m responsesToolNames) cloneToolConfig(config *genai.ToolConfig) *genai.ToolConfig {
	if config == nil || config.FunctionCallingConfig == nil {
		return config
	}
	copyConfig := *config
	copyFunctionConfig := *config.FunctionCallingConfig
	copyFunctionConfig.AllowedFunctionNames = m.toResponseNames(config.FunctionCallingConfig.AllowedFunctionNames)
	copyConfig.FunctionCallingConfig = &copyFunctionConfig
	return &copyConfig
}

func (m responsesToolNames) toResponseNames(names []string) []string {
	cloned := make([]string, len(names))
	for index, name := range names {
		cloned[index] = m.toResponseName(name)
	}
	return cloned
}

func (m responsesToolNames) cloneContents(contents []*genai.Content) []*genai.Content {
	cloned := make([]*genai.Content, 0, len(contents))
	for _, content := range contents {
		cloned = append(cloned, m.cloneContent(content, m.toResponseName))
	}
	return cloned
}

func (m responsesToolNames) restoreResponse(response *model.LLMResponse) *model.LLMResponse {
	copyResponse := *response
	copyResponse.Content = m.cloneContent(response.Content, m.restoreResponseName)
	return &copyResponse
}

func (m responsesToolNames) cloneContent(content *genai.Content, mapName func(string) string) *genai.Content {
	if content == nil {
		return nil
	}
	copyContent := *content
	copyContent.Parts = make([]*genai.Part, 0, len(content.Parts))
	for _, part := range content.Parts {
		copyContent.Parts = append(copyContent.Parts, cloneResponsePart(part, mapName))
	}
	return &copyContent
}

func cloneResponsePart(part *genai.Part, mapName func(string) string) *genai.Part {
	if part == nil {
		return nil
	}
	copyPart := *part
	if part.FunctionCall != nil {
		copyCall := *part.FunctionCall
		copyCall.Name = mapName(part.FunctionCall.Name)
		copyPart.FunctionCall = &copyCall
	}
	if part.FunctionResponse != nil {
		copyResult := *part.FunctionResponse
		copyResult.Name = mapName(part.FunctionResponse.Name)
		copyPart.FunctionResponse = &copyResult
	}
	return &copyPart
}
