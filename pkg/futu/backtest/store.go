// Package backtest provides a SQLite-backed K-line store for Futu historical
// data, compatible with bbgo's backtest engine via the service.BackTestable
// interface.
package backtest

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/jmoiron/sqlx"
	_ "modernc.org/sqlite"

	"github.com/c9s/bbgo/pkg/fixedpoint"
	"github.com/c9s/bbgo/pkg/types"
)

// KLineTable is the SQLite table name for Futu historical K-lines.
const KLineTable = "futu_klines"

// FutuKLineStore implements service.BackTestable for Futu data stored in
// SQLite. It uses a single futu_klines table compatible with bbgo's kline
// schema, extended with a market column for HK/US/etc. segmentation and a
// rehab_type column to store multiple price-adjustment modes side-by-side.
type FutuKLineStore struct {
	mu        sync.RWMutex
	db        *sqlx.DB
	rehabType string // "forward" | "backward" | "none" — filters all queries
}

// NewFutuKLineStore opens or creates a SQLite database at the given path and
// ensures the futu_klines table exists.
func NewFutuKLineStore(dbPath string) (*FutuKLineStore, error) {
	db, err := sqlx.Open("sqlite", dbPath+"?_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)")
	if err != nil {
		return nil, fmt.Errorf("open sqlite backtest store: %w", err)
	}
	store := &FutuKLineStore{db: db, rehabType: "forward"}
	if err := store.migrate(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("migrate sqlite backtest store: %w", err)
	}
	return store, nil
}

// Close shuts down the database connection.
func (s *FutuKLineStore) Close() error {
	return s.db.Close()
}

// SetRehabType configures the price-adjustment mode used for all subsequent
// queries.  Must be called before a backtest run.  Valid values:
// "forward" (前复权), "backward" (后复权), "none" (不复权).
func (s *FutuKLineStore) SetRehabType(rehabType string) {
	s.rehabType = rehabType
}

// DB returns the underlying sqlx.DB for advanced queries.
func (s *FutuKLineStore) DB() *sqlx.DB {
	return s.db
}

// RehabTypeName converts a qotcommonpb.RehabType enum to the store's string
// representation: "forward", "backward", or "none".
func RehabTypeName(rehabType int32) string {
	switch rehabType {
	case 1:
		return "forward"
	case 2:
		return "backward"
	default:
		return "none"
	}
}

func (s *FutuKLineStore) migrate() error {
	_, err := s.db.Exec(strings.Join([]string{
		`CREATE TABLE IF NOT EXISTS ` + KLineTable + ` (`,
		`  gid         INTEGER PRIMARY KEY AUTOINCREMENT,`,
		`  exchange    TEXT    NOT NULL DEFAULT 'futu',`,
		`  start_time  TEXT    NOT NULL,`,
		`  end_time    TEXT    NOT NULL,`,
		`  interval    TEXT    NOT NULL,`,
		`  symbol      TEXT    NOT NULL,`,
		`  market      TEXT    NOT NULL DEFAULT '',`,
		`  rehab_type  TEXT    NOT NULL DEFAULT 'forward',`,
		`  open        REAL    NOT NULL,`,
		`  high        REAL    NOT NULL,`,
		`  low         REAL    NOT NULL,`,
		`  close       REAL    NOT NULL DEFAULT 0.0,`,
		`  volume      REAL    NOT NULL DEFAULT 0.0,`,
		`  turnover    REAL    NOT NULL DEFAULT 0.0,`,
		`  closed      INTEGER NOT NULL DEFAULT 1,`,
		`  last_trade_id INTEGER NOT NULL DEFAULT 0,`,
		`  num_trades  INTEGER NOT NULL DEFAULT 0`,
		`)`,
	}, " "))
	if err != nil {
		return fmt.Errorf("create %s table: %w", KLineTable, err)
	}

	indexDDL := fmt.Sprintf(
		`CREATE INDEX IF NOT EXISTS %s_end_time_symbol_interval ON %s (end_time, symbol, interval)`,
		KLineTable, KLineTable,
	)
	_, err = s.db.Exec(indexDDL)
	if err != nil {
		return fmt.Errorf("create index on %s: %w", KLineTable, err)
	}

	uniqueDDL := fmt.Sprintf(
		`CREATE UNIQUE INDEX IF NOT EXISTS %s_symbol_interval_end_time_rehab ON %s (symbol, interval, end_time, rehab_type)`,
		KLineTable, KLineTable,
	)
	_, err = s.db.Exec(uniqueDDL)
	if err != nil {
		return fmt.Errorf("create unique index on %s: %w", KLineTable, err)
	}
	return nil
}

// --- service.BackTestable implementation ---

func (s *FutuKLineStore) Verify(
	sourceExchange types.Exchange, symbols []string, startTime time.Time, endTime time.Time,
) error {
	for _, symbol := range symbols {
		for interval := range types.SupportedIntervals {
			missing, err := s.findMissingRanges(symbol, interval, startTime, endTime)
			if err != nil {
				return err
			}
			if len(missing) > 0 {
				return fmt.Errorf("symbol %s interval %s has %d missing ranges from %s to %s",
					symbol, interval, len(missing), startTime, endTime)
			}
		}
	}
	return nil
}

func (s *FutuKLineStore) findMissingRanges(
	symbol string, interval types.Interval, startTime, endTime time.Time,
) ([]string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	rows, err := s.db.Query(
		`SELECT start_time, end_time FROM `+KLineTable+` WHERE symbol = ? AND interval = ? AND rehab_type = ? AND end_time >= ? AND end_time <= ? ORDER BY end_time ASC`,
		symbol, string(interval), s.rehabType, startTime.Format(time.RFC3339Nano), endTime.Format(time.RFC3339Nano),
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var missing []string
	expected := startTime
	step := interval.Duration()
	for rows.Next() {
		var st, et string
		if err := rows.Scan(&st, &et); err != nil {
			return nil, err
		}
		endT, err := time.Parse(time.RFC3339Nano, et)
		if err != nil {
			continue
		}
		if endT.After(expected) {
			missing = append(missing, fmt.Sprintf("%s – %s (missing %v)",
				expected.Format(time.RFC3339), endT.Format(time.RFC3339),
				endT.Sub(expected)))
		}
		expected = endT.Add(step)
	}

	if expected.Before(endTime) {
		missing = append(missing, fmt.Sprintf("%s – %s (missing %v)",
			expected.Format(time.RFC3339), endTime.Format(time.RFC3339),
			endTime.Sub(expected)))
	}
	return missing, rows.Err()
}

func (s *FutuKLineStore) Sync(
	ctx context.Context, ex types.Exchange, symbol string,
	intervals []types.Interval, since, until time.Time,
) error {
	// The sync logic is external (pkg/futu/backtest/sync.go).
	// This method is a no-op for the BackTestable interface; actual sync
	// is driven by the dedicated sync command.
	return nil
}

func (s *FutuKLineStore) QueryKLine(
	ex types.Exchange, symbol string, interval types.Interval,
	orderBy string, limit int,
) (*types.KLine, error) {
	kline, err := s.queryKLine(symbol, interval, orderBy, limit)
	if err != nil {
		return nil, err
	}
	return kline, nil
}

func (s *FutuKLineStore) queryKLine(
	symbol string, interval types.Interval, orderBy string, limit int,
) (*types.KLine, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	normalizedOrder := strings.ToUpper(strings.TrimSpace(orderBy))
	if normalizedOrder != "ASC" && normalizedOrder != "DESC" {
		normalizedOrder = "DESC"
	}

	query := fmt.Sprintf(
		`SELECT start_time, end_time, interval, symbol, open, high, low, close, volume, closed, last_trade_id, num_trades FROM %s WHERE symbol = ? AND interval = ? AND rehab_type = ? ORDER BY end_time %s LIMIT ?`,
		KLineTable, normalizedOrder,
	)

	row := s.db.QueryRow(query, symbol, string(interval), s.rehabType, limit)
	return scanKLine(row)
}

// queryLatestKLineInRange returns the most recent K-line for the given
// symbol+interval whose end_time falls within [since, until].
func (s *FutuKLineStore) queryLatestKLineInRange(
	symbol string, interval types.Interval,
	since, until time.Time,
) (*types.KLine, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	query := fmt.Sprintf(
		`SELECT start_time, end_time, interval, symbol, open, high, low, close, volume, closed, last_trade_id, num_trades FROM %s WHERE symbol = ? AND interval = ? AND rehab_type = ? AND end_time >= ? AND end_time <= ? ORDER BY end_time DESC LIMIT 1`,
		KLineTable,
	)

	row := s.db.QueryRow(
		query,
		symbol,
		string(interval),
		s.rehabType,
		since.UTC().Format(time.RFC3339Nano),
		until.UTC().Format(time.RFC3339Nano),
	)
	return scanKLine(row)
}

// isBatchCovered returns true when the local store already contains enough
// K-lines to cover [cursor, batchEnd] for the given symbol+interval.  It
// queries the latest end_time in the batch window; if that end_time reaches
// batchEnd (within one bar of tolerance) the batch is considered covered.
// This allows syncInterval to skip batches that were already fetched in a
// previous sync run — without being fooled by data that sits entirely
// outside the batch.
func (s *FutuKLineStore) isBatchCovered(
	symbol string, interval types.Interval,
	cursor, batchEnd time.Time,
	rehabType string,
) (bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	query := fmt.Sprintf(
		`SELECT end_time FROM %s WHERE symbol = ? AND interval = ? AND rehab_type = ? AND end_time >= ? AND end_time <= ? ORDER BY end_time DESC LIMIT 1`,
		KLineTable,
	)

	var endTimeStr string
	err := s.db.QueryRow(
		query,
		symbol,
		string(interval),
		rehabType,
		cursor.UTC().Format(time.RFC3339Nano),
		batchEnd.UTC().Format(time.RFC3339Nano),
	).Scan(&endTimeStr)
	if err != nil {
		// sql.ErrNoRows means no data in this batch → not covered
		return false, nil
	}

	endTime, err := time.Parse(time.RFC3339Nano, endTimeStr)
	if err != nil {
		return false, nil
	}

	// Allow one bar of tolerance for market close / irregular gaps.
	return !endTime.Add(interval.Duration()).Before(batchEnd), nil
}

func (s *FutuKLineStore) QueryKLinesForward(
	exchange types.Exchange, symbol string, interval types.Interval,
	startTime time.Time, limit int,
) ([]types.KLine, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	query := fmt.Sprintf(
		`SELECT start_time, end_time, interval, symbol, open, high, low, close, volume, closed, last_trade_id, num_trades FROM %s WHERE symbol = ? AND interval = ? AND rehab_type = ? AND end_time >= ? ORDER BY end_time ASC LIMIT ?`,
		KLineTable,
	)
	rows, err := s.db.Query(query, symbol, string(interval), s.rehabType, startTime.Format(time.RFC3339Nano), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanKLines(rows)
}

func (s *FutuKLineStore) QueryKLinesBackward(
	exchange types.Exchange, symbol string, interval types.Interval,
	endTime time.Time, limit int,
) ([]types.KLine, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	query := fmt.Sprintf(
		`SELECT start_time, end_time, interval, symbol, open, high, low, close, volume, closed, last_trade_id, num_trades FROM %s WHERE symbol = ? AND interval = ? AND rehab_type = ? AND end_time <= ? ORDER BY end_time DESC LIMIT ?`,
		KLineTable,
	)
	rows, err := s.db.Query(query, symbol, string(interval), s.rehabType, endTime.Format(time.RFC3339Nano), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	klines, err := scanKLines(rows)
	if err != nil {
		return nil, err
	}
	// Reverse to ascending order (bbgo expects ascending).
	reverseKLines(klines)
	return klines, nil
}

func (s *FutuKLineStore) QueryKLinesCh(
	since, until time.Time, exchange types.Exchange,
	symbols []string, intervals []types.Interval,
) (chan types.KLine, chan error) {
	ch := make(chan types.KLine, 1000)
	errCh := make(chan error, 1)

	go func() {
		defer close(ch)
		defer close(errCh)

		s.mu.RLock()
		defer s.mu.RUnlock()

		if len(symbols) == 0 || len(intervals) == 0 {
			return
		}

		// Build a single UNION ALL query so klines from all intervals are
		// returned interleaved by end_time ASC.  This is required by bbgo's
		// ConsumeKLine pump: 1m (requiredInterval) klines must arrive
		// between higher-interval klines so the cache never accumulates
		// two bars of the same non-1m interval (which would panic).
		placeholders := make([]string, 0, len(symbols)*len(intervals))
		args := make([]interface{}, 0, len(symbols)*len(intervals)*4)

		for _, symbol := range symbols {
			for _, interval := range intervals {
				placeholders = append(placeholders, "(? , ?)")
				args = append(args, symbol, string(interval))
			}
		}

		args = append(args, s.rehabType, since.Format(time.RFC3339Nano), until.Format(time.RFC3339Nano))

		query := fmt.Sprintf(
			`SELECT start_time, end_time, interval, symbol, open, high, low, close, volume, closed, last_trade_id, num_trades FROM %s WHERE (symbol, interval) IN (%s) AND rehab_type = ? AND end_time >= ? AND end_time <= ? ORDER BY end_time ASC`,
			KLineTable,
			strings.Join(placeholders, ", "),
		)

		rows, err := s.db.Query(query, args...)
		if err != nil {
			errCh <- err
			return
		}
		defer rows.Close()

		klines, err := scanKLines(rows)
		if err != nil {
			errCh <- err
			return
		}
		for _, k := range klines {
			ch <- k
		}
	}()

	return ch, errCh
}

// --- Batch Insert ---

// InsertKLine inserts a single K-line into the store. Duplicates (same
// symbol+interval+end_time+rehab_type) are replaced.
func (s *FutuKLineStore) InsertKLine(kline types.KLine, market string, rehabType string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, err := s.db.Exec(
		`INSERT OR REPLACE INTO `+KLineTable+` (exchange, start_time, end_time, interval, symbol, market, rehab_type, open, high, low, close, volume, closed, last_trade_id, num_trades) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		"futu",
		kline.StartTime.Time().UTC().Format(time.RFC3339Nano),
		kline.EndTime.Time().UTC().Format(time.RFC3339Nano),
		string(kline.Interval),
		kline.Symbol,
		market,
		rehabType,
		kline.Open.Float64(),
		kline.High.Float64(),
		kline.Low.Float64(),
		kline.Close.Float64(),
		kline.Volume.Float64(),
		boolToInt(kline.Closed),
		0, 0,
	)
	return err
}

// InsertKLines batch-inserts K-lines into the store.
func (s *FutuKLineStore) InsertKLines(klines []types.KLine, market string, rehabType string) error {
	for _, k := range klines {
		if err := s.InsertKLine(k, market, rehabType); err != nil {
			return err
		}
	}
	return nil
}

// --- Helpers ---

func scanKLine(row *sql.Row) (*types.KLine, error) {
	var startTime, endTime, interval, symbol string
	var open, high, low, close, volume float64
	var closed, lastTradeID, numTrades int

	if err := row.Scan(&startTime, &endTime, &interval, &symbol,
		&open, &high, &low, &close, &volume, &closed, &lastTradeID, &numTrades,
	); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}

	st, err := time.Parse(time.RFC3339Nano, startTime)
	if err != nil {
		return nil, fmt.Errorf("parse start_time: %w", err)
	}
	et, err := time.Parse(time.RFC3339Nano, endTime)
	if err != nil {
		return nil, fmt.Errorf("parse end_time: %w", err)
	}

	return &types.KLine{
		StartTime:      types.Time(st),
		EndTime:        types.Time(et),
		Interval:       types.Interval(interval),
		Symbol:         symbol,
		Open:           floatToFixed(open),
		High:           floatToFixed(high),
		Low:            floatToFixed(low),
		Close:          floatToFixed(close),
		Volume:         floatToFixed(volume),
		Closed:         closed != 0,
		LastTradeID:    uint64(lastTradeID),
		NumberOfTrades: uint64(numTrades),
	}, nil
}

func scanKLines(rows *sql.Rows) ([]types.KLine, error) {
	var klines []types.KLine
	for rows.Next() {
		var startTime, endTime, interval, symbol string
		var open, high, low, close, volume float64
		var closed, lastTradeID, numTrades int

		if err := rows.Scan(&startTime, &endTime, &interval, &symbol,
			&open, &high, &low, &close, &volume, &closed, &lastTradeID, &numTrades,
		); err != nil {
			return nil, err
		}

		st, err := time.Parse(time.RFC3339Nano, startTime)
		if err != nil {
			return nil, fmt.Errorf("parse start_time: %w", err)
		}
		et, err := time.Parse(time.RFC3339Nano, endTime)
		if err != nil {
			return nil, fmt.Errorf("parse end_time: %w", err)
		}

		klines = append(klines, types.KLine{
			StartTime:      types.Time(st),
			EndTime:        types.Time(et),
			Interval:       types.Interval(interval),
			Symbol:         symbol,
			Open:           floatToFixed(open),
			High:           floatToFixed(high),
			Low:            floatToFixed(low),
			Close:          floatToFixed(close),
			Volume:         floatToFixed(volume),
			Closed:         closed != 0,
			LastTradeID:    uint64(lastTradeID),
			NumberOfTrades: uint64(numTrades),
		})
	}
	return klines, rows.Err()
}

func reverseKLines(klines []types.KLine) {
	for i, j := 0, len(klines)-1; i < j; i, j = i+1, j-1 {
		klines[i], klines[j] = klines[j], klines[i]
	}
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func floatToFixed(f float64) fixedpoint.Value {
	return fixedpoint.NewFromFloat(f)
}
