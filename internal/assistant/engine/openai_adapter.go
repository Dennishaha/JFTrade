package adk

import (
	"context"
	"strings"

	"github.com/jftrade/jftrade-main/internal/assistant/engine/completionreview"
	"github.com/jftrade/jftrade-main/internal/assistant/engine/providers"
	jadkmodel "github.com/jftrade/jftrade-main/internal/assistant/model"
	adkmodel "google.golang.org/adk/v2/model"
	"google.golang.org/genai"
)

const GoogleADKModule = providers.GoogleADKModule

type assistantExecutionResult = jadkmodel.AssistantExecutionResult

// responsesClient owns bounded non-agent Responses calls used for context
// compaction and chat completion review. It deliberately does not accept a
// reasoning effort, so these supporting calls never inject an explicit level.
type responsesClient struct{}

type generatedTextResult struct {
	Text string
}

type responseTextOptions struct {
	MaxOutputTokens int32
	JSONSchema      any
}

func newResponsesClient() responsesClient { return responsesClient{} }

func (c responsesClient) generateText(
	ctx context.Context,
	provider Provider,
	apiKey string,
	modelName string,
	system string,
	prompt string,
) (string, error) {
	result, err := c.generateTextWithOptions(ctx, provider, apiKey, modelName, system, prompt, responseTextOptions{})
	return result.Text, err
}

func (c responsesClient) generateCompletionReview(
	ctx context.Context,
	provider Provider,
	apiKey string,
	modelName string,
	system string,
	prompt string,
) (generatedTextResult, error) {
	return c.generateTextWithOptions(ctx, provider, apiKey, modelName, system, prompt, responseTextOptions{
		MaxOutputTokens: 1200,
		JSONSchema:      completionreview.JSONSchema(),
	})
}

func (responsesClient) generateTextWithOptions(
	ctx context.Context,
	provider Provider,
	apiKey string,
	modelName string,
	system string,
	prompt string,
	options responseTextOptions,
) (generatedTextResult, error) {
	llm, err := providers.NewOpenAIResponsesADKModel(ctx, provider, apiKey, modelName)
	if err != nil {
		return generatedTextResult{}, err
	}
	config := &genai.GenerateContentConfig{
		SystemInstruction: genai.NewContentFromText(system, genai.RoleUser),
		MaxOutputTokens:   options.MaxOutputTokens,
	}
	if options.JSONSchema != nil {
		config.ResponseMIMEType = "application/json"
		config.ResponseJsonSchema = options.JSONSchema
	}
	request := &adkmodel.LLMRequest{Config: config, Contents: []*genai.Content{genai.NewContentFromText(prompt, genai.RoleUser)}}
	var reply strings.Builder
	for response, responseErr := range llm.GenerateContent(ctx, request, false) {
		if responseErr != nil {
			return generatedTextResult{Text: strings.TrimSpace(reply.String())}, responseErr
		}
		if response == nil {
			continue
		}
		if response.Content == nil {
			continue
		}
		for _, part := range response.Content.Parts {
			if part != nil {
				reply.WriteString(part.Text)
			}
		}
	}
	return generatedTextResult{Text: strings.TrimSpace(reply.String())}, nil
}

func sanitizeSchemaForOpenAI(schema map[string]any) map[string]any {
	return jadkmodel.SanitizeSchemaForOpenAI(schema)
}

func rawVisibleTextFromParts(parts []*genai.Part) (string, string) {
	return providers.RawVisibleTextFromParts(parts)
}

func partsFromReplyAndReasoning(reply string, reasoning string) []*genai.Part {
	return providers.PartsFromReplyAndReasoning(reply, reasoning)
}
