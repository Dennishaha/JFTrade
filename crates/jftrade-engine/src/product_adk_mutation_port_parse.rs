use std::collections::BTreeMap;

use serde_json::{Map, Value};

use super::{AdkMutationInput, AdkMutationOperation, ErrorSpec};

pub(super) fn parse_input(
    method: &str,
    path: &str,
    body: Option<&[u8]>,
    headers: &BTreeMap<String, String>,
) -> Result<AdkMutationInput, ErrorSpec> {
    let (operation, identifiers) = super::parse_route(method, path)?;
    let body = if super::accepts_workflow_inputs(operation) {
        super::parse_workflow_inputs(body)?
    } else if super::ignores_body(operation) {
        Value::Object(Map::new())
    } else {
        super::parse_object_body(
            body,
            super::body_required(operation),
            super::body_error_message(operation),
        )?
    };
    let webhook_secret = (operation == AdkMutationOperation::RunWorkflowWebhook)
        .then(|| super::webhook_secret(headers))
        .flatten();
    Ok(AdkMutationInput {
        operation,
        identifiers,
        body,
        webhook_secret,
    })
}
