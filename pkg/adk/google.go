package adk

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"iter"
	"net/http"
	"strings"

	"google.golang.org/adk/model"
	"google.golang.org/genai"
)

const GoogleADKModule = "google.golang.org/adk"

type openAICompatibleADKModel struct {
	provider Provider
	apiKey   string
	model    string
}

func newOpenAICompatibleADKModel(provider Provider, apiKey string, modelName string) model.LLM {
	return &openAICompatibleADKModel{
		provider: provider,
		apiKey:   strings.TrimSpace(apiKey),
		model:    defaultString(modelName, provider.Model),
	}
}

func (m *openAICompatibleADKModel) Name() string {
	return m.model
}

func (m *openAICompatibleADKModel) GenerateContent(ctx context.Context, req *model.LLMRequest, stream bool) iter.Seq2[*model.LLMResponse, error] {
	return func(yield func(*model.LLMResponse, error) bool) {
		stopped := false
		safeYield := func(resp *model.LLMResponse, err error) bool {
			if stopped {
				return false
			}
			if !yield(resp, err) {
				stopped = true
				return false
			}
			return true
		}
		if stream {
			if err := m.generateStream(ctx, req, safeYield); err != nil && !stopped && !errors.Is(err, context.Canceled) {
				safeYield(nil, err)
			}
			return
		}
		response, err := m.generate(ctx, req)
		safeYield(response, err)
	}
}

func (m *openAICompatibleADKModel) generate(ctx context.Context, req *model.LLMRequest) (*model.LLMResponse, error) {
	payload := m.buildChatRequest(req, false)
	return m.doGenerate(ctx, payload)
}

func (m *openAICompatibleADKModel) generateStream(
	ctx context.Context,
	req *model.LLMRequest,
	yield func(*model.LLMResponse, error) bool,
) error {
	payload := m.buildChatRequest(req, true)
	httpReq, err := m.newChatRequest(ctx, payload)
	if err != nil {
		return err
	}
	resp, err := newProviderHTTPClient(providerRequestTimeout(m.provider)).Do(httpReq)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, readErr := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
		if readErr != nil {
			return readErr
		}
		errDetail := strings.TrimSpace(string(body))
		if errDetail == "" {
			errDetail = resp.Status
		}
		return fmt.Errorf("provider returned %d: %s", resp.StatusCode, errDetail)
	}
	if !strings.Contains(strings.ToLower(resp.Header.Get("Content-Type")), "text/event-stream") {
		response, err := m.decodeChatResponse(resp.Body)
		if err != nil {
			return err
		}
		yield(response, nil)
		return nil
	}

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 64<<10), 2<<20)
	var dataLines []string
	state := openAIStreamAggregationState{}

	flushEvent := func() error {
		if len(dataLines) == 0 {
			return nil
		}
		payload := strings.Join(dataLines, "\n")
		dataLines = dataLines[:0]
		if strings.TrimSpace(payload) == "" {
			return nil
		}
		if strings.TrimSpace(payload) == "[DONE]" {
			return io.EOF
		}
		var parsed openAIChatStreamResponse
		if err := json.Unmarshal([]byte(payload), &parsed); err != nil {
			return fmt.Errorf("decode OpenAI-compatible ADK stream chunk: %w", err)
		}
		if parsed.Error != nil && parsed.Error.Message != "" {
			return fmt.Errorf("provider returned: %s", parsed.Error.Message)
		}
		for _, choice := range parsed.Choices {
			if err := state.consume(choice.Delta, yield); err != nil {
				return err
			}
			if err := state.consumeMessage(choice.Message, yield); err != nil {
				return err
			}
		}
		return nil
	}

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			err := flushEvent()
			if err == io.EOF {
				break
			}
			if err != nil {
				return err
			}
			continue
		}
		if strings.HasPrefix(line, "data:") {
			dataLines = append(dataLines, strings.TrimSpace(strings.TrimPrefix(line, "data:")))
		}
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	if err := flushEvent(); err != nil && err != io.EOF {
		return err
	}
	final, err := state.finalResponse()
	if err != nil {
		return err
	}
	if final == nil {
		return fmt.Errorf("provider returned an empty reply")
	}
	yield(final, nil)
	return nil
}

func (m *openAICompatibleADKModel) buildChatRequest(req *model.LLMRequest, stream bool) openAIChatRequest {
	messages := make([]openAIChatMessage, 0, len(req.Contents)+1)
	if req.Config != nil && req.Config.SystemInstruction != nil {
		if text := genAIContentText(req.Config.SystemInstruction); text != "" {
			messages = append(messages, openAIChatMessage{Role: "system", Content: text})
		}
	}
	for _, content := range req.Contents {
		messages = append(messages, openAIMessagesFromGenAI(content)...)
	}
	messages = trimMessagesForProvider(messages, maxProviderPayloadBytes)

	payload := openAIChatRequest{
		Model:       defaultString(req.Model, m.model),
		Messages:    messages,
		Temperature: 0.2,
		Stream:      stream,
		Tools:       openAIToolsFromGenAIConfig(req.Config),
	}
	if len(payload.Tools) > 0 {
		payload.ToolChoice = "auto"
	}
	return payload
}

func (m *openAICompatibleADKModel) doGenerate(ctx context.Context, payload openAIChatRequest) (*model.LLMResponse, error) {
	httpReq, err := m.newChatRequest(ctx, payload)
	if err != nil {
		return nil, err
	}
	resp, err := newProviderHTTPClient(providerRequestTimeout(m.provider)).Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, readErr := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
		if readErr != nil {
			return nil, readErr
		}
		errDetail := strings.TrimSpace(string(body))
		if errDetail == "" {
			errDetail = resp.Status
		}
		return nil, fmt.Errorf("provider returned %d: %s", resp.StatusCode, errDetail)
	}
	return m.decodeChatResponse(resp.Body)
}

func (m *openAICompatibleADKModel) newChatRequest(ctx context.Context, payload openAIChatRequest) (*http.Request, error) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	endpoint := strings.TrimRight(m.provider.BaseURL, "/") + "/chat/completions"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if m.apiKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+m.apiKey)
	}
	for key, value := range m.provider.DefaultHeaders {
		if strings.TrimSpace(key) != "" && strings.TrimSpace(value) != "" {
			httpReq.Header.Set(key, value)
		}
	}
	return httpReq, nil
}

func (m *openAICompatibleADKModel) decodeChatResponse(body io.Reader) (*model.LLMResponse, error) {
	raw, err := io.ReadAll(io.LimitReader(body, 4<<20))
	if err != nil {
		return nil, err
	}
	var parsed openAIChatResponse
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil, fmt.Errorf("decode OpenAI-compatible ADK response: %w", err)
	}
	if parsed.Error != nil && parsed.Error.Message != "" {
		return nil, fmt.Errorf("provider returned: %s", parsed.Error.Message)
	}
	if len(parsed.Choices) == 0 {
		return nil, fmt.Errorf("provider returned no choices")
	}
	return openAIMessageToADKResponse(parsed.Choices[0].Message, false)
}

func openAIMessageToADKResponse(message openAIChatMessage, partial bool) (*model.LLMResponse, error) {
	replyText, reasoningText := extractVisibleAndReasoningText(message.Content, message.ReasoningContent, message.Reasoning)
	parts := make([]*genai.Part, 0, len(message.ToolCalls)+2)
	parts = append(parts, partsFromReplyAndReasoning(replyText, reasoningText)...)
	for _, call := range message.ToolCalls {
		args := map[string]any{}
		if strings.TrimSpace(call.Function.Arguments) != "" {
			if err := json.Unmarshal([]byte(call.Function.Arguments), &args); err != nil {
				return nil, fmt.Errorf("decode tool arguments for %s: %w", call.Function.Name, err)
			}
		}
		// Restore the tool name from the sanitized form (dots → hyphens → dots)
		// because openAIToolsFromGenAIConfig sanitizes names before sending to the
		// provider, and the provider echoes back the sanitized name.
		toolName := restoreToolNameFromOpenAI(strings.TrimSpace(call.Function.Name))
		parts = append(parts, &genai.Part{FunctionCall: &genai.FunctionCall{
			ID: call.ID, Name: toolName, Args: args,
		}})
	}
	return &model.LLMResponse{
		Content:      genai.NewContentFromParts(parts, genai.RoleModel),
		Partial:      partial,
		TurnComplete: !partial,
	}, nil
}

type openAIStreamAggregationState struct {
	content   strings.Builder
	reasoning strings.Builder
	toolCalls []openAIToolCall
}

func (s *openAIStreamAggregationState) consume(delta openAIChatStreamDelta, yield func(*model.LLMResponse, error) bool) error {
	if delta.Content != "" || delta.ReasoningContent != "" || delta.Reasoning != "" {
		replyText, reasoningText := extractVisibleAndReasoningText(delta.Content, delta.ReasoningContent, delta.Reasoning)
		s.content.WriteString(replyText)
		s.reasoning.WriteString(reasoningText)
		response, err := openAIMessageToADKResponse(openAIChatMessage{
			Content:          replyText,
			ReasoningContent: reasoningText,
		}, true)
		if err != nil {
			return err
		}
		if !yield(response, nil) {
			return context.Canceled
		}
	}
	if len(delta.ToolCalls) > 0 {
		s.mergeToolCalls(delta.ToolCalls)
	}
	return nil
}

func (s *openAIStreamAggregationState) consumeMessage(message openAIChatMessage, yield func(*model.LLMResponse, error) bool) error {
	if message.Content != "" || message.ReasoningContent != "" || message.Reasoning != "" {
		replyText, reasoningText := extractVisibleAndReasoningText(message.Content, message.ReasoningContent, message.Reasoning)
		s.content.WriteString(replyText)
		s.reasoning.WriteString(reasoningText)
		response, err := openAIMessageToADKResponse(openAIChatMessage{
			Content:          replyText,
			ReasoningContent: reasoningText,
		}, true)
		if err != nil {
			return err
		}
		if !yield(response, nil) {
			return context.Canceled
		}
	}
	if len(message.ToolCalls) > 0 {
		s.mergeToolCalls(message.ToolCalls)
	}
	return nil
}

func (s *openAIStreamAggregationState) mergeToolCalls(chunks []openAIToolCall) {
	for _, chunk := range chunks {
		index := chunk.Index
		if index < 0 {
			index = 0
		}
		for len(s.toolCalls) <= index {
			s.toolCalls = append(s.toolCalls, openAIToolCall{})
		}
		current := &s.toolCalls[index]
		if chunk.ID != "" {
			current.ID = chunk.ID
		}
		if chunk.Type != "" {
			current.Type = chunk.Type
		}
		if chunk.Function.Name != "" {
			current.Function.Name = chunk.Function.Name
		}
		if chunk.Function.Arguments != "" {
			current.Function.Arguments += chunk.Function.Arguments
		}
	}
}

func (s *openAIStreamAggregationState) finalResponse() (*model.LLMResponse, error) {
	if s.content.Len() == 0 && len(s.toolCalls) == 0 {
		if s.reasoning.Len() == 0 {
			return nil, nil
		}
	}
	if s.content.Len() == 0 && s.reasoning.Len() == 0 && len(s.toolCalls) == 0 {
		return nil, nil
	}
	return openAIMessageToADKResponse(openAIChatMessage{
		Content:          s.content.String(),
		ReasoningContent: s.reasoning.String(),
		ToolCalls:        s.toolCalls,
	}, false)
}

type localADKModel struct {
	agent Agent
	tools *ToolRegistry
}

func (m localADKModel) Name() string { return "jftrade-local" }

func (m localADKModel) GenerateContent(_ context.Context, req *model.LLMRequest, _ bool) iter.Seq2[*model.LLMResponse, error] {
	return func(yield func(*model.LLMResponse, error) bool) {
		var latestText string
		var summaries []string
		for _, content := range req.Contents {
			for _, part := range content.Parts {
				if part.Text != "" {
					latestText = part.Text
				}
				if part.FunctionResponse != nil {
					summaries = append(summaries, summarizeToolOutput(part.FunctionResponse.Name, part.FunctionResponse.Response))
				}
			}
		}
		if len(summaries) > 0 {
			yield(&model.LLMResponse{
				Content:      genai.NewContentFromText(localReply(latestText, summaries, nil), genai.RoleModel),
				TurnComplete: true,
			}, nil)
			return
		}
		invocations := SelectToolInvocations(latestText, m.agent, m.tools)
		if len(invocations) > 0 {
			parts := make([]*genai.Part, 0, len(invocations))
			for _, invocation := range invocations {
				parts = append(parts, &genai.Part{FunctionCall: &genai.FunctionCall{
					ID: "call-" + normalizeID(invocation.Name), Name: invocation.Name, Args: invocation.Input,
				}})
			}
			yield(&model.LLMResponse{
				Content:      genai.NewContentFromParts(parts, genai.RoleModel),
				TurnComplete: true,
			}, nil)
			return
		}
		yield(&model.LLMResponse{
			Content:      genai.NewContentFromText(localReply(latestText, nil, nil), genai.RoleModel),
			TurnComplete: true,
		}, nil)
	}
}

func openAIMessagesFromGenAI(content *genai.Content) []openAIChatMessage {
	if content == nil {
		return nil
	}
	role := "user"
	if content.Role == genai.RoleModel {
		role = "assistant"
	}
	var text strings.Builder
	var reasoning strings.Builder
	var calls []openAIToolCall
	var messages []openAIChatMessage
	for _, part := range content.Parts {
		if part.Text != "" {
			if part.Thought {
				reasoning.WriteString(part.Text)
			} else {
				text.WriteString(part.Text)
			}
		}
		if part.FunctionCall != nil {
			rawArgs, _ := json.Marshal(part.FunctionCall.Args)
			call := openAIToolCall{ID: part.FunctionCall.ID, Type: "function"}
			call.Function.Name = part.FunctionCall.Name
			call.Function.Arguments = string(rawArgs)
			calls = append(calls, call)
		}
		if part.FunctionResponse != nil {
			rawResponse, _ := json.Marshal(part.FunctionResponse.Response)
			messages = append(messages, openAIChatMessage{
				Role:       "tool",
				Content:    string(rawResponse),
				Name:       part.FunctionResponse.Name,
				ToolCallID: part.FunctionResponse.ID,
			})
		}
	}
	if text.Len() > 0 || reasoning.Len() > 0 || len(calls) > 0 {
		messages = append([]openAIChatMessage{{
			Role:             role,
			Content:          text.String(),
			ReasoningContent: reasoning.String(),
			ToolCalls:        calls,
		}}, messages...)
	}
	return messages
}

func openAIToolsFromGenAIConfig(config *genai.GenerateContentConfig) []openAITool {
	if config == nil {
		return nil
	}
	var result []openAITool
	for _, item := range config.Tools {
		for _, declaration := range item.FunctionDeclarations {
			parameters, _ := declaration.ParametersJsonSchema.(map[string]any)
			if parameters == nil {
				raw, _ := json.Marshal(declaration.ParametersJsonSchema)
				_ = json.Unmarshal(raw, &parameters)
			}
			parameters = sanitizeSchemaForOpenAI(parameters)
			result = append(result, openAITool{
				Type: "function",
				Function: openAIToolFunction{
					Name: sanitizeToolNameForOpenAI(declaration.Name), Description: declaration.Description, Parameters: parameters,
				},
			})
		}
	}
	return result
}

func genAIContentText(content *genai.Content) string {
	if content == nil {
		return ""
	}
	var builder strings.Builder
	for _, part := range content.Parts {
		builder.WriteString(part.Text)
	}
	return strings.TrimSpace(builder.String())
}

// sanitizeSchemaForOpenAI removes fields that many OpenAI-compatible providers
// reject (e.g. "additionalProperties": true) from a JSON Schema object.
func sanitizeSchemaForOpenAI(schema map[string]any) map[string]any {
	if schema == nil {
		return nil
	}
	out := make(map[string]any, len(schema))
	for k, v := range schema {
		switch k {
		case "additionalProperties":
			// Many providers reject additionalProperties:true; omit the field entirely.
			if boolVal, ok := v.(bool); ok && boolVal {
				continue
			}
			out[k] = v
		case "properties":
			if nested, ok := v.(map[string]any); ok {
				sanitized := make(map[string]any, len(nested))
				for pk, pv := range nested {
					if sub, ok := pv.(map[string]any); ok {
						sanitized[pk] = sanitizeSchemaForOpenAI(sub)
					} else {
						sanitized[pk] = pv
					}
				}
				out[k] = sanitized
			} else {
				out[k] = v
			}
		case "items":
			if sub, ok := v.(map[string]any); ok {
				out[k] = sanitizeSchemaForOpenAI(sub)
			} else {
				out[k] = v
			}
		default:
			out[k] = v
		}
	}
	return out
}
