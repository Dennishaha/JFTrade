package workflowexec

import (
	jfadk "github.com/jftrade/jftrade-main/internal/assistant/engine"
)

// The executor tests build real engine-root runtimes and use root DTOs that
// were previously re-exported by the runtime facade. Alias them from the
// engine root package directly so the executor leaf package stays production
// independent of both the root package and the facade.
type (
	Runtime              = jfadk.Runtime
	Store                = jfadk.Store
	Provider             = jfadk.Provider
	ToolRegistry         = jfadk.ToolRegistry
	AgentWriteRequest    = jfadk.AgentWriteRequest
	ProviderWriteRequest = jfadk.ProviderWriteRequest
	WorkflowStepState    = jfadk.WorkflowStepState
)

func NewRuntime(store *Store, tools *ToolRegistry) *Runtime {
	return jfadk.NewRuntime(store, tools)
}

func NewStore(dbPath string, secretsPath string, skillsPath string) (*Store, error) {
	return jfadk.NewStore(dbPath, secretsPath, skillsPath)
}

func NewToolRegistry() *ToolRegistry {
	return jfadk.NewToolRegistry()
}

const AgentStatusEnabled = jfadk.AgentStatusEnabled

const (
	PermissionModeApproval     = jfadk.PermissionModeApproval
	PermissionModeLessApproval = jfadk.PermissionModeLessApproval
	PermissionModeAll          = jfadk.PermissionModeAll
	ApprovalStatusApproved     = jfadk.ApprovalStatusApproved
	InputRequestStatusPending  = jfadk.InputRequestStatusPending
)
