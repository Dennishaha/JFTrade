package futu

import (
	"time"

	"github.com/c9s/bbgo/pkg/fixedpoint"
	"github.com/c9s/bbgo/pkg/types"
	"github.com/shopspring/decimal"

	qotcommonpb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotcommon"
	"github.com/jftrade/jftrade-main/pkg/market"
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
	QuoteTime  string
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
	Session            market.Session
	ExtendedHours      bool
	PreMarket          *ExtendedMarketQuote
	AfterMarket        *ExtendedMarketQuote
	Overnight          *ExtendedMarketQuote
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
	lastPrice := fixedpointFromDecimal(snapshot.Price)
	resolvedAt := futuQuoteTime(basicQot.GetUpdateTimestamp(), basicQot.GetUpdateTime(), canonical)
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
	return quoteSnapshotFromBasicQotAt(basicQot, canonical, time.Now().UTC())
}

func quoteSnapshotFromBasicQotAt(basicQot *qotcommonpb.BasicQot, canonical string, now time.Time) *QuoteSnapshot {
	quoteAt := futuQuoteTimeAt(basicQot.GetUpdateTimestamp(), basicQot.GetUpdateTime(), canonical, now)
	quoteTime := quoteAt.Format(time.RFC3339Nano)
	preMarket := extendedMarketQuoteFromProto(basicQot.GetPreMarket(), quoteTime)
	afterMarket := extendedMarketQuoteFromProto(basicQot.GetAfterMarket(), quoteTime)
	overnight := extendedMarketQuoteFromProto(basicQot.GetOvernight(), quoteTime)
	session := sessionFromExtendedBlocksAt(canonical, preMarket, afterMarket, overnight, now)
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
	if market.ShouldUseRegularCloseAsPreviousClose(canonical, session, regularSessionClose) {
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
		QuoteAt:            quoteAt,
		Session:            session,
		ExtendedHours:      market.IsExtendedSession(session),
		PreMarket:          preMarket,
		AfterMarket:        afterMarket,
		Overnight:          overnight,
	}
}

func extendedMarketQuoteFromProto(data *qotcommonpb.PreAfterMarketData, quoteTime string) *ExtendedMarketQuote {
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
		QuoteTime:  quoteTime,
	}
}

func activeExtendedQuoteForSession(session market.Session, preMarket *ExtendedMarketQuote, afterMarket *ExtendedMarketQuote, overnight *ExtendedMarketQuote) *ExtendedMarketQuote {
	switch session {
	case market.SessionPre:
		return preMarket
	case market.SessionAfter:
		return afterMarket
	case market.SessionOvernight:
		return overnight
	default:
		return nil
	}
}

func cloneInt64AsFloat64(value *int64) *float64 {
	if value == nil {
		return nil
	}
	return new(float64(*value))
}

func sessionFromExtendedBlocksAt(canonical string, preMarket, afterMarket, overnight *ExtendedMarketQuote, now time.Time) market.Session {
	clockSession := market.ClassifySession(canonical, now)
	switch clockSession {
	case market.SessionOvernight:
		if overnight != nil && decimalPositive(overnight.Price) {
			return market.SessionOvernight
		}
	case market.SessionAfter:
		if afterMarket != nil && decimalPositive(afterMarket.Price) {
			return market.SessionAfter
		}
	case market.SessionPre:
		if preMarket != nil && decimalPositive(preMarket.Price) {
			return market.SessionPre
		}
	}
	return clockSession
}

func futuQuoteTime(timestamp float64, fallback string, symbol string) time.Time {
	return futuQuoteTimeAt(timestamp, fallback, symbol, time.Now().UTC())
}

func futuQuoteTimeAt(timestamp float64, fallback string, symbol string, recordedAt time.Time) time.Time {
	if timestamp > 0 {
		seconds := int64(timestamp)
		nanos := int64((timestamp - float64(seconds)) * 1e9)
		return time.Unix(seconds, nanos).UTC()
	}
	loc := time.UTC
	if profile, ok := market.ProfileForSymbol(symbol); ok && profile.Location != nil {
		loc = profile.Location
	}
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02 15:04:05", "2006-01-02"} {
		parsed, err := time.ParseInLocation(layout, fallback, loc)
		if err == nil {
			return parsed.UTC()
		}
	}
	if recordedAt.IsZero() {
		recordedAt = time.Now().UTC()
	}
	return recordedAt.UTC()
}
