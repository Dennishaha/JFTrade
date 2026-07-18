package productfeatures

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jftrade/jftrade-main/pkg/broker"
)

func TestPredictionEligibilityRejectsFutuSecuritiesAndAcceptsFutuInc(t *testing.T) {
	hkFirm := "FUTUSECURITIES"
	usFirm := "FUTUINC"
	hk := &featureBroker{id: "hk", accounts: []broker.Account{{
		ID: "1", SecurityFirm: &hkFirm, MarketAuthorities: []string{"HK"},
	}}}
	if _, err := predictionEligibility(t.Context(), hk, "1"); !errors.Is(err, ErrPredictionIneligible) {
		t.Fatalf("HK prediction eligibility error = %v", err)
	}

	us := &featureBroker{id: "us", accounts: []broker.Account{{
		ID: "2", SecurityFirm: &usFirm, MarketAuthorities: []string{"US"},
	}}}
	if firm, err := predictionEligibility(t.Context(), us, "2"); err != nil || firm != "FUTUINC" {
		t.Fatalf("US prediction eligibility = %q, %v", firm, err)
	}
}

func TestQueryDoesNotFallbackWhenBrokerIsExplicit(t *testing.T) {
	registry := broker.NewRegistry()
	registry.Register(&featureBroker{id: "first", feature: broker.FeatureMarketIntraday})
	registry.Register(&featureBroker{id: "second", feature: broker.FeatureOptionChain})
	service := NewService(registry, "first", []string{"second"}, nil)

	_, err := service.Query(t.Context(), broker.FeatureQuery{
		BrokerID:  "first",
		Market:    "US",
		FeatureID: broker.FeatureOptionChain,
	})
	if !errors.Is(err, ErrCapabilityUnavailable) {
		t.Fatalf("Query() error = %v, want explicit broker capability error", err)
	}
}

func TestBatchSnapshotsUsesOptionalSourceWithoutSubscriptions(t *testing.T) {
	registry := broker.NewRegistry()
	registry.Register(&featureBroker{id: "snapshot-broker", feature: broker.FeatureMarketSnapshots})
	service := NewService(registry, "snapshot-broker", nil, nil)
	fixedNow := time.Date(2026, 7, 17, 9, 30, 0, 0, time.UTC)
	service.now = func() time.Time { return fixedNow }

	result, err := service.BatchSnapshots(t.Context(), broker.FeatureQuery{
		BrokerID:  "snapshot-broker",
		AccountID: "account-1",
	}, []string{" us.aapl ", "US.AAPL"})
	if err != nil {
		t.Fatalf("BatchSnapshots() error = %v", err)
	}
	if len(result.Entries) != 1 || result.Entries[0]["symbol"] != "US.AAPL" {
		t.Fatalf("BatchSnapshots() entries = %#v", result.Entries)
	}
	if result.Provider.BrokerID != "snapshot-broker" || result.Provider.SelectionReason != "explicit_broker" {
		t.Fatalf("BatchSnapshots() provider = %#v", result.Provider)
	}
	if result.Metadata["subscriptionCreated"] != false {
		t.Fatalf("BatchSnapshots() metadata = %#v, want subscriptionCreated=false", result.Metadata)
	}

	cached, err := service.BatchSnapshots(t.Context(), broker.FeatureQuery{
		BrokerID:  "snapshot-broker",
		AccountID: "account-1",
	}, []string{"US.AAPL"})
	if err != nil {
		t.Fatalf("cached BatchSnapshots() error = %v", err)
	}
	if cached.Metadata["fromCache"] != true {
		t.Fatalf("cached BatchSnapshots() metadata = %#v", cached.Metadata)
	}
}

func TestBatchSnapshotsRejectsUnsupportedRegionsAndOversizedRequests(t *testing.T) {
	registry := broker.NewRegistry()
	registry.Register(&featureBroker{id: "snapshot-broker", feature: broker.FeatureMarketSnapshots})
	service := NewService(registry, "snapshot-broker", nil, nil)

	if _, err := service.BatchSnapshots(t.Context(), broker.FeatureQuery{}, []string{"SG.D05"}); !errors.Is(err, ErrInvalidQuery) {
		t.Fatalf("SG BatchSnapshots() error = %v, want invalid query", err)
	}
	symbols := make([]string, 201)
	for index := range symbols {
		symbols[index] = "US.AAPL"
	}
	if _, err := service.BatchSnapshots(t.Context(), broker.FeatureQuery{}, symbols); !errors.Is(err, ErrInvalidQuery) {
		t.Fatalf("oversized BatchSnapshots() error = %v, want invalid query", err)
	}
}

func TestPredictionSubscriptionLeasesReferenceCountVisibleContracts(t *testing.T) {
	firm := "FUTUINC"
	adapter := &featureBroker{
		id: "prediction-broker", feature: broker.FeaturePredictionDepth,
		accounts: []broker.Account{{
			ID: "eligible", SecurityFirm: &firm, MarketAuthorities: []string{"US"},
		}},
	}
	registry := broker.NewRegistry()
	registry.Register(adapter)
	service := NewService(registry, adapter.id, nil, nil)

	first, err := service.AcquirePredictionSubscription(
		t.Context(), adapter.id, "eligible", "event.one", []string{"order_book"},
	)
	if err != nil {
		t.Fatalf("first AcquirePredictionSubscription() error = %v", err)
	}
	second, err := service.AcquirePredictionSubscription(
		t.Context(), adapter.id, "eligible", "US.EVENT.ONE", []string{"ORDER_BOOK"},
	)
	if err != nil {
		t.Fatalf("second AcquirePredictionSubscription() error = %v", err)
	}
	if first.LeaseID == second.LeaseID || adapter.subscribeCalls != 1 {
		t.Fatalf("leases = %#v / %#v, subscribe calls = %d", first, second, adapter.subscribeCalls)
	}
	if err := service.ReleasePredictionSubscription(t.Context(), first.LeaseID); err != nil {
		t.Fatalf("release first lease: %v", err)
	}
	if adapter.unsubscribeCalls != 0 {
		t.Fatalf("unsubscribe calls after first release = %d", adapter.unsubscribeCalls)
	}
	if err := service.ReleasePredictionSubscription(t.Context(), second.LeaseID); err != nil {
		t.Fatalf("release second lease: %v", err)
	}
	if adapter.unsubscribeCalls != 1 {
		t.Fatalf("unsubscribe calls = %d, want 1", adapter.unsubscribeCalls)
	}
	if err := service.ReleasePredictionSubscription(t.Context(), second.LeaseID); err != nil {
		t.Fatalf("idempotent release: %v", err)
	}
}

func TestProductFeatureServiceRoutesEveryOptionalInterfaceAndCaches(t *testing.T) {
	firm := "FUTUINC"
	adapter := &featureBroker{
		id: "full",
		accounts: []broker.Account{{
			ID: "eligible", SecurityFirm: &firm, MarketAuthorities: []string{"US"},
		}},
	}
	for _, definition := range broker.BuiltinCapabilityCatalog.Features {
		adapter.features = append(adapter.features, definition.ID)
	}
	registry := broker.NewRegistry()
	registry.Register(adapter)
	ensured := 0
	service := NewService(registry, adapter.id, nil, func() { ensured++ })

	capabilities := service.Capabilities()
	if service.Catalog().Version == "" || len(capabilities["brokers"].([]broker.Descriptor)) != 1 {
		t.Fatalf("capabilities = %#v", capabilities)
	}
	for _, definition := range broker.BuiltinCapabilityCatalog.Features {
		if definition.Access != broker.FeatureAccessRead ||
			definition.AdapterInterface == "MarketDataReader" ||
			definition.AdapterInterface == "BatchSnapshotSource" ||
			definition.AdapterInterface == "ProductRuleProvider" {
			continue
		}
		query := broker.FeatureQuery{
			BrokerID: adapter.id, AccountID: "eligible", Market: "US",
			InstrumentID: "US.AAPL", FeatureID: definition.ID, PageSize: 5,
			Params: map[string]any{"refresh": true},
		}
		result, err := service.Query(t.Context(), query)
		if err != nil {
			t.Errorf("Query(%s/%s): %v", definition.ID, definition.AdapterInterface, err)
			continue
		}
		if result.Provider.BrokerID != adapter.id || result.Entries == nil || result.AsOf.IsZero() {
			t.Errorf("Query(%s) result = %#v", definition.ID, result)
		}
	}
	if ensured == 0 || adapter.queryCalls == 0 {
		t.Fatalf("ensure=%d queryCalls=%d", ensured, adapter.queryCalls)
	}

	query := broker.FeatureQuery{
		BrokerID: adapter.id, Market: "US", InstrumentID: "US.AAPL",
		FeatureID: broker.FeatureResearchFinancials,
	}
	first, err := service.Query(t.Context(), query)
	if err != nil {
		t.Fatal(err)
	}
	second, err := service.Query(t.Context(), query)
	if err != nil || second.Metadata["fromCache"] != true || first == second {
		t.Fatalf("cached result first=%#v second=%#v err=%v", first, second, err)
	}

	written, err := service.ApplyCustomization(t.Context(), broker.CustomizationAction{
		BrokerID: adapter.id, FeatureID: broker.FeaturePriceAlertSet,
		Action: "set", Payload: map[string]any{"price": 100},
	})
	if err != nil || len(written.Entries) != 1 || written.Provider.FeatureID != broker.FeaturePriceAlertSet {
		t.Fatalf("ApplyCustomization() = %#v, %v", written, err)
	}
}

func TestProductFeatureServiceFailureBoundaries(t *testing.T) {
	if _, err := (*Service)(nil).Query(t.Context(), broker.FeatureQuery{}); err == nil {
		t.Fatal("nil service query succeeded")
	}
	registry := broker.NewRegistry()
	bare := &bareFeatureBroker{id: "bare", feature: broker.FeatureMarketIntraday}
	registry.Register(bare)
	service := NewService(registry, bare.id, nil, nil)

	if _, err := service.Query(t.Context(), broker.FeatureQuery{FeatureID: "missing"}); err == nil {
		t.Fatal("unknown feature succeeded")
	}
	if _, err := service.Query(t.Context(), broker.FeatureQuery{
		BrokerID: bare.id, Market: "US", FeatureID: broker.FeatureMarketIntraday,
	}); err == nil {
		t.Fatal("missing optional interface succeeded")
	}
	if _, err := service.ApplyCustomization(t.Context(), broker.CustomizationAction{
		FeatureID: broker.FeatureMarketSnapshot,
	}); err == nil {
		t.Fatal("read feature accepted as customization")
	}
	if _, err := normalizePredictionDataTypes([]string{"bad"}); err == nil {
		t.Fatal("unsupported prediction data type succeeded")
	}
	if _, err := normalizePredictionDataTypes(nil); err == nil {
		t.Fatal("empty prediction data types succeeded")
	}
	if got := firstNonEmpty("", " value "); got != " value " {
		t.Fatalf("firstNonEmpty() = %q", got)
	}
}

func TestProductFeatureServiceExhaustiveFailureAndNormalizationBranches(t *testing.T) {
	ctx := t.Context()
	if _, err := (*Service)(nil).BatchSnapshots(ctx, broker.FeatureQuery{}, []string{"US.AAPL"}); err == nil {
		t.Fatal("nil batch snapshot service succeeded")
	}
	if _, err := (*Service)(nil).AcquirePredictionSubscription(ctx, "", "", "EVENT", []string{"KLINE"}); err == nil {
		t.Fatal("nil prediction subscription service succeeded")
	}
	if _, err := (*Service)(nil).ApplyCustomization(ctx, broker.CustomizationAction{}); err == nil {
		t.Fatal("nil customization service succeeded")
	}

	full := &featureBroker{id: "branches", features: []broker.FeatureID{
		broker.FeatureMarketSnapshot,
		broker.FeatureMarketSnapshots,
	}}
	registry := broker.NewRegistry()
	registry.Register(full)
	svc := NewService(registry, full.id, nil, nil)
	if _, err := svc.Query(ctx, broker.FeatureQuery{FeatureID: broker.FeaturePriceAlertSet}); err == nil {
		t.Fatal("write feature accepted by Query")
	}
	if _, err := svc.BatchSnapshots(ctx, broker.FeatureQuery{}, nil); err == nil {
		t.Fatal("empty snapshot batch succeeded")
	}
	if _, err := normalizeSnapshotSymbols([]string{"invalid"}); err == nil {
		t.Fatal("unqualified snapshot symbol succeeded")
	}
	if _, err := normalizeSnapshotSymbols([]string{" ", " "}); err == nil {
		t.Fatal("blank snapshot symbols succeeded")
	}
	if got := marketFromSymbol("US.AAPL"); got != "US" {
		t.Fatalf("marketFromSymbol() = %q", got)
	}
	for _, symbol := range []string{"HK.00700", "SH.600519", "SZ.000001"} {
		if marketFromSymbol(symbol) == "" {
			t.Errorf("marketFromSymbol(%q) was empty", symbol)
		}
	}

	full.snapshotErr = errors.New("snapshot failed")
	if _, err := svc.BatchSnapshots(ctx, broker.FeatureQuery{}, []string{"US.AAPL"}); err == nil {
		t.Fatal("snapshot upstream error was hidden")
	}
	full.snapshotErr = nil
	full.snapshotNil = true
	result, err := svc.BatchSnapshots(ctx, broker.FeatureQuery{
		FeatureID: broker.FeatureMarketSnapshot,
		Params:    map[string]any{"refresh": true},
	}, []string{"US.AAPL"})
	if err != nil || len(result.Entries) != 0 {
		t.Fatalf("nil snapshot result = %#v, %v", result, err)
	}

	bareRegistry := broker.NewRegistry()
	bare := &bareFeatureBroker{id: "missing-snapshot", feature: broker.FeatureMarketSnapshots}
	bareRegistry.Register(bare)
	bareService := NewService(bareRegistry, bare.id, nil, nil)
	if _, err := bareService.BatchSnapshots(ctx, broker.FeatureQuery{}, []string{"US.AAPL"}); err == nil {
		t.Fatal("broker without BatchSnapshotSource succeeded")
	}
	full.snapshotNil = false
	full.markets = []string{"HK"}
	if _, err := svc.BatchSnapshots(ctx, broker.FeatureQuery{
		Params: map[string]any{"refresh": true},
	}, []string{"US.AAPL"}); err == nil {
		t.Fatal("unsupported snapshot market succeeded")
	}
	full.markets = nil

	if _, err := jsonObject(make(chan int)); err == nil {
		t.Fatal("jsonObject marshaled a channel")
	}
	if _, err := jsonObject("scalar"); err == nil {
		t.Fatal("jsonObject decoded a scalar as object")
	}
}

func TestProductFeaturePredictionAndCustomizationFailureBranches(t *testing.T) {
	ctx := t.Context()
	firm := "FUTUINC"
	full := &featureBroker{
		id: "prediction-branches",
		features: []broker.FeatureID{
			broker.FeaturePredictionDepth,
			broker.FeaturePredictionHistory,
			broker.FeaturePriceAlertSet,
		},
		accounts: []broker.Account{{
			ID: "eligible", SecurityFirm: &firm, MarketAuthorities: []string{"US"},
		}},
	}
	registry := broker.NewRegistry()
	registry.Register(full)
	svc := NewService(registry, full.id, nil, nil)

	for _, instrumentID := range []string{"", "US."} {
		if _, err := svc.AcquirePredictionSubscription(
			ctx, full.id, "eligible", instrumentID, []string{"KLINE"},
		); err == nil {
			t.Errorf("invalid prediction instrument %q succeeded", instrumentID)
		}
	}
	full.subscribeErr = errors.New("subscription failed")
	if _, err := svc.AcquirePredictionSubscription(
		ctx, full.id, "eligible", "EVENT", []string{"ticker", "kline", "ticker"},
	); err == nil {
		t.Fatal("subscription upstream error was hidden")
	}
	full.subscribeErr = nil
	lease, err := svc.AcquirePredictionSubscription(
		ctx, full.id, "eligible", "EVENT", []string{"ticker", "kline", "ticker"},
	)
	if err != nil || len(lease.DataTypes) != 2 || lease.Provider.FeatureID != broker.FeaturePredictionHistory {
		t.Fatalf("history subscription = %#v, %v", lease, err)
	}
	full.unsubscribeErr = errors.New("unsubscribe failed")
	if err := svc.ReleasePredictionSubscription(ctx, lease.LeaseID); err == nil {
		t.Fatal("unsubscribe upstream error was hidden")
	}
	if err := svc.ReleasePredictionSubscription(ctx, " "); err == nil {
		t.Fatal("blank lease release succeeded")
	}

	missingReader := &bareFeatureBroker{id: "missing-prediction", feature: broker.FeaturePredictionDepth}
	missingReader.accounts = full.accounts
	missingRegistry := broker.NewRegistry()
	missingRegistry.Register(missingReader)
	missingService := NewService(missingRegistry, missingReader.id, nil, nil)
	if _, err := missingService.AcquirePredictionSubscription(
		ctx, missingReader.id, "eligible", "EVENT", []string{"ORDER_BOOK"},
	); err == nil {
		t.Fatal("missing PredictionMarketReader succeeded")
	}

	full.customizationErr = errors.New("customization failed")
	if _, err := svc.ApplyCustomization(ctx, broker.CustomizationAction{
		BrokerID: full.id, FeatureID: broker.FeaturePriceAlertSet,
	}); err == nil {
		t.Fatal("customization upstream error was hidden")
	}
	full.customizationErr = nil
	full.customizationNil = true
	written, err := svc.ApplyCustomization(ctx, broker.CustomizationAction{
		BrokerID: full.id, FeatureID: broker.FeaturePriceAlertSet,
	})
	if err != nil || written == nil || written.Provider.BrokerID != full.id {
		t.Fatalf("nil customization response = %#v, %v", written, err)
	}
	if _, err := svc.ApplyCustomization(ctx, broker.CustomizationAction{
		BrokerID: "missing", FeatureID: broker.FeaturePriceAlertSet,
	}); err == nil {
		t.Fatal("missing customization broker succeeded")
	}
	if _, err := missingService.ApplyCustomization(ctx, broker.CustomizationAction{
		BrokerID: missingReader.id, FeatureID: broker.FeaturePriceAlertSet,
	}); err == nil {
		t.Fatal("missing CustomizationService succeeded")
	}
}

func TestProductFeatureServiceNormalizesCoreMarketCandlesWithProvider(t *testing.T) {
	reader := &featureMarketDataReader{
		snapshot: &broker.KLineSnapshot{
			Symbol: "US.AAPL", Period: "5m",
			KLines: []broker.KLineItem{{Time: "2026-07-18T09:35:00-04:00"}},
		},
	}
	adapter := &featureBroker{
		id: "core-candles", feature: broker.FeatureMarketCandles, marketData: reader,
	}
	registry := broker.NewRegistry()
	registry.Register(adapter)
	service := NewService(registry, adapter.id, nil, nil)

	result, err := service.Query(t.Context(), broker.FeatureQuery{
		BrokerID: adapter.id, AccountID: "account-1", TradingEnvironment: "SIMULATE",
		Market: "US", InstrumentID: "US.AAPL", FeatureID: broker.FeatureMarketCandles,
		PageSize: 20, Params: map[string]any{
			"operation": "historical", "period": "5m", "limit": 20,
			"startTime": "2026-07-18", "endTime": "2026-07-19", "refresh": true,
		},
	})
	if err != nil {
		t.Fatalf("Query market.candles: %v", err)
	}
	if result.Provider.BrokerID != adapter.id ||
		result.Provider.FeatureID != broker.FeatureMarketCandles ||
		result.ResolvedInstrument == nil ||
		result.ResolvedInstrument.InstrumentID != "US.AAPL" ||
		len(result.Entries) != 1 ||
		result.Metadata["period"] != "5m" {
		t.Fatalf("normalized candle result = %#v", result)
	}
	if reader.query.Symbol != "US.AAPL" || reader.query.Period != "5m" ||
		reader.query.Limit != 20 || reader.query.BrokerID != adapter.id {
		t.Fatalf("broker candle query = %#v", reader.query)
	}
}

func TestProductFeatureDirectAdapterCacheAndEligibilityBranches(t *testing.T) {
	ctx := t.Context()
	bare := &bareFeatureBroker{id: "bare"}
	interfaces := []string{
		"MarketDataReader", "BatchSnapshotSource", "MarketMicrostructureReader",
		"InstrumentProfileReader", "DerivativeCatalogReader", "OptionAnalyticsReader",
		"InstrumentResearchReader", "MarketResearchReader", "PredictionMarketReader",
		"TechnicalIndicatorReader", "CustomizationService", "ProductRuleProvider",
	}
	for _, adapterInterface := range interfaces {
		if _, err := queryResolvedFeature(ctx, bare, adapterInterface, broker.FeatureQuery{
			FeatureID: broker.FeatureMarketIntraday,
		}); err == nil {
			t.Errorf("queryResolvedFeature(%s) succeeded", adapterInterface)
		}
	}

	firm := "FUTUINC"
	accountCases := []*featureBroker{
		{id: "discover-error", accountErr: errors.New("accounts unavailable")},
		{id: "nil-firm", accounts: []broker.Account{{ID: "1", SecurityFirm: nil}}},
		{id: "wrong-authority", accounts: []broker.Account{{
			ID: "1", SecurityFirm: &firm, MarketAuthorities: []string{"HK"},
		}}},
	}
	for _, adapter := range accountCases {
		if _, err := predictionEligibility(ctx, adapter, "1"); err == nil {
			t.Errorf("predictionEligibility(%s) succeeded", adapter.id)
		}
	}
	if containsFold([]string{"HK"}, "US") || !containsFold([]string{"us"}, "US") {
		t.Fatal("containsFold returned an unexpected value")
	}

	svc := NewService(nil, "", nil, nil)
	if brokers := svc.Capabilities()["brokers"].([]broker.Descriptor); len(brokers) != 0 {
		t.Fatalf("nil registry capabilities = %#v", brokers)
	}
	svc.now = func() time.Time { return time.Unix(20, 0) }
	svc.cache["expired"] = cacheEntry{
		expiresAt: time.Unix(10, 0),
		result:    &broker.FeatureResult{},
	}
	if got := svc.cached("missing"); got != nil {
		t.Fatalf("missing cache = %#v", got)
	}
	if got := svc.cached("expired"); got != nil {
		t.Fatalf("expired cache = %#v", got)
	}
	svc.cache["metadata"] = cacheEntry{
		expiresAt: time.Unix(30, 0),
		result:    &broker.FeatureResult{},
	}
	if got := svc.cached("metadata"); got == nil || got.Metadata["fromCache"] != true {
		t.Fatalf("cached metadata result = %#v", got)
	}
	if cloneResult(nil) != nil || boolParam(nil, "missing") {
		t.Fatal("nil helper result was unexpected")
	}
	fixed := time.Unix(100, 0)
	svc.now = func() time.Time { return fixed }
	last := 12.5
	snapshotResult := svc.batchSnapshotResult(
		broker.FeatureQuery{FeatureID: broker.FeatureMarketSnapshots},
		[]string{"US.AAPL"},
		broker.FeatureResolution{
			Broker: fullFeatureResolutionBroker(),
			Capability: broker.FeatureCapability{
				State: broker.CapabilityAvailable,
			},
		},
		&broker.SecuritySnapshotResult{Snapshots: []broker.SecuritySnapshotItem{{
			Symbol: "US.AAPL", LastPrice: &last, ObservedAt: fixed.Add(time.Second),
		}}},
	)
	if !snapshotResult.AsOf.Equal(fixed.Add(time.Second)) {
		t.Fatalf("batch snapshot asOf = %s", snapshotResult.AsOf)
	}
	if got := firstNonEmpty("", " "); got != "" {
		t.Fatalf("firstNonEmpty blank = %q", got)
	}
}

func fullFeatureResolutionBroker() broker.Broker {
	return &featureBroker{id: "resolution", feature: broker.FeatureMarketSnapshots}
}

type featureBroker struct {
	id               string
	feature          broker.FeatureID
	features         []broker.FeatureID
	accounts         []broker.Account
	subscribeCalls   int
	unsubscribeCalls int
	queryCalls       int
	accountErr       error
	snapshotErr      error
	snapshotNil      bool
	subscribeErr     error
	unsubscribeErr   error
	customizationErr error
	customizationNil bool
	queryNil         bool
	markets          []string
	marketData       broker.MarketDataReader
}

func (b *featureBroker) ID() string { return b.id }
func (b *featureBroker) Descriptor() broker.Descriptor {
	features := []broker.FeatureCapability{}
	featureIDs := append([]broker.FeatureID(nil), b.features...)
	if b.feature != "" {
		featureIDs = append(featureIDs, b.feature)
	}
	for _, featureID := range featureIDs {
		features = append(features, broker.FeatureCapability{
			ID:      featureID,
			Markets: []string{"US"},
			Access:  broker.FeatureAccessRead,
			State:   broker.CapabilityAvailable,
		})
	}
	markets := b.markets
	if len(markets) == 0 {
		markets = []string{"US"}
	}
	capabilities := make([]broker.MarketCapability, 0, len(markets))
	for _, market := range markets {
		marketFeatures := append([]broker.FeatureCapability(nil), features...)
		for index := range marketFeatures {
			marketFeatures[index].Markets = []string{market}
		}
		capabilities = append(capabilities, broker.MarketCapability{Market: market, Features: marketFeatures})
	}
	return broker.Descriptor{
		ID:                b.id,
		CapabilityVersion: broker.BuiltinCapabilityCatalog.Version,
		Capabilities:      capabilities,
	}
}
func (b *featureBroker) DiscoverAccounts(context.Context) ([]broker.Account, error) {
	return b.accounts, b.accountErr
}
func (b *featureBroker) Trading() broker.TradingService      { return nil }
func (b *featureBroker) MarketData() broker.MarketDataReader { return b.marketData }
func (b *featureBroker) QuerySecuritySnapshot(
	_ context.Context,
	query broker.SecuritySnapshotQuery,
) (*broker.SecuritySnapshotResult, error) {
	if b.snapshotErr != nil {
		return nil, b.snapshotErr
	}
	if b.snapshotNil {
		return nil, nil
	}
	items := make([]broker.SecuritySnapshotItem, 0, len(query.Symbols))
	for _, symbol := range query.Symbols {
		price := 215.5
		items = append(items, broker.SecuritySnapshotItem{
			Symbol: symbol, LastPrice: &price,
			ObservedAt: time.Date(2026, 7, 17, 9, 29, 59, 0, time.UTC),
		})
	}
	return &broker.SecuritySnapshotResult{AccountID: query.AccountID, Snapshots: items}, nil
}
func (b *featureBroker) QueryPredictionMarket(
	_ context.Context,
	query broker.FeatureQuery,
) (*broker.FeatureResult, error) {
	return b.featureResult(query)
}
func (b *featureBroker) SubscribePredictionMarket(context.Context, broker.PredictionSubscription) error {
	b.subscribeCalls++
	return b.subscribeErr
}
func (b *featureBroker) UnsubscribePredictionMarket(context.Context, broker.PredictionSubscription) error {
	b.unsubscribeCalls++
	return b.unsubscribeErr
}

func (b *featureBroker) featureResult(query broker.FeatureQuery) (*broker.FeatureResult, error) {
	b.queryCalls++
	if b.queryNil {
		return nil, nil
	}
	return &broker.FeatureResult{Metadata: map[string]any{"feature": query.FeatureID}}, nil
}

func (b *featureBroker) QueryMarketMicrostructure(ctx context.Context, query broker.FeatureQuery) (*broker.FeatureResult, error) {
	return b.featureResult(query)
}
func (b *featureBroker) QueryInstrumentProfile(ctx context.Context, query broker.FeatureQuery) (*broker.FeatureResult, error) {
	return b.featureResult(query)
}
func (b *featureBroker) QueryDerivativeCatalog(ctx context.Context, query broker.FeatureQuery) (*broker.FeatureResult, error) {
	return b.featureResult(query)
}
func (b *featureBroker) QueryOptionAnalytics(ctx context.Context, query broker.FeatureQuery) (*broker.FeatureResult, error) {
	return b.featureResult(query)
}
func (b *featureBroker) QueryInstrumentResearch(ctx context.Context, query broker.FeatureQuery) (*broker.FeatureResult, error) {
	return b.featureResult(query)
}
func (b *featureBroker) QueryMarketResearch(ctx context.Context, query broker.FeatureQuery) (*broker.FeatureResult, error) {
	return b.featureResult(query)
}
func (b *featureBroker) QueryTechnicalIndicator(ctx context.Context, query broker.FeatureQuery) (*broker.FeatureResult, error) {
	return b.featureResult(query)
}
func (b *featureBroker) QueryCustomization(ctx context.Context, query broker.FeatureQuery) (*broker.FeatureResult, error) {
	return b.featureResult(query)
}
func (b *featureBroker) ApplyCustomization(
	context.Context,
	broker.CustomizationAction,
) (*broker.CustomizationResult, error) {
	if b.customizationErr != nil {
		return nil, b.customizationErr
	}
	if b.customizationNil {
		return nil, nil
	}
	return &broker.CustomizationResult{Entries: []map[string]any{{"accepted": true}}}, nil
}

type bareFeatureBroker struct {
	id       string
	feature  broker.FeatureID
	accounts []broker.Account
}

func (b *bareFeatureBroker) ID() string { return b.id }
func (b *bareFeatureBroker) Descriptor() broker.Descriptor {
	return broker.Descriptor{ID: b.id, Capabilities: []broker.MarketCapability{{
		Market: "US", Features: []broker.FeatureCapability{{
			ID: b.feature, Markets: []string{"US"}, Access: broker.FeatureAccessRead,
			State: broker.CapabilityAvailable,
		}},
	}}}
}
func (b *bareFeatureBroker) DiscoverAccounts(context.Context) ([]broker.Account, error) {
	return b.accounts, nil
}
func (b *bareFeatureBroker) Trading() broker.TradingService      { return nil }
func (b *bareFeatureBroker) MarketData() broker.MarketDataReader { return nil }

type featureMarketDataReader struct {
	broker.MarketDataReader
	query    broker.KLineQuery
	snapshot *broker.KLineSnapshot
	err      error
}

func (r *featureMarketDataReader) QueryKLines(
	_ context.Context,
	query broker.KLineQuery,
) (*broker.KLineSnapshot, error) {
	r.query = query
	return r.snapshot, r.err
}
