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
	"time"

	"github.com/c9s/bbgo/pkg/exchange"
	"github.com/c9s/bbgo/pkg/fixedpoint"
	"github.com/c9s/bbgo/pkg/types"
	"google.golang.org/protobuf/proto"

	"github.com/jftrade/jftrade-main/pkg/futu/opend"
	initpb "github.com/jftrade/jftrade-main/pkg/futu/pb/initconnect"
	qotcommonpb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotcommon"
	qotgetbasicqotpb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotgetbasicqot"
	historypb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotrequesthistorykl"
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

	mu                        sync.Mutex
	client                    *opend.Client
	ready                     bool
	basicQotSubscriptions     map[string]struct{}
	basicQotPushSubscriptions map[string]struct{}
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
		addr:                      cfg.Addr,
		webSocketKey:              cfg.WebSocketKey,
		basicQotSubscriptions:     map[string]struct{}{},
		basicQotPushSubscriptions: map[string]struct{}{},
	}
}

// Client exposes the underlying OpenD client for advanced (non-bbgo) callers.
func (e *Exchange) Client() *opend.Client {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.client
}

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

func (e *Exchange) QueryKLines(ctx context.Context, symbol string, interval types.Interval, options types.KLineQueryOptions) ([]types.KLine, error) {
	security, canonicalSymbol, err := futuSecurityFromSymbol(symbol)
	if err != nil {
		return nil, err
	}
	klType, err := futuKLTypeFromInterval(interval)
	if err != nil {
		return nil, err
	}
	beginAt, endAt, limit := futuKLineQueryWindow(interval, options)
	request := &historypb.Request{C2S: &historypb.C2S{
		RehabType:   proto.Int32(int32(qotcommonpb.RehabType_RehabType_None)),
		KlType:      proto.Int32(int32(klType)),
		Security:    security,
		BeginTime:   proto.String(beginAt.Format("2006-01-02 15:04:05")),
		EndTime:     proto.String(endAt.Format("2006-01-02 15:04:05")),
		MaxAckKLNum: proto.Int32(int32(limit)),
	}}
	var response historypb.Response
	if err := e.callProto(ctx, opend.ProtoRequestHistoryKL, request, &response); err != nil {
		return nil, err
	}
	if response.GetRetType() != 0 {
		return nil, fmt.Errorf("opend RequestHistoryKL retType=%d errCode=%d retMsg=%s", response.GetRetType(), response.GetErrCode(), response.GetRetMsg())
	}

	klines := make([]types.KLine, 0, len(response.GetS2C().GetKlList()))
	for _, candle := range response.GetS2C().GetKlList() {
		if candle.GetIsBlank() {
			continue
		}
		klines = append(klines, futuKLineFromProto(candle, canonicalSymbol, interval))
	}
	return klines, nil
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

	request := &qotgetbasicqotpb.Request{C2S: &qotgetbasicqotpb.C2S{SecurityList: securityList}}
	var response qotgetbasicqotpb.Response
	if err := e.withClient(ctx, func(client *opend.Client) error {
		if err := e.ensureBasicQotSubscriptions(ctx, client, requests); err != nil {
			return err
		}
		return client.Call(ctx, opend.ProtoGetBasicQot, request, &response)
	}); err != nil {
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
		if _, exists := e.basicQotSubscriptions[request.canonical]; exists {
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
		e.basicQotSubscriptions[request.canonical] = struct{}{}
	}
	return nil
}

func subscribeBasicQot(ctx context.Context, client *opend.Client, securities []*qotcommonpb.Security) error {
	request := &qotsubpb.Request{C2S: &qotsubpb.C2S{
		SecurityList:     securities,
		SubTypeList:      []int32{int32(qotcommonpb.SubType_SubType_Basic)},
		IsSubOrUnSub:     proto.Bool(true),
		IsRegOrUnRegPush: proto.Bool(false),
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

func tickerFromBasicQot(basicQot *qotcommonpb.BasicQot) *types.Ticker {
	if basicQot == nil {
		return nil
	}
	lastPrice := fixedpoint.NewFromFloat(basicQot.GetCurPrice())
	resolvedAt := futuQuoteTime(basicQot.GetUpdateTimestamp(), basicQot.GetUpdateTime())
	return &types.Ticker{
		Time:   resolvedAt,
		Volume: fixedpoint.NewFromFloat(float64(basicQot.GetVolume())),
		Last:   lastPrice,
		Open:   fixedpoint.NewFromFloat(basicQot.GetOpenPrice()),
		High:   fixedpoint.NewFromFloat(basicQot.GetHighPrice()),
		Low:    fixedpoint.NewFromFloat(basicQot.GetLowPrice()),
		Buy:    lastPrice,
		Sell:   lastPrice,
	}
}

func (e *Exchange) ensureClient(ctx context.Context) (*opend.Client, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.client == nil {
		e.client = e.newClient()
	}
	if e.ready {
		return e.client, nil
	}
	if err := e.client.Connect(ctx); err != nil {
		_ = e.invalidateClientLocked()
		return nil, err
	}

	initReq := &initpb.Request{C2S: &initpb.C2S{
		ClientVer:           proto.Int32(101),
		ClientID:            proto.String("jftrade-bbgo"),
		RecvNotify:          proto.Bool(false),
		ProgrammingLanguage: proto.String("Go"),
	}}
	var initResp initpb.Response
	if err := e.client.Call(ctx, opend.ProtoInitConnect, initReq, &initResp); err != nil {
		_ = e.invalidateClientLocked()
		return nil, err
	}
	if initResp.GetRetType() != 0 {
		err := fmt.Errorf("opend InitConnect retType=%d errCode=%d retMsg=%s", initResp.GetRetType(), initResp.GetErrCode(), initResp.GetRetMsg())
		_ = e.invalidateClientLocked()
		return nil, err
	}

	e.ready = true
	if e.basicQotSubscriptions == nil {
		e.basicQotSubscriptions = map[string]struct{}{}
	}
	if e.basicQotPushSubscriptions == nil {
		e.basicQotPushSubscriptions = map[string]struct{}{}
	}
	return e.client, nil
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
	_ = e.invalidateClientLocked()
}

func (e *Exchange) invalidateClientLocked() error {
	client := e.client
	e.client = nil
	e.ready = false
	e.basicQotSubscriptions = map[string]struct{}{}
	e.basicQotPushSubscriptions = map[string]struct{}{}
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

func futuKLTypeFromInterval(interval types.Interval) (qotcommonpb.KLType, error) {
	switch interval {
	case types.Interval1m:
		return qotcommonpb.KLType_KLType_1Min, nil
	case types.Interval3m:
		return qotcommonpb.KLType_KLType_3Min, nil
	case types.Interval5m:
		return qotcommonpb.KLType_KLType_5Min, nil
	case types.Interval15m:
		return qotcommonpb.KLType_KLType_15Min, nil
	case types.Interval30m:
		return qotcommonpb.KLType_KLType_30Min, nil
	case types.Interval1h:
		return qotcommonpb.KLType_KLType_60Min, nil
	case types.Interval1d:
		return qotcommonpb.KLType_KLType_Day, nil
	case types.Interval1w:
		return qotcommonpb.KLType_KLType_Week, nil
	case types.Interval1mo:
		return qotcommonpb.KLType_KLType_Month, nil
	default:
		return qotcommonpb.KLType_KLType_Unknown, fmt.Errorf("futu exchange: unsupported interval %q", interval)
	}
}

func futuKLineQueryWindow(interval types.Interval, options types.KLineQueryOptions) (time.Time, time.Time, int) {
	limit := options.Limit
	if limit < 1 {
		limit = 200
	}
	if limit > 1000 {
		limit = 1000
	}
	endAt := time.Now()
	if options.EndTime != nil {
		endAt = *options.EndTime
	}
	lookback := interval.Duration() * time.Duration(limit) * 4
	minimumLookback := 36 * time.Hour
	if interval.Duration() >= 24*time.Hour {
		minimumLookback = 45 * 24 * time.Hour
	}
	if lookback < minimumLookback {
		lookback = minimumLookback
	}
	beginAt := endAt.Add(-lookback)
	if options.StartTime != nil {
		beginAt = *options.StartTime
	}
	if !beginAt.Before(endAt) {
		beginAt = endAt.Add(-lookback)
	}
	return beginAt, endAt, limit
}

func futuKLineFromProto(candle *qotcommonpb.KLine, symbol string, interval types.Interval) types.KLine {
	startAt := futuQuoteTime(candle.GetTimestamp(), candle.GetTime()).UTC()
	endAt := startAt.Add(interval.Duration()).Add(-time.Millisecond)
	if endAt.Before(startAt) {
		endAt = startAt
	}
	return types.KLine{
		Exchange:    Name,
		Symbol:      symbol,
		StartTime:   types.Time(startAt),
		EndTime:     types.Time(endAt),
		Interval:    interval,
		Open:        fixedpoint.NewFromFloat(candle.GetOpenPrice()),
		Close:       fixedpoint.NewFromFloat(candle.GetClosePrice()),
		High:        fixedpoint.NewFromFloat(candle.GetHighPrice()),
		Low:         fixedpoint.NewFromFloat(candle.GetLowPrice()),
		Volume:      fixedpoint.NewFromFloat(float64(candle.GetVolume())),
		QuoteVolume: fixedpoint.NewFromFloat(candle.GetTurnover()),
		Closed:      !endAt.After(time.Now().UTC()),
	}
}

func futuQuoteTime(timestamp float64, fallback string) time.Time {
	if timestamp > 0 {
		seconds := int64(timestamp)
		nanos := int64((timestamp - float64(seconds)) * 1e9)
		return time.Unix(seconds, nanos)
	}
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02 15:04:05", "2006-01-02"} {
		parsed, err := time.ParseInLocation(layout, fallback, time.Local)
		if err == nil {
			return parsed
		}
	}
	return time.Now()
}
