use std::collections::BTreeMap;

use serde::{Deserialize, Serialize};

pub const BUILTIN_SOURCE_ID: &str = "builtin_rules";
pub const MANUAL_OVERRIDE_SOURCE_ID: &str = "manual_override";

const ALL_MARKETS: &[&str] = &["US", "HK", "CN", "SH", "SZ"];

/// Provider-neutral descriptor for one calendar source.  A provider adapter
/// may later supply these descriptors, but the domain does not own transport
/// URLs, clients, or provider SDK values.
#[derive(Clone, Debug, Eq, PartialEq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
pub struct CalendarSourceDescriptor {
    pub id: String,
    pub kind: String,
    pub authority: String,
    pub markets: Vec<String>,
}

/// Mutable manager-owned source status represented at the domain boundary.
/// Times deliberately stay as wire strings; runtime/transport layers choose
/// how to obtain and format them.
#[derive(Clone, Debug, Default, Eq, PartialEq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
pub struct CalendarSourceStatus {
    pub source_id: String,
    pub enabled: bool,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub last_success_at: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub last_failure_at: Option<String>,
    #[serde(skip_serializing_if = "String::is_empty")]
    pub last_error: String,
    #[serde(skip_serializing_if = "is_zero_i32")]
    pub consecutive_failures: i32,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub next_refresh_at: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub last_snapshot_fetched_at: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub last_probe_at: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub last_probe_success_at: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub last_probe_failure_at: Option<String>,
    #[serde(skip_serializing_if = "String::is_empty")]
    pub last_probe_status: String,
    #[serde(skip_serializing_if = "String::is_empty")]
    pub last_probe_error: String,
    #[serde(skip_serializing_if = "String::is_empty")]
    pub last_probe_market: String,
    #[serde(skip_serializing_if = "is_zero_i32")]
    pub last_probe_schedules: i32,
    #[serde(skip_serializing_if = "String::is_empty")]
    pub health_state: String,
    #[serde(skip_serializing_if = "String::is_empty")]
    pub health_fingerprint: String,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub last_alert_at: Option<String>,
    #[serde(skip_serializing_if = "String::is_empty")]
    pub last_alert_status: String,
    #[serde(skip_serializing_if = "String::is_empty")]
    pub last_alert_fingerprint: String,
}

/// Wire-compatible source row returned by the system calendar projection.
#[derive(Clone, Debug, Eq, PartialEq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
pub struct CalendarSourceProjection {
    pub id: String,
    pub kind: String,
    pub authority: String,
    pub markets: Vec<String>,
    pub enabled: bool,
    pub availability_note: String,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub last_success_at: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub last_failure_at: Option<String>,
    pub last_error: String,
    pub consecutive_failures: i32,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub next_refresh_at: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub last_snapshot_fetched_at: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub last_probe_at: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub last_probe_success_at: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub last_probe_failure_at: Option<String>,
    pub last_probe_status: String,
    pub last_probe_error: String,
    pub last_probe_market: String,
    pub last_probe_schedules: i32,
    pub health_state: String,
    pub health_fingerprint: String,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub last_alert_at: Option<String>,
    pub last_alert_status: String,
    pub last_alert_fingerprint: String,
}

#[derive(Clone, Debug, Eq, PartialEq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
pub struct CalendarSourcesSnapshot {
    pub sources: Vec<CalendarSourceProjection>,
}

impl CalendarSourceProjection {
    pub fn from_parts(
        descriptor: CalendarSourceDescriptor,
        enabled: bool,
        status: CalendarSourceStatus,
    ) -> Self {
        Self {
            id: descriptor.id.clone(),
            kind: descriptor.kind,
            authority: descriptor.authority,
            markets: normalize_markets(descriptor.markets),
            enabled,
            availability_note: source_availability_note(&descriptor.id),
            last_success_at: status.last_success_at,
            last_failure_at: status.last_failure_at,
            last_error: status.last_error,
            consecutive_failures: status.consecutive_failures,
            next_refresh_at: status.next_refresh_at,
            last_snapshot_fetched_at: status.last_snapshot_fetched_at,
            last_probe_at: status.last_probe_at,
            last_probe_success_at: status.last_probe_success_at,
            last_probe_failure_at: status.last_probe_failure_at,
            last_probe_status: status.last_probe_status,
            last_probe_error: status.last_probe_error,
            last_probe_market: status.last_probe_market,
            last_probe_schedules: status.last_probe_schedules,
            health_state: status.health_state,
            health_fingerprint: status.health_fingerprint,
            last_alert_at: status.last_alert_at,
            last_alert_status: status.last_alert_status,
            last_alert_fingerprint: status.last_alert_fingerprint,
        }
    }
}

/// Default descriptors owned by the calendar domain.  External adapters can
/// extend this list, but these builtins must always be present.
pub fn default_source_descriptors() -> Vec<CalendarSourceDescriptor> {
    let all_markets = || {
        ALL_MARKETS
            .iter()
            .map(|market| (*market).to_owned())
            .collect()
    };
    let mut descriptors = vec![
        CalendarSourceDescriptor {
            id: MANUAL_OVERRIDE_SOURCE_ID.to_owned(),
            kind: "manual_override".to_owned(),
            authority: "settings".to_owned(),
            markets: all_markets(),
        },
        CalendarSourceDescriptor {
            id: BUILTIN_SOURCE_ID.to_owned(),
            kind: "builtin_rules".to_owned(),
            authority: "builtin".to_owned(),
            markets: all_markets(),
        },
        CalendarSourceDescriptor {
            id: "nyse_official".to_owned(),
            kind: "official_html".to_owned(),
            authority: "NYSE".to_owned(),
            markets: vec!["US".to_owned()],
        },
        CalendarSourceDescriptor {
            id: "nasdaq_verifier".to_owned(),
            kind: "official_html".to_owned(),
            authority: "Nasdaq".to_owned(),
            markets: vec!["US".to_owned()],
        },
        CalendarSourceDescriptor {
            id: "hk_gov_1823_ical".to_owned(),
            kind: "official_ical".to_owned(),
            authority: "GovHK 1823".to_owned(),
            markets: vec!["HK".to_owned()],
        },
        CalendarSourceDescriptor {
            id: "mainland_official_notice".to_owned(),
            kind: "official_html".to_owned(),
            authority: "Shanghai Stock Exchange".to_owned(),
            markets: vec!["CN".to_owned(), "SH".to_owned(), "SZ".to_owned()],
        },
    ];
    descriptors.sort_by(|left, right| left.id.cmp(&right.id));
    descriptors
}

/// Mirrors the settings normalizer's source-id compatibility alias and
/// preserve-first deduplication.  IDs are not lowercased because Go's source
/// policy treats source IDs as stable case-sensitive identifiers.
pub fn normalize_source_ids(values: impl IntoIterator<Item = String>) -> Vec<String> {
    let mut normalized = Vec::new();
    for value in values {
        let value = value.trim();
        let value = if value == "hkex_official" {
            "hk_gov_1823_ical"
        } else {
            value
        };
        if !value.is_empty() && !normalized.iter().any(|item| item == value) {
            normalized.push(value.to_owned());
        }
    }
    normalized
}

pub fn source_enabled(source_id: &str, enabled_source_ids: &[String]) -> bool {
    matches!(source_id, BUILTIN_SOURCE_ID | MANUAL_OVERRIDE_SOURCE_ID)
        || enabled_source_ids.iter().any(|value| value == source_id)
}

pub fn project_sources(
    descriptors: impl IntoIterator<Item = CalendarSourceDescriptor>,
    enabled_source_ids: impl IntoIterator<Item = String>,
    statuses: &BTreeMap<String, CalendarSourceStatus>,
) -> Vec<CalendarSourceProjection> {
    let enabled_source_ids = normalize_source_ids(enabled_source_ids);
    let mut descriptors = descriptors.into_iter().collect::<Vec<_>>();
    descriptors.sort_by(|left, right| left.id.cmp(&right.id));
    descriptors
        .into_iter()
        .map(|descriptor| {
            let enabled = source_enabled(&descriptor.id, &enabled_source_ids);
            let status =
                statuses
                    .get(&descriptor.id)
                    .cloned()
                    .unwrap_or_else(|| CalendarSourceStatus {
                        source_id: descriptor.id.clone(),
                        ..CalendarSourceStatus::default()
                    });
            CalendarSourceProjection::from_parts(descriptor, enabled, status)
        })
        .collect()
}

pub fn project_default_sources(
    enabled_source_ids: impl IntoIterator<Item = String>,
    statuses: &BTreeMap<String, CalendarSourceStatus>,
) -> CalendarSourcesSnapshot {
    CalendarSourcesSnapshot {
        sources: project_sources(default_source_descriptors(), enabled_source_ids, statuses),
    }
}

pub fn source_availability_note(source_id: &str) -> String {
    match source_id.trim() {
        "nyse_official" => "Primary US source. The NYSE multi-year holiday table is parsed directly and Nasdaq remains a secondary verifier.".to_owned(),
        "nasdaq_verifier" => "Secondary US verifier. Nasdaq currently serves automated requests unreliably in some environments, so it stays available as an opt-in cross-check instead of a default source.".to_owned(),
        "hk_gov_1823_ical" => "Official Hong Kong holiday iCal. It publishes a rolling multi-year window, so future-year coverage may appear later than the current anchor year.".to_owned(),
        "mainland_official_notice" => "Current adapter targets the SSE English Trading Schedule page. Default policy keeps it disabled until verified current-year mainland schedules are available.".to_owned(),
        BUILTIN_SOURCE_ID => "Offline fallback rules bundled with the application. They remain available even when all external sources fail.".to_owned(),
        MANUAL_OVERRIDE_SOURCE_ID => "Operator-defined overrides always take precedence over remote and builtin calendars.".to_owned(),
        _ => String::new(),
    }
}

fn normalize_markets(values: Vec<String>) -> Vec<String> {
    let mut normalized = Vec::new();
    for value in values {
        let value = value.trim().to_ascii_uppercase();
        if !value.is_empty() && !normalized.contains(&value) {
            normalized.push(value);
        }
    }
    normalized
}

const fn is_zero_i32(value: &i32) -> bool {
    *value == 0
}

#[cfg(test)]
mod tests {
    use super::*;
    use serde::Deserialize;
    use serde_json::{Value, json};

    #[derive(Debug, Deserialize)]
    #[serde(rename_all = "camelCase")]
    struct CalendarSourcesFixture {
        version: String,
        zero_status: Value,
        recovered_status: Value,
        default_sources: Vec<Value>,
    }

    #[test]
    fn default_sources_match_go_ids_notes_and_stable_order() {
        let descriptors = default_source_descriptors();
        let ids = descriptors
            .iter()
            .map(|descriptor| descriptor.id.as_str())
            .collect::<Vec<_>>();
        assert_eq!(
            ids,
            [
                "builtin_rules",
                "hk_gov_1823_ical",
                "mainland_official_notice",
                "manual_override",
                "nasdaq_verifier",
                "nyse_official"
            ]
        );
        assert!(descriptors.iter().all(|descriptor| {
            !descriptor.markets.is_empty() && !source_availability_note(&descriptor.id).is_empty()
        }));
    }

    #[test]
    fn settings_source_ids_trim_alias_deduplicate_and_project_enabled() {
        let enabled = normalize_source_ids([
            " hkex_official ".to_owned(),
            "nyse_official".to_owned(),
            "nyse_official".to_owned(),
            " ".to_owned(),
        ]);
        assert_eq!(enabled, ["hk_gov_1823_ical", "nyse_official"]);
        let snapshot = project_default_sources(enabled, &BTreeMap::new());
        let by_id = snapshot
            .sources
            .iter()
            .map(|source| (source.id.as_str(), source))
            .collect::<BTreeMap<_, _>>();
        assert!(by_id["builtin_rules"].enabled);
        assert!(by_id["manual_override"].enabled);
        assert!(by_id["nyse_official"].enabled);
        assert!(!by_id["nasdaq_verifier"].enabled);
        assert_eq!(
            by_id["hk_gov_1823_ical"].availability_note,
            source_availability_note("hk_gov_1823_ical")
        );
    }

    #[test]
    fn status_is_projected_without_losing_runtime_fields() {
        let statuses = BTreeMap::from([(
            "nyse_official".to_owned(),
            CalendarSourceStatus {
                source_id: "nyse_official".to_owned(),
                last_success_at: Some("2026-08-20T01:02:03Z".to_owned()),
                health_state: "healthy".to_owned(),
                consecutive_failures: 2,
                ..CalendarSourceStatus::default()
            },
        )]);
        let snapshot = project_default_sources(Vec::<String>::new(), &statuses);
        let source = snapshot
            .sources
            .iter()
            .find(|source| source.id == "nyse_official")
            .expect("NYSE source");
        assert_eq!(
            source.last_success_at.as_deref(),
            Some("2026-08-20T01:02:03Z")
        );
        assert_eq!(source.health_state, "healthy");
        assert_eq!(source.consecutive_failures, 2);
        assert_eq!(source.last_error, "");
    }

    #[test]
    fn source_wire_rejects_unknown_fields_and_keeps_zero_status_shape() {
        let snapshot = project_default_sources(Vec::<String>::new(), &BTreeMap::new());
        let value = serde_json::to_value(&snapshot.sources[0]).expect("serialize source");
        for field in [
            "id",
            "kind",
            "authority",
            "markets",
            "enabled",
            "availabilityNote",
            "lastProbeStatus",
            "healthState",
        ] {
            assert!(value.get(field).is_some(), "missing wire field {field}");
        }
        for field in [
            "lastSuccessAt",
            "lastFailureAt",
            "nextRefreshAt",
            "lastSnapshotFetchedAt",
            "lastProbeAt",
            "lastProbeSuccessAt",
            "lastProbeFailureAt",
            "lastAlertAt",
        ] {
            assert!(value.get(field).is_none(), "zero time field {field} leaked");
        }
        let mut unknown = value;
        unknown["unexpected"] = json!(true);
        assert!(serde_json::from_value::<CalendarSourceProjection>(unknown).is_err());
    }

    #[test]
    fn nonzero_time_fields_use_wire_rfc3339_and_zero_status_omits_them() {
        let zero = serde_json::to_value(CalendarSourceStatus {
            source_id: "nyse_official".to_owned(),
            ..CalendarSourceStatus::default()
        })
        .expect("serialize zero source status");
        assert_eq!(zero["sourceId"], "nyse_official");
        assert_eq!(zero["enabled"], false);
        for field in [
            "lastSuccessAt",
            "lastFailureAt",
            "nextRefreshAt",
            "lastSnapshotFetchedAt",
            "lastProbeAt",
            "lastProbeSuccessAt",
            "lastProbeFailureAt",
            "lastAlertAt",
        ] {
            assert!(
                zero.get(field).is_none(),
                "zero status field {field} leaked"
            );
        }
        let nonzero = serde_json::to_value(CalendarSourceStatus {
            source_id: "nyse_official".to_owned(),
            enabled: true,
            last_success_at: Some("2026-06-23T09:30:00Z".to_owned()),
            ..CalendarSourceStatus::default()
        })
        .expect("serialize nonzero source status");
        assert_eq!(nonzero["enabled"], true);
        assert_eq!(nonzero["lastSuccessAt"], "2026-06-23T09:30:00Z");
    }

    #[test]
    fn custom_descriptors_are_normalized_and_sorted_but_not_fabricated() {
        let descriptors = vec![
            CalendarSourceDescriptor {
                id: "z".to_owned(),
                kind: "custom".to_owned(),
                authority: "custom".to_owned(),
                markets: vec![" us ".to_owned(), "US".to_owned(), "".to_owned()],
            },
            CalendarSourceDescriptor {
                id: "a".to_owned(),
                kind: "custom".to_owned(),
                authority: "custom".to_owned(),
                markets: vec!["hk".to_owned()],
            },
        ];
        let result = project_sources(descriptors, Vec::<String>::new(), &BTreeMap::new());
        assert_eq!(result[0].id, "a");
        assert_eq!(result[1].id, "z");
        assert_eq!(result[1].markets, ["US"]);
        assert_eq!(result[1].availability_note, "");
    }

    #[test]
    fn calendar_source_projection_matches_go_reference_fixture() {
        let fixture: CalendarSourcesFixture = serde_json::from_str(include_str!(
            "../../../tests/fixtures/compatibility/api-transport/calendar-sources.json"
        ))
        .expect("calendar source fixture");
        assert_eq!(fixture.version, "stage9.calendar-sources.v1");

        let zero_status = serde_json::to_value(CalendarSourceStatus {
            source_id: "nyse_official".to_owned(),
            enabled: true,
            ..CalendarSourceStatus::default()
        })
        .expect("encode zero source status");
        assert_eq!(zero_status, fixture.zero_status);

        let recovered_status = serde_json::to_value(CalendarSourceStatus {
            source_id: "unknown_source".to_owned(),
            enabled: true,
            last_success_at: Some("2026-06-23T09:30:00Z".to_owned()),
            last_error: "recovered".to_owned(),
            consecutive_failures: 2,
            last_probe_status: "recovered".to_owned(),
            last_probe_market: "US".to_owned(),
            last_probe_schedules: 8,
            health_state: "healthy".to_owned(),
            health_fingerprint: "fingerprint-1".to_owned(),
            last_alert_status: "recovered".to_owned(),
            last_alert_fingerprint: "alert-1".to_owned(),
            ..CalendarSourceStatus::default()
        })
        .expect("encode recovered source status");
        assert_eq!(recovered_status, fixture.recovered_status);

        let projected = project_default_sources(Vec::<String>::new(), &BTreeMap::new());
        let projected = serde_json::to_value(projected.sources).expect("encode source projection");
        assert_eq!(
            projected,
            serde_json::to_value(fixture.default_sources).expect("encode fixture sources")
        );
    }
}
