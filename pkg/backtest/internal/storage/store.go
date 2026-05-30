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

	"github.com/c9s/bbgo/pkg/fixedpoint"
	"github.com/jmoiron/sqlx"
	_ "modernc.org/sqlite"

	"github.com/c9s/bbgo/pkg/types"
	"github.com/jftrade/jftrade-main/pkg/futu"
)

// FutuKLineStore implements service.BackTestable for Futu data stored in
// SQLite. It uses a compact backtest-only local_klines table keyed by
// symbol+interval+rehab_type (table-level) and end_time (row-level).
type FutuKLineStore struct {
	mu                sync.RWMutex
	db                *sqlx.DB
	rehabType         string // "forward" | "backward" | "none" — filters all queries
	readSessionScope  string
	writeSessionScope string
	tableExistsCache  sync.Map
}

// NewFutuKLineStore opens or creates a SQLite database at the given path and
// lazily creates per-series tables as data is inserted.
func NewFutuKLineStore(dbPath string) (*FutuKLineStore, error) {
	db, err := sqlx.Open("sqlite", dbPath+"?_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)")
	if err != nil {
		return nil, fmt.Errorf("open sqlite backtest store: %w", err)
	}
	store := &FutuKLineStore{
		db:                db,
		rehabType:         normalizeRehabTypeName("forward"),
		readSessionScope:  klineReadSessionScopeAuto,
		writeSessionScope: klineSessionScopeLegacy,
	}
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

func (s *FutuKLineStore) SetReadSessionScope(scope string) {
	s.readSessionScope = normalizeReadSessionScopeName(scope)
}

func (s *FutuKLineStore) SetWriteSessionScope(scope string) {
	s.writeSessionScope = normalizeKLineSessionScopeName(scope)
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
		`  open        TEXT    NOT NULL,`,
		`  high        TEXT    NOT NULL,`,
		`  low         TEXT    NOT NULL,`,
		`  close       TEXT    NOT NULL,`,
		`  volume      TEXT    NOT NULL,`,
		`  PRIMARY KEY (end_time)`,
		`) WITHOUT ROWID`,
	}, " "))
	if err != nil {
		return fmt.Errorf("create %s table: %w", tableName, err)
	}
	s.tableExistsCache.Store(tableName, true)
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
	if cached, ok := s.tableExistsCache.Load(tableName); ok {
		return cached.(bool), nil
	}
	var count int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = ?`, tableName).Scan(&count); err != nil {
		return false, err
	}
	exists := count > 0
	s.tableExistsCache.Store(tableName, exists)
	return exists, nil
}

func (s *FutuKLineStore) writeTableName(symbol string, interval types.Interval, rehabType string) string {
	return klineTableNameForSessionScope(symbol, interval, rehabType, s.writeSessionScope)
}

func (s *FutuKLineStore) readTableNames(symbol string, interval types.Interval, rehabType string) ([3]string, int) {
	var tableNames [3]string
	add := func(index int, scope string) {
		tableNames[index] = klineTableNameForSessionScope(symbol, interval, rehabType, scope)
	}

	switch normalizeReadSessionScopeName(s.readSessionScope) {
	case klineSessionScopeRegular:
		add(0, klineSessionScopeRegular)
		add(1, klineSessionScopeLegacy)
		add(2, klineSessionScopeExtended)
		return tableNames, 3
	case klineSessionScopeExtended:
		add(0, klineSessionScopeExtended)
		add(1, klineSessionScopeLegacy)
		add(2, klineSessionScopeRegular)
		return tableNames, 3
	case klineSessionScopeLegacy:
		add(0, klineSessionScopeLegacy)
		return tableNames, 1
	default:
		add(0, klineSessionScopeLegacy)
		add(1, klineSessionScopeExtended)
		add(2, klineSessionScopeRegular)
		return tableNames, 3
	}
}

func (s *FutuKLineStore) selectReadTableName(symbol string, interval types.Interval, rehabType string, since, until time.Time) (string, error) {
	var firstExisting string
	tableNames, tableCount := s.readTableNames(symbol, interval, rehabType)
	for index := 0; index < tableCount; index++ {
		tableName := tableNames[index]
		exists, err := s.klineTableExists(tableName)
		if err != nil {
			return "", err
		}
		if !exists {
			continue
		}
		if firstExisting == "" {
			firstExisting = tableName
		}
		if since.IsZero() || until.IsZero() {
			return tableName, nil
		}
		missing, err := s.findSelectionMissingRangesInPhysicalTable(tableName, interval, since, until)
		if err != nil {
			return "", err
		}
		if len(missing) == 0 {
			return tableName, nil
		}
	}
	return firstExisting, nil
}

func (s *FutuKLineStore) findSelectionMissingRangesInPhysicalTable(
	tableName string, interval types.Interval, startTime, endTime time.Time,
) ([]string, error) {
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

	boundariesCovered, err := s.hasKLineBoundaryPair(tableName, leftBoundary, rightBoundary)
	if err != nil {
		return nil, err
	}
	if !boundariesCovered {
		return fullWindowMissingRange(startTime, endTime), nil
	}
	return nil, nil
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

	baseSince := alignTimeToIntervalStart(startTime, interval)
	baseUntil := endTime
	if isTradingPeriodAggregationInterval(interval) {
		baseSince, baseUntil = tradingPeriodAggregationBaseRange(symbol, interval, startTime, endTime, false)
	} else if shouldUseSessionAwareIntradayAggregation(symbol, interval) {
		baseSince, baseUntil = sessionAwareIntradayAggregationBaseRange(symbol, interval, startTime, endTime, false)
	}
	var preferredMissing []string
	for index, baseInterval := range aggregationBaseIntervals(interval) {
		baseMissing, err := s.findMissingRangesInTable(symbol, baseInterval, s.rehabType, baseSince, baseUntil)
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
	var firstMissing []string
	tableNames, tableCount := s.readTableNames(symbol, interval, rehabType)
	for index := 0; index < tableCount; index++ {
		tableName := tableNames[index]
		exists, err := s.klineTableExists(tableName)
		if err != nil {
			return nil, err
		}
		if !exists {
			continue
		}
		missing, err := s.findMissingRangesInPhysicalTable(tableName, interval, startTime, endTime)
		if err != nil {
			return nil, err
		}
		if len(missing) == 0 {
			return nil, nil
		}
		if len(firstMissing) == 0 {
			firstMissing = missing
		}
	}
	if len(firstMissing) > 0 {
		return firstMissing, nil
	}
	return fullWindowMissingRange(startTime, endTime), nil
}

func (s *FutuKLineStore) findMissingRangesInWriteTable(
	symbol string, interval types.Interval, rehabType string, startTime, endTime time.Time,
) ([]string, error) {
	tableName := s.writeTableName(symbol, interval, rehabType)
	return s.findMissingRangesInPhysicalTable(tableName, interval, startTime, endTime)
}

func (s *FutuKLineStore) findMissingRangesInPhysicalTable(
	tableName string, interval types.Interval, startTime, endTime time.Time,
) ([]string, error) {
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

func (s *FutuKLineStore) hasKLineBoundaryPair(tableName string, left, right time.Time) (bool, error) {
	leftMillis := timeToUnixMillis(left)
	rightMillis := timeToUnixMillis(right)
	expectedCount := 2
	if leftMillis == rightMillis {
		expectedCount = 1
	}
	var actualCount int
	err := s.db.QueryRow(
		`SELECT COUNT(DISTINCT end_time) FROM `+quoteIdentifier(tableName)+` WHERE end_time IN (?, ?)`,
		leftMillis,
		rightMillis,
	).Scan(&actualCount)
	if err != nil {
		return false, err
	}
	return actualCount == expectedCount, nil
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
		baseSince := alignTimeToIntervalStart(since, interval)
		baseUntil := until
		if isTradingPeriodAggregationInterval(interval) {
			baseSince, baseUntil = tradingPeriodAggregationBaseRange(symbol, interval, since, until, false)
		} else if shouldUseSessionAwareIntradayAggregation(symbol, interval) {
			baseSince, baseUntil = sessionAwareIntradayAggregationBaseRange(symbol, interval, since, until, false)
		}
		for _, baseInterval := range candidates {
			baseMissing, baseErr := s.findMissingRangesInTable(symbol, baseInterval, s.rehabType, baseSince, baseUntil)
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
	tableName, err := s.selectReadTableName(symbol, interval, s.rehabType, time.Time{}, time.Time{})
	if err != nil || tableName == "" {
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
	tableName := s.writeTableName(symbol, interval, rehabType)
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
	if shouldUseSessionAwareIntradayAggregation(symbol, interval) {
		return s.queryAggregatedSessionAwareIntradayKLinesForwardLocked(symbol, interval, startTime, limit, false)
	}
	if isTradingPeriodAggregationInterval(interval) {
		return s.queryAggregatedTradingPeriodKLinesForwardLocked(symbol, interval, startTime, limit)
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
	if shouldUseSessionAwareIntradayAggregation(symbol, interval) {
		return s.queryAggregatedSessionAwareIntradayKLinesBackwardLocked(symbol, interval, endTime, limit, false)
	}
	if isTradingPeriodAggregationInterval(interval) {
		return s.queryAggregatedTradingPeriodKLinesBackwardLocked(symbol, interval, endTime, limit)
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
	ch := make(chan types.KLine, klineQueryChannelBufferSize(symbols, intervals))
	errCh := make(chan error, 1)

	go func() {
		defer close(ch)
		defer close(errCh)

		if len(symbols) == 0 || len(intervals) == 0 {
			return
		}

		s.mu.RLock()
		defer s.mu.RUnlock()

		if len(symbols) == 1 && len(intervals) == 1 {
			symbol := symbols[0]
			interval := intervals[0]
			source, err := s.resolveReadSource(symbol, interval, since, until)
			if err != nil {
				errCh <- err
				return
			}

			if source.synthesize {
				series, err := s.queryAggregatedKLinesInRange(symbol, interval, source.baseInterval, since, until)
				if err != nil {
					errCh <- err
					return
				}
				for _, k := range series {
					ch <- k
				}
			} else {
				if err := s.streamStoredKLinesInRangeToChannel(symbol, interval, s.rehabType, since, until, ch); err != nil {
					errCh <- err
					return
				}
			}
			return
		}

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

func (s *FutuKLineStore) StreamKLines(
	since, until time.Time, exchange types.Exchange,
	symbols []string, intervals []types.Interval,
	emit func(types.KLine),
) error {
	if emit == nil || len(symbols) == 0 || len(intervals) == 0 {
		return nil
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	if len(symbols) == 1 && len(intervals) == 1 {
		symbol := symbols[0]
		interval := intervals[0]
		source, err := s.resolveReadSource(symbol, interval, since, until)
		if err != nil {
			return err
		}

		if source.synthesize {
			series, err := s.queryAggregatedKLinesInRange(symbol, interval, source.baseInterval, since, until)
			if err != nil {
				return err
			}
			for _, kline := range series {
				emit(kline)
			}
			return nil
		}

		return s.streamStoredKLinesInRange(symbol, interval, s.rehabType, since, until, emit)
	}

	klines := make([]types.KLine, 0, len(symbols)*len(intervals))
	for _, symbol := range symbols {
		for _, interval := range intervals {
			source, err := s.resolveReadSource(symbol, interval, since, until)
			if err != nil {
				return err
			}

			var series []types.KLine
			if source.synthesize {
				series, err = s.queryAggregatedKLinesInRange(symbol, interval, source.baseInterval, since, until)
			} else {
				series, err = s.queryStoredKLinesInRange(symbol, interval, s.rehabType, since, until)
			}
			if err != nil {
				return err
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
	for _, kline := range klines {
		emit(kline)
	}
	return nil
}

// --- Batch Insert ---

// InsertKLine inserts a single K-line into the store. Duplicates (same
// end_time in the same series table) are replaced.
func (s *FutuKLineStore) InsertKLine(kline types.KLine, rehabType string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.insertKLineLocked(kline, rehabType, nil)
}

func (s *FutuKLineStore) insertKLineLocked(kline types.KLine, rehabType string, ensuredTables map[string]struct{}) error {
	tableName := s.writeTableName(kline.Symbol, kline.Interval, rehabType)
	if ensuredTables != nil {
		if _, ok := ensuredTables[tableName]; !ok {
			if err := s.ensureKLineTable(tableName); err != nil {
				return err
			}
			ensuredTables[tableName] = struct{}{}
		}
	} else {
		if err := s.ensureKLineTable(tableName); err != nil {
			return err
		}
	}

	_, err := s.db.Exec(
		klineInsertStatement(tableName),
		timeToUnixMillis(kline.EndTime.Time()),
		timeToUnixMillis(kline.StartTime.Time()),
		kline.Open.String(),
		kline.High.String(),
		kline.Low.String(),
		kline.Close.String(),
		kline.Volume.String(),
	)
	return err
}

func klineInsertStatement(tableName string) string {
	return `INSERT INTO ` + quoteIdentifier(tableName) + ` (end_time, start_time, open, high, low, close, volume) VALUES (?, ?, ?, ?, ?, ?, ?) ` +
		`ON CONFLICT(end_time) DO UPDATE SET ` +
		`start_time = excluded.start_time, open = excluded.open, high = excluded.high, low = excluded.low, close = excluded.close, volume = excluded.volume`
}

// InsertKLines batch-inserts K-lines into the store.
func (s *FutuKLineStore) InsertKLines(klines []types.KLine, rehabType string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(klines) == 0 {
		return nil
	}
	if len(klines) == 1 {
		return s.insertKLineLocked(klines[0], rehabType, nil)
	}

	tableNames := make([]string, len(klines))
	ensuredTables := make(map[string]struct{})
	for index, k := range klines {
		tableName := s.writeTableName(k.Symbol, k.Interval, rehabType)
		tableNames[index] = tableName
		if _, ok := ensuredTables[tableName]; ok {
			continue
		}
		if err := s.ensureKLineTable(tableName); err != nil {
			return err
		}
		ensuredTables[tableName] = struct{}{}
	}

	tx, err := s.db.Beginx()
	if err != nil {
		return err
	}
	stmts := make(map[string]*sqlx.Stmt, len(ensuredTables))
	defer func() {
		for _, stmt := range stmts {
			_ = stmt.Close()
		}
	}()

	for index, k := range klines {
		tableName := tableNames[index]
		stmt, ok := stmts[tableName]
		if !ok {
			stmt, err = tx.Preparex(klineInsertStatement(tableName))
			if err != nil {
				_ = tx.Rollback()
				return err
			}
			stmts[tableName] = stmt
		}

		if _, err := stmt.Exec(
			timeToUnixMillis(k.EndTime.Time()),
			timeToUnixMillis(k.StartTime.Time()),
			k.Open.String(),
			k.High.String(),
			k.Low.String(),
			k.Close.String(),
			k.Volume.String(),
		); err != nil {
			_ = tx.Rollback()
			return err
		}
	}

	if err := tx.Commit(); err != nil {
		_ = tx.Rollback()
		return err
	}
	return nil
}

func (s *FutuKLineStore) queryStoredKLinesForward(symbol string, interval types.Interval, rehabType string, startTime time.Time, limit int) ([]types.KLine, error) {
	var firstNonEmpty []types.KLine
	tableNames, tableCount := s.readTableNames(symbol, interval, rehabType)
	for index := 0; index < tableCount; index++ {
		tableName := tableNames[index]
		exists, err := s.klineTableExists(tableName)
		if err != nil {
			return nil, err
		}
		if !exists {
			continue
		}
		rows, err := s.queryStoredKLinesForwardFromTable(tableName, symbol, interval, startTime, limit)
		if err != nil {
			return nil, err
		}
		if len(rows) == 0 {
			continue
		}
		if len(rows) >= limit {
			return rows, nil
		}
		if len(firstNonEmpty) == 0 {
			firstNonEmpty = rows
		}
	}
	return firstNonEmpty, nil
}

func (s *FutuKLineStore) queryStoredKLinesForwardFromTable(tableName string, symbol string, interval types.Interval, startTime time.Time, limit int) ([]types.KLine, error) {

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
	return scanKLinesWithCapacity(rows, symbol, interval, limit)
}

func (s *FutuKLineStore) queryStoredKLinesBackward(symbol string, interval types.Interval, rehabType string, endTime time.Time, limit int) ([]types.KLine, error) {
	var firstNonEmpty []types.KLine
	tableNames, tableCount := s.readTableNames(symbol, interval, rehabType)
	for index := 0; index < tableCount; index++ {
		tableName := tableNames[index]
		exists, err := s.klineTableExists(tableName)
		if err != nil {
			return nil, err
		}
		if !exists {
			continue
		}
		rows, err := s.queryStoredKLinesBackwardFromTable(tableName, symbol, interval, endTime, limit)
		if err != nil {
			return nil, err
		}
		if len(rows) == 0 {
			continue
		}
		if len(rows) >= limit {
			return rows, nil
		}
		if len(firstNonEmpty) == 0 {
			firstNonEmpty = rows
		}
	}
	return firstNonEmpty, nil
}

func (s *FutuKLineStore) queryStoredKLinesBackwardFromTable(tableName string, symbol string, interval types.Interval, endTime time.Time, limit int) ([]types.KLine, error) {

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

	klines, err := scanKLinesWithCapacity(rows, symbol, interval, limit)
	if err != nil {
		return nil, err
	}
	reverseKLines(klines)
	return klines, nil
}

func (s *FutuKLineStore) queryStoredKLinesInRange(symbol string, interval types.Interval, rehabType string, since, until time.Time) ([]types.KLine, error) {
	tableName, err := s.selectReadTableName(symbol, interval, rehabType, since, until)
	if err != nil || tableName == "" {
		return nil, err
	}
	return s.queryStoredKLinesInRangeFromTable(tableName, symbol, interval, since, until)
}

func (s *FutuKLineStore) streamStoredKLinesInRange(symbol string, interval types.Interval, rehabType string, since, until time.Time, emit func(types.KLine)) error {
	tableName, err := s.selectReadTableName(symbol, interval, rehabType, since, until)
	if err != nil || tableName == "" {
		return err
	}
	return s.streamStoredKLinesInRangeFromTable(tableName, symbol, interval, since, until, emit)
}

func (s *FutuKLineStore) streamStoredKLinesInRangeToChannel(symbol string, interval types.Interval, rehabType string, since, until time.Time, ch chan<- types.KLine) error {
	return s.streamStoredKLinesInRange(symbol, interval, rehabType, since, until, func(kline types.KLine) {
		ch <- kline
	})
}

func (s *FutuKLineStore) queryStoredKLinesInRangeFromTable(tableName string, symbol string, interval types.Interval, since, until time.Time) ([]types.KLine, error) {

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

func (s *FutuKLineStore) streamStoredKLinesInRangeFromTable(tableName string, symbol string, interval types.Interval, since, until time.Time, emit func(types.KLine)) error {
	query := fmt.Sprintf(
		`SELECT %s FROM %s WHERE end_time >= ? AND end_time <= ? ORDER BY end_time ASC`,
		selectKLineColumns,
		quoteIdentifier(tableName),
	)
	rows, err := s.db.Query(query, timeToUnixMillis(since), timeToUnixMillis(until))
	if err != nil {
		return err
	}
	defer rows.Close()
	return streamKLines(rows, symbol, interval, emit)
}

func klineQueryChannelBufferSize(symbols []string, intervals []types.Interval) int {
	if len(symbols) == 1 && len(intervals) == 1 {
		return 256
	}
	return 512
}

func (s *FutuKLineStore) QueryTradingPeriodKLinesInRange(symbol string, interval types.Interval, since, until time.Time, includeExtendedHours bool) ([]types.KLine, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.queryTradingPeriodKLinesInRangeLocked(symbol, interval, since, until, includeExtendedHours)
}

func (s *FutuKLineStore) QuerySessionAwareIntradayKLinesInRange(symbol string, interval types.Interval, since, until time.Time, includeExtendedHours bool) ([]types.KLine, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.querySessionAwareIntradayKLinesInRangeLocked(symbol, interval, since, until, includeExtendedHours)
}

func (s *FutuKLineStore) QueryDailyKLinesInRange(symbol string, since, until time.Time, includeExtendedHours bool) ([]types.KLine, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.queryDailyKLinesInRangeLocked(symbol, since, until, includeExtendedHours)
}

func (s *FutuKLineStore) queryAggregatedKLinesInRange(symbol string, interval, baseInterval types.Interval, since, until time.Time) ([]types.KLine, error) {
	if interval == types.Interval1d {
		return s.queryDailyKLinesInRangeLocked(symbol, since, until, false)
	}
	if isTradingPeriodAggregationInterval(interval) {
		return s.queryAggregatedTradingPeriodKLinesInRangeLocked(symbol, interval, baseInterval, since, until, false)
	}
	if shouldUseSessionAwareIntradayAggregation(symbol, interval) {
		return s.queryAggregatedSessionAwareIntradayKLinesInRangeLocked(symbol, interval, baseInterval, since, until, false)
	}
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

func (s *FutuKLineStore) queryDailyKLinesInRangeLocked(symbol string, since, until time.Time, includeExtendedHours bool) ([]types.KLine, error) {
	dailySince := alignTimeToIntervalStart(since, types.Interval1d)
	dailyUntil := latestClosedKLineEndAtOrBefore(until, types.Interval1d)
	if dailyUntil.Before(dailySince) {
		return nil, nil
	}

	if !includeExtendedHours {
		stored, err := s.queryStoredKLinesInRange(symbol, types.Interval1d, s.rehabType, dailySince, dailyUntil)
		if err != nil {
			return nil, err
		}
		if len(stored) > 0 {
			return stored, nil
		}
	}

	baseInterval, err := s.resolveDailyAggregationBaseInterval(symbol, dailySince, dailyUntil, includeExtendedHours)
	if err != nil {
		if includeExtendedHours {
			stored, storedErr := s.queryStoredKLinesInRange(symbol, types.Interval1d, s.rehabType, dailySince, dailyUntil)
			if storedErr != nil {
				return nil, storedErr
			}
			if len(stored) > 0 {
				return stored, nil
			}
		}
		return nil, err
	}

	return s.queryAggregatedDailyKLinesInRangeLocked(symbol, baseInterval, dailySince, dailyUntil, includeExtendedHours)
}

func (s *FutuKLineStore) queryTradingPeriodKLinesInRangeLocked(symbol string, interval types.Interval, since, until time.Time, includeExtendedHours bool) ([]types.KLine, error) {
	if interval == types.Interval1d {
		return s.queryDailyKLinesInRangeLocked(symbol, since, until, includeExtendedHours)
	}
	if !isTradingPeriodAggregationInterval(interval) {
		return nil, nil
	}

	baseInterval, err := s.resolveTradingPeriodAggregationBaseInterval(symbol, interval, since, until, includeExtendedHours)
	if err != nil {
		return nil, err
	}
	return s.queryAggregatedTradingPeriodKLinesInRangeLocked(symbol, interval, baseInterval, since, until, includeExtendedHours)
}

func (s *FutuKLineStore) querySessionAwareIntradayKLinesInRangeLocked(symbol string, interval types.Interval, since, until time.Time, includeExtendedHours bool) ([]types.KLine, error) {
	if !shouldUseSessionAwareIntradayAggregation(symbol, interval) {
		return nil, nil
	}

	baseInterval, err := s.resolveSessionAwareIntradayAggregationBaseInterval(symbol, interval, since, until, includeExtendedHours)
	if err != nil {
		return nil, err
	}
	return s.queryAggregatedSessionAwareIntradayKLinesInRangeLocked(symbol, interval, baseInterval, since, until, includeExtendedHours)
}

func (s *FutuKLineStore) resolveSessionAwareIntradayAggregationBaseInterval(symbol string, interval types.Interval, since, until time.Time, includeExtendedHours bool) (types.Interval, error) {
	candidates := aggregationBaseIntervals(interval)
	if includeExtendedHours {
		filtered := make([]types.Interval, 0, len(candidates))
		for _, candidate := range candidates {
			if candidate.Duration() > 0 && candidate.Duration() <= time.Hour {
				filtered = append(filtered, candidate)
			}
		}
		candidates = filtered
	}
	if len(candidates) == 0 {
		return "", fmt.Errorf("missing K-line coverage for %s %s [%s, %s]; download lower-interval data covering the full range", symbol, interval, since.UTC().Format(time.RFC3339), until.UTC().Format(time.RFC3339))
	}

	baseSince, baseUntil := sessionAwareIntradayAggregationBaseRange(symbol, interval, since, until, includeExtendedHours)
	for _, baseInterval := range candidates {
		baseMissing, err := s.findMissingRangesInTable(symbol, baseInterval, s.rehabType, baseSince, baseUntil)
		if err != nil {
			return "", err
		}
		if len(baseMissing) == 0 {
			return baseInterval, nil
		}
	}

	preferred := candidates[0]
	if includeExtendedHours {
		return "", fmt.Errorf("missing K-line coverage for %s %s [%s, %s]; download %s-or-lower data covering the full range for extended-hours intraday aggregation", symbol, interval, since.UTC().Format(time.RFC3339), until.UTC().Format(time.RFC3339), preferred)
	}
	return "", fmt.Errorf("missing K-line coverage for %s %s [%s, %s]; download %s data covering the full range", symbol, interval, since.UTC().Format(time.RFC3339), until.UTC().Format(time.RFC3339), preferred)
}

func (s *FutuKLineStore) queryAggregatedSessionAwareIntradayKLinesInRangeLocked(symbol string, interval, baseInterval types.Interval, since, until time.Time, includeExtendedHours bool) ([]types.KLine, error) {
	baseSince, baseUntil := sessionAwareIntradayAggregationBaseRange(symbol, interval, since, until, includeExtendedHours)
	baseRows, err := s.queryStoredKLinesInRange(symbol, baseInterval, s.rehabType, baseSince, baseUntil)
	if err != nil {
		return nil, err
	}
	return aggregateSessionAwareIntradayKLinesFromBase(symbol, interval, baseRows, since, until, includeExtendedHours), nil
}

func (s *FutuKLineStore) resolveTradingPeriodAggregationBaseInterval(symbol string, interval types.Interval, since, until time.Time, includeExtendedHours bool) (types.Interval, error) {
	candidates := aggregationBaseIntervals(interval)
	if includeExtendedHours {
		filtered := make([]types.Interval, 0, len(candidates))
		for _, candidate := range candidates {
			if candidate.Duration() > 0 && candidate.Duration() <= time.Hour {
				filtered = append(filtered, candidate)
			}
		}
		candidates = filtered
	}
	if len(candidates) == 0 {
		return "", fmt.Errorf("missing K-line coverage for %s %s [%s, %s]; download lower-interval data covering the full range", symbol, interval, since.UTC().Format(time.RFC3339), until.UTC().Format(time.RFC3339))
	}

	baseSince, baseUntil := tradingPeriodAggregationBaseRange(symbol, interval, since, until, includeExtendedHours)
	for _, baseInterval := range candidates {
		baseMissing, err := s.findMissingRangesInTable(symbol, baseInterval, s.rehabType, baseSince, baseUntil)
		if err != nil {
			return "", err
		}
		if len(baseMissing) == 0 {
			return baseInterval, nil
		}
	}

	preferred := candidates[0]
	if includeExtendedHours {
		return "", fmt.Errorf("missing K-line coverage for %s %s [%s, %s]; download %s-or-lower data covering the full range for extended-hours trading-period aggregation", symbol, interval, since.UTC().Format(time.RFC3339), until.UTC().Format(time.RFC3339), preferred)
	}
	return "", fmt.Errorf("missing K-line coverage for %s %s [%s, %s]; download %s data covering the full range", symbol, interval, since.UTC().Format(time.RFC3339), until.UTC().Format(time.RFC3339), preferred)
}

func (s *FutuKLineStore) queryAggregatedTradingPeriodKLinesInRangeLocked(symbol string, interval, baseInterval types.Interval, since, until time.Time, includeExtendedHours bool) ([]types.KLine, error) {
	baseSince, baseUntil := tradingPeriodAggregationBaseRange(symbol, interval, since, until, includeExtendedHours)
	baseRows, err := s.queryStoredKLinesInRange(symbol, baseInterval, s.rehabType, baseSince, baseUntil)
	if err != nil {
		return nil, err
	}
	return aggregateTradingPeriodKLinesFromBase(symbol, interval, baseRows, since, until, includeExtendedHours), nil
}

func (s *FutuKLineStore) resolveDailyAggregationBaseInterval(symbol string, since, until time.Time, includeExtendedHours bool) (types.Interval, error) {
	candidates := prioritizeDailyAggregationBaseIntervals(aggregationBaseIntervals(types.Interval1d), includeExtendedHours)
	if len(candidates) == 0 {
		return "", fmt.Errorf("missing K-line coverage for %s 1d [%s, %s]; download 1d data covering the full range", symbol, since.UTC().Format(time.RFC3339), until.UTC().Format(time.RFC3339))
	}

	baseSince, baseUntil := dailyAggregationBaseRange(symbol, since, until, includeExtendedHours)
	for _, baseInterval := range candidates {
		baseMissing, err := s.findMissingRangesInTable(symbol, baseInterval, s.rehabType, baseSince, baseUntil)
		if err != nil {
			return "", err
		}
		if len(baseMissing) == 0 {
			return baseInterval, nil
		}
	}

	preferred := candidates[0]
	if includeExtendedHours {
		return "", fmt.Errorf("missing K-line coverage for %s 1d [%s, %s]; download %s data covering the full range for extended-hours daily aggregation", symbol, since.UTC().Format(time.RFC3339), until.UTC().Format(time.RFC3339), preferred)
	}
	return "", fmt.Errorf("missing K-line coverage for %s 1d [%s, %s]; download %s data covering the full range", symbol, since.UTC().Format(time.RFC3339), until.UTC().Format(time.RFC3339), preferred)
}

func (s *FutuKLineStore) queryAggregatedDailyKLinesInRangeLocked(symbol string, baseInterval types.Interval, since, until time.Time, includeExtendedHours bool) ([]types.KLine, error) {
	baseSince, baseUntil := dailyAggregationBaseRange(symbol, since, until, includeExtendedHours)
	baseRows, err := s.queryStoredKLinesInRange(symbol, baseInterval, s.rehabType, baseSince, baseUntil)
	if err != nil {
		return nil, err
	}
	return aggregateDailyKLinesFromBase(symbol, baseRows, since, until, includeExtendedHours), nil
}

func prioritizeDailyAggregationBaseIntervals(candidates []types.Interval, includeExtendedHours bool) []types.Interval {
	if !includeExtendedHours || len(candidates) == 0 {
		return candidates
	}
	preferred := make([]types.Interval, 0, len(candidates))
	fallback := make([]types.Interval, 0, len(candidates))
	for _, candidate := range candidates {
		if candidate.Duration() > 0 && candidate.Duration() <= time.Hour {
			preferred = append(preferred, candidate)
			continue
		}
		fallback = append(fallback, candidate)
	}
	return append(preferred, fallback...)
}

func dailyAggregationBaseRange(symbol string, since, until time.Time, includeExtendedHours bool) (time.Time, time.Time) {
	baseSince := alignTimeToIntervalStart(since, types.Interval1d)
	baseUntil := latestClosedKLineEndAtOrBefore(until, types.Interval1d)
	if includeExtendedHours && strings.HasPrefix(strings.ToUpper(strings.TrimSpace(symbol)), "US.") {
		baseSince = baseSince.Add(-6 * time.Hour)
		baseUntil = baseUntil.Add(6 * time.Hour)
	}
	return baseSince, baseUntil
}

func tradingPeriodAggregationBaseRange(symbol string, interval types.Interval, since, until time.Time, includeExtendedHours bool) (time.Time, time.Time) {
	baseSince := alignTimeToIntervalStart(since, types.Interval1d)
	if interval != types.Interval1d {
		if labelStart, ok := futu.TradingPeriodLabelStartForDate(symbol, since, tradingPeriodUnit(interval)); ok {
			baseSince = labelStart
		}
	}
	baseUntil := latestClosedKLineEndAtOrBefore(until, types.Interval1d)
	if includeExtendedHours && strings.HasPrefix(strings.ToUpper(strings.TrimSpace(symbol)), "US.") {
		baseSince = baseSince.Add(-6 * time.Hour)
		baseUntil = baseUntil.Add(6 * time.Hour)
	}
	return baseSince, baseUntil
}

func sessionAwareIntradayAggregationBaseRange(symbol string, interval types.Interval, since, until time.Time, includeExtendedHours bool) (time.Time, time.Time) {
	baseSince := since
	if bucketStart, _, ok := futu.SessionAwareIntradayBucketBounds(symbol, since, interval.Duration(), includeExtendedHours); ok {
		baseSince = bucketStart
	}
	return baseSince, until
}

func aggregateDailyKLinesFromBase(symbol string, baseRows []types.KLine, since, until time.Time, includeExtendedHours bool) []types.KLine {
	if len(baseRows) == 0 {
		return nil
	}

	labelSince := alignTimeToIntervalStart(since, types.Interval1d)
	labelUntil := alignTimeToIntervalStart(until, types.Interval1d)
	aggregated := make([]types.KLine, 0, len(baseRows))
	var current types.KLine
	var currentLabel string
	currentVolume := fixedpoint.Zero

	flush := func() {
		if currentLabel == "" {
			return
		}
		current.Volume = currentVolume
		aggregated = append(aggregated, current)
	}

	for _, base := range baseRows {
		labelAt, ok := futu.TradingDayLabelStart(symbol, dailyAggregationObservedAt(base), includeExtendedHours)
		if !ok {
			continue
		}
		if labelAt.Before(labelSince) || labelAt.After(labelUntil) {
			continue
		}

		labelKey := labelAt.Format("2006-01-02")
		if currentLabel == "" || currentLabel != labelKey {
			flush()
			currentLabel = labelKey
			currentVolume = fixedpoint.Zero
			current = types.KLine{
				StartTime: types.Time(labelAt),
				EndTime:   types.Time(labelAt.Add(24 * time.Hour).Add(-time.Millisecond)),
				Interval:  types.Interval1d,
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
		}
		currentVolume = currentVolume.Add(base.Volume)
	}
	flush()
	return aggregated
}

func dailyAggregationObservedAt(kline types.KLine) time.Time {
	observedAt := kline.StartTime.Time().UTC()
	if observedAt.IsZero() {
		return kline.EndTime.Time().UTC()
	}
	return observedAt
}

func aggregateTradingPeriodKLinesFromBase(symbol string, interval types.Interval, baseRows []types.KLine, since, until time.Time, includeExtendedHours bool) []types.KLine {
	if len(baseRows) == 0 {
		return nil
	}

	unit := tradingPeriodUnit(interval)
	if unit == "" {
		return nil
	}

	aggregated := make([]types.KLine, 0, len(baseRows))
	var current types.KLine
	var currentLabel string
	currentVolume := fixedpoint.Zero
	flush := func() {
		if currentLabel == "" {
			return
		}
		endAt := current.EndTime.Time()
		if endAt.Before(since) || endAt.After(until) {
			return
		}
		current.Volume = currentVolume
		aggregated = append(aggregated, current)
	}

	for _, base := range baseRows {
		labelAt, ok := tradingPeriodLabelForBaseKLine(symbol, base, unit, includeExtendedHours)
		if !ok {
			continue
		}

		labelKey := labelAt.Format("2006-01-02")
		if currentLabel == "" || currentLabel != labelKey {
			flush()
			currentLabel = labelKey
			currentVolume = fixedpoint.Zero
			current = types.KLine{
				StartTime: types.Time(labelAt),
				EndTime:   types.Time(tradingPeriodLabelEnd(labelAt, interval)),
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
		}
		currentVolume = currentVolume.Add(base.Volume)
	}
	flush()
	return aggregated
}

func aggregateSessionAwareIntradayKLinesFromBase(symbol string, interval types.Interval, baseRows []types.KLine, since, until time.Time, includeExtendedHours bool) []types.KLine {
	if len(baseRows) == 0 {
		return nil
	}

	aggregated := make([]types.KLine, 0, len(baseRows))
	var current types.KLine
	var currentBucketStart time.Time
	var currentBucketEnd time.Time
	currentVolume := fixedpoint.Zero
	bucketDuration := interval.Duration()

	flush := func() {
		if currentBucketStart.IsZero() {
			return
		}
		endAt := current.EndTime.Time()
		if endAt.Before(since) || endAt.After(until) {
			return
		}
		current.Volume = currentVolume
		aggregated = append(aggregated, current)
	}

	for _, base := range baseRows {
		observedAt := dailyAggregationObservedAt(base)
		bucketStart, bucketEnd, ok := currentBucketStart, currentBucketEnd, !currentBucketStart.IsZero() && !observedAt.Before(currentBucketStart) && !observedAt.After(currentBucketEnd)
		if !ok {
			bucketStart, bucketEnd, ok = futu.SessionAwareIntradayBucketBounds(symbol, observedAt, bucketDuration, includeExtendedHours)
			if !ok {
				continue
			}
		}

		if currentBucketStart.IsZero() || !bucketStart.Equal(currentBucketStart) {
			flush()
			currentBucketStart = bucketStart
			currentBucketEnd = bucketEnd
			currentVolume = fixedpoint.Zero
			current = types.KLine{
				StartTime: types.Time(bucketStart),
				EndTime:   types.Time(bucketEnd),
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
		}
		currentVolume = currentVolume.Add(base.Volume)
	}
	flush()
	return aggregated
}

func tradingPeriodLabelForBaseKLine(symbol string, kline types.KLine, unit string, includeExtendedHours bool) (time.Time, bool) {
	if kline.Interval == types.Interval1d {
		return futu.TradingPeriodLabelStartForDate(symbol, kline.StartTime.Time(), unit)
	}
	return futu.TradingPeriodLabelStart(symbol, dailyAggregationObservedAt(kline), unit, includeExtendedHours)
}

func (s *FutuKLineStore) queryAggregatedTradingPeriodKLinesForwardLocked(symbol string, interval types.Interval, startTime time.Time, limit int) ([]types.KLine, error) {
	normalizedLimit := limit
	if normalizedLimit <= 0 {
		normalizedLimit = 1
	}

	labelStart, ok := futu.TradingPeriodLabelStartForDate(symbol, startTime, tradingPeriodUnit(interval))
	if !ok {
		return nil, nil
	}
	if tradingPeriodLabelEnd(labelStart, interval).Before(startTime) {
		labelStart = shiftTradingPeriodLabel(labelStart, interval, 1)
	}
	effectiveUntil := tradingPeriodLabelEnd(shiftTradingPeriodLabel(labelStart, interval, normalizedLimit-1), interval)
	source, err := s.resolveReadSource(symbol, interval, labelStart, effectiveUntil)
	if err != nil {
		return nil, err
	}
	if !source.synthesize {
		return nil, nil
	}
	aggregated, err := s.queryAggregatedKLinesInRange(symbol, interval, source.baseInterval, labelStart, effectiveUntil)
	if err != nil {
		return nil, err
	}
	if len(aggregated) > normalizedLimit {
		aggregated = aggregated[:normalizedLimit]
	}
	return aggregated, nil
}

func (s *FutuKLineStore) queryAggregatedTradingPeriodKLinesBackwardLocked(symbol string, interval types.Interval, endTime time.Time, limit int) ([]types.KLine, error) {
	normalizedLimit := limit
	if normalizedLimit <= 0 {
		normalizedLimit = 1
	}

	anchor := endTime.Add(-time.Millisecond)
	labelStart, ok := futu.TradingPeriodLabelStartForDate(symbol, anchor, tradingPeriodUnit(interval))
	if !ok {
		return nil, nil
	}
	if tradingPeriodLabelEnd(labelStart, interval).After(anchor) {
		labelStart = shiftTradingPeriodLabel(labelStart, interval, -1)
	}
	since := shiftTradingPeriodLabel(labelStart, interval, -(normalizedLimit - 1))
	effectiveUntil := tradingPeriodLabelEnd(labelStart, interval)
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
	if len(aggregated) > normalizedLimit {
		aggregated = aggregated[len(aggregated)-normalizedLimit:]
	}
	return aggregated, nil
}

func (s *FutuKLineStore) queryAggregatedSessionAwareIntradayKLinesForwardLocked(symbol string, interval types.Interval, startTime time.Time, limit int, includeExtendedHours bool) ([]types.KLine, error) {
	normalizedLimit := limit
	if normalizedLimit <= 0 {
		normalizedLimit = 1
	}

	rows := make([]types.KLine, 0, normalizedLimit)
	cursor := startTime
	lastStart := time.Time{}
	for len(rows) < normalizedLimit {
		batch, err := s.querySessionAwareIntradayKLinesInRangeLocked(symbol, interval, cursor, cursor.Add(14*24*time.Hour), includeExtendedHours)
		if err != nil {
			return nil, err
		}
		if len(batch) == 0 {
			break
		}

		appended := 0
		for _, row := range batch {
			if row.EndTime.Time().Before(startTime) {
				continue
			}
			if !lastStart.IsZero() && row.StartTime.Time().Equal(lastStart) {
				continue
			}
			rows = append(rows, row)
			lastStart = row.StartTime.Time()
			appended++
			if len(rows) == normalizedLimit {
				return rows, nil
			}
		}

		nextCursor := batch[len(batch)-1].EndTime.Time().Add(time.Millisecond)
		if appended == 0 || !nextCursor.After(cursor) {
			break
		}
		cursor = nextCursor
	}
	return rows, nil
}

func (s *FutuKLineStore) queryAggregatedSessionAwareIntradayKLinesBackwardLocked(symbol string, interval types.Interval, endTime time.Time, limit int, includeExtendedHours bool) ([]types.KLine, error) {
	normalizedLimit := limit
	if normalizedLimit <= 0 {
		normalizedLimit = 1
	}

	rows := make([]types.KLine, 0, normalizedLimit)
	cursor := endTime.Add(-time.Millisecond)
	for len(rows) < normalizedLimit {
		batch, err := s.querySessionAwareIntradayKLinesInRangeLocked(symbol, interval, cursor.Add(-14*24*time.Hour), cursor, includeExtendedHours)
		if err != nil {
			return nil, err
		}
		if len(batch) == 0 {
			break
		}

		appended := 0
		prepend := make([]types.KLine, 0, normalizedLimit-len(rows))
		for index := len(batch) - 1; index >= 0 && len(rows)+len(prepend) < normalizedLimit; index-- {
			row := batch[index]
			if row.EndTime.Time().After(cursor) {
				continue
			}
			if len(rows) > 0 && row.StartTime.Time().Equal(rows[0].StartTime.Time()) {
				continue
			}
			prepend = append(prepend, row)
			appended++
		}
		if len(prepend) > 0 {
			reverseKLines(prepend)
			rows = append(prepend, rows...)
		}

		nextCursor := batch[0].StartTime.Time().Add(-time.Millisecond)
		if appended == 0 || !nextCursor.Before(cursor) {
			break
		}
		cursor = nextCursor
	}
	if len(rows) > normalizedLimit {
		rows = rows[len(rows)-normalizedLimit:]
	}
	return rows, nil
}

func isTradingPeriodAggregationInterval(interval types.Interval) bool {
	switch interval {
	case types.Interval1d, types.Interval1w, types.Interval1mo:
		return true
	default:
		return false
	}
}

func tradingPeriodUnit(interval types.Interval) string {
	switch interval {
	case types.Interval1d:
		return "day"
	case types.Interval1w:
		return "week"
	case types.Interval1mo:
		return "month"
	default:
		return ""
	}
}

func tradingPeriodLabelEnd(labelAt time.Time, interval types.Interval) time.Time {
	switch interval {
	case types.Interval1d:
		return labelAt.AddDate(0, 0, 1).Add(-time.Millisecond)
	case types.Interval1w:
		return labelAt.AddDate(0, 0, 7).Add(-time.Millisecond)
	case types.Interval1mo:
		return labelAt.AddDate(0, 1, 0).Add(-time.Millisecond)
	default:
		return labelAt
	}
}

func shiftTradingPeriodLabel(labelAt time.Time, interval types.Interval, steps int) time.Time {
	switch interval {
	case types.Interval1d:
		return labelAt.AddDate(0, 0, steps)
	case types.Interval1w:
		return labelAt.AddDate(0, 0, 7*steps)
	case types.Interval1mo:
		return labelAt.AddDate(0, steps, 0)
	default:
		return labelAt
	}
}

func shouldUseSessionAwareIntradayAggregation(symbol string, interval types.Interval) bool {
	if !isSessionAwareIntradayAggregationInterval(interval) {
		return false
	}
	_, ok := futu.TradingProfileForSymbol(symbol)
	return ok
}

func isSessionAwareIntradayAggregationInterval(interval types.Interval) bool {
	duration := interval.Duration()
	return duration > time.Hour && duration < 24*time.Hour
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
	currentVolume := fixedpoint.Zero

	flush := func() {
		if currentCount != factor {
			return
		}
		endAt := current.EndTime.Time()
		if endAt.Before(since) || endAt.After(until) {
			return
		}
		current.Volume = currentVolume
		aggregated = append(aggregated, current)
	}

	for _, base := range baseRows {
		bucketStart := alignTimeToIntervalStart(base.StartTime.Time(), interval)
		if currentCount == 0 || !bucketStart.Equal(currentBucketStart) {
			flush()
			currentBucketStart = bucketStart
			currentCount = 0
			currentVolume = fixedpoint.Zero
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
		currentVolume = currentVolume.Add(base.Volume)
	}
	flush()
	return aggregated
}

// --- Helpers ---
