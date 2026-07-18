package productfeatures

import (
	"context"
	"slices"
	"strings"
	"time"

	"github.com/jftrade/jftrade-main/pkg/broker"
)

type CapabilityQuery struct {
	BrokerID           string
	AccountID          string
	TradingEnvironment string
	Market             string
	FeatureID          broker.FeatureID
	ProductClass       broker.ProductClass
	MarketSegment      broker.MarketSegment
}

type RuntimeCapabilityStatus struct {
	BrokerID     string                      `json:"brokerId"`
	SecurityFirm string                      `json:"securityFirm,omitempty"`
	Market       string                      `json:"market"`
	FeatureID    broker.FeatureID            `json:"featureId"`
	Capability   broker.FeatureCapability    `json:"capability"`
	Evaluation   broker.CapabilityEvaluation `json:"evaluation"`
}

func (s *Service) Catalog() broker.CapabilityCatalog {
	return broker.BuiltinCapabilityCatalog
}

func (s *Service) Capabilities() map[string]any {
	return s.CapabilitiesContext(context.Background(), CapabilityQuery{})
}

func (s *Service) CapabilitiesContext(
	ctx context.Context,
	query CapabilityQuery,
) map[string]any {
	descriptors := s.staticCapabilityDescriptors()
	statuses := make([]RuntimeCapabilityStatus, 0)
	for _, descriptor := range descriptors {
		if query.BrokerID != "" && !strings.EqualFold(query.BrokerID, descriptor.ID) {
			continue
		}
		selected := s.registry.Lookup(descriptor.ID)
		for _, market := range descriptor.Capabilities {
			if query.Market != "" && !strings.EqualFold(query.Market, market.Market) {
				continue
			}
			for _, capability := range market.Features {
				if query.FeatureID != "" && query.FeatureID != capability.ID {
					continue
				}
				if query.ProductClass != "" &&
					len(capability.ProductClasses) > 0 &&
					!containsProductClass(capability.ProductClasses, query.ProductClass) {
					continue
				}
				if query.MarketSegment != "" &&
					len(capability.MarketSegments) > 0 &&
					!containsMarketSegment(capability.MarketSegments, query.MarketSegment) {
					continue
				}
				evaluation := staticRuntimeEvaluation(capability)
				definition, definitionFound := broker.BuiltinCapabilityCatalog.Definition(capability.ID)
				if !definitionFound ||
					!broker.ImplementsAdapterInterface(selected, definition.AdapterInterface) {
					evaluation = adapterInterfaceUnavailableEvaluation(definition.AdapterInterface)
				} else if evaluator, ok := selected.(broker.CapabilityEvaluator); ok {
					runtime, err := evaluator.EvaluateCapability(ctx, broker.CapabilityEvaluationRequest{
						FeatureID: capability.ID, BrokerID: descriptor.ID,
						AccountID: query.AccountID, TradingEnvironment: query.TradingEnvironment,
						Market: market.Market, MarketSegment: query.MarketSegment,
						ProductClass: query.ProductClass, DeclaredCapability: capability,
					})
					if err != nil {
						runtime = unavailableRuntimeEvaluation(err.Error())
					}
					evaluation = runtime
				}
				evaluatedCapability := capability
				evaluatedCapability.State = evaluation.State
				evaluatedCapability.ReasonCode = evaluation.Code
				evaluatedCapability.Reason = evaluation.Reason
				statuses = append(statuses, RuntimeCapabilityStatus{
					BrokerID: descriptor.ID, SecurityFirm: descriptor.SecurityFirm,
					Market: market.Market, FeatureID: capability.ID,
					Capability: evaluatedCapability, Evaluation: evaluation,
				})
			}
		}
	}
	return map[string]any{
		"catalog": broker.BuiltinCapabilityCatalog,
		"brokers": descriptors,
		"runtime": statuses,
	}
}

func adapterInterfaceUnavailableEvaluation(interfaceName string) broker.CapabilityEvaluation {
	now := time.Now().UTC()
	reason := "The broker adapter does not implement the declared capability interface."
	if strings.TrimSpace(interfaceName) != "" {
		reason = "The broker adapter does not implement " + interfaceName + "."
	}
	unavailable := broker.CapabilityCheck{
		State: broker.CapabilityUnavailable, Code: "ADAPTER_INTERFACE_UNAVAILABLE",
		Reason: reason, CheckedAt: now,
	}
	return broker.CapabilityEvaluation{
		State: broker.CapabilityUnavailable, Code: unavailable.Code, Reason: reason,
		Connection: unavailable, Account: unavailable, QuoteRight: unavailable,
		CheckedAt: now,
	}
}

func staticRuntimeEvaluation(
	capability broker.FeatureCapability,
) broker.CapabilityEvaluation {
	now := time.Now().UTC()
	notRequired := broker.CapabilityCheck{
		State: broker.CapabilityAvailable, Code: "NOT_REQUIRED",
		Reason: "This runtime dimension is not required.", CheckedAt: now,
	}
	unknown := broker.CapabilityCheck{
		State: broker.CapabilityDegraded, Code: "RUNTIME_STATUS_UNKNOWN",
		Reason: "The broker does not expose this runtime dimension.", CheckedAt: now,
	}
	result := broker.CapabilityEvaluation{
		State: capability.State, Code: capability.ReasonCode, Reason: capability.Reason,
		Connection: notRequired, Account: notRequired, QuoteRight: notRequired,
		CheckedAt: now,
	}
	if capability.RequiresConnection {
		result.Connection = unknown
	}
	if capability.RequiresAccount {
		result.Account = unknown
	}
	if capability.RequiresQuoteRight {
		result.QuoteRight = unknown
	}
	if result.State == broker.CapabilityAvailable &&
		(result.Connection.State == broker.CapabilityDegraded ||
			result.Account.State == broker.CapabilityDegraded ||
			result.QuoteRight.State == broker.CapabilityDegraded) {
		result.State = broker.CapabilityDegraded
		result.Code = "RUNTIME_STATUS_PARTIAL"
		result.Reason = "Static support is available but runtime state is incomplete."
	}
	return result
}

func unavailableRuntimeEvaluation(reason string) broker.CapabilityEvaluation {
	now := time.Now().UTC()
	unavailable := broker.CapabilityCheck{
		State: broker.CapabilityUnavailable, Code: "RUNTIME_EVALUATION_FAILED",
		Reason: reason, CheckedAt: now,
	}
	return broker.CapabilityEvaluation{
		State: broker.CapabilityUnavailable, Code: unavailable.Code, Reason: reason,
		Connection: unavailable, Account: unavailable, QuoteRight: unavailable,
		CheckedAt: now,
	}
}

func containsProductClass(values []broker.ProductClass, target broker.ProductClass) bool {
	return slices.Contains(values, target)
}

func containsMarketSegment(values []broker.MarketSegment, target broker.MarketSegment) bool {
	return slices.Contains(values, target)
}
