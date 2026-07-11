package datamigration

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/jftrade/jftrade-main/internal/store/sqliteconn"
)

const (
	CleanupSoftDeleted     = "soft-deleted"
	CleanupBacktestHistory = "backtest-history"
	previewLifetime        = 10 * time.Minute
	backupMinimumInterval  = 30 * time.Second
	backupRetentionPerDB   = 3
	backupQuotaFloorBytes  = int64(5 << 30)
)

var (
	ErrMaintenanceConflict = errors.New("database maintenance conflict")
	ErrPreviewNotFound     = errors.New("cleanup preview not found or expired")
	ErrPreviewStale        = errors.New("cleanup preview is stale")
	ErrBackupRateLimited   = errors.New("database backup rate limit exceeded")
	ErrBackupQuotaExceeded = errors.New("database backup storage quota exceeded")
)

type StorageStats struct {
	MainBytes        int64  `json:"mainBytes"`
	WALBytes         int64  `json:"walBytes"`
	SHMBytes         int64  `json:"shmBytes"`
	TotalBytes       int64  `json:"totalBytes"`
	FreePageBytes    int64  `json:"freePageBytes"`
	ReclaimableBytes int64  `json:"reclaimableBytes"`
	Error            string `json:"error,omitempty"`
}

type CleanableItem struct {
	Kind           string `json:"kind"`
	Label          string `json:"label"`
	Count          int    `json:"count"`
	EstimatedBytes int64  `json:"estimatedBytes"`
}

type DatabaseOverview struct {
	DatabaseStatus
	Storage   StorageStats    `json:"storage"`
	Cleanable []CleanableItem `json:"cleanable"`
}

type OverviewTotals struct {
	MainBytes        int64 `json:"mainBytes"`
	WALBytes         int64 `json:"walBytes"`
	SHMBytes         int64 `json:"shmBytes"`
	TotalBytes       int64 `json:"totalBytes"`
	ReclaimableBytes int64 `json:"reclaimableBytes"`
}

type OverviewResponse struct {
	Databases []DatabaseOverview `json:"databases"`
	Totals    OverviewTotals     `json:"totals"`
	CheckedAt string             `json:"checkedAt"`
}

type OverviewRequest struct {
	SummaryOnly bool
	DatabaseID  string
}

type CleanupPreviewRequest struct {
	Kind          string `json:"kind"`
	DatabaseID    string `json:"databaseId"`
	OlderThanDays int    `json:"olderThanDays,omitempty"`
	KeepLatest    int    `json:"keepLatest,omitempty"`
}

type CleanupPreview struct {
	PreviewID        string          `json:"previewId"`
	ExpiresAt        string          `json:"expiresAt"`
	Kind             string          `json:"kind"`
	DatabaseID       string          `json:"databaseId"`
	CandidateCount   int             `json:"candidateCount"`
	EstimatedBytes   int64           `json:"estimatedBytes"`
	Items            []CleanableItem `json:"items"`
	ConfirmationText string          `json:"confirmationText"`
	WillCompact      bool            `json:"willCompact"`
}

type CleanupExecuteRequest struct {
	PreviewID    string `json:"previewId"`
	Confirmation string `json:"confirmation"`
}

type CleanupResult struct {
	DatabaseID     string `json:"databaseId"`
	DeletedCount   int    `json:"deletedCount"`
	EstimatedBytes int64  `json:"estimatedBytes"`
	BeforeBytes    int64  `json:"beforeBytes"`
	AfterBytes     int64  `json:"afterBytes"`
	ReclaimedBytes int64  `json:"reclaimedBytes"`
	Compacted      bool   `json:"compacted"`
	Warning        string `json:"warning,omitempty"`
}

type CompactRequest struct {
	Confirmation string `json:"confirmation"`
}

type CompactResult struct {
	DatabaseID     string `json:"databaseId"`
	BeforeBytes    int64  `json:"beforeBytes"`
	AfterBytes     int64  `json:"afterBytes"`
	ReclaimedBytes int64  `json:"reclaimedBytes"`
	Compacted      bool   `json:"compacted"`
}

type BackupResult struct {
	DatabaseID string `json:"databaseId"`
	BackupPath string `json:"backupPath"`
	SizeBytes  int64  `json:"sizeBytes"`
	CreatedAt  string `json:"createdAt"`
}

type CleanupCandidate struct {
	ID             string
	Category       string
	EstimatedBytes int64
}

type MaintenanceHooks struct {
	BusyReason func(databaseID string) string
	Purge      func(context.Context, string, []CleanupCandidate) (int, error)
	Compact    func(context.Context, string) error
}

type storedPreview struct {
	response   CleanupPreview
	candidates []CleanupCandidate
	expiresAt  time.Time
}

type maintenanceState struct {
	mu         sync.Mutex
	previews   map[string]storedPreview
	locks      map[string]*sync.Mutex
	hooks      MaintenanceHooks
	now        func() time.Time
	backupLock sync.Mutex
	backupLast map[string]time.Time
}

func (m *Manager) initializeMaintenance() {
	m.maintenance = maintenanceState{
		previews:   make(map[string]storedPreview),
		locks:      make(map[string]*sync.Mutex),
		now:        time.Now,
		backupLast: make(map[string]time.Time),
	}
	for _, descriptor := range m.descriptors {
		m.maintenance.locks[descriptor.ID] = &sync.Mutex{}
	}
}

func (m *Manager) SetMaintenanceHooks(hooks MaintenanceHooks) {
	m.maintenance.mu.Lock()
	m.maintenance.hooks = hooks
	m.maintenance.mu.Unlock()
}

func (m *Manager) Overview(ctx context.Context, requests ...OverviewRequest) (OverviewResponse, error) {
	var request OverviewRequest
	if len(requests) > 0 {
		request = requests[0]
	}
	request.DatabaseID = strings.TrimSpace(request.DatabaseID)
	statuses, err := m.Statuses(ctx)
	if err != nil {
		return OverviewResponse{}, err
	}
	response := OverviewResponse{
		Databases: make([]DatabaseOverview, 0, len(statuses)),
		CheckedAt: m.maintenance.now().UTC().Format(time.RFC3339Nano),
	}
	for _, status := range statuses {
		if request.DatabaseID != "" && status.ID != request.DatabaseID {
			continue
		}
		overview := DatabaseOverview{DatabaseStatus: status}
		if !request.SummaryOnly {
			overview.Storage = inspectStorage(ctx, status)
		}
		if !request.SummaryOnly && status.Status == "ready" {
			overview.Cleanable = inspectCleanable(ctx, status)
		}
		response.Databases = append(response.Databases, overview)
		response.Totals.MainBytes += overview.Storage.MainBytes
		response.Totals.WALBytes += overview.Storage.WALBytes
		response.Totals.SHMBytes += overview.Storage.SHMBytes
		response.Totals.TotalBytes += overview.Storage.TotalBytes
		response.Totals.ReclaimableBytes += overview.Storage.ReclaimableBytes
	}
	if request.DatabaseID != "" && len(response.Databases) == 0 {
		return OverviewResponse{}, fmt.Errorf("unknown database id %q", request.DatabaseID)
	}
	return response, nil
}

func inspectStorage(ctx context.Context, status DatabaseStatus) StorageStats {
	stats := StorageStats{
		MainBytes: fileSize(status.Path),
		WALBytes:  fileSize(status.Path + "-wal"),
		SHMBytes:  fileSize(status.Path + "-shm"),
	}
	stats.TotalBytes = stats.MainBytes + stats.WALBytes + stats.SHMBytes
	stats.ReclaimableBytes = stats.WALBytes
	if status.Status != "ready" {
		return stats
	}
	db, err := sqliteconn.OpenReadOnly(status.Path)
	if err != nil {
		stats.Error = err.Error()
		return stats
	}
	defer func() { _ = db.Close() }()
	var pageSize, freePages int64
	if err := db.QueryRowContext(ctx, `SELECT page_size, freelist_count FROM pragma_page_size(), pragma_freelist_count()`).Scan(&pageSize, &freePages); err != nil {
		stats.Error = err.Error()
		return stats
	}
	stats.FreePageBytes = pageSize * freePages
	stats.ReclaimableBytes += stats.FreePageBytes
	return stats
}

func inspectCleanable(ctx context.Context, status DatabaseStatus) []CleanableItem {
	db, err := sqliteconn.OpenReadOnly(status.Path)
	if err != nil {
		return nil
	}
	defer func() { _ = db.Close() }()
	switch status.ID {
	case DatabaseStrategy:
		return queryCleanable(ctx, db, CleanupSoftDeleted, "已删除策略", `SELECT COUNT(*), COALESCE(SUM(LENGTH(script) + LENGTH(visual_model_json)), 0) FROM strategy_design_definitions WHERE deleted_at IS NOT NULL AND TRIM(deleted_at) <> ''`)
	case DatabaseADK:
		items := make([]CleanableItem, 0, 3)
		queries := []struct{ label, table string }{{"已删除智能体", "adk_agents"}, {"已删除工作流", "adk_workflows"}, {"已删除触发器", "adk_workflow_triggers"}}
		for _, query := range queries {
			items = append(items, queryCleanable(ctx, db, CleanupSoftDeleted, query.label, `SELECT COUNT(*), COALESCE(SUM(LENGTH(payload_json)), 0) FROM `+query.table+` WHERE COALESCE(json_extract(payload_json, '$.deletedAt'), '') <> ''`)...)
		}
		return items
	case DatabaseBacktestRuns:
		return queryCleanable(ctx, db, CleanupBacktestHistory, "已结束回测", `SELECT COUNT(*), COALESCE(SUM(LENGTH(request_json) + LENGTH(result_json)), 0) FROM backtest_runs WHERE status IN ('completed', 'failed', 'cancelled')`)
	default:
		return nil
	}
}

func queryCleanable(ctx context.Context, db sqliteReader, kind, label, query string) []CleanableItem {
	var count int
	var bytes int64
	if err := db.QueryRowContext(ctx, query).Scan(&count, &bytes); err != nil {
		return nil
	}
	return []CleanableItem{{Kind: kind, Label: label, Count: count, EstimatedBytes: bytes}}
}

func fileSize(path string) int64 {
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() {
		return 0
	}
	return info.Size()
}

func (m *Manager) PreviewCleanup(ctx context.Context, request CleanupPreviewRequest) (CleanupPreview, error) {
	request.Kind = strings.TrimSpace(request.Kind)
	request.DatabaseID = strings.TrimSpace(request.DatabaseID)
	switch request.Kind {
	case CleanupBacktestHistory:
		if request.DatabaseID != DatabaseBacktestRuns {
			return CleanupPreview{}, fmt.Errorf("backtest history cleanup requires %s", DatabaseBacktestRuns)
		}
		if request.OlderThanDays == 0 {
			request.OlderThanDays = 30
		}
		if request.KeepLatest == 0 {
			request.KeepLatest = 20
		}
		if request.OlderThanDays < 1 || request.OlderThanDays > 3650 || request.KeepLatest < 1 || request.KeepLatest > 10000 {
			return CleanupPreview{}, fmt.Errorf("backtest retention must use 1-3650 days and keep 1-10000 runs")
		}
	case CleanupSoftDeleted:
		if request.DatabaseID != DatabaseStrategy && request.DatabaseID != DatabaseADK {
			return CleanupPreview{}, fmt.Errorf("soft-deleted cleanup is unsupported for database %q", request.DatabaseID)
		}
	default:
		return CleanupPreview{}, fmt.Errorf("unknown cleanup kind %q", request.Kind)
	}
	descriptor, ok := m.descriptorMap()[request.DatabaseID]
	if !ok {
		return CleanupPreview{}, fmt.Errorf("unknown database id %q", request.DatabaseID)
	}
	status, err := m.currentDatabaseStatus(ctx, request.DatabaseID)
	if err != nil {
		return CleanupPreview{}, err
	}
	if status.Status != "ready" || status.RebuildScheduled {
		return CleanupPreview{}, fmt.Errorf("database %s is not ready for cleanup", request.DatabaseID)
	}
	candidates, err := cleanupCandidates(ctx, descriptor, request, m.maintenance.now())
	if err != nil {
		return CleanupPreview{}, err
	}
	items, totalBytes := summarizeCandidates(candidates)
	now := m.maintenance.now().UTC()
	previewID, err := newPreviewID()
	if err != nil {
		return CleanupPreview{}, err
	}
	response := CleanupPreview{
		PreviewID: previewID, ExpiresAt: now.Add(previewLifetime).Format(time.RFC3339Nano),
		Kind: request.Kind, DatabaseID: request.DatabaseID, CandidateCount: len(candidates),
		EstimatedBytes: totalBytes, Items: items, WillCompact: true,
		ConfirmationText: fmt.Sprintf("CLEANUP %s %d", request.DatabaseID, len(candidates)),
	}
	m.maintenance.mu.Lock()
	m.pruneExpiredPreviewsLocked(now)
	m.maintenance.previews[previewID] = storedPreview{response: response, candidates: candidates, expiresAt: now.Add(previewLifetime)}
	m.maintenance.mu.Unlock()
	return response, nil
}

func cleanupCandidates(ctx context.Context, descriptor Descriptor, request CleanupPreviewRequest, now time.Time) ([]CleanupCandidate, error) {
	db, err := sqliteconn.OpenReadOnly(descriptor.Path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = db.Close() }()
	if request.Kind == CleanupBacktestHistory {
		rows, err := db.QueryContext(ctx, `SELECT id, updated_at, LENGTH(request_json) + LENGTH(result_json) FROM backtest_runs WHERE status IN ('completed', 'failed', 'cancelled') ORDER BY updated_at DESC, id ASC`)
		if err != nil {
			return nil, err
		}
		defer func() { _ = rows.Close() }()
		cutoff := now.UTC().Add(-time.Duration(request.OlderThanDays) * 24 * time.Hour)
		candidates := []CleanupCandidate{}
		index := 0
		for rows.Next() {
			var id, updatedAt string
			var bytes int64
			if err := rows.Scan(&id, &updatedAt, &bytes); err != nil {
				return nil, err
			}
			updated, err := time.Parse(time.RFC3339Nano, updatedAt)
			if err == nil && index >= request.KeepLatest && updated.Before(cutoff) {
				candidates = append(candidates, CleanupCandidate{ID: id, Category: "回测结果", EstimatedBytes: bytes})
			}
			index++
		}
		return candidates, rows.Err()
	}
	if descriptor.ID == DatabaseStrategy {
		return queryCandidates(ctx, db, `SELECT id, LENGTH(script) + LENGTH(visual_model_json) FROM strategy_design_definitions WHERE deleted_at IS NOT NULL AND TRIM(deleted_at) <> '' ORDER BY id`, "策略定义")
	}
	candidates := []CleanupCandidate{}
	for _, query := range []struct{ sql, category string }{
		{`SELECT id, LENGTH(payload_json) FROM adk_agents WHERE COALESCE(json_extract(payload_json, '$.deletedAt'), '') <> '' ORDER BY id`, "智能体"},
		{`SELECT id, LENGTH(payload_json) FROM adk_workflows WHERE COALESCE(json_extract(payload_json, '$.deletedAt'), '') <> '' ORDER BY id`, "工作流"},
		{`SELECT id, LENGTH(payload_json) FROM adk_workflow_triggers WHERE COALESCE(json_extract(payload_json, '$.deletedAt'), '') <> '' OR workflow_id IN (SELECT id FROM adk_workflows WHERE COALESCE(json_extract(payload_json, '$.deletedAt'), '') <> '') ORDER BY id`, "触发器"},
	} {
		items, err := queryCandidates(ctx, db, query.sql, query.category)
		if err != nil {
			return nil, err
		}
		candidates = append(candidates, items...)
	}
	return candidates, nil
}

func queryCandidates(ctx context.Context, db sqliteReader, query, category string) ([]CleanupCandidate, error) {
	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	items := []CleanupCandidate{}
	for rows.Next() {
		var id string
		var bytes int64
		if err := rows.Scan(&id, &bytes); err != nil {
			return nil, err
		}
		items = append(items, CleanupCandidate{ID: id, Category: category, EstimatedBytes: bytes})
	}
	return items, rows.Err()
}

type sqliteReader interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func summarizeCandidates(candidates []CleanupCandidate) ([]CleanableItem, int64) {
	byCategory := map[string]*CleanableItem{}
	var total int64
	for _, candidate := range candidates {
		item := byCategory[candidate.Category]
		if item == nil {
			item = &CleanableItem{Kind: candidate.Category, Label: candidate.Category}
			byCategory[candidate.Category] = item
		}
		item.Count++
		item.EstimatedBytes += candidate.EstimatedBytes
		total += candidate.EstimatedBytes
	}
	items := make([]CleanableItem, 0, len(byCategory))
	for _, item := range byCategory {
		items = append(items, *item)
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Label < items[j].Label })
	return items, total
}

func (m *Manager) ExecuteCleanup(ctx context.Context, request CleanupExecuteRequest) (CleanupResult, error) {
	now := m.maintenance.now().UTC()
	m.maintenance.mu.Lock()
	m.pruneExpiredPreviewsLocked(now)
	preview, ok := m.maintenance.previews[strings.TrimSpace(request.PreviewID)]
	hooks := m.maintenance.hooks
	m.maintenance.mu.Unlock()
	if !ok {
		return CleanupResult{}, ErrPreviewNotFound
	}
	if request.Confirmation != preview.response.ConfirmationText {
		return CleanupResult{}, fmt.Errorf("confirmation text does not match")
	}
	lock := m.maintenance.locks[preview.response.DatabaseID]
	if lock == nil || !lock.TryLock() {
		return CleanupResult{}, ErrMaintenanceConflict
	}
	defer lock.Unlock()
	if hooks.BusyReason != nil {
		if reason := strings.TrimSpace(hooks.BusyReason(preview.response.DatabaseID)); reason != "" {
			return CleanupResult{}, fmt.Errorf("%w: %s", ErrMaintenanceConflict, reason)
		}
	}
	descriptor := m.descriptorMap()[preview.response.DatabaseID]
	status, err := m.currentDatabaseStatus(ctx, preview.response.DatabaseID)
	if err != nil {
		return CleanupResult{}, err
	}
	if status.Status != "ready" || status.RebuildScheduled {
		return CleanupResult{}, ErrPreviewStale
	}
	m.maintenance.mu.Lock()
	if _, stillAvailable := m.maintenance.previews[strings.TrimSpace(request.PreviewID)]; !stillAvailable {
		m.maintenance.mu.Unlock()
		return CleanupResult{}, ErrPreviewNotFound
	}
	delete(m.maintenance.previews, strings.TrimSpace(request.PreviewID))
	m.maintenance.mu.Unlock()
	if hooks.Purge == nil {
		return CleanupResult{}, fmt.Errorf("database cleanup is unavailable")
	}
	before := inspectStorage(ctx, DatabaseStatus{Descriptor: descriptor, Status: "ready"}).TotalBytes
	deleted, err := hooks.Purge(ctx, preview.response.DatabaseID, preview.candidates)
	if err != nil {
		return CleanupResult{}, err
	}
	if deleted != len(preview.candidates) {
		return CleanupResult{}, ErrPreviewStale
	}
	result := CleanupResult{DatabaseID: preview.response.DatabaseID, DeletedCount: deleted, EstimatedBytes: preview.response.EstimatedBytes, BeforeBytes: before}
	if hooks.Compact != nil {
		if err := hooks.Compact(ctx, preview.response.DatabaseID); err != nil {
			result.Warning = "数据已删除，但文件未完全收缩：" + err.Error()
		} else {
			result.Compacted = true
		}
	}
	result.AfterBytes = inspectStorage(ctx, DatabaseStatus{Descriptor: descriptor, Status: "ready"}).TotalBytes
	if result.BeforeBytes > result.AfterBytes {
		result.ReclaimedBytes = result.BeforeBytes - result.AfterBytes
	}
	return result, nil
}

func (m *Manager) Compact(ctx context.Context, databaseID string, request CompactRequest) (CompactResult, error) {
	databaseID = strings.TrimSpace(databaseID)
	descriptor, ok := m.descriptorMap()[databaseID]
	if !ok {
		return CompactResult{}, fmt.Errorf("unknown database id %q", databaseID)
	}
	if request.Confirmation != "COMPACT "+databaseID {
		return CompactResult{}, fmt.Errorf("confirmation text does not match")
	}
	status, err := m.currentDatabaseStatus(ctx, databaseID)
	if err != nil {
		return CompactResult{}, err
	}
	if status.Status != "ready" || status.RebuildScheduled {
		return CompactResult{}, fmt.Errorf("database %s is not ready for compaction", databaseID)
	}
	lock := m.maintenance.locks[databaseID]
	if lock == nil || !lock.TryLock() {
		return CompactResult{}, ErrMaintenanceConflict
	}
	defer lock.Unlock()
	m.maintenance.mu.Lock()
	hooks := m.maintenance.hooks
	m.maintenance.mu.Unlock()
	if hooks.BusyReason != nil {
		if reason := strings.TrimSpace(hooks.BusyReason(databaseID)); reason != "" {
			return CompactResult{}, fmt.Errorf("%w: %s", ErrMaintenanceConflict, reason)
		}
	}
	if hooks.Compact == nil {
		return CompactResult{}, fmt.Errorf("database compaction is unavailable")
	}
	before := inspectStorage(ctx, DatabaseStatus{Descriptor: descriptor, Status: "ready"}).TotalBytes
	if err := hooks.Compact(ctx, databaseID); err != nil {
		return CompactResult{}, err
	}
	after := inspectStorage(ctx, DatabaseStatus{Descriptor: descriptor, Status: "ready"}).TotalBytes
	result := CompactResult{DatabaseID: databaseID, BeforeBytes: before, AfterBytes: after, Compacted: true}
	if before > after {
		result.ReclaimedBytes = before - after
	}
	return result, nil
}

// BackupConfirmationText returns the exact confirmation required for a backup.
func BackupConfirmationText(databaseID string) string {
	return "BACKUP " + strings.TrimSpace(databaseID)
}

// Backup creates a transactionally consistent SQLite snapshot with VACUUM
// INTO. It serializes against destructive maintenance but does not mutate the
// source database or require application downtime.
func (m *Manager) Backup(ctx context.Context, databaseID, confirmation string) (BackupResult, error) {
	databaseID = strings.TrimSpace(databaseID)
	descriptor, ok := m.descriptorMap()[databaseID]
	if !ok {
		return BackupResult{}, fmt.Errorf("unknown database id %q", databaseID)
	}
	if strings.TrimSpace(confirmation) != BackupConfirmationText(databaseID) {
		return BackupResult{}, fmt.Errorf("confirmation text does not match")
	}
	status, err := m.currentDatabaseStatus(ctx, databaseID)
	if err != nil {
		return BackupResult{}, err
	}
	if status.Status != "ready" && status.Status != "incompatible" {
		return BackupResult{}, fmt.Errorf("database %s is not available for backup", databaseID)
	}
	if !m.maintenance.backupLock.TryLock() {
		return BackupResult{}, ErrMaintenanceConflict
	}
	defer m.maintenance.backupLock.Unlock()
	lock := m.maintenance.locks[databaseID]
	if lock == nil || !lock.TryLock() {
		return BackupResult{}, ErrMaintenanceConflict
	}
	defer lock.Unlock()
	now := m.maintenance.now().UTC()
	m.maintenance.mu.Lock()
	lastBackup := m.maintenance.backupLast[databaseID]
	m.maintenance.mu.Unlock()
	if !lastBackup.IsZero() && now.Before(lastBackup.Add(backupMinimumInterval)) {
		retryAfter := lastBackup.Add(backupMinimumInterval).Sub(now).Round(time.Second)
		return BackupResult{}, fmt.Errorf("%w: retry after %s", ErrBackupRateLimited, retryAfter)
	}

	result, err := m.createBackupSnapshot(ctx, descriptor, status.Status, now)
	if err != nil {
		return BackupResult{}, err
	}
	m.maintenance.mu.Lock()
	m.maintenance.backupLast[databaseID] = now
	m.maintenance.mu.Unlock()
	return result, nil
}

func verifySQLiteBackup(ctx context.Context, path string) error {
	db, err := sqliteconn.OpenReadOnly(path)
	if err != nil {
		return err
	}
	defer func() { _ = db.Close() }()
	var result string
	if err := db.QueryRowContext(ctx, `SELECT quick_check FROM pragma_quick_check`).Scan(&result); err != nil {
		return err
	}
	if !strings.EqualFold(strings.TrimSpace(result), "ok") {
		return fmt.Errorf("SQLite quick_check returned %q", result)
	}
	return nil
}

func (m *Manager) pruneExpiredPreviewsLocked(now time.Time) {
	for id, preview := range m.maintenance.previews {
		if !now.Before(preview.expiresAt) {
			delete(m.maintenance.previews, id)
		}
	}
}

func (m *Manager) currentDatabaseStatus(ctx context.Context, databaseID string) (DatabaseStatus, error) {
	statuses, err := m.Statuses(ctx)
	if err != nil {
		return DatabaseStatus{}, err
	}
	for _, status := range statuses {
		if status.ID == databaseID {
			return status, nil
		}
	}
	return DatabaseStatus{}, fmt.Errorf("unknown database id %q", databaseID)
}

func newPreviewID() (string, error) {
	buffer := make([]byte, 16)
	if _, err := rand.Read(buffer); err != nil {
		return "", err
	}
	return hex.EncodeToString(buffer), nil
}
