package marketdata

import (
	"fmt"
	"slices"
	"strings"
)

type CandleSession string

const (
	CandleSessionRegular   CandleSession = "regular"
	CandleSessionExtended  CandleSession = "extended"
	CandleSessionOvernight CandleSession = "overnight"
)

var candleSessionOrder = []CandleSession{
	CandleSessionRegular,
	CandleSessionExtended,
	CandleSessionOvernight,
}

// HistoricalCandlesQuery keeps every request dimension together so provider
// adapters cannot accidentally omit session scope during pagination.
type HistoricalCandlesQuery struct {
	Market            string
	Symbol            string
	Period            string
	Limit             int
	FromTime          string
	ToTime            string
	// BeforeTime is an exclusive historical-page cursor. It must remain
	// distinct from ToTime because upstream providers can use coarser time
	// precision than the public RFC3339 value.
	BeforeTime        string
	Sessions          []CandleSession
	SessionsSpecified bool
}

func ParseCandleSessions(values []string) ([]CandleSession, error) {
	seen := make(map[CandleSession]struct{}, len(candleSessionOrder))
	provided := false
	for _, value := range values {
		for _, token := range strings.Split(value, ",") {
			provided = true
			token = strings.TrimSpace(token)
			if token == "" {
				continue
			}
			session := CandleSession(strings.ToLower(token))
			if !slices.Contains(candleSessionOrder, session) {
				return nil, fmt.Errorf("%w: %q", ErrInvalidCandleSessions, token)
			}
			seen[session] = struct{}{}
		}
	}
	if provided && len(seen) == 0 {
		return nil, fmt.Errorf("%w: at least one session is required", ErrInvalidCandleSessions)
	}
	return orderedCandleSessions(seen), nil
}

func ResolveCandleSessions(
	requested []CandleSession,
	specified bool,
	available []CandleSession,
) ([]CandleSession, error) {
	availableSet := make(map[CandleSession]struct{}, len(available))
	for _, session := range available {
		availableSet[session] = struct{}{}
	}
	if !specified {
		return orderedCandleSessions(availableSet), nil
	}
	if len(requested) == 0 {
		return nil, fmt.Errorf("%w: at least one session is required", ErrInvalidCandleSessions)
	}
	requestedSet := make(map[CandleSession]struct{}, len(requested))
	for _, session := range requested {
		if _, ok := availableSet[session]; !ok {
			return nil, fmt.Errorf("%w: session %q is unsupported", ErrInvalidCandleSessions, session)
		}
		requestedSet[session] = struct{}{}
	}
	return orderedCandleSessions(requestedSet), nil
}

func CandleSessionStrings(sessions []CandleSession) []string {
	values := make([]string, 0, len(sessions))
	for _, session := range sessions {
		values = append(values, string(session))
	}
	return values
}

func ContainsCandleSession(sessions []CandleSession, wanted CandleSession) bool {
	return slices.Contains(sessions, wanted)
}

func CandleSessionForLabel(label string) CandleSession {
	switch strings.ToLower(strings.TrimSpace(label)) {
	case "regular":
		return CandleSessionRegular
	case "pre", "after", "extended":
		return CandleSessionExtended
	case "overnight":
		return CandleSessionOvernight
	default:
		return ""
	}
}

func FilterCandlesBySessions(
	candles []map[string]any,
	sessions []CandleSession,
) []map[string]any {
	if len(sessions) == 0 {
		return candles
	}
	filtered := make([]map[string]any, 0, len(candles))
	for _, candle := range candles {
		label, _ := candle["session"].(string)
		group := CandleSessionForLabel(label)
		if group == "" {
			group = CandleSessionRegular
		}
		if ContainsCandleSession(sessions, group) {
			filtered = append(filtered, candle)
		}
	}
	return filtered
}

func orderedCandleSessions(set map[CandleSession]struct{}) []CandleSession {
	result := make([]CandleSession, 0, len(set))
	for _, session := range candleSessionOrder {
		if _, ok := set[session]; ok {
			result = append(result, session)
		}
	}
	return result
}
