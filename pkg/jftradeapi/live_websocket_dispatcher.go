package jftradeapi

import (
	"context"
	"time"

	"github.com/gorilla/websocket"
)

type liveWebSocketDispatcher struct {
	server                  *Server
	requestCtx              context.Context
	conn                    *websocket.Conn
	clientClosed            <-chan struct{}
	lastSentByInstrument    map[string]string
	lastSentNotificationSeq uint64
	heartbeatInterval       time.Duration
	dataInterval            time.Duration
}

func newLiveWebSocketDispatcher(server *Server, requestCtx context.Context, conn *websocket.Conn, clientClosed <-chan struct{}) *liveWebSocketDispatcher {
	return &liveWebSocketDispatcher{
		server:               server,
		requestCtx:           requestCtx,
		conn:                 conn,
		clientClosed:         clientClosed,
		lastSentByInstrument: map[string]string{},
		heartbeatInterval:    15 * time.Second,
		dataInterval:         liveTickDispatchInterval,
	}
}

func (dispatcher *liveWebSocketDispatcher) writeInitialEvents() error {
	if err := dispatcher.writeHeartbeat(); err != nil {
		return err
	}
	return dispatcher.writeLiveData()
}

func (dispatcher *liveWebSocketDispatcher) run() error {
	heartbeatTicker := time.NewTicker(dispatcher.heartbeatInterval)
	defer heartbeatTicker.Stop()
	dataTicker := time.NewTicker(dispatcher.dataInterval)
	defer dataTicker.Stop()

	for {
		select {
		case <-dispatcher.requestCtx.Done():
			return nil
		case <-dispatcher.clientClosed:
			return nil
		case <-heartbeatTicker.C:
			if err := dispatcher.writeHeartbeat(); err != nil {
				return err
			}
		case <-dataTicker.C:
			if err := dispatcher.writeLiveData(); err != nil {
				return err
			}
		}
	}
}

func (dispatcher *liveWebSocketDispatcher) writeHeartbeat() error {
	return writeHeartbeat(dispatcher.conn)
}

func (dispatcher *liveWebSocketDispatcher) writeLiveData() error {
	if err := dispatcher.server.writeLiveMarketTicks(dispatcher.requestCtx, dispatcher.conn, dispatcher.lastSentByInstrument); err != nil {
		return err
	}
	if err := dispatcher.server.writeLiveNotifications(dispatcher.conn, &dispatcher.lastSentNotificationSeq); err != nil {
		return err
	}
	return nil
}
