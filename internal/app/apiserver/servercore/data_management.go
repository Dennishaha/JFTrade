package servercore

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jftrade/jftrade-main/internal/store/sqliteconn"

	"github.com/jftrade/jftrade-main/internal/app/apiserver/datamigration"
	dmsrv "github.com/jftrade/jftrade-main/internal/datamanagement"
	trdsrv "github.com/jftrade/jftrade-main/internal/trading"
	jfadk "github.com/jftrade/jftrade-main/pkg/adk"
	bt "github.com/jftrade/jftrade-main/pkg/backtest"
)

func translateDataManagementError(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, datamigration.ErrMaintenanceConflict):
		return fmt.Errorf("%w: %v", dmsrv.ErrDatabaseMaintenanceConflict, err)
	case errors.Is(err, datamigration.ErrPreviewNotFound):
		return fmt.Errorf("%w: %v", dmsrv.ErrCleanupPreviewNotFound, err)
	case errors.Is(err, datamigration.ErrPreviewStale):
		return fmt.Errorf("%w: %v", dmsrv.ErrCleanupPreviewStale, err)
	default:
		return err
	}
}

type dataManagementBackend struct {
	manager *datamigration.Manager
}

func (s *Server) newDataManagementService() *dmsrv.Service {
	if s == nil || s.dataMigration == nil {
		return dmsrv.NewService(nil)
	}
	return dmsrv.NewService(dataManagementBackend{manager: s.dataMigration})
}

func (b dataManagementBackend) Overview(ctx context.Context, request dmsrv.OverviewRequest) (any, error) {
	if b.manager == nil {
		return map[string]any{"databases": []any{}}, nil
	}
	return b.manager.Overview(ctx, datamigration.OverviewRequest{
		SummaryOnly: request.SummaryOnly,
		DatabaseID:  request.DatabaseID,
	})
}

func (b dataManagementBackend) PreviewCleanup(ctx context.Context, request dmsrv.CleanupPreviewRequest) (any, error) {
	if b.manager == nil {
		return nil, fmt.Errorf("database cleanup preview is unavailable")
	}
	result, err := b.manager.PreviewCleanup(ctx, datamigration.CleanupPreviewRequest{
		Kind:          request.Kind,
		DatabaseID:    request.DatabaseID,
		OlderThanDays: request.OlderThanDays,
		KeepLatest:    request.KeepLatest,
	})
	return result, translateDataManagementError(err)
}

func (b dataManagementBackend) ExecuteCleanup(ctx context.Context, request dmsrv.CleanupExecuteRequest) (any, error) {
	if b.manager == nil {
		return nil, fmt.Errorf("database cleanup is unavailable")
	}
	result, err := b.manager.ExecuteCleanup(ctx, datamigration.CleanupExecuteRequest{
		PreviewID:    request.PreviewID,
		Confirmation: request.Confirmation,
	})
	return result, translateDataManagementError(err)
}

func (b dataManagementBackend) Compact(ctx context.Context, databaseID string, request dmsrv.CompactRequest) (any, error) {
	if b.manager == nil {
		return nil, fmt.Errorf("database compaction is unavailable")
	}
	result, err := b.manager.Compact(ctx, databaseID, datamigration.CompactRequest{Confirmation: request.Confirmation})
	return result, translateDataManagementError(err)
}

func (b dataManagementBackend) Backup(ctx context.Context, request dmsrv.BackupRequest) (any, error) {
	if b.manager == nil {
		return nil, fmt.Errorf("database backup is unavailable")
	}
	result, err := b.manager.Backup(ctx, request.DatabaseID)
	if err != nil {
		return nil, translateDataManagementError(err)
	}
	return dmsrv.BackupResult{
		DatabaseID: result.DatabaseID,
		BackupPath: result.BackupPath,
		SizeBytes:  result.SizeBytes,
		CreatedAt:  result.CreatedAt,
	}, nil
}

func (b dataManagementBackend) Rebuild(ctx context.Context, request dmsrv.RebuildRequest) (any, error) {
	if b.manager == nil {
		return nil, fmt.Errorf("database rebuild is unavailable")
	}
	ids := append([]string{}, request.DatabaseIDs...)
	if strings.TrimSpace(request.DatabaseID) != "" {
		ids = append(ids, request.DatabaseID)
	}
	return b.manager.ScheduleRebuild(ctx, datamigration.RebuildRequest{
		DatabaseIDs:  ids,
		Mode:         request.Mode,
		Confirmation: request.Confirmation,
	})
}

func (s *Server) configureDataManagement() {
	if s == nil || s.dataMigration == nil {
		return
	}
	s.dataMigration.SetMaintenanceHooks(datamigration.MaintenanceHooks{
		BusyReason: s.databaseMaintenanceBusyReason,
		Purge:      s.purgeDatabaseCandidates,
		Compact:    s.compactDatabase,
	})
}

func (s *Server) databaseMaintenanceBusyReason(databaseID string) string {
	switch databaseID {
	case datamigration.DatabaseBacktest, datamigration.DatabaseBacktestRuns:
		if s.backtestRuns != nil {
			s.backtestRuns.mu.RLock()
			defer s.backtestRuns.mu.RUnlock()
			for _, run := range s.backtestRuns.runs {
				if run != nil && (run.Status == "queued" || run.Status == "running") {
					return "存在正在排队或运行的回测"
				}
			}
		}
		if s.backtestSyncTasks != nil {
			s.backtestSyncTasks.mu.RLock()
			defer s.backtestSyncTasks.mu.RUnlock()
			if len(s.backtestSyncTasks.cancels) > 0 {
				return "存在正在运行的行情同步"
			}
		}
	case datamigration.DatabaseStrategy:
		if s.strategyRuntimeManager != nil {
			s.strategyRuntimeManager.mu.RLock()
			defer s.strategyRuntimeManager.mu.RUnlock()
			if len(s.strategyRuntimeManager.runtimes) > 0 || len(s.strategyRuntimeManager.starting) > 0 {
				return "存在活动策略实例"
			}
		}
	case datamigration.DatabaseExecution:
		if s.executionOrders != nil {
			s.executionOrders.mu.RLock()
			defer s.executionOrders.mu.RUnlock()
			for _, order := range s.executionOrders.orders {
				if !trdsrv.IsTerminalOrderStatus(order.Status) {
					return "存在非终态执行订单"
				}
			}
		}
	case datamigration.DatabaseADK, datamigration.DatabaseADKSession:
		if s.adkRuntime != nil {
			active, err := s.adkRuntime.HasDatabaseActivity(context.Background())
			if err != nil {
				return "无法确认 ADK 运行状态"
			}
			if active {
				return "存在活动、暂停或等待审批的 ADK 运行"
			}
		}
	}
	return ""
}

func (s *Server) purgeDatabaseCandidates(ctx context.Context, databaseID string, candidates []datamigration.CleanupCandidate) (int, error) {
	switch databaseID {
	case datamigration.DatabaseStrategy:
		ids := make([]string, 0, len(candidates))
		for _, candidate := range candidates {
			ids = append(ids, candidate.ID)
		}
		return s.designStore.purgeDeletedDefinitions(ctx, ids)
	case datamigration.DatabaseADK:
		if s.adkRuntime == nil || s.adkRuntime.Store() == nil {
			return 0, fmt.Errorf("adk database is unavailable")
		}
		ids := jfadk.DeletedConfigIDs{}
		for _, candidate := range candidates {
			switch candidate.Category {
			case "智能体":
				ids.Agents = append(ids.Agents, candidate.ID)
			case "工作流":
				ids.Workflows = append(ids.Workflows, candidate.ID)
			case "触发器":
				ids.Triggers = append(ids.Triggers, candidate.ID)
			}
		}
		deleted, err := s.adkRuntime.Store().PurgeDeletedConfigs(ctx, ids)
		if errors.Is(err, jfadk.ErrCleanupCandidatesChanged) {
			return 0, datamigration.ErrPreviewStale
		}
		if err != nil {
			return 0, err
		}
		if deleted != len(candidates) {
			return 0, datamigration.ErrPreviewStale
		}
		return deleted, nil
	case datamigration.DatabaseBacktestRuns:
		ids := make([]string, 0, len(candidates))
		for _, candidate := range candidates {
			ids = append(ids, candidate.ID)
		}
		return s.backtestRuns.purgeTerminalRuns(ctx, ids)
	default:
		return 0, fmt.Errorf("cleanup is unsupported for database %q", databaseID)
	}
}

func (s *Server) compactDatabase(ctx context.Context, databaseID string) error {
	switch databaseID {
	case datamigration.DatabaseBacktest:
		store, err := bt.NewFutuKLineStore(s.dataMigrationPath(databaseID))
		if err != nil {
			return err
		}
		defer func() { _ = store.Close() }()
		return store.CompactDatabase(ctx)
	case datamigration.DatabaseBacktestRuns:
		if s.backtestRuns == nil || s.backtestRuns.db == nil {
			return fmt.Errorf("backtest run database is unavailable")
		}
		s.backtestRuns.mu.Lock()
		defer s.backtestRuns.mu.Unlock()
		return compactSQLX(ctx, s.backtestRuns.db)
	case datamigration.DatabaseStrategy:
		if s.designStore == nil || s.designStore.db == nil {
			return fmt.Errorf("strategy database is unavailable")
		}
		s.designStore.mu.Lock()
		defer s.designStore.mu.Unlock()
		return compactSQLX(ctx, s.designStore.db)
	case datamigration.DatabaseExecution:
		if s.executionOrders == nil || s.executionOrders.persistence == nil || s.executionOrders.persistence.db == nil {
			return fmt.Errorf("execution database is unavailable")
		}
		return compactSQLX(ctx, s.executionOrders.persistence.db)
	case datamigration.DatabaseADK:
		if s.adkRuntime == nil || s.adkRuntime.Store() == nil {
			return fmt.Errorf("adk database is unavailable")
		}
		return s.adkRuntime.Store().CompactDatabase(ctx)
	case datamigration.DatabaseADKSession:
		if s.adkRuntime == nil {
			return fmt.Errorf("adk session database is unavailable")
		}
		return s.adkRuntime.CompactSessionDatabase(ctx)
	default:
		return fmt.Errorf("unknown database id %q", databaseID)
	}
}

func (s *Server) dataMigrationPath(databaseID string) string {
	for _, status := range mustDatabaseStatuses(s.dataMigration) {
		if status.ID == databaseID {
			return status.Path
		}
	}
	return ""
}

func mustDatabaseStatuses(manager *datamigration.Manager) []datamigration.DatabaseStatus {
	if manager == nil {
		return nil
	}
	statuses, _ := manager.Statuses(context.Background())
	return statuses
}

func compactSQLX(ctx context.Context, db *sqliteconn.DB) error {
	if db == nil {
		return fmt.Errorf("database is unavailable")
	}
	if _, err := db.ExecContext(ctx, `PRAGMA wal_checkpoint(TRUNCATE)`); err != nil {
		return err
	}
	_, err := db.ExecContext(ctx, `VACUUM`)
	return err
}

func (s *strategyDesignStore) purgeDeletedDefinitions(ctx context.Context, ids []string) (int, error) {
	if s == nil || s.db == nil {
		return 0, fmt.Errorf("strategy database is unavailable")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	tx, err := s.db.BeginWrite(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()
	deleted := 0
	for _, id := range ids {
		result, err := tx.ExecContext(ctx, `DELETE FROM `+strategyDesignDefinitionTable+` WHERE id = ? AND deleted_at IS NOT NULL AND TRIM(deleted_at) <> ''`, strings.TrimSpace(id))
		if err != nil {
			return 0, err
		}
		count, err := result.RowsAffected()
		if err != nil {
			return 0, err
		}
		deleted += int(count)
	}
	if deleted != len(ids) {
		return 0, datamigration.ErrPreviewStale
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return deleted, nil
}

func (s *backtestRunStore) purgeTerminalRuns(ctx context.Context, ids []string) (int, error) {
	if s == nil || s.db == nil {
		return 0, fmt.Errorf("backtest run database is unavailable")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	tx, err := s.db.BeginWrite(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()
	deleted := 0
	for _, id := range ids {
		result, err := tx.ExecContext(ctx, `DELETE FROM `+backtestRunTable+` WHERE id = ? AND status IN ('completed', 'failed', 'cancelled')`, strings.TrimSpace(id))
		if err != nil {
			return 0, err
		}
		count, err := result.RowsAffected()
		if err != nil {
			return 0, err
		}
		deleted += int(count)
	}
	if deleted != len(ids) {
		return 0, datamigration.ErrPreviewStale
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	for _, id := range ids {
		delete(s.runs, strings.TrimSpace(id))
	}
	return deleted, nil
}
