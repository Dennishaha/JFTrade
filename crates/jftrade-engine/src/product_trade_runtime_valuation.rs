use std::sync::Arc;

use jftrade_integration_futu::{
    ValuationDetailQuery, ValuationDetailQueryError, ValuationDetailReadPort,
    ValuationDetailSnapshot,
};

use super::SharedTradeReadRuntime;

impl SharedTradeReadRuntime {
    pub(crate) fn set_valuation_detail(&self, reader: Option<Arc<dyn ValuationDetailReadPort>>) {
        *self
            .valuation_detail
            .write()
            .unwrap_or_else(|error| error.into_inner()) = reader;
    }

    pub(crate) fn valuation_detail_available(&self) -> bool {
        self.valuation_detail
            .read()
            .unwrap_or_else(|error| error.into_inner())
            .is_some()
    }

    pub(crate) fn valuation_detail(
        &self,
        query: &ValuationDetailQuery,
    ) -> Result<ValuationDetailSnapshot, ValuationDetailQueryError> {
        let reader = self
            .valuation_detail
            .read()
            .unwrap_or_else(|error| error.into_inner())
            .clone()
            .ok_or_else(|| {
                ValuationDetailQueryError::InvalidQuery(
                    "Futu valuation detail runtime is unavailable".to_owned(),
                )
            })?;
        reader.query(query)
    }
}
