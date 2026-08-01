package marketdata

import (
	"strings"
	"time"

	"github.com/shopspring/decimal"
)

const (
	TickFreshness   = 1500 * time.Millisecond
	CacheRetention  = 30 * time.Minute
	MaxCacheSamples = 30000
)

type TickKind string

const (
	TickKindQuote TickKind = "quote"
	TickKindTrade TickKind = "trade"
)

type ExtendedQuote struct {
	Price            *decimal.Decimal
	HighPrice        *decimal.Decimal
	LowPrice         *decimal.Decimal
	Volume           *decimal.Decimal
	Turnover         *decimal.Decimal
	ChangeVal        *decimal.Decimal
	ChangeRate       *decimal.Decimal
	Amplitude        *decimal.Decimal
	QuoteTime        string
	TradingDate      string
	ExchangeTimezone string
	SessionStartAt   string
	SessionEndAt     string
}

type Tick struct {
	InstrumentID       string
	Market             string
	Symbol             string
	Price              decimal.Decimal
	Bid                decimal.Decimal
	Ask                decimal.Decimal
	OpenPrice          *decimal.Decimal
	HighPrice          *decimal.Decimal
	LowPrice           *decimal.Decimal
	PreviousClosePrice *decimal.Decimal
	LastClosePrice     *decimal.Decimal
	// Volume is the provider's cumulative volume counter for the active volume
	// sequence (normally the current market session). It is never a per-event
	// quantity.
	Volume decimal.Decimal
	// VolumeDelta is the non-negative volume represented by this event. Quote
	// snapshots that do not carry an explicit delta leave it at zero.
	VolumeDelta   decimal.Decimal
	Turnover      decimal.Decimal
	QuoteAt       string
	ObservedAt    string
	Source        string
	Session       string
	ExtendedHours bool
	PreMarket     *ExtendedQuote
	AfterMarket   *ExtendedQuote
	Overnight     *ExtendedQuote
	Kind          TickKind
}

func NormalizeInstrumentID(instrumentID string) (string, string, string, bool) {
	instrumentID = strings.ToUpper(strings.TrimSpace(instrumentID))
	parts := strings.SplitN(instrumentID, ".", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", "", false
	}
	return instrumentID, parts[0], parts[1], true
}
