package backtest

import (
	"context"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/jftrade/jftrade-main/pkg/bbgo/fixedpoint"
	"github.com/jftrade/jftrade-main/pkg/bbgo/types"
)

type stage3OrderCapture struct {
	order       types.Order
	submittedAt time.Time
}

type stage3ReferenceState struct {
	orders          map[uint64]stage3OrderCapture
	orderSequence   []uint64
	trades          []types.Trade
	appliedTradeFee map[uint64]appliedTradeFees
}

type stage3AccountQuerier struct {
	account *types.Account
}

func (querier stage3AccountQuerier) QueryAccount(context.Context) (*types.Account, error) {
	return querier.account, nil
}

func runStage3ReferenceCorpus(t testing.TB, corpus stage3Corpus) stage3CorpusOutput {
	t.Helper()
	if corpus.Version != 1 {
		t.Fatalf("stage 3 corpus version = %d, want 1", corpus.Version)
	}
	output := stage3CorpusOutput{
		Version:        corpus.Version,
		ExecutionModel: stage3ExecutionModel,
		Cases:          make([]stage3CaseOutput, 0, len(corpus.Cases)),
	}
	seen := map[string]struct{}{}
	for index := range corpus.Cases {
		if _, exists := seen[corpus.Cases[index].ID]; exists {
			t.Fatalf("duplicate stage 3 case %q", corpus.Cases[index].ID)
		}
		seen[corpus.Cases[index].ID] = struct{}{}
		output.Cases = append(output.Cases, runStage3ReferenceCase(t, &corpus.Cases[index]))
	}
	return output
}

func runStage3ReferenceCase(t testing.TB, testCase *stage3Case) stage3CaseOutput {
	t.Helper()
	market := stage3ReferenceMarket(t, testCase)
	initialBalance := stage3Fixed(t, testCase.InitialBalance)
	account := types.NewAccount()
	account.UpdateBalances(types.BalanceMap{
		testCase.QuoteCurrency: {
			Currency:  testCase.QuoteCurrency,
			Available: initialBalance,
		},
	})
	stream := types.NewStandardStream()
	result := &RunResult{ExecutionModel: stage3ExecutionModel}
	collector := newResultCollector(testCase.Symbol, types.Interval1m, testCase.QuoteCurrency, time.Time{}, result)
	state := &stage3ReferenceState{
		orders:          map[uint64]stage3OrderCapture{},
		appliedTradeFee: map[uint64]appliedTradeFees{},
	}
	feeEngine := newBacktestFeeEngine(
		account,
		testCase.QuoteCurrency,
		"stock",
		stage3ReferenceTradingCosts(t, testCase.FeeRules),
		result,
		func(trade types.Trade, fees appliedTradeFees) {
			state.appliedTradeFee[trade.ID] = fees
			collector.recordTradeFees(trade, fees)
		},
	)
	stream.OnTradeUpdate(feeEngine.onTradeUpdate)
	stream.OnTradeUpdate(func(trade types.Trade) {
		state.trades = append(state.trades, trade)
	})
	stream.OnOrderUpdate(func(order types.Order) {
		capture, exists := state.orders[order.OrderID]
		if !exists {
			capture.submittedAt = order.CreationTime.Time()
			state.orderSequence = append(state.orderSequence, order.OrderID)
		}
		capture.order = order
		state.orders[order.OrderID] = capture
		collector.onOrderUpdate(order)
	})
	matcher := newConservativeBarExecutor(account, &stream, conservativeBarExecutorOptions{
		ProcessOrdersOnClose: testCase.ProcessOrdersOnClose,
		SlippageTicks:        testCase.SlippageTicks,
		WarningSink:          result,
	})

	status := "completed"
	processedBars := 0
	createdOrders := map[string]types.Order{}
	processedAtomicGroups := map[string]struct{}{}
	for barIndex, fixtureCandle := range testCase.Candles {
		if testCase.CancelBeforeBar != nil && *testCase.CancelBeforeBar == barIndex {
			status = "cancelled"
			break
		}
		candle := stage3ReferenceCandle(t, testCase.Symbol, fixtureCandle)
		matcher.onKLineClosed(candle)
		collector.onKLineClosed(context.Background(), stage3AccountQuerier{account: account}, candle)
		for intentIndex := range testCase.Intents {
			intent := &testCase.Intents[intentIndex]
			if intent.BarIndex != barIndex {
				continue
			}
			if intent.Action == "cancel" {
				if order, ok := createdOrders[intent.TargetID]; ok {
					if err := matcher.CancelOrders(context.Background(), order); err != nil {
						t.Fatal(err)
					}
				}
				continue
			}
			if intent.AtomicGroupID == "" {
				created := stage3SubmitOrders(t, matcher, market, []stage3OrderIntent{*intent})
				createdOrders[intent.ID] = created[0]
				continue
			}
			if _, exists := processedAtomicGroups[intent.AtomicGroupID]; exists {
				continue
			}
			processedAtomicGroups[intent.AtomicGroupID] = struct{}{}
			group := stage3AtomicGroupForBar(testCase.Intents, barIndex, intent.AtomicGroupID)
			created := stage3SubmitAtomicOrders(t, matcher, market, intent.AtomicGroupID, group)
			for index := range group {
				createdOrders[group[index].ID] = created[index]
			}
		}
		processedBars++
	}
	feeEngine.finalize()
	collector.finalize(context.Background(), stage3AccountQuerier{account: account}, initialBalance.Float64())
	return buildStage3ReferenceOutput(t, testCase, status, processedBars, account, result, collector, state)
}

func stage3ReferenceMarket(t testing.TB, testCase *stage3Case) types.Market {
	t.Helper()
	return types.Market{
		Exchange:      types.ExchangeBacktest,
		Symbol:        testCase.Symbol,
		BaseCurrency:  testCase.BaseCurrency,
		QuoteCurrency: testCase.QuoteCurrency,
		TickSize:      stage3Fixed(t, testCase.Market.TickSize),
		StepSize:      stage3Fixed(t, testCase.Market.QuantityStep),
		MinQuantity:   stage3Fixed(t, testCase.Market.MinQuantity),
	}
}

func stage3ReferenceCandle(t testing.TB, symbol string, candle stage3Candle) types.KLine {
	t.Helper()
	return types.KLine{
		Symbol:    symbol,
		Interval:  types.Interval1m,
		StartTime: types.Time(stage3Time(t, candle.Start)),
		EndTime:   types.Time(stage3Time(t, candle.End)),
		Open:      stage3Fixed(t, candle.Open),
		High:      stage3Fixed(t, candle.High),
		Low:       stage3Fixed(t, candle.Low),
		Close:     stage3Fixed(t, candle.Close),
		Volume:    stage3Fixed(t, candle.Volume),
	}
}

func stage3ReferenceTradingCosts(t testing.TB, input []stage3FeeRule) TradingCosts {
	t.Helper()
	costs := TradingCosts{
		BrokerFees: FeeSchedule{Mode: tradingCostModeCustom},
		MarketFees: FeeSchedule{Mode: tradingCostModeCustom},
	}
	for _, inputRule := range input {
		rule := FeeRule{
			ID:          inputRule.ID,
			Label:       inputRule.Label,
			Side:        inputRule.Side,
			Basis:       inputRule.Basis,
			Rate:        stage3Float(t, inputRule.Rate),
			FixedAmount: stage3Float(t, inputRule.FixedAmount),
			MinAmount:   stage3Float(t, inputRule.MinAmount),
			MaxAmount:   stage3Float(t, inputRule.MaxAmount),
			MaxRate:     stage3Float(t, inputRule.MaxRate),
			Rounding:    inputRule.Rounding,
		}
		switch inputRule.Group {
		case feeGroupBroker:
			rule.Category = feeCategoryBroker
			costs.BrokerFees.Rules = append(costs.BrokerFees.Rules, rule)
		case feeGroupMarket:
			rule.Category = feeCategoryExchange
			costs.MarketFees.Rules = append(costs.MarketFees.Rules, rule)
		default:
			t.Fatalf("unsupported fee group %q", inputRule.Group)
		}
	}
	return costs
}

func stage3SubmitOrders(
	t testing.TB,
	matcher *conservativeBarExecutor,
	market types.Market,
	intents []stage3OrderIntent,
) types.OrderSlice {
	t.Helper()
	orders := make([]types.SubmitOrder, 0, len(intents))
	for _, intent := range intents {
		orders = append(orders, stage3SubmitOrder(t, market, intent))
	}
	created, err := matcher.SubmitOrders(context.Background(), orders...)
	if err != nil {
		t.Fatal(err)
	}
	return created
}

func stage3SubmitAtomicOrders(
	t testing.TB,
	matcher *conservativeBarExecutor,
	market types.Market,
	groupID string,
	intents []stage3OrderIntent,
) types.OrderSlice {
	t.Helper()
	orders := make([]PineWorkerAtomicOrder, 0, len(intents))
	for _, intent := range intents {
		orders = append(orders, PineWorkerAtomicOrder{
			CommandID:  intent.ID,
			ParentID:   intent.ParentID,
			OCOGroupID: intent.OCOGroupID,
			Order:      stage3SubmitOrder(t, market, intent),
		})
	}
	created, err := matcher.SubmitAtomicPineOrders(context.Background(), groupID, orders...)
	if err != nil {
		t.Fatal(err)
	}
	return created
}

func stage3SubmitOrder(t testing.TB, market types.Market, intent stage3OrderIntent) types.SubmitOrder {
	t.Helper()
	return types.SubmitOrder{
		ClientOrderID: intent.ID,
		Symbol:        market.Symbol,
		Side:          types.SideType(strings.ToUpper(intent.Side)),
		Type:          stage3OrderType(t, intent.OrderType),
		Price:         stage3Fixed(t, intent.LimitPrice),
		StopPrice:     stage3Fixed(t, intent.StopPrice),
		Quantity:      stage3Fixed(t, intent.Quantity),
		ReduceOnly:    intent.ReduceOnly,
		Market:        market,
	}
}

func stage3OrderType(t testing.TB, value string) types.OrderType {
	t.Helper()
	switch value {
	case "", "market":
		return types.OrderTypeMarket
	case "limit":
		return types.OrderTypeLimit
	case "limit_maker":
		return types.OrderTypeLimitMaker
	case "stop_market":
		return types.OrderTypeStopMarket
	case "stop_limit":
		return types.OrderTypeStopLimit
	default:
		return types.OrderType(strings.ToUpper(value))
	}
}

func stage3AtomicGroupForBar(intents []stage3OrderIntent, barIndex int, groupID string) []stage3OrderIntent {
	group := make([]stage3OrderIntent, 0)
	for _, intent := range intents {
		if intent.BarIndex == barIndex && intent.AtomicGroupID == groupID {
			group = append(group, intent)
		}
	}
	return group
}

func buildStage3ReferenceOutput(
	t testing.TB,
	testCase *stage3Case,
	status string,
	processedBars int,
	account *types.Account,
	result *RunResult,
	collector *resultCollector,
	state *stage3ReferenceState,
) stage3CaseOutput {
	t.Helper()
	cash, _ := account.Balance(testCase.QuoteCurrency)
	base, _ := account.Balance(testCase.BaseCurrency)
	realizedPnL := fixedpoint.Zero
	for _, trade := range result.Trades {
		realizedPnL = realizedPnL.Add(fixedpoint.NewFromFloat(trade.PnL))
	}
	output := stage3CaseOutput{
		ID:              testCase.ID,
		Status:          status,
		ProcessedBars:   processedBars,
		Cash:            cash.Total().String(),
		BasePosition:    base.Total().String(),
		FinalEquity:     stage3FloatText(result.FinalBalance),
		RealizedPnL:     realizedPnL.String(),
		TotalBrokerFees: stage3FloatText(result.TotalBrokerFees),
		TotalMarketFees: stage3FloatText(result.TotalMarketFees),
		TotalFees:       stage3FloatText(result.TotalFees),
		TotalFills:      len(state.trades),
		TotalTrades:     result.TotalTrades,
		WinRate:         stage3MetricText(result.WinRate),
		Orders:          make([]stage3OrderOutput, 0, len(state.orderSequence)),
		Fills:           make([]stage3FillOutput, 0, len(state.trades)),
		EquityCurve:     make([]stage3EquityPoint, 0, len(result.PnLCurve)),
		DrawdownCurve:   make([]stage3DrawdownPoint, 0, len(result.DrawdownCurve)),
		FeeBreakdown:    make([]stage3FeeBreakdown, 0, len(result.FeeBreakdown)),
		Indicators:      stage3ReferenceIndicators(t, testCase.Candles[:processedBars], testCase.IndicatorPeriods),
		Warnings:        append([]string{}, result.Warnings...),
	}
	output.WinningTrades = stage3WinningTrades(result)
	output.MaxDrawdown = stage3MetricText(result.MaxDrawdown)
	output.CurrentDrawdown = stage3MetricText(result.CurrentDrawdown)
	for _, orderID := range state.orderSequence {
		capture := state.orders[orderID]
		output.Orders = append(output.Orders, stage3CapturedOrder(capture))
	}
	for index, trade := range state.trades {
		fees := state.appliedTradeFee[trade.ID]
		realized := 0.0
		if index < len(result.Trades) {
			realized = result.Trades[index].PnL
		}
		capture := state.orders[trade.OrderID]
		output.Fills = append(output.Fills, stage3CapturedFill(trade, capture.order, fees, realized))
	}
	for _, point := range result.PnLCurve {
		output.EquityCurve = append(output.EquityCurve, stage3EquityPoint{
			Time:   point.Time,
			Equity: stage3FloatText(point.Equity),
		})
	}
	for _, point := range result.DrawdownCurve {
		output.DrawdownCurve = append(output.DrawdownCurve, stage3DrawdownPoint{
			Time:     point.Time,
			Drawdown: stage3MetricText(point.Drawdown),
		})
	}
	for _, entry := range result.FeeBreakdown {
		output.FeeBreakdown = append(output.FeeBreakdown, stage3FeeBreakdown{
			RuleID: entry.RuleID,
			Label:  entry.Label,
			Group:  entry.Group,
			Amount: stage3FloatText(entry.Amount),
			Count:  entry.Count,
		})
	}
	sort.Slice(output.FeeBreakdown, func(i, j int) bool {
		if output.FeeBreakdown[i].Group != output.FeeBreakdown[j].Group {
			return output.FeeBreakdown[i].Group < output.FeeBreakdown[j].Group
		}
		return output.FeeBreakdown[i].RuleID < output.FeeBreakdown[j].RuleID
	})
	if len(collector.pnlCurve) != len(output.EquityCurve) {
		t.Fatal("stage 3 collector equity output lost points")
	}
	stage3PopulateResultHash(t, &output)
	return output
}

func stage3CapturedOrder(capture stage3OrderCapture) stage3OrderOutput {
	order := capture.order
	filledAt := ""
	if order.Status != types.OrderStatusNew {
		filledAt = order.UpdateTime.Time().UTC().Format(time.RFC3339Nano)
	}
	return stage3OrderOutput{
		OrderID:        fmt.Sprint(order.OrderID),
		ClientOrderID:  order.ClientOrderID,
		Side:           strings.ToLower(string(order.Side)),
		OrderType:      strings.ToLower(string(order.Type)),
		Quantity:       order.Quantity.String(),
		Status:         string(order.Status),
		FilledQuantity: order.ExecutedQuantity.String(),
		FilledPrice:    order.AveragePrice.String(),
		SubmittedAt:    capture.submittedAt.UTC().Format(time.RFC3339Nano),
		FilledAt:       filledAt,
		ReduceOnly:     order.ReduceOnly,
	}
}

func stage3CapturedFill(trade types.Trade, order types.Order, fees appliedTradeFees, realized float64) stage3FillOutput {
	return stage3FillOutput{
		TradeID:       fmt.Sprint(trade.ID),
		OrderID:       fmt.Sprint(trade.OrderID),
		ClientOrderID: order.ClientOrderID,
		Side:          strings.ToLower(string(trade.Side)),
		Price:         trade.Price.String(),
		Quantity:      trade.Quantity.String(),
		QuoteQuantity: trade.QuoteQuantity.String(),
		Time:          trade.Time.Time().UTC().Format(time.RFC3339Nano),
		Maker:         trade.IsMaker,
		BrokerFee:     fixedpoint.NewFromFloat(fees.BrokerFee).String(),
		MarketFee:     fixedpoint.NewFromFloat(fees.MarketFee).String(),
		TotalFee:      fixedpoint.NewFromFloat(fees.TotalFee).String(),
		RealizedPnL:   fixedpoint.NewFromFloat(realized).String(),
	}
}

func stage3WinningTrades(result *RunResult) int {
	wins := 0
	for _, trade := range result.Trades {
		if trade.PnL > 0 {
			wins++
		}
	}
	return wins
}

func stage3ReferenceIndicators(t testing.TB, candles []stage3Candle, periods []int) []stage3Indicator {
	t.Helper()
	closes := make([]fixedpoint.Value, len(candles))
	for index := range candles {
		closes[index] = stage3Fixed(t, candles[index].Close)
	}
	output := make([]stage3Indicator, 0, len(periods)*2)
	for _, period := range periods {
		if period <= 0 {
			t.Fatalf("invalid stage 3 indicator period %d", period)
		}
		output = append(output,
			stage3Indicator{Kind: "sma", Period: period, Values: stage3SMA(t, closes, period)},
			stage3Indicator{Kind: "ema", Period: period, Values: stage3EMA(t, closes, period)},
		)
	}
	return output
}

func stage3SMA(t testing.TB, values []fixedpoint.Value, period int) []*string {
	t.Helper()
	output := make([]*string, 0, len(values))
	sum := fixedpoint.Zero
	for index, value := range values {
		sum = sum.Add(value)
		if index >= period {
			sum = sum.Sub(values[index-period])
		}
		if index+1 < period {
			output = append(output, nil)
			continue
		}
		text := sum.Div(fixedpoint.NewFromInt(int64(period))).String()
		output = append(output, &text)
	}
	return output
}

func stage3EMA(t testing.TB, values []fixedpoint.Value, period int) []*string {
	t.Helper()
	output := make([]*string, 0, len(values))
	if len(values) == 0 {
		return output
	}
	alpha := fixedpoint.NewFromInt(2).Div(fixedpoint.NewFromInt(int64(period + 1)))
	oneMinusAlpha := fixedpoint.One.Sub(alpha)
	current := values[0]
	output = append(output, stage3StringPointer(current.String()))
	for _, value := range values[1:] {
		current = value.Mul(alpha).Add(current.Mul(oneMinusAlpha))
		output = append(output, stage3StringPointer(current.String()))
	}
	return output
}

func stage3StringPointer(value string) *string {
	return &value
}

func stage3FloatText(value float64) string {
	return fmt.Sprintf("%g", value)
}

func stage3PopulateResultHash(t testing.TB, output *stage3CaseOutput) {
	t.Helper()
	output.ResultHash = ""
	data, err := json.Marshal(output)
	if err != nil {
		t.Fatal(err)
	}
	hasher := fnv.New64a()
	if _, err := hasher.Write(data); err != nil {
		t.Fatal(err)
	}
	output.ResultHash = fmt.Sprintf("fnv1a64:%016x", hasher.Sum64())
}

func stage3Fixed(t testing.TB, value string) fixedpoint.Value {
	t.Helper()
	parsed, err := fixedpoint.NewFromString(value)
	if err != nil {
		t.Fatalf("invalid fixed value %q: %v", value, err)
	}
	return parsed
}

func stage3Float(t testing.TB, value string) float64 {
	t.Helper()
	return stage3Fixed(t, value).Float64()
}

func stage3Time(t testing.TB, value string) time.Time {
	t.Helper()
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		t.Fatalf("invalid stage 3 time %q: %v", value, err)
	}
	return parsed
}
