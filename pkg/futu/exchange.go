// Package futu plugs a Futu OpenD-backed exchange into bbgo.
//
// Importing this package (typically via a blank import in main) registers
// "futu" as a bbgo exchange via pkg/exchange.Register, without modifying bbgo.
package futu

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/c9s/bbgo/pkg/exchange"
	"github.com/c9s/bbgo/pkg/fixedpoint"
	"github.com/c9s/bbgo/pkg/types"
	"google.golang.org/protobuf/proto"

	"github.com/jftrade/jftrade-main/pkg/futu/opend"
	initpb "github.com/jftrade/jftrade-main/pkg/futu/pb/initconnect"
	notifypb "github.com/jftrade/jftrade-main/pkg/futu/pb/notify"
	qotcommonpb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotcommon"
	qotgetbasicqotpb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotgetbasicqot"
	qotsubpb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotsub"
)

// Name is the bbgo exchange name used in configs and env-var prefix.
const Name types.ExchangeName = "futu"

// EnvOpenDAddr names the env var carrying the OpenD WebSocket address.
const EnvOpenDAddr = "FUTU_OPEND_ADDR"

// EnvOpenDWebSocketKey names the env var carrying the optional OpenD
// WebSocket plain-text key. FutuOpenD stores the MD5 value, while clients send
// the plain value during the WebSocket handshake.
const EnvOpenDWebSocketKey = "FUTU_OPEND_WEBSOCKET_KEY"

// DefaultOpenDAddr is the fallback OpenD API address used when neither the
// session-prefixed env var nor FUTU_OPEND_ADDR is set.
const DefaultOpenDAddr = "127.0.0.1:11110"

// ErrNotSupported is returned for bbgo Exchange methods that do not map
// cleanly to the Futu trading domain (e.g. spot taker/maker fee math).
var ErrNotSupported = errors.New("futu exchange: operation not supported")

// Exchange implements bbgo's types.Exchange backed by OpenD.
//
// Only a minimal subset of the interface is wired to live OpenD RPCs in this
// edition; the rest returns ErrNotSupported with a clear message. This keeps
// the binary buildable inside bbgo while leaving room for incremental
// adapter work without touching bbgo internals.
type Exchange struct {
	addr         string
	webSocketKey string

	mu                   sync.Mutex
	sessionMu            sync.RWMutex
	client               *opend.Client
	ready                bool
	subscriptions        subscriptionRegistry
	systemNotifyClient   *opend.Client
	systemNotifyHandlers []func(*notifypb.Response)
	klineSessions        map[string]klineSessionRecord
	marketSessionSamples map[string][]marketSessionSample

	// customMarkets holds market info for symbols that are not natively
	// returned by QueryMarkets but should be known to the exchange — e.g.
	// backtest symbols that the live OpenD connection hasn't discovered.
	customMarkets types.MarketMap
}

// NewExchange constructs an Exchange. It does not dial OpenD: bbgo expects
// constructors to be cheap; the underlying client lazily connects.
func NewExchange(addr string) *Exchange {
	cfg := opend.Config{Addr: addr}
	return NewExchangeWithConfig(cfg)
}

// NewExchangeWithConfig constructs an Exchange with the full OpenD client
// configuration.
func NewExchangeWithConfig(cfg opend.Config) *Exchange {
	return &Exchange{
		addr:          cfg.Addr,
		webSocketKey:  cfg.WebSocketKey,
		subscriptions: newSubscriptionRegistry(),
	}
}

// Client exposes the underlying OpenD client for advanced (non-bbgo) callers.
func (e *Exchange) Client() *opend.Client {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.client
}

// OnSystemNotify registers a handler for OpenD system notifications (protocol
// 1003). Handlers survive client reconnects and are rebound to fresh OpenD
// sessions automatically.
func (e *Exchange) OnSystemNotify(fn func(*notifypb.Response)) {
	if fn == nil {
		return
	}

	e.mu.Lock()
	defer e.mu.Unlock()
	e.systemNotifyHandlers = append(e.systemNotifyHandlers, fn)
	e.bindSystemNotifyLocked(e.client)
}

// EnsureSystemNotifications brings up the shared OpenD session so registered
// system-notify handlers can receive protocol 1003 pushes.
func (e *Exchange) EnsureSystemNotifications(ctx context.Context) error {
	_, err := e.ensureClient(ctx)
	return err
}

// --- bbgo types.ExchangeMinimal ---

func (e *Exchange) Name() types.ExchangeName    { return Name }
func (e *Exchange) PlatformFeeCurrency() string { return "HKD" }

// --- bbgo types.ExchangeMarketDataService ---

func (e *Exchange) NewStream() types.Stream { return NewStream(e) }

func (e *Exchange) QueryMarkets(ctx context.Context) (types.MarketMap, error) {
	base := types.MarketMap{
		"HK.00700": {
			Exchange:        Name,
			Symbol:          "HK.00700",
			LocalSymbol:     "HK.00700",
			PricePrecision:  3,
			VolumePrecision: 0,
			QuotePrecision:  3,
			QuoteCurrency:   "HKD",
			BaseCurrency:    "HK.00700",
			MinQuantity:     fixedpoint.One,
			StepSize:        fixedpoint.One,
			TickSize:        fixedpoint.NewFromFloat(0.001),
		},
	}
	e.mu.Lock()
	for symbol, market := range e.customMarkets {
		base[symbol] = market
	}
	e.mu.Unlock()
	return base, nil
}

// EnsureMarket makes a minimal types.Market available for symbol so that
// backtest matching books and Market() lookups succeed. It derives market
// parameters (quote currency, tick size, precision) from the symbol prefix.
func (e *Exchange) EnsureMarket(symbol string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.customMarkets == nil {
		e.customMarkets = make(types.MarketMap)
	}
	sym := strings.ToUpper(strings.TrimSpace(symbol))
	if _, ok := e.customMarkets[sym]; ok {
		return
	}
	e.customMarkets[sym] = inferMarket(sym)
}

// inferMarket builds a reasonable types.Market from a "MARKET.CODE" symbol.
func inferMarket(symbol string) types.Market {
	sym := strings.ToUpper(strings.TrimSpace(symbol))
	m := types.Market{
		Exchange:        Name,
		Symbol:          sym,
		LocalSymbol:     sym,
		PricePrecision:  2,
		VolumePrecision: 0,
		QuotePrecision:  2,
		MinQuantity:     fixedpoint.One,
		StepSize:        fixedpoint.One,
	}
	switch {
	case strings.HasPrefix(sym, "US."):
		m.QuoteCurrency = "USD"
		m.BaseCurrency = sym
		m.TickSize = fixedpoint.NewFromFloat(0.01)
	case strings.HasPrefix(sym, "HK."):
		m.QuoteCurrency = "HKD"
		m.BaseCurrency = sym
		m.PricePrecision = 3
		m.QuotePrecision = 3
		m.TickSize = fixedpoint.NewFromFloat(0.001)
	default:
		m.QuoteCurrency = "HKD"
		m.BaseCurrency = sym
		m.PricePrecision = 3
		m.QuotePrecision = 3
		m.TickSize = fixedpoint.NewFromFloat(0.001)
	}
	return m
}

func (e *Exchange) QueryTicker(ctx context.Context, symbol string) (*types.Ticker, error) {
	basicQot, err := e.queryBasicQot(ctx, symbol)
	if err != nil {
		return nil, err
	}
	return tickerFromBasicQot(basicQot), nil
}

func (e *Exchange) QueryTickers(ctx context.Context, symbol ...string) (map[string]types.Ticker, error) {
	quotes, err := e.queryBasicQotList(ctx, symbol)
	if err != nil {
		return nil, err
	}
	tickers := make(map[string]types.Ticker, len(quotes))
	for currentSymbol, basicQot := range quotes {
		ticker := tickerFromBasicQot(basicQot)
		if ticker != nil {
			tickers[currentSymbol] = *ticker
		}
	}
	return tickers, nil
}

// QueryQuoteSnapshot returns BasicQot fields, including US pre-market,
// after-hours, and overnight quote blocks when OpenD provides them.
func (e *Exchange) QueryQuoteSnapshot(ctx context.Context, symbol string) (*QuoteSnapshot, error) {
	basicQot, err := e.queryBasicQot(ctx, symbol)
	if err != nil {
		return nil, err
	}
	canonical, err := futuSymbolFromSecurity(basicQot.GetSecurity())
	if err != nil {
		canonical = strings.TrimSpace(strings.ToUpper(symbol))
	}
	snapshot := quoteSnapshotFromBasicQot(basicQot, canonical)
	if snapshot != nil {
		e.RecordMarketSessionSample(canonical, snapshot.Session, snapshot.QuoteAt)
	}
	return snapshot, nil
}

// --- bbgo types.ExchangeAccountService ---

func (e *Exchange) QueryAccount(ctx context.Context) (*types.Account, error) {
	acc := types.NewAccount()
	acc.AccountType = "futu"
	return acc, nil
}

func (e *Exchange) QueryAccountBalances(ctx context.Context) (types.BalanceMap, error) {
	return types.BalanceMap{
		"HKD": types.Balance{Currency: "HKD", Available: fixedpoint.Zero, Locked: fixedpoint.Zero},
	}, nil
}

// --- bbgo types.ExchangeTradeService ---

func (e *Exchange) SubmitOrder(ctx context.Context, order types.SubmitOrder) (*types.Order, error) {
	return nil, fmt.Errorf("%w: SubmitOrder symbol=%s side=%s qty=%s", ErrNotSupported, order.Symbol, order.Side, order.Quantity.String())
}

func (e *Exchange) QueryOpenOrders(ctx context.Context, symbol string) ([]types.Order, error) {
	return nil, nil
}

func (e *Exchange) CancelOrders(ctx context.Context, orders ...types.Order) error {
	return fmt.Errorf("%w: CancelOrders n=%d", ErrNotSupported, len(orders))
}

// Connect dials OpenD now, useful for health checks and tests.
func (e *Exchange) Connect(ctx context.Context) error {
	_, err := e.ensureClient(ctx)
	return err
}

// Close terminates the cached OpenD session.
func (e *Exchange) Close() error {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.invalidateClientLocked()
}

func init() {
	exchange.Register(Name, exchange.Factory{
		EnvLoader: func(prefix string) (exchange.Options, error) {
			addr := os.Getenv(prefix + "_OPEND_ADDR")
			if addr == "" {
				addr = os.Getenv(EnvOpenDAddr)
			}
			webSocketKey := os.Getenv(prefix + "_OPEND_WEBSOCKET_KEY")
			if webSocketKey == "" {
				webSocketKey = os.Getenv(EnvOpenDWebSocketKey)
			}
			if webSocketKey == "" {
				webSocketKey = os.Getenv("JFTRADE_FUTU_WEBSOCKET_KEY")
			}
			if addr == "" {
				// Fall back to the default OpenD WebSocket endpoint so that bbgo
				// can finish session bootstrap when the operator runs the
				// out-of-the-box FutuOpenD GUI on the same host. Configuration
				// via env still takes precedence when provided.
				addr = DefaultOpenDAddr
			}
			// bbgo will pass the loaded options into Constructor below.
			return exchange.Options{"OPEND_ADDR": addr, "OPEND_WEBSOCKET_KEY": webSocketKey}, nil
		},
		Constructor: func(opts exchange.Options) (types.Exchange, error) {
			addr := opts["OPEND_ADDR"]
			if addr == "" {
				addr = os.Getenv(EnvOpenDAddr)
			}
			webSocketKey := opts["OPEND_WEBSOCKET_KEY"]
			if webSocketKey == "" {
				webSocketKey = os.Getenv(EnvOpenDWebSocketKey)
			}
			if webSocketKey == "" {
				webSocketKey = os.Getenv("JFTRADE_FUTU_WEBSOCKET_KEY")
			}
			if addr == "" {
				addr = DefaultOpenDAddr
			}
			return NewExchangeWithConfig(opend.Config{Addr: addr, WebSocketKey: webSocketKey}), nil
		},
	})
}

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

func (e *Exchange) queryBasicQot(ctx context.Context, symbol string) (*qotcommonpb.BasicQot, error) {
	quotes, err := e.queryBasicQotList(ctx, []string{symbol})
	if err != nil {
		return nil, err
	}
	canonical := strings.TrimSpace(strings.ToUpper(symbol))
	quote := quotes[canonical]
	if quote == nil {
		return nil, fmt.Errorf("opend GetBasicQot returned no quotes for %s", symbol)
	}
	return quote, nil
}

type basicQotRequest struct {
	canonical string
	security  *qotcommonpb.Security
}

func (e *Exchange) queryBasicQotList(ctx context.Context, symbols []string) (map[string]*qotcommonpb.BasicQot, error) {
	requests := make([]basicQotRequest, 0, len(symbols))
	securityList := make([]*qotcommonpb.Security, 0, len(symbols))
	seen := make(map[string]struct{}, len(symbols))
	for _, symbol := range symbols {
		security, canonical, err := futuSecurityFromSymbol(symbol)
		if err != nil {
			return nil, err
		}
		if _, exists := seen[canonical]; exists {
			continue
		}
		seen[canonical] = struct{}{}
		requests = append(requests, basicQotRequest{canonical: canonical, security: security})
		securityList = append(securityList, security)
	}
	if len(requests) == 0 {
		return map[string]*qotcommonpb.BasicQot{}, nil
	}

	reqStart := time.Now()
	request := &qotgetbasicqotpb.Request{C2S: &qotgetbasicqotpb.C2S{SecurityList: securityList}}
	var response qotgetbasicqotpb.Response
	if err := e.withClient(ctx, func(client *opend.Client) error {
		subStart := time.Now()
		if err := e.ensureBasicQotSubscriptions(ctx, client, requests); err != nil {
			return err
		}
		subElapsed := time.Since(subStart)

		callStart := time.Now()
		if err := client.Call(ctx, opend.ProtoGetBasicQot, request, &response); err != nil {
			return err
		}
		callElapsed := time.Since(callStart)

		log.Printf("futu GetBasicQot symbols=%d sub=%v rpc=%v total=%v",
			len(requests), subElapsed, callElapsed, time.Since(reqStart))
		return nil
	}); err != nil {
		log.Printf("futu GetBasicQot symbols=%d failed after %v: %v",
			len(requests), time.Since(reqStart), err)
		return nil, err
	}
	if response.GetRetType() != 0 {
		return nil, fmt.Errorf("opend GetBasicQot retType=%d errCode=%d retMsg=%s", response.GetRetType(), response.GetErrCode(), response.GetRetMsg())
	}

	quotes := make(map[string]*qotcommonpb.BasicQot, len(response.GetS2C().GetBasicQotList()))
	for _, quote := range response.GetS2C().GetBasicQotList() {
		canonical, err := futuSymbolFromSecurity(quote.GetSecurity())
		if err != nil {
			continue
		}
		quotes[canonical] = quote
	}
	if len(quotes) == 0 {
		return nil, fmt.Errorf("opend GetBasicQot returned no quotes")
	}
	return quotes, nil
}

func (e *Exchange) ensureBasicQotSubscriptions(ctx context.Context, client *opend.Client, requests []basicQotRequest) error {
	e.mu.Lock()
	missing := make([]basicQotRequest, 0, len(requests))
	for _, request := range requests {
		if e.subscriptions.hasBasicQot(request.canonical) {
			continue
		}
		missing = append(missing, request)
	}
	e.mu.Unlock()
	if len(missing) == 0 {
		return nil
	}

	securityList := make([]*qotcommonpb.Security, 0, len(missing))
	for _, request := range missing {
		securityList = append(securityList, request.security)
	}
	if err := subscribeBasicQot(ctx, client, securityList); err != nil {
		return err
	}

	e.mu.Lock()
	defer e.mu.Unlock()
	for _, request := range missing {
		e.subscriptions.markBasicQot(request.canonical)
	}
	return nil
}

func subscribeBasicQot(ctx context.Context, client *opend.Client, securities []*qotcommonpb.Security) error {
	// Intentionally omit IsRegOrUnRegPush: per Qot_Sub.proto, "该参数不指定不做
	// 注册反注册操作" — leaving it unset preserves any push registration the
	// stream layer has already installed on this OpenD connection. Sending
	// `false` here would explicitly toggle push state and could silently
	// unregister Qot_UpdateBasicQot pushes for these securities.
	request := &qotsubpb.Request{C2S: &qotsubpb.C2S{
		SecurityList: securities,
		SubTypeList:  []int32{int32(qotcommonpb.SubType_SubType_Basic)},
		IsSubOrUnSub: proto.Bool(true),
	}}
	var response qotsubpb.Response
	if err := client.Call(ctx, opend.ProtoQotSub, request, &response); err != nil {
		return err
	}
	if response.GetRetType() != 0 {
		return fmt.Errorf("opend Qot_Sub retType=%d errCode=%d retMsg=%s", response.GetRetType(), response.GetErrCode(), response.GetRetMsg())
	}
	return nil
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
	if keepAliveInterval := initResp.GetS2C().GetKeepAliveInterval(); keepAliveInterval > 0 {
		e.client.StartKeepAlive(time.Duration(keepAliveInterval) * time.Second)
	}
	e.subscriptions.ensure()
	e.bindSystemNotifyLocked(e.client)
	log.Printf("futu OpenD session established to %s (connect=%v init=%v total=%v)",
		e.addr, connectElapsed, time.Since(initStart), time.Since(connectStart))
	return e.client, nil
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

func futuSymbolFromSecurity(security *qotcommonpb.Security) (string, error) {
	if security == nil {
		return "", fmt.Errorf("futu exchange: security is required")
	}
	market, err := futuMarketCodeFromQotMarket(qotcommonpb.QotMarket(security.GetMarket()))
	if err != nil {
		return "", err
	}
	code := strings.TrimSpace(strings.ToUpper(security.GetCode()))
	if code == "" {
		return "", fmt.Errorf("futu exchange: security code is required")
	}
	return market + "." + code, nil
}

func futuMarketCodeFromQotMarket(market qotcommonpb.QotMarket) (string, error) {
	switch market {
	case qotcommonpb.QotMarket_QotMarket_HK_Security:
		return "HK", nil
	case qotcommonpb.QotMarket_QotMarket_US_Security:
		return "US", nil
	case qotcommonpb.QotMarket_QotMarket_CNSH_Security:
		return "SH", nil
	case qotcommonpb.QotMarket_QotMarket_CNSZ_Security:
		return "SZ", nil
	default:
		return "", fmt.Errorf("unsupported market %q", market.String())
	}
}

func futuSecurityFromSymbol(symbol string) (*qotcommonpb.Security, string, error) {
	trimmed := strings.TrimSpace(strings.ToUpper(symbol))
	if trimmed == "" {
		return nil, "", fmt.Errorf("futu exchange: symbol is required")
	}
	separator := "."
	if strings.Contains(trimmed, ":") {
		separator = ":"
	}
	parts := strings.SplitN(trimmed, separator, 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return nil, "", fmt.Errorf("futu exchange: symbol %q must be in MARKET.CODE form", symbol)
	}
	qotMarket, err := futuQotMarketForCode(parts[0])
	if err != nil {
		return nil, "", err
	}
	canonical := parts[0] + "." + parts[1]
	return &qotcommonpb.Security{Market: proto.Int32(int32(qotMarket)), Code: proto.String(parts[1])}, canonical, nil
}

func futuQotMarketForCode(market string) (qotcommonpb.QotMarket, error) {
	switch strings.ToUpper(strings.TrimSpace(market)) {
	case "HK":
		return qotcommonpb.QotMarket_QotMarket_HK_Security, nil
	case "US":
		return qotcommonpb.QotMarket_QotMarket_US_Security, nil
	case "SH", "CNSH":
		return qotcommonpb.QotMarket_QotMarket_CNSH_Security, nil
	case "SZ", "CNSZ":
		return qotcommonpb.QotMarket_QotMarket_CNSZ_Security, nil
	default:
		return qotcommonpb.QotMarket_QotMarket_Unknown, fmt.Errorf("unsupported market %q", market)
	}
}
