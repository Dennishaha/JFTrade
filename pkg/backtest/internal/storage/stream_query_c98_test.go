package storage

import (
	"strings"
	"testing"
	"time"

	"github.com/jftrade/jftrade-main/pkg/bbgo/types"
)

func TestCoverage98StoredStreamFailuresRemainVisibleToAllQueryShapes(t *testing.T) {
	start := time.Date(2026, time.June, 15, 1, 30, 0, 0, time.UTC)
	until := start.Add(2*time.Minute - time.Millisecond)

	t.Run("single channel streaming exposes a damaged direct table", func(t *testing.T) {
		store := newTestKLineStore(t)
		seedEndTimeOnlyKLineCoverage(t, store, "TEST.CORRUPT.DIRECT", start, until)

		rows, errCh := store.QueryKLinesCh(start, until, nil, []string{"TEST.CORRUPT.DIRECT"}, []types.Interval{types.Interval1m})
		for row := range rows {
			t.Fatalf("damaged direct table emitted a row: %#v", row)
		}
		var streamErr error
		for err := range errCh {
			streamErr = err
		}
		if streamErr == nil || !strings.Contains(streamErr.Error(), "start_time") {
			t.Fatalf("single direct stream error = %v, want malformed-row error", streamErr)
		}
	})

	t.Run("backward and multi-symbol reads do not hide damaged direct rows", func(t *testing.T) {
		store := newTestKLineStore(t)
		seedEndTimeOnlyKLineCoverage(t, store, "TEST.CORRUPT.BACKWARD", start, until)

		if _, err := store.QueryKLinesBackward(nil, "TEST.CORRUPT.BACKWARD", types.Interval1m, until, 2); err == nil || !strings.Contains(err.Error(), "start_time") {
			t.Fatalf("backward malformed-row error = %v", err)
		}
		if err := store.StreamKLines(start, until, nil, []string{"TEST.CORRUPT.BACKWARD", "TEST.UNRELATED"}, []types.Interval{types.Interval1m}, func(types.KLine) {
			t.Fatal("multi-symbol stream emitted a row from a damaged table")
		}); err == nil || !strings.Contains(err.Error(), "start_time") {
			t.Fatalf("multi-symbol malformed-row error = %v", err)
		}
	})
}

func TestCoverage98MultiSymbolStreamUsesSymbolAsFinalStableSortKey(t *testing.T) {
	store := newTestKLineStore(t)
	start := time.Date(2026, time.June, 15, 1, 30, 0, 0, time.UTC)
	bars := []types.KLine{
		testKLine("TEST.ZETA", types.Interval1m, start, time.Minute, 100, 101, 99, 100.5, 10),
		testKLine("TEST.ALPHA", types.Interval1m, start, time.Minute, 200, 201, 199, 200.5, 20),
	}
	if err := store.InsertKLines(bars, "forward"); err != nil {
		t.Fatalf("InsertKLines: %v", err)
	}

	var streamed []types.KLine
	if err := store.StreamKLines(start, start.Add(time.Minute-time.Millisecond), nil, []string{"TEST.ZETA", "TEST.ALPHA"}, []types.Interval{types.Interval1m}, func(kline types.KLine) {
		streamed = append(streamed, kline)
	}); err != nil {
		t.Fatalf("StreamKLines: %v", err)
	}
	if len(streamed) != 2 || streamed[0].Symbol != "TEST.ALPHA" || streamed[1].Symbol != "TEST.ZETA" {
		t.Fatalf("stable multi-symbol order = %#v", streamed)
	}
}
