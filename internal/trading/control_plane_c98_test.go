package trading

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jftrade/jftrade-main/pkg/broker"
)

func TestCoverage98ControlPlaneRetainsActivationAndBoundsRepeatedAuditEvents(t *testing.T) {
	plane, err := NewRealTradeControlPlane("")
	if err != nil {
		t.Fatalf("NewRealTradeControlPlane: %v", err)
	}

	if _, err := plane.ActivateKillSwitch(t.Context(), RealTradeKillSwitchCommand{
		TradingEnvironment: "real",
		OperatorID:         "first-operator",
		Reason:             "first incident",
	}); err != nil {
		t.Fatalf("initial ActivateKillSwitch: %v", err)
	}
	initialKillSwitchActivation := plane.state.KillSwitch.ActivatedAt
	if initialKillSwitchActivation == "" {
		t.Fatal("initial kill-switch activation time is empty")
	}
	if _, err := plane.ActivateKillSwitch(t.Context(), RealTradeKillSwitchCommand{
		OperatorID: "second-operator",
		Reason:     "incident remains active",
	}); err != nil {
		t.Fatalf("repeated ActivateKillSwitch: %v", err)
	}
	if got := plane.state.KillSwitch; got.ActivatedAt != initialKillSwitchActivation || got.OperatorID != "second-operator" {
		t.Fatalf("repeated kill-switch activation = %#v, want original activation with refreshed operator", got)
	}

	maxQuantity := 10.0
	if _, err := plane.UpdateRuntimeRiskConfig(t.Context(), RealTradeRuntimeRiskCommand{
		RealTradingEnabled: true,
		MaxOrderQuantity:   &maxQuantity,
		OperatorID:         "first-operator",
	}); err != nil {
		t.Fatalf("initial UpdateRuntimeRiskConfig: %v", err)
	}
	initialRiskActivation := plane.state.RiskConfig.ActivatedAt
	maxNotional := 1000.0
	if _, err := plane.UpdateRuntimeRiskConfig(t.Context(), RealTradeRuntimeRiskCommand{
		RealTradingEnabled: true,
		MaxOrderNotional:   &maxNotional,
		OperatorID:         "second-operator",
	}); err != nil {
		t.Fatalf("repeated UpdateRuntimeRiskConfig: %v", err)
	}
	if got := plane.state.RiskConfig; got.ActivatedAt != initialRiskActivation || got.MaxOrderNotional == nil || *got.MaxOrderNotional != maxNotional {
		t.Fatalf("repeated runtime-risk activation = %#v, want original activation with new limit", got)
	}

	for index := 0; index <= realTradeControlEventLimit; index++ {
		if _, err := plane.ActivateKillSwitch(t.Context(), RealTradeKillSwitchCommand{Reason: "operator reconfirmed incident"}); err != nil {
			t.Fatalf("reconfirm kill switch #%d: %v", index, err)
		}
	}
	events := plane.Snapshot()["killSwitchEvents"].([]RealTradeControlEvent)
	if len(events) != realTradeControlEventLimit {
		t.Fatalf("bounded kill-switch events = %d, want %d", len(events), realTradeControlEventLimit)
	}
	if events[0].Action != "KILL_SWITCH_ACTIVATE" || events[0].Reason == nil || *events[0].Reason != "operator reconfirmed incident" {
		t.Fatalf("newest bounded kill-switch event = %#v", events[0])
	}
}

func TestCoverage98ControlPlaneTreatsEmptyStateAsFreshAndRejectsUnavailableMutations(t *testing.T) {
	emptyStatePath := filepath.Join(t.TempDir(), "real-trade-control.json")
	if err := os.WriteFile(emptyStatePath, []byte(" \n\t "), 0o600); err != nil {
		t.Fatalf("write blank persisted state: %v", err)
	}
	emptyPlane, err := NewRealTradeControlPlane(emptyStatePath)
	if err != nil {
		t.Fatalf("NewRealTradeControlPlane for blank state: %v", err)
	}
	if snapshot := emptyPlane.Snapshot(); snapshot["controlPlaneAvailable"] != true || snapshot["killSwitchActive"] != false {
		t.Fatalf("blank state snapshot = %#v", snapshot)
	}

	unavailablePath := t.TempDir()
	unavailable, err := NewRealTradeControlPlane(unavailablePath)
	if err == nil || unavailable == nil {
		t.Fatalf("NewRealTradeControlPlane(directory) = (%#v, %v), want unavailable plane and load error", unavailable, err)
	}
	maxQuantity := 1.0
	mutations := []struct {
		name string
		call func() error
	}{
		{
			name: "activate kill switch",
			call: func() error {
				_, err := unavailable.ActivateKillSwitch(context.Background(), RealTradeKillSwitchCommand{})
				return err
			},
		},
		{
			name: "release kill switch",
			call: func() error {
				_, err := unavailable.ReleaseKillSwitch(context.Background(), RealTradeKillSwitchCommand{})
				return err
			},
		},
		{
			name: "update risk configuration",
			call: func() error {
				_, err := unavailable.UpdateRuntimeRiskConfig(context.Background(), RealTradeRuntimeRiskCommand{RealTradingEnabled: true, MaxOrderQuantity: &maxQuantity})
				return err
			},
		},
		{
			name: "disable risk configuration",
			call: func() error {
				_, err := unavailable.DisableRuntimeRiskConfig(context.Background(), RealTradeRuntimeRiskCommand{})
				return err
			},
		},
		{
			name: "activate hard stop",
			call: func() error {
				_, err := unavailable.ActivateHardStop(context.Background(), RealTradeHardStopCommand{})
				return err
			},
		},
		{
			name: "release hard stop",
			call: func() error {
				_, err := unavailable.ReleaseHardStop(context.Background(), "missing", RealTradeHardStopCommand{})
				return err
			},
		},
	}
	for _, mutation := range mutations {
		if err := mutation.call(); err == nil || !strings.Contains(err.Error(), "unavailable") {
			t.Fatalf("%s error = %v, want unavailable control-plane error", mutation.name, err)
		}
	}

	invalidNotional := -1.0
	if _, err := emptyPlane.UpdateRuntimeRiskConfig(t.Context(), RealTradeRuntimeRiskCommand{MaxOrderNotional: &invalidNotional}); err == nil || !strings.Contains(err.Error(), "maxOrderNotional") {
		t.Fatalf("invalid notional error = %v, want maxOrderNotional validation", err)
	}
	if _, err := emptyPlane.ReleaseHardStop(t.Context(), "not-present", RealTradeHardStopCommand{}); err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("missing hard-stop release error = %v, want not-found error", err)
	}

	price := 10.0
	decision := emptyPlane.EvaluatePlaceOrder(t.Context(), ExecutionOrderCommand{Query: broker.PlaceOrderQuery{
		ReadQuery: broker.ReadQuery{TradingEnvironment: "PAPER"},
		Quantity:  1,
		Price:     &price,
	}})
	if !decision.Allows() {
		t.Fatalf("non-real order decision = %#v, want allow without real-trade audit side effect", decision)
	}
}

func TestCoverage98ControlPlaneKeepsStateWhenAtomicPersistenceCannotComplete(t *testing.T) {
	writeBlockedPlane, err := NewRealTradeControlPlane("")
	if err != nil {
		t.Fatalf("NewRealTradeControlPlane: %v", err)
	}
	writeBlockedPlane.path = filepath.Join(t.TempDir(), "real-trade-control.json")
	if err := os.Mkdir(writeBlockedPlane.path+".tmp", 0o700); err != nil {
		t.Fatalf("create stale atomic-write directory: %v", err)
	}
	if _, err := writeBlockedPlane.ActivateKillSwitch(t.Context(), RealTradeKillSwitchCommand{Reason: "storage incident"}); err == nil || !strings.Contains(err.Error(), "write real-trade control state") {
		t.Fatalf("write-blocked ActivateKillSwitch error = %v", err)
	}
	if writeBlockedPlane.state.KillSwitch != nil || len(writeBlockedPlane.state.Events) != 0 {
		t.Fatalf("failed kill-switch persistence changed in-memory state: %#v", writeBlockedPlane.state)
	}

	renameBlockedPlane, err := NewRealTradeControlPlane("")
	if err != nil {
		t.Fatalf("NewRealTradeControlPlane: %v", err)
	}
	renameBlockedPlane.path = filepath.Join(t.TempDir(), "real-trade-control.json")
	if err := os.Mkdir(renameBlockedPlane.path, 0o700); err != nil {
		t.Fatalf("create replacement target directory: %v", err)
	}
	if _, err := renameBlockedPlane.ActivateHardStop(t.Context(), RealTradeHardStopCommand{AccountID: "ACC-1"}); err == nil || !strings.Contains(err.Error(), "replace real-trade control state") {
		t.Fatalf("rename-blocked ActivateHardStop error = %v", err)
	}
	if len(renameBlockedPlane.state.HardStops) != 0 || len(renameBlockedPlane.state.Events) != 0 {
		t.Fatalf("failed hard-stop persistence changed in-memory state: %#v", renameBlockedPlane.state)
	}

	plane, err := NewRealTradeControlPlane("")
	if err != nil {
		t.Fatalf("NewRealTradeControlPlane: %v", err)
	}
	marketSnapshot, err := plane.ActivateHardStop(t.Context(), RealTradeHardStopCommand{Market: "us"})
	if err != nil {
		t.Fatalf("ActivateHardStop market scope: %v", err)
	}
	marketStop := marketSnapshot["hardStopEntries"].([]RealTradeHardStopEntry)[0]
	if marketStop.AccountID != "*" || marketStop.HardStopScope != "MARKET" {
		t.Fatalf("market hard stop normalization = %#v", marketStop)
	}
	accountSnapshot, err := plane.ActivateHardStop(t.Context(), RealTradeHardStopCommand{AccountID: "ACC-1", HardStopScope: "account"})
	if err != nil {
		t.Fatalf("ActivateHardStop account scope: %v", err)
	}
	entries := accountSnapshot["hardStopEntries"].([]RealTradeHardStopEntry)
	if len(entries) != 2 || entries[1].HardStopScope != "ACCOUNT" {
		t.Fatalf("active hard-stop entries = %#v", entries)
	}
	if _, err := plane.ReleaseHardStop(t.Context(), marketStop.ID, RealTradeHardStopCommand{OperatorID: "tester", Reason: "market resumed"}); err != nil {
		t.Fatalf("ReleaseHardStop market scope: %v", err)
	}
	remaining := plane.Snapshot()["hardStopEntries"].([]RealTradeHardStopEntry)
	if len(remaining) != 1 || remaining[0].ID != entries[1].ID {
		t.Fatalf("remaining hard stops after release = %#v", remaining)
	}
}
