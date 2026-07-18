package broker

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"
)

type FeatureRouteRequest struct {
	BrokerID           string        `json:"brokerId,omitempty"`
	AccountID          string        `json:"accountId,omitempty"`
	TradingEnvironment string        `json:"tradingEnvironment,omitempty"`
	FeatureID          FeatureID     `json:"featureId"`
	Market             string        `json:"market,omitempty"`
	MarketSegment      MarketSegment `json:"marketSegment,omitempty"`
	ProductClass       ProductClass  `json:"productClass,omitempty"`
}

type FeatureResolution struct {
	Broker            Broker               `json:"-"`
	BrokerID          string               `json:"brokerId"`
	SecurityFirm      string               `json:"securityFirm,omitempty"`
	Capability        FeatureCapability    `json:"capability"`
	SelectionReason   string               `json:"selectionReason"`
	CapabilityVersion string               `json:"capabilityVersion"`
	Evaluation        CapabilityEvaluation `json:"evaluation"`
	ResolvedAt        time.Time            `json:"resolvedAt"`
}

// BrokerFeatureRouter chooses a broker once per request. Callers retain the
// returned resolution through preview and submission; the router is not
// consulted again after a preview has locked broker/account/capability version.
type BrokerFeatureRouter struct {
	registry      *Registry
	defaultBroker string
	fallbackOrder []string
	catalog       CapabilityCatalog
	now           func() time.Time
}

func NewBrokerFeatureRouter(registry *Registry, defaultBroker string, fallbackOrder []string) *BrokerFeatureRouter {
	return &BrokerFeatureRouter{
		registry:      registry,
		defaultBroker: strings.TrimSpace(defaultBroker),
		fallbackOrder: append([]string(nil), fallbackOrder...),
		catalog:       BuiltinCapabilityCatalog,
		now:           time.Now,
	}
}

func (r *BrokerFeatureRouter) Resolve(request FeatureRouteRequest) (FeatureResolution, error) {
	return r.ResolveContext(context.Background(), request)
}

func (r *BrokerFeatureRouter) ResolveContext(
	ctx context.Context,
	request FeatureRouteRequest,
) (FeatureResolution, error) {
	if r == nil || r.registry == nil {
		return FeatureResolution{}, fmt.Errorf("broker feature router is unavailable")
	}
	if _, ok := r.catalog.Definition(request.FeatureID); !ok {
		return FeatureResolution{}, fmt.Errorf("unknown broker feature %q", request.FeatureID)
	}
	explicit := strings.TrimSpace(request.BrokerID)
	if explicit != "" {
		return r.resolveBroker(ctx, explicit, request, "explicit_broker")
	}

	candidates := make([]string, 0, 1+len(r.fallbackOrder))
	if r.defaultBroker != "" {
		candidates = append(candidates, r.defaultBroker)
	}
	candidates = append(candidates, r.fallbackOrder...)
	if len(candidates) == 0 {
		candidates = r.registry.IDs()
	}

	var reasons []string
	seen := make(map[string]struct{}, len(candidates))
	for _, id := range candidates {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		resolution, err := r.resolveBroker(ctx, id, request, "configured_fallback")
		if err == nil {
			if id == r.defaultBroker {
				resolution.SelectionReason = "default_broker"
			}
			return resolution, nil
		}
		reasons = append(reasons, err.Error())
	}
	return FeatureResolution{}, fmt.Errorf("no broker can serve %q: %s", request.FeatureID, strings.Join(reasons, "; "))
}

func (r *BrokerFeatureRouter) resolveBroker(
	ctx context.Context,
	id string,
	request FeatureRouteRequest,
	reason string,
) (FeatureResolution, error) {
	selected := r.registry.Lookup(id)
	if selected == nil {
		return FeatureResolution{}, fmt.Errorf("broker %q is not registered", id)
	}
	capability, ok := descriptorFeature(selected.Descriptor(), request)
	if !ok {
		return FeatureResolution{}, fmt.Errorf("broker %q does not declare feature %q for market %q", id, request.FeatureID, request.Market)
	}
	definition, _ := r.catalog.Definition(request.FeatureID)
	if !ImplementsAdapterInterface(selected, definition.AdapterInterface) {
		return FeatureResolution{}, fmt.Errorf(
			"broker %q feature %q is unavailable: adapter interface %s is not implemented",
			id, request.FeatureID, definition.AdapterInterface,
		)
	}
	if capability.State == CapabilityUnavailable {
		detail := capability.Reason
		if detail == "" {
			detail = capability.ReasonCode
		}
		return FeatureResolution{}, fmt.Errorf("broker %q feature %q is unavailable: %s", id, request.FeatureID, detail)
	}
	evaluation := defaultCapabilityEvaluation(capability, r.now().UTC())
	if evaluator, ok := selected.(CapabilityEvaluator); ok {
		var err error
		evaluation, err = evaluator.EvaluateCapability(ctx, CapabilityEvaluationRequest{
			FeatureID: request.FeatureID, BrokerID: id,
			AccountID: request.AccountID, TradingEnvironment: request.TradingEnvironment,
			Market: request.Market, MarketSegment: request.MarketSegment,
			ProductClass: request.ProductClass, DeclaredCapability: capability,
		})
		if err != nil {
			return FeatureResolution{}, fmt.Errorf(
				"broker %q feature %q runtime evaluation failed: %w",
				id, request.FeatureID, err,
			)
		}
	}
	capability.State = evaluation.State
	capability.ReasonCode = evaluation.Code
	capability.Reason = evaluation.Reason
	if evaluation.State == CapabilityUnavailable {
		return FeatureResolution{}, fmt.Errorf(
			"broker %q feature %q is unavailable: %s",
			id, request.FeatureID, firstCapabilityReason(evaluation),
		)
	}
	return FeatureResolution{
		Broker:            selected,
		BrokerID:          id,
		Capability:        capability,
		SelectionReason:   reason,
		CapabilityVersion: r.catalog.Version,
		Evaluation:        evaluation,
		ResolvedAt:        r.now().UTC(),
	}, nil
}

func defaultCapabilityEvaluation(
	capability FeatureCapability,
	checkedAt time.Time,
) CapabilityEvaluation {
	notRequired := CapabilityCheck{
		State: CapabilityAvailable, Code: "NOT_REQUIRED",
		Reason: "This runtime dimension is not required.", CheckedAt: checkedAt,
	}
	unknown := CapabilityCheck{
		State: CapabilityDegraded, Code: "RUNTIME_STATUS_UNKNOWN",
		Reason: "The broker does not expose this runtime dimension.", CheckedAt: checkedAt,
	}
	evaluation := CapabilityEvaluation{
		State: capability.State, Code: capability.ReasonCode, Reason: capability.Reason,
		Connection: notRequired, Account: notRequired, QuoteRight: notRequired,
		CheckedAt: checkedAt,
	}
	if capability.RequiresConnection {
		evaluation.Connection = unknown
	}
	if capability.RequiresAccount {
		evaluation.Account = unknown
	}
	if capability.RequiresQuoteRight {
		evaluation.QuoteRight = unknown
	}
	if evaluation.State == CapabilityAvailable &&
		(evaluation.Connection.State == CapabilityDegraded ||
			evaluation.Account.State == CapabilityDegraded ||
			evaluation.QuoteRight.State == CapabilityDegraded) {
		evaluation.State = CapabilityDegraded
		evaluation.Code = "RUNTIME_STATUS_PARTIAL"
		evaluation.Reason = "Static support is available but one or more runtime dimensions are unknown."
	}
	return evaluation
}

func firstCapabilityReason(evaluation CapabilityEvaluation) string {
	if strings.TrimSpace(evaluation.Reason) != "" {
		return evaluation.Reason
	}
	for _, check := range []CapabilityCheck{
		evaluation.Connection, evaluation.Account, evaluation.QuoteRight,
	} {
		if check.State == CapabilityUnavailable && strings.TrimSpace(check.Reason) != "" {
			return check.Reason
		}
	}
	return "runtime capability is unavailable"
}

func descriptorFeature(descriptor Descriptor, request FeatureRouteRequest) (FeatureCapability, bool) {
	for _, market := range descriptor.Capabilities {
		if request.Market != "" && !strings.EqualFold(market.Market, request.Market) {
			continue
		}
		for _, feature := range market.Features {
			if feature.ID != request.FeatureID {
				continue
			}
			if request.ProductClass != "" && len(feature.ProductClasses) > 0 &&
				!containsProductClass(feature.ProductClasses, request.ProductClass) {
				continue
			}
			return feature, true
		}
	}
	return FeatureCapability{}, false
}

func containsProductClass(values []ProductClass, target ProductClass) bool {
	return slices.Contains(values, target)
}
