package productfeatures

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jftrade/jftrade-main/pkg/broker"
)

func TestQuotePredictionComboValidatesPersistsAndPublishesServerExpiry(t *testing.T) {
	firm := "FUTUINC"
	adapter := &quoteFeatureBroker{
		featureBroker: &featureBroker{
			id: "prediction-rfq", feature: broker.FeaturePredictionComboQuote,
			accounts: []broker.Account{{
				ID: "account-1", SecurityFirm: &firm, MarketAuthorities: []string{"US"},
			}},
		},
		result: &broker.FeatureResult{Metadata: map[string]any{
			"quoteId": "quote-1", "bidPrice": 0.42, "askPrice": 0.45, "shouldRetry": true,
		}},
	}
	registry := broker.NewRegistry()
	registry.Register(adapter)
	service := NewService(registry, adapter.id, nil, nil)
	store := &recordingPredictionQuoteStore{}
	service.SetPredictionQuoteStore(store)
	receivedAt := time.Date(2026, 7, 18, 12, 0, 0, 0, time.UTC)
	service.now = func() time.Time { return receivedAt }

	request := validPredictionComboQuoteRequest(adapter.id)
	result, err := service.QuotePredictionCombo(t.Context(), request)
	if err != nil {
		t.Fatalf("QuotePredictionCombo: %v", err)
	}
	if store.saved.QuoteID != "quote-1" ||
		store.saved.BrokerID != adapter.id ||
		store.saved.LegsHash == "" ||
		store.saved.BidPrice == nil || *store.saved.BidPrice != 0.42 ||
		store.saved.AskPrice == nil || *store.saved.AskPrice != 0.45 ||
		!store.saved.ShouldRetry ||
		!store.saved.ExpiresAt.Equal(receivedAt.Add(30*time.Second)) ||
		store.saved.ExpirySource != "jftrade_policy" {
		t.Fatalf("saved RFQ = %#v", store.saved)
	}
	if result.Metadata["quoteExpiresAt"] != receivedAt.Add(30*time.Second).Format(time.RFC3339Nano) ||
		result.Metadata["expirySource"] != "jftrade_policy" ||
		len(result.Warnings) != 1 {
		t.Fatalf("normalized RFQ result = %#v", result)
	}
	if adapter.query.TradingEnvironment != "SIMULATE" ||
		adapter.query.AccountID != "account-1" ||
		adapter.query.PageSize != 2 ||
		len(adapter.query.Params["legs"].([]any)) != 2 {
		t.Fatalf("broker RFQ query = %#v", adapter.query)
	}
}

func TestQuotePredictionComboRejectsInvalidAndUnpersistableQuotes(t *testing.T) {
	valid := validPredictionComboQuoteRequest("prediction-rfq")
	cases := []struct {
		name   string
		mutate func(*PredictionComboQuoteRequest)
	}{
		{"missing context", func(value *PredictionComboQuoteRequest) { value.AccountID = " " }},
		{"one leg", func(value *PredictionComboQuoteRequest) { value.Legs = value.Legs[:1] }},
		{"wrong product", func(value *PredictionComboQuoteRequest) {
			value.Legs[0].ProductClass = broker.ProductClassEquity
		}},
		{"wrong market", func(value *PredictionComboQuoteRequest) { value.Legs[0].InstrumentID = "HK.1" }},
		{"wrong side", func(value *PredictionComboQuoteRequest) { value.Legs[0].Side = "hold" }},
		{"wrong prediction side", func(value *PredictionComboQuoteRequest) {
			value.Legs[0].PredictionSide = "maybe"
		}},
		{"wrong ratio", func(value *PredictionComboQuoteRequest) { value.Legs[0].Ratio = 0 }},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			request := valid
			request.Legs = append([]broker.OrderLegIntent(nil), valid.Legs...)
			testCase.mutate(&request)
			if _, err := NewService(nil, "", nil, nil).QuotePredictionCombo(
				t.Context(),
				request,
			); !errors.Is(err, ErrInvalidQuery) {
				t.Fatalf("error = %v", err)
			}
		})
	}

	firm := "FUTUINC"
	adapter := &quoteFeatureBroker{
		featureBroker: &featureBroker{
			id: "prediction-rfq", feature: broker.FeaturePredictionComboQuote,
			accounts: []broker.Account{{
				ID: "account-1", SecurityFirm: &firm, MarketAuthorities: []string{"US"},
			}},
		},
		result: &broker.FeatureResult{Metadata: map[string]any{}},
	}
	registry := broker.NewRegistry()
	registry.Register(adapter)
	service := NewService(registry, adapter.id, nil, nil)
	if _, err := service.QuotePredictionCombo(
		t.Context(),
		validPredictionComboQuoteRequest(adapter.id),
	); err == nil || !strings.Contains(err.Error(), "quoteId") {
		t.Fatalf("missing quote ID error = %v", err)
	}
	adapter.result.Metadata["quoteId"] = "quote-1"
	if _, err := service.QuotePredictionCombo(
		t.Context(),
		validPredictionComboQuoteRequest(adapter.id),
	); err == nil || !strings.Contains(err.Error(), "persistence") {
		t.Fatalf("missing persistence error = %v", err)
	}
	service.SetPredictionQuoteStore(&recordingPredictionQuoteStore{err: errors.New("disk full")})
	if _, err := service.QuotePredictionCombo(
		t.Context(),
		validPredictionComboQuoteRequest(adapter.id),
	); err == nil || !strings.Contains(err.Error(), "disk full") {
		t.Fatalf("persistence failure = %v", err)
	}
	adapter.err = errors.New("rfq unavailable")
	if _, err := service.QuotePredictionCombo(
		t.Context(),
		validPredictionComboQuoteRequest(adapter.id),
	); !errors.Is(err, adapter.err) {
		t.Fatalf("upstream RFQ error = %v", err)
	}
}

func TestPredictionPushSourceCachesFreshUniqueUpdates(t *testing.T) {
	base := &featureBroker{id: "stream"}
	adapter := &streamFeatureBroker{featureBroker: base}
	service := NewService(nil, adapter.id, nil, nil)
	now := time.Date(2026, 7, 18, 12, 0, 0, 0, time.UTC)
	service.now = func() time.Time { return now }

	service.ensurePredictionPushSource(adapter.id, &bareFeatureBroker{id: "bare"})
	service.ensurePredictionPushSource(adapter.id, adapter)
	service.ensurePredictionPushSource(adapter.id, adapter)
	if adapter.registerCalls != 1 || adapter.listener == nil {
		t.Fatalf("stream registrations = %d", adapter.registerCalls)
	}
	adapter.listener(broker.PredictionMarketUpdate{
		InstrumentID: "US.EVENT", DataType: "ORDER_BOOK", Sequence: "1",
		AsOf: now, Entries: []map[string]any{{"price": 0.4}},
	})
	adapter.listener(broker.PredictionMarketUpdate{
		InstrumentID: "US.EVENT", DataType: "ORDER_BOOK", Sequence: "1",
		AsOf: now.Add(time.Second), Entries: []map[string]any{{"price": 0.9}},
	})
	result := service.predictionPushResult(broker.FeatureQuery{
		BrokerID: adapter.id, InstrumentID: "US.EVENT",
		FeatureID: broker.FeaturePredictionDepth, Params: map[string]any{},
	})
	if result == nil || result.Entries[0]["price"] != 0.4 || result.Metadata["sequence"] != "1" {
		t.Fatalf("deduplicated push result = %#v", result)
	}
	adapter.listener(broker.PredictionMarketUpdate{
		InstrumentID: "US.EVENT", DataType: "ORDER_BOOK",
		AsOf: now, Entries: []map[string]any{{"price": 0.5}},
	})
	if result = service.predictionPushResult(broker.FeatureQuery{
		BrokerID: adapter.id, InstrumentID: "US.EVENT",
		FeatureID: broker.FeaturePredictionDepth, Params: map[string]any{},
	}); result == nil || result.Entries[0]["price"] != 0.5 {
		t.Fatalf("sequence-free push result = %#v", result)
	}

	for operation, dataType := range map[string]string{"candles": "KLINE", "ticks": "TICKER"} {
		adapter.listener(broker.PredictionMarketUpdate{
			InstrumentID: "US.EVENT", DataType: dataType, Sequence: operation, AsOf: now,
		})
		if result := service.predictionPushResult(broker.FeatureQuery{
			BrokerID: adapter.id, InstrumentID: "US.EVENT",
			FeatureID: broker.FeaturePredictionHistory,
			Params:    map[string]any{"operation": operation},
		}); result == nil || result.Metadata["dataType"] != dataType {
			t.Fatalf("%s push result = %#v", operation, result)
		}
	}
	if result := service.predictionPushResult(broker.FeatureQuery{
		FeatureID: broker.FeaturePredictionHistory, Params: map[string]any{"operation": "other"},
	}); result != nil {
		t.Fatalf("unsupported push result = %#v", result)
	}
	service.now = func() time.Time { return now.Add(6 * time.Second) }
	if result := service.predictionPushResult(broker.FeatureQuery{
		BrokerID: adapter.id, InstrumentID: "US.EVENT",
		FeatureID: broker.FeaturePredictionDepth, Params: map[string]any{},
	}); result != nil {
		t.Fatalf("stale push result = %#v", result)
	}
}

func TestCoreCandleBridgeValidatesBoundariesAndProductSemantics(t *testing.T) {
	if _, _, err := normalizeCoreCandleQuery(nil, broker.FeatureQuery{}); !errors.Is(err, ErrInvalidQuery) {
		t.Fatalf("empty instrument error = %v", err)
	}
	if _, _, err := normalizeCoreCandleQuery(nil, broker.FeatureQuery{
		InstrumentID: "SG.D05",
	}); !errors.Is(err, ErrInvalidQuery) {
		t.Fatalf("unsupported market error = %v", err)
	}
	if _, _, err := normalizeCoreCandleQuery(nil, broker.FeatureQuery{
		InstrumentID: "US.AAPL", Params: map[string]any{"operation": "future"},
	}); !errors.Is(err, ErrInvalidQuery) {
		t.Fatalf("unsupported operation error = %v", err)
	}
	if _, _, err := normalizeCoreCandleQuery(nil, broker.FeatureQuery{
		BrokerID: "fallback", InstrumentID: "US.AAPL",
		Params: map[string]any{"limit": 501},
	}); !errors.Is(err, ErrInvalidQuery) {
		t.Fatalf("oversized limit error = %v", err)
	}
	query, operation, err := normalizeCoreCandleQuery(nil, broker.FeatureQuery{
		BrokerID: "fallback", InstrumentID: " us.aapl ",
		Params: map[string]any{"limit": -1, "fromTime": "from", "toTime": "to"},
	})
	if err != nil || query.BrokerID != "fallback" || query.Limit != 50 ||
		query.Period != "1m" || query.FromTime != "from" || query.ToTime != "to" ||
		operation != "historical" {
		t.Fatalf("default candle query = %#v, %q, %v", query, operation, err)
	}
	if int32Param(map[string]any{"limit": "bad"}, "limit", 12) != 12 ||
		int32Param(nil, "limit", 13) != 13 ||
		resolutionBrokerID(&bareFeatureBroker{id: " chosen "}, "fallback") != "chosen" ||
		resolutionBrokerID(&bareFeatureBroker{id: " "}, "fallback") != "fallback" {
		t.Fatal("candle helper normalization failed")
	}

	if _, err := queryCoreMarketDataFeature(
		t.Context(),
		&bareFeatureBroker{id: "bare"},
		broker.FeatureQuery{FeatureID: broker.FeatureMarketCandles},
	); err == nil {
		t.Fatal("nil market-data reader succeeded")
	}
	reader := &featureMarketDataReader{err: errors.New("candles failed")}
	adapter := &featureBroker{id: "candles", marketData: reader}
	if _, err := queryCoreMarketDataFeature(t.Context(), adapter, broker.FeatureQuery{
		FeatureID: broker.FeatureMarketIntraday,
	}); err == nil {
		t.Fatal("unsupported core feature succeeded")
	}
	if _, err := queryCoreMarketDataFeature(t.Context(), adapter, broker.FeatureQuery{
		FeatureID: broker.FeatureMarketCandles, InstrumentID: "US.AAPL",
	}); !errors.Is(err, reader.err) {
		t.Fatalf("market-data error = %v", err)
	}

	for _, productClass := range []broker.ProductClass{
		broker.ProductClassOption, broker.ProductClassFuture, broker.ProductClassEquity,
	} {
		result, resultErr := normalizedCoreCandleResult(
			broker.FeatureQuery{ProductClass: productClass},
			broker.KLineQuery{
				ReadQuery: broker.ReadQuery{Market: "US"},
				Symbol:    "US.TEST", Period: "1m",
			},
			"current",
			nil,
		)
		if resultErr != nil || result.ResolvedInstrument == nil {
			t.Fatalf("%s result = %#v, %v", productClass, result, resultErr)
		}
		want := broker.QuantityModeUnits
		if productClass == broker.ProductClassOption || productClass == broker.ProductClassFuture {
			want = broker.QuantityModeContracts
		}
		if result.ResolvedInstrument.QuantityMode != want {
			t.Fatalf("%s quantity mode = %s", productClass, result.ResolvedInstrument.QuantityMode)
		}
	}
}

func validPredictionComboQuoteRequest(brokerID string) PredictionComboQuoteRequest {
	return PredictionComboQuoteRequest{
		BrokerID: brokerID, AccountID: " account-1 ",
		TradingEnvironment: " simulate ", MVC: " mvc-1 ",
		Legs: []broker.OrderLegIntent{
			{
				InstrumentID: " us.event-one ", Side: " buy ", PredictionSide: " yes ",
				Ratio: 1,
			},
			{
				InstrumentID: "US.EVENT-TWO", Side: "SELL", PredictionSide: "NO",
				ProductClass: broker.ProductClassEventContract, Ratio: 2,
			},
		},
	}
}

type quoteFeatureBroker struct {
	*featureBroker
	query  broker.FeatureQuery
	result *broker.FeatureResult
	err    error
}

func (b *quoteFeatureBroker) QueryPredictionMarket(
	_ context.Context,
	query broker.FeatureQuery,
) (*broker.FeatureResult, error) {
	b.query = query
	return b.result, b.err
}

type recordingPredictionQuoteStore struct {
	saved broker.PredictionQuoteRecord
	err   error
}

func (s *recordingPredictionQuoteStore) SavePredictionQuote(
	_ context.Context,
	record broker.PredictionQuoteRecord,
) error {
	s.saved = record
	return s.err
}

func (*recordingPredictionQuoteStore) ValidatePredictionQuote(
	context.Context,
	string,
	string,
	string,
	string,
	string,
	string,
) (broker.PredictionQuoteRecord, error) {
	return broker.PredictionQuoteRecord{}, nil
}

func (*recordingPredictionQuoteStore) ConsumePredictionQuote(
	context.Context,
	string,
	string,
	string,
	string,
	string,
	string,
	string,
	string,
) error {
	return nil
}

type streamFeatureBroker struct {
	*featureBroker
	listener      func(broker.PredictionMarketUpdate)
	registerCalls int
}

func (b *streamFeatureBroker) OnPredictionMarketUpdate(
	listener func(broker.PredictionMarketUpdate),
) func() {
	b.registerCalls++
	b.listener = listener
	return func() { b.listener = nil }
}
