use crate::product::MarketDataOptionsReadSnapshotError;

pub(super) fn options_bad_request(message: &str) -> MarketDataOptionsReadSnapshotError {
    MarketDataOptionsReadSnapshotError::Failed {
        status: 400,
        code: "BAD_REQUEST".to_owned(),
        message: message.to_owned(),
    }
}

pub(super) fn map_option_chain_error(
    error: jftrade_integration_futu::OptionChainQueryError,
) -> MarketDataOptionsReadSnapshotError {
    match error {
        jftrade_integration_futu::OptionChainQueryError::InvalidQuery(message) => {
            options_bad_request(&message)
        }
        other => MarketDataOptionsReadSnapshotError::Failed {
            status: 502,
            code: "BAD_GATEWAY".to_owned(),
            message: other.to_string(),
        },
    }
}

pub(super) fn map_option_expiration_error(
    error: jftrade_integration_futu::OptionExpirationQueryError,
) -> MarketDataOptionsReadSnapshotError {
    match error {
        jftrade_integration_futu::OptionExpirationQueryError::InvalidQuery(message) => {
            options_bad_request(&message)
        }
        other => MarketDataOptionsReadSnapshotError::Failed {
            status: 502,
            code: "BAD_GATEWAY".to_owned(),
            message: other.to_string(),
        },
    }
}
