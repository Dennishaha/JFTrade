package rustmigration

import (
	"encoding/json"
	"os"
	"testing"

	jfadkruntime "github.com/jftrade/jftrade-main/internal/assistant/engine/workflowruntime"
	assistantmodel "github.com/jftrade/jftrade-main/internal/assistant/model"
)

const stage9AssistantAgentTemplatesVersion = "stage9.assistant-agent-templates.v1"

type stage9AssistantAgentTemplatesReference struct {
	Version   string                             `json:"version"`
	Templates []assistantmodel.AgentWriteRequest `json:"templates"`
}

// TestStage9AssistantAgentTemplatesReference captures the static Go owner
// output. AgentTemplates is intentionally callable without a runtime/store,
// so this corpus does not touch Assistant persistence or provider state.
func TestStage9AssistantAgentTemplatesReference(t *testing.T) {
	output := os.Getenv("JFTRADE_STAGE9_ASSISTANT_AGENT_TEMPLATES_REFERENCE")
	if output == "" {
		return
	}
	reference := stage9AssistantAgentTemplatesReference{
		Version:   stage9AssistantAgentTemplatesVersion,
		Templates: jfadkruntime.BuiltinAgentTemplates(),
	}
	if len(reference.Templates) != 1 {
		t.Fatalf("Go builtin agent templates = %d, want 1", len(reference.Templates))
	}
	contents, err := json.MarshalIndent(reference, "", "  ")
	if err != nil {
		t.Fatalf("encode Go agent-template reference: %v", err)
	}
	if err := os.WriteFile(output, append(contents, '\n'), 0o600); err != nil {
		t.Fatalf("write Go agent-template reference: %v", err)
	}
}
