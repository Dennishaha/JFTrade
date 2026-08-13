package application

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

type platformBundle struct{}
type marketDataBundle struct{}
type tradingBundle struct{}
type strategyBacktestBundle struct{}
type assistantHTTPBundle struct{}

func TestInstallersExpressTypedDependencyOrder(t *testing.T) {
	var order []string
	installers := Installers[platformBundle, marketDataBundle, tradingBundle, strategyBacktestBundle, assistantHTTPBundle]{
		Platform: func() (platformBundle, error) {
			order = append(order, "platform")
			return platformBundle{}, nil
		},
		MarketData: func(platformBundle) (marketDataBundle, error) {
			order = append(order, "market-data")
			return marketDataBundle{}, nil
		},
		Trading: func(marketDataBundle) (tradingBundle, error) {
			order = append(order, "trading")
			return tradingBundle{}, nil
		},
		StrategyBacktest: func(tradingBundle) (strategyBacktestBundle, error) {
			order = append(order, "strategy-backtest")
			return strategyBacktestBundle{}, nil
		},
		AssistantHTTP: func(strategyBacktestBundle) (assistantHTTPBundle, error) {
			order = append(order, "assistant-http")
			return assistantHTTPBundle{}, nil
		},
	}
	if err := installers.Run(); err != nil {
		t.Fatal(err)
	}
	want := []string{"platform", "market-data", "trading", "strategy-backtest", "assistant-http"}
	if !reflect.DeepEqual(order, want) {
		t.Fatalf("installer order = %v, want %v", order, want)
	}
}

func TestInstallersRollbackPartialInitialization(t *testing.T) {
	resources := new(Resources)
	startupErr := errors.New("trading unavailable")
	var closed []string
	installers := Installers[platformBundle, marketDataBundle, tradingBundle, strategyBacktestBundle, assistantHTTPBundle]{
		Platform: func() (platformBundle, error) {
			_ = resources.Register("platform", func() error { closed = append(closed, "platform"); return nil })
			return platformBundle{}, nil
		},
		MarketData: func(platformBundle) (marketDataBundle, error) {
			_ = resources.Register("market data", func() error { closed = append(closed, "market-data"); return nil })
			return marketDataBundle{}, nil
		},
		Trading: func(marketDataBundle) (tradingBundle, error) { return tradingBundle{}, startupErr },
		StrategyBacktest: func(tradingBundle) (strategyBacktestBundle, error) {
			t.Fatal("strategy/backtest ran after trading failure")
			return strategyBacktestBundle{}, nil
		},
		AssistantHTTP: func(strategyBacktestBundle) (assistantHTTPBundle, error) {
			t.Fatal("assistant/http ran after trading failure")
			return assistantHTTPBundle{}, nil
		},
		Rollback: resources.Rollback,
	}
	err := installers.Run()
	if !errors.Is(err, startupErr) || !strings.Contains(err.Error(), "install trading") {
		t.Fatalf("installer error = %v", err)
	}
	if !reflect.DeepEqual(closed, []string{"market-data", "platform"}) {
		t.Fatalf("rollback order = %v", closed)
	}
}
