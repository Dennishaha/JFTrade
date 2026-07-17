package storage

import (
	"strings"
	"testing"
	"time"

	"github.com/jftrade/jftrade-main/pkg/bbgo/types"
)

// These cases model a partially restored K-line database. Read selection only
// needs end_time coverage, but the eventual reader must still surface a broken
// table schema rather than silently reporting a coverage miss or an empty bar
// list to a backtest caller.
func TestCoverage98DailyAggregationPropagatesDamagedStorageErrors(t *testing.T) {
	dayStart := time.Date(2026, time.June, 15, 0, 0, 0, 0, time.UTC)
	dayEnd := dayStart.Add(24 * time.Hour).Add(-time.Millisecond)

	t.Run("extended-hours fallback does not hide a damaged stored daily table", func(t *testing.T) {
		store := newTestKLineStore(t)
		table := store.writeTableName("US.DAMAGED_DAILY", types.Interval1d, "forward")
		if _, err := store.db.Exec(`CREATE TABLE ` + quoteIdentifier(table) + ` (end_time INTEGER PRIMARY KEY)`); err != nil {
			t.Fatalf("create damaged daily table: %v", err)
		}
		if _, err := store.db.Exec(`INSERT INTO `+quoteIdentifier(table)+` (end_time) VALUES (?)`, timeToUnixMillis(dayEnd)); err != nil {
			t.Fatalf("seed damaged daily coverage: %v", err)
		}

		_, err := store.QueryDailyKLinesInRange("US.DAMAGED_DAILY", dayStart, dayEnd, true)
		if err == nil || strings.Contains(err.Error(), "missing K-line coverage") {
			t.Fatalf("damaged daily fallback error = %v, want schema error", err)
		}
	})

	t.Run("base-interval coverage lookup reports a damaged schema", func(t *testing.T) {
		store := newTestKLineStore(t)
		table := store.writeTableName("US.DAMAGED_BASE", types.Interval12h, "forward")
		if _, err := store.db.Exec(`CREATE TABLE ` + quoteIdentifier(table) + ` (not_a_kline INTEGER)`); err != nil {
			t.Fatalf("create damaged base table: %v", err)
		}

		_, err := store.QueryDailyKLinesInRange("US.DAMAGED_BASE", dayStart, dayEnd, false)
		if err == nil || strings.Contains(err.Error(), "missing K-line coverage") {
			t.Fatalf("damaged base coverage error = %v, want schema error", err)
		}
	})
}
