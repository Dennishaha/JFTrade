package productfeatures

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/jftrade/jftrade-main/pkg/broker"
)

func normalizeQuery(query *broker.FeatureQuery) {
	query.BrokerID = strings.TrimSpace(query.BrokerID)
	query.AccountID = strings.TrimSpace(query.AccountID)
	query.TradingEnvironment = strings.ToUpper(strings.TrimSpace(query.TradingEnvironment))
	query.Market = strings.ToUpper(strings.TrimSpace(query.Market))
	query.InstrumentID = strings.ToUpper(strings.TrimSpace(query.InstrumentID))
	query.Cursor = strings.TrimSpace(query.Cursor)
	if query.PageSize <= 0 {
		query.PageSize = 100
	}
	if query.PageSize > 1000 {
		query.PageSize = 1000
	}
	if query.Params == nil {
		query.Params = make(map[string]any)
	}
}

func predictionEligibility(ctx context.Context, selected broker.Broker, accountID string) (string, error) {
	accounts, err := selected.DiscoverAccounts(ctx)
	if err != nil {
		return "", fmt.Errorf("%w: account eligibility could not be verified: %v", ErrPredictionIneligible, err)
	}
	for _, account := range accounts {
		if accountID != "" && account.ID != accountID {
			continue
		}
		firm := ""
		if account.SecurityFirm != nil {
			firm = strings.ToUpper(strings.TrimSpace(*account.SecurityFirm))
		}
		if firm != "FUTUINC" {
			continue
		}
		if len(account.MarketAuthorities) > 0 && !containsFold(account.MarketAuthorities, "US") {
			continue
		}
		return firm, nil
	}
	return "", ErrPredictionIneligible
}

func containsFold(values []string, target string) bool {
	for _, value := range values {
		if strings.EqualFold(value, target) {
			return true
		}
	}
	return false
}

func missingInterface(id broker.FeatureID, name string) error {
	return fmt.Errorf("%w: broker does not implement %s for %s", ErrCapabilityUnavailable, name, id)
}

func featureTTL(query broker.FeatureQuery) time.Duration {
	switch query.FeatureID {
	case broker.FeatureMarketIntraday, broker.FeatureMarketTicks, broker.FeatureMarketDepth,
		broker.FeatureMarketBrokerQueue, broker.FeaturePredictionDepth:
		return 0
	case broker.FeatureResearchNews:
		return time.Minute
	case broker.FeatureOptionChain, broker.FeatureOptionScreen, broker.FeatureOptionAnalysis,
		broker.FeatureOptionEvents, broker.FeatureWarrants, broker.FeatureFutures,
		broker.FeatureResearchScreen, broker.FeatureResearchRankings:
		return 45 * time.Second
	case broker.FeaturePredictionComboQuote:
		return 0
	case broker.FeaturePredictionSnapshot:
		return 3 * time.Second
	case broker.FeaturePredictionHistory, broker.FeaturePredictionComboEligible:
		return 30 * time.Second
	case broker.FeaturePredictionDiscover, broker.FeatureInstrumentProfile,
		broker.FeatureResearchInstrument:
		return 15 * time.Minute
	case broker.FeatureResearchFinancials, broker.FeatureResearchValuation,
		broker.FeatureResearchAnalyst, broker.FeatureResearchOwnership,
		broker.FeatureResearchCorporateAction, broker.FeatureResearchShortInterest,
		broker.FeatureResearchCalendar, broker.FeatureResearchMacro,
		broker.FeatureResearchInstitutions, broker.FeatureResearchIndustry:
		return time.Hour
	default:
		return 30 * time.Second
	}
}

func queryCacheKey(query broker.FeatureQuery, version string) string {
	content, _ := json.Marshal(query)
	return version + ":" + string(content)
}

func (s *Service) cached(key string) *broker.FeatureResult {
	now := s.now()
	s.cacheMu.Lock()
	defer s.cacheMu.Unlock()
	entry, ok := s.cache[key]
	if !ok {
		return nil
	}
	if !now.Before(entry.expiresAt) {
		delete(s.cache, key)
		return nil
	}
	result := cloneResult(entry.result)
	if result.Metadata == nil {
		result.Metadata = make(map[string]any)
	}
	result.Metadata["fromCache"] = true
	return result
}

func (s *Service) putCache(key string, result *broker.FeatureResult, ttl time.Duration) {
	s.cacheMu.Lock()
	s.cache[key] = cacheEntry{expiresAt: s.now().Add(ttl), result: cloneResult(result)}
	s.cacheMu.Unlock()
}

func cloneResult(source *broker.FeatureResult) *broker.FeatureResult {
	if source == nil {
		return nil
	}
	content, _ := json.Marshal(source)
	var result broker.FeatureResult
	_ = json.Unmarshal(content, &result)
	return &result
}

func boolParam(params map[string]any, key string) bool {
	value, _ := params[key].(bool)
	return value
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
