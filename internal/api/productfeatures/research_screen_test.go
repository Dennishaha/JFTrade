package productfeatures

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	service "github.com/jftrade/jftrade-main/internal/productfeatures"
	"github.com/jftrade/jftrade-main/pkg/broker"
)

func TestResearchScreenCatalogRouteAndValidation(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/catalog", handleResearchScreenCatalog())

	rec := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/catalog?brokerId=futu&market=US", nil)
	router.ServeHTTP(rec, request)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"version":"futu-stock-screen-v1"`) ||
		!strings.Contains(rec.Body.String(), `"factor":"`) && !strings.Contains(rec.Body.String(), `"factors":`) {
		t.Fatalf("catalog status=%d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"currencyBasis":"quote"`) ||
		!strings.Contains(rec.Body.String(), `"displayFormat":"compact_amount"`) {
		t.Fatalf("catalog omitted display semantics: %s", rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "providerId") {
		t.Fatalf("catalog leaked provider IDs: %s", rec.Body.String())
	}

	rec = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodGet, "/catalog?market=SG", nil)
	router.ServeHTTP(rec, request)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("invalid market status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestNormalizeResearchScreenQueryDefaultsAndRejectsNonV2Input(t *testing.T) {
	query := broker.ScreenQueryV2{
		ScreenDefinitionV2: broker.ScreenDefinitionV2{
			BrokerID: " FUTU ", Market: "us",
			CatalogVersion:     "futu-stock-screen-v1",
			QuerySchemaVersion: broker.ScreenQuerySchemaVersionV2,
			Columns: []broker.ScreenColumn{{
				ID: "price", Factor: broker.FactorRef{InstanceID: "price", FactorKey: "simple.price"},
			}},
		},
	}
	if err := normalizeResearchScreenQuery(&query); err != nil {
		t.Fatal(err)
	}
	if query.BrokerID != "futu" || query.Market != "US" || query.Page.Limit != 50 {
		t.Fatalf("normalized query = %#v", query)
	}
	query.QuerySchemaVersion = 1
	if err := normalizeResearchScreenQuery(&query); err == nil {
		t.Fatal("V1 query schema was accepted")
	}
}

func TestWriteResearchScreenErrorReturnsStructured429(t *testing.T) {
	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	writeResearchScreenError(ctx, broker.NewResearchScreenRateLimitError(2500*time.Millisecond))
	if rec.Code != http.StatusTooManyRequests || rec.Header().Get("Retry-After") != "3" ||
		!strings.Contains(rec.Body.String(), "RESEARCH_SCREEN_RATE_LIMITED") {
		t.Fatalf("status=%d retry=%q body=%s", rec.Code, rec.Header().Get("Retry-After"), rec.Body.String())
	}
}

func TestResearchScreenPostUsesTypedDefinitionAndOffset(t *testing.T) {
	adapter := &apiFeatureBroker{}
	registry := broker.NewRegistry()
	registry.Register(adapter)
	svc := service.NewService(registry, adapter.ID(), nil, nil)
	router := gin.New()
	RegisterRoutes(router.Group("/api/v1"), svc)

	rec := performFeatureRequest(
		t, router, http.MethodPost,
		"/api/v1/research/screens",
		`{
			"brokerId":"api-test",
			"market":"US",
			"catalogVersion":"futu-stock-screen-v1",
			"querySchemaVersion":2,
			"conditions":[{
				"id":"price-filter",
				"factor":{"instanceId":"price-filter","factorKey":"simple.price"},
				"operator":"between",
				"value":{"min":10}
			}],
			"columns":[{
				"columnId":"price-column",
				"factor":{"instanceId":"price-column","factorKey":"simple.price"}
			}],
			"sorts":[{
				"sortId":"market-cap-sort",
				"factor":{"instanceId":"market-cap-sort","factorKey":"simple.market_cap"},
				"direction":"desc"
			}],
			"page":{"offset":50,"limit":25}
		}`,
	)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if adapter.lastQuery.FeatureID != broker.FeatureResearchScreen ||
		adapter.lastQuery.Params["operation"] != "stock_v2" ||
		adapter.lastQuery.Params["pageFrom"] != 50 ||
		adapter.lastQuery.Cursor != "50" || adapter.lastQuery.PageSize != 25 {
		t.Fatalf("query = %#v", adapter.lastQuery)
	}
	definition, ok := adapter.lastQuery.Params["researchScreenDefinition"].(broker.ScreenDefinitionV2)
	if !ok || len(definition.Conditions) != 1 ||
		definition.Conditions[0].Factor.FactorKey != "simple.price" {
		t.Fatalf("typed definition = %#v", adapter.lastQuery.Params["researchScreenDefinition"])
	}
}

func TestResearchScreenPostPreservesExecutableV2Definition(t *testing.T) {
	adapter := &apiFeatureBroker{}
	registry := broker.NewRegistry()
	registry.Register(adapter)
	svc := service.NewService(registry, adapter.ID(), nil, nil)
	router := gin.New()
	RegisterRoutes(router.Group("/api/v1"), svc)

	rec := performFeatureRequest(
		t, router, http.MethodPost,
		"/api/v1/research/screens",
		`{
			"brokerId":"api-test",
			"market":"US",
			"catalogVersion":"futu-stock-screen-v1",
			"querySchemaVersion":2,
			"conditions":[{
				"id":"price-range",
				"factor":{"instanceId":"price-filter","factorKey":"simple.price"},
				"operator":"between",
				"value":{"min":10.5,"minIncludes":false,"max":120,"maxIncludes":true}
			}],
			"columns":[{
				"columnId":"price-column",
				"factor":{"instanceId":"price-result","factorKey":"simple.price"},
				"label":"最新价"
			}],
			"page":{"limit":25}
		}`,
	)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}

	definition, ok := adapter.lastQuery.Params["researchScreenDefinition"].(broker.ScreenDefinitionV2)
	if !ok {
		t.Fatalf("typed definition = %#v", adapter.lastQuery.Params["researchScreenDefinition"])
	}
	if definition.BrokerID != "api-test" || definition.Market != "US" ||
		definition.QuerySchemaVersion != broker.ScreenQuerySchemaVersionV2 ||
		len(definition.Conditions) != 1 || len(definition.Columns) != 1 {
		t.Fatalf("normalized V2 definition = %#v", definition)
	}
	value, ok := definition.Conditions[0].Value.(map[string]any)
	if !ok || value["min"] != 10.5 || value["minIncludes"] != false ||
		value["max"] != float64(120) || value["maxIncludes"] != true {
		t.Fatalf("V2 interval value = %#v", definition.Conditions[0].Value)
	}
	if !strings.Contains(rec.Body.String(), `"catalogVersion":"futu-stock-screen-v1"`) ||
		!strings.Contains(rec.Body.String(), `"columnId":"price-column"`) ||
		!strings.Contains(rec.Body.String(), `"instanceId":"price-result"`) {
		t.Fatalf("V2 result metadata missing: %s", rec.Body.String())
	}
}

func TestResearchScreenPostRejectsV1Payload(t *testing.T) {
	adapter := &apiFeatureBroker{}
	registry := broker.NewRegistry()
	registry.Register(adapter)
	svc := service.NewService(registry, adapter.ID(), nil, nil)
	router := gin.New()
	RegisterRoutes(router.Group("/api/v1"), svc)

	rec := performFeatureRequest(
		t, router, http.MethodPost,
		"/api/v1/research/screens",
		`{
			"brokerId":"api-test",
			"market":"US",
			"catalogVersion":"futu-stock-screen-v1",
			"querySchemaVersion":2,
			"filters":[{"factor":"simple.price","min":{"value":10}}],
			"columns":[{"factor":"simple.price"}]
		}`,
	)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("V1 payload status=%d body=%s", rec.Code, rec.Body.String())
	}
	if adapter.lastQuery.FeatureID != "" {
		t.Fatalf("V1 payload reached broker: %#v", adapter.lastQuery)
	}
}

func TestTypedResearchScreenResultOmitsUnknownTotal(t *testing.T) {
	hasMore := false
	typed, err := typedResearchScreenResult(&broker.FeatureResult{
		Entries:  []map[string]any{},
		HasMore:  &hasMore,
		Warnings: []string{"combined A-share total is not exact"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if typed.Total != nil {
		t.Fatalf("unknown total = %#v, want nil", typed.Total)
	}
	content, err := json.Marshal(typed)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(content), `"total"`) {
		t.Fatalf("unknown total was serialized: %s", content)
	}

	total := 7
	typed, err = typedResearchScreenResult(&broker.FeatureResult{
		Entries: []map[string]any{},
		Total:   &total,
	})
	if err != nil {
		t.Fatal(err)
	}
	if typed.Total == nil || *typed.Total != total {
		t.Fatalf("known total = %#v, want %d", typed.Total, total)
	}

	typed, err = typedResearchScreenResult(&broker.FeatureResult{
		Entries: []map[string]any{{
			"stockId":       "80700",
			"instrumentId":  "HK.80700",
			"market":        "HK",
			"symbol":        "80700",
			"name":          "腾讯控股-R",
			"quoteCurrency": "CNY",
			"productClass":  broker.ProductClassEquity,
			"values":        map[string]broker.ResearchScreenValue{},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(typed.Entries) != 1 || typed.Entries[0].QuoteCurrency != "CNY" {
		t.Fatalf("typed quote currency = %#v", typed.Entries)
	}
}
