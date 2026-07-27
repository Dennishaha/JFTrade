package strategy

import (
	"context"
	"path/filepath"
	"strings"
	"sync"

	stratsrv "github.com/jftrade/jftrade-main/internal/strategy"
	strategycatalog "github.com/jftrade/jftrade-main/internal/strategy/catalog"
	runtimeactivity "github.com/jftrade/jftrade-main/internal/strategy/runtimeactivity"
)

const (
	defaultStrategyCatalogFilename = "strategy-catalog.json"
	defaultStrategyPluginDirName   = "plugins"
)

// CatalogResource combines only the catalog, runtime activity, availability
// and lifecycle ports needed by application assembly.
type CatalogResource interface {
	stratsrv.CatalogStore
	strategycatalog.RuntimeEventStore
	strategycatalog.PluginRegistry
	strategycatalog.Configurator
	runtimeactivity.Store
	Available() bool
	Close() error
}

type catalogResource struct {
	*strategycatalog.Service
	runtimeactivity.Store

	available bool
	closeFn   func() error
	closeOnce sync.Once
	closeErr  error
}

var (
	_ CatalogResource = (*catalogResource)(nil)
)

func NewCatalog(catalogPath, targetDir string) (CatalogResource, error) {
	activity, err := openRuntimeActivity(DeriveDBPath(catalogPath))
	if err != nil {
		return nil, err
	}
	repository := &catalogRepository{store: activity}
	service, err := strategycatalog.New(repository, activity, targetDir)
	if err != nil {
		_ = activity.Close()
		return nil, err
	}
	return &catalogResource{
		Service:   service,
		Store:     activity,
		available: true,
		closeFn:   activity.Close,
	}, nil
}

func NewUnavailableCatalog(_ string, targetDir string) CatalogResource {
	activity := unavailableRuntimeActivity{}
	service, _ := strategycatalog.New(nil, activity, targetDir)
	return &catalogResource{
		Service: service,
		Store:   activity,
	}
}

func (r *catalogResource) Available() bool {
	return r != nil && r.available
}

func (r *catalogResource) Close() error {
	if r == nil {
		return nil
	}
	r.closeOnce.Do(func() {
		if r.closeFn != nil {
			r.closeErr = r.closeFn()
		}
	})
	return r.closeErr
}

func DeriveCatalogPath(settingsPath string) string {
	directory := filepath.Dir(strings.TrimSpace(settingsPath))
	if directory == "" || directory == "." {
		return defaultStrategyCatalogFilename
	}
	return filepath.Join(directory, defaultStrategyCatalogFilename)
}

func DerivePluginTargetDir(settingsPath string) string {
	directory := filepath.Dir(strings.TrimSpace(settingsPath))
	if directory == "" || directory == "." {
		return defaultStrategyPluginDirName
	}
	return filepath.Join(directory, defaultStrategyPluginDirName)
}

type unavailableRuntimeActivity struct{}

func (unavailableRuntimeActivity) AppendLog(context.Context, runtimeactivity.LogEvent) error {
	return nil
}

func (unavailableRuntimeActivity) ListLogs(context.Context, runtimeactivity.LogQuery) ([]runtimeactivity.LogEvent, error) {
	return []runtimeactivity.LogEvent{}, nil
}

func (unavailableRuntimeActivity) CountLogs(context.Context, runtimeactivity.LogQuery) (int, error) {
	return 0, nil
}

func (unavailableRuntimeActivity) ListRecentLogsTail(context.Context, string, int) ([]runtimeactivity.LogEvent, error) {
	return []runtimeactivity.LogEvent{}, nil
}

func (unavailableRuntimeActivity) AppendAudit(context.Context, runtimeactivity.AuditEvent) error {
	return nil
}

func (unavailableRuntimeActivity) ListAudit(context.Context, runtimeactivity.AuditQuery) ([]runtimeactivity.AuditEvent, error) {
	return []runtimeactivity.AuditEvent{}, nil
}

func (unavailableRuntimeActivity) CountAudit(context.Context, runtimeactivity.AuditQuery) (int, error) {
	return 0, nil
}

func (unavailableRuntimeActivity) UpsertObservation(context.Context, runtimeactivity.ObservationSnapshot) error {
	return nil
}

func (unavailableRuntimeActivity) GetObservation(context.Context, string) (runtimeactivity.ObservationSnapshot, bool, error) {
	return runtimeactivity.ObservationSnapshot{}, false, nil
}
