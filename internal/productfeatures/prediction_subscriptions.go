package productfeatures

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/jftrade/jftrade-main/pkg/broker"
)

func (s *Service) AcquirePredictionSubscription(
	ctx context.Context,
	brokerID, accountID, instrumentID string,
	dataTypes []string,
) (*PredictionSubscriptionLease, error) {
	if s == nil || s.router == nil {
		return nil, fmt.Errorf("product feature service is unavailable")
	}
	if s.ensure != nil {
		s.ensure()
	}
	instrumentID = normalizePredictionInstrumentID(instrumentID)
	normalizedTypes, err := normalizePredictionDataTypes(dataTypes)
	if err != nil || instrumentID == "" {
		return nil, fmt.Errorf("%w: invalid prediction subscription", ErrInvalidQuery)
	}
	featureID := broker.FeaturePredictionHistory
	if len(normalizedTypes) == 1 && normalizedTypes[0] == "ORDER_BOOK" {
		featureID = broker.FeaturePredictionDepth
	}
	resolution, err := s.router.ResolveContext(ctx, broker.FeatureRouteRequest{
		BrokerID: brokerID, AccountID: accountID,
		FeatureID: featureID, Market: "US",
		MarketSegment: broker.MarketSegmentPrediction,
		ProductClass:  broker.ProductClassEventContract,
	})
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrCapabilityUnavailable, err)
	}
	securityFirm, err := predictionEligibility(ctx, resolution.Broker, accountID)
	if err != nil {
		return nil, err
	}
	// BrokerFeatureRouter verifies the catalog adapter interface before it
	// returns a resolution.
	reader := resolution.Broker.(broker.PredictionMarketReader)
	s.ensurePredictionPushSource(resolution.BrokerID, resolution.Broker)
	subscription := broker.PredictionSubscription{
		BrokerID: resolution.BrokerID, AccountID: accountID,
		InstrumentID: instrumentID, DataTypes: normalizedTypes,
	}
	key := strings.Join([]string{
		resolution.BrokerID, accountID, instrumentID, strings.Join(normalizedTypes, ","),
	}, "|")
	provider := broker.ProviderAttribution{
		BrokerID: resolution.BrokerID, SecurityFirm: securityFirm, FeatureID: featureID,
		Capability: resolution.Capability.State, SelectionReason: resolution.SelectionReason,
		ResolvedAt: resolution.ResolvedAt, AsOf: s.now().UTC(),
	}

	s.predictionSubscriptionMu.Lock()
	defer s.predictionSubscriptionMu.Unlock()
	if s.predictionSubscriptionCounts[key] == 0 {
		if err := reader.SubscribePredictionMarket(ctx, subscription); err != nil {
			return nil, err
		}
	}
	s.predictionSubscriptionCounts[key]++
	s.predictionSubscriptionSeq++
	leaseID := fmt.Sprintf("prediction-lease-%d-%d", s.now().UTC().UnixNano(), s.predictionSubscriptionSeq)
	s.predictionSubscriptionLeases[leaseID] = predictionSubscriptionLease{
		Key: key, BrokerID: resolution.BrokerID, AccountID: accountID,
		InstrumentID: instrumentID, DataTypes: append([]string(nil), normalizedTypes...),
		Provider: provider,
	}
	return &PredictionSubscriptionLease{
		LeaseID: leaseID, InstrumentID: instrumentID,
		DataTypes: append([]string(nil), normalizedTypes...), Provider: provider,
	}, nil
}

func (s *Service) ensurePredictionPushSource(brokerID string, selected broker.Broker) {
	source, ok := selected.(broker.PredictionMarketStreamSource)
	if !ok {
		return
	}
	s.predictionPushMu.Lock()
	if s.predictionPushUnsubscribe[brokerID] != nil {
		s.predictionPushMu.Unlock()
		return
	}
	s.predictionPushUnsubscribe[brokerID] = source.OnPredictionMarketUpdate(
		func(update broker.PredictionMarketUpdate) {
			key := predictionPushKey(brokerID, update.InstrumentID, update.DataType)
			s.predictionPushMu.Lock()
			current, exists := s.predictionPushCache[key]
			if !exists || update.Sequence == "" || current.Sequence != update.Sequence {
				s.predictionPushCache[key] = update
			}
			s.predictionPushMu.Unlock()
		},
	)
	s.predictionPushMu.Unlock()
}

func (s *Service) predictionPushResult(query broker.FeatureQuery) *broker.FeatureResult {
	dataType := ""
	operation := strings.ToLower(strings.TrimSpace(fmt.Sprint(query.Params["operation"])))
	switch {
	case query.FeatureID == broker.FeaturePredictionDepth:
		dataType = "ORDER_BOOK"
	case query.FeatureID == broker.FeaturePredictionHistory && operation == "candles":
		dataType = "KLINE"
	case query.FeatureID == broker.FeaturePredictionHistory && operation == "ticks":
		dataType = "TICKER"
	default:
		return nil
	}
	key := predictionPushKey(query.BrokerID, query.InstrumentID, dataType)
	s.predictionPushMu.Lock()
	update, ok := s.predictionPushCache[key]
	s.predictionPushMu.Unlock()
	if !ok || s.now().UTC().Sub(update.AsOf) > 5*time.Second {
		return nil
	}
	return &broker.FeatureResult{
		AsOf: update.AsOf, Entries: append([]map[string]any(nil), update.Entries...),
		Metadata: map[string]any{"source": "push", "dataType": dataType, "sequence": update.Sequence},
	}
}

func predictionPushKey(brokerID, instrumentID, dataType string) string {
	return strings.ToLower(strings.TrimSpace(brokerID)) + "|" +
		strings.ToUpper(strings.TrimSpace(instrumentID)) + "|" +
		strings.ToUpper(strings.TrimSpace(dataType))
}

func (s *Service) ReleasePredictionSubscription(ctx context.Context, leaseID string) error {
	leaseID = strings.TrimSpace(leaseID)
	if leaseID == "" {
		return fmt.Errorf("%w: prediction subscription leaseId is required", ErrInvalidQuery)
	}
	s.predictionSubscriptionMu.Lock()
	defer s.predictionSubscriptionMu.Unlock()
	lease, ok := s.predictionSubscriptionLeases[leaseID]
	if !ok {
		return nil
	}
	delete(s.predictionSubscriptionLeases, leaseID)
	count := s.predictionSubscriptionCounts[lease.Key]
	if count > 1 {
		s.predictionSubscriptionCounts[lease.Key] = count - 1
		return nil
	}
	delete(s.predictionSubscriptionCounts, lease.Key)
	selected := s.registry.Lookup(lease.BrokerID)
	reader, ok := selected.(broker.PredictionMarketReader)
	if !ok {
		return nil
	}
	return reader.UnsubscribePredictionMarket(ctx, broker.PredictionSubscription{
		BrokerID: lease.BrokerID, AccountID: lease.AccountID,
		InstrumentID: lease.InstrumentID, DataTypes: lease.DataTypes,
	})
}

func normalizePredictionInstrumentID(value string) string {
	value = strings.ToUpper(strings.TrimSpace(value))
	code := strings.TrimPrefix(value, "US.")
	if code == "" {
		return ""
	}
	return "US." + code
}

func normalizePredictionDataTypes(values []string) ([]string, error) {
	allowed := map[string]struct{}{"ORDER_BOOK": {}, "KLINE": {}, "TICKER": {}}
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.ToUpper(strings.TrimSpace(value))
		if _, ok := allowed[value]; !ok {
			return nil, fmt.Errorf("unsupported prediction data type %q", value)
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	if len(result) == 0 {
		return nil, fmt.Errorf("at least one prediction data type is required")
	}
	slices.Sort(result)
	return result, nil
}
