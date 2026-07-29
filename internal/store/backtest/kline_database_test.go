package backtest

import (
	"path/filepath"
	"testing"
	"time"
)

func TestCheckKLineCoverageOwnsConcreteHistoryStore(t *testing.T) {
	start := time.Date(2026, time.January, 2, 14, 30, 0, 0, time.UTC)
	err := CheckKLineCoverage(
		filepath.Join(t.TempDir(), "backtest.db"),
		"US.AAPL",
		"1m",
		start,
		start.Add(time.Hour),
		"forward",
		"regular",
	)
	if err == nil {
		t.Fatal("CheckKLineCoverage(empty database) error = nil")
	}
}
