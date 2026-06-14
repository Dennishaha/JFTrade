package indicatorruntime

type indicatorRequirements struct {
	ma             []movingAverageConfig
	securitySource []securitySourceConfig
	rsi            []int
	rsiSource      []sourcePeriodConfig
	macd           []macdConfig
	bollinger      []bollingerConfig
	kdj            []kdjConfig
	atr            []int
	stdev          []int
	stdevSource    []sourcePeriodConfig
	variance       []sourcePeriodConfig
	windows        []windowConfig
	cum            []sourceConfig
	stoch          []sourcePeriodConfig
	cci            []int
	cciSource      []sourcePeriodConfig
	williamsR      []int
	vwap           []sourceConfig
	mfi            []sourcePeriodConfig
	dmi            []dmiConfig
	supertrend     []supertrendConfig
	sar            []sarConfig
	stopLoss       []stopLossConfig
	rsiDivergence  []rsiDivergenceConfig
	macdDivergence []macdDivergenceConfig
	kdjDivergence  []kdjDivergenceConfig
	advanced       []advancedIndicatorConfig
}
