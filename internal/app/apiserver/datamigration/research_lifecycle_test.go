package datamigration

import (
	"os"
	"path/filepath"
	"testing"

	apiruntime "github.com/jftrade/jftrade-main/internal/app/apiserver/runtime"
	researchstore "github.com/jftrade/jftrade-main/internal/store/research"
)

func TestResearchDatabaseParticipatesInStatusBackupAndRebuild(t *testing.T) {
	root := t.TempDir()
	settingsPath := filepath.Join(root, "settings.json")
	manager := NewManager(settingsPath, filepath.Join(root, "backtest.db"))
	path := apiruntime.DeriveResearchDBPath(settingsPath)
	store, err := researchstore.Open(t.Context(), path)
	if err != nil {
		t.Fatalf("open research store: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("close research store: %v", err)
	}

	statuses, err := manager.Statuses(t.Context())
	if err != nil {
		t.Fatalf("Statuses: %v", err)
	}
	var found *DatabaseStatus
	for index := range statuses {
		if statuses[index].ID == DatabaseResearch {
			found = &statuses[index]
			break
		}
	}
	if found == nil || found.Status != "ready" || found.CurrentVersion == nil || *found.CurrentVersion != ResearchSchemaVersion {
		t.Fatalf("research status = %#v", found)
	}
	backup, err := manager.Backup(t.Context(), DatabaseResearch, BackupConfirmationText(DatabaseResearch))
	if err != nil {
		t.Fatalf("Backup: %v", err)
	}
	if backup.DatabaseID != DatabaseResearch || backup.SizeBytes == 0 {
		t.Fatalf("backup = %#v", backup)
	}
	if _, err := os.Stat(backup.BackupPath); err != nil {
		t.Fatalf("backup path: %v", err)
	}

	if _, err := manager.ScheduleRebuild(t.Context(), RebuildRequest{
		Mode: "single", DatabaseIDs: []string{DatabaseResearch}, Confirmation: "REBUILD " + DatabaseResearch,
	}); err != nil {
		t.Fatalf("ScheduleRebuild: %v", err)
	}
	if err := manager.ApplyPending(); err != nil {
		t.Fatalf("ApplyPending: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("research database still exists after ApplyPending: %v", err)
	}
	reopened, err := researchstore.Open(t.Context(), path)
	if err != nil {
		t.Fatalf("reopen research store: %v", err)
	}
	if err := reopened.Close(); err != nil {
		t.Fatalf("close rebuilt research store: %v", err)
	}
	if err := manager.CompletePending(t.Context()); err != nil {
		t.Fatalf("CompletePending: %v", err)
	}
}
