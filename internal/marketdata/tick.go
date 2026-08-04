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

// QuoteFieldAvailability makes nullable upstream quote fields explicit without
// weakening the decimal invariants used by the cache and collector. Providers
// that leave Authoritative false retain the legacy behavior where all four
// decimal fields are available, including a legitimate zero value.
type QuoteFieldAvailability struct {
	Authoritative bool
	Bid           bool
	Ask           bool
	Volume        bool
	Turnover      bool
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
	Availability  QuoteFieldAvailability
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
