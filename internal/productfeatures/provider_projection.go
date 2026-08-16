package productfeatures

import (
	"strings"
	"time"

	"github.com/jftrade/jftrade-main/internal/marketdata"
	"github.com/jftrade/jftrade-main/pkg/broker"
)

// embeddedProviderSelectionReason explains that a product feature read was
// served by the embedded market-data provider instead of a registered broker.
const embeddedProviderSelectionReason = "embedded-market-data-provider"

// projectProviderNews converts provider-neutral news into the broker feature
// envelope. Nullable upstream fields are omitted from the entry documents.
func projectProviderNews(
	descriptor marketdata.ProviderDescriptor,
	query *broker.FeatureQuery,
	response marketdata.NewsResponse,
	market string,
	now time.Time,
) *broker.FeatureResult {
	entries := make([]map[string]any, 0, len(response.Entries))
	latest := time.Time{}
	for _, entry := range response.Entries {
		projected := map[string]any{}
		if entry.Title != nil {
			projected["title"] = *entry.Title
		}
		if entry.Link != nil {
			projected["link"] = *entry.Link
		}
		if entry.Publisher != nil {
			projected["publisher"] = *entry.Publisher
		}
		if entry.Summary != nil {
			projected["summary"] = *entry.Summary
		}
		if entry.PublishedAt != nil {
			projected["publishedAt"] = *entry.PublishedAt
			if parsed, err := time.Parse(time.RFC3339Nano, *entry.PublishedAt); err == nil &&
				parsed.After(latest) {
				latest = parsed
			}
		}
		entries = append(entries, projected)
	}
	asOf := now
	if !latest.IsZero() {
		asOf = latest.UTC()
	}
	return embeddedFeatureResult(descriptor, query, response.InstrumentID, market, asOf, now, entries, response.Source)
}

// projectProviderCorporateActions converts dividend/split events into the
// broker feature envelope with a plain-language statement per event.
func projectProviderCorporateActions(
	descriptor marketdata.ProviderDescriptor,
	query *broker.FeatureQuery,
	response marketdata.CorporateActionsResponse,
	market string,
	now time.Time,
) *broker.FeatureResult {
	entries := make([]map[string]any, 0, len(response.Events))
	for _, event := range response.Events {
		projected := map[string]any{}
		if kind := strings.TrimSpace(event.Kind); kind != "" {
			projected["kind"] = kind
		}
		if exDate := strings.TrimSpace(event.ExDate); exDate != "" {
			projected["exDate"] = exDate
		}
		if statement := corporateActionStatement(event); statement != "" {
			projected["statement"] = statement
		}
		entries = append(entries, projected)
	}
	return embeddedFeatureResult(descriptor, query, response.InstrumentID, market, now, now, entries, response.Source)
}

// corporateActionStatement renders the event terms the console displays, for
// example "每股派息 0.5" or "1 拆 4".
func corporateActionStatement(event marketdata.CorporateActionEvent) string {
	switch strings.ToLower(strings.TrimSpace(event.Kind)) {
	case "dividend":
		if event.Amount != nil {
			return "每股派息 " + event.Amount.String()
		}
	case "split":
		if event.Ratio != nil {
			return "1 拆 " + event.Ratio.String()
		}
	}
	return ""
}

func embeddedFeatureResult(
	descriptor marketdata.ProviderDescriptor,
	query *broker.FeatureQuery,
	instrumentID string,
	market string,
	asOf time.Time,
	now time.Time,
	entries []map[string]any,
	source string,
) *broker.FeatureResult {
	total := len(entries)
	hasMore := false
	if instrumentID == "" {
		instrumentID = strings.ToUpper(strings.TrimSpace(query.InstrumentID))
	}
	return &broker.FeatureResult{
		Provider: broker.ProviderAttribution{
			BrokerID:        strings.TrimSpace(descriptor.BrokerID),
			FeatureID:       query.FeatureID,
			Capability:      broker.CapabilityAvailable,
			SelectionReason: embeddedProviderSelectionReason,
			ResolvedAt:      now,
			AsOf:            asOf,
		},
		ResolvedInstrument: embeddedResolvedInstrument(query, instrumentID, market),
		AsOf:               asOf,
		Entries:            entries,
		HasMore:            &hasMore,
		Total:              &total,
		Metadata:           map[string]any{"source": source},
	}
}

func embeddedResolvedInstrument(
	query *broker.FeatureQuery,
	instrumentID string,
	market string,
) *broker.Instrument {
	code := instrumentID
	if _, value, ok := strings.Cut(instrumentID, "."); ok {
		code = value
	}
	productClass := query.ProductClass
	if productClass == "" {
		productClass = broker.ProductClassUnknown
	}
	segment := query.MarketSegment
	if segment == "" {
		segment = broker.MarketSegmentSecurities
	}
	return &broker.Instrument{
		InstrumentID:  instrumentID,
		Code:          code,
		ProductClass:  productClass,
		MarketSegment: segment,
		QuoteMarket:   market,
		TradeMarket:   market,
		QuantityMode:  broker.QuantityModeUnits,
	}
}
