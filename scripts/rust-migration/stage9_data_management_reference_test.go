package rustmigration

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jftrade/jftrade-main/internal/app/apiserver/datamigration"
	appruntime "github.com/jftrade/jftrade-main/internal/app/apiserver/runtime"
	"github.com/jftrade/jftrade-main/internal/store/sqliteconn"
	"github.com/jftrade/jftrade-main/internal/store/sqliteschema"
)

const stage9DataManagementVersion = "stage9.data-management-overview.v1"
const stage9DataManagementCleanupVersion = "stage9.data-management-cleanup-preview.v1"

type stage9DataManagementReference struct {
	Version      string                         `json:"version"`
	All          datamigration.OverviewResponse `json:"all"`
	Summary      datamigration.OverviewResponse `json:"summary"`
	Filtered     datamigration.OverviewResponse `json:"filtered"`
	UnknownError string                         `json:"unknownError"`
}

type stage9DataManagementCleanupCase struct {
	Name        string                              `json:"name"`
	Request     datamigration.CleanupPreviewRequest `json:"request"`
	EvaluatedAt string                              `json:"evaluatedAt,omitempty"`
	Response    *datamigration.CleanupPreview       `json:"response,omitempty"`
	Error       string                              `json:"error,omitempty"`
}

type stage9DataManagementCleanupReference struct {
	Version string                            `json:"version"`
	Cases   []stage9DataManagementCleanupCase `json:"cases"`
}

func TestStage9DataManagementOverviewReference(t *testing.T) {
	root := os.Getenv("JFTRADE_STAGE9_DATA_MANAGEMENT_ROOT")
	output := os.Getenv("JFTRADE_STAGE9_DATA_MANAGEMENT_REFERENCE")
	if root == "" || output == "" {
		return
	}
	settingsPath := filepath.Join(root, "settings.json")
	paths := stage9DataManagementPaths(t, root, settingsPath)
	if err := os.WriteFile(settingsPath, []byte("{}\n"), 0o600); err != nil {
		t.Fatalf("seed stage 9 settings: %v", err)
	}
	for _, definition := range sqliteschema.Definitions() {
		path := paths[definition.ID]
		db, err := sqliteconn.Open(path)
		if err != nil {
			t.Fatalf("open %s database: %v", definition.ID, err)
		}
		if err := sqliteschema.InitializeCurrent(context.Background(), db, path, definition.ID); err != nil {
			_ = db.Close()
			t.Fatalf("initialize %s database: %v", definition.ID, err)
		}
		seedStage9CleanableRows(t, db, definition.ID)
		if err := db.Close(); err != nil {
			t.Fatalf("close %s database: %v", definition.ID, err)
		}
	}
	if err := os.Remove(paths[datamigration.DatabaseWatchlist]); err != nil {
		t.Fatalf("remove watchlist database for missing-state corpus: %v", err)
	}
	research, err := sqliteconn.Open(paths[datamigration.DatabaseResearch])
	if err != nil {
		t.Fatalf("open research database for incompatible-state corpus: %v", err)
	}
	if _, err := research.Exec(`CREATE TABLE rogue_stage9_table (id TEXT PRIMARY KEY)`); err != nil {
		_ = research.Close()
		t.Fatalf("add incompatible research table: %v", err)
	}
	if err := research.Close(); err != nil {
		t.Fatalf("close incompatible research database: %v", err)
	}
	marker, err := json.Marshal(map[string]any{
		"databaseIds": []string{datamigration.DatabaseBacktest, datamigration.DatabaseStrategy},
		"backups":     []any{},
		"createdAt":   "2026-08-20T00:00:00Z",
	})
	if err != nil {
		t.Fatalf("encode rebuild marker: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, datamigration.RebuildMarkerFilename), marker, 0o600); err != nil {
		t.Fatalf("write rebuild marker: %v", err)
	}

	manager := datamigration.NewManager(settingsPath, paths[datamigration.DatabaseBacktest])
	all, err := manager.Overview(t.Context())
	if err != nil {
		t.Fatalf("read complete data-management overview: %v", err)
	}
	all.CheckedAt = "2026-08-20T00:00:00Z"
	summary, err := manager.Overview(t.Context(), datamigration.OverviewRequest{SummaryOnly: true})
	if err != nil {
		t.Fatalf("read summary data-management overview: %v", err)
	}
	summary.CheckedAt = "2026-08-20T00:00:00Z"
	filtered, err := manager.Overview(t.Context(), datamigration.OverviewRequest{DatabaseID: datamigration.DatabaseStrategy})
	if err != nil {
		t.Fatalf("read filtered data-management overview: %v", err)
	}
	filtered.CheckedAt = "2026-08-20T00:00:00Z"
	_, unknownErr := manager.Overview(t.Context(), datamigration.OverviewRequest{DatabaseID: "unknown"})
	reference := stage9DataManagementReference{
		Version: stage9DataManagementVersion, All: all, Summary: summary, Filtered: filtered,
	}
	if unknownErr != nil {
		reference.UnknownError = unknownErr.Error()
	}
	contents, err := json.MarshalIndent(reference, "", "  ")
	if err != nil {
		t.Fatalf("encode stage 9 data-management reference: %v", err)
	}
	if err := os.WriteFile(output, append(contents, '\n'), 0o600); err != nil {
		t.Fatalf("write stage 9 data-management reference: %v", err)
	}
}

// TestStage9DataManagementCleanupPreviewReference records only the preview
// side of cleanup. It deliberately never invokes ExecuteCleanup so the
// differential corpus cannot mutate the seeded databases.
func TestStage9DataManagementCleanupPreviewReference(t *testing.T) {
	root := os.Getenv("JFTRADE_STAGE9_DATA_MANAGEMENT_CLEANUP_ROOT")
	output := os.Getenv("JFTRADE_STAGE9_DATA_MANAGEMENT_CLEANUP_REFERENCE")
	if root == "" || output == "" {
		return
	}
	if err := os.MkdirAll(root, 0o700); err != nil {
		t.Fatalf("create cleanup corpus root: %v", err)
	}
	settingsPath := filepath.Join(root, "settings.json")
	paths := stage9DataManagementPaths(t, root, settingsPath)
	if err := os.WriteFile(settingsPath, []byte("{}\n"), 0o600); err != nil {
		t.Fatalf("seed cleanup settings: %v", err)
	}
	seedStage9CleanupDatabases(t, paths)

	manager := datamigration.NewManager(settingsPath, paths[datamigration.DatabaseBacktest])
	cases := make([]stage9DataManagementCleanupCase, 0, 7)
	appendPreview := func(name string, request datamigration.CleanupPreviewRequest) {
		t.Helper()
		response, err := manager.PreviewCleanup(t.Context(), request)
		if err != nil {
			t.Fatalf("preview %s: %v", name, err)
		}
		if len(response.PreviewID) != 32 {
			t.Fatalf("preview %s id length = %d, want 32", name, len(response.PreviewID))
		}
		expiresAt, err := time.Parse(time.RFC3339Nano, response.ExpiresAt)
		if err != nil {
			t.Fatalf("preview %s expiry: %v", name, err)
		}
		// Go's maintenance clock is intentionally private. Deriving the
		// evaluation instant from the exact response expiry gives Rust a
		// deterministic instant without adding a production test hook.
		cases = append(cases, stage9DataManagementCleanupCase{
			Name:        name,
			Request:     request,
			EvaluatedAt: expiresAt.Add(-10 * time.Minute).Format(time.RFC3339Nano),
			Response: &datamigration.CleanupPreview{
				PreviewID:        "00000000000000000000000000000000",
				ExpiresAt:        response.ExpiresAt,
				Kind:             response.Kind,
				DatabaseID:       response.DatabaseID,
				CandidateCount:   response.CandidateCount,
				EstimatedBytes:   response.EstimatedBytes,
				Items:            response.Items,
				ConfirmationText: response.ConfirmationText,
				WillCompact:      response.WillCompact,
			},
		})
	}
	appendError := func(name string, request datamigration.CleanupPreviewRequest) {
		t.Helper()
		_, err := manager.PreviewCleanup(t.Context(), request)
		if err == nil {
			t.Fatalf("invalid preview %s succeeded", name)
		}
		cases = append(cases, stage9DataManagementCleanupCase{Name: name, Request: request, Error: err.Error()})
	}

	appendPreview("backtest-defaults", datamigration.CleanupPreviewRequest{
		Kind: datamigration.CleanupBacktestHistory, DatabaseID: datamigration.DatabaseBacktestRuns,
	})
	appendPreview("backtest-cutoff-and-keep-latest", datamigration.CleanupPreviewRequest{
		Kind: datamigration.CleanupBacktestHistory, DatabaseID: datamigration.DatabaseBacktestRuns,
		OlderThanDays: 45, KeepLatest: 2,
	})
	appendPreview("strategy-soft-deleted", datamigration.CleanupPreviewRequest{
		Kind: datamigration.CleanupSoftDeleted, DatabaseID: datamigration.DatabaseStrategy,
	})
	appendPreview("adk-soft-deleted", datamigration.CleanupPreviewRequest{
		Kind: datamigration.CleanupSoftDeleted, DatabaseID: datamigration.DatabaseADK,
	})
	appendError("backtest-invalid-retention", datamigration.CleanupPreviewRequest{
		Kind: datamigration.CleanupBacktestHistory, DatabaseID: datamigration.DatabaseBacktestRuns,
		OlderThanDays: 3651, KeepLatest: 1,
	})
	appendError("unsupported-soft-deleted-database", datamigration.CleanupPreviewRequest{
		Kind: datamigration.CleanupSoftDeleted, DatabaseID: datamigration.DatabaseWatchlist,
	})
	appendError("unknown-cleanup-kind", datamigration.CleanupPreviewRequest{
		Kind: "unsupported", DatabaseID: datamigration.DatabaseStrategy,
	})

	reference := stage9DataManagementCleanupReference{
		Version: stage9DataManagementCleanupVersion,
		Cases:   cases,
	}
	contents, err := json.MarshalIndent(reference, "", "  ")
	if err != nil {
		t.Fatalf("encode cleanup reference: %v", err)
	}
	if err := os.WriteFile(output, append(contents, '\n'), 0o600); err != nil {
		t.Fatalf("write cleanup reference: %v", err)
	}
}

func seedStage9CleanupDatabases(t *testing.T, paths map[string]string) {
	t.Helper()
	for _, databaseID := range []string{
		datamigration.DatabaseBacktestRuns,
		datamigration.DatabaseStrategy,
		datamigration.DatabaseADK,
	} {
		path := paths[databaseID]
		db, err := sqliteconn.Open(path)
		if err != nil {
			t.Fatalf("open cleanup %s database: %v", databaseID, err)
		}
		if err := sqliteschema.InitializeCurrent(context.Background(), db, path, databaseID); err != nil {
			_ = db.Close()
			t.Fatalf("initialize cleanup %s database: %v", databaseID, err)
		}
		if err := seedStage9CleanupRows(db, databaseID); err != nil {
			_ = db.Close()
			t.Fatalf("seed cleanup %s rows: %v", databaseID, err)
		}
		if err := db.Close(); err != nil {
			t.Fatalf("close cleanup %s database: %v", databaseID, err)
		}
	}
}

func seedStage9CleanupRows(db *sqliteconn.DB, databaseID string) error {
	now := time.Now().UTC()
	switch databaseID {
	case datamigration.DatabaseBacktestRuns:
		for index := 0; index < 25; index++ {
			updated := now.Add(-time.Duration(40+index) * 24 * time.Hour).Format(time.RFC3339Nano)
			id := fmt.Sprintf("run-%02d", index)
			request := fmt.Sprintf("req-%02d", index)
			result := fmt.Sprintf("result-%02d", index)
			if _, err := db.Exec(`INSERT INTO backtest_runs (id, status, request_json, result_json, created_at, updated_at) VALUES (?, 'completed', ?, ?, ?, ?)`, id, request, result, updated, updated); err != nil {
				return err
			}
		}
		_, err := db.Exec(`INSERT INTO backtest_runs (id, status, request_json, result_json, created_at, updated_at) VALUES ('running-old', 'running', '{}', '{}', ?, ?)`, now.Add(-100*24*time.Hour).Format(time.RFC3339Nano), now.Add(-100*24*time.Hour).Format(time.RFC3339Nano))
		return err
	case datamigration.DatabaseStrategy:
		_, err := db.Exec(`INSERT INTO strategy_design_definitions (id, script, visual_model_json, deleted_at) VALUES ('strategy-deleted', 'plot(close)', '{"plot":true}', '2026-01-01T00:00:00Z'), ('strategy-active', 'plot(open)', '{}', NULL), ('strategy-blank-delete', 'plot(high)', '{}', '   ')`)
		return err
	case datamigration.DatabaseADK:
		const timestamp = "2026-01-01T00:00:00Z"
		if _, err := db.Exec(`INSERT INTO adk_agents (id, payload_json, created_at, updated_at) VALUES ('agent-deleted', '{"deletedAt":"2026-01-01T00:00:00Z","name":"old"}', ?, ?), ('agent-active', '{}', ?, ?)`, timestamp, timestamp, timestamp, timestamp); err != nil {
			return err
		}
		if _, err := db.Exec(`INSERT INTO adk_workflows (id, status, payload_json, created_at, updated_at) VALUES ('workflow-deleted', 'deleted', '{"deletedAt":"2026-01-01T00:00:00Z"}', ?, ?), ('workflow-active', 'active', '{}', ?, ?)`, timestamp, timestamp, timestamp, timestamp); err != nil {
			return err
		}
		_, err := db.Exec(`INSERT INTO adk_workflow_triggers (id, workflow_id, trigger_type, status, next_run_at, payload_json, created_at, updated_at) VALUES ('trigger-direct', 'workflow-active', 'manual', 'disabled', '', '{"deletedAt":"2026-01-01T00:00:00Z"}', ?, ?), ('trigger-child', 'workflow-deleted', 'manual', 'disabled', '', '{}', ?, ?), ('trigger-active', 'workflow-active', 'manual', 'enabled', '', '{}', ?, ?)`, timestamp, timestamp, timestamp, timestamp, timestamp, timestamp)
		return err
	default:
		return nil
	}
}

func stage9DataManagementPaths(t *testing.T, root, settingsPath string) map[string]string {
	t.Helper()
	overrides := map[string]string{
		"JFTRADE_BACKTEST_DB":         filepath.Join(root, "backtest.db"),
		"JFTRADE_BACKTEST_RUN_DB":     filepath.Join(root, "backtest-runs.db"),
		"JFTRADE_STRATEGY_RUNTIME_DB": filepath.Join(root, "strategy-runtime.db"),
		"JFTRADE_EXECUTION_ORDER_DB":  filepath.Join(root, "execution-orders.db"),
		"JFTRADE_ADK_DB":              filepath.Join(root, "adk.db"),
		"JFTRADE_ADK_SESSION_DB":      filepath.Join(root, "adk-session.db"),
		"JFTRADE_WATCHLIST_DB":        filepath.Join(root, "watchlists.db"),
		"JFTRADE_RESEARCH_DB":         filepath.Join(root, "research.db"),
	}
	for name, value := range overrides {
		t.Setenv(name, value)
	}
	return map[string]string{
		datamigration.DatabaseBacktest:     overrides["JFTRADE_BACKTEST_DB"],
		datamigration.DatabaseBacktestRuns: appruntime.DeriveBacktestRunDBPath(settingsPath),
		datamigration.DatabaseStrategy:     appruntime.DeriveStrategyRuntimeDBPath(settingsPath),
		datamigration.DatabaseExecution:    appruntime.DeriveExecutionOrderDBPath(settingsPath),
		datamigration.DatabaseADK:          appruntime.DeriveADKDBPath(settingsPath),
		datamigration.DatabaseADKSession:   appruntime.DeriveADKSessionDBPath(settingsPath),
		datamigration.DatabaseADKArtifact:  appruntime.DeriveADKArtifactDBPath(settingsPath),
		datamigration.DatabaseWatchlist:    appruntime.DeriveWatchlistDBPath(settingsPath),
		datamigration.DatabaseResearch:     appruntime.DeriveResearchDBPath(settingsPath),
	}
}

func seedStage9CleanableRows(t *testing.T, db *sqliteconn.DB, databaseID string) {
	t.Helper()
	var statement string
	switch databaseID {
	case datamigration.DatabaseBacktestRuns:
		statement = `INSERT INTO backtest_runs (id, status, request_json, result_json) VALUES ('run-1', 'completed', '{}', '{"ok":true}')`
	case datamigration.DatabaseStrategy:
		statement = `INSERT INTO strategy_design_definitions (id, script, visual_model_json, deleted_at) VALUES ('strategy-1', 'plot(close)', '{}', '2026-08-20T00:00:00Z')`
	case datamigration.DatabaseADK:
		statement = `INSERT INTO adk_agents (id, payload_json, created_at, updated_at) VALUES ('agent-1', '{"deletedAt":"2026-08-20T00:00:00Z"}', '2026-08-20T00:00:00Z', '2026-08-20T00:00:00Z')`
	default:
		return
	}
	if _, err := db.Exec(statement); err != nil {
		t.Fatalf("seed %s cleanable row: %v", databaseID, err)
	}
}
