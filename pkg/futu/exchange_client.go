package futu

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"google.golang.org/protobuf/proto"

	"github.com/jftrade/jftrade-main/pkg/futu/opend"
	initpb "github.com/jftrade/jftrade-main/pkg/futu/pb/initconnect"
	notifypb "github.com/jftrade/jftrade-main/pkg/futu/pb/notify"
	qotupdateorderbookpb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotupdateorderbook"
	trdcommonpb "github.com/jftrade/jftrade-main/pkg/futu/pb/trdcommon"
	"github.com/jftrade/jftrade-main/pkg/observability"
)

// --- Client lifecycle management (connect / reconnect / keepalive / invalidate) ---

func (e *Exchange) callProto(ctx context.Context, protoID uint32, req proto.Message, resp proto.Message) error {
	return e.withClient(ctx, func(client *opend.Client) error {
		return client.Call(ctx, protoID, req, resp)
	})
}

func (e *Exchange) withClient(ctx context.Context, fn func(*opend.Client) error) error {
	ctx = observability.WithFields(ctx, observability.Fields{BrokerID: "futu", Source: "opend"})
	if _, hasDeadline := ctx.Deadline(); !hasDeadline {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
	}
	var lastErr error
	for range 2 {
		client, err := e.ensureClient(ctx)
		if err != nil {
			observability.ErrorWithImportance(ctx, observability.ImportanceHigh, "opend client unavailable", err)
			return err
		}
		if err := fn(client); err != nil {
			if !isRecoverableOpenDErr(err) {
				if !errors.Is(err, ErrSubscriptionRequired) {
					observability.ErrorWithImportance(ctx, observability.ImportanceHigh, "opend query failed", err)
				}
				return err
			}
			lastErr = err
			log.Printf("futu withClient: recoverable error, invalidating client and retrying: %v", err)
			e.invalidateClient()
			continue
		}
		return nil
	}
	return lastErr
}

func (e *Exchange) ensureClient(ctx context.Context) (*opend.Client, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.client == nil {
		e.client = e.newClient()
	}
	if e.ready {
		select {
		case <-e.client.Done():
			log.Printf("futu OpenD client done; reconnecting to %s", e.addr)
			jftradeErr6 := e.invalidateClientLocked()
			jftradeLogError(jftradeErr6)
			e.client = e.newClient()
		default:
			e.bindOrderBookNotifyLocked(e.client)
			e.bindSystemNotifyLocked(e.client)
			e.bindTradeUpdateNotifyLocked(e.client)
			return e.client, nil
		}
	}
	log.Printf("futu OpenD connecting new session to %s (ready=%v clientNil=%v)",
		e.addr, e.ready, e.client == nil)

	connectStart := time.Now()
	if err := e.client.Connect(ctx); err != nil {
		jftradeErr2 := e.invalidateClientLocked()
		jftradeLogError(jftradeErr2)
		log.Printf("futu OpenD connect to %s failed after %v: %v", e.addr, time.Since(connectStart), err)
		return nil, err
	}
	connectElapsed := time.Since(connectStart)

	initStart := time.Now()
	initState, err := e.initializeOpenDSessionLocked(ctx, connectElapsed)
	if err != nil {
		jftradeErr1 := e.invalidateClientLocked()
		jftradeLogError(jftradeErr1)
		log.Printf("futu OpenD session initialization to %s failed after %v (connect=%v): %v",
			e.addr, time.Since(initStart), connectElapsed, err)
		return nil, err
	}

	if keepAliveInterval := initState.GetKeepAliveInterval(); keepAliveInterval > 0 {
		e.client.StartKeepAlive(time.Duration(keepAliveInterval) * time.Second)
	}
	e.subscriptions.ensure()
	e.bindOrderBookNotifyLocked(e.client)
	e.bindSystemNotifyLocked(e.client)
	e.bindTradeUpdateNotifyLocked(e.client)
	if err := e.resubscribeTradeAccountPushLocked(ctx, e.client); err != nil {
		jftradeErr5 := e.invalidateClientLocked()
		jftradeLogError(jftradeErr5)
		log.Printf("futu OpenD trade account push resubscribe failed after reconnect: %v", err)
		return nil, err
	}
	e.ready = true
	e.connectionGeneration++
	log.Printf("futu OpenD session established to %s (connect=%v init=%v total=%v)",
		e.addr, connectElapsed, time.Since(initStart), time.Since(connectStart))
	return e.client, nil
}

func (e *Exchange) initializeOpenDSessionLocked(ctx context.Context, connectElapsed time.Duration) (*initpb.S2C, error) {
	initReq := &initpb.Request{C2S: &initpb.C2S{
		ClientVer:           new(int32(101)),
		ClientID:            new("jftrade-bbgo"),
		RecvNotify:          new(true),
		ProgrammingLanguage: new("Go"),
	}}
	var initResp initpb.Response
	if err := e.client.Call(ctx, opend.ProtoInitConnect, initReq, &initResp); err != nil {
		return nil, fmt.Errorf("opend InitConnect failed after TCP connect %v: %w", connectElapsed, err)
	}
	initState, err := validateInitConnectResponse(&initResp)
	if err != nil {
		return nil, err
	}
	e.client.SetConnID(initState.GetConnID())
	// OpenD may push initial status notifications immediately after InitConnect,
	// before the version-bearing GetGlobalState response arrives.
	e.bindSystemNotifyLocked(e.client)
	globalState, err := e.client.GetGlobalState(ctx)
	if err != nil {
		return nil, fmt.Errorf("opend GetGlobalState during session setup: %w", err)
	}
	serverBuildNo := globalState.ServerBuildNo
	if err := opend.ValidateMinimumVersion(globalState.ServerVer, &serverBuildNo); err != nil {
		return nil, err
	}
	return initState, nil
}

func validateInitConnectResponse(initResp *initpb.Response) (*initpb.S2C, error) {
	if initResp.GetRetType() != 0 {
		return nil, fmt.Errorf("opend InitConnect retType=%d errCode=%d retMsg=%s", initResp.GetRetType(), initResp.GetErrCode(), initResp.GetRetMsg())
	}
	initState := initResp.GetS2C()
	if initState == nil {
		return nil, fmt.Errorf("opend InitConnect returned no server state")
	}
	if err := opend.ValidateMinimumVersion(initState.GetServerVer(), nil); err != nil {
		return nil, err
	}
	return initState, nil
}

// --- Notification binding & dispatch ---

func (e *Exchange) bindOrderBookNotifyLocked(client *opend.Client) {
	e.handlerMu.RLock()
	hasHandlers := len(e.orderBookNotifyHandlers) > 0
	e.handlerMu.RUnlock()
	if client == nil || !hasHandlers || e.orderBookNotifyClient == client {
		return
	}
	client.SubscribeOrderBook(e.dispatchOrderBookNotify)
	e.orderBookNotifyClient = client
}

func (e *Exchange) dispatchOrderBookNotify(update *qotupdateorderbookpb.S2C) {
	if update == nil {
		return
	}
	symbol, err := futuSymbolFromSecurity(update.GetSecurity())
	if err != nil || symbol == "" {
		return
	}

	e.handlerMu.RLock()
	handlers := make([]func(string), 0, len(e.orderBookNotifyHandlers))
	for _, handler := range e.orderBookNotifyHandlers {
		handlers = append(handlers, handler)
	}
	e.handlerMu.RUnlock()

	for _, handler := range handlers {
		handler(symbol)
	}
}

func (e *Exchange) bindSystemNotifyLocked(client *opend.Client) {
	e.handlerMu.RLock()
	hasHandlers := len(e.systemNotifyHandlers) > 0
	e.handlerMu.RUnlock()
	if client == nil || !hasHandlers || e.systemNotifyClient == client {
		return
	}
	client.SubscribeNotify(e.dispatchSystemNotify)
	e.systemNotifyClient = client
}

func (e *Exchange) dispatchSystemNotify(response *notifypb.Response) {
	e.handlerMu.RLock()
	handlers := append([]func(*notifypb.Response){}, e.systemNotifyHandlers...)
	e.handlerMu.RUnlock()
	for _, handler := range handlers {
		handler(response)
	}
}

func (e *Exchange) bindTradeUpdateNotifyLocked(client *opend.Client) {
	if client == nil || e.orderUpdateNotifyClient == client {
		return
	}
	e.handlerMu.RLock()
	hasHandlers := len(e.orderUpdateHandlers) > 0 || len(e.orderFillUpdateHandlers) > 0
	e.handlerMu.RUnlock()
	if !hasHandlers {
		return
	}
	client.SubscribeOrderUpdate(e.dispatchOrderUpdateNotify)
	client.SubscribeOrderFillUpdate(e.dispatchOrderFillUpdateNotify)
	e.orderUpdateNotifyClient = client
}

func (e *Exchange) dispatchOrderUpdateNotify(header *trdcommonpb.TrdHeader, order *trdcommonpb.Order) {
	e.handlerMu.RLock()
	handlers := make([]func(*trdcommonpb.TrdHeader, *trdcommonpb.Order), 0, len(e.orderUpdateHandlers))
	for _, handler := range e.orderUpdateHandlers {
		handlers = append(handlers, handler)
	}
	e.handlerMu.RUnlock()
	for _, handler := range handlers {
		handler(header, order)
	}
}

func (e *Exchange) dispatchOrderFillUpdateNotify(header *trdcommonpb.TrdHeader, fill *trdcommonpb.OrderFill) {
	e.handlerMu.RLock()
	handlers := make([]func(*trdcommonpb.TrdHeader, *trdcommonpb.OrderFill), 0, len(e.orderFillUpdateHandlers))
	for _, handler := range e.orderFillUpdateHandlers {
		handlers = append(handlers, handler)
	}
	e.handlerMu.RUnlock()
	for _, handler := range handlers {
		handler(header, fill)
	}
}

func (e *Exchange) resubscribeTradeAccountPushLocked(ctx context.Context, client *opend.Client) error {
	if client == nil || len(e.tradeAccountPushIDs) == 0 || e.tradeAccountPushClient == client {
		return nil
	}
	ids := append([]uint64(nil), e.tradeAccountPushIDs...)
	if err := client.SubscribeAccountPush(ctx, ids); err != nil {
		return err
	}
	e.tradeAccountPushClient = client
	return nil
}

// --- Client construction / invalidation ---

func (e *Exchange) newClient() *opend.Client {
	return opend.New(opend.Config{
		Addr:             e.addr,
		WebSocketKey:     e.webSocketKey,
		HandshakeTimeout: 3 * time.Second,
		RequestTimeout:   8 * time.Second,
	})
}

func (e *Exchange) invalidateClient() {
	e.mu.Lock()
	defer e.mu.Unlock()
	log.Printf("futu OpenD invalidateClient (public) addr=%s", e.addr)
	jftradeErr4 := e.invalidateClientLocked()
	jftradeLogError(jftradeErr4)
}

func (e *Exchange) invalidateClientLocked() error {
	client := e.client
	if e.ready {
		e.connectionGeneration++
	}
	e.client = nil
	e.ready = false
	e.subscriptions.reset()
	e.orderBookNotifyClient = nil
	e.systemNotifyClient = nil
	e.orderUpdateNotifyClient = nil
	e.tradeAccountPushClient = nil
	if client != nil {
		return client.Close()
	}
	return nil
}

func isRecoverableOpenDErr(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, opend.ErrClosed) || errors.Is(err, opend.ErrRequestTimeout) {
		return true
	}
	lower := strings.ToLower(err.Error())
	return strings.Contains(lower, "broken pipe") ||
		strings.Contains(lower, "connection reset") ||
		strings.Contains(lower, "eof") ||
		strings.Contains(lower, "use of closed network connection")
}

func jftradeLogError(values ...any) {
	for _, value := range values {
		if err, ok := value.(error); ok && err != nil {
			log.Printf("best-effort operation failed: %v", err)
		}
	}
}
