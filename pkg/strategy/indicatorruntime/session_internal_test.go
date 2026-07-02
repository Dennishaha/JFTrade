package indicatorruntime

import (
	"testing"
	"time"

	"github.com/jftrade/jftrade-main/pkg/market"
)

func TestResolveSessionAwareWindowStartWithCacheTracksBoundariesAndSeriesLength(t *testing.T) {
	base := time.Date(2026, time.June, 23, 13, 30, 0, 0, time.UTC)
	cache := newSnapshotSeriesCache()

	endTimes := []time.Time{
		base,
		base.Add(time.Minute),
		base.Add(2 * time.Minute),
	}
	sessions := []market.Session{
		market.SessionRegular,
		market.SessionRegular,
		market.SessionPre,
	}

	if got := resolveSessionAwareWindowStartWithCache(endTimes, sessions, 0, 1, cache); got != -1 {
		t.Fatalf("resolveSessionAwareWindowStartWithCache(boundary) = %d, want -1", got)
	}
	if !cache.stopLossWindowStart.valid || cache.stopLossWindowStart.resolvedStart != -1 || cache.stopLossWindowStart.seriesLength != 3 {
		t.Fatalf("stopLossWindowStart cache = %+v", cache.stopLossWindowStart)
	}

	endTimes = append(endTimes, base.Add(3*time.Minute))
	sessions = []market.Session{
		market.SessionRegular,
		market.SessionRegular,
		market.SessionRegular,
		market.SessionRegular,
	}
	if got := resolveSessionAwareWindowStartWithCache(endTimes, sessions, 0, 1, cache); got != 0 {
		t.Fatalf("resolveSessionAwareWindowStartWithCache(recomputed) = %d, want 0", got)
	}
	if cache.stopLossWindowStart.seriesLength != 4 || cache.stopLossWindowStart.resolvedStart != 0 {
		t.Fatalf("stopLossWindowStart cache after recompute = %+v", cache.stopLossWindowStart)
	}
}

func TestMaxMinSliceFromWindowStartWithCacheInvalidatesOnSeriesGrowth(t *testing.T) {
	cache := newSnapshotSeriesCache()
	closes := []float64{10, 12, 9, 11}

	peak, trough := maxMinSliceFromWindowStartWithCache(closes, 1, cache)
	if peak != 12 || trough != 9 {
		t.Fatalf("maxMinSliceFromWindowStartWithCache(initial) = %v, %v", peak, trough)
	}
	if !cache.stopLossWindowExtrema.valid || cache.stopLossWindowExtrema.windowStart != 1 || cache.stopLossWindowExtrema.seriesLength != 4 {
		t.Fatalf("stopLossWindowExtrema cache = %+v", cache.stopLossWindowExtrema)
	}

	closes = append(closes, 20)
	peak, trough = maxMinSliceFromWindowStartWithCache(closes, 1, cache)
	if peak != 20 || trough != 9 {
		t.Fatalf("maxMinSliceFromWindowStartWithCache(grown) = %v, %v", peak, trough)
	}
	if cache.stopLossWindowExtrema.seriesLength != 5 || cache.stopLossWindowExtrema.peakClose != 20 {
		t.Fatalf("stopLossWindowExtrema cache after recompute = %+v", cache.stopLossWindowExtrema)
	}
}

func TestStopLossTradingWindowSelectionCacheAndTrailingSnapshot(t *testing.T) {
	cache := newSnapshotSeriesCache()
	closes := []float64{100, 80, 90, 85}
	endTimes := []time.Time{
		time.Date(2026, time.May, 28, 19, 59, 59, 0, time.UTC),
		time.Date(2026, time.May, 28, 21, 0, 0, 0, time.UTC),
		time.Date(2026, time.May, 29, 14, 0, 0, 0, time.UTC),
		time.Date(2026, time.May, 29, 19, 30, 0, 0, time.UTC),
	}

	selected := selectStopLossTradingWindowIndicesWithCache(endTimes, 2, "day", "US.AAPL", len(closes), false, cache)
	if len(selected) < 2 {
		t.Fatalf("selected trading window indices = %#v, want at least current and reference", selected)
	}
	selectedAgain := selectStopLossTradingWindowIndicesWithCache(endTimes, 2, "day", "US.AAPL", len(closes), false, cache)
	if len(selectedAgain) != len(selected) || &selectedAgain[0] != &selected[0] {
		t.Fatalf("selection cache was not reused: first=%#v second=%#v", selected, selectedAgain)
	}

	snapshot := buildStopLossSnapshotForSymbolWithOptionsAndCache(
		closes,
		endTimes,
		nil,
		stopLossConfig{mode: "trailingStop", direction: "long", timeValue: 2, timeUnit: "day", percentage: 5, windowPolicy: "continuous"},
		1,
		"US.AAPL",
		false,
		cache,
	)
	if snapshot == nil {
		t.Fatal("expected trading-window trailing stop snapshot")
	}
	if !readSnapshotBool(t, snapshot, "longTriggered") {
		t.Fatalf("expected long trailing stop trigger, got %#v", snapshot)
	}
	if readSnapshotBool(t, snapshot, "shortTriggered") {
		t.Fatalf("did not expect short trailing trigger, got %#v", snapshot)
	}

	shorter := selectStopLossTradingWindowIndicesWithCache(endTimes, 2, "day", "US.AAPL", len(closes)-1, false, cache)
	if cache.stopLossWindowSelect.upperBound != len(closes)-1 {
		t.Fatalf("selection cache did not recompute for upperBound change: %+v", cache.stopLossWindowSelect)
	}
	if len(shorter) == len(selected) && len(shorter) > 0 && shorter[0] == selected[0] {
		t.Fatalf("selection with shorter upperBound did not change: first=%#v shorter=%#v", selected, shorter)
	}

	uncached := selectStopLossTradingWindowIndicesWithCache(endTimes, 2, "day", "US.AAPL", len(closes), false, nil)
	if len(uncached) != len(selected) {
		t.Fatalf("uncached selection = %#v, want same length as cached %#v", uncached, selected)
	}
}

func TestSessionAwareHelpersHandleTimeBreaksAndSparseSelections(t *testing.T) {
	base := time.Date(2026, time.June, 23, 13, 30, 0, 0, time.UTC)

	if got := sessionAwareSeriesLength(
		[]time.Time{base, base.Add(time.Minute)},
		[]market.Session{market.SessionRegular, market.SessionRegular, market.SessionAfter},
	); got != 3 {
		t.Fatalf("sessionAwareSeriesLength() = %d, want 3", got)
	}

	if got := resolveSessionAwareWindowStart(nil, nil, -1, 1); got != -1 {
		t.Fatalf("resolveSessionAwareWindowStart(negative) = %d, want -1", got)
	}
	if got := resolveSessionAwareWindowStart([]time.Time{base}, []market.Session{market.SessionRegular}, 0, tradingSessionMinutesPerDay); got != 0 {
		t.Fatalf("resolveSessionAwareWindowStart(daily interval) = %d, want 0", got)
	}
	if got := resolveSessionAwareWindowStart(
		[]time.Time{base, base.Add(5 * time.Minute)},
		[]market.Session{market.SessionRegular, market.SessionRegular},
		0,
		1,
	); got != -1 {
		t.Fatalf("resolveSessionAwareWindowStart(time break) = %d, want -1", got)
	}

	if !isSessionBoundary(market.SessionRegular, market.SessionAfter, base, base.Add(time.Minute), 1) {
		t.Fatal("isSessionBoundary(session change) = false, want true")
	}
	if isSessionBreak(time.Time{}, base, 1) {
		t.Fatal("isSessionBreak(zero previous) = true, want false")
	}
	if !isSessionBreak(base, base, 1) {
		t.Fatal("isSessionBreak(non-increasing) = false, want true")
	}
	if !isSessionBreak(base, base.Add(3*time.Minute), 1) {
		t.Fatal("isSessionBreak(large gap) = false, want true")
	}
	if isSessionBreak(base, base.Add(2*time.Minute), 1) {
		t.Fatal("isSessionBreak(double expected gap) = true, want false")
	}

	if peak, trough := maxMinSelectedCloses([]float64{10, 20, 5, 30}, []int{1, 3}); peak != 30 || trough != 20 {
		t.Fatalf("maxMinSelectedCloses(sparse) = %v, %v", peak, trough)
	}
	if peak, trough := maxMinSelectedCloses([]float64{10, 20}, nil); peak != 0 || trough != 0 {
		t.Fatalf("maxMinSelectedCloses(empty) = %v, %v", peak, trough)
	}
	if got := readMarketSessionAt([]market.Session{market.SessionRegular}, 3); got != market.SessionUnknown {
		t.Fatalf("readMarketSessionAt(out of range) = %q", got)
	}
	if got := readTimeAt([]time.Time{base}, 2); !got.IsZero() {
		t.Fatalf("readTimeAt(out of range) = %v, want zero", got)
	}
	if peak, trough := maxMinSlice(nil); peak != 0 || trough != 0 {
		t.Fatalf("maxMinSlice(empty) = %v, %v", peak, trough)
	}
}
