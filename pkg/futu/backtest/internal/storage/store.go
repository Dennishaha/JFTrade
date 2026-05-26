// Package storage contains the SQLite-backed Futu backtest store
// implementation and compact futu_klines schema helpers.
package storage

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/jmoiron/sqlx"
	_ "modernc.org/sqlite"

	"github.com/c9s/bbgo/pkg/types"
)

// FutuKLineStore implements service.BackTestable for Futu data stored in
// SQLite. It uses a compact backtest-only futu_klines table keyed by
// symbol+interval+rehab_type+end_time.
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
	store := &FutuKLineStore{db: db, rehabType: normalizeRehabTypeName("forward")}
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
	s.rehabType = normalizeRehabTypeName(rehabType)
}

// DB returns the underlying sqlx.DB for advanced queries.
func (s *FutuKLineStore) DB() *sqlx.DB {
	return s.db
}

func (s *FutuKLineStore) migrate() error {
	_, err := s.db.Exec(strings.Join([]string{
		`CREATE TABLE IF NOT EXISTS ` + KLineTable + ` (`,
		`  symbol      TEXT    NOT NULL,`,
		`  interval    INTEGER NOT NULL,`,
		`  rehab_type  INTEGER NOT NULL,`,
		`  end_time    INTEGER NOT NULL,`,
		`  start_time  INTEGER NOT NULL,`,
		`  open        REAL    NOT NULL,`,
		`  high        REAL    NOT NULL,`,
		`  low         REAL    NOT NULL,`,
		`  close       REAL    NOT NULL,`,
		`  volume      REAL    NOT NULL,`,
		`  PRIMARY KEY (symbol, interval, rehab_type, end_time)`,
		`) WITHOUT ROWID`,
	}, " "))
	if err != nil {
		return fmt.Errorf("create %s table: %w", KLineTable, err)
	}
	if err := s.ensureCompactSchema(); err != nil {
		return err
	}
	return nil
}

func (s *FutuKLineStore) ensureCompactSchema() error {
	rows, err := s.db.Query(`PRAGMA table_info(` + KLineTable + `)`)
	if err != nil {
		return fmt.Errorf("inspect %s schema: %w", KLineTable, err)
	}
	defer rows.Close()

	got := make([]string, 0, len(expectedKLineSchemaColumns()))
	for rows.Next() {
		var cid, notNull, pk int
		var name, dataType string
		var defaultValue sql.NullString
		if err := rows.Scan(&cid, &name, &dataType, &notNull, &defaultValue, &pk); err != nil {
			return fmt.Errorf("scan %s schema: %w", KLineTable, err)
		}
		got = append(got, fmt.Sprintf("%s:%s:%d", name, strings.ToUpper(dataType), pk))
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate %s schema: %w", KLineTable, err)
	}

	want := expectedKLineSchemaColumns()
	if len(got) != len(want) {
		return fmt.Errorf("%s schema is obsolete; rebuild the backtest database", KLineTable)
	}
	for index := range want {
		if got[index] != want[index] {
			return fmt.Errorf("%s schema is obsolete; rebuild the backtest database", KLineTable)
		}
	}

	var ddl string
	if err := s.db.QueryRow(`SELECT sql FROM sqlite_master WHERE type = 'table' AND name = ?`, KLineTable).Scan(&ddl); err != nil {
		return fmt.Errorf("load %s ddl: %w", KLineTable, err)
	}
	if !strings.Contains(strings.ToUpper(ddl), "WITHOUT ROWID") {
		return fmt.Errorf("%s schema is obsolete; rebuild the backtest database", KLineTable)
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
		`SELECT start_time, end_time FROM `+KLineTable+` WHERE symbol = ? AND interval = ? AND rehab_type = ? AND end_time >= ? AND start_time <= ? ORDER BY start_time ASC`,
		symbol, intervalStorageValue(interval), rehabTypeCode(s.rehabType), timeToUnixMillis(startTime), timeToUnixMillis(endTime),
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var missing []string
	expected := startTime.UTC()
	windowEnd := endTime.UTC()
	for rows.Next() {
		var startTimeMillis, endTimeMillis int64
		if err := rows.Scan(&startTimeMillis, &endTimeMillis); err != nil {
			return nil, err
		}
		startT := timeFromUnixMillis(startTimeMillis)
		endT := timeFromUnixMillis(endTimeMillis)
		if endT.Before(expected) {
			continue
		}

		coverageStart := startT
		if coverageStart.Before(expected) {
			coverageStart = expected
		}
		if coverageStart.After(windowEnd) {
			break
		}

		if coverageStart.After(expected) {
			missing = append(missing, fmt.Sprintf("%s – %s (missing %v)",
				expected.Format(time.RFC3339), coverageStart.Format(time.RFC3339),
				coverageStart.Sub(expected)))
		}

		coverageEnd := endT
		if coverageEnd.After(windowEnd) {
			coverageEnd = windowEnd
		}
		expected = coverageEnd.Add(time.Millisecond)
		if expected.After(windowEnd) {
			break
		}
	}

	if !expected.After(windowEnd) {
		missing = append(missing, fmt.Sprintf("%s – %s (missing %v)",
			expected.Format(time.RFC3339), windowEnd.Format(time.RFC3339),
			windowEnd.Sub(expected)))
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
		`SELECT %s FROM %s WHERE symbol = ? AND interval = ? AND rehab_type = ? ORDER BY end_time %s LIMIT ?`,
		selectKLineColumns,
		KLineTable, normalizedOrder,
	)

	row := s.db.QueryRow(query, symbol, intervalStorageValue(interval), rehabTypeCode(s.rehabType), limit)
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
		`SELECT %s FROM %s WHERE symbol = ? AND interval = ? AND rehab_type = ? AND end_time >= ? AND end_time <= ? ORDER BY end_time DESC LIMIT 1`,
		selectKLineColumns,
		KLineTable,
	)

	row := s.db.QueryRow(
		query,
		symbol,
		intervalStorageValue(interval),
		rehabTypeCode(s.rehabType),
		timeToUnixMillis(since),
		timeToUnixMillis(until),
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

	var endTimeMillis int64
	err := s.db.QueryRow(
		query,
		symbol,
		intervalStorageValue(interval),
		rehabTypeCode(rehabType),
		timeToUnixMillis(cursor),
		timeToUnixMillis(batchEnd),
	).Scan(&endTimeMillis)
	if err != nil {
		// sql.ErrNoRows means no data in this batch → not covered
		return false, nil
	}

	endTime := timeFromUnixMillis(endTimeMillis)

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
		`SELECT %s FROM %s WHERE symbol = ? AND interval = ? AND rehab_type = ? AND end_time >= ? ORDER BY end_time ASC LIMIT ?`,
		selectKLineColumns,
		KLineTable,
	)
	rows, err := s.db.Query(query, symbol, intervalStorageValue(interval), rehabTypeCode(s.rehabType), timeToUnixMillis(startTime), limit)
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
		`SELECT %s FROM %s WHERE symbol = ? AND interval = ? AND rehab_type = ? AND end_time <= ? ORDER BY end_time DESC LIMIT ?`,
		selectKLineColumns,
		KLineTable,
	)
	rows, err := s.db.Query(query, symbol, intervalStorageValue(interval), rehabTypeCode(s.rehabType), timeToUnixMillis(endTime), limit)
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
				args = append(args, symbol, intervalStorageValue(interval))
			}
		}

		args = append(args, rehabTypeCode(s.rehabType), timeToUnixMillis(since), timeToUnixMillis(until))

		query := fmt.Sprintf(
			`SELECT %s FROM %s WHERE (symbol, interval) IN (%s) AND rehab_type = ? AND end_time >= ? AND end_time <= ? ORDER BY end_time ASC`,
			selectKLineColumns,
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
func (s *FutuKLineStore) InsertKLine(kline types.KLine, rehabType string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	intervalValue := intervalStorageValue(kline.Interval)
	rehabValue := rehabTypeCode(rehabType)

	_, err := s.db.Exec(
		`INSERT INTO `+KLineTable+` (symbol, interval, rehab_type, end_time, start_time, open, high, low, close, volume) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?) `+
			`ON CONFLICT(symbol, interval, rehab_type, end_time) DO UPDATE SET `+
			`start_time = excluded.start_time, open = excluded.open, high = excluded.high, low = excluded.low, close = excluded.close, volume = excluded.volume`,
		kline.Symbol,
		intervalValue,
		rehabValue,
		timeToUnixMillis(kline.EndTime.Time()),
		timeToUnixMillis(kline.StartTime.Time()),
		kline.Open.Float64(),
		kline.High.Float64(),
		kline.Low.Float64(),
		kline.Close.Float64(),
		kline.Volume.Float64(),
	)
	return err
}

// InsertKLines batch-inserts K-lines into the store.
func (s *FutuKLineStore) InsertKLines(klines []types.KLine, rehabType string) error {
	for _, k := range klines {
		if err := s.InsertKLine(k, rehabType); err != nil {
			return err
		}
	}
	return nil
}

// --- Helpers ---
