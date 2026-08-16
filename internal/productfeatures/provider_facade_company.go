package productfeatures

import (
	"context"
	"strings"
	"time"

	"github.com/jftrade/jftrade-main/internal/marketdata"
	"github.com/jftrade/jftrade-main/pkg/broker"
)

// embeddedCompanyResearch routes instrument-scoped company research reads
// (profile, financials, analyst consensus, ownership) to the embedded
// market-data provider. Each feature accepts only its default operation
// (apps/web/src/components/research/useInstrumentResearchController.ts:36-47);
// any other operation resolves to a capability-unavailable error instead of
// falling through to broker routing.
func (s *Service) embeddedCompanyResearch(
	ctx context.Context,
	reader EmbeddedResearchReader,
	descriptor marketdata.ProviderDescriptor,
	query *broker.FeatureQuery,
	market string,
	symbol string,
	now time.Time,
) (*broker.FeatureResult, error) {
	operation := strings.ToLower(stringParam(query.Params, "operation"))
	switch query.FeatureID {
	case broker.FeatureResearchInstrument:
		if err := requireEmbeddedOperation(query.FeatureID, operation, "profile"); err != nil {
			return nil, err
		}
		response, err := reader.GetCompanyProfile(ctx, market, symbol)
		if err != nil {
			return nil, err
		}
		return projectProviderCompanyProfile(descriptor, query, response, market, now), nil
	case broker.FeatureResearchFinancials:
		if err := requireEmbeddedOperation(query.FeatureID, operation, "statements"); err != nil {
			return nil, err
		}
		statement := strings.ToLower(stringParam(query.Params, "statement"))
		response, err := reader.GetFinancialStatements(ctx, market, symbol, statement)
		if err != nil {
			return nil, err
		}
		return projectProviderFinancialStatements(descriptor, query, response, market, now), nil
	case broker.FeatureResearchAnalyst:
		if err := requireEmbeddedOperation(query.FeatureID, operation, "consensus"); err != nil {
			return nil, err
		}
		response, err := reader.GetAnalystConsensus(ctx, market, symbol)
		if err != nil {
			return nil, err
		}
		return projectProviderAnalystConsensus(descriptor, query, response, market, now), nil
	default:
		if err := requireEmbeddedOperation(query.FeatureID, operation, "overview"); err != nil {
			return nil, err
		}
		response, err := reader.GetOwnership(ctx, market, symbol)
		if err != nil {
			return nil, err
		}
		return projectProviderOwnership(descriptor, query, response, market, now), nil
	}
}

// requireEmbeddedOperation accepts the feature's single embedded operation (or
// an omitted operation) and rejects everything else as capability-unavailable.
func requireEmbeddedOperation(featureID broker.FeatureID, operation, want string) error {
	if operation == "" || operation == want {
		return nil
	}
	return errEmbeddedResearchOperation(featureID, operation)
}
