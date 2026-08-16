package productfeatures

import (
	"context"
	"fmt"

	"github.com/jftrade/jftrade-main/pkg/broker"
)

// queryResolvedFeature dispatches a routed read to the optional broker adapter
// interface declared by the capability catalog.
func queryResolvedFeature(
	ctx context.Context,
	selected broker.Broker,
	adapterInterface string,
	query broker.FeatureQuery,
) (*broker.FeatureResult, error) {
	switch adapterInterface {
	case "MarketDataReader":
		return queryCoreMarketDataFeature(ctx, selected, query)
	case "BatchSnapshotSource":
		return nil, fmt.Errorf("feature %q is served by the snapshot service", query.FeatureID)
	case "MarketMicrostructureReader":
		reader, ok := selected.(broker.MarketMicrostructureReader)
		if !ok {
			return nil, missingInterface(query.FeatureID, adapterInterface)
		}
		return reader.QueryMarketMicrostructure(ctx, query)
	case "InstrumentProfileReader":
		reader, ok := selected.(broker.InstrumentProfileReader)
		if !ok {
			return nil, missingInterface(query.FeatureID, adapterInterface)
		}
		return reader.QueryInstrumentProfile(ctx, query)
	case "DerivativeCatalogReader":
		reader, ok := selected.(broker.DerivativeCatalogReader)
		if !ok {
			return nil, missingInterface(query.FeatureID, adapterInterface)
		}
		return reader.QueryDerivativeCatalog(ctx, query)
	case "OptionAnalyticsReader":
		reader, ok := selected.(broker.OptionAnalyticsReader)
		if !ok {
			return nil, missingInterface(query.FeatureID, adapterInterface)
		}
		return reader.QueryOptionAnalytics(ctx, query)
	case "InstrumentResearchReader":
		reader, ok := selected.(broker.InstrumentResearchReader)
		if !ok {
			return nil, missingInterface(query.FeatureID, adapterInterface)
		}
		return reader.QueryInstrumentResearch(ctx, query)
	case "MarketResearchReader":
		reader, ok := selected.(broker.MarketResearchReader)
		if !ok {
			return nil, missingInterface(query.FeatureID, adapterInterface)
		}
		return reader.QueryMarketResearch(ctx, query)
	case "PredictionMarketReader":
		reader, ok := selected.(broker.PredictionMarketReader)
		if !ok {
			return nil, missingInterface(query.FeatureID, adapterInterface)
		}
		return reader.QueryPredictionMarket(ctx, query)
	case "TechnicalIndicatorReader":
		reader, ok := selected.(broker.TechnicalIndicatorReader)
		if !ok {
			return nil, missingInterface(query.FeatureID, adapterInterface)
		}
		return reader.QueryTechnicalIndicator(ctx, query)
	case "CustomizationService":
		reader, ok := selected.(broker.CustomizationService)
		if !ok {
			return nil, missingInterface(query.FeatureID, adapterInterface)
		}
		return reader.QueryCustomization(ctx, query)
	default:
		return nil, fmt.Errorf("feature %q has unsupported adapter interface %q", query.FeatureID, adapterInterface)
	}
}
