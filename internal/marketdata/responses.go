package marketdata

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
	Source         string
	ResolvedAt     string
	FromCache      bool
	ExtendedHours  bool
	IncludeSession bool
}

func (response CandlesResponseDTO) JSON() CandlesResponse {
	meta := map[string]any{
		"instrumentId":  response.Instrument.InstrumentID,
		"source":        response.Source,
		"resolvedAt":    response.ResolvedAt,
		"fromCache":     response.FromCache,
		"extendedHours": response.ExtendedHours,
	}
	if response.IncludeSession {
		session := "regular"
		if response.ExtendedHours {
			session = "all"
		}
		meta["session"] = session
	}
	return CandlesResponse{
		"request": map[string]any{
			"instrument": response.Instrument.JSON(),
			"period":     response.Period,
			"limit":      response.Limit,
		},
		"candles":       response.Candles,
		"totalReturned": len(response.Candles),
		"meta":          meta,
	}
}

type TickEventDTO struct {
	Instrument InstrumentDTO
	Snapshot   map[string]any
	ObservedAt string
	BrokerID   string
	Source     string
}

func (event TickEventDTO) JSON() map[string]any {
	return map[string]any{
		"type":       "market-data.tick",
		"at":         event.ObservedAt,
		"brokerId":   event.BrokerID,
		"instrument": event.Instrument.JSON(),
		"snapshot":   event.Snapshot,
		"source":     event.Source,
	}
}
