package broker

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"
)

type FeatureID string

const (
	FeatureMarketSearch            FeatureID = "market.search"
	FeatureInstrumentProfile       FeatureID = "market.instrument_profile"
	FeatureMarketSnapshot          FeatureID = "market.snapshot"
	FeatureMarketSnapshots         FeatureID = "market.snapshots"
	FeatureMarketCandles           FeatureID = "market.candles"
	FeatureMarketIntraday          FeatureID = "market.intraday"
	FeatureMarketTicks             FeatureID = "market.ticks"
	FeatureMarketDepth             FeatureID = "market.depth"
	FeatureMarketBrokerQueue       FeatureID = "market.broker_queue"
	FeatureMarketCapitalFlow       FeatureID = "market.capital_flow"
	FeatureOptionChain             FeatureID = "derivatives.option_chain"
	FeatureOptionScreen            FeatureID = "derivatives.option_screen"
	FeatureOptionAnalysis          FeatureID = "derivatives.option_analysis"
	FeatureOptionEvents            FeatureID = "derivatives.option_events"
	FeatureWarrants                FeatureID = "derivatives.warrants"
	FeatureFutures                 FeatureID = "derivatives.futures"
	FeatureResearchInstrument      FeatureID = "research.instrument"
	FeatureResearchFinancials      FeatureID = "research.financials"
	FeatureResearchValuation       FeatureID = "research.valuation"
	FeatureResearchAnalyst         FeatureID = "research.analyst"
	FeatureResearchOwnership       FeatureID = "research.ownership"
	FeatureResearchCorporateAction FeatureID = "research.corporate_actions"
	FeatureResearchShortInterest   FeatureID = "research.short_interest"
	FeatureResearchNews            FeatureID = "research.news"
	FeatureResearchScreen          FeatureID = "research.screen"
	FeatureResearchCalendar        FeatureID = "research.calendar"
	FeatureResearchMacro           FeatureID = "research.macro"
	FeatureResearchRankings        FeatureID = "research.rankings"
	FeatureResearchInstitutions    FeatureID = "research.institutions"
	FeatureResearchIndustry        FeatureID = "research.industry"
	FeatureTechnicalIndicator      FeatureID = "research.technical_indicators"
	FeaturePredictionDiscover      FeatureID = "prediction.discover"
	FeaturePredictionSnapshot      FeatureID = "prediction.snapshot"
	FeaturePredictionDepth         FeatureID = "prediction.depth"
	FeaturePredictionHistory       FeatureID = "prediction.history"
	FeaturePredictionComboEligible FeatureID = "prediction.combo_eligible"
	FeaturePredictionComboQuote    FeatureID = "prediction.combo_quote"
	FeatureExecutionOrderPreview   FeatureID = "execution.order_preview"
	FeatureExecutionOrderPlace     FeatureID = "execution.order_place"
	FeatureExecutionOrderCancel    FeatureID = "execution.order_cancel"
	FeatureExecutionComboPreview   FeatureID = "execution.combo_preview"
	FeatureExecutionComboPlace     FeatureID = "execution.combo_place"
	FeatureExecutionComboCancel    FeatureID = "execution.combo_cancel"
	FeatureExecutionBuyingPower    FeatureID = "execution.buying_power"
	FeaturePriceAlertList          FeatureID = "alerts.price.list"
	FeaturePriceAlertSet           FeatureID = "alerts.price.set"
	FeatureOptionEventAlertList    FeatureID = "alerts.option_event.list"
	FeatureOptionEventAlertSet     FeatureID = "alerts.option_event.set"
	FeatureRemoteWatchlistList     FeatureID = "watchlist.remote.list"
	FeatureRemoteWatchlistModify   FeatureID = "watchlist.remote.modify"
)

type CapabilityState string

const (
	CapabilityAvailable   CapabilityState = "available"
	CapabilityDegraded    CapabilityState = "degraded"
	CapabilityUnavailable CapabilityState = "unavailable"
)

type FeatureAccess string

const (
	FeatureAccessRead  FeatureAccess = "read"
	FeatureAccessWrite FeatureAccess = "write"
	FeatureAccessTrade FeatureAccess = "trade"
)

type PermissionClass string

const (
	PermissionReadOnly      PermissionClass = "read_only"
	PermissionWriteExternal PermissionClass = "write_external"
	PermissionLiveTrading   PermissionClass = "live_trading"
)

type ApprovalLevel string

const (
	ApprovalNone     ApprovalLevel = "none"
	ApprovalHigh     ApprovalLevel = "high"
	ApprovalCritical ApprovalLevel = "critical"
)

// FeatureCapability is a broker's concrete support declaration. Availability
// can be downgraded at runtime for connection, quote entitlement, account
// authority, security-firm eligibility, or rate-limit reasons.
type FeatureCapability struct {
	ID                 FeatureID       `json:"id"`
	Markets            []string        `json:"markets"`
	ProductClasses     []ProductClass  `json:"productClasses,omitempty"`
	MarketSegments     []MarketSegment `json:"marketSegments,omitempty"`
	Access             FeatureAccess   `json:"access"`
	State              CapabilityState `json:"state"`
	ReasonCode         string          `json:"reasonCode,omitempty"`
	Reason             string          `json:"reason,omitempty"`
	RequiresConnection bool            `json:"requiresConnection,omitempty"`
	RequiresAccount    bool            `json:"requiresAccount,omitempty"`
	RequiresQuoteRight bool            `json:"requiresQuoteRight,omitempty"`
	Limits             map[string]any  `json:"limits,omitempty"`
}

// CapabilityCheck is one runtime dimension of a broker capability. Static
// support and runtime usability are intentionally separate: a broker can
// implement an operation while its current connection, account, or quote
// entitlement makes that operation degraded or unavailable.
type CapabilityCheck struct {
	State     CapabilityState `json:"state"`
	Code      string          `json:"code,omitempty"`
	Reason    string          `json:"reason,omitempty"`
	CheckedAt time.Time       `json:"checkedAt"`
}

type CapabilityEvaluation struct {
	State      CapabilityState `json:"state"`
	Code       string          `json:"code,omitempty"`
	Reason     string          `json:"reason,omitempty"`
	Connection CapabilityCheck `json:"connection"`
	Account    CapabilityCheck `json:"account"`
	QuoteRight CapabilityCheck `json:"quoteRight"`
	CheckedAt  time.Time       `json:"checkedAt"`
}

type CapabilityEvaluationRequest struct {
	FeatureID          FeatureID         `json:"featureId"`
	BrokerID           string            `json:"brokerId,omitempty"`
	AccountID          string            `json:"accountId,omitempty"`
	TradingEnvironment string            `json:"tradingEnvironment,omitempty"`
	Market             string            `json:"market,omitempty"`
	MarketSegment      MarketSegment     `json:"marketSegment,omitempty"`
	ProductClass       ProductClass      `json:"productClass,omitempty"`
	DeclaredCapability FeatureCapability `json:"declaredCapability"`
}

// CapabilityEvaluator is optional. Brokers that implement it expose actual
// connection, account, and quote-right state to routing and capability APIs.
type CapabilityEvaluator interface {
	EvaluateCapability(context.Context, CapabilityEvaluationRequest) (CapabilityEvaluation, error)
}

type CapabilitySurface struct {
	API         string `json:"api,omitempty"`
	UI          string `json:"ui,omitempty"`
	Tool        string `json:"tool,omitempty"`
	ReadOnlyMCP bool   `json:"readOnlyMcp,omitempty"`
}

type CapabilityProtocol struct {
	BrokerID string `json:"brokerId"`
	Key      string `json:"key"`
	ID       uint32 `json:"id"`
	Kind     string `json:"kind"` // request | push
}

// CapabilityOperation makes catalog coverage machine-checkable at operation
// granularity instead of treating a large feature family as one opaque item.
type CapabilityOperation struct {
	ID          string               `json:"id"`
	HTTPMethod  string               `json:"httpMethod"`
	API         string               `json:"api"`
	UISurfaceID string               `json:"uiSurfaceId"`
	Tool        string               `json:"tool,omitempty"`
	Protocols   []CapabilityProtocol `json:"protocols,omitempty"`
	TestID      string               `json:"testId"`
}

// CapabilityDefinition is the machine-checkable source of truth connecting an
// OpenD-facing feature to the broker interface and its product surfaces.
type CapabilityDefinition struct {
	ID               FeatureID             `json:"id"`
	AdapterInterface string                `json:"adapterInterface"`
	Access           FeatureAccess         `json:"access"`
	Permission       PermissionClass       `json:"permission"`
	Approval         ApprovalLevel         `json:"approval"`
	Surface          CapabilitySurface     `json:"surface"`
	TestMapping      string                `json:"testMapping"`
	Operations       []CapabilityOperation `json:"operations"`
}

type CapabilityCatalog struct {
	Version  string                 `json:"version"`
	Features []CapabilityDefinition `json:"features"`
}

func (c CapabilityCatalog) Validate() error {
	if strings.TrimSpace(c.Version) == "" {
		return fmt.Errorf("capability catalog version is required")
	}
	seen := make(map[FeatureID]struct{}, len(c.Features))
	for _, feature := range c.Features {
		if feature.ID == "" {
			return fmt.Errorf("capability feature id is required")
		}
		if _, ok := seen[feature.ID]; ok {
			return fmt.Errorf("duplicate capability feature %q", feature.ID)
		}
		seen[feature.ID] = struct{}{}
		if strings.TrimSpace(feature.AdapterInterface) == "" {
			return fmt.Errorf("capability %q has no broker interface mapping", feature.ID)
		}
		if strings.TrimSpace(feature.Surface.API) == "" {
			return fmt.Errorf("capability %q has no API surface", feature.ID)
		}
		if strings.TrimSpace(feature.Surface.UI) == "" &&
			strings.TrimSpace(feature.Surface.Tool) == "" {
			return fmt.Errorf("capability %q has neither UI nor tool surface", feature.ID)
		}
		if strings.TrimSpace(feature.TestMapping) == "" {
			return fmt.Errorf("capability %q has no test mapping", feature.ID)
		}
		if feature.Access == FeatureAccessTrade &&
			(feature.Permission != PermissionLiveTrading || feature.Approval != ApprovalCritical) {
			return fmt.Errorf("trading capability %q must require live_trading + critical", feature.ID)
		}
		if feature.Access == FeatureAccessWrite &&
			(feature.Permission != PermissionWriteExternal || feature.Approval != ApprovalHigh) {
			return fmt.Errorf("write capability %q must require write_external + high", feature.ID)
		}
		if feature.Surface.ReadOnlyMCP && feature.Access != FeatureAccessRead {
			return fmt.Errorf("write capability %q cannot be exposed by read-only MCP", feature.ID)
		}
		if len(feature.Operations) == 0 {
			return fmt.Errorf("capability %q has no operation mapping", feature.ID)
		}
		operationIDs := make(map[string]struct{}, len(feature.Operations))
		for _, operation := range feature.Operations {
			if strings.TrimSpace(operation.ID) == "" {
				return fmt.Errorf("capability %q has an operation without id", feature.ID)
			}
			if _, ok := operationIDs[operation.ID]; ok {
				return fmt.Errorf("capability %q has duplicate operation %q", feature.ID, operation.ID)
			}
			operationIDs[operation.ID] = struct{}{}
			if strings.TrimSpace(operation.HTTPMethod) == "" ||
				strings.TrimSpace(operation.API) == "" ||
				strings.TrimSpace(operation.TestID) == "" {
				return fmt.Errorf("capability %q operation %q has incomplete surface mapping", feature.ID, operation.ID)
			}
			if strings.TrimSpace(operation.UISurfaceID) == "" &&
				strings.TrimSpace(operation.Tool) == "" {
				return fmt.Errorf("capability %q operation %q has neither UI nor tool mapping", feature.ID, operation.ID)
			}
		}
	}
	return nil
}

func (c CapabilityCatalog) Definition(id FeatureID) (CapabilityDefinition, bool) {
	for _, feature := range c.Features {
		if feature.ID == id {
			return feature, true
		}
	}
	return CapabilityDefinition{}, false
}

func (c CapabilityCatalog) SortedFeatureIDs() []FeatureID {
	ids := make([]FeatureID, 0, len(c.Features))
	for _, feature := range c.Features {
		ids = append(ids, feature.ID)
	}
	slices.Sort(ids)
	return ids
}

func catalogRead(id FeatureID, adapterInterface, api, ui, tool, _ string) CapabilityDefinition {
	test := capabilityTestID(id)
	return CapabilityDefinition{
		ID:               id,
		AdapterInterface: adapterInterface,
		Access:           FeatureAccessRead,
		Permission:       PermissionReadOnly,
		Approval:         ApprovalNone,
		Surface: CapabilitySurface{
			API:         api,
			UI:          ui,
			Tool:        tool,
			ReadOnlyMCP: tool != "",
		},
		TestMapping: test,
		Operations:  capabilityOperations(id, "GET", api, ui, tool, test),
	}
}

func catalogWrite(id FeatureID, adapterInterface, api, ui, tool, _ string) CapabilityDefinition {
	test := capabilityTestID(id)
	return CapabilityDefinition{
		ID:               id,
		AdapterInterface: adapterInterface,
		Access:           FeatureAccessWrite,
		Permission:       PermissionWriteExternal,
		Approval:         ApprovalHigh,
		Surface:          CapabilitySurface{API: api, UI: ui, Tool: tool},
		TestMapping:      test,
		Operations:       capabilityOperations(id, "POST", api, ui, tool, test),
	}
}

func catalogTrade(id FeatureID, adapterInterface, api, ui, tool, _ string) CapabilityDefinition {
	test := capabilityTestID(id)
	return CapabilityDefinition{
		ID:               id,
		AdapterInterface: adapterInterface,
		Access:           FeatureAccessTrade,
		Permission:       PermissionLiveTrading,
		Approval:         ApprovalCritical,
		Surface:          CapabilitySurface{API: api, UI: ui, Tool: tool},
		TestMapping:      test,
		Operations:       capabilityOperations(id, "POST", api, ui, tool, test),
	}
}
