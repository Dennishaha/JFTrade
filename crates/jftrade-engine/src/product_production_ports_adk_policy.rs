// Permission-policy projection for the production tool catalog.
//
// Textually included inside `impl ProductionToolCatalog` by
// `product_production_ports_adk.rs`, which keeps the production file under the
// repository's 800-line architecture limit.

/// Whether a call to `name` has to wait for the operator in `mode`.
///
/// The decision uses the same descriptor fields the reference passes to
/// `ToolRequiresApproval` (permission class, risk level, explicit per-mode
/// list) and the agent's resolved permission mode.  A tool that is missing
/// from the catalog still requires confirmation: failing closed is the only
/// safe default when the metadata is unavailable.
impl ProductionToolCatalog {
    /// A keyed descriptor requires a handler read of the invocation context;
    /// the runtime must not infer this capability from the tool's name.
    pub(crate) fn requires_idempotency_key(&self, name: &str) -> bool {
        self.values().into_iter().any(|tool| {
            tool.get("id").and_then(Value::as_str) == Some(name)
                && tool.get("idempotencyMode").and_then(Value::as_str) == Some("keyed")
        })
    }

    pub(crate) fn requires_approval(&self, name: &str, mode: &str) -> bool {
        let Some(tool) = self
            .values()
            .into_iter()
            .find(|tool| tool.get("id").and_then(Value::as_str) == Some(name))
        else {
            return true;
        };
        let string_list = |field: &str| {
            tool.get(field)
                .and_then(Value::as_array)
                .map(|modes| {
                    modes
                        .iter()
                        .filter_map(Value::as_str)
                        .map(str::to_owned)
                        .collect::<Vec<_>>()
                })
                .unwrap_or_default()
        };
        let descriptor = jftrade_assistant::ToolDescriptor {
            name: name.to_owned(),
            display_name: String::new(),
            description: String::new(),
            category: String::new(),
            permission: tool
                .get("permission")
                .and_then(Value::as_str)
                .unwrap_or_default()
                .to_owned(),
            risk_level: tool
                .get("riskLevel")
                .and_then(Value::as_str)
                // A descriptor that lost its risk level is treated as the highest
                // risk rather than as an automatically executable read.
                .unwrap_or("critical")
                .to_owned(),
            idempotency_mode: jftrade_assistant::ToolIdempotencyMode::ReplaySafe,
            allowed_modes: string_list("allowedModes"),
            requires_approval_in: string_list("requiresApprovalIn"),
            input_schema: Value::Null,
        };
        jftrade_assistant::tool_requires_approval(&descriptor, mode)
    }
    }
