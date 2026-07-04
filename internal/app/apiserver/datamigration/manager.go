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
	DatabaseBacktest       = "backtest"
	DatabaseBacktestRuns   = "backtest-runs"
	DatabaseStrategy       = "strategy"
	DatabaseExecution      = "execution-orders"
	DatabaseADK            = "adk"
	DatabaseADKSession     = "adk-session"
	RebuildMarkerFilename  = "database-rebuild.json"
	SchemaVersion          = 1
	ExecutionSchemaVersion = 2
	BatchConfirmationText  = "REBUILD INCOMPATIBLE DATABASES"
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
	DatabaseIDs []string `json:"databaseIds"`
	CreatedAt   string   `json:"createdAt"`
}

type Manager struct {
	settingsPath string
	descriptors  []Descriptor
	unavailable  map[string]error
	maintenance  maintenanceState
}

func NewManager(settingsPath string, backtestDBPath string) *Manager {
	manager := &Manager{
		settingsPath: strings.TrimSpace(settingsPath),
		unavailable:  make(map[string]error),
		descriptors: []Descriptor{
			{ID: DatabaseBacktest, Name: "行情回测数据", Path: strings.TrimSpace(backtestDBPath), Description: "历史 K 线、覆盖范围与行情同步数据。", Features: []string{"回测行情", "K 线同步"}, Version: SchemaVersion},
			{ID: DatabaseBacktestRuns, Name: "回测运行历史", Path: apiruntime.DeriveBacktestRunDBPath(settingsPath), Description: "回测请求、状态和结果。", Features: []string{"回测历史", "研究回测结果"}, Version: SchemaVersion},
			{ID: DatabaseStrategy, Name: "策略数据", Path: apiruntime.DeriveStrategyRuntimeDBPath(settingsPath), Description: "策略定义、插件目录、运行日志、审计和观察状态。", Features: []string{"策略定义", "策略插件", "策略运行"}, Version: SchemaVersion},
			{ID: DatabaseExecution, Name: "执行订单", Path: apiruntime.DeriveExecutionOrderDBPath(settingsPath), Description: "执行订单、状态事件、成交去重和序列。", Features: []string{"订单执行", "成交同步"}, Version: ExecutionSchemaVersion},
			{ID: DatabaseADK, Name: "ADK 数据", Path: apiruntime.DeriveADKDBPath(settingsPath), Description: "模型、智能体、技能、会话运行、任务、审批和记忆。", Features: []string{"智能体配置", "ADK 工作流"}, Version: SchemaVersion},
			{ID: DatabaseADKSession, Name: "ADK 会话", Path: apiruntime.DeriveADKSessionDBPath(settingsPath), Description: "GO-ADK 原始会话事件和状态。", Features: []string{"对话上下文", "工具事件"}, Version: SchemaVersion},
		},
	}
	manager.initializeMaintenance()
	return manager
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
	statuses, err := m.Statuses(ctx)
	if err != nil {
		return RebuildResult{}, err
	}
	statusByID := make(map[string]DatabaseStatus, len(statuses))
	for _, status := range statuses {
		statusByID[status.ID] = status
	}
	ids := normalizeIDs(request.DatabaseIDs)
	switch strings.TrimSpace(request.Mode) {
	case "incompatible":
		if strings.TrimSpace(request.Confirmation) != BatchConfirmationText {
			return RebuildResult{}, fmt.Errorf("confirmation text does not match")
		}
		ids = ids[:0]
		for _, status := range statuses {
			if status.Status != "ready" {
				ids = append(ids, status.ID)
			}
		}
	default:
		if len(ids) != 1 {
			return RebuildResult{}, fmt.Errorf("exactly one database id is required")
		}
		status, ok := statusByID[ids[0]]
		if !ok {
			return RebuildResult{}, fmt.Errorf("unknown database id %q", ids[0])
		}
		if strings.TrimSpace(request.Confirmation) != status.ConfirmationText {
			return RebuildResult{}, fmt.Errorf("confirmation text does not match")
		}
	}
	if len(ids) == 0 {
		return RebuildResult{}, fmt.Errorf("no databases require rebuild")
	}
	for _, id := range ids {
		if _, ok := statusByID[id]; !ok {
			return RebuildResult{}, fmt.Errorf("unknown database id %q", id)
		}
	}
	existing, err := m.readMarker()
	if err != nil {
		return RebuildResult{}, err
	}
	ids = normalizeIDs(append(existing.DatabaseIDs, ids...))
	if err := m.writeMarker(marker{DatabaseIDs: ids, CreatedAt: time.Now().UTC().Format(time.RFC3339Nano)}); err != nil {
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
	for _, id := range pending.DatabaseIDs {
		descriptor, ok := byID[id]
		if !ok {
			return fmt.Errorf("rebuild marker contains unknown database id %q", id)
		}
		for _, suffix := range []string{"", "-wal", "-shm"} {
			if err := os.Remove(descriptor.Path + suffix); err != nil && !errors.Is(err, os.ErrNotExist) {
				return fmt.Errorf("remove %s database file %s: %w", id, descriptor.Path+suffix, err)
			}
		}
	}
	return nil
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
	err = db.QueryRowContext(ctx,
		`SELECT version FROM `+sqliteschema.MetadataTable+` WHERE component_id = ? LIMIT 1`,
		descriptor.ID,
	).Scan(&version)
	if err != nil {
		status.Status = "incompatible"
		status.Error = "schema metadata is missing or unreadable"
		return status
	}
	status.CurrentVersion = &version
	if version != descriptor.Version {
		status.Status = "incompatible"
		status.Error = fmt.Sprintf("schema version %d does not match required version %d", version, descriptor.Version)
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
	if err := os.WriteFile(temp, raw, 0o600); err != nil {
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
