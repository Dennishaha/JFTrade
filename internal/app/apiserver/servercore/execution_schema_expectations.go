package servercore

func expectedExecutionOrderColumns() []string {
	return []string{
		"internal_order_id:TEXT:1", "broker_id:TEXT:0", "broker_order_id:TEXT:0", "broker_order_id_ex:TEXT:0",
		"source:TEXT:0", "source_detail:TEXT:0", "trading_environment:TEXT:0", "account_id:TEXT:0", "market:TEXT:0",
		"symbol:TEXT:0", "side:TEXT:0", "order_type:TEXT:0", "status:TEXT:0", "requested_quantity:REAL:0",
		"requested_price:REAL:0", "filled_quantity:REAL:0", "filled_average_price:REAL:0", "remark:TEXT:0",
		"last_error:TEXT:0", "last_error_code:TEXT:0", "last_error_source:TEXT:0", "submitted_at:TEXT:0",
		"updated_at:TEXT:0", "created_at:TEXT:0",
		"raw_broker_status:TEXT:0",
		"order_kind:TEXT:0", "product_class:TEXT:0", "quantity_mode:TEXT:0", "client_order_id:TEXT:0",
		"preview_id:TEXT:0", "normalized_request:TEXT:0", "requested_amount:REAL:0", "payout:REAL:0",
		"fees:REAL:0",
	}
}

func expectedExecutionOrderLegColumns() []string {
	return []string{
		"id:TEXT:1", "internal_order_id:TEXT:0", "leg_index:INTEGER:0", "broker_leg_id:TEXT:0",
		"instrument_id:TEXT:0", "product_class:TEXT:0", "side:TEXT:0", "ratio:INTEGER:0",
		"prediction_side:TEXT:0", "requested_quantity:REAL:0", "requested_amount:REAL:0",
		"requested_price:REAL:0", "status:TEXT:0", "filled_quantity:REAL:0", "filled_amount:REAL:0",
		"average_price:REAL:0", "fees:REAL:0", "payout:REAL:0", "updated_at:TEXT:0", "created_at:TEXT:0",
	}
}

func expectedExecutionOrderPreviewColumns() []string {
	return []string{
		"preview_id:TEXT:1", "request_hash:TEXT:0", "broker_id:TEXT:0", "capability_version:TEXT:0",
		"account_id:TEXT:0", "expires_at:TEXT:0", "quote_expires_at:TEXT:0", "rfq_id:TEXT:0",
		"normalized_request:TEXT:0", "created_at:TEXT:0", "consumed_at:TEXT:0",
	}
}

func expectedExecutionPredictionQuoteColumns() []string {
	return []string{
		"quote_id:TEXT:1", "broker_id:TEXT:0", "account_id:TEXT:0",
		"trading_environment:TEXT:0", "mvc:TEXT:0", "legs_hash:TEXT:0",
		"bid_price:REAL:0", "ask_price:REAL:0", "should_retry:INTEGER:0",
		"received_at:TEXT:0", "expires_at:TEXT:0", "expiry_source:TEXT:0",
		"status:TEXT:0", "consumed_at:TEXT:0", "consumed_preview_id:TEXT:0",
		"consumed_client_order_id:TEXT:0",
	}
}

func expectedExecutionOrderEventColumns() []string {
	return []string{
		"id:TEXT:1", "internal_order_id:TEXT:0", "event_type:TEXT:0", "previous_status:TEXT:0",
		"next_status:TEXT:0", "payload_json:TEXT:0", "created_at:TEXT:0",
	}
}

func expectedExecutionSeenFillColumns() []string {
	return []string{"fill_key:TEXT:1", "created_at:TEXT:0"}
}

func expectedExecutionSequenceColumns() []string {
	return []string{"name:TEXT:1", "value:INTEGER:0"}
}
