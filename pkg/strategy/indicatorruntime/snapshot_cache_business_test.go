package indicatorruntime

import "testing"

func TestSnapshotSeriesCacheCoversAdditionalMovingAverageSequences(t *testing.T) {
	values := []float64{1, 2, 3, 4, 5, 6, 7, 8, 9}
	var nilCache *snapshotSeriesCache
	for _, tc := range []struct {
		name string
		seq  []float64
	}{
		{name: "nil smma", seq: nilCache.getSMMASequence(values, 3)},
		{name: "nil wma", seq: nilCache.getWMASequence(values, 3)},
		{name: "nil tma", seq: nilCache.getTMASequence(values, 3)},
		{name: "nil hma", seq: nilCache.getHMASequence(values, 4)},
	} {
		if len(tc.seq) == 0 {
			t.Fatalf("%s sequence is empty", tc.name)
		}
	}

	cache := newSnapshotSeriesCache()
	assertCachedSequenceReuse(t, "smma", cache.getSMMASequence(values, 3), cache.getSMMASequence(values, 3))
	assertCachedSequenceReuse(t, "wma", cache.getWMASequence(values, 3), cache.getWMASequence(values, 3))
	assertCachedSequenceReuse(t, "tma", cache.getTMASequence(values, 3), cache.getTMASequence(values, 3))
	assertCachedSequenceReuse(t, "hma", cache.getHMASequence(values, 4), cache.getHMASequence(values, 4))

	if cache.getMovingAverageSnapshot(movingAverageConfig{averageType: "EMA", period: 3}, 0, 0, false, false) != nil {
		t.Fatal("moving average snapshot without current or previous value = non-nil")
	}
	if cache.getWindowSnapshot(windowConfig{function: "highest", source: "high", period: 3}, 0, 0, false, false) != nil {
		t.Fatal("window snapshot without current or previous value = non-nil")
	}
	if cache.getScalarSnapshot("empty", 0, false) != nil {
		t.Fatal("scalar snapshot without current value = non-nil")
	}

	cache.reset()
	if len(cache.smma) != 0 || len(cache.wma) != 0 || len(cache.tma) != 0 || len(cache.hma) != 0 {
		t.Fatalf("reset did not clear additional MA caches: smma=%d wma=%d tma=%d hma=%d", len(cache.smma), len(cache.wma), len(cache.tma), len(cache.hma))
	}
}

func TestSnapshotSeriesCacheClearsPreviousStateWhenSeriesShrinks(t *testing.T) {
	cache := newSnapshotSeriesCache()
	macdCfg := macdConfig{fastPeriod: 3, slowPeriod: 5, signalPeriod: 2}
	if cache.getMACDSnapshot(macdCfg, macdSeries{}) != nil {
		t.Fatal("empty MACD series snapshot = non-nil")
	}
	macdFirst := cache.getMACDSnapshotValues(macdCfg, 3, 2, 0, 0, true, false).(*indicatorMACDSnapshot)
	if macdFirst.hasPrevious {
		t.Fatalf("single MACD snapshot hasPrevious = true: %#v", macdFirst)
	}
	macdSecond := cache.getMACDSnapshotValues(macdCfg, 5, 3, 3, 2, true, true).(*indicatorMACDSnapshot)
	if macdSecond != macdFirst || !macdSecond.hasPrevious || macdSecond.previousHistogram != 2 {
		t.Fatalf("MACD snapshot reuse/previous = %#v %#v", macdFirst, macdSecond)
	}
	macdThird := cache.getMACDSnapshotValues(macdCfg, 7, 4, 0, 0, true, false).(*indicatorMACDSnapshot)
	if macdThird != macdFirst || macdThird.hasPrevious || macdThird.previousDiff != 0 || macdThird.previousHistogram != 0 {
		t.Fatalf("MACD snapshot did not clear previous values: %#v", macdThird)
	}
	if cache.getMACDSnapshotValues(macdCfg, 0, 0, 0, 0, false, false) != nil {
		t.Fatal("MACD snapshot without current value = non-nil")
	}

	kdjCfg := kdjConfig{period: 3, m1: 3, m2: 3}
	if cache.getKDJSnapshot(kdjCfg, kdjSeries{k: []float64{1}, d: nil, j: []float64{1}}) != nil {
		t.Fatal("incomplete KDJ series snapshot = non-nil")
	}
	kdjSingle := cache.getKDJSnapshot(kdjCfg, kdjSeries{k: []float64{1}, d: []float64{2}, j: []float64{3}}).(*indicatorKDJSnapshot)
	if kdjSingle.hasPrevious {
		t.Fatalf("single KDJ snapshot hasPrevious = true: %#v", kdjSingle)
	}
	kdjFull := cache.getKDJSnapshot(kdjCfg, kdjSeries{k: []float64{1, 4}, d: []float64{2, 5}, j: []float64{3, 6}}).(*indicatorKDJSnapshot)
	if kdjFull != kdjSingle || !kdjFull.hasPrevious || kdjFull.previousK != 1 || kdjFull.previousD != 2 || kdjFull.previousJ != 3 {
		t.Fatalf("KDJ snapshot reuse/previous = %#v %#v", kdjSingle, kdjFull)
	}
	kdjReset := cache.getKDJSnapshotValues(kdjCfg, 7, 8, 9, 0, 0, 0, true, false).(*indicatorKDJSnapshot)
	if kdjReset != kdjSingle || kdjReset.hasPrevious || kdjReset.previousK != 0 || kdjReset.previousD != 0 || kdjReset.previousJ != 0 {
		t.Fatalf("KDJ snapshot did not clear previous values: %#v", kdjReset)
	}
	if cache.getKDJSnapshotValues(kdjCfg, 0, 0, 0, 0, 0, 0, false, false) != nil {
		t.Fatal("KDJ snapshot without current value = non-nil")
	}
}

func TestBuildSnapshotKeyCacheCoversIndicatorFamiliesAndLegacyMA(t *testing.T) {
	legacyMA := movingAverageConfig{averageType: "MA", period: 20}
	sourceMA := movingAverageConfig{averageType: "EMA", period: 9, timeUnit: "day", source: "hlc3"}
	security := securitySourceConfig{source: "close", timeUnit: "week", lookback: 1}
	rsiSource := sourcePeriodConfig{source: "hlc3", period: 14}
	macd := macdConfig{fastPeriod: 12, slowPeriod: 26, signalPeriod: 9}
	bollinger := bollingerConfig{period: 20, multiplier: 2}
	kdj := kdjConfig{period: 9, m1: 3, m2: 3}
	stdevSource := sourcePeriodConfig{source: "volume", period: 11}
	variance := sourcePeriodConfig{source: "volume", period: 8}
	window := windowConfig{function: "highest", source: "high", period: 10}
	cum := sourceConfig{source: "volume"}
	stoch := sourcePeriodConfig{source: "close", period: 14, timeUnit: "day"}
	cciSource := sourcePeriodConfig{source: "close", period: 20}
	vwap := sourceConfig{source: "hlc3"}
	mfi := sourcePeriodConfig{source: "hlc3", period: 15}
	dmi := dmiConfig{diLength: 14, adxSmoothing: 14}
	supertrend := supertrendConfig{factor: 3, atrPeriod: 10}
	sar := sarConfig{start: 0.02, increment: 0.02, maximum: 0.2}
	stopLoss := stopLossConfig{mode: "stopLoss", direction: "long", timeValue: 2, percentage: 4, windowPolicy: "continuous"}
	rsiDiv := rsiDivergenceConfig{period: 14, direction: "top", lookback: 5}
	macdDiv := macdDivergenceConfig{fastPeriod: 12, slowPeriod: 26, signalPeriod: 9, direction: "bottom", lookback: 6}
	kdjDiv := kdjDivergenceConfig{period: 9, m1: 3, m2: 3, direction: "top", lookback: 4}
	advanced := advancedIndicatorConfig{key: "atr:14:day", kind: "atr", period: 14, timeUnit: "day"}

	cache := buildSnapshotKeyCache(indicatorRequirements{
		ma:             []movingAverageConfig{legacyMA, sourceMA},
		securitySource: []securitySourceConfig{security},
		rsi:            []int{14},
		rsiSource:      []sourcePeriodConfig{rsiSource},
		macd:           []macdConfig{macd},
		bollinger:      []bollingerConfig{bollinger},
		kdj:            []kdjConfig{kdj},
		atr:            []int{14},
		stdev:          []int{20},
		stdevSource:    []sourcePeriodConfig{stdevSource},
		variance:       []sourcePeriodConfig{variance},
		windows:        []windowConfig{window},
		cum:            []sourceConfig{cum},
		stoch:          []sourcePeriodConfig{stoch},
		cci:            []int{20},
		cciSource:      []sourcePeriodConfig{cciSource},
		williamsR:      []int{14},
		vwap:           []sourceConfig{vwap},
		mfi:            []sourcePeriodConfig{mfi},
		dmi:            []dmiConfig{dmi},
		supertrend:     []supertrendConfig{supertrend},
		sar:            []sarConfig{sar},
		stopLoss:       []stopLossConfig{stopLoss},
		rsiDivergence:  []rsiDivergenceConfig{rsiDiv},
		macdDivergence: []macdDivergenceConfig{macdDiv},
		kdjDivergence:  []kdjDivergenceConfig{kdjDiv},
		advanced:       []advancedIndicatorConfig{advanced},
	})

	expected := map[string]string{
		"legacy ma":    cache.maLegacy[legacyMA],
		"source ma":    cache.ma[sourceMA],
		"security":     cache.securitySource[security],
		"rsi":          cache.rsi[14],
		"rsi source":   cache.rsiSource[rsiSource],
		"macd":         cache.macd[macd],
		"bollinger":    cache.bollinger[bollinger],
		"kdj":          cache.kdj[kdj],
		"atr":          cache.atr[14],
		"stdev":        cache.stdev[20],
		"stdev source": cache.stdevSource[stdevSource],
		"variance":     cache.variance[variance],
		"window":       cache.windows[window],
		"cum":          cache.cum[cum],
		"stoch":        cache.stoch[stoch],
		"cci":          cache.cci[20],
		"cci source":   cache.cciSource[cciSource],
		"williams":     cache.williamsR[14],
		"vwap":         cache.vwap[vwap],
		"mfi":          cache.mfi[mfi],
		"dmi":          cache.dmi[dmi],
		"supertrend":   cache.supertrend[supertrend],
		"sar":          cache.sar[sar],
		"stop loss":    cache.stopLoss[stopLoss],
		"rsi div":      cache.rsiDivergence[rsiDiv],
		"macd div":     cache.macdDivergence[macdDiv],
		"kdj div":      cache.kdjDivergence[kdjDiv],
		"advanced":     cache.advanced[advanced],
	}
	for name, key := range expected {
		if key == "" {
			t.Fatalf("%s key is empty in snapshot key cache", name)
		}
	}
	if cache.maLegacy[legacyMA] != "ma:20" {
		t.Fatalf("legacy MA key = %q, want ma:20", cache.maLegacy[legacyMA])
	}
	if cache.atr[14] != "atr:14" || cache.stdev[20] != "stdev:20" || cache.kdjDivergence[kdjDiv] != "divergence:kdj:9:3:3:top:4" {
		t.Fatalf("key cache generated unexpected keys: atr=%q stdev=%q kdjDiv=%q", cache.atr[14], cache.stdev[20], cache.kdjDivergence[kdjDiv])
	}
	if cache.resultCapacity <= len(expected) {
		t.Fatalf("result capacity = %d, want legacy MA to add extra capacity over %d keys", cache.resultCapacity, len(expected))
	}
}

func TestIndicatorSnapshotFieldReadersCoverNilAndPreviousFallbacks(t *testing.T) {
	var scalar *indicatorScalarSnapshot
	if value, ok := scalar.ScalarValue(); ok || value != 0 {
		t.Fatalf("nil scalar ScalarValue = %v/%v, want 0/false", value, ok)
	}
	scalar = &indicatorScalarSnapshot{current: 4.5, hasCurrent: true}
	if value, ok := scalar.ScalarValue(); !ok || value != 4.5 {
		t.Fatalf("scalar ScalarValue = %v/%v, want 4.5/true", value, ok)
	}

	series := &indicatorSeriesSnapshot{}
	if value, ok := series.PreferredScalarValue(); ok || value != 0 {
		t.Fatalf("empty series PreferredScalarValue = %v/%v, want 0/false", value, ok)
	}
	if _, _, _, _, ok := series.SeriesField("close"); ok {
		t.Fatal("series accepted unsupported field")
	}
	if value, ok := series.FieldValue("value"); !ok || value != nil {
		t.Fatalf("series value without current = %#v/%v, want nil/true", value, ok)
	}
	if _, ok := series.FieldValue("unknown"); ok {
		t.Fatal("series accepted unknown field")
	}

	macd := &indicatorMACDSnapshot{diff: 2, signal: 1, histogram: 2}
	if value, ok := macd.PreferredScalarValue(); !ok || value != 2 {
		t.Fatalf("MACD PreferredScalarValue = %v/%v, want 2/true", value, ok)
	}
	if current, previous, currentOK, previousOK, fieldOK := macd.SeriesField("signal"); !fieldOK || !currentOK || previousOK || current != 1 || previous != 0 {
		t.Fatalf("MACD signal field = %v %v %v %v %v", current, previous, currentOK, previousOK, fieldOK)
	}
	if value, ok := macd.FieldValue("previousDiff"); !ok || value != nil {
		t.Fatalf("MACD previousDiff without previous = %#v/%v, want nil/true", value, ok)
	}
	if _, ok := macd.FieldValue("unknown"); ok {
		t.Fatal("MACD accepted unknown field")
	}

	kdj := &indicatorKDJSnapshot{k: 10, d: 8, j: 14}
	if value, ok := kdj.PreferredScalarValue(); !ok || value != 10 {
		t.Fatalf("KDJ PreferredScalarValue = %v/%v, want 10/true", value, ok)
	}
	if current, previous, currentOK, previousOK, fieldOK := kdj.SeriesField("j"); !fieldOK || !currentOK || previousOK || current != 14 || previous != 0 {
		t.Fatalf("KDJ j field = %v %v %v %v %v", current, previous, currentOK, previousOK, fieldOK)
	}
	if value, ok := kdj.FieldValue("previousK"); !ok || value != nil {
		t.Fatalf("KDJ previousK without previous = %#v/%v, want nil/true", value, ok)
	}
	if _, ok := kdj.FieldValue("unknown"); ok {
		t.Fatal("KDJ accepted unknown field")
	}
}

func assertCachedSequenceReuse(t *testing.T, name string, first []float64, second []float64) {
	t.Helper()
	if len(first) == 0 || len(second) == 0 {
		t.Fatalf("%s cached sequences must be non-empty: first=%#v second=%#v", name, first, second)
	}
	if &first[0] != &second[0] {
		t.Fatalf("%s sequence was not reused", name)
	}
}
