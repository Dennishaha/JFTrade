package jftradeapi

import (
	"context"
	"strconv"
	"time"

	"github.com/gorilla/websocket"

	"github.com/jftrade/jftrade-main/pkg/broker"
)

type liveWebSocketWriter struct {
	conn *websocket.Conn
}

func (w liveWebSocketWriter) WriteEvent(value any) error {
	return w.conn.WriteJSON(value)
}

type liveWebSocketDispatcher struct {
	server                  *Server
	requestCtx              context.Context
	writer                  liveEventWriter
	client                  *liveWebSocketClient
	clientClosed            <-chan struct{}
	depthUpdated            <-chan struct{}
	lastSentByInstrument    map[string]string
	lastSentNotificationSeq uint64
	lastSecurityResolvedAt  map[string]string
	lastDepthResolvedAt     map[string]string
	wroteLiveData           bool
	heartbeatInterval       time.Duration
	dataInterval            time.Duration
	consoleRefreshInterval  time.Duration
	securityDetailsInterval time.Duration
	depthRefreshInterval    time.Duration
}

func newLiveWebSocketDispatcher(
	server *Server,
	requestCtx context.Context,
	conn *websocket.Conn,
	client *liveWebSocketClient,
	clientClosed <-chan struct{},
	depthUpdated <-chan struct{},
) *liveWebSocketDispatcher {
	return &liveWebSocketDispatcher{
		server:                  server,
		requestCtx:              requestCtx,
		writer:                  liveWebSocketWriter{conn: conn},
		client:                  client,
		clientClosed:            clientClosed,
		depthUpdated:            depthUpdated,
		lastSentByInstrument:    map[string]string{},
		lastSecurityResolvedAt:  map[string]string{},
		lastDepthResolvedAt:     map[string]string{},
		heartbeatInterval:       15 * time.Second,
		dataInterval:            liveTickDispatchInterval,
		consoleRefreshInterval:  15 * time.Second,
		securityDetailsInterval: marketSecurityDetailsStreamInterval,
		depthRefreshInterval:    marketDepthStreamRefreshInterval,
	}
}

func (dispatcher *liveWebSocketDispatcher) writeInitialEvents() error {
	if err := dispatcher.writeHeartbeat(); err != nil {
		return err
	}
	if err := dispatcher.writeLiveData(); err != nil {
		return err
	}
	return dispatcher.writeAuxiliarySubscriptions(true)
}

func (dispatcher *liveWebSocketDispatcher) run() error {
	heartbeatTicker := time.NewTicker(dispatcher.heartbeatInterval)
	defer heartbeatTicker.Stop()
	dataTicker := time.NewTicker(dispatcher.dataInterval)
	defer dataTicker.Stop()
	consoleTicker := time.NewTicker(dispatcher.consoleRefreshInterval)
	defer consoleTicker.Stop()
	securityTicker := time.NewTicker(dispatcher.securityDetailsInterval)
	defer securityTicker.Stop()
	depthTicker := time.NewTicker(dispatcher.depthRefreshInterval)
	defer depthTicker.Stop()

	for {
		select {
		case <-dispatcher.requestCtx.Done():
			return nil
		case <-dispatcher.clientClosed:
			return nil
		case <-dispatcher.client.updated:
			if err := dispatcher.writeAuxiliarySubscriptions(true); err != nil {
				return err
			}
		case <-dispatcher.depthUpdated:
			if err := dispatcher.writeDepthEvents(true); err != nil {
				return err
			}
		case <-heartbeatTicker.C:
			if err := dispatcher.writeHeartbeat(); err != nil {
				return err
			}
		case <-dataTicker.C:
			if err := dispatcher.writeLiveData(); err != nil {
				return err
			}
		case <-consoleTicker.C:
			if err := dispatcher.writeConsoleRefresh(); err != nil {
				return err
			}
		case <-securityTicker.C:
			if err := dispatcher.writeSecurityDetailsEvents(false); err != nil {
				return err
			}
		case <-depthTicker.C:
			if err := dispatcher.writeDepthEvents(false); err != nil {
				return err
			}
		}
	}
}

func (dispatcher *liveWebSocketDispatcher) writeHeartbeat() error {
	return writeHeartbeat(
		dispatcher.writer,
		dispatcher.server.liveHeartbeatEvent(dispatcher.heartbeatInterval),
	)
}

func (dispatcher *liveWebSocketDispatcher) writeLiveData() error {
	subscriptions := dispatcher.client.snapshot()
	if err := dispatcher.server.writeLiveMarketTicks(
		dispatcher.requestCtx,
		dispatcher.writer,
		subscriptions.ActiveInstruments,
		dispatcher.lastSentByInstrument,
		!dispatcher.wroteLiveData,
	); err != nil {
		return err
	}
	dispatcher.wroteLiveData = true
	if err := dispatcher.server.writeLiveNotifications(dispatcher.writer, &dispatcher.lastSentNotificationSeq); err != nil {
		return err
	}
	return nil
}

func (dispatcher *liveWebSocketDispatcher) writeAuxiliarySubscriptions(force bool) error {
	if err := dispatcher.writeConsoleRefresh(); err != nil {
		return err
	}
	if err := dispatcher.writeSecurityDetailsEvents(force); err != nil {
		return err
	}
	if err := dispatcher.writeDepthEvents(force); err != nil {
		return err
	}
	return nil
}

func (dispatcher *liveWebSocketDispatcher) writeConsoleRefresh() error {
	if !dispatcher.client.snapshot().ConsoleRefresh {
		return nil
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	return dispatcher.writer.WriteEvent(map[string]any{
		"type":      "console.refresh",
		"at":        now,
		"checkedAt": now,
	})
}

func (dispatcher *liveWebSocketDispatcher) writeSecurityDetailsEvents(force bool) error {
	for _, subscription := range dispatcher.client.snapshot().SecurityDetails {
		response, err := dispatcher.server.marketSecurityDetailsResponse(
			dispatcher.requestCtx,
			"/api/v1/market-data/securities/"+subscription.Market+"/"+subscription.Symbol,
		)
		if err != nil {
			continue
		}
		resolvedAt := liveEventResolvedAt(response)
		if !force && dispatcher.lastSecurityResolvedAt[subscription.InstrumentID] == resolvedAt {
			continue
		}
		event := cloneLiveEventMap(response)
		event["type"] = "market.security-details"
		event["at"] = resolvedAt
		if err := dispatcher.writer.WriteEvent(event); err != nil {
			return err
		}
		dispatcher.lastSecurityResolvedAt[subscription.InstrumentID] = resolvedAt
	}
	return nil
}

func (dispatcher *liveWebSocketDispatcher) writeDepthEvents(force bool) error {
	for _, subscription := range dispatcher.client.snapshot().Depth {
		if subscriber, ok := dispatcher.server.futuBroker().(broker.OrderBookSubscriber); ok {
			_ = subscriber.SubscribeOrderBook(dispatcher.requestCtx, broker.OrderBookSubscribeRequest{
				ReadQuery: brokerReadQuery(subscription.InstrumentID),
				Symbols:   []string{subscription.InstrumentID},
				Num:       subscription.Num,
			})
		}
		response, err := dispatcher.server.marketDepthResponseForInstrument(dispatcher.requestCtx, subscription.Market, subscription.Symbol, marketDepthQuery{
			Num: newOptionalIntValue(int(subscription.Num)),
		})
		if err != nil {
			continue
		}
		resolvedAt := liveEventResolvedAt(response)
		key := subscription.InstrumentID + "|" + strconv.Itoa(int(subscription.Num))
		if !force && dispatcher.lastDepthResolvedAt[key] == resolvedAt {
			continue
		}
		event := cloneLiveEventMap(response)
		event["type"] = "market.depth"
		event["at"] = resolvedAt
		if err := dispatcher.writer.WriteEvent(event); err != nil {
			return err
		}
		dispatcher.lastDepthResolvedAt[key] = resolvedAt
	}
	return nil
}

func cloneLiveEventMap(value map[string]any) map[string]any {
	result := make(map[string]any, len(value)+2)
	for key, item := range value {
		result[key] = item
	}
	return result
}

func liveEventResolvedAt(value map[string]any) string {
	meta, _ := value["meta"].(map[string]any)
	resolvedAt, _ := meta["resolvedAt"].(string)
	return resolvedAt
}
