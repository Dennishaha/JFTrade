package live

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/gorilla/websocket"

	livecore "github.com/jftrade/jftrade-main/internal/live"
)

type eventWriter interface {
	WriteEvent(any) error
}

type webSocketWriter struct {
	conn *websocket.Conn
}

func (w webSocketWriter) WriteEvent(value any) error {
	return w.conn.WriteJSON(value)
}

type dispatcher struct {
	handler                 *Handler
	requestCtx              context.Context
	writer                  eventWriter
	client                  *livecore.Client
	clientClosed            <-chan struct{}
	depthUpdated            <-chan struct{}
	lastSentByInstrument    map[string]string
	lastSentNotificationSeq uint64
	lastSecurityResolvedAt  map[string]string
	lastDepthResolvedAt     map[string]string
	wroteLiveData           bool
}

func newDispatcher(
	handler *Handler,
	requestCtx context.Context,
	conn *websocket.Conn,
	client *livecore.Client,
	clientClosed <-chan struct{},
	depthUpdated <-chan struct{},
) *dispatcher {
	handler.promoteConnection(conn)
	return &dispatcher{
		handler:                handler,
		requestCtx:             requestCtx,
		writer:                 webSocketWriter{conn: conn},
		client:                 client,
		clientClosed:           clientClosed,
		depthUpdated:           depthUpdated,
		lastSentByInstrument:   map[string]string{},
		lastSecurityResolvedAt: map[string]string{},
		lastDepthResolvedAt:    map[string]string{},
	}
}

func (d *dispatcher) writeInitialEvents() error {
	if err := d.writeHeartbeat(); err != nil {
		return err
	}
	if err := d.writeLiveData(); err != nil {
		return err
	}
	return d.writeAuxiliarySubscriptions(true)
}

func (d *dispatcher) run() error {
	heartbeatTicker := time.NewTicker(d.handler.options.HeartbeatInterval)
	defer heartbeatTicker.Stop()
	dataTicker := time.NewTicker(d.handler.options.DataInterval)
	defer dataTicker.Stop()
	consoleTicker := time.NewTicker(d.handler.options.ConsoleRefreshInterval)
	defer consoleTicker.Stop()
	securityTicker := time.NewTicker(d.handler.options.SecurityDetailsInterval)
	defer securityTicker.Stop()
	depthTicker := time.NewTicker(d.handler.options.DepthRefreshInterval)
	defer depthTicker.Stop()

	for {
		select {
		case <-d.requestCtx.Done():
			return nil
		case <-d.clientClosed:
			return nil
		case <-d.client.Updated():
			if err := d.writeAuxiliarySubscriptions(true); err != nil {
				return err
			}
		case <-d.depthUpdated:
			if err := d.writeDepthEvents(true); err != nil {
				return err
			}
		case <-heartbeatTicker.C:
			if err := d.writeHeartbeat(); err != nil {
				return err
			}
		case <-dataTicker.C:
			if err := d.writeLiveData(); err != nil {
				return err
			}
		case <-consoleTicker.C:
			if err := d.writeConsoleRefresh(); err != nil {
				return err
			}
		case <-securityTicker.C:
			if err := d.writeSecurityDetailsEvents(false); err != nil {
				return err
			}
		case <-depthTicker.C:
			if err := d.writeDepthEvents(false); err != nil {
				return err
			}
		}
	}
}

func (d *dispatcher) writeHeartbeat() error {
	return d.writer.WriteEvent(d.handler.backend.Heartbeat(
		d.handler.options.HeartbeatInterval,
		d.handler.Stats(),
		d.handler.ActiveInstrumentIDs(),
	))
}

func (d *dispatcher) writeLiveData() error {
	initialObservedAt := ""
	if !d.wroteLiveData {
		initialObservedAt = time.Now().UTC().Format(time.RFC3339Nano)
	}
	ticks, err := d.handler.backend.MarketTicks(
		d.requestCtx,
		d.client.Snapshot().ActiveInstruments,
		initialObservedAt,
	)
	if err != nil {
		return err
	}
	for _, tick := range ticks {
		if tick.InstrumentID == "" || d.lastSentByInstrument[tick.InstrumentID] == tick.ObservedAt {
			continue
		}
		if err := d.writer.WriteEvent(tick.Payload); err != nil {
			return err
		}
		d.lastSentByInstrument[tick.InstrumentID] = tick.ObservedAt
	}
	d.wroteLiveData = true
	return d.writeNotifications()
}

func (d *dispatcher) writeNotifications() error {
	for _, event := range d.handler.backend.NotificationsAfter(d.lastSentNotificationSeq) {
		if err := d.writer.WriteEvent(notificationEventMap(event)); err != nil {
			return err
		}
		d.lastSentNotificationSeq = event.Sequence
	}
	return nil
}

func (d *dispatcher) writeAuxiliarySubscriptions(force bool) error {
	if err := d.writeConsoleRefresh(); err != nil {
		return err
	}
	if err := d.writeSecurityDetailsEvents(force); err != nil {
		return err
	}
	return d.writeDepthEvents(force)
}

func (d *dispatcher) writeConsoleRefresh() error {
	if !d.client.Snapshot().ConsoleRefresh {
		return nil
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	return d.writer.WriteEvent(map[string]any{
		"type": "console.refresh", "at": now, "checkedAt": now,
	})
}

func (d *dispatcher) writeSecurityDetailsEvents(force bool) error {
	for _, subscription := range d.client.Snapshot().SecurityDetails {
		response, err := d.handler.backend.SecurityDetails(
			d.requestCtx, subscription.Market, subscription.Symbol,
		)
		if err != nil {
			continue
		}
		resolvedAt := eventResolvedAt(response)
		if !force && d.lastSecurityResolvedAt[subscription.InstrumentID] == resolvedAt {
			continue
		}
		event := cloneEventMap(response)
		event["type"] = "market.security-details"
		event["at"] = resolvedAt
		if err := d.writer.WriteEvent(event); err != nil {
			return err
		}
		d.lastSecurityResolvedAt[subscription.InstrumentID] = resolvedAt
	}
	return nil
}

func (d *dispatcher) writeDepthEvents(force bool) error {
	for _, subscription := range d.client.Snapshot().Depth {
		d.handler.backend.SubscribeDepth(d.requestCtx, subscription.InstrumentID, subscription.Num)
		response, err := d.handler.backend.Depth(
			d.requestCtx, subscription.Market, subscription.Symbol, subscription.Num,
		)
		if err != nil {
			continue
		}
		resolvedAt := eventResolvedAt(response)
		key := subscription.InstrumentID + "|" + strconv.Itoa(int(subscription.Num))
		if !force && d.lastDepthResolvedAt[key] == resolvedAt {
			continue
		}
		event := cloneEventMap(response)
		event["type"] = "market.depth"
		event["at"] = resolvedAt
		if err := d.writer.WriteEvent(event); err != nil {
			return err
		}
		d.lastDepthResolvedAt[key] = resolvedAt
	}
	return nil
}

func notificationEventMap(event livecore.Event) map[string]any {
	payload := map[string]any{
		"type":     "system.notification",
		"id":       fmt.Sprintf("system-notification-%d", event.Sequence),
		"at":       event.At,
		"level":    event.Level,
		"title":    event.Title,
		"source":   event.Source,
		"brokerId": event.BrokerID,
		"category": event.Category,
	}
	if event.Message != "" {
		payload["message"] = event.Message
	}
	return payload
}

func cloneEventMap(value map[string]any) map[string]any {
	result := make(map[string]any, len(value)+2)
	for key, item := range value {
		result[key] = item
	}
	return result
}

func eventResolvedAt(value map[string]any) string {
	meta, _ := value["meta"].(map[string]any)
	resolvedAt, _ := meta["resolvedAt"].(string)
	return resolvedAt
}
