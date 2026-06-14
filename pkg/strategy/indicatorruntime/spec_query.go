package indicatorruntime

func (r indicatorRequirements) isEmpty() bool {
	return len(r.ma) == 0 &&
		len(r.securitySource) == 0 &&
		len(r.rsi) == 0 &&
		len(r.rsiSource) == 0 &&
		len(r.macd) == 0 &&
		len(r.bollinger) == 0 &&
		len(r.kdj) == 0 &&
		len(r.atr) == 0 &&
		len(r.stdev) == 0 &&
		len(r.stdevSource) == 0 &&
		len(r.variance) == 0 &&
		len(r.windows) == 0 &&
		len(r.cum) == 0 &&
		len(r.stoch) == 0 &&
		len(r.cci) == 0 &&
		len(r.cciSource) == 0 &&
		len(r.williamsR) == 0 &&
		len(r.vwap) == 0 &&
		len(r.mfi) == 0 &&
		len(r.dmi) == 0 &&
		len(r.supertrend) == 0 &&
		len(r.sar) == 0 &&
		len(r.stopLoss) == 0 &&
		len(r.rsiDivergence) == 0 &&
		len(r.macdDivergence) == 0 &&
		len(r.kdjDivergence) == 0 &&
		len(r.advanced) == 0
}
