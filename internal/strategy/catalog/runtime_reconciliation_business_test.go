package catalog

import (
	"strings"
	"testing"
	"time"

	stratsrv "github.com/jftrade/jftrade-main/internal/strategy"
	runtimeactivity "github.com/jftrade/jftrade-main/internal/strategy/runtimeactivity"
)

func TestCatalogRuntimeTransitionsPersistStateAndActivity(t *testing.T) {
	service, repository, _ := newCatalogBusinessService(t, Snapshot{Strategies: []stratsrv.ManagedInstance{
		catalogBusinessInstance("runtime", "mean-revert", "1.0.0", StatusStopped),
	}})

	for _, transition := range []struct {
		status string
		kind   string
	}{
		{status: StatusRunning, kind: "started"},
		{status: StatusPaused, kind: "paused"},
		{status: StatusStopped, kind: "stopped"},
	} {
		item, err := service.TransitionInstance("runtime", transition.status)
		if err != nil {
			t.Fatalf("TransitionInstance(%s): %v", transition.status, err)
		}
		if item.Status != transition.status {
			t.Fatalf("transition status = %q, want %q", item.Status, transition.status)
		}
		audit, ok := service.GetAudit("runtime", stratsrv.AuditQuery{Kind: transition.kind})
		if !ok || len(audit.Entries) != 1 || audit.Entries[0].Kind != transition.kind {
			t.Fatalf("transition audit = %#v, found=%v", audit, ok)
		}
	}
	if err := service.AppendRuntimeEvent("runtime", "broker order failed", "order_submit_failed", "upstream"); err != nil {
		t.Fatalf("AppendRuntimeEvent: %v", err)
	}
	logs, ok := service.GetLogs("runtime", stratsrv.LogQuery{Level: " ERROR ", Limit: 1})
	if !ok || len(logs.Logs) != 1 || !strings.Contains(logs.Logs[0], "broker order failed") {
		t.Fatalf("runtime logs = %#v, found=%v", logs, ok)
	}
	if logs.Page.Total != 1 || logs.Page.Returned != 1 || logs.Page.HasMore {
		t.Fatalf("runtime log page = %#v", logs.Page)
	}
	if repository.saveCount() != 3 {
		t.Fatalf("runtime transition saves = %d, want 3", repository.saveCount())
	}
}

func TestCatalogRuntimeFailureReconcilesOnlyRunningInstance(t *testing.T) {
	service, repository, _ := newCatalogBusinessService(t, Snapshot{Strategies: []stratsrv.ManagedInstance{
		catalogBusinessInstance("running", "mean-revert", "1.0.0", StatusRunning),
		catalogBusinessInstance("stopped", "trend", "1.0.0", StatusStopped),
	}})
	if err := service.ReconcileRuntimeFailure("stopped", "ignored"); err != nil {
		t.Fatalf("ReconcileRuntimeFailure stopped: %v", err)
	}
	if repository.saveCount() != 0 {
		t.Fatalf("stopped reconcile save count = %d", repository.saveCount())
	}
	if err := service.ReconcileRuntimeFailure("running", "worker exited"); err != nil {
		t.Fatalf("ReconcileRuntimeFailure running: %v", err)
	}
	running, _ := service.GetInstance("running")
	if running.Status != StatusStopped {
		t.Fatalf("running status after failure = %q", running.Status)
	}
	audit, ok := service.GetAudit("running", stratsrv.AuditQuery{Kind: "runtime_exited"})
	if !ok || len(audit.Entries) != 1 || audit.Entries[0].Detail != "worker exited" {
		t.Fatalf("runtime failure audit = %#v, found=%v", audit, ok)
	}
	logs, _ := service.GetLogs("running", stratsrv.LogQuery{Level: "error"})
	if len(logs.Logs) != 1 || !strings.Contains(logs.Logs[0], "worker exited") {
		t.Fatalf("runtime failure logs = %#v", logs)
	}
}

func TestCatalogStartupReconcileResetsStaleRunningAndPausedState(t *testing.T) {
	service, repository, _ := newCatalogBusinessService(t, Snapshot{Strategies: []stratsrv.ManagedInstance{
		catalogBusinessInstance("running", "a", "1.0.0", StatusRunning),
		catalogBusinessInstance("paused", "b", "1.0.0", StatusPaused),
		catalogBusinessInstance("stopped", "c", "1.0.0", StatusStopped),
	}})
	changed, err := service.ReconcileOnStartup()
	if err != nil {
		t.Fatalf("ReconcileOnStartup: %v", err)
	}
	if changed != 2 || repository.saveCount() != 1 {
		t.Fatalf("startup reconcile changed/saves = %d/%d", changed, repository.saveCount())
	}
	for _, id := range []string{"running", "paused", "stopped"} {
		instance, _ := service.GetInstance(id)
		if instance.Status != StatusStopped {
			t.Fatalf("%s status = %q", id, instance.Status)
		}
	}
	for _, id := range []string{"running", "paused"} {
		audit, _ := service.GetAudit(id, stratsrv.AuditQuery{Kind: "reconciled"})
		if len(audit.Entries) != 1 || !strings.Contains(audit.Entries[0].Detail, "server startup reset stale") {
			t.Fatalf("%s reconcile audit = %#v", id, audit)
		}
	}
	if changed, err := service.ReconcileOnStartup(); err != nil || changed != 0 {
		t.Fatalf("second ReconcileOnStartup = %d, %v", changed, err)
	}
	if repository.saveCount() != 1 {
		t.Fatalf("unchanged startup reconcile persisted again: %d", repository.saveCount())
	}
}

func TestCatalogActivitySupportsPagingFilteringAndRuntimeObservationEnrichment(t *testing.T) {
	service, _, activity := newCatalogBusinessService(t, Snapshot{Strategies: []stratsrv.ManagedInstance{
		catalogBusinessInstance("activity", "mean-revert", "1.0.0", StatusRunning),
	}})
	start := time.Date(2026, 7, 1, 10, 0, 0, 0, time.UTC)
	for index, level := range []string{"info", "warning", "error"} {
		at := start.Add(time.Duration(index) * time.Minute)
		if err := activity.AppendLog(t.Context(), runtimeactivity.LogEvent{
			InstanceID: "activity",
			At:         at,
			Raw:        level,
			Level:      level,
			Source:     "runtime",
		}); err != nil {
			t.Fatal(err)
		}
		if err := activity.AppendAudit(t.Context(), runtimeactivity.AuditEvent{
			InstanceID: "activity",
			At:         at,
			Kind:       "kind." + level,
			Detail:     level,
		}); err != nil {
			t.Fatal(err)
		}
	}
	logs, ok := service.GetLogs("activity", stratsrv.LogQuery{Limit: 1, Offset: 1})
	if !ok || len(logs.Logs) != 1 || logs.Logs[0] != "warning" {
		t.Fatalf("paged logs = %#v, found=%v", logs, ok)
	}
	if logs.Page.Total != 3 || logs.Page.HasMore != true {
		t.Fatalf("paged log metadata = %#v", logs.Page)
	}
	audit, ok := service.GetAudit("activity", stratsrv.AuditQuery{Kind: "kind.error"})
	if !ok || len(audit.Entries) != 1 || audit.Entries[0].Detail != "error" {
		t.Fatalf("filtered audit = %#v, found=%v", audit, ok)
	}

	updatedAt := start.Add(time.Hour)
	lastSignalAt := start.Add(30 * time.Minute)
	if err := activity.UpsertObservation(t.Context(), runtimeactivity.ObservationSnapshot{
		InstanceID:    "activity",
		ActualStatus:  StatusRunning,
		ActiveSymbols: []string{"US.AAPL"},
		LastSignalAt:  &lastSignalAt,
		UpdatedAt:     &updatedAt,
	}); err != nil {
		t.Fatal(err)
	}
	items := service.ListInstances()
	if len(items) != 1 || items[0].RuntimeObservation == nil {
		t.Fatalf("enriched instances = %#v", items)
	}
	if items[0].RuntimeObservation.ActualStatus != StatusRunning ||
		len(items[0].RuntimeObservation.ActiveSymbols) != 1 ||
		items[0].RuntimeObservation.LastSignalAt == nil {
		t.Fatalf("runtime observation = %#v", items[0].RuntimeObservation)
	}
	if len(items[0].Logs) != 3 || items[0].Logs[2] != "error" {
		t.Fatalf("recent logs = %#v", items[0].Logs)
	}

	service.SetObservationSource(ObservationSourceFunc(func(string) (stratsrv.RuntimeObservation, bool) {
		return stratsrv.RuntimeObservation{ActualStatus: StatusPaused, ActiveSymbols: []string{"HK.00700"}}, true
	}))
	items = service.ListInstances()
	if items[0].RuntimeObservation == nil || items[0].RuntimeObservation.ActualStatus != StatusPaused {
		t.Fatalf("live observation should override persisted snapshot: %#v", items[0].RuntimeObservation)
	}
}
