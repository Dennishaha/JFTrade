package pineruntime

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	bbgo2 "github.com/c9s/bbgo/pkg/bbgo"
	"github.com/c9s/bbgo/pkg/fixedpoint"
	"github.com/c9s/bbgo/pkg/types"
	exprast "github.com/expr-lang/expr/ast"
	trdcommonpb "github.com/jftrade/jftrade-main/pkg/futu/pb/trdcommon"
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
	mu               sync.Mutex
	ctx              context.Context
	strategy         *Strategy
	program          *strategyir.Program
	plan             strategyir.Requirements
	hooks            map[strategyir.HookKind]*strategyir.HookBlock
	ifScopePlans     map[*strategyir.IfStmt]ifScopePlan
	displayName      string
	definitionID     string
	symbol           string
	interval         types.Interval
	expressionCache  map[string]exprast.Node
	bindingCache     map[*strategyir.LetStmt]cachedIndicatorBinding
	protectCache     map[*strategyir.ProtectStmt]cachedProtectRequirement
	divergenceCache  map[divergenceRequirementCacheKey]string
	engine           *strategyindicatorruntime.IndicatorEngine
	session          *bbgo2.ExchangeSession
	executor         bbgo2.OrderExecutor
	baseScope        *evaluationScope
	reusableScope    *evaluationScope
	persistentValues map[string]any
	variableCapacity int
	bindingCapacity  int
	previousClose    float64
	previousOpen     float64
	previousHigh     float64
	previousLow      float64
	previousVolume   float64
	hasPreviousClose bool
	barIndex         int
	positionCache    cachedPositionSnapshot
	entrySubmitCount map[string]int
	maxPyramiding    int
	pendingOrders    map[string]pendingOrder
	pendingSequence  int
	barssinceStates  map[string]*barssinceState
	valuewhenStates  map[string]*valuewhenState
	historyTargets   map[string]historyTarget
	historyValues    map[string]*historyBuffer
}

type historyBuffer struct {
	values []any
	next   int
	count  int
}

func newHistoryBuffer(capacity int) *historyBuffer {
	if capacity < 1 {
		capacity = 1
	}
	return &historyBuffer{values: make([]any, capacity)}
}

func (b *historyBuffer) push(value any) {
	if b == nil || len(b.values) == 0 {
		return
	}
	b.values[b.next] = value
	b.next = (b.next + 1) % len(b.values)
	if b.count < len(b.values) {
		b.count++
	}
}

func (b *historyBuffer) lookup(lookback int) (any, bool) {
	if b == nil || lookback <= 0 || lookback > b.count {
		return nil, false
	}
	index := b.next - lookback
	if index < 0 {
		index += len(b.values)
	}
	return b.values[index], true
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
	id         string
	sequence   int
	action     strategyir.OrderAction
	intent     strategyir.OrderIntent
	orderType  types.OrderType
	quantity   float64
	limitPrice float64
	stopPrice  float64
	hasLimit   bool
	hasStop    bool
	comment    string
	alert      string
	disable    bool
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

func (s *evaluationScope) variable(name string) (any, bool) {
	for current := s; current != nil; current = current.parent {
		if current.variables == nil {
			if value, ok := current.reservedVariable(name); ok {
				return value, true
			}
			continue
		}
		value, ok := current.variables[name]
		if ok {
			return value, true
		}
		if value, ok := current.reservedVariable(name); ok {
			return value, true
		}
	}
	return nil, false
}

func (s *evaluationScope) reservedVariable(name string) (any, bool) {
	if s == nil {
		return nil, false
	}
	switch name {
	case "indicators":
		if s.indicators == nil {
			return nil, false
		}
		return s.indicators, true
	case "kline":
		if s.currentKline == nil {
			return nil, false
		}
		return &s.klinePayload, true
	case "close":
		if !s.hasBarData {
			return nil, false
		}
		return &s.closeSeries, true
	case "open":
		if !s.hasBarData {
			return nil, false
		}
		return &s.openSeries, true
	case "high":
		if !s.hasBarData {
			return nil, false
		}
		return &s.highSeries, true
	case "low":
		if !s.hasBarData {
			return nil, false
		}
		return &s.lowSeries, true
	case "volume":
		if !s.hasBarData {
			return nil, false
		}
		return &s.volumeSeries, true
	case "hl2":
		if !s.hasBarData {
			return nil, false
		}
		return &s.hl2Series, true
	case "hlc3":
		if !s.hasBarData {
			return nil, false
		}
		return &s.hlc3Series, true
	case "ohlc4":
		if !s.hasBarData {
			return nil, false
		}
		return &s.ohlc4Series, true
	case "position_size":
		position := s.currentPosition()
		if position == nil {
			return 0.0, true
		}
		return position.Quantity, true
	case "position_avg_price":
		position := s.currentPosition()
		if position == nil || position.Quantity == 0 || position.AveragePrice <= 0 {
			return nil, true
		}
		return position.AveragePrice, true
	case "equity":
		if s.runtime == nil {
			return 0.0, true
		}
		return s.runtime.getTotalAccountValue(), true
	case "bar_index":
		return float64(s.barIndex), true
	case "time":
		if s.currentKline == nil {
			return nil, false
		}
		return float64(pineBarTime(s.currentKline).UnixMilli()), true
	case "hour":
		if s.currentKline == nil {
			return nil, false
		}
		return float64(pineBarTime(s.currentKline).Hour()), true
	case "minute":
		if s.currentKline == nil {
			return nil, false
		}
		return float64(pineBarTime(s.currentKline).Minute()), true
	case "dayofweek":
		if s.currentKline == nil {
			return nil, false
		}
		return float64(pineDayOfWeek(pineBarTime(s.currentKline))), true
	case "dayofmonth":
		if s.currentKline == nil {
			return nil, false
		}
		return float64(pineBarTime(s.currentKline).Day()), true
	case "month":
		if s.currentKline == nil {
			return nil, false
		}
		return float64(pineBarTime(s.currentKline).Month()), true
	case "year":
		if s.currentKline == nil {
			return nil, false
		}
		return float64(pineBarTime(s.currentKline).Year()), true
	case "syminfo_tickerid":
		if s.runtime == nil {
			return "", true
		}
		return s.runtime.symbol, true
	case "syminfo_prefix":
		if s.runtime == nil {
			return "", true
		}
		return pineSymbolPrefix(s.runtime.symbol), true
	case "timeframe_period":
		if s.runtime == nil {
			return "", true
		}
		return string(s.runtime.interval), true
	case "timeframe_isintraday":
		if s.runtime == nil {
			return false, true
		}
		return pineTimeframeIsIntraday(s.runtime.interval), true
	case "timeframe_isminutes":
		if s.runtime == nil {
			return false, true
		}
		return pineTimeframeIsMinutes(s.runtime.interval), true
	case "timeframe_isdaily":
		if s.runtime == nil {
			return false, true
		}
		return pineTimeframeUnit(s.runtime.interval) == "day", true
	case "timeframe_isweekly":
		if s.runtime == nil {
			return false, true
		}
		return pineTimeframeUnit(s.runtime.interval) == "week", true
	case "timeframe_ismonthly":
		if s.runtime == nil {
			return false, true
		}
		return pineTimeframeUnit(s.runtime.interval) == "month", true
	case "barstate_isfirst":
		return s.barIndex == 0, true
	case "barstate_isnew":
		return s.hasBarData, true
	case "barstate_isconfirmed":
		return s.hasBarData, true
	case "barstate_ishistory":
		return bbgo2.IsBackTesting, true
	case "barstate_isrealtime":
		return !bbgo2.IsBackTesting, true
	case "barstate_islast":
		return s.hasBarData, true
	case "session_ismarket":
		return s.currentSession == market.SessionRegular, true
	case "session_ispremarket":
		return s.currentSession == market.SessionPre, true
	case "session_ispostmarket":
		return s.currentSession == market.SessionAfter, true
	default:
		return nil, false
	}
}

func (s *evaluationScope) currentPosition() *positionSnapshot {
	if s == nil || s.runtime == nil {
		return nil
	}
	symbol := strings.TrimSpace(s.currentKlineSymbol)
	if symbol == "" {
		symbol = s.runtime.symbol
	}
	if symbol == "" {
		return nil
	}
	return s.runtime.getPosition(symbol, s.currentKlineTime)
}

func (s *evaluationScope) setVariable(name string, value any) {
	if s == nil {
		return
	}
	if s.variables == nil {
		s.variables = map[string]any{}
	}
	s.variables[name] = value
}

func (s *evaluationScope) assignVariable(name string, value any) {
	if s == nil {
		return
	}
	for current := s; current != nil; current = current.parent {
		if current.variables == nil {
			continue
		}
		if _, ok := current.variables[name]; ok {
			current.variables[name] = value
			return
		}
	}
	s.setVariable(name, value)
}

func (s *evaluationScope) binding(name string) (indicatorBinding, bool) {
	for current := s; current != nil; current = current.parent {
		if current.bindings == nil {
			continue
		}
		value, ok := current.bindings[name]
		if ok {
			return value, true
		}
	}
	return indicatorBinding{}, false
}

func (s *evaluationScope) setBinding(name string, binding indicatorBinding) {
	if s == nil {
		return
	}
	if s.bindings == nil {
		s.bindings = map[string]indicatorBinding{}
	}
	s.bindings[name] = binding
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
		ctx:              ctx,
		strategy:         strategy,
		program:          program,
		plan:             plan,
		hooks:            buildHookCache(program),
		ifScopePlans:     buildIfScopePlans(program),
		displayName:      displayName,
		definitionID:     definitionID,
		symbol:           symbol,
		interval:         interval,
		expressionCache:  map[string]exprast.Node{},
		bindingCache:     map[*strategyir.LetStmt]cachedIndicatorBinding{},
		protectCache:     map[*strategyir.ProtectStmt]cachedProtectRequirement{},
		divergenceCache:  map[divergenceRequirementCacheKey]string{},
		persistentValues: map[string]any{},
		engine:           engine,
		session:          session,
		executor:         orderExecutor,
		entrySubmitCount: map[string]int{},
		maxPyramiding:    normalizeRuntimePyramiding(program),
		pendingOrders:    map[string]pendingOrder{},
		barssinceStates:  map[string]*barssinceState{},
		valuewhenStates:  map[string]*valuewhenState{},
		historyTargets:   collectProgramHistoryTargets(program),
		barIndex:         -1,
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

func (r *strategyRuntime) cachedDivergenceRequirementKey(binding indicatorBinding, direction string, lookback int) (string, bool) {
	cacheKey := divergenceRequirementCacheKey{bindingKey: binding.Key, direction: direction, lookback: lookback}
	if r != nil && r.divergenceCache != nil {
		if cached, hit := r.divergenceCache[cacheKey]; hit {
			return cached, true
		}
	}
	key, ok := buildDivergenceRequirementKey(binding, direction, lookback)
	if !ok || r == nil || r.divergenceCache == nil {
		return key, ok
	}
	r.divergenceCache[cacheKey] = key
	return key, true
}

func (r *strategyRuntime) runInit() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.runHookLocked(strategyir.HookInit, nil, market.SessionUnknown)
}

func (r *strategyRuntime) handleKLineClosed(kline types.KLine) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if symbol := r.symbol; symbol != "" && kline.Symbol != symbol {
		return
	}
	if interval := r.interval; interval != "" && kline.Interval != interval {
		return
	}
	r.barIndex++

	resolvedSession := r.resolveKLineSession(kline)
	if r.engine != nil {
		r.engine.Push(kline, resolvedSession)
	}
	if err := r.triggerPendingOrders(&kline); err != nil {
		errMsg := err.Error()
		bbgo2.Notify("pine strategy %s pending order error: %s", r.displayName, errMsg)
		if r.strategy.OnError != nil {
			r.strategy.OnError(errMsg)
		}
	}
	if err := r.runHookLocked(strategyir.HookKLineClose, &kline, resolvedSession); err != nil {
		errMsg := err.Error()
		bbgo2.Notify("pine strategy %s onKLineClosed error: %s", r.displayName, errMsg)
		if r.strategy.OnError != nil {
			r.strategy.OnError(errMsg)
		}
	}
	r.previousClose = kline.Close.Float64()
	r.previousOpen = kline.Open.Float64()
	r.previousHigh = kline.High.Float64()
	r.previousLow = kline.Low.Float64()
	r.previousVolume = kline.Volume.Float64()
	r.hasPreviousClose = true
}

func (r *strategyRuntime) runHookLocked(kind strategyir.HookKind, kline *types.KLine, session market.Session) error {
	hook := r.hooks[kind]
	if hook == nil {
		return nil
	}
	scope := r.newScope(kline, session)
	_, err := r.executeStatements(hook.Statements, scope)
	if err != nil {
		return err
	}
	r.recordHistorySnapshots(scope)
	return nil
}

func buildHookCache(program *strategyir.Program) map[strategyir.HookKind]*strategyir.HookBlock {
	hooks := make(map[strategyir.HookKind]*strategyir.HookBlock)
	if program == nil {
		return hooks
	}
	for index := range program.Hooks {
		hook := &program.Hooks[index]
		hooks[hook.Kind] = hook
	}
	return hooks
}

func findHook(program *strategyir.Program, kind strategyir.HookKind) (strategyir.HookBlock, bool) {
	if program == nil {
		return strategyir.HookBlock{}, false
	}
	for _, hook := range program.Hooks {
		if hook.Kind == kind {
			return hook, true
		}
	}
	return strategyir.HookBlock{}, false
}

func countProgramLetStatements(program *strategyir.Program) int {
	if program == nil {
		return 0
	}
	total := 0
	for _, hook := range program.Hooks {
		total += countLetStatements(hook.Statements)
	}
	return total
}

func countLetStatements(statements []strategyir.Statement) int {
	total := 0
	for _, statement := range statements {
		switch typed := statement.(type) {
		case *strategyir.LetStmt:
			total++
		case *strategyir.IfStmt:
			total += countLetStatements(typed.Then)
			total += countLetStatements(typed.Else)
		}
	}
	return total
}

func buildIfScopePlans(program *strategyir.Program) map[*strategyir.IfStmt]ifScopePlan {
	if program == nil {
		return nil
	}
	plans := make(map[*strategyir.IfStmt]ifScopePlan)
	for _, hook := range program.Hooks {
		collectIfScopePlans(hook.Statements, plans)
	}
	return plans
}

func collectIfScopePlans(statements []strategyir.Statement, plans map[*strategyir.IfStmt]ifScopePlan) {
	for _, statement := range statements {
		typed, ok := statement.(*strategyir.IfStmt)
		if !ok {
			continue
		}
		plans[typed] = ifScopePlan{
			thenNeedsClone: countLetStatements(typed.Then) > 0,
			elseNeedsClone: countLetStatements(typed.Else) > 0,
		}
		collectIfScopePlans(typed.Then, plans)
		collectIfScopePlans(typed.Else, plans)
	}
}

func collectProgramHistoryTargets(program *strategyir.Program) map[string]historyTarget {
	targets := map[string]historyTarget{}
	if program == nil {
		return targets
	}
	for _, hook := range program.Hooks {
		collectStatementHistoryTargets(hook.Statements, targets)
	}
	return targets
}

func collectStatementHistoryTargets(statements []strategyir.Statement, targets map[string]historyTarget) {
	for _, statement := range statements {
		switch typed := statement.(type) {
		case *strategyir.LetStmt:
			collectExpressionHistoryTargets(typed.Expression, targets)
		case *strategyir.IfStmt:
			collectExpressionHistoryTargets(typed.Condition, targets)
			collectStatementHistoryTargets(typed.Then, targets)
			collectStatementHistoryTargets(typed.Else, targets)
		case *strategyir.OrderStmt:
			collectExpressionHistoryTargets(typed.QuantityExpression, targets)
			collectExpressionHistoryTargets(typed.LimitExpression, targets)
			collectExpressionHistoryTargets(typed.StopExpression, targets)
		case *strategyir.ExitStmt:
			collectExpressionHistoryTargets(typed.QuantityExpression, targets)
			collectExpressionHistoryTargets(typed.StopExpression, targets)
			collectExpressionHistoryTargets(typed.LimitExpression, targets)
			collectExpressionHistoryTargets(typed.TrailPoints, targets)
			collectExpressionHistoryTargets(typed.TrailOffset, targets)
		case *strategyir.ProtectStmt:
			collectExpressionHistoryTargets(typed.QuantityExpression, targets)
			collectExpressionHistoryTargets(typed.TimeValueExpression, targets)
			collectExpressionHistoryTargets(typed.PercentageExpression, targets)
		}
	}
}

func (r *strategyRuntime) preparseProgramExpressions() error {
	if r == nil || r.program == nil {
		return nil
	}
	scope := &evaluationScope{runtime: r}
	for _, hook := range r.program.Hooks {
		if err := preparseStatementExpressions(hook.Statements, scope); err != nil {
			return err
		}
	}
	return nil
}

func preparseStatementExpressions(statements []strategyir.Statement, scope *evaluationScope) error {
	for _, statement := range statements {
		var expressions []string
		switch typed := statement.(type) {
		case *strategyir.LetStmt:
			expressions = []string{typed.Expression}
		case *strategyir.IfStmt:
			expressions = []string{typed.Condition}
			if err := preparseStatementExpressions(typed.Then, scope); err != nil {
				return err
			}
			if err := preparseStatementExpressions(typed.Else, scope); err != nil {
				return err
			}
		case *strategyir.OrderStmt:
			expressions = []string{typed.QuantityExpression, typed.LimitExpression, typed.StopExpression}
		case *strategyir.ExitStmt:
			expressions = []string{typed.QuantityExpression, typed.StopExpression, typed.LimitExpression, typed.TrailPoints, typed.TrailOffset}
		case *strategyir.ProtectStmt:
			expressions = []string{typed.QuantityExpression, typed.TimeValueExpression, typed.PercentageExpression}
		}
		for _, expression := range expressions {
			if strings.TrimSpace(expression) == "" {
				continue
			}
			if _, err := parseExpression(expression, scope); err != nil {
				return fmt.Errorf("line %d: %w", statement.SourceRange().StartLine, err)
			}
		}
	}
	return nil
}

func collectExpressionHistoryTargets(expression string, targets map[string]historyTarget) {
	if strings.TrimSpace(expression) == "" || targets == nil {
		return
	}
	parsed, err := parseExpression(expression, nil)
	if err != nil {
		return
	}
	collectHistoryTargetsFromNode(parsed, targets)
}

func collectHistoryTargetsFromNode(node exprast.Node, targets map[string]historyTarget) {
	switch typed := node.(type) {
	case nil:
		return
	case *exprast.CallNode:
		name, ok := typed.Callee.(*exprast.IdentifierNode)
		if ok && strings.EqualFold(strings.TrimSpace(name.Value), "history") && len(typed.Arguments) == 2 {
			lookback := historyLookbackFromNode(typed.Arguments[1])
			key := expressionNodeKey(typed.Arguments[0])
			if existing, ok := targets[key]; !ok || lookback > existing.maxLookback {
				targets[key] = historyTarget{key: key, expression: typed.Arguments[0], maxLookback: lookback}
			}
		}
		collectHistoryTargetsFromNode(typed.Callee, targets)
		for _, argument := range typed.Arguments {
			collectHistoryTargetsFromNode(argument, targets)
		}
	case *exprast.UnaryNode:
		collectHistoryTargetsFromNode(typed.Node, targets)
	case *exprast.BinaryNode:
		collectHistoryTargetsFromNode(typed.Left, targets)
		collectHistoryTargetsFromNode(typed.Right, targets)
	case *exprast.MemberNode:
		collectHistoryTargetsFromNode(typed.Node, targets)
		collectHistoryTargetsFromNode(typed.Property, targets)
	case *exprast.PredicateNode:
		collectHistoryTargetsFromNode(typed.Node, targets)
	}
}

func historyLookbackFromNode(node exprast.Node) int {
	switch typed := node.(type) {
	case *exprast.IntegerNode:
		return typed.Value
	case *exprast.FloatNode:
		return int(math.Trunc(typed.Value))
	default:
		return 0
	}
}

func (r *strategyRuntime) recordHistorySnapshots(scope *evaluationScope) {
	if r == nil || scope == nil || len(r.historyTargets) == 0 {
		return
	}
	if r.historyValues == nil {
		r.historyValues = buildHistoryBuffers(r.historyTargets)
	}
	for key, target := range r.historyTargets {
		value, err := evaluateAST(target.expression, scope)
		if err != nil {
			value = nil
		}
		buffer := r.historyValues[key]
		if buffer == nil {
			buffer = newHistoryBuffer(target.maxLookback)
			r.historyValues[key] = buffer
		}
		buffer.push(snapshotExpressionValue(value))
	}
}

func buildHistoryBuffers(targets map[string]historyTarget) map[string]*historyBuffer {
	buffers := make(map[string]*historyBuffer, len(targets))
	for key, target := range targets {
		buffers[key] = newHistoryBuffer(target.maxLookback)
	}
	return buffers
}

func (r *strategyRuntime) newScope(kline *types.KLine, session market.Session) *evaluationScope {
	var indicators map[string]any
	if r.engine != nil {
		indicators = r.engine.SnapshotBorrowed()
	}
	scope := r.reusableScope
	if scope == nil {
		scope = &evaluationScope{
			runtime:   r,
			parent:    r.baseScope,
			variables: make(map[string]any, r.variableCapacity),
		}
		if r.bindingCapacity > 0 {
			scope.bindings = make(map[string]indicatorBinding, r.bindingCapacity)
		}
		r.reusableScope = scope
	}
	scope.parent = r.baseScope
	clear(scope.variables)
	if scope.bindings != nil {
		clear(scope.bindings)
	}
	scope.indicators = indicators
	scope.currentKline = kline
	scope.currentSession = session
	scope.currentKlineTime = time.Time{}
	scope.currentKlineSymbol = ""
	scope.klinePayload = klinePayloadView{}
	scope.closeSeries = seriesNumber{}
	scope.openSeries = seriesNumber{}
	scope.highSeries = seriesNumber{}
	scope.lowSeries = seriesNumber{}
	scope.volumeSeries = seriesNumber{}
	scope.hl2Series = seriesNumber{}
	scope.hlc3Series = seriesNumber{}
	scope.ohlc4Series = seriesNumber{}
	scope.hasBarData = false
	scope.barIndex = r.barIndex
	if kline != nil {
		scope.currentKlineTime = kline.EndTime.Time()
		scope.currentKlineSymbol = kline.Symbol
		scope.klinePayload = klinePayloadView{kline: kline, session: session}
		scope.closeSeries = seriesNumber{Current: kline.Close.Float64(), Previous: r.previousClose, HasCurrent: true, HasPrevious: r.hasPreviousClose}
		scope.openSeries = seriesNumber{Current: kline.Open.Float64(), Previous: r.previousOpen, HasCurrent: true, HasPrevious: r.hasPreviousClose}
		scope.highSeries = seriesNumber{Current: kline.High.Float64(), Previous: r.previousHigh, HasCurrent: true, HasPrevious: r.hasPreviousClose}
		scope.lowSeries = seriesNumber{Current: kline.Low.Float64(), Previous: r.previousLow, HasCurrent: true, HasPrevious: r.hasPreviousClose}
		scope.volumeSeries = seriesNumber{Current: kline.Volume.Float64(), Previous: r.previousVolume, HasCurrent: true, HasPrevious: r.hasPreviousClose}
		currentHL2 := (scope.highSeries.Current + scope.lowSeries.Current) / 2
		currentHLC3 := (scope.highSeries.Current + scope.lowSeries.Current + scope.closeSeries.Current) / 3
		currentOHLC4 := (scope.openSeries.Current + scope.highSeries.Current + scope.lowSeries.Current + scope.closeSeries.Current) / 4
		previousHL2 := (r.previousHigh + r.previousLow) / 2
		previousHLC3 := (r.previousHigh + r.previousLow + r.previousClose) / 3
		previousOHLC4 := (r.previousOpen + r.previousHigh + r.previousLow + r.previousClose) / 4
		scope.hl2Series = seriesNumber{Current: currentHL2, Previous: previousHL2, HasCurrent: true, HasPrevious: r.hasPreviousClose}
		scope.hlc3Series = seriesNumber{Current: currentHLC3, Previous: previousHLC3, HasCurrent: true, HasPrevious: r.hasPreviousClose}
		scope.ohlc4Series = seriesNumber{Current: currentOHLC4, Previous: previousOHLC4, HasCurrent: true, HasPrevious: r.hasPreviousClose}
		scope.hasBarData = true
	}
	return scope
}

func pineBarTime(kline *types.KLine) time.Time {
	if kline == nil {
		return time.Time{}
	}
	if !kline.StartTime.Time().IsZero() {
		return kline.StartTime.Time()
	}
	return kline.EndTime.Time()
}

func pineDayOfWeek(value time.Time) int {
	return int(value.Weekday()) + 1
}

func pineSymbolPrefix(symbol string) string {
	trimmed := strings.TrimSpace(symbol)
	if trimmed == "" {
		return ""
	}
	if index := strings.Index(trimmed, ":"); index > 0 {
		return trimmed[:index]
	}
	if index := strings.Index(trimmed, "."); index > 0 {
		return trimmed[:index]
	}
	return ""
}

func pineTimeframeUnit(interval types.Interval) string {
	value := strings.ToLower(strings.TrimSpace(string(interval)))
	switch {
	case strings.HasSuffix(value, "mo"), strings.HasSuffix(value, "mon"), strings.HasSuffix(value, "month"):
		return "month"
	case strings.HasSuffix(value, "w"):
		return "week"
	case strings.HasSuffix(value, "d"):
		return "day"
	case strings.HasSuffix(value, "h"):
		return "hour"
	case strings.HasSuffix(value, "m"):
		return "minute"
	default:
		return ""
	}
}

func pineTimeframeIsMinutes(interval types.Interval) bool {
	return pineTimeframeUnit(interval) == "minute"
}

func pineTimeframeIsIntraday(interval types.Interval) bool {
	switch pineTimeframeUnit(interval) {
	case "minute", "hour":
		return true
	default:
		return false
	}
}

func (r *strategyRuntime) executeStatements(statements []strategyir.Statement, scope *evaluationScope) (bool, error) {
	for _, statement := range statements {
		switch typed := statement.(type) {
		case *strategyir.LetStmt:
			if err := r.executeLetStatement(typed, scope); err != nil {
				return false, err
			}
		case *strategyir.IfStmt:
			condition, err := evaluateBoolExpression(typed.Condition, scope)
			if err != nil {
				return false, fmt.Errorf("pine line %d: %w", typed.Range.StartLine, err)
			}
			plan := ifScopePlan{thenNeedsClone: true, elseNeedsClone: true}
			if r != nil && r.ifScopePlans != nil {
				if cached, ok := r.ifScopePlans[typed]; ok {
					plan = cached
				}
			}
			if condition {
				branchScope := scope
				if plan.thenNeedsClone {
					branchScope = scope.clone()
				}
				stopped, err := r.executeStatements(typed.Then, branchScope)
				if err != nil || stopped {
					return stopped, err
				}
				continue
			}
			branchScope := scope
			if plan.elseNeedsClone {
				branchScope = scope.clone()
			}
			stopped, err := r.executeStatements(typed.Else, branchScope)
			if err != nil || stopped {
				return stopped, err
			}
		case *strategyir.LogStmt:
			r.log(typed.Message)
		case *strategyir.NotifyStmt:
			r.notify(typed.Message)
		case *strategyir.OrderStmt:
			if err := r.executeOrderStatement(typed, scope); err != nil {
				return false, err
			}
		case *strategyir.ExitStmt:
			stopped, err := r.executeExitStatement(typed, scope)
			if err != nil || stopped {
				return stopped, err
			}
		case *strategyir.CancelStmt:
			r.executeCancelStatement(typed)
		case *strategyir.ProtectStmt:
			stopped, err := r.executeProtectStatement(typed, scope)
			if err != nil || stopped {
				return stopped, err
			}
		default:
			return false, fmt.Errorf("unsupported IR statement type %T", statement)
		}
	}
	return false, nil
}

func (r *strategyRuntime) executeLetStatement(statement *strategyir.LetStmt, scope *evaluationScope) error {
	binding, recognized, err := r.parseIndicatorBinding(statement)
	if err != nil {
		return err
	}
	if recognized {
		scope.setBinding(statement.Name, binding)
		if snapshot, ok := scope.indicators[binding.Key]; ok {
			scope.setVariable(statement.Name, snapshot)
		} else {
			scope.setVariable(statement.Name, nil)
		}
		return nil
	}
	if statement.Mode == strategyir.AssignmentModeVar {
		if r != nil && r.persistentValues != nil {
			if value, ok := r.persistentValues[statement.Name]; ok {
				scope.setVariable(statement.Name, value)
				return nil
			}
		}
	}
	value, err := evaluateExpression(statement.Expression, scope)
	if err != nil {
		return fmt.Errorf("pine line %d: %w", statement.Range.StartLine, err)
	}
	switch statement.Mode {
	case strategyir.AssignmentModeVar:
		if r != nil && r.persistentValues != nil {
			r.persistentValues[statement.Name] = persistentValue(nil, value)
		}
		scope.setVariable(statement.Name, value)
	case strategyir.AssignmentModeReassign:
		if r != nil && r.persistentValues != nil {
			if previous, ok := r.persistentValues[statement.Name]; ok {
				value = persistentValue(previous, value)
				r.persistentValues[statement.Name] = value
			}
		}
		scope.assignVariable(statement.Name, value)
	default:
		scope.setVariable(statement.Name, value)
	}
	return nil
}

func persistentValue(previous any, current any) any {
	currentFloat, currentOK := coerceFloatValue(current)
	if !currentOK {
		return current
	}
	result := seriesNumber{Current: currentFloat, HasCurrent: true}
	if previousFloat, previousOK := coerceFloatValue(previous); previousOK {
		result.Previous = previousFloat
		result.HasPrevious = true
	}
	return result
}

func (r *strategyRuntime) executeOrderStatement(statement *strategyir.OrderStmt, scope *evaluationScope) error {
	position := r.getPosition(r.symbol, scope.currentKlineTime)
	availablePositionQty := 0.0
	if position != nil {
		availablePositionQty = math.Floor(absFloat(firstPositiveFloat(absFloat(position.AvailableQuantity), absFloat(position.Quantity))))
	}

	intent := normalizeOrderIntent(statement.Intent)
	action := statement.Action
	if intent == strategyir.OrderIntentFlatten {
		if position == nil || availablePositionQty <= 0 {
			r.internalLog("无持仓可平，跳过全部平仓")
			return nil
		}
		switch position.Direction {
		case "LONG":
			action = strategyir.OrderActionSell
		case "SHORT":
			action = strategyir.OrderActionCover
		default:
			r.internalLog("无持仓可平，跳过全部平仓")
			return nil
		}
	}

	entryPolicy := normalizeEntryPolicy(statement.EntryPolicy)
	sameDirectionEntryCount := 0
	switch action {
	case strategyir.OrderActionBuy:
		if intent == strategyir.OrderIntentEntry {
			sameDirectionEntryCount = r.sameDirectionEntryCount("LONG", position, availablePositionQty)
		}
		if intent == strategyir.OrderIntentEntry && shouldSkipLongEntry(position, availablePositionQty, entryPolicy, r.maxPyramiding, sameDirectionEntryCount) {
			r.internalLog("已有多头持仓，按策略跳过开多")
			return nil
		}
	case strategyir.OrderActionSell:
		if intent != strategyir.OrderIntentNet && (position == nil || position.Direction != "LONG" || availablePositionQty <= 0) {
			r.internalLog("无多头持仓可平，跳过卖出")
			return nil
		}
	case strategyir.OrderActionShort:
		if intent == strategyir.OrderIntentEntry {
			sameDirectionEntryCount = r.sameDirectionEntryCount("SHORT", position, availablePositionQty)
		}
		if intent == strategyir.OrderIntentEntry && shouldSkipShortEntry(position, availablePositionQty, entryPolicy, r.maxPyramiding, sameDirectionEntryCount) {
			r.internalLog("已有空头持仓，按策略跳过开空")
			return nil
		}
	case strategyir.OrderActionCover:
		if intent != strategyir.OrderIntentNet && (position == nil || position.Direction != "SHORT" || availablePositionQty <= 0) {
			r.internalLog("无空头持仓可平，跳过买入平空")
			return nil
		}
	default:
		return fmt.Errorf("pine line %d: unsupported order action %q", statement.Range.StartLine, action)
	}

	orderPrice, limitPrice, err := r.resolveOrderPrice(statement, scope)
	if err != nil {
		return fmt.Errorf("pine line %d: %w", statement.Range.StartLine, err)
	}
	if orderPrice <= 0 || math.IsNaN(orderPrice) || math.IsInf(orderPrice, 0) {
		return fmt.Errorf("pine line %d: order price must be positive", statement.Range.StartLine)
	}

	quantityMode, ok := indicatorbinding.ParseQuantityMode(statement.QuantityMode)
	if !ok {
		return fmt.Errorf("pine line %d: unsupported order quantity mode %q", statement.Range.StartLine, statement.QuantityMode)
	}
	quantity, err := r.resolveOrderQuantity(statement, scope, position, availablePositionQty, orderPrice, quantityMode)
	if err != nil {
		return fmt.Errorf("pine line %d: %w", statement.Range.StartLine, err)
	}
	if quantity <= 0 {
		return nil
	}
	if r.isPlaceBlockedDuringWarmup(scope.currentKlineTime) {
		r.internalLog("place order suppressed during warmup")
		return nil
	}

	if r.shouldStorePendingOrder(statement, intent) {
		r.storePendingOrder(statement, action, intent, quantity, limitPrice, orderPrice)
		return nil
	}

	orderSide, err := exchangeSideForAction(action)
	if err != nil {
		return fmt.Errorf("pine line %d: %w", statement.Range.StartLine, err)
	}
	if err := r.submitOrder(orderSide, normalizeOrderType(statement.OrderType), quantity, limitPrice); err != nil {
		return fmt.Errorf("pine line %d: %w", statement.Range.StartLine, err)
	}
	r.emitOrderMetadata(statement.Comment, statement.AlertMessage, statement.DisableAlert)
	switch intent {
	case strategyir.OrderIntentNet:
		r.resetEntrySubmitCount("LONG")
		r.resetEntrySubmitCount("SHORT")
	default:
		r.recordSubmittedOrderAction(action, quantity, availablePositionQty, sameDirectionEntryCount)
	}
	return nil
}

func (r *strategyRuntime) shouldStorePendingOrder(statement *strategyir.OrderStmt, intent strategyir.OrderIntent) bool {
	if statement == nil {
		return false
	}
	if strings.TrimSpace(statement.StopExpression) != "" {
		return true
	}
	if strings.TrimSpace(statement.LimitExpression) == "" {
		return false
	}
	switch intent {
	case strategyir.OrderIntentEntry, strategyir.OrderIntentNet:
		return true
	default:
		return false
	}
}

func (r *strategyRuntime) storePendingOrder(statement *strategyir.OrderStmt, action strategyir.OrderAction, intent strategyir.OrderIntent, quantity float64, limitPrice float64, stopPrice float64) {
	if r == nil || statement == nil || quantity <= 0 {
		return
	}
	id := strings.TrimSpace(statement.ID)
	if id == "" {
		id = fmt.Sprintf("line:%d", statement.Range.StartLine)
	}
	r.pendingSequence++
	orderType := normalizeOrderType(statement.OrderType)
	pending := pendingOrder{
		id:         id,
		sequence:   r.pendingSequence,
		action:     action,
		intent:     intent,
		orderType:  orderType,
		quantity:   quantity,
		limitPrice: limitPrice,
		stopPrice:  stopPrice,
		hasLimit:   strings.TrimSpace(statement.LimitExpression) != "",
		hasStop:    strings.TrimSpace(statement.StopExpression) != "",
		comment:    statement.Comment,
		alert:      statement.AlertMessage,
		disable:    statement.DisableAlert,
	}
	if !pending.hasLimit {
		pending.limitPrice = 0
	}
	if !pending.hasStop {
		pending.stopPrice = 0
	}
	r.pendingOrders[id] = pending
	r.internalLog("registered pending order " + id)
}

func (r *strategyRuntime) triggerPendingOrders(kline *types.KLine) error {
	if r == nil || kline == nil || len(r.pendingOrders) == 0 {
		return nil
	}
	orders := make([]pendingOrder, 0, len(r.pendingOrders))
	for _, order := range r.pendingOrders {
		orders = append(orders, order)
	}
	sort.Slice(orders, func(left, right int) bool {
		return orders[left].sequence < orders[right].sequence
	})
	high := kline.High.Float64()
	low := kline.Low.Float64()
	for _, order := range orders {
		if !pendingOrderTriggered(order, high, low) {
			continue
		}
		delete(r.pendingOrders, order.id)
		side, err := exchangeSideForAction(order.action)
		if err != nil {
			return err
		}
		orderType := types.OrderTypeMarket
		limitPrice := 0.0
		if order.hasLimit && !order.hasStop {
			orderType = types.OrderTypeLimit
			limitPrice = order.limitPrice
		}
		if err := r.submitOrder(side, orderType, order.quantity, limitPrice); err != nil {
			return err
		}
		r.emitOrderMetadata(order.comment, order.alert, order.disable)
		switch order.intent {
		case strategyir.OrderIntentNet:
			r.resetEntrySubmitCount("LONG")
			r.resetEntrySubmitCount("SHORT")
		default:
			r.recordSubmittedOrderAction(order.action, order.quantity, 0, 0)
		}
	}
	return nil
}

func pendingOrderTriggered(order pendingOrder, high float64, low float64) bool {
	switch {
	case order.hasStop:
		switch order.action {
		case strategyir.OrderActionBuy, strategyir.OrderActionCover:
			return high >= order.stopPrice
		case strategyir.OrderActionSell, strategyir.OrderActionShort:
			return low <= order.stopPrice
		default:
			return false
		}
	case order.hasLimit:
		switch order.action {
		case strategyir.OrderActionBuy, strategyir.OrderActionCover:
			return low <= order.limitPrice
		case strategyir.OrderActionSell, strategyir.OrderActionShort:
			return high >= order.limitPrice
		default:
			return false
		}
	default:
		return false
	}
}

func (r *strategyRuntime) executeProtectStatement(statement *strategyir.ProtectStmt, scope *evaluationScope) (bool, error) {
	requirement, err := r.resolveProtectRequirement(statement)
	if err != nil {
		return false, err
	}
	rawSnapshot, ok := scope.indicators[requirement.key]
	if !ok || rawSnapshot == nil {
		r.internalLog("waiting for indicator " + requirement.key)
		return false, nil
	}
	position := r.getPosition(r.symbol, scope.currentKlineTime)
	if position == nil {
		return false, nil
	}
	shouldExitLong := requirement.allowLongExit && position.Direction == "LONG" && readBool(rawSnapshot, "longTriggered")
	shouldExitShort := requirement.allowShortExit && position.Direction == "SHORT" && readBool(rawSnapshot, "shortTriggered")
	availableQuantity := math.Floor(absFloat(firstPositiveFloat(absFloat(position.AvailableQuantity), absFloat(position.Quantity))))
	quantity := 0.0
	if shouldExitLong || shouldExitShort {
		quantityMode := strings.TrimSpace(statement.QuantityMode)
		if quantityMode == "" {
			quantityMode = "symbol_position_percent"
		}
		quantityExpr := strings.TrimSpace(statement.QuantityExpression)
		if quantityExpr == "" {
			quantityExpr = "100"
		}
		mode, ok := indicatorbinding.ParseQuantityMode(quantityMode)
		if !ok {
			return false, fmt.Errorf("pine line %d: unsupported exit quantity mode %q", statement.Range.StartLine, statement.QuantityMode)
		}
		closePrice := 0.0
		if scope.currentKline != nil {
			closePrice = scope.currentKline.Close.Float64()
		}
		quantity, err = r.resolveOrderQuantity(&strategyir.OrderStmt{
			Range:              statement.Range,
			Action:             strategyir.OrderActionSell,
			Intent:             strategyir.OrderIntentClose,
			QuantityMode:       mode,
			QuantityExpression: quantityExpr,
		}, scope, position, availableQuantity, closePrice, mode)
		if err != nil {
			return false, fmt.Errorf("pine line %d: %w", statement.Range.StartLine, err)
		}
		if quantity <= 0 {
			return false, nil
		}
	}
	if shouldExitLong {
		if r.isPlaceBlockedDuringWarmup(scope.currentKlineTime) {
			r.internalLog("protect exit suppressed during warmup")
			return true, nil
		}
		if err := r.submitOrder(types.SideTypeSell, types.OrderTypeMarket, quantity, 0); err != nil {
			return false, fmt.Errorf("pine line %d: %w", statement.Range.StartLine, err)
		}
		r.emitOrderMetadata(statement.Comment, statement.AlertMessage, statement.DisableAlert)
		r.resetEntrySubmitCount("LONG")
		return true, nil
	}
	if shouldExitShort {
		if r.isPlaceBlockedDuringWarmup(scope.currentKlineTime) {
			r.internalLog("protect exit suppressed during warmup")
			return true, nil
		}
		if err := r.submitOrder(types.SideTypeBuy, types.OrderTypeMarket, quantity, 0); err != nil {
			return false, fmt.Errorf("pine line %d: %w", statement.Range.StartLine, err)
		}
		r.emitOrderMetadata(statement.Comment, statement.AlertMessage, statement.DisableAlert)
		r.resetEntrySubmitCount("SHORT")
		return true, nil
	}
	return false, nil
}

func (r *strategyRuntime) executeExitStatement(statement *strategyir.ExitStmt, scope *evaluationScope) (bool, error) {
	position := r.getPosition(r.symbol, scope.currentKlineTime)
	if position == nil {
		return false, nil
	}
	availableQuantity := math.Floor(absFloat(firstPositiveFloat(absFloat(position.AvailableQuantity), absFloat(position.Quantity))))
	if availableQuantity <= 0 {
		return false, nil
	}
	direction := normalizeProtectDirection(statement.Direction)
	if direction == "" || direction == "both" {
		if position.Direction == "SHORT" {
			direction = "short"
		} else {
			direction = "long"
		}
	}
	allowLongExit := direction != "short"
	allowShortExit := direction != "long"
	stopPrice, hasStop, err := evaluateOptionalFloatExpression(statement.StopExpression, scope)
	if err != nil {
		return false, fmt.Errorf("pine line %d: %w", statement.Range.StartLine, err)
	}
	limitPrice, hasLimit, err := evaluateOptionalFloatExpression(statement.LimitExpression, scope)
	if err != nil {
		return false, fmt.Errorf("pine line %d: %w", statement.Range.StartLine, err)
	}
	if !hasStop && !hasLimit {
		return false, nil
	}
	high, low, closePrice := currentBarPrices(scope)
	triggered := false
	triggerPrice := closePrice
	if allowLongExit && position.Direction == "LONG" {
		stopTriggered := hasStop && low <= stopPrice
		limitTriggered := hasLimit && high >= limitPrice
		if stopTriggered && limitTriggered {
			r.internalLog("strategy.exit bracket hit stop and limit in same bar; using stop-first")
		}
		switch {
		case stopTriggered:
			triggered = true
			triggerPrice = stopPrice
		case limitTriggered:
			triggered = true
			triggerPrice = limitPrice
		}
	}
	if allowShortExit && position.Direction == "SHORT" {
		stopTriggered := hasStop && high >= stopPrice
		limitTriggered := hasLimit && low <= limitPrice
		if stopTriggered && limitTriggered {
			r.internalLog("strategy.exit bracket hit stop and limit in same bar; using stop-first")
		}
		switch {
		case stopTriggered:
			triggered = true
			triggerPrice = stopPrice
		case limitTriggered:
			triggered = true
			triggerPrice = limitPrice
		}
	}
	if !triggered {
		return false, nil
	}
	quantityMode := strings.TrimSpace(statement.QuantityMode)
	if quantityMode == "" {
		quantityMode = "symbol_position_percent"
	}
	mode, ok := indicatorbinding.ParseQuantityMode(quantityMode)
	if !ok {
		return false, fmt.Errorf("pine line %d: unsupported exit quantity mode %q", statement.Range.StartLine, statement.QuantityMode)
	}
	quantityExpr := strings.TrimSpace(statement.QuantityExpression)
	if quantityExpr == "" {
		quantityExpr = "100"
	}
	quantity, err := r.resolveOrderQuantity(&strategyir.OrderStmt{
		Range:              statement.Range,
		Intent:             strategyir.OrderIntentClose,
		QuantityMode:       mode,
		QuantityExpression: quantityExpr,
	}, scope, position, availableQuantity, triggerPrice, mode)
	if err != nil {
		return false, fmt.Errorf("pine line %d: %w", statement.Range.StartLine, err)
	}
	if quantity <= 0 {
		return false, nil
	}
	if r.isPlaceBlockedDuringWarmup(scope.currentKlineTime) {
		r.internalLog("exit suppressed during warmup")
		return true, nil
	}
	if position.Direction == "LONG" {
		if err := r.submitOrder(types.SideTypeSell, types.OrderTypeMarket, quantity, 0); err != nil {
			return false, fmt.Errorf("pine line %d: %w", statement.Range.StartLine, err)
		}
		r.emitOrderMetadata(statement.Comment, statement.AlertMessage, statement.DisableAlert)
		r.resetEntrySubmitCount("LONG")
		return true, nil
	}
	if position.Direction == "SHORT" {
		if err := r.submitOrder(types.SideTypeBuy, types.OrderTypeMarket, quantity, 0); err != nil {
			return false, fmt.Errorf("pine line %d: %w", statement.Range.StartLine, err)
		}
		r.emitOrderMetadata(statement.Comment, statement.AlertMessage, statement.DisableAlert)
		r.resetEntrySubmitCount("SHORT")
		return true, nil
	}
	return false, nil
}

func (r *strategyRuntime) executeCancelStatement(statement *strategyir.CancelStmt) {
	if r == nil || statement == nil || len(r.pendingOrders) == 0 {
		return
	}
	if statement.All {
		clear(r.pendingOrders)
		r.internalLog("cancelled all pending orders")
		return
	}
	id := strings.TrimSpace(statement.ID)
	if id == "" {
		return
	}
	if _, ok := r.pendingOrders[id]; ok {
		delete(r.pendingOrders, id)
		r.internalLog("cancelled pending order " + id)
		return
	}
	r.internalLog("pending order " + id + " not found for cancel")
}

func evaluateOptionalFloatExpression(expression string, scope *evaluationScope) (float64, bool, error) {
	trimmed := strings.TrimSpace(expression)
	if trimmed == "" {
		return 0, false, nil
	}
	value, err := evaluateFloatExpression(trimmed, scope)
	if err != nil {
		return 0, false, err
	}
	return value, true, nil
}

func currentBarPrices(scope *evaluationScope) (float64, float64, float64) {
	if scope == nil || scope.currentKline == nil {
		return 0, 0, 0
	}
	return scope.currentKline.High.Float64(), scope.currentKline.Low.Float64(), scope.currentKline.Close.Float64()
}

func (r *strategyRuntime) resolveProtectRequirement(statement *strategyir.ProtectStmt) (cachedProtectRequirement, error) {
	if cached, ok := r.protectCache[statement]; ok {
		return cached, cached.err
	}
	key, err := buildProtectRequirementKey(statement)
	cached := cachedProtectRequirement{key: key, err: err}
	if err == nil {
		direction := normalizeProtectDirection(statement.Direction)
		cached.allowLongExit = direction != "short"
		cached.allowShortExit = direction != "long"
	}
	r.protectCache[statement] = cached
	return cached, err
}

func (r *strategyRuntime) resolveOrderPrice(statement *strategyir.OrderStmt, scope *evaluationScope) (float64, float64, error) {
	closePrice := 0.0
	if scope.currentKline != nil {
		closePrice = scope.currentKline.Close.Float64()
	}
	if strings.TrimSpace(statement.StopExpression) != "" {
		value, err := evaluateFloatExpression(statement.StopExpression, scope)
		if err != nil {
			return 0, 0, err
		}
		return value, 0, nil
	}
	orderType := normalizeOrderType(statement.OrderType)
	if orderType == types.OrderTypeMarket {
		return closePrice, 0, nil
	}
	limitPrice := closePrice
	if strings.TrimSpace(statement.LimitExpression) != "" {
		value, err := evaluateFloatExpression(statement.LimitExpression, scope)
		if err != nil {
			return 0, 0, err
		}
		limitPrice = value
	}
	return limitPrice, limitPrice, nil
}

func (r *strategyRuntime) resolveOrderQuantity(
	statement *strategyir.OrderStmt,
	scope *evaluationScope,
	position *positionSnapshot,
	availablePositionQty float64,
	orderPrice float64,
	quantityMode string,
) (float64, error) {
	value, err := evaluateFloatExpression(statement.QuantityExpression, scope)
	if err != nil {
		return 0, err
	}
	closingAction := isClosingOrderQuantity(statement)
	switch quantityMode {
	case "shares":
		quantity := math.Floor(value)
		if quantity <= 0 {
			r.internalLog("shares quantity computed as 0, skipping order")
			return 0, nil
		}
		return quantity, nil
	case "amount":
		quantity := math.Floor(value / orderPrice)
		if quantity <= 0 {
			r.internalLog("amount quantity computed as 0, skipping order")
			return 0, nil
		}
		return quantity, nil
	case "account_position_percent":
		accountTotalValue := r.getTotalAccountValue()
		targetAmount := accountTotalValue * value / 100
		rawQuantity := 0.0
		if targetAmount > 0 {
			rawQuantity = math.Floor(targetAmount / orderPrice)
		}
		return clampPercentBasedQuantity(rawQuantity, availablePositionQty, closingAction), nil
	case "symbol_position_percent":
		if closingAction {
			rawQuantity := 0.0
			if availablePositionQty > 0 {
				rawQuantity = math.Floor(availablePositionQty * value / 100)
			}
			return clampPercentBasedQuantity(rawQuantity, availablePositionQty, true), nil
		}
		currentPositionValue := 0.0
		if position != nil {
			currentPositionValue = absFloat(position.MarketValue)
		}
		targetValue := currentPositionValue * value / 100
		rawQuantity := 0.0
		if targetValue > 0 {
			rawQuantity = math.Floor(targetValue / orderPrice)
		}
		return clampPercentBasedQuantity(rawQuantity, availablePositionQty, closingAction), nil
	default:
		quantity := math.Floor(value)
		if quantity <= 0 {
			return 0, nil
		}
		return quantity, nil
	}
}

func isClosingOrderQuantity(statement *strategyir.OrderStmt) bool {
	if statement == nil {
		return false
	}
	switch statement.Intent {
	case strategyir.OrderIntentClose, strategyir.OrderIntentFlatten:
		return true
	case strategyir.OrderIntentNet:
		return false
	case "":
		return statement.Action == strategyir.OrderActionSell || statement.Action == strategyir.OrderActionCover
	default:
		return false
	}
}

func clampPercentBasedQuantity(rawQuantity float64, availablePositionQty float64, closingAction bool) float64 {
	if rawQuantity > 0 {
		if availablePositionQty > 0 {
			return math.Min(rawQuantity, availablePositionQty)
		}
		return rawQuantity
	}
	if closingAction && availablePositionQty > 0 {
		return 1
	}
	return 0
}

func (r *strategyRuntime) submitOrder(side types.SideType, orderType types.OrderType, quantity float64, limitPrice float64) error {
	if r.executor == nil {
		return fmt.Errorf("order executor is not available")
	}
	if r.session == nil {
		return fmt.Errorf("exchange session is not available")
	}
	symbol := r.symbol
	market, ok := r.session.Market(symbol)
	if !ok {
		return fmt.Errorf("market %s is not loaded in this session", symbol)
	}
	order := types.SubmitOrder{
		ClientOrderID: fmt.Sprintf("pine-go-%d", time.Now().UnixNano()),
		Symbol:        symbol,
		Side:          side,
		Type:          orderType,
		Quantity:      fixedpoint.NewFromFloat(quantity),
		Market:        market,
	}
	if orderType == types.OrderTypeLimit {
		if limitPrice <= 0 {
			return fmt.Errorf("limit price must be positive")
		}
		order.Price = fixedpoint.NewFromFloat(limitPrice)
		order.TimeInForce = types.TimeInForceGTC
	}
	if _, err := r.executor.SubmitOrders(r.ctx, order); err != nil {
		return fmt.Errorf("submit order: %w", err)
	}
	r.clearPositionCache()
	return nil
}

func (r *strategyRuntime) emitOrderMetadata(comment string, alertMessage string, disableAlert bool) {
	if trimmed := strings.TrimSpace(comment); trimmed != "" {
		r.internalLog("order comment: " + trimmed)
	}
	if disableAlert {
		return
	}
	if trimmed := strings.TrimSpace(alertMessage); trimmed != "" {
		r.notify(trimmed)
	}
}

func (r *strategyRuntime) getPosition(symbol string, barTime time.Time) *positionSnapshot {
	if cached, ok := r.cachedPosition(symbol, barTime); ok {
		return cached
	}
	if r.session == nil {
		return nil
	}
	market, ok := r.session.Market(symbol)
	if !ok {
		return nil
	}
	var baseQuantity fixedpoint.Value
	position := runtimePositionForSymbol(r.session, symbol)
	if position != nil {
		baseQuantity = position.Base
	}
	availableQuantity := fixedpoint.Zero
	if account := runtimeAccount(r.session); account != nil && market.BaseCurrency != "" {
		if balance, ok := account.Balance(market.BaseCurrency); ok {
			availableQuantity = balance.Available
			if baseQuantity.IsZero() {
				baseQuantity = balance.Total()
			}
		}
	}
	lastPrice, _ := r.session.LastPrice(symbol)
	marketPrice := lastPrice
	if marketPrice.IsZero() && position != nil {
		marketPrice = position.AverageCost
	}
	averagePrice := 0.0
	if position != nil {
		averagePrice = position.AverageCost.Float64()
	}
	direction := "FLAT"
	if baseQuantity.Sign() > 0 {
		direction = "LONG"
	} else if baseQuantity.Sign() < 0 {
		direction = "SHORT"
	}
	snapshot := &positionSnapshot{
		Symbol:            symbol,
		Quantity:          baseQuantity.Float64(),
		AvailableQuantity: availableQuantity.Float64(),
		MarketValue:       marketPrice.Mul(baseQuantity).Float64(),
		AveragePrice:      averagePrice,
		Direction:         direction,
	}
	r.storeCachedPosition(symbol, barTime, snapshot)
	return snapshot
}

func (r *strategyRuntime) getAvailableCash() float64 {
	account := runtimeAccount(r.session)
	if account == nil {
		return 0
	}
	if quoteCurrency := r.strategyQuoteCurrency(); quoteCurrency != "" {
		if balance, ok := account.Balance(quoteCurrency); ok {
			if !balance.Available.IsZero() {
				return balance.Available.Float64()
			}
			if !balance.NetAsset.IsZero() {
				return balance.NetAsset.Float64()
			}
		}
	}
	if !account.TotalAccountValue.IsZero() {
		return account.TotalAccountValue.Float64()
	}
	total := fixedpoint.Zero
	for _, balance := range account.Balances() {
		total = total.Add(balance.Available)
	}
	if !total.IsZero() {
		return total.Float64()
	}
	for _, balance := range account.Balances() {
		total = total.Add(balance.NetAsset)
	}
	return total.Float64()
}

func (r *strategyRuntime) getTotalAccountValue() float64 {
	account := runtimeAccount(r.session)
	if account == nil {
		return 0
	}
	if !account.TotalAccountValue.IsZero() {
		return account.TotalAccountValue.Float64()
	}
	total := fixedpoint.Zero
	for _, balance := range account.Balances() {
		total = total.Add(balance.NetAsset)
	}
	if !total.IsZero() {
		return total.Float64()
	}
	for _, balance := range account.Balances() {
		total = total.Add(balance.Available)
	}
	return total.Float64()
}

func (r *strategyRuntime) getMarginBuyingPower() float64 {
	funds := r.brokerFunds()
	if funds == nil {
		return 0
	}
	return funds.GetPower()
}

func (r *strategyRuntime) getShortSellingPower() float64 {
	funds := r.brokerFunds()
	if funds == nil {
		return 0
	}
	return funds.GetMaxPowerShort()
}

func (r *strategyRuntime) brokerFunds() *trdcommonpb.Funds {
	account := runtimeAccount(r.session)
	if account == nil || account.RawAccount == nil {
		return nil
	}
	funds, _ := account.RawAccount.(*trdcommonpb.Funds)
	return funds
}

func (r *strategyRuntime) strategyQuoteCurrency() string {
	if r.session == nil || r.strategy == nil {
		return ""
	}
	market, ok := r.session.Market(r.symbol)
	if !ok {
		return ""
	}
	return strings.ToUpper(strings.TrimSpace(market.QuoteCurrency))
}

func (r *strategyRuntime) resolveKLineSession(kline types.KLine) market.Session {
	if r.session != nil && r.session.Exchange != nil {
		if resolver, ok := r.session.Exchange.(klineSessionResolver); ok {
			if session, ok := resolver.ResolveKLineSession(kline); ok {
				return session
			}
		}
	}
	strategySymbol := ""
	if r.strategy != nil {
		strategySymbol = r.strategy.Symbol
	}
	resolvedSymbol := strings.ToUpper(strings.TrimSpace(strategySymbol))
	if resolvedSymbol == "" {
		resolvedSymbol = strings.ToUpper(strings.TrimSpace(kline.Symbol))
	}
	observedAt := kline.StartTime.Time().UTC()
	if observedAt.IsZero() {
		observedAt = kline.EndTime.Time().UTC()
	}
	if resolvedSymbol == "" || observedAt.IsZero() {
		return market.SessionUnknown
	}
	return market.ClassifySession(resolvedSymbol, observedAt)
}

func (r *strategyRuntime) isPlaceBlockedDuringWarmup(currentKlineTime time.Time) bool {
	return !r.strategy.WarmupUntil.IsZero() && !currentKlineTime.IsZero() && currentKlineTime.Before(r.strategy.WarmupUntil)
}

func (r *strategyRuntime) log(message string) {
	bbgo2.Notify("pine strategy %s: %s", r.displayName, strings.TrimSpace(message))
}

func (r *strategyRuntime) internalLog(message string) {
	if bbgo2.IsBackTesting {
		return
	}
	r.log(message)
}

func (r *strategyRuntime) notify(message string) {
	bbgo2.Notify("pine strategy %s: %s", r.displayName, strings.TrimSpace(message))
}

func (r *strategyRuntime) cachedPosition(symbol string, barTime time.Time) (*positionSnapshot, bool) {
	if r == nil || !r.positionCache.valid {
		return nil, false
	}
	if r.positionCache.symbol != symbol || !r.positionCache.barTime.Equal(barTime) {
		return nil, false
	}
	return r.positionCache.value, true
}

func (r *strategyRuntime) storeCachedPosition(symbol string, barTime time.Time, snapshot *positionSnapshot) {
	if r == nil {
		return
	}
	r.positionCache = cachedPositionSnapshot{
		barTime: barTime,
		symbol:  symbol,
		value:   snapshot,
		valid:   true,
	}
}

func (r *strategyRuntime) clearPositionCache() {
	if r == nil {
		return
	}
	r.positionCache = cachedPositionSnapshot{}
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

func normalizeRuntimePyramiding(program *strategyir.Program) int {
	if program == nil || program.Metadata.Pyramiding <= 0 {
		return 1
	}
	return program.Metadata.Pyramiding
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
	case "highest", "lowest", "highestbars", "lowestbars", "change", "mom", "roc", "rising", "falling", "sum":
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
