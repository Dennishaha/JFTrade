package servercore

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/jftrade/jftrade-main/internal/store/sqliteconn"
	stratsrv "github.com/jftrade/jftrade-main/internal/strategy"
	"github.com/jftrade/jftrade-main/pkg/besteffort"
	"github.com/jftrade/jftrade-main/pkg/strategy/pineworker"
)

const (
	defaultStrategyDesignFilename        = "strategy-definitions.json"
	strategyDesignDefinitionTable        = "strategy_design_definitions"
	strategyDesignDefinitionVersionTable = "strategy_definition_versions"
	strategyRuntimePinePlan              = pineworker.RuntimeID
	defaultStrategyVersion               = "0.1.0"
)

var errUnsupportedLegacyStrategyDefinition = errors.New("unsupported legacy strategy definition")

type strategyDesignStore struct {
	path   string
	dbPath string
	db     *sqliteconn.DB
	mu     sync.RWMutex
}

type strategyDesignDefinitionRow struct {
	ID              string         `db:"id"`
	Name            string         `db:"name"`
	Version         string         `db:"version"`
	Description     string         `db:"description"`
	Runtime         string         `db:"runtime"`
	SourceFormat    string         `db:"source_format"`
	Symbol          string         `db:"symbol"`
	Interval        string         `db:"interval"`
	Script          string         `db:"script"`
	VisualModelJSON string         `db:"visual_model_json"`
	CreatedAt       string         `db:"created_at"`
	UpdatedAt       string         `db:"updated_at"`
	DeletedAt       sql.NullString `db:"deleted_at"`
}

type strategyDesignDefinitionVersionRow struct {
	DefinitionID    string `db:"definition_id"`
	Version         string `db:"version"`
	Name            string `db:"name"`
	Description     string `db:"description"`
	Runtime         string `db:"runtime"`
	SourceFormat    string `db:"source_format"`
	Symbol          string `db:"symbol"`
	Interval        string `db:"interval"`
	Script          string `db:"script"`
	VisualModelJSON string `db:"visual_model_json"`
	CreatedAt       string `db:"created_at"`
	UpdatedAt       string `db:"updated_at"`
	SavedAt         string `db:"saved_at"`
	IsCurrent       int    `db:"is_current"`
}

func NewStrategyDesignStore(path string) (*strategyDesignStore, error) {
	store := &strategyDesignStore{path: path, dbPath: deriveStrategyDesignDBPath(path)}
	if err := store.openDB(); err != nil {
		return nil, err
	}
	if err := store.load(); err != nil {
		jftradeErr1 := store.Close()
		besteffort.LogError(jftradeErr1)
		return nil, err
	}
	return store, nil
}

func (s *strategyDesignStore) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

func (s *strategyDesignStore) load() error {
	return nil
}

func (s *strategyDesignStore) listDefinitions() []stratsrv.Definition {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.listDefinitionsFromDBLocked()
}

func (s *strategyDesignStore) listDefinitionsFromDBLocked() []stratsrv.Definition {
	rows := []strategyDesignDefinitionRow{}
	if err := s.db.Select(&rows,
		`SELECT id, name, version, description, runtime, source_format, symbol, interval, script, visual_model_json, created_at, updated_at, deleted_at `+
			`FROM `+strategyDesignDefinitionTable+` `+
			`WHERE deleted_at IS NULL OR TRIM(deleted_at) = '' `+
			`ORDER BY updated_at DESC, id ASC`); err != nil {
		return []stratsrv.Definition{}
	}
	items := make([]stratsrv.Definition, 0, len(rows))
	for _, row := range rows {
		definition, err := strategyDesignDefinitionFromRow(row)
		if err != nil {
			continue
		}
		items = append(items, definition)
	}
	return items
}

func (s *strategyDesignStore) definition(id string) (stratsrv.Definition, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	id = strings.TrimSpace(id)
	row, ok, err := s.definitionRowLocked(id, false)
	if err != nil || !ok {
		return stratsrv.Definition{}, false, err
	}
	definition, defErr := strategyDesignDefinitionFromRow(row)
	if defErr != nil {
		return stratsrv.Definition{}, false, defErr
	}
	return definition, true, nil
}

func (s *strategyDesignStore) listDefinitionVersions(definitionID string) ([]stratsrv.DefinitionVersionSummary, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	definitionID = strings.TrimSpace(definitionID)
	if definitionID == "" {
		return []stratsrv.DefinitionVersionSummary{}, false, nil
	}
	rows := []strategyDesignDefinitionVersionRow{}
	if err := s.db.Select(&rows,
		`SELECT v.definition_id, v.version, v.name, v.description, v.runtime, v.source_format, v.symbol, v.interval, v.script, v.visual_model_json, v.created_at, v.updated_at, v.saved_at, `+
			`CASE WHEN d.id IS NOT NULL AND (d.deleted_at IS NULL OR TRIM(d.deleted_at) = '') AND d.version = v.version THEN 1 ELSE 0 END AS is_current `+
			`FROM `+strategyDesignDefinitionVersionTable+` v `+
			`LEFT JOIN `+strategyDesignDefinitionTable+` d ON d.id = v.definition_id `+
			`WHERE v.definition_id = ? `+
			`ORDER BY v.saved_at DESC, v.version DESC`,
		definitionID,
	); err != nil {
		return nil, false, err
	}
	items := make([]stratsrv.DefinitionVersionSummary, 0, len(rows))
	for _, row := range rows {
		items = append(items, stratsrv.DefinitionVersionSummary{
			DefinitionID: row.DefinitionID,
			Version:      row.Version,
			Name:         row.Name,
			SavedAt:      row.SavedAt,
			IsCurrent:    row.IsCurrent != 0,
		})
	}
	return items, len(items) > 0, nil
}

func (s *strategyDesignStore) definitionVersion(definitionID string, version string) (stratsrv.DefinitionVersion, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	definitionID = strings.TrimSpace(definitionID)
	version = strings.TrimSpace(version)
	if definitionID == "" || version == "" {
		return stratsrv.DefinitionVersion{}, false, nil
	}
	var row strategyDesignDefinitionVersionRow
	err := s.db.Get(&row,
		`SELECT v.definition_id, v.version, v.name, v.description, v.runtime, v.source_format, v.symbol, v.interval, v.script, v.visual_model_json, v.created_at, v.updated_at, v.saved_at, `+
			`CASE WHEN d.id IS NOT NULL AND (d.deleted_at IS NULL OR TRIM(d.deleted_at) = '') AND d.version = v.version THEN 1 ELSE 0 END AS is_current `+
			`FROM `+strategyDesignDefinitionVersionTable+` v `+
			`LEFT JOIN `+strategyDesignDefinitionTable+` d ON d.id = v.definition_id `+
			`WHERE v.definition_id = ? AND v.version = ?`,
		definitionID,
		version,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return stratsrv.DefinitionVersion{}, false, nil
	}
	if err != nil {
		return stratsrv.DefinitionVersion{}, false, err
	}
	definition, err := strategyDesignDefinitionFromRow(strategyDesignDefinitionRow{
		ID:              row.DefinitionID,
		Name:            row.Name,
		Version:         row.Version,
		Description:     row.Description,
		Runtime:         row.Runtime,
		SourceFormat:    row.SourceFormat,
		Symbol:          row.Symbol,
		Interval:        row.Interval,
		Script:          row.Script,
		VisualModelJSON: row.VisualModelJSON,
		CreatedAt:       row.CreatedAt,
		UpdatedAt:       row.UpdatedAt,
	})
	if err != nil {
		return stratsrv.DefinitionVersion{}, false, err
	}
	return stratsrv.DefinitionVersion{
		Definition:   definition,
		DefinitionID: row.DefinitionID,
		SavedAt:      row.SavedAt,
		IsCurrent:    row.IsCurrent != 0,
	}, true, nil
}

func (s *strategyDesignStore) saveDefinition(input stratsrv.Definition) (stratsrv.Definition, error) {
	normalized, err := normalizeStrategyDesignDefinition(input)
	if err != nil {
		return stratsrv.Definition{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	return s.saveDefinitionToDBLocked(normalized)
}

func (s *strategyDesignStore) saveDefinitionToDBLocked(normalized stratsrv.Definition) (stratsrv.Definition, error) {
	ctx := context.Background()
	tx, err := s.db.BeginWrite(ctx, nil)
	if err != nil {
		return stratsrv.Definition{}, err
	}
	defer func() { _ = tx.Rollback() }()

	row, found, err := strategyDesignDefinitionRowFromQuerier(tx, normalized.ID, true)
	if err != nil {
		return stratsrv.Definition{}, err
	}
	if found {
		existing, err := strategyDesignDefinitionFromRow(row)
		if err != nil {
			return stratsrv.Definition{}, err
		}
		normalized.CreatedAt = existing.CreatedAt
		normalized.Version = existing.Version
		normalized.Script = syncStrategyScriptVersion(normalized.Script, normalized.Version)
		deleted := row.DeletedAt.Valid && strings.TrimSpace(row.DeletedAt.String) != ""
		changed := strategyDesignDefinitionMeaningfullyChanged(existing, normalized)
		if !changed && !deleted {
			return existing, nil
		}
		if changed {
			normalized.Version = nextStrategyDefinitionVersion(existing.Version)
			normalized.Script = syncStrategyScriptVersion(normalized.Script, normalized.Version)
		}
		now := time.Now().UTC().Format(time.RFC3339Nano)
		normalized.UpdatedAt = now
		if err := upsertStrategyDesignDefinition(ctx, tx, normalized, nil); err != nil {
			return stratsrv.Definition{}, err
		}
		if changed {
			if err := insertStrategyDesignDefinitionVersion(ctx, tx, normalized, now); err != nil {
				return stratsrv.Definition{}, err
			}
		}
		if err := tx.Commit(); err != nil {
			return stratsrv.Definition{}, err
		}
		return normalized, nil
	}

	normalized.Version = defaultStrategyVersion
	normalized.Script = syncStrategyScriptVersion(normalized.Script, normalized.Version)
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if err := upsertStrategyDesignDefinition(ctx, tx, normalized, nil); err != nil {
		return stratsrv.Definition{}, err
	}
	if err := insertStrategyDesignDefinitionVersion(ctx, tx, normalized, now); err != nil {
		return stratsrv.Definition{}, err
	}
	if err := tx.Commit(); err != nil {
		return stratsrv.Definition{}, err
	}
	return normalized, nil
}

func (s *strategyDesignStore) deleteDefinition(id string) (stratsrv.Definition, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	id = strings.TrimSpace(id)
	row, ok, err := s.definitionRowLocked(id, false)
	if err != nil {
		return stratsrv.Definition{}, err
	}
	if !ok {
		return stratsrv.Definition{}, os.ErrNotExist
	}
	definition, err := strategyDesignDefinitionFromRow(row)
	if err != nil {
		return stratsrv.Definition{}, err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := s.db.ExecContext(context.Background(),
		`UPDATE `+strategyDesignDefinitionTable+` SET updated_at = ?, deleted_at = ? WHERE id = ?`,
		now,
		now,
		id,
	); err != nil {
		return stratsrv.Definition{}, err
	}
	return definition, nil
}

func (s *strategyDesignStore) definitionRowLocked(id string, includeDeleted bool) (strategyDesignDefinitionRow, bool, error) {
	return strategyDesignDefinitionRowFromQuerier(s.db, id, includeDeleted)
}

type strategyDesignDefinitionRowQuerier interface {
	Get(dest any, query string, args ...any) error
}

func strategyDesignDefinitionRowFromQuerier(querier strategyDesignDefinitionRowQuerier, id string, includeDeleted bool) (strategyDesignDefinitionRow, bool, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return strategyDesignDefinitionRow{}, false, nil
	}
	query := `SELECT id, name, version, description, runtime, source_format, symbol, interval, script, visual_model_json, created_at, updated_at, deleted_at FROM ` + strategyDesignDefinitionTable + ` WHERE id = ?`
	if !includeDeleted {
		query += ` AND (deleted_at IS NULL OR TRIM(deleted_at) = '')`
	}
	var row strategyDesignDefinitionRow
	if err := querier.Get(&row, query, id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return strategyDesignDefinitionRow{}, false, nil
		}
		return strategyDesignDefinitionRow{}, false, err
	}
	return row, true, nil
}

func (s *strategyDesignStore) upsertDefinitionLocked(definition stratsrv.Definition, deletedAt *string) error {
	return upsertStrategyDesignDefinition(context.Background(), s.db, definition, deletedAt)
}

type strategyDesignDefinitionExecer interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}

func upsertStrategyDesignDefinition(ctx context.Context, executor strategyDesignDefinitionExecer, definition stratsrv.Definition, deletedAt *string) error {
	row, err := strategyDesignDefinitionRowFromDefinition(definition)
	if err != nil {
		return err
	}
	var deletedValue any
	if deletedAt != nil {
		deletedValue = strings.TrimSpace(*deletedAt)
	}
	_, err = executor.ExecContext(ctx,
		`INSERT INTO `+strategyDesignDefinitionTable+` (`+
			`id, name, version, description, runtime, source_format, symbol, interval, script, visual_model_json, created_at, updated_at, deleted_at`+
			`) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?) `+
			`ON CONFLICT(id) DO UPDATE SET `+
			`name = excluded.name, `+
			`version = excluded.version, `+
			`description = excluded.description, `+
			`runtime = excluded.runtime, `+
			`source_format = excluded.source_format, `+
			`symbol = excluded.symbol, `+
			`interval = excluded.interval, `+
			`script = excluded.script, `+
			`visual_model_json = excluded.visual_model_json, `+
			`created_at = excluded.created_at, `+
			`updated_at = excluded.updated_at, `+
			`deleted_at = excluded.deleted_at`,
		row.ID,
		row.Name,
		row.Version,
		row.Description,
		row.Runtime,
		row.SourceFormat,
		row.Symbol,
		row.Interval,
		row.Script,
		row.VisualModelJSON,
		row.CreatedAt,
		row.UpdatedAt,
		deletedValue,
	)
	return err
}

func insertStrategyDesignDefinitionVersion(ctx context.Context, executor strategyDesignDefinitionExecer, definition stratsrv.Definition, savedAt string) error {
	row, err := strategyDesignDefinitionRowFromDefinition(definition)
	if err != nil {
		return err
	}
	_, err = executor.ExecContext(ctx,
		`INSERT INTO `+strategyDesignDefinitionVersionTable+` (`+
			`definition_id, version, name, description, runtime, source_format, symbol, interval, script, visual_model_json, created_at, updated_at, saved_at`+
			`) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		row.ID,
		row.Version,
		row.Name,
		row.Description,
		row.Runtime,
		row.SourceFormat,
		row.Symbol,
		row.Interval,
		row.Script,
		row.VisualModelJSON,
		row.CreatedAt,
		row.UpdatedAt,
		strings.TrimSpace(savedAt),
	)
	return err
}
