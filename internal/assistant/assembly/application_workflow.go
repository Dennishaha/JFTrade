package assembly

import "strings"

func splitWorkflowInstrumentID(instrumentID string) (string, string, bool) {
	parts := strings.SplitN(strings.TrimSpace(instrumentID), ".", 2)
	if len(parts) != 2 {
		return "", "", false
	}
	market := strings.ToUpper(strings.TrimSpace(parts[0]))
	symbol := strings.ToUpper(strings.TrimSpace(parts[1]))
	if market == "" || symbol == "" {
		return "", "", false
	}
	return market, symbol, true
}
