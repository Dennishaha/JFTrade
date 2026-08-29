use std::sync::Arc;

use jftrade_integration_futu::{
    OptionChainDate, OptionChainQuery, OptionChainQueryError, OptionChainReadPort,
    OptionContractRankQuery, OptionContractRankQueryError, OptionContractRankReadPort,
    OptionContractRankSnapshot, OptionEventPage, OptionEventQuery, OptionEventQueryError,
    OptionEventReadPort, OptionExerciseProbabilityQuery, OptionExerciseProbabilityQueryError,
    OptionExerciseProbabilityReadPort, OptionExerciseProbabilitySnapshot, OptionExpirationDate,
    OptionExpirationQuery, OptionExpirationQueryError, OptionExpirationReadPort,
    OptionMarketStatisticQuery, OptionMarketStatisticQueryError, OptionMarketStatisticReadPort,
    OptionMarketStatisticSnapshot, OptionQuote, OptionQuoteQuery, OptionQuoteQueryError,
    OptionQuoteReadPort, OptionScreenPage, OptionScreenQuery, OptionScreenQueryError,
    OptionScreenReadPort, OptionStrategyAnalysisQuery, OptionStrategyAnalysisQueryError,
    OptionStrategyAnalysisReadPort, OptionStrategyAnalysisSnapshot, OptionStrategyQuery,
    OptionStrategyQueryError, OptionStrategyReadPort, OptionStrategySnapshot,
    OptionStrategySpreadQuery, OptionStrategySpreadQueryError, OptionStrategySpreadReadPort,
    OptionStrategySpreadSnapshot, OptionUnderlyingHisStatisticQuery,
    OptionUnderlyingHisStatisticQueryError, OptionUnderlyingHisStatisticReadPort,
    OptionUnderlyingHisStatisticSnapshot, OptionUnderlyingHisVolatilityQuery,
    OptionUnderlyingHisVolatilityQueryError, OptionUnderlyingHisVolatilityReadPort,
    OptionUnderlyingHisVolatilitySnapshot, OptionUnderlyingOverviewQuery,
    OptionUnderlyingOverviewQueryError, OptionUnderlyingOverviewReadPort,
    OptionUnderlyingOverviewSnapshot, OptionUnderlyingRankQuery, OptionUnderlyingRankQueryError,
    OptionUnderlyingRankReadPort, OptionUnderlyingRankSnapshot, OptionVolatilityQuery,
    OptionVolatilityQueryError, OptionVolatilityReadPort,
    OptionZeroDteScreenerPage, OptionZeroDteScreenerQuery, OptionZeroDteScreenerQueryError,
    OptionZeroDteScreenerReadPort, OptionEarningsScreenerPage, OptionEarningsScreenerQuery,
    OptionEarningsScreenerQueryError, OptionEarningsScreenerReadPort,
    OptionZeroDteContractItem, OptionZeroDteContractQuery, OptionZeroDteContractQueryError,
    OptionZeroDteContractReadPort, OptionSellerScreenerItem, OptionSellerScreenerQuery,
    OptionSellerScreenerQueryError, OptionSellerScreenerReadPort,
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

    pub(crate) fn set_option_expirations(&self, reader: Option<Arc<dyn OptionExpirationReadPort>>) {
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

    pub(crate) fn set_option_volatility(&self, reader: Option<Arc<dyn OptionVolatilityReadPort>>) {
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
    ) -> Result<jftrade_integration_futu::OptionVolatilitySnapshot, OptionVolatilityQueryError>
    {
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

    pub(crate) fn set_option_exercise_probability(
        &self,
        reader: Option<Arc<dyn OptionExerciseProbabilityReadPort>>,
    ) {
        *self
            .option_exercise_probability
            .write()
            .unwrap_or_else(|error| error.into_inner()) = reader;
    }

    pub(crate) fn option_exercise_probability_available(&self) -> bool {
        self.option_exercise_probability
            .read()
            .unwrap_or_else(|error| error.into_inner())
            .is_some()
    }

    pub(crate) fn option_exercise_probability(
        &self,
        query: &OptionExerciseProbabilityQuery,
    ) -> Result<OptionExerciseProbabilitySnapshot, OptionExerciseProbabilityQueryError> {
        let reader = self
            .option_exercise_probability
            .read()
            .unwrap_or_else(|error| error.into_inner())
            .clone()
            .ok_or_else(|| {
                OptionExerciseProbabilityQueryError::InvalidQuery(
                    "Futu option exercise probability runtime is unavailable".to_owned(),
                )
            })?;
        reader.query(query)
    }

    pub(crate) fn set_option_underlying_overview(
        &self,
        reader: Option<Arc<dyn OptionUnderlyingOverviewReadPort>>,
    ) {
        *self
            .option_underlying_overview
            .write()
            .unwrap_or_else(|error| error.into_inner()) = reader;
    }

    pub(crate) fn option_underlying_overview_available(&self) -> bool {
        self.option_underlying_overview
            .read()
            .unwrap_or_else(|error| error.into_inner())
            .is_some()
    }

    pub(crate) fn option_underlying_overview(
        &self,
        query: &OptionUnderlyingOverviewQuery,
    ) -> Result<OptionUnderlyingOverviewSnapshot, OptionUnderlyingOverviewQueryError> {
        let reader = self
            .option_underlying_overview
            .read()
            .unwrap_or_else(|error| error.into_inner())
            .clone()
            .ok_or_else(|| {
                OptionUnderlyingOverviewQueryError::InvalidQuery(
                    "Futu option underlying overview runtime is unavailable".to_owned(),
                )
            })?;
        reader.query(query)
    }

    pub(crate) fn set_option_underlying_his_volatility(
        &self,
        reader: Option<Arc<dyn OptionUnderlyingHisVolatilityReadPort>>,
    ) {
        *self
            .option_underlying_his_volatility
            .write()
            .unwrap_or_else(|error| error.into_inner()) = reader;
    }

    pub(crate) fn option_underlying_his_volatility_available(&self) -> bool {
        self.option_underlying_his_volatility
            .read()
            .unwrap_or_else(|error| error.into_inner())
            .is_some()
    }

    pub(crate) fn option_underlying_his_volatility(
        &self,
        query: &OptionUnderlyingHisVolatilityQuery,
    ) -> Result<OptionUnderlyingHisVolatilitySnapshot, OptionUnderlyingHisVolatilityQueryError>
    {
        let reader = self
            .option_underlying_his_volatility
            .read()
            .unwrap_or_else(|error| error.into_inner())
            .clone()
            .ok_or_else(|| {
                OptionUnderlyingHisVolatilityQueryError::InvalidQuery(
                    "Futu option underlying historical volatility runtime is unavailable"
                        .to_owned(),
                )
            })?;
        reader.query(query)
    }

    pub(crate) fn set_option_market_statistic(
        &self,
        reader: Option<Arc<dyn OptionMarketStatisticReadPort>>,
    ) {
        *self
            .option_market_statistic
            .write()
            .unwrap_or_else(|error| error.into_inner()) = reader;
    }

    pub(crate) fn option_market_statistic_available(&self) -> bool {
        self.option_market_statistic
            .read()
            .unwrap_or_else(|error| error.into_inner())
            .is_some()
    }

    pub(crate) fn option_market_statistic(
        &self,
        query: &OptionMarketStatisticQuery,
    ) -> Result<OptionMarketStatisticSnapshot, OptionMarketStatisticQueryError> {
        let reader = self
            .option_market_statistic
            .read()
            .unwrap_or_else(|error| error.into_inner())
            .clone()
            .ok_or_else(|| {
                OptionMarketStatisticQueryError::InvalidQuery(
                    "Futu option market statistic runtime is unavailable".to_owned(),
                )
            })?;
        reader.query(query)
    }

    pub(crate) fn set_option_underlying_his_statistic(
        &self,
        reader: Option<Arc<dyn OptionUnderlyingHisStatisticReadPort>>,
    ) {
        *self
            .option_underlying_his_statistic
            .write()
            .unwrap_or_else(|error| error.into_inner()) = reader;
    }

    pub(crate) fn option_underlying_his_statistic_available(&self) -> bool {
        self.option_underlying_his_statistic
            .read()
            .unwrap_or_else(|error| error.into_inner())
            .is_some()
    }

    pub(crate) fn option_underlying_his_statistic(
        &self,
        query: &OptionUnderlyingHisStatisticQuery,
    ) -> Result<OptionUnderlyingHisStatisticSnapshot, OptionUnderlyingHisStatisticQueryError> {
        let reader = self
            .option_underlying_his_statistic
            .read()
            .unwrap_or_else(|error| error.into_inner())
            .clone()
            .ok_or_else(|| {
                OptionUnderlyingHisStatisticQueryError::InvalidQuery(
                    "Futu option underlying historical statistic runtime is unavailable".to_owned(),
                )
            })?;
        reader.query(query)
    }

    pub(crate) fn set_option_strategy_spread(
        &self,
        reader: Option<Arc<dyn OptionStrategySpreadReadPort>>,
    ) {
        *self
            .option_strategy_spread
            .write()
            .unwrap_or_else(|error| error.into_inner()) = reader;
    }

    pub(crate) fn option_strategy_spread_available(&self) -> bool {
        self.option_strategy_spread
            .read()
            .unwrap_or_else(|error| error.into_inner())
            .is_some()
    }

    pub(crate) fn option_strategy_spread(
        &self,
        query: &OptionStrategySpreadQuery,
    ) -> Result<OptionStrategySpreadSnapshot, OptionStrategySpreadQueryError> {
        let reader = self
            .option_strategy_spread
            .read()
            .unwrap_or_else(|error| error.into_inner())
            .clone()
            .ok_or_else(|| {
                OptionStrategySpreadQueryError::InvalidQuery(
                    "Futu option strategy spread runtime is unavailable".to_owned(),
                )
            })?;
        reader.query(query)
    }

    pub(crate) fn set_option_strategy(&self, reader: Option<Arc<dyn OptionStrategyReadPort>>) {
        *self
            .option_strategy
            .write()
            .unwrap_or_else(|error| error.into_inner()) = reader;
    }

    pub(crate) fn option_strategy_available(&self) -> bool {
        self.option_strategy
            .read()
            .unwrap_or_else(|error| error.into_inner())
            .is_some()
    }

    pub(crate) fn option_strategy(
        &self,
        query: &OptionStrategyQuery,
    ) -> Result<OptionStrategySnapshot, OptionStrategyQueryError> {
        let reader = self
            .option_strategy
            .read()
            .unwrap_or_else(|error| error.into_inner())
            .clone()
            .ok_or_else(|| {
                OptionStrategyQueryError::InvalidQuery(
                    "Futu option strategy runtime is unavailable".to_owned(),
                )
            })?;
        reader.query(query)
    }

    pub(crate) fn set_option_strategy_analysis(
        &self,
        reader: Option<Arc<dyn OptionStrategyAnalysisReadPort>>,
    ) {
        *self
            .option_strategy_analysis
            .write()
            .unwrap_or_else(|error| error.into_inner()) = reader;
    }

    pub(crate) fn option_strategy_analysis_available(&self) -> bool {
        self.option_strategy_analysis
            .read()
            .unwrap_or_else(|error| error.into_inner())
            .is_some()
    }

    pub(crate) fn option_strategy_analysis(
        &self,
        query: &OptionStrategyAnalysisQuery,
    ) -> Result<OptionStrategyAnalysisSnapshot, OptionStrategyAnalysisQueryError> {
        let reader = self
            .option_strategy_analysis
            .read()
            .unwrap_or_else(|error| error.into_inner())
            .clone()
            .ok_or_else(|| {
                OptionStrategyAnalysisQueryError::InvalidQuery(
                    "Futu option strategy analysis runtime is unavailable".to_owned(),
                )
            })?;
        reader.query(query)
    }

    pub(crate) fn set_option_underlying_rank(
        &self,
        reader: Option<Arc<dyn OptionUnderlyingRankReadPort>>,
    ) {
        *self
            .option_underlying_rank
            .write()
            .unwrap_or_else(|error| error.into_inner()) = reader;
    }

    pub(crate) fn option_underlying_rank_available(&self) -> bool {
        self.option_underlying_rank
            .read()
            .unwrap_or_else(|error| error.into_inner())
            .is_some()
    }

    pub(crate) fn option_underlying_rank(
        &self,
        query: &OptionUnderlyingRankQuery,
    ) -> Result<OptionUnderlyingRankSnapshot, OptionUnderlyingRankQueryError> {
        let reader = self
            .option_underlying_rank
            .read()
            .unwrap_or_else(|error| error.into_inner())
            .clone()
            .ok_or_else(|| {
                OptionUnderlyingRankQueryError::InvalidQuery(
                    "Futu option underlying rank runtime is unavailable".to_owned(),
                )
            })?;
        reader.query(query)
    }

    pub(crate) fn set_option_contract_rank(
        &self,
        reader: Option<Arc<dyn OptionContractRankReadPort>>,
    ) {
        *self
            .option_contract_rank
            .write()
            .unwrap_or_else(|error| error.into_inner()) = reader;
    }

    pub(crate) fn option_contract_rank_available(&self) -> bool {
        self.option_contract_rank
            .read()
            .unwrap_or_else(|error| error.into_inner())
            .is_some()
    }

    pub(crate) fn option_contract_rank(
        &self,
        query: &OptionContractRankQuery,
    ) -> Result<OptionContractRankSnapshot, OptionContractRankQueryError> {
        let reader = self
            .option_contract_rank
            .read()
            .unwrap_or_else(|error| error.into_inner())
            .clone()
            .ok_or_else(|| {
                OptionContractRankQueryError::InvalidQuery(
                    "Futu option contract rank runtime is unavailable".to_owned(),
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

    pub(crate) fn set_option_zero_dte_screener(
        &self,
        reader: Option<Arc<dyn OptionZeroDteScreenerReadPort>>,
    ) {
        *self
            .option_zero_dte_screener
            .write()
            .unwrap_or_else(|error| error.into_inner()) = reader;
    }

    pub(crate) fn option_zero_dte_screener_available(&self) -> bool {
        self.option_zero_dte_screener
            .read()
            .unwrap_or_else(|error| error.into_inner())
            .is_some()
    }

    pub(crate) fn option_zero_dte_screener(
        &self,
        query: &OptionZeroDteScreenerQuery,
    ) -> Result<OptionZeroDteScreenerPage, OptionZeroDteScreenerQueryError> {
        let reader = self
            .option_zero_dte_screener
            .read()
            .unwrap_or_else(|error| error.into_inner())
            .clone()
            .ok_or_else(|| {
                OptionZeroDteScreenerQueryError::InvalidQuery(
                    "Futu 0DTE screener runtime is unavailable".to_owned(),
                )
            })?;
        reader.query(query)
    }

    pub(crate) fn set_option_earnings_screener(
        &self,
        reader: Option<Arc<dyn OptionEarningsScreenerReadPort>>,
    ) {
        *self
            .option_earnings_screener
            .write()
            .unwrap_or_else(|error| error.into_inner()) = reader;
    }

    pub(crate) fn option_earnings_screener_available(&self) -> bool {
        self.option_earnings_screener
            .read()
            .unwrap_or_else(|error| error.into_inner())
            .is_some()
    }

    pub(crate) fn option_earnings_screener(
        &self,
        query: &OptionEarningsScreenerQuery,
    ) -> Result<OptionEarningsScreenerPage, OptionEarningsScreenerQueryError> {
        let reader = self
            .option_earnings_screener
            .read()
            .unwrap_or_else(|error| error.into_inner())
            .clone()
            .ok_or_else(|| {
                OptionEarningsScreenerQueryError::InvalidQuery(
                    "Futu earnings screener runtime is unavailable".to_owned(),
                )
            })?;
        reader.query(query)
    }

    pub(crate) fn set_option_zero_dte_contract(
        &self,
        reader: Option<Arc<dyn OptionZeroDteContractReadPort>>,
    ) {
        *self
            .option_zero_dte_contract
            .write()
            .unwrap_or_else(|error| error.into_inner()) = reader;
    }

    pub(crate) fn option_zero_dte_contract_available(&self) -> bool {
        self.option_zero_dte_contract
            .read()
            .unwrap_or_else(|error| error.into_inner())
            .is_some()
    }

    pub(crate) fn option_zero_dte_contract(
        &self,
        query: &OptionZeroDteContractQuery,
    ) -> Result<Vec<OptionZeroDteContractItem>, OptionZeroDteContractQueryError> {
        let reader = self
            .option_zero_dte_contract
            .read()
            .unwrap_or_else(|error| error.into_inner())
            .clone()
            .ok_or_else(|| {
                OptionZeroDteContractQueryError::InvalidQuery(
                    "Futu 0DTE contract runtime is unavailable".to_owned(),
                )
            })?;
        reader.query(query)
    }

    pub(crate) fn set_option_seller_screener(
        &self,
        reader: Option<Arc<dyn OptionSellerScreenerReadPort>>,
    ) {
        *self
            .option_seller_screener
            .write()
            .unwrap_or_else(|error| error.into_inner()) = reader;
    }

    pub(crate) fn option_seller_screener_available(&self) -> bool {
        self.option_seller_screener
            .read()
            .unwrap_or_else(|error| error.into_inner())
            .is_some()
    }

    pub(crate) fn option_seller_screener(
        &self,
        query: &OptionSellerScreenerQuery,
    ) -> Result<Vec<OptionSellerScreenerItem>, OptionSellerScreenerQueryError> {
        let reader = self
            .option_seller_screener
            .read()
            .unwrap_or_else(|error| error.into_inner())
            .clone()
            .ok_or_else(|| {
                OptionSellerScreenerQueryError::InvalidQuery(
                    "Futu seller screener runtime is unavailable".to_owned(),
                )
            })?;
        reader.query(query)
    }
}
