package adk

import (
	"context"
	"strings"

	"github.com/jftrade/jftrade-main/internal/assistant/engine/providers"
	jadkmodel "github.com/jftrade/jftrade-main/internal/assistant/model"
	adkmodel "google.golang.org/adk/v2/model"
	"google.golang.org/genai"
)

const GoogleADKModule = providers.GoogleADKModule

type assistantExecutionResult = jadkmodel.AssistantExecutionResult

// responsesClient owns the small non-agent Responses call used for context
// compaction. It deliberately does not accept a reasoning effort so background
// maintenance never injects an explicit reasoning field.
type responsesClient struct{}

func newResponsesClient() responsesClient { return responsesClient{} }

func (responsesClient) generateText(
	ctx context.Context,
	provider Provider,
	apiKey string,
	modelName string,
	system string,
	prompt string,
) (string, error) {
	llm, err := providers.NewOpenAIResponsesADKModel(ctx, provider, apiKey, modelName)
	if err != nil {
		return "", err
	}
	request := &adkmodel.LLMRequest{
		Config: &genai.GenerateContentConfig{
			SystemInstruction: genai.NewContentFromText(system, genai.RoleUser),
		},
		Contents: []*genai.Content{genai.NewContentFromText(prompt, genai.RoleUser)},
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

func sanitizeSchemaForOpenAI(schema map[string]any) map[string]any {
	return jadkmodel.SanitizeSchemaForOpenAI(schema)
}

func rawVisibleTextFromParts(parts []*genai.Part) (string, string) {
	return providers.RawVisibleTextFromParts(parts)
}

func partsFromReplyAndReasoning(reply string, reasoning string) []*genai.Part {
	return providers.PartsFromReplyAndReasoning(reply, reasoning)
}
