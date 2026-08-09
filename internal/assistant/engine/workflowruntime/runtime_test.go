package workflowruntime

import (
	"context"
	"path/filepath"
	"testing"

	jfadk "github.com/jftrade/jftrade-main/internal/assistant/engine"
)

func TestFacadeConstructorsAndSessionService(t *testing.T) {
	runtime := NewRuntime(nil, nil)
	if runtime == nil {
		t.Fatal("NewRuntime returned nil")
	}
	if got := NewRuntimeWithSessionService(nil, nil, nil); got == nil {
		t.Fatal("NewRuntimeWithSessionService returned nil")
	}
	if got := NewToolRegistry(); got == nil {
		t.Fatal("NewToolRegistry returned nil")
	}
	dir := t.TempDir()
	store, err := NewStore(filepath.Join(dir, "adk.db"), filepath.Join(dir, "secrets.json"), filepath.Join(dir, "skills"))
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	if store == nil {
		t.Fatal("NewStore returned nil store")
	}
	t.Cleanup(func() { _ = store.Close() })
	sessionService, err := NewSQLiteSessionService(filepath.Join(dir, "sessions.db"))
	if err != nil {
		t.Fatalf("NewSQLiteSessionService: %v", err)
	}
	if sessionService == nil {
		t.Fatal("NewSQLiteSessionService returned nil")
	}
	if err := CloseSessionService(sessionService); err != nil {
		t.Fatalf("CloseSessionService: %v", err)
	}

	_, _ = NewLocalMCPHandler(runtime)
	if _, err := NewLocalMCPHandler(nil); err == nil {
		t.Fatal("NewLocalMCPHandler(nil) should fail")
	}
}

func TestFacadeNormalizersAndHelpers(t *testing.T) {
	_ = NormalizeRun(jfadk.Run{})
	_ = NormalizeAgent(jfadk.Agent{})
	_ = NormalizeTimelineEntry(jfadk.TimelineEntry{})
	_ = NormalizeChatResponse(jfadk.ChatResponse{})
	_ = NormalizeWorkflowDefinition(jfadk.WorkflowDefinition{})
	_ = NormalizeWorkflowTrigger(jfadk.WorkflowTrigger{})
	_ = NormalizeWorkflowTriggerLog(jfadk.WorkflowTriggerLog{})
	_ = NormalizeApprovalResolution(jfadk.ApprovalResolution{})
	_ = NormalizeSessionsResponse(jfadk.SessionsResponse{})
	_, _, _ = NormalizeChatRequestIdentity(jfadk.ChatRequest{})
	if got := ToolRequiredSkillNames(jfadk.ToolDescriptor{}); got == nil {
		t.Fatal("ToolRequiredSkillNames returned nil")
	}
	_ = ToolRequiresApproval(jfadk.ToolDescriptor{}, PermissionModeApproval)
	_ = ToolRequiresApproval(jfadk.ToolDescriptor{}, PermissionModeAll)
	if got := ToolDescriptorsForAgent(jfadk.Agent{}, nil); got != nil {
		t.Fatalf("ToolDescriptorsForAgent with nil registry = %#v", got)
	}
	if _, ok := ToolInvocationSessionID(context.Background()); ok {
		t.Fatal("empty context should not carry a tool invocation session")
	}
	if got := InputRequestErrorKind(nil); got == "" {
		t.Fatal("InputRequestErrorKind(nil) should return a kind")
	}
	if len(BuiltinAgentTemplates()) == 0 {
		t.Fatal("BuiltinAgentTemplates should not be empty")
	}
	if _, ok := BuiltinAgentTemplate("missing-agent"); ok {
		t.Fatal("missing builtin agent template should not be found")
	}
	if IsBuiltinAgentID("") {
		t.Fatal("empty agent id should not be builtin")
	}
	if IsPrimaryBuiltinAgentID("") {
		t.Fatal("empty agent id should not be primary builtin")
	}
}

func TestFacadeConstantsRemainBoundToEngine(t *testing.T) {
	if PermissionModeApproval != jfadk.PermissionModeApproval ||
		WorkModeChat != jfadk.WorkModeChat ||
		RunStatusRunning != jfadk.RunStatusRunning ||
		ToolIdempotencyFailClosed != jfadk.ToolIdempotencyFailClosed {
		t.Fatal("facade constants drifted from engine")
	}
}
