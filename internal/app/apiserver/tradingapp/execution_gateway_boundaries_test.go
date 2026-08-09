package tradingapp

import (
	"strings"
	"testing"

	"github.com/jftrade/jftrade-main/pkg/broker"
)

func TestComboOrderQuantityModeMapsEventParlaysToAmount(t *testing.T) {
	if got := ComboOrderQuantityMode(broker.OrderKindEventParlay); got != broker.QuantityModeAmount {
		t.Fatalf("event parlay quantity mode = %q", got)
	}
	if got := ComboOrderQuantityMode(broker.OrderKindOptionCombo); got != broker.QuantityModeContracts {
		t.Fatalf("option combo quantity mode = %q", got)
	}
}

func TestNormalizedBrokerComboIntentKeepsClientOrderIdentity(t *testing.T) {
	got := NormalizedBrokerComboIntent(broker.ComboOrderIntent{ClientOrderID: "client"})
	if !strings.Contains(got, "client") {
		t.Fatalf("normalized broker combo = %s", got)
	}
}
