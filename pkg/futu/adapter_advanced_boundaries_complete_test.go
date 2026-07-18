package futu

import (
	"strings"
	"testing"
	"time"

	"github.com/jftrade/jftrade-main/pkg/broker"
	"github.com/jftrade/jftrade-main/pkg/futu/opend"
	qotcommonpb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotcommon"
	eventcontractsnapshotpb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotgeteventcontractsnapshot"
	qotgetsearchquotepb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotgetsearchquote"
)

func TestFutuPredictionMilestonesResolveOwningEventBeforeQuery(t *testing.T) {
	server := startQuoteOpenDServer(t)
	defer server.stop()
	adapter := newTestBrokerAdapter(t, server).(*futuAdapter)
	ctx := t.Context()

	if _, err := adapter.resolvePredictionEventID(ctx, " "); err == nil {
		t.Fatal("blank prediction contract resolved an event")
	}
	if _, err := adapter.resolvePredictionEventID(ctx, "US.EC.EMPTY"); err == nil ||
		!strings.Contains(err.Error(), "no snapshot") {
		t.Fatalf("empty snapshot error = %v", err)
	}
	if _, err := adapter.QueryPredictionMarket(ctx, broker.FeatureQuery{
		Market: "US", InstrumentID: "US.EC.EMPTY",
		FeatureID: broker.FeaturePredictionDiscover,
		Params:    map[string]any{"operation": "milestones"},
	}); err == nil {
		t.Fatal("milestones query hid its event-resolution failure")
	}
	server.setAdvancedResponse(3445,
		&eventcontractsnapshotpb.Response{
			RetType: new(int32(0)),
			S2C: &eventcontractsnapshotpb.S2C{SnapshotList: []*eventcontractsnapshotpb.SnapshotItem{{
				Code: &qotcommonpb.Security{Market: new(int32(101)), Code: new("EC.NO.EVENT")},
			}}},
		})
	if _, err := adapter.resolvePredictionEventID(ctx, "US.EC.NO.EVENT"); err == nil ||
		!strings.Contains(err.Error(), "no owning event") {
		t.Fatalf("missing event error = %v", err)
	}
	server.setAdvancedResponse(3445,
		&eventcontractsnapshotpb.Response{
			RetType: new(int32(0)),
			S2C: &eventcontractsnapshotpb.S2C{SnapshotList: []*eventcontractsnapshotpb.SnapshotItem{{
				Code:      &qotcommonpb.Security{Market: new(int32(101)), Code: new("EC.ONE")},
				EventCode: &qotcommonpb.Security{Market: new(int32(101)), Code: new("EVENT.ONE")},
			}}},
		})
	eventID, err := adapter.resolvePredictionEventID(ctx, "US.EC.ONE")
	if err != nil || eventID != "US.EVENT.ONE" {
		t.Fatalf("resolved event = %q, %v", eventID, err)
	}
	result, err := adapter.QueryPredictionMarket(ctx, broker.FeatureQuery{
		Market: "US", InstrumentID: "US.EC.ONE",
		MarketSegment: broker.MarketSegmentPrediction,
		ProductClass:  broker.ProductClassEventContract,
		FeatureID:     broker.FeaturePredictionDiscover,
		Params:        map[string]any{"operation": "milestones"},
	})
	if err != nil || result == nil {
		t.Fatalf("milestones query = %#v, %v", result, err)
	}

	dead := NewBrokerAdapter(NewExchangeWithConfig(opend.Config{
		Addr: "127.0.0.1:1", RequestTimeout: 30 * time.Millisecond,
	})).(*futuAdapter)
	if _, err := dead.resolvePredictionEventID(ctx, "US.EC.FAIL"); err == nil ||
		!strings.Contains(err.Error(), "resolve prediction event") {
		t.Fatalf("milestone transport error = %v", err)
	}
}

func TestFutuAdvancedPredictionSnapshotSearchFallbackAndQueryValidation(t *testing.T) {
	server := startQuoteOpenDServer(t)
	defer server.stop()
	server.searchQuotes = []*qotgetsearchquotepb.SearchQuote{{
		Market: new(int32(qotcommonpb.QotMarket_QotMarket_US_Security)),
		Code:   new("AAPL"),
		Name:   new("Apple"),
	}}
	server.setAdvancedResponse(3445,
		&eventcontractsnapshotpb.Response{
			RetType: new(int32(0)),
			S2C: &eventcontractsnapshotpb.S2C{SnapshotList: []*eventcontractsnapshotpb.SnapshotItem{{
				Code: &qotcommonpb.Security{Market: new(int32(101)), Code: new("EC.ONE")},
			}}},
		})
	adapter := newTestBrokerAdapter(t, server).(*futuAdapter)
	ctx := t.Context()

	search, err := adapter.queryInstrumentSearchFeature(ctx, broker.FeatureQuery{
		Market: "US", PageSize: 500, Params: map[string]any{"query": "Apple"},
	})
	if err != nil || len(search.Entries) != 1 {
		t.Fatalf("query-name search fallback = %#v, %v", search, err)
	}
	snapshot, err := adapter.queryInstrumentSnapshotFeature(ctx, broker.FeatureQuery{
		Market: "US", InstrumentID: "US.EC.ONE",
		MarketSegment: broker.MarketSegmentPrediction,
		FeatureID:     broker.FeatureInstrumentProfile,
		Params:        map[string]any{},
	})
	if err != nil || snapshot.ResolvedInstrument == nil ||
		snapshot.ResolvedInstrument.ProductClass != broker.ProductClassEventContract {
		t.Fatalf("prediction profile snapshot = %#v, %v", snapshot, err)
	}
	if _, err := adapter.queryAdvancedFeatureWithProtocols(ctx, broker.FeatureQuery{
		Market: "US", FeatureID: broker.FeaturePredictionComboQuote,
		Params: map[string]any{"operation": "quote", "legs": []any{}},
	}, featureProtocols[broker.FeaturePredictionComboQuote]); err == nil {
		t.Fatal("malformed combo RFQ query succeeded")
	}

	invalidReplay := newTestBrokerAdapter(t, server).(*futuAdapter)
	invalidReplay.predictionSubscriptions["invalid"] = broker.PredictionSubscription{}
	valid := broker.PredictionSubscription{
		InstrumentID: "US.EC.ONE", DataTypes: []string{"TICKER"},
	}
	if err := invalidReplay.updatePredictionSubscription(ctx, valid, true); err == nil {
		t.Fatal("invalid existing subscription replay was hidden")
	}
	invalidReplay.predictionStreamClients = make(map[*opend.Client]struct{})
	if _, err := invalidReplay.QueryPredictionMarket(ctx, broker.FeatureQuery{
		Market: "US", FeatureID: broker.FeaturePredictionDiscover,
		Params: map[string]any{"operation": "categories"},
	}); err == nil {
		t.Fatal("query-time push-handler replay failure was hidden")
	}

	failingServer := startQuoteOpenDServer(t)
	defer failingServer.stop()
	failingServer.setDropProto(opend.ProtoQotSubEventContract)
	failing := newTestBrokerAdapter(t, failingServer).(*futuAdapter)
	if err := failing.updatePredictionSubscription(ctx, valid, true); err == nil {
		t.Fatal("subscription transport error was hidden")
	}
}

func TestFutuPredictionComboQuoteTranslationRejectsEveryInvalidLegShape(t *testing.T) {
	valid := map[string]any{
		"instrumentId": "US.EC.ONE", "predictionSide": "YES",
		"side": "BUY", "ratio": 1.0,
	}
	cases := []map[string]any{
		{},
		{"legs": []any{valid}},
		{"legs": []any{"invalid", valid}},
		{"legs": []any{
			map[string]any{"instrumentId": "HK.EC.ONE", "predictionSide": "YES"},
			valid,
		}},
		{"legs": []any{
			map[string]any{"instrumentId": "US.", "predictionSide": "YES"},
			valid,
		}},
		{"legs": []any{
			map[string]any{"instrumentId": "US.EC.ONE", "predictionSide": "MAYBE"},
			valid,
		}},
		{"legs": []any{
			map[string]any{
				"instrumentId": "US.EC.ONE", "predictionSide": "NO",
				"side": "SELL", "ratio": 0.0,
			},
			valid,
		}},
	}
	for index, params := range cases {
		if err := translatePredictionComboQuoteParams(params); err == nil {
			t.Errorf("invalid RFQ params %d succeeded", index)
		}
	}
	params := map[string]any{"legs": []any{
		map[string]any{
			"instrumentId": "US.EC.ONE", "predictionSide": "NO",
			"side": "SELL", "ratio": 2.0,
		},
		valid,
	}}
	if err := translatePredictionComboQuoteParams(params); err != nil {
		t.Fatalf("valid SELL/NO RFQ translation: %v", err)
	}
}
