//! Production execution preview operations.
//!
//! The OpenD product-rule and combo-preview readers are separate capabilities
//! from order submission. Keep their lifecycle and fail-closed checks in a
//! small sibling module so the main execution adapter remains reviewable.

use super::*;
use super::execution_order_helpers::{parse_product_rule_request, product_rule_rejection};

impl ProductionExecutionPort {
    pub(super) fn buying_power_preview(
        &self,
        payload: &Value,
    ) -> Result<Value, ExecutionWritePortError> {
        // Parsing still provides the baseline 400 response for malformed
        // payloads, but there is no production ProductRule/OpenD reader yet.
        // Never default to {allowed:true}: that would present a successful
        // buying-power decision without broker evidence.
        let request = parse_product_rule_request(payload)?;
        if let Some((code, message)) = product_rule_rejection(&request) {
            return Err(failed(400, code, message));
        }
        // Keep request validation independent from external readiness.  An
        // invalid command is a 400 even if OpenD is disconnected; only a
        // well-formed command reaches the unavailable ProductRule adapter.
        self.ensure_futu_runtime()?;
        Err(ExecutionWritePortError::Unavailable(
            "Futu product-rule adapter is unavailable".to_owned(),
        ))
    }

    pub(super) fn combo_preview(
        &self,
        payload: &Value,
    ) -> Result<Value, ExecutionWritePortError> {
        let _parsed = parse_combo(payload)
            .map_err(|message| failed(400, "BAD_REQUEST", message))?;
        self.ensure_futu_runtime()?;
        Err(ExecutionWritePortError::Unavailable(
            "Futu combo preview provider is unavailable".to_owned(),
        ))
    }

    pub(super) fn ensure_futu_runtime(&self) -> Result<(), ExecutionWritePortError> {
        let snapshot = self.active_provider_state.snapshot();
        if snapshot.provider != Some(jftrade_settings::MarketDataProvider::Futu) {
            return Err(ExecutionWritePortError::Unavailable(
                "Futu is not the active market-data provider".to_owned(),
            ));
        }
        if !snapshot.opend_ready {
            return Err(ExecutionWritePortError::Unavailable(
                "Futu OpenD runtime is not ready".to_owned(),
            ));
        }
        Ok(())
    }
}
