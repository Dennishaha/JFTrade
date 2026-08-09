package servercoretest

import (
	"encoding/json"
	"net/http"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/jftrade/jftrade-main/internal/app/apiserver/servercore"
)

func TestOpenAPIPreservesLegacySchemaNames(t *testing.T) {
	store, err := servercore.NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	srv := newHTTPTestServer(t, store)

	resp, err := jftradeTestHTTPGet(t, srv.URL+"/swagger/doc.json")
	if err != nil {
		t.Fatalf("GET /swagger/doc.json: %v", err)
	}
	defer func() { jftradeCheckTestError(t, resp.Body.Close()) }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("/swagger/doc.json status = %d", resp.StatusCode)
	}

	var spec struct {
		Definitions map[string]json.RawMessage `json:"definitions"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&spec); err != nil {
		t.Fatalf("parse /swagger/doc.json: %v", err)
	}

	required := []string{
		"adk.Agent", "adk.AgentWriteRequest", "adk.Approval", "adk.ApprovalResolution",
		"adk.AuditEvent", "adk.ChatResponse", "adk.InputAnswer", "adk.InputOption",
		"adk.InputQuestion", "adk.InputRequest", "adk.InputResolution", "adk.MemoryEntry",
		"adk.Provider", "adk.Run", "adk.RunOptions", "adk.RunUsage", "adk.Session",
		"adk.SessionComposerState", "adk.SessionContextBreakdown", "adk.SessionContextSnapshot",
		"adk.SessionsResponse", "adk.Skill", "adk.Task", "adk.TimelineEntry", "adk.ToolCall",
		"adk.ToolDescriptor", "adk.TranscriptEntry", "adk.WorkflowCanvasEdge",
		"adk.WorkflowCanvasGraph", "adk.WorkflowCanvasNode", "adk.WorkflowCanvasPoint",
		"adk.WorkflowDefinition", "adk.WorkflowNodeRun", "adk.WorkflowResult",
		"adk.WorkflowStepState", "adk.WorkflowTrigger", "adk.WorkflowTriggerLog",
		"servercore.WebSessionData", "servercore.webLoginRequest",
	}
	missing := make([]string, 0)
	for _, name := range required {
		if _, ok := spec.Definitions[name]; !ok {
			missing = append(missing, name)
		}
	}
	if len(missing) > 0 {
		t.Fatalf("OpenAPI is missing stable schema names:\n%s", strings.Join(missing, "\n"))
	}

	leaked := make([]string, 0)
	for name := range spec.Definitions {
		if strings.HasPrefix(name, "model.") || strings.HasPrefix(name, "workflowruntime.") ||
			name == "webaccess.WebSessionData" || name == "webaccess.webLoginRequest" {
			leaked = append(leaked, name)
		}
	}
	if len(leaked) > 0 {
		sort.Strings(leaked)
		t.Fatalf("OpenAPI exposes internal package ownership through schema names:\n%s", strings.Join(leaked, "\n"))
	}
}
