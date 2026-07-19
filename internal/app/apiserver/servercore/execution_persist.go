package servercore

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/jftrade/jftrade-main/internal/store/sqliteconn"
	"github.com/jftrade/jftrade-main/internal/store/sqliteschema"
	"github.com/jftrade/jftrade-main/pkg/besteffort"
)

const (
	defaultExecutionOrderDBFilename = "execution-orders.db"
	executionOrderTable             = "execution_orders"
	executionOrderEventTable        = "execution_order_events"
	executionOrderLegTable          = "execution_order_legs"
	executionOrderPreviewTable      = "execution_order_previews"
	executionPredictionQuoteTable   = "execution_prediction_quotes"
	executionSeenFillTable          = "execution_seen_fills"
	executionSequenceTable          = "execution_sequences"
)

type executionOrderSQLiteStore struct {
	db             *sqliteconn.DB
	path           string
	beginMigration func(context.Context, *sql.TxOptions) (executionMigrationTx, error)
}

type executionMigrationTx interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
	Commit() error
	Rollback() error
}

type executionSchemaRows interface {
	Next() bool
	Scan(...any) error
	Err() error
	Close() error
}

type executionOrderSummaryRow struct {
	InternalOrderID    string          `db:"internal_order_id"`
	BrokerID           string          `db:"broker_id"`
	BrokerOrderID      sql.NullString  `db:"broker_order_id"`
	BrokerOrderIDEx    sql.NullString  `db:"broker_order_id_ex"`
	Source             string          `db:"source"`
	SourceDetail       string          `db:"source_detail"`
	TradingEnvironment string          `db:"trading_environment"`
	AccountID          string          `db:"account_id"`
	Market             string          `db:"market"`
	Symbol             sql.NullString  `db:"symbol"`
	Side               sql.NullString  `db:"side"`
	OrderType          sql.NullString  `db:"order_type"`
	Status             string          `db:"status"`
	RawBrokerStatus    sql.NullString  `db:"raw_broker_status"`
	RequestedQuantity  sql.NullFloat64 `db:"requested_quantity"`
	RequestedPrice     sql.NullFloat64 `db:"requested_price"`
	FilledQuantity     sql.NullFloat64 `db:"filled_quantity"`
	FilledAveragePrice sql.NullFloat64 `db:"filled_average_price"`
	Remark             sql.NullString  `db:"remark"`
	LastError          sql.NullString  `db:"last_error"`
	LastErrorCode      sql.NullString  `db:"last_error_code"`
	LastErrorSource    sql.NullString  `db:"last_error_source"`
	SubmittedAt        sql.NullString  `db:"submitted_at"`
	UpdatedAt          string          `db:"updated_at"`
	CreatedAt          string          `db:"created_at"`
	OrderKind          string          `db:"order_kind"`
	ProductClass       string          `db:"product_class"`
	QuantityMode       string          `db:"quantity_mode"`
	ClientOrderID      sql.NullString  `db:"client_order_id"`
	PreviewID          sql.NullString  `db:"preview_id"`
	NormalizedRequest  string          `db:"normalized_request"`
	RequestedAmount    sql.NullFloat64 `db:"requested_amount"`
	Payout             sql.NullFloat64 `db:"payout"`
	Fees               sql.NullFloat64 `db:"fees"`
}

type executionOrderLegRow struct {
	ID                string          `db:"id"`
	InternalOrderID   string          `db:"internal_order_id"`
	LegIndex          int             `db:"leg_index"`
	BrokerLegID       sql.NullString  `db:"broker_leg_id"`
	InstrumentID      string          `db:"instrument_id"`
	ProductClass      string          `db:"product_class"`
	Side              string          `db:"side"`
	Ratio             int             `db:"ratio"`
	PredictionSide    string          `db:"prediction_side"`
	RequestedQuantity sql.NullFloat64 `db:"requested_quantity"`
	RequestedAmount   sql.NullFloat64 `db:"requested_amount"`
	RequestedPrice    sql.NullFloat64 `db:"requested_price"`
	Status            string          `db:"status"`
	FilledQuantity    sql.NullFloat64 `db:"filled_quantity"`
	FilledAmount      sql.NullFloat64 `db:"filled_amount"`
	AveragePrice      sql.NullFloat64 `db:"average_price"`
	Fees              sql.NullFloat64 `db:"fees"`
	Payout            sql.NullFloat64 `db:"payout"`
	UpdatedAt         string          `db:"updated_at"`
	CreatedAt         string          `db:"created_at"`
}

type executionOrderPreviewRow struct {
	PreviewID         string         `db:"preview_id"`
	RequestHash       string         `db:"request_hash"`
	BrokerID          string         `db:"broker_id"`
	CapabilityVersion string         `db:"capability_version"`
	AccountID         string         `db:"account_id"`
	ExpiresAt         string         `db:"expires_at"`
	QuoteExpiresAt    sql.NullString `db:"quote_expires_at"`
	RFQID             sql.NullString `db:"rfq_id"`
	NormalizedRequest string         `db:"normalized_request"`
	CreatedAt         string         `db:"created_at"`
	ConsumedAt        sql.NullString `db:"consumed_at"`
}

type executionPredictionQuoteRow struct {
	QuoteID            string          `db:"quote_id"`
	BrokerID           string          `db:"broker_id"`
	AccountID          string          `db:"account_id"`
	TradingEnvironment string          `db:"trading_environment"`
	MVC                string          `db:"mvc"`
	LegsHash           string          `db:"legs_hash"`
	BidPrice           sql.NullFloat64 `db:"bid_price"`
	AskPrice           sql.NullFloat64 `db:"ask_price"`
	ShouldRetry        bool            `db:"should_retry"`
	ReceivedAt         string          `db:"received_at"`
	ExpiresAt          string          `db:"expires_at"`
	ExpirySource       string          `db:"expiry_source"`
	Status             string          `db:"status"`
	ConsumedAt         sql.NullString  `db:"consumed_at"`
	ConsumedPreviewID  sql.NullString  `db:"consumed_preview_id"`
	ConsumedClientID   sql.NullString  `db:"consumed_client_order_id"`
}

type executionOrderEventRow struct {
	ID              string         `db:"id"`
	InternalOrderID string         `db:"internal_order_id"`
	EventType       string         `db:"event_type"`
	PreviousStatus  sql.NullString `db:"previous_status"`
	NextStatus      string         `db:"next_status"`
	PayloadJSON     string         `db:"payload_json"`
	CreatedAt       string         `db:"created_at"`
}

type executionSeenFillRow struct {
	FillKey   string `db:"fill_key"`
	CreatedAt string `db:"created_at"`
}

type executionSequenceRow struct {
	Name  string `db:"name"`
	Value uint64 `db:"value"`
}

func deriveExecutionOrderDBPath(settingsPath string) string {
	if envPath := strings.TrimSpace(os.Getenv("JFTRADE_EXECUTION_ORDER_DB")); envPath != "" {
		return envPath
	}
	directory := filepath.Dir(strings.TrimSpace(settingsPath))
	if directory == "" || directory == "." {
		return defaultExecutionOrderDBFilename
	}
	return filepath.Join(directory, defaultExecutionOrderDBFilename)
}

func newExecutionOrderStoreWithDB(dbPath string) (*executionOrderStore, error) {
	persistence, err := newExecutionOrderSQLiteStore(dbPath)
	if err != nil {
		return nil, err
	}
	return newExecutionOrderStoreWithPersistence(persistence)
}

func newExecutionOrderStoreWithPersistence(persistence *executionOrderSQLiteStore) (*executionOrderStore, error) {
	store := newExecutionOrderStore()
	store.persistence = persistence
	if err := store.loadFromDB(); err != nil {
		jftradeErr2 := persistence.Close()
		besteffort.LogError(jftradeErr2)
		return nil, err
	}
	store.startPersistenceWorker()
	return store, nil
}

func newExecutionOrderSQLiteStore(dbPath string) (*executionOrderSQLiteStore, error) {
	return newExecutionOrderSQLiteStoreWithDeps(dbPath, os.Stat, sqliteconn.OpenX)
}

func newExecutionOrderSQLiteStoreWithDeps(
	dbPath string,
	stat func(string) (os.FileInfo, error),
	open func(string, ...sqliteconn.Option) (*sqliteconn.DB, error),
) (*executionOrderSQLiteStore, error) {
	trimmedPath := strings.TrimSpace(dbPath)
	if trimmedPath == "" {
		return nil, fmt.Errorf("execution order db path is required")
	}
	directory := filepath.Dir(trimmedPath)
	if directory != "" && directory != "." {
		if err := os.MkdirAll(directory, 0o755); err != nil {
			return nil, fmt.Errorf("create execution order db directory: %w", err)
		}
	}
	newDatabase := true
	if info, statErr := stat(trimmedPath); statErr == nil {
		newDatabase = info.Size() == 0
	} else if !errors.Is(statErr, os.ErrNotExist) {
		return nil, fmt.Errorf("inspect execution order sqlite store: %w", statErr)
	}

	db, err := open(trimmedPath)
	if err != nil {
		return nil, fmt.Errorf("open execution order sqlite store: %w", err)
	}
	store := &executionOrderSQLiteStore{db: db, path: trimmedPath}
	if !newDatabase {
		if err := store.migrateLegacySchema(); err != nil {
			jftradeErr1 := db.Close()
			besteffort.LogError(jftradeErr1)
			return nil, fmt.Errorf("migrate execution order sqlite store: %w", err)
		}
	}
	if err := store.initializeOrValidateSchema(); err != nil {
		jftradeErr1 := db.Close()
		besteffort.LogError(jftradeErr1)
		return nil, fmt.Errorf("migrate execution order sqlite store: %w", err)
	}
	return store, nil
}

func (s *executionOrderSQLiteStore) migrateLegacySchema() error {
	if err := s.migrateSchemaV1ToV2(); err != nil {
		return err
	}
	if err := s.migrateSchemaV2ToV3(); err != nil {
		return err
	}
	if err := s.migrateSchemaV3ToV4(); err != nil {
		return err
	}
	return s.migrateSchemaV4ToV5()
}

func (s *executionOrderSQLiteStore) migrateSchemaV1ToV2() error {
	var metadataTable string
	err := s.db.QueryRowxContext(context.Background(),
		`SELECT name FROM sqlite_master WHERE type = 'table' AND name = ? LIMIT 1`,
		sqliteschema.MetadataTable,
	).Scan(&metadataTable)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}

	var version int
	err = s.db.QueryRowxContext(context.Background(),
		`SELECT version FROM `+sqliteschema.MetadataTable+` WHERE component_id = ? LIMIT 1`,
		"execution-orders",
	).Scan(&version)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		return err
	}
	if version != 1 {
		return nil
	}

	beginMigration := s.beginMigration
	if beginMigration == nil {
		beginMigration = func(ctx context.Context, opts *sql.TxOptions) (executionMigrationTx, error) {
			return s.db.BeginWrite(ctx, opts)
		}
	}
	tx, err := beginMigration(context.Background(), nil)
	if err != nil {
		return err
	}
	rollback := true
	defer func() {
		if rollback {
			_ = tx.Rollback()
		}
	}()
	if _, err := tx.ExecContext(context.Background(),
		`ALTER TABLE `+executionOrderTable+` ADD COLUMN raw_broker_status TEXT`,
	); err != nil {
		return err
	}
	if _, err := tx.ExecContext(context.Background(),
		`UPDATE `+executionOrderTable+` SET raw_broker_status = status WHERE TRIM(status) <> ''`,
	); err != nil {
		return err
	}
	if _, err := tx.ExecContext(context.Background(),
		`UPDATE `+sqliteschema.MetadataTable+` SET version = 2 WHERE component_id = ?`,
		"execution-orders",
	); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	rollback = false
	return nil
}

func (s *executionOrderSQLiteStore) migrateSchemaV2ToV3() error {
	version, exists, err := s.executionSchemaVersion()
	if err != nil || !exists || version != 2 {
		return err
	}
	existingColumns := []string{}
	if err := s.db.Select(&existingColumns, `SELECT name FROM pragma_table_info(?)`, executionOrderTable); err != nil {
		return err
	}
	return s.executeSchemaV3Migration(executionSchemaV3Statements(existingColumns))
}

func (s *executionOrderSQLiteStore) executionSchemaVersion() (int, bool, error) {
	var metadataTable string
	err := s.db.QueryRowxContext(context.Background(),
		`SELECT name FROM sqlite_master WHERE type = 'table' AND name = ? LIMIT 1`,
		sqliteschema.MetadataTable,
	).Scan(&metadataTable)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	var version int
	err = s.db.QueryRowxContext(context.Background(),
		`SELECT version FROM `+sqliteschema.MetadataTable+` WHERE component_id = ? LIMIT 1`,
		"execution-orders",
	).Scan(&version)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	return version, true, nil
}

func (s *executionOrderSQLiteStore) migrateSchemaV3ToV4() error {
	version, exists, err := s.executionSchemaVersion()
	if err != nil || !exists || version != 3 {
		return err
	}
	tx, err := s.db.BeginWrite(context.Background(), nil)
	if err != nil {
		return err
	}
	rollback := true
	defer func() {
		if rollback {
			_ = tx.Rollback()
		}
	}()
	if _, err := tx.ExecContext(context.Background(), executionPredictionQuoteSchemaStatement()); err != nil {
		return err
	}
	if _, err := tx.ExecContext(context.Background(),
		`CREATE INDEX IF NOT EXISTS idx_execution_prediction_quotes_expiry ON `+
			executionPredictionQuoteTable+` (status, expires_at)`,
	); err != nil {
		return err
	}
	if _, err := tx.ExecContext(context.Background(),
		`UPDATE `+sqliteschema.MetadataTable+` SET version = 4 WHERE component_id = ?`,
		"execution-orders",
	); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	rollback = false
	return nil
}

func (s *executionOrderSQLiteStore) migrateSchemaV4ToV5() error {
	version, exists, err := s.executionSchemaVersion()
	if err != nil || !exists || version != 4 {
		return err
	}
	tx, err := s.db.BeginWrite(context.Background(), nil)
	if err != nil {
		return err
	}
	rollback := true
	defer func() {
		if rollback {
			_ = tx.Rollback()
		}
	}()
	if _, err := tx.ExecContext(context.Background(),
		`ALTER TABLE `+executionOrderTable+` ADD COLUMN fees REAL`,
	); err != nil {
		return err
	}
	if _, err := tx.ExecContext(context.Background(),
		`UPDATE `+sqliteschema.MetadataTable+` SET version = 5 WHERE component_id = ?`,
		"execution-orders",
	); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	rollback = false
	return nil
}

func executionSchemaV3Statements(existingColumns []string) []string {
	existingSet := make(map[string]struct{}, len(existingColumns))
	for _, column := range existingColumns {
		existingSet[column] = struct{}{}
	}
	columnStatements := []struct{ name, statement string }{
		{"order_kind", `ALTER TABLE ` + executionOrderTable + ` ADD COLUMN order_kind TEXT NOT NULL DEFAULT 'single'`},
		{"product_class", `ALTER TABLE ` + executionOrderTable + ` ADD COLUMN product_class TEXT NOT NULL DEFAULT 'unknown'`},
		{"quantity_mode", `ALTER TABLE ` + executionOrderTable + ` ADD COLUMN quantity_mode TEXT NOT NULL DEFAULT 'units'`},
		{"client_order_id", `ALTER TABLE ` + executionOrderTable + ` ADD COLUMN client_order_id TEXT`},
		{"preview_id", `ALTER TABLE ` + executionOrderTable + ` ADD COLUMN preview_id TEXT`},
		{"normalized_request", `ALTER TABLE ` + executionOrderTable + ` ADD COLUMN normalized_request TEXT NOT NULL DEFAULT '{}'`},
		{"requested_amount", `ALTER TABLE ` + executionOrderTable + ` ADD COLUMN requested_amount REAL`},
		{"payout", `ALTER TABLE ` + executionOrderTable + ` ADD COLUMN payout REAL`},
	}
	statements := make([]string, 0, len(columnStatements)+4)
	for _, column := range columnStatements {
		if _, ok := existingSet[column.name]; !ok {
			statements = append(statements, column.statement)
		}
	}
	statements = append(statements,
		executionOrderLegSchemaStatement(), executionOrderPreviewSchemaStatement(),
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_execution_orders_client_id ON `+executionOrderTable+` (broker_id, trading_environment, account_id, client_order_id) WHERE client_order_id IS NOT NULL AND TRIM(client_order_id) <> ''`,
		`CREATE INDEX IF NOT EXISTS idx_execution_order_legs_order ON `+executionOrderLegTable+` (internal_order_id, leg_index ASC)`,
	)
	return statements
}

func (s *executionOrderSQLiteStore) executeSchemaV3Migration(statements []string) error {
	beginMigration := s.beginMigration
	if beginMigration == nil {
		beginMigration = func(ctx context.Context, opts *sql.TxOptions) (executionMigrationTx, error) {
			return s.db.BeginWrite(ctx, opts)
		}
	}
	tx, err := beginMigration(context.Background(), nil)
	if err != nil {
		return err
	}
	rollback := true
	defer func() {
		if rollback {
			_ = tx.Rollback()
		}
	}()
	for _, statement := range statements {
		if _, err := tx.ExecContext(context.Background(), statement); err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(context.Background(),
		`UPDATE `+sqliteschema.MetadataTable+` SET version = 3 WHERE component_id = ?`,
		"execution-orders",
	); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	rollback = false
	return nil
}

func (s *executionOrderSQLiteStore) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

func (s *executionOrderSQLiteStore) initializeOrValidateSchema() error {
	return sqliteschema.InitializeOrValidate(
		context.Background(),
		s.db,
		s.path,
		"execution-orders",
		5,
		executionSchemaStatements(),
		func(ctx context.Context, _ sqliteschema.Database) error {
			return s.validateExecutionSchemas(ctx)
		},
	)
}

func executionSchemaStatements() []string {
	return []string{
		strings.Join([]string{
			`CREATE TABLE IF NOT EXISTS ` + executionOrderTable + ` (`,
			`  internal_order_id    TEXT PRIMARY KEY,`,
			`  broker_id            TEXT NOT NULL DEFAULT '',`,
			`  broker_order_id      TEXT,`,
			`  broker_order_id_ex   TEXT,`,
			`  source               TEXT NOT NULL DEFAULT '',`,
			`  source_detail        TEXT NOT NULL DEFAULT '',`,
			`  trading_environment  TEXT NOT NULL DEFAULT '',`,
			`  account_id           TEXT NOT NULL DEFAULT '',`,
			`  market               TEXT NOT NULL DEFAULT '',`,
			`  symbol               TEXT,`,
			`  side                 TEXT,`,
			`  order_type           TEXT,`,
			`  status               TEXT NOT NULL DEFAULT '',`,
			`  requested_quantity   REAL,`,
			`  requested_price      REAL,`,
			`  filled_quantity      REAL,`,
			`  filled_average_price REAL,`,
			`  remark               TEXT,`,
			`  last_error           TEXT,`,
			`  last_error_code      TEXT,`,
			`  last_error_source    TEXT,`,
			`  submitted_at         TEXT,`,
			`  updated_at           TEXT NOT NULL DEFAULT '',`,
			`  created_at           TEXT NOT NULL DEFAULT '',`,
			`  raw_broker_status    TEXT,`,
			`  order_kind           TEXT NOT NULL DEFAULT 'single',`,
			`  product_class        TEXT NOT NULL DEFAULT 'unknown',`,
			`  quantity_mode        TEXT NOT NULL DEFAULT 'units',`,
			`  client_order_id      TEXT,`,
			`  preview_id           TEXT,`,
			`  normalized_request   TEXT NOT NULL DEFAULT '{}',`,
			`  requested_amount     REAL,`,
			`  payout               REAL,`,
			`  fees                 REAL`,
			`)`,
		}, " "),
		executionOrderLegSchemaStatement(),
		executionOrderPreviewSchemaStatement(),
		executionPredictionQuoteSchemaStatement(),
		strings.Join([]string{
			`CREATE TABLE IF NOT EXISTS ` + executionOrderEventTable + ` (`,
			`  id                TEXT PRIMARY KEY,`,
			`  internal_order_id TEXT NOT NULL,`,
			`  event_type        TEXT NOT NULL DEFAULT '',`,
			`  previous_status   TEXT,`,
			`  next_status       TEXT NOT NULL DEFAULT '',`,
			`  payload_json      TEXT NOT NULL DEFAULT '{}',`,
			`  created_at        TEXT NOT NULL DEFAULT ''`,
			`)`,
		}, " "),
		strings.Join([]string{
			`CREATE TABLE IF NOT EXISTS ` + executionSeenFillTable + ` (`,
			`  fill_key   TEXT PRIMARY KEY,`,
			`  created_at TEXT NOT NULL DEFAULT ''`,
			`)`,
		}, " "),
		strings.Join([]string{
			`CREATE TABLE IF NOT EXISTS ` + executionSequenceTable + ` (`,
			`  name  TEXT PRIMARY KEY,`,
			`  value INTEGER NOT NULL DEFAULT 0`,
			`)`,
		}, " "),
		`CREATE INDEX IF NOT EXISTS idx_execution_orders_updated ON ` + executionOrderTable + ` (updated_at DESC, created_at DESC, internal_order_id DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_execution_orders_broker_order ON ` + executionOrderTable + ` (broker_id, trading_environment, account_id, market, broker_order_id)`,
		`CREATE INDEX IF NOT EXISTS idx_execution_orders_broker_order_ex ON ` + executionOrderTable + ` (broker_id, trading_environment, account_id, market, broker_order_id_ex)`,
		`CREATE INDEX IF NOT EXISTS idx_execution_order_events_order ON ` + executionOrderEventTable + ` (internal_order_id, created_at ASC, id ASC)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_execution_orders_client_id ON ` + executionOrderTable + ` (broker_id, trading_environment, account_id, client_order_id) WHERE client_order_id IS NOT NULL AND TRIM(client_order_id) <> ''`,
		`CREATE INDEX IF NOT EXISTS idx_execution_order_legs_order ON ` + executionOrderLegTable + ` (internal_order_id, leg_index ASC)`,
		`CREATE INDEX IF NOT EXISTS idx_execution_prediction_quotes_expiry ON ` + executionPredictionQuoteTable + ` (status, expires_at)`,
	}
}

func executionOrderLegSchemaStatement() string {
	return strings.Join([]string{
		`CREATE TABLE IF NOT EXISTS ` + executionOrderLegTable + ` (`,
		`  id                 TEXT PRIMARY KEY,`,
		`  internal_order_id  TEXT NOT NULL,`,
		`  leg_index          INTEGER NOT NULL,`,
		`  broker_leg_id      TEXT,`,
		`  instrument_id      TEXT NOT NULL,`,
		`  product_class      TEXT NOT NULL DEFAULT 'unknown',`,
		`  side               TEXT NOT NULL DEFAULT '',`,
		`  ratio              INTEGER NOT NULL DEFAULT 1,`,
		`  prediction_side    TEXT NOT NULL DEFAULT '',`,
		`  requested_quantity REAL,`,
		`  requested_amount   REAL,`,
		`  requested_price    REAL,`,
		`  status             TEXT NOT NULL DEFAULT '',`,
		`  filled_quantity    REAL,`,
		`  filled_amount      REAL,`,
		`  average_price      REAL,`,
		`  fees               REAL,`,
		`  payout             REAL,`,
		`  updated_at         TEXT NOT NULL DEFAULT '',`,
		`  created_at         TEXT NOT NULL DEFAULT ''`,
		`)`,
	}, " ")
}

func executionPredictionQuoteSchemaStatement() string {
	return strings.Join([]string{
		`CREATE TABLE IF NOT EXISTS ` + executionPredictionQuoteTable + ` (`,
		`  quote_id                 TEXT PRIMARY KEY,`,
		`  broker_id                TEXT NOT NULL,`,
		`  account_id               TEXT NOT NULL,`,
		`  trading_environment      TEXT NOT NULL,`,
		`  mvc                      TEXT NOT NULL,`,
		`  legs_hash                TEXT NOT NULL,`,
		`  bid_price                REAL,`,
		`  ask_price                REAL,`,
		`  should_retry             INTEGER NOT NULL DEFAULT 0,`,
		`  received_at              TEXT NOT NULL,`,
		`  expires_at               TEXT NOT NULL,`,
		`  expiry_source            TEXT NOT NULL DEFAULT 'jftrade_policy',`,
		`  status                   TEXT NOT NULL DEFAULT 'active',`,
		`  consumed_at              TEXT,`,
		`  consumed_preview_id      TEXT,`,
		`  consumed_client_order_id TEXT`,
		`)`,
	}, " ")
}

func executionOrderPreviewSchemaStatement() string {
	return strings.Join([]string{
		`CREATE TABLE IF NOT EXISTS ` + executionOrderPreviewTable + ` (`,
		`  preview_id         TEXT PRIMARY KEY,`,
		`  request_hash       TEXT NOT NULL,`,
		`  broker_id          TEXT NOT NULL,`,
		`  capability_version TEXT NOT NULL,`,
		`  account_id         TEXT NOT NULL,`,
		`  expires_at         TEXT NOT NULL,`,
		`  quote_expires_at   TEXT,`,
		`  rfq_id             TEXT,`,
		`  normalized_request TEXT NOT NULL,`,
		`  created_at         TEXT NOT NULL,`,
		`  consumed_at        TEXT`,
		`)`,
	}, " ")
}

func (s *executionOrderSQLiteStore) validateExecutionSchemas(context.Context) error {
	if err := s.ensureExistingSchemaCanBeOpened(); err != nil {
		return err
	}
	for _, schema := range expectedExecutionSchemas() {
		if err := s.ensureSchema(schema.table, schema.columns); err != nil {
			return err
		}
	}
	return nil
}

func expectedExecutionSchemas() []struct {
	table   string
	columns []string
} {
	return []struct {
		table   string
		columns []string
	}{
		{table: executionOrderTable, columns: expectedExecutionOrderColumns()},
		{table: executionOrderLegTable, columns: expectedExecutionOrderLegColumns()},
		{table: executionOrderPreviewTable, columns: expectedExecutionOrderPreviewColumns()},
		{table: executionPredictionQuoteTable, columns: expectedExecutionPredictionQuoteColumns()},
		{table: executionOrderEventTable, columns: expectedExecutionOrderEventColumns()},
		{table: executionSeenFillTable, columns: expectedExecutionSeenFillColumns()},
		{table: executionSequenceTable, columns: expectedExecutionSequenceColumns()},
	}
}

func (s *executionOrderSQLiteStore) ensureExistingSchemaCanBeOpened() error {
	rows := []string{}
	if err := s.db.Select(&rows, `SELECT name FROM sqlite_master WHERE type = 'table' AND name IN (?, ?, ?, ?, ?, ?, ?)`,
		executionOrderTable,
		executionOrderLegTable,
		executionOrderPreviewTable,
		executionPredictionQuoteTable,
		executionOrderEventTable,
		executionSeenFillTable,
		executionSequenceTable,
	); err != nil {
		return err
	}
	if len(rows) == 0 {
		return nil
	}
	existing := make(map[string]struct{}, len(rows))
	for _, row := range rows {
		existing[row] = struct{}{}
	}
	for _, tableName := range []string{
		executionOrderTable, executionOrderLegTable, executionOrderPreviewTable,
		executionPredictionQuoteTable,
		executionOrderEventTable, executionSeenFillTable, executionSequenceTable,
	} {
		if _, ok := existing[tableName]; !ok {
			return fmt.Errorf("%s schema is obsolete; rebuild the execution order database", tableName)
		}
	}
	return nil
}

func (s *executionOrderSQLiteStore) ensureSchema(tableName string, want []string) error {
	rows, err := s.db.QueryContext(context.Background(), `PRAGMA table_info(`+tableName+`)`)
	if err != nil {
		return fmt.Errorf("inspect %s schema: %w", tableName, err)
	}
	return inspectExecutionSchemaRows(tableName, want, rows)
}

func inspectExecutionSchemaRows(tableName string, want []string, rows executionSchemaRows) error {
	defer func() { besteffort.LogError(rows.Close()) }()

	got := make([]string, 0, len(want))
	for rows.Next() {
		var cid, notNull, pk int
		var name, dataType string
		var defaultValue sql.NullString
		if err := rows.Scan(&cid, &name, &dataType, &notNull, &defaultValue, &pk); err != nil {
			return fmt.Errorf("scan %s schema: %w", tableName, err)
		}
		got = append(got, fmt.Sprintf("%s:%s:%d", name, strings.ToUpper(dataType), pk))
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate %s schema: %w", tableName, err)
	}
	if len(got) != len(want) {
		return fmt.Errorf("%s schema is obsolete; rebuild the execution order database", tableName)
	}
	for index := range want {
		if got[index] != want[index] {
			return fmt.Errorf("%s schema is obsolete; rebuild the execution order database", tableName)
		}
	}
	return nil
}
