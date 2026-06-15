package indicatorruntime

const (
	tradingSessionMinutesPerDay   = 390
	tradingSessionMinutesPerWeek  = tradingSessionMinutesPerDay * 5
	tradingSessionMinutesPerMonth = tradingSessionMinutesPerDay * 20
)

type movingAverageConfig struct {
	averageType string
	period      int
	timeUnit    string
	source      string
}

type sourceConfig struct {
	source string
}

type securitySourceConfig struct {
	source   string
	timeUnit string
	lookback int
}

type sourcePeriodConfig struct {
	source   string
	period   int
	timeUnit string
}

type dmiConfig struct {
	diLength     int
	adxSmoothing int
}

type supertrendConfig struct {
	factor    float64
	atrPeriod int
}

type sarConfig struct {
	start     float64
	increment float64
	maximum   float64
}

type stopLossConfig struct {
	mode         string
	direction    string
	timeValue    int
	timeUnit     string
	percentage   float64
	windowPolicy string
}

type macdConfig struct {
	fastPeriod   int
	slowPeriod   int
	signalPeriod int
}

type bollingerConfig struct {
	period     int
	multiplier float64
}

type windowConfig struct {
	function string
	source   string
	period   int
}

type advancedIndicatorConfig struct {
	key        string
	kind       string
	source     string
	source2    string
	timeUnit   string
	period     int
	left       int
	right      int
	offset     int
	multiplier float64
	parameter  float64
	useTR      bool
}

type kdjConfig struct {
	period int
	m1     int
	m2     int
}

type rsiDivergenceConfig struct {
	period    int
	direction string
	lookback  int
}

type macdDivergenceConfig struct {
	fastPeriod   int
	slowPeriod   int
	signalPeriod int
	direction    string
	lookback     int
}

type kdjDivergenceConfig struct {
	period    int
	m1        int
	m2        int
	direction string
	lookback  int
}
