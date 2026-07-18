package servercore

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	trdsrv "github.com/jftrade/jftrade-main/internal/trading"
	"github.com/jftrade/jftrade-main/pkg/broker"
)

func (s *executionOrderStore) loadFromDB() error {
	if s == nil || s.persistence == nil {
		return nil
	}
	orders, err := s.persistence.loadOrders()
	if err != nil {
		return err
	}
	legs, err := s.persistence.loadOrderLegs()
	if err != nil {
		return err
	}
	for index := range orders {
		orders[index].Legs = legs[orders[index].InternalOrderID]
	}
	events, err := s.persistence.loadEvents()
	if err != nil {
		return err
	}
	fillKeys, err := s.persistence.loadSeenFillKeys()
	if err != nil {
		return err
	}
	sequences, err := s.persistence.loadSequences()
	if err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	for _, order := range orders {
		s.orders[order.InternalOrderID] = order
		s.linkBrokerOrderLocked(order)
		if seq := executionSequenceSuffix(order.InternalOrderID, "exec-"); seq > s.nextOrderSeq {
			s.nextOrderSeq = seq
		}
	}
	for _, event := range events {
		s.events[event.InternalOrderID] = append(s.events[event.InternalOrderID], event)
		if seq := executionSequenceSuffix(event.ID, "evt-"); seq > s.nextEventSeq {
			s.nextEventSeq = seq
		}
	}
	for _, row := range fillKeys {
		s.seenFillKeys[row.FillKey] = row.CreatedAt
	}
	if seq := sequences["orders"]; seq > s.nextOrderSeq {
		s.nextOrderSeq = seq
	}
	if seq := sequences["events"]; seq > s.nextEventSeq {
		s.nextEventSeq = seq
	}
	return nil
}

func (s *executionOrderSQLiteStore) loadOrders() ([]executionOrderSummaryResponse, error) {
	rows := []executionOrderSummaryRow{}
	if err := s.db.Select(&rows,
		`SELECT internal_order_id, broker_id, broker_order_id, broker_order_id_ex, source, source_detail, trading_environment, account_id, market, symbol, side, order_type, status, raw_broker_status, requested_quantity, requested_price, filled_quantity, filled_average_price, remark, last_error, last_error_code, last_error_source, submitted_at, updated_at, created_at, order_kind, product_class, quantity_mode, client_order_id, preview_id, normalized_request, requested_amount, payout, fees FROM `+
			executionOrderTable); err != nil {
		return nil, err
	}
	orders := make([]executionOrderSummaryResponse, 0, len(rows))
	for _, row := range rows {
		orders = append(orders, executionOrderSummaryFromRow(row))
	}
	return orders, nil
}

func (s *executionOrderSQLiteStore) loadOrderLegs() (map[string][]trdsrv.ExecutionOrderLeg, error) {
	rows := []executionOrderLegRow{}
	if err := s.db.Select(&rows,
		`SELECT id, internal_order_id, leg_index, broker_leg_id, instrument_id, product_class, side, ratio, prediction_side, requested_quantity, requested_amount, requested_price, status, filled_quantity, filled_amount, average_price, fees, payout, updated_at, created_at FROM `+
			executionOrderLegTable+` ORDER BY internal_order_id ASC, leg_index ASC`); err != nil {
		return nil, err
	}
	result := make(map[string][]trdsrv.ExecutionOrderLeg)
	for _, row := range rows {
		leg := executionOrderLegFromRow(row)
		result[leg.InternalOrderID] = append(result[leg.InternalOrderID], leg)
	}
	return result, nil
}

func (s *executionOrderSQLiteStore) loadEvents() ([]executionOrderEventResponse, error) {
	rows := []executionOrderEventRow{}
	if err := s.db.Select(&rows,
		`SELECT id, internal_order_id, event_type, previous_status, next_status, payload_json, created_at FROM `+
			executionOrderEventTable+` ORDER BY created_at ASC, id ASC`); err != nil {
		return nil, err
	}
	events := make([]executionOrderEventResponse, 0, len(rows))
	for _, row := range rows {
		events = append(events, executionOrderEventFromRow(row))
	}
	return events, nil
}

func (s *executionOrderSQLiteStore) loadSeenFillKeys() ([]executionSeenFillRow, error) {
	keys := []executionSeenFillRow{}
	if err := s.db.Select(&keys, `SELECT fill_key, created_at FROM `+executionSeenFillTable); err != nil {
		return nil, err
	}
	return keys, nil
}

func (s *executionOrderSQLiteStore) loadSequences() (map[string]uint64, error) {
	rows := []executionSequenceRow{}
	if err := s.db.Select(&rows, `SELECT name, value FROM `+executionSequenceTable); err != nil {
		return nil, err
	}
	result := make(map[string]uint64, len(rows))
	for _, row := range rows {
		result[row.Name] = row.Value
	}
	return result, nil
}

func (s *executionOrderSQLiteStore) persistOrder(order executionOrderSummaryResponse) error {
	row := executionOrderSummaryToRow(order)
	_, err := s.db.NamedExec(
		`INSERT INTO `+executionOrderTable+` (internal_order_id, broker_id, broker_order_id, broker_order_id_ex, source, source_detail, trading_environment, account_id, market, symbol, side, order_type, status, raw_broker_status, requested_quantity, requested_price, filled_quantity, filled_average_price, remark, last_error, last_error_code, last_error_source, submitted_at, updated_at, created_at, order_kind, product_class, quantity_mode, client_order_id, preview_id, normalized_request, requested_amount, payout, fees) `+
			`VALUES (:internal_order_id, :broker_id, :broker_order_id, :broker_order_id_ex, :source, :source_detail, :trading_environment, :account_id, :market, :symbol, :side, :order_type, :status, :raw_broker_status, :requested_quantity, :requested_price, :filled_quantity, :filled_average_price, :remark, :last_error, :last_error_code, :last_error_source, :submitted_at, :updated_at, :created_at, :order_kind, :product_class, :quantity_mode, :client_order_id, :preview_id, :normalized_request, :requested_amount, :payout, :fees) `+
			`ON CONFLICT(internal_order_id) DO UPDATE SET broker_id = excluded.broker_id, broker_order_id = excluded.broker_order_id, broker_order_id_ex = excluded.broker_order_id_ex, source = excluded.source, source_detail = excluded.source_detail, trading_environment = excluded.trading_environment, account_id = excluded.account_id, market = excluded.market, symbol = excluded.symbol, side = excluded.side, order_type = excluded.order_type, status = excluded.status, raw_broker_status = excluded.raw_broker_status, requested_quantity = excluded.requested_quantity, requested_price = excluded.requested_price, filled_quantity = excluded.filled_quantity, filled_average_price = excluded.filled_average_price, remark = excluded.remark, last_error = excluded.last_error, last_error_code = excluded.last_error_code, last_error_source = excluded.last_error_source, submitted_at = excluded.submitted_at, updated_at = excluded.updated_at, created_at = excluded.created_at, order_kind = excluded.order_kind, product_class = excluded.product_class, quantity_mode = excluded.quantity_mode, client_order_id = excluded.client_order_id, preview_id = excluded.preview_id, normalized_request = excluded.normalized_request, requested_amount = excluded.requested_amount, payout = excluded.payout, fees = excluded.fees`,
		row,
	)
	if err != nil {
		return err
	}
	return s.persistOrderLegs(order.InternalOrderID, order.Legs)
}

func (s *executionOrderSQLiteStore) persistOrderLegs(internalOrderID string, legs []trdsrv.ExecutionOrderLeg) error {
	if _, err := s.db.ExecContext(context.Background(),
		`DELETE FROM `+executionOrderLegTable+` WHERE internal_order_id = ?`, internalOrderID); err != nil {
		return err
	}
	for _, leg := range legs {
		if _, err := s.db.NamedExec(
			`INSERT INTO `+executionOrderLegTable+` (id, internal_order_id, leg_index, broker_leg_id, instrument_id, product_class, side, ratio, prediction_side, requested_quantity, requested_amount, requested_price, status, filled_quantity, filled_amount, average_price, fees, payout, updated_at, created_at) `+
				`VALUES (:id, :internal_order_id, :leg_index, :broker_leg_id, :instrument_id, :product_class, :side, :ratio, :prediction_side, :requested_quantity, :requested_amount, :requested_price, :status, :filled_quantity, :filled_amount, :average_price, :fees, :payout, :updated_at, :created_at)`,
			executionOrderLegToRow(leg),
		); err != nil {
			return err
		}
	}
	return nil
}

func (s *executionOrderSQLiteStore) persistEvent(event executionOrderEventResponse) error {
	row := executionOrderEventToRow(event)
	_, err := s.db.NamedExec(
		`INSERT INTO `+executionOrderEventTable+` (id, internal_order_id, event_type, previous_status, next_status, payload_json, created_at) `+
			`VALUES (:id, :internal_order_id, :event_type, :previous_status, :next_status, :payload_json, :created_at) `+
			`ON CONFLICT(id) DO UPDATE SET internal_order_id = excluded.internal_order_id, event_type = excluded.event_type, previous_status = excluded.previous_status, next_status = excluded.next_status, payload_json = excluded.payload_json, created_at = excluded.created_at`,
		row,
	)
	return err
}

func (s *executionOrderSQLiteStore) savePreview(record trdsrv.ExecutionPreviewRecord) error {
	row := executionOrderPreviewRow{
		PreviewID: record.PreviewID, RequestHash: record.RequestHash, BrokerID: record.BrokerID,
		CapabilityVersion: record.CapabilityVersion, AccountID: record.AccountID,
		ExpiresAt: record.ExpiresAt, QuoteExpiresAt: stringPointerNull(stringPointerOrNil(record.QuoteExpiresAt)),
		RFQID: stringPointerNull(stringPointerOrNil(record.RFQID)), NormalizedRequest: record.NormalizedRequest,
		CreatedAt: record.CreatedAt,
	}
	_, err := s.db.NamedExec(
		`INSERT INTO `+executionOrderPreviewTable+` (preview_id, request_hash, broker_id, capability_version, account_id, expires_at, quote_expires_at, rfq_id, normalized_request, created_at, consumed_at) `+
			`VALUES (:preview_id, :request_hash, :broker_id, :capability_version, :account_id, :expires_at, :quote_expires_at, :rfq_id, :normalized_request, :created_at, :consumed_at) `+
			`ON CONFLICT(preview_id) DO UPDATE SET request_hash = excluded.request_hash, broker_id = excluded.broker_id, capability_version = excluded.capability_version, account_id = excluded.account_id, expires_at = excluded.expires_at, quote_expires_at = excluded.quote_expires_at, rfq_id = excluded.rfq_id, normalized_request = excluded.normalized_request, created_at = excluded.created_at, consumed_at = NULL`,
		row,
	)
	return err
}

func (s *executionOrderSQLiteStore) savePredictionQuote(
	record broker.PredictionQuoteRecord,
) error {
	row := executionPredictionQuoteRow{
		QuoteID: record.QuoteID, BrokerID: record.BrokerID,
		AccountID: record.AccountID, TradingEnvironment: record.TradingEnvironment,
		MVC: record.MVC, LegsHash: record.LegsHash,
		BidPrice:     float64PointerNull(record.BidPrice),
		AskPrice:     float64PointerNull(record.AskPrice),
		ShouldRetry:  record.ShouldRetry,
		ReceivedAt:   record.ReceivedAt.UTC().Format(time.RFC3339Nano),
		ExpiresAt:    record.ExpiresAt.UTC().Format(time.RFC3339Nano),
		ExpirySource: record.ExpirySource, Status: firstNonEmptyString(record.Status, "active"),
	}
	_, err := s.db.NamedExec(
		`INSERT INTO `+executionPredictionQuoteTable+` (`+
			`quote_id, broker_id, account_id, trading_environment, mvc, legs_hash, bid_price, ask_price, should_retry, received_at, expires_at, expiry_source, status, consumed_at, consumed_preview_id, consumed_client_order_id) `+
			`VALUES (:quote_id, :broker_id, :account_id, :trading_environment, :mvc, :legs_hash, :bid_price, :ask_price, :should_retry, :received_at, :expires_at, :expiry_source, :status, :consumed_at, :consumed_preview_id, :consumed_client_order_id)`,
		row,
	)
	return err
}

func (s *executionOrderSQLiteStore) predictionQuote(
	quoteID, brokerID, accountID, environment, mvc, legsHash string,
) (broker.PredictionQuoteRecord, error) {
	var row executionPredictionQuoteRow
	err := s.db.Get(&row,
		`SELECT quote_id, broker_id, account_id, trading_environment, mvc, legs_hash, bid_price, ask_price, should_retry, received_at, expires_at, expiry_source, status, consumed_at, consumed_preview_id, consumed_client_order_id FROM `+
			executionPredictionQuoteTable+` WHERE quote_id = ? LIMIT 1`,
		strings.TrimSpace(quoteID),
	)
	if errors.Is(err, sql.ErrNoRows) {
		return broker.PredictionQuoteRecord{}, fmt.Errorf("prediction RFQ not found")
	}
	if err != nil {
		return broker.PredictionQuoteRecord{}, err
	}
	if !strings.EqualFold(row.BrokerID, strings.TrimSpace(brokerID)) ||
		row.AccountID != strings.TrimSpace(accountID) ||
		!strings.EqualFold(row.TradingEnvironment, strings.TrimSpace(environment)) ||
		row.MVC != strings.TrimSpace(mvc) || row.LegsHash != strings.TrimSpace(legsHash) {
		return broker.PredictionQuoteRecord{}, fmt.Errorf("prediction RFQ broker, account, environment, MVC, or legs changed")
	}
	receivedAt, receivedErr := time.Parse(time.RFC3339Nano, row.ReceivedAt)
	expiresAt, expiresErr := time.Parse(time.RFC3339Nano, row.ExpiresAt)
	if receivedErr != nil || expiresErr != nil {
		return broker.PredictionQuoteRecord{}, fmt.Errorf("prediction RFQ timestamps are invalid")
	}
	record := broker.PredictionQuoteRecord{
		QuoteID: row.QuoteID, BrokerID: row.BrokerID, AccountID: row.AccountID,
		TradingEnvironment: row.TradingEnvironment, MVC: row.MVC, LegsHash: row.LegsHash,
		BidPrice: nullFloat64Pointer(row.BidPrice), AskPrice: nullFloat64Pointer(row.AskPrice),
		ShouldRetry: row.ShouldRetry, ReceivedAt: receivedAt, ExpiresAt: expiresAt,
		ExpirySource: row.ExpirySource, Status: row.Status,
		ConsumedPreviewID: row.ConsumedPreviewID.String,
		ConsumedClientID:  row.ConsumedClientID.String,
	}
	if row.ConsumedAt.Valid {
		consumedAt, parseErr := time.Parse(time.RFC3339Nano, row.ConsumedAt.String)
		if parseErr == nil {
			record.ConsumedAt = &consumedAt
		}
	}
	if !time.Now().UTC().Before(record.ExpiresAt) {
		return broker.PredictionQuoteRecord{}, fmt.Errorf("prediction RFQ expired; request a new quote")
	}
	return record, nil
}

func (s *executionOrderSQLiteStore) consumePredictionQuote(
	quoteID, brokerID, accountID, environment, mvc, legsHash, previewID, clientOrderID string,
) error {
	record, err := s.predictionQuote(quoteID, brokerID, accountID, environment, mvc, legsHash)
	if err != nil {
		return err
	}
	if record.Status == "consumed" {
		if record.ConsumedPreviewID == previewID && record.ConsumedClientID == clientOrderID {
			return nil
		}
		return fmt.Errorf("prediction RFQ already consumed")
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	result, err := s.db.ExecContext(context.Background(),
		`UPDATE `+executionPredictionQuoteTable+` SET status = 'consumed', consumed_at = ?, consumed_preview_id = ?, consumed_client_order_id = ? `+
			`WHERE quote_id = ? AND status = 'active' AND expires_at > ?`,
		now, previewID, clientOrderID, quoteID, now,
	)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected != 1 {
		return fmt.Errorf("prediction RFQ expired or already consumed")
	}
	return nil
}

func (s *executionOrderSQLiteStore) consumePreview(
	previewID, brokerID, accountID, requestHash, clientOrderID string,
) error {
	if strings.TrimSpace(clientOrderID) == "" {
		return fmt.Errorf("clientOrderId is required")
	}
	var row executionOrderPreviewRow
	if err := s.db.Get(&row,
		`SELECT preview_id, request_hash, broker_id, capability_version, account_id, expires_at, quote_expires_at, rfq_id, normalized_request, created_at, consumed_at FROM `+
			executionOrderPreviewTable+` WHERE preview_id = ? LIMIT 1`, previewID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("preview not found")
		}
		return err
	}
	if !strings.EqualFold(row.BrokerID, brokerID) || row.AccountID != accountID {
		return fmt.Errorf("preview broker or account changed")
	}
	if row.CapabilityVersion != broker.BuiltinCapabilityCatalog.Version {
		return fmt.Errorf("capability version changed")
	}
	if row.RequestHash != requestHash {
		return fmt.Errorf("preview request changed")
	}
	if row.ConsumedAt.Valid {
		// The request hash includes clientOrderId. Returning success for an
		// identical replay lets the durable order ledger return the existing
		// parent order without making another broker call.
		return nil
	}
	now := time.Now().UTC()
	expiresAt, err := time.Parse(time.RFC3339Nano, row.ExpiresAt)
	if err != nil || !now.Before(expiresAt) {
		return fmt.Errorf("preview expired")
	}
	if row.QuoteExpiresAt.Valid {
		quoteExpiresAt, parseErr := time.Parse(time.RFC3339Nano, row.QuoteExpiresAt.String)
		if parseErr != nil || !now.Before(quoteExpiresAt) {
			return fmt.Errorf("broker quote expired; request a new RFQ")
		}
	}
	result, err := s.db.ExecContext(context.Background(),
		`UPDATE `+executionOrderPreviewTable+` SET consumed_at = ? WHERE preview_id = ? AND consumed_at IS NULL`,
		now.Format(time.RFC3339Nano), previewID,
	)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected != 1 {
		return fmt.Errorf("preview already consumed")
	}
	return nil
}

func (s *executionOrderSQLiteStore) persistSeenFillKey(fillKey string, createdAt string) error {
	if strings.TrimSpace(fillKey) == "" {
		return nil
	}
	_, err := s.db.ExecContext(context.Background(), `INSERT OR IGNORE INTO `+executionSeenFillTable+` (fill_key, created_at) VALUES (?, ?)`, fillKey, createdAt)
	return err
}

func (s *executionOrderSQLiteStore) persistSequence(name string, value uint64) error {
	if strings.TrimSpace(name) == "" {
		return nil
	}
	_, err := s.db.ExecContext(context.Background(),
		`INSERT INTO `+executionSequenceTable+` (name, value) VALUES (?, ?) `+
			`ON CONFLICT(name) DO UPDATE SET value = excluded.value`,
		name,
		value,
	)
	return err
}

func (s *executionOrderSQLiteStore) deleteSeenFillKeysBefore(cutoff time.Time) error {
	if cutoff.IsZero() {
		return nil
	}
	_, err := s.db.ExecContext(context.Background(), `DELETE FROM `+executionSeenFillTable+` WHERE created_at < ?`, cutoff.UTC().Format(time.RFC3339Nano))
	return err
}

func executionOrderSummaryFromRow(row executionOrderSummaryRow) executionOrderSummaryResponse {
	return executionOrderSummaryResponse{
		InternalOrderID:    row.InternalOrderID,
		BrokerID:           row.BrokerID,
		BrokerOrderID:      nullStringPointer(row.BrokerOrderID),
		BrokerOrderIDEx:    nullStringPointer(row.BrokerOrderIDEx),
		Source:             row.Source,
		SourceDetail:       row.SourceDetail,
		TradingEnvironment: row.TradingEnvironment,
		AccountID:          row.AccountID,
		Market:             row.Market,
		Symbol:             nullStringPointer(row.Symbol),
		Side:               nullStringPointer(row.Side),
		OrderType:          nullStringPointer(row.OrderType),
		Status:             trdsrv.CanonicalStoredOrderStatus(row.Status),
		RawBrokerStatus:    nullStringPointer(row.RawBrokerStatus),
		RequestedQuantity:  nullFloat64Pointer(row.RequestedQuantity),
		RequestedPrice:     nullFloat64Pointer(row.RequestedPrice),
		FilledQuantity:     nullFloat64Pointer(row.FilledQuantity),
		FilledAveragePrice: nullFloat64Pointer(row.FilledAveragePrice),
		Remark:             nullStringPointer(row.Remark),
		LastError:          nullStringPointer(row.LastError),
		LastErrorCode:      nullStringPointer(row.LastErrorCode),
		LastErrorSource:    nullStringPointer(row.LastErrorSource),
		SubmittedAt:        nullStringPointer(row.SubmittedAt),
		UpdatedAt:          row.UpdatedAt,
		CreatedAt:          row.CreatedAt,
		OrderKind:          broker.OrderKind(row.OrderKind),
		ProductClass:       broker.ProductClass(row.ProductClass),
		QuantityMode:       broker.QuantityMode(row.QuantityMode),
		ClientOrderID:      nullStringPointer(row.ClientOrderID),
		PreviewID:          nullStringPointer(row.PreviewID),
		NormalizedRequest:  row.NormalizedRequest,
		RequestedAmount:    nullFloat64Pointer(row.RequestedAmount),
		Fees:               nullFloat64Pointer(row.Fees),
		Payout:             nullFloat64Pointer(row.Payout),
	}
}

func executionOrderSummaryToRow(order executionOrderSummaryResponse) executionOrderSummaryRow {
	return executionOrderSummaryRow{
		InternalOrderID:    order.InternalOrderID,
		BrokerID:           order.BrokerID,
		BrokerOrderID:      stringPointerNull(order.BrokerOrderID),
		BrokerOrderIDEx:    stringPointerNull(order.BrokerOrderIDEx),
		Source:             order.Source,
		SourceDetail:       order.SourceDetail,
		TradingEnvironment: order.TradingEnvironment,
		AccountID:          order.AccountID,
		Market:             order.Market,
		Symbol:             stringPointerNull(order.Symbol),
		Side:               stringPointerNull(order.Side),
		OrderType:          stringPointerNull(order.OrderType),
		Status:             order.Status,
		RawBrokerStatus:    stringPointerNull(order.RawBrokerStatus),
		RequestedQuantity:  float64PointerNull(order.RequestedQuantity),
		RequestedPrice:     float64PointerNull(order.RequestedPrice),
		FilledQuantity:     float64PointerNull(order.FilledQuantity),
		FilledAveragePrice: float64PointerNull(order.FilledAveragePrice),
		Remark:             stringPointerNull(order.Remark),
		LastError:          stringPointerNull(order.LastError),
		LastErrorCode:      stringPointerNull(order.LastErrorCode),
		LastErrorSource:    stringPointerNull(order.LastErrorSource),
		SubmittedAt:        stringPointerNull(order.SubmittedAt),
		UpdatedAt:          order.UpdatedAt,
		CreatedAt:          order.CreatedAt,
		OrderKind:          string(order.OrderKind),
		ProductClass:       string(order.ProductClass),
		QuantityMode:       string(order.QuantityMode),
		ClientOrderID:      stringPointerNull(order.ClientOrderID),
		PreviewID:          stringPointerNull(order.PreviewID),
		NormalizedRequest:  order.NormalizedRequest,
		RequestedAmount:    float64PointerNull(order.RequestedAmount),
		Fees:               float64PointerNull(order.Fees),
		Payout:             float64PointerNull(order.Payout),
	}
}

func executionOrderLegFromRow(row executionOrderLegRow) trdsrv.ExecutionOrderLeg {
	return trdsrv.ExecutionOrderLeg{
		ID: row.ID, InternalOrderID: row.InternalOrderID, Index: row.LegIndex,
		BrokerLegID: nullStringPointer(row.BrokerLegID), InstrumentID: row.InstrumentID,
		ProductClass: broker.ProductClass(row.ProductClass), Side: row.Side, Ratio: row.Ratio,
		PredictionSide: row.PredictionSide, RequestedQuantity: nullFloat64Pointer(row.RequestedQuantity),
		RequestedAmount: nullFloat64Pointer(row.RequestedAmount), RequestedPrice: nullFloat64Pointer(row.RequestedPrice),
		Status: trdsrv.CanonicalStoredOrderStatus(row.Status), FilledQuantity: nullFloat64Pointer(row.FilledQuantity),
		FilledAmount: nullFloat64Pointer(row.FilledAmount), AveragePrice: nullFloat64Pointer(row.AveragePrice),
		Fees: nullFloat64Pointer(row.Fees), Payout: nullFloat64Pointer(row.Payout),
		UpdatedAt: row.UpdatedAt, CreatedAt: row.CreatedAt,
	}
}

func executionOrderLegToRow(leg trdsrv.ExecutionOrderLeg) executionOrderLegRow {
	return executionOrderLegRow{
		ID: leg.ID, InternalOrderID: leg.InternalOrderID, LegIndex: leg.Index,
		BrokerLegID: stringPointerNull(leg.BrokerLegID), InstrumentID: leg.InstrumentID,
		ProductClass: string(leg.ProductClass), Side: leg.Side, Ratio: leg.Ratio,
		PredictionSide: leg.PredictionSide, RequestedQuantity: float64PointerNull(leg.RequestedQuantity),
		RequestedAmount: float64PointerNull(leg.RequestedAmount), RequestedPrice: float64PointerNull(leg.RequestedPrice),
		Status: leg.Status, FilledQuantity: float64PointerNull(leg.FilledQuantity),
		FilledAmount: float64PointerNull(leg.FilledAmount), AveragePrice: float64PointerNull(leg.AveragePrice),
		Fees: float64PointerNull(leg.Fees), Payout: float64PointerNull(leg.Payout),
		UpdatedAt: leg.UpdatedAt, CreatedAt: leg.CreatedAt,
	}
}

func executionOrderEventFromRow(row executionOrderEventRow) executionOrderEventResponse {
	return executionOrderEventResponse{
		ID:              row.ID,
		InternalOrderID: row.InternalOrderID,
		EventType:       row.EventType,
		PreviousStatus:  canonicalPersistedEventStatusPointer(row.EventType, nullStringPointer(row.PreviousStatus)),
		NextStatus:      canonicalPersistedEventStatus(row.EventType, row.NextStatus),
		PayloadJSON:     row.PayloadJSON,
		CreatedAt:       row.CreatedAt,
	}
}

func canonicalPersistedEventStatus(eventType, status string) string {
	if strings.HasPrefix(strings.ToUpper(strings.TrimSpace(eventType)), "BROKER_") {
		return trdsrv.CanonicalBrokerOrderStatus(status)
	}
	return trdsrv.CanonicalStoredOrderStatus(status)
}

func canonicalPersistedEventStatusPointer(eventType string, status *string) *string {
	if status == nil {
		return nil
	}
	canonical := canonicalPersistedEventStatus(eventType, *status)
	return &canonical
}

func executionOrderEventToRow(event executionOrderEventResponse) executionOrderEventRow {
	return executionOrderEventRow{
		ID:              event.ID,
		InternalOrderID: event.InternalOrderID,
		EventType:       event.EventType,
		PreviousStatus:  stringPointerNull(event.PreviousStatus),
		NextStatus:      event.NextStatus,
		PayloadJSON:     event.PayloadJSON,
		CreatedAt:       event.CreatedAt,
	}
}

func nullStringPointer(value sql.NullString) *string {
	if !value.Valid {
		return nil
	}
	return &value.String
}

func stringPointerNull(value *string) sql.NullString {
	if value == nil {
		return sql.NullString{}
	}
	return sql.NullString{String: *value, Valid: true}
}

func nullFloat64Pointer(value sql.NullFloat64) *float64 {
	if !value.Valid {
		return nil
	}
	return &value.Float64
}

func float64PointerNull(value *float64) sql.NullFloat64 {
	if value == nil {
		return sql.NullFloat64{}
	}
	return sql.NullFloat64{Float64: *value, Valid: true}
}

func executionSequenceSuffix(value string, prefix string) uint64 {
	trimmed := strings.TrimPrefix(strings.TrimSpace(value), prefix)
	if trimmed == value {
		return 0
	}
	var seq uint64
	if _, err := fmt.Sscanf(trimmed, "%d", &seq); err != nil {
		return 0
	}
	return seq
}
