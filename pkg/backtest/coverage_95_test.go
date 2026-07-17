package backtest

import (
	"math"
	"testing"
	"time"

	"github.com/jftrade/jftrade-main/pkg/bbgo/fixedpoint"
	"github.com/jftrade/jftrade-main/pkg/bbgo/types"

	strategyir "github.com/jftrade/jftrade-main/pkg/strategy/ir"
	"github.com/jftrade/jftrade-main/pkg/strategy/pineworker"
)

func TestTradingCostHelperBoundaryRules(t *testing.T) {
	for _, mode := range []string{"market_preset", "custom", "script", "none", "unknown"} {
		if mode != "unknown" && normalizeTradingCostMode(mode) != mode {
			t.Fatalf("normalizeTradingCostMode(%q) = %q", mode, normalizeTradingCostMode(mode))
		}
	}
	if normalizeTradingCostMode("unknown") != "" || normalizeFeeCategory("unknown", feeGroupBroker) != feeCategoryBroker || normalizeFeeCategory("unknown", feeGroupMarket) != feeCategoryExchange {
		t.Fatal("fee mode/category defaults are incorrect")
	}
	if normalizeFeeSide("BUY") != feeSideBuy || normalizeFeeSide("sell") != feeSideSell || normalizeFeeSide("other") != feeSideBoth {
		t.Fatal("fee side normalization is incorrect")
	}
	if normalizeFeeBasis("contract") != feeBasisShare || normalizeFeeBasis("order") != feeBasisOrder || normalizeFeeBasis("other") != feeBasisNotional {
		t.Fatal("fee basis normalization is incorrect")
	}
	if normalizeInstrumentType("fund") != instrumentTypeETF || normalizeInstrumentType("other") != instrumentTypeStock {
		t.Fatal("instrument type normalization is incorrect")
	}
	if normalizeAppliesTo(nil) != nil {
		t.Fatal("empty applies-to list should remain nil")
	}
	if values := normalizeAppliesTo([]string{"ETF", "fund", "stock"}); len(values) != 2 || values[0] != instrumentTypeETF || values[1] != instrumentTypeStock {
		t.Fatalf("normalized applies-to = %#v", values)
	}
	if normalizeCostMarket("", "sz.000001") != "CN" || normalizeCostMarket("SH", "") != "CN" {
		t.Fatal("cost market normalization is incorrect")
	}
	if defaultBrokerFeeSchedule("unsupported", "USD").Mode != tradingCostModeNone || defaultMarketFeeSchedule("unsupported", "USD", "stock").Mode != tradingCostModeNone {
		t.Fatal("unsupported markets should not receive default fee rules")
	}
	if costs := defaultTradingCostsForRun("", "SH.600519", "cny", "fund"); costs.BrokerFees.Mode != tradingCostModeNone || costs.MarketFees.PresetID != "stock_connect_etf_market_fees_2026_06_30" {
		t.Fatalf("default CN ETF costs = %#v", costs)
	}

	preset := FeeSchedule{Mode: tradingCostModeMarketPreset, Rules: []FeeRule{{ID: "preset"}}}
	script := FeeSchedule{Mode: tradingCostModeScript, Rules: []FeeRule{{ID: "script"}}}
	for _, test := range []struct {
		group     string
		requested FeeSchedule
		wantMode  string
		wantID    string
	}{
		{group: feeGroupBroker, requested: FeeSchedule{Mode: "none"}, wantMode: tradingCostModeNone},
		{group: feeGroupBroker, requested: FeeSchedule{Mode: "script"}, wantMode: tradingCostModeScript, wantID: "script"},
		{group: feeGroupMarket, requested: FeeSchedule{Mode: "script"}, wantMode: tradingCostModeNone},
		{group: feeGroupBroker, requested: FeeSchedule{Mode: "market_preset"}, wantMode: tradingCostModeMarketPreset, wantID: "preset"},
		{group: feeGroupBroker, requested: FeeSchedule{Mode: "invalid"}, wantMode: tradingCostModeMarketPreset, wantID: "preset"},
	} {
		resolved := resolveFeeSchedule(test.group, test.requested, preset, script)
		if resolved.Mode != test.wantMode || (test.wantID != "" && (len(resolved.Rules) != 1 || resolved.Rules[0].ID != test.wantID)) {
			t.Fatalf("resolveFeeSchedule(%#v) = %#v", test, resolved)
		}
	}
	for _, test := range []struct {
		metadata strategyir.StrategyMetadata
		wantMode string
		basis    string
	}{
		{metadata: strategyir.StrategyMetadata{}, wantMode: tradingCostModeNone},
		{metadata: strategyir.StrategyMetadata{CommissionType: "percent", CommissionValue: 1}, wantMode: tradingCostModeScript, basis: feeBasisNotional},
		{metadata: strategyir.StrategyMetadata{CommissionType: "cash_per_order", CommissionValue: 1}, wantMode: tradingCostModeScript, basis: feeBasisOrder},
		{metadata: strategyir.StrategyMetadata{CommissionType: "cash_per_contract", CommissionValue: 1}, wantMode: tradingCostModeScript, basis: feeBasisShare},
		{metadata: strategyir.StrategyMetadata{CommissionType: "unsupported", CommissionValue: 1}, wantMode: tradingCostModeNone},
	} {
		schedule := pineBrokerFeeSchedule(test.metadata, "usd")
		if schedule.Mode != test.wantMode || (test.basis != "" && schedule.Rules[0].Basis != test.basis) {
			t.Fatalf("pineBrokerFeeSchedule(%#v) = %#v", test.metadata, schedule)
		}
	}
}

func TestFeeAndReplayPrimitiveBoundaryHelpers(t *testing.T) {
	trade := types.Trade{
		ID: 7, Side: types.SideTypeBuy, Price: fixedpoint.NewFromFloat(10), Quantity: fixedpoint.NewFromFloat(2),
	}
	if !feeRuleAppliesToSide(FeeRule{}, trade) || !feeRuleAppliesToSide(FeeRule{Side: feeSideBuy}, trade) || feeRuleAppliesToSide(FeeRule{Side: feeSideSell}, trade) {
		t.Fatal("fee side applicability is incorrect")
	}
	trade.Side = "other"
	if feeRuleAppliesToSide(FeeRule{Side: feeSideBuy}, trade) {
		t.Fatal("unknown trade side should not match")
	}
	if !feeRuleAppliesToInstrument(FeeRule{}, "stock") || !feeRuleAppliesToInstrument(FeeRule{AppliesTo: []string{"fund"}}, "etf") || feeRuleAppliesToInstrument(FeeRule{AppliesTo: []string{"etf"}}, "stock") {
		t.Fatal("instrument applicability is incorrect")
	}
	if got := feeOrderRuleKey("broker", FeeRule{ID: "fee"}, trade); got != "broker|fee|7" {
		t.Fatalf("fallback fee rule key = %q", got)
	}
	trade.OrderID = 9
	if got := feeOrderRuleKey("broker", FeeRule{ID: "fee"}, trade); got != "broker|fee|9" {
		t.Fatalf("order fee rule key = %q", got)
	}
	if ruleCapAmount(FeeRule{}, 100) != 0 || ruleCapAmount(FeeRule{MaxAmount: 5}, 100) != 5 || ruleCapAmount(FeeRule{MaxAmount: 5, MaxRate: 0.02}, 100) != 2 {
		t.Fatal("fee cap calculation is incorrect")
	}
	if roundedFeeAmount(1.1, "ceil_hkd") != 2 || roundedFeeAmount(1.001, "ceil_cent") != 1.01 || roundedFeeAmount(1.25, "") != 1.25 {
		t.Fatal("fee rounding is incorrect")
	}
	if positive := math.IsNaN(roundedFeeAmount(math.NaN(), "")); !positive {
		t.Fatal("rounding should preserve NaN for caller validation")
	}
	var nilEngine *backtestFeeEngine
	nilEngine.onTradeUpdate(types.Trade{})
	nilEngine.finalize()
	disableBacktestNativeFeeRates(nil)

	var batch *pineWorkerReplayKLineBatch
	if batch.Len() != 0 {
		t.Fatal("nil replay batch should have zero length")
	}
	if _, ok := batch.At(0); ok {
		t.Fatal("nil replay batch unexpectedly returned a bar")
	}
	batch = &pineWorkerReplayKLineBatch{}
	if batch.flatten() != nil {
		t.Fatal("empty replay batch should flatten to nil")
	}
	batch.forEach(nil)
	batch.append(types.KLine{Symbol: "US.AAPL", Interval: types.Interval("1m")})
	if _, ok := batch.At(-1); ok {
		t.Fatal("negative replay index unexpectedly succeeded")
	}
	if _, ok := batch.At(1); ok {
		t.Fatal("out-of-range replay index unexpectedly succeeded")
	}
	visited := 0
	batch.forEach(func(types.KLine) bool { visited++; return false })
	if visited != 1 || len(batch.flatten()) != 1 || batch.resultCapacity(time.Time{}) != 0 {
		t.Fatalf("replay batch boundaries visited=%d flattened=%d", visited, len(batch.flatten()))
	}
	if _, err := collectPineWorkerReplayKLineBatch(nil, time.Time{}, time.Time{}, nil, "US.AAPL", types.Interval("1m")); err == nil {
		t.Fatal("nil replay streamer was accepted")
	}
}

func TestPineWorkerCommandAndReplayValidationEdges(t *testing.T) {
	if got := (pineWorkerIgnoredOrderError{reason: "ignored"}).Error(); got != "ignored" {
		t.Fatalf("ignored-order error = %q", got)
	}
	warnings := &recordingIgnoredOrderWarnings{}
	executor := &PineWorkerCommandExecutor{Symbol: "", WarningSink: warnings}
	executor.warnIgnoredOrder(WorkerOrderCommand{Kind: "entry", FromEntry: "fallback", BarIndex: 1}, "test")
	executor.warnIgnoredOrder(WorkerOrderCommand{Kind: "entry", BarIndex: 2}, "test")
	if len(warnings.messages) != 2 {
		t.Fatalf("warning fallbacks = %#v", warnings.messages)
	}

	sizer := &pineWorkerReplaySizer{}
	executor.PositionSizer = sizer
	for _, test := range []struct {
		name      string
		net       float64
		direction string
		wantSkip  bool
		wantSide  types.SideType
	}{
		{name: "auto long", net: 2, wantSide: types.SideTypeSell},
		{name: "auto short", net: -2, wantSide: types.SideTypeBuy},
		{name: "auto flat", net: 0, wantSkip: true},
		{name: "long without long", net: -2, direction: "long", wantSkip: true},
		{name: "short without short", net: 2, direction: "short", wantSkip: true},
		{name: "unrecognized direction", net: 2, direction: "manual"},
	} {
		t.Run(test.name, func(t *testing.T) {
			sizer.netPosition = fixedpoint.NewFromFloat(test.net)
			resolved, skipped, err := executor.resolvePositionCloseCommand(WorkerOrderCommand{Kind: "close", Direction: test.direction})
			if err != nil || skipped != test.wantSkip || (!skipped && test.wantSide != "" && resolved.Side != test.wantSide) {
				t.Fatalf("resolve close = %#v / %v / %v", resolved, skipped, err)
			}
		})
	}
	if _, skipped, err := (&PineWorkerCommandExecutor{}).resolvePositionCloseCommand(WorkerOrderCommand{Kind: "close"}); err != nil || skipped {
		t.Fatalf("close without position reader = skipped %v err %v", skipped, err)
	}
	if _, skipped, err := executor.resolvePositionCloseCommand(WorkerOrderCommand{Kind: "entry"}); err != nil || skipped {
		t.Fatalf("entry should not be converted to a position close: %v / %v", skipped, err)
	}

	market := types.Market{StepSize: fixedpoint.NewFromFloat(0.1)}
	if normalizePineWorkerOrderQuantity(market, fixedpoint.Zero).Sign() != 0 || normalizePineWorkerOrderQuantity(market, fixedpoint.NewFromFloat(1.29)).Float64() != 1.2 {
		t.Fatal("step-size normalization is incorrect")
	}
	market = types.Market{VolumePrecision: 2}
	if normalizePineWorkerOrderQuantity(market, fixedpoint.NewFromFloat(1.239)).Float64() != 1.23 {
		t.Fatal("precision normalization is incorrect")
	}
	if _, err := requireTradablePineWorkerCommandQuantity(WorkerOrderCommand{ID: "zero"}, market, fixedpoint.Zero); err == nil {
		t.Fatal("zero quantity was accepted")
	}
	market.MinQuantity = fixedpoint.NewFromFloat(2)
	if _, err := requireTradablePineWorkerCommandQuantity(WorkerOrderCommand{ID: "small"}, market, fixedpoint.NewFromFloat(1)); err == nil {
		t.Fatal("below-minimum quantity was accepted")
	}
	if isPineWorkerShortReplayCommand(WorkerOrderCommand{Kind: "cancel", Direction: "short"}) || !isPineWorkerShortReplayCommand(WorkerOrderCommand{Kind: "close", Direction: "short"}) {
		t.Fatal("short replay command classification is incorrect")
	}

	for _, test := range []struct {
		kind, direction string
		want            types.SideType
		wantErr         bool
	}{
		{kind: "entry", direction: "buy", want: types.SideTypeBuy},
		{kind: "entry", direction: "sell", want: types.SideTypeSell},
		{kind: "close", direction: "", want: types.SideTypeSell},
		{kind: "close", direction: "cover", want: types.SideTypeBuy},
		{kind: "close", direction: "invalid", wantErr: true},
		{kind: "unknown", direction: "long", wantErr: true},
	} {
		side, err := sideForWorkerIntent(test.kind, test.direction)
		if (err != nil) != test.wantErr || (!test.wantErr && side != test.want) {
			t.Fatalf("sideForWorkerIntent(%q, %q) = %q, %v", test.kind, test.direction, side, err)
		}
	}
	if canonicalWorkerIntentDirection("entry", "buy") != "long" || canonicalWorkerIntentDirection("entry", "sell") != "short" || canonicalWorkerIntentDirection("close", "sell") != "long" {
		t.Fatal("worker direction canonicalization is incorrect")
	}

	candle := pineworker.Candle{OpenTime: 1}
	for _, request := range []PineWorkerReplayPlanRequest{
		{},
		{Source: "strategy"},
		{Source: "strategy", Symbol: "US.AAPL"},
		{Source: "strategy", Symbol: "US.AAPL", Timeframe: "1"},
	} {
		if _, err := buildPineWorkerBacktestRequest(request, nil); err == nil {
			t.Fatalf("invalid replay request %#v was accepted", request)
		}
	}
	valid := PineWorkerReplayPlanRequest{Source: "strategy", Symbol: "US.AAPL", Timeframe: "1", Params: map[string]string{"x": "1"}}
	request, err := buildPineWorkerBacktestRequest(valid, []pineworker.Candle{candle})
	if err != nil || request.JobID != "backtest:US.AAPL:1" || request.Params["x"] != "1" {
		t.Fatalf("built replay request = %#v, %v", request, err)
	}
	valid.Params["x"] = "changed"
	if request.Params["x"] != "1" {
		t.Fatal("replay request did not isolate parameter map")
	}
}

func TestPineWorkerAdapterAndPumpFailureContracts(t *testing.T) {
	if _, err := (PineWorkerReplayPlanner{}).Plan(t.Context(), PineWorkerReplayPlanRequest{}); err == nil {
		t.Fatal("planner accepted an invalid replay request")
	}
	if _, err := planPineWorkerCompactReplay(t.Context(), PineWorkerBacktestAdapter{}, PineWorkerReplayPlanRequest{}, nil); err == nil {
		t.Fatal("compact planner accepted an invalid replay request")
	}
	if err := (&PineWorkerReplayPump{}).validateKLine(types.KLine{}); err == nil {
		t.Fatal("pump validation accepted a plan without candles")
	}

	adapter := PineWorkerBacktestAdapter{Runner: &fakePineWorkerBacktestRunner{response: pineworker.RunScriptResponse{
		OrderIntents: []pineworker.OrderIntent{{Kind: "invalid"}},
	}}}
	if _, _, err := adapter.Run(t.Context(), validWorkerBacktestRequest()); err == nil {
		t.Fatal("adapter accepted an invalid worker command")
	}
	if _, err := CommandsFromOrderIntents([]pineworker.OrderIntent{{Kind: "cancel"}, {Kind: "invalid"}}); err == nil {
		t.Fatal("command conversion accepted a later invalid intent")
	}

	plan := PineWorkerReplayPlan{CandleCount: 1, Request: pineworker.RunScriptRequest{Candles: []pineworker.Candle{{OpenTime: 1}}}}
	pump := &PineWorkerReplayPump{Plan: plan}
	if err := pump.Consume(t.Context(), types.KLine{}); err == nil {
		t.Fatal("replay pump accepted a nil consumer")
	}
	pump.Consumer = coveragePineWorkerConsumer{}
	if err := pump.Consume(t.Context(), types.KLine{}); err == nil {
		t.Fatal("replay pump accepted a nil command executor")
	}
	pump.CommandExecutor = &PineWorkerCommandExecutor{}
	if err := pump.Consume(t.Context(), types.KLine{StartTime: types.Time(time.UnixMilli(2))}); err == nil {
		t.Fatal("replay pump accepted a kline with the wrong open time")
	}

	streamer := &fakePineWorkerReplayStreamer{}
	if _, err := collectPineWorkerReplayKLineBatch(streamer, time.Time{}, time.Time{}, nil, "", types.Interval1m); err == nil {
		t.Fatal("empty replay symbol was accepted")
	}
	if _, err := collectPineWorkerReplayKLineBatch(streamer, time.Time{}, time.Time{}, nil, "US.AAPL", ""); err == nil {
		t.Fatal("empty replay interval was accepted")
	}
	var nilSizer *pineWorkerReplaySizer
	if nilSizer.NetPosition().Sign() != 0 {
		t.Fatal("nil replay sizer should have no position")
	}
	commands, err := normalizeReplayCommands(
		[]pineworker.Candle{{OpenTime: 10}},
		[]WorkerOrderCommand{{Kind: "order", BarIndex: 0}, {Kind: "cancel", BarIndex: 0}},
	)
	if err != nil || len(commands) != 2 || commands[0].Kind != "cancel" || commands[1].Time != 10 {
		t.Fatalf("normalized replay commands = %#v, %v", commands, err)
	}
}

type coveragePineWorkerConsumer struct{}

func (coveragePineWorkerConsumer) ConsumeKLine(types.KLine, types.Interval) {}
