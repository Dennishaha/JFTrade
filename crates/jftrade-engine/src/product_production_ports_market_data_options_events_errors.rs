use super::*;

pub(super) fn map_error(
    error: jftrade_integration_futu::OptionEventQueryError,
) -> MarketDataOptionsReadSnapshotError {
    match error {
        jftrade_integration_futu::OptionEventQueryError::InvalidQuery(message) => {
            bad_request(&message)
        }
        other => MarketDataOptionsReadSnapshotError::Failed {
            status: 502,
            code: "BAD_GATEWAY".to_owned(),
            message: other.to_string(),
        },
    }
}

pub(super) fn map_zero_dte_error(
    error: jftrade_integration_futu::OptionZeroDteScreenerQueryError,
) -> MarketDataOptionsReadSnapshotError {
    use jftrade_integration_futu::OptionZeroDteScreenerQueryError as Error;
    match error {
        Error::InvalidQuery(message) => bad_request(&message),
        other => bad_gateway(other.to_string()),
    }
}

pub(super) fn map_earnings_error(
    error: jftrade_integration_futu::OptionEarningsScreenerQueryError,
) -> MarketDataOptionsReadSnapshotError {
    use jftrade_integration_futu::OptionEarningsScreenerQueryError as Error;
    match error {
        Error::InvalidQuery(message) => bad_request(&message),
        other => bad_gateway(other.to_string()),
    }
}

pub(super) fn map_seller_error(
    error: jftrade_integration_futu::OptionSellerScreenerQueryError,
) -> MarketDataOptionsReadSnapshotError {
    use jftrade_integration_futu::OptionSellerScreenerQueryError as Error;
    match error {
        Error::InvalidQuery(message) => bad_request(&message),
        other => bad_gateway(other.to_string()),
    }
}

pub(super) fn map_zero_dte_contract_error(
    error: jftrade_integration_futu::OptionZeroDteContractQueryError,
) -> MarketDataOptionsReadSnapshotError {
    use jftrade_integration_futu::OptionZeroDteContractQueryError as Error;
    match error {
        Error::InvalidQuery(message) => context_bad_request(&message),
        other => bad_gateway(other.to_string()),
    }
}

pub(super) fn bad_gateway(message: String) -> MarketDataOptionsReadSnapshotError {
    MarketDataOptionsReadSnapshotError::Failed {
        status: 502,
        code: "BAD_GATEWAY".to_owned(),
        message,
    }
}
