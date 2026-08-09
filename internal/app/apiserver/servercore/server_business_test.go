package servercore

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jftrade/jftrade-main/internal/app/apiserver/webaccess"
	jfsettings "github.com/jftrade/jftrade-main/internal/jftsettings"
	mdsrv "github.com/jftrade/jftrade-main/internal/marketdata"
	strategystore "github.com/jftrade/jftrade-main/internal/store/strategy"
	stratsrv "github.com/jftrade/jftrade-main/internal/strategy"
	"github.com/jftrade/jftrade-main/internal/strategy/liveruntime"
	"github.com/jftrade/jftrade-main/pkg/broker"
	"github.com/jftrade/jftrade-main/pkg/strategy/pineworker"
	"github.com/shopspring/decimal"
)

func TestWorkflowAndMarketRuntimeBoundaryHelpers(t *testing.T) {
	panicked := false
	func() {
		defer func() {
			if recover() != nil {
				panicked = true
			}
		}()
		(*serverApplication)(nil).handlePushMarketdataTick(mdsrv.Tick{Kind: mdsrv.TickKindTrade})
		(&Server{}).handlePushMarketdataTick(mdsrv.Tick{Kind: "quote"})
		(&Server{}).handlePushMarketdataTick(mdsrv.Tick{Kind: mdsrv.TickKindTrade, InstrumentID: "US.AAPL", Price: decimal.NewFromFloat(101.5), Volume: decimal.NewFromInt(2)})
	}()
	if panicked {
		t.Fatal("nil market tick boundary panicked")
	}
}

func TestStrategyRuntimeBrokerBridgeDelegates(t *testing.T) {
	ctx := context.Background()
	funds := &broker.FundsSnapshot{AccountID: "acct-1", Market: "US"}
	positions := []broker.PositionSnapshot{{AccountID: "acct-1", Symbol: "US.AAPL", Quantity: 3}}
	reader := &servercoreFakeBrokerReader{funds: funds, positions: positions}
	trading := &servercoreFakeBrokerTrading{}
	bridge := &strategyRuntimeBrokerBridge{broker: servercoreFakeBroker{reader: reader, trading: trading}}

	query := broker.ReadQuery{AccountID: "acct-1", Market: "US"}
	gotFunds, err := bridge.QueryBrokerFunds(ctx, query)
	if err != nil || gotFunds != funds || reader.fundsQuery.AccountID != "acct-1" {
		t.Fatalf("QueryBrokerFunds() = %#v err=%v reader=%#v", gotFunds, err, reader)
	}
	gotPositions, err := bridge.QueryBrokerPositions(ctx, query)
	if err != nil || len(gotPositions) != 1 || reader.positionsQuery.Market != "US" {
		t.Fatalf("QueryBrokerPositions() = %#v err=%v reader=%#v", gotPositions, err, reader)
	}
	placeQuery := broker.PlaceOrderQuery{ReadQuery: query, Symbol: "US.AAPL", Quantity: 1}
	gotOrder, err := bridge.PlaceBrokerOrder(ctx, placeQuery)
	if err == nil || gotOrder != nil || !strings.Contains(err.Error(), "pre-trade boundary") || trading.placeQuery.Symbol != "" {
		t.Fatalf("PlaceBrokerOrder() = %#v err=%v trading=%#v, want fail-closed bridge", gotOrder, err, trading)
	}

	missingReader := &strategyRuntimeBrokerBridge{broker: servercoreFakeBroker{}}
	if _, err := missingReader.QueryBrokerFunds(ctx, query); err == nil || !strings.Contains(err.Error(), "market data not available") {
		t.Fatalf("missing funds reader error = %v", err)
	}
	if _, err := missingReader.QueryBrokerPositions(ctx, query); err == nil || !strings.Contains(err.Error(), "market data not available") {
		t.Fatalf("missing positions reader error = %v", err)
	}
	if _, err := missingReader.PlaceBrokerOrder(ctx, placeQuery); err == nil || !strings.Contains(err.Error(), "pre-trade boundary") {
		t.Fatalf("direct placement guard error = %v", err)
	}
}

func TestTimeStatusAndDefaultScriptBoundaries(t *testing.T) {
	parsed := httpTime("2026-06-20T13:30:00.123456789+08:00")
	if parsed.IsZero() || parsed.Location() != time.UTC || parsed.Format(time.RFC3339Nano) != "2026-06-20T05:30:00.123456789Z" {
		t.Fatalf("httpTime parsed = %s", parsed.Format(time.RFC3339Nano))
	}
	if !httpTime("not-time").IsZero() {
		t.Fatalf("invalid httpTime should be zero")
	}

	script := strategystore.DefaultPine(`Quote "Name"`)
	if !strings.Contains(script, `strategy("Quote \"Name\""`) || !strings.Contains(script, "ta.crossover") {
		t.Fatalf("default strategy script = %q", script)
	}
}

func TestStrategyBindingHelpersNormalizeLooseAPIParams(t *testing.T) {
	binding := normalizeStrategyInstanceBinding(stratsrv.InstanceBinding{}, map[string]any{
		"instruments": []any{
			map[string]any{"market": " hk ", "code": " 00700 "},
			map[string]any{"market": "HK", "code": "00700"},
			map[string]any{"market": "US", "code": "AAPL"},
			"ignored",
		},
		"symbols":       []any{"ignored-when-instruments-exist"},
		"interval":      " 15m ",
		"executionMode": "notify_only",
		"brokerAccount": map[string]any{
			"brokerId":           " FUTU ",
			"accountId":          " 123 ",
			"tradingEnvironment": " simulate ",
			"market":             " hk ",
		},
		"runtimeRisk": map[string]any{
			"mode":             " enforce ",
			"closeOnly":        true,
			"maxOrderQuantity": float32(100),
			"maxOrderNotional": int64(20000),
			"dailyMaxOrders":   float64(3),
			"pauseOnReject":    true,
		},
	})
	if strings.Join(binding.Symbols, ",") != "HK.00700,US.AAPL" {
		t.Fatalf("symbols = %#v", binding.Symbols)
	}
	if len(binding.Instruments) != 2 || binding.Instruments[0].Market != "HK" || binding.Instruments[0].Code != "00700" {
		t.Fatalf("instruments = %#v", binding.Instruments)
	}
	if binding.Interval != "15m" || binding.ExecutionMode != strategyExecutionModeNotifyOnly {
		t.Fatalf("interval/mode = %q/%q", binding.Interval, binding.ExecutionMode)
	}
	if binding.BrokerAccount == nil || binding.BrokerAccount.BrokerID != "futu" || binding.BrokerAccount.TradingEnvironment != "SIMULATE" || binding.BrokerAccount.Market != "HK" {
		t.Fatalf("broker account = %#v", binding.BrokerAccount)
	}
	if binding.RuntimeRisk.Mode != "enforce" || !binding.RuntimeRisk.CloseOnly || binding.RuntimeRisk.MaxOrderQuantity == nil || *binding.RuntimeRisk.MaxOrderQuantity != 100 || binding.RuntimeRisk.DailyMaxOrders == nil || *binding.RuntimeRisk.DailyMaxOrders != 3 {
		t.Fatalf("runtime risk = %#v", binding.RuntimeRisk)
	}

	fallback := normalizeStrategyInstanceBinding(stratsrv.InstanceBinding{}, map[string]any{
		"symbol":        "us:aapl",
		"runtimeRisk":   map[string]any{"mode": "invalid", "maxOrderQuantity": -1, "dailyMaxOrders": 1.5},
		"brokerAccount": map[string]any{},
	})
	if len(fallback.Symbols) != 1 || fallback.Symbols[0] != "US.AAPL" || fallback.Interval != "5m" || fallback.ExecutionMode != strategyExecutionModeLive {
		t.Fatalf("fallback binding = %#v", fallback)
	}
	if fallback.BrokerAccount != nil || fallback.RuntimeRisk.Mode != "off" || fallback.RuntimeRisk.MaxOrderQuantity != nil || fallback.RuntimeRisk.DailyMaxOrders != nil {
		t.Fatalf("fallback risk/account = %#v/%#v", fallback.RuntimeRisk, fallback.BrokerAccount)
	}

	instance := &stratsrv.ManagedInstance{Params: map[string]any{
		"symbols":       []string{"HK.00700"},
		"executionMode": "live",
	}}
	applyStrategyBindingParams(instance)
	if instance.Params["symbol"] != "HK.00700" || instance.Params["interval"] != "5m" {
		t.Fatalf("applied params = %#v", instance.Params)
	}
	if _, ok := instance.Params["brokerAccount"]; ok {
		t.Fatalf("empty broker account should not be written: %#v", instance.Params)
	}
}

type servercoreFakeBroker struct {
	reader  broker.MarketDataReader
	trading broker.TradingService
}

func (b servercoreFakeBroker) ID() string { return "fake" }

func (b servercoreFakeBroker) Descriptor() broker.Descriptor { return broker.Descriptor{ID: "fake"} }

func (b servercoreFakeBroker) DiscoverAccounts(context.Context) ([]broker.Account, error) {
	return []broker.Account{{ID: "acct-1", BrokerID: "fake"}}, nil
}

func (b servercoreFakeBroker) Trading() broker.TradingService { return b.trading }

func (b servercoreFakeBroker) MarketData() broker.MarketDataReader { return b.reader }

type servercoreFakeBrokerReader struct {
	funds          *broker.FundsSnapshot
	positions      []broker.PositionSnapshot
	fundsQuery     broker.ReadQuery
	positionsQuery broker.ReadQuery
}

func (r *servercoreFakeBrokerReader) QueryFunds(_ context.Context, query broker.ReadQuery) (*broker.FundsSnapshot, error) {
	r.fundsQuery = query
	return r.funds, nil
}

func (r *servercoreFakeBrokerReader) QueryPositions(_ context.Context, query broker.ReadQuery) ([]broker.PositionSnapshot, error) {
	r.positionsQuery = query
	return r.positions, nil
}

func (r *servercoreFakeBrokerReader) QueryOrders(context.Context, broker.ReadQuery, string) ([]broker.OrderSnapshot, error) {
	return nil, nil
}

func (r *servercoreFakeBrokerReader) QueryHistoryOrders(context.Context, broker.OrderHistoryQuery) ([]broker.OrderSnapshot, error) {
	return nil, nil
}

func (r *servercoreFakeBrokerReader) QueryOrderFills(context.Context, broker.OrderFillQuery) ([]broker.OrderFillSnapshot, error) {
	return nil, nil
}

func (r *servercoreFakeBrokerReader) QueryHistoryOrderFills(context.Context, broker.OrderFillHistoryQuery) ([]broker.OrderFillSnapshot, error) {
	return nil, nil
}

func (r *servercoreFakeBrokerReader) QueryOrderFees(context.Context, broker.OrderFeeQuery) ([]broker.OrderFeeSnapshot, error) {
	return nil, nil
}

func (r *servercoreFakeBrokerReader) QueryMarginRatios(context.Context, broker.MarginRatioQuery) ([]broker.MarginRatioSnapshot, error) {
	return nil, nil
}

func (r *servercoreFakeBrokerReader) QueryCashFlows(context.Context, broker.CashFlowQuery) ([]broker.CashFlowSnapshot, error) {
	return nil, nil
}

func (r *servercoreFakeBrokerReader) QueryMaxTradeQuantity(context.Context, broker.MaxTradeQuantityQuery) (*broker.MaxTradeQuantitySnapshot, error) {
	return nil, nil
}

func (r *servercoreFakeBrokerReader) QueryQuote(context.Context, broker.QuoteQuery) (*broker.QuoteSnapshot, error) {
	return nil, nil
}

func (r *servercoreFakeBrokerReader) QueryKLines(context.Context, broker.KLineQuery) (*broker.KLineSnapshot, error) {
	return nil, nil
}

func (r *servercoreFakeBrokerReader) QuerySecurityInfo(context.Context, broker.SecurityInfoQuery) (*broker.SecurityInfoSnapshot, error) {
	return nil, nil
}

func (r *servercoreFakeBrokerReader) QuerySecuritySearch(context.Context, broker.SecuritySearchQuery) (*broker.SecuritySearchSnapshot, error) {
	return nil, nil
}

func (r *servercoreFakeBrokerReader) QuerySecuritySnapshot(context.Context, broker.SecuritySnapshotQuery) (*broker.SecuritySnapshotResult, error) {
	return nil, nil
}

func (r *servercoreFakeBrokerReader) QueryOrderBook(context.Context, broker.OrderBookQuery) (*broker.OrderBookSnapshot, error) {
	return nil, nil
}

type servercoreFakeBrokerTrading struct {
	result     *broker.PlaceOrderResult
	placeQuery broker.PlaceOrderQuery
}

func (t *servercoreFakeBrokerTrading) PlaceOrder(_ context.Context, query broker.PlaceOrderQuery) (*broker.PlaceOrderResult, error) {
	t.placeQuery = query
	return t.result, nil
}

func (t *servercoreFakeBrokerTrading) CancelOrders(context.Context, broker.ReadQuery, ...broker.CancelOrder) error {
	return nil
}

func TestServerCloseAggregatesPineWorkerRunnerErrorsOnce(t *testing.T) {
	backtestErr := errors.New("backtest runner stopped with in-flight work")
	instanceErr := errors.New("instance runner transport close failed")
	backtestRunner := &errorClosingPineWorkerRunner{err: backtestErr}
	instanceRunner := &errorClosingPineWorkerRunner{err: instanceErr}
	server := &Server{}
	server.runtimes.SetPineWorkerRunners(backtestRunner, instanceRunner)

	err := server.Close()
	if err == nil {
		t.Fatal("Close should surface pine worker shutdown errors")
		return
	}
	if !strings.Contains(err.Error(), "backtestPineWorkerRunner close") ||
		!strings.Contains(err.Error(), "instancePineWorkerRunner close") {
		t.Fatalf("Close err = %v", err)
	}
	if !errors.Is(err, backtestErr) || !errors.Is(err, instanceErr) {
		t.Fatalf("Close err should wrap both runner errors: %v", err)
	}

	if secondErr := server.Close(); secondErr == nil || secondErr.Error() != err.Error() {
		t.Fatalf("second Close err = %v, want same aggregated error %v", secondErr, err)
	}
	if backtestRunner.closed != 1 || instanceRunner.closed != 1 {
		t.Fatalf("runner close counts = %d/%d, want one close each", backtestRunner.closed, instanceRunner.closed)
	}
}

func TestServerSidecarFrontendRuntimeConfigFollowsSecuritySettings(t *testing.T) {
	frontendDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(frontendDir, "index.html"), []byte("<html>sidecar</html>"), 0o644); err != nil {
		t.Fatalf("WriteFile index.html: %v", err)
	}

	server := &Server{auth: webaccess.NewAuth(jfsettings.SecuritySettings{})}
	server.SetFrontendFS(os.DirFS(frontendDir), " http://127.0.0.1:3000/api/ ")
	server.ApplySecuritySettings(webSecuritySettings(t, false))
	if server.frontend == nil {
		t.Fatalf("SetFrontendFS did not mount frontend")
	}
	if server.auth == nil || !server.auth.WebAccessEnabled() {
		t.Fatalf("ApplySecuritySettings did not enable Web password auth")
	}

	recorder := httptest.NewRecorder()
	server.frontend.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/runtime-config.js", nil))
	body := recorder.Body.String()
	if recorder.Code != http.StatusOK {
		t.Fatalf("runtime config status = %d body=%q", recorder.Code, body)
	}
	if !strings.Contains(body, `"apiBaseUrl":"http://127.0.0.1:3000/api"`) || !strings.Contains(body, `"authRequired":true`) {
		t.Fatalf("runtime config body = %q", body)
	}

	server.ApplySecuritySettings(jfsettings.SecuritySettings{})
	if server.auth.WebAccessEnabled() {
		t.Fatalf("ApplySecuritySettings should disable Web access")
	}
	recorder = httptest.NewRecorder()
	server.frontend.ServeHTTP(recorder, httptest.NewRequest(http.MethodHead, "/runtime-config.js", nil))
	if recorder.Code != http.StatusOK || recorder.Body.Len() != 0 {
		t.Fatalf("runtime config HEAD status/body = %d/%q", recorder.Code, recorder.Body.String())
	}
}

func TestBrokerExecutionExchangePrefersRuntimeProviderAndRespectsDisabledIntegration(t *testing.T) {
	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	server := &Server{serverApplication: serverApplication{
		store: store,
	}}
	if got := brokerExecutionExchangeFor(&server.serverApplication); got != nil {
		t.Fatalf("brokerExecutionExchange disabled integration = %#v, want nil", got)
	}

	stub := newStrategyRuntimeStubExchange()
	runtime := liveruntime.NewManager(liveruntime.Dependencies{
		ExchangeProvider: func() liveruntime.Exchange { return stub },
	})
	server.runtimes.SetStrategyRuntime(runtime, runtime)
	if got := brokerExecutionExchangeFor(&server.serverApplication); got != stub {
		t.Fatalf("brokerExecutionExchange should prefer runtime provider, got %#v", got)
	}

	server.runtimes.StrategyRuntime().SetExchangeProvider(func() liveruntime.Exchange { return nil })
	if got := brokerExecutionExchangeFor(&server.serverApplication); got != nil {
		t.Fatalf("brokerExecutionExchange nil provider with disabled integration = %#v, want nil", got)
	}
}

type errorClosingPineWorkerRunner struct {
	err    error
	closed int
}

func (runner *errorClosingPineWorkerRunner) RunScript(context.Context, pineworker.RunScriptRequest) (pineworker.RunScriptResponse, error) {
	return pineworker.RunScriptResponse{}, nil
}

func (runner *errorClosingPineWorkerRunner) Close(context.Context) error {
	runner.closed++
	return runner.err
}
