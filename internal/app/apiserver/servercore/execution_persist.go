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
	db   *sqliteconn.DB
	path string
}

type executionMigrationTx interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
	Commit() error
	Rollback() error
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
	if err := sqliteschema.ValidateCurrentFile(context.Background(), dbPath, sqliteschema.DatabaseExecution); err != nil {
		return nil, fmt.Errorf("validate execution order sqlite store: %w", err)
	}
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
	if info, statErr := stat(trimmedPath); statErr == nil {
		if !info.Mode().IsRegular() {
			return nil, fmt.Errorf("inspect execution order sqlite store: database path is not a regular file")
		}
	} else if !errors.Is(statErr, os.ErrNotExist) {
		return nil, fmt.Errorf("inspect execution order sqlite store: %w", statErr)
	}

	db, err := open(trimmedPath)
	if err != nil {
		return nil, fmt.Errorf("open execution order sqlite store: %w", err)
	}
	store := &executionOrderSQLiteStore{db: db, path: trimmedPath}
	if err := store.initializeOrValidateSchema(); err != nil {
		jftradeErr1 := db.Close()
		besteffort.LogError(jftradeErr1)
		return nil, fmt.Errorf("initialize or validate execution order sqlite store: %w", err)
	}
	return store, nil
}

func (s *executionOrderSQLiteStore) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

func (s *executionOrderSQLiteStore) initializeOrValidateSchema() error {
	return sqliteschema.InitializeCurrent(context.Background(), s.db, s.path, sqliteschema.DatabaseExecution)
}
