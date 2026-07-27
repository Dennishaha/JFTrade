package servercore

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	appcomposition "github.com/jftrade/jftrade-main/internal/app/apiserver/application"
	asst "github.com/jftrade/jftrade-main/internal/assistant"
	assistantassembly "github.com/jftrade/jftrade-main/internal/assistant/assembly"
	assistanttestkit "github.com/jftrade/jftrade-main/internal/assistant/testkit"
	btsrv "github.com/jftrade/jftrade-main/internal/backtest"
	mdsrv "github.com/jftrade/jftrade-main/internal/marketdata"
	stratsrv "github.com/jftrade/jftrade-main/internal/strategy"
	"github.com/jftrade/jftrade-main/internal/strategy/instanceview"
	trdsrv "github.com/jftrade/jftrade-main/internal/trading"
	"github.com/jftrade/jftrade-main/pkg/broker"
	strategyir "github.com/jftrade/jftrade-main/pkg/strategy/ir"
)

func testApplicationAdapter(server *Server) *assistantassembly.ApplicationAdapter {
	return assistantassembly.NewApplicationAdapter(appcomposition.AssistantPorts(testAssistantOptions(server)))
}

func testAssistantOptions(server *Server) appcomposition.AssistantOptions {
	if server == nil {
		return appcomposition.AssistantOptions{}
	}
	return appcomposition.AssistantOptions{
		Settings:        server.store,
		Runtime:         server.runtimes.Assistant(),
		Health:          server.futuCoordinator(),
		System:          server.sysSvc,
		MarketData:      server.marketdataSvc,
		Strategy:        server.strategySvc,
		Trading:         server.tradingSvc,
		Backtest:        server.backtestSvc,
		ProductFeatures: server.productFeaturesSvc,
		Watchlist:       server.watchlistSvc,
	}
}

func assistantRuntime(server *Server) assistantassembly.Runtime {
	if server == nil {
		return nil
	}
	return server.runtimes.Assistant()
}

func (s *Server) adkToolDeps() ToolDeps {
	return testApplicationAdapter(s).ToolDeps()
}

func newAssistantWorkflowToolManager(server *Server) WorkflowToolManager {
	return assistantassembly.NewWorkflowToolManager(func() *asst.Service {
		if server == nil {
			return nil
		}
		return server.assistantSvc
	})
}

func (*Server) populateADKBrokerToolDeps(*ToolDeps)   {}
func (*Server) populateADKStrategyToolDeps(*ToolDeps) {}
func (*Server) populateADKBacktestToolDeps(*ToolDeps) {}

func (s *Server) adkExecutionOrderEvents(internalOrderID string) any {
	return s.adkToolDeps().ExecutionOrderEvents(internalOrderID)
}

func (s *Server) adkBrokerOrders(ctx context.Context, input BrokerReadInput) (any, error) {
	return s.adkToolDeps().BrokerOrders(ctx, input)
}

func (s *Server) adkBrokerFills(ctx context.Context, input BrokerReadInput) (any, error) {
	return s.adkToolDeps().BrokerFills(ctx, input)
}

func (s *Server) adkBrokerCashFlows(ctx context.Context, input BrokerReadInput) (any, error) {
	return s.adkToolDeps().BrokerCashFlows(ctx, input)
}

func (s *Server) adkBrokerFees(ctx context.Context, input BrokerReadInput) (any, error) {
	return s.adkToolDeps().BrokerFees(ctx, input)
}

func (s *Server) adkBrokerMarginRatios(ctx context.Context, input BrokerReadInput) (any, error) {
	return s.adkToolDeps().BrokerMarginRatios(ctx, input)
}

func (s *Server) adkUpdateStrategyInstanceMode(instanceID string, executionMode string) (any, error) {
	return s.adkToolDeps().UpdateStrategyInstanceMode(instanceID, executionMode)
}

func (s *Server) adkListStrategyDefinitionVersions(
	definitionID string,
) ([]stratsrv.DefinitionVersionSummary, bool, error) {
	return s.adkToolDeps().ListStrategyDefinitionVersions(definitionID)
}

func (s *Server) adkGetStrategyDefinitionVersion(
	definitionID string,
	version string,
) (stratsrv.DefinitionVersion, bool, error) {
	return s.adkToolDeps().GetStrategyDefinitionVersion(definitionID, version)
}

func (s *Server) adkSaveStrategyDraft(input StrategyDraftInput) (any, error) {
	return s.adkToolDeps().SaveStrategyDraft(input)
}

func (s *Server) adkSaveStrategyDefinition(input StrategyDefinitionInput) (any, error) {
	return s.adkToolDeps().SaveStrategyDefinition(input)
}

func (s *Server) adkStrategyInstanceSummaries() []StrategyInstanceSummary {
	return s.adkToolDeps().ListStrategyInstances()
}

func (s *Server) adkEnqueueBacktest(input BacktestStartInput) (BacktestRunRef, error) {
	return s.adkToolDeps().EnqueueBacktest(input)
}

func (s *Server) adkStartResearchBacktest(input ResearchBacktestInput) (BacktestRunSummary, error) {
	return s.adkToolDeps().StartResearchBacktest(input)
}

func strategyVisualModelFromInput(value any) (*stratsrv.VisualModel, error) {
	if value == nil {
		return nil, nil
	}
	data, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("visualModel must be a valid object: %w", err)
	}
	var model stratsrv.VisualModel
	if err := json.Unmarshal(data, &model); err != nil {
		return nil, fmt.Errorf("visualModel must be a valid object: %w", err)
	}
	return stratsrv.NormalizeVisualModel(&model)
}

func brokerReadQueryFromADK(service *trdsrv.Service, input BrokerReadInput) broker.ReadQuery {
	return service.ReadQuery("futu", input.TradingEnvironment, input.AccountID, input.Market)
}

func normalizeTradingBrokerScope(value string) (string, error) {
	switch strings.ToUpper(strings.TrimSpace(value)) {
	case "", "CURRENT":
		return "CURRENT", nil
	case "HISTORY":
		return "HISTORY", nil
	default:
		return "", fmt.Errorf("query parameter scope is invalid")
	}
}

func mergeADKBrokerValues(groups ...[]string) []string {
	seen := make(map[string]struct{})
	var values []string
	for _, group := range groups {
		for _, raw := range group {
			for part := range strings.SplitSeq(raw, ",") {
				value := strings.TrimSpace(part)
				key := strings.ToUpper(value)
				if value == "" {
					continue
				}
				if _, ok := seen[key]; ok {
					continue
				}
				seen[key] = struct{}{}
				values = append(values, value)
			}
		}
	}
	return values
}

func strategySummaryDefinitionID(item stratsrv.InstanceView) string {
	if definitionID := strings.TrimSpace(item.Definition.StrategyID); definitionID != "" {
		return definitionID
	}
	return instanceview.DefinitionIDFromParams(item.Params)
}

func brokerBindingMarket(binding *stratsrv.BrokerAccountBinding) string {
	if binding == nil {
		return ""
	}
	return binding.Market
}

func brokerBindingAccountID(binding *stratsrv.BrokerAccountBinding) string {
	if binding == nil {
		return ""
	}
	return binding.AccountID
}

func backtestRunSummaryFromSrvRun(run *btsrv.RunState) BacktestRunSummary {
	if run == nil {
		return BacktestRunSummary{}
	}
	return BacktestRunSummary{
		ID: run.ID, Status: run.Status, DefinitionID: run.Request.DefinitionID,
		DefinitionVersion: run.Request.DefinitionVersion, Market: run.Request.Market,
		Code: run.Request.Code, Symbol: run.Request.Symbol, Interval: run.Request.Interval,
		StartDate: run.Request.StartDate, EndDate: run.Request.EndDate,
		StartTime: run.Request.StartTime, EndTime: run.Request.EndTime,
		MarketTimezone: run.Request.MarketTimezone, InitialBalance: run.Request.InitialBalance,
		RehabType: run.Request.RehabType, ChartType: string(run.Request.ChartType),
		UseExtendedHours: run.Request.UseExtendedHours, Result: run.Result,
		CreatedAt: run.CreatedAt, UpdatedAt: run.UpdatedAt,
	}
}

type assistantOptimizationRuns struct {
	server *Server
}

func (a assistantOptimizationRuns) Get(runID string) (asst.OptimizationRun, bool) {
	if a.server == nil || a.server.backtestSvc == nil {
		return asst.OptimizationRun{}, false
	}
	run, ok, err := a.server.backtestSvc.GetResult(runID)
	if err != nil || !ok {
		return asst.OptimizationRun{}, false
	}
	return asst.OptimizationRun{Status: run.Status, Result: run.Result}, true
}

func (a assistantOptimizationRuns) Cancel(runID string) {
	if a.server != nil && a.server.backtestSvc != nil {
		a.server.backtestSvc.Cancel(runID)
	}
}

func (s *serverApplication) workflowMarketSnapshot(
	ctx context.Context,
	instrumentID string,
) (map[string]any, error) {
	adapter := assistantassembly.NewApplicationAdapter(assistantassembly.ApplicationPorts{
		MarketData: func() *mdsrv.Service {
			if s == nil {
				return nil
			}
			return s.marketdataSvc
		},
	})
	return adapter.WorkflowMarketSnapshot(ctx, instrumentID)
}

func splitWorkflowInstrumentID(instrumentID string) (string, string, bool) {
	parts := strings.SplitN(strings.TrimSpace(instrumentID), ".", 2)
	if len(parts) != 2 {
		return "", "", false
	}
	market := strings.ToUpper(strings.TrimSpace(parts[0]))
	symbol := strings.ToUpper(strings.TrimSpace(parts[1]))
	if market == "" || symbol == "" {
		return "", "", false
	}
	return market, symbol, true
}

func registerJFTradeProductTools(registry *assistanttestkit.ToolRegistry, deps ToolDeps) {
	assistantassembly.RegisterProductToolSet(registry, deps)
}

func registerJFTradeADKStrategyTools(store *assistanttestkit.Store, registry *assistanttestkit.ToolRegistry, deps ToolDeps) {
	assistantassembly.RegisterStrategyTools(store, registry, deps)
}

func registerADKStrategyResearchTools(registry *assistanttestkit.ToolRegistry, deps ToolDeps) {
	assistantassembly.RegisterStrategyResearchTools(registry, deps)
}

func registerADKStrategyOptimizationTools(store *assistanttestkit.Store, registry *assistanttestkit.ToolRegistry, deps ToolDeps) {
	assistantassembly.RegisterStrategyOptimizationTools(store, registry, deps)
}

func RegisterJFTradeADKTools(store *assistanttestkit.Store, registry *assistanttestkit.ToolRegistry, deps ToolDeps) {
	assistantassembly.RegisterJFTradeADKTools(store, registry, deps)
}

func recordADKWorkflowAudit(ctx context.Context, deps ToolDeps, kind string, subjectID string, detail string, metadata map[string]any) {
	assistantassembly.RecordWorkflowAudit(ctx, deps, kind, subjectID, detail, metadata)
}

func StrategyValidatePineToolPayload(input map[string]any) map[string]any {
	return assistantassembly.StrategyValidatePineToolPayload(input)
}

func ValidateADKStrategyDraftScript(script string) error {
	return assistantassembly.ValidateADKStrategyDraftScript(script)
}

func validateADKStrategyDraftScript(script string) error {
	return assistantassembly.ValidateADKStrategyDraftScript(script)
}

func ValidateADKStrategyScript(toolName string, script string) (StrategyPineValidation, error) {
	return assistantassembly.ValidateADKStrategyScript(toolName, script)
}

func StrategyMetadataPayload(program *strategyir.Program) map[string]any {
	return assistantassembly.StrategyMetadataPayload(program)
}

func BuildCompiledHookKinds(program *strategyir.Program) []string {
	return assistantassembly.BuildCompiledHookKinds(program)
}

func BuildCompiledRequirementsPayload(requirements strategyir.Requirements) map[string]any {
	return assistantassembly.BuildCompiledRequirementsPayload(requirements)
}

func SummarizeADKBacktestRuns(runs []BacktestRunSummary) map[string]any {
	return assistantassembly.SummarizeADKBacktestRuns(runs)
}

func SourceFormatPineV6() string {
	return assistantassembly.SourceFormatPineV6()
}
