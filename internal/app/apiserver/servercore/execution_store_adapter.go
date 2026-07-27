package servercore

import (
	"encoding/json"
	"strings"

	tradingstore "github.com/jftrade/jftrade-main/internal/store/trading"
	trdsrv "github.com/jftrade/jftrade-main/internal/trading"
)

// executionOrderStore remains an assembly-local alias while the ledger,
// SQLite schema and maintenance implementation live in internal/store/trading.
type executionOrderStore = tradingstore.Resource

func newExecutionOrderStore() executionOrderStore {
	return tradingstore.NewInMemory()
}

func newExecutionOrderStoreWithDB(dbPath string) (executionOrderStore, error) {
	return tradingstore.New(dbPath)
}

func deriveExecutionOrderDBPath(settingsPath string) string {
	return tradingstore.DerivePath(settingsPath)
}

func marshalExecutionPayload(payload any) string {
	if payload == nil {
		return "{}"
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return "{}"
	}
	return string(encoded)
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func executionStringPointerOrNil(value string) *string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return &value
}

func derefString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func executionFillLookupKey(
	brokerID, accountID, tradingEnvironment, market, brokerFillID string,
	brokerFillIDEx *string,
) string {
	fillID := strings.TrimSpace(brokerFillID)
	if fillID == "" {
		fillID = strings.TrimSpace(derefString(brokerFillIDEx))
	}
	if fillID == "" {
		return ""
	}
	return strings.Join([]string{
		strings.ToUpper(strings.TrimSpace(brokerID)),
		strings.ToUpper(strings.TrimSpace(tradingEnvironment)),
		strings.TrimSpace(accountID),
		strings.ToUpper(strings.TrimSpace(market)),
		fillID,
	}, "|")
}

func canonicalPlacedRecordStatus(status string) string {
	status = strings.TrimSpace(status)
	if status == trdsrv.OrderStatusSubmissionUnknown || status == trdsrv.OrderStatusSubmitting {
		return status
	}
	return trdsrv.CanonicalBrokerOrderStatus(status)
}
