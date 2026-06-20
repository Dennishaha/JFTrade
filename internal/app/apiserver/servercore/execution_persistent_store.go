package servercore

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"
	// Register the modernc SQLite driver for database/sql.
	_ "modernc.org/sqlite"
)

const (
	defaultExecutionOrderDBFilename = "execution-orders.db"
	executionOrderTable             = "execution_orders"
	executionOrderEventTable        = "execution_order_events"
	executionSeenFillTable          = "execution_seen_fills"
	executionSequenceTable          = "execution_sequences"
)

type executionOrderSQLiteStore struct {
	db *sqlx.DB
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
	store := newExecutionOrderStore()
	store.persistence = persistence
	if err := store.loadFromDB(); err != nil {
		jftradeErr2 := persistence.Close()
		jftradeLogError(jftradeErr2)
		return nil, err
	}
	store.startPersistenceWorker()
	return store, nil
}

func newExecutionOrderSQLiteStore(dbPath string) (*executionOrderSQLiteStore, error) {
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

	db, err := sqlx.Open("sqlite", trimmedPath+"?_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)")
	if err != nil {
		return nil, fmt.Errorf("open execution order sqlite store: %w", err)
	}
	store := &executionOrderSQLiteStore{db: db}
	if err := store.migrate(); err != nil {
		jftradeErr1 := db.Close()
		jftradeLogError(jftradeErr1)
		return nil, fmt.Errorf("migrate execution order sqlite store: %w", err)
	}
	return store, nil
}

func (s *executionOrderSQLiteStore) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

func (s *executionOrderSQLiteStore) migrate() error {
	if err := s.ensureExistingSchemaCanBeOpened(); err != nil {
		return err
	}
	for _, statement := range []string{
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
			`  created_at           TEXT NOT NULL DEFAULT ''`,
			`)`,
		}, " "),
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
	} {
		if _, err := s.db.ExecContext(context.Background(), statement); err != nil {
			return err
		}
	}
	for _, schema := range []struct {
		table   string
		columns []string
	}{
		{table: executionOrderTable, columns: expectedExecutionOrderColumns()},
		{table: executionOrderEventTable, columns: expectedExecutionOrderEventColumns()},
		{table: executionSeenFillTable, columns: expectedExecutionSeenFillColumns()},
		{table: executionSequenceTable, columns: expectedExecutionSequenceColumns()},
	} {
		if err := s.ensureSchema(schema.table, schema.columns); err != nil {
			return err
		}
	}
	return nil
}

func (s *executionOrderSQLiteStore) ensureExistingSchemaCanBeOpened() error {
	rows := []string{}
	if err := s.db.Select(&rows, `SELECT name FROM sqlite_master WHERE type = 'table' AND name IN (?, ?, ?, ?)`,
		executionOrderTable,
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
	for _, tableName := range []string{executionOrderTable, executionOrderEventTable, executionSeenFillTable, executionSequenceTable} {
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
	defer func() { jftradeLogError(rows.Close()) }()

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

func expectedExecutionOrderColumns() []string {
	return []string{
		"internal_order_id:TEXT:1", "broker_id:TEXT:0", "broker_order_id:TEXT:0", "broker_order_id_ex:TEXT:0",
		"source:TEXT:0", "source_detail:TEXT:0", "trading_environment:TEXT:0", "account_id:TEXT:0", "market:TEXT:0",
		"symbol:TEXT:0", "side:TEXT:0", "order_type:TEXT:0", "status:TEXT:0", "requested_quantity:REAL:0",
		"requested_price:REAL:0", "filled_quantity:REAL:0", "filled_average_price:REAL:0", "remark:TEXT:0",
		"last_error:TEXT:0", "last_error_code:TEXT:0", "last_error_source:TEXT:0", "submitted_at:TEXT:0",
		"updated_at:TEXT:0", "created_at:TEXT:0",
	}
}

func expectedExecutionOrderEventColumns() []string {
	return []string{
		"id:TEXT:1", "internal_order_id:TEXT:0", "event_type:TEXT:0", "previous_status:TEXT:0",
		"next_status:TEXT:0", "payload_json:TEXT:0", "created_at:TEXT:0",
	}
}

func expectedExecutionSeenFillColumns() []string {
	return []string{"fill_key:TEXT:1", "created_at:TEXT:0"}
}

func expectedExecutionSequenceColumns() []string {
	return []string{"name:TEXT:1", "value:INTEGER:0"}
}

func (s *executionOrderStore) loadFromDB() error {
	if s == nil || s.persistence == nil {
		return nil
	}
	orders, err := s.persistence.loadOrders()
	if err != nil {
		return err
	}
	events, err := s.persistence.loadEvents()
	if err != nil {
		return err
	}
	fillKeys, err := s.persistence.loadSeenFillKeys()
	if err != nil {
		return err
	}
	sequences, err := s.persistence.loadSequences()
	if err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	for _, order := range orders {
		s.orders[order.InternalOrderID] = order
		s.linkBrokerOrderLocked(order)
		if seq := executionSequenceSuffix(order.InternalOrderID, "exec-"); seq > s.nextOrderSeq {
			s.nextOrderSeq = seq
		}
	}
	for _, event := range events {
		s.events[event.InternalOrderID] = append(s.events[event.InternalOrderID], event)
		if seq := executionSequenceSuffix(event.ID, "evt-"); seq > s.nextEventSeq {
			s.nextEventSeq = seq
		}
	}
	for _, row := range fillKeys {
		s.seenFillKeys[row.FillKey] = row.CreatedAt
	}
	if seq := sequences["orders"]; seq > s.nextOrderSeq {
		s.nextOrderSeq = seq
	}
	if seq := sequences["events"]; seq > s.nextEventSeq {
		s.nextEventSeq = seq
	}
	return nil
}

func (s *executionOrderSQLiteStore) loadOrders() ([]executionOrderSummaryResponse, error) {
	rows := []executionOrderSummaryRow{}
	if err := s.db.Select(&rows,
		`SELECT internal_order_id, broker_id, broker_order_id, broker_order_id_ex, source, source_detail, trading_environment, account_id, market, symbol, side, order_type, status, requested_quantity, requested_price, filled_quantity, filled_average_price, remark, last_error, last_error_code, last_error_source, submitted_at, updated_at, created_at FROM `+
			executionOrderTable); err != nil {
		return nil, err
	}
	orders := make([]executionOrderSummaryResponse, 0, len(rows))
	for _, row := range rows {
		orders = append(orders, executionOrderSummaryFromRow(row))
	}
	return orders, nil
}

func (s *executionOrderSQLiteStore) loadEvents() ([]executionOrderEventResponse, error) {
	rows := []executionOrderEventRow{}
	if err := s.db.Select(&rows,
		`SELECT id, internal_order_id, event_type, previous_status, next_status, payload_json, created_at FROM `+
			executionOrderEventTable+` ORDER BY created_at ASC, id ASC`); err != nil {
		return nil, err
	}
	events := make([]executionOrderEventResponse, 0, len(rows))
	for _, row := range rows {
		events = append(events, executionOrderEventFromRow(row))
	}
	return events, nil
}

func (s *executionOrderSQLiteStore) loadSeenFillKeys() ([]executionSeenFillRow, error) {
	keys := []executionSeenFillRow{}
	if err := s.db.Select(&keys, `SELECT fill_key, created_at FROM `+executionSeenFillTable); err != nil {
		return nil, err
	}
	return keys, nil
}

func (s *executionOrderSQLiteStore) loadSequences() (map[string]uint64, error) {
	rows := []executionSequenceRow{}
	if err := s.db.Select(&rows, `SELECT name, value FROM `+executionSequenceTable); err != nil {
		return nil, err
	}
	result := make(map[string]uint64, len(rows))
	for _, row := range rows {
		result[row.Name] = row.Value
	}
	return result, nil
}

func (s *executionOrderSQLiteStore) persistOrder(order executionOrderSummaryResponse) error {
	row := executionOrderSummaryToRow(order)
	_, err := s.db.NamedExec(
		`INSERT INTO `+executionOrderTable+` (internal_order_id, broker_id, broker_order_id, broker_order_id_ex, source, source_detail, trading_environment, account_id, market, symbol, side, order_type, status, requested_quantity, requested_price, filled_quantity, filled_average_price, remark, last_error, last_error_code, last_error_source, submitted_at, updated_at, created_at) `+
			`VALUES (:internal_order_id, :broker_id, :broker_order_id, :broker_order_id_ex, :source, :source_detail, :trading_environment, :account_id, :market, :symbol, :side, :order_type, :status, :requested_quantity, :requested_price, :filled_quantity, :filled_average_price, :remark, :last_error, :last_error_code, :last_error_source, :submitted_at, :updated_at, :created_at) `+
			`ON CONFLICT(internal_order_id) DO UPDATE SET broker_id = excluded.broker_id, broker_order_id = excluded.broker_order_id, broker_order_id_ex = excluded.broker_order_id_ex, source = excluded.source, source_detail = excluded.source_detail, trading_environment = excluded.trading_environment, account_id = excluded.account_id, market = excluded.market, symbol = excluded.symbol, side = excluded.side, order_type = excluded.order_type, status = excluded.status, requested_quantity = excluded.requested_quantity, requested_price = excluded.requested_price, filled_quantity = excluded.filled_quantity, filled_average_price = excluded.filled_average_price, remark = excluded.remark, last_error = excluded.last_error, last_error_code = excluded.last_error_code, last_error_source = excluded.last_error_source, submitted_at = excluded.submitted_at, updated_at = excluded.updated_at, created_at = excluded.created_at`,
		row,
	)
	return err
}

func (s *executionOrderSQLiteStore) persistEvent(event executionOrderEventResponse) error {
	row := executionOrderEventToRow(event)
	_, err := s.db.NamedExec(
		`INSERT INTO `+executionOrderEventTable+` (id, internal_order_id, event_type, previous_status, next_status, payload_json, created_at) `+
			`VALUES (:id, :internal_order_id, :event_type, :previous_status, :next_status, :payload_json, :created_at) `+
			`ON CONFLICT(id) DO UPDATE SET internal_order_id = excluded.internal_order_id, event_type = excluded.event_type, previous_status = excluded.previous_status, next_status = excluded.next_status, payload_json = excluded.payload_json, created_at = excluded.created_at`,
		row,
	)
	return err
}

func (s *executionOrderSQLiteStore) persistSeenFillKey(fillKey string, createdAt string) error {
	if strings.TrimSpace(fillKey) == "" {
		return nil
	}
	_, err := s.db.ExecContext(context.Background(), `INSERT OR IGNORE INTO `+executionSeenFillTable+` (fill_key, created_at) VALUES (?, ?)`, fillKey, createdAt)
	return err
}

func (s *executionOrderSQLiteStore) persistSequence(name string, value uint64) error {
	if strings.TrimSpace(name) == "" {
		return nil
	}
	_, err := s.db.ExecContext(context.Background(),
		`INSERT INTO `+executionSequenceTable+` (name, value) VALUES (?, ?) `+
			`ON CONFLICT(name) DO UPDATE SET value = excluded.value`,
		name,
		value,
	)
	return err
}

func (s *executionOrderSQLiteStore) deleteSeenFillKeysBefore(cutoff time.Time) error {
	if cutoff.IsZero() {
		return nil
	}
	_, err := s.db.ExecContext(context.Background(), `DELETE FROM `+executionSeenFillTable+` WHERE created_at < ?`, cutoff.UTC().Format(time.RFC3339Nano))
	return err
}

func executionOrderSummaryFromRow(row executionOrderSummaryRow) executionOrderSummaryResponse {
	return executionOrderSummaryResponse{
		InternalOrderID:    row.InternalOrderID,
		BrokerID:           row.BrokerID,
		BrokerOrderID:      nullStringPointer(row.BrokerOrderID),
		BrokerOrderIDEx:    nullStringPointer(row.BrokerOrderIDEx),
		Source:             row.Source,
		SourceDetail:       row.SourceDetail,
		TradingEnvironment: row.TradingEnvironment,
		AccountID:          row.AccountID,
		Market:             row.Market,
		Symbol:             nullStringPointer(row.Symbol),
		Side:               nullStringPointer(row.Side),
		OrderType:          nullStringPointer(row.OrderType),
		Status:             row.Status,
		RequestedQuantity:  nullFloat64Pointer(row.RequestedQuantity),
		RequestedPrice:     nullFloat64Pointer(row.RequestedPrice),
		FilledQuantity:     nullFloat64Pointer(row.FilledQuantity),
		FilledAveragePrice: nullFloat64Pointer(row.FilledAveragePrice),
		Remark:             nullStringPointer(row.Remark),
		LastError:          nullStringPointer(row.LastError),
		LastErrorCode:      nullStringPointer(row.LastErrorCode),
		LastErrorSource:    nullStringPointer(row.LastErrorSource),
		SubmittedAt:        nullStringPointer(row.SubmittedAt),
		UpdatedAt:          row.UpdatedAt,
		CreatedAt:          row.CreatedAt,
	}
}

func executionOrderSummaryToRow(order executionOrderSummaryResponse) executionOrderSummaryRow {
	return executionOrderSummaryRow{
		InternalOrderID:    order.InternalOrderID,
		BrokerID:           order.BrokerID,
		BrokerOrderID:      stringPointerNull(order.BrokerOrderID),
		BrokerOrderIDEx:    stringPointerNull(order.BrokerOrderIDEx),
		Source:             order.Source,
		SourceDetail:       order.SourceDetail,
		TradingEnvironment: order.TradingEnvironment,
		AccountID:          order.AccountID,
		Market:             order.Market,
		Symbol:             stringPointerNull(order.Symbol),
		Side:               stringPointerNull(order.Side),
		OrderType:          stringPointerNull(order.OrderType),
		Status:             order.Status,
		RequestedQuantity:  float64PointerNull(order.RequestedQuantity),
		RequestedPrice:     float64PointerNull(order.RequestedPrice),
		FilledQuantity:     float64PointerNull(order.FilledQuantity),
		FilledAveragePrice: float64PointerNull(order.FilledAveragePrice),
		Remark:             stringPointerNull(order.Remark),
		LastError:          stringPointerNull(order.LastError),
		LastErrorCode:      stringPointerNull(order.LastErrorCode),
		LastErrorSource:    stringPointerNull(order.LastErrorSource),
		SubmittedAt:        stringPointerNull(order.SubmittedAt),
		UpdatedAt:          order.UpdatedAt,
		CreatedAt:          order.CreatedAt,
	}
}

func executionOrderEventFromRow(row executionOrderEventRow) executionOrderEventResponse {
	return executionOrderEventResponse{
		ID:              row.ID,
		InternalOrderID: row.InternalOrderID,
		EventType:       row.EventType,
		PreviousStatus:  nullStringPointer(row.PreviousStatus),
		NextStatus:      row.NextStatus,
		PayloadJSON:     row.PayloadJSON,
		CreatedAt:       row.CreatedAt,
	}
}

func executionOrderEventToRow(event executionOrderEventResponse) executionOrderEventRow {
	return executionOrderEventRow{
		ID:              event.ID,
		InternalOrderID: event.InternalOrderID,
		EventType:       event.EventType,
		PreviousStatus:  stringPointerNull(event.PreviousStatus),
		NextStatus:      event.NextStatus,
		PayloadJSON:     event.PayloadJSON,
		CreatedAt:       event.CreatedAt,
	}
}

func nullStringPointer(value sql.NullString) *string {
	if !value.Valid {
		return nil
	}
	return &value.String
}

func stringPointerNull(value *string) sql.NullString {
	if value == nil {
		return sql.NullString{}
	}
	return sql.NullString{String: *value, Valid: true}
}

func nullFloat64Pointer(value sql.NullFloat64) *float64 {
	if !value.Valid {
		return nil
	}
	return &value.Float64
}

func float64PointerNull(value *float64) sql.NullFloat64 {
	if value == nil {
		return sql.NullFloat64{}
	}
	return sql.NullFloat64{Float64: *value, Valid: true}
}

func executionSequenceSuffix(value string, prefix string) uint64 {
	trimmed := strings.TrimPrefix(strings.TrimSpace(value), prefix)
	if trimmed == value {
		return 0
	}
	var seq uint64
	if _, err := fmt.Sscanf(trimmed, "%d", &seq); err != nil {
		return 0
	}
	return seq
}
