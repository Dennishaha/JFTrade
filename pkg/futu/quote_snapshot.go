package futu

import (
	"strings"
	"time"

	"github.com/c9s/bbgo/pkg/fixedpoint"
	"github.com/c9s/bbgo/pkg/types"

	qotcommonpb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotcommon"
)

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
	LastClosePrice     *float64
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

// sessionFromExtendedBlocks derives the current market session from Futu's
// extended-data blocks rather than the wall clock. This correctly handles
// market holidays and early-close sessions without an external holiday calendar.
func sessionFromExtendedBlocks(canonical string, preMarket, afterMarket, overnight *ExtendedMarketQuote) MarketSession {
	return sessionFromExtendedBlocksAt(canonical, preMarket, afterMarket, overnight, time.Now().UTC())
}

func sessionFromExtendedBlocksAt(canonical string, preMarket, afterMarket, overnight *ExtendedMarketQuote, now time.Time) MarketSession {
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
