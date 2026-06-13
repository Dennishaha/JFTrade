package indicatorruntime

import "strconv"

func buildSnapshotKeyCache(requirements indicatorRequirements) snapshotKeyCache {
	cache := snapshotKeyCache{
		resultCapacity: len(requirements.ma) + len(requirements.securitySource) + len(requirements.rsi) + len(requirements.rsiSource) + len(requirements.macd) + len(requirements.bollinger) + len(requirements.kdj) + len(requirements.atr) + len(requirements.stdev) + len(requirements.stdevSource) + len(requirements.variance) + len(requirements.windows) + len(requirements.cum) + len(requirements.stoch) + len(requirements.cci) + len(requirements.cciSource) + len(requirements.williamsR) + len(requirements.vwap) + len(requirements.mfi) + len(requirements.dmi) + len(requirements.supertrend) + len(requirements.sar) + len(requirements.stopLoss) + len(requirements.rsiDivergence) + len(requirements.macdDivergence) + len(requirements.kdjDivergence),
	}
	if len(requirements.ma) > 0 {
		cache.ma = make(map[movingAverageConfig]string, len(requirements.ma))
		for _, config := range requirements.ma {
			cache.ma[config] = maIndicatorKey(config)
			if config.averageType == "MA" && normalizeIndicatorTimeUnit(config.timeUnit) == "" && normalizeSourceOrClose(config.source) == "close" {
				if cache.maLegacy == nil {
					cache.maLegacy = make(map[movingAverageConfig]string)
				}
				cache.maLegacy[config] = legacyMAIndicatorKey(config.period)
				cache.resultCapacity++
			}
		}
	}
	if len(requirements.securitySource) > 0 {
		cache.securitySource = make(map[securitySourceConfig]string, len(requirements.securitySource))
		for _, config := range requirements.securitySource {
			cache.securitySource[config] = securitySourceIndicatorKey(config)
		}
	}
	if len(requirements.rsi) > 0 {
		cache.rsi = make(map[int]string, len(requirements.rsi))
		for _, period := range requirements.rsi {
			cache.rsi[period] = rsiIndicatorKey(period)
		}
	}
	if len(requirements.rsiSource) > 0 {
		cache.rsiSource = make(map[sourcePeriodConfig]string, len(requirements.rsiSource))
		for _, config := range requirements.rsiSource {
			cache.rsiSource[config] = sourcePeriodIndicatorKey("rsi", config, "close")
		}
	}
	if len(requirements.macd) > 0 {
		cache.macd = make(map[macdConfig]string, len(requirements.macd))
		for _, config := range requirements.macd {
			cache.macd[config] = macdIndicatorKey(config.fastPeriod, config.slowPeriod, config.signalPeriod)
		}
	}
	if len(requirements.bollinger) > 0 {
		cache.bollinger = make(map[bollingerConfig]string, len(requirements.bollinger))
		for _, config := range requirements.bollinger {
			cache.bollinger[config] = bollingerIndicatorKey(config.period, config.multiplier)
		}
	}
	if len(requirements.kdj) > 0 {
		cache.kdj = make(map[kdjConfig]string, len(requirements.kdj))
		for _, config := range requirements.kdj {
			cache.kdj[config] = kdjIndicatorKey(config.period, config.m1, config.m2)
		}
	}
	if len(requirements.atr) > 0 {
		cache.atr = make(map[int]string, len(requirements.atr))
		for _, period := range requirements.atr {
			cache.atr[period] = atrIndicatorKey(period)
		}
	}
	if len(requirements.stdev) > 0 {
		cache.stdev = make(map[int]string, len(requirements.stdev))
		for _, period := range requirements.stdev {
			cache.stdev[period] = stdevIndicatorKey(period)
		}
	}
	if len(requirements.stdevSource) > 0 {
		cache.stdevSource = make(map[sourcePeriodConfig]string, len(requirements.stdevSource))
		for _, config := range requirements.stdevSource {
			cache.stdevSource[config] = sourcePeriodIndicatorKey("stdev", config, "close")
		}
	}
	if len(requirements.variance) > 0 {
		cache.variance = make(map[sourcePeriodConfig]string, len(requirements.variance))
		for _, config := range requirements.variance {
			cache.variance[config] = varianceIndicatorKey(config)
		}
	}
	if len(requirements.windows) > 0 {
		cache.windows = make(map[windowConfig]string, len(requirements.windows))
		for _, config := range requirements.windows {
			cache.windows[config] = windowIndicatorKey(config)
		}
	}
	if len(requirements.cum) > 0 {
		cache.cum = make(map[sourceConfig]string, len(requirements.cum))
		for _, config := range requirements.cum {
			cache.cum[config] = sourceIndicatorKey("cum", config)
		}
	}
	if len(requirements.stoch) > 0 {
		cache.stoch = make(map[sourcePeriodConfig]string, len(requirements.stoch))
		for _, config := range requirements.stoch {
			cache.stoch[config] = stochIndicatorKey(config)
		}
	}
	if len(requirements.cci) > 0 {
		cache.cci = make(map[int]string, len(requirements.cci))
		for _, period := range requirements.cci {
			cache.cci[period] = cciIndicatorKey(period)
		}
	}
	if len(requirements.cciSource) > 0 {
		cache.cciSource = make(map[sourcePeriodConfig]string, len(requirements.cciSource))
		for _, config := range requirements.cciSource {
			cache.cciSource[config] = sourcePeriodIndicatorKey("cci", config, "hlc3")
		}
	}
	if len(requirements.williamsR) > 0 {
		cache.williamsR = make(map[int]string, len(requirements.williamsR))
		for _, period := range requirements.williamsR {
			cache.williamsR[period] = williamsRIndicatorKey(period)
		}
	}
	if len(requirements.vwap) > 0 {
		cache.vwap = make(map[sourceConfig]string, len(requirements.vwap))
		for _, config := range requirements.vwap {
			cache.vwap[config] = sourceIndicatorKey("vwap", config)
		}
	}
	if len(requirements.mfi) > 0 {
		cache.mfi = make(map[sourcePeriodConfig]string, len(requirements.mfi))
		for _, config := range requirements.mfi {
			cache.mfi[config] = "mfi:" + normalizeSourceOrClose(config.source) + ":" + strconv.Itoa(config.period)
		}
	}
	if len(requirements.dmi) > 0 {
		cache.dmi = make(map[dmiConfig]string, len(requirements.dmi))
		for _, config := range requirements.dmi {
			cache.dmi[config] = dmiIndicatorKey(config)
		}
	}
	if len(requirements.supertrend) > 0 {
		cache.supertrend = make(map[supertrendConfig]string, len(requirements.supertrend))
		for _, config := range requirements.supertrend {
			cache.supertrend[config] = supertrendIndicatorKey(config)
		}
	}
	if len(requirements.sar) > 0 {
		cache.sar = make(map[sarConfig]string, len(requirements.sar))
		for _, config := range requirements.sar {
			cache.sar[config] = sarIndicatorKey(config)
		}
	}
	if len(requirements.stopLoss) > 0 {
		cache.stopLoss = make(map[stopLossConfig]string, len(requirements.stopLoss))
		for _, config := range requirements.stopLoss {
			cache.stopLoss[config] = stopLossIndicatorKey(config)
		}
	}
	if len(requirements.rsiDivergence) > 0 {
		cache.rsiDivergence = make(map[rsiDivergenceConfig]string, len(requirements.rsiDivergence))
		for _, config := range requirements.rsiDivergence {
			cache.rsiDivergence[config] = rsiDivergenceIndicatorKey(config.period, config.direction, config.lookback)
		}
	}
	if len(requirements.macdDivergence) > 0 {
		cache.macdDivergence = make(map[macdDivergenceConfig]string, len(requirements.macdDivergence))
		for _, config := range requirements.macdDivergence {
			cache.macdDivergence[config] = macdDivergenceIndicatorKey(config.fastPeriod, config.slowPeriod, config.signalPeriod, config.direction, config.lookback)
		}
	}
	if len(requirements.kdjDivergence) > 0 {
		cache.kdjDivergence = make(map[kdjDivergenceConfig]string, len(requirements.kdjDivergence))
		for _, config := range requirements.kdjDivergence {
			cache.kdjDivergence[config] = kdjDivergenceIndicatorKey(config.period, config.m1, config.m2, config.direction, config.lookback)
		}
	}
	return cache
}
