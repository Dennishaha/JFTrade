//! The reviewed Go MCP tool schemas projected for the Rust listener.
//!
//! MCP is a wire-facing boundary, so descriptors must not fall back to an
//! open-ended object when the production executor is unavailable.  This
//! catalog intentionally contains only deterministic JSON-schema builders;
//! readiness is projected separately by `product_mcp_protocol`.

use serde_json::{Map, Value, json};

type Properties = Map<String, Value>;

/// Return a fresh schema for one reviewed MCP tool.
pub(crate) fn schema_for(name: &str) -> Value {
    product_schema_for(name)
        .or_else(|| core_schema_for(name))
        .unwrap_or_else(|| panic!("unmapped reviewed MCP tool schema: {name}"))
}

include!("product_mcp_schema_catalog_dispatch.rs");
include!("product_mcp_schema_catalog_product.rs");
include!("product_mcp_schema_catalog_core.rs");
include!("product_mcp_schema_catalog_helpers.rs");
include!("product_mcp_schema_validation.rs");
