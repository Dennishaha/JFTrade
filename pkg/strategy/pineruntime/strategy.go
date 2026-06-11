package pineruntime

import (
	"context"
	"fmt"
	"math"
	"strconv"
	"strings"
	"sync"
	"time"

	bbgo2 "github.com/c9s/bbgo/pkg/bbgo"
	"github.com/c9s/bbgo/pkg/fixedpoint"
	"github.com/c9s/bbgo/pkg/types"
	exprast "github.com/expr-lang/expr/ast"
	"github.com/jftrade/jftrade-main/pkg/futu"
	trdcommonpb "github.com/jftrade/jftrade-main/pkg/futu/pb/trdcommon"
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
	StrategyID       string         `json:"strategyId"`
	Name             string         `json:"name"`
	Symbol           string         `json:"symbol"`
	Interval         types.Interval `json:"interval"`
	Script           string         `json:"script"`
	DefinitionID     string         `json:"definitionId"`
	UseExtendedHours bool           `json:"-"`
	WarmupUntil      time.Time      `json:"-"`
	OnError          func(string)   `json:"-"`
}

type strategyRuntime struct {
	mu               sync.Mutex
	ctx              context.Context
	strategy         *Strategy
	program          *strategyir.Program
	plan             strategyir.Requirements
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
	positionCache    cachedPositionSnapshot
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

type ifScopePlan struct {
	thenNeedsClone bool
	elseNeedsClone bool
}

type positionSnapshot struct {
	Symbol            string
	Quantity          float64
	AvailableQuantity float64
	MarketValue       float64
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
	currentSession     futu.MarketSession
	klinePayload       klinePayloadView
	closeSeries        seriesNumber
	openSeries         seriesNumber
	highSeries         seriesNumber
	lowSeries          seriesNumber
	volumeSeries       seriesNumber
	hasBarData         bool
}

type klinePayloadView struct {
	kline         *types.KLine
	session       futu.MarketSession
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
	default:
		return nil, false
	}
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
	ResolveKLineSession(kline types.KLine) (futu.MarketSession, bool)
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
	program, err := strategypine.ParseScript(s.Script)
	if err != nil {
		return fmt.Errorf("parse pine strategy: %w", err)
	}
	plan, err := strategyir.PlanRequirements(program)
	if err != nil {
		return fmt.Errorf("plan pine strategy: %w", err)
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
	runtime.baseScope.runtime = runtime
	runtime.reusableScope = &evaluationScope{
		runtime:   runtime,
		parent:    runtime.baseScope,
		variables: make(map[string]any, runtime.variableCapacity),
	}
	if runtime.bindingCapacity > 0 {
		runtime.reusableScope.bindings = make(map[string]indicatorBinding, runtime.bindingCapacity)
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
	return r.runHookLocked(strategyir.HookInit, nil, futu.MarketSessionUnknown)
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

	resolvedSession := r.resolveKLineSession(kline)
	if r.engine != nil {
		r.engine.Push(kline, resolvedSession)
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

func (r *strategyRuntime) runHookLocked(kind strategyir.HookKind, kline *types.KLine, session futu.MarketSession) error {
	hook, ok := findHook(r.program, kind)
	if !ok {
		return nil
	}
	scope := r.newScope(kline, session)
	_, err := r.executeStatements(hook.Statements, scope)
	return err
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

func (r *strategyRuntime) newScope(kline *types.KLine, session futu.MarketSession) *evaluationScope {
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
	scope.hasBarData = false
	if kline != nil {
		scope.currentKlineTime = kline.EndTime.Time()
		scope.currentKlineSymbol = kline.Symbol
		scope.klinePayload = klinePayloadView{kline: kline, session: session}
		scope.closeSeries = seriesNumber{Current: kline.Close.Float64(), Previous: r.previousClose, HasCurrent: true, HasPrevious: r.hasPreviousClose}
		scope.openSeries = seriesNumber{Current: kline.Open.Float64(), Previous: r.previousOpen, HasCurrent: true, HasPrevious: r.hasPreviousClose}
		scope.highSeries = seriesNumber{Current: kline.High.Float64(), Previous: r.previousHigh, HasCurrent: true, HasPrevious: r.hasPreviousClose}
		scope.lowSeries = seriesNumber{Current: kline.Low.Float64(), Previous: r.previousLow, HasCurrent: true, HasPrevious: r.hasPreviousClose}
		scope.volumeSeries = seriesNumber{Current: kline.Volume.Float64(), Previous: r.previousVolume, HasCurrent: true, HasPrevious: r.hasPreviousClose}
		scope.hasBarData = true
	}
	return scope
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

	entryPolicy := normalizeEntryPolicy(statement.EntryPolicy)
	switch statement.Action {
	case strategyir.OrderActionBuy:
		if shouldSkipLongEntry(position, availablePositionQty, entryPolicy) {
			r.internalLog("已有多头持仓，按策略跳过开多")
			return nil
		}
	case strategyir.OrderActionSell:
		if position == nil || position.Direction != "LONG" || availablePositionQty <= 0 {
			r.internalLog("无多头持仓可平，跳过卖出")
			return nil
		}
	case strategyir.OrderActionShort:
		if shouldSkipShortEntry(position, availablePositionQty, entryPolicy) {
			r.internalLog("已有空头持仓，按策略跳过开空")
			return nil
		}
	case strategyir.OrderActionCover:
		if position == nil || position.Direction != "SHORT" || availablePositionQty <= 0 {
			r.internalLog("无空头持仓可平，跳过买入平空")
			return nil
		}
	default:
		return fmt.Errorf("pine line %d: unsupported order action %q", statement.Range.StartLine, statement.Action)
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

	orderSide, err := exchangeSideForAction(statement.Action)
	if err != nil {
		return fmt.Errorf("pine line %d: %w", statement.Range.StartLine, err)
	}
	if err := r.submitOrder(orderSide, normalizeOrderType(statement.OrderType), quantity, limitPrice); err != nil {
		return fmt.Errorf("pine line %d: %w", statement.Range.StartLine, err)
	}
	return nil
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
	quantity := math.Floor(absFloat(firstPositiveFloat(absFloat(position.AvailableQuantity), absFloat(position.Quantity))))
	if quantity <= 0 {
		return false, nil
	}
	shouldExitLong := requirement.allowLongExit && position.Direction == "LONG" && readBool(rawSnapshot, "longTriggered")
	shouldExitShort := requirement.allowShortExit && position.Direction == "SHORT" && readBool(rawSnapshot, "shortTriggered")
	if shouldExitLong {
		if r.isPlaceBlockedDuringWarmup(scope.currentKlineTime) {
			r.internalLog("protect exit suppressed during warmup")
			return true, nil
		}
		if err := r.submitOrder(types.SideTypeSell, types.OrderTypeMarket, quantity, 0); err != nil {
			return false, fmt.Errorf("pine line %d: %w", statement.Range.StartLine, err)
		}
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
		return true, nil
	}
	return false, nil
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
	closingAction := statement.Action == strategyir.OrderActionSell || statement.Action == strategyir.OrderActionCover
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

func (r *strategyRuntime) resolveKLineSession(kline types.KLine) futu.MarketSession {
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
		return futu.MarketSessionUnknown
	}
	return futu.ClassifyMarketSession(resolvedSymbol, observedAt)
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

func klinePayload(kline types.KLine, session futu.MarketSession) *klinePayloadView {
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
		if p.session == futu.MarketSessionUnknown {
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

func shouldSkipLongEntry(position *positionSnapshot, availablePositionQty float64, entryPolicy string) bool {
	if entryPolicy == "allow" {
		return false
	}
	if entryPolicy == "flat_only" {
		return position != nil && position.Quantity != 0
	}
	return position != nil && position.Direction == "LONG" && availablePositionQty > 0
}

func shouldSkipShortEntry(position *positionSnapshot, availablePositionQty float64, entryPolicy string) bool {
	if entryPolicy == "allow" {
		return false
	}
	if entryPolicy == "flat_only" {
		return position != nil && position.Quantity != 0
	}
	return position != nil && position.Direction == "SHORT" && availablePositionQty > 0
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
		if len(args) < 2 || len(args) > 3 {
			return indicatorBinding{}, false, fmt.Errorf("pine line %d: ma() requires type, period, and optional time unit", statement.Range.StartLine)
		}
		averageType, ok := indicatorbinding.ParseMovingAverageType(args[0])
		if !ok {
			return indicatorBinding{}, false, fmt.Errorf("pine line %d: ma() type %q is not supported", statement.Range.StartLine, strings.TrimSpace(args[0]))
		}
		period, err := indicatorbinding.ParsePositiveInt(args[1])
		if err != nil {
			return indicatorBinding{}, false, fmt.Errorf("pine line %d: ma() period must be a positive integer", statement.Range.StartLine)
		}
		timeUnit := ""
		if len(args) == 3 {
			parsedTimeUnit, ok := indicatorbinding.ParseIndicatorTimeUnitValue(args[2])
			if !ok {
				return indicatorBinding{}, false, fmt.Errorf("pine line %d: ma() time unit %q is not supported", statement.Range.StartLine, strings.TrimSpace(args[2]))
			}
			timeUnit = parsedTimeUnit
		}
		return indicatorBinding{Alias: statement.Name, Kind: "ma", Key: indicatorbinding.BuildMovingAverageKey(averageType, period, timeUnit), Args: []string{averageType, strconv.Itoa(period), timeUnit}}, true, nil
	case "rsi":
		period, err := indicatorbinding.ExpectOnePositiveIntArg(statement.Range.StartLine, name, args)
		if err != nil {
			return indicatorBinding{}, false, err
		}
		return indicatorBinding{Alias: statement.Name, Kind: "rsi", Key: "rsi:" + strconv.Itoa(period), Args: []string{strconv.Itoa(period)}}, true, nil
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
	case "cci":
		period, err := indicatorbinding.ExpectOnePositiveIntArg(statement.Range.StartLine, name, args)
		if err != nil {
			return indicatorBinding{}, false, err
		}
		return indicatorBinding{Alias: statement.Name, Kind: "cci", Key: "cci:" + strconv.Itoa(period), Args: []string{strconv.Itoa(period)}}, true, nil
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
