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
)

// --- Client lifecycle management (connect / reconnect / keepalive / invalidate) ---

func (e *Exchange) callProto(ctx context.Context, protoID uint32, req proto.Message, resp proto.Message) error {
	return e.withClient(ctx, func(client *opend.Client) error {
		return client.Call(ctx, protoID, req, resp)
	})
}

func (e *Exchange) withClient(ctx context.Context, fn func(*opend.Client) error) error {
	if _, hasDeadline := ctx.Deadline(); !hasDeadline {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
	}
	var lastErr error
	for attempt := 0; attempt < 2; attempt++ {
		client, err := e.ensureClient(ctx)
		if err != nil {
			return err
		}
		if err := fn(client); err != nil {
			if !isRecoverableOpenDErr(err) {
				return err
			}
			lastErr = err
			log.Printf("futu withClient: recoverable error, invalidating client and retrying: %v", err)
			e.invalidateClient()
			continue
		}
		return nil
	}
	if lastErr != nil {
		return lastErr
	}
	return fmt.Errorf("opend: unavailable client")
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
			_ = e.invalidateClientLocked()
			e.client = e.newClient()
		default:
			e.bindOrderBookNotifyLocked(e.client)
			e.bindSystemNotifyLocked(e.client)
			return e.client, nil
		}
	}
	log.Printf("futu OpenD connecting new session to %s (ready=%v clientNil=%v)",
		e.addr, e.ready, e.client == nil)

	connectStart := time.Now()
	if err := e.client.Connect(ctx); err != nil {
		_ = e.invalidateClientLocked()
		log.Printf("futu OpenD connect to %s failed after %v: %v", e.addr, time.Since(connectStart), err)
		return nil, err
	}
	connectElapsed := time.Since(connectStart)

	initStart := time.Now()
	initReq := &initpb.Request{C2S: &initpb.C2S{
		ClientVer:           proto.Int32(101),
		ClientID:            proto.String("jftrade-bbgo"),
		RecvNotify:          proto.Bool(true),
		ProgrammingLanguage: proto.String("Go"),
	}}
	var initResp initpb.Response
	if err := e.client.Call(ctx, opend.ProtoInitConnect, initReq, &initResp); err != nil {
		_ = e.invalidateClientLocked()
		log.Printf("futu OpenD InitConnect to %s failed after %v (connect=%v): %v",
			e.addr, time.Since(initStart), connectElapsed, err)
		return nil, err
	}
	if initResp.GetRetType() != 0 {
		err := fmt.Errorf("opend InitConnect retType=%d errCode=%d retMsg=%s", initResp.GetRetType(), initResp.GetErrCode(), initResp.GetRetMsg())
		log.Printf("futu OpenD InitConnect error: %v", err)
		_ = e.invalidateClientLocked()
		return nil, err
	}

	e.ready = true
	e.client.SetConnID(initResp.GetS2C().GetConnID())
	if keepAliveInterval := initResp.GetS2C().GetKeepAliveInterval(); keepAliveInterval > 0 {
		e.client.StartKeepAlive(time.Duration(keepAliveInterval) * time.Second)
	}
	e.subscriptions.ensure()
	e.bindOrderBookNotifyLocked(e.client)
	e.bindSystemNotifyLocked(e.client)
	log.Printf("futu OpenD session established to %s (connect=%v init=%v total=%v)",
		e.addr, connectElapsed, time.Since(initStart), time.Since(connectStart))
	return e.client, nil
}

// --- Notification binding & dispatch ---

func (e *Exchange) bindOrderBookNotifyLocked(client *opend.Client) {
	if client == nil || len(e.orderBookNotifyHandlers) == 0 || e.orderBookNotifyClient == client {
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

	e.mu.Lock()
	handlers := make([]func(string), 0, len(e.orderBookNotifyHandlers))
	for _, handler := range e.orderBookNotifyHandlers {
		handlers = append(handlers, handler)
	}
	e.mu.Unlock()

	for _, handler := range handlers {
		handler(symbol)
	}
}

func (e *Exchange) bindSystemNotifyLocked(client *opend.Client) {
	if client == nil || len(e.systemNotifyHandlers) == 0 || e.systemNotifyClient == client {
		return
	}
	client.SubscribeNotify(e.dispatchSystemNotify)
	e.systemNotifyClient = client
}

func (e *Exchange) dispatchSystemNotify(response *notifypb.Response) {
	e.mu.Lock()
	handlers := append([]func(*notifypb.Response){}, e.systemNotifyHandlers...)
	e.mu.Unlock()
	for _, handler := range handlers {
		handler(response)
	}
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
	_ = e.invalidateClientLocked()
}

func (e *Exchange) invalidateClientLocked() error {
	client := e.client
	e.client = nil
	e.ready = false
	e.subscriptions.reset()
	e.orderBookNotifyClient = nil
	e.systemNotifyClient = nil
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
