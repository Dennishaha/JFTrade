package pineruntime

import (
	"context"
	"testing"
	"time"

	bbgo2 "github.com/c9s/bbgo/pkg/bbgo"
	"github.com/c9s/bbgo/pkg/fixedpoint"
	"github.com/c9s/bbgo/pkg/types"

	"github.com/jftrade/jftrade-main/pkg/market"
	strategyir "github.com/jftrade/jftrade-main/pkg/strategy/ir"
)

func TestRuntimeHelpersNormalizePoliciesAndEntryGuards(t *testing.T) {
	if got := normalizeRuntimePyramiding(nil); got != 1 {
		t.Fatalf("normalizeRuntimePyramiding(nil) = %d", got)
	}
	if got := normalizeRuntimePyramiding(&strategyir.Program{}); got != 1 {
		t.Fatalf("normalizeRuntimePyramiding(default) = %d", got)
	}
	if got := normalizeRuntimePyramiding(&strategyir.Program{Metadata: strategyir.StrategyMetadata{Pyramiding: 3}}); got != 3 {
		t.Fatalf("normalizeRuntimePyramiding(custom) = %d", got)
	}

	if got := normalizeRuntimeAllowedEntryDirection(nil); got != "all" {
		t.Fatalf("normalizeRuntimeAllowedEntryDirection(nil) = %q", got)
	}
	if got := normalizeRuntimeAllowedEntryDirection(&strategyir.Program{Metadata: strategyir.StrategyMetadata{AllowedEntryDirection: " LONG "}}); got != "long" {
		t.Fatalf("normalizeRuntimeAllowedEntryDirection(long) = %q", got)
	}
	if got := normalizeRuntimeAllowedEntryDirection(&strategyir.Program{Metadata: strategyir.StrategyMetadata{AllowedEntryDirection: "short"}}); got != "short" {
		t.Fatalf("normalizeRuntimeAllowedEntryDirection(short) = %q", got)
	}
	if got := normalizeRuntimeAllowedEntryDirection(&strategyir.Program{Metadata: strategyir.StrategyMetadata{AllowedEntryDirection: "weird"}}); got != "all" {
		t.Fatalf("normalizeRuntimeAllowedEntryDirection(default) = %q", got)
	}

	if got := normalizeEntryPolicy("flatOnly"); got != "flat_only" {
		t.Fatalf("normalizeEntryPolicy(flatOnly) = %q", got)
	}
	if got := normalizeEntryPolicy("allow"); got != "allow" {
		t.Fatalf("normalizeEntryPolicy(allow) = %q", got)
	}
	if got := normalizeEntryPolicy("other"); got != "same_direction" {
		t.Fatalf("normalizeEntryPolicy(default) = %q", got)
	}

	if got := normalizeProtectDirection("LONG"); got != "long" {
		t.Fatalf("normalizeProtectDirection(long) = %q", got)
	}
	if got := normalizeProtectDirection("short"); got != "short" {
		t.Fatalf("normalizeProtectDirection(short) = %q", got)
	}
	if got := normalizeProtectDirection(""); got != "auto" {
		t.Fatalf("normalizeProtectDirection(default) = %q", got)
	}

	if got := firstPositiveFloat(-1, 0, 2.5, 7); got != 2.5 {
		t.Fatalf("firstPositiveFloat() = %v", got)
	}
	if got := absFloat(-3.2); got != 3.2 {
		t.Fatalf("absFloat() = %v", got)
	}

	longPosition := &positionSnapshot{Direction: "LONG", Quantity: 1}
	shortPosition := &positionSnapshot{Direction: "SHORT", Quantity: -1}
	if shouldSkipLongEntry(longPosition, 1, "allow", 1, 99) {
		t.Fatal("shouldSkipLongEntry allow = true")
	}
	if !shouldSkipLongEntry(longPosition, 1, "flat_only", 1, 0) {
		t.Fatal("shouldSkipLongEntry flat_only = false")
	}
	if !shouldSkipLongEntry(longPosition, 1, "same_direction", 2, 2) {
		t.Fatal("shouldSkipLongEntry pyramiding = false")
	}
	if shouldSkipLongEntry(longPosition, 0, "same_direction", 2, 2) {
		t.Fatal("shouldSkipLongEntry zero available = true")
	}

	if shouldSkipShortEntry(shortPosition, 1, "allow", 1, 99) {
		t.Fatal("shouldSkipShortEntry allow = true")
	}
	if !shouldSkipShortEntry(shortPosition, 1, "flat_only", 1, 0) {
		t.Fatal("shouldSkipShortEntry flat_only = false")
	}
	if !shouldSkipShortEntry(shortPosition, 1, "same_direction", 2, 2) {
		t.Fatal("shouldSkipShortEntry pyramiding = false")
	}

	if side, err := exchangeSideForAction(strategyir.OrderActionBuy); err != nil || side != types.SideTypeBuy {
		t.Fatalf("exchangeSideForAction(buy) = %q, %v", side, err)
	}
	if side, err := exchangeSideForAction(strategyir.OrderActionCover); err != nil || side != types.SideTypeBuy {
		t.Fatalf("exchangeSideForAction(cover) = %q, %v", side, err)
	}
	if side, err := exchangeSideForAction(strategyir.OrderActionSell); err != nil || side != types.SideTypeSell {
		t.Fatalf("exchangeSideForAction(sell) = %q, %v", side, err)
	}
	if _, err := exchangeSideForAction(strategyir.OrderAction("bad")); err == nil {
		t.Fatal("exchangeSideForAction(invalid) error = nil")
	}

	if got := normalizeOrderType(" limit "); got != types.OrderTypeLimit {
		t.Fatalf("normalizeOrderType(limit) = %q", got)
	}
	if got := normalizeOrderType("market"); got != types.OrderTypeMarket {
		t.Fatalf("normalizeOrderType(market) = %q", got)
	}
	if got := normalizeOrderIntent(strategyir.OrderIntentClose); got != strategyir.OrderIntentClose {
		t.Fatalf("normalizeOrderIntent(close) = %q", got)
	}
	if got := normalizeOrderIntent(strategyir.OrderIntent("other")); got != strategyir.OrderIntentEntry {
		t.Fatalf("normalizeOrderIntent(default) = %q", got)
	}

	metadataRuntime := &strategyRuntime{}
	statement := &strategyir.ExitStmt{
		Comment:         "generic",
		CommentProfit:   "take",
		CommentLoss:     "stop",
		CommentTrailing: "trail",
		AlertMessage:    "base alert",
		AlertProfit:     "profit alert",
		AlertLoss:       "loss alert",
		AlertTrailing:   "trail alert",
	}
	if got := metadataRuntime.resolveExitMetadata(statement, exitTriggerProfit); got.comment != "take" || got.alert != "profit alert" {
		t.Fatalf("resolveExitMetadata(profit) = %#v", got)
	}
	if got := metadataRuntime.resolveExitMetadata(statement, exitTriggerLoss); got.comment != "stop" || got.alert != "loss alert" {
		t.Fatalf("resolveExitMetadata(loss) = %#v", got)
	}
	if got := metadataRuntime.resolveExitMetadata(statement, exitTriggerTrailing); got.comment != "trail" || got.alert != "trail alert" {
		t.Fatalf("resolveExitMetadata(trailing) = %#v", got)
	}
	if got := metadataRuntime.resolveExitMetadata(&strategyir.ExitStmt{Comment: "generic", AlertMessage: "base"}, exitTriggerProfit); got.comment != "generic" || got.alert != "base" {
		t.Fatalf("resolveExitMetadata(fallback) = %#v", got)
	}
}

func TestRuntimeEntryCountStateTransitionsAndCacheHelpers(t *testing.T) {
	runtime := &strategyRuntime{
		strategy:         &Strategy{WarmupUntil: time.Date(2026, time.June, 22, 12, 0, 0, 0, time.UTC)},
		entrySubmitCount: map[string]int{"LONG": 2, "SHORT": 1},
		pendingOrders: map[string]pendingOrder{
			"sell":  {action: strategyir.OrderActionSell},
			"buy":   {action: strategyir.OrderActionBuy},
			"cover": {action: strategyir.OrderActionCover},
		},
		trailingExits: map[string]trailingExitState{
			"long-exit":  {direction: "LONG"},
			"short-exit": {direction: "SHORT"},
		},
	}

	if !runtime.isPlaceBlockedDuringWarmup(time.Date(2026, time.June, 22, 11, 0, 0, 0, time.UTC)) {
		t.Fatal("isPlaceBlockedDuringWarmup(before) = false")
	}
	if runtime.isPlaceBlockedDuringWarmup(time.Date(2026, time.June, 22, 13, 0, 0, 0, time.UTC)) {
		t.Fatal("isPlaceBlockedDuringWarmup(after) = true")
	}
	if runtime.sameDirectionEntryCount("long", &positionSnapshot{Direction: "LONG"}, 1) != 2 {
		t.Fatalf("sameDirectionEntryCount(existing) = %d", runtime.sameDirectionEntryCount("long", &positionSnapshot{Direction: "LONG"}, 1))
	}
	if got := (&strategyRuntime{}).sameDirectionEntryCount("short", &positionSnapshot{Direction: "SHORT"}, 2); got != 1 {
		t.Fatalf("sameDirectionEntryCount(fallback) = %d", got)
	}

	runtime.recordSubmittedOrderAction(strategyir.OrderActionBuy, 1, 0, 2)
	if runtime.entrySubmitCount["LONG"] != 3 {
		t.Fatalf("recordSubmittedOrderAction buy count = %#v", runtime.entrySubmitCount)
	}
	runtime.recordSubmittedOrderAction(strategyir.OrderActionSell, 0.5, 1, 0)
	if runtime.entrySubmitCount["LONG"] != 2 {
		t.Fatalf("recordSubmittedOrderAction sell count = %#v", runtime.entrySubmitCount)
	}

	runtime.recordEntryOrderAction(strategyir.OrderActionBuy, 1, 1, 0, entryOrderAdjustment{reversed: true, closeOnly: true})
	if _, exists := runtime.entrySubmitCount["SHORT"]; exists {
		t.Fatalf("reversed closeOnly should reset short count: %#v", runtime.entrySubmitCount)
	}
	if _, exists := runtime.trailingExits["short-exit"]; exists {
		t.Fatalf("reversed closeOnly should clear short trailing exits: %#v", runtime.trailingExits)
	}
	if _, exists := runtime.pendingOrders["buy"]; exists {
		t.Fatalf("long reversal should clear short buy pending order: %#v", runtime.pendingOrders)
	}
	if _, exists := runtime.pendingOrders["cover"]; exists {
		t.Fatalf("long reversal should clear short cover order: %#v", runtime.pendingOrders)
	}

	runtime.recordEntryOrderAction(strategyir.OrderActionShort, 1, 0, 1, entryOrderAdjustment{})
	if runtime.entrySubmitCount["SHORT"] != 2 {
		t.Fatalf("recordEntryOrderAction short count = %#v", runtime.entrySubmitCount)
	}

	barTime := time.Date(2026, time.June, 22, 10, 0, 0, 0, time.UTC)
	snapshot := &positionSnapshot{Symbol: "US.AAPL", Quantity: 3}
	if _, ok := runtime.cachedPosition("US.AAPL", barTime); ok {
		t.Fatal("cachedPosition before store = true")
	}
	runtime.storeCachedPosition("US.AAPL", barTime, snapshot)
	cached, ok := runtime.cachedPosition("US.AAPL", barTime)
	if !ok || cached != snapshot {
		t.Fatalf("cachedPosition after store = %#v, %v", cached, ok)
	}
	runtime.clearPositionCache()
	if _, ok := runtime.cachedPosition("US.AAPL", barTime); ok {
		t.Fatal("cachedPosition after clear = true")
	}
}

func TestStrategyIdentityAndSubscriptionBehavior(t *testing.T) {
	strategy := &Strategy{Symbol: "us.aapl", Interval: types.Interval5m}
	if got := strategy.ID(); got != ID {
		t.Fatalf("ID() = %q", got)
	}

	session := newPineTestSession()
	strategy.Subscribe(session)
	found := false
	for sub := range session.Subscriptions {
		if sub.Channel == types.KLineChannel && sub.Symbol == "US.AAPL" && sub.Options.Interval == types.Interval5m {
			found = true
		}
	}
	if !found {
		t.Fatalf("subscriptions = %#v", session.Subscriptions)
	}

	blank := &Strategy{}
	blank.Subscribe(session)
	if len(session.Subscriptions) != 1 {
		t.Fatalf("blank strategy should not add subscription: %#v", session.Subscriptions)
	}
}

func TestRuntimeSessionAndPositionHelpers(t *testing.T) {
	session := newPineTestSession()
	session.SetMarkets(types.MarketMap{
		"US.AAPL": {Symbol: "US.AAPL", BaseCurrency: "AAPL", QuoteCurrency: "USD"},
		"US.TSLA": {Symbol: "US.TSLA", BaseCurrency: "TSLA", QuoteCurrency: "USD"},
	})

	account := session.GetAccount()
	account.TotalAccountValue = fixedpoint.NewFromFloat(1000)

	runtime := &strategyRuntime{
		session:  session,
		strategy: &Strategy{Symbol: "US.AAPL"},
		symbol:   "US.AAPL",
	}

	if got := runtime.getTotalAccountValue(); got != 1000 {
		t.Fatalf("getTotalAccountValue(totalAccountValue) = %v", got)
	}

	account.TotalAccountValue = fixedpoint.Zero
	account.SetBalance("USD", types.Balance{NetAsset: fixedpoint.NewFromFloat(300)})
	account.SetBalance("AAPL", types.Balance{NetAsset: fixedpoint.NewFromFloat(2), Available: fixedpoint.NewFromFloat(1.5)})
	if got := runtime.getTotalAccountValue(); got != 302 {
		t.Fatalf("getTotalAccountValue(netAsset) = %v", got)
	}

	account.SetBalance("USD", types.Balance{Available: fixedpoint.NewFromFloat(120)})
	account.SetBalance("AAPL", types.Balance{Available: fixedpoint.NewFromFloat(5)})
	if got := runtime.getTotalAccountValue(); got != 125 {
		t.Fatalf("getTotalAccountValue(available) = %v", got)
	}

	pos, ok := session.Position("US.AAPL")
	if !ok {
		t.Fatal("Position(US.AAPL) not found")
	}
	pos.Base = fixedpoint.NewFromFloat(3)
	pos.AverageCost = fixedpoint.NewFromFloat(100)
	session.LastPrices()["US.AAPL"] = fixedpoint.NewFromFloat(110)
	barTime := time.Date(2026, time.June, 22, 10, 0, 0, 0, time.UTC)

	snapshot := runtime.getPosition("US.AAPL", barTime)
	if snapshot == nil {
		t.Fatal("getPosition() = nil")
	}
	if snapshot.Direction != "LONG" || snapshot.Quantity != 3 || snapshot.AvailableQuantity != 5 || snapshot.MarketValue != 330 || snapshot.AveragePrice != 100 {
		t.Fatalf("position snapshot = %#v", snapshot)
	}
	if cached, ok := runtime.cachedPosition("US.AAPL", barTime); !ok || cached != snapshot {
		t.Fatalf("cached snapshot = %#v, %v", cached, ok)
	}

	shortPos, ok := session.Position("US.TSLA")
	if !ok {
		t.Fatal("Position(US.TSLA) not found")
	}
	shortPos.Base = fixedpoint.NewFromFloat(-2)
	shortPos.AverageCost = fixedpoint.NewFromFloat(200)
	session.LastPrices()["US.TSLA"] = fixedpoint.Zero
	shortSnapshot := runtime.getPosition("US.TSLA", barTime.Add(time.Minute))
	if shortSnapshot == nil || shortSnapshot.Direction != "SHORT" || shortSnapshot.MarketValue != -400 {
		t.Fatalf("short snapshot = %#v", shortSnapshot)
	}

	scope := &evaluationScope{runtime: runtime, currentKlineTime: barTime.Add(2 * time.Minute)}
	current := scope.currentPosition()
	if current == nil || current.Symbol != "US.AAPL" {
		t.Fatalf("currentPosition() = %#v", current)
	}

	if got := (&evaluationScope{}).currentPosition(); got != nil {
		t.Fatalf("nil currentPosition() = %#v", got)
	}
	if got := (&strategyRuntime{}).getTotalAccountValue(); got != 0 {
		t.Fatalf("nil runtime getTotalAccountValue() = %v", got)
	}
	if got := (&strategyRuntime{}).getPosition("US.AAPL", barTime); got != nil {
		t.Fatalf("nil runtime getPosition() = %#v", got)
	}
}

func TestResolveKLineSessionUsesResolverAndFallbackClassification(t *testing.T) {
	resolved := market.SessionAfter
	session := newPineTestSessionWithExchange(&pineTestExchange{
		resolveSession: func(types.KLine) (market.Session, bool) {
			return resolved, true
		},
	})
	runtime := &strategyRuntime{
		session:  session,
		strategy: &Strategy{Symbol: "US.AAPL"},
	}
	kline := types.KLine{
		Symbol:    "US.AAPL",
		StartTime: types.Time(time.Date(2026, time.June, 22, 13, 30, 0, 0, time.UTC)),
		EndTime:   types.Time(time.Date(2026, time.June, 22, 14, 30, 0, 0, time.UTC)),
	}
	if got := runtime.resolveKLineSession(kline); got != resolved {
		t.Fatalf("resolveKLineSession(resolver) = %q", got)
	}

	fallbackRuntime := &strategyRuntime{strategy: &Strategy{Symbol: "US.AAPL"}}
	expected := market.ClassifySession("US.AAPL", kline.StartTime.Time().UTC())
	if got := fallbackRuntime.resolveKLineSession(kline); got != expected {
		t.Fatalf("resolveKLineSession(fallback) = %q, want %q", got, expected)
	}

	if got := (&strategyRuntime{}).resolveKLineSession(types.KLine{}); got != market.SessionUnknown {
		t.Fatalf("resolveKLineSession(zero) = %q", got)
	}
}

func TestKLinePayloadAndScopeReservedVariables(t *testing.T) {
	barTime := time.Date(2026, time.June, 22, 13, 30, 0, 0, time.UTC)
	kline := &types.KLine{
		Symbol:      "US.AAPL",
		Interval:    types.Interval1h,
		StartTime:   types.Time(barTime),
		EndTime:     types.Time(barTime.Add(time.Hour)),
		Open:        fixedpoint.NewFromFloat(100),
		High:        fixedpoint.NewFromFloat(110),
		Low:         fixedpoint.NewFromFloat(95),
		Close:       fixedpoint.NewFromFloat(108),
		Volume:      fixedpoint.NewFromFloat(200),
		QuoteVolume: fixedpoint.NewFromFloat(500),
		Closed:      true,
	}
	payload := &klinePayloadView{kline: kline, session: market.SessionRegular}
	if got, ok := payload.FieldValue("symbol"); !ok || got != "US.AAPL" {
		t.Fatalf("FieldValue(symbol) = %#v, %v", got, ok)
	}
	if got, ok := payload.FieldValue("interval"); !ok || got != "1h" {
		t.Fatalf("FieldValue(interval) = %#v, %v", got, ok)
	}
	if got, ok := payload.FieldValue("startTime"); !ok || got == "" {
		t.Fatalf("FieldValue(startTime) = %#v, %v", got, ok)
	}
	if !payload.hasStartTime {
		t.Fatal("FieldValue(startTime) did not cache formatted time")
	}
	if got, ok := payload.FieldValue("close"); !ok || got != 108.0 {
		t.Fatalf("FieldValue(close) = %#v, %v", got, ok)
	}
	if got, ok := payload.FieldValue("quoteVolume"); !ok || got != 500.0 {
		t.Fatalf("FieldValue(quoteVolume) = %#v, %v", got, ok)
	}
	if got, ok := payload.FieldValue("session"); !ok || got != string(market.SessionRegular) {
		t.Fatalf("FieldValue(session) = %#v, %v", got, ok)
	}
	if got, ok := (&klinePayloadView{kline: kline}).FieldValue("session"); !ok || got != "" {
		t.Fatalf("FieldValue(session unknown) = %#v, %v", got, ok)
	}
	if got, ok := (&klinePayloadView{}).FieldValue("symbol"); ok || got != nil {
		t.Fatalf("FieldValue(nil payload) = %#v, %v", got, ok)
	}

	session := newPineTestSession()
	session.SetMarkets(types.MarketMap{
		"US.AAPL": {Symbol: "US.AAPL", BaseCurrency: "AAPL", QuoteCurrency: "USD"},
	})
	account := session.GetAccount()
	account.TotalAccountValue = fixedpoint.NewFromFloat(750)
	pos, _ := session.Position("US.AAPL")
	pos.Base = fixedpoint.NewFromFloat(2)
	pos.AverageCost = fixedpoint.NewFromFloat(101)
	session.LastPrices()["US.AAPL"] = fixedpoint.NewFromFloat(109)

	runtime := &strategyRuntime{
		session:  session,
		symbol:   "US.AAPL",
		interval: types.Interval1h,
	}
	parent := &evaluationScope{variables: map[string]any{"custom": "parent-value"}}
	scope := &evaluationScope{
		runtime:            runtime,
		parent:             parent,
		currentKline:       kline,
		currentKlineTime:   barTime,
		currentKlineSymbol: "US.AAPL",
		currentSession:     market.SessionRegular,
		klinePayload:       *payload,
		hasBarData:         true,
		barIndex:           0,
	}

	if got, ok := scope.variable("custom"); !ok || got != "parent-value" {
		t.Fatalf("variable(custom) = %#v, %v", got, ok)
	}
	if got, ok := scope.reservedVariable("position_size"); !ok || got != 2.0 {
		t.Fatalf("reservedVariable(position_size) = %#v, %v", got, ok)
	}
	if got, ok := scope.reservedVariable("position_avg_price"); !ok || got != 101.0 {
		t.Fatalf("reservedVariable(position_avg_price) = %#v, %v", got, ok)
	}
	if got, ok := scope.reservedVariable("equity"); !ok || got != 750.0 {
		t.Fatalf("reservedVariable(equity) = %#v, %v", got, ok)
	}
	if got, ok := scope.reservedVariable("syminfo_tickerid"); !ok || got != "US.AAPL" {
		t.Fatalf("reservedVariable(syminfo_tickerid) = %#v, %v", got, ok)
	}
	if got, ok := scope.reservedVariable("syminfo_prefix"); !ok || got != "US" {
		t.Fatalf("reservedVariable(syminfo_prefix) = %#v, %v", got, ok)
	}
	if got, ok := scope.reservedVariable("timeframe_period"); !ok || got != "1h" {
		t.Fatalf("reservedVariable(timeframe_period) = %#v, %v", got, ok)
	}
	if got, ok := scope.reservedVariable("timeframe_isintraday"); !ok || got != true {
		t.Fatalf("reservedVariable(timeframe_isintraday) = %#v, %v", got, ok)
	}
	if got, ok := scope.reservedVariable("session_ismarket"); !ok || got != true {
		t.Fatalf("reservedVariable(session_ismarket) = %#v, %v", got, ok)
	}
	if got, ok := scope.reservedVariable("barstate_isfirst"); !ok || got != true {
		t.Fatalf("reservedVariable(barstate_isfirst) = %#v, %v", got, ok)
	}
}

func TestPendingOrderRegistrationTriggerAndCancelSemantics(t *testing.T) {
	entryStmt := &strategyir.OrderStmt{
		Range:              strategyir.SourceRange{StartLine: 12},
		ID:                 "entry-1",
		Action:             strategyir.OrderActionBuy,
		Intent:             strategyir.OrderIntentEntry,
		QuantityMode:       "shares",
		QuantityExpression: "2",
		OrderType:          string(types.OrderTypeLimit),
		LimitExpression:    "100",
		Comment:            "entry",
		AlertMessage:       "buy alert",
	}
	closeStmt := &strategyir.OrderStmt{
		Range:              strategyir.SourceRange{StartLine: 18},
		ID:                 "close-1",
		Action:             strategyir.OrderActionSell,
		Intent:             strategyir.OrderIntentClose,
		QuantityMode:       "shares",
		QuantityExpression: "1",
		OrderType:          string(types.OrderTypeLimit),
		LimitExpression:    "101",
	}
	stopStmt := &strategyir.OrderStmt{
		Range:              strategyir.SourceRange{StartLine: 24},
		ID:                 "stop-1",
		Action:             strategyir.OrderActionShort,
		Intent:             strategyir.OrderIntentClose,
		QuantityMode:       "shares",
		QuantityExpression: "1",
		StopExpression:     "99",
	}

	runtime := &strategyRuntime{pendingOrders: map[string]pendingOrder{}}
	if !runtime.shouldStorePendingOrder(entryStmt, strategyir.OrderIntentEntry) {
		t.Fatal("shouldStorePendingOrder(entry limit) = false")
	}
	if runtime.shouldStorePendingOrder(closeStmt, strategyir.OrderIntentClose) {
		t.Fatal("shouldStorePendingOrder(close limit) = true")
	}
	if !runtime.shouldStorePendingOrder(stopStmt, strategyir.OrderIntentClose) {
		t.Fatal("shouldStorePendingOrder(stop) = false")
	}

	runtime.storePendingOrder(entryStmt, strategyir.OrderActionBuy, strategyir.OrderIntentEntry, 2, 100, 0)
	pending := runtime.pendingOrders["entry-1"]
	if pending.sequence != 1 || pending.limitPrice != 100 || !pending.hasLimit || pending.hasStop {
		t.Fatalf("storePendingOrder(limit) = %#v", pending)
	}

	runtime.storePendingOrder(entryStmt, strategyir.OrderActionBuy, strategyir.OrderIntentEntry, 2, 100, 0)
	if runtime.pendingOrders["entry-1"].sequence != 1 {
		t.Fatalf("same pending order should preserve sequence: %#v", runtime.pendingOrders["entry-1"])
	}

	stopLimitStmt := &strategyir.OrderStmt{
		Range:              strategyir.SourceRange{StartLine: 30},
		ID:                 "stop-limit-1",
		Action:             strategyir.OrderActionBuy,
		Intent:             strategyir.OrderIntentEntry,
		QuantityMode:       "shares",
		QuantityExpression: "1",
		OrderType:          string(types.OrderTypeLimit),
		LimitExpression:    "101",
		StopExpression:     "103",
	}
	runtime.storePendingOrder(stopLimitStmt, strategyir.OrderActionBuy, strategyir.OrderIntentEntry, 1, 101, 103)
	stopLimit := runtime.pendingOrders["stop-limit-1"]
	if !stopLimit.hasLimit || !stopLimit.hasStop || stopLimit.limitPrice != 101 || stopLimit.stopPrice != 103 {
		t.Fatalf("storePendingOrder(stopLimit) = %#v", stopLimit)
	}

	if got := timeFromScope(&evaluationScope{currentKlineTime: time.Date(2026, time.June, 22, 15, 0, 0, 0, time.UTC)}); got.IsZero() {
		t.Fatalf("timeFromScope() = %v", got)
	}
	if !timeFromScope(nil).IsZero() {
		t.Fatal("timeFromScope(nil) should be zero")
	}

	if !pendingLimitTriggered(pendingOrder{action: strategyir.OrderActionBuy, limitPrice: 100}, 110, 99) {
		t.Fatal("pendingLimitTriggered(buy) = false")
	}
	if !pendingStopTriggered(pendingOrder{action: strategyir.OrderActionSell, stopPrice: 98}, 110, 97) {
		t.Fatal("pendingStopTriggered(sell) = false")
	}
	if !pendingOrderTriggered(pendingOrder{action: strategyir.OrderActionBuy, hasLimit: true, limitPrice: 100}, 110, 99) {
		t.Fatal("pendingOrderTriggered(limit) = false")
	}
	if pendingOrderTriggered(pendingOrder{action: strategyir.OrderActionBuy}, 110, 99) {
		t.Fatal("pendingOrderTriggered(empty) = true")
	}

	cancelRuntime := &strategyRuntime{
		pendingOrders: map[string]pendingOrder{
			"one": {id: "one"},
			"two": {id: "two"},
		},
	}
	cancelRuntime.executeCancelStatement(&strategyir.CancelStmt{ID: "one"})
	if _, exists := cancelRuntime.pendingOrders["one"]; exists {
		t.Fatalf("executeCancelStatement(id) = %#v", cancelRuntime.pendingOrders)
	}
	cancelRuntime.executeCancelStatement(&strategyir.CancelStmt{All: true})
	if len(cancelRuntime.pendingOrders) != 0 {
		t.Fatalf("executeCancelStatement(all) = %#v", cancelRuntime.pendingOrders)
	}
}

func TestTriggerPendingOrdersActivatesAndSubmitsInSequence(t *testing.T) {
	session := newPineTestSession()
	session.SetMarkets(types.MarketMap{
		"US.AAPL": {Symbol: "US.AAPL", BaseCurrency: "AAPL", QuoteCurrency: "USD"},
	})

	executor := &pineTestExecutor{}
	runtime := &strategyRuntime{
		ctx:              context.Background(),
		session:          session,
		executor:         executor,
		symbol:           "US.AAPL",
		pendingOrders:    map[string]pendingOrder{},
		entrySubmitCount: map[string]int{},
	}

	limitStmt := &strategyir.OrderStmt{
		Range:              strategyir.SourceRange{StartLine: 40},
		ID:                 "limit-buy",
		Action:             strategyir.OrderActionBuy,
		Intent:             strategyir.OrderIntentEntry,
		QuantityMode:       "shares",
		QuantityExpression: "2",
		OrderType:          string(types.OrderTypeLimit),
		LimitExpression:    "100",
	}
	stopLimitStmt := &strategyir.OrderStmt{
		Range:              strategyir.SourceRange{StartLine: 50},
		ID:                 "stop-limit-buy",
		Action:             strategyir.OrderActionBuy,
		Intent:             strategyir.OrderIntentEntry,
		QuantityMode:       "shares",
		QuantityExpression: "1",
		OrderType:          string(types.OrderTypeLimit),
		LimitExpression:    "101",
		StopExpression:     "103",
	}
	runtime.storePendingOrder(limitStmt, strategyir.OrderActionBuy, strategyir.OrderIntentEntry, 2, 100, 0)
	runtime.storePendingOrder(stopLimitStmt, strategyir.OrderActionBuy, strategyir.OrderIntentEntry, 1, 101, 103)

	scope := &evaluationScope{
		runtime:          runtime,
		currentKlineTime: time.Date(2026, time.June, 22, 15, 0, 0, 0, time.UTC),
		currentKline: &types.KLine{
			Symbol: "US.AAPL",
			Close:  fixedpoint.NewFromFloat(102),
			High:   fixedpoint.NewFromFloat(104),
			Low:    fixedpoint.NewFromFloat(99),
		},
	}

	if err := runtime.triggerPendingOrders(scope.currentKline, scope); err != nil {
		t.Fatalf("triggerPendingOrders(first) error = %v", err)
	}
	if len(executor.orders) != 2 {
		t.Fatalf("submitted orders after first trigger = %#v", executor.orders)
	}
	if _, exists := runtime.pendingOrders["limit-buy"]; exists {
		t.Fatalf("limit order should be removed after submit: %#v", runtime.pendingOrders)
	}
	stopLimit := runtime.pendingOrders["stop-limit-buy"]
	if !stopLimit.activated || !stopLimit.submitted {
		t.Fatalf("stop-limit after first trigger = %#v", stopLimit)
	}
	if runtime.entrySubmitCount["LONG"] != 2 {
		t.Fatalf("entry submit count = %#v", runtime.entrySubmitCount)
	}

	if err := runtime.triggerPendingOrders(scope.currentKline, scope); err != nil {
		t.Fatalf("triggerPendingOrders(second) error = %v", err)
	}
	if len(executor.orders) != 2 {
		t.Fatalf("submitted orders should not repeat after submitted stop-limit: %#v", executor.orders)
	}
}

func TestOrderPricingQuantityAndExitExecutionSemantics(t *testing.T) {
	session := newPineTestSession()
	session.SetMarkets(types.MarketMap{
		"US.AAPL": {
			Symbol:        "US.AAPL",
			BaseCurrency:  "AAPL",
			QuoteCurrency: "USD",
			TickSize:      fixedpoint.NewFromFloat(0.05),
		},
	})
	account := session.GetAccount()
	account.TotalAccountValue = fixedpoint.NewFromFloat(1000)
	account.SetBalance("AAPL", types.Balance{Available: fixedpoint.NewFromFloat(5)})
	pos, _ := session.Position("US.AAPL")
	pos.Base = fixedpoint.NewFromFloat(5)
	pos.AverageCost = fixedpoint.NewFromFloat(100)
	session.LastPrices()["US.AAPL"] = fixedpoint.NewFromFloat(108)

	executor := &pineTestExecutor{}
	runtime := &strategyRuntime{
		ctx:              context.Background(),
		session:          session,
		executor:         executor,
		symbol:           "US.AAPL",
		strategy:         &Strategy{},
		entrySubmitCount: map[string]int{"LONG": 3, "SHORT": 2},
	}
	scope := &evaluationScope{
		runtime:          runtime,
		currentKlineTime: time.Date(2026, time.June, 22, 15, 0, 0, 0, time.UTC),
		currentKline: &types.KLine{
			Symbol: "US.AAPL",
			Close:  fixedpoint.NewFromFloat(102),
			High:   fixedpoint.NewFromFloat(110),
			Low:    fixedpoint.NewFromFloat(95),
		},
	}

	if orderPrice, limitPrice, err := runtime.resolveOrderPrice(&strategyir.OrderStmt{OrderType: string(types.OrderTypeMarket)}, scope); err != nil || orderPrice != 102 || limitPrice != 0 {
		t.Fatalf("resolveOrderPrice(market) = %v %v %v", orderPrice, limitPrice, err)
	}
	if orderPrice, limitPrice, err := runtime.resolveOrderPrice(&strategyir.OrderStmt{OrderType: string(types.OrderTypeLimit), LimitExpression: "101"}, scope); err != nil || orderPrice != 101 || limitPrice != 101 {
		t.Fatalf("resolveOrderPrice(limit) = %v %v %v", orderPrice, limitPrice, err)
	}
	if orderPrice, limitPrice, err := runtime.resolveOrderPrice(&strategyir.OrderStmt{StopExpression: "99", LimitExpression: "98"}, scope); err != nil || orderPrice != 99 || limitPrice != 98 {
		t.Fatalf("resolveOrderPrice(stop-limit) = %v %v %v", orderPrice, limitPrice, err)
	}

	if value, ok, err := evaluateOptionalFloatExpression("", scope); err != nil || ok || value != 0 {
		t.Fatalf("evaluateOptionalFloatExpression(empty) = %v %v %v", value, ok, err)
	}
	if value, ok, err := evaluateOptionalFloatExpression("105", scope); err != nil || !ok || value != 105 {
		t.Fatalf("evaluateOptionalFloatExpression(value) = %v %v %v", value, ok, err)
	}
	if high, low, close := currentBarPrices(scope); high != 110 || low != 95 || close != 102 {
		t.Fatalf("currentBarPrices() = %v %v %v", high, low, close)
	}
	if trailingExitKey(&strategyir.ExitStmt{ID: " exit ", FromEntry: " entry "}) != "exit\x00entry" {
		t.Fatalf("trailingExitKey() = %q", trailingExitKey(&strategyir.ExitStmt{ID: " exit ", FromEntry: " entry "}))
	}
	if runtime.marketTickSize() != 0.05 {
		t.Fatalf("marketTickSize() = %v", runtime.marketTickSize())
	}

	if !isClosingOrderQuantity(&strategyir.OrderStmt{Intent: strategyir.OrderIntentClose}) {
		t.Fatal("isClosingOrderQuantity(close) = false")
	}
	if !isClosingOrderQuantity(&strategyir.OrderStmt{Action: strategyir.OrderActionSell}) {
		t.Fatal("isClosingOrderQuantity(sell) = false")
	}
	if isClosingOrderQuantity(&strategyir.OrderStmt{Intent: strategyir.OrderIntentNet}) {
		t.Fatal("isClosingOrderQuantity(net) = true")
	}

	if got := clampPercentBasedQuantity(0, 3, true); got != 1 {
		t.Fatalf("clampPercentBasedQuantity(closing fallback) = %v", got)
	}
	if got := clampPercentBasedQuantity(5, 3, false); got != 3 {
		t.Fatalf("clampPercentBasedQuantity(limit) = %v", got)
	}

	sharesQty, err := runtime.resolveOrderQuantity(&strategyir.OrderStmt{
		Action:             strategyir.OrderActionBuy,
		QuantityMode:       "shares",
		QuantityExpression: "2",
	}, scope, &positionSnapshot{Quantity: 5, AvailableQuantity: 5, MarketValue: 540}, 5, 100, "shares")
	if err != nil || sharesQty != 2 {
		t.Fatalf("resolveOrderQuantity(shares) = %v, %v", sharesQty, err)
	}
	accountPercentQty, err := runtime.resolveOrderQuantity(&strategyir.OrderStmt{
		Action:             strategyir.OrderActionBuy,
		QuantityMode:       "account_position_percent",
		QuantityExpression: "20",
	}, scope, &positionSnapshot{Quantity: 5, AvailableQuantity: 5, MarketValue: 540}, 5, 100, "account_position_percent")
	if err != nil || accountPercentQty != 2 {
		t.Fatalf("resolveOrderQuantity(account_position_percent) = %v, %v", accountPercentQty, err)
	}
	symbolPercentQty, err := runtime.resolveOrderQuantity(&strategyir.OrderStmt{
		Action:             strategyir.OrderActionSell,
		Intent:             strategyir.OrderIntentClose,
		QuantityMode:       "symbol_position_percent",
		QuantityExpression: "40",
	}, scope, &positionSnapshot{Quantity: 5, AvailableQuantity: 5, MarketValue: 540}, 5, 100, "symbol_position_percent")
	if err != nil || symbolPercentQty != 2 {
		t.Fatalf("resolveOrderQuantity(symbol_position_percent) = %v, %v", symbolPercentQty, err)
	}

	exitStmt := &strategyir.ExitStmt{
		Range:              strategyir.SourceRange{StartLine: 60},
		ID:                 "exit-1",
		QuantityMode:       "shares",
		QuantityExpression: "2",
		StopExpression:     "96",
		LimitExpression:    "109",
		Comment:            "take profit",
		AlertMessage:       "exit alert",
	}
	triggered, err := runtime.executeExitStatement(exitStmt, scope)
	if err != nil || !triggered {
		t.Fatalf("executeExitStatement(triggered) = %v, %v", triggered, err)
	}
	if len(executor.orders) != 1 {
		t.Fatalf("exit submitted orders = %#v", executor.orders)
	}
	if executor.orders[0].Side != types.SideTypeSell || executor.orders[0].Type != types.OrderTypeMarket || executor.orders[0].Quantity.Float64() != 2 {
		t.Fatalf("exit submit order = %#v", executor.orders[0])
	}
	if _, exists := runtime.entrySubmitCount["LONG"]; exists {
		t.Fatalf("executeExitStatement should reset long count: %#v", runtime.entrySubmitCount)
	}

	warmupExecutor := &pineTestExecutor{}
	warmupRuntime := &strategyRuntime{
		ctx:              context.Background(),
		session:          session,
		executor:         warmupExecutor,
		symbol:           "US.AAPL",
		strategy:         &Strategy{WarmupUntil: scope.currentKlineTime.Add(time.Hour)},
		entrySubmitCount: map[string]int{"LONG": 1},
	}
	warmupTriggered, err := warmupRuntime.executeExitStatement(exitStmt, scope)
	if err != nil || !warmupTriggered {
		t.Fatalf("executeExitStatement(warmup) = %v, %v", warmupTriggered, err)
	}
	if len(warmupExecutor.orders) != 0 {
		t.Fatalf("warmup should suppress exit submit: %#v", warmupExecutor.orders)
	}
}

func TestExecuteOrderStatementBusinessSemantics(t *testing.T) {
	t.Run("stores pending limit entry without immediate submit", func(t *testing.T) {
		session := newPineTestSession()
		session.SetMarkets(types.MarketMap{
			"US.AAPL": {Symbol: "US.AAPL", BaseCurrency: "AAPL", QuoteCurrency: "USD"},
		})
		executor := &pineTestExecutor{}
		runtime := &strategyRuntime{
			ctx:              context.Background(),
			session:          session,
			executor:         executor,
			symbol:           "US.AAPL",
			strategy:         &Strategy{},
			pendingOrders:    map[string]pendingOrder{},
			entrySubmitCount: map[string]int{},
		}
		scope := &evaluationScope{
			runtime:          runtime,
			currentKlineTime: time.Date(2026, time.June, 22, 15, 0, 0, 0, time.UTC),
			currentKline: &types.KLine{
				Symbol: "US.AAPL",
				Close:  fixedpoint.NewFromFloat(102),
				High:   fixedpoint.NewFromFloat(103),
				Low:    fixedpoint.NewFromFloat(101),
			},
		}
		statement := &strategyir.OrderStmt{
			Range:              strategyir.SourceRange{StartLine: 70},
			ID:                 "entry-limit",
			Action:             strategyir.OrderActionBuy,
			Intent:             strategyir.OrderIntentEntry,
			QuantityMode:       "shares",
			QuantityExpression: "2",
			OrderType:          string(types.OrderTypeLimit),
			LimitExpression:    "101",
			Comment:            "queue entry",
		}

		if err := runtime.executeOrderStatement(statement, scope); err != nil {
			t.Fatalf("executeOrderStatement(limit entry) error = %v", err)
		}
		if len(executor.orders) != 0 {
			t.Fatalf("limit entry should not submit immediately: %#v", executor.orders)
		}
		pending, ok := runtime.pendingOrders["entry-limit"]
		if !ok || pending.quantity != 2 || !pending.hasLimit || pending.limitPrice != 101 || pending.intent != strategyir.OrderIntentEntry {
			t.Fatalf("pending limit entry = %#v, %v", pending, ok)
		}
	})

	t.Run("flatten closes existing long position at market", func(t *testing.T) {
		session := newPineTestSession()
		session.SetMarkets(types.MarketMap{
			"US.AAPL": {Symbol: "US.AAPL", BaseCurrency: "AAPL", QuoteCurrency: "USD"},
		})
		account := session.GetAccount()
		account.SetBalance("AAPL", types.Balance{Available: fixedpoint.NewFromFloat(3)})
		pos, _ := session.Position("US.AAPL")
		pos.Base = fixedpoint.NewFromFloat(3)
		pos.AverageCost = fixedpoint.NewFromFloat(100)
		session.LastPrices()["US.AAPL"] = fixedpoint.NewFromFloat(102)

		executor := &pineTestExecutor{}
		runtime := &strategyRuntime{
			ctx:              context.Background(),
			session:          session,
			executor:         executor,
			symbol:           "US.AAPL",
			strategy:         &Strategy{},
			entrySubmitCount: map[string]int{"LONG": 3},
		}
		scope := &evaluationScope{
			runtime:          runtime,
			currentKlineTime: time.Date(2026, time.June, 22, 15, 1, 0, 0, time.UTC),
			currentKline: &types.KLine{
				Symbol: "US.AAPL",
				Close:  fixedpoint.NewFromFloat(102),
			},
		}
		statement := &strategyir.OrderStmt{
			Range:              strategyir.SourceRange{StartLine: 80},
			Action:             strategyir.OrderActionBuy,
			Intent:             strategyir.OrderIntentFlatten,
			QuantityMode:       "symbol_position_percent",
			QuantityExpression: "100",
			Comment:            "flatten long",
		}

		if err := runtime.executeOrderStatement(statement, scope); err != nil {
			t.Fatalf("executeOrderStatement(flatten) error = %v", err)
		}
		if len(executor.orders) != 1 {
			t.Fatalf("flatten submitted orders = %#v", executor.orders)
		}
		order := executor.orders[0]
		if order.Side != types.SideTypeSell || order.Type != types.OrderTypeMarket || order.Quantity.Float64() != 3 {
			t.Fatalf("flatten order = %#v", order)
		}
		if _, exists := runtime.entrySubmitCount["LONG"]; exists {
			t.Fatalf("flatten should clear long entry count: %#v", runtime.entrySubmitCount)
		}
	})

	t.Run("reversing short to long submits close plus entry and clears short state", func(t *testing.T) {
		session := newPineTestSession()
		session.SetMarkets(types.MarketMap{
			"US.AAPL": {Symbol: "US.AAPL", BaseCurrency: "AAPL", QuoteCurrency: "USD"},
		})
		pos, _ := session.Position("US.AAPL")
		pos.Base = fixedpoint.NewFromFloat(-3)
		pos.AverageCost = fixedpoint.NewFromFloat(100)
		session.LastPrices()["US.AAPL"] = fixedpoint.NewFromFloat(101)

		executor := &pineTestExecutor{}
		runtime := &strategyRuntime{
			ctx:      context.Background(),
			session:  session,
			executor: executor,
			symbol:   "US.AAPL",
			strategy: &Strategy{},
			pendingOrders: map[string]pendingOrder{
				"cover-short": {id: "cover-short", action: strategyir.OrderActionCover},
			},
			trailingExits: map[string]trailingExitState{
				"short-exit": {direction: "SHORT"},
			},
			entrySubmitCount: map[string]int{"SHORT": 2},
		}
		scope := &evaluationScope{
			runtime:          runtime,
			currentKlineTime: time.Date(2026, time.June, 22, 15, 2, 0, 0, time.UTC),
			currentKline: &types.KLine{
				Symbol: "US.AAPL",
				Close:  fixedpoint.NewFromFloat(101),
			},
		}
		statement := &strategyir.OrderStmt{
			Range:              strategyir.SourceRange{StartLine: 90},
			Action:             strategyir.OrderActionBuy,
			Intent:             strategyir.OrderIntentEntry,
			QuantityMode:       "shares",
			QuantityExpression: "2",
			Comment:            "reverse to long",
		}

		if err := runtime.executeOrderStatement(statement, scope); err != nil {
			t.Fatalf("executeOrderStatement(reverse) error = %v", err)
		}
		if len(executor.orders) != 1 {
			t.Fatalf("reverse submitted orders = %#v", executor.orders)
		}
		order := executor.orders[0]
		if order.Side != types.SideTypeBuy || order.Quantity.Float64() != 5 {
			t.Fatalf("reverse order = %#v", order)
		}
		if _, exists := runtime.pendingOrders["cover-short"]; exists {
			t.Fatalf("reverse should clear short pending orders: %#v", runtime.pendingOrders)
		}
		if _, exists := runtime.trailingExits["short-exit"]; exists {
			t.Fatalf("reverse should clear short trailing exits: %#v", runtime.trailingExits)
		}
		if _, exists := runtime.entrySubmitCount["SHORT"]; exists {
			t.Fatalf("reverse should clear short entry count: %#v", runtime.entrySubmitCount)
		}
		if runtime.entrySubmitCount["LONG"] != 1 {
			t.Fatalf("reverse long entry count = %#v", runtime.entrySubmitCount)
		}
	})

	t.Run("disallowed reverse only closes short exposure", func(t *testing.T) {
		session := newPineTestSession()
		session.SetMarkets(types.MarketMap{
			"US.AAPL": {Symbol: "US.AAPL", BaseCurrency: "AAPL", QuoteCurrency: "USD"},
		})
		pos, _ := session.Position("US.AAPL")
		pos.Base = fixedpoint.NewFromFloat(-4)
		pos.AverageCost = fixedpoint.NewFromFloat(100)
		session.LastPrices()["US.AAPL"] = fixedpoint.NewFromFloat(99)

		executor := &pineTestExecutor{}
		runtime := &strategyRuntime{
			ctx:                   context.Background(),
			session:               session,
			executor:              executor,
			symbol:                "US.AAPL",
			strategy:              &Strategy{},
			allowedEntryDirection: "short",
			pendingOrders: map[string]pendingOrder{
				"cover-short": {id: "cover-short", action: strategyir.OrderActionCover},
			},
			trailingExits: map[string]trailingExitState{
				"short-exit": {direction: "short"},
			},
			entrySubmitCount: map[string]int{"SHORT": 2},
		}
		scope := &evaluationScope{
			runtime:          runtime,
			currentKlineTime: time.Date(2026, time.June, 22, 15, 3, 0, 0, time.UTC),
			currentKline: &types.KLine{
				Symbol: "US.AAPL",
				Close:  fixedpoint.NewFromFloat(99),
			},
		}
		statement := &strategyir.OrderStmt{
			Range:              strategyir.SourceRange{StartLine: 100},
			Action:             strategyir.OrderActionBuy,
			Intent:             strategyir.OrderIntentEntry,
			QuantityMode:       "shares",
			QuantityExpression: "2",
		}

		if err := runtime.executeOrderStatement(statement, scope); err != nil {
			t.Fatalf("executeOrderStatement(closeOnly reverse) error = %v", err)
		}
		if len(executor.orders) != 1 {
			t.Fatalf("closeOnly reverse orders = %#v", executor.orders)
		}
		order := executor.orders[0]
		if order.Side != types.SideTypeBuy || order.Quantity.Float64() != 4 {
			t.Fatalf("closeOnly reverse order = %#v", order)
		}
		if _, exists := runtime.entrySubmitCount["LONG"]; exists {
			t.Fatalf("closeOnly reverse should not open long count: %#v", runtime.entrySubmitCount)
		}
		if _, exists := runtime.entrySubmitCount["SHORT"]; exists {
			t.Fatalf("closeOnly reverse should clear short count: %#v", runtime.entrySubmitCount)
		}
		if _, exists := runtime.pendingOrders["cover-short"]; exists {
			t.Fatalf("closeOnly reverse should clear short pending orders: %#v", runtime.pendingOrders)
		}
		if _, exists := runtime.trailingExits["short-exit"]; exists {
			t.Fatalf("closeOnly reverse should clear short trailing exits: %#v", runtime.trailingExits)
		}
	})
}

func TestExecuteTrailingExitBusinessSemantics(t *testing.T) {
	t.Run("long trailing exit activates then triggers on pullback", func(t *testing.T) {
		session := newPineTestSession()
		session.SetMarkets(types.MarketMap{
			"US.AAPL": {
				Symbol:        "US.AAPL",
				BaseCurrency:  "AAPL",
				QuoteCurrency: "USD",
				TickSize:      fixedpoint.NewFromFloat(0.05),
			},
		})
		account := session.GetAccount()
		account.SetBalance("AAPL", types.Balance{Available: fixedpoint.NewFromFloat(5)})
		pos, _ := session.Position("US.AAPL")
		pos.Base = fixedpoint.NewFromFloat(5)
		pos.AverageCost = fixedpoint.NewFromFloat(100)
		session.LastPrices()["US.AAPL"] = fixedpoint.NewFromFloat(101)

		executor := &pineTestExecutor{}
		runtime := &strategyRuntime{
			ctx:              context.Background(),
			session:          session,
			executor:         executor,
			symbol:           "US.AAPL",
			strategy:         &Strategy{},
			trailingExits:    map[string]trailingExitState{},
			entrySubmitCount: map[string]int{"LONG": 2},
		}
		statement := &strategyir.ExitStmt{
			Range:              strategyir.SourceRange{StartLine: 110},
			ID:                 "trail-long",
			TrailPoints:        "20",
			TrailOffset:        "10",
			QuantityMode:       "symbol_position_percent",
			QuantityExpression: "50",
			Comment:            "long trailing",
		}
		firstScope := &evaluationScope{
			runtime:          runtime,
			currentKlineTime: time.Date(2026, time.June, 22, 15, 4, 0, 0, time.UTC),
			currentKline: &types.KLine{
				Symbol: "US.AAPL",
				Close:  fixedpoint.NewFromFloat(101.4),
				High:   fixedpoint.NewFromFloat(101.5),
				Low:    fixedpoint.NewFromFloat(101.2),
			},
		}

		triggered, err := runtime.executeExitStatement(statement, firstScope)
		if err != nil || triggered {
			t.Fatalf("executeExitStatement(long activate) = %v, %v", triggered, err)
		}
		state, ok := runtime.trailingExits["trail-long\x00"]
		if !ok || !state.activated || state.direction != "long" || state.stopPrice != 101 {
			t.Fatalf("long trailing state after activation = %#v, %v", state, ok)
		}
		if len(executor.orders) != 0 {
			t.Fatalf("activation bar should not submit orders: %#v", executor.orders)
		}

		secondScope := &evaluationScope{
			runtime:          runtime,
			currentKlineTime: time.Date(2026, time.June, 22, 15, 5, 0, 0, time.UTC),
			currentKline: &types.KLine{
				Symbol: "US.AAPL",
				Close:  fixedpoint.NewFromFloat(101.6),
				High:   fixedpoint.NewFromFloat(102),
				Low:    fixedpoint.NewFromFloat(101.4),
			},
		}

		triggered, err = runtime.executeExitStatement(statement, secondScope)
		if err != nil || !triggered {
			t.Fatalf("executeExitStatement(long trigger) = %v, %v", triggered, err)
		}
		if len(executor.orders) != 1 {
			t.Fatalf("long trailing submitted orders = %#v", executor.orders)
		}
		order := executor.orders[0]
		if order.Side != types.SideTypeSell || order.Type != types.OrderTypeMarket || order.Quantity.Float64() != 2 {
			t.Fatalf("long trailing order = %#v", order)
		}
		if _, exists := runtime.trailingExits["trail-long\x00"]; exists {
			t.Fatalf("triggered long trailing exit should be cleared: %#v", runtime.trailingExits)
		}
		if _, exists := runtime.entrySubmitCount["LONG"]; exists {
			t.Fatalf("triggered long trailing exit should clear long count: %#v", runtime.entrySubmitCount)
		}
	})

	t.Run("short trailing exit with trail price triggers and warmup suppresses submit", func(t *testing.T) {
		session := newPineTestSession()
		session.SetMarkets(types.MarketMap{
			"US.AAPL": {
				Symbol:        "US.AAPL",
				BaseCurrency:  "AAPL",
				QuoteCurrency: "USD",
				TickSize:      fixedpoint.NewFromFloat(0.05),
			},
		})
		pos, _ := session.Position("US.AAPL")
		pos.Base = fixedpoint.NewFromFloat(-4)
		pos.AverageCost = fixedpoint.NewFromFloat(100)
		session.LastPrices()["US.AAPL"] = fixedpoint.NewFromFloat(99)

		statement := &strategyir.ExitStmt{
			Range:              strategyir.SourceRange{StartLine: 120},
			ID:                 "trail-short",
			Direction:          "short",
			TrailPrice:         "99",
			TrailOffset:        "20",
			QuantityMode:       "shares",
			QuantityExpression: "1",
			Comment:            "short trailing",
		}
		scope := &evaluationScope{
			currentKlineTime: time.Date(2026, time.June, 22, 15, 6, 0, 0, time.UTC),
			currentKline: &types.KLine{
				Symbol: "US.AAPL",
				Close:  fixedpoint.NewFromFloat(99),
				High:   fixedpoint.NewFromFloat(100),
				Low:    fixedpoint.NewFromFloat(98.5),
			},
		}

		executor := &pineTestExecutor{}
		runtime := &strategyRuntime{
			ctx:              context.Background(),
			session:          session,
			executor:         executor,
			symbol:           "US.AAPL",
			strategy:         &Strategy{},
			trailingExits:    map[string]trailingExitState{},
			entrySubmitCount: map[string]int{"SHORT": 2},
		}
		scope.runtime = runtime

		triggered, err := runtime.executeExitStatement(statement, scope)
		if err != nil || !triggered {
			t.Fatalf("executeExitStatement(short trigger) = %v, %v", triggered, err)
		}
		if len(executor.orders) != 1 {
			t.Fatalf("short trailing submitted orders = %#v", executor.orders)
		}
		order := executor.orders[0]
		if order.Side != types.SideTypeBuy || order.Type != types.OrderTypeMarket || order.Quantity.Float64() != 1 {
			t.Fatalf("short trailing order = %#v", order)
		}
		if _, exists := runtime.entrySubmitCount["SHORT"]; exists {
			t.Fatalf("short trailing should clear short count: %#v", runtime.entrySubmitCount)
		}

		warmupExecutor := &pineTestExecutor{}
		warmupRuntime := &strategyRuntime{
			ctx:              context.Background(),
			session:          session,
			executor:         warmupExecutor,
			symbol:           "US.AAPL",
			strategy:         &Strategy{WarmupUntil: scope.currentKlineTime.Add(time.Minute)},
			trailingExits:    map[string]trailingExitState{},
			entrySubmitCount: map[string]int{"SHORT": 1},
		}
		scope.runtime = warmupRuntime

		triggered, err = warmupRuntime.executeExitStatement(statement, scope)
		if err != nil || !triggered {
			t.Fatalf("executeExitStatement(short warmup) = %v, %v", triggered, err)
		}
		if len(warmupExecutor.orders) != 0 {
			t.Fatalf("warmup trailing exit should not submit orders: %#v", warmupExecutor.orders)
		}
		if warmupRuntime.entrySubmitCount["SHORT"] != 1 {
			t.Fatalf("warmup trailing exit should preserve short count: %#v", warmupRuntime.entrySubmitCount)
		}
	})
}

type pineTestExchange struct {
	resolveSession func(types.KLine) (market.Session, bool)
}

type pineTestExecutor struct {
	orders []types.SubmitOrder
}

func (e *pineTestExecutor) SubmitOrders(ctx context.Context, orders ...types.SubmitOrder) (types.OrderSlice, error) {
	e.orders = append(e.orders, orders...)
	return nil, nil
}

func (e *pineTestExecutor) CancelOrders(ctx context.Context, orders ...types.Order) error {
	return nil
}

func (e *pineTestExchange) Name() types.ExchangeName { return types.ExchangeBinance }
func (e *pineTestExchange) PlatformFeeCurrency() string {
	return "USD"
}
func (e *pineTestExchange) NewStream() types.Stream {
	stream := types.NewStandardStream()
	return &stream
}
func (e *pineTestExchange) QueryMarkets(context.Context) (types.MarketMap, error) { return nil, nil }
func (e *pineTestExchange) QueryTicker(context.Context, string) (*types.Ticker, error) {
	return nil, nil
}
func (e *pineTestExchange) QueryTickers(context.Context, ...string) (map[string]types.Ticker, error) {
	return nil, nil
}
func (e *pineTestExchange) QueryKLines(context.Context, string, types.Interval, types.KLineQueryOptions) ([]types.KLine, error) {
	return nil, nil
}
func (e *pineTestExchange) QueryAccount(context.Context) (*types.Account, error) {
	return types.NewAccount(), nil
}
func (e *pineTestExchange) QueryAccountBalances(context.Context) (types.BalanceMap, error) {
	return types.BalanceMap{}, nil
}
func (e *pineTestExchange) SubmitOrder(context.Context, types.SubmitOrder) (*types.Order, error) {
	return nil, nil
}
func (e *pineTestExchange) QueryOpenOrders(context.Context, string) ([]types.Order, error) {
	return nil, nil
}
func (e *pineTestExchange) CancelOrders(context.Context, ...types.Order) error { return nil }
func (e *pineTestExchange) ResolveKLineSession(kline types.KLine) (market.Session, bool) {
	if e.resolveSession != nil {
		return e.resolveSession(kline)
	}
	return market.SessionUnknown, false
}

func newPineTestSession() *bbgo2.ExchangeSession {
	return newPineTestSessionWithExchange(&pineTestExchange{})
}

func newPineTestSessionWithExchange(exchange types.Exchange) *bbgo2.ExchangeSession {
	session := bbgo2.NewExchangeSession("test", exchange)
	session.Account = types.NewAccount()
	return session
}
