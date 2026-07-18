package broker

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestPredictionQuoteLegsHashNormalizesBrokerNeutralLegs(t *testing.T) {
	amount := 10.0
	left := PredictionQuoteLegsHash(" mvc ", []OrderLegIntent{{
		InstrumentID: " us.ec.one ", ProductClass: ProductClassEventContract,
		Side: " buy ", Ratio: 1, Amount: &amount, PredictionSide: " yes ",
	}})
	right := PredictionQuoteLegsHash("mvc", []OrderLegIntent{{
		InstrumentID: "US.EC.ONE", ProductClass: ProductClassEventContract,
		Side: "BUY", Ratio: 1, Amount: &amount, PredictionSide: "YES",
	}})
	if left == "" || left != right {
		t.Fatalf("normalized leg hashes = %q and %q", left, right)
	}
}

func TestCapabilityOperationSurfaceFallbacksAndOverrides(t *testing.T) {
	operations := capabilityOperations(
		FeatureID("custom.read"), "GET", "/api/custom", "/workspace?surface=custom",
		"custom.read", "TestCustom",
	)
	if len(operations) != 1 || operations[0].ID != "custom.read" {
		t.Fatalf("protocol-free operation = %#v", operations)
	}
	history := capabilityOperations(
		FeaturePredictionHistory, "GET", "/api/history", "/workspace",
		"prediction.history", "TestHistory",
	)
	var subscribe CapabilityOperation
	for _, operation := range history {
		if operation.ID == "subscribe" {
			subscribe = operation
		}
	}
	if subscribe.HTTPMethod != "POST" || subscribe.Tool != "" ||
		subscribe.UISurfaceID != "workspace.chart" {
		t.Fatalf("subscription override = %#v", subscribe)
	}
	for route, expected := range map[string]string{
		"":                              "",
		"/workspace":                    "workspace.root",
		"/workspace?tab=options":        "workspace.options",
		"/workspace?surface=order":      "workspace.order",
		"/research":                     "research.market",
		"/research?section=macro":       "research.macro",
		"/watchlist":                    "watchlist.root",
		"/":                             "app.root",
		"/settings/integrations?x=true": "settings.integrations.x.true",
		"%zz":                           "%zz",
	} {
		if got := capabilityUISurfaceID(route); got != expected {
			t.Errorf("surface %q = %q, want %q", route, got, expected)
		}
	}
	if capabilityHTTPMethod(FeaturePredictionComboQuote, "GET") != "POST" ||
		capabilityHTTPMethod(FeatureMarketSearch, "GET") != "GET" {
		t.Fatal("capability HTTP method override changed")
	}

	customFeature := FeatureID("custom.hidden")
	capabilityProtocolSpecs[customFeature] = map[string][]catalogProtocolSpec{
		"hidden": {{key: "Hidden", id: 1, kind: "request"}},
	}
	capabilityOperationSurfaceOverrides[customFeature] = map[string]capabilityOperationSurfaceOverride{
		"hidden": {noUI: true, noTool: true},
	}
	t.Cleanup(func() {
		delete(capabilityProtocolSpecs, customFeature)
		delete(capabilityOperationSurfaceOverrides, customFeature)
	})
	hidden := capabilityOperations(
		customFeature, "GET", "/api/hidden", "/workspace",
		"custom.hidden", "TestHidden",
	)
	if len(hidden) != 1 || hidden[0].UISurfaceID != "" || hidden[0].Tool != "" {
		t.Fatalf("hidden operation override = %#v", hidden)
	}
}

func TestCapabilityCatalogOperationValidationFailures(t *testing.T) {
	valid := CapabilityDefinition{
		ID: FeatureMarketSearch, AdapterInterface: "Reader",
		Access: FeatureAccessRead, Permission: PermissionReadOnly,
		Approval:    ApprovalNone,
		Surface:     CapabilitySurface{API: "/search", Tool: "market.search"},
		TestMapping: "TestSearch",
		Operations: []CapabilityOperation{{
			ID: "search", HTTPMethod: "GET", API: "/search",
			Tool: "market.search", TestID: "TestSearch/search",
		}},
	}
	cases := []CapabilityDefinition{
		func() CapabilityDefinition { value := valid; value.Operations = nil; return value }(),
		func() CapabilityDefinition {
			value := valid
			value.Operations = []CapabilityOperation{{}}
			return value
		}(),
		func() CapabilityDefinition {
			value := valid
			value.Operations = append(value.Operations, value.Operations[0])
			return value
		}(),
		func() CapabilityDefinition {
			value := valid
			value.Operations[0].HTTPMethod = ""
			return value
		}(),
		func() CapabilityDefinition {
			value := valid
			value.Operations[0].Tool = ""
			value.Operations[0].UISurfaceID = ""
			return value
		}(),
	}
	for index, feature := range cases {
		catalog := CapabilityCatalog{Version: "test", Features: []CapabilityDefinition{feature}}
		if err := catalog.Validate(); err == nil {
			t.Errorf("invalid operation case %d validated", index)
		}
	}
	complete := CapabilityDefinition{
		ID: FeatureMarketSearch, AdapterInterface: "Reader",
		Access: FeatureAccessRead, Permission: PermissionReadOnly,
		Approval:    ApprovalNone,
		Surface:     CapabilitySurface{API: "/search", Tool: "market.search"},
		TestMapping: "TestSearch",
		Operations: []CapabilityOperation{{
			ID: "search", HTTPMethod: "GET", API: "/search",
			Tool: "market.search", TestID: "TestSearch/search",
		}},
	}
	if err := (CapabilityCatalog{
		Version:  "test",
		Features: []CapabilityDefinition{complete, complete},
	}).Validate(); err == nil || !strings.Contains(err.Error(), "duplicate capability") {
		t.Fatalf("duplicate feature validation = %v", err)
	}
	withoutSurface := complete
	withoutSurface.Operations = append([]CapabilityOperation(nil), complete.Operations...)
	withoutSurface.Operations[0].Tool = ""
	withoutSurface.Operations[0].UISurfaceID = ""
	if err := (CapabilityCatalog{
		Version: "test", Features: []CapabilityDefinition{withoutSurface},
	}).Validate(); err == nil || !strings.Contains(err.Error(), "neither UI nor tool") {
		t.Fatalf("missing operation surface validation = %v", err)
	}
}

func TestBrokerFeatureRouterRejectsDeclaredFeatureWithoutAdapterInterface(t *testing.T) {
	capability := FeatureCapability{
		ID: FeatureOptionAnalysis, Markets: []string{"US"},
		Access: FeatureAccessRead, State: CapabilityAvailable,
	}
	registry := NewRegistry()
	registry.Register(&runtimeCapabilityBroker{id: "partial", capability: capability})
	_, err := NewBrokerFeatureRouter(registry, "partial", nil).Resolve(
		FeatureRouteRequest{BrokerID: "partial", FeatureID: FeatureOptionAnalysis, Market: "US"},
	)
	if err == nil || !strings.Contains(err.Error(), "OptionAnalyticsReader") {
		t.Fatalf("missing adapter interface resolution = %v", err)
	}
}

func TestBrokerFeatureRouterRuntimeEvaluatorFailuresAndReasons(t *testing.T) {
	now := time.Date(2026, 7, 18, 0, 0, 0, 0, time.UTC)
	capability := FeatureCapability{
		ID: FeatureMarketSnapshot, Markets: []string{"US"},
		Access: FeatureAccessRead, State: CapabilityAvailable,
		RequiresConnection: true, RequiresAccount: true, RequiresQuoteRight: true,
	}
	registry := NewRegistry()
	failing := &runtimeCapabilityBroker{
		id: "failing", capability: capability, evaluationErr: errors.New("probe failed"),
	}
	registry.Register(failing)
	router := NewBrokerFeatureRouter(registry, "failing", nil)
	router.now = func() time.Time { return now }
	if _, err := router.ResolveContext(t.Context(), FeatureRouteRequest{
		FeatureID: FeatureMarketSnapshot, Market: "US",
	}); err == nil || !strings.Contains(err.Error(), "probe failed") {
		t.Fatalf("runtime evaluator error = %v", err)
	}

	unavailable := &runtimeCapabilityBroker{
		id: "unavailable", capability: capability,
		evaluation: CapabilityEvaluation{
			State:      CapabilityUnavailable,
			Connection: CapabilityCheck{State: CapabilityAvailable},
			Account: CapabilityCheck{
				State: CapabilityUnavailable, Reason: "account denied",
			},
			QuoteRight: CapabilityCheck{State: CapabilityAvailable},
		},
	}
	registry.Register(unavailable)
	if _, err := NewBrokerFeatureRouter(registry, "", nil).Resolve(
		FeatureRouteRequest{
			BrokerID: "unavailable", FeatureID: FeatureMarketSnapshot, Market: "US",
		},
	); err == nil || !strings.Contains(err.Error(), "account denied") {
		t.Fatalf("runtime unavailable error = %v", err)
	}
	unavailable.evaluation.Reason = "top-level reason"
	if _, err := NewBrokerFeatureRouter(registry, "", nil).Resolve(
		FeatureRouteRequest{
			BrokerID: "unavailable", FeatureID: FeatureMarketSnapshot, Market: "US",
		},
	); err == nil || !strings.Contains(err.Error(), "top-level reason") {
		t.Fatalf("top-level runtime reason = %v", err)
	}
	unavailable.evaluation.Reason = ""
	unavailable.evaluation.Account.Reason = ""
	if _, err := NewBrokerFeatureRouter(registry, "", nil).Resolve(
		FeatureRouteRequest{
			BrokerID: "unavailable", FeatureID: FeatureMarketSnapshot, Market: "US",
		},
	); err == nil || !strings.Contains(err.Error(), "runtime capability is unavailable") {
		t.Fatalf("fallback runtime reason = %v", err)
	}

	static := defaultCapabilityEvaluation(capability, now)
	if static.State != CapabilityDegraded ||
		static.Connection.Code != "RUNTIME_STATUS_UNKNOWN" ||
		static.Account.Code != "RUNTIME_STATUS_UNKNOWN" ||
		static.QuoteRight.Code != "RUNTIME_STATUS_UNKNOWN" {
		t.Fatalf("default runtime evaluation = %#v", static)
	}
	capability.State = CapabilityDegraded
	static = defaultCapabilityEvaluation(capability, now)
	if static.State != CapabilityDegraded {
		t.Fatalf("declared degraded evaluation = %#v", static)
	}
}

type runtimeCapabilityBroker struct {
	id            string
	capability    FeatureCapability
	evaluation    CapabilityEvaluation
	evaluationErr error
}

func (b *runtimeCapabilityBroker) ID() string { return b.id }

func (b *runtimeCapabilityBroker) Descriptor() Descriptor {
	return Descriptor{
		ID: b.id, SecurityFirm: "Test Securities",
		Capabilities: []MarketCapability{{
			Market: "US", Features: []FeatureCapability{b.capability},
		}},
	}
}

func (b *runtimeCapabilityBroker) DiscoverAccounts(context.Context) ([]Account, error) {
	return nil, nil
}

func (b *runtimeCapabilityBroker) Trading() TradingService      { return nil }
func (b *runtimeCapabilityBroker) MarketData() MarketDataReader { return nil }

func (b *runtimeCapabilityBroker) QuerySecuritySnapshot(
	context.Context,
	SecuritySnapshotQuery,
) (*SecuritySnapshotResult, error) {
	return &SecuritySnapshotResult{}, nil
}

func (b *runtimeCapabilityBroker) EvaluateCapability(
	context.Context,
	CapabilityEvaluationRequest,
) (CapabilityEvaluation, error) {
	return b.evaluation, b.evaluationErr
}
