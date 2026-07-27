package strategy

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jftrade/jftrade-main/pkg/bbgo/fixedpoint"
	"github.com/jftrade/jftrade-main/pkg/bbgo/types"
	"github.com/jftrade/jftrade-main/pkg/strategy/pineworker"
)

func TestDefaultPineProducesEscapedCanonicalStarterStrategy(t *testing.T) {
	t.Run("blank name uses the product default", func(t *testing.T) {
		source := DefaultPine(" ")
		if !strings.Contains(source, `strategy("Pine Strategy"`) {
			t.Fatalf("DefaultPine(blank) source = %q", source)
		}
		if !strings.Contains(source, `strategy.entry("Long", strategy.long)`) {
			t.Fatalf("DefaultPine(blank) is missing the canonical entry: %q", source)
		}
	})

	t.Run("user name is trimmed and escaped as Pine source", func(t *testing.T) {
		source := DefaultPine(`  opening "range"  `)
		if !strings.Contains(source, `strategy("opening \"range\""`) {
			t.Fatalf("DefaultPine(quoted name) source = %q", source)
		}
	})
}

func TestServiceDelegatesDefinitionVersionHistoryWithIdentityPreserved(t *testing.T) {
	history := &recordingDefinitionHistoryStore{
		summaries: []DefinitionVersionSummary{{Version: "2.1.0"}},
		version:   DefinitionVersion{Definition: Definition{Version: "1.9.0"}},
	}
	service := NewService(history, &fakeCatalogStore{}, &fakeRuntimeManager{})

	summaries, found, err := service.ListDefinitionVersions("mean-reversion")
	if err != nil || !found || len(summaries) != 1 || summaries[0].Version != "2.1.0" {
		t.Fatalf("ListDefinitionVersions() = %#v, %v, %v", summaries, found, err)
	}
	if history.listedDefinitionID != "mean-reversion" {
		t.Fatalf("listed definition id = %q", history.listedDefinitionID)
	}

	version, found, err := service.GetDefinitionVersion("mean-reversion", "1.9.0")
	if err != nil || !found || version.Version != "1.9.0" {
		t.Fatalf("GetDefinitionVersion() = %#v, %v, %v", version, found, err)
	}
	if history.loadedDefinitionID != "mean-reversion" || history.loadedVersion != "1.9.0" {
		t.Fatalf("loaded version identity = %q@%q", history.loadedDefinitionID, history.loadedVersion)
	}
}

func TestCommandsFromOrderIntentsRejectsInvalidAtomicOCOLegs(t *testing.T) {
	base := pineworker.OrderIntent{
		Kind:          "exit",
		ID:            "protect",
		Direction:     "long",
		LimitPrice:    110,
		HasLimitPrice: true,
		StopPrice:     95,
		HasStopPrice:  true,
		AtomicGroupID: "bracket",
		OCOGroupID:    "protective",
	}

	badLimit := base
	badLimit.LimitPrice = -1
	if _, err := CommandsFromOrderIntents([]pineworker.OrderIntent{badLimit}); err == nil ||
		!strings.Contains(err.Error(), "limit price must be positive") {
		t.Fatalf("invalid limit OCO error = %v", err)
	}

	badStop := base
	badStop.StopPrice = -1
	if _, err := CommandsFromOrderIntents([]pineworker.OrderIntent{badStop}); err == nil ||
		!strings.Contains(err.Error(), "stop price must be positive") {
		t.Fatalf("invalid stop OCO error = %v", err)
	}
}

func TestWorkerIntentDirectionAliasesPreserveTradingSide(t *testing.T) {
	tests := []struct {
		name      string
		intent    pineworker.OrderIntent
		direction string
		side      types.SideType
	}{
		{
			name:      "buy entry is canonical long",
			intent:    pineworker.OrderIntent{Kind: "entry", Direction: "buy"},
			direction: "long",
			side:      types.SideTypeBuy,
		},
		{
			name:      "sell close targets a long position",
			intent:    pineworker.OrderIntent{Kind: "close", Direction: "sell", HasQuantity: true, Quantity: 1},
			direction: "long",
			side:      types.SideTypeSell,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			command, ok, err := CommandFromOrderIntent(test.intent)
			if err != nil || !ok {
				t.Fatalf("CommandFromOrderIntent() error = %v ok = %v", err, ok)
			}
			if command.Direction != test.direction || command.Side != test.side {
				t.Fatalf("command direction/side = %q/%q", command.Direction, command.Side)
			}
		})
	}

	if _, err := sideForWorkerIntent("close", "sideways"); err == nil ||
		!strings.Contains(err.Error(), "unsupported pine worker close direction") {
		t.Fatalf("unsupported close direction error = %v", err)
	}
	if _, err := sideForWorkerIntent("bracket", "long"); err == nil ||
		!strings.Contains(err.Error(), "unsupported pine worker order intent kind") {
		t.Fatalf("unsupported intent kind error = %v", err)
	}
}

func TestExecuteBarCommandsPreflightsBeforeBrokerSideEffects(t *testing.T) {
	t.Run("unsupported command rejects the whole bar", func(t *testing.T) {
		orders := &fakeWorkerOrderExecutor{}
		executor := validLiveCommandExecutor(orders)
		err := executor.ExecuteBarCommands(t.Context(), []WorkerOrderCommand{
			{Kind: "entry", ID: "valid", Side: types.SideTypeBuy, Quantity: 1},
			{Kind: "replace", ID: "unsupported"},
		})
		if err == nil || !strings.Contains(err.Error(), "unsupported pine worker command kind") {
			t.Fatalf("ExecuteBarCommands() error = %v", err)
		}
		if len(orders.submitted) != 0 {
			t.Fatalf("preflight failure submitted orders = %#v", orders.submitted)
		}
	})

	t.Run("blank cancellation identity rejects the whole bar", func(t *testing.T) {
		orders := &fakeWorkerOrderExecutor{}
		executor := validLiveCommandExecutor(orders)
		err := executor.ExecuteBarCommands(t.Context(), []WorkerOrderCommand{
			{Kind: "entry", ID: "valid", Side: types.SideTypeBuy, Quantity: 1},
			{Kind: "cancel", ID: " "},
		})
		if err == nil || !strings.Contains(err.Error(), "cancel command id is required") {
			t.Fatalf("ExecuteBarCommands() error = %v", err)
		}
		if len(orders.submitted) != 0 {
			t.Fatalf("preflight failure submitted orders = %#v", orders.submitted)
		}
	})

	t.Run("ignored market order leaves the broker untouched", func(t *testing.T) {
		orders := &fakeWorkerOrderExecutor{}
		warnings := &recordingIgnoredOrderWarnings{}
		executor := validLiveCommandExecutor(orders)
		executor.WarningSink = warnings
		executor.RejectOrdersWithoutMarketRules = true

		err := executor.ExecuteBarCommands(t.Context(), []WorkerOrderCommand{{
			Kind: "entry", ID: "missing-rules", Side: types.SideTypeBuy, Quantity: 1,
		}})
		if err != nil {
			t.Fatalf("ExecuteBarCommands() error = %v", err)
		}
		if len(orders.submitted) != 0 || warnings.ignored != 1 {
			t.Fatalf("ignored order submitted=%#v warnings=%#v", orders.submitted, warnings.messages)
		}
	})

	t.Run("ordinary submission failure is returned with command identity", func(t *testing.T) {
		orders := &fakeWorkerOrderExecutor{submitErr: errors.New("broker unavailable")}
		executor := validLiveCommandExecutor(orders)
		err := executor.ExecuteBarCommands(t.Context(), []WorkerOrderCommand{{
			Kind: "entry", ID: "open-long", Side: types.SideTypeBuy, Quantity: 1,
		}})
		if err == nil || !strings.Contains(err.Error(), "submit pine worker command open-long") ||
			!strings.Contains(err.Error(), "broker unavailable") {
			t.Fatalf("ExecuteBarCommands() error = %v", err)
		}
	})

	t.Run("invalid order dependency prevents all submissions", func(t *testing.T) {
		orders := &fakeWorkerOrderExecutor{}
		executor := validLiveCommandExecutor(orders)
		executor.MarketResolver = nil
		err := executor.ExecuteBarCommands(t.Context(), []WorkerOrderCommand{{
			Kind: "entry", ID: "open-long", Side: types.SideTypeBuy, Quantity: 1,
		}})
		if err == nil || !strings.Contains(err.Error(), "market resolver is required") {
			t.Fatalf("ExecuteBarCommands() error = %v", err)
		}
		if len(orders.submitted) != 0 {
			t.Fatalf("invalid dependency submitted orders = %#v", orders.submitted)
		}
	})
}

func TestAtomicPineOrderValidationRejectsUnsafeGroupShapes(t *testing.T) {
	entry := WorkerOrderCommand{
		Kind: "entry", ID: "long", Side: types.SideTypeBuy, Quantity: 1, AtomicGroupID: "bracket",
	}
	protectiveLimit := WorkerOrderCommand{
		Kind: "exit", ID: "take-profit", ParentID: "long", Side: types.SideTypeSell,
		Quantity: 1, AtomicGroupID: "bracket", ReduceOnly: true, OrderType: types.OrderTypeLimit,
	}
	plan := func(commands ...WorkerOrderCommand) []liveCommandPlan {
		plans := make([]liveCommandPlan, 0, len(commands))
		for _, command := range commands {
			plans = append(plans, liveCommandPlan{
				command: command,
				order:   types.SubmitOrder{Type: command.OrderType},
			})
		}
		return plans
	}

	tests := []struct {
		name     string
		commands []WorkerOrderCommand
		plans    []liveCommandPlan
		want     string
	}{
		{
			name: "single leg cannot claim atomic placement", commands: []WorkerOrderCommand{entry},
			plans: plan(entry), want: "requires at least two commands",
		},
		{
			name: "entry cannot be reduce only",
			commands: []WorkerOrderCommand{
				withWorkerCommand(entry, func(command *WorkerOrderCommand) { command.ReduceOnly = true }),
				entry,
			},
			plans: plan(entry, entry), want: "reduce-only entry",
		},
		{
			name: "entry needs stable identity",
			commands: []WorkerOrderCommand{
				withWorkerCommand(entry, func(command *WorkerOrderCommand) { command.ID = "" }),
				entry,
			},
			plans: plan(entry, entry), want: "entry without an id",
		},
		{
			name: "cancellation is not placement",
			commands: []WorkerOrderCommand{
				entry,
				{Kind: "cancel", ID: "long", AtomicGroupID: "bracket"},
			},
			plans: plan(entry), want: "cannot contain cancellation commands",
		},
		{
			name: "unknown command cannot enter atomic placement",
			commands: []WorkerOrderCommand{
				entry,
				{Kind: "replace", ID: "long", AtomicGroupID: "bracket"},
			},
			plans: plan(entry), want: "contains unsupported command kind",
		},
		{
			name: "entry cannot be an OCO child",
			commands: []WorkerOrderCommand{
				withWorkerCommand(entry, func(command *WorkerOrderCommand) { command.OCOGroupID = "oco" }),
				entry,
			},
			plans: plan(entry, entry), want: "cannot be an OCO child",
		},
		{
			name:     "preflight-skipped protection makes group indivisible",
			commands: []WorkerOrderCommand{entry, protectiveLimit},
			plans: []liveCommandPlan{
				{command: entry},
				{command: protectiveLimit, skip: true},
			},
			want: "contains an order that cannot be placed",
		},
		{
			name: "protective exit must be reduce only",
			commands: []WorkerOrderCommand{
				entry,
				withWorkerCommand(protectiveLimit, func(command *WorkerOrderCommand) { command.ReduceOnly = false }),
			},
			plans: plan(
				entry,
				withWorkerCommand(protectiveLimit, func(command *WorkerOrderCommand) { command.ReduceOnly = false }),
			),
			want: "non-reduce-only protective exit",
		},
		{
			name: "OCO protection needs two legs",
			commands: []WorkerOrderCommand{
				entry,
				withWorkerCommand(protectiveLimit, func(command *WorkerOrderCommand) { command.OCOGroupID = "oco" }),
			},
			plans: plan(
				entry,
				withWorkerCommand(protectiveLimit, func(command *WorkerOrderCommand) { command.OCOGroupID = "oco" }),
			),
			want: "requires exactly two protective legs",
		},
		{
			name: "OCO pair needs a limit leg",
			commands: []WorkerOrderCommand{
				entry,
				withWorkerCommand(protectiveLimit, func(command *WorkerOrderCommand) {
					command.ID = "stop-a"
					command.OCOGroupID = "oco"
					command.OrderType = types.OrderTypeStopMarket
				}),
				withWorkerCommand(protectiveLimit, func(command *WorkerOrderCommand) {
					command.ID = "stop-b"
					command.OCOGroupID = "oco"
					command.OrderType = types.OrderTypeStopMarket
				}),
			},
			plans: plan(
				entry,
				withWorkerCommand(protectiveLimit, func(command *WorkerOrderCommand) {
					command.ID = "stop-a"
					command.OCOGroupID = "oco"
					command.OrderType = types.OrderTypeStopMarket
				}),
				withWorkerCommand(protectiveLimit, func(command *WorkerOrderCommand) {
					command.ID = "stop-b"
					command.OCOGroupID = "oco"
					command.OrderType = types.OrderTypeStopMarket
				}),
			),
			want: "requires one limit leg",
		},
		{
			name: "OCO pair needs a stop leg",
			commands: []WorkerOrderCommand{
				entry,
				withWorkerCommand(protectiveLimit, func(command *WorkerOrderCommand) { command.ID = "limit-a"; command.OCOGroupID = "oco" }),
				withWorkerCommand(protectiveLimit, func(command *WorkerOrderCommand) { command.ID = "limit-b"; command.OCOGroupID = "oco" }),
			},
			plans: plan(
				entry,
				withWorkerCommand(protectiveLimit, func(command *WorkerOrderCommand) { command.ID = "limit-a"; command.OCOGroupID = "oco" }),
				withWorkerCommand(protectiveLimit, func(command *WorkerOrderCommand) { command.ID = "limit-b"; command.OCOGroupID = "oco" }),
			),
			want: "requires one stop leg",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := validatePineWorkerAtomicGroup("bracket", test.commands, test.plans)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("validatePineWorkerAtomicGroup() error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestAtomicPineOrderSubmissionIsAllOrNothing(t *testing.T) {
	t.Run("broker atomic rejection is identified by group", func(t *testing.T) {
		orders := &controlledAtomicOrderExecutor{atomicErr: errors.New("atomic placement rejected")}
		executor := atomicLiveCommandExecutor(orders)
		err := executor.ExecuteBarCommands(t.Context(), pineWorkerAtomicBracketCommands())
		if err == nil || !strings.Contains(err.Error(), "submit pine worker atomic group bracket-1") ||
			!strings.Contains(err.Error(), "atomic placement rejected") {
			t.Fatalf("ExecuteBarCommands() error = %v", err)
		}
		if len(executor.activeOrders) != 0 {
			t.Fatalf("rejected atomic group tracked orders = %#v", executor.activeOrders)
		}
	})

	t.Run("partial broker result is rejected instead of tracked", func(t *testing.T) {
		orders := &controlledAtomicOrderExecutor{returnedOrders: 1}
		executor := atomicLiveCommandExecutor(orders)
		err := executor.ExecuteBarCommands(t.Context(), pineWorkerAtomicBracketCommands())
		if err == nil || !strings.Contains(err.Error(), "returned 1 orders for 3 commands") {
			t.Fatalf("ExecuteBarCommands() error = %v", err)
		}
		if len(executor.activeOrders) != 0 {
			t.Fatalf("partial atomic result tracked orders = %#v", executor.activeOrders)
		}
	})

	t.Run("single-command entry point rejects atomic fragments", func(t *testing.T) {
		executor := validLiveCommandExecutor(&fakeWorkerOrderExecutor{})
		err := executor.Execute(t.Context(), WorkerOrderCommand{
			Kind: "entry", ID: "orphan-leg", AtomicGroupID: "bracket",
		})
		if err == nil || !strings.Contains(err.Error(), "complete bar command group") {
			t.Fatalf("Execute() error = %v", err)
		}
	})
}

func TestPositionAwareCloseNeverCrossesTheWrongSide(t *testing.T) {
	tests := []struct {
		name          string
		position      fixedpoint.Value
		direction     string
		side          types.SideType
		wantSubmitted bool
		wantSide      types.SideType
		wantWarning   string
	}{
		{
			name: "automatic close sells an open long", position: fixedpoint.NewFromFloat(3),
			wantSubmitted: true, wantSide: types.SideTypeSell,
		},
		{
			name: "automatic close ignores a flat account", position: fixedpoint.Zero,
			wantWarning: "no open position is available",
		},
		{
			name: "short close ignores a non-short account", position: fixedpoint.NewFromFloat(3),
			direction: "short", wantWarning: "no short position is open",
		},
		{
			name: "unknown direction is left to the explicit side", position: fixedpoint.NewFromFloat(3),
			direction: "custom", side: types.SideTypeBuy, wantSubmitted: true, wantSide: types.SideTypeBuy,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			orders := &fakeWorkerOrderExecutor{}
			warnings := &recordingIgnoredOrderWarnings{}
			executor := validLiveCommandExecutor(orders)
			executor.PositionSizer = positionAwareCommandSizer{position: test.position}
			executor.WarningSink = warnings

			err := executor.Execute(t.Context(), WorkerOrderCommand{
				Kind: "close", ID: "close-position", Direction: test.direction,
				Side: test.side, Quantity: 1,
			})
			if err != nil {
				t.Fatalf("Execute() error = %v", err)
			}
			if test.wantSubmitted {
				if len(orders.submitted) != 1 || orders.submitted[0].Side != test.wantSide {
					t.Fatalf("submitted orders = %#v", orders.submitted)
				}
				return
			}
			if len(orders.submitted) != 0 || len(warnings.messages) != 1 ||
				!strings.Contains(warnings.messages[0], test.wantWarning) {
				t.Fatalf("ignored close submitted=%#v warnings=%#v", orders.submitted, warnings.messages)
			}
		})
	}

	t.Run("sizer without position reader preserves an explicit close", func(t *testing.T) {
		orders := &fakeWorkerOrderExecutor{}
		executor := validLiveCommandExecutor(orders)
		executor.PositionSizer = fixedPineWorkerCommandSizer{quantity: fixedpoint.One}
		err := executor.Execute(t.Context(), WorkerOrderCommand{
			Kind: "close", ID: "explicit-close", Direction: "long",
			Side: types.SideTypeSell, Quantity: 1,
		})
		if err != nil || len(orders.submitted) != 1 || orders.submitted[0].Side != types.SideTypeSell {
			t.Fatalf("explicit close error=%v submitted=%#v", err, orders.submitted)
		}
	})
}

func TestIgnoredOrderWarningsRetainFallbackIdentityAndSymbol(t *testing.T) {
	warnings := &recordingIgnoredOrderWarnings{}
	executor := &LiveCommandExecutor{WarningSink: warnings}

	executor.warnIgnoredOrder(WorkerOrderCommand{
		Kind: "exit", FromEntry: "long-entry", BarIndex: 8,
	}, "position is already flat")
	executor.warnIgnoredOrder(WorkerOrderCommand{
		Kind: "entry", BarIndex: 9,
	}, "market is closed")

	if len(warnings.messages) != 2 {
		t.Fatalf("warning messages = %#v", warnings.messages)
	}
	if !strings.Contains(warnings.messages[0], `exit command "long-entry" for <unknown>`) {
		t.Fatalf("from-entry warning = %q", warnings.messages[0])
	}
	if !strings.Contains(warnings.messages[1], `entry command "<anonymous>" for <unknown>`) {
		t.Fatalf("anonymous warning = %q", warnings.messages[1])
	}
}

func TestLiveOrderQuantityRespectsMinimumAndPrecision(t *testing.T) {
	t.Run("positive quantity below minimum is ignored with reason", func(t *testing.T) {
		orders := &fakeWorkerOrderExecutor{}
		warnings := &recordingIgnoredOrderWarnings{}
		market := testLiveCommandMarket()
		market.StepSize = fixedpoint.One
		market.MinQuantity = fixedpoint.NewFromInt(10)
		executor := &LiveCommandExecutor{
			Symbol: "US.AAPL", OrderExecutor: orders,
			MarketResolver: fakeWorkerMarketResolver{"US.AAPL": market},
			WarningSink:    warnings,
		}
		err := executor.Execute(t.Context(), WorkerOrderCommand{
			Kind: "entry", ID: "small-order", Side: types.SideTypeBuy, Quantity: 5,
		})
		if err != nil {
			t.Fatalf("Execute() error = %v", err)
		}
		if len(orders.submitted) != 0 || len(warnings.messages) != 1 ||
			!strings.Contains(warnings.messages[0], "less than market min quantity") {
			t.Fatalf("small order submitted=%#v warnings=%#v", orders.submitted, warnings.messages)
		}
	})

	t.Run("volume precision rounds down when no step is available", func(t *testing.T) {
		market := testLiveCommandMarket()
		market.StepSize = fixedpoint.Zero
		market.MinQuantity = fixedpoint.Zero
		market.VolumePrecision = 2
		executor := &LiveCommandExecutor{
			Symbol: "US.AAPL", OrderExecutor: &fakeWorkerOrderExecutor{},
			MarketResolver: fakeWorkerMarketResolver{"US.AAPL": market},
		}
		order, err := executor.SubmitOrderFromCommand(WorkerOrderCommand{
			Kind: "entry", ID: "precise-order", Side: types.SideTypeBuy, Quantity: 1.239,
		})
		if err != nil {
			t.Fatalf("SubmitOrderFromCommand() error = %v", err)
		}
		if order.Quantity.Float64() != 1.23 {
			t.Fatalf("normalized quantity = %s", order.Quantity)
		}
	})

	if got := normalizePineWorkerOrderQuantity(types.Market{}, fixedpoint.NewFromInt(-1)); !got.IsZero() {
		t.Fatalf("negative normalized quantity = %s", got)
	}
	if isPineWorkerShortCommand(WorkerOrderCommand{Kind: "cancel", Direction: "short"}) {
		t.Fatal("cancellation must not be classified as a short order")
	}
	if got := (liveIgnoredOrderError{reason: "market lot unavailable"}).Error(); got != "market lot unavailable" {
		t.Fatalf("ignored-order error = %q", got)
	}
}

func TestCancelByIntentDeduplicatesAliasesAndToleratesStaleMappings(t *testing.T) {
	orders := &fakeWorkerOrderExecutor{}
	executor := validLiveCommandExecutor(orders)
	executor.activeOrders = map[string]types.Order{
		"protect:limit": {SubmitOrder: types.SubmitOrder{ClientOrderID: "protect:limit"}},
	}
	executor.activeOrderAliases = map[string][]string{
		"protect": {"protect:limit", "protect:limit", "missing-leg"},
	}

	if err := executor.Execute(t.Context(), WorkerOrderCommand{Kind: "cancel", ID: "protect"}); err != nil {
		t.Fatalf("cancel OCO intent error = %v", err)
	}
	if len(orders.cancelled) != 1 || orders.cancelled[0].ClientOrderID != "protect:limit" {
		t.Fatalf("cancelled orders = %#v", orders.cancelled)
	}
	if len(executor.activeOrders) != 0 {
		t.Fatalf("active orders after cancel = %#v", executor.activeOrders)
	}

	executor.activeOrderAliases["stale-intent"] = []string{"missing-leg"}
	if err := executor.Execute(t.Context(), WorkerOrderCommand{Kind: "cancel", ID: "stale-intent"}); err != nil {
		t.Fatalf("cancel stale intent error = %v", err)
	}
	if len(orders.cancelled) != 1 {
		t.Fatalf("stale intent caused broker cancellation = %#v", orders.cancelled)
	}
}

func withWorkerCommand(
	command WorkerOrderCommand,
	update func(*WorkerOrderCommand),
) WorkerOrderCommand {
	update(&command)
	return command
}

type recordingDefinitionHistoryStore struct {
	fakeDesignStore
	summaries          []DefinitionVersionSummary
	version            DefinitionVersion
	listedDefinitionID string
	loadedDefinitionID string
	loadedVersion      string
}

func (store *recordingDefinitionHistoryStore) ListDefinitionVersions(
	definitionID string,
) ([]DefinitionVersionSummary, bool, error) {
	store.listedDefinitionID = definitionID
	return store.summaries, true, nil
}

func (store *recordingDefinitionHistoryStore) GetDefinitionVersion(
	definitionID string,
	version string,
) (DefinitionVersion, bool, error) {
	store.loadedDefinitionID = definitionID
	store.loadedVersion = version
	return store.version, true, nil
}

type controlledAtomicOrderExecutor struct {
	fakeWorkerOrderExecutor
	atomicErr      error
	returnedOrders int
}

func (executor *controlledAtomicOrderExecutor) SubmitAtomicPineOrders(
	_ context.Context,
	_ string,
	orders ...LiveAtomicOrder,
) (types.OrderSlice, error) {
	if executor.atomicErr != nil {
		return nil, executor.atomicErr
	}
	created := make(types.OrderSlice, 0, executor.returnedOrders)
	for index := 0; index < executor.returnedOrders && index < len(orders); index++ {
		created = append(created, types.Order{
			SubmitOrder: orders[index].Order,
			Status:      types.OrderStatusNew,
		})
	}
	return created, nil
}

func atomicLiveCommandExecutor(orders LiveOrderExecutor) *LiveCommandExecutor {
	return &LiveCommandExecutor{
		Symbol: "US.AAPL", OrderExecutor: orders,
		MarketResolver: fakeWorkerMarketResolver{"US.AAPL": testLiveCommandMarket()},
	}
}

type positionAwareCommandSizer struct {
	position fixedpoint.Value
}

func (sizer positionAwareCommandSizer) NetPosition() fixedpoint.Value {
	return sizer.position
}

func (sizer positionAwareCommandSizer) QuantityForCommand(
	WorkerOrderCommand,
	types.Market,
) (fixedpoint.Value, error) {
	return sizer.position.Abs(), nil
}
