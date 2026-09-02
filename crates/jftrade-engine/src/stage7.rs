use std::collections::BTreeMap;

use jftrade_api::{
    ApiFailure, ApiOutput, ApiPort, ApiRequest, DEFAULT_WEBSOCKET_LIMIT, PortFuture, RouteCatalog,
    RouteCatalogError, RouteSpec, SseEvent, canonical_origin, encode_comment, encode_event,
    encode_retry,
};
use jftrade_calendar::{CalendarPolicy, SessionWindow, normalize_policy};
use jftrade_datamanagement::{CleanupCandidate, CleanupPreview, preview_cleanup, verify_execute};
use jftrade_research::{PresetUpdate, ScreenPreset, create_preset, update_preset};
use jftrade_settings::{
    ProviderSelectionPlan, SecuritySettingsInput, SecurityUpdatePlan, plan_provider_selection,
    plan_security_update,
};
use jftrade_watchlist::{MembershipPlan, normalize_limit, plan_membership_replace};
use serde::{Deserialize, Serialize};
use serde_json::{Value, json};
use thiserror::Error;

#[derive(Clone, Debug, PartialEq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
pub struct Stage7Input {
    pub version: String,
    pub routes: Vec<RouteSpec>,
    pub route_probes: Vec<RouteProbe>,
    pub research: ResearchProbe,
    pub watchlist: WatchlistProbe,
    pub settings: SettingsProbe,
    pub calendar: CalendarProbe,
    pub cleanup: CleanupProbe,
}

#[derive(Clone, Debug, Eq, PartialEq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
pub struct RouteProbe {
    pub method: String,
    pub path: String,
}

#[derive(Clone, Debug, PartialEq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
pub struct ResearchProbe {
    pub preset_id: String,
    pub name: String,
    pub definition: Value,
    pub update: PresetUpdate,
}

#[derive(Clone, Debug, Eq, PartialEq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
pub struct WatchlistProbe {
    pub instrument_id: String,
    pub group_ids: Vec<String>,
    pub new_group_names: Vec<String>,
    pub expected_revision: u64,
    pub requested_limit: usize,
}

#[derive(Clone, Debug, Eq, PartialEq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
pub struct SettingsProbe {
    pub security: SecuritySettingsInput,
    pub provider_id: String,
}

#[derive(Clone, Debug, Eq, PartialEq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
pub struct CalendarProbe {
    pub market: String,
    pub sources: Vec<String>,
    pub sessions: Vec<SessionWindow>,
}

#[derive(Clone, Debug, Eq, PartialEq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
pub struct CleanupProbe {
    pub database_id: String,
    pub preview_candidates: Vec<CleanupCandidate>,
    pub execute_candidates: Vec<CleanupCandidate>,
}

#[derive(Clone, Debug, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct Stage7Output {
    pub version: String,
    pub routes: Vec<RouteSpec>,
    pub route_groups: BTreeMap<String, usize>,
    pub route_probes: Vec<RouteProbeResult>,
    pub transport: TransportProjection,
    pub research: ScreenPreset,
    pub watchlist: MembershipPlan,
    pub normalized_page_limit: usize,
    pub security: SecurityUpdatePlan,
    pub provider: ProviderSelectionPlan,
    pub calendar: CalendarPolicy,
    pub cleanup: CleanupProjection,
}

#[derive(Clone, Debug, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct RouteProbeResult {
    pub method: String,
    pub path: String,
    pub allowed: bool,
}

#[derive(Clone, Debug, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct TransportProjection {
    pub canonical_origin: Option<String>,
    pub sse: String,
    pub websocket_limit: usize,
}

#[derive(Clone, Debug, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct CleanupProjection {
    pub preview: CleanupPreview,
    pub approved_candidates: Vec<CleanupCandidate>,
}

pub struct Stage7Assembly {
    routes: RouteCatalog,
}

impl Stage7Assembly {
    pub fn new(routes: Vec<RouteSpec>) -> Result<Self, Stage7Error> {
        Ok(Self {
            routes: RouteCatalog::new(routes)?,
        })
    }

    pub fn routes(&self) -> &[RouteSpec] {
        self.routes.routes()
    }

    pub fn evaluate(&self, input: Stage7Input) -> Result<Stage7Output, Stage7Error> {
        let research = create_preset(
            &input.research.preset_id,
            &input.research.name,
            input.research.definition,
        )?;
        let research = update_preset(&research, input.research.update)?;
        let watchlist = plan_membership_replace(
            &input.watchlist.instrument_id,
            input.watchlist.group_ids,
            input.watchlist.new_group_names,
            input.watchlist.expected_revision,
        )?;
        let normalized_page_limit = normalize_limit(input.watchlist.requested_limit);
        let security = plan_security_update(input.settings.security)?;
        let provider = plan_provider_selection(&input.settings.provider_id)?;
        let calendar = normalize_policy(
            &input.calendar.market,
            input.calendar.sources,
            input.calendar.sessions,
        )?;
        let preview = preview_cleanup(
            &input.cleanup.database_id,
            input.cleanup.preview_candidates,
            None,
        )?;
        let approved_candidates = verify_execute(&preview, input.cleanup.execute_candidates, None)?;
        let route_probes = input
            .route_probes
            .into_iter()
            .map(|probe| RouteProbeResult {
                allowed: self.routes.allows(&probe.method, &probe.path),
                method: probe.method,
                path: probe.path,
            })
            .collect();
        let mut route_groups = BTreeMap::new();
        for route in self.routes.routes() {
            *route_groups.entry(route_group(&route.path)).or_insert(0) += 1;
        }
        Ok(Stage7Output {
            version: input.version,
            routes: self.routes.routes().to_vec(),
            route_groups,
            route_probes,
            transport: TransportProjection {
                canonical_origin: canonical_origin(" HTTPS://LOCALHOST:3000/path "),
                sse: format!(
                    "{}{}{}",
                    encode_retry(3000),
                    encode_event(&SseEvent {
                        id: Some("42".to_owned()),
                        data: json!({"ready": true}),
                    })
                    .expect("fixed SSE JSON is serializable"),
                    encode_comment("heartbeat"),
                ),
                websocket_limit: DEFAULT_WEBSOCKET_LIMIT,
            },
            research,
            watchlist,
            normalized_page_limit,
            security,
            provider,
            calendar,
            cleanup: CleanupProjection {
                preview,
                approved_candidates,
            },
        })
    }
}

impl ApiPort for Stage7Assembly {
    fn dispatch(&self, request: ApiRequest) -> PortFuture<'_> {
        Box::pin(async move {
            if !self.routes.allows(&request.method, &request.path) {
                return Err(ApiFailure::new(
                    404,
                    "NOT_FOUND",
                    format!("unknown endpoint {}", request.path),
                ));
            }
            Ok(ApiOutput::Json(json!({
                "owner": "rust-stage7-shadow",
                "operation": format!("{} {}", request.method, request.path),
                "requestId": request.request_id,
            })))
        })
    }
}

fn route_group(path: &str) -> String {
    path.trim_start_matches("/api/v1/")
        .split('/')
        .next()
        .unwrap_or("_root")
        .to_owned()
}

#[derive(Debug, Error)]
pub enum Stage7Error {
    #[error("invalid route catalog: {0:?}")]
    Route(#[from] RouteCatalogError),
    #[error(transparent)]
    Research(#[from] jftrade_research::ResearchError),
    #[error(transparent)]
    Watchlist(#[from] jftrade_watchlist::WatchlistError),
    #[error(transparent)]
    Settings(#[from] jftrade_settings::SettingsError),
    #[error(transparent)]
    Calendar(#[from] jftrade_calendar::CalendarError),
    #[error(transparent)]
    DataManagement(#[from] jftrade_datamanagement::MaintenanceError),
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn assembly_rejects_unregistered_operations() {
        let assembly = Stage7Assembly::new(vec![RouteSpec {
            method: "GET".to_owned(),
            path: "/api/v1/settings".to_owned(),
        }])
        .expect("assembly");
        assert!(assembly.routes.allows("GET", "/api/v1/settings"));
        assert!(!assembly.routes.allows("DELETE", "/api/v1/settings"));
    }
}
