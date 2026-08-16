package assembly

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	assistanttestkit "github.com/jftrade/jftrade-main/internal/assistant/testkit"
	mdsrv "github.com/jftrade/jftrade-main/internal/marketdata"
)

func TestADKMarketNewsAndCorporateActionsToolsForwardNormalizedInputs(t *testing.T) {
	registry := assistanttestkit.NewToolRegistry()
	var newsMarket, newsSymbol string
	var newsLimit int
	var actionsMarket, actionsSymbol string
	var actionsFrom, actionsTo time.Time
	RegisterJFTradeADKTools(nil, registry, ToolDeps{
		MarketNews: func(_ context.Context, market, symbol string, limit int) (any, error) {
			newsMarket, newsSymbol, newsLimit = market, symbol, limit
			return map[string]any{"instrumentId": market + "." + symbol, "entries": []any{}}, nil
		},
		MarketCorporateActions: func(_ context.Context, market, symbol string, from, to time.Time) (any, error) {
			actionsMarket, actionsSymbol, actionsFrom, actionsTo = market, symbol, from, to
			return map[string]any{"instrumentId": market + "." + symbol, "events": []any{}}, nil
		},
	})

	newsTool := getAssemblyTool(t, registry, "market.news")
	if newsTool.Descriptor.Permission != "read_internal" || newsTool.Descriptor.RiskLevel != "low" ||
		len(newsTool.Descriptor.RequiresApprovalIn) != 0 {
		t.Fatalf("market.news descriptor = %#v", newsTool.Descriptor)
	}
	if _, err := newsTool.Handler(t.Context(), map[string]any{}); err == nil {
		t.Fatal("market.news missing instrument error = nil")
	}
	if _, err := newsTool.Handler(t.Context(), map[string]any{"market": "US", "symbol": "AAPL", "limit": 51}); err == nil {
		t.Fatal("market.news limit range error = nil")
	}
	output, err := newsTool.Handler(t.Context(), map[string]any{"query": "check us.msft headlines", "limit": "5"})
	if err != nil {
		t.Fatalf("market.news Handler: %v", err)
	}
	if newsMarket != "US" || newsSymbol != "MSFT" || newsLimit != 5 {
		t.Fatalf("market.news args = %q %q %d", newsMarket, newsSymbol, newsLimit)
	}
	if output.(map[string]any)["instrumentId"] != "US.MSFT" {
		t.Fatalf("market.news output = %#v", output)
	}
	if _, err := newsTool.Handler(t.Context(), map[string]any{"market": "US", "symbol": "AAPL"}); err != nil {
		t.Fatalf("market.news default limit: %v", err)
	}
	if newsLimit != 10 {
		t.Fatalf("market.news default limit = %d", newsLimit)
	}

	actionsTool := getAssemblyTool(t, registry, "market.corporate_actions")
	if actionsTool.Descriptor.Permission != "read_internal" || actionsTool.Descriptor.RiskLevel != "low" ||
		len(actionsTool.Descriptor.RequiresApprovalIn) != 0 {
		t.Fatalf("market.corporate_actions descriptor = %#v", actionsTool.Descriptor)
	}
	if _, err := actionsTool.Handler(t.Context(), map[string]any{"market": "US", "symbol": "AAPL", "from": "yesterday"}); err == nil {
		t.Fatal("market.corporate_actions invalid from error = nil")
	}
	if _, err := actionsTool.Handler(t.Context(), map[string]any{
		"market": "US", "symbol": "AAPL",
		"from": "2026-01-01T00:00:00Z", "to": "2025-01-01T00:00:00Z",
	}); err == nil {
		t.Fatal("market.corporate_actions reversed range error = nil")
	}
	if _, err := actionsTool.Handler(t.Context(), map[string]any{
		"market": "hk", "symbol": "00700",
		"from": "2025-01-01T08:00:00+08:00", "to": "2026-01-01T00:00:00Z",
	}); err != nil {
		t.Fatalf("market.corporate_actions Handler: %v", err)
	}
	if actionsMarket != "HK" || actionsSymbol != "00700" ||
		!actionsFrom.Equal(time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)) ||
		!actionsTo.Equal(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("market.corporate_actions args = %q %q %v %v", actionsMarket, actionsSymbol, actionsFrom, actionsTo)
	}
	for _, name := range []string{"market.news", "market.corporate_actions"} {
		tool := getAssemblyTool(t, registry, name)
		if len(tool.Descriptor.RequiredSkills) != 1 || tool.Descriptor.RequiredSkills[0] != "jftrade-market" {
			t.Fatalf("%s required skills = %#v", name, tool.Descriptor.RequiredSkills)
		}
	}
}

func TestADKMarketNewsAndCorporateActionsToolsFailClosedWithoutPorts(t *testing.T) {
	registry := assistanttestkit.NewToolRegistry()
	RegisterJFTradeADKTools(nil, registry, ToolDeps{})
	for _, name := range []string{"market.news", "market.corporate_actions"} {
		tool := getAssemblyTool(t, registry, name)
		if _, err := tool.Handler(t.Context(), map[string]any{"market": "US", "symbol": "AAPL"}); err == nil {
			t.Fatalf("%s succeeded without owner port", name)
		}
	}
}

func TestADKMarketNewsToolSurfacesProviderCapabilityAsClearMessage(t *testing.T) {
	registry := assistanttestkit.NewToolRegistry()
	unsupported := fmt.Errorf(
		"%w: active provider %q does not support instrument news",
		mdsrv.ErrCapabilityUnsupported, "futu-opend",
	)
	RegisterJFTradeADKTools(nil, registry, ToolDeps{
		MarketNews: func(context.Context, string, string, int) (any, error) { return nil, unsupported },
		MarketCorporateActions: func(context.Context, string, string, time.Time, time.Time) (any, error) {
			return nil, fmt.Errorf(
				"%w: active provider %q does not support corporate actions",
				mdsrv.ErrCapabilityUnsupported, "futu-opend",
			)
		},
	})
	newsTool := getAssemblyTool(t, registry, "market.news")
	_, err := newsTool.Handler(t.Context(), map[string]any{"market": "US", "symbol": "AAPL"})
	if !errors.Is(err, mdsrv.ErrCapabilityUnsupported) || !strings.Contains(err.Error(), "futu-opend") {
		t.Fatalf("market.news unsupported error = %v", err)
	}
	actionsTool := getAssemblyTool(t, registry, "market.corporate_actions")
	_, err = actionsTool.Handler(t.Context(), map[string]any{"market": "US", "symbol": "AAPL"})
	if !errors.Is(err, mdsrv.ErrCapabilityUnsupported) || !strings.Contains(err.Error(), "corporate actions") {
		t.Fatalf("market.corporate_actions unsupported error = %v", err)
	}
}
