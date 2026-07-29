package assembly

import (
	"errors"
	"testing"

	jfadk "github.com/jftrade/jftrade-main/internal/assistant/engine"
	dmsrv "github.com/jftrade/jftrade-main/internal/datamanagement"
)

func TestDatabaseMaintenanceOwnsADKBusyPurgeAndCompactPaths(t *testing.T) {
	handle, err := Open(Options{Paths: testRuntimePaths(t.TempDir())})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if closeErr := handle.Close(); closeErr != nil {
			t.Fatalf("Close: %v", closeErr)
		}
	})
	runtime := handle.runtime

	config := handle.DatabaseMaintenance(MaintenanceRuntimeDatabase)
	session := handle.DatabaseMaintenance(MaintenanceSessionDatabase)
	artifact := handle.DatabaseMaintenance(MaintenanceArtifactDatabase)

	agent, err := runtime.Store().SaveAgent(t.Context(), jfadk.AgentWriteRequest{
		ID:     "cleanup-agent",
		Name:   "Cleanup Agent",
		Status: jfadk.AgentStatusEnabled,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := runtime.Store().DeleteAgent(t.Context(), agent.ID); err != nil {
		t.Fatal(err)
	}
	deleted, err := config.PurgeMaintenanceCandidates(
		t.Context(),
		[]dmsrv.CleanupCandidate{{ID: agent.ID, Category: "智能体"}},
	)
	if err != nil || deleted != 1 {
		t.Fatalf("purge = %d, %v", deleted, err)
	}
	if _, err := config.PurgeMaintenanceCandidates(
		t.Context(),
		[]dmsrv.CleanupCandidate{{ID: "unknown", Category: "未来类型"}},
	); !errors.Is(err, dmsrv.ErrCleanupCandidatesChanged) {
		t.Fatalf("unknown category error = %v", err)
	}

	run := jfadk.Run{ID: "active-run", Status: jfadk.RunStatusRunning}
	if err := runtime.Store().SaveRun(t.Context(), run); err != nil {
		t.Fatal(err)
	}
	for _, maintenance := range []*DatabaseMaintenance{config, session, artifact} {
		if reason := maintenance.MaintenanceBusyReason(t.Context()); reason == "" {
			t.Fatalf("%s maintenance did not report active run", maintenance.resource)
		}
	}
	run.Status = jfadk.RunStatusCompleted
	if err := runtime.Store().SaveRun(t.Context(), run); err != nil {
		t.Fatal(err)
	}
	if reason := config.MaintenanceBusyReason(t.Context()); reason != "" {
		t.Fatalf("completed run remained busy: %q", reason)
	}

	for _, maintenance := range []*DatabaseMaintenance{config, session, artifact} {
		if err := maintenance.CompactMaintenanceResource(t.Context()); err != nil {
			t.Fatalf("compact %s: %v", maintenance.resource, err)
		}
	}
}

func TestDatabaseMaintenanceFailsClosedWithoutOwnedRuntime(t *testing.T) {
	var handle *Handle
	for _, resource := range []MaintenanceResource{
		MaintenanceRuntimeDatabase,
		MaintenanceSessionDatabase,
		MaintenanceArtifactDatabase,
		"unknown",
	} {
		maintenance := handle.DatabaseMaintenance(resource)
		if reason := maintenance.MaintenanceBusyReason(t.Context()); reason != "" {
			t.Fatalf("%s nil busy reason = %q", resource, reason)
		}
		if err := maintenance.CompactMaintenanceResource(t.Context()); err == nil {
			t.Fatalf("%s nil compact succeeded", resource)
		}
	}
	if _, err := handle.DatabaseMaintenance(
		MaintenanceRuntimeDatabase,
	).PurgeMaintenanceCandidates(t.Context(), nil); err == nil {
		t.Fatal("nil runtime purge succeeded")
	}
	if err := (&Handle{runtime: &jfadk.Runtime{}}).DatabaseMaintenance(
		"unknown",
	).CompactMaintenanceResource(t.Context()); err == nil {
		t.Fatal("unknown resource compact succeeded")
	}
}
