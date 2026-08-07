package marketdata

import "github.com/shopspring/decimal"

type InstrumentDTO struct {
	Market       string
	Symbol       string
	InstrumentID string
}

func (instrument InstrumentDTO) JSON() map[string]any {
	return map[string]any{
		"market":       instrument.Market,
		"symbol":       instrument.Symbol,
		"instrumentId": instrument.InstrumentID,
	}
}

type SnapshotResponseDTO struct {
	Instrument InstrumentDTO
	Snapshot   map[string]any
	Source     string
	ResolvedAt string
	FromCache  bool
}

func (response SnapshotResponseDTO) JSON() MarketSnapshot {
	return MarketSnapshot{
		"request":  response.Instrument.JSON(),
		"snapshot": response.Snapshot,
		"meta": map[string]any{
			"instrumentId": response.Instrument.InstrumentID,
			"source":       response.Source,
			"resolvedAt":   response.ResolvedAt,
			"fromCache":    response.FromCache,
		},
	}
}

type CandlesResponseDTO struct {
	Instrument     InstrumentDTO
	Period         string
	Limit          int
	Candles        []map[string]any
	Pagination     CandlePagination
	Source         string
	ResolvedAt     string
	FromCache      bool
	ExtendedHours  bool
	IncludeSession bool
	Sessions       []CandleSession
}

// CandlePagination keeps historical-page progress tied to the exact oldest
// returned candle. Its zero value represents a terminal page.
type CandlePagination struct {
	HasMore    bool
	NextBefore string
}

func (response CandlesResponseDTO) JSON() CandlesResponse {
	meta := map[string]any{
		"instrumentId":  response.Instrument.InstrumentID,
		"source":        response.Source,
		"resolvedAt":    response.ResolvedAt,
		"fromCache":     response.FromCache,
		"extendedHours": response.ExtendedHours,
		"sessions":      CandleSessionStrings(response.Sessions),
	}
	if response.IncludeSession {
		session := "regular"
		if response.ExtendedHours {
			session = "all"
		}
		meta["session"] = session
	}
	pagination := map[string]any{"hasMore": response.Pagination.HasMore}
	if response.Pagination.HasMore && response.Pagination.NextBefore != "" {
		pagination["nextBefore"] = response.Pagination.NextBefore
	}
	return CandlesResponse{
		"request": map[string]any{
			"instrument": response.Instrument.JSON(),
			"period":     response.Period,
			"limit":      response.Limit,
			"sessions":   CandleSessionStrings(response.Sessions),
		},
		"candles":       response.Candles,
		"totalReturned": len(response.Candles),
		"pagination":    pagination,
		"meta":          meta,
	}
}

type TickEventDTO struct {
	Instrument       InstrumentDTO
	Snapshot         map[string]any
	ObservedAt       string
	BrokerID         string
	Source           string
	CumulativeVolume *decimal.Decimal
	VolumeDelta      decimal.Decimal
}

func (event TickEventDTO) JSON() map[string]any {
	return map[string]any{
		"type":       "market-data.tick",
		"at":         event.ObservedAt,
		"brokerId":   event.BrokerID,
		"instrument": event.Instrument.JSON(),
		"snapshot":   event.Snapshot,
		"source":     event.Source,
		// Keep snapshot.volume as the compatibility field while making both
		// volume semantics explicit for live-event consumers.
		"cumulativeVolume": optionalDecimalString(event.CumulativeVolume),
		"volumeDelta":      event.VolumeDelta.String(),
	}
}
