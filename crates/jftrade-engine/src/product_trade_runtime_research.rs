//! Shared runtime slots for Futu's advanced research readers.

use std::sync::Arc;

use jftrade_integration_futu::{
    FutuIndicatorCalculation, FutuIndicatorList, FutuIndicatorListQuery,
    FutuIndicatorQueryError, FutuIndicatorReadPort, FutuInstitutionQuery,
    FutuInstitutionQueryError, FutuInstitutionReadPort, FutuInstitutionResult,
    FutuShortInterestQuery, FutuShortInterestQueryError,
    FutuShortInterestReadPort, FutuShortInterestResult, IndicatorCalcQuery,
};

use super::SharedTradeReadRuntime;

impl SharedTradeReadRuntime {
    pub(crate) fn set_institution_reader(
        &self,
        reader: Option<Arc<dyn FutuInstitutionReadPort>>,
    ) {
        *self
            .institution_reader
            .write()
            .unwrap_or_else(|error| error.into_inner()) = reader;
    }

    pub(crate) fn institution_reader_available(&self) -> bool {
        self.institution_reader
            .read()
            .unwrap_or_else(|error| error.into_inner())
            .is_some()
    }

    pub(crate) fn institution_reader(&self) -> Option<Arc<dyn FutuInstitutionReadPort>> {
        self.institution_reader
            .read()
            .unwrap_or_else(|error| error.into_inner())
            .clone()
    }

    pub(crate) fn institution(
        &self,
        query: &FutuInstitutionQuery,
    ) -> Result<FutuInstitutionResult, FutuInstitutionQueryError> {
        self.institution_reader()
            .ok_or_else(|| {
                FutuInstitutionQueryError::InvalidQuery(
                    "Futu institution research runtime is unavailable".to_owned(),
                )
            })?
            .query(query)
    }

    pub(crate) fn set_short_interest_reader(
        &self,
        reader: Option<Arc<dyn FutuShortInterestReadPort>>,
    ) {
        *self
            .short_interest_reader
            .write()
            .unwrap_or_else(|error| error.into_inner()) = reader;
    }

    pub(crate) fn short_interest_reader_available(&self) -> bool {
        self.short_interest_reader
            .read()
            .unwrap_or_else(|error| error.into_inner())
            .is_some()
    }

    pub(crate) fn short_interest_reader(&self) -> Option<Arc<dyn FutuShortInterestReadPort>> {
        self.short_interest_reader
            .read()
            .unwrap_or_else(|error| error.into_inner())
            .clone()
    }

    pub(crate) fn short_interest(
        &self,
        query: &FutuShortInterestQuery,
    ) -> Result<FutuShortInterestResult, FutuShortInterestQueryError> {
        self.short_interest_reader()
            .ok_or_else(|| {
                FutuShortInterestQueryError::InvalidQuery(
                    "Futu short-interest research runtime is unavailable".to_owned(),
                )
            })?
            .query(query)
    }

    pub(crate) fn set_technical_indicator_reader(
        &self,
        reader: Option<Arc<dyn FutuIndicatorReadPort>>,
    ) {
        *self
            .technical_indicator_reader
            .write()
            .unwrap_or_else(|error| error.into_inner()) = reader;
    }

    pub(crate) fn technical_indicator_reader_available(&self) -> bool {
        self.technical_indicator_reader
            .read()
            .unwrap_or_else(|error| error.into_inner())
            .is_some()
    }

    pub(crate) fn technical_indicator_reader(&self) -> Option<Arc<dyn FutuIndicatorReadPort>> {
        self.technical_indicator_reader
            .read()
            .unwrap_or_else(|error| error.into_inner())
            .clone()
    }

    pub(crate) fn technical_indicator_list(
        &self,
        query: &FutuIndicatorListQuery,
    ) -> Result<FutuIndicatorList, FutuIndicatorQueryError> {
        self.technical_indicator_reader()
            .ok_or_else(|| {
                FutuIndicatorQueryError::InvalidQuery(
                    "Futu technical-indicator research runtime is unavailable".to_owned(),
                )
            })?
            .list(query)
    }

    pub(crate) fn technical_indicator_calculate(
        &self,
        query: &IndicatorCalcQuery,
    ) -> Result<FutuIndicatorCalculation, FutuIndicatorQueryError> {
        self.technical_indicator_reader()
            .ok_or_else(|| {
                FutuIndicatorQueryError::InvalidQuery(
                    "Futu technical-indicator research runtime is unavailable".to_owned(),
                )
            })?
            .calculate(query)
    }
}
