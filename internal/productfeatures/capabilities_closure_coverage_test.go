package productfeatures

import (
	"context"
	"errors"
	"testing"

	"github.com/jftrade/jftrade-main/pkg/broker"
)

func TestCapabilitiesContextFiltersAndReportsRuntimeEvaluation(t *testing.T) {
	adapter := &capabilityFeatureBroker{
		featureBroker: &featureBroker{
			id: "runtime", feature: broker.FeaturePredictionDiscover,
		},
		evaluation: broker.CapabilityEvaluation{
			State: broker.CapabilityUnavailable, Code: "ACCOUNT_INELIGIBLE",
			Reason: "Prediction account is unavailable.",
		},
	}
	registry := broker.NewRegistry()
	registry.Register(adapter)
	service := NewService(registry, adapter.id, nil, nil)

	result := service.CapabilitiesContext(t.Context(), CapabilityQuery{
		BrokerID: adapter.id, AccountID: "account-1", Market: "US",
		FeatureID: broker.FeaturePredictionDiscover,
	})
	statuses := result["runtime"].([]RuntimeCapabilityStatus)
	if len(statuses) != 1 ||
		statuses[0].Evaluation.Code != "ACCOUNT_INELIGIBLE" ||
		statuses[0].Capability.State != broker.CapabilityUnavailable ||
		adapter.request.AccountID != "account-1" {
		t.Fatalf("runtime statuses = %#v, request = %#v", statuses, adapter.request)
	}

	if statuses := service.CapabilitiesContext(t.Context(), CapabilityQuery{
		BrokerID: "other",
	})["runtime"].([]RuntimeCapabilityStatus); len(statuses) != 0 {
		t.Fatalf("broker filter statuses = %#v", statuses)
	}
	if statuses := service.CapabilitiesContext(t.Context(), CapabilityQuery{
		BrokerID: adapter.id, Market: "HK",
	})["runtime"].([]RuntimeCapabilityStatus); len(statuses) != 0 {
		t.Fatalf("market filter statuses = %#v", statuses)
	}
	if statuses := service.CapabilitiesContext(t.Context(), CapabilityQuery{
		BrokerID: adapter.id, FeatureID: broker.FeatureMarketSnapshot,
	})["runtime"].([]RuntimeCapabilityStatus); len(statuses) != 0 {
		t.Fatalf("feature filter statuses = %#v", statuses)
	}

	adapter.err = errors.New("eligibility probe failed")
	statuses = service.CapabilitiesContext(t.Context(), CapabilityQuery{
		BrokerID: adapter.id, FeatureID: broker.FeaturePredictionDiscover,
	})["runtime"].([]RuntimeCapabilityStatus)
	if len(statuses) != 1 ||
		statuses[0].Evaluation.Code != "RUNTIME_EVALUATION_FAILED" ||
		statuses[0].Evaluation.Connection.State != broker.CapabilityUnavailable {
		t.Fatalf("failed runtime statuses = %#v", statuses)
	}
}

func TestCapabilitiesContextFiltersProductsAndSegments(t *testing.T) {
	adapter := &featureBroker{
		id: "filtered", feature: broker.FeaturePredictionDiscover,
	}
	registry := broker.NewRegistry()
	registry.Register(adapter)
	service := NewService(registry, adapter.id, nil, nil)
	descriptor := adapter.Descriptor()
	descriptor.Capabilities[0].Features[0].ProductClasses = []broker.ProductClass{
		broker.ProductClassEventContract,
	}
	descriptor.Capabilities[0].Features[0].MarketSegments = []broker.MarketSegment{
		broker.MarketSegmentPrediction,
	}
	registry.Replace(&descriptorFeatureBroker{featureBroker: adapter, descriptor: descriptor})

	if statuses := service.CapabilitiesContext(t.Context(), CapabilityQuery{
		BrokerID: adapter.id, ProductClass: broker.ProductClassEquity,
	})["runtime"].([]RuntimeCapabilityStatus); len(statuses) != 0 {
		t.Fatalf("product filter statuses = %#v", statuses)
	}
	if statuses := service.CapabilitiesContext(t.Context(), CapabilityQuery{
		BrokerID: adapter.id, ProductClass: broker.ProductClassEventContract,
		MarketSegment: broker.MarketSegmentSecurities,
	})["runtime"].([]RuntimeCapabilityStatus); len(statuses) != 0 {
		t.Fatalf("segment filter statuses = %#v", statuses)
	}
	if statuses := service.CapabilitiesContext(t.Context(), CapabilityQuery{
		BrokerID: adapter.id, ProductClass: broker.ProductClassEventContract,
		MarketSegment: broker.MarketSegmentPrediction,
	})["runtime"].([]RuntimeCapabilityStatus); len(statuses) != 1 {
		t.Fatalf("matching filter statuses = %#v", statuses)
	}
}

func TestCapabilitiesContextMarksDeclaredButMissingInterfaceUnavailable(t *testing.T) {
	adapter := &bareFeatureBroker{
		id: "partial", feature: broker.FeatureOptionAnalysis,
	}
	registry := broker.NewRegistry()
	registry.Register(adapter)
	service := NewService(registry, adapter.id, nil, nil)

	statuses := service.CapabilitiesContext(t.Context(), CapabilityQuery{
		BrokerID: adapter.id, Market: "US",
		FeatureID: broker.FeatureOptionAnalysis,
	})["runtime"].([]RuntimeCapabilityStatus)
	if len(statuses) != 1 ||
		statuses[0].Capability.State != broker.CapabilityUnavailable ||
		statuses[0].Evaluation.Code != "ADAPTER_INTERFACE_UNAVAILABLE" {
		t.Fatalf("missing adapter interface statuses = %#v", statuses)
	}
}

func TestStaticRuntimeEvaluationDistinguishesRequiredDimensions(t *testing.T) {
	result := staticRuntimeEvaluation(broker.FeatureCapability{
		State: broker.CapabilityAvailable, RequiresConnection: true,
		RequiresAccount: true, RequiresQuoteRight: true,
	})
	if result.State != broker.CapabilityDegraded ||
		result.Code != "RUNTIME_STATUS_PARTIAL" ||
		result.Connection.Code != "RUNTIME_STATUS_UNKNOWN" ||
		result.Account.Code != "RUNTIME_STATUS_UNKNOWN" ||
		result.QuoteRight.Code != "RUNTIME_STATUS_UNKNOWN" {
		t.Fatalf("required runtime evaluation = %#v", result)
	}
	staticUnavailable := staticRuntimeEvaluation(broker.FeatureCapability{
		State: broker.CapabilityUnavailable, ReasonCode: "DISABLED", Reason: "Disabled.",
	})
	if staticUnavailable.State != broker.CapabilityUnavailable ||
		staticUnavailable.Connection.Code != "NOT_REQUIRED" {
		t.Fatalf("static unavailable evaluation = %#v", staticUnavailable)
	}
	if !containsProductClass(
		[]broker.ProductClass{broker.ProductClassOption},
		broker.ProductClassOption,
	) || !containsMarketSegment(
		[]broker.MarketSegment{broker.MarketSegmentPrediction},
		broker.MarketSegmentPrediction,
	) {
		t.Fatal("capability membership helpers failed")
	}
}

type capabilityFeatureBroker struct {
	*featureBroker
	request    broker.CapabilityEvaluationRequest
	evaluation broker.CapabilityEvaluation
	err        error
}

func (b *capabilityFeatureBroker) EvaluateCapability(
	_ context.Context,
	request broker.CapabilityEvaluationRequest,
) (broker.CapabilityEvaluation, error) {
	b.request = request
	return b.evaluation, b.err
}

type descriptorFeatureBroker struct {
	*featureBroker
	descriptor broker.Descriptor
}

func (b *descriptorFeatureBroker) Descriptor() broker.Descriptor {
	return b.descriptor
}
