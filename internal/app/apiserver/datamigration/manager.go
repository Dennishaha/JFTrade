package datamigration

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	apiruntime "github.com/jftrade/jftrade-main/internal/app/apiserver/runtime"
	"github.com/jftrade/jftrade-main/internal/store/sqliteconn"
	"github.com/jftrade/jftrade-main/internal/store/sqliteschema"
)

const (
	DatabaseBacktest      = sqliteschema.DatabaseBacktest
	DatabaseBacktestRuns  = sqliteschema.DatabaseBacktestRuns
	DatabaseStrategy      = sqliteschema.DatabaseStrategy
	DatabaseExecution     = sqliteschema.DatabaseExecution
	DatabaseADK           = sqliteschema.DatabaseADK
	DatabaseADKSession    = sqliteschema.DatabaseADKSession
	DatabaseADKArtifact   = sqliteschema.DatabaseADKArtifact
	DatabaseWatchlist     = sqliteschema.DatabaseWatchlist
	DatabaseResearch      = sqliteschema.DatabaseResearch
	RebuildMarkerFilename = "database-rebuild.json"
	BatchConfirmationText = "REBUILD INCOMPATIBLE DATABASES"
)

type Descriptor struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Path        string   `json:"path"`
	Description string   `json:"description"`
	Features    []string `json:"features"`
	Version     int      `json:"expectedVersion"`
}

type DatabaseStatus struct {
	Descriptor
	Status           string `json:"status"`
	CurrentVersion   *int   `json:"currentVersion"`
	Error            string `json:"error,omitempty"`
	RebuildScheduled bool   `json:"rebuildScheduled"`
	RestartRequired  bool   `json:"restartRequired"`
	ConfirmationText string `json:"confirmationText"`
}

type RebuildRequest struct {
	DatabaseIDs  []string `json:"databaseIds"`
	Mode         string   `json:"mode"`
	Confirmation string   `json:"confirmation"`
}

type RebuildResult struct {
	DatabaseIDs     []string `json:"databaseIds"`
	RestartRequired bool     `json:"restartRequired"`
	Scheduled       bool     `json:"scheduled"`
}

type marker struct {
	DatabaseIDs []string       `json:"databaseIds"`
	Backups     []markerBackup `json:"backups"`
	CreatedAt   string         `json:"createdAt"`
}

type markerBackup struct {
	DatabaseID string `json:"databaseId"`
	Path       string `json:"path"`
	SizeBytes  int64  `json:"sizeBytes"`
	SHA256     string `json:"sha256"`
}

type markerTemporaryFile interface {
	Write([]byte) (int, error)
	Close() error
}

type Manager struct {
	settingsPath   string
	descriptors    []Descriptor
	unavailable    map[string]error
	maintenance    maintenanceState
	openMarkerTemp func(string) (markerTemporaryFile, error)
}

func NewManager(settingsPath string, backtestDBPath string) *Manager {
	manager := &Manager{
		settingsPath: strings.TrimSpace(settingsPath),
		unavailable:  make(map[string]error),
		openMarkerTemp: func(path string) (markerTemporaryFile, error) {
			return os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		},
		descriptors: []Descriptor{
			currentDescriptor(DatabaseBacktest, "行情回测数据", strings.TrimSpace(backtestDBPath), "历史 K 线、覆盖范围与行情同步数据。", "回测行情", "K 线同步"),
			currentDescriptor(DatabaseBacktestRuns, "回测运行历史", apiruntime.DeriveBacktestRunDBPath(settingsPath), "回测请求、状态和结果。", "回测历史", "研究回测结果"),
			currentDescriptor(DatabaseStrategy, "策略数据", apiruntime.DeriveStrategyRuntimeDBPath(settingsPath), "策略定义、插件目录、运行日志、审计和观察状态。", "策略定义", "策略插件", "策略运行"),
			currentDescriptor(DatabaseExecution, "执行订单", apiruntime.DeriveExecutionOrderDBPath(settingsPath), "执行订单、状态事件、成交去重和序列。", "订单执行", "成交同步"),
			currentDescriptor(DatabaseADK, "ADK 数据", apiruntime.DeriveADKDBPath(settingsPath), "模型、智能体、技能、会话运行、任务、审批和记忆。", "智能体配置", "ADK 工作流"),
			currentDescriptor(DatabaseADKSession, "ADK 会话", apiruntime.DeriveADKSessionDBPath(settingsPath), "ADK 原始会话事件和状态。", "对话上下文", "工具事件"),
			currentDescriptor(DatabaseADKArtifact, "ADK 工件", apiruntime.DeriveADKArtifactDBPath(settingsPath), "ADK 工具输出和版本化工件。", "工具工件", "上下文卸载"),
			currentDescriptor(DatabaseWatchlist, "自选股", apiruntime.DeriveWatchlistDBPath(settingsPath), "本地自选分组、成员、券商导入绑定、快照与审计记录。", "自选分组", "券商导入", "来源对账"),
			currentDescriptor(DatabaseResearch, "研究数据", apiruntime.DeriveResearchDBPath(settingsPath), "研究中心股票筛选预设与后续研究持久化数据。", "股票筛选预设"),
		},
	}
	manager.initializeMaintenance()
	return manager
}

func currentDescriptor(id, name, path, description string, features ...string) Descriptor {
	definition := sqliteschema.MustDefinition(id)
	return Descriptor{
		ID:          definition.ID,
		Name:        name,
		Path:        strings.TrimSpace(path),
		Description: description,
		Features:    append([]string(nil), features...),
		Version:     definition.Version,
	}
}

func (m *Manager) SetUnavailable(id string, err error) {
	if m == nil || err == nil {
		return
	}
	if _, ok := m.descriptorMap()[id]; ok {
		m.unavailable[id] = err
	}
}

func (m *Manager) Statuses(ctx context.Context) ([]DatabaseStatus, error) {
	scheduled, err := m.readMarker()
	if err != nil {
		return nil, err
	}
	scheduledSet := make(map[string]struct{}, len(scheduled.DatabaseIDs))
	for _, id := range scheduled.DatabaseIDs {
		scheduledSet[id] = struct{}{}
	}
	statuses := make([]DatabaseStatus, 0, len(m.descriptors))
	for _, descriptor := range m.descriptors {
		status := inspectDatabase(ctx, descriptor)
		if unavailableErr := m.unavailable[descriptor.ID]; unavailableErr != nil {
			status.Status = "unavailable"
			if sqliteschema.IsIncompatible(unavailableErr) {
				status.Status = "incompatible"
			}
			status.Error = unavailableErr.Error()
		}
		_, status.RebuildScheduled = scheduledSet[descriptor.ID]
		status.RestartRequired = status.RebuildScheduled
		status.ConfirmationText = "REBUILD " + descriptor.ID
		statuses = append(statuses, status)
	}
	return statuses, nil
}

func (m *Manager) ScheduleRebuild(ctx context.Context, request RebuildRequest) (RebuildResult, error) {
	if !m.maintenance.backupLock.TryLock() {
		return RebuildResult{}, ErrMaintenanceConflict
	}
	defer m.maintenance.backupLock.Unlock()

	statuses, err := m.Statuses(ctx)
	if err != nil {
		return RebuildResult{}, err
	}
	statusByID := make(map[string]DatabaseStatus, len(statuses))
	for _, status := range statuses {
		statusByID[status.ID] = status
	}
	ids, err := selectRebuildIDs(statuses, statusByID, request)
	if err != nil {
		return RebuildResult{}, err
	}
	unlock, err := m.tryLockDatabases(ids)
	if err != nil {
		return RebuildResult{}, err
	}
	defer unlock()
	return m.scheduleRebuildLocked(ctx, ids, statusByID)
}

func selectRebuildIDs(statuses []DatabaseStatus, statusByID map[string]DatabaseStatus, request RebuildRequest) ([]string, error) {
	ids := normalizeIDs(request.DatabaseIDs)
	switch strings.TrimSpace(request.Mode) {
	case "incompatible":
		if strings.TrimSpace(request.Confirmation) != BatchConfirmationText {
			return nil, fmt.Errorf("confirmation text does not match")
		}
		ids = ids[:0]
		for _, status := range statuses {
			if status.Status == "incompatible" {
				ids = append(ids, status.ID)
			}
		}
	default:
		if len(ids) != 1 {
			return nil, fmt.Errorf("exactly one database id is required")
		}
		status, ok := statusByID[ids[0]]
		if !ok {
			return nil, fmt.Errorf("unknown database id %q", ids[0])
		}
		if strings.TrimSpace(request.Confirmation) != status.ConfirmationText {
			return nil, fmt.Errorf("confirmation text does not match")
		}
	}
	if len(ids) == 0 {
		return nil, fmt.Errorf("no databases require rebuild")
	}
	for _, id := range ids {
		status, ok := statusByID[id]
		if !ok {
			return nil, fmt.Errorf("unknown database id %q", id)
		}
		if status.Status != "ready" && status.Status != "incompatible" {
			return nil, fmt.Errorf("database %s is not available for a verified rebuild backup", id)
		}
	}
	return ids, nil
}

func (m *Manager) tryLockDatabases(ids []string) (func(), error) {
	locked := make([]string, 0, len(ids))
	for _, id := range ids {
		lock := m.maintenance.locks[id]
		if lock == nil || !lock.TryLock() {
			for index := len(locked) - 1; index >= 0; index-- {
				m.maintenance.locks[locked[index]].Unlock()
			}
			return nil, ErrMaintenanceConflict
		}
		locked = append(locked, id)
	}
	return func() {
		for index := len(locked) - 1; index >= 0; index-- {
			m.maintenance.locks[locked[index]].Unlock()
		}
	}, nil
}

func (m *Manager) scheduleRebuildLocked(
	ctx context.Context,
	ids []string,
	statusByID map[string]DatabaseStatus,
) (RebuildResult, error) {
	existing, err := m.readMarker()
	if err != nil {
		return RebuildResult{}, err
	}
	existingByID := make(map[string]markerBackup, len(existing.Backups))
	for _, backup := range existing.Backups {
		existingByID[backup.DatabaseID] = backup
	}
	created := make([]markerBackup, 0, len(ids))
	createdPaths := make([]string, 0, len(ids))
	for _, id := range ids {
		if _, alreadyScheduled := existingByID[id]; alreadyScheduled {
			continue
		}
		result, backupErr := m.createBackupSnapshot(
			ctx, statusByID[id].Descriptor, statusByID[id].Status, time.Now().UTC(), createdPaths...,
		)
		if backupErr != nil {
			removeMarkerBackups(created)
			return RebuildResult{}, fmt.Errorf("create verified rebuild backup for %s: %w", id, backupErr)
		}
		backup := markerBackup{DatabaseID: id, Path: result.BackupPath, SizeBytes: result.SizeBytes, SHA256: result.SHA256}
		created = append(created, backup)
		createdPaths = append(createdPaths, backup.Path)
		existingByID[id] = backup
	}
	ids = normalizeIDs(append(existing.DatabaseIDs, ids...))
	backups := make([]markerBackup, 0, len(ids))
	for _, id := range ids {
		backup, ok := existingByID[id]
		if !ok {
			removeMarkerBackups(created)
			return RebuildResult{}, fmt.Errorf("rebuild marker for %s has no verified backup", id)
		}
		backups = append(backups, backup)
	}
	if err := m.writeMarker(marker{DatabaseIDs: ids, Backups: backups, CreatedAt: time.Now().UTC().Format(time.RFC3339Nano)}); err != nil {
		removeMarkerBackups(created)
		return RebuildResult{}, err
	}
	return RebuildResult{DatabaseIDs: ids, RestartRequired: true, Scheduled: true}, nil
}

func (m *Manager) ApplyPending() error {
	pending, err := m.readMarker()
	if err != nil || len(pending.DatabaseIDs) == 0 {
		return err
	}
	byID := m.descriptorMap()
	backups := make(map[string]markerBackup, len(pending.Backups))
	scheduled := make(map[string]struct{}, len(pending.DatabaseIDs))
	for _, id := range pending.DatabaseIDs {
		scheduled[id] = struct{}{}
	}
	for _, backup := range pending.Backups {
		if _, duplicate := backups[backup.DatabaseID]; duplicate {
			return fmt.Errorf("rebuild marker contains duplicate backup for %q", backup.DatabaseID)
		}
		if _, ok := scheduled[backup.DatabaseID]; !ok {
			return fmt.Errorf("rebuild marker contains backup for unscheduled database %q", backup.DatabaseID)
		}
		backups[backup.DatabaseID] = backup
	}
	// Verify every snapshot before deleting any source file. A corrupt or missing
	// snapshot must leave the complete set of original databases untouched.
	for _, id := range pending.DatabaseIDs {
		descriptor, ok := byID[id]
		if !ok {
			return fmt.Errorf("rebuild marker contains unknown database id %q", id)
		}
		backup, ok := backups[id]
		if !ok {
			return fmt.Errorf("rebuild marker for %s has no verified backup", id)
		}
		if err := m.verifyMarkerBackup(context.Background(), descriptor, backup); err != nil {
			return fmt.Errorf("verify rebuild backup for %s: %w", id, err)
		}
	}
	for _, id := range pending.DatabaseIDs {
		descriptor := byID[id]
		for _, suffix := range []string{"", "-wal", "-shm"} {
			if err := os.Remove(descriptor.Path + suffix); err != nil && !errors.Is(err, os.ErrNotExist) {
				return fmt.Errorf("remove %s database file %s: %w", id, descriptor.Path+suffix, err)
			}
		}
	}
	return nil
}

func removeMarkerBackups(backups []markerBackup) {
	for _, backup := range backups {
		_ = os.Remove(backup.Path)
	}
}

func (m *Manager) verifyMarkerBackup(ctx context.Context, descriptor Descriptor, backup markerBackup) error {
	if backup.DatabaseID != descriptor.ID {
		return fmt.Errorf("backup database id %q does not match %q", backup.DatabaseID, descriptor.ID)
	}
	backupDir := filepath.Join(filepath.Dir(m.settingsPath), "backups")
	relative, err := filepath.Rel(backupDir, backup.Path)
	if err != nil || relative == "." || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) || filepath.IsAbs(relative) {
		return fmt.Errorf("backup path is outside the managed backup directory")
	}
	parsedID, _, ok := m.parseManagedBackupFilename(filepath.Base(backup.Path))
	if !ok || parsedID != descriptor.ID {
		return fmt.Errorf("backup filename is not managed for %s", descriptor.ID)
	}
	info, err := os.Lstat(backup.Path)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || info.Size() != backup.SizeBytes || backup.SizeBytes <= 0 {
		return fmt.Errorf("backup size or file type does not match marker")
	}
	digest, err := fileSHA256(backup.Path)
	if err != nil {
		return err
	}
	if !strings.EqualFold(digest, strings.TrimSpace(backup.SHA256)) {
		return fmt.Errorf("backup SHA-256 does not match marker")
	}
	return verifySQLiteBackup(ctx, backup.Path)
}

func (m *Manager) CompletePending(ctx context.Context) error {
	pending, err := m.readMarker()
	if err != nil || len(pending.DatabaseIDs) == 0 {
		return err
	}
	statuses, err := m.Statuses(ctx)
	if err != nil {
		return err
	}
	statusByID := make(map[string]DatabaseStatus, len(statuses))
	for _, status := range statuses {
		statusByID[status.ID] = status
	}
	for _, id := range pending.DatabaseIDs {
		if statusByID[id].Status != "ready" {
			return fmt.Errorf("rebuilt database %s did not initialize successfully: %s", id, statusByID[id].Error)
		}
	}
	return os.Remove(m.markerPath())
}

func inspectDatabase(ctx context.Context, descriptor Descriptor) (status DatabaseStatus) {
	status = DatabaseStatus{Descriptor: descriptor, Status: "missing"}
	info, err := os.Stat(descriptor.Path)
	if errors.Is(err, os.ErrNotExist) {
		return status
	}
	if err != nil {
		status.Status = "unavailable"
		status.Error = err.Error()
		return status
	}
	if !info.Mode().IsRegular() {
		status.Status = "unavailable"
		status.Error = "database path is not a regular file"
		return status
	}
	db, err := sqliteconn.OpenReadOnly(descriptor.Path)
	if err != nil {
		status.Status = "unavailable"
		status.Error = err.Error()
		return status
	}
	defer func() {
		if closeErr := db.Close(); closeErr != nil && status.Status == "ready" {
			status.Status = "unavailable"
			status.CurrentVersion = nil
			status.Error = closeErr.Error()
		}
	}()
	var version int
	if err = db.QueryRowContext(ctx,
		`SELECT version FROM `+sqliteschema.MetadataTable+` WHERE component_id = ? LIMIT 1`,
		descriptor.ID,
	).Scan(&version); err == nil {
		status.CurrentVersion = &version
	}
	if err := sqliteschema.ValidateCurrent(ctx, db, descriptor.Path, descriptor.ID); err != nil {
		status.Status = "incompatible"
		if !sqliteschema.IsIncompatible(err) {
			status.Status = "unavailable"
		}
		status.Error = err.Error()
		return status
	}
	status.Status = "ready"
	return status
}

func (m *Manager) descriptorMap() map[string]Descriptor {
	result := make(map[string]Descriptor, len(m.descriptors))
	for _, descriptor := range m.descriptors {
		result[descriptor.ID] = descriptor
	}
	return result
}

func (m *Manager) markerPath() string {
	return filepath.Join(filepath.Dir(m.settingsPath), RebuildMarkerFilename)
}

func (m *Manager) readMarker() (marker, error) {
	raw, err := os.ReadFile(m.markerPath())
	if errors.Is(err, os.ErrNotExist) {
		return marker{}, nil
	}
	if err != nil {
		return marker{}, err
	}
	var value marker
	if err := json.Unmarshal(raw, &value); err != nil {
		return marker{}, fmt.Errorf("decode database rebuild marker: %w", err)
	}
	value.DatabaseIDs = normalizeIDs(value.DatabaseIDs)
	sort.Slice(value.Backups, func(i, j int) bool { return value.Backups[i].DatabaseID < value.Backups[j].DatabaseID })
	return value, nil
}

func (m *Manager) writeMarker(value marker) error {
	path := m.markerPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	temp := path + ".tmp"
	file, err := m.openMarkerTemp(temp)
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(temp) }()
	if _, err := file.Write(raw); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	return os.Rename(temp, path)
}

func normalizeIDs(values []string) []string {
	seen := map[string]struct{}{}
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}
