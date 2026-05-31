package futu

import (
	"strings"
	"time"

	"github.com/c9s/bbgo/pkg/fixedpoint"
	"github.com/c9s/bbgo/pkg/types"
	"github.com/shopspring/decimal"

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
	Price      *decimal.Decimal
	HighPrice  *decimal.Decimal
	LowPrice   *decimal.Decimal
	Volume     *float64
	Turnover   *decimal.Decimal
	ChangeVal  *decimal.Decimal
	ChangeRate *decimal.Decimal
	Amplitude  *decimal.Decimal
}

// QuoteSnapshot preserves extended-session fields that do not fit into bbgo's
// generic Ticker model.
type QuoteSnapshot struct {
	Symbol             string
	Price              decimal.Decimal
	Bid                decimal.Decimal
	Ask                decimal.Decimal
	OpenPrice          *decimal.Decimal
	HighPrice          *decimal.Decimal
	LowPrice           *decimal.Decimal
	PreviousClosePrice *decimal.Decimal
	LastClosePrice     *decimal.Decimal
	Volume             float64
	Turnover           decimal.Decimal
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
	lastPrice := fixedpointFromDecimal(snapshot.Price)
	resolvedAt := futuQuoteTime(basicQot.GetUpdateTimestamp(), basicQot.GetUpdateTime())
	return &types.Ticker{
		Time:   resolvedAt,
		Volume: fixedpoint.NewFromFloat(snapshot.Volume),
		Last:   lastPrice,
		Open:   fixedpointFromDecimalPtr(snapshot.OpenPrice),
		High:   fixedpointFromDecimalPtr(snapshot.HighPrice),
		Low:    fixedpointFromDecimalPtr(snapshot.LowPrice),
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

	regularSessionClose := decimalFromFloat64(basicQot.GetCurPrice())

	price := regularSessionClose
	highPrice := decimalPtrFromFloat64(basicQot.HighPrice)
	lowPrice := decimalPtrFromFloat64(basicQot.LowPrice)
	volume := float64(basicQot.GetVolume())
	turnover := decimalFromFloat64(basicQot.GetTurnover())
	if activeExtended != nil {
		if decimalPositive(activeExtended.Price) {
			price = *activeExtended.Price
		}
		if decimalPositive(activeExtended.HighPrice) {
			highPrice = activeExtended.HighPrice
		}
		if decimalPositive(activeExtended.LowPrice) {
			lowPrice = activeExtended.LowPrice
		}
		if activeExtended.Volume != nil {
			volume = *activeExtended.Volume
		}
		if activeExtended.Turnover != nil {
			turnover = *activeExtended.Turnover
		}
	}

	prevClosePrice := decimalPtrFromFloat64(basicQot.LastClosePrice)
	if session != MarketSessionRegular && regularSessionClose.GreaterThan(decimal.Zero) {
		prevClosePrice = &regularSessionClose
	}

	return &QuoteSnapshot{
		Symbol:             canonical,
		Price:              price,
		Bid:                price,
		Ask:                price,
		OpenPrice:          decimalPtrFromFloat64(basicQot.OpenPrice),
		HighPrice:          highPrice,
		LowPrice:           lowPrice,
		PreviousClosePrice: prevClosePrice,
		LastClosePrice:     decimalPtrFromFloat64(basicQot.LastClosePrice),
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
		Price:      decimalPtrFromFloat64(data.Price),
		HighPrice:  decimalPtrFromFloat64(data.HighPrice),
		LowPrice:   decimalPtrFromFloat64(data.LowPrice),
		Volume:     cloneInt64AsFloat64(data.Volume),
		Turnover:   decimalPtrFromFloat64(data.Turnover),
		ChangeVal:  decimalPtrFromFloat64(data.ChangeVal),
		ChangeRate: decimalPtrFromFloat64(data.ChangeRate),
		Amplitude:  decimalPtrFromFloat64(data.Amplitude),
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

func cloneInt64AsFloat64(value *int64) *float64 {
	if value == nil {
		return nil
	}
	clone := float64(*value)
	return &clone
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
		if overnight != nil && decimalPositive(overnight.Price) {
			return MarketSessionOvernight
		}
	case MarketSessionAfter:
		if afterMarket != nil && decimalPositive(afterMarket.Price) {
			return MarketSessionAfter
		}
	case MarketSessionPre:
		if preMarket != nil && decimalPositive(preMarket.Price) {
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
