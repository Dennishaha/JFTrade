// Package storage contains the SQLite-backed Futu backtest store
// implementation and compact local_klines schema helpers.
package storage

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/jmoiron/sqlx"
	_ "modernc.org/sqlite"

	"github.com/c9s/bbgo/pkg/types"
)

// FutuKLineStore implements service.BackTestable for Futu data stored in
// SQLite. It uses a compact backtest-only local_klines table keyed by
// symbol+interval+rehab_type (table-level) and end_time (row-level).
type FutuKLineStore struct {
	mu        sync.RWMutex
	db        *sqlx.DB
	rehabType string // "forward" | "backward" | "none" — filters all queries
}

// NewFutuKLineStore opens or creates a SQLite database at the given path and
// lazily creates per-series tables as data is inserted.
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
	return nil
}

func (s *FutuKLineStore) ensureKLineTable(tableName string) error {
	_, err := s.db.Exec(strings.Join([]string{
		`CREATE TABLE IF NOT EXISTS ` + quoteIdentifier(tableName) + ` (`,
		`  end_time    INTEGER NOT NULL,`,
		`  start_time  INTEGER NOT NULL,`,
		`  open        REAL    NOT NULL,`,
		`  high        REAL    NOT NULL,`,
		`  low         REAL    NOT NULL,`,
		`  close       REAL    NOT NULL,`,
		`  volume      REAL    NOT NULL,`,
		`  PRIMARY KEY (end_time)`,
		`) WITHOUT ROWID`,
	}, " "))
	if err != nil {
		return fmt.Errorf("create %s table: %w", tableName, err)
	}
	return s.ensureCompactSchema(tableName)
}

func (s *FutuKLineStore) ensureCompactSchema(tableName string) error {
	rows, err := s.db.Query(`PRAGMA table_info(` + quoteIdentifier(tableName) + `)`)
	if err != nil {
		return fmt.Errorf("inspect %s schema: %w", tableName, err)
	}
	defer rows.Close()

	got := make([]string, 0, len(expectedKLineSchemaColumns()))
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

	want := expectedKLineSchemaColumns()
	if len(got) != len(want) {
		return fmt.Errorf("%s schema is obsolete; rebuild the backtest database", tableName)
	}
	for index := range want {
		if got[index] != want[index] {
			return fmt.Errorf("%s schema is obsolete; rebuild the backtest database", tableName)
		}
	}

	var ddl string
	if err := s.db.QueryRow(`SELECT sql FROM sqlite_master WHERE type = 'table' AND name = ?`, tableName).Scan(&ddl); err != nil {
		return fmt.Errorf("load %s ddl: %w", tableName, err)
	}
	if !strings.Contains(strings.ToUpper(ddl), "WITHOUT ROWID") {
		return fmt.Errorf("%s schema is obsolete; rebuild the backtest database", tableName)
	}

	return nil
}

func (s *FutuKLineStore) klineTableExists(tableName string) (bool, error) {
	var count int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = ?`, tableName).Scan(&count); err != nil {
		return false, err
	}
	return count > 0, nil
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

	directMissing, err := s.findMissingRangesInTable(symbol, interval, s.rehabType, startTime, endTime)
	if err != nil {
		return nil, err
	}
	if len(directMissing) == 0 {
		return nil, nil
	}
	if !canAggregateFromLowerInterval(interval) {
		return directMissing, nil
	}

	alignedStart := alignTimeToIntervalStart(startTime, interval)
	var preferredMissing []string
	for index, baseInterval := range aggregationBaseIntervals(interval) {
		baseMissing, err := s.findMissingRangesInTable(symbol, baseInterval, s.rehabType, alignedStart, endTime)
		if err != nil {
			return nil, err
		}
		if len(baseMissing) == 0 {
			return nil, nil
		}
		if index == 0 {
			preferredMissing = baseMissing
		}
	}
	if len(preferredMissing) > 0 {
		return preferredMissing, nil
	}
	return directMissing, nil
}

func (s *FutuKLineStore) findMissingRangesInTable(
	symbol string, interval types.Interval, rehabType string, startTime, endTime time.Time,
) ([]string, error) {
	tableName := klineTableName(symbol, interval, rehabType)
	exists, err := s.klineTableExists(tableName)
	if err != nil {
		return nil, err
	}
	if !exists {
		return fullWindowMissingRange(startTime, endTime), nil
	}

	leftBoundary := firstClosedKLineEndAtOrAfter(startTime, interval)
	rightBoundary := latestClosedKLineEndAtOrBefore(endTime, interval)
	if rightBoundary.Before(leftBoundary) {
		return nil, nil
	}

	leftCovered, err := s.hasKLineEndingAtOrAfter(tableName, leftBoundary)
	if err != nil {
		return nil, err
	}
	rightCovered, err := s.hasKLineEndingAtOrBefore(tableName, rightBoundary)
	if err != nil {
		return nil, err
	}

	if !leftCovered || !rightCovered {
		return fullWindowMissingRange(startTime, endTime), nil
	}

	return nil, nil
}

func (s *FutuKLineStore) hasKLineEndingAtOrAfter(tableName string, at time.Time) (bool, error) {
	var endTimeMillis int64
	err := s.db.QueryRow(
		`SELECT end_time FROM `+quoteIdentifier(tableName)+` WHERE end_time >= ? ORDER BY end_time ASC LIMIT 1`,
		timeToUnixMillis(at),
	).Scan(&endTimeMillis)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

func (s *FutuKLineStore) hasKLineEndingAtOrBefore(tableName string, at time.Time) (bool, error) {
	var endTimeMillis int64
	err := s.db.QueryRow(
		`SELECT end_time FROM `+quoteIdentifier(tableName)+` WHERE end_time <= ? ORDER BY end_time DESC LIMIT 1`,
		timeToUnixMillis(at),
	).Scan(&endTimeMillis)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

func fullWindowMissingRange(startTime, endTime time.Time) []string {
	return []string{fmt.Sprintf("%s – %s (missing %v)",
		startTime.UTC().Format(time.RFC3339),
		endTime.UTC().Format(time.RFC3339),
		endTime.UTC().Sub(startTime.UTC()),
	)}
}

type klineReadSource struct {
	synthesize   bool
	baseInterval types.Interval
}

func (s *FutuKLineStore) EnsureCoverage(symbol string, interval types.Interval, since, until time.Time) error {
	s.mu.RLock()
	defer s.mu.RUnlock()
	_, err := s.resolveReadSource(symbol, interval, since, until)
	return err
}

func (s *FutuKLineStore) resolveReadSource(symbol string, interval types.Interval, since, until time.Time) (klineReadSource, error) {
	directMissing, err := s.findMissingRangesInTable(symbol, interval, s.rehabType, since, until)
	if err != nil {
		return klineReadSource{}, err
	}
	if len(directMissing) == 0 {
		return klineReadSource{}, nil
	}
	candidates := aggregationBaseIntervals(interval)
	if len(candidates) > 0 {
		alignedStart := alignTimeToIntervalStart(since, interval)
		for _, baseInterval := range candidates {
			baseMissing, baseErr := s.findMissingRangesInTable(symbol, baseInterval, s.rehabType, alignedStart, until)
			if baseErr != nil {
				return klineReadSource{}, baseErr
			}
			if len(baseMissing) == 0 {
				return klineReadSource{synthesize: true, baseInterval: baseInterval}, nil
			}
		}
		preferred := candidates[0]
		return klineReadSource{}, fmt.Errorf("missing K-line coverage for %s %s [%s, %s]; download %s data covering the full range", symbol, interval, since.UTC().Format(time.RFC3339), until.UTC().Format(time.RFC3339), preferred)
	}
	return klineReadSource{}, fmt.Errorf("missing K-line coverage for %s %s [%s, %s]; download %s data covering the full range", symbol, interval, since.UTC().Format(time.RFC3339), until.UTC().Format(time.RFC3339), interval)
}

func (s *FutuKLineStore) Sync(
	ctx context.Context, ex types.Exchange, symbol string,
	intervals []types.Interval, since, until time.Time,
) error {
	// The sync logic is external (pkg/backtest/sync.go).
	// This method is a no-op for the BackTestable interface; actual sync
	// is driven by the dedicated sync command.
	return nil
}

func (s *FutuKLineStore) QueryKLine(
	ex types.Exchange, symbol string, interval types.Interval,
	orderBy string, limit int,
) (*types.KLine, error) {
	if limit <= 0 {
		limit = 1
	}
	normalizedOrder := strings.ToUpper(strings.TrimSpace(orderBy))
	kline, err := s.queryKLine(symbol, interval, normalizedOrder, limit)
	if err != nil || kline != nil {
		return kline, err
	}
	if normalizedOrder == "ASC" {
		return nil, nil
	}
	return nil, nil
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
	tableName := klineTableName(symbol, interval, s.rehabType)
	exists, err := s.klineTableExists(tableName)
	if err != nil || !exists {
		return nil, err
	}

	query := fmt.Sprintf(
		`SELECT %s FROM %s ORDER BY end_time %s LIMIT ?`,
		selectKLineColumns,
		quoteIdentifier(tableName), normalizedOrder,
	)

	row := s.db.QueryRow(query, limit)
	return scanKLine(row, symbol, interval)
}

// queryLatestKLineInRange returns the most recent K-line for the given
// symbol+interval whose end_time falls within [since, until].
func (s *FutuKLineStore) queryLatestKLineInRange(
	symbol string, interval types.Interval,
	since, until time.Time,
) (*types.KLine, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	source, err := s.resolveReadSource(symbol, interval, since, until)
	if err != nil {
		return nil, err
	}
	if source.synthesize {
		klines, synthErr := s.queryAggregatedKLinesInRange(symbol, interval, source.baseInterval, since, until)
		if synthErr != nil || len(klines) == 0 {
			return nil, synthErr
		}
		kline := klines[len(klines)-1]
		return &kline, nil
	}
	klines, queryErr := s.queryStoredKLinesInRange(symbol, interval, s.rehabType, since, until)
	if queryErr != nil || len(klines) == 0 {
		return nil, queryErr
	}
	kline := klines[len(klines)-1]
	return &kline, nil
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
	tableName := klineTableName(symbol, interval, rehabType)
	exists, err := s.klineTableExists(tableName)
	if err != nil {
		return false, err
	}
	if !exists {
		return false, nil
	}

	query := fmt.Sprintf(
		`SELECT end_time FROM %s WHERE end_time >= ? AND end_time <= ? ORDER BY end_time DESC LIMIT 1`,
		quoteIdentifier(tableName),
	)

	var endTimeMillis int64
	err = s.db.QueryRow(
		query,
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
	stored, err := s.queryStoredKLinesForward(symbol, interval, s.rehabType, startTime, limit)
	if err != nil {
		return nil, err
	}
	if len(stored) > 0 || !canAggregateFromLowerInterval(interval) {
		return stored, nil
	}
	effectiveUntil := firstClosedKLineEndAtOrAfter(startTime, interval).Add(interval.Duration() * time.Duration(limit-1))
	source, err := s.resolveReadSource(symbol, interval, startTime, effectiveUntil)
	if err != nil {
		return nil, err
	}
	if !source.synthesize {
		return nil, nil
	}
	aggregated, err := s.queryAggregatedKLinesInRange(symbol, interval, source.baseInterval, startTime, effectiveUntil)
	if err != nil {
		return nil, err
	}
	if len(aggregated) > limit {
		aggregated = aggregated[:limit]
	}
	return aggregated, nil
}

func (s *FutuKLineStore) QueryKLinesBackward(
	exchange types.Exchange, symbol string, interval types.Interval,
	endTime time.Time, limit int,
) ([]types.KLine, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	stored, err := s.queryStoredKLinesBackward(symbol, interval, s.rehabType, endTime, limit)
	if err != nil {
		return nil, err
	}
	if len(stored) > 0 || !canAggregateFromLowerInterval(interval) {
		return stored, nil
	}
	effectiveUntil := endTime.Add(-time.Millisecond)
	effectiveUntil = latestClosedKLineEndAtOrBefore(effectiveUntil, interval)
	since := alignTimeToIntervalStart(effectiveUntil.Add(-interval.Duration()*time.Duration(limit-1)), interval)
	source, err := s.resolveReadSource(symbol, interval, since, effectiveUntil)
	if err != nil {
		return nil, err
	}
	if !source.synthesize {
		return nil, nil
	}
	aggregated, err := s.queryAggregatedKLinesInRange(symbol, interval, source.baseInterval, since, effectiveUntil)
	if err != nil {
		return nil, err
	}
	if len(aggregated) > limit {
		aggregated = aggregated[len(aggregated)-limit:]
	}
	return aggregated, nil
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

		if len(symbols) == 0 || len(intervals) == 0 {
			return
		}

		s.mu.RLock()
		defer s.mu.RUnlock()

		klines := make([]types.KLine, 0, len(symbols)*len(intervals))
		for _, symbol := range symbols {
			for _, interval := range intervals {
				source, err := s.resolveReadSource(symbol, interval, since, until)
				if err != nil {
					errCh <- err
					return
				}

				var series []types.KLine
				if source.synthesize {
					series, err = s.queryAggregatedKLinesInRange(symbol, interval, source.baseInterval, since, until)
				} else {
					series, err = s.queryStoredKLinesInRange(symbol, interval, s.rehabType, since, until)
				}
				if err != nil {
					errCh <- err
					return
				}
				klines = append(klines, series...)
			}
		}

		sort.Slice(klines, func(i, j int) bool {
			leftEnd := klines[i].EndTime.Time()
			rightEnd := klines[j].EndTime.Time()
			if !leftEnd.Equal(rightEnd) {
				return leftEnd.Before(rightEnd)
			}
			leftInterval := intervalStorageValue(klines[i].Interval)
			rightInterval := intervalStorageValue(klines[j].Interval)
			if leftInterval != rightInterval {
				return leftInterval < rightInterval
			}
			return klines[i].Symbol < klines[j].Symbol
		})
		for _, k := range klines {
			ch <- k
		}
	}()

	return ch, errCh
}

// --- Batch Insert ---

// InsertKLine inserts a single K-line into the store. Duplicates (same
// end_time in the same series table) are replaced.
func (s *FutuKLineStore) InsertKLine(kline types.KLine, rehabType string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	tableName := klineTableName(kline.Symbol, kline.Interval, rehabType)
	if err := s.ensureKLineTable(tableName); err != nil {
		return err
	}

	_, err := s.db.Exec(
		`INSERT INTO `+quoteIdentifier(tableName)+` (end_time, start_time, open, high, low, close, volume) VALUES (?, ?, ?, ?, ?, ?, ?) `+
			`ON CONFLICT(end_time) DO UPDATE SET `+
			`start_time = excluded.start_time, open = excluded.open, high = excluded.high, low = excluded.low, close = excluded.close, volume = excluded.volume`,
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

func (s *FutuKLineStore) queryStoredKLinesForward(symbol string, interval types.Interval, rehabType string, startTime time.Time, limit int) ([]types.KLine, error) {
	tableName := klineTableName(symbol, interval, rehabType)
	exists, err := s.klineTableExists(tableName)
	if err != nil || !exists {
		return nil, err
	}

	query := fmt.Sprintf(
		`SELECT %s FROM %s WHERE end_time >= ? ORDER BY end_time ASC LIMIT ?`,
		selectKLineColumns,
		quoteIdentifier(tableName),
	)
	rows, err := s.db.Query(query, timeToUnixMillis(startTime), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanKLines(rows, symbol, interval)
}

func (s *FutuKLineStore) queryStoredKLinesBackward(symbol string, interval types.Interval, rehabType string, endTime time.Time, limit int) ([]types.KLine, error) {
	tableName := klineTableName(symbol, interval, rehabType)
	exists, err := s.klineTableExists(tableName)
	if err != nil || !exists {
		return nil, err
	}

	query := fmt.Sprintf(
		`SELECT %s FROM %s WHERE end_time <= ? ORDER BY end_time DESC LIMIT ?`,
		selectKLineColumns,
		quoteIdentifier(tableName),
	)
	rows, err := s.db.Query(query, timeToUnixMillis(endTime), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	klines, err := scanKLines(rows, symbol, interval)
	if err != nil {
		return nil, err
	}
	reverseKLines(klines)
	return klines, nil
}

func (s *FutuKLineStore) queryStoredKLinesInRange(symbol string, interval types.Interval, rehabType string, since, until time.Time) ([]types.KLine, error) {
	tableName := klineTableName(symbol, interval, rehabType)
	exists, err := s.klineTableExists(tableName)
	if err != nil || !exists {
		return nil, err
	}

	query := fmt.Sprintf(
		`SELECT %s FROM %s WHERE end_time >= ? AND end_time <= ? ORDER BY end_time ASC`,
		selectKLineColumns,
		quoteIdentifier(tableName),
	)
	rows, err := s.db.Query(query, timeToUnixMillis(since), timeToUnixMillis(until))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanKLines(rows, symbol, interval)
}

func (s *FutuKLineStore) queryAggregatedKLinesInRange(symbol string, interval, baseInterval types.Interval, since, until time.Time) ([]types.KLine, error) {
	if interval == baseInterval || interval.Duration() <= baseInterval.Duration() {
		return nil, nil
	}
	baseSince := alignTimeToIntervalStart(since, interval)
	baseRows, err := s.queryStoredKLinesInRange(symbol, baseInterval, s.rehabType, baseSince, until)
	if err != nil {
		return nil, err
	}
	return aggregateKLinesFromBase(symbol, interval, baseInterval, baseRows, since, until), nil
}

func aggregateKLinesFromBase(symbol string, interval, baseInterval types.Interval, baseRows []types.KLine, since, until time.Time) []types.KLine {
	if len(baseRows) == 0 {
		return nil
	}

	factor := int(interval.Duration() / baseInterval.Duration())
	if factor <= 1 {
		return nil
	}

	aggregated := make([]types.KLine, 0, len(baseRows)/factor)
	var current types.KLine
	var currentBucketStart time.Time
	currentCount := 0
	currentVolume := 0.0

	flush := func() {
		if currentCount != factor {
			return
		}
		endAt := current.EndTime.Time()
		if endAt.Before(since) || endAt.After(until) {
			return
		}
		current.Volume = floatToFixed(currentVolume)
		aggregated = append(aggregated, current)
	}

	for _, base := range baseRows {
		bucketStart := alignTimeToIntervalStart(base.StartTime.Time(), interval)
		if currentCount == 0 || !bucketStart.Equal(currentBucketStart) {
			flush()
			currentBucketStart = bucketStart
			currentCount = 0
			currentVolume = 0
			current = types.KLine{
				StartTime: types.Time(bucketStart),
				EndTime:   base.EndTime,
				Interval:  interval,
				Symbol:    symbol,
				Open:      base.Open,
				High:      base.High,
				Low:       base.Low,
				Close:     base.Close,
				Closed:    true,
			}
		} else {
			if base.High.Compare(current.High) > 0 {
				current.High = base.High
			}
			if base.Low.Compare(current.Low) < 0 {
				current.Low = base.Low
			}
			current.Close = base.Close
			current.EndTime = base.EndTime
		}
		currentCount++
		currentVolume += base.Volume.Float64()
	}
	flush()
	return aggregated
}

// --- Helpers ---
