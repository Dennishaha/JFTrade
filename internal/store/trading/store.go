// Package trading owns the durable execution ledger and its SQLite schema.
package trading

import (
	"context"
	"fmt"
	"strings"

	trdsrv "github.com/jftrade/jftrade-main/internal/trading"
	"github.com/jftrade/jftrade-main/pkg/broker"
)

// New opens the execution ledger at dbPath and loads its durable state.
func New(dbPath string) (Resource, error) {
	return openStore(dbPath)
}

func openStore(dbPath string) (*Store, error) {
	return newExecutionOrderStoreWithDB(dbPath)
}

// NewInMemory returns an available in-process ledger without persistence. It is
// used when the on-disk database is incompatible so the application can still
// start and offer its existing rebuild flow.
func NewInMemory() Resource {
	return newExecutionOrderStore()
}

// DerivePath resolves the execution database next to the settings file unless
// an explicit environment override is configured.
func DerivePath(settingsPath string) string {
	return deriveExecutionOrderDBPath(settingsPath)
}

// Available reports whether this ledger has durable SQLite persistence.
func (s *Store) Available() bool {
	return s != nil && s.persistence != nil && s.persistence.db != nil
}

// ListOrders implements trading.OrderStore.
func (s *Store) ListOrders(_ context.Context, filter trdsrv.ExecutionOrderFilter) (trdsrv.ExecutionOrders, error) {
	if s == nil {
		return trdsrv.ExecutionOrders{}, trdsrv.ErrOrderStoreUnavailable
	}
	return s.listOrdersFiltered(filter), nil
}

// AllOrders returns the complete cloned ledger ordered by recency.
func (s *Store) AllOrders() trdsrv.ExecutionOrders {
	if s == nil {
		return trdsrv.ExecutionOrders{}
	}
	return s.listOrders()
}

// FilteredOrders returns a cloned ledger view for in-process gateways.
func (s *Store) FilteredOrders(filter trdsrv.ExecutionOrderFilter) trdsrv.ExecutionOrders {
	if s == nil {
		return trdsrv.ExecutionOrders{}
	}
	return s.listOrdersFiltered(filter)
}

// OrderEvents implements trading.OrderStore.
func (s *Store) OrderEvents(_ context.Context, internalOrderID string) (trdsrv.ExecutionOrderEvents, error) {
	if s == nil {
		return trdsrv.ExecutionOrderEvents{}, trdsrv.ErrOrderStoreUnavailable
	}
	return s.orderEvents(strings.TrimSpace(internalOrderID)), nil
}

// Events returns cloned events for an in-process gateway.
func (s *Store) Events(internalOrderID string) trdsrv.ExecutionOrderEvents {
	if s == nil {
		return trdsrv.ExecutionOrderEvents{}
	}
	return s.orderEvents(strings.TrimSpace(internalOrderID))
}

// Order returns a cloned ledger entry.
func (s *Store) Order(internalOrderID string) (trdsrv.ExecutionOrder, bool) {
	if s == nil {
		return trdsrv.ExecutionOrder{}, false
	}
	return s.order(strings.TrimSpace(internalOrderID))
}

// PrepareSubmission durably reserves a client order before broker submission.
func (s *Store) PrepareSubmission(input trdsrv.ExecutionPlacedOrderRecord) (trdsrv.ExecutionOrder, bool, error) {
	if s == nil {
		return trdsrv.ExecutionOrder{}, false, trdsrv.ErrOrderStoreUnavailable
	}
	return s.prepareSubmission(input)
}

// MarkSubmissionUnknown records an indeterminate broker submission result.
func (s *Store) MarkSubmissionUnknown(internalOrderID string, submitErr error) trdsrv.ExecutionOrder {
	if s == nil {
		return trdsrv.ExecutionOrder{}
	}
	return s.markSubmissionUnknown(internalOrderID, submitErr)
}

// RecordPlacedOrder merges an accepted broker order into the ledger.
func (s *Store) RecordPlacedOrder(input trdsrv.ExecutionPlacedOrderRecord) trdsrv.ExecutionOrder {
	if s == nil {
		return trdsrv.ExecutionOrder{}
	}
	return s.recordPlacedOrder(input)
}

// MarkCancelRequested records an accepted cancel command.
func (s *Store) MarkCancelRequested(internalOrderID string, payload any) (trdsrv.ExecutionOrder, bool) {
	if s == nil {
		return trdsrv.ExecutionOrder{}, false
	}
	return s.markCancelRequested(internalOrderID, payload)
}

// ApplyBrokerOrder reconciles a broker snapshot with the durable ledger.
func (s *Store) ApplyBrokerOrder(
	brokerID string,
	snapshot broker.OrderSnapshot,
	discoveredEventType string,
	updatedEventType string,
	source string,
	sourceDetail string,
) (trdsrv.ExecutionOrder, *trdsrv.ExecutionOrderEvent, bool) {
	if s == nil {
		return trdsrv.ExecutionOrder{}, nil, false
	}
	return s.upsertBrokerOrderWithSource(
		brokerID, snapshot, discoveredEventType, updatedEventType, source, sourceDetail,
	)
}

// ApplyBrokerFill reconciles a broker fill with the durable ledger.
func (s *Store) ApplyBrokerFill(
	brokerID string,
	fill broker.OrderFillSnapshot,
) (trdsrv.ExecutionOrder, *trdsrv.ExecutionOrderEvent, bool) {
	if s == nil {
		return trdsrv.ExecutionOrder{}, nil, false
	}
	return s.recordBrokerOrderFill(brokerID, fill)
}

// ApplyBrokerFee records broker-reported aggregate order fees.
func (s *Store) ApplyBrokerFee(
	brokerID string,
	fee broker.OrderFeeSnapshot,
) (trdsrv.ExecutionOrder, *trdsrv.ExecutionOrderEvent, bool) {
	if s == nil {
		return trdsrv.ExecutionOrder{}, nil, false
	}
	return s.recordBrokerOrderFee(brokerID, fee)
}

// SavePreview implements trading.ExecutionPreviewStore. An in-memory degraded
// store keeps the historical no-op behavior for preview persistence.
func (s *Store) SavePreview(record trdsrv.ExecutionPreviewRecord) error {
	if !s.Available() {
		return nil
	}
	s.databaseMu.Lock()
	defer s.databaseMu.Unlock()
	return s.persistence.savePreview(record)
}

// ConsumePreview implements trading.ExecutionPreviewStore.
func (s *Store) ConsumePreview(previewID, brokerID, accountID, requestHash, clientOrderID string) error {
	if !s.Available() {
		return nil
	}
	s.databaseMu.Lock()
	defer s.databaseMu.Unlock()
	return s.persistence.consumePreview(previewID, brokerID, accountID, requestHash, clientOrderID)
}

// SavePredictionQuote implements broker.PredictionQuoteStore.
func (s *Store) SavePredictionQuote(_ context.Context, record broker.PredictionQuoteRecord) error {
	if !s.Available() {
		return fmt.Errorf("prediction quote persistence is unavailable")
	}
	s.databaseMu.Lock()
	defer s.databaseMu.Unlock()
	return s.persistence.savePredictionQuote(record)
}

// ValidatePredictionQuote implements broker.PredictionQuoteStore.
func (s *Store) ValidatePredictionQuote(
	_ context.Context,
	quoteID, brokerID, accountID, environment, mvc, legsHash string,
) (broker.PredictionQuoteRecord, error) {
	if !s.Available() {
		return broker.PredictionQuoteRecord{}, fmt.Errorf("prediction quote persistence is unavailable")
	}
	s.databaseMu.Lock()
	defer s.databaseMu.Unlock()
	return s.persistence.predictionQuote(quoteID, brokerID, accountID, environment, mvc, legsHash)
}

// ConsumePredictionQuote implements broker.PredictionQuoteStore.
func (s *Store) ConsumePredictionQuote(
	_ context.Context,
	quoteID, brokerID, accountID, environment, mvc, legsHash, previewID, clientOrderID string,
) error {
	if !s.Available() {
		return fmt.Errorf("prediction quote persistence is unavailable")
	}
	s.databaseMu.Lock()
	defer s.databaseMu.Unlock()
	return s.persistence.consumePredictionQuote(
		quoteID, brokerID, accountID, environment, mvc, legsHash, previewID, clientOrderID,
	)
}

// ConfigureSeenFillRetention applies bounded fill-deduplication retention.
func (s *Store) ConfigureSeenFillRetention(days int) {
	if s != nil {
		s.configureSeenFillRetention(days)
	}
}

// SeenFillRetentionDays exposes the effective setting for status and tests.
func (s *Store) SeenFillRetentionDays() int {
	if s == nil {
		return 0
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.seenFillRetentionDays
}

// HasSeenFill reports whether a broker fill key is in the deduplication ledger.
func (s *Store) HasSeenFill(fillKey string) bool {
	if s == nil {
		return false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	_, ok := s.seenFillKeys[strings.TrimSpace(fillKey)]
	return ok
}

var (
	_ trdsrv.OrderStore            = (*Store)(nil)
	_ trdsrv.ExecutionPreviewStore = (*Store)(nil)
	_ broker.PredictionQuoteStore  = (*Store)(nil)
)
