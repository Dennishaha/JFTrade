package catalog

import (
	"context"
	"errors"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	stratsrv "github.com/jftrade/jftrade-main/internal/strategy"
	runtimeactivity "github.com/jftrade/jftrade-main/internal/strategy/runtimeactivity"
	strategydefinition "github.com/jftrade/jftrade-main/pkg/strategy/definition"
)

var errCatalogRepositoryUnavailable = errors.New("catalog repository unavailable")

type catalogMemoryRepository struct {
	mu        sync.Mutex
	snapshot  Snapshot
	loadErr   error
	saveErr   error
	saveCalls int
}

func (r *catalogMemoryRepository) Load(context.Context) (Snapshot, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.loadErr != nil {
		return Snapshot{}, r.loadErr
	}
	return cloneSnapshot(r.snapshot), nil
}

func (r *catalogMemoryRepository) Save(_ context.Context, snapshot Snapshot) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.saveCalls++
	if r.saveErr != nil {
		return r.saveErr
	}
	r.snapshot = cloneSnapshot(snapshot)
	return nil
}

func (r *catalogMemoryRepository) durableSnapshot() Snapshot {
	r.mu.Lock()
	defer r.mu.Unlock()
	return cloneSnapshot(r.snapshot)
}

func (r *catalogMemoryRepository) saveCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.saveCalls
}

type catalogMemoryActivityStore struct {
	mu           sync.Mutex
	logs         []runtimeactivity.LogEvent
	audit        []runtimeactivity.AuditEvent
	observations map[string]runtimeactivity.ObservationSnapshot
}

func newCatalogMemoryActivityStore() *catalogMemoryActivityStore {
	return &catalogMemoryActivityStore{
		logs:         []runtimeactivity.LogEvent{},
		audit:        []runtimeactivity.AuditEvent{},
		observations: map[string]runtimeactivity.ObservationSnapshot{},
	}
}

func (s *catalogMemoryActivityStore) AppendLog(_ context.Context, event runtimeactivity.LogEvent) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	event.ID = int64(len(s.logs) + 1)
	s.logs = append(s.logs, event)
	return nil
}

func (s *catalogMemoryActivityStore) ListLogs(
	_ context.Context,
	query runtimeactivity.LogQuery,
) ([]runtimeactivity.LogEvent, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	filtered := make([]runtimeactivity.LogEvent, 0, len(s.logs))
	for _, event := range s.logs {
		if !catalogLogMatches(event, query) {
			continue
		}
		filtered = append(filtered, event)
	}
	return catalogPage(filtered, query.Offset, query.Limit), nil
}

func (s *catalogMemoryActivityStore) CountLogs(
	_ context.Context,
	query runtimeactivity.LogQuery,
) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	total := 0
	for _, event := range s.logs {
		if catalogLogMatches(event, query) {
			total++
		}
	}
	return total, nil
}

func (s *catalogMemoryActivityStore) ListRecentLogsTail(
	_ context.Context,
	instanceID string,
	limit int,
) ([]runtimeactivity.LogEvent, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	filtered := make([]runtimeactivity.LogEvent, 0, len(s.logs))
	for _, event := range s.logs {
		if event.InstanceID == instanceID {
			filtered = append(filtered, event)
		}
	}
	if limit <= 0 || limit >= len(filtered) {
		return append([]runtimeactivity.LogEvent(nil), filtered...), nil
	}
	return append([]runtimeactivity.LogEvent(nil), filtered[len(filtered)-limit:]...), nil
}

func (s *catalogMemoryActivityStore) AppendAudit(
	_ context.Context,
	event runtimeactivity.AuditEvent,
) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	event.ID = int64(len(s.audit) + 1)
	s.audit = append(s.audit, event)
	return nil
}

func (s *catalogMemoryActivityStore) ListAudit(
	_ context.Context,
	query runtimeactivity.AuditQuery,
) ([]runtimeactivity.AuditEvent, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	filtered := make([]runtimeactivity.AuditEvent, 0, len(s.audit))
	for _, event := range s.audit {
		if !catalogAuditMatches(event, query) {
			continue
		}
		filtered = append(filtered, event)
	}
	return catalogPage(filtered, query.Offset, query.Limit), nil
}

func (s *catalogMemoryActivityStore) CountAudit(
	_ context.Context,
	query runtimeactivity.AuditQuery,
) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	total := 0
	for _, event := range s.audit {
		if catalogAuditMatches(event, query) {
			total++
		}
	}
	return total, nil
}

func (s *catalogMemoryActivityStore) UpsertObservation(
	_ context.Context,
	snapshot runtimeactivity.ObservationSnapshot,
) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.observations[snapshot.InstanceID] = snapshot
	return nil
}

func (s *catalogMemoryActivityStore) GetObservation(
	_ context.Context,
	instanceID string,
) (runtimeactivity.ObservationSnapshot, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	snapshot, ok := s.observations[instanceID]
	return snapshot, ok, nil
}

func catalogLogMatches(event runtimeactivity.LogEvent, query runtimeactivity.LogQuery) bool {
	if event.InstanceID != query.InstanceID {
		return false
	}
	if query.Level != "" && event.Level != query.Level {
		return false
	}
	return catalogTimeMatches(event.At, query.FromAt, query.ToAt)
}

func catalogAuditMatches(event runtimeactivity.AuditEvent, query runtimeactivity.AuditQuery) bool {
	if event.InstanceID != query.InstanceID {
		return false
	}
	if query.Kind != "" && event.Kind != query.Kind {
		return false
	}
	return catalogTimeMatches(event.At, query.FromAt, query.ToAt)
}

func catalogTimeMatches(at time.Time, fromAt, toAt *time.Time) bool {
	if fromAt != nil && at.Before(*fromAt) {
		return false
	}
	if toAt != nil && at.After(*toAt) {
		return false
	}
	return true
}

func catalogPage[T any](items []T, offset, limit int) []T {
	if offset >= len(items) {
		return []T{}
	}
	end := min(offset+limit, len(items))
	return append([]T(nil), items[offset:end]...)
}

type catalogDefinitionStore struct {
	definition stratsrv.Definition
	found      bool
	err        error
}

func (s catalogDefinitionStore) GetDefinition(string) (stratsrv.Definition, bool, error) {
	return s.definition, s.found, s.err
}

func catalogBusinessDefinition(id, version string) stratsrv.Definition {
	return stratsrv.Definition{
		ID:           id,
		Name:         "Catalog " + id,
		Version:      version,
		Runtime:      stratsrv.RuntimePinePlan,
		SourceFormat: strategydefinition.SourceFormatPineV6,
		Symbol:       "US.AAPL",
		Interval:     "1m",
		Script: strings.Join([]string{
			"//@version=6",
			`strategy("Catalog", overlay=true)`,
			`strategy.entry("Long", strategy.long, qty=1)`,
		}, "\n"),
	}
}

func catalogBusinessInstance(id, definitionID, version, status string) stratsrv.ManagedInstance {
	return stratsrv.ManagedInstance{
		ID:       id,
		PluginID: "pine-plan",
		Definition: stratsrv.DefinitionSummary{
			StrategyID: definitionID,
			Name:       "Catalog " + definitionID,
			Version:    version,
		},
		Binding: stratsrv.InstanceBinding{
			Symbols:       []string{"US.AAPL"},
			Interval:      "1m",
			ExecutionMode: ExecutionModeNotifyOnly,
		},
		Params: map[string]any{
			"definitionId": definitionID,
			"runtime":      stratsrv.RuntimePinePlan,
			"sourceFormat": strategydefinition.SourceFormatPineV6,
			"symbol":       "US.AAPL",
			"symbols":      []string{"US.AAPL"},
			"interval":     "1m",
			"script":       catalogBusinessDefinition(definitionID, version).Script,
		},
		Status:    status,
		CreatedAt: "2026-07-01T00:00:00Z",
	}
}

func newCatalogBusinessService(
	t *testing.T,
	snapshot Snapshot,
) (*Service, *catalogMemoryRepository, *catalogMemoryActivityStore) {
	t.Helper()
	repository := &catalogMemoryRepository{snapshot: cloneSnapshot(snapshot)}
	activity := newCatalogMemoryActivityStore()
	service, err := New(repository, activity, t.TempDir())
	if err != nil {
		t.Fatalf("New catalog service: %v", err)
	}
	return service, repository, activity
}

func assertCatalogStringSet(t *testing.T, got, want []string) {
	t.Helper()
	got = append([]string(nil), got...)
	want = append([]string(nil), want...)
	slices.Sort(got)
	slices.Sort(want)
	if !slices.Equal(got, want) {
		t.Fatalf("values = %v, want %v", got, want)
	}
}
