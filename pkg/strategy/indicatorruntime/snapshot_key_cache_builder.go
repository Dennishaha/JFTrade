package indicatorruntime

import "strconv"

type snapshotKeyCacheBuilder struct {
	requirements indicatorRequirements
	cache        snapshotKeyCache
}

func newSnapshotKeyCacheBuilder(requirements indicatorRequirements) *snapshotKeyCacheBuilder {
	return &snapshotKeyCacheBuilder{
		requirements: requirements,
		cache: snapshotKeyCache{
			resultCapacity: snapshotResultCapacity(requirements),
		},
	}
}

func snapshotResultCapacity(requirements indicatorRequirements) int {
	return len(requirements.ma) + len(requirements.securitySource) + len(requirements.rsi) + len(requirements.rsiSource) +
		len(requirements.macd) + len(requirements.bollinger) + len(requirements.kdj) + len(requirements.atr) +
		len(requirements.stdev) + len(requirements.stdevSource) + len(requirements.variance) + len(requirements.windows) +
		len(requirements.cum) + len(requirements.stoch) + len(requirements.cci) + len(requirements.cciSource) +
		len(requirements.williamsR) + len(requirements.vwap) + len(requirements.mfi) + len(requirements.dmi) +
		len(requirements.supertrend) + len(requirements.sar) + len(requirements.stopLoss) + len(requirements.rsiDivergence) +
		len(requirements.macdDivergence) + len(requirements.kdjDivergence) + len(requirements.advanced)
}

func (b *snapshotKeyCacheBuilder) populate() {
	b.populateMovingAverageKeys()
	b.populateSecuritySourceKeys()
	b.populateRSIKeys()
	b.populateMACDKeys()
	b.populateBollingerKeys()
	b.populateKDJKeys()
	b.populateATRKeys()
	b.populateStdDevKeys()
	b.populateVarianceKeys()
	b.populateWindowKeys()
	b.populateCumKeys()
	b.populateStochKeys()
	b.populateCCIKeys()
	b.populateWilliamsRKeys()
	b.populateVWAPKeys()
	b.populateMFIKeys()
	b.populateDMIKeys()
	b.populateSupertrendKeys()
	b.populateSARKeys()
	b.populateStopLossKeys()
	b.populateDivergenceKeys()
	b.populateAdvancedKeys()
}

func (b *snapshotKeyCacheBuilder) populateMovingAverageKeys() {
	if len(b.requirements.ma) == 0 {
		return
	}
	b.cache.ma = make(map[movingAverageConfig]string, len(b.requirements.ma))
	for _, config := range b.requirements.ma {
		b.cache.ma[config] = maIndicatorKey(config)
		if config.averageType == "MA" && normalizeIndicatorTimeUnit(config.timeUnit) == "" && normalizeSourceOrClose(config.source) == "close" {
			if b.cache.maLegacy == nil {
				b.cache.maLegacy = make(map[movingAverageConfig]string)
			}
			b.cache.maLegacy[config] = legacyMAIndicatorKey(config.period)
			b.cache.resultCapacity++
		}
	}
}

func (b *snapshotKeyCacheBuilder) populateSecuritySourceKeys() {
	if len(b.requirements.securitySource) == 0 {
		return
	}
	b.cache.securitySource = make(map[securitySourceConfig]string, len(b.requirements.securitySource))
	for _, config := range b.requirements.securitySource {
		b.cache.securitySource[config] = securitySourceIndicatorKey(config)
	}
}

func (b *snapshotKeyCacheBuilder) populateRSIKeys() {
	if len(b.requirements.rsi) > 0 {
		b.cache.rsi = make(map[int]string, len(b.requirements.rsi))
		for _, period := range b.requirements.rsi {
			b.cache.rsi[period] = rsiIndicatorKey(period)
		}
	}
	if len(b.requirements.rsiSource) > 0 {
		b.cache.rsiSource = make(map[sourcePeriodConfig]string, len(b.requirements.rsiSource))
		for _, config := range b.requirements.rsiSource {
			b.cache.rsiSource[config] = sourcePeriodIndicatorKey("rsi", config, "close")
		}
	}
}

func (b *snapshotKeyCacheBuilder) populateMACDKeys() {
	if len(b.requirements.macd) == 0 {
		return
	}
	b.cache.macd = make(map[macdConfig]string, len(b.requirements.macd))
	for _, config := range b.requirements.macd {
		b.cache.macd[config] = macdIndicatorKey(config.fastPeriod, config.slowPeriod, config.signalPeriod)
	}
}

func (b *snapshotKeyCacheBuilder) populateBollingerKeys() {
	if len(b.requirements.bollinger) == 0 {
		return
	}
	b.cache.bollinger = make(map[bollingerConfig]string, len(b.requirements.bollinger))
	for _, config := range b.requirements.bollinger {
		b.cache.bollinger[config] = bollingerIndicatorKey(config.period, config.multiplier)
	}
}

func (b *snapshotKeyCacheBuilder) populateKDJKeys() {
	if len(b.requirements.kdj) == 0 {
		return
	}
	b.cache.kdj = make(map[kdjConfig]string, len(b.requirements.kdj))
	for _, config := range b.requirements.kdj {
		b.cache.kdj[config] = kdjIndicatorKey(config.period, config.m1, config.m2)
	}
}

func (b *snapshotKeyCacheBuilder) populateATRKeys() {
	if len(b.requirements.atr) == 0 {
		return
	}
	b.cache.atr = make(map[int]string, len(b.requirements.atr))
	for _, period := range b.requirements.atr {
		b.cache.atr[period] = atrIndicatorKey(period)
	}
}

func (b *snapshotKeyCacheBuilder) populateStdDevKeys() {
	if len(b.requirements.stdev) > 0 {
		b.cache.stdev = make(map[int]string, len(b.requirements.stdev))
		for _, period := range b.requirements.stdev {
			b.cache.stdev[period] = stdevIndicatorKey(period)
		}
	}
	if len(b.requirements.stdevSource) > 0 {
		b.cache.stdevSource = make(map[sourcePeriodConfig]string, len(b.requirements.stdevSource))
		for _, config := range b.requirements.stdevSource {
			b.cache.stdevSource[config] = sourcePeriodIndicatorKey("stdev", config, "close")
		}
	}
}

func (b *snapshotKeyCacheBuilder) populateVarianceKeys() {
	if len(b.requirements.variance) == 0 {
		return
	}
	b.cache.variance = make(map[sourcePeriodConfig]string, len(b.requirements.variance))
	for _, config := range b.requirements.variance {
		b.cache.variance[config] = varianceIndicatorKey(config)
	}
}

func (b *snapshotKeyCacheBuilder) populateWindowKeys() {
	if len(b.requirements.windows) == 0 {
		return
	}
	b.cache.windows = make(map[windowConfig]string, len(b.requirements.windows))
	for _, config := range b.requirements.windows {
		b.cache.windows[config] = windowIndicatorKey(config)
	}
}

func (b *snapshotKeyCacheBuilder) populateCumKeys() {
	if len(b.requirements.cum) == 0 {
		return
	}
	b.cache.cum = make(map[sourceConfig]string, len(b.requirements.cum))
	for _, config := range b.requirements.cum {
		b.cache.cum[config] = sourceIndicatorKey("cum", config)
	}
}

func (b *snapshotKeyCacheBuilder) populateStochKeys() {
	if len(b.requirements.stoch) == 0 {
		return
	}
	b.cache.stoch = make(map[sourcePeriodConfig]string, len(b.requirements.stoch))
	for _, config := range b.requirements.stoch {
		b.cache.stoch[config] = stochIndicatorKey(config)
	}
}

func (b *snapshotKeyCacheBuilder) populateCCIKeys() {
	if len(b.requirements.cci) > 0 {
		b.cache.cci = make(map[int]string, len(b.requirements.cci))
		for _, period := range b.requirements.cci {
			b.cache.cci[period] = cciIndicatorKey(period)
		}
	}
	if len(b.requirements.cciSource) > 0 {
		b.cache.cciSource = make(map[sourcePeriodConfig]string, len(b.requirements.cciSource))
		for _, config := range b.requirements.cciSource {
			b.cache.cciSource[config] = sourcePeriodIndicatorKey("cci", config, "hlc3")
		}
	}
}

func (b *snapshotKeyCacheBuilder) populateWilliamsRKeys() {
	if len(b.requirements.williamsR) == 0 {
		return
	}
	b.cache.williamsR = make(map[int]string, len(b.requirements.williamsR))
	for _, period := range b.requirements.williamsR {
		b.cache.williamsR[period] = williamsRIndicatorKey(period)
	}
}

func (b *snapshotKeyCacheBuilder) populateVWAPKeys() {
	if len(b.requirements.vwap) == 0 {
		return
	}
	b.cache.vwap = make(map[sourceConfig]string, len(b.requirements.vwap))
	for _, config := range b.requirements.vwap {
		b.cache.vwap[config] = sourceIndicatorKey("vwap", config)
	}
}

func (b *snapshotKeyCacheBuilder) populateMFIKeys() {
	if len(b.requirements.mfi) == 0 {
		return
	}
	b.cache.mfi = make(map[sourcePeriodConfig]string, len(b.requirements.mfi))
	for _, config := range b.requirements.mfi {
		b.cache.mfi[config] = "mfi:" + normalizeSourceOrClose(config.source) + ":" + strconv.Itoa(config.period)
	}
}

func (b *snapshotKeyCacheBuilder) populateDMIKeys() {
	if len(b.requirements.dmi) == 0 {
		return
	}
	b.cache.dmi = make(map[dmiConfig]string, len(b.requirements.dmi))
	for _, config := range b.requirements.dmi {
		b.cache.dmi[config] = dmiIndicatorKey(config)
	}
}

func (b *snapshotKeyCacheBuilder) populateSupertrendKeys() {
	if len(b.requirements.supertrend) == 0 {
		return
	}
	b.cache.supertrend = make(map[supertrendConfig]string, len(b.requirements.supertrend))
	for _, config := range b.requirements.supertrend {
		b.cache.supertrend[config] = supertrendIndicatorKey(config)
	}
}

func (b *snapshotKeyCacheBuilder) populateSARKeys() {
	if len(b.requirements.sar) == 0 {
		return
	}
	b.cache.sar = make(map[sarConfig]string, len(b.requirements.sar))
	for _, config := range b.requirements.sar {
		b.cache.sar[config] = sarIndicatorKey(config)
	}
}

func (b *snapshotKeyCacheBuilder) populateStopLossKeys() {
	if len(b.requirements.stopLoss) == 0 {
		return
	}
	b.cache.stopLoss = make(map[stopLossConfig]string, len(b.requirements.stopLoss))
	for _, config := range b.requirements.stopLoss {
		b.cache.stopLoss[config] = stopLossIndicatorKey(config)
	}
}

func (b *snapshotKeyCacheBuilder) populateDivergenceKeys() {
	if len(b.requirements.rsiDivergence) > 0 {
		b.cache.rsiDivergence = make(map[rsiDivergenceConfig]string, len(b.requirements.rsiDivergence))
		for _, config := range b.requirements.rsiDivergence {
			b.cache.rsiDivergence[config] = rsiDivergenceIndicatorKey(config.period, config.direction, config.lookback)
		}
	}
	if len(b.requirements.macdDivergence) > 0 {
		b.cache.macdDivergence = make(map[macdDivergenceConfig]string, len(b.requirements.macdDivergence))
		for _, config := range b.requirements.macdDivergence {
			b.cache.macdDivergence[config] = macdDivergenceIndicatorKey(config.fastPeriod, config.slowPeriod, config.signalPeriod, config.direction, config.lookback)
		}
	}
	if len(b.requirements.kdjDivergence) > 0 {
		b.cache.kdjDivergence = make(map[kdjDivergenceConfig]string, len(b.requirements.kdjDivergence))
		for _, config := range b.requirements.kdjDivergence {
			b.cache.kdjDivergence[config] = kdjDivergenceIndicatorKey(config.period, config.m1, config.m2, config.direction, config.lookback)
		}
	}
}

func (b *snapshotKeyCacheBuilder) populateAdvancedKeys() {
	if len(b.requirements.advanced) == 0 {
		return
	}
	b.cache.advanced = make(map[advancedIndicatorConfig]string, len(b.requirements.advanced))
	for _, config := range b.requirements.advanced {
		b.cache.advanced[config] = config.key
	}
}
