use std::sync::Arc;

use jftrade_integration_futu::{
    OptionExpirationDate, OptionExpirationQuery, OptionExpirationQueryError,
    OptionExpirationReadPort,
};

use super::product_trade_runtime_projection::SharedTradeReadRuntime;

impl SharedTradeReadRuntime {
    pub(crate) fn set_option_expirations(
        &self,
        reader: Option<Arc<dyn OptionExpirationReadPort>>,
    ) {
        *self
            .option_expirations
            .write()
            .unwrap_or_else(|error| error.into_inner()) = reader;
    }

    pub(crate) fn option_expirations_available(&self) -> bool {
        self.option_expirations
            .read()
            .unwrap_or_else(|error| error.into_inner())
            .is_some()
    }

    pub(crate) fn option_expirations(
        &self,
        query: &OptionExpirationQuery,
    ) -> Result<Vec<OptionExpirationDate>, OptionExpirationQueryError> {
        let reader = self
            .option_expirations
            .read()
            .unwrap_or_else(|error| error.into_inner())
            .clone()
            .ok_or_else(|| {
                OptionExpirationQueryError::InvalidQuery(
                    "Futu option expiration runtime is unavailable".to_owned(),
                )
            })?;
        reader.query(query)
    }
}
