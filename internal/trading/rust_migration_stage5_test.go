package trading

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/jftrade/jftrade-main/pkg/broker"
)

type stage5Decimal float64

func (value *stage5Decimal) UnmarshalJSON(data []byte) error {
	text := string(bytes.Trim(data, `"`))
	parsed, err := strconv.ParseFloat(text, 64)
	if err != nil {
		return err
	}
	*value = stage5Decimal(parsed)
	return nil
}

type stage5TradingCorpus struct {
	Version           string                  `json:"version"`
	RiskConfig        stage5RiskConfig        `json:"riskConfig"`
	StatusCases       []string                `json:"statusCases"`
	Transitions       []stage5TransitionCase  `json:"transitions"`
	Commands          []stage5OrderCommand    `json:"commands"`
	PositionRefreshes []stage5PositionRefresh `json:"positionRefreshes"`
}

type stage5RiskConfig struct {
	RealTradingEnabled bool             `json:"realTradingEnabled"`
	KillSwitchActive   bool             `json:"killSwitchActive"`
	MaxOrderQuantity   *stage5Decimal   `json:"maxOrderQuantity"`
	MaxOrderNotional   *stage5Decimal   `json:"maxOrderNotional"`
	HardStops          []stage5HardStop `json:"hardStops"`
}

type stage5HardStop struct {
	BrokerID  string  `json:"brokerId"`
	AccountID string  `json:"accountId"`
	Market    *string `json:"market"`
	Symbol    *string `json:"symbol"`
}

type stage5TransitionCase struct {
	Current  string `json:"current"`
	Incoming string `json:"incoming"`
}

type stage5OrderCommand struct {
	IdempotencyKey string         `json:"idempotencyKey"`
	BrokerID       string         `json:"brokerId"`
	AccountID      string         `json:"accountId"`
	Environment    string         `json:"environment"`
	Market         string         `json:"market"`
	Symbol         string         `json:"symbol"`
	Side           string         `json:"side"`
	Quantity       stage5Decimal  `json:"quantity"`
	Price          *stage5Decimal `json:"price"`
	ClientOrderID  string         `json:"clientOrderId"`
}

type stage5PositionRefresh struct {
	RefreshID  string `json:"refreshId"`
	AccountID  string `json:"accountId"`
	Generation uint64 `json:"generation"`
	Sequence   uint64 `json:"sequence"`
	Positions  []struct {
		AccountID        string        `json:"accountId"`
		Market           string        `json:"market"`
		Symbol           string        `json:"symbol"`
		Quantity         stage5Decimal `json:"quantity"`
		SellableQuantity stage5Decimal `json:"sellableQuantity"`
		LastPrice        stage5Decimal `json:"lastPrice"`
	} `json:"positions"`
}

type stage5TradingExpected struct {
	Version  string `json:"version"`
	Statuses []struct {
		Raw    string `json:"raw"`
		Status string `json:"status"`
	} `json:"statuses"`
	Transitions []struct {
		Current  string `json:"current"`
		Incoming string `json:"incoming"`
		Status   string `json:"status"`
		Accepted bool   `json:"accepted"`
	} `json:"transitions"`
	Commands []struct {
		OK    bool `json:"ok"`
		Value struct {
			Accepted       bool    `json:"accepted"`
			Dispatch       bool    `json:"dispatch"`
			IdempotencyKey string  `json:"idempotencyKey"`
			ReasonCode     *string `json:"reasonCode"`
			Replayed       bool    `json:"replayed"`
		} `json:"value"`
	} `json:"commands"`
	PositionRefreshes []struct {
		OK    bool   `json:"ok"`
		Error string `json:"error"`
		Value string `json:"value"`
	} `json:"positionRefreshes"`
	Positions []struct {
		AccountID        string        `json:"accountId"`
		Market           string        `json:"market"`
		Symbol           string        `json:"symbol"`
		Quantity         stage5Decimal `json:"quantity"`
		SellableQuantity stage5Decimal `json:"sellableQuantity"`
		LastPrice        stage5Decimal `json:"lastPrice"`
	} `json:"positions"`
}

func TestRustMigrationStage5TradingStatusAndRiskMatchCorpus(t *testing.T) {
	var corpus stage5TradingCorpus
	var expected stage5TradingExpected
	readStage5TradingFixture(t, "trading-strategy-corpus.json", &corpus)
	readStage5TradingFixture(t, "trading-strategy-corpus.expected.json", &expected)
	if corpus.Version != "stage5.v1" || expected.Version != corpus.Version {
		t.Fatalf("stage 5 versions = corpus %q expected %q", corpus.Version, expected.Version)
	}
	if len(corpus.StatusCases) != len(expected.Statuses) {
		t.Fatalf("status cases = %d, expected %d", len(corpus.StatusCases), len(expected.Statuses))
	}
	for index, raw := range corpus.StatusCases {
		got := CanonicalBrokerOrderStatus(raw)
		want := expected.Statuses[index]
		if want.Raw != raw || want.Status != got {
			t.Fatalf("status[%d] = %q => %q, want %#v", index, raw, got, want)
		}
	}
	for index, transition := range corpus.Transitions {
		got, accepted := ReconcileCanonicalOrderStatus(transition.Current, transition.Incoming)
		want := expected.Transitions[index]
		if got != want.Status || accepted != want.Accepted {
			t.Fatalf("transition[%d] = (%q, %v), want (%q, %v)", index, got, accepted, want.Status, want.Accepted)
		}
	}

	gateway := NewStaticPreTradeRiskGateway(func() PreTradeRiskConfig {
		return stage5GoRiskConfig(corpus.RiskConfig)
	})
	if len(corpus.Commands) != len(expected.Commands) {
		t.Fatalf("commands = %d, expected %d", len(corpus.Commands), len(expected.Commands))
	}
	for index, input := range corpus.Commands {
		price := stage5FloatPointer(input.Price)
		decision := gateway.EvaluatePlaceOrder(t.Context(), ExecutionOrderCommand{
			BrokerID: input.BrokerID,
			Symbol:   input.Symbol,
			Side:     input.Side,
			Query: broker.PlaceOrderQuery{
				ReadQuery: broker.ReadQuery{
					BrokerID: input.BrokerID, AccountID: input.AccountID,
					TradingEnvironment: input.Environment, Market: input.Market,
				},
				Symbol: input.Symbol, Side: input.Side, Quantity: float64(input.Quantity),
				Price: price, ClientOrderID: input.ClientOrderID,
			},
		})
		want := expected.Commands[index].Value
		if !expected.Commands[index].OK || want.Dispatch || want.IdempotencyKey != input.IdempotencyKey {
			t.Fatalf("invalid shadow command contract[%d] = %#v", index, expected.Commands[index])
		}
		if decision.Allows() != want.Accepted || !stage5OptionalStringEqual(want.ReasonCode, decision.ReasonCode) {
			t.Fatalf("risk[%d] = allow %v code %q, want allow %v code %v", index, decision.Allows(), decision.ReasonCode, want.Accepted, want.ReasonCode)
		}
	}
	if !expected.Commands[len(expected.Commands)-1].Value.Replayed {
		t.Fatal("duplicate command is not pinned as an idempotent replay")
	}
	assertStage5PositionRefreshContract(t, corpus.PositionRefreshes, expected)
}

func assertStage5PositionRefreshContract(t *testing.T, refreshes []stage5PositionRefresh, expected stage5TradingExpected) {
	t.Helper()
	if len(refreshes) != len(expected.PositionRefreshes) {
		t.Fatalf("position refreshes = %d, expected %d", len(refreshes), len(expected.PositionRefreshes))
	}
	seen := map[string]bool{}
	var generation, sequence uint64
	var positions []broker.PositionSnapshot
	for index, refresh := range refreshes {
		want := expected.PositionRefreshes[index]
		if seen[refresh.RefreshID] {
			if !want.OK || want.Value != "duplicate" {
				t.Fatalf("duplicate position refresh[%d] = %#v", index, want)
			}
			continue
		}
		if refresh.Generation < generation || refresh.Generation == generation && refresh.Sequence <= sequence {
			if !want.OK || want.Value != "stale" {
				t.Fatalf("stale position refresh[%d] = %#v", index, want)
			}
			continue
		}
		candidate := make([]broker.PositionSnapshot, 0, len(refresh.Positions))
		valid := true
		for _, position := range refresh.Positions {
			if position.AccountID != refresh.AccountID || strings.TrimSpace(position.Market) == "" || strings.TrimSpace(position.Symbol) == "" {
				valid = false
				break
			}
			candidate = append(candidate, broker.PositionSnapshot{
				AccountID: position.AccountID, Market: position.Market, Symbol: position.Symbol,
				Quantity: float64(position.Quantity), SellableQuantity: float64(position.SellableQuantity),
				LastPrice: float64(position.LastPrice),
			})
		}
		if !valid {
			if want.OK || want.Error == "" {
				t.Fatalf("invalid position refresh[%d] is not rejected: %#v", index, want)
			}
			continue
		}
		if !want.OK || want.Value != "applied" {
			t.Fatalf("position refresh[%d] = %#v, want applied", index, want)
		}
		seen[refresh.RefreshID] = true
		generation, sequence, positions = refresh.Generation, refresh.Sequence, candidate
	}
	if len(positions) != len(expected.Positions) {
		t.Fatalf("final positions = %d, expected %d", len(positions), len(expected.Positions))
	}
	for index, position := range positions {
		want := expected.Positions[index]
		if position.AccountID != want.AccountID || position.Market != want.Market || position.Symbol != want.Symbol ||
			position.Quantity != float64(want.Quantity) || position.SellableQuantity != float64(want.SellableQuantity) || position.LastPrice != float64(want.LastPrice) {
			t.Fatalf("position[%d] = %#v, want %#v", index, position, want)
		}
	}
}

func stage5GoRiskConfig(input stage5RiskConfig) PreTradeRiskConfig {
	hardStops := make([]RealTradeHardStopEntry, 0, len(input.HardStops))
	for _, hardStop := range input.HardStops {
		hardStops = append(hardStops, RealTradeHardStopEntry{
			BrokerID: hardStop.BrokerID, AccountID: hardStop.AccountID,
			TradingEnvironment: "REAL", Market: hardStop.Market, Symbol: hardStop.Symbol,
		})
	}
	return PreTradeRiskConfig{
		RealTradingEnabled:      input.RealTradingEnabled,
		RuntimeKillSwitch:       input.KillSwitchActive,
		RuntimeMaxOrderQty:      stage5FloatPointer(input.MaxOrderQuantity),
		RuntimeMaxOrderNotional: stage5FloatPointer(input.MaxOrderNotional),
		RuntimeHardStops:        hardStops,
	}
}

func stage5FloatPointer(value *stage5Decimal) *float64 {
	if value == nil {
		return nil
	}
	converted := float64(*value)
	return &converted
}

func stage5OptionalStringEqual(expected *string, actual string) bool {
	if expected == nil {
		return actual == ""
	}
	return *expected == actual
}

func readStage5TradingFixture(t *testing.T, name string, target any) {
	t.Helper()
	directory := os.Getenv("JFTRADE_STAGE5_FIXTURE_ROOT")
	if directory == "" {
		_, source, _, ok := runtime.Caller(0)
		if !ok {
			t.Fatal("resolve stage 5 trading test source")
		}
		directory = filepath.Join(filepath.Dir(source), "..", "..", "tests", "fixtures", "rust-migration", "stage5")
	}
	content, err := os.ReadFile(filepath.Join(directory, name))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(content, target); err != nil {
		t.Fatal(err)
	}
}
