// Package assembly owns construction and lifecycle for the ADK runtime and
// its local MCP transport. Application packages provide narrow callbacks for
// cross-domain tools; this package never depends on HTTP handlers or Gin.
package assembly

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	assistant "github.com/jftrade/jftrade-main/internal/assistant"
	jfadk "github.com/jftrade/jftrade-main/pkg/adk"
	jfsettings "github.com/jftrade/jftrade-main/pkg/jftsettings"
)

// WorkflowEvent is the broker-neutral event consumed by assistant workflows.
type WorkflowEvent struct {
	ID       string
	Type     string
	Source   string
	EntityID string
	At       string
	Payload  map[string]any
}

// RuntimeLimits is the dynamic runtime limits snapshot consumed by ADK.
type RuntimeLimits struct {
	RunTimeout time.Duration
}

// Paths are application-derived persistent locations. Keeping derivation in
// the application layer prevents this package from depending on the API server.
type Paths struct {
	Database string
	Session  string
	Secrets  string
	Skills   string
}

// Options describes one assistant runtime assembly.
type Options struct {
	Paths          Paths
	RuntimeLimits  func() RuntimeLimits
	Tools          *ToolDeps
	ServiceOptions []assistant.Option
}

// Runtime is the single assistant lifecycle and integration surface exposed to
// the application composition root.
type Runtime interface {
	Service() *assistant.Service
	Available() bool
	RecordAudit(context.Context, string, string, string, map[string]any)
	RegisterTool(jfadk.ToolDescriptor, jfadk.ToolFunc) error
	Tool(string) (jfadk.RegisteredTool, bool)
	HasTool(string) bool
	StartWorkflowScheduler(context.Context)
	WatchedWorkflowInstruments(context.Context) []string
	HandleWorkflowEvent(context.Context, WorkflowEvent)
	ReconfigureMCP(jfsettings.MCPServerSettings) error
	MCPStatus() jfsettings.MCPServerStatus
	DatabaseMaintenance(MaintenanceResource) *DatabaseMaintenance
	Close() error
}

// Handle owns the runtime, assistant service, and local MCP listener. Close is
// safe to call repeatedly and always shuts down the listener before the service
// closes the shared runtime.
type Handle struct {
	runtime *jfadk.Runtime
	service *assistant.Service
	mcp     *mcpServerManager

	closeOnce sync.Once
	closeErr  error
}

var _ Runtime = (*Handle)(nil)

// Open constructs the persistent ADK store, tool registry, SQLite session
// service, assistant facade, and stopped MCP listener as one owned unit.
func Open(options Options) (*Handle, error) {
	runtime, err := openRuntime(options)
	if err != nil {
		return nil, err
	}
	service := assistant.NewService(runtime, options.ServiceOptions...)
	return &Handle{
		runtime: runtime,
		service: service,
		mcp:     newMCPServerManager(runtime),
	}, nil
}

func openRuntime(options Options) (*jfadk.Runtime, error) {
	store, err := jfadk.NewStore(
		options.Paths.Database,
		options.Paths.Secrets,
		options.Paths.Skills,
	)
	if err != nil {
		return nil, fmt.Errorf("open ADK store: %w", err)
	}
	registry := jfadk.NewToolRegistry()
	if options.Tools != nil {
		RegisterJFTradeADKTools(store, registry, *options.Tools)
	}
	sessionService, err := jfadk.NewSQLiteSessionService(options.Paths.Session)
	if err != nil {
		return nil, errors.Join(fmt.Errorf("open ADK session service: %w", err), closeStoreAfterOpenFailure(store))
	}
	runtime := jfadk.NewRuntimeWithSessionService(store, registry, sessionService)
	if options.RuntimeLimits != nil {
		runtime.SetRuntimeLimitsProvider(func() jfadk.RuntimeLimits {
			limits := options.RuntimeLimits()
			return jfadk.RuntimeLimits{RunTimeout: limits.RunTimeout}
		})
	}
	return runtime, nil
}

func closeStoreAfterOpenFailure(store *jfadk.Store) error {
	if store == nil {
		return nil
	}
	if err := store.Close(); err != nil {
		return fmt.Errorf("close ADK store after session failure: %w", err)
	}
	return nil
}

// DatabaseProbe separates availability failures from best-effort cleanup
// failures so the application can preserve degraded-start semantics.
type DatabaseProbe struct {
	OpenError  error
	CloseError error
}

// InspectRuntimeDatabase verifies that the ADK configuration database can be
// opened and reports cleanup separately.
func InspectRuntimeDatabase(paths Paths) DatabaseProbe {
	store, err := jfadk.NewStore(
		paths.Database,
		paths.Secrets,
		paths.Skills,
	)
	if err != nil {
		return DatabaseProbe{OpenError: err}
	}
	return DatabaseProbe{CloseError: store.Close()}
}

// InspectSessionDatabase verifies that the ADK session database can be opened
// and reports cleanup separately.
func InspectSessionDatabase(paths Paths) DatabaseProbe {
	service, err := jfadk.NewSQLiteSessionService(paths.Session)
	if err != nil {
		return DatabaseProbe{OpenError: err}
	}
	return DatabaseProbe{CloseError: jfadk.CloseSessionService(service)}
}

// ProbeRuntimeDatabase is a compact probe for tests and simple callers.
func ProbeRuntimeDatabase(paths Paths) error {
	probe := InspectRuntimeDatabase(paths)
	return errors.Join(probe.OpenError, probe.CloseError)
}

// ProbeSessionDatabase is a compact probe for tests and simple callers.
func ProbeSessionDatabase(paths Paths) error {
	probe := InspectSessionDatabase(paths)
	return errors.Join(probe.OpenError, probe.CloseError)
}

// Service returns the assistant business facade backed by Runtime.
func (h *Handle) Service() *assistant.Service {
	if h == nil {
		return nil
	}
	return h.service
}

// Available reports whether the assistant service has an initialized runtime.
func (h *Handle) Available() bool {
	return h != nil && h.service != nil && h.service.Available()
}

// RecordAudit persists one assistant audit event without exposing Runtime.Store.
func (h *Handle) RecordAudit(
	ctx context.Context,
	kind string,
	subjectID string,
	detail string,
	metadata map[string]any,
) {
	if h == nil || h.runtime == nil {
		return
	}
	h.runtime.RecordAudit(ctx, kind, subjectID, detail, metadata)
}

// RegisterTool installs one integration tool without exposing ToolRegistry.
func (h *Handle) RegisterTool(
	descriptor jfadk.ToolDescriptor,
	handler jfadk.ToolFunc,
) error {
	if h == nil || h.runtime == nil || h.runtime.Tools() == nil {
		return errors.New("ADK tool registry is unavailable")
	}
	h.runtime.Tools().Register(descriptor, handler)
	return nil
}

// Tool returns one registered integration tool without exposing ToolRegistry.
func (h *Handle) Tool(name string) (jfadk.RegisteredTool, bool) {
	if h == nil || h.runtime == nil || h.runtime.Tools() == nil {
		return jfadk.RegisteredTool{}, false
	}
	return h.runtime.Tools().Get(name)
}

// HasTool reports whether a named integration tool is registered.
func (h *Handle) HasTool(name string) bool {
	_, ok := h.Tool(name)
	return ok
}

// StartWorkflowScheduler starts background workflow trigger reconciliation.
func (h *Handle) StartWorkflowScheduler(ctx context.Context) {
	if h == nil || h.service == nil {
		return
	}
	h.service.StartWorkflowScheduler(ctx)
}

// WatchedWorkflowInstruments returns the current market-trigger demand.
func (h *Handle) WatchedWorkflowInstruments(ctx context.Context) []string {
	if h == nil || h.service == nil {
		return nil
	}
	return h.service.WatchedWorkflowInstruments(ctx)
}

// HandleWorkflowEvent forwards one broker-neutral event to workflow triggers.
func (h *Handle) HandleWorkflowEvent(ctx context.Context, event WorkflowEvent) {
	if h == nil || h.service == nil {
		return
	}
	h.service.HandleWorkflowEvent(ctx, jfadk.WorkflowEvent{
		ID:       event.ID,
		Type:     event.Type,
		Source:   event.Source,
		EntityID: event.EntityID,
		At:       event.At,
		Payload:  event.Payload,
	})
}

// ReconfigureMCP applies local MCP listener settings synchronously.
func (h *Handle) ReconfigureMCP(settings jfsettings.MCPServerSettings) error {
	if h == nil || h.mcp == nil {
		return errors.New("MCP server manager is unavailable")
	}
	return h.mcp.Reconfigure(settings)
}

// MCPStatus returns the current listener state.
func (h *Handle) MCPStatus() jfsettings.MCPServerStatus {
	if h == nil || h.mcp == nil {
		return jfsettings.MCPServerStatus{LastError: "MCP server manager is unavailable"}
	}
	return h.mcp.Status()
}

// DatabaseMaintenance returns the maintenance adapter for one ADK-owned
// database without exposing the runtime or its stores to the composition root.
func (h *Handle) DatabaseMaintenance(resource MaintenanceResource) *DatabaseMaintenance {
	if h == nil {
		return newDatabaseMaintenance(nil, resource)
	}
	return newDatabaseMaintenance(h.runtime, resource)
}

// Close stops MCP first, then the assistant scheduler and shared ADK runtime.
func (h *Handle) Close() error {
	if h == nil {
		return nil
	}
	h.closeOnce.Do(func() {
		var errs []error
		if h.mcp != nil {
			if err := h.mcp.Close(); err != nil {
				errs = append(errs, fmt.Errorf("local MCP server close: %w", err))
			}
		}
		if h.service != nil {
			if err := h.service.Close(); err != nil {
				errs = append(errs, fmt.Errorf("assistant service close: %w", err))
			}
		} else if h.runtime != nil {
			if err := h.runtime.Close(); err != nil {
				errs = append(errs, fmt.Errorf("ADK runtime close: %w", err))
			}
		}
		h.closeErr = errors.Join(errs...)
	})
	return h.closeErr
}
