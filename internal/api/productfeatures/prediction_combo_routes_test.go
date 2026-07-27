package productfeatures

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	service "github.com/jftrade/jftrade-main/internal/productfeatures"
	"github.com/jftrade/jftrade-main/pkg/broker"
)

func TestPredictionComboQuoteAcceptsContextFromQueryAndMapsFailures(t *testing.T) {
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
	legs := `"mvc":"mvc-1","legs":[` +
		`{"instrumentId":"US.EC.ONE","productClass":"event_contract","side":"BUY","ratio":1,"predictionSide":"YES"},` +
		`{"instrumentId":"US.EC.TWO","productClass":"event_contract","side":"BUY","ratio":1,"predictionSide":"NO"}]`

	rec := performFeatureRequest(
		t,
		router,
		http.MethodPost,
		"/api/v1/market-data/prediction/combos/quotes?brokerId=api-test&accountId=eligible&tradingEnvironment=SIMULATE",
		"{"+legs+"}",
	)
	if rec.Code != http.StatusOK {
		t.Fatalf("query-context RFQ status=%d body=%s", rec.Code, rec.Body.String())
	}
	rec = performFeatureRequest(
		t,
		router,
		http.MethodPost,
		"/api/v1/market-data/prediction/combos/quotes",
		`{"brokerId":"api-test"}`,
	)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("invalid RFQ status=%d body=%s", rec.Code, rec.Body.String())
	}
	adapter.queryErr = errAPIUpstream
	rec = performFeatureRequest(
		t,
		router,
		http.MethodPost,
		"/api/v1/market-data/prediction/combos/quotes?brokerId=api-test&accountId=eligible&tradingEnvironment=SIMULATE",
		"{"+legs+"}",
	)
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("failed RFQ status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestPostQueryMapsUpstreamFailureAndRouteHelpersCoverAcceptedEnums(t *testing.T) {
	gin.SetMode(gin.TestMode)
	adapter := &apiFeatureBroker{queryErr: errAPIUpstream}
	registry := broker.NewRegistry()
	registry.Register(adapter)
	router := gin.New()
	RegisterRoutes(router.Group("/api/v1"), service.NewService(registry, adapter.ID(), nil, nil))
	rec := performFeatureRequest(
		t,
		router,
		http.MethodPost,
		"/api/v1/market-data/options/analysis/US.AAPL?brokerId=api-test",
		`{"operation":"strategy"}`,
	)
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("failed post query status=%d body=%s", rec.Code, rec.Body.String())
	}

	for _, segment := range []broker.MarketSegment{
		broker.MarketSegmentSecurities,
		broker.MarketSegmentDerivatives,
		broker.MarketSegmentPrediction,
	} {
		if got := firstMarketSegment(string(segment), "fallback"); got != segment {
			t.Fatalf("firstMarketSegment(%s) = %s", segment, got)
		}
	}
	for _, productClass := range []broker.ProductClass{
		broker.ProductClassEquity,
		broker.ProductClassFund,
		broker.ProductClassOption,
		broker.ProductClassWarrant,
		broker.ProductClassCBBC,
		broker.ProductClassFuture,
		broker.ProductClassEventContract,
		broker.ProductClassIndex,
		broker.ProductClassBond,
		broker.ProductClassPlate,
	} {
		if got := firstProductClass(string(productClass), broker.ProductClassUnknown); got != productClass {
			t.Fatalf("firstProductClass(%s) = %s", productClass, got)
		}
	}

	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest(
		http.MethodGet,
		"/?underlying=%20us.aapl%20&operation=current&marketSegment=derivatives&productClass=option",
		strings.NewReader(""),
	)
	query := routeQuery(ctx, queryRoute{
		feature: broker.FeatureOptionAnalysis, operation: "fallback", defaultMarket: "HK",
	})
	if query.InstrumentID != "US.AAPL" ||
		query.Market != "US" ||
		query.Params["operation"] != "current" ||
		query.MarketSegment != broker.MarketSegmentDerivatives ||
		query.ProductClass != broker.ProductClassOption {
		t.Fatalf("route query = %#v", query)
	}
}

var errAPIUpstream = &apiRouteError{"upstream failed"}

type apiRouteError struct {
	message string
}

func (e *apiRouteError) Error() string {
	return e.message
}
