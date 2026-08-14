package assembly

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	jfadkruntime "github.com/jftrade/jftrade-main/internal/assistant/engine/workflowruntime"
	trdsrv "github.com/jftrade/jftrade-main/internal/trading"
	"github.com/jftrade/jftrade-main/pkg/broker"
)

func TestPortfolioSummaryScansAllRealAccountsAndRanksNonEmptyFirst(t *testing.T) {
	accounts := []BrokerAccountView{
		{AccountID: "REAL-9985", TradingEnvironment: "REAL", MarketAuthorities: []string{"HK"}},
		{AccountID: "REAL-7281", TradingEnvironment: "REAL", MarketAuthorities: []string{"US"}},
		{AccountID: "REAL-8240", TradingEnvironment: "REAL", MarketAuthorities: []string{"US"}},
		{AccountID: "SIM-8240", TradingEnvironment: "SIMULATE", MarketAuthorities: []string{"US"}},
	}
	registry := newPortfolioTestRegistry(ToolDeps{
		ManagedAccounts:    func() any { return []string{"settings-only"} },
		BrokerEnabled:      func() bool { return true },
		DefaultTradeMarket: func() string { return "HK" },
		BrokerRuntime: func(context.Context) (BrokerRuntimeView, error) {
			return BrokerRuntimeView{Connectivity: "connected", Accounts: accounts}, nil
		},
		BrokerAccountRead: func(_ context.Context, query broker.ReadQuery, _ time.Duration) BrokerAccountReadResult {
			if query.AccountID != "REAL-8240" {
				return BrokerAccountReadResult{Funds: accountPayload(query), Positions: []any{}, Errors: []string{}}
			}
			return BrokerAccountReadResult{
				Funds: accountPayload(query), Positions: []map[string]any{{"accountId": query.AccountID, "symbol": "AAPL"}},
				PositionCount: 1, HasAssetsOrPositions: true, Errors: []string{},
			}
		},
		ExecutionOrders: func(_ context.Context, input BrokerReadInput) (any, int, error) {
			if input.AccountID != "REAL-8240" {
				return []any{}, 0, nil
			}
			return []map[string]any{{"accountId": input.AccountID}, {"accountId": input.AccountID}}, 2, nil
		},
	})

	tool, _ := registry.Get("portfolio.summary")
	output, err := tool.Handler(t.Context(), map[string]any{"tradingEnvironment": "REAL"})
	if err != nil {
		t.Fatalf("portfolio.summary: %v", err)
	}
	payload := output.(map[string]any)
	summaries := payload["accountSummaries"].([]portfolioAccountSummary)
	if len(summaries) != 3 || summaries[0].Account.AccountID != "REAL-8240" {
		t.Fatalf("account summaries = %#v, want all REAL accounts with REAL-8240 first", summaries)
	}
	if summaries[0].PositionCount != 1 || summaries[0].OrderCount != 2 || summaries[0].Partial {
		t.Fatalf("non-empty account summary = %#v", summaries[0])
	}
	queryMarkets := map[string]string{}
	for _, summary := range summaries {
		queryMarkets[summary.Account.AccountID] = summary.QueryMarket
	}
	if queryMarkets["REAL-9985"] != "HK" || queryMarkets["REAL-7281"] != "US" || queryMarkets["REAL-8240"] != "US" {
		t.Fatalf("per-account read markets = %#v", queryMarkets)
	}
	funds := summaries[0].Funds.(map[string]any)
	positions := summaries[0].Positions.([]map[string]any)
	orders := summaries[0].Orders.([]map[string]any)
	if funds["accountId"] != summaries[0].Account.AccountID ||
		positions[0]["accountId"] != summaries[0].Account.AccountID ||
		orders[0]["accountId"] != summaries[0].Account.AccountID {
		t.Fatalf("cross-account data leak in summary = %#v", summaries[0])
	}
	if _, exists := payload["funds"]; exists {
		t.Fatal("multi-account summary exposed ambiguous top-level funds")
	}
	if len(payload["discoveredAccounts"].([]BrokerAccountView)) != 4 {
		t.Fatalf("discovered accounts = %#v", payload["discoveredAccounts"])
	}
	if payload["partial"] != false || payload["brokerEnabled"] != true {
		t.Fatalf("portfolio state = %#v", payload)
	}
}

func TestPortfolioLayeredToolsKeepDiscoveryOverviewAndPositionsSeparate(t *testing.T) {
	accounts := []BrokerAccountView{{AccountID: "REAL-8240", TradingEnvironment: "REAL", MarketAuthorities: []string{"US"}}}
	combinedReads := 0
	positionsReads := 0
	registry := newPortfolioTestRegistry(ToolDeps{
		DefaultTradeMarket: func() string { return "US" },
		BrokerRuntime: func(context.Context) (BrokerRuntimeView, error) {
			return BrokerRuntimeView{Connectivity: "connected", Accounts: accounts}, nil
		},
		BrokerAccountRead: func(_ context.Context, query broker.ReadQuery, _ time.Duration) BrokerAccountReadResult {
			combinedReads++
			return BrokerAccountReadResult{
				Funds: accountPayload(query), Positions: []map[string]any{{"symbol": "AAPL"}},
				PositionCount: 1, HasAssetsOrPositions: true, Errors: []string{},
			}
		},
		BrokerPositionsRead: func(_ context.Context, _ broker.ReadQuery, _ time.Duration) BrokerAccountReadResult {
			positionsReads++
			return BrokerAccountReadResult{
				Positions:     &trdsrv.BrokerPositionsResponse{Positions: []trdsrv.BrokerPosition{{Symbol: "AAPL"}}},
				PositionCount: 1, HasAssetsOrPositions: true, Errors: []string{},
			}
		},
		ExecutionOrders: func(context.Context, BrokerReadInput) (any, int, error) {
			return []map[string]any{{"id": "order-1"}}, 1, nil
		},
	})

	accountsTool, _ := registry.Get("portfolio.accounts")
	accountsOutput, err := accountsTool.Handler(t.Context(), map[string]any{"tradingEnvironment": "REAL"})
	if err != nil {
		t.Fatalf("portfolio.accounts: %v", err)
	}
	accountsPayload := accountsOutput.(map[string]any)
	if len(accountsPayload["discoveredAccounts"].([]BrokerAccountView)) != 1 {
		t.Fatalf("discovered accounts = %#v", accountsPayload)
	}

	overviewTool, _ := registry.Get("portfolio.overview")
	overviewOutput, err := overviewTool.Handler(t.Context(), map[string]any{"tradingEnvironment": "REAL"})
	if err != nil {
		t.Fatalf("portfolio.overview: %v", err)
	}
	overviewPayload := overviewOutput.(map[string]any)
	overviews := overviewPayload["accountOverviews"].([]portfolioAccountOverview)
	if len(overviews) != 1 || overviews[0].PositionCount != 1 || overviews[0].OrderCount != 1 {
		t.Fatalf("account overviews = %#v", overviews)
	}
	if _, exists := overviewPayload["funds"]; exists {
		t.Fatalf("overview leaked funds: %#v", overviewPayload)
	}

	positionsTool, _ := registry.Get("portfolio.positions")
	positionsOutput, err := positionsTool.Handler(t.Context(), map[string]any{"tradingEnvironment": "REAL"})
	if err != nil {
		t.Fatalf("portfolio.positions: %v", err)
	}
	positionsPayload := positionsOutput.(map[string]any)
	positions := positionsPayload["accountPositions"].([]map[string]any)
	if len(positions) != 1 || positions[0]["positions"] == nil {
		t.Fatalf("account positions = %#v", positions)
	}
	if _, exists := positions[0]["funds"]; exists {
		t.Fatalf("positions leaked funds: %#v", positions[0])
	}
	if positionsReads != 1 || combinedReads != 1 {
		t.Fatalf("layered reads positions=%d combined=%d, want positions=1 combined=1", positionsReads, combinedReads)
	}
}

func TestPortfolioLayeredToolsReportValidationDiscoveryAndPartialReadStates(t *testing.T) {
	t.Run("required trading environment", func(t *testing.T) {
		registry := newPortfolioTestRegistry(ToolDeps{})
		for _, name := range []string{"portfolio.accounts", "portfolio.overview", "portfolio.positions"} {
			tool, _ := registry.Get(name)
			if _, err := tool.Handler(t.Context(), map[string]any{}); err == nil || !strings.Contains(err.Error(), "tradingEnvironment is required") {
				t.Fatalf("%s error = %v", name, err)
			}
		}
	})

	t.Run("discovery failure", func(t *testing.T) {
		registry := newPortfolioTestRegistry(ToolDeps{
			BrokerRuntime: func(context.Context) (BrokerRuntimeView, error) {
				return BrokerRuntimeView{}, errors.New("OpenD account discovery failed")
			},
		})
		for _, name := range []string{"portfolio.accounts", "portfolio.overview", "portfolio.positions"} {
			tool, _ := registry.Get(name)
			output, err := tool.Handler(t.Context(), map[string]any{"tradingEnvironment": "REAL"})
			if err != nil {
				t.Fatalf("%s: %v", name, err)
			}
			payload := output.(map[string]any)
			selection := payload["selection"].(portfolioSelection)
			if payload["partial"] != true || selection.Status != "discovery_failed" {
				t.Fatalf("%s payload = %#v", name, payload)
			}
		}
	})

	t.Run("runtime and account read warnings", func(t *testing.T) {
		accounts := []BrokerAccountView{
			{AccountID: "REAL-3", TradingEnvironment: "REAL", MarketAuthorities: []string{"US"}},
			{AccountID: "REAL-1", TradingEnvironment: "REAL", MarketAuthorities: []string{"US"}},
			{AccountID: "REAL-2", TradingEnvironment: "REAL", MarketAuthorities: []string{"US"}},
		}
		registry := newPortfolioTestRegistry(ToolDeps{
			BrokerRuntime: func(context.Context) (BrokerRuntimeView, error) {
				return BrokerRuntimeView{Accounts: accounts, LastError: "quote login state is unknown"}, nil
			},
			BrokerPositionsRead: func(_ context.Context, query broker.ReadQuery, _ time.Duration) BrokerAccountReadResult {
				switch query.AccountID {
				case "REAL-1":
					return BrokerAccountReadResult{Positions: []any{"AAPL"}, PositionCount: 1, HasAssetsOrPositions: true, Errors: []string{}}
				case "REAL-2":
					return BrokerAccountReadResult{Partial: true, Errors: []string{"positions unavailable"}}
				default:
					return BrokerAccountReadResult{Errors: []string{}}
				}
			},
		})

		accountsTool, _ := registry.Get("portfolio.accounts")
		output, err := accountsTool.Handler(t.Context(), map[string]any{"tradingEnvironment": "REAL"})
		if err != nil {
			t.Fatalf("portfolio.accounts: %v", err)
		}
		accountsPayload := output.(map[string]any)
		if accountsPayload["partial"] != true || len(accountsPayload["warnings"].([]string)) != 1 {
			t.Fatalf("accounts payload = %#v", accountsPayload)
		}

		positionsTool, _ := registry.Get("portfolio.positions")
		output, err = positionsTool.Handler(t.Context(), map[string]any{"tradingEnvironment": "REAL"})
		if err != nil {
			t.Fatalf("portfolio.positions: %v", err)
		}
		positionsPayload := output.(map[string]any)
		positions := positionsPayload["accountPositions"].([]map[string]any)
		warnings := positionsPayload["warnings"].([]string)
		first := positions[0]["account"].(BrokerAccountView)
		if positionsPayload["partial"] != true || first.AccountID != "REAL-1" || len(warnings) != 2 {
			t.Fatalf("positions payload = %#v", positionsPayload)
		}
	})

	t.Run("unavailable overview and positions readers", func(t *testing.T) {
		registry := newPortfolioTestRegistry(ToolDeps{
			BrokerRuntime: func(context.Context) (BrokerRuntimeView, error) {
				return BrokerRuntimeView{Accounts: []BrokerAccountView{{AccountID: "REAL-1", TradingEnvironment: "REAL", MarketAuthorities: []string{"US"}}}}, nil
			},
			ExecutionOrders: func(context.Context, BrokerReadInput) (any, int, error) {
				return nil, 0, errors.New("orders unavailable")
			},
		})
		for _, name := range []string{"portfolio.overview", "portfolio.positions"} {
			tool, _ := registry.Get(name)
			output, err := tool.Handler(t.Context(), map[string]any{"tradingEnvironment": "REAL"})
			if err != nil {
				t.Fatalf("%s: %v", name, err)
			}
			payload := output.(map[string]any)
			if payload["partial"] != true || len(payload["warnings"].([]string)) == 0 {
				t.Fatalf("%s payload = %#v", name, payload)
			}
		}
	})
}

func TestPortfolioAccountResolutionSupportsExactSuffixAndIsolation(t *testing.T) {
	accounts := normalizedBrokerAccounts([]BrokerAccountView{
		{AccountID: "REAL-A-8240", TradingEnvironment: "REAL", MarketAuthorities: []string{"US"}},
		{AccountID: "REAL-B-8240", TradingEnvironment: "REAL", MarketAuthorities: []string{"HK"}},
		{AccountID: "REAL-7281", TradingEnvironment: "REAL", MarketAuthorities: []string{"US"}},
		{AccountID: "SIM-7281", TradingEnvironment: "SIMULATE", MarketAuthorities: []string{"US"}},
	})
	tests := []struct {
		name, environment, market, accountID, status, mode, selected string
	}{
		{name: "exact wins", environment: "REAL", accountID: "REAL-A-8240", status: "resolved", mode: "exact", selected: "REAL-A-8240"},
		{name: "unique suffix", environment: "REAL", accountID: "7281", status: "resolved", mode: "unique_suffix", selected: "REAL-7281"},
		{name: "suffix conflict", environment: "REAL", accountID: "8240", status: "ambiguous", mode: "suffix"},
		{name: "missing", environment: "REAL", accountID: "9999", status: "not_found", mode: "account_id"},
		{name: "environment isolation", environment: "SIMULATE", accountID: "7281", status: "resolved", mode: "unique_suffix", selected: "SIM-7281"},
		{name: "market isolation", environment: "REAL", market: "HK", accountID: "8240", status: "resolved", mode: "unique_suffix", selected: "REAL-B-8240"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			matched, selection := resolvePortfolioAccounts(accounts, test.environment, test.market, test.accountID)
			if selection.Status != test.status || selection.Mode != test.mode {
				t.Fatalf("selection = %#v", selection)
			}
			if test.selected != "" && (len(matched) != 1 || matched[0].AccountID != test.selected) {
				t.Fatalf("matched = %#v, want %s", matched, test.selected)
			}
			if test.status == "ambiguous" && len(selection.CandidateAccounts) != 2 {
				t.Fatalf("ambiguous candidates = %#v", selection.CandidateAccounts)
			}
		})
	}
}

func TestPortfolioSummaryKeepsPartialAccountResultsAndDiscoveryFailuresVisible(t *testing.T) {
	t.Run("one account timeout", func(t *testing.T) {
		registry := newPortfolioTestRegistry(ToolDeps{
			BrokerRuntime: func(context.Context) (BrokerRuntimeView, error) {
				return BrokerRuntimeView{Accounts: []BrokerAccountView{
					{AccountID: "REAL-1", TradingEnvironment: "REAL", MarketAuthorities: []string{"US"}},
					{AccountID: "REAL-2", TradingEnvironment: "REAL", MarketAuthorities: []string{"US"}},
				}}, nil
			},
			BrokerAccountRead: func(_ context.Context, query broker.ReadQuery, _ time.Duration) BrokerAccountReadResult {
				if query.AccountID == "REAL-1" {
					return BrokerAccountReadResult{Partial: true, Errors: []string{"positions: deadline exceeded"}}
				}
				return BrokerAccountReadResult{Funds: accountPayload(query), Positions: []any{}, Errors: []string{}}
			},
			ExecutionOrders: func(context.Context, BrokerReadInput) (any, int, error) { return []any{}, 0, nil },
		})
		tool, _ := registry.Get("portfolio.summary")
		output, err := tool.Handler(t.Context(), map[string]any{"tradingEnvironment": "REAL"})
		if err != nil {
			t.Fatalf("portfolio.summary: %v", err)
		}
		payload := output.(map[string]any)
		if payload["partial"] != true || len(payload["accountSummaries"].([]portfolioAccountSummary)) != 2 {
			t.Fatalf("partial payload = %#v", payload)
		}
		if warnings := payload["warnings"].([]string); len(warnings) != 1 || !strings.Contains(warnings[0], "deadline exceeded") {
			t.Fatalf("warnings = %#v", warnings)
		}
	})

	t.Run("discovery failure", func(t *testing.T) {
		registry := newPortfolioTestRegistry(ToolDeps{
			BrokerRuntime: func(context.Context) (BrokerRuntimeView, error) {
				return BrokerRuntimeView{}, errors.New("OpenD account discovery timed out")
			},
		})
		tool, _ := registry.Get("portfolio.summary")
		output, err := tool.Handler(t.Context(), map[string]any{"tradingEnvironment": "REAL"})
		if err != nil {
			t.Fatalf("portfolio.summary: %v", err)
		}
		payload := output.(map[string]any)
		selection := payload["selection"].(portfolioSelection)
		if payload["partial"] != true || selection.Status != "discovery_failed" {
			t.Fatalf("discovery failure payload = %#v", payload)
		}
	})
}

func TestAccountOrdersFiltersAccountEnvironmentMarketAndActiveStatus(t *testing.T) {
	orders := []trdsrv.ExecutionOrder{
		{InternalOrderID: "active", BrokerID: "futu", AccountID: "FULL-8240", TradingEnvironment: "REAL", Market: "US", Status: trdsrv.OrderStatusSubmitted},
		{InternalOrderID: "filled", BrokerID: "futu", AccountID: "FULL-8240", TradingEnvironment: "REAL", Market: "US", Status: trdsrv.OrderStatusFilled},
		{InternalOrderID: "other-account", BrokerID: "futu", AccountID: "FULL-7281", TradingEnvironment: "REAL", Market: "US", Status: trdsrv.OrderStatusSubmitted},
		{InternalOrderID: "simulate", BrokerID: "futu", AccountID: "FULL-8240", TradingEnvironment: "SIMULATE", Market: "US", Status: trdsrv.OrderStatusSubmitted},
		{InternalOrderID: "hk", BrokerID: "futu", AccountID: "FULL-8240", TradingEnvironment: "REAL", Market: "HK", Status: trdsrv.OrderStatusSubmitted},
	}
	service := trdsrv.NewService(trdsrv.WithListOrders(func(context.Context, trdsrv.ExecutionOrderFilter) (trdsrv.ExecutionOrders, error) {
		return trdsrv.ExecutionOrders{Orders: orders}, nil
	}))
	adapter := NewApplicationAdapter(ApplicationPorts{Trading: func() *trdsrv.Service { return service }})
	registry := newPortfolioTestRegistry(ToolDeps{
		BrokerRuntime: func(context.Context) (BrokerRuntimeView, error) {
			return BrokerRuntimeView{Accounts: []BrokerAccountView{{
				AccountID: "FULL-8240", TradingEnvironment: "REAL", MarketAuthorities: []string{"US", "HK"},
			}}}, nil
		},
		ExecutionOrders: adapter.executionOrders,
	})
	tool, _ := registry.Get("account.orders")
	output, err := tool.Handler(t.Context(), map[string]any{
		"accountId": "8240", "tradingEnvironment": "REAL", "market": "US", "activeOnly": true,
	})
	if err != nil {
		t.Fatalf("account.orders: %v", err)
	}
	payload := output.(map[string]any)
	filtered := payload["orders"].([]trdsrv.ExecutionOrder)
	if payload["count"] != 1 || len(filtered) != 1 || filtered[0].InternalOrderID != "active" {
		t.Fatalf("filtered orders = %#v, count=%#v", filtered, payload["count"])
	}
	selection := payload["selection"].(portfolioSelection)
	if selection.Mode != "unique_suffix" || selection.SelectedAccountIDs[0] != "FULL-8240" {
		t.Fatalf("selection = %#v", selection)
	}
}

func TestApplicationPortfolioReadsRuntimeFundsAndPositionsFromOneBrokerAccount(t *testing.T) {
	totalAssets := 1250.0
	lastError := "runtime warning"
	reader := &portfolioAccountReader{
		funds: &broker.FundsSnapshot{
			AccountID: "FULL-8240", TradingEnvironment: "REAL", Market: "US", TotalAssets: &totalAssets,
		},
		positions: []broker.PositionSnapshot{{
			AccountID: "FULL-8240", TradingEnvironment: "REAL", Market: "US", Symbol: "AAPL", Quantity: 2,
		}},
	}
	active := portfolioBroker{reader: reader}
	service := trdsrv.NewService(
		trdsrv.WithActiveBroker(func() broker.Broker { return active }),
		trdsrv.WithBrokerRuntime(func(context.Context) *trdsrv.BrokerRuntimeResponse {
			return &trdsrv.BrokerRuntimeResponse{
				Session: trdsrv.BrokerRuntimeSession{Connectivity: "connected", LastError: &lastError},
				Accounts: []trdsrv.BrokerRuntimeAccount{{
					AccountID: "FULL-8240", TradingEnvironment: "REAL", AccountType: "MARGIN",
					MarketAuthorities: []string{"US", "HK"},
				}},
			}
		}),
	)
	adapter := NewApplicationAdapter(ApplicationPorts{Trading: func() *trdsrv.Service { return service }})
	runtime, err := adapter.brokerRuntime(t.Context())
	if err != nil {
		t.Fatalf("brokerRuntime: %v", err)
	}
	if runtime.Connectivity != "connected" || runtime.LastError != lastError || len(runtime.Accounts) != 1 ||
		runtime.Accounts[0].AccountID != "FULL-8240" || runtime.Accounts[0].MarketAuthorities[0] != "US" {
		t.Fatalf("runtime projection = %#v", runtime)
	}

	read := adapter.brokerAccountRead(t.Context(), broker.ReadQuery{
		BrokerID: "futu", AccountID: "FULL-8240", TradingEnvironment: "REAL", Market: "US",
	}, time.Second)
	if read.Partial || !read.HasAssetsOrPositions || read.PositionCount != 1 || len(read.Errors) != 0 {
		t.Fatalf("account read = %#v", read)
	}
	if read.Funds.(*trdsrv.BrokerFundsResponse).Summary.AccountID != "FULL-8240" ||
		read.Positions.(*trdsrv.BrokerPositionsResponse).Positions[0].Symbol != "AAPL" {
		t.Fatalf("account payloads = funds %#v positions %#v", read.Funds, read.Positions)
	}
	if reader.fundsCalls != 1 || reader.positionCalls != 1 {
		t.Fatalf("combined account read calls funds=%d positions=%d", reader.fundsCalls, reader.positionCalls)
	}

	positionsRead := adapter.brokerPositionsRead(t.Context(), broker.ReadQuery{
		BrokerID: "futu", AccountID: "FULL-8240", TradingEnvironment: "REAL", Market: "US",
	}, time.Second)
	if positionsRead.Partial || positionsRead.PositionCount != 1 || positionsRead.Funds != nil {
		t.Fatalf("positions-only read = %#v", positionsRead)
	}
	if reader.fundsCalls != 1 || reader.positionCalls != 2 {
		t.Fatalf("positions-only calls funds=%d positions=%d, want 1/2", reader.fundsCalls, reader.positionCalls)
	}

	nilRuntimeService := trdsrv.NewService(
		trdsrv.WithActiveBroker(func() broker.Broker { return active }),
		trdsrv.WithBrokerRuntime(func(context.Context) *trdsrv.BrokerRuntimeResponse { return nil }),
	)
	nilRuntimeAdapter := NewApplicationAdapter(ApplicationPorts{Trading: func() *trdsrv.Service { return nilRuntimeService }})
	if _, err := nilRuntimeAdapter.brokerRuntime(t.Context()); err == nil || !strings.Contains(err.Error(), "empty response") {
		t.Fatalf("nil runtime error = %v", err)
	}
}

func TestApplicationPortfolioMarksIncompleteBrokerResponsesPartial(t *testing.T) {
	read := summarizeBrokerAccountResponses(nil, nil)
	if !read.Partial || len(read.Errors) != 2 || read.HasAssetsOrPositions || read.PositionCount != 0 {
		t.Fatalf("nil responses = %#v", read)
	}
	positionsOnly := summarizeBrokerPositionsResponse(nil)
	if !positionsOnly.Partial || len(positionsOnly.Errors) != 1 || positionsOnly.PositionCount != 0 {
		t.Fatalf("nil positions-only response = %#v", positionsOnly)
	}

	fundsError := "funds unavailable"
	positionsError := "positions unavailable"
	read = summarizeBrokerAccountResponses(
		&trdsrv.BrokerFundsResponse{BrokerReadStatus: trdsrv.BrokerReadStatus{LastError: &fundsError}},
		&trdsrv.BrokerPositionsResponse{
			BrokerReadStatus: trdsrv.BrokerReadStatus{LastError: &positionsError},
			Positions:        []trdsrv.BrokerPosition{{Symbol: "AAPL"}},
		},
	)
	if !read.Partial || !read.HasAssetsOrPositions || read.PositionCount != 1 || len(read.Errors) != 2 {
		t.Fatalf("errored responses = %#v", read)
	}
	positionsOnly = summarizeBrokerPositionsResponse(&trdsrv.BrokerPositionsResponse{
		BrokerReadStatus: trdsrv.BrokerReadStatus{LastError: &positionsError},
		Positions:        []trdsrv.BrokerPosition{{Symbol: "AAPL"}},
	})
	if !positionsOnly.Partial || !positionsOnly.HasAssetsOrPositions || positionsOnly.PositionCount != 1 || len(positionsOnly.Errors) != 1 {
		t.Fatalf("errored positions-only response = %#v", positionsOnly)
	}

	cash := 10.0
	assets := 20.0
	for name, funds := range map[string]*trdsrv.BrokerFundsResponse{
		"currency balance": {CurrencyBalances: []trdsrv.BrokerCurrencyBalance{{Cash: &cash}}},
		"market asset":     {MarketAssets: []trdsrv.BrokerMarketAsset{{Assets: &assets}}},
	} {
		t.Run(name, func(t *testing.T) {
			if !brokerFundsHaveAssets(funds) {
				t.Fatalf("funds were classified empty: %#v", funds)
			}
		})
	}
	zero := 0.0
	if brokerFundsHaveAssets(nil) || brokerFundsHaveAssets(&trdsrv.BrokerFundsResponse{
		Summary: &trdsrv.BrokerFundsSummary{TotalAssets: &zero},
	}) {
		t.Fatal("nil or zero funds were classified as assets")
	}
}

type portfolioAccountReader struct {
	broker.MarketDataReader
	funds         *broker.FundsSnapshot
	positions     []broker.PositionSnapshot
	fundsCalls    int
	positionCalls int
}

func (r *portfolioAccountReader) QueryFunds(context.Context, broker.ReadQuery) (*broker.FundsSnapshot, error) {
	r.fundsCalls++
	return r.funds, nil
}

func (r *portfolioAccountReader) QueryPositions(context.Context, broker.ReadQuery) ([]broker.PositionSnapshot, error) {
	r.positionCalls++
	return r.positions, nil
}

type portfolioBroker struct {
	reader broker.MarketDataReader
}

func (portfolioBroker) ID() string { return "futu" }

func (portfolioBroker) Descriptor() broker.Descriptor { return broker.Descriptor{} }

func (portfolioBroker) DiscoverAccounts(context.Context) ([]broker.Account, error) { return nil, nil }

func (portfolioBroker) Trading() broker.TradingService { return nil }

func (b portfolioBroker) MarketData() broker.MarketDataReader { return b.reader }

func newPortfolioTestRegistry(deps ToolDeps) *jfadkruntime.ToolRegistry {
	registry := jfadkruntime.NewToolRegistry()
	registerJFTradeADKPortfolioTools(registry, deps)
	return registry
}

func accountPayload(query broker.ReadQuery) map[string]any {
	return map[string]any{
		"accountId": query.AccountID, "tradingEnvironment": query.TradingEnvironment, "market": query.Market,
	}
}
