package productfeatures

import (
	"errors"
	"fmt"
	"maps"
	"strings"
	"testing"
	"time"

	"github.com/jftrade/jftrade-main/pkg/broker"
)

func TestProductFeatureServiceRemainingRoutingAndDegradationBranches(t *testing.T) {
	ctx := t.Context()
	registry := broker.NewRegistry()
	registry.Register(&featureBroker{id: "z-broker"})
	registry.Register(&featureBroker{id: "a-broker"})
	svc := NewService(registry, "a-broker", nil, nil)
	descriptors := svc.Capabilities()["brokers"].([]broker.Descriptor)
	if len(descriptors) != 2 || descriptors[0].ID != "a-broker" || descriptors[1].ID != "z-broker" {
		t.Fatalf("sorted capabilities = %#v", descriptors)
	}

	hkFirm := "FUTUSECURITIES"
	ineligible := &featureBroker{
		id: "ineligible-query", feature: broker.FeaturePredictionDiscover,
		accounts: []broker.Account{{
			ID: "hk-1", SecurityFirm: &hkFirm, MarketAuthorities: []string{"HK"},
		}},
	}
	ineligibleRegistry := broker.NewRegistry()
	ineligibleRegistry.Register(ineligible)
	ineligibleService := NewService(ineligibleRegistry, ineligible.id, nil, nil)
	if _, err := ineligibleService.Query(ctx, broker.FeatureQuery{
		BrokerID: ineligible.id, AccountID: "hk-1", Market: "US",
		FeatureID: broker.FeaturePredictionDiscover,
	}); !errors.Is(err, ErrPredictionIneligible) {
		t.Fatalf("ineligible prediction query error = %v", err)
	}

	nilResult := &featureBroker{
		id: "nil-result", feature: broker.FeatureResearchFinancials, queryNil: true,
	}
	nilRegistry := broker.NewRegistry()
	nilRegistry.Register(nilResult)
	nilService := NewService(nilRegistry, nilResult.id, nil, nil)
	result, err := nilService.Query(ctx, broker.FeatureQuery{
		BrokerID: nilResult.id, Market: "US", FeatureID: broker.FeatureResearchFinancials,
		Params: map[string]any{"refresh": true},
	})
	if err != nil || result == nil || result.Entries == nil || result.AsOf.IsZero() {
		t.Fatalf("nil adapter result normalization = %#v, %v", result, err)
	}

	firm := "FUTUINC"
	prediction := &featureBroker{
		id: "replace-reader",
		features: []broker.FeatureID{
			broker.FeaturePredictionDepth,
			broker.FeatureMarketSnapshots,
		},
		accounts: []broker.Account{{
			ID: "us-1", SecurityFirm: &firm, MarketAuthorities: []string{"US"},
		}},
	}
	predictionRegistry := broker.NewRegistry()
	predictionRegistry.Register(prediction)
	ensured := 0
	predictionService := NewService(predictionRegistry, prediction.id, nil, func() { ensured++ })
	if _, err := predictionService.BatchSnapshots(
		ctx, broker.FeatureQuery{}, []string{"US.AAPL"},
	); err != nil {
		t.Fatalf("ensured batch snapshots: %v", err)
	}
	lease, err := predictionService.AcquirePredictionSubscription(
		ctx, prediction.id, "us-1", "EVENTONE", []string{"ORDER_BOOK"},
	)
	if err != nil {
		t.Fatalf("ensured prediction lease: %v", err)
	}
	predictionRegistry.Replace(&bareFeatureBroker{
		id: prediction.id, feature: broker.FeaturePredictionDepth, accounts: prediction.accounts,
	})
	if err := predictionService.ReleasePredictionSubscription(ctx, lease.LeaseID); err != nil {
		t.Fatalf("release after adapter replacement: %v", err)
	}
	if ensured < 2 {
		t.Fatalf("ensure calls = %d, want batch and prediction checks", ensured)
	}
	if _, err := predictionService.AcquirePredictionSubscription(
		ctx, "missing", "us-1", "EVENTONE", []string{"ORDER_BOOK"},
	); !errors.Is(err, ErrCapabilityUnavailable) {
		t.Fatalf("missing prediction broker error = %v", err)
	}

	ineligiblePrediction := &featureBroker{
		id: "ineligible-subscription", feature: broker.FeaturePredictionDepth,
		accounts: []broker.Account{{
			ID: "hk-1", SecurityFirm: &hkFirm, MarketAuthorities: []string{"HK"},
		}},
	}
	ineligiblePredictionRegistry := broker.NewRegistry()
	ineligiblePredictionRegistry.Register(ineligiblePrediction)
	if _, err := NewService(
		ineligiblePredictionRegistry,
		ineligiblePrediction.id,
		nil,
		nil,
	).AcquirePredictionSubscription(
		ctx, ineligiblePrediction.id, "hk-1", "EVENTONE", []string{"ORDER_BOOK"},
	); !errors.Is(err, ErrPredictionIneligible) {
		t.Fatalf("ineligible prediction subscription error = %v", err)
	}

	wrongMarket := &featureBroker{
		id: "wrong-market-snapshot", feature: broker.FeatureMarketSnapshots,
		markets: []string{"HK"},
	}
	wrongMarketRegistry := broker.NewRegistry()
	wrongMarketRegistry.Register(wrongMarket)
	if _, _, err := NewService(
		wrongMarketRegistry,
		wrongMarket.id,
		nil,
		nil,
	).resolveBatchSnapshotSource(
		broker.FeatureQuery{FeatureID: broker.FeatureMarketSnapshots},
		[]string{"US.AAPL"},
	); !errors.Is(err, ErrCapabilityUnavailable) {
		t.Fatalf("resolved snapshot market validation error = %v", err)
	}

	writeOnly := &bareFeatureBroker{id: "write-only", feature: broker.FeaturePriceAlertSet}
	writeRegistry := broker.NewRegistry()
	writeRegistry.Register(writeOnly)
	if _, err := NewService(writeRegistry, writeOnly.id, nil, nil).ApplyCustomization(
		ctx,
		broker.CustomizationAction{
			BrokerID: writeOnly.id, FeatureID: broker.FeaturePriceAlertSet,
		},
	); err == nil {
		t.Fatal("write capability without CustomizationService succeeded")
	}

	if _, err := normalizeSnapshotSymbols([]string{}); err == nil {
		t.Fatal("empty normalized snapshot set succeeded")
	}
	if err := validateSnapshotMarkets(broker.Descriptor{
		ID: "wrong-feature",
		Capabilities: []broker.MarketCapability{{
			Market: "US", Features: []broker.FeatureCapability{{
				ID: broker.FeatureMarketSnapshot, State: broker.CapabilityAvailable,
			}},
		}},
	}, []string{"US.AAPL"}); err == nil {
		t.Fatal("descriptor without batch snapshot feature succeeded")
	}
	if err := validateSnapshotMarkets(broker.Descriptor{
		ID: "multi-market",
		Capabilities: []broker.MarketCapability{
			{Market: "HK"},
			{
				Market: "US", Features: []broker.FeatureCapability{{
					ID: broker.FeatureMarketSnapshots, State: broker.CapabilityAvailable,
				}},
			},
		},
	}, []string{"US.AAPL"}); err != nil {
		t.Fatalf("multi-market descriptor validation: %v", err)
	}
	query := broker.FeatureQuery{PageSize: 1001}
	normalizeQuery(&query)
	if query.PageSize != 1000 || query.Params == nil {
		t.Fatalf("normalized oversized query = %#v", query)
	}

	eligibleAfterMismatch := &featureBroker{id: "account-filter", accounts: []broker.Account{
		{ID: "other", SecurityFirm: &firm, MarketAuthorities: []string{"US"}},
		{ID: "target", SecurityFirm: &firm},
	}}
	if got, err := predictionEligibility(ctx, eligibleAfterMismatch, "target"); err != nil || got != firm {
		t.Fatalf("filtered prediction eligibility = %q, %v", got, err)
	}
}

func TestOptionFeatureValidationRejectsMalformedAdvancedFilters(t *testing.T) {
	base := broker.FeatureQuery{
		FeatureID:    broker.FeatureOptionEvents,
		Market:       "US",
		InstrumentID: "US.AAPL260718C00200000",
		Params: map[string]any{
			"operation":       "zero_dte_contract",
			"expiryTimestamp": "1784390400",
		},
	}
	tests := []struct {
		name   string
		mutate func(*broker.FeatureQuery)
	}{
		{
			name: "unserializable chain locator",
			mutate: func(query *broker.FeatureQuery) {
				query.Params["chainLocator"] = make(chan int)
			},
		},
		{
			name: "chain locator without product code",
			mutate: func(query *broker.FeatureQuery) {
				query.Params["chainLocator"] = map[string]any{"market": "US"}
			},
		},
		{
			name: "unsupported contract sort",
			mutate: func(query *broker.FeatureQuery) {
				query.Params["chainLocator"] = map[string]any{"productCode": "AAPL"}
				query.Params["sort"] = "gamma"
			},
		},
		{
			name: "unsupported option type",
			mutate: func(query *broker.FeatureQuery) {
				query.Params["chainLocator"] = map[string]any{"productCode": "AAPL"}
				query.Params["optionType"] = "straddle"
			},
		},
		{
			name: "unsupported seller strategy",
			mutate: func(query *broker.FeatureQuery) {
				query.Params = map[string]any{"operation": "seller", "sellerStrategy": "naked_call"}
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			query := base
			query.Params = map[string]any{}
			maps.Copy(query.Params, base.Params)
			test.mutate(&query)
			if err := validateOptionFeatureQuery(query); err == nil {
				t.Fatal("malformed option query succeeded")
			}
		})
	}

	if _, err := queryCoreMarketDataFeature(
		t.Context(),
		&featureBroker{id: "core", marketData: &featureMarketDataReader{}},
		broker.FeatureQuery{FeatureID: broker.FeatureMarketCandles},
	); !errors.Is(err, ErrInvalidQuery) {
		t.Fatalf("invalid normalized core candle query error = %v", err)
	}

	for _, query := range []broker.FeatureQuery{
		{
			FeatureID: broker.FeatureOptionAnalysis, Market: "US", InstrumentID: "US.AAPL",
			Params: map[string]any{"operation": "quote"},
		},
		{
			FeatureID: broker.FeatureOptionAnalysis, Market: "US", InstrumentID: "US.AAPL260718C00200000",
			Params: map[string]any{"operation": "underlying_overview"},
		},
		{
			FeatureID: broker.FeatureOptionEvents, Market: "HK",
			Params: map[string]any{"operation": "zero_dte"},
		},
		{
			FeatureID: broker.FeatureOptionEvents, Market: "US", InstrumentID: "US.AAPL",
			Params: map[string]any{"operation": "zero_dte_contract"},
		},
	} {
		if err := validateOptionFeatureQuery(query); err == nil {
			t.Fatalf("invalid option query succeeded: %#v", query)
		}
	}

	registry := broker.NewRegistry()
	adapter := &featureBroker{id: "option-validation", feature: broker.FeatureOptionAnalysis}
	registry.Register(adapter)
	if _, err := NewService(registry, adapter.id, nil, nil).Query(t.Context(), broker.FeatureQuery{
		BrokerID: adapter.id, FeatureID: broker.FeatureOptionAnalysis,
		Market: "US", InstrumentID: "US.AAPL", Params: map[string]any{"operation": "quote"},
	}); !errors.Is(err, ErrInvalidQuery) {
		t.Fatalf("service option validation error = %v", err)
	}
}

func TestResearchInstitutionDetailQueriesRequireInstitutionID(t *testing.T) {
	for _, operation := range []string{
		"profile",
		"distribution",
		"holding_changes",
		"holdings",
	} {
		t.Run(operation+"/missing", func(t *testing.T) {
			err := validateResearchInstitutionQuery(broker.FeatureQuery{
				FeatureID: broker.FeatureResearchInstitutions,
				Params:    map[string]any{"operation": operation},
			})
			if !errors.Is(err, ErrInvalidQuery) ||
				!strings.Contains(err.Error(), "positive integer institutionId") {
				t.Fatalf("error = %v, want ErrInvalidQuery for institutionId", err)
			}
		})
	}

	for _, institutionID := range []any{0, -1, 1.5, "not-an-id", int64(1 << 32)} {
		t.Run(fmt.Sprint(institutionID), func(t *testing.T) {
			err := validateResearchInstitutionQuery(broker.FeatureQuery{
				FeatureID: broker.FeatureResearchInstitutions,
				Params: map[string]any{
					"operation":     "holding_changes",
					"institutionId": institutionID,
				},
			})
			if !errors.Is(err, ErrInvalidQuery) {
				t.Fatalf("error = %v, want ErrInvalidQuery", err)
			}
		})
	}

	for _, query := range []broker.FeatureQuery{
		{
			FeatureID: broker.FeatureResearchInstitutions,
			Params:    map[string]any{"operation": "list"},
		},
		{
			FeatureID: broker.FeatureResearchInstitutions,
			Params: map[string]any{
				"operation":     "holding_changes",
				"institutionId": int64(202),
			},
		},
		{
			FeatureID: broker.FeatureResearchCalendar,
			Params:    map[string]any{"operation": "holding_changes"},
		},
	} {
		if err := validateResearchInstitutionQuery(query); err != nil {
			t.Fatalf("valid query rejected: query=%#v error=%v", query, err)
		}
	}
}

func TestQueryUsesFreshPredictionPushBeforePolling(t *testing.T) {
	firm := "FUTUINC"
	adapter := &streamFeatureBroker{featureBroker: &featureBroker{
		id: "push-query", feature: broker.FeaturePredictionDepth,
		accounts: []broker.Account{{
			ID: "eligible", SecurityFirm: &firm, MarketAuthorities: []string{"US"},
		}},
	}}
	registry := broker.NewRegistry()
	registry.Register(adapter)
	service := NewService(registry, adapter.id, nil, nil)
	now := time.Date(2026, 7, 18, 12, 0, 0, 0, time.UTC)
	service.now = func() time.Time { return now }
	service.ensurePredictionPushSource(adapter.id, adapter)
	adapter.listener(broker.PredictionMarketUpdate{
		InstrumentID: "US.EVENT", DataType: "ORDER_BOOK", Sequence: "7", AsOf: now,
		Entries: []map[string]any{{"price": 0.63}},
	})

	result, err := service.Query(t.Context(), broker.FeatureQuery{
		BrokerID: adapter.id, AccountID: "eligible", Market: "US",
		InstrumentID: "US.EVENT", FeatureID: broker.FeaturePredictionDepth,
		Params: map[string]any{},
	})
	if err != nil || len(result.Entries) != 1 || result.Entries[0]["price"] != 0.63 || adapter.queryCalls != 0 {
		t.Fatalf("pushed query result = %#v, calls=%d, err=%v", result, adapter.queryCalls, err)
	}
}
