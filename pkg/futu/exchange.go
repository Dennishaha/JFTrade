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
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/c9s/bbgo/pkg/exchange"
	"github.com/c9s/bbgo/pkg/fixedpoint"
	"github.com/c9s/bbgo/pkg/types"
	"google.golang.org/protobuf/proto"

	"github.com/jftrade/jftrade-main/pkg/futu/opend"
	commonpb "github.com/jftrade/jftrade-main/pkg/futu/pb/common"
	initpb "github.com/jftrade/jftrade-main/pkg/futu/pb/initconnect"
	qotcommonpb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotcommon"
	qotgetbasicqotpb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotgetbasicqot"
	qotgetklpb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotgetkl"
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

const maxHistoryKLinePages = 8

// ErrNotSupported is returned for bbgo Exchange methods that do not map
// cleanly to the Futu trading domain (e.g. spot taker/maker fee math).
var ErrNotSupported = errors.New("futu exchange: operation not supported")

// MarketSession identifies the active US equity trading session.
type MarketSession string

const (
	MarketSessionUnknown   MarketSession = "unknown"
	MarketSessionClosed    MarketSession = "closed"
	MarketSessionPre       MarketSession = "pre"
	MarketSessionRegular   MarketSession = "regular"
	MarketSessionAfter     MarketSession = "after"
	MarketSessionOvernight MarketSession = "overnight"
)

// ExtendedMarketQuote holds a Futu BasicQot pre-market, after-hours, or
// overnight quote block.
type ExtendedMarketQuote struct {
	Price      *float64
	HighPrice  *float64
	LowPrice   *float64
	Volume     *float64
	Turnover   *float64
	ChangeVal  *float64
	ChangeRate *float64
	Amplitude  *float64
}

// QuoteSnapshot preserves extended-session fields that do not fit into bbgo's
// generic Ticker model.
type QuoteSnapshot struct {
	Symbol             string
	Price              float64
	Bid                float64
	Ask                float64
	OpenPrice          *float64
	HighPrice          *float64
	LowPrice           *float64
	PreviousClosePrice *float64
	LastClosePrice     *float64 // 历史收盘：始终 = GetLastClosePrice()（上个交易日收盘）
	Volume             float64
	Turnover           float64
	QuoteAt            time.Time
	Session            MarketSession
	ExtendedHours      bool
	PreMarket          *ExtendedMarketQuote
	AfterMarket        *ExtendedMarketQuote
	Overnight          *ExtendedMarketQuote
}

var usEasternLocation = func() *time.Location {
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		return time.UTC
	}
	return loc
}()

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
	klineSubscriptions        map[string]struct{}
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
		klineSubscriptions:        map[string]struct{}{},
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
	return quoteSnapshotFromBasicQot(basicQot, canonical), nil
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
	klines := make([]types.KLine, 0, limit)
	nextReqKey := []byte(nil)
	for page := 0; page < maxHistoryKLinePages; page++ {
		request := &historypb.Request{C2S: &historypb.C2S{
			RehabType:   proto.Int32(int32(qotcommonpb.RehabType_RehabType_None)),
			KlType:      proto.Int32(int32(klType)),
			Security:    security,
			BeginTime:   proto.String(beginAt.Format("2006-01-02 15:04:05")),
			EndTime:     proto.String(endAt.Format("2006-01-02 15:04:05")),
			MaxAckKLNum: proto.Int32(int32(limit)),
		}}
		if len(nextReqKey) > 0 {
			request.C2S.NextReqKey = nextReqKey
		}
		if shouldRequestExtendedKLines(canonicalSymbol, interval) {
			request.C2S.ExtendedTime = proto.Bool(true)
			request.C2S.Session = proto.Int32(int32(commonpb.Session_Session_ALL))
		}

		var response historypb.Response
		if err := e.callProto(ctx, opend.ProtoRequestHistoryKL, request, &response); err != nil {
			return nil, err
		}
		if response.GetRetType() != 0 {
			return nil, fmt.Errorf("opend RequestHistoryKL retType=%d errCode=%d retMsg=%s", response.GetRetType(), response.GetErrCode(), response.GetRetMsg())
		}

		for _, candle := range response.GetS2C().GetKlList() {
			if candle.GetIsBlank() {
				continue
			}
			klines = append(klines, futuKLineFromProto(candle, canonicalSymbol, interval))
		}

		nextReqKey = response.GetS2C().GetNextReqKey()
		if len(nextReqKey) == 0 {
			break
		}
		if page == maxHistoryKLinePages-1 {
			return nil, fmt.Errorf("opend RequestHistoryKL pagination exceeded %d pages", maxHistoryKLinePages)
		}
	}
	if shouldQueryCurrentKLine(interval, endAt) {
		currentKLines, err := e.queryCurrentKLines(ctx, security, canonicalSymbol, interval, klType)
		if err == nil {
			klines = mergeKLinesByStartTime(klines, filterKLinesByWindow(currentKLines, beginAt, endAt))
		}
	}
	sort.Slice(klines, func(i, j int) bool {
		return klines[i].StartTime.Time().Before(klines[j].StartTime.Time())
	})
	if len(klines) > limit {
		klines = klines[len(klines)-limit:]
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

type klineSubscriptionRequest struct {
	canonical    string
	security     *qotcommonpb.Security
	subType      qotcommonpb.SubType
	extendedTime bool
	session      commonpb.Session
}

func (request klineSubscriptionRequest) cacheKey() string {
	return fmt.Sprintf("%s:%d:%t:%d", request.canonical, request.subType, request.extendedTime, request.session)
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

func (e *Exchange) queryCurrentKLines(ctx context.Context, security *qotcommonpb.Security, canonicalSymbol string, interval types.Interval, klType qotcommonpb.KLType) ([]types.KLine, error) {
	subType, err := futuSubTypeFromInterval(interval)
	if err != nil {
		return nil, err
	}

	subscription := klineSubscriptionRequest{
		canonical: canonicalSymbol,
		security:  security,
		subType:   subType,
	}
	if shouldRequestExtendedKLines(canonicalSymbol, interval) {
		subscription.extendedTime = true
		subscription.session = commonpb.Session_Session_ALL
	}

	request := &qotgetklpb.Request{C2S: &qotgetklpb.C2S{
		RehabType: proto.Int32(int32(qotcommonpb.RehabType_RehabType_None)),
		KlType:    proto.Int32(int32(klType)),
		Security:  security,
		ReqNum:    proto.Int32(2),
	}}

	var response qotgetklpb.Response
	if err := e.withClient(ctx, func(client *opend.Client) error {
		if err := e.ensureKLineSubscription(ctx, client, subscription); err != nil {
			return err
		}
		return client.Call(ctx, opend.ProtoGetKL, request, &response)
	}); err != nil {
		return nil, err
	}
	if response.GetRetType() != 0 {
		return nil, fmt.Errorf("opend GetKL retType=%d errCode=%d retMsg=%s", response.GetRetType(), response.GetErrCode(), response.GetRetMsg())
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

func (e *Exchange) ensureKLineSubscription(ctx context.Context, client *opend.Client, request klineSubscriptionRequest) error {
	cacheKey := request.cacheKey()

	e.mu.Lock()
	_, exists := e.klineSubscriptions[cacheKey]
	e.mu.Unlock()
	if exists {
		return nil
	}

	if err := subscribeKLine(ctx, client, request); err != nil {
		return err
	}

	e.mu.Lock()
	defer e.mu.Unlock()
	if e.klineSubscriptions == nil {
		e.klineSubscriptions = map[string]struct{}{}
	}
	e.klineSubscriptions[cacheKey] = struct{}{}
	return nil
}

func subscribeKLine(ctx context.Context, client *opend.Client, request klineSubscriptionRequest) error {
	subscription := &qotsubpb.Request{C2S: &qotsubpb.C2S{
		SecurityList:     []*qotcommonpb.Security{request.security},
		SubTypeList:      []int32{int32(request.subType)},
		IsSubOrUnSub:     proto.Bool(true),
		IsRegOrUnRegPush: proto.Bool(false),
	}}
	if request.extendedTime {
		subscription.C2S.ExtendedTime = proto.Bool(true)
		subscription.C2S.Session = proto.Int32(int32(request.session))
	}

	var response qotsubpb.Response
	if err := client.Call(ctx, opend.ProtoQotSub, subscription, &response); err != nil {
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
	canonical, err := futuSymbolFromSecurity(basicQot.GetSecurity())
	if err != nil {
		canonical = ""
	}
	snapshot := quoteSnapshotFromBasicQot(basicQot, canonical)
	lastPrice := fixedpoint.NewFromFloat(snapshot.Price)
	resolvedAt := futuQuoteTime(basicQot.GetUpdateTimestamp(), basicQot.GetUpdateTime())
	return &types.Ticker{
		Time:   resolvedAt,
		Volume: fixedpoint.NewFromFloat(snapshot.Volume),
		Last:   lastPrice,
		Open:   fixedpoint.NewFromFloat(valueOrZero(snapshot.OpenPrice)),
		High:   fixedpoint.NewFromFloat(valueOrZero(snapshot.HighPrice)),
		Low:    fixedpoint.NewFromFloat(valueOrZero(snapshot.LowPrice)),
		Buy:    lastPrice,
		Sell:   lastPrice,
	}
}

func quoteSnapshotFromBasicQot(basicQot *qotcommonpb.BasicQot, canonical string) *QuoteSnapshot {
	preMarket := extendedMarketQuoteFromProto(basicQot.GetPreMarket())
	afterMarket := extendedMarketQuoteFromProto(basicQot.GetAfterMarket())
	overnight := extendedMarketQuoteFromProto(basicQot.GetOvernight())
	session := sessionFromExtendedBlocks(canonical, preMarket, afterMarket, overnight)
	activeExtended := activeExtendedQuoteForSession(session, preMarket, afterMarket, overnight)

	// GetCurPrice() during extended sessions holds today's regular-session
	// closing price (the last regular-session trade). Capture it before the
	// extended override so it can be used as previousClosePrice.
	regularSessionClose := basicQot.GetCurPrice()

	price := regularSessionClose
	highPrice := floatPtr(basicQot.GetHighPrice())
	lowPrice := floatPtr(basicQot.GetLowPrice())
	volume := float64(basicQot.GetVolume())
	turnover := basicQot.GetTurnover()
	if activeExtended != nil {
		if activeExtended.Price != nil && *activeExtended.Price > 0 {
			price = *activeExtended.Price
		}
		if activeExtended.HighPrice != nil && *activeExtended.HighPrice > 0 {
			highPrice = activeExtended.HighPrice
		}
		if activeExtended.LowPrice != nil && *activeExtended.LowPrice > 0 {
			lowPrice = activeExtended.LowPrice
		}
		if activeExtended.Volume != nil {
			volume = *activeExtended.Volume
		}
		if activeExtended.Turnover != nil {
			turnover = *activeExtended.Turnover
		}
	}

	// During extended sessions (pre/after/overnight), use the captured
	// regularSessionClose as previousClosePrice so the frontend "最近盘中收盘"
	// label shows today's (most recent) regular-session close, not 昨收 which
	// only carries the previous trading day's close.  During regular session
	// there is no extended override, so fall back to GetLastClosePrice().
	prevClosePrice := basicQot.GetLastClosePrice()
	if IsExtendedMarketSession(session) && regularSessionClose > 0 {
		prevClosePrice = regularSessionClose
	}

	return &QuoteSnapshot{
		Symbol:             canonical,
		Price:              price,
		Bid:                price,
		Ask:                price,
		OpenPrice:          floatPtr(basicQot.GetOpenPrice()),
		HighPrice:          highPrice,
		LowPrice:           lowPrice,
		PreviousClosePrice: floatPtr(prevClosePrice),
		LastClosePrice:     floatPtr(basicQot.GetLastClosePrice()),
		Volume:             volume,
		Turnover:           turnover,
		QuoteAt:            futuQuoteTime(basicQot.GetUpdateTimestamp(), basicQot.GetUpdateTime()).UTC(),
		Session:            session,
		ExtendedHours:      IsExtendedMarketSession(session),
		PreMarket:          preMarket,
		AfterMarket:        afterMarket,
		Overnight:          overnight,
	}
}

func extendedMarketQuoteFromProto(data *qotcommonpb.PreAfterMarketData) *ExtendedMarketQuote {
	if data == nil {
		return nil
	}
	return &ExtendedMarketQuote{
		Price:      cloneFloat64(data.Price),
		HighPrice:  cloneFloat64(data.HighPrice),
		LowPrice:   cloneFloat64(data.LowPrice),
		Volume:     cloneInt64AsFloat64(data.Volume),
		Turnover:   cloneFloat64(data.Turnover),
		ChangeVal:  cloneFloat64(data.ChangeVal),
		ChangeRate: cloneFloat64(data.ChangeRate),
		Amplitude:  cloneFloat64(data.Amplitude),
	}
}

func activeExtendedQuoteForSession(session MarketSession, preMarket *ExtendedMarketQuote, afterMarket *ExtendedMarketQuote, overnight *ExtendedMarketQuote) *ExtendedMarketQuote {
	switch session {
	case MarketSessionPre:
		return preMarket
	case MarketSessionAfter:
		return afterMarket
	case MarketSessionOvernight:
		return overnight
	default:
		return nil
	}
}

func cloneFloat64(value *float64) *float64 {
	if value == nil {
		return nil
	}
	clone := *value
	return &clone
}

func cloneInt64AsFloat64(value *int64) *float64 {
	if value == nil {
		return nil
	}
	clone := float64(*value)
	return &clone
}

func floatPtr(value float64) *float64 {
	clone := value
	return &clone
}

func valueOrZero(value *float64) float64 {
	if value == nil {
		return 0
	}
	return *value
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
			_ = e.invalidateClientLocked()
			e.client = e.newClient()
		default:
			return e.client, nil
		}
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
	if keepAliveInterval := initResp.GetS2C().GetKeepAliveInterval(); keepAliveInterval > 0 {
		e.client.StartKeepAlive(time.Duration(keepAliveInterval) * time.Second)
	}
	if e.basicQotSubscriptions == nil {
		e.basicQotSubscriptions = map[string]struct{}{}
	}
	if e.basicQotPushSubscriptions == nil {
		e.basicQotPushSubscriptions = map[string]struct{}{}
	}
	if e.klineSubscriptions == nil {
		e.klineSubscriptions = map[string]struct{}{}
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
	e.klineSubscriptions = map[string]struct{}{}
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

func futuSubTypeFromInterval(interval types.Interval) (qotcommonpb.SubType, error) {
	switch interval {
	case types.Interval1m:
		return qotcommonpb.SubType_SubType_KL_1Min, nil
	case types.Interval3m:
		return qotcommonpb.SubType_SubType_KL_3Min, nil
	case types.Interval5m:
		return qotcommonpb.SubType_SubType_KL_5Min, nil
	case types.Interval15m:
		return qotcommonpb.SubType_SubType_KL_15Min, nil
	case types.Interval30m:
		return qotcommonpb.SubType_SubType_KL_30Min, nil
	case types.Interval1h:
		return qotcommonpb.SubType_SubType_KL_60Min, nil
	case types.Interval1d:
		return qotcommonpb.SubType_SubType_KL_Day, nil
	case types.Interval1w:
		return qotcommonpb.SubType_SubType_KL_Week, nil
	case types.Interval1mo:
		return qotcommonpb.SubType_SubType_KL_Month, nil
	default:
		return qotcommonpb.SubType_SubType_None, fmt.Errorf("futu exchange: unsupported interval %q", interval)
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

func shouldQueryCurrentKLine(interval types.Interval, endAt time.Time) bool {
	duration := interval.Duration()
	if duration <= 0 {
		return false
	}
	return !endAt.Before(time.Now().UTC().Add(-duration))
}

func filterKLinesByWindow(klines []types.KLine, beginAt time.Time, endAt time.Time) []types.KLine {
	filtered := make([]types.KLine, 0, len(klines))
	for _, kline := range klines {
		startAt := kline.StartTime.Time().UTC()
		finishAt := kline.EndTime.Time().UTC()
		if finishAt.Before(beginAt) || startAt.After(endAt) {
			continue
		}
		filtered = append(filtered, kline)
	}
	return filtered
}

func mergeKLinesByStartTime(slices ...[]types.KLine) []types.KLine {
	byStartTime := make(map[int64]types.KLine)
	for _, slice := range slices {
		for _, kline := range slice {
			byStartTime[kline.StartTime.Time().UTC().UnixNano()] = kline
		}
	}
	merged := make([]types.KLine, 0, len(byStartTime))
	for _, kline := range byStartTime {
		merged = append(merged, kline)
	}
	return merged
}

func futuKLineFromProto(candle *qotcommonpb.KLine, symbol string, interval types.Interval) types.KLine {
	labelAt := futuQuoteTime(candle.GetTimestamp(), candle.GetTime()).UTC()
	startAt := futuHistoryKLineStartTime(labelAt, interval)
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

func futuHistoryKLineStartTime(labelAt time.Time, interval types.Interval) time.Time {
	duration := interval.Duration()
	if duration <= 0 || duration >= 24*time.Hour {
		return labelAt
	}

	return labelAt.Add(-duration)
}

// sessionFromExtendedBlocks derives the current market session from Futu's
// extended-data blocks rather than the wall clock.  This correctly handles
// market holidays and early-close ("half-day") sessions without an external
// holiday calendar:
//
//   - overnight block has a price  → MarketSessionOvernight
//   - after-market block has a price → MarketSessionAfter
//   - pre-market block has a price AND clock confirms pre-market window
//     (clock guard is necessary because Futu keeps pre-market data alive
//     through the subsequent regular session, so data alone is ambiguous)
//     → MarketSessionPre
//   - fallback → ClassifyMarketSession (clock-based, handles Sat/Sun/etc.)
func sessionFromExtendedBlocks(canonical string, preMarket, afterMarket, overnight *ExtendedMarketQuote) MarketSession {
	return sessionFromExtendedBlocksAt(canonical, preMarket, afterMarket, overnight, time.Now().UTC())
}

func sessionFromExtendedBlocksAt(canonical string, preMarket, afterMarket, overnight *ExtendedMarketQuote, now time.Time) MarketSession {
	// All three extended-session blocks (pre, after, overnight) can persist
	// past their actual trading window — Futu's BasicQot keeps yesterday's
	// after-market and last night's overnight prices populated well into the
	// next pre-market and regular session. Without a clock guard, that stale
	// data would freeze `activeExtended` (and therefore `snapshot.Price`) on
	// the previous session's close, blocking all downstream tick updates.
	// Treat the block as "currently active" only when the clock agrees.
	clockSession := ClassifyMarketSession(canonical, now)
	switch clockSession {
	case MarketSessionOvernight:
		if overnight != nil && overnight.Price != nil && *overnight.Price > 0 {
			return MarketSessionOvernight
		}
	case MarketSessionAfter:
		if afterMarket != nil && afterMarket.Price != nil && *afterMarket.Price > 0 {
			return MarketSessionAfter
		}
	case MarketSessionPre:
		if preMarket != nil && preMarket.Price != nil && *preMarket.Price > 0 {
			return MarketSessionPre
		}
	}
	return clockSession
}

// ClassifyMarketSession classifies US equities into regular, pre-market,
// after-hours, or overnight sessions using America/New_York clock time.
func ClassifyMarketSession(symbol string, at time.Time) MarketSession {
	if !strings.HasPrefix(strings.ToUpper(strings.TrimSpace(symbol)), "US.") {
		return MarketSessionUnknown
	}
	local := at.In(usEasternLocation)
	weekday := local.Weekday()
	minutes := local.Hour()*60 + local.Minute()

	if weekday == time.Saturday {
		return MarketSessionClosed
	}
	if weekday == time.Sunday {
		if minutes >= 20*60 {
			return MarketSessionOvernight
		}
		return MarketSessionClosed
	}
	if weekday == time.Friday && minutes >= 20*60 {
		return MarketSessionClosed
	}

	switch {
	case minutes < 4*60:
		return MarketSessionOvernight
	case minutes < 9*60+30:
		return MarketSessionPre
	case minutes < 16*60:
		return MarketSessionRegular
	case minutes < 20*60:
		return MarketSessionAfter
	default:
		return MarketSessionOvernight
	}
}

func IsExtendedMarketSession(session MarketSession) bool {
	return session == MarketSessionPre || session == MarketSessionAfter || session == MarketSessionOvernight
}

func shouldRequestExtendedKLines(symbol string, interval types.Interval) bool {
	return strings.HasPrefix(strings.ToUpper(strings.TrimSpace(symbol)), "US.") && interval.Duration() <= time.Hour
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
