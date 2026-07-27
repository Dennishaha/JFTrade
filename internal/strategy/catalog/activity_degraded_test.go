package catalog

import (
	"context"
	"errors"
	"testing"

	stratsrv "github.com/jftrade/jftrade-main/internal/strategy"
	runtimeactivity "github.com/jftrade/jftrade-main/internal/strategy/runtimeactivity"
)

type degradedActivityRepository struct {
	snapshot Snapshot
}

func (r *degradedActivityRepository) Load(context.Context) (Snapshot, error) {
	return cloneSnapshot(r.snapshot), nil
}

func (r *degradedActivityRepository) Save(_ context.Context, snapshot Snapshot) error {
	r.snapshot = cloneSnapshot(snapshot)
	return nil
}

type degradedActivityStore struct {
	err error
}

func (s degradedActivityStore) AppendLog(context.Context, runtimeactivity.LogEvent) error {
	return s.err
}

func (s degradedActivityStore) ListLogs(context.Context, runtimeactivity.LogQuery) ([]runtimeactivity.LogEvent, error) {
	return nil, s.err
}

func (s degradedActivityStore) CountLogs(context.Context, runtimeactivity.LogQuery) (int, error) {
	return 0, s.err
}

func (s degradedActivityStore) ListRecentLogsTail(context.Context, string, int) ([]runtimeactivity.LogEvent, error) {
	return nil, s.err
}

func (s degradedActivityStore) AppendAudit(context.Context, runtimeactivity.AuditEvent) error {
	return s.err
}

func (s degradedActivityStore) ListAudit(context.Context, runtimeactivity.AuditQuery) ([]runtimeactivity.AuditEvent, error) {
	return nil, s.err
}

func (s degradedActivityStore) CountAudit(context.Context, runtimeactivity.AuditQuery) (int, error) {
	return 0, s.err
}

func (s degradedActivityStore) UpsertObservation(context.Context, runtimeactivity.ObservationSnapshot) error {
	return s.err
}

func (s degradedActivityStore) GetObservation(context.Context, string) (runtimeactivity.ObservationSnapshot, bool, error) {
	return runtimeactivity.ObservationSnapshot{}, false, s.err
}

func TestCatalogActivityReturnsEmptyPagesWhenActivityStoreIsUnavailable(t *testing.T) {
	repository := &degradedActivityRepository{snapshot: Snapshot{
		Strategies: []stratsrv.ManagedInstance{{ID: "activity"}},
	}}
	for _, activity := range []runtimeactivity.Store{
		nil,
		degradedActivityStore{err: errors.New("activity store unavailable")},
	} {
		service, err := New(repository, activity, t.TempDir())
		if err != nil {
			t.Fatalf("New: %v", err)
		}
		logs, ok := service.GetLogs("activity", stratsrv.LogQuery{Limit: -1, Offset: -1})
		if !ok || len(logs.Logs) != 0 || logs.Page.Total != 0 {
			t.Fatalf("degraded log page = %#v, %v", logs, ok)
		}
		audit, ok := service.GetAudit("activity", stratsrv.AuditQuery{Limit: -1, Offset: -1})
		if !ok || len(audit.Entries) != 0 || audit.Page.Total != 0 {
			t.Fatalf("degraded audit page = %#v, %v", audit, ok)
		}
	}
}
