package servercore

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jftrade/jftrade-main/internal/app/apiserver/datamigration"
	assistant "github.com/jftrade/jftrade-main/internal/assistant"
	dmsrv "github.com/jftrade/jftrade-main/internal/datamanagement"
)

func TestDataManagementADKCleanupAndCompactionPaths(t *testing.T) {
	root := t.TempDir()
	t.Setenv("JFTRADE_BACKTEST_DB", filepath.Join(root, "backtest.db"))
	settings, err := NewSettingsStore(filepath.Join(root, "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	server := newTestServer(t, settings)

	adkStore := serverADKTestStore(t, server)
	agent, err := adkStore.SaveAgent(t.Context(), assistant.AgentWriteRequest{
		ID: "cleanup-agent", Name: "Cleanup Agent", Status: assistant.AgentStatusEnabled,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := adkStore.DeleteAgent(t.Context(), agent.ID); err != nil {
		t.Fatal(err)
	}
	previewValue, err := server.dataManagementSvc.PreviewCleanup(t.Context(), dmsrv.CleanupPreviewRequest{
		Kind: datamigration.CleanupSoftDeleted, DatabaseID: datamigration.DatabaseADK,
	})
	if err != nil {
		t.Fatal(err)
	}
	preview := previewValue.(datamigration.CleanupPreview)
	if _, err := server.dataManagementSvc.ExecuteCleanup(t.Context(), dmsrv.CleanupExecuteRequest{
		PreviewID: preview.PreviewID, Confirmation: preview.ConfirmationText,
	}); err != nil {
		t.Fatal(err)
	}

	for _, databaseID := range []string{
		datamigration.DatabaseADK,
		datamigration.DatabaseADKSession,
		datamigration.DatabaseADKArtifact,
	} {
		if _, err := server.dataManagementSvc.Compact(t.Context(), databaseID, dmsrv.CompactRequest{
			Confirmation: "COMPACT " + databaseID,
		}); err != nil {
			t.Fatalf("compact %s: %v", databaseID, err)
		}
	}
}

func TestDatabaseMaintenanceADKBusyAndPurgeBoundaries(t *testing.T) {
	root := t.TempDir()
	settings, err := NewSettingsStore(filepath.Join(root, "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	server := newTestServer(t, settings)
	if err := serverADKTestStore(t, server).SaveRun(t.Context(), assistant.Run{
		ID: "active-run", Status: assistant.RunStatusRunning,
	}); err != nil {
		t.Fatalf("SaveRun: %v", err)
	}
	if reason := server.newMaintenanceRegistry().BusyReason(t.Context(), datamigration.DatabaseADK); !strings.Contains(reason, "ADK") {
		t.Fatalf("ADK busy reason = %q", reason)
	}

	bare := &Server{}
	if _, err := bare.newMaintenanceRegistry().Purge(t.Context(), datamigration.DatabaseADK, nil); err == nil {
		t.Fatal("nil ADK store purge error = nil")
	}
	maintenance := server.newMaintenanceRegistry()
	candidates := []dmsrv.CleanupCandidate{
		{ID: "missing-agent", Category: "智能体"},
		{ID: "missing-workflow", Category: "工作流"},
		{ID: "missing-trigger", Category: "触发器"},
	}
	if _, err := maintenance.Purge(t.Context(), datamigration.DatabaseADK, candidates); !errors.Is(err, dmsrv.ErrCleanupCandidatesChanged) {
		t.Fatalf("changed ADK candidates error = %v", err)
	}

	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := maintenance.Purge(canceled, datamigration.DatabaseADK, []dmsrv.CleanupCandidate{{
		ID: "missing-agent", Category: "智能体",
	}}); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled ADK purge error = %v", err)
	}
}
