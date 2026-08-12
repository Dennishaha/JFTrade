package sqliteschema

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"reflect"
	"regexp"
	"sort"
	"strings"

	"github.com/jftrade/jftrade-main/internal/store/sqliteconn"
	"github.com/jmoiron/sqlx"
)

const (
	DatabaseBacktest     = "backtest"
	DatabaseBacktestRuns = "backtest-runs"
	DatabaseStrategy     = "strategy"
	DatabaseExecution    = "execution-orders"
	DatabaseADK          = "adk"
	DatabaseADKSession   = "adk-session"
	DatabaseADKArtifact  = "adk-artifact"
	DatabaseWatchlist    = "watchlist"
	DatabaseResearch     = "research"
)

const (
	BacktestVersion     = 2
	BacktestRunsVersion = 1
	StrategyVersion     = 2
	ExecutionVersion    = 5
	ADKVersion          = 4
	ADKSessionVersion   = 4
	ADKArtifactVersion  = 1
	WatchlistVersion    = 1
	ResearchVersion     = 1
)

// Definition is the complete current schema contract for one managed database.
// Statements are used both to initialize new files and to derive the expected
// structural manifest used for strict validation.
type Definition struct {
	ID           string
	Version      int
	Statements   []string
	DynamicTable *DynamicTableDefinition
}

type DynamicTableDefinition struct {
	Pattern       string
	PrototypeName string
	Statement     string
}

var currentDefinitions = []Definition{
	backtestDefinition(),
	backtestRunsDefinition(),
	strategyDefinition(),
	executionDefinition(),
	adkDefinition(),
	adkSessionDefinition(),
	adkArtifactDefinition(),
	watchlistDefinition(),
	researchDefinition(),
}

func Definitions() []Definition {
	result := make([]Definition, len(currentDefinitions))
	for index, definition := range currentDefinitions {
		result[index] = cloneDefinition(definition)
	}
	return result
}

func DefinitionFor(id string) (Definition, bool) {
	id = strings.TrimSpace(id)
	for _, definition := range currentDefinitions {
		if definition.ID == id {
			return cloneDefinition(definition), true
		}
	}
	return Definition{}, false
}

func MustDefinition(id string) Definition {
	definition, ok := DefinitionFor(id)
	if !ok {
		panic("unknown SQLite schema definition: " + id)
	}
	return definition
}

func Version(id string) (int, bool) {
	definition, ok := DefinitionFor(id)
	return definition.Version, ok
}

func Statements(id string) []string {
	definition, ok := DefinitionFor(id)
	if !ok {
		return nil
	}
	return append([]string(nil), definition.Statements...)
}

func cloneDefinition(definition Definition) Definition {
	definition.Statements = append([]string(nil), definition.Statements...)
	if definition.DynamicTable != nil {
		cloned := *definition.DynamicTable
		definition.DynamicTable = &cloned
	}
	return definition
}

func InitializeCurrent(ctx context.Context, db Database, path, id string) error {
	definition, ok := DefinitionFor(id)
	if !ok {
		return fmt.Errorf("unknown SQLite database id %q", id)
	}
	return InitializeOrValidate(ctx, db, path, definition.ID, definition.Version, definition.Statements,
		func(ctx context.Context, db Database) error {
			return validateDefinition(ctx, db, path, definition)
		},
	)
}

// ValidateCurrent validates an already-open database without modifying it.
func ValidateCurrent(ctx context.Context, db Database, path, id string) error {
	definition, ok := DefinitionFor(id)
	if !ok {
		return fmt.Errorf("unknown SQLite database id %q", id)
	}
	if err := ValidateMetadata(ctx, db, path, definition.ID, definition.Version); err != nil {
		return err
	}
	if err := validateDefinition(ctx, db, path, definition); err != nil {
		if IsIncompatible(err) {
			return err
		}
		return &IncompatibleError{Component: definition.ID, Path: path, Reason: err.Error()}
	}
	return nil
}

// ValidateCurrentFile performs a read-only preflight. Stores use it before a
// read-write connection is opened so incompatible files remain byte-for-byte
// untouched.
func ValidateCurrentFile(ctx context.Context, path, id string) (resultErr error) {
	newDatabase, err := DatabaseIsNew(path)
	if err != nil || newDatabase {
		return err
	}
	db, err := sqliteconn.OpenReadOnly(path)
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := db.Close(); closeErr != nil && resultErr == nil {
			resultErr = fmt.Errorf("close read-only SQLite schema preflight: %w", closeErr)
		}
	}()
	return ValidateCurrent(ctx, db, path, id)
}

type schemaSnapshot struct {
	tables   map[string]tableSnapshot
	views    []string
	triggers []string
}

type tableSnapshot struct {
	columns      []columnSnapshot
	indexes      []indexSnapshot
	foreignKeys  []foreignKeySnapshot
	withoutRowID bool
	strict       bool
}

type columnSnapshot struct {
	name         string
	typeName     string
	notNull      int
	defaultValue string
	primaryKey   int
	hidden       int
}

type indexSnapshot struct {
	name    string
	unique  int
	origin  string
	partial int
	columns []indexColumnSnapshot
	sql     string
}

type indexColumnSnapshot struct {
	sequence int
	columnID int
	name     string
	desc     int
	collate  string
	key      int
}

type foreignKeySnapshot struct {
	id       int
	sequence int
	table    string
	from     string
	to       string
	onUpdate string
	onDelete string
	match    string
}

func validateDefinition(ctx context.Context, db Database, path string, definition Definition) (resultErr error) {
	expectedDB, err := expectedDatabase(definition)
	if err != nil {
		return fmt.Errorf("build %s schema manifest: %w", definition.ID, err)
	}
	defer func() {
		if closeErr := expectedDB.Close(); closeErr != nil && resultErr == nil {
			resultErr = fmt.Errorf("close expected %s schema manifest: %w", definition.ID, closeErr)
		}
	}()
	expected, err := inspectSchema(ctx, expectedDB)
	if err != nil {
		return fmt.Errorf("inspect expected %s schema: %w", definition.ID, err)
	}
	actual, err := inspectSchema(ctx, db)
	if err != nil {
		return err
	}

	dynamicPattern, err := compileDynamicPattern(definition)
	if err != nil {
		return err
	}
	var dynamicPrototype tableSnapshot
	if definition.DynamicTable != nil {
		prototype, ok := expected.tables[definition.DynamicTable.PrototypeName]
		if !ok {
			return fmt.Errorf("dynamic table prototype %s is missing", definition.DynamicTable.PrototypeName)
		}
		dynamicPrototype = prototype
		delete(expected.tables, definition.DynamicTable.PrototypeName)
	}

	for tableName, expectedTable := range expected.tables {
		actualTable, ok := actual.tables[tableName]
		if !ok {
			return &IncompatibleError{Component: definition.ID, Path: path, Reason: "required table is missing: " + tableName}
		}
		if err := compareTable(tableName, actualTable, expectedTable); err != nil {
			return &IncompatibleError{Component: definition.ID, Path: path, Reason: err.Error()}
		}
		delete(actual.tables, tableName)
	}
	for tableName, actualTable := range actual.tables {
		if dynamicPattern == nil || !dynamicPattern.MatchString(tableName) {
			return &IncompatibleError{Component: definition.ID, Path: path, Reason: "unknown application table: " + tableName}
		}
		if err := compareDynamicTable(tableName, actualTable, dynamicPrototype); err != nil {
			return &IncompatibleError{Component: definition.ID, Path: path, Reason: err.Error()}
		}
	}
	if !equalStrings(actual.views, expected.views) {
		return &IncompatibleError{Component: definition.ID, Path: path, Reason: "views do not match current schema"}
	}
	if !equalStrings(actual.triggers, expected.triggers) {
		return &IncompatibleError{Component: definition.ID, Path: path, Reason: "triggers do not match current schema"}
	}
	if err := validateIntegrity(ctx, db); err != nil {
		return &IncompatibleError{Component: definition.ID, Path: path, Reason: err.Error()}
	}
	return nil
}

func compileDynamicPattern(definition Definition) (*regexp.Regexp, error) {
	if definition.DynamicTable == nil {
		return nil, nil
	}
	pattern, err := regexp.Compile(definition.DynamicTable.Pattern)
	if err != nil {
		return nil, fmt.Errorf("compile %s dynamic table pattern: %w", definition.ID, err)
	}
	return pattern, nil
}

func expectedDatabase(definition Definition) (*sqlx.DB, error) {
	db, err := sqlx.Open("sqlite", ":memory:")
	if err != nil {
		return nil, err
	}
	closeWithError := func(err error) (*sqlx.DB, error) {
		_ = db.Close()
		return nil, err
	}
	for _, statement := range definition.Statements {
		if strings.TrimSpace(statement) == "" {
			continue
		}
		if _, err := db.Exec(statement); err != nil {
			return closeWithError(err)
		}
	}
	if definition.DynamicTable != nil {
		if _, err := db.Exec(definition.DynamicTable.Statement); err != nil {
			return closeWithError(err)
		}
	}
	if _, err := db.Exec(`CREATE TABLE ` + MetadataTable + ` (component_id TEXT PRIMARY KEY, version INTEGER NOT NULL, created_at TEXT NOT NULL)`); err != nil {
		return closeWithError(err)
	}
	if _, err := db.Exec(`INSERT INTO `+MetadataTable+` (component_id, version, created_at) VALUES (?, ?, 'manifest')`, definition.ID, definition.Version); err != nil {
		return closeWithError(err)
	}
	return db, nil
}

func inspectSchema(ctx context.Context, db Database) (schemaSnapshot, error) {
	result := schemaSnapshot{tables: map[string]tableSnapshot{}}
	rows, err := db.QueryxContext(ctx, `SELECT type, name, COALESCE(sql, '') FROM sqlite_master
		WHERE type IN ('table', 'view', 'trigger') AND name NOT LIKE 'sqlite_%' ORDER BY type, name`)
	if err != nil {
		return result, err
	}
	tableSQL := map[string]string{}
	for rows.Next() {
		var objectType, name, ddl string
		if err := rows.Scan(&objectType, &name, &ddl); err != nil {
			_ = rows.Close()
			return result, err
		}
		switch objectType {
		case "table":
			tableSQL[name] = ddl
		case "view":
			result.views = append(result.views, name+":"+normalizeSQL(ddl))
		case "trigger":
			result.triggers = append(result.triggers, name+":"+normalizeSQL(ddl))
		}
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return result, err
	}
	if err := rows.Close(); err != nil {
		return result, err
	}
	for name, ddl := range tableSQL {
		table, err := inspectTable(ctx, db, name, ddl)
		if err != nil {
			return result, err
		}
		result.tables[name] = table
	}
	sort.Strings(result.views)
	sort.Strings(result.triggers)
	return result, nil
}

func inspectTable(ctx context.Context, db Database, tableName, ddl string) (tableSnapshot, error) {
	result := tableSnapshot{
		withoutRowID: strings.Contains(normalizeSQL(ddl), " without rowid"),
		strict:       strings.HasSuffix(normalizeSQL(ddl), " strict"),
	}
	columns, err := inspectColumns(ctx, db, tableName)
	if err != nil {
		return result, err
	}
	result.columns = columns
	indexes, err := inspectIndexes(ctx, db, tableName)
	if err != nil {
		return result, err
	}
	result.indexes = indexes
	foreignKeys, err := inspectForeignKeys(ctx, db, tableName)
	if err != nil {
		return result, err
	}
	result.foreignKeys = foreignKeys
	return result, nil
}

func inspectColumns(ctx context.Context, db Database, tableName string) ([]columnSnapshot, error) {
	columns, err := db.QueryxContext(ctx, `PRAGMA table_xinfo(`+quoteSQLiteIdentifier(tableName)+`)`)
	if err != nil {
		return nil, err
	}
	result := make([]columnSnapshot, 0)
	for columns.Next() {
		var column columnSnapshot
		var columnID int
		var defaultValue sql.NullString
		if err := columns.Scan(&columnID, &column.name, &column.typeName, &column.notNull, &defaultValue, &column.primaryKey, &column.hidden); err != nil {
			_ = columns.Close()
			return nil, err
		}
		column.typeName = strings.ToUpper(strings.TrimSpace(column.typeName))
		if defaultValue.Valid {
			column.defaultValue = normalizeSQL(defaultValue.String)
		}
		result = append(result, column)
	}
	if err := columns.Err(); err != nil {
		_ = columns.Close()
		return nil, err
	}
	if err := columns.Close(); err != nil {
		return nil, err
	}
	return result, nil
}

func inspectIndexes(ctx context.Context, db Database, tableName string) ([]indexSnapshot, error) {
	indexes, err := db.QueryxContext(ctx, `PRAGMA index_list(`+quoteSQLiteIdentifier(tableName)+`)`)
	if err != nil {
		return nil, err
	}
	result := make([]indexSnapshot, 0)
	for indexes.Next() {
		var sequence int
		var index indexSnapshot
		if err := indexes.Scan(&sequence, &index.name, &index.unique, &index.origin, &index.partial); err != nil {
			_ = indexes.Close()
			return nil, err
		}
		result = append(result, index)
	}
	if err := indexes.Err(); err != nil {
		_ = indexes.Close()
		return nil, err
	}
	if err := indexes.Close(); err != nil {
		return nil, err
	}
	for indexNumber := range result {
		index := &result[indexNumber]
		indexColumns, err := db.QueryxContext(ctx, `PRAGMA index_xinfo(`+quoteSQLiteIdentifier(index.name)+`)`)
		if err != nil {
			return nil, err
		}
		for indexColumns.Next() {
			var column indexColumnSnapshot
			var name sql.NullString
			if err := indexColumns.Scan(&column.sequence, &column.columnID, &name, &column.desc, &column.collate, &column.key); err != nil {
				_ = indexColumns.Close()
				return nil, err
			}
			if name.Valid {
				column.name = name.String
			}
			index.columns = append(index.columns, column)
		}
		if err := indexColumns.Err(); err != nil {
			_ = indexColumns.Close()
			return nil, err
		}
		if err := indexColumns.Close(); err != nil {
			return nil, err
		}
		var indexSQL sql.NullString
		err = db.QueryRowxContext(ctx, `SELECT sql FROM sqlite_master WHERE type = 'index' AND name = ?`, index.name).Scan(&indexSQL)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return nil, err
		}
		if indexSQL.Valid {
			index.sql = normalizeIndexSQL(indexSQL.String)
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].name < result[j].name })
	return result, nil
}

func inspectForeignKeys(ctx context.Context, db Database, tableName string) ([]foreignKeySnapshot, error) {
	foreignKeys, err := db.QueryxContext(ctx, `PRAGMA foreign_key_list(`+quoteSQLiteIdentifier(tableName)+`)`)
	if err != nil {
		return nil, err
	}
	result := make([]foreignKeySnapshot, 0)
	for foreignKeys.Next() {
		var foreignKey foreignKeySnapshot
		if err := foreignKeys.Scan(&foreignKey.id, &foreignKey.sequence, &foreignKey.table, &foreignKey.from, &foreignKey.to,
			&foreignKey.onUpdate, &foreignKey.onDelete, &foreignKey.match); err != nil {
			_ = foreignKeys.Close()
			return nil, err
		}
		result = append(result, foreignKey)
	}
	if err := foreignKeys.Err(); err != nil {
		_ = foreignKeys.Close()
		return nil, err
	}
	if err := foreignKeys.Close(); err != nil {
		return nil, err
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].id != result[j].id {
			return result[i].id < result[j].id
		}
		return result[i].sequence < result[j].sequence
	})
	return result, nil
}

func compareTable(name string, actual, expected tableSnapshot) error {
	if !reflect.DeepEqual(actual.columns, expected.columns) {
		return fmt.Errorf("%s columns do not match current schema", name)
	}
	if !reflect.DeepEqual(actual.indexes, expected.indexes) {
		return fmt.Errorf("%s indexes do not match current schema: actual=%#v expected=%#v", name, actual.indexes, expected.indexes)
	}
	if !reflect.DeepEqual(actual.foreignKeys, expected.foreignKeys) {
		return fmt.Errorf("%s foreign keys do not match current schema", name)
	}
	if actual.withoutRowID != expected.withoutRowID || actual.strict != expected.strict {
		return fmt.Errorf("%s table options do not match current schema", name)
	}
	return nil
}

func compareDynamicTable(name string, actual, expected tableSnapshot) error {
	normalizeAutomaticIndexes := func(indexes []indexSnapshot) []indexSnapshot {
		result := append([]indexSnapshot(nil), indexes...)
		for index := range result {
			if result[index].origin != "c" {
				result[index].name = ""
				result[index].sql = ""
			}
		}
		return result
	}
	actual.indexes = normalizeAutomaticIndexes(actual.indexes)
	expected.indexes = normalizeAutomaticIndexes(expected.indexes)
	return compareTable(name, actual, expected)
}

func validateIntegrity(ctx context.Context, db Database) error {
	rows, err := db.QueryxContext(ctx, `PRAGMA quick_check`)
	if err != nil {
		return err
	}
	quickCheckOK := false
	for rows.Next() {
		var result string
		if err := rows.Scan(&result); err != nil {
			_ = rows.Close()
			return err
		}
		if strings.EqualFold(strings.TrimSpace(result), "ok") {
			quickCheckOK = true
			continue
		}
		_ = rows.Close()
		return fmt.Errorf("quick_check failed: %s", result)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return err
	}
	if err := rows.Close(); err != nil {
		return err
	}
	if !quickCheckOK {
		return fmt.Errorf("quick_check did not return ok")
	}

	foreignKeys, err := db.QueryxContext(ctx, `PRAGMA foreign_key_check`)
	if err != nil {
		return err
	}
	if foreignKeys.Next() {
		var table string
		var rowID sql.NullInt64
		var parent string
		var foreignKeyID int
		if err := foreignKeys.Scan(&table, &rowID, &parent, &foreignKeyID); err != nil {
			_ = foreignKeys.Close()
			return err
		}
		_ = foreignKeys.Close()
		return fmt.Errorf("foreign_key_check failed for %s row %d referencing %s constraint %d", table, rowID.Int64, parent, foreignKeyID)
	}
	if err := foreignKeys.Err(); err != nil {
		_ = foreignKeys.Close()
		return err
	}
	return foreignKeys.Close()
}

func normalizeSQL(value string) string {
	return strings.ToLower(strings.Join(strings.Fields(strings.TrimSpace(value)), " "))
}

func normalizeIndexSQL(value string) string {
	value = normalizeSQL(value)
	value = strings.Replace(value, "create unique index if not exists ", "create unique index ", 1)
	value = strings.Replace(value, "create index if not exists ", "create index ", 1)
	return value
}

func quoteSQLiteIdentifier(value string) string {
	return `"` + strings.ReplaceAll(value, `"`, `""`) + `"`
}

func equalStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}
