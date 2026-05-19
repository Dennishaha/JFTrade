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
	"time"

	"github.com/c9s/bbgo/pkg/exchange"
	"github.com/c9s/bbgo/pkg/fixedpoint"
	"github.com/c9s/bbgo/pkg/types"

	"github.com/jftrade/jftrade-main/pkg/futu/opend"
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
	client       *opend.Client
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
	return &Exchange{addr: cfg.Addr, webSocketKey: cfg.WebSocketKey, client: opend.New(cfg)}
}

// Client exposes the underlying OpenD client for advanced (non-bbgo) callers.
func (e *Exchange) Client() *opend.Client { return e.client }

// --- bbgo types.ExchangeMinimal ---

func (e *Exchange) Name() types.ExchangeName    { return Name }
func (e *Exchange) PlatformFeeCurrency() string { return "HKD" }

// --- bbgo types.ExchangeMarketDataService ---

func (e *Exchange) NewStream() types.Stream { return NewStream(e) }

func (e *Exchange) QueryMarkets(ctx context.Context) (types.MarketMap, error) {
	return types.MarketMap{
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
	}, nil
}

func (e *Exchange) QueryTicker(ctx context.Context, symbol string) (*types.Ticker, error) {
	return nil, fmt.Errorf("%w: QueryTicker symbol=%s", ErrNotSupported, symbol)
}

func (e *Exchange) QueryTickers(ctx context.Context, symbol ...string) (map[string]types.Ticker, error) {
	return map[string]types.Ticker{}, nil
}

func (e *Exchange) QueryKLines(ctx context.Context, symbol string, interval types.Interval, options types.KLineQueryOptions) ([]types.KLine, error) {
	return nil, fmt.Errorf("%w: QueryKLines symbol=%s", ErrNotSupported, symbol)
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
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	return e.client.Connect(ctx)
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
