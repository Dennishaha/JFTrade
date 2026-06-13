// Package futu plugs a Futu OpenD-backed exchange into bbgo.
//
// Importing this package (typically via a blank import in main) registers
// "futu" as a bbgo exchange via pkg/exchange.Register, without modifying bbgo.
package futu

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/c9s/bbgo/pkg/exchange"
	"github.com/c9s/bbgo/pkg/fixedpoint"
	"github.com/c9s/bbgo/pkg/types"
	"google.golang.org/protobuf/proto"

	"github.com/jftrade/jftrade-main/pkg/futu/opend"
	notifypb "github.com/jftrade/jftrade-main/pkg/futu/pb/notify"
	qotcommonpb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotcommon"
	"github.com/jftrade/jftrade-main/pkg/market"
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

	mu                      sync.Mutex
	sessionMu               sync.RWMutex
	client                  *opend.Client
	ready                   bool
	subscriptions           subscriptionRegistry
	systemNotifyClient      *opend.Client
	systemNotifyHandlers    []func(*notifypb.Response)
	orderBookNotifyClient   *opend.Client
	orderBookNotifyHandlers map[uint64]func(string)
	nextOrderBookHandlerID  uint64
	klineSessions           map[string]klineSessionRecord
	marketSessionSamples    map[string][]marketSessionSample

	// customMarkets holds market info for symbols that are not natively
	// returned by QueryMarkets but should be known to the exchange — e.g.
	// backtest symbols that the live OpenD connection hasn't discovered.
	customMarkets types.MarketMap

	marginRatioCacheMu sync.RWMutex
	marginRatioCache   map[string]marginRatioCacheEntry
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
		addr:             cfg.Addr,
		webSocketKey:     cfg.WebSocketKey,
		subscriptions:    newSubscriptionRegistry(),
		marginRatioCache: make(map[string]marginRatioCacheEntry),
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

// OnOrderBookUpdate registers a handler for OpenD Qot_UpdateOrderBook pushes.
// Handlers survive client reconnects and are rebound automatically.
func (e *Exchange) OnOrderBookUpdate(fn func(string)) func() {
	if fn == nil {
		return func() {}
	}

	e.mu.Lock()
	if e.orderBookNotifyHandlers == nil {
		e.orderBookNotifyHandlers = make(map[uint64]func(string))
	}
	e.nextOrderBookHandlerID++
	handlerID := e.nextOrderBookHandlerID
	e.orderBookNotifyHandlers[handlerID] = fn
	e.bindOrderBookNotifyLocked(e.client)
	e.mu.Unlock()

	return func() {
		e.mu.Lock()
		defer e.mu.Unlock()
		delete(e.orderBookNotifyHandlers, handlerID)
	}
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
	if instrument, err := market.ParseInstrument(market.InstrumentInput{Symbol: sym}); err == nil {
		m.Symbol = instrument.Symbol
		m.LocalSymbol = instrument.Symbol
		sym = instrument.Symbol
	}
	if profile, ok := market.ProfileForSymbol(sym); ok {
		m.QuoteCurrency = profile.QuoteCurrency
		m.BaseCurrency = sym
		m.PricePrecision = profile.PricePrecision
		m.QuotePrecision = profile.QuotePrecision
		m.TickSize = fixedpoint.NewFromFloat(profile.TickSize)
		return m
	}
	m.QuoteCurrency = "HKD"
	m.BaseCurrency = sym
	m.PricePrecision = 3
	m.QuotePrecision = 3
	m.TickSize = fixedpoint.NewFromFloat(0.001)
	return m
}

// --- bbgo types.ExchangeAccountService ---

func (e *Exchange) QueryAccount(ctx context.Context) (*types.Account, error) {
	return e.queryAccount(ctx)
}

func (e *Exchange) QueryAccountBalances(ctx context.Context) (types.BalanceMap, error) {
	return e.queryAccountBalances(ctx)
}

// --- bbgo types.ExchangeTradeService ---

func (e *Exchange) SubmitOrder(ctx context.Context, order types.SubmitOrder) (*types.Order, error) {
	return e.submitOrder(ctx, order)
}

func (e *Exchange) QueryOpenOrders(ctx context.Context, symbol string) ([]types.Order, error) {
	return e.queryOpenOrders(ctx, symbol)
}

func (e *Exchange) CancelOrders(ctx context.Context, orders ...types.Order) error {
	return e.cancelOrders(ctx, orders...)
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
	instrument, err := market.ParseInstrument(market.InstrumentInput{Symbol: symbol})
	if err != nil {
		if strings.TrimSpace(symbol) == "" {
			return nil, "", fmt.Errorf("futu exchange: symbol is required")
		}
		return nil, "", err
	}
	if instrument.Symbol == "" || instrument.Prefix == "" || instrument.Code == "" {
		return nil, "", fmt.Errorf("futu exchange: symbol is required")
	}
	qotMarket, err := futuQotMarketForCode(instrument.Prefix)
	if err != nil {
		return nil, "", err
	}
	return &qotcommonpb.Security{Market: proto.Int32(int32(qotMarket)), Code: proto.String(instrument.Code)}, instrument.Symbol, nil
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
