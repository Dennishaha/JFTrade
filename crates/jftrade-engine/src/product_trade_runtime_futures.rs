use std::sync::Arc;

use jftrade_integration_futu::{
    FutureInfo, FutureInfoQuery, FutureInfoQueryError, FutureInfoReadPort,
};

use super::SharedTradeReadRuntime;

impl SharedTradeReadRuntime {
    pub(crate) fn set_future_info(&self, reader: Option<Arc<dyn FutureInfoReadPort>>) {
        *self
            .future_info
            .write()
            .unwrap_or_else(|error| error.into_inner()) = reader;
    }

    pub(crate) fn future_info_available(&self) -> bool {
        self.future_info
            .read()
            .unwrap_or_else(|error| error.into_inner())
            .is_some()
    }

    pub(crate) fn future_info(
        &self,
        query: &FutureInfoQuery,
    ) -> Result<Vec<FutureInfo>, FutureInfoQueryError> {
        let reader = self
            .future_info
            .read()
            .unwrap_or_else(|error| error.into_inner())
            .clone()
            .ok_or_else(|| {
                FutureInfoQueryError::InvalidQuery(
                    "Futu future info runtime is unavailable".to_owned(),
                )
            })?;
        reader.query(query)
    }
}
