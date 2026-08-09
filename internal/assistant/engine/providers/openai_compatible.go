package providers

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
	"sort"
	"strings"

	jfadkmodel "github.com/jftrade/jftrade-main/internal/assistant/model"
	"github.com/jftrade/jftrade-main/pkg/besteffort"
	"google.golang.org/adk/v2/model"
	"google.golang.org/genai"
)

const GoogleADKModule = "google.golang.org/adk/v2"

type OpenAICompatibleADKModel struct {
	Provider jfadkmodel.Provider
	APIKey   string
	Model    string
}

func NewOpenAICompatibleADKModel(provider jfadkmodel.Provider, apiKey string, modelName string) model.LLM {
	return &OpenAICompatibleADKModel{
		Provider: provider,
		APIKey:   strings.TrimSpace(apiKey),
		Model:    jfadkmodel.DefaultString(modelName, provider.Model),
	}
}

func (m *OpenAICompatibleADKModel) Name() string {
	return m.Model
}

func (m *OpenAICompatibleADKModel) GenerateContent(ctx context.Context, req *model.LLMRequest, stream bool) iter.Seq2[*model.LLMResponse, error] {
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
			if err := m.GenerateStream(ctx, req, safeYield); err != nil && !stopped && !errors.Is(err, context.Canceled) {
				safeYield(nil, err)
			}
			return
		}
		response, err := m.generate(ctx, req)
		safeYield(response, err)
	}
}

func (m *OpenAICompatibleADKModel) generate(ctx context.Context, req *model.LLMRequest) (*model.LLMResponse, error) {
	payload := m.BuildChatRequest(req, false)
	return m.DoGenerate(ctx, payload)
}

func (m *OpenAICompatibleADKModel) GenerateStream(
	ctx context.Context,
	req *model.LLMRequest,
	yield func(*model.LLMResponse, error) bool,
) error {
	payload := m.BuildChatRequest(req, true)
	httpReq, err := m.NewChatRequest(ctx, payload)
	if err != nil {
		return err
	}
	resp, err := NewHTTPClient(ProviderRequestTimeout(m.Provider)).Do(httpReq)
	if err != nil {
		return err
	}
	defer func() { besteffort.LogError(resp.Body.Close()) }()
	if err := ProviderResponseError(resp); err != nil {
		return err
	}
	if !strings.Contains(strings.ToLower(resp.Header.Get("Content-Type")), "text/event-stream") {
		return m.generateStreamFallbackResponse(resp.Body, yield)
	}
	return m.consumeChatEventStream(resp.Body, yield)
}

func ProviderResponseError(resp *http.Response) error {
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}
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

func (m *OpenAICompatibleADKModel) generateStreamFallbackResponse(
	body io.Reader,
	yield func(*model.LLMResponse, error) bool,
) error {
	response, err := m.DecodeChatResponse(body)
	if err != nil {
		return err
	}
	yield(response, nil)
	return nil
}

func (m *OpenAICompatibleADKModel) consumeChatEventStream(
	body io.Reader,
	yield func(*model.LLMResponse, error) bool,
) error {
	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 0, 64<<10), 2<<20)
	var dataLines []string
	state := OpenAIStreamAggregationState{}

	flushEvent := func() error {
		err := ConsumeOpenAIEventData(dataLines, &state, yield)
		dataLines = dataLines[:0]
		return err
	}
	if err := ScanOpenAIEventStream(scanner, &dataLines, flushEvent); err != nil {
		return err
	}
	return YieldFinalOpenAIStreamResponse(&state, yield)
}

func ConsumeOpenAIEventData(
	dataLines []string,
	state *OpenAIStreamAggregationState,
	yield func(*model.LLMResponse, error) bool,
) error {
	if len(dataLines) == 0 {
		return nil
	}
	payload := strings.Join(dataLines, "\n")
	if strings.TrimSpace(payload) == "" {
		return nil
	}
	if strings.TrimSpace(payload) == "[DONE]" {
		return io.EOF
	}
	var parsed OpenAIChatStreamResponse
	if err := json.Unmarshal([]byte(payload), &parsed); err != nil {
		return fmt.Errorf("decode OpenAI-compatible ADK stream chunk: %w", err)
	}
	if parsed.Error != nil && parsed.Error.Message != "" {
		return fmt.Errorf("provider returned: %s", parsed.Error.Message)
	}
	for _, choice := range parsed.Choices {
		if err := state.Consume(choice.Delta, yield); err != nil {
			return err
		}
		if err := state.ConsumeMessage(choice.Message, yield); err != nil {
			return err
		}
	}
	return nil
}

func ScanOpenAIEventStream(scanner *bufio.Scanner, dataLines *[]string, flushEvent func() error) error {
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			err := flushEvent()
			if errors.Is(err, io.EOF) {
				break
			}
			if err != nil {
				return err
			}
			continue
		}
		if after, ok := strings.CutPrefix(line, "data:"); ok {
			*dataLines = append(*dataLines, strings.TrimSpace(after))
		}
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	if err := flushEvent(); err != nil && !errors.Is(err, io.EOF) {
		return err
	}
	return nil
}

func YieldFinalOpenAIStreamResponse(state *OpenAIStreamAggregationState, yield func(*model.LLMResponse, error) bool) error {
	final, err := state.FinalResponse()
	if err != nil {
		return err
	}
	if final == nil {
		return fmt.Errorf("provider returned an empty reply")
	}
	yield(final, nil)
	return nil
}

func (m *OpenAICompatibleADKModel) BuildChatRequest(req *model.LLMRequest, stream bool) OpenAIChatRequest {
	messages := make([]OpenAIChatMessage, 0, len(req.Contents)+1)
	if req.Config != nil && req.Config.SystemInstruction != nil {
		if text := GenAIContentText(req.Config.SystemInstruction); text != "" {
			messages = append(messages, OpenAIChatMessage{Role: "system", Content: text})
		}
	}
	for _, content := range req.Contents {
		messages = append(messages, OpenAIMessagesFromGenAI(content)...)
	}
	messages = TrimMessagesForProvider(messages, MaxProviderPayloadBytes)

	payload := OpenAIChatRequest{
		Model:       jfadkmodel.DefaultString(req.Model, m.Model),
		Messages:    messages,
		Temperature: 0.2,
		Stream:      stream,
		Tools:       OpenAIToolsFromGenAIConfig(req.Config),
	}
	if len(payload.Tools) > 0 {
		payload.ToolChoice = "auto"
	}
	return payload
}

func (m *OpenAICompatibleADKModel) DoGenerate(ctx context.Context, payload OpenAIChatRequest) (*model.LLMResponse, error) {
	httpReq, err := m.NewChatRequest(ctx, payload)
	if err != nil {
		return nil, err
	}
	resp, err := NewHTTPClient(ProviderRequestTimeout(m.Provider)).Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer func() { besteffort.LogError(resp.Body.Close()) }()
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
	return m.DecodeChatResponse(resp.Body)
}

func (m *OpenAICompatibleADKModel) NewChatRequest(ctx context.Context, payload OpenAIChatRequest) (*http.Request, error) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	endpoint := strings.TrimRight(m.Provider.BaseURL, "/") + "/chat/completions"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if m.APIKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+m.APIKey)
	}
	for key, value := range m.Provider.DefaultHeaders {
		if strings.TrimSpace(key) != "" && strings.TrimSpace(value) != "" {
			httpReq.Header.Set(key, value)
		}
	}
	return httpReq, nil
}

func (m *OpenAICompatibleADKModel) DecodeChatResponse(body io.Reader) (*model.LLMResponse, error) {
	raw, err := io.ReadAll(io.LimitReader(body, 4<<20))
	if err != nil {
		return nil, err
	}
	var parsed OpenAIChatResponse
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil, fmt.Errorf("decode OpenAI-compatible ADK response: %w", err)
	}
	if parsed.Error != nil && parsed.Error.Message != "" {
		return nil, fmt.Errorf("provider returned: %s", parsed.Error.Message)
	}
	if len(parsed.Choices) == 0 {
		return nil, fmt.Errorf("provider returned no choices")
	}
	return OpenAIMessageToADKResponse(parsed.Choices[0].Message, false)
}

func OpenAIMessageToADKResponse(message OpenAIChatMessage, partial bool) (*model.LLMResponse, error) {
	replyText, reasoningText := ExtractVisibleAndReasoningText(message.Content, message.ReasoningContent, message.Reasoning)
	parts := make([]*genai.Part, 0, len(message.ToolCalls)+2)
	if partial {
		parts = append(parts, RawPartsFromReplyAndReasoning(replyText, reasoningText)...)
	} else {
		parts = append(parts, PartsFromReplyAndReasoning(replyText, reasoningText)...)
	}
	for _, call := range message.ToolCalls {
		args := map[string]any{}
		if strings.TrimSpace(call.Function.Arguments) != "" {
			if err := json.Unmarshal([]byte(call.Function.Arguments), &args); err != nil {
				return nil, fmt.Errorf("decode tool arguments for %s: %w", call.Function.Name, err)
			}
		}
		// Restore the tool name from the sanitized form (dots → hyphens → dots)
		// because OpenAIToolsFromGenAIConfig sanitizes names before sending to the
		// provider, and the provider echoes back the sanitized name.
		toolName := RestoreToolNameFromOpenAI(strings.TrimSpace(call.Function.Name))
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

type OpenAIStreamAggregationState struct {
	Content   strings.Builder
	Reasoning strings.Builder
	ToolCalls []OpenAIToolCall
}

func (s *OpenAIStreamAggregationState) Consume(delta OpenAIChatStreamDelta, yield func(*model.LLMResponse, error) bool) error {
	if delta.Content != "" || delta.ReasoningContent != "" || delta.Reasoning != "" {
		replyText, reasoningText := ExtractVisibleAndReasoningText(delta.Content, delta.ReasoningContent, delta.Reasoning)
		s.Content.WriteString(replyText)
		s.Reasoning.WriteString(reasoningText)
		response, err := OpenAIMessageToADKResponse(OpenAIChatMessage{
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
		s.MergeToolCalls(delta.ToolCalls)
	}
	return nil
}

func (s *OpenAIStreamAggregationState) ConsumeMessage(message OpenAIChatMessage, yield func(*model.LLMResponse, error) bool) error {
	if message.Content != "" || message.ReasoningContent != "" || message.Reasoning != "" {
		replyText, reasoningText := ExtractVisibleAndReasoningText(message.Content, message.ReasoningContent, message.Reasoning)
		s.Content.WriteString(replyText)
		s.Reasoning.WriteString(reasoningText)
		response, err := OpenAIMessageToADKResponse(OpenAIChatMessage{
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
		s.MergeToolCalls(message.ToolCalls)
	}
	return nil
}

func (s *OpenAIStreamAggregationState) MergeToolCalls(chunks []OpenAIToolCall) {
	for _, chunk := range chunks {
		index := max(chunk.Index, 0)
		for len(s.ToolCalls) <= index {
			s.ToolCalls = append(s.ToolCalls, OpenAIToolCall{})
		}
		current := &s.ToolCalls[index]
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

func (s *OpenAIStreamAggregationState) FinalResponse() (*model.LLMResponse, error) {
	if s.Content.Len() == 0 && len(s.ToolCalls) == 0 {
		if s.Reasoning.Len() == 0 {
			return nil, nil
		}
	}
	if s.Content.Len() == 0 && s.Reasoning.Len() == 0 && len(s.ToolCalls) == 0 {
		return nil, nil
	}
	return OpenAIMessageToADKResponse(OpenAIChatMessage{
		Content:          s.Content.String(),
		ReasoningContent: s.Reasoning.String(),
		ToolCalls:        s.ToolCalls,
	}, false)
}

func OpenAIMessagesFromGenAI(content *genai.Content) []OpenAIChatMessage {
	if content == nil {
		return nil
	}
	role := "user"
	if content.Role == genai.RoleModel {
		role = "assistant"
	}
	var text strings.Builder
	var reasoning strings.Builder
	var calls []OpenAIToolCall
	var messages []OpenAIChatMessage
	for _, part := range content.Parts {
		if part.Text != "" {
			if part.Thought {
				reasoning.WriteString(part.Text)
			} else {
				text.WriteString(part.Text)
			}
		}
		if part.FunctionCall != nil {
			rawArgs, jftradeErr2 := json.Marshal(part.FunctionCall.Args)
			besteffort.LogError(jftradeErr2)
			call := OpenAIToolCall{ID: part.FunctionCall.ID, Type: "function"}
			call.Function.Name = part.FunctionCall.Name
			call.Function.Arguments = string(rawArgs)
			calls = append(calls, call)
		}
		if part.FunctionResponse != nil {
			rawResponse, jftradeErr4 := json.Marshal(part.FunctionResponse.Response)
			besteffort.LogError(jftradeErr4)
			messages = append(messages, OpenAIChatMessage{
				Role:       "tool",
				Content:    string(rawResponse),
				Name:       part.FunctionResponse.Name,
				ToolCallID: part.FunctionResponse.ID,
			})
		}
	}
	if text.Len() > 0 || reasoning.Len() > 0 || len(calls) > 0 {
		messages = append([]OpenAIChatMessage{{
			Role:             role,
			Content:          text.String(),
			ReasoningContent: reasoning.String(),
			ToolCalls:        calls,
		}}, messages...)
	}
	return messages
}

func OpenAIToolsFromGenAIConfig(config *genai.GenerateContentConfig) []OpenAITool {
	if config == nil {
		return nil
	}
	var result []OpenAITool
	for _, item := range config.Tools {
		for _, declaration := range item.FunctionDeclarations {
			parameters := jfadkmodel.OptionalTypeAssertion[map[string]any](declaration.ParametersJsonSchema)
			if parameters == nil {
				raw, jftradeErr3 := json.Marshal(declaration.ParametersJsonSchema)
				besteffort.LogError(jftradeErr3)
				jftradeErr1 := json.Unmarshal(raw, &parameters)
				besteffort.LogError(jftradeErr1)
			}
			parameters = SanitizeSchemaForOpenAI(parameters)
			result = append(result, OpenAITool{
				Type: "function",
				Function: OpenAIToolFunction{
					Name: SanitizeToolNameForOpenAI(declaration.Name), Description: declaration.Description, Parameters: parameters,
				},
			})
		}
	}
	sort.SliceStable(result, func(i int, j int) bool {
		return result[i].Function.Name < result[j].Function.Name
	})
	return result
}

func GenAIContentText(content *genai.Content) string {
	if content == nil {
		return ""
	}
	var builder strings.Builder
	for _, part := range content.Parts {
		builder.WriteString(part.Text)
	}
	return strings.TrimSpace(builder.String())
}

// SanitizeSchemaForOpenAI removes fields that many OpenAI-compatible providers
// reject (e.g. "additionalProperties": true) from a JSON Schema object.
func SanitizeSchemaForOpenAI(schema map[string]any) map[string]any {
	return jfadkmodel.SanitizeSchemaForOpenAI(schema)
}
