// Package workflowruntime exposes the Assistant runtime composition seams.
// Domain contracts, status constants, and normalization belong to model;
// this package is limited to runtime, store, tool, session, and executor wiring.
package workflowruntime

import (
	"context"

	jfadk "github.com/jftrade/jftrade-main/internal/assistant/engine"
	enginepersistence "github.com/jftrade/jftrade-main/internal/assistant/engine/persistence"
	workflowexec "github.com/jftrade/jftrade-main/internal/assistant/engine/workflowexec"
	assistantmodel "github.com/jftrade/jftrade-main/internal/assistant/model"
	adksession "google.golang.org/adk/v2/session"
)

type (
	Runtime                  = jfadk.Runtime
	Store                    = jfadk.Store
	ToolRegistry             = jfadk.ToolRegistry
	RegisteredTool           = jfadk.RegisteredTool
	ToolFunc                 = jfadk.ToolFunc
	LocalMCPHandler          = jfadk.LocalMCPHandler
	SQLiteSessionService     = enginepersistence.SQLiteSessionService
	DeletedConfigIDs         = jfadk.DeletedConfigIDs
	WorkflowCanvasRunRequest = jfadk.WorkflowCanvasRunRequest
)

var LocalMCPReadOnlyToolNames = jfadk.LocalMCPReadOnlyToolNames

func NewRuntime(store *Store, tools *ToolRegistry) *Runtime {
	runtime := jfadk.NewRuntime(store, tools)
	runtime.SetWorkflowExecutor(workflowexec.NewWorkflowExecutor(runtime))
	return runtime
}

func NewRuntimeWithSessionService(store *Store, tools *ToolRegistry, sessionService adksession.Service) *Runtime {
	runtime := jfadk.NewRuntimeWithSessionService(store, tools, sessionService)
	runtime.SetWorkflowExecutor(workflowexec.NewWorkflowExecutor(runtime))
	return runtime
}

func NewStore(dbPath string, secretsPath string, skillsPath string) (*Store, error) {
	return jfadk.NewStore(dbPath, secretsPath, skillsPath)
}

func NewToolRegistry() *ToolRegistry {
	return jfadk.NewToolRegistry()
}

func NewSQLiteSessionService(path string) (*SQLiteSessionService, error) {
	return enginepersistence.NewSQLiteSessionService(path)
}

func CloseSessionService(service adksession.Service) error {
	return enginepersistence.CloseSessionService(service)
}

func NewLocalMCPHandler(runtime *Runtime) (*LocalMCPHandler, error) {
	return jfadk.NewLocalMCPHandler(runtime)
}

func ToolDescriptorsForAgent(agent assistantmodel.Agent, registry *ToolRegistry) []assistantmodel.ToolDescriptor {
	return jfadk.ToolDescriptorsForAgent(agent, registry)
}

func ToolInvocationSessionID(ctx context.Context) (string, bool) {
	return jfadk.ToolInvocationSessionID(ctx)
}

func BuiltinAgentTemplates() []assistantmodel.AgentWriteRequest {
	return jfadk.BuiltinAgentTemplates()
}

func BuiltinAgentTemplate(id string) (assistantmodel.AgentWriteRequest, bool) {
	return jfadk.BuiltinAgentTemplate(id)
}

func IsBuiltinAgentID(id string) bool {
	return jfadk.IsBuiltinAgentID(id)
}

func IsPrimaryBuiltinAgentID(id string) bool {
	return jfadk.IsPrimaryBuiltinAgentID(id)
}

// WorkflowExecutor is injected into the engine root by the composition layer.
type WorkflowExecutor = workflowexec.WorkflowExecutor

func NewWorkflowExecutor(runtime assistantmodel.WorkflowExecutorRuntime) *WorkflowExecutor {
	return workflowexec.NewWorkflowExecutor(runtime)
}
