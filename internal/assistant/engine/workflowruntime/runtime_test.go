package workflowruntime

import (
	"context"
	"path/filepath"
	"testing"

	assistantmodel "github.com/jftrade/jftrade-main/internal/assistant/model"
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

func TestFacadeKeepsRuntimeAssemblyHelpers(t *testing.T) {
	if got := ToolDescriptorsForAgent(assistantmodel.Agent{}, nil); got != nil {
		t.Fatalf("ToolDescriptorsForAgent with nil registry = %#v", got)
	}
	if _, ok := ToolInvocationSessionID(context.Background()); ok {
		t.Fatal("empty context should not carry a tool invocation session")
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
