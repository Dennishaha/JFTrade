package futu

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/jftrade/jftrade-main/pkg/broker"
	"github.com/jftrade/jftrade-main/pkg/futu/opend"
	updateklinepb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotupdateeventcontractkline"
	updateorderbookpb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotupdateeventcontractorderbook"
	updatetickerpb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotupdateeventcontractticker"
)

func (a *futuAdapter) OnPredictionMarketUpdate(
	handler func(broker.PredictionMarketUpdate),
) func() {
	if handler == nil {
		return func() {}
	}
	a.predictionStreamMu.Lock()
	a.predictionStreamNextID++
	id := a.predictionStreamNextID
	a.predictionStreamListeners[id] = handler
	a.predictionStreamMu.Unlock()
	return func() {
		a.predictionStreamMu.Lock()
		delete(a.predictionStreamListeners, id)
		a.predictionStreamMu.Unlock()
	}
}

func (a *futuAdapter) ensurePredictionPushHandlers(ctx context.Context, client *opend.Client) error {
	if client == nil {
		return nil
	}
	a.predictionStreamMu.Lock()
	if _, exists := a.predictionStreamClients[client]; exists {
		a.predictionStreamMu.Unlock()
		return nil
	}
	a.predictionStreamClients = map[*opend.Client]struct{}{client: {}}
	subscriptions := make([]broker.PredictionSubscription, 0, len(a.predictionSubscriptions))
	for _, subscription := range a.predictionSubscriptions {
		subscriptions = append(subscriptions, subscription)
	}
	a.predictionStreamMu.Unlock()

	client.SubscribeEventContractOrderBook(a.handlePredictionOrderBookPush)
	client.SubscribeEventContractKline(a.handlePredictionKlinePush)
	client.SubscribeEventContractTicker(a.handlePredictionTickerPush)
	sort.Slice(subscriptions, func(i, j int) bool {
		return predictionSubscriptionKey(subscriptions[i]) < predictionSubscriptionKey(subscriptions[j])
	})
	for _, subscription := range subscriptions {
		params, err := predictionSubscriptionParams(subscription, true)
		if err != nil {
			return err
		}
		if _, err := client.CallAdvanced(ctx, "Qot_SubEventContract", params); err != nil {
			return fmt.Errorf("futu: replay prediction subscription %s: %w", subscription.InstrumentID, err)
		}
	}
	return nil
}

func (a *futuAdapter) handlePredictionOrderBookPush(value *updateorderbookpb.S2C) {
	a.emitPredictionPush("ORDER_BOOK", "Qot_GetEventContractOrderBook", value)
}

func (a *futuAdapter) handlePredictionKlinePush(value *updateklinepb.S2C) {
	a.emitPredictionPush("KLINE", "Qot_GetEventContractKline", value)
}

func (a *futuAdapter) handlePredictionTickerPush(value *updatetickerpb.S2C) {
	a.emitPredictionPush("TICKER", "Qot_GetEventContractTicker", value)
}

func predictionSubscriptionKey(subscription broker.PredictionSubscription) string {
	dataTypes := append([]string(nil), subscription.DataTypes...)
	sort.Strings(dataTypes)
	return strings.ToUpper(strings.TrimSpace(subscription.InstrumentID)) + "|" + strings.Join(dataTypes, ",")
}

func (a *futuAdapter) emitPredictionPush(dataType, protocol string, value any) {
	payload, err := structMap(value)
	if err != nil {
		return
	}
	result := featureResultFromProtocolPayload(broker.FeatureQuery{
		Market: "US", MarketSegment: broker.MarketSegmentPrediction,
		ProductClass: broker.ProductClassEventContract,
	}, protocol, payload)
	for _, entry := range result.Entries {
		instrumentID := predictionEntryInstrumentID(entry)
		if instrumentID == "" {
			continue
		}
		update := broker.PredictionMarketUpdate{
			InstrumentID: instrumentID, DataType: dataType,
			Sequence: predictionEntrySequence(dataType, entry),
			AsOf:     time.Now().UTC(), Entries: []map[string]any{entry},
		}
		a.predictionStreamMu.Lock()
		listeners := make([]func(broker.PredictionMarketUpdate), 0, len(a.predictionStreamListeners))
		for _, listener := range a.predictionStreamListeners {
			listeners = append(listeners, listener)
		}
		a.predictionStreamMu.Unlock()
		for _, listener := range listeners {
			listener(update)
		}
	}
}

func predictionEntryInstrumentID(entry map[string]any) string {
	for _, key := range []string{"code", "contractSecurity"} {
		if value := securityInstrumentID(entry[key]); value != "" {
			return value
		}
	}
	return ""
}

func predictionEntrySequence(dataType string, entry map[string]any) string {
	var values []any
	switch dataType {
	case "TICKER":
		values, _ = entry["tickerList"].([]any)
	case "KLINE":
		values, _ = entry["klineList"].([]any)
	}
	if len(values) > 0 {
		if last, ok := values[len(values)-1].(map[string]any); ok {
			for _, key := range []string{"sequence", "timeKey", "time"} {
				if value := strings.TrimSpace(fmt.Sprint(last[key])); value != "" && value != "<nil>" {
					return value
				}
			}
		}
	}
	return ""
}

var _ broker.PredictionMarketStreamSource = (*futuAdapter)(nil)
