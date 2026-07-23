package broker

// ResearchInstrumentEntry is the canonical identity and quote projection used
// by research lists. Adapter-specific fields may coexist beside these JSON
// fields in FeatureResult entries for backward compatibility.
type ResearchInstrumentEntry struct {
	InstrumentID string       `json:"instrumentId,omitempty"`
	Market       string       `json:"market,omitempty"`
	Symbol       string       `json:"symbol,omitempty"`
	Name         string       `json:"name,omitempty"`
	ProductClass ProductClass `json:"productClass,omitempty"`
	Price        *float64     `json:"price,omitempty"`
	ChangeRate   *float64     `json:"changeRate,omitempty"`
}

// ResearchPlateEntry is the canonical board/sector projection.
type ResearchPlateEntry struct {
	ResearchInstrumentEntry
	MarketValue *float64 `json:"marketValue,omitempty"`
	RiseCount   *int64   `json:"riseCount,omitempty"`
	FallCount   *int64   `json:"fallCount,omitempty"`
	EqualCount  *int64   `json:"equalCount,omitempty"`
	Description string   `json:"description,omitempty"`
}

// ResearchCalendarEvent normalizes the calendar families. EventTimestamp is
// Unix seconds and EventTime is an RFC3339 UTC timestamp when OpenD supplies a
// timestamp; protocol-native date/time fields remain available alongside it.
type ResearchCalendarEvent struct {
	ResearchInstrumentEntry
	CalendarType   string   `json:"calendarType"`
	Title          string   `json:"title,omitempty"`
	Region         string   `json:"region,omitempty"`
	Importance     *int64   `json:"importance,omitempty"`
	PreviousValue  string   `json:"previousValue,omitempty"`
	ForecastValue  string   `json:"forecastValue,omitempty"`
	ActualValue    string   `json:"actualValue,omitempty"`
	EventTimestamp *float64 `json:"eventTimestamp,omitempty"`
	EventDate      string   `json:"eventDate,omitempty"`
	EventTime      string   `json:"eventTime,omitempty"`
}

// ResearchIpoEntry adds canonical issue fields to a calendar event.
type ResearchIpoEntry struct {
	ResearchCalendarEvent
	ListingDate   string   `json:"listingDate,omitempty"`
	IssueVolume   *int64   `json:"issueVolume,omitempty"`
	IssuePrice    *float64 `json:"issuePrice,omitempty"`
	IssuePriceMin *float64 `json:"issuePriceMin,omitempty"`
	IssuePriceMax *float64 `json:"issuePriceMax,omitempty"`
}

// ResearchInstitutionEntry is the stable list projection for institutions.
type ResearchInstitutionEntry struct {
	InstitutionID      *int64   `json:"institutionId,omitempty"`
	Name               string   `json:"name,omitempty"`
	MarketValue        *float64 `json:"marketValue,omitempty"`
	MarketValueChange  *float64 `json:"marketValueChange,omitempty"`
	HoldingCount       *int64   `json:"holdingCount,omitempty"`
	HoldingCountChange *int64   `json:"holdingCountChange,omitempty"`
	AsOfDate           string   `json:"asOfDate,omitempty"`
}
