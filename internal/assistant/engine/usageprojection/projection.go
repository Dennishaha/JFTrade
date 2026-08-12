package usageprojection

import (
	"strings"

	"github.com/jftrade/jftrade-main/internal/assistant/model"
	"google.golang.org/genai"
)

// Tracker projects each final ADK usage event exactly once onto persisted run usage.
type Tracker struct {
	seen map[string]struct{}
}

func (t *Tracker) Accumulate(eventID string, partial bool, metadata *genai.GenerateContentResponseUsageMetadata, current *model.RunUsage) (*model.RunUsage, bool) {
	eventID = strings.TrimSpace(eventID)
	if partial || metadata == nil || eventID == "" {
		return current, false
	}
	if t.seen == nil {
		t.seen = make(map[string]struct{})
	}
	if _, exists := t.seen[eventID]; exists {
		return current, false
	}
	t.seen[eventID] = struct{}{}
	usage := model.RunUsage{}
	if current != nil {
		usage = *current
	}
	usage.ModelCalls++
	usage.TokensIn += max(0, int(metadata.PromptTokenCount))
	usage.TokensOut += max(0, int(metadata.CandidatesTokenCount))
	return &usage, true
}
