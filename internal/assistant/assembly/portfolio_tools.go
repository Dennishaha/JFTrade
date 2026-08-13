package assembly

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	jfadkruntime "github.com/jftrade/jftrade-main/internal/assistant/engine/workflowruntime"
	assistantmodel "github.com/jftrade/jftrade-main/internal/assistant/model"
	"github.com/jftrade/jftrade-main/pkg/broker"
)

// BrokerRuntimeView is the broker-neutral account discovery projection used by
// assistant portfolio tools.
type BrokerRuntimeView struct {
	Connectivity string              `json:"connectivity"`
	LastError    string              `json:"lastError,omitempty"`
	Accounts     []BrokerAccountView `json:"accounts"`
}

// BrokerAccountView identifies one account discovered from the live broker
// runtime. Managed Settings accounts are deliberately not used for discovery.
type BrokerAccountView struct {
	AccountID            string   `json:"accountId"`
	TradingEnvironment   string   `json:"tradingEnvironment"`
	AccountType          string   `json:"accountType"`
	AccountRole          *string  `json:"accountRole"`
	SecurityFirm         *string  `json:"securityFirm"`
	MarketAuthorities    []string `json:"marketAuthorities"`
	SimulatedAccountType *string  `json:"simulatedAccountType"`
}

// BrokerAccountReadResult keeps funds and positions from the same resolved
// account together and records whether either read was incomplete.
type BrokerAccountReadResult struct {
	Funds                any
	Positions            any
	PositionCount        int
	HasAssetsOrPositions bool
	Partial              bool
	Errors               []string
}

type portfolioSelection struct {
	Status             string              `json:"status"`
	Mode               string              `json:"mode"`
	Message            string              `json:"message,omitempty"`
	RequestedAccountID string              `json:"requestedAccountId,omitempty"`
	TradingEnvironment string              `json:"tradingEnvironment"`
	Market             string              `json:"market,omitempty"`
	CandidateAccounts  []BrokerAccountView `json:"candidateAccounts"`
	SelectedAccountIDs []string            `json:"selectedAccountIds"`
}

type portfolioAccountSummary struct {
	Account              BrokerAccountView `json:"account"`
	QueryMarket          string            `json:"queryMarket"`
	Funds                any               `json:"funds"`
	Positions            any               `json:"positions"`
	PositionCount        int               `json:"positionCount"`
	Orders               any               `json:"orders"`
	OrderCount           int               `json:"orderCount"`
	HasAssetsOrPositions bool              `json:"hasAssetsOrPositions"`
	Partial              bool              `json:"partial"`
	Errors               []string          `json:"errors"`
}

func registerJFTradeADKPortfolioTools(registry *jfadkruntime.ToolRegistry, deps ToolDeps) {
	registry.Register(assistantmodel.ToolDescriptor{
		Name: "portfolio.summary", DisplayName: "组合摘要",
		Description: "从券商 runtime 发现账户并按账户读取资金、持仓和订单；未指定 accountId 时扫描指定环境的全部账户。",
		Category:    "portfolio", Permission: "read_internal", RiskLevel: "low",
		OutputSummary:  "券商发现账户、选择状态和不跨账户聚合的逐账户摘要。",
		RequiredSkills: []string{"jftrade-portfolio"}, InputSchema: portfolioToolSchema(false),
	}, func(ctx context.Context, input map[string]any) (any, error) {
		return portfolioSummary(ctx, input, deps)
	})
	registry.Register(assistantmodel.ToolDescriptor{
		Name: "account.orders", DisplayName: "订单摘要",
		Description: "按账户、交易环境、可选市场和 activeOnly 读取结构化执行订单；不接受自由文本 query。",
		Category:    "portfolio", Permission: "read_internal", RiskLevel: "low",
		OutputSummary:  "实际过滤后的执行订单、准确数量和账户选择状态。",
		RequiredSkills: []string{"jftrade-portfolio"}, InputSchema: portfolioToolSchema(true),
	}, func(ctx context.Context, input map[string]any) (any, error) {
		return accountOrders(ctx, input, deps)
	})
}

func portfolioToolSchema(orders bool) map[string]any {
	properties := map[string]any{
		"accountId":          stringSchema(1, 128),
		"tradingEnvironment": enumSchema("SIMULATE", "REAL"),
		"market":             enumSchema(productMarketEnum...),
	}
	if orders {
		properties["activeOnly"] = map[string]any{"type": "boolean"}
	}
	return objectSchema(properties, []string{"tradingEnvironment"})
}

func portfolioSummary(ctx context.Context, input map[string]any, deps ToolDeps) (any, error) {
	environment := strings.ToUpper(strings.TrimSpace(stringValue(input, "tradingEnvironment")))
	if environment == "" {
		return nil, fmt.Errorf("tradingEnvironment is required")
	}
	market := strings.ToUpper(strings.TrimSpace(stringValue(input, "market")))
	accountID := strings.TrimSpace(stringValue(input, "accountId"))
	base := portfolioBaseResult(deps)
	runtime, err := discoverBrokerRuntime(ctx, deps)
	base["discoveredAccounts"] = runtime.Accounts
	base["brokerRuntime"] = map[string]any{"connectivity": runtime.Connectivity, "lastError": runtime.LastError}
	if err != nil {
		selection := failedPortfolioSelection(environment, market, accountID, "discovery_failed", err.Error())
		base["selection"], base["accountSummaries"], base["partial"] = selection, []portfolioAccountSummary{}, true
		base["warnings"] = []string{err.Error()}
		return base, nil
	}

	targets, selection := resolvePortfolioAccounts(runtime.Accounts, environment, market, accountID)
	base["selection"] = selection
	if selection.Status != "resolved" {
		base["accountSummaries"], base["partial"] = []portfolioAccountSummary{}, true
		base["warnings"] = []string{selection.Message}
		return base, nil
	}
	summaries := readPortfolioAccounts(ctx, targets, market, deps)
	partial, warnings := portfolioReadStatus(runtime.LastError, summaries)
	base["accountSummaries"], base["partial"], base["warnings"] = summaries, partial, warnings
	if len(summaries) == 1 {
		addLegacyPortfolioFields(base, summaries[0])
	}
	return base, nil
}

func portfolioBaseResult(deps ToolDeps) map[string]any {
	managed := any([]any{})
	if deps.ManagedAccounts != nil {
		managed = deps.ManagedAccounts()
	}
	return map[string]any{
		"accounts": managed, "managedAccounts": managed,
		"brokerEnabled": callBool(deps.BrokerEnabled), "checkedAt": nowStringRFC3339Nano(),
	}
}

func discoverBrokerRuntime(ctx context.Context, deps ToolDeps) (BrokerRuntimeView, error) {
	if deps.BrokerRuntime == nil {
		return BrokerRuntimeView{Accounts: []BrokerAccountView{}}, fmt.Errorf("broker account discovery is unavailable")
	}
	runtime, err := deps.BrokerRuntime(ctx)
	runtime.Accounts = normalizedBrokerAccounts(runtime.Accounts)
	return runtime, err
}

func normalizedBrokerAccounts(accounts []BrokerAccountView) []BrokerAccountView {
	result := append([]BrokerAccountView(nil), accounts...)
	for index := range result {
		result[index].AccountID = strings.TrimSpace(result[index].AccountID)
		result[index].TradingEnvironment = strings.ToUpper(strings.TrimSpace(result[index].TradingEnvironment))
		markets := make([]string, 0, len(result[index].MarketAuthorities))
		for _, market := range result[index].MarketAuthorities {
			if value := strings.ToUpper(strings.TrimSpace(market)); value != "" {
				markets = append(markets, value)
			}
		}
		sort.Strings(markets)
		result[index].MarketAuthorities = compactStrings(markets)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].TradingEnvironment != result[j].TradingEnvironment {
			return result[i].TradingEnvironment < result[j].TradingEnvironment
		}
		return result[i].AccountID < result[j].AccountID
	})
	return result
}

func resolvePortfolioAccounts(
	accounts []BrokerAccountView,
	environment string,
	market string,
	requestedID string,
) ([]BrokerAccountView, portfolioSelection) {
	candidates := make([]BrokerAccountView, 0, len(accounts))
	for _, account := range accounts {
		if !strings.EqualFold(account.TradingEnvironment, environment) {
			continue
		}
		if market != "" && !accountSupportsMarket(account, market) {
			continue
		}
		candidates = append(candidates, account)
	}
	selection := portfolioSelection{
		Status: "resolved", Mode: "all_matching_accounts", RequestedAccountID: requestedID,
		TradingEnvironment: environment, Market: market, CandidateAccounts: candidates, SelectedAccountIDs: []string{},
	}
	if requestedID == "" {
		if len(candidates) == 0 {
			selection.Status, selection.Message = "not_found", "no broker accounts matched the requested environment and market"
			return nil, selection
		}
		selection.SelectedAccountIDs = accountIDs(candidates)
		return candidates, selection
	}
	selection.Mode = "account_id"
	for _, candidate := range candidates {
		if candidate.AccountID == requestedID {
			selection.Mode, selection.SelectedAccountIDs = "exact", []string{candidate.AccountID}
			return []BrokerAccountView{candidate}, selection
		}
	}
	matches := make([]BrokerAccountView, 0, 1)
	for _, candidate := range candidates {
		if strings.HasSuffix(candidate.AccountID, requestedID) {
			matches = append(matches, candidate)
		}
	}
	if len(matches) == 1 {
		selection.Mode, selection.SelectedAccountIDs = "unique_suffix", []string{matches[0].AccountID}
		return matches, selection
	}
	selection.Status = "not_found"
	selection.Message = fmt.Sprintf("accountId %q did not match a discovered account", requestedID)
	if len(matches) > 1 {
		selection.Status = "ambiguous"
		selection.Mode = "suffix"
		selection.CandidateAccounts = matches
		selection.Message = fmt.Sprintf("accountId suffix %q matched multiple discovered accounts", requestedID)
	}
	return nil, selection
}

func readPortfolioAccounts(
	ctx context.Context,
	accounts []BrokerAccountView,
	explicitMarket string,
	deps ToolDeps,
) []portfolioAccountSummary {
	summaries := make([]portfolioAccountSummary, len(accounts))
	var wait sync.WaitGroup
	for index, account := range accounts {
		wait.Add(1)
		go func() {
			defer wait.Done()
			summaries[index] = readPortfolioAccount(ctx, account, explicitMarket, deps)
		}()
	}
	wait.Wait()
	sort.SliceStable(summaries, func(i, j int) bool {
		if summaries[i].HasAssetsOrPositions != summaries[j].HasAssetsOrPositions {
			return summaries[i].HasAssetsOrPositions
		}
		return summaries[i].Account.AccountID < summaries[j].Account.AccountID
	})
	return summaries
}

func readPortfolioAccount(
	ctx context.Context,
	account BrokerAccountView,
	explicitMarket string,
	deps ToolDeps,
) portfolioAccountSummary {
	queryMarket := selectAccountReadMarket(account, explicitMarket, callString(deps.DefaultTradeMarket))
	query := broker.ReadQuery{
		BrokerID: "futu", AccountID: account.AccountID,
		TradingEnvironment: account.TradingEnvironment, Market: queryMarket,
	}
	read := BrokerAccountReadResult{Errors: []string{}}
	if deps.BrokerAccountRead == nil {
		read.Partial, read.Errors = true, []string{"broker funds and positions reader is unavailable"}
	} else {
		read = deps.BrokerAccountRead(ctx, query, 8*time.Second)
	}
	orders, orderCount, orderErr := readExecutionOrders(ctx, deps, BrokerReadInput{
		TradingEnvironment: account.TradingEnvironment, AccountID: account.AccountID, Market: explicitMarket,
	})
	if orderErr != nil {
		read.Partial = true
		read.Errors = append(read.Errors, "orders: "+orderErr.Error())
	}
	return portfolioAccountSummary{
		Account: account, QueryMarket: queryMarket, Funds: read.Funds, Positions: read.Positions,
		PositionCount: read.PositionCount, Orders: orders, OrderCount: orderCount,
		HasAssetsOrPositions: read.HasAssetsOrPositions, Partial: read.Partial,
		Errors: nonNilStrings(read.Errors),
	}
}

func accountOrders(ctx context.Context, input map[string]any, deps ToolDeps) (any, error) {
	environment := strings.ToUpper(strings.TrimSpace(stringValue(input, "tradingEnvironment")))
	if environment == "" {
		return nil, fmt.Errorf("tradingEnvironment is required")
	}
	market := strings.ToUpper(strings.TrimSpace(stringValue(input, "market")))
	requestedID := strings.TrimSpace(stringValue(input, "accountId"))
	selection := portfolioSelection{
		Status: "resolved", Mode: "all_matching_orders", RequestedAccountID: requestedID,
		TradingEnvironment: environment, Market: market, CandidateAccounts: []BrokerAccountView{}, SelectedAccountIDs: []string{},
	}
	discovered := []BrokerAccountView{}
	accountID := ""
	if requestedID != "" {
		runtime, err := discoverBrokerRuntime(ctx, deps)
		discovered = runtime.Accounts
		if err != nil {
			selection = failedPortfolioSelection(environment, market, requestedID, "discovery_failed", err.Error())
			return emptyAccountOrdersResult(selection, discovered, boolInput(input, "activeOnly")), nil
		}
		matched, resolved := resolvePortfolioAccounts(runtime.Accounts, environment, market, requestedID)
		selection = resolved
		if selection.Status != "resolved" {
			return emptyAccountOrdersResult(selection, discovered, boolInput(input, "activeOnly")), nil
		}
		accountID = matched[0].AccountID
	}
	activeOnly := boolInput(input, "activeOnly")
	orders, count, err := readExecutionOrders(ctx, deps, BrokerReadInput{
		TradingEnvironment: environment, AccountID: accountID, Market: market, ActiveOnly: activeOnly,
	})
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"orders": orders, "count": count, "activeOnly": activeOnly, "partial": false,
		"selection": selection, "discoveredAccounts": discovered, "checkedAt": nowStringRFC3339Nano(),
	}, nil
}

func readExecutionOrders(ctx context.Context, deps ToolDeps, input BrokerReadInput) (any, int, error) {
	if deps.ExecutionOrders == nil {
		return []any{}, 0, fmt.Errorf("execution order reader is unavailable")
	}
	return deps.ExecutionOrders(ctx, input)
}

func emptyAccountOrdersResult(selection portfolioSelection, accounts []BrokerAccountView, activeOnly bool) map[string]any {
	return map[string]any{
		"orders": []any{}, "count": 0, "activeOnly": activeOnly, "partial": true,
		"selection": selection, "discoveredAccounts": accounts,
		"warnings": []string{selection.Message}, "checkedAt": nowStringRFC3339Nano(),
	}
}

func failedPortfolioSelection(environment, market, accountID, status, message string) portfolioSelection {
	return portfolioSelection{
		Status: status, Mode: "none", Message: message, RequestedAccountID: accountID,
		TradingEnvironment: environment, Market: market,
		CandidateAccounts: []BrokerAccountView{}, SelectedAccountIDs: []string{},
	}
}

func portfolioReadStatus(runtimeError string, summaries []portfolioAccountSummary) (bool, []string) {
	partial := strings.TrimSpace(runtimeError) != ""
	warnings := []string{}
	if partial {
		warnings = append(warnings, "broker runtime: "+runtimeError)
	}
	for _, summary := range summaries {
		if summary.Partial {
			partial = true
			for _, message := range summary.Errors {
				warnings = append(warnings, summary.Account.AccountID+": "+message)
			}
		}
	}
	return partial, warnings
}

func addLegacyPortfolioFields(result map[string]any, summary portfolioAccountSummary) {
	result["funds"], result["positions"], result["orders"] = summary.Funds, summary.Positions, summary.Orders
	result["positionCount"], result["orderCount"] = summary.PositionCount, summary.OrderCount
}

func selectAccountReadMarket(account BrokerAccountView, explicitMarket, defaultMarket string) string {
	if explicitMarket != "" {
		return explicitMarket
	}
	defaultMarket = strings.ToUpper(strings.TrimSpace(defaultMarket))
	if defaultMarket != "" && accountSupportsMarket(account, defaultMarket) {
		return defaultMarket
	}
	if len(account.MarketAuthorities) > 0 {
		return account.MarketAuthorities[0]
	}
	if defaultMarket != "" {
		return defaultMarket
	}
	return "HK"
}

func accountSupportsMarket(account BrokerAccountView, market string) bool {
	for _, authority := range account.MarketAuthorities {
		if strings.EqualFold(authority, market) {
			return true
		}
	}
	return false
}

func accountIDs(accounts []BrokerAccountView) []string {
	result := make([]string, len(accounts))
	for index, account := range accounts {
		result[index] = account.AccountID
	}
	return result
}

func compactStrings(values []string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		if len(result) == 0 || result[len(result)-1] != value {
			result = append(result, value)
		}
	}
	return result
}

func nonNilStrings(values []string) []string {
	if values == nil {
		return []string{}
	}
	return values
}

func boolInput(input map[string]any, key string) bool {
	value, _ := input[key].(bool)
	return value
}

func callString(fn func() string) string {
	if fn == nil {
		return ""
	}
	return fn()
}
