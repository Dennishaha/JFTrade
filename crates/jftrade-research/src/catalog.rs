use std::collections::BTreeMap;
use std::sync::OnceLock;

use serde::Deserialize;
use serde_json::Value;
use thiserror::Error;

const CATALOG_FIXTURE: &str =
    include_str!("../../../tests/fixtures/rust-migration/stage9/research-screen-catalogs.json");
const CATALOG_FIXTURE_VERSION: &str = "stage9.research-screen-catalogs.v1";

#[derive(Debug, Deserialize)]
struct CatalogFixture {
    version: String,
    catalogs: BTreeMap<String, Value>,
}

#[derive(Clone, Debug, Eq, PartialEq, Error)]
pub enum ScreenCatalogError {
    #[error("unsupported stock-screen market")]
    UnsupportedFutuMarket,
    #[error("unsupported stock-screen market for {0}")]
    UnsupportedEmbeddedMarket(String),
    #[error("the stock-screen factor catalog is not available for broker {0}")]
    BrokerUnavailable(String),
    #[error("stock-screen catalog fixture is invalid: {0}")]
    FixtureInvalid(String),
}

/// Returns the exact Go-owned stock-screen catalog for a supported broker and market.
///
/// The fixture is generated from `pkg/researchscreen` and contains every catalog
/// variant accepted by the current HTTP handler. It is intentionally immutable:
/// this route must not activate a provider or consult a network/SQLite runtime.
pub fn screen_catalog(broker_id: &str, market: &str) -> Result<Value, ScreenCatalogError> {
    let broker_id = broker_id.trim().to_ascii_lowercase();
    let market = market.trim().to_ascii_uppercase();
    let key = match broker_id.as_str() {
        "" | "futu" => {
            if !market.is_empty() && !matches!(market.as_str(), "HK" | "US" | "SH" | "SZ") {
                return Err(ScreenCatalogError::UnsupportedFutuMarket);
            }
            format!("futu|{market}")
        }
        "yfinance" => {
            if !market.is_empty() && market != "US" {
                return Err(ScreenCatalogError::UnsupportedEmbeddedMarket(broker_id));
            }
            format!("yfinance|{market}")
        }
        "akshare" => {
            if !market.is_empty() && !matches!(market.as_str(), "SH" | "SZ" | "CN" | "HK" | "US") {
                return Err(ScreenCatalogError::UnsupportedEmbeddedMarket(broker_id));
            }
            format!("akshare|{market}")
        }
        _ => return Err(ScreenCatalogError::BrokerUnavailable(broker_id)),
    };
    let fixture = catalog_fixture()?;
    fixture
        .catalogs
        .get(&key)
        .cloned()
        .ok_or_else(|| ScreenCatalogError::FixtureInvalid(format!("missing catalog {key}")))
}

fn catalog_fixture() -> Result<&'static CatalogFixture, ScreenCatalogError> {
    static FIXTURE: OnceLock<Result<CatalogFixture, String>> = OnceLock::new();
    FIXTURE
        .get_or_init(|| {
            let fixture: CatalogFixture = serde_json::from_str(CATALOG_FIXTURE)
                .map_err(|error| format!("decode fixture: {error}"))?;
            if fixture.version != CATALOG_FIXTURE_VERSION || fixture.catalogs.len() != 13 {
                return Err(format!(
                    "expected {CATALOG_FIXTURE_VERSION} with 13 catalogs, got {} with {}",
                    fixture.version,
                    fixture.catalogs.len()
                ));
            }
            Ok(fixture)
        })
        .as_ref()
        .map_err(|error| ScreenCatalogError::FixtureInvalid(error.clone()))
}

pub(crate) fn normalization_catalog(
    catalog_version: &str,
    market: &str,
) -> Result<&'static Value, String> {
    let key = if catalog_version == "futu-stock-screen-v1" {
        format!("futu|{market}")
    } else if catalog_version
        .trim()
        .eq_ignore_ascii_case("embedded-stock-screen-v1")
    {
        format!("akshare|{market}")
    } else {
        return Err(format!("unsupported catalog {catalog_version:?}"));
    };
    let fixture = catalog_fixture().map_err(|error| error.to_string())?;
    fixture
        .catalogs
        .get(&key)
        .ok_or_else(|| format!("missing normalization catalog {key}"))
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn catalog_variants_match_go_fixture_shape_and_validation() {
        for (broker, market, version) in [
            ("futu", "", "futu-stock-screen-v1"),
            ("futu", "US", "futu-stock-screen-v1"),
            ("yfinance", "", "embedded-stock-screen-v1"),
            ("yfinance", "US", "embedded-stock-screen-v1"),
            ("akshare", "CN", "embedded-stock-screen-v1"),
        ] {
            let catalog = screen_catalog(broker, market).expect("catalog");
            assert_eq!(catalog["version"], version);
            assert_eq!(catalog["provider"], broker);
            assert!(
                catalog["factors"]
                    .as_array()
                    .is_some_and(|factors| !factors.is_empty())
            );
            assert!(catalog.to_string().find("providerId").is_none());
        }
        assert_eq!(
            screen_catalog("futu", "SG"),
            Err(ScreenCatalogError::UnsupportedFutuMarket)
        );
        assert_eq!(
            screen_catalog("yfinance", "HK"),
            Err(ScreenCatalogError::UnsupportedEmbeddedMarket(
                "yfinance".into()
            ))
        );
        assert_eq!(
            screen_catalog("unknown", ""),
            Err(ScreenCatalogError::BrokerUnavailable("unknown".into()))
        );
    }
}
