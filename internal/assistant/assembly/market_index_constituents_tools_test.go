package assembly

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	assistanttestkit "github.com/jftrade/jftrade-main/internal/assistant/testkit"
	mdsrv "github.com/jftrade/jftrade-main/internal/marketdata"
)

func TestADKMarketIndexConstituentsToolForwardsNormalizedInputs(t *testing.T) {
	registry := assistanttestkit.NewToolRegistry()
	var gotMarket, gotSymbol string
	var gotLimit int
	RegisterJFTradeADKTools(nil, registry, ToolDeps{
		MarketIndexConstituents: func(_ context.Context, market, symbol string, limit int) (any, error) {
			gotMarket, gotSymbol, gotLimit = market, symbol, limit
			return map[string]any{
				"instrumentId": market + "." + symbol,
				"constituents": []any{map[string]any{"code": "600519", "name": "贵州茅台", "weight": nil}},
			}, nil
		},
	})

	tool := getAssemblyTool(t, registry, "market.index_constituents")
	if tool.Descriptor.Permission != "read_internal" || tool.Descriptor.RiskLevel != "low" ||
		len(tool.Descriptor.RequiresApprovalIn) != 0 {
		t.Fatalf("market.index_constituents descriptor = %#v", tool.Descriptor)
	}
	if len(tool.Descriptor.RequiredSkills) != 1 || tool.Descriptor.RequiredSkills[0] != "jftrade-market" {
		t.Fatalf("market.index_constituents required skills = %#v", tool.Descriptor.RequiredSkills)
	}
	if _, err := tool.Handler(t.Context(), map[string]any{}); err == nil {
		t.Fatal("market.index_constituents missing instrument error = nil")
	}
	if _, err := tool.Handler(t.Context(), map[string]any{"market": "SH", "symbol": "000300", "limit": 1001}); err == nil {
		t.Fatal("market.index_constituents limit range error = nil")
	}
	output, err := tool.Handler(t.Context(), map[string]any{"market": "sh", "symbol": "000300", "limit": "300"})
	if err != nil {
		t.Fatalf("market.index_constituents Handler: %v", err)
	}
	if gotMarket != "SH" || gotSymbol != "000300" || gotLimit != 300 {
		t.Fatalf("market.index_constituents args = %q %q %d", gotMarket, gotSymbol, gotLimit)
	}
	if output.(map[string]any)["instrumentId"] != "SH.000300" {
		t.Fatalf("market.index_constituents output = %#v", output)
	}
	if _, err := tool.Handler(t.Context(), map[string]any{"market": "SH", "symbol": "000300"}); err != nil {
		t.Fatalf("market.index_constituents default limit: %v", err)
	}
	if gotLimit != 200 {
		t.Fatalf("market.index_constituents default limit = %d", gotLimit)
	}
}

func TestADKMarketIndexConstituentsToolFailsClosedWithoutPort(t *testing.T) {
	registry := assistanttestkit.NewToolRegistry()
	RegisterJFTradeADKTools(nil, registry, ToolDeps{})
	tool := getAssemblyTool(t, registry, "market.index_constituents")
	if _, err := tool.Handler(t.Context(), map[string]any{"market": "SH", "symbol": "000300"}); err == nil {
		t.Fatal("market.index_constituents succeeded without owner port")
	}
}

func TestADKMarketIndexConstituentsToolSurfacesProviderCapabilityAsClearMessage(t *testing.T) {
	registry := assistanttestkit.NewToolRegistry()
	unsupported := fmt.Errorf(
		"%w: active provider %q does not support index constituents",
		mdsrv.ErrCapabilityUnsupported, "futu-opend",
	)
	RegisterJFTradeADKTools(nil, registry, ToolDeps{
		MarketIndexConstituents: func(context.Context, string, string, int) (any, error) {
			return nil, unsupported
		},
	})
	tool := getAssemblyTool(t, registry, "market.index_constituents")
	_, err := tool.Handler(t.Context(), map[string]any{"market": "SH", "symbol": "000300"})
	if !errors.Is(err, mdsrv.ErrCapabilityUnsupported) || !strings.Contains(err.Error(), "futu-opend") {
		t.Fatalf("market.index_constituents unsupported error = %v", err)
	}
}
