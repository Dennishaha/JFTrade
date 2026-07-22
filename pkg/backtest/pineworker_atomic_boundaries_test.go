package backtest

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jftrade/jftrade-main/pkg/bbgo/types"
)

type atomicResultBoundaryExecutor struct {
	fakeWorkerOrderExecutor
	created types.OrderSlice
	err     error
}

func (executor *atomicResultBoundaryExecutor) SubmitAtomicPineOrders(
	context.Context,
	string,
	...PineWorkerAtomicOrder,
) (types.OrderSlice, error) {
	return executor.created, executor.err
}

func TestPineWorkerAtomicGroupRejectsEveryUnsafeShape(t *testing.T) {
	validEntry := WorkerOrderCommand{Kind: "entry", ID: "parent", AtomicGroupID: "group"}
	validLimit := WorkerOrderCommand{
		Kind: "exit", ID: "limit", ParentID: "parent", AtomicGroupID: "group", OCOGroupID: "oco", ReduceOnly: true,
	}
	validStop := WorkerOrderCommand{
		Kind: "exit", ID: "stop", ParentID: "parent", AtomicGroupID: "group", OCOGroupID: "oco", ReduceOnly: true,
	}
	validPlans := []pineWorkerCommandPlan{
		{command: validEntry, order: types.SubmitOrder{Type: types.OrderTypeMarket}},
		{command: validLimit, order: types.SubmitOrder{Type: types.OrderTypeLimit}},
		{command: validStop, order: types.SubmitOrder{Type: types.OrderTypeStopMarket}},
	}
	tests := []struct {
		name     string
		commands []WorkerOrderCommand
		plans    []pineWorkerCommandPlan
		want     string
	}{
		{name: "single leg", commands: []WorkerOrderCommand{validEntry}, plans: validPlans[:1], want: "at least two"},
		{name: "reduce-only entry", commands: []WorkerOrderCommand{{Kind: "entry", ID: "parent", ReduceOnly: true}, validLimit}, plans: validPlans, want: "reduce-only entry"},
		{name: "entry without id", commands: []WorkerOrderCommand{{Kind: "entry"}, validLimit}, plans: validPlans, want: "entry without an id"},
		{name: "cancellation", commands: []WorkerOrderCommand{{Kind: "cancel", ID: "x"}, validLimit}, plans: validPlans, want: "cannot contain cancellation"},
		{name: "unsupported", commands: []WorkerOrderCommand{{Kind: "replace", ID: "x"}, validLimit}, plans: validPlans, want: "unsupported command"},
		{name: "entry as OCO child", commands: []WorkerOrderCommand{{Kind: "entry", ID: "parent", OCOGroupID: "oco"}, validLimit}, plans: validPlans, want: "cannot be an OCO child"},
		{name: "unplaceable leg", commands: []WorkerOrderCommand{validEntry, validLimit}, plans: []pineWorkerCommandPlan{{command: validEntry}, {command: validLimit, skip: true}}, want: "cannot be placed"},
		{name: "non-reduce exit", commands: []WorkerOrderCommand{validEntry, func() WorkerOrderCommand { next := validLimit; next.ReduceOnly = false; return next }()}, plans: []pineWorkerCommandPlan{{command: validEntry}, {command: func() WorkerOrderCommand { next := validLimit; next.ReduceOnly = false; return next }()}}, want: "non-reduce-only"},
		{name: "one OCO leg", commands: []WorkerOrderCommand{validEntry, validLimit}, plans: validPlans[:2], want: "exactly two"},
		{name: "OCO missing limit", commands: []WorkerOrderCommand{validEntry, validLimit, validStop}, plans: []pineWorkerCommandPlan{{command: validEntry}, {command: validLimit, order: types.SubmitOrder{Type: types.OrderTypeStopMarket}}, {command: validStop, order: types.SubmitOrder{Type: types.OrderTypeStopMarket}}}, want: "one limit leg"},
		{name: "OCO missing stop", commands: []WorkerOrderCommand{validEntry, validLimit, validStop}, plans: []pineWorkerCommandPlan{{command: validEntry}, {command: validLimit, order: types.SubmitOrder{Type: types.OrderTypeLimit}}, {command: validStop, order: types.SubmitOrder{Type: types.OrderTypeLimit}}}, want: "one stop leg"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := validatePineWorkerAtomicGroup("group", test.commands, test.plans)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("validation error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestPineWorkerAtomicExecutionFailureBoundaries(t *testing.T) {
	plans := []pineWorkerCommandPlan{
		{command: WorkerOrderCommand{Kind: "entry", ID: "parent", AtomicGroupID: "group"}, order: types.SubmitOrder{ClientOrderID: "parent"}},
		{command: WorkerOrderCommand{Kind: "entry", ID: "other", AtomicGroupID: "other"}, order: types.SubmitOrder{ClientOrderID: "other"}},
	}
	ordinary := validPineWorkerCommandExecutor(&fakeWorkerOrderExecutor{})
	if err := ordinary.executeAtomicGroup(t.Context(), "group", plans); err == nil || !strings.Contains(err.Error(), "atomic placement capability") {
		t.Fatalf("ordinary atomic execution error = %v", err)
	}
	forced := errors.New("atomic submit failed")
	failing := &PineWorkerCommandExecutor{OrderExecutor: &atomicResultBoundaryExecutor{err: forced}}
	if err := failing.executeAtomicGroup(t.Context(), "group", plans); !errors.Is(err, forced) {
		t.Fatalf("atomic submit error = %v", err)
	}
	mismatched := &PineWorkerCommandExecutor{OrderExecutor: &atomicResultBoundaryExecutor{created: types.OrderSlice{}}}
	if err := mismatched.executeAtomicGroup(t.Context(), "group", plans); err == nil || !strings.Contains(err.Error(), "returned 0 orders for 1") {
		t.Fatalf("atomic result count error = %v", err)
	}
	if err := ordinary.Execute(t.Context(), WorkerOrderCommand{Kind: "entry", ID: "parent", AtomicGroupID: "group"}); err == nil || !strings.Contains(err.Error(), "complete bar command group") {
		t.Fatalf("single atomic command error = %v", err)
	}
	if err := ordinary.executePlanned(t.Context(), pineWorkerCommandPlan{command: WorkerOrderCommand{Kind: "replace"}}); err == nil || !strings.Contains(err.Error(), "unsupported") {
		t.Fatalf("unsupported planned command error = %v", err)
	}
	if err := ordinary.ExecuteBarCommands(t.Context(), []WorkerOrderCommand{{Kind: "replace"}}); err == nil || !strings.Contains(err.Error(), "unsupported") {
		t.Fatalf("unsupported bar command error = %v", err)
	}

	atomicFailure := &fakeAtomicWorkerOrderExecutor{}
	atomicFailure.submitErr = forced
	atomicExecutor := &PineWorkerCommandExecutor{
		Symbol: "US.AAPL", OrderExecutor: atomicFailure,
		MarketResolver: fakeWorkerMarketResolver{"US.AAPL": testPineWorkerShortReplayMarket()},
	}
	if err := atomicExecutor.ExecuteBarCommands(t.Context(), pineWorkerAtomicBracketCommands()); !errors.Is(err, forced) {
		t.Fatalf("atomic bar submission error = %v", err)
	}

	ordinary.activeOrderAliases = map[string][]string{"duplicate": {"order", "order"}, "missing": {"absent"}}
	ordinary.activeOrders = map[string]types.Order{"order": {SubmitOrder: types.SubmitOrder{ClientOrderID: "order"}}}
	if err := ordinary.Execute(t.Context(), WorkerOrderCommand{Kind: "cancel", ID: "duplicate"}); err != nil {
		t.Fatalf("deduplicated cancel: %v", err)
	}
	if err := ordinary.Execute(t.Context(), WorkerOrderCommand{Kind: "cancel", ID: "missing"}); err != nil {
		t.Fatalf("missing aliased order cancel: %v", err)
	}
}
