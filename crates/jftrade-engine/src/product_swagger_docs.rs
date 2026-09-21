//! Frozen Swagger 2.0 contract for the offline `/swagger` documentation panel.
//!
//! Parity baseline `go:452dea11`: `internal/app/apiserver/servercore/openapi.go`
//! served the `docs/swagger` bundle (Swagger 2.0, title `JFTrade Debug API`) at
//! `/swagger/doc.json`. Rust embeds the contract source that
//! `pnpm run check:contracts` validates against the production route manifest,
//! so the panel and the console read one document instead of a second copy.

use jftrade_api::SwaggerDocs;

/// Swagger 2.0 contract served at `/swagger/doc.json`.
pub(crate) const SWAGGER_DOCUMENT: &str = include_str!("../../../contracts/openapi/openapi.json");

/// Documentation panel injected into the product transport state.
pub(crate) fn swagger_docs() -> SwaggerDocs {
    SwaggerDocs::new(SWAGGER_DOCUMENT)
}

#[cfg(test)]
mod tests {
    use super::*;
    use serde_json::Value;

    // Parity: go:452dea11:internal/app/apiserver/servercoretest/swagger_openapi_test.go:72 TestOpenAPISpecExposesCorePaths
    #[test]
    fn swagger_docs_embed_the_frozen_debug_api_contract() {
        let spec: Value =
            serde_json::from_str(SWAGGER_DOCUMENT).expect("frozen Swagger 2.0 contract");
        assert_eq!(spec["swagger"], "2.0");
        assert_eq!(spec["info"]["title"], "JFTrade Debug API");
        for path in [
            "/api/v1/system/status",
            "/api/v1/market-data/candles/{market}/{symbol}",
            "/api/v1/ws/live",
        ] {
            assert!(spec["paths"].get(path).is_some(), "missing {path}");
        }
        assert_eq!(
            swagger_docs(),
            SwaggerDocs::new(SWAGGER_DOCUMENT),
            "product wiring must serve the embedded contract"
        );
    }
}
