package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/jftrade/jftrade-main/pkg/bbgo/types"
)

func (s *FutuKLineStore) selectReadTableName(symbol string, interval types.Interval, rehabType string, since, until time.Time) (string, error) {
	var firstExisting string
	tableNames, tableCount := s.readTableNames(symbol, interval, rehabType)
	for index := range tableCount {
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
	for index := range tableCount {
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
	err := s.db.QueryRowContext(context.Background(),
		`SELECT end_time FROM `+quoteIdentifier(tableName)+` WHERE end_time >= ? ORDER BY end_time ASC LIMIT 1`,
		timeToUnixMillis(at),
	).Scan(&endTimeMillis)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

func (s *FutuKLineStore) hasKLineEndingAtOrBefore(tableName string, at time.Time) (bool, error) {
	var endTimeMillis int64
	err := s.db.QueryRowContext(context.Background(),
		`SELECT end_time FROM `+quoteIdentifier(tableName)+` WHERE end_time <= ? ORDER BY end_time DESC LIMIT 1`,
		timeToUnixMillis(at),
	).Scan(&endTimeMillis)
	if errors.Is(err, sql.ErrNoRows) {
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
	err := s.db.QueryRowContext(context.Background(),
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
	err = s.db.QueryRowContext(context.Background(),
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
