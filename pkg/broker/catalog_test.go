package broker_test

import (
	"context"
	"strings"
	"testing"

	"github.com/jftrade/jftrade-main/pkg/broker"
)

func TestCapabilityCatalog(t *testing.T) {
	catalog := broker.BuiltinCapabilityCatalog
	if err := catalog.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
	if len(catalog.Features) < 45 {
		t.Fatalf("catalog feature count = %d, want complete product surface", len(catalog.Features))
	}

	required := []broker.FeatureID{
		broker.FeatureOptionChain,
		broker.FeatureOptionAnalysis,
		broker.FeatureResearchFinancials,
		broker.FeaturePredictionDiscover,
		broker.FeaturePredictionComboQuote,
		broker.FeatureExecutionComboPlace,
		broker.FeatureRemoteWatchlistModify,
	}
	for _, id := range required {
		if _, ok := catalog.Definition(id); !ok {
			t.Errorf("missing required capability %q", id)
		}
	}
	warrants, ok := catalog.Definition(broker.FeatureWarrants)
	if !ok || warrants.Surface.UI != "/workspace?tab=warrants" {
		t.Fatalf("warrant UI surface = %#v", warrants.Surface)
	}
	futures, ok := catalog.Definition(broker.FeatureFutures)
	if !ok || futures.Surface.UI != "/workspace" {
		t.Fatalf("future UI surface = %#v", futures.Surface)
	}
}

func TestAdapterInterfaceSupportRejectsNilMissingAndUnknownImplementations(t *testing.T) {
	if broker.ImplementsAdapterInterface(nil, "BatchSnapshotSource") {
		t.Fatal("nil broker unexpectedly implements an adapter interface")
	}
	partial := &capabilityBroker{id: "partial"}
	for _, interfaceName := range []string{
		"MarketDataReader",
		"TradingService",
		"MarketMicrostructureReader",
		"InstrumentProfileReader",
		"OptionAnalyticsReader",
		"InstrumentResearchReader",
		"MarketResearchReader",
		"PredictionMarketReader",
		"TechnicalIndicatorReader",
		"CustomizationService",
		"ProductRuleProvider",
		"ComboTradingService",
		"EventContractTradingService",
		"UnknownReader",
	} {
		if broker.ImplementsAdapterInterface(partial, interfaceName) {
			t.Errorf("partial broker unexpectedly implements %s", interfaceName)
		}
	}
	if !broker.ImplementsAdapterInterface(partial, "BatchSnapshotSource") ||
		!broker.ImplementsAdapterInterface(partial, "DerivativeCatalogReader") {
		t.Fatal("partial broker's implemented interfaces were not detected")
	}
}

func TestCapabilityCatalogRejectsUnsafeWriteMCP(t *testing.T) {
	catalog := broker.CapabilityCatalog{
		Version: "test",
		Features: []broker.CapabilityDefinition{{
			ID:               broker.FeaturePriceAlertSet,
			AdapterInterface: "CustomizationService",
			Access:           broker.FeatureAccessWrite,
			Permission:       broker.PermissionWriteExternal,
			Approval:         broker.ApprovalHigh,
			Surface: broker.CapabilitySurface{
				API:         "/api/test",
				UI:          "/test",
				Tool:        "alerts.price.set",
				ReadOnlyMCP: true,
			},
			TestMapping: "TestCapabilityCatalogRejectsUnsafeWriteMCP",
		}},
	}
	if err := catalog.Validate(); err == nil || !strings.Contains(err.Error(), "read-only MCP") {
		t.Fatalf("Validate() error = %v, want read-only MCP rejection", err)
	}
}

func TestBrokerFeatureRouterHonorsExplicitSelectionAndFallback(t *testing.T) {
	registry := broker.NewRegistry()
	registry.Register(&capabilityBroker{
		id: "partial-a",
		features: []broker.FeatureCapability{{
			ID:      broker.FeatureMarketSnapshot,
			Markets: []string{"US"},
			Access:  broker.FeatureAccessRead,
			State:   broker.CapabilityAvailable,
		}},
	})
	registry.Register(&capabilityBroker{
		id: "partial-b",
		features: []broker.FeatureCapability{{
			ID:      broker.FeatureOptionChain,
			Markets: []string{"US"},
			Access:  broker.FeatureAccessRead,
			State:   broker.CapabilityAvailable,
		}},
	})
	router := broker.NewBrokerFeatureRouter(registry, "partial-a", []string{"partial-b"})

	resolution, err := router.Resolve(broker.FeatureRouteRequest{
		FeatureID: broker.FeatureOptionChain,
		Market:    "US",
	})
	if err != nil {
		t.Fatalf("fallback Resolve() error = %v", err)
	}
	if resolution.BrokerID != "partial-b" || resolution.SelectionReason != "configured_fallback" {
		t.Fatalf("fallback resolution = %#v", resolution)
	}

	_, err = router.Resolve(broker.FeatureRouteRequest{
		BrokerID:  "partial-a",
		FeatureID: broker.FeatureOptionChain,
		Market:    "US",
	})
	if err == nil || !strings.Contains(err.Error(), "partial-a") {
		t.Fatalf("explicit Resolve() error = %v, want no fallback", err)
	}
}

func TestCapabilityCatalogValidationAndOrderingBranches(t *testing.T) {
	valid := broker.CapabilityDefinition{
		ID:               broker.FeatureMarketSnapshot,
		AdapterInterface: "BatchSnapshotSource",
		Access:           broker.FeatureAccessRead,
		Permission:       broker.PermissionReadOnly,
		Approval:         broker.ApprovalNone,
		Surface:          broker.CapabilitySurface{API: "/snapshot", Tool: "market.snapshot"},
		TestMapping:      "TestCapabilityCatalogValidationAndOrderingBranches",
	}
	cases := []broker.CapabilityCatalog{
		{},
		{Version: "test", Features: []broker.CapabilityDefinition{{}}},
		{Version: "test", Features: []broker.CapabilityDefinition{valid, valid}},
		{Version: "test", Features: []broker.CapabilityDefinition{{ID: valid.ID}}},
		{Version: "test", Features: []broker.CapabilityDefinition{{ID: valid.ID, AdapterInterface: "Reader"}}},
		{Version: "test", Features: []broker.CapabilityDefinition{{
			ID: valid.ID, AdapterInterface: "Reader", Surface: broker.CapabilitySurface{API: "/x"},
		}}},
		{Version: "test", Features: []broker.CapabilityDefinition{{
			ID: valid.ID, AdapterInterface: "Reader", Surface: broker.CapabilitySurface{API: "/x", UI: "/x"},
		}}},
		{Version: "test", Features: []broker.CapabilityDefinition{{
			ID: valid.ID, AdapterInterface: "Reader", Access: broker.FeatureAccessTrade,
			Surface: broker.CapabilitySurface{API: "/x", UI: "/x"}, TestMapping: "x",
		}}},
		{Version: "test", Features: []broker.CapabilityDefinition{{
			ID: valid.ID, AdapterInterface: "Reader", Access: broker.FeatureAccessWrite,
			Surface: broker.CapabilitySurface{API: "/x", UI: "/x"}, TestMapping: "x",
		}}},
	}
	for index, catalog := range cases {
		if err := catalog.Validate(); err == nil {
			t.Errorf("case %d unexpectedly validated: %#v", index, catalog)
		}
	}

	catalog := broker.CapabilityCatalog{Version: "test", Features: []broker.CapabilityDefinition{
		valid,
		func() broker.CapabilityDefinition {
			item := valid
			item.ID = broker.FeatureMarketSearch
			return item
		}(),
	}}
	ids := catalog.SortedFeatureIDs()
	if len(ids) != 2 || ids[0] != broker.FeatureMarketSearch || ids[1] != broker.FeatureMarketSnapshot {
		t.Fatalf("SortedFeatureIDs() = %#v", ids)
	}
	if _, ok := catalog.Definition("missing"); ok {
		t.Fatal("Definition(missing) unexpectedly found a feature")
	}
}

func TestBrokerFeatureRouterFailureAndProductBranches(t *testing.T) {
	if _, err := (*broker.BrokerFeatureRouter)(nil).Resolve(broker.FeatureRouteRequest{}); err == nil {
		t.Fatal("nil router resolved")
	}
	registry := broker.NewRegistry()
	option := &capabilityBroker{id: "option", features: []broker.FeatureCapability{{
		ID: broker.FeatureOptionChain, Markets: []string{"US"},
		ProductClasses: []broker.ProductClass{broker.ProductClassOption},
		Access:         broker.FeatureAccessRead, State: broker.CapabilityAvailable,
	}}}
	unavailable := &capabilityBroker{id: "unavailable", features: []broker.FeatureCapability{{
		ID: broker.FeatureOptionChain, Markets: []string{"US"},
		Access: broker.FeatureAccessRead, State: broker.CapabilityUnavailable,
		ReasonCode: "NO_RIGHT", Reason: "quote right missing",
	}}}
	registry.Register(option)
	registry.Register(unavailable)
	router := broker.NewBrokerFeatureRouter(registry, "", nil)

	for _, request := range []broker.FeatureRouteRequest{
		{FeatureID: "missing"},
		{BrokerID: "unknown", FeatureID: broker.FeatureOptionChain, Market: "US"},
		{BrokerID: "unavailable", FeatureID: broker.FeatureOptionChain, Market: "US"},
		{BrokerID: "option", FeatureID: broker.FeatureOptionChain, Market: "US", ProductClass: broker.ProductClassFuture},
	} {
		if _, err := router.Resolve(request); err == nil {
			t.Errorf("Resolve(%#v) unexpectedly succeeded", request)
		}
	}
	resolution, err := router.Resolve(broker.FeatureRouteRequest{
		BrokerID: "option", FeatureID: broker.FeatureOptionChain,
		Market: "US", ProductClass: broker.ProductClassOption,
	})
	if err != nil || resolution.BrokerID != "option" {
		t.Fatalf("option resolution = %#v, %v", resolution, err)
	}
}

func TestBrokerFeatureRouterCandidateOrderingDeduplicationAndReasonFallback(t *testing.T) {
	registry := broker.NewRegistry()
	available := &capabilityBroker{id: "available", features: []broker.FeatureCapability{{
		ID: broker.FeatureMarketSnapshot, Markets: []string{"US"},
		Access: broker.FeatureAccessRead, State: broker.CapabilityAvailable,
	}}}
	unavailable := &capabilityBroker{id: "reason-code-only", features: []broker.FeatureCapability{{
		ID: broker.FeatureMarketSnapshot, Markets: []string{"US"},
		Access: broker.FeatureAccessRead, State: broker.CapabilityUnavailable,
		ReasonCode: "NO_QUOTE_RIGHT",
	}}}
	registry.Register(available)
	registry.Register(unavailable)

	fromRegistry, err := broker.NewBrokerFeatureRouter(registry, "", nil).Resolve(
		broker.FeatureRouteRequest{FeatureID: broker.FeatureMarketSnapshot, Market: "US"},
	)
	if err != nil || fromRegistry.BrokerID != available.id {
		t.Fatalf("registry candidate resolution = %#v, %v", fromRegistry, err)
	}

	defaultResolution, err := broker.NewBrokerFeatureRouter(registry, available.id, nil).Resolve(
		broker.FeatureRouteRequest{FeatureID: broker.FeatureMarketSnapshot, Market: "US"},
	)
	if err != nil || defaultResolution.SelectionReason != "default_broker" {
		t.Fatalf("default broker resolution = %#v, %v", defaultResolution, err)
	}

	fallbackResolution, err := broker.NewBrokerFeatureRouter(
		registry,
		"",
		[]string{" ", "missing", "missing", available.id},
	).Resolve(broker.FeatureRouteRequest{
		FeatureID: broker.FeatureMarketSnapshot, Market: "US",
	})
	if err != nil || fallbackResolution.BrokerID != available.id ||
		fallbackResolution.SelectionReason != "configured_fallback" {
		t.Fatalf("deduplicated fallback resolution = %#v, %v", fallbackResolution, err)
	}

	for _, request := range []broker.FeatureRouteRequest{
		{
			BrokerID: "reason-code-only", FeatureID: broker.FeatureMarketSnapshot,
			Market: "US",
		},
		{
			BrokerID: available.id, FeatureID: broker.FeatureMarketSnapshot,
			Market: "HK",
		},
	} {
		if _, err := broker.NewBrokerFeatureRouter(registry, "", nil).Resolve(request); err == nil {
			t.Errorf("Resolve(%#v) unexpectedly succeeded", request)
		}
	}
}

type capabilityBroker struct {
	id       string
	features []broker.FeatureCapability
}

func (b *capabilityBroker) ID() string { return b.id }

func (b *capabilityBroker) Descriptor() broker.Descriptor {
	return broker.Descriptor{
		ID:                b.id,
		CapabilityVersion: broker.BuiltinCapabilityCatalog.Version,
		Capabilities: []broker.MarketCapability{{
			Market:   "US",
			Features: b.features,
		}},
	}
}

func (b *capabilityBroker) DiscoverAccounts(context.Context) ([]broker.Account, error) {
	return nil, nil
}

func (b *capabilityBroker) Trading() broker.TradingService      { return nil }
func (b *capabilityBroker) MarketData() broker.MarketDataReader { return nil }

func (b *capabilityBroker) QuerySecuritySnapshot(
	context.Context,
	broker.SecuritySnapshotQuery,
) (*broker.SecuritySnapshotResult, error) {
	return &broker.SecuritySnapshotResult{}, nil
}

func (b *capabilityBroker) QueryDerivativeCatalog(
	context.Context,
	broker.FeatureQuery,
) (*broker.FeatureResult, error) {
	return &broker.FeatureResult{}, nil
}
