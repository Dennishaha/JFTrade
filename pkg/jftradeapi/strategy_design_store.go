package jftradeapi

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	strategydefinition "github.com/jftrade/jftrade-main/pkg/strategy/definition"
	"github.com/jmoiron/sqlx"
	"golang.org/x/mod/semver"
	_ "modernc.org/sqlite"
)

const (
	defaultStrategyDesignFilename = "strategy-definitions.json"
	strategyDesignDefinitionTable = "strategy_design_definitions"
	strategyRuntimePinePlan       = "pine-go-plan"
	defaultStrategyVersion        = "0.1.0"
)

var errUnsupportedLegacyStrategyDefinition = errors.New("unsupported legacy strategy definition")

type strategyVisualNode struct {
	ID         string         `json:"id"`
	Type       string         `json:"type"`
	X          float64        `json:"x"`
	Y          float64        `json:"y"`
	Text       string         `json:"text,omitempty"`
	Properties map[string]any `json:"properties,omitempty"`
}

type strategyVisualEdge struct {
	ID           string         `json:"id,omitempty"`
	Type         string         `json:"type,omitempty"`
	SourceNodeID string         `json:"sourceNodeId"`
	TargetNodeID string         `json:"targetNodeId"`
	Text         string         `json:"text,omitempty"`
	Properties   map[string]any `json:"properties,omitempty"`
}

type strategyVisualModel struct {
	Engine  string               `json:"engine,omitempty"`
	Version int                  `json:"version,omitempty"`
	Nodes   []strategyVisualNode `json:"nodes,omitempty"`
	Edges   []strategyVisualEdge `json:"edges,omitempty"`
}

type strategyDesignDefinition struct {
	ID           string               `json:"id"`
	Name         string               `json:"name"`
	Version      string               `json:"version"`
	Description  string               `json:"description"`
	Runtime      string               `json:"runtime"`
	SourceFormat string               `json:"sourceFormat"`
	Symbol       string               `json:"symbol,omitempty"`
	Interval     string               `json:"interval,omitempty"`
	Script       string               `json:"script"`
	VisualModel  *strategyVisualModel `json:"visualModel,omitempty"`
	CreatedAt    string               `json:"createdAt"`
	UpdatedAt    string               `json:"updatedAt"`
}

type strategyDesignStore struct {
	path   string
	dbPath string
	db     *sqlx.DB
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
		_ = store.Close()
		return nil, err
	}
	return store, nil
}

func deriveStrategyDesignPath(settingsPath string) string {
	directory := filepath.Dir(strings.TrimSpace(settingsPath))
	if directory == "" || directory == "." {
		return defaultStrategyDesignFilename
	}
	return filepath.Join(directory, defaultStrategyDesignFilename)
}

func deriveStrategyDesignDBPath(legacyPath string) string {
	if envPath := strings.TrimSpace(os.Getenv("JFTRADE_STRATEGY_RUNTIME_DB")); envPath != "" {
		return envPath
	}
	directory := filepath.Dir(strings.TrimSpace(legacyPath))
	if directory == "" || directory == "." {
		return defaultStrategyRuntimeDBFilename
	}
	return filepath.Join(directory, defaultStrategyRuntimeDBFilename)
}

func (s *strategyDesignStore) openDB() error {
	trimmedPath := strings.TrimSpace(s.dbPath)
	if trimmedPath == "" {
		return fmt.Errorf("strategy design db path is required")
	}
	directory := filepath.Dir(trimmedPath)
	if directory != "" && directory != "." {
		if err := os.MkdirAll(directory, 0o755); err != nil {
			return fmt.Errorf("create strategy design db directory: %w", err)
		}
	}
	db, err := sqlx.Open("sqlite", trimmedPath+"?_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)")
	if err != nil {
		return fmt.Errorf("open strategy design sqlite store: %w", err)
	}
	s.db = db
	return nil
}

func (s *strategyDesignStore) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

func (s *strategyDesignStore) load() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.migrateLocked()
}

func (s *strategyDesignStore) migrateLocked() error {
	for _, statement := range []string{
		strings.Join([]string{
			`CREATE TABLE IF NOT EXISTS ` + strategyDesignDefinitionTable + ` (`,
			`  id                TEXT PRIMARY KEY,`,
			`  name              TEXT NOT NULL DEFAULT '',`,
			`  version           TEXT NOT NULL DEFAULT '',`,
			`  description       TEXT NOT NULL DEFAULT '',`,
			`  runtime           TEXT NOT NULL DEFAULT '',`,
			`  source_format     TEXT NOT NULL DEFAULT '',`,
			`  symbol            TEXT NOT NULL DEFAULT '',`,
			`  interval          TEXT NOT NULL DEFAULT '',`,
			`  script            TEXT NOT NULL DEFAULT '',`,
			`  visual_model_json TEXT NOT NULL DEFAULT '',`,
			`  created_at        TEXT NOT NULL DEFAULT '',`,
			`  updated_at        TEXT NOT NULL DEFAULT '',`,
			`  deleted_at        TEXT`,
			`)`,
		}, " "),
		`CREATE INDEX IF NOT EXISTS idx_strategy_design_definitions_updated_at ON ` + strategyDesignDefinitionTable + ` (updated_at DESC, id ASC)`,
		`CREATE INDEX IF NOT EXISTS idx_strategy_design_definitions_deleted_at ON ` + strategyDesignDefinitionTable + ` (deleted_at)`,
	} {
		if _, err := s.db.Exec(statement); err != nil {
			return err
		}
	}
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
	if _, err := s.db.Exec(
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
	_, err = s.db.Exec(
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

func strategyDesignDefinitionFromRow(row strategyDesignDefinitionRow) (strategyDesignDefinition, error) {
	var visualModel *strategyVisualModel
	if strings.TrimSpace(row.VisualModelJSON) != "" {
		var parsed strategyVisualModel
		if err := json.Unmarshal([]byte(row.VisualModelJSON), &parsed); err != nil {
			return strategyDesignDefinition{}, err
		}
		visualModel = &parsed
	}
	return normalizeStrategyDesignDefinition(strategyDesignDefinition{
		ID:           row.ID,
		Name:         row.Name,
		Version:      row.Version,
		Description:  row.Description,
		Runtime:      row.Runtime,
		SourceFormat: row.SourceFormat,
		Symbol:       row.Symbol,
		Interval:     row.Interval,
		Script:       row.Script,
		VisualModel:  visualModel,
		CreatedAt:    row.CreatedAt,
		UpdatedAt:    row.UpdatedAt,
	})
}

func strategyDesignDefinitionRowFromDefinition(definition strategyDesignDefinition) (strategyDesignDefinitionRow, error) {
	visualModelJSON := ""
	if definition.VisualModel != nil {
		data, err := json.Marshal(definition.VisualModel)
		if err != nil {
			return strategyDesignDefinitionRow{}, err
		}
		visualModelJSON = string(data)
	}
	return strategyDesignDefinitionRow{
		ID:              definition.ID,
		Name:            definition.Name,
		Version:         definition.Version,
		Description:     definition.Description,
		Runtime:         definition.Runtime,
		SourceFormat:    definition.SourceFormat,
		Symbol:          definition.Symbol,
		Interval:        definition.Interval,
		Script:          definition.Script,
		VisualModelJSON: visualModelJSON,
		CreatedAt:       definition.CreatedAt,
		UpdatedAt:       definition.UpdatedAt,
	}, nil
}

func normalizeStrategyDesignDefinition(input strategyDesignDefinition) (strategyDesignDefinition, error) {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	input.ID = strings.TrimSpace(input.ID)
	if input.ID == "" {
		input.ID = generateStrategyDefinitionID()
	}
	input.Name = strings.TrimSpace(input.Name)
	if input.Name == "" {
		input.Name = input.ID
	}
	input.Version = normalizeStrategySemanticVersion(input.Version)
	input.Description = strings.TrimSpace(input.Description)
	sourceFormat, err := normalizeStrategyDesignSourceFormat(input.SourceFormat)
	if err != nil {
		return strategyDesignDefinition{}, err
	}
	runtime, err := normalizeStrategyRuntime(input.Runtime)
	if err != nil {
		return strategyDesignDefinition{}, err
	}
	input.SourceFormat = sourceFormat
	input.Runtime = runtime
	input.Symbol = strings.ToUpper(strings.TrimSpace(input.Symbol))
	input.Interval = strings.TrimSpace(input.Interval)
	visualModel, err := normalizeStrategyVisualModel(input.VisualModel)
	if err != nil {
		return strategyDesignDefinition{}, err
	}
	input.VisualModel = visualModel
	if strings.TrimSpace(input.Script) == "" {
		input.Script = defaultStrategyDesignScript(input.Name, input.SourceFormat)
	}
	input.Script = syncStrategyScriptVersion(input.Script, input.Version)
	if input.CreatedAt == "" {
		input.CreatedAt = now
	}
	if input.UpdatedAt == "" {
		input.UpdatedAt = now
	}
	return input, nil
}

func generateStrategyDefinitionID() string {
	id, err := uuid.NewRandom()
	if err != nil {
		return "pine-strategy-" + time.Now().UTC().Format("20060102150405.000000000")
	}
	return id.String()
}

func defaultStrategyDesignScript(name string, sourceFormat string) string {
	_ = sourceFormat
	return defaultStrategyDesignPine(name)
}

func defaultStrategyDesignPine(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		name = "Pine Strategy"
	}
	escapedName := strings.ReplaceAll(name, `"`, `\"`)
	return "//@version=6\n" +
		"strategy(\"" + escapedName + "\", overlay=true, default_qty_type=strategy.percent_of_equity, default_qty_value=10)\n\n" +
		"// JFTrade executes supported Pine strategy statements on each closed K line.\n" +
		"fast = ta.ema(close, 8)\n" +
		"slow = ta.ema(close, 21)\n" +
		"if ta.crossover(fast, slow)\n" +
		"    strategy.entry(\"Long\", strategy.long)\n"
}

func normalizeStrategySemanticVersion(value string) string {
	canonical := canonicalStrategySemanticVersion(value)
	if canonical == "" {
		return defaultStrategyVersion
	}
	return strings.TrimPrefix(canonical, "v")
}

func canonicalStrategySemanticVersion(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if !strings.HasPrefix(value, "v") {
		value = "v" + value
	}
	return semver.Canonical(value)
}

func nextStrategyDefinitionVersion(current string) string {
	canonical := canonicalStrategySemanticVersion(current)
	if canonical == "" {
		return defaultStrategyVersion
	}
	parts := strings.Split(strings.TrimPrefix(canonical, "v"), ".")
	if len(parts) != 3 {
		return defaultStrategyVersion
	}
	major, majorErr := strconv.Atoi(parts[0])
	minor, minorErr := strconv.Atoi(parts[1])
	patch, patchErr := strconv.Atoi(parts[2])
	if majorErr != nil || minorErr != nil || patchErr != nil {
		return defaultStrategyVersion
	}
	return fmt.Sprintf("%d.%d.%d", major, minor, patch+1)
}

func syncStrategyScriptVersion(script string, version string) string {
	if strings.TrimSpace(script) == "" {
		return script
	}
	return script
}

func strategyDesignDefinitionMeaningfullyChanged(left, right strategyDesignDefinition) bool {
	left.CreatedAt = ""
	left.UpdatedAt = ""
	left.Version = ""
	left.Script = syncStrategyScriptVersion(left.Script, defaultStrategyVersion)
	right.CreatedAt = ""
	right.UpdatedAt = ""
	right.Version = ""
	right.Script = syncStrategyScriptVersion(right.Script, defaultStrategyVersion)
	return !strategyDesignDefinitionsEqual(left, right)
}

func normalizeStrategyRuntime(runtime string) (string, error) {
	normalized := strings.TrimSpace(strings.ToLower(runtime))
	if normalized == "" || normalized == strategyRuntimePinePlan {
		return strategyRuntimePinePlan, nil
	}
	return "", fmt.Errorf("%w: runtime %q is no longer supported; use %s", errUnsupportedLegacyStrategyDefinition, runtime, strategyRuntimePinePlan)
}

func normalizeStrategyDesignSourceFormat(sourceFormat string) (string, error) {
	normalized := strings.TrimSpace(strings.ToLower(sourceFormat))
	if normalized == "" || normalized == strategydefinition.SourceFormatPineV6 {
		return strategydefinition.SourceFormatPineV6, nil
	}
	return "", fmt.Errorf("%w: sourceFormat %q is no longer supported; use %s", errUnsupportedLegacyStrategyDefinition, sourceFormat, strategydefinition.SourceFormatPineV6)
}

func normalizeStrategyVisualModel(model *strategyVisualModel) (*strategyVisualModel, error) {
	if model == nil {
		return nil, nil
	}
	normalized := *model
	if strings.TrimSpace(normalized.Engine) == "" {
		normalized.Engine = "logic-flow"
	}
	if normalized.Version == 0 {
		normalized.Version = 1
	}
	if normalized.Nodes == nil {
		normalized.Nodes = []strategyVisualNode{}
	}
	for index := range normalized.Nodes {
		if normalized.Nodes[index].Properties == nil {
			normalized.Nodes[index].Properties = map[string]any{}
		}
		if err := validateStrategyVisualNodeProperties(normalized.Nodes[index].Properties); err != nil {
			return nil, err
		}
	}
	if normalized.Edges == nil {
		normalized.Edges = []strategyVisualEdge{}
	}
	for index := range normalized.Edges {
		if normalized.Edges[index].Type == "" {
			normalized.Edges[index].Type = "polyline"
		}
		if normalized.Edges[index].Properties == nil {
			normalized.Edges[index].Properties = map[string]any{}
		}
	}
	return &normalized, nil
}

func validateStrategyVisualNodeProperties(properties map[string]any) error {
	blockKind, _ := properties["blockKind"].(string)
	switch strings.TrimSpace(blockKind) {
	case "codeBlock", "technicalIndicator":
		return fmt.Errorf("%w: visual block %q is no longer supported; rebuild it with Pine v6 blocks or pineSnippet", errUnsupportedLegacyStrategyDefinition, blockKind)
	default:
		return nil
	}
}

func strategyDesignDefinitionsEqual(left, right strategyDesignDefinition) bool {
	leftJSON, leftErr := json.Marshal(left)
	rightJSON, rightErr := json.Marshal(right)
	if leftErr != nil || rightErr != nil {
		return false
	}
	return string(leftJSON) == string(rightJSON)
}
