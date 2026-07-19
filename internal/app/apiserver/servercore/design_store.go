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
	defaultStrategyDesignFilename = "strategy-definitions.json"
	strategyDesignDefinitionTable = "strategy_design_definitions"
	strategyRuntimePinePlan       = pineworker.RuntimeID
	defaultStrategyVersion        = "0.1.0"
)

var errUnsupportedLegacyStrategyDefinition = errors.New("unsupported legacy strategy definition")

type strategyVisualNode = stratsrv.VisualNode
type strategyVisualEdge = stratsrv.VisualEdge
type strategyVisualModel = stratsrv.VisualModel
type strategyDesignDefinition = stratsrv.Definition

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

func (s *strategyDesignStore) listDefinitions() []strategyDesignDefinition {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.listDefinitionsFromDBLocked()
}

func (s *strategyDesignStore) listDefinitionsFromDBLocked() []strategyDesignDefinition {
	rows := []strategyDesignDefinitionRow{}
	if err := s.db.Select(&rows,
		`SELECT id, name, version, description, runtime, source_format, symbol, interval, script, visual_model_json, created_at, updated_at, deleted_at `+
			`FROM `+strategyDesignDefinitionTable+` `+
			`WHERE deleted_at IS NULL OR TRIM(deleted_at) = '' `+
			`ORDER BY updated_at DESC, id ASC`); err != nil {
		return []strategyDesignDefinition{}
	}
	items := make([]strategyDesignDefinition, 0, len(rows))
	for _, row := range rows {
		definition, err := strategyDesignDefinitionFromRow(row)
		if err != nil {
			continue
		}
		items = append(items, definition)
	}
	return items
}

func (s *strategyDesignStore) definition(id string) (strategyDesignDefinition, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	id = strings.TrimSpace(id)
	row, ok, err := s.definitionRowLocked(id, false)
	if err != nil || !ok {
		return strategyDesignDefinition{}, false, err
	}
	definition, defErr := strategyDesignDefinitionFromRow(row)
	if defErr != nil {
		return strategyDesignDefinition{}, false, defErr
	}
	return definition, true, nil
}

func (s *strategyDesignStore) saveDefinition(input strategyDesignDefinition) (strategyDesignDefinition, error) {
	normalized, err := normalizeStrategyDesignDefinition(input)
	if err != nil {
		return strategyDesignDefinition{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	return s.saveDefinitionToDBLocked(normalized)
}

func (s *strategyDesignStore) saveDefinitionToDBLocked(normalized strategyDesignDefinition) (strategyDesignDefinition, error) {
	row, found, err := s.definitionRowLocked(normalized.ID, true)
	if err != nil {
		return strategyDesignDefinition{}, err
	}
	if found {
		existing, err := strategyDesignDefinitionFromRow(row)
		if err != nil {
			return strategyDesignDefinition{}, err
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
		normalized.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
		if err := s.upsertDefinitionLocked(normalized, nil); err != nil {
			return strategyDesignDefinition{}, err
		}
		return normalized, nil
	}

	normalized.Version = defaultStrategyVersion
	normalized.Script = syncStrategyScriptVersion(normalized.Script, normalized.Version)
	if err := s.upsertDefinitionLocked(normalized, nil); err != nil {
		return strategyDesignDefinition{}, err
	}
	return normalized, nil
}

func (s *strategyDesignStore) deleteDefinition(id string) (strategyDesignDefinition, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	id = strings.TrimSpace(id)
	row, ok, err := s.definitionRowLocked(id, false)
	if err != nil {
		return strategyDesignDefinition{}, err
	}
	if !ok {
		return strategyDesignDefinition{}, os.ErrNotExist
	}
	definition, err := strategyDesignDefinitionFromRow(row)
	if err != nil {
		return strategyDesignDefinition{}, err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := s.db.ExecContext(context.Background(),
		`UPDATE `+strategyDesignDefinitionTable+` SET updated_at = ?, deleted_at = ? WHERE id = ?`,
		now,
		now,
		id,
	); err != nil {
		return strategyDesignDefinition{}, err
	}
	return definition, nil
}

func (s *strategyDesignStore) definitionRowLocked(id string, includeDeleted bool) (strategyDesignDefinitionRow, bool, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return strategyDesignDefinitionRow{}, false, nil
	}
	query := `SELECT id, name, version, description, runtime, source_format, symbol, interval, script, visual_model_json, created_at, updated_at, deleted_at FROM ` + strategyDesignDefinitionTable + ` WHERE id = ?`
	if !includeDeleted {
		query += ` AND (deleted_at IS NULL OR TRIM(deleted_at) = '')`
	}
	var row strategyDesignDefinitionRow
	if err := s.db.Get(&row, query, id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return strategyDesignDefinitionRow{}, false, nil
		}
		return strategyDesignDefinitionRow{}, false, err
	}
	return row, true, nil
}

func (s *strategyDesignStore) upsertDefinitionLocked(definition strategyDesignDefinition, deletedAt *string) error {
	row, err := strategyDesignDefinitionRowFromDefinition(definition)
	if err != nil {
		return err
	}
	var deletedValue any
	if deletedAt != nil {
		deletedValue = strings.TrimSpace(*deletedAt)
	}
	_, err = s.db.ExecContext(context.Background(),
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
