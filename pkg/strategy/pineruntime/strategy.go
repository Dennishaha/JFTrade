package pineruntime

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	bbgo2 "github.com/c9s/bbgo/pkg/bbgo"
	"github.com/c9s/bbgo/pkg/types"
	exprast "github.com/expr-lang/expr/ast"
	"github.com/jftrade/jftrade-main/pkg/market"
	"github.com/jftrade/jftrade-main/pkg/strategy/indicatorbinding"
	strategyindicatorruntime "github.com/jftrade/jftrade-main/pkg/strategy/indicatorruntime"
	strategyir "github.com/jftrade/jftrade-main/pkg/strategy/ir"
	strategypine "github.com/jftrade/jftrade-main/pkg/strategy/pine"
)

const ID = "pine-go-plan"

func init() {
	bbgo2.RegisterStrategy(ID, &Strategy{})
}

type Strategy struct {
	StrategyID       string                   `json:"strategyId"`
	Name             string                   `json:"name"`
	Symbol           string                   `json:"symbol"`
	Interval         types.Interval           `json:"interval"`
	Script           string                   `json:"script"`
	DefinitionID     string                   `json:"definitionId"`
	UseExtendedHours bool                     `json:"-"`
	WarmupUntil      time.Time                `json:"-"`
	OnError          func(string)             `json:"-"`
	Program          *strategyir.Program      `json:"-"`
	Requirements     *strategyir.Requirements `json:"-"`
}

type strategyRuntime struct {
	mu                    sync.Mutex
	ctx                   context.Context
	strategy              *Strategy
	program               *strategyir.Program
	plan                  strategyir.Requirements
	hooks                 map[strategyir.HookKind]*strategyir.HookBlock
	ifScopePlans          map[*strategyir.IfStmt]ifScopePlan
	displayName           string
	definitionID          string
	symbol                string
	interval              types.Interval
	expressionCache       map[string]exprast.Node
	bindingCache          map[*strategyir.LetStmt]cachedIndicatorBinding
	protectCache          map[*strategyir.ProtectStmt]cachedProtectRequirement
	divergenceCache       map[divergenceRequirementCacheKey]string
	engine                *strategyindicatorruntime.IndicatorEngine
	session               *bbgo2.ExchangeSession
	executor              bbgo2.OrderExecutor
	baseScope             *evaluationScope
	reusableScope         *evaluationScope
	persistentValues      map[string]any
	variableCapacity      int
	bindingCapacity       int
	previousClose         float64
	previousOpen          float64
	previousHigh          float64
	previousLow           float64
	previousVolume        float64
	hasPreviousClose      bool
	barIndex              int
	positionCache         cachedPositionSnapshot
	entrySubmitCount      map[string]int
	maxPyramiding         int
	allowedEntryDirection string
	pendingOrders         map[string]pendingOrder
	pendingSequence       int
	trailingExits         map[string]trailingExitState
	barssinceStates       map[string]*barssinceState
	valuewhenStates       map[string]*valuewhenState
	historyTargets        map[string]historyTarget
	historyValues         map[string]*historyBuffer
}

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

type klinePayloadView struct {
	kline         *types.KLine
	session       market.Session
	startTimeText string
	endTimeText   string
	hasStartTime  bool
	hasEndTime    bool
}

type klineSessionResolver interface {
	ResolveKLineSession(kline types.KLine) (market.Session, bool)
}

func (s *Strategy) ID() string {
	return ID
}

func (s *Strategy) Subscribe(session *bbgo2.ExchangeSession) {
	if session == nil {
		return
	}
	symbol := strings.ToUpper(strings.TrimSpace(s.Symbol))
	if symbol == "" {
		return
	}
	interval := s.Interval
	if interval == "" {
		interval = types.Interval1m
	}
	session.Subscribe(types.KLineChannel, symbol, types.SubscribeOptions{Interval: interval})
}

func (s *Strategy) Run(ctx context.Context, orderExecutor bbgo2.OrderExecutor, session *bbgo2.ExchangeSession) error {
	program := s.Program
	var plan strategyir.Requirements
	if s.Requirements != nil {
		plan = *s.Requirements
	}
	if program == nil || s.Requirements == nil {
		compilation, err := strategypine.Compile(s.Script)
		if err != nil {
			return fmt.Errorf("parse pine strategy: %w", err)
		}
		program = compilation.Program
		plan = compilation.Requirements
	}
	runtime, err := newStrategyRuntime(ctx, s, program, plan, orderExecutor, session)
	if err != nil {
		return err
	}
	if err := runtime.runInit(); err != nil {
		return err
	}
	session.MarketDataStream.OnKLineClosed(func(kline types.KLine) {
		runtime.handleKLineClosed(kline)
	})
	return nil
}

func newStrategyRuntime(
	ctx context.Context,
	strategy *Strategy,
	program *strategyir.Program,
	plan strategyir.Requirements,
	orderExecutor bbgo2.OrderExecutor,
	session *bbgo2.ExchangeSession,
) (*strategyRuntime, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	displayName := strategyName(strategy)
	definitionID := strings.TrimSpace(strategy.DefinitionID)
	symbol := strings.ToUpper(strings.TrimSpace(strategy.Symbol))
	interval := defaultInterval(strategy.Interval)
	letCount := countProgramLetStatements(program)
	engine, err := strategyindicatorruntime.NewIndicatorEngineForPlanWithOptions(
		plan,
		defaultInterval(strategy.Interval),
		strategy.Symbol,
		strategyindicatorruntime.RuntimeOptions{IncludeExtendedHours: strategy.UseExtendedHours},
	)
	if err != nil {
		return nil, fmt.Errorf("create pine indicator engine: %w", err)
	}
	runtime := &strategyRuntime{
		ctx:                   ctx,
		strategy:              strategy,
		program:               program,
		plan:                  plan,
		hooks:                 buildHookCache(program),
		ifScopePlans:          buildIfScopePlans(program),
		displayName:           displayName,
		definitionID:          definitionID,
		symbol:                symbol,
		interval:              interval,
		expressionCache:       map[string]exprast.Node{},
		bindingCache:          map[*strategyir.LetStmt]cachedIndicatorBinding{},
		protectCache:          map[*strategyir.ProtectStmt]cachedProtectRequirement{},
		divergenceCache:       map[divergenceRequirementCacheKey]string{},
		persistentValues:      map[string]any{},
		engine:                engine,
		session:               session,
		executor:              orderExecutor,
		entrySubmitCount:      map[string]int{},
		maxPyramiding:         normalizeRuntimePyramiding(program),
		allowedEntryDirection: normalizeRuntimeAllowedEntryDirection(program),
		pendingOrders:         map[string]pendingOrder{},
		trailingExits:         map[string]trailingExitState{},
		barssinceStates:       map[string]*barssinceState{},
		valuewhenStates:       map[string]*valuewhenState{},
		historyTargets:        collectProgramHistoryTargets(program),
		barIndex:              -1,
		baseScope: &evaluationScope{
			variables: map[string]any{
				"id":           displayName,
				"name":         displayName,
				"definitionId": definitionID,
				"symbol":       symbol,
				"interval":     string(interval),
				"isBacktest":   bbgo2.IsBackTesting,
			},
		},
		variableCapacity: letCount,
		bindingCapacity:  letCount,
	}
	runtime.historyValues = buildHistoryBuffers(runtime.historyTargets)
	runtime.baseScope.runtime = runtime
	runtime.reusableScope = &evaluationScope{
		runtime:   runtime,
		parent:    runtime.baseScope,
		variables: make(map[string]any, runtime.variableCapacity),
	}
	if runtime.bindingCapacity > 0 {
		runtime.reusableScope.bindings = make(map[string]indicatorBinding, runtime.bindingCapacity)
	}
	if err := runtime.preparseProgramExpressions(); err != nil {
		return nil, fmt.Errorf("preparse pine expressions: %w", err)
	}
	return runtime, nil
}

func runtimeAccount(session *bbgo2.ExchangeSession) *types.Account {
	if session == nil {
		return nil
	}
	return session.GetAccount()
}

func runtimePositionForSymbol(session *bbgo2.ExchangeSession, symbol string) *types.Position {
	if session == nil {
		return nil
	}
	positions := session.Positions()
	if positions == nil {
		return nil
	}
	return positions[symbol]
}

func defaultInterval(interval types.Interval) types.Interval {
	if interval == "" {
		return types.Interval1m
	}
	return interval
}

func strategyName(strategy *Strategy) string {
	if strategy == nil {
		return ID
	}
	if name := strings.TrimSpace(strategy.Name); name != "" {
		return name
	}
	if name := strings.TrimSpace(strategy.StrategyID); name != "" {
		return name
	}
	if name := strings.TrimSpace(strategy.DefinitionID); name != "" {
		return name
	}
	return ID
}

func klinePayload(kline types.KLine, session market.Session) *klinePayloadView {
	return &klinePayloadView{kline: &kline, session: session}
}

func (p *klinePayloadView) FieldValue(name string) (any, bool) {
	if p == nil || p.kline == nil {
		return nil, false
	}
	switch name {
	case "symbol":
		return p.kline.Symbol, true
	case "interval":
		return string(p.kline.Interval), true
	case "startTime":
		if !p.hasStartTime {
			p.startTimeText = p.kline.StartTime.Time().Format("2006-01-02T15:04:05.000Z07:00")
			p.hasStartTime = true
		}
		return p.startTimeText, true
	case "endTime":
		if !p.hasEndTime {
			p.endTimeText = p.kline.EndTime.Time().Format("2006-01-02T15:04:05.000Z07:00")
			p.hasEndTime = true
		}
		return p.endTimeText, true
	case "open":
		return p.kline.Open.Float64(), true
	case "high":
		return p.kline.High.Float64(), true
	case "low":
		return p.kline.Low.Float64(), true
	case "close":
		return p.kline.Close.Float64(), true
	case "volume":
		return p.kline.Volume.Float64(), true
	case "quoteVolume":
		return p.kline.QuoteVolume.Float64(), true
	case "closed":
		return p.kline.Closed, true
	case "session":
		if p.session == market.SessionUnknown {
			return nil, true
		}
		return string(p.session), true
	default:
		return nil, false
	}
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

func (r *strategyRuntime) sameDirectionEntryCount(direction string, position *positionSnapshot, availablePositionQty float64) int {
	if r == nil {
		return 0
	}
	normalized := strings.ToUpper(strings.TrimSpace(direction))
	count := 0
	if r.entrySubmitCount != nil {
		count = r.entrySubmitCount[normalized]
	}
	if count == 0 && position != nil && position.Direction == normalized && availablePositionQty > 0 {
		return 1
	}
	return count
}

func (r *strategyRuntime) recordSubmittedOrderAction(action strategyir.OrderAction, quantity float64, availablePositionQty float64, sameDirectionEntryCount int) {
	if r == nil {
		return
	}
	switch action {
	case strategyir.OrderActionBuy:
		r.incrementEntrySubmitCount("LONG", sameDirectionEntryCount)
	case strategyir.OrderActionShort:
		r.incrementEntrySubmitCount("SHORT", sameDirectionEntryCount)
	case strategyir.OrderActionSell:
		r.reduceEntrySubmitCount("LONG", quantity, availablePositionQty)
	case strategyir.OrderActionCover:
		r.reduceEntrySubmitCount("SHORT", quantity, availablePositionQty)
	}
}

func (r *strategyRuntime) recordEntryOrderAction(action strategyir.OrderAction, quantity float64, availablePositionQty float64, sameDirectionEntryCount int, adjustment entryOrderAdjustment) {
	if r == nil {
		return
	}
	switch action {
	case strategyir.OrderActionBuy:
		if adjustment.reversed {
			r.resetEntrySubmitCount("SHORT")
			r.clearOrderStateForDirection("SHORT")
		}
		if adjustment.closeOnly {
			return
		}
		r.incrementEntrySubmitCount("LONG", sameDirectionEntryCount)
	case strategyir.OrderActionShort:
		if adjustment.reversed {
			r.resetEntrySubmitCount("LONG")
			r.clearOrderStateForDirection("LONG")
		}
		if adjustment.closeOnly {
			return
		}
		r.incrementEntrySubmitCount("SHORT", sameDirectionEntryCount)
	default:
		r.recordSubmittedOrderAction(action, quantity, availablePositionQty, sameDirectionEntryCount)
	}
}

func (r *strategyRuntime) clearOrderStateForDirection(direction string) {
	if r == nil {
		return
	}
	for key, state := range r.trailingExits {
		if strings.EqualFold(state.direction, direction) {
			delete(r.trailingExits, key)
		}
	}
	for id, order := range r.pendingOrders {
		switch strings.ToUpper(strings.TrimSpace(direction)) {
		case "LONG":
			if order.action == strategyir.OrderActionSell {
				delete(r.pendingOrders, id)
			}
		case "SHORT":
			if order.action == strategyir.OrderActionBuy || order.action == strategyir.OrderActionCover {
				delete(r.pendingOrders, id)
			}
		}
	}
}

func (r *strategyRuntime) incrementEntrySubmitCount(direction string, observedCount int) {
	if r.entrySubmitCount == nil {
		r.entrySubmitCount = map[string]int{}
	}
	normalized := strings.ToUpper(strings.TrimSpace(direction))
	current := r.entrySubmitCount[normalized]
	if observedCount > current {
		current = observedCount
	}
	r.entrySubmitCount[normalized] = current + 1
}

func (r *strategyRuntime) reduceEntrySubmitCount(direction string, quantity float64, availablePositionQty float64) {
	if quantity >= availablePositionQty || availablePositionQty <= 0 {
		r.resetEntrySubmitCount(direction)
		return
	}
	normalized := strings.ToUpper(strings.TrimSpace(direction))
	if r.entrySubmitCount == nil || r.entrySubmitCount[normalized] <= 1 {
		r.resetEntrySubmitCount(direction)
		return
	}
	r.entrySubmitCount[normalized]--
}

func (r *strategyRuntime) resetEntrySubmitCount(direction string) {
	if r == nil || r.entrySubmitCount == nil {
		return
	}
	delete(r.entrySubmitCount, strings.ToUpper(strings.TrimSpace(direction)))
}

func shouldSkipLongEntry(position *positionSnapshot, availablePositionQty float64, entryPolicy string, maxPyramiding int, sameDirectionEntryCount int) bool {
	if entryPolicy == "allow" {
		return false
	}
	if entryPolicy == "flat_only" {
		return position != nil && position.Quantity != 0
	}
	if maxPyramiding <= 0 {
		maxPyramiding = 1
	}
	return position != nil && position.Direction == "LONG" && availablePositionQty > 0 && sameDirectionEntryCount >= maxPyramiding
}

func shouldSkipShortEntry(position *positionSnapshot, availablePositionQty float64, entryPolicy string, maxPyramiding int, sameDirectionEntryCount int) bool {
	if entryPolicy == "allow" {
		return false
	}
	if entryPolicy == "flat_only" {
		return position != nil && position.Quantity != 0
	}
	if maxPyramiding <= 0 {
		maxPyramiding = 1
	}
	return position != nil && position.Direction == "SHORT" && availablePositionQty > 0 && sameDirectionEntryCount >= maxPyramiding
}

func exchangeSideForAction(action strategyir.OrderAction) (types.SideType, error) {
	switch action {
	case strategyir.OrderActionBuy, strategyir.OrderActionCover:
		return types.SideTypeBuy, nil
	case strategyir.OrderActionSell, strategyir.OrderActionShort:
		return types.SideTypeSell, nil
	default:
		return "", fmt.Errorf("unsupported order action %q", action)
	}
}

func normalizeOrderType(value string) types.OrderType {
	if strings.EqualFold(strings.TrimSpace(value), string(types.OrderTypeLimit)) {
		return types.OrderTypeLimit
	}
	return types.OrderTypeMarket
}

func normalizeOrderIntent(value strategyir.OrderIntent) strategyir.OrderIntent {
	switch value {
	case strategyir.OrderIntentClose, strategyir.OrderIntentNet, strategyir.OrderIntentFlatten:
		return value
	case strategyir.OrderIntentEntry:
		return strategyir.OrderIntentEntry
	default:
		return strategyir.OrderIntentEntry
	}
}

func normalizeEntryPolicy(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "flatonly", "flat_only":
		return "flat_only"
	case "allow":
		return "allow"
	default:
		return "same_direction"
	}
}

func normalizeProtectDirection(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "long":
		return "long"
	case "short":
		return "short"
	default:
		return "auto"
	}
}

func firstPositiveFloat(values ...float64) float64 {
	for _, value := range values {
		if value > 0 {
			return value
		}
	}
	return 0
}

func absFloat(value float64) float64 {
	if value < 0 {
		return -value
	}
	return value
}

func readBool(values any, key string) bool {
	value, ok := readObjectField(values, key)
	if !ok || value == missingObjectField {
		return false
	}
	result, _ := coerceBoolValue(value)
	return result
}

func (r *strategyRuntime) parseIndicatorBinding(statement *strategyir.LetStmt) (indicatorBinding, bool, error) {
	if r == nil || statement == nil {
		return parseIndicatorBinding(statement)
	}
	if cached, ok := r.bindingCache[statement]; ok {
		return cached.binding, cached.recognized, cached.err
	}
	binding, recognized, err := parseIndicatorBinding(statement)
	r.bindingCache[statement] = cachedIndicatorBinding{binding: binding, recognized: recognized, err: err}
	return binding, recognized, err
}

func parseIndicatorBinding(statement *strategyir.LetStmt) (indicatorBinding, bool, error) {
	name, args, ok := indicatorbinding.ParseFunctionCall(statement.Expression)
	if !ok {
		return indicatorBinding{}, false, nil
	}
	switch indicatorbinding.NormalizeFunctionName(name) {
	case "linreg", "obv", "pivothigh", "pivotlow", "kc", "kcw", "alma",
		"cmo", "tsi", "correlation", "dev", "median", "percentile_linear_interpolation",
		"percentile_nearest_rank", "percentrank", "swma":
		return parseAdvancedRuntimeBinding(statement.Range.StartLine, statement.Name, indicatorbinding.NormalizeFunctionName(name), args)
	case "ma":
		if len(args) < 2 || len(args) > 4 {
			return indicatorBinding{}, false, fmt.Errorf("pine line %d: ma() requires type, period, optional time unit, and optional source", statement.Range.StartLine)
		}
		averageType, ok := indicatorbinding.ParseMovingAverageType(args[0])
		if !ok {
			return indicatorBinding{}, false, fmt.Errorf("pine line %d: ma() type %q is not supported", statement.Range.StartLine, strings.TrimSpace(args[0]))
		}
		period, err := indicatorbinding.ParsePositiveInt(args[1])
		if err != nil {
			return indicatorBinding{}, false, fmt.Errorf("pine line %d: ma() period must be a positive integer", statement.Range.StartLine)
		}
		timeUnit, source, err := indicatorbinding.ParseMovingAverageOptionalArgs(args[2:])
		if err != nil {
			return indicatorBinding{}, false, fmt.Errorf("pine line %d: %w", statement.Range.StartLine, err)
		}
		maArgs := []string{averageType, strconv.Itoa(period), timeUnit}
		if strings.TrimSpace(source) != "" && strings.TrimSpace(source) != "close" {
			maArgs = append(maArgs, source)
		}
		return indicatorBinding{Alias: statement.Name, Kind: "ma", Key: indicatorbinding.BuildMovingAverageKeyWithSource(averageType, period, timeUnit, source), Args: maArgs}, true, nil
	case "rsi":
		source, period, err := parseSourcePeriodArgs(statement.Range.StartLine, name, args, "close", "14")
		if err != nil {
			return indicatorBinding{}, false, err
		}
		key := sourcePeriodRuntimeKey("rsi", source, period, "close")
		return indicatorBinding{Alias: statement.Name, Kind: "rsi", Key: key, Args: sourcePeriodRuntimeArgs(source, period, "close")}, true, nil
	case "macd":
		values, err := indicatorbinding.ExpectPositiveIntArgs(statement.Range.StartLine, name, args, 3)
		if err != nil {
			return indicatorBinding{}, false, err
		}
		return indicatorBinding{Alias: statement.Name, Kind: "macd", Key: fmt.Sprintf("macd:%d:%d:%d", values[0], values[1], values[2]), Args: indicatorbinding.IntsToStrings(values)}, true, nil
	case "kdj":
		values, err := indicatorbinding.ExpectPositiveIntArgs(statement.Range.StartLine, name, args, 3)
		if err != nil {
			return indicatorBinding{}, false, err
		}
		return indicatorBinding{Alias: statement.Name, Kind: "kdj", Key: fmt.Sprintf("kdj:%d:%d:%d", values[0], values[1], values[2]), Args: indicatorbinding.IntsToStrings(values)}, true, nil
	case "atr":
		period, err := indicatorbinding.ExpectOnePositiveIntArg(statement.Range.StartLine, name, args)
		if err != nil {
			return indicatorBinding{}, false, err
		}
		return indicatorBinding{Alias: statement.Name, Kind: "atr", Key: "atr:" + strconv.Itoa(period), Args: []string{strconv.Itoa(period)}}, true, nil
	case "stdev":
		source, period, err := parseSourcePeriodArgs(statement.Range.StartLine, name, args, "close", "20")
		if err != nil {
			return indicatorBinding{}, false, err
		}
		key := sourcePeriodRuntimeKey("stdev", source, period, "close")
		return indicatorBinding{Alias: statement.Name, Kind: "stdev", Key: key, Args: sourcePeriodRuntimeArgs(source, period, "close")}, true, nil
	case "variance":
		source, period, err := parseSourcePeriodArgs(statement.Range.StartLine, name, args, "close", "20")
		if err != nil {
			return indicatorBinding{}, false, err
		}
		key := "variance:" + source + ":" + strconv.Itoa(period)
		return indicatorBinding{Alias: statement.Name, Kind: "variance", Key: key, Args: []string{source, strconv.Itoa(period)}}, true, nil
	case "cum":
		if len(args) != 1 {
			return indicatorBinding{}, false, fmt.Errorf("pine line %d: cum() requires one source argument", statement.Range.StartLine)
		}
		source, ok := indicatorbinding.ParsePriceSource(args[0])
		if !ok {
			return indicatorBinding{}, false, fmt.Errorf("pine line %d: cum() source %q is not supported; use open/high/low/close/volume/hl2/hlc3/ohlc4", statement.Range.StartLine, strings.TrimSpace(args[0]))
		}
		return indicatorBinding{Alias: statement.Name, Kind: "cum", Key: "cum:" + source, Args: []string{source}}, true, nil
	case "highest", "lowest", "highestbars", "lowestbars", "change", "mom", "roc", "range", "mode", "rising", "falling", "sum":
		if len(args) != 2 {
			return indicatorBinding{}, false, fmt.Errorf("pine line %d: %s() requires source and length arguments", statement.Range.StartLine, name)
		}
		source, ok := indicatorbinding.ParsePriceSource(args[0])
		if !ok {
			return indicatorBinding{}, false, fmt.Errorf("pine line %d: %s() source %q is not supported; use open/high/low/close/volume/hl2/hlc3/ohlc4", statement.Range.StartLine, name, strings.TrimSpace(args[0]))
		}
		period, err := indicatorbinding.ParsePositiveInt(args[1])
		if err != nil {
			return indicatorBinding{}, false, fmt.Errorf("pine line %d: %s() length must be a positive integer", statement.Range.StartLine, name)
		}
		function := indicatorbinding.NormalizeFunctionName(name)
		key := function + ":" + source + ":" + strconv.Itoa(period)
		return indicatorBinding{Alias: statement.Name, Kind: function, Key: key, Args: []string{source, strconv.Itoa(period)}}, true, nil
	case "stoch":
		if len(args) != 4 {
			return indicatorBinding{}, false, fmt.Errorf("pine line %d: stoch() requires source, high, low, and length arguments", statement.Range.StartLine)
		}
		source, ok := parseStochSource(args[0])
		if !ok {
			return indicatorBinding{}, false, fmt.Errorf("pine line %d: stoch() source %q is not supported; use open/high/low/close/hl2/hlc3/ohlc4", statement.Range.StartLine, strings.TrimSpace(args[0]))
		}
		if !strings.EqualFold(strings.TrimSpace(args[1]), "high") || !strings.EqualFold(strings.TrimSpace(args[2]), "low") {
			return indicatorBinding{}, false, fmt.Errorf("pine line %d: stoch() currently supports literal high and low arguments only", statement.Range.StartLine)
		}
		period, err := indicatorbinding.ParsePositiveInt(args[3])
		if err != nil {
			return indicatorBinding{}, false, fmt.Errorf("pine line %d: stoch() length must be a positive integer", statement.Range.StartLine)
		}
		return indicatorBinding{Alias: statement.Name, Kind: "stoch", Key: "stoch:" + source + ":" + strconv.Itoa(period), Args: []string{source, strconv.Itoa(period)}}, true, nil
	case "cci":
		source, period, err := parseSourcePeriodArgs(statement.Range.StartLine, name, args, "hlc3", "20")
		if err != nil {
			return indicatorBinding{}, false, err
		}
		key := sourcePeriodRuntimeKey("cci", source, period, "hlc3")
		return indicatorBinding{Alias: statement.Name, Kind: "cci", Key: key, Args: sourcePeriodRuntimeArgs(source, period, "hlc3")}, true, nil
	case "vwap":
		if len(args) != 1 {
			return indicatorBinding{}, false, fmt.Errorf("pine line %d: vwap() requires one source argument", statement.Range.StartLine)
		}
		source, ok := indicatorbinding.ParsePriceSource(args[0])
		if !ok {
			return indicatorBinding{}, false, fmt.Errorf("pine line %d: vwap() source %q is not supported; use open/high/low/close/volume/hl2/hlc3/ohlc4", statement.Range.StartLine, strings.TrimSpace(args[0]))
		}
		return indicatorBinding{Alias: statement.Name, Kind: "vwap", Key: "vwap:" + source, Args: []string{source}}, true, nil
	case "mfi":
		source, period, err := parseSourcePeriodArgs(statement.Range.StartLine, name, args, "hlc3", "14")
		if err != nil {
			return indicatorBinding{}, false, err
		}
		key := "mfi:" + source + ":" + strconv.Itoa(period)
		return indicatorBinding{Alias: statement.Name, Kind: "mfi", Key: key, Args: []string{source, strconv.Itoa(period)}}, true, nil
	case "dmi":
		values, err := indicatorbinding.ExpectPositiveIntArgs(statement.Range.StartLine, name, args, 2)
		if err != nil {
			return indicatorBinding{}, false, err
		}
		key := fmt.Sprintf("dmi:%d:%d", values[0], values[1])
		return indicatorBinding{Alias: statement.Name, Kind: "dmi", Key: key, Args: indicatorbinding.IntsToStrings(values)}, true, nil
	case "supertrend":
		if len(args) != 2 {
			return indicatorBinding{}, false, fmt.Errorf("pine line %d: supertrend() requires factor and atrPeriod", statement.Range.StartLine)
		}
		factor, err := indicatorbinding.ParsePositiveFloat(args[0])
		if err != nil {
			return indicatorBinding{}, false, fmt.Errorf("pine line %d: supertrend() factor must be a positive number", statement.Range.StartLine)
		}
		period, err := indicatorbinding.ParsePositiveInt(args[1])
		if err != nil {
			return indicatorBinding{}, false, fmt.Errorf("pine line %d: supertrend() atrPeriod must be a positive integer", statement.Range.StartLine)
		}
		factorText := strconv.FormatFloat(factor, 'f', -1, 64)
		key := "supertrend:" + factorText + ":" + strconv.Itoa(period)
		return indicatorBinding{Alias: statement.Name, Kind: "supertrend", Key: key, Args: []string{factorText, strconv.Itoa(period)}}, true, nil
	case "sar":
		if len(args) != 3 {
			return indicatorBinding{}, false, fmt.Errorf("pine line %d: sar() requires start, increment, and max", statement.Range.StartLine)
		}
		start, err := indicatorbinding.ParsePositiveFloat(args[0])
		if err != nil {
			return indicatorBinding{}, false, fmt.Errorf("pine line %d: sar() start must be a positive number", statement.Range.StartLine)
		}
		increment, err := indicatorbinding.ParsePositiveFloat(args[1])
		if err != nil {
			return indicatorBinding{}, false, fmt.Errorf("pine line %d: sar() increment must be a positive number", statement.Range.StartLine)
		}
		maximum, err := indicatorbinding.ParsePositiveFloat(args[2])
		if err != nil {
			return indicatorBinding{}, false, fmt.Errorf("pine line %d: sar() max must be a positive number", statement.Range.StartLine)
		}
		key := "sar:" + strconv.FormatFloat(start, 'f', -1, 64) + ":" + strconv.FormatFloat(increment, 'f', -1, 64) + ":" + strconv.FormatFloat(maximum, 'f', -1, 64)
		return indicatorBinding{Alias: statement.Name, Kind: "sar", Key: key, Args: []string{strconv.FormatFloat(start, 'f', -1, 64), strconv.FormatFloat(increment, 'f', -1, 64), strconv.FormatFloat(maximum, 'f', -1, 64)}}, true, nil
	case "security_source":
		if len(args) < 2 || len(args) > 3 {
			return indicatorBinding{}, false, fmt.Errorf("pine line %d: security_source() requires source, time unit, and optional lookback", statement.Range.StartLine)
		}
		source, ok := indicatorbinding.ParsePriceSource(args[0])
		if !ok {
			return indicatorBinding{}, false, fmt.Errorf("pine line %d: security_source() source %q is not supported; use open/high/low/close/volume/hl2/hlc3/ohlc4", statement.Range.StartLine, strings.TrimSpace(args[0]))
		}
		timeUnit, ok := indicatorbinding.ParseIndicatorTimeUnitValue(args[1])
		if !ok || timeUnit == "" {
			return indicatorBinding{}, false, fmt.Errorf("pine line %d: security_source() time unit %q is not supported", statement.Range.StartLine, strings.TrimSpace(args[1]))
		}
		lookback := 0
		if len(args) == 3 {
			parsed, err := strconv.Atoi(strings.TrimSpace(args[2]))
			if err != nil || parsed < 0 {
				return indicatorBinding{}, false, fmt.Errorf("pine line %d: security_source() lookback must be a non-negative integer", statement.Range.StartLine)
			}
			lookback = parsed
		}
		key := "security_source:" + timeUnit + ":" + source
		bindingArgs := []string{source, timeUnit}
		if lookback > 0 {
			key += ":" + strconv.Itoa(lookback)
			bindingArgs = append(bindingArgs, strconv.Itoa(lookback))
		}
		return indicatorBinding{Alias: statement.Name, Kind: "security_source", Key: key, Args: bindingArgs}, true, nil
	case "williams_r", "williamsr":
		period, err := indicatorbinding.ExpectOnePositiveIntArg(statement.Range.StartLine, name, args)
		if err != nil {
			return indicatorBinding{}, false, err
		}
		return indicatorBinding{Alias: statement.Name, Kind: "williamsr", Key: "williamsr:" + strconv.Itoa(period), Args: []string{strconv.Itoa(period)}}, true, nil
	case "bollinger":
		if len(args) != 2 {
			return indicatorBinding{}, false, fmt.Errorf("pine line %d: bollinger() requires period and multiplier", statement.Range.StartLine)
		}
		period, err := indicatorbinding.ParsePositiveInt(args[0])
		if err != nil {
			return indicatorBinding{}, false, fmt.Errorf("pine line %d: bollinger() period must be a positive integer", statement.Range.StartLine)
		}
		multiplier, err := indicatorbinding.ParsePositiveFloat(args[1])
		if err != nil {
			return indicatorBinding{}, false, fmt.Errorf("pine line %d: bollinger() multiplier must be a positive number", statement.Range.StartLine)
		}
		multiplierText := strconv.FormatFloat(multiplier, 'f', -1, 64)
		return indicatorBinding{Alias: statement.Name, Kind: "bollinger", Key: "bollinger:" + strconv.Itoa(period) + ":" + multiplierText, Args: []string{strconv.Itoa(period), multiplierText}}, true, nil
	default:
		return indicatorBinding{}, false, nil
	}
}

func parseAdvancedRuntimeBinding(lineNumber int, alias, name string, args []string) (indicatorBinding, bool, error) {
	planned, recognized, err := parseAdvancedIndicatorBindingRuntime(lineNumber, alias, name, args)
	return planned, recognized, err
}

func parseAdvancedIndicatorBindingRuntime(lineNumber int, alias, name string, args []string) (indicatorBinding, bool, error) {
	sourceArg := func(value string) (string, error) {
		source, ok := indicatorbinding.ParsePriceSource(value)
		if !ok {
			return "", fmt.Errorf("pine line %d: %s() source %q is not supported", lineNumber, name, strings.TrimSpace(value))
		}
		return source, nil
	}
	timeUnit := ""
	parseTimeUnit := func(index int) error {
		if len(args) <= index {
			return nil
		}
		if len(args) != index+1 {
			return fmt.Errorf("pine line %d: %s() received an invalid argument count", lineNumber, name)
		}
		parsed, ok := indicatorbinding.ParseIndicatorTimeUnitValue(args[index])
		if !ok || parsed == "" {
			return fmt.Errorf("pine line %d: %s() timeframe %q is not supported", lineNumber, name, strings.TrimSpace(args[index]))
		}
		timeUnit = parsed
		args = args[:index]
		return nil
	}
	withTimeUnit := func(key string) string {
		if timeUnit == "" {
			return key
		}
		return key + ":" + timeUnit
	}
	switch name {
	case "cmo", "dev", "median", "percentrank":
		if err := parseTimeUnit(2); err != nil {
			return indicatorBinding{}, false, err
		}
		if len(args) != 2 {
			return indicatorBinding{}, false, fmt.Errorf("pine line %d: %s() requires source and length", lineNumber, name)
		}
		source, err := sourceArg(args[0])
		if err != nil {
			return indicatorBinding{}, false, err
		}
		period, err := indicatorbinding.ParsePositiveInt(args[1])
		if err != nil {
			return indicatorBinding{}, false, err
		}
		return indicatorBinding{Alias: alias, Kind: name, Key: withTimeUnit(fmt.Sprintf("%s:%s:%d", name, source, period)), Args: []string{source, strconv.Itoa(period)}}, true, nil
	case "tsi":
		if err := parseTimeUnit(3); err != nil {
			return indicatorBinding{}, false, err
		}
		if len(args) != 3 {
			return indicatorBinding{}, false, fmt.Errorf("pine line %d: tsi() requires source, short length, and long length", lineNumber)
		}
		source, err := sourceArg(args[0])
		if err != nil {
			return indicatorBinding{}, false, err
		}
		shortPeriod, err := indicatorbinding.ParsePositiveInt(args[1])
		if err != nil {
			return indicatorBinding{}, false, err
		}
		longPeriod, err := indicatorbinding.ParsePositiveInt(args[2])
		if err != nil {
			return indicatorBinding{}, false, err
		}
		return indicatorBinding{Alias: alias, Kind: name, Key: withTimeUnit(fmt.Sprintf("tsi:%s:%d:%d", source, shortPeriod, longPeriod)), Args: []string{source, strconv.Itoa(shortPeriod), strconv.Itoa(longPeriod)}}, true, nil
	case "correlation":
		if err := parseTimeUnit(3); err != nil {
			return indicatorBinding{}, false, err
		}
		if len(args) != 3 {
			return indicatorBinding{}, false, fmt.Errorf("pine line %d: correlation() requires source, second source, and length", lineNumber)
		}
		source, err := sourceArg(args[0])
		if err != nil {
			return indicatorBinding{}, false, err
		}
		source2, err := sourceArg(args[1])
		if err != nil {
			return indicatorBinding{}, false, err
		}
		period, err := indicatorbinding.ParsePositiveInt(args[2])
		if err != nil {
			return indicatorBinding{}, false, err
		}
		return indicatorBinding{Alias: alias, Kind: name, Key: withTimeUnit(fmt.Sprintf("correlation:%s:%s:%d", source, source2, period)), Args: []string{source, source2, strconv.Itoa(period)}}, true, nil
	case "percentile_linear_interpolation", "percentile_nearest_rank":
		if err := parseTimeUnit(3); err != nil {
			return indicatorBinding{}, false, err
		}
		if len(args) != 3 {
			return indicatorBinding{}, false, fmt.Errorf("pine line %d: %s() requires source, length, and percentage", lineNumber, name)
		}
		source, err := sourceArg(args[0])
		if err != nil {
			return indicatorBinding{}, false, err
		}
		period, err := indicatorbinding.ParsePositiveInt(args[1])
		if err != nil {
			return indicatorBinding{}, false, err
		}
		percentage, err := strconv.ParseFloat(strings.TrimSpace(args[2]), 64)
		if err != nil || percentage < 0 || percentage > 100 {
			return indicatorBinding{}, false, fmt.Errorf("pine line %d: %s() percentage must be between 0 and 100", lineNumber, name)
		}
		return indicatorBinding{Alias: alias, Kind: name, Key: withTimeUnit(fmt.Sprintf("%s:%s:%d:%s", name, source, period, strconv.FormatFloat(percentage, 'f', -1, 64))), Args: []string{source, strconv.Itoa(period), strconv.FormatFloat(percentage, 'f', -1, 64)}}, true, nil
	case "swma":
		if err := parseTimeUnit(1); err != nil {
			return indicatorBinding{}, false, err
		}
		if len(args) != 1 {
			return indicatorBinding{}, false, fmt.Errorf("pine line %d: swma() requires one source", lineNumber)
		}
		source, err := sourceArg(args[0])
		if err != nil {
			return indicatorBinding{}, false, err
		}
		return indicatorBinding{Alias: alias, Kind: name, Key: withTimeUnit("swma:" + source), Args: []string{source}}, true, nil
	case "linreg":
		if err := parseTimeUnit(3); err != nil {
			return indicatorBinding{}, false, err
		}
		if len(args) != 3 {
			return indicatorBinding{}, false, fmt.Errorf("pine line %d: linreg() requires source, length, and offset", lineNumber)
		}
		source, err := sourceArg(args[0])
		if err != nil {
			return indicatorBinding{}, false, err
		}
		period, err := indicatorbinding.ParsePositiveInt(args[1])
		if err != nil {
			return indicatorBinding{}, false, err
		}
		offset, err := strconv.Atoi(strings.TrimSpace(args[2]))
		if err != nil || offset < 0 {
			return indicatorBinding{}, false, fmt.Errorf("pine line %d: linreg() offset must be non-negative", lineNumber)
		}
		return indicatorBinding{Alias: alias, Kind: name, Key: withTimeUnit(fmt.Sprintf("linreg:%s:%d:%d", source, period, offset)), Args: args}, true, nil
	case "obv":
		if len(args) == 0 {
			args = []string{"close"}
		}
		if err := parseTimeUnit(1); err != nil {
			return indicatorBinding{}, false, err
		}
		if len(args) != 1 {
			return indicatorBinding{}, false, fmt.Errorf("pine line %d: obv() accepts one source", lineNumber)
		}
		source, err := sourceArg(args[0])
		if err != nil {
			return indicatorBinding{}, false, err
		}
		return indicatorBinding{Alias: alias, Kind: name, Key: withTimeUnit("obv:" + source), Args: args}, true, nil
	case "pivothigh", "pivotlow":
		if err := parseTimeUnit(3); err != nil {
			return indicatorBinding{}, false, err
		}
		source := "high"
		if name == "pivotlow" {
			source = "low"
		}
		lengthArgs := args
		if len(args) == 3 {
			var err error
			source, err = sourceArg(args[0])
			if err != nil {
				return indicatorBinding{}, false, err
			}
			lengthArgs = args[1:]
		}
		if len(lengthArgs) != 2 {
			return indicatorBinding{}, false, fmt.Errorf("pine line %d: %s() requires left and right bars", lineNumber, name)
		}
		left, err := indicatorbinding.ParsePositiveInt(lengthArgs[0])
		if err != nil {
			return indicatorBinding{}, false, err
		}
		right, err := indicatorbinding.ParsePositiveInt(lengthArgs[1])
		if err != nil {
			return indicatorBinding{}, false, err
		}
		return indicatorBinding{Alias: alias, Kind: name, Key: withTimeUnit(fmt.Sprintf("%s:%s:%d:%d", name, source, left, right)), Args: args}, true, nil
	case "kc", "kcw":
		if err := parseTimeUnit(4); err != nil {
			return indicatorBinding{}, false, err
		}
		if len(args) < 3 || len(args) > 4 {
			return indicatorBinding{}, false, fmt.Errorf("pine line %d: %s() requires source, length, multiplier, and optional useTrueRange", lineNumber, name)
		}
		source, err := sourceArg(args[0])
		if err != nil {
			return indicatorBinding{}, false, err
		}
		period, err := indicatorbinding.ParsePositiveInt(args[1])
		if err != nil {
			return indicatorBinding{}, false, err
		}
		multiplier, err := indicatorbinding.ParsePositiveFloat(args[2])
		if err != nil {
			return indicatorBinding{}, false, err
		}
		useTR := true
		if len(args) == 4 {
			useTR, err = strconv.ParseBool(strings.TrimSpace(args[3]))
			if err != nil {
				return indicatorBinding{}, false, err
			}
		}
		return indicatorBinding{Alias: alias, Kind: name, Key: withTimeUnit(fmt.Sprintf("%s:%s:%d:%s:%t", name, source, period, strconv.FormatFloat(multiplier, 'f', -1, 64), useTR)), Args: args}, true, nil
	case "alma":
		if err := parseTimeUnit(4); err != nil {
			return indicatorBinding{}, false, err
		}
		if len(args) != 4 {
			return indicatorBinding{}, false, fmt.Errorf("pine line %d: alma() requires source, length, offset, and sigma", lineNumber)
		}
		source, err := sourceArg(args[0])
		if err != nil {
			return indicatorBinding{}, false, err
		}
		period, err := indicatorbinding.ParsePositiveInt(args[1])
		if err != nil {
			return indicatorBinding{}, false, err
		}
		offset, err := strconv.ParseFloat(strings.TrimSpace(args[2]), 64)
		if err != nil {
			return indicatorBinding{}, false, err
		}
		sigma, err := indicatorbinding.ParsePositiveFloat(args[3])
		if err != nil {
			return indicatorBinding{}, false, err
		}
		return indicatorBinding{Alias: alias, Kind: name, Key: withTimeUnit(fmt.Sprintf("alma:%s:%d:%s:%s", source, period, strconv.FormatFloat(offset, 'f', -1, 64), strconv.FormatFloat(sigma, 'f', -1, 64))), Args: args}, true, nil
	default:
		return indicatorBinding{}, false, nil
	}
}

func parseStochSource(value string) (string, bool) {
	source, ok := indicatorbinding.ParsePriceSource(value)
	if !ok || source == "volume" {
		return "", false
	}
	return source, true
}

func parseSourcePeriodArgs(lineNumber int, name string, args []string, defaultSource string, defaultPeriod string) (string, int, error) {
	sourceText, periodText := defaultSource, defaultPeriod
	if len(args) == 1 {
		periodText = strings.TrimSpace(args[0])
	} else if len(args) >= 2 {
		sourceText = strings.TrimSpace(args[0])
		periodText = strings.TrimSpace(args[1])
	}
	source, ok := indicatorbinding.ParsePriceSource(sourceText)
	if !ok {
		return "", 0, fmt.Errorf("pine line %d: %s() source %q is not supported; use open/high/low/close/volume/hl2/hlc3/ohlc4", lineNumber, name, sourceText)
	}
	period, err := indicatorbinding.ParsePositiveInt(periodText)
	if err != nil {
		return "", 0, fmt.Errorf("pine line %d: %s() length must be a positive integer", lineNumber, name)
	}
	return source, period, nil
}

func sourcePeriodRuntimeKey(prefix string, source string, period int, legacySource string) string {
	if strings.TrimSpace(source) == "" || source == legacySource {
		return prefix + ":" + strconv.Itoa(period)
	}
	return prefix + ":" + source + ":" + strconv.Itoa(period)
}

func sourcePeriodRuntimeArgs(source string, period int, legacySource string) []string {
	periodText := strconv.Itoa(period)
	if strings.TrimSpace(source) == "" || source == legacySource {
		return []string{periodText}
	}
	return []string{source, periodText}
}

func buildDivergenceRequirementKey(binding indicatorBinding, direction string, lookback int) (string, bool) {
	switch binding.Kind {
	case "rsi":
		return "divergence:" + binding.Key + ":" + direction + ":" + strconv.Itoa(lookback), true
	case "macd":
		return "divergence:" + binding.Key + ":" + direction + ":" + strconv.Itoa(lookback), true
	case "kdj":
		return "divergence:" + binding.Key + ":" + direction + ":" + strconv.Itoa(lookback), true
	default:
		return "", false
	}
}

func buildProtectRequirementKey(statement *strategyir.ProtectStmt) (string, error) {
	mode, ok := indicatorbinding.ParseProtectMode(statement.Mode)
	if !ok {
		return "", fmt.Errorf("pine line %d: protect mode %q is not supported", statement.Range.StartLine, strings.TrimSpace(statement.Mode))
	}
	direction, ok := indicatorbinding.ParseProtectDirection(statement.Direction)
	if !ok {
		return "", fmt.Errorf("pine line %d: protect direction %q is not supported", statement.Range.StartLine, strings.TrimSpace(statement.Direction))
	}
	timeValue, err := indicatorbinding.ParsePositiveInt(statement.TimeValueExpression)
	if err != nil {
		return "", fmt.Errorf("pine line %d: protect time value must be a positive integer", statement.Range.StartLine)
	}
	timeUnit, ok := indicatorbinding.ParseIndicatorTimeUnitValue(statement.TimeUnit)
	if !ok {
		return "", fmt.Errorf("pine line %d: protect time unit %q is not supported", statement.Range.StartLine, strings.TrimSpace(statement.TimeUnit))
	}
	if timeUnit == "" {
		timeUnit = "bar"
	}
	percentage, err := indicatorbinding.ParsePercentage(statement.PercentageExpression)
	if err != nil {
		return "", fmt.Errorf("pine line %d: protect percentage must be a positive number", statement.Range.StartLine)
	}
	windowPolicy, ok := indicatorbinding.ParseProtectWindowPolicy(statement.WindowPolicy)
	if !ok {
		return "", fmt.Errorf("pine line %d: protect window policy %q is not supported", statement.Range.StartLine, strings.TrimSpace(statement.WindowPolicy))
	}
	if mode == "stopLoss" && windowPolicy == "continuous" {
		return fmt.Sprintf("sl:%s:%d:%s:%s", direction, timeValue, timeUnit, strconv.FormatFloat(percentage, 'f', -1, 64)), nil
	}
	return fmt.Sprintf("risk:%s:%s:%d:%s:%s:%s", mode, direction, timeValue, timeUnit, strconv.FormatFloat(percentage, 'f', -1, 64), windowPolicy), nil
}
