package catalog

import (
	"context"
	"sync"

	stratsrv "github.com/jftrade/jftrade-main/internal/strategy"
	runtimeactivity "github.com/jftrade/jftrade-main/internal/strategy/runtimeactivity"
)

const (
	StatusRunning = "RUNNING"
	StatusPaused  = "PAUSED"
	StatusStopped = "STOPPED"

	ExecutionModeLive       = "live"
	ExecutionModeNotifyOnly = "notify_only"

	defaultPluginDir = "plugins"
	pluginType       = "strategy-go-plugin"
	pluginBuildMode  = "plugin"
)

type PluginArtifact struct {
	Path  string                    `json:"path"`
	Build stratsrv.PluginBuildTuple `json:"build"`
}

type ManagedPlugin struct {
	Descriptor   stratsrv.PluginDescriptor   `json:"descriptor"`
	Artifact     *PluginArtifact             `json:"artifact,omitempty"`
	Installation stratsrv.PluginInstallation `json:"installation"`
}

// Snapshot is the aggregate persisted by a catalog Repository. Runtime logs,
// audit events and observations deliberately live behind separate activity
// ports and are never embedded in this payload.
type Snapshot struct {
	TargetDir  string                     `json:"targetDir,omitempty"`
	Plugins    []ManagedPlugin            `json:"plugins,omitempty"`
	Strategies []stratsrv.ManagedInstance `json:"strategies,omitempty"`
	Operations []stratsrv.PluginOperation `json:"operations,omitempty"`
}

// Repository persists catalog aggregate snapshots. It is defined by the
// consumer and implemented by internal/store/strategy.
type Repository interface {
	Load(context.Context) (Snapshot, error)
	Save(context.Context, Snapshot) error
}

type DefinitionStore interface {
	GetDefinition(string) (stratsrv.Definition, bool, error)
}

type ObservationSource interface {
	GetObservation(string) (stratsrv.RuntimeObservation, bool)
}

type ObservationSourceFunc func(string) (stratsrv.RuntimeObservation, bool)

func (f ObservationSourceFunc) GetObservation(id string) (stratsrv.RuntimeObservation, bool) {
	if f == nil {
		return stratsrv.RuntimeObservation{}, false
	}
	return f(id)
}

// RuntimeEventStore is the narrow catalog surface consumed by the live
// strategy runtime. It avoids exposing aggregate state or persistence details.
type RuntimeEventStore interface {
	GetInstance(string) (stratsrv.ManagedInstance, bool)
	AppendRuntimeEvent(string, string, string, string) error
	TransitionRuntime(string, string, string, string) (stratsrv.InstanceView, error)
	ReconcileRuntimeFailure(string, string) error
}

// PluginRegistry accepts plugin descriptors discovered by application
// integration code. Install/uninstall state transitions remain on CatalogStore.
type PluginRegistry interface {
	RegisterPlugin(ManagedPlugin) error
}

// Configurator binds cross-domain definition and live-observation sources at
// the application composition root.
type Configurator interface {
	SetDefinitionStore(DefinitionStore)
	SetObservationSource(ObservationSource)
}

type Service struct {
	repository Repository
	activity   runtimeactivity.Store
	targetDir  string

	mu                sync.RWMutex
	data              Snapshot
	definitions       DefinitionStore
	observationSource ObservationSource
}

var (
	_ stratsrv.CatalogStore = (*Service)(nil)
	_ RuntimeEventStore     = (*Service)(nil)
	_ PluginRegistry        = (*Service)(nil)
	_ Configurator          = (*Service)(nil)
)

func New(repository Repository, activity runtimeactivity.Store, targetDir string) (*Service, error) {
	service := &Service{
		repository: repository,
		activity:   activity,
		targetDir:  normalizeTargetDir(targetDir),
		data: Snapshot{
			Plugins:    []ManagedPlugin{},
			Strategies: []stratsrv.ManagedInstance{},
			Operations: []stratsrv.PluginOperation{},
		},
	}
	if repository == nil {
		service.data.TargetDir = service.targetDir
		return service, nil
	}
	snapshot, err := repository.Load(context.Background())
	if err != nil {
		return nil, err
	}
	service.data = service.normalizeSnapshot(snapshot)
	if service.data.TargetDir == "" {
		service.data.TargetDir = service.targetDir
		if err := repository.Save(context.Background(), service.snapshotLocked()); err != nil {
			return nil, err
		}
	}
	return service, nil
}

func (s *Service) SetDefinitionStore(store DefinitionStore) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.definitions = store
}

func (s *Service) SetObservationSource(source ObservationSource) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.observationSource = source
}

func (s *Service) snapshotLocked() Snapshot {
	return cloneSnapshot(s.data)
}

func (s *Service) persistLocked() error {
	if s.repository == nil {
		return nil
	}
	return s.repository.Save(context.Background(), s.snapshotLocked())
}
