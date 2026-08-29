use std::sync::Arc;

use jftrade_integration_futu::{
    OptionChainDate, OptionChainQuery, OptionChainQueryError, OptionChainReadPort,
    OptionExpirationDate, OptionExpirationQuery, OptionExpirationQueryError,
    OptionExpirationReadPort, OptionScreenPage, OptionScreenQuery, OptionScreenQueryError,
    OptionScreenReadPort, OptionQuote, OptionQuoteQuery, OptionQuoteQueryError,
    OptionQuoteReadPort, OptionVolatilityQuery, OptionVolatilityQueryError,
    OptionVolatilityReadPort, OptionEventPage, OptionEventQuery, OptionEventQueryError,
    OptionEventReadPort,
};

use super::product_trade_runtime_projection::SharedTradeReadRuntime;

impl SharedTradeReadRuntime {
    pub(crate) fn set_option_chains(&self, reader: Option<Arc<dyn OptionChainReadPort>>) {
        *self
            .option_chains
            .write()
            .unwrap_or_else(|error| error.into_inner()) = reader;
    }

    pub(crate) fn option_chains_available(&self) -> bool {
        self.option_chains
            .read()
            .unwrap_or_else(|error| error.into_inner())
            .is_some()
    }

    pub(crate) fn option_chains(
        &self,
        query: &OptionChainQuery,
    ) -> Result<Vec<OptionChainDate>, OptionChainQueryError> {
        let reader = self
            .option_chains
            .read()
            .unwrap_or_else(|error| error.into_inner())
            .clone()
            .ok_or_else(|| {
                OptionChainQueryError::InvalidQuery(
                    "Futu option chain runtime is unavailable".to_owned(),
                )
            })?;
        reader.query(query)
    }

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

    pub(crate) fn set_option_screens(&self, reader: Option<Arc<dyn OptionScreenReadPort>>) {
        *self
            .option_screens
            .write()
            .unwrap_or_else(|error| error.into_inner()) = reader;
    }

    pub(crate) fn option_screens_available(&self) -> bool {
        self.option_screens
            .read()
            .unwrap_or_else(|error| error.into_inner())
            .is_some()
    }

    pub(crate) fn option_screens(
        &self,
        query: &OptionScreenQuery,
    ) -> Result<OptionScreenPage, OptionScreenQueryError> {
        let reader = self
            .option_screens
            .read()
            .unwrap_or_else(|error| error.into_inner())
            .clone()
            .ok_or_else(|| {
                OptionScreenQueryError::InvalidQuery(
                    "Futu option screen runtime is unavailable".to_owned(),
                )
            })?;
        reader.query(query)
    }

    pub(crate) fn set_option_quotes(&self, reader: Option<Arc<dyn OptionQuoteReadPort>>) {
        *self
            .option_quotes
            .write()
            .unwrap_or_else(|error| error.into_inner()) = reader;
    }

    pub(crate) fn option_quotes_available(&self) -> bool {
        self.option_quotes
            .read()
            .unwrap_or_else(|error| error.into_inner())
            .is_some()
    }

    pub(crate) fn option_quotes(
        &self,
        query: &OptionQuoteQuery,
    ) -> Result<Vec<OptionQuote>, OptionQuoteQueryError> {
        let reader = self
            .option_quotes
            .read()
            .unwrap_or_else(|error| error.into_inner())
            .clone()
            .ok_or_else(|| {
                OptionQuoteQueryError::InvalidQuery(
                    "Futu option quote runtime is unavailable".to_owned(),
                )
            })?;
        reader.query(query)
    }

    pub(crate) fn set_option_volatility(
        &self,
        reader: Option<Arc<dyn OptionVolatilityReadPort>>,
    ) {
        *self
            .option_volatility
            .write()
            .unwrap_or_else(|error| error.into_inner()) = reader;
    }

    pub(crate) fn option_volatility_available(&self) -> bool {
        self.option_volatility
            .read()
            .unwrap_or_else(|error| error.into_inner())
            .is_some()
    }

    pub(crate) fn option_volatility(
        &self,
        query: &OptionVolatilityQuery,
    ) -> Result<jftrade_integration_futu::OptionVolatilitySnapshot, OptionVolatilityQueryError> {
        let reader = self
            .option_volatility
            .read()
            .unwrap_or_else(|error| error.into_inner())
            .clone()
            .ok_or_else(|| {
                OptionVolatilityQueryError::InvalidQuery(
                    "Futu option volatility runtime is unavailable".to_owned(),
                )
            })?;
        reader.query(query)
    }

    pub(crate) fn set_option_events(&self, reader: Option<Arc<dyn OptionEventReadPort>>) {
        *self
            .option_events
            .write()
            .unwrap_or_else(|error| error.into_inner()) = reader;
    }

    pub(crate) fn option_events_available(&self) -> bool {
        self.option_events
            .read()
            .unwrap_or_else(|error| error.into_inner())
            .is_some()
    }

    pub(crate) fn option_events(
        &self,
        query: &OptionEventQuery,
    ) -> Result<OptionEventPage, OptionEventQueryError> {
        let reader = self
            .option_events
            .read()
            .unwrap_or_else(|error| error.into_inner())
            .clone()
            .ok_or_else(|| {
                OptionEventQueryError::InvalidQuery(
                    "Futu option event runtime is unavailable".to_owned(),
                )
            })?;
        reader.query(query)
    }
}
