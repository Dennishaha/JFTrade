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

	mdsrv "github.com/jftrade/jftrade-main/internal/marketdata"
	"github.com/jftrade/jftrade-main/pkg/broker"
	commonpb "github.com/jftrade/jftrade-main/pkg/futu/pb/common"
	"github.com/jftrade/jftrade-main/pkg/strategy/pineworker"
	"github.com/shopspring/decimal"
)

func TestRuntimeDefaultsAndLayoutBoundaries(t *testing.T) {
	development := ResolveLaunchDefaults(false)
	if development.APIBind != defaultDevelopmentAPIBind || development.GUIBind != "" {
		t.Fatalf("development defaults = %#v", development)
	}
	release := ResolveLaunchDefaults(true)
	if release.APIBind != defaultReleaseAPIBind || release.GUIBind != defaultReleaseGUIBind {
		t.Fatalf("release defaults = %#v", release)
	}
	if got := APIBaseURLForBind(":3000"); got != "http://127.0.0.1:3000" {
		t.Fatalf("APIBaseURLForBind(:3000) = %q", got)
	}
	if got := PortFromBind("127.0.0.1:5173", 3000); got != 5173 {
		t.Fatalf("PortFromBind = %d, want 5173", got)
	}
	if got := PortFromBind("invalid", 3000); got != 3000 {
		t.Fatalf("PortFromBind invalid = %d, want default", got)
	}

	root := t.TempDir()
	settingsPath := filepath.Join(root, "runtime", "settings.json")
	backtestPath := filepath.Join(root, "data", "backtest.db")
	if err := EnsureRuntimeLayout(settingsPath, backtestPath); err != nil {
		t.Fatalf("EnsureRuntimeLayout: %v", err)
	}
	for _, dir := range []string{filepath.Dir(settingsPath), filepath.Dir(backtestPath)} {
		if !directoryExists(dir) {
			t.Fatalf("runtime directory %s was not created", dir)
		}
	}
}

func TestWorkflowAndMarketRuntimeBoundaryHelpers(t *testing.T) {
	if _, err := (*Server)(nil).workflowMarketSnapshot(context.Background(), "US.AAPL"); err == nil || !strings.Contains(err.Error(), "market data service is unavailable") {
		t.Fatalf("nil workflowMarketSnapshot error = %v", err)
	}
	if _, err := (&Server{}).workflowMarketSnapshot(context.Background(), "bad-instrument"); err == nil || !strings.Contains(err.Error(), "market data service is unavailable") {
		t.Fatalf("unavailable workflowMarketSnapshot error = %v", err)
	}

	market, symbol, ok := splitWorkflowInstrumentID(" us.aapl ")
	if !ok || market != "US" || symbol != "AAPL" {
		t.Fatalf("splitWorkflowInstrumentID = %q/%q/%v", market, symbol, ok)
	}
	for _, raw := range []string{"", "US", ".AAPL", "US."} {
		if market, symbol, ok := splitWorkflowInstrumentID(raw); ok {
			t.Fatalf("splitWorkflowInstrumentID(%q) = %q/%q/true, want invalid", raw, market, symbol)
		}
	}

	if got := strategyRuntimeMarketFromSymbol("hk.00700", "US"); got != "HK" {
		t.Fatalf("strategyRuntimeMarketFromSymbol dotted = %q", got)
	}
	if got := strategyRuntimeMarketFromSymbol("us:aapl", "HK"); got != "US" {
		t.Fatalf("strategyRuntimeMarketFromSymbol colon = %q", got)
	}
	if got := strategyRuntimeMarketFromSymbol("AAPL", " us "); got != "US" {
		t.Fatalf("strategyRuntimeMarketFromSymbol fallback = %q", got)
	}

	if code, label := strategyRuntimeStartError(errors.New("missing provider")); code != 400 || label != "BAD_REQUEST" {
		t.Fatalf("missing provider start error = %d/%s", code, label)
	}
	if code, label := strategyRuntimeStartError(errors.New("broker gateway down")); code != 502 || label != "STRATEGY_RUNTIME_START_FAILED" {
		t.Fatalf("gateway start error = %d/%s", code, label)
	}

	(*Server)(nil).handlePushMarketdataTick(mdsrv.Tick{Kind: mdsrv.TickKindTrade})
	(&Server{}).handlePushMarketdataTick(mdsrv.Tick{Kind: "quote"})
	(&Server{}).handlePushMarketdataTick(mdsrv.Tick{Kind: mdsrv.TickKindTrade, InstrumentID: "US.AAPL", Price: decimal.NewFromFloat(101.5), Volume: 2})
}

func TestMarketdataProviderAndBrokerBridgeDelegates(t *testing.T) {
	ctx := context.Background()
	expectedErr := errors.New("details failed")
	provider := &marketdataProvider{
		descriptor: func(context.Context) (mdsrv.ProviderDescriptor, error) {
			return mdsrv.ProviderDescriptor{ProviderID: "futu-opend", DisplayName: "Futu OpenD", Source: "bbgo:futu"}, nil
		},
		getMarkets: func(context.Context) ([]mdsrv.MarketProfile, error) {
			return []mdsrv.MarketProfile{{"code": "US"}}, nil
		},
		normalizeInstrument: func(_ context.Context, input map[string]any) (map[string]any, error) {
			return map[string]any{"instrumentId": strings.ToUpper(input["instrumentId"].(string))}, nil
		},
		getSecurityDetails: func(context.Context, string, string) (mdsrv.SecurityDetails, error) {
			return nil, expectedErr
		},
		querySnapshot: func(context.Context, string) (*mdsrv.Tick, error) {
			return &mdsrv.Tick{InstrumentID: "US.AAPL", Kind: mdsrv.TickKindTrade}, nil
		},
		queryTicker: func(context.Context, string) (*mdsrv.Tick, error) {
			return &mdsrv.Tick{InstrumentID: "US.MSFT", Kind: mdsrv.TickKindQuote}, nil
		},
		getHistoricalCandles: func(context.Context, string, string, string, int, string, string) (mdsrv.CandlesResponse, error) {
			return mdsrv.CandlesResponse{"period": "1m"}, nil
		},
		getDepth: func(context.Context, string, string, int) (mdsrv.DepthResponse, error) {
			return mdsrv.DepthResponse{"asks": 1}, nil
		},
		health: func(context.Context) (mdsrv.HealthStatus, error) {
			return mdsrv.HealthStatus{Connected: true, ActiveCount: 2}, nil
		},
	}

	if markets, err := provider.GetMarkets(ctx); err != nil || len(markets) != 1 || markets[0]["code"] != "US" {
		t.Fatalf("GetMarkets() = %#v err=%v", markets, err)
	}
	if descriptor, err := provider.Descriptor(ctx); err != nil || descriptor.ProviderID != "futu-opend" {
		t.Fatalf("Descriptor() = %+v err=%v", descriptor, err)
	}
	normalized, err := provider.NormalizeInstrument(ctx, map[string]any{"instrumentId": "us.aapl"})
	if err != nil || normalized["instrumentId"] != "US.AAPL" {
		t.Fatalf("NormalizeInstrument() = %#v err=%v", normalized, err)
	}
	if _, err := provider.GetSecurityDetails(ctx, "US", "AAPL"); !errors.Is(err, expectedErr) {
		t.Fatalf("GetSecurityDetails() err=%v, want %v", err, expectedErr)
	}
	if tick, err := provider.QuerySnapshot(ctx, "US.AAPL"); err != nil || tick.InstrumentID != "US.AAPL" {
		t.Fatalf("QuerySnapshot() = %#v err=%v", tick, err)
	}
	if tick, err := provider.QueryTicker(ctx, "US.MSFT"); err != nil || tick.Kind != mdsrv.TickKindQuote {
		t.Fatalf("QueryTicker() = %#v err=%v", tick, err)
	}
	if candles, err := provider.GetHistoricalCandles(ctx, "US", "AAPL", "1m", 10, "", ""); err != nil || candles["period"] != "1m" {
		t.Fatalf("GetHistoricalCandles() = %#v err=%v", candles, err)
	}
	if depth, err := provider.GetDepth(ctx, "US", "AAPL", 5); err != nil || depth["asks"] != 1 {
		t.Fatalf("GetDepth() = %#v err=%v", depth, err)
	}
	if health, err := provider.Health(ctx); err != nil || !health.Connected || health.ActiveCount != 2 {
		t.Fatalf("Health() = %#v err=%v", health, err)
	}

	funds := &broker.FundsSnapshot{AccountID: "acct-1", Market: "US"}
	positions := []broker.PositionSnapshot{{AccountID: "acct-1", Symbol: "US.AAPL", Quantity: 3}}
	orderResult := &broker.PlaceOrderResult{BrokerOrderID: "order-1", Status: "SUBMITTED"}
	reader := &servercoreFakeBrokerReader{funds: funds, positions: positions}
	trading := &servercoreFakeBrokerTrading{result: orderResult}
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
	if err != nil || gotOrder != orderResult || trading.placeQuery.Symbol != "US.AAPL" {
		t.Fatalf("PlaceBrokerOrder() = %#v err=%v trading=%#v", gotOrder, err, trading)
	}

	missingReader := &strategyRuntimeBrokerBridge{broker: servercoreFakeBroker{}}
	if _, err := missingReader.QueryBrokerFunds(ctx, query); err == nil || !strings.Contains(err.Error(), "market data not available") {
		t.Fatalf("missing funds reader error = %v", err)
	}
	if _, err := missingReader.QueryBrokerPositions(ctx, query); err == nil || !strings.Contains(err.Error(), "market data not available") {
		t.Fatalf("missing positions reader error = %v", err)
	}
	if _, err := missingReader.PlaceBrokerOrder(ctx, placeQuery); err == nil || !strings.Contains(err.Error(), "trading not available") {
		t.Fatalf("missing trading error = %v", err)
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

	cutoff := time.Date(2026, time.June, 20, 0, 0, 0, 0, time.UTC)
	if !executionTimestampBefore("2026-06-19T23:59:59Z", cutoff) {
		t.Fatalf("timestamp before cutoff not detected")
	}
	if executionTimestampBefore("", cutoff) || executionTimestampBefore("bad", cutoff) || executionTimestampBefore("2026-06-20T00:00:00Z", cutoff) {
		t.Fatalf("executionTimestampBefore accepted empty/bad/equal timestamp")
	}

	if boolValue(nil) {
		t.Fatalf("nil boolValue = true")
	}
	if value := true; !boolValue(&value) {
		t.Fatalf("true boolValue = false")
	}
	if got := programStatusString(nil); got != "Unavailable" {
		t.Fatalf("nil programStatusString = %q", got)
	}
	statusType := commonpb.ProgramStatusType_ProgramStatusType_NeedPhoneVerifyCode
	status := &commonpb.ProgramStatus{Type: &statusType, StrExtDesc: new("scan QR code")}
	if got := programStatusString(status); !strings.Contains(got, "NeedPhoneVerifyCode: scan QR code") {
		t.Fatalf("programStatusString = %q", got)
	}

	script := defaultStrategyDesignScript(`Quote "Name"`, "pine")
	if !strings.Contains(script, `strategy("Quote \"Name\""`) || !strings.Contains(script, "ta.crossover") {
		t.Fatalf("default strategy script = %q", script)
	}
}

func TestStrategyBindingHelpersNormalizeLooseAPIParams(t *testing.T) {
	binding := normalizeStrategyInstanceBinding(strategyInstanceBinding{}, map[string]any{
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

	fallback := normalizeStrategyInstanceBinding(strategyInstanceBinding{}, map[string]any{
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

	instance := &managedStrategyInstance{Params: map[string]any{
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

func TestServerSidecarBoundaryMethodsAreNilSafe(t *testing.T) {
	var server *Server
	server.SetAPIPort(3001)
	server.ConfigureAuthOrigins("http://127.0.0.1:5173")
	server.SetFrontendFS(os.DirFS(t.TempDir()), "http://127.0.0.1:3000")
	server.ApplySecuritySettings(SecuritySettings{AdminAuthRequired: true})
	if err := server.Close(); err != nil {
		t.Fatalf("nil Close = %v", err)
	}

	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/system/status", nil)
	server.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("nil ServeHTTP status = %d, want 404", recorder.Code)
	}

	empty := &Server{}
	recorder = httptest.NewRecorder()
	empty.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("empty ServeHTTP status = %d, want 404", recorder.Code)
	}
}

func TestServerCloseAggregatesPineWorkerRunnerErrorsOnce(t *testing.T) {
	backtestErr := errors.New("backtest runner stopped with in-flight work")
	instanceErr := errors.New("instance runner transport close failed")
	backtestRunner := &errorClosingPineWorkerRunner{err: backtestErr}
	instanceRunner := &errorClosingPineWorkerRunner{err: instanceErr}
	server := &Server{
		backtestPineWorkerRunner: backtestRunner,
		instancePineWorkerRunner: instanceRunner,
	}

	err := server.Close()
	if err == nil {
		t.Fatal("Close should surface pine worker shutdown errors")
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

	server := &Server{auth: &adminAuth{}}
	server.SetFrontendFS(os.DirFS(frontendDir), " http://127.0.0.1:3000/api/ ")
	server.ApplySecuritySettings(SecuritySettings{AdminAuthRequired: true})
	if server.frontend == nil {
		t.Fatalf("SetFrontendFS did not mount frontend")
	}
	if server.auth == nil || !server.auth.enabled {
		t.Fatalf("ApplySecuritySettings did not enable administrator auth")
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

	server.ApplySecuritySettings(SecuritySettings{})
	if server.auth.enabled {
		t.Fatalf("ApplySecuritySettings should disable administrator auth")
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
	server := &Server{store: store}
	if got := server.brokerExecutionExchange(); got != nil {
		t.Fatalf("brokerExecutionExchange disabled integration = %#v, want nil", got)
	}

	stub := newStrategyRuntimeStubExchange()
	server.strategyRuntimeManager = &strategyRuntimeManager{exchangeProvider: func() strategyRuntimeExchange { return stub }}
	if got := server.brokerExecutionExchange(); got != stub {
		t.Fatalf("brokerExecutionExchange should prefer runtime provider, got %#v", got)
	}

	server.strategyRuntimeManager.exchangeProvider = func() strategyRuntimeExchange { return nil }
	if got := server.brokerExecutionExchange(); got != nil {
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

func directoryExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}
