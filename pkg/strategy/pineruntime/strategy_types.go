package pineruntime

import (
	"time"

	"github.com/c9s/bbgo/pkg/types"
	exprast "github.com/expr-lang/expr/ast"
	"github.com/jftrade/jftrade-main/pkg/market"
	strategyir "github.com/jftrade/jftrade-main/pkg/strategy/ir"
)

type historyBuffer struct {
	values []any
	next   int
	count  int
}

type cachedIndicatorBinding struct {
	binding    indicatorBinding
	recognized bool
	err        error
}

type cachedProtectRequirement struct {
	key            string
	allowLongExit  bool
	allowShortExit bool
	err            error
}

type cachedPositionSnapshot struct {
	barTime time.Time
	symbol  string
	value   *positionSnapshot
	valid   bool
}

type indicatorBinding struct {
	Alias string
	Kind  string
	Key   string
	Args  []string
}

type divergenceRequirementCacheKey struct {
	bindingKey string
	direction  string
	lookback   int
}

type pendingOrder struct {
	id                 string
	sequence           int
	action             strategyir.OrderAction
	intent             strategyir.OrderIntent
	orderType          types.OrderType
	quantity           float64
	quantityMode       string
	quantityExpression string
	entryPolicy        string
	rangeInfo          strategyir.SourceRange
	limitPrice         float64
	stopPrice          float64
	hasLimit           bool
	hasStop            bool
	activated          bool
	submitted          bool
	comment            string
	alert              string
	disable            bool
}

type trailingExitState struct {
	activated bool
	extreme   float64
	stopPrice float64
	direction string
}

type ifScopePlan struct {
	thenNeedsClone bool
	elseNeedsClone bool
}

type positionSnapshot struct {
	Symbol            string
	Quantity          float64
	AvailableQuantity float64
	MarketValue       float64
	AveragePrice      float64
	Direction         string
}

type evaluationScope struct {
	runtime            *strategyRuntime
	parent             *evaluationScope
	variables          map[string]any
	bindings           map[string]indicatorBinding
	indicators         map[string]any
	currentKline       *types.KLine
	currentKlineTime   time.Time
	currentKlineSymbol string
	currentSession     market.Session
	klinePayload       klinePayloadView
	closeSeries        seriesNumber
	openSeries         seriesNumber
	highSeries         seriesNumber
	lowSeries          seriesNumber
	volumeSeries       seriesNumber
	hl2Series          seriesNumber
	hlc3Series         seriesNumber
	ohlc4Series        seriesNumber
	hasBarData         bool
	barIndex           int
}

type barssinceState struct {
	lastBarIndex int
	hasCached    bool
	cached       any
	seen         bool
	value        int
}

type valuewhenState struct {
	lastBarIndex int
	hasCached    bool
	cached       any
	values       []any
}

type historyTarget struct {
	key         string
	expression  exprast.Node
	maxLookback int
}

type klineSessionResolver interface {
	ResolveKLineSession(kline types.KLine) (market.Session, bool)
}

func (s *evaluationScope) clone() *evaluationScope {
	return &evaluationScope{
		runtime:            s.runtime,
		parent:             s,
		indicators:         s.indicators,
		currentKline:       s.currentKline,
		currentKlineTime:   s.currentKlineTime,
		currentKlineSymbol: s.currentKlineSymbol,
		currentSession:     s.currentSession,
		klinePayload:       s.klinePayload,
		closeSeries:        s.closeSeries,
		openSeries:         s.openSeries,
		highSeries:         s.highSeries,
		lowSeries:          s.lowSeries,
		volumeSeries:       s.volumeSeries,
		hasBarData:         s.hasBarData,
	}
}
