package productfeatures

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	service "github.com/jftrade/jftrade-main/internal/productfeatures"
	"github.com/jftrade/jftrade-main/pkg/broker"
)

func TestProductFeatureRoutesCoverReadWritePredictionAndSnapshots(t *testing.T) {
	gin.SetMode(gin.TestMode)
	firm := "FUTUINC"
	adapter := &apiFeatureBroker{accounts: []broker.Account{{
		ID: "eligible", SecurityFirm: &firm, MarketAuthorities: []string{"US"},
	}}}
	registry := broker.NewRegistry()
	registry.Register(adapter)
	svc := service.NewService(registry, adapter.ID(), nil, nil)
	svc.SetPredictionQuoteStore(&apiPredictionQuoteStore{})
	router := gin.New()
	RegisterRoutes(router.Group("/api/v1"), svc)

	requests := []struct {
		method string
		path   string
		body   string
	}{
		{http.MethodGet, "/api/v1/brokers/capabilities", ""},
		{http.MethodPost, "/api/v1/market-data/snapshots?brokerId=api-test", `{"instrumentIds":["US.AAPL"]}`},
		{http.MethodGet, "/api/v1/market-data/instruments/US.AAPL/profile?brokerId=api-test&pageSize=5&active=true&ratio=1.5&ids=1&ids=2", ""},
		{http.MethodPost, "/api/v1/market-data/options/analysis/US.AAPL?brokerId=api-test", `{"operation":"strategy"}`},
		{http.MethodGet, "/api/v1/market-data/options/analysis/US.BABA260724C80000?market=US&operation=volatility&brokerId=api-test", ""},
		{http.MethodPost, "/api/v1/market-data/options/events/zero-dte-contracts?brokerId=api-test", `{"market":"US","underlyingInstrumentId":"US.BABA","expiryTimestamp":1784332800,"chain":{"productCode":"BABA","multiplier":100,"contractSize":100,"expirationType":2},"sort":"volume","optionType":"call"}`},
		{http.MethodGet, "/api/v1/research/financials/US.AAPL?brokerId=api-test&refresh=true", ""},
		{http.MethodGet, "/api/v1/market-data/prediction/events?brokerId=api-test&accountId=eligible", ""},
		{http.MethodGet, "/api/v1/market-data/prediction/events/EVENT-1/contracts?brokerId=api-test&accountId=eligible", ""},
		{http.MethodGet, "/api/v1/market-data/prediction/contracts/EVENT/snapshot?brokerId=api-test&accountId=eligible", ""},
		{http.MethodPost, "/api/v1/market-data/prediction/contracts/EVENT/subscriptions?brokerId=api-test&accountId=eligible", `{"dataTypes":["ORDER_BOOK"]}`},
		{http.MethodPost, "/api/v1/market-data/prediction/combos/quotes", `{"brokerId":"api-test","accountId":"eligible","tradingEnvironment":"SIMULATE","mvc":"mvc-1","legs":[{"instrumentId":"US.EC.ONE","productClass":"event_contract","side":"BUY","ratio":1,"predictionSide":"YES"},{"instrumentId":"US.EC.TWO","productClass":"event_contract","side":"BUY","ratio":1,"predictionSide":"NO"}]}`},
		{http.MethodPost, "/api/v1/alerts/price?brokerId=api-test", `{"symbol":"US.AAPL","price":100}`},
	}
	var leaseID string
	for _, request := range requests {
		rec := performFeatureRequest(t, router, request.method, request.path, request.body)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s %s status=%d body=%s", request.method, request.path, rec.Code, rec.Body.String())
		}
		if strings.Contains(request.path, "/subscriptions") {
			leaseID = responseStringField(rec.Body.String(), "leaseId")
		}
		if strings.Contains(request.path, "/options/events/zero-dte-contracts") {
			locator, ok := adapter.lastQuery.Params["chainLocator"].(broker.OptionZeroDteChainLocator)
			if !ok || locator.ProductCode != "BABA" ||
				adapter.lastQuery.InstrumentID != "US.BABA" {
				t.Fatalf("0DTE query = %#v", adapter.lastQuery)
			}
		}
	}
	if leaseID == "" {
		t.Fatal("subscription response did not contain leaseId")
	}
	released := performFeatureRequest(
		t, router, http.MethodDelete,
		"/api/v1/market-data/prediction/contracts/EVENT/subscriptions/"+leaseID, "",
	)
	if released.Code != http.StatusOK || adapter.unsubscribeCalls != 1 {
		t.Fatalf("release status=%d body=%s unsubscribe=%d", released.Code, released.Body.String(), adapter.unsubscribeCalls)
	}
}

func TestProductFeatureRoutesMapValidationCapabilityEligibilityAndBrokerErrors(t *testing.T) {
	gin.SetMode(gin.TestMode)
	adapter := &apiFeatureBroker{}
	registry := broker.NewRegistry()
	registry.Register(adapter)
	svc := service.NewService(registry, adapter.ID(), nil, nil)
	router := gin.New()
	RegisterRoutes(router.Group("/api/v1"), svc)

	cases := []struct {
		method string
		path   string
		body   string
		status int
	}{
		{http.MethodPost, "/api/v1/market-data/snapshots", `{`, http.StatusBadRequest},
		{http.MethodPost, "/api/v1/market-data/snapshots", `{"instrumentIds":[]}`, http.StatusBadRequest},
		{http.MethodPost, "/api/v1/alerts/price", `{`, http.StatusBadRequest},
		{http.MethodPost, "/api/v1/market-data/options/analysis/US.AAPL", `{`, http.StatusBadRequest},
		{http.MethodGet, "/api/v1/market-data/options/analysis/US.BABA?market=US&operation=volatility", "", http.StatusBadRequest},
		{http.MethodGet, "/api/v1/market-data/options/analysis/US.BABA260724C80000?market=US&operation=underlying_overview", "", http.StatusBadRequest},
		{http.MethodGet, "/api/v1/market-data/options/events?market=US&operation=zero_dte_contract&underlying=US.BABA", "", http.StatusBadRequest},
		{http.MethodPost, "/api/v1/market-data/options/events/zero-dte-contracts", `{`, http.StatusBadRequest},
		{http.MethodPost, "/api/v1/market-data/options/events/zero-dte-contracts", `{"market":"US","underlyingInstrumentId":"US.BABA","expiryTimestamp":1784332800,"chain":{"productCode":"BABA"},"sort":"gamma"}`, http.StatusBadRequest},
		{http.MethodGet, "/api/v1/market-data/options/events?market=HK&operation=zero_dte", "", http.StatusUnprocessableEntity},
		{http.MethodPost, "/api/v1/market-data/prediction/contracts/EVENT/subscriptions", `{`, http.StatusBadRequest},
		{http.MethodPost, "/api/v1/market-data/prediction/combos/quotes", `{`, http.StatusBadRequest},
		{http.MethodGet, "/api/v1/market-data/instruments/US.AAPL/profile?brokerId=missing", "", http.StatusConflict},
		{http.MethodGet, "/api/v1/market-data/prediction/events?brokerId=api-test", "", http.StatusForbidden},
	}
	for _, test := range cases {
		rec := performFeatureRequest(t, router, test.method, test.path, test.body)
		if rec.Code != test.status {
			t.Errorf("%s %s status=%d want=%d body=%s", test.method, test.path, rec.Code, test.status, rec.Body.String())
		}
	}

	adapter.queryErr = errors.New("upstream failed")
	rec := performFeatureRequest(
		t, router, http.MethodGet,
		"/api/v1/market-data/instruments/US.AAPL/profile?brokerId=api-test", "",
	)
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("broker error status=%d body=%s", rec.Code, rec.Body.String())
	}

	adapter.queryErr = nil
	adapter.snapshotErr = errors.New("snapshot failed")
	rec = performFeatureRequest(t, router, http.MethodPost, "/api/v1/market-data/snapshots", `{"symbols":["US.AAPL"]}`)
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("snapshot broker error status=%d body=%s", rec.Code, rec.Body.String())
	}
	adapter.snapshotErr = broker.NewSnapshotRateLimitError(6500*time.Millisecond, nil)
	rec = performFeatureRequest(t, router, http.MethodPost, "/api/v1/market-data/snapshots", `{"symbols":["US.AAPL"]}`)
	if rec.Code != http.StatusTooManyRequests || rec.Header().Get("Retry-After") != "7" || !strings.Contains(rec.Body.String(), "MARKET_SNAPSHOT_RATE_LIMITED") {
		t.Fatalf("snapshot rate limit status=%d retry=%q body=%s", rec.Code, rec.Header().Get("Retry-After"), rec.Body.String())
	}
	adapter.snapshotErr = broker.ErrSnapshotRateLimited
	rec = performFeatureRequest(t, router, http.MethodPost, "/api/v1/market-data/snapshots", `{"symbols":["US.AAPL"]}`)
	if rec.Code != http.StatusTooManyRequests || rec.Header().Get("Retry-After") != "1" {
		t.Fatalf("snapshot default retry status=%d retry=%q body=%s", rec.Code, rec.Header().Get("Retry-After"), rec.Body.String())
	}
	adapter.snapshotErr = nil
	adapter.customizationErr = errors.New("write failed")
	rec = performFeatureRequest(t, router, http.MethodPost, "/api/v1/alerts/price", `{"price":100}`)
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("customization error status=%d body=%s", rec.Code, rec.Body.String())
	}
	adapter.customizationErr = nil
	firm := "FUTUINC"
	adapter.accounts = []broker.Account{{
		ID: "eligible", SecurityFirm: &firm, MarketAuthorities: []string{"US"},
	}}
	adapter.subscribeErr = errors.New("subscribe failed")
	rec = performFeatureRequest(
		t, router, http.MethodPost,
		"/api/v1/market-data/prediction/contracts/EVENT/subscriptions?accountId=eligible",
		`{"dataTypes":["ORDER_BOOK"]}`,
	)
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("subscribe error status=%d body=%s", rec.Code, rec.Body.String())
	}
	adapter.subscribeErr = nil
	rec = performFeatureRequest(
		t, router, http.MethodPost,
		"/api/v1/market-data/prediction/contracts/EVENT/subscriptions?accountId=eligible",
		`{"dataTypes":["ORDER_BOOK"]}`,
	)
	leaseID := responseStringField(rec.Body.String(), "leaseId")
	adapter.unsubscribeErr = errors.New("unsubscribe failed")
	rec = performFeatureRequest(
		t, router, http.MethodDelete,
		"/api/v1/market-data/prediction/contracts/EVENT/subscriptions/"+leaseID, "",
	)
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("unsubscribe error status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestProductFeatureRouteHelpers(t *testing.T) {
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest(http.MethodGet, "/?flag=false&integer=12&number=1.5&text=value", nil)
	params := queryParameters(ctx)
	if params["flag"] != false || params["integer"] != int64(12) || params["number"] != 1.5 || params["text"] != "value" {
		t.Fatalf("queryParameters() = %#v", params)
	}
	if got := firstQueryValue(ctx, "missing"); got != "" {
		t.Fatalf("firstQueryValue() = %q", got)
	}
	if got := firstPathValue(ctx, "missing"); got != "" {
		t.Fatalf("firstPathValue() = %q", got)
	}
	if got := marketFromInstrument("SG.D05"); got != "" {
		t.Fatalf("marketFromInstrument(SG.D05) = %q", got)
	}
	if got := predictionRoute("events"); got.defaultMarket != "US" {
		t.Fatalf("predictionRoute() = %#v", got)
	}
}

func performFeatureRequest(
	t *testing.T,
	router http.Handler,
	method string,
	path string,
	body string,
) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(t.Context(), method, path, strings.NewReader(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	router.ServeHTTP(rec, req)
	return rec
}

func responseStringField(body, field string) string {
	marker := `"` + field + `":"`
	_, after, ok := strings.Cut(body, marker)
	if !ok {
		return ""
	}
	value := after
	if before, _, ok := strings.Cut(value, `"`); ok {
		return before
	}
	return ""
}

type apiFeatureBroker struct {
	accounts         []broker.Account
	queryErr         error
	snapshotErr      error
	customizationErr error
	subscribeErr     error
	unsubscribeErr   error
	unsubscribeCalls int
	lastQuery        broker.FeatureQuery
}

func (b *apiFeatureBroker) ID() string { return "api-test" }
func (b *apiFeatureBroker) Descriptor() broker.Descriptor {
	features := make([]broker.FeatureCapability, 0, len(broker.BuiltinCapabilityCatalog.Features))
	for _, definition := range broker.BuiltinCapabilityCatalog.Features {
		features = append(features, broker.FeatureCapability{
			ID: definition.ID, Markets: []string{"US"}, Access: definition.Access,
			State: broker.CapabilityAvailable,
		})
	}
	return broker.Descriptor{
		ID: b.ID(), SecurityFirm: "Moomoo US",
		Capabilities: []broker.MarketCapability{{Market: "US", Features: features}},
	}
}
func (b *apiFeatureBroker) DiscoverAccounts(context.Context) ([]broker.Account, error) {
	return b.accounts, nil
}
func (b *apiFeatureBroker) Trading() broker.TradingService      { return nil }
func (b *apiFeatureBroker) MarketData() broker.MarketDataReader { return nil }
func (b *apiFeatureBroker) result(query broker.FeatureQuery) (*broker.FeatureResult, error) {
	b.lastQuery = query
	if b.queryErr != nil {
		return nil, b.queryErr
	}
	result := &broker.FeatureResult{
		Entries: []map[string]any{{"feature": query.FeatureID}},
		AsOf:    time.Now().UTC(),
	}
	if query.FeatureID == broker.FeaturePredictionComboQuote {
		result.Metadata = map[string]any{"quoteId": "quote-api", "bidPrice": 0.4, "askPrice": 0.45}
	}
	return result, nil
}
func (b *apiFeatureBroker) QueryMarketMicrostructure(ctx context.Context, q broker.FeatureQuery) (*broker.FeatureResult, error) {
	return b.result(q)
}
func (b *apiFeatureBroker) QueryInstrumentProfile(ctx context.Context, q broker.FeatureQuery) (*broker.FeatureResult, error) {
	return b.result(q)
}
func (b *apiFeatureBroker) QueryDerivativeCatalog(ctx context.Context, q broker.FeatureQuery) (*broker.FeatureResult, error) {
	return b.result(q)
}
func (b *apiFeatureBroker) QueryOptionAnalytics(ctx context.Context, q broker.FeatureQuery) (*broker.FeatureResult, error) {
	return b.result(q)
}
func (b *apiFeatureBroker) QueryInstrumentResearch(ctx context.Context, q broker.FeatureQuery) (*broker.FeatureResult, error) {
	return b.result(q)
}
func (b *apiFeatureBroker) QueryMarketResearch(ctx context.Context, q broker.FeatureQuery) (*broker.FeatureResult, error) {
	return b.result(q)
}
func (b *apiFeatureBroker) QueryPredictionMarket(ctx context.Context, q broker.FeatureQuery) (*broker.FeatureResult, error) {
	return b.result(q)
}
func (b *apiFeatureBroker) QueryTechnicalIndicator(ctx context.Context, q broker.FeatureQuery) (*broker.FeatureResult, error) {
	return b.result(q)
}
func (b *apiFeatureBroker) QueryCustomization(ctx context.Context, q broker.FeatureQuery) (*broker.FeatureResult, error) {
	return b.result(q)
}
func (b *apiFeatureBroker) ApplyCustomization(context.Context, broker.CustomizationAction) (*broker.CustomizationResult, error) {
	if b.customizationErr != nil {
		return nil, b.customizationErr
	}
	return &broker.CustomizationResult{Entries: []map[string]any{{"accepted": true}}}, nil
}
func (b *apiFeatureBroker) QuerySecuritySnapshot(
	_ context.Context,
	query broker.SecuritySnapshotQuery,
) (*broker.SecuritySnapshotResult, error) {
	if b.snapshotErr != nil {
		return nil, b.snapshotErr
	}
	return &broker.SecuritySnapshotResult{Snapshots: []broker.SecuritySnapshotItem{{
		Symbol: query.Symbols[0], ObservedAt: time.Now().UTC(),
	}}}, nil
}
func (b *apiFeatureBroker) SubscribePredictionMarket(context.Context, broker.PredictionSubscription) error {
	return b.subscribeErr
}
func (b *apiFeatureBroker) UnsubscribePredictionMarket(context.Context, broker.PredictionSubscription) error {
	b.unsubscribeCalls++
	return b.unsubscribeErr
}

type apiPredictionQuoteStore struct{}

func (*apiPredictionQuoteStore) SavePredictionQuote(context.Context, broker.PredictionQuoteRecord) error {
	return nil
}
func (*apiPredictionQuoteStore) ValidatePredictionQuote(
	context.Context, string, string, string, string, string, string,
) (broker.PredictionQuoteRecord, error) {
	return broker.PredictionQuoteRecord{}, nil
}
func (*apiPredictionQuoteStore) ConsumePredictionQuote(
	context.Context, string, string, string, string, string, string, string, string,
) error {
	return nil
}
