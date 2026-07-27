package assembly

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	assistant "github.com/jftrade/jftrade-main/internal/assistant"
	jfadk "github.com/jftrade/jftrade-main/pkg/adk"
	jfsettings "github.com/jftrade/jftrade-main/pkg/jftsettings"
)

func TestOpenBuildsToolsServiceAndIdempotentLifecycle(t *testing.T) {
	paths := testRuntimePaths(t.TempDir())
	tools := ToolDeps{
		SystemStatus: func() map[string]any {
			return map[string]any{"ready": true}
		},
	}
	handle, err := Open(Options{
		Paths: paths,
		RuntimeLimits: func() RuntimeLimits {
			return RuntimeLimits{RunTimeout: time.Second}
		},
		Tools: &tools,
	})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if !handle.Available() || handle.Service() == nil || !handle.HasTool("system.status") {
		t.Fatalf("assembly = %#v", handle)
	}
	if handle.MCPStatus().Running {
		t.Fatalf("initial MCP status = %#v", handle.MCPStatus())
	}
	if err := handle.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if err := handle.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}
}

func TestRuntimeDatabaseProbesUseProvidedLayout(t *testing.T) {
	paths := testRuntimePaths(t.TempDir())
	if err := ProbeRuntimeDatabase(paths); err != nil {
		t.Fatalf("ProbeRuntimeDatabase: %v", err)
	}
	if err := ProbeSessionDatabase(paths); err != nil {
		t.Fatalf("ProbeSessionDatabase: %v", err)
	}
}

func TestOpenOwnsApplicationToolRegistration(t *testing.T) {
	tools := ToolDeps{SystemStatus: func() map[string]any { return map[string]any{"ready": true} }}
	handle, err := Open(Options{Paths: testRuntimePaths(t.TempDir()), Tools: &tools})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() {
		if closeErr := handle.Close(); closeErr != nil {
			t.Fatalf("Close: %v", closeErr)
		}
	})
	if !handle.HasTool("system.status") {
		t.Fatal("system.status was not registered from Options.Tools")
	}
	if !handle.HasTool("strategy.research_backtest") {
		t.Fatal("strategy.research_backtest was not registered from Options.Tools")
	}
}

func TestHandleExposesNarrowAuditAndToolOperations(t *testing.T) {
	handle, err := Open(Options{Paths: testRuntimePaths(t.TempDir())})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() {
		if closeErr := handle.Close(); closeErr != nil {
			t.Fatalf("Close: %v", closeErr)
		}
	})

	handle.RecordAudit(
		t.Context(),
		"integration.connected",
		"provider-1",
		"integration connected",
		map[string]any{"transport": "local"},
	)
	events, err := handle.Service().GetAudit(t.Context(), assistant.AuditQuery{
		Kind:      "integration.connected",
		SubjectID: "provider-1",
	})
	if err != nil {
		t.Fatalf("GetAudit: %v", err)
	}
	if len(events) != 1 || events[0].Detail != "integration connected" {
		t.Fatalf("audit events = %#v", events)
	}

	err = handle.RegisterTool(jfadk.ToolDescriptor{
		Name:        "integration.echo",
		DisplayName: "Echo",
	}, func(_ context.Context, input map[string]any) (any, error) {
		return input["value"], nil
	})
	if err != nil {
		t.Fatalf("RegisterTool: %v", err)
	}
	tool, ok := handle.Tool("integration.echo")
	if !ok || tool.Handler == nil {
		t.Fatalf("registered tool = %#v, %v", tool, ok)
	}
	output, err := tool.Handler(t.Context(), map[string]any{"value": "ready"})
	if err != nil || output != "ready" {
		t.Fatalf("tool output = %#v, err = %v", output, err)
	}
}

func testRuntimePaths(dir string) Paths {
	return Paths{
		Database: filepath.Join(dir, "adk.db"),
		Session:  filepath.Join(dir, "adk-sessions.db"),
		Secrets:  filepath.Join(dir, "secrets"),
		Skills:   filepath.Join(dir, "skills"),
	}
}

func TestNilHandleLifecycleIsSafe(t *testing.T) {
	var handle *Handle
	if handle.Available() || handle.Service() != nil || handle.HasTool("system.status") {
		t.Fatal("nil handle exposed a dependency")
	}
	if _, ok := handle.Tool("system.status"); ok {
		t.Fatal("nil handle returned a tool")
	}
	if err := handle.RegisterTool(jfadk.ToolDescriptor{Name: "test"}, func(context.Context, map[string]any) (any, error) {
		return nil, nil
	}); err == nil {
		t.Fatal("nil RegisterTool error = nil")
	}
	handle.RecordAudit(t.Context(), "ignored", "", "", nil)
	handle.StartWorkflowScheduler(t.Context())
	handle.HandleWorkflowEvent(t.Context(), WorkflowEvent{Type: "ignored"})
	if got := handle.WatchedWorkflowInstruments(t.Context()); got != nil {
		t.Fatalf("nil watched instruments = %#v", got)
	}
	if err := handle.Close(); err != nil {
		t.Fatalf("nil Close: %v", err)
	}
	if err := handle.ReconfigureMCP(jfsettings.MCPServerSettings{}); err == nil {
		t.Fatal("nil ReconfigureMCP error = nil")
	}
	if handle.MCPStatus().LastError == "" {
		t.Fatal("nil MCPStatus missing diagnostic")
	}
}
