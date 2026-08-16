package productfeatures

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jftrade/jftrade-main/internal/marketdata"
	"github.com/jftrade/jftrade-main/pkg/broker"
)

// embeddedMarketResearch routes market-scoped research operations (rankings
// and industry boards) to the embedded market-data provider. Operations without
// an embedded feed resolve to a capability-unavailable error instead of falling
// through to broker routing.
func (s *Service) embeddedMarketResearch(
	ctx context.Context,
	reader EmbeddedResearchReader,
	descriptor marketdata.ProviderDescriptor,
	query *broker.FeatureQuery,
	market string,
	symbol string,
	rawPageSize int,
	now time.Time,
) (*broker.FeatureResult, error) {
	operation := strings.ToLower(stringParam(query.Params, "operation"))
	if query.FeatureID == broker.FeatureResearchRankings {
		return s.embeddedRankings(ctx, reader, descriptor, query, market, operation, rawPageSize, now)
	}
	return s.embeddedIndustry(ctx, reader, descriptor, query, market, symbol, operation, rawPageSize, now)
}

// embeddedRankings maps the frontend ranking operations
// (apps/web/src/components/research/MarketRankingsView.vue:69-91 and
// apps/web/src/components/research/MarketHomeView.vue:32-63) to provider kinds.
func (s *Service) embeddedRankings(
	ctx context.Context,
	reader EmbeddedResearchReader,
	descriptor marketdata.ProviderDescriptor,
	query *broker.FeatureQuery,
	market string,
	operation string,
	rawPageSize int,
	now time.Time,
) (*broker.FeatureResult, error) {
	switch operation {
	case "top_movers":
		kind := "gainers"
		if strings.ToLower(stringParam(query.Params, "direction")) == "down" {
			kind = "losers"
		}
		response, err := reader.GetRankings(ctx, market, kind, embeddedRankingsLimit(query, rawPageSize))
		if err != nil {
			return nil, err
		}
		return projectProviderRankings(descriptor, query, response, market, now), nil
	case "hot":
		response, err := reader.GetRankings(ctx, market, "active", embeddedRankingsLimit(query, rawPageSize))
		if err != nil {
			return nil, err
		}
		return projectProviderRankings(descriptor, query, response, market, now), nil
	case "heatmap":
		return s.embeddedIndustryBoards(ctx, reader, descriptor, query, market, now)
	default:
		return nil, errEmbeddedResearchOperation(query.FeatureID, operation)
	}
}

// embeddedIndustry maps the frontend industry operations
// (apps/web/src/components/research/ConceptSectorView.vue:31-41 plate_list,
// ConceptSectorView.vue:84-98 plate_members) to board reads. Industry chain
// operations (chains, chain_detail, chains_by_plate, plate, plate_stocks) have
// no embedded feed.
func (s *Service) embeddedIndustry(
	ctx context.Context,
	reader EmbeddedResearchReader,
	descriptor marketdata.ProviderDescriptor,
	query *broker.FeatureQuery,
	market string,
	symbol string,
	operation string,
	rawPageSize int,
	now time.Time,
) (*broker.FeatureResult, error) {
	switch operation {
	case "plate_list":
		return s.embeddedIndustryBoards(ctx, reader, descriptor, query, market, now)
	case "plate_members":
		board := symbol
		if board == "" {
			board = strings.TrimSpace(stringParam(query.Params, "plateId"))
		}
		if board == "" {
			return nil, fmt.Errorf("plate_members requires a plate instrumentId")
		}
		kind, err := embeddedMemberBoardKind(query)
		if err != nil {
			return nil, err
		}
		response, err := reader.GetIndustryMembers(
			ctx, market, kind, board, embeddedRankingsLimit(query, rawPageSize),
		)
		if err != nil {
			return nil, err
		}
		return projectProviderIndustryMembers(descriptor, query, response, market, now), nil
	default:
		return nil, errEmbeddedResearchOperation(query.FeatureID, operation)
	}
}

func (s *Service) embeddedIndustryBoards(
	ctx context.Context,
	reader EmbeddedResearchReader,
	descriptor marketdata.ProviderDescriptor,
	query *broker.FeatureQuery,
	market string,
	now time.Time,
) (*broker.FeatureResult, error) {
	kind, err := embeddedBoardKind(query)
	if err != nil {
		return nil, err
	}
	response, err := reader.GetIndustries(ctx, market, kind)
	if err != nil {
		return nil, err
	}
	return projectProviderIndustryBoards(descriptor, query, response, market, now), nil
}

// embeddedBoardKind maps the frontend plateType to the sidecar board kind;
// region/theme boards have no embedded feed.
func embeddedBoardKind(query *broker.FeatureQuery) (string, error) {
	switch plateType := strings.ToLower(stringParam(query.Params, "plateType")); plateType {
	case "", "industry":
		return "industry", nil
	case "concept":
		return "concept", nil
	default:
		return "", fmt.Errorf(
			"%w: embedded market-data provider does not serve %s plateType %q",
			ErrCapabilityUnavailable, query.FeatureID, plateType,
		)
	}
}

// embeddedMemberBoardKind keeps an omitted plateType empty so the sidecar
// resolves the board by name; ConceptSectorView does not resend plateType on
// plate_members requests.
func embeddedMemberBoardKind(query *broker.FeatureQuery) (string, error) {
	switch plateType := strings.ToLower(stringParam(query.Params, "plateType")); plateType {
	case "":
		return "", nil
	case "industry", "concept":
		return plateType, nil
	default:
		return "", fmt.Errorf(
			"%w: embedded market-data provider does not serve %s plateType %q",
			ErrCapabilityUnavailable, query.FeatureID, plateType,
		)
	}
}

// errEmbeddedResearchOperation reports an operation the embedded provider has
// no feed for. It rides the ErrCapabilityUnavailable 409 transport mapping so
// the console renders the provider-unsupported state instead of a broker
// resolution failure.
func errEmbeddedResearchOperation(featureID broker.FeatureID, operation string) error {
	if operation == "" {
		operation = "(none)"
	}
	return fmt.Errorf(
		"%w: embedded market-data provider does not serve %s operation %q",
		ErrCapabilityUnavailable, featureID, operation,
	)
}

// embeddedRankingsLimit mirrors embeddedNewsLimit for market-wide lists.
func embeddedRankingsLimit(query *broker.FeatureQuery, rawPageSize int) int {
	limit := rawPageSize
	if limit <= 0 {
		limit = int(int32Param(query.Params, "limit", 0))
	}
	if limit <= 0 {
		limit = marketdata.DefaultRankingsLimit
	}
	return min(max(limit, 1), marketdata.MaxRankingsLimit)
}
