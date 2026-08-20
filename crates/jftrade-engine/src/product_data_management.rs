use std::env;
use std::path::Path;
use std::sync::Arc;

use jftrade_datamanagement::{
    CleanupPreviewIdPort, CleanupPreviewService, DATABASE_ADK, DATABASE_ADK_ARTIFACT,
    DATABASE_ADK_SESSION, DATABASE_BACKTEST, DATABASE_BACKTEST_RUNS, DATABASE_EXECUTION,
    DATABASE_RESEARCH, DATABASE_STRATEGY, DATABASE_WATCHLIST, ManagedDatabasePaths,
    OverviewService, managed_database_descriptors,
};
use jftrade_store_sqlite::{ManagedDatabaseCleanupCandidateStore, ManagedDatabaseOverviewStore};

const REBUILD_MARKER_FILENAME: &str = "database-rebuild.json";

pub fn overview_service(settings_path: &Path) -> OverviewService {
    overview_service_with_lookup(settings_path, |name| env::var(name).ok())
}

pub fn cleanup_preview_service(settings_path: &Path) -> CleanupPreviewService {
    cleanup_preview_service_with_lookup(settings_path, |name| env::var(name).ok())
}

fn overview_service_with_lookup(
    settings_path: &Path,
    lookup: impl Fn(&str) -> Option<String>,
) -> OverviewService {
    let (descriptors, marker_path) = database_descriptors(settings_path, lookup);
    OverviewService::new(
        descriptors,
        Arc::new(ManagedDatabaseOverviewStore::new(marker_path)),
    )
}

fn cleanup_preview_service_with_lookup(
    settings_path: &Path,
    lookup: impl Fn(&str) -> Option<String>,
) -> CleanupPreviewService {
    let (descriptors, marker_path) = database_descriptors(settings_path, lookup);
    let overview = Arc::new(ManagedDatabaseOverviewStore::new(marker_path));
    CleanupPreviewService::new(
        descriptors,
        overview,
        Arc::new(ManagedDatabaseCleanupCandidateStore),
        Arc::new(SystemCleanupPreviewIds),
    )
}

fn database_descriptors(
    settings_path: &Path,
    lookup: impl Fn(&str) -> Option<String>,
) -> (
    Vec<jftrade_datamanagement::DatabaseDescriptor>,
    std::path::PathBuf,
) {
    let root = settings_path
        .parent()
        .filter(|path| !path.as_os_str().is_empty())
        .unwrap_or_else(|| Path::new(""));
    let path = |environment: &str, filename: &str| {
        lookup(environment)
            .filter(|value| !value.trim().is_empty())
            .map(|value| value.trim().to_owned())
            .unwrap_or_else(|| root.join(filename).to_string_lossy().into_owned())
    };
    let adk_session = path("JFTRADE_ADK_SESSION_DB", "adk-session.db");
    let adk_artifact = Path::new(&adk_session)
        .parent()
        .filter(|parent| !parent.as_os_str().is_empty())
        .unwrap_or_else(|| Path::new(""))
        .join("adk-artifact.db")
        .to_string_lossy()
        .into_owned();
    let paths = ManagedDatabasePaths::new([
        (
            DATABASE_BACKTEST,
            path("JFTRADE_BACKTEST_DB", "backtest.db"),
        ),
        (
            DATABASE_BACKTEST_RUNS,
            path("JFTRADE_BACKTEST_RUN_DB", "backtest-runs.db"),
        ),
        (
            DATABASE_STRATEGY,
            path("JFTRADE_STRATEGY_RUNTIME_DB", "strategy-runtime.db"),
        ),
        (
            DATABASE_EXECUTION,
            path("JFTRADE_EXECUTION_ORDER_DB", "execution-orders.db"),
        ),
        (DATABASE_ADK, path("JFTRADE_ADK_DB", "adk.db")),
        (DATABASE_ADK_SESSION, adk_session),
        (DATABASE_ADK_ARTIFACT, adk_artifact),
        (
            DATABASE_WATCHLIST,
            path("JFTRADE_WATCHLIST_DB", "watchlists.db"),
        ),
        (
            DATABASE_RESEARCH,
            path("JFTRADE_RESEARCH_DB", "research.db"),
        ),
    ]);
    (
        managed_database_descriptors(&paths),
        root.join(REBUILD_MARKER_FILENAME),
    )
}

#[derive(Debug, Default)]
struct SystemCleanupPreviewIds;

impl CleanupPreviewIdPort for SystemCleanupPreviewIds {
    fn new_preview_id(&self) -> Result<String, String> {
        let mut bytes = [0_u8; 16];
        getrandom::fill(&mut bytes).map_err(|error| error.to_string())?;
        let mut id = String::with_capacity(bytes.len() * 2);
        for byte in bytes {
            use std::fmt::Write as _;
            write!(&mut id, "{byte:02x}").map_err(|error| error.to_string())?;
        }
        Ok(id)
    }
}

#[cfg(test)]
mod tests {
    use std::collections::BTreeMap;
    use std::str::FromStr;
    use std::sync::Arc;

    use jftrade_datamanagement::{CleanupPreviewRequest, OverviewRequest};
    use jftrade_kernel::WireTimestamp;
    use jftrade_store_sqlite::ManagedDatabaseCleanupCandidateStore;
    use serde::Deserialize;
    use serde_json::{Value, json};

    use super::*;

    #[test]
    fn paths_follow_go_environment_overrides_and_adk_artifact_lifecycle() {
        let overrides = BTreeMap::from([
            ("JFTRADE_BACKTEST_DB", "/override/backtest.db".to_owned()),
            (
                "JFTRADE_ADK_SESSION_DB",
                "/override/adk/session.db".to_owned(),
            ),
        ]);
        let service = overview_service_with_lookup(Path::new("/runtime/settings.json"), |name| {
            overrides.get(name).cloned()
        });
        let response = service
            .overview(
                OverviewRequest {
                    summary_only: true,
                    ..OverviewRequest::default()
                },
                "2026-08-20T00:00:00Z".to_owned(),
            )
            .expect("overview");
        let by_id = response
            .databases
            .into_iter()
            .map(|database| (database.descriptor.id.clone(), database.descriptor.path))
            .collect::<BTreeMap<_, _>>();
        assert_eq!(by_id[DATABASE_BACKTEST], "/override/backtest.db");
        assert_eq!(by_id[DATABASE_STRATEGY], "/runtime/strategy-runtime.db");
        assert_eq!(by_id[DATABASE_ADK_SESSION], "/override/adk/session.db");
        assert_eq!(
            by_id[DATABASE_ADK_ARTIFACT],
            "/override/adk/adk-artifact.db"
        );
    }

    #[test]
    fn stage9_data_management_overview_matches_current_go_owner() {
        let Some(root) = std::env::var_os("JFTRADE_STAGE9_DATA_MANAGEMENT_ROOT") else {
            return;
        };
        let Some(reference_path) = std::env::var_os("JFTRADE_STAGE9_DATA_MANAGEMENT_REFERENCE")
        else {
            return;
        };
        let root = Path::new(&root);
        let service = overview_service_with_lookup(&root.join("settings.json"), |_| None);
        let checked_at = "2026-08-20T00:00:00Z".to_owned();
        let all = service
            .overview(OverviewRequest::default(), checked_at.clone())
            .expect("complete overview");
        let summary = service
            .overview(
                OverviewRequest {
                    summary_only: true,
                    ..OverviewRequest::default()
                },
                checked_at.clone(),
            )
            .expect("summary overview");
        let filtered = service
            .overview(
                OverviewRequest {
                    database_id: DATABASE_STRATEGY.to_owned(),
                    ..OverviewRequest::default()
                },
                checked_at.clone(),
            )
            .expect("filtered overview");
        let unknown_error = service
            .overview(
                OverviewRequest {
                    database_id: "unknown".to_owned(),
                    ..OverviewRequest::default()
                },
                checked_at,
            )
            .expect_err("unknown database must fail")
            .to_string();
        let actual = json!({
            "version": "stage9.data-management-overview.v1",
            "all": all,
            "summary": summary,
            "filtered": filtered,
            "unknownError": unknown_error,
        });
        let expected: Value = serde_json::from_slice(
            &std::fs::read(reference_path).expect("read Go data-management reference"),
        )
        .expect("decode Go data-management reference");
        assert_eq!(actual, expected);
    }

    #[derive(Debug, Deserialize)]
    #[serde(rename_all = "camelCase")]
    struct CleanupPreviewReference {
        version: String,
        cases: Vec<CleanupPreviewReferenceCase>,
    }

    #[derive(Debug, Deserialize)]
    #[serde(rename_all = "camelCase")]
    struct CleanupPreviewReferenceCase {
        name: String,
        request: CleanupPreviewRequest,
        evaluated_at: Option<String>,
        response: Option<Value>,
        error: Option<String>,
    }

    #[test]
    fn stage9_data_management_cleanup_preview_matches_current_go_owner() {
        let Some(root) = std::env::var_os("JFTRADE_STAGE9_DATA_MANAGEMENT_CLEANUP_ROOT") else {
            return;
        };
        let Some(reference_path) =
            std::env::var_os("JFTRADE_STAGE9_DATA_MANAGEMENT_CLEANUP_REFERENCE")
        else {
            return;
        };
        let root = std::path::Path::new(&root);
        let settings_path = root.join("settings.json");
        let (descriptors, marker_path) = database_descriptors(&settings_path, |_| None);
        let overview = Arc::new(ManagedDatabaseOverviewStore::new(marker_path));
        let service = jftrade_datamanagement::CleanupPreviewService::new(
            descriptors,
            overview,
            Arc::new(ManagedDatabaseCleanupCandidateStore),
            Arc::new(TestCleanupPreviewIds),
        );
        let reference: CleanupPreviewReference = serde_json::from_slice(
            &std::fs::read(reference_path).expect("read Go cleanup preview reference"),
        )
        .expect("decode Go cleanup preview reference");
        assert_eq!(
            reference.version,
            "stage9.data-management-cleanup-preview.v1"
        );
        assert!(reference.cases.len() >= 7);

        for case in reference.cases {
            let result = match case.response {
                Some(expected) => {
                    let evaluated_at = case
                        .evaluated_at
                        .as_deref()
                        .expect("valid cleanup case has evaluatedAt");
                    let now = WireTimestamp::from_str(evaluated_at)
                        .expect("parse cleanup evaluation instant")
                        .into_inner();
                    let response = service
                        .preview_at(case.request, now)
                        .unwrap_or_else(|error| panic!("preview {}: {error}", case.name));
                    assert_eq!(response.preview_id.len(), 32, "preview {} id", case.name);
                    assert!(
                        response
                            .preview_id
                            .bytes()
                            .all(|byte| byte.is_ascii_hexdigit()),
                        "preview {} id is not hexadecimal",
                        case.name
                    );
                    let expires_at = WireTimestamp::from_str(&response.expires_at)
                        .expect("parse cleanup expiry")
                        .into_inner();
                    assert_eq!(
                        expires_at.unix_timestamp() - now.unix_timestamp(),
                        10 * 60,
                        "preview {} expiry lifetime",
                        case.name
                    );
                    let mut actual =
                        serde_json::to_value(response).expect("encode cleanup response");
                    actual["previewId"] = expected["previewId"].clone();
                    assert_eq!(actual, expected, "cleanup preview case {}", case.name);
                    None
                }
                None => Some(
                    service
                        .preview(case.request)
                        .expect_err("invalid cleanup preview should fail")
                        .to_string(),
                ),
            };
            if let Some(actual_error) = result {
                assert_eq!(
                    Some(actual_error),
                    case.error,
                    "cleanup preview error case {}",
                    case.name
                );
            }
        }
    }

    #[derive(Debug, Default)]
    struct TestCleanupPreviewIds;

    impl jftrade_datamanagement::CleanupPreviewIdPort for TestCleanupPreviewIds {
        fn new_preview_id(&self) -> Result<String, String> {
            Ok("0123456789abcdef0123456789abcdef".to_owned())
        }
    }
}
