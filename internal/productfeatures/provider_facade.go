package productfeatures

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jftrade/jftrade-main/internal/marketdata"
	"github.com/jftrade/jftrade-main/pkg/broker"
)

// EmbeddedResearchReader reads instrument news, corporate actions, market
// rankings, and industry boards from the embedded market-data provider.
// *marketdata.Service satisfies it.
type EmbeddedResearchReader interface {
	GetNews(ctx context.Context, market, symbol string, limit int) (marketdata.NewsResponse, error)
	GetCorporateActions(
		ctx context.Context,
		market, symbol string,
		from, to time.Time,
	) (marketdata.CorporateActionsResponse, error)
	GetRankings(ctx context.Context, market, kind string, limit int) (marketdata.RankingsResponse, error)
	GetIndustries(ctx context.Context, market, kind string) (marketdata.IndustryBoardsResponse, error)
	GetIndustryMembers(
		ctx context.Context,
		market, kind, board string,
		limit int,
	) (marketdata.IndustryMembersResponse, error)
}

// WithEmbeddedProviderResearch lets the product feature pipeline serve
// instrument news and corporate actions from the embedded market-data provider
// (yfinance/akshare) when that provider is active or explicitly requested. The
// market-data service is assembled after this service in the composition root,
// so both dependencies are looked up lazily per query.
func WithEmbeddedProviderResearch(
	reader func() EmbeddedResearchReader,
	activeProvider func(context.Context) (marketdata.ProviderDescriptor, error),
) Option {
	return func(service *Service) {
		service.embeddedReader = reader
		service.activeProvider = activeProvider
	}
}

// WithLazyEmbeddedProviderResearch adapts a market-data service that is
// assembled after this service in the composition root. The getter runs per
// query; while it returns nil the facade stays disabled.
func WithLazyEmbeddedProviderResearch(serviceFn func() *marketdata.Service) Option {
	return WithEmbeddedProviderResearch(
		func() EmbeddedResearchReader {
			if service := serviceFn(); service != nil {
				return service
			}
			return nil
		},
		func(ctx context.Context) (marketdata.ProviderDescriptor, error) {
			service := serviceFn()
			if service == nil {
				return marketdata.ProviderDescriptor{}, nil
			}
			return service.ProviderDescriptor(ctx)
		},
	)
}

// queryEmbeddedProviderResearch intercepts news, corporate-action, rankings,
// and industry-board reads before broker routing when the embedded market-data
// provider owns them. The boolean result reports whether the query was handled
// (a nil result with a true flag still carries the returned error).
func (s *Service) queryEmbeddedProviderResearch(
	ctx context.Context,
	query *broker.FeatureQuery,
	rawPageSize int,
) (*broker.FeatureResult, bool, error) {
	switch query.FeatureID {
	case broker.FeatureResearchNews, broker.FeatureResearchCorporateAction,
		broker.FeatureResearchRankings, broker.FeatureResearchIndustry:
	default:
		return nil, false, nil
	}
	if s.embeddedReader == nil || s.activeProvider == nil {
		return nil, false, nil
	}
	descriptor, err := s.activeProvider(ctx)
	if err != nil || !embeddedProviderServes(descriptor, query.BrokerID) {
		return nil, false, nil
	}
	reader := s.embeddedReader()
	if reader == nil {
		return nil, false, nil
	}
	instrumentScoped := query.FeatureID == broker.FeatureResearchNews ||
		query.FeatureID == broker.FeatureResearchCorporateAction
	market, symbol, ok := embeddedResearchInstrument(query)
	if instrumentScoped && !ok {
		return nil, false, nil
	}
	if market == "" {
		market = strings.ToUpper(strings.TrimSpace(query.Market))
	}
	if market == "" {
		market = strings.ToUpper(strings.TrimSpace(descriptor.DefaultMarket))
	}
	now := s.now().UTC()
	var result *broker.FeatureResult
	var readErr error
	switch query.FeatureID {
	case broker.FeatureResearchNews:
		result, readErr = s.embeddedNews(ctx, reader, descriptor, query, market, symbol, rawPageSize, now)
	case broker.FeatureResearchCorporateAction:
		result, readErr = s.embeddedCorporateActions(ctx, reader, descriptor, query, market, symbol, now)
	default:
		result, readErr = s.embeddedMarketResearch(ctx, reader, descriptor, query, market, symbol, rawPageSize, now)
	}
	if readErr != nil {
		return nil, true, mapEmbeddedProviderError(readErr, market)
	}
	return result, true, nil
}

// embeddedProviderServes mirrors usesActiveNonBrokerProvider in
// internal/api/marketdata: the embedded provider only serves a query when the
// active provider is not futu and an explicit brokerId matches the active
// descriptor's brokerId or providerId.
func embeddedProviderServes(descriptor marketdata.ProviderDescriptor, requested string) bool {
	brokerID := strings.TrimSpace(descriptor.BrokerID)
	providerID := strings.TrimSpace(descriptor.ProviderID)
	if brokerID == "" && providerID == "" {
		return false
	}
	if brokerID == "" || strings.EqualFold(brokerID, "futu") {
		return false
	}
	requested = strings.TrimSpace(requested)
	if requested == "" {
		return true
	}
	return strings.EqualFold(requested, brokerID) || strings.EqualFold(requested, providerID)
}

func embeddedResearchInstrument(query *broker.FeatureQuery) (string, string, bool) {
	instrumentID := strings.ToUpper(strings.TrimSpace(query.InstrumentID))
	if instrumentID == "" {
		return "", "", false
	}
	market := strings.ToUpper(strings.TrimSpace(query.Market))
	symbol := instrumentID
	if prefix, code, found := strings.Cut(instrumentID, "."); found {
		if market == "" {
			market = prefix
		}
		symbol = code
	}
	if market == "" || strings.TrimSpace(symbol) == "" {
		return "", "", false
	}
	return market, symbol, true
}

func (s *Service) embeddedNews(
	ctx context.Context,
	reader EmbeddedResearchReader,
	descriptor marketdata.ProviderDescriptor,
	query *broker.FeatureQuery,
	market string,
	symbol string,
	rawPageSize int,
	now time.Time,
) (*broker.FeatureResult, error) {
	response, err := reader.GetNews(ctx, market, symbol, embeddedNewsLimit(query, rawPageSize))
	if err != nil {
		return nil, err
	}
	return projectProviderNews(descriptor, query, response, market, now), nil
}

func (s *Service) embeddedCorporateActions(
	ctx context.Context,
	reader EmbeddedResearchReader,
	descriptor marketdata.ProviderDescriptor,
	query *broker.FeatureQuery,
	market string,
	symbol string,
	now time.Time,
) (*broker.FeatureResult, error) {
	response, err := reader.GetCorporateActions(ctx, market, symbol, now.AddDate(-2, 0, 0), now)
	if err != nil {
		return nil, err
	}
	return projectProviderCorporateActions(descriptor, query, response, market, now), nil
}

// embeddedNewsLimit prefers an explicit pageSize, then a limit param, then the
// provider default, clamped to the range the market-data service accepts.
func embeddedNewsLimit(query *broker.FeatureQuery, rawPageSize int) int {
	limit := rawPageSize
	if limit <= 0 {
		limit = int(int32Param(query.Params, "limit", 0))
	}
	if limit <= 0 {
		limit = marketdata.DefaultNewsLimit
	}
	return min(max(limit, 1), marketdata.MaxNewsLimit)
}

// mapEmbeddedProviderError folds the market-data capability error into the
// broker capability contract (HTTP 409); provider warming and busy sentinels
// pass through so the transport can map them to 503.
func mapEmbeddedProviderError(err error, market string) error {
	if errors.Is(err, marketdata.ErrCapabilityUnsupported) {
		return fmt.Errorf("%w: %w (market %s)", ErrCapabilityUnavailable, err, market)
	}
	return err
}
