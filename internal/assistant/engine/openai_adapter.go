package adk

import (
	"bufio"
	"context"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/jftrade/jftrade-main/internal/assistant/engine/providers"
	jfadkmodel "github.com/jftrade/jftrade-main/internal/assistant/model"
	adkmodel "google.golang.org/adk/v2/model"
	"google.golang.org/genai"
)

const GoogleADKModule = providers.GoogleADKModule

type (
	openAIChatMessage            = providers.OpenAIChatMessage
	openAIChatRequest            = providers.OpenAIChatRequest
	openAITool                   = providers.OpenAITool
	openAIToolFunction           = providers.OpenAIToolFunction
	openAIToolCall               = providers.OpenAIToolCall
	openAIChatResponse           = providers.OpenAIChatResponse
	openAIChatStreamDelta        = providers.OpenAIChatStreamDelta
	assistantExecutionResult     = providers.AssistantExecutionResult
	openAICompatibleADKModel     = providers.OpenAICompatibleADKModel
	openAIStreamAggregationState = providers.OpenAIStreamAggregationState
)

type openAIClient struct {
	impl providers.OpenAIClient
}

func newOpenAIClient() openAIClient {
	return openAIClient{}
}

func providerRequestTimeout(provider Provider) time.Duration {
	return providers.ProviderRequestTimeout(provider)
}

func (c openAIClient) chat(ctx context.Context, provider Provider, apiKey string, model string, messages []openAIChatMessage) (string, error) {
	return c.impl.Chat(ctx, provider, apiKey, model, messages)
}

func (c openAIClient) selectTools(
	ctx context.Context,
	provider Provider,
	apiKey string,
	model string,
	messages []openAIChatMessage,
	descriptors []ToolDescriptor,
) ([]jfadkmodel.ToolInvocation, error) {
	tools := openAIToolsFromDescriptors(descriptors)
	return c.impl.SelectTools(ctx, provider, apiKey, model, messages, tools)
}

func (c openAIClient) chatStream(
	ctx context.Context,
	provider Provider,
	apiKey string,
	model string,
	messages []openAIChatMessage,
	onDelta func(ChatDelta) error,
) (assistantExecutionResult, error) {
	return c.impl.ChatStream(ctx, provider, apiKey, model, messages, onDelta)
}

func (c openAIClient) readStreamingResponse(body io.Reader, onDelta func(ChatDelta) error) (assistantExecutionResult, error) {
	return c.impl.ReadStreamingResponse(body, onDelta)
}

func (c openAIClient) emitStructuredMessage(message openAIChatMessage, onDelta func(ChatDelta) error) (assistantExecutionResult, error) {
	return c.impl.EmitStructuredMessage(message, onDelta)
}

func newOpenAICompatibleADKModel(provider Provider, apiKey string, modelName string) adkmodel.LLM {
	return providers.NewOpenAICompatibleADKModel(provider, apiKey, modelName)
}

func providerResponseError(resp *http.Response) error {
	return providers.ProviderResponseError(resp)
}

func scanOpenAIEventStream(scanner *bufio.Scanner, dataLines *[]string, flushEvent func() error) error {
	return providers.ScanOpenAIEventStream(scanner, dataLines, flushEvent)
}

func yieldFinalOpenAIStreamResponse(state *openAIStreamAggregationState, yield func(*adkmodel.LLMResponse, error) bool) error {
	return providers.YieldFinalOpenAIStreamResponse(state, yield)
}

func openAIMessageToADKResponse(message openAIChatMessage, partial bool) (*adkmodel.LLMResponse, error) {
	return providers.OpenAIMessageToADKResponse(message, partial)
}

func openAIMessagesFromGenAI(content *genai.Content) []openAIChatMessage {
	return providers.OpenAIMessagesFromGenAI(content)
}

func genAIContentText(content *genai.Content) string {
	return providers.GenAIContentText(content)
}

func sanitizeSchemaForOpenAI(schema map[string]any) map[string]any {
	return jfadkmodel.SanitizeSchemaForOpenAI(schema)
}

func truncateBytes(s string, maxBytes int) string {
	return providers.TruncateBytes(s, maxBytes)
}

func normalizeMessagesForProvider(messages []openAIChatMessage) []openAIChatMessage {
	return providers.NormalizeMessagesForProvider(messages)
}

func normalizeProviderMessageRole(role string) string {
	return providers.NormalizeProviderMessageRole(role)
}

func removeToolCallFromMessage(message *openAIChatMessage, id string) {
	providers.RemoveToolCallFromMessage(message, id)
}

func trimMessagesForProvider(messages []openAIChatMessage, maxTotalBytes int) []openAIChatMessage {
	return providers.TrimMessagesForProvider(messages, maxTotalBytes)
}

func appendStreamChoice(
	splitter *providers.LegacyAssistantContentSplitter,
	replyBuilder *strings.Builder,
	reasoningBuilder *strings.Builder,
	content string,
	reasoningContent string,
	reasoning string,
	onDelta func(ChatDelta) error,
) error {
	return providers.AppendStreamChoice(splitter, replyBuilder, reasoningBuilder, content, reasoningContent, reasoning, onDelta)
}

func rawVisibleTextFromParts(parts []*genai.Part) (string, string) {
	return providers.RawVisibleTextFromParts(parts)
}

func partsFromReplyAndReasoning(reply string, reasoning string) []*genai.Part {
	return providers.PartsFromReplyAndReasoning(reply, reasoning)
}
