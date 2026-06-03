package jftradeapi

import (
	"context"
	"time"
)

type liveEventWriter interface {
	WriteEvent(value any) error
}

type liveStreamDispatcher struct {
	server                  *Server
	requestCtx              context.Context
	writer                  liveEventWriter
	clientClosed            <-chan struct{}
	lastSentByInstrument    map[string]string
	wroteLiveData           bool
	lastSentNotificationSeq uint64
	heartbeatInterval       time.Duration
	dataInterval            time.Duration
}

func newLiveStreamDispatcher(server *Server, requestCtx context.Context, writer liveEventWriter, clientClosed <-chan struct{}) *liveStreamDispatcher {
	return &liveStreamDispatcher{
		server:               server,
		requestCtx:           requestCtx,
		writer:               writer,
		clientClosed:         clientClosed,
		lastSentByInstrument: map[string]string{},
		heartbeatInterval:    15 * time.Second,
		dataInterval:         liveTickDispatchInterval,
	}
}

func (dispatcher *liveStreamDispatcher) writeInitialEvents() error {
	if err := dispatcher.writeHeartbeat(); err != nil {
		return err
	}
	return dispatcher.writeLiveData()
}

func (dispatcher *liveStreamDispatcher) run() error {
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

func (dispatcher *liveStreamDispatcher) writeHeartbeat() error {
	return writeHeartbeat(
		dispatcher.writer,
		dispatcher.server.liveHeartbeatEvent(dispatcher.heartbeatInterval),
	)
}

func (dispatcher *liveStreamDispatcher) writeLiveData() error {
	if err := dispatcher.server.writeLiveMarketTicks(dispatcher.requestCtx, dispatcher.writer, dispatcher.lastSentByInstrument, !dispatcher.wroteLiveData); err != nil {
		return err
	}
	dispatcher.wroteLiveData = true
	if err := dispatcher.server.writeLiveNotifications(dispatcher.writer, &dispatcher.lastSentNotificationSeq); err != nil {
		return err
	}
	return nil
}
