// Package testkit exposes concrete ADK construction only for cross-package
// integration tests. Production application code must use assembly.Runtime.
package testkit

import (
	jfadk "github.com/jftrade/jftrade-main/pkg/adk"
	adksession "google.golang.org/adk/v2/session"
)

type (
	Runtime      = jfadk.Runtime
	Store        = jfadk.Store
	ToolRegistry = jfadk.ToolRegistry
)

func NewStore(databasePath string, secretsPath string, skillsPath string) (*Store, error) {
	return jfadk.NewStore(databasePath, secretsPath, skillsPath)
}

func NewToolRegistry() *ToolRegistry {
	return jfadk.NewToolRegistry()
}

func NewRuntimeWithSessionService(
	store *Store,
	tools *ToolRegistry,
	sessionService adksession.Service,
) *Runtime {
	return jfadk.NewRuntimeWithSessionService(store, tools, sessionService)
}
