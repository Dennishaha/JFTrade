package pineruntime

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	bbgo2 "github.com/c9s/bbgo/pkg/bbgo"
	"github.com/c9s/bbgo/pkg/types"
	exprast "github.com/expr-lang/expr/ast"
	"github.com/jftrade/jftrade-main/pkg/market"
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
	previousBarTime       time.Time
	hasPreviousClose      bool
	hasPreviousBarTime    bool
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

type klinePayloadView struct {
	kline         *types.KLine
	session       market.Session
	startTimeText string
	endTimeText   string
	hasStartTime  bool
	hasEndTime    bool
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
