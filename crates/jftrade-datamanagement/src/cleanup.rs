use std::collections::{BTreeMap, HashMap};
use std::sync::{Arc, Mutex};

use jftrade_kernel::WireTimestamp;
use serde::{Deserialize, Serialize};
use thiserror::Error;
use time::format_description::well_known::Rfc3339;
use time::{Duration, OffsetDateTime};

use crate::{
    CleanupCandidate, DatabaseDescriptor, DatabaseOverviewPort, DatabaseStatus, preview_cleanup,
};

pub const CLEANUP_BACKTEST_HISTORY: &str = "backtest-history";
pub const CLEANUP_SOFT_DELETED: &str = "soft-deleted";
const PREVIEW_LIFETIME: Duration = Duration::minutes(10);

#[derive(Clone, Debug, Default, Eq, PartialEq, Deserialize)]
#[serde(rename_all = "camelCase", default)]
pub struct CleanupPreviewRequest {
    pub kind: String,
    pub database_id: String,
    pub older_than_days: i32,
    pub keep_latest: i32,
}

#[derive(Clone, Debug, Eq, PartialEq)]
pub struct CleanupCandidateQuery {
    pub kind: String,
    pub older_than_days: i32,
    pub keep_latest: i32,
    pub cutoff: Option<WireTimestamp>,
}

#[derive(Clone, Debug, Eq, PartialEq)]
pub struct CleanupCandidateRecord {
    pub id: String,
    pub category: String,
    pub estimated_bytes: i64,
}

#[derive(Clone, Debug, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct CleanupPreviewItem {
    pub kind: String,
    pub label: String,
    pub count: i64,
    pub estimated_bytes: i64,
}

#[derive(Clone, Debug, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct CleanupPreviewResponse {
    pub preview_id: String,
    pub expires_at: String,
    pub kind: String,
    pub database_id: String,
    pub candidate_count: i64,
    pub estimated_bytes: i64,
    pub items: Vec<CleanupPreviewItem>,
    pub confirmation_text: String,
    pub will_compact: bool,
}

#[derive(Clone, Debug, Eq, PartialEq)]
pub struct ApprovedCleanupPreview {
    pub response: CleanupPreviewResponse,
    pub candidates: Vec<CleanupCandidateRecord>,
    pub fingerprint: String,
}

pub trait CleanupCandidatePort: Send + Sync {
    fn candidates(
        &self,
        descriptor: &DatabaseDescriptor,
        query: &CleanupCandidateQuery,
    ) -> Result<Vec<CleanupCandidateRecord>, String>;
}

pub trait CleanupPreviewIdPort: Send + Sync {
    fn new_preview_id(&self) -> Result<String, String>;
}

#[derive(Clone, Debug)]
struct StoredPreview {
    approved: ApprovedCleanupPreview,
    expires_at: OffsetDateTime,
}

pub struct CleanupPreviewService {
    descriptors: BTreeMap<String, DatabaseDescriptor>,
    overview: Arc<dyn DatabaseOverviewPort>,
    candidates: Arc<dyn CleanupCandidatePort>,
    preview_ids: Arc<dyn CleanupPreviewIdPort>,
    previews: Mutex<HashMap<String, StoredPreview>>,
}

#[derive(Clone, Debug, Eq, PartialEq, Error)]
pub enum CleanupPreviewError {
    #[error("{0}")]
    Rejected(String),
    #[error("cleanup preview state is unavailable")]
    StateUnavailable,
}

impl CleanupPreviewService {
    pub fn new(
        descriptors: Vec<DatabaseDescriptor>,
        overview: Arc<dyn DatabaseOverviewPort>,
        candidates: Arc<dyn CleanupCandidatePort>,
        preview_ids: Arc<dyn CleanupPreviewIdPort>,
    ) -> Self {
        Self {
            descriptors: descriptors
                .into_iter()
                .map(|descriptor| (descriptor.id.clone(), descriptor))
                .collect(),
            overview,
            candidates,
            preview_ids,
            previews: Mutex::new(HashMap::new()),
        }
    }

    pub fn preview(
        &self,
        request: CleanupPreviewRequest,
    ) -> Result<CleanupPreviewResponse, CleanupPreviewError> {
        self.preview_at(request, OffsetDateTime::now_utc())
    }

    pub fn preview_at(
        &self,
        request: CleanupPreviewRequest,
        now: OffsetDateTime,
    ) -> Result<CleanupPreviewResponse, CleanupPreviewError> {
        let (request, query) = normalize_request(request, now)?;
        let descriptor = self.descriptors.get(&request.database_id).ok_or_else(|| {
            CleanupPreviewError::Rejected(format!("unknown database id {:?}", request.database_id))
        })?;
        let scheduled = self
            .overview
            .scheduled_rebuilds()
            .map_err(CleanupPreviewError::Rejected)?;
        let inspection = self.overview.inspect(descriptor, true);
        if inspection.status != DatabaseStatus::Ready || scheduled.contains(&request.database_id) {
            return Err(CleanupPreviewError::Rejected(format!(
                "database {} is not ready for cleanup",
                request.database_id
            )));
        }
        let candidates = self
            .candidates
            .candidates(descriptor, &query)
            .map_err(CleanupPreviewError::Rejected)?;
        let (items, estimated_bytes) = summarize_candidates(&candidates);
        let candidate_count = i64::try_from(candidates.len()).unwrap_or(i64::MAX);
        let expires_at = now.checked_add(PREVIEW_LIFETIME).ok_or_else(|| {
            CleanupPreviewError::Rejected("cleanup preview expiry is out of range".to_owned())
        })?;
        let preview_id = self
            .preview_ids
            .new_preview_id()
            .map_err(CleanupPreviewError::Rejected)?;
        if !valid_preview_id(&preview_id) {
            return Err(CleanupPreviewError::Rejected(
                "cleanup preview id generator returned an invalid id".to_owned(),
            ));
        }
        let response = CleanupPreviewResponse {
            preview_id: preview_id.clone(),
            expires_at: expires_at
                .format(&Rfc3339)
                .map_err(|error| CleanupPreviewError::Rejected(error.to_string()))?,
            kind: request.kind,
            database_id: request.database_id.clone(),
            candidate_count,
            estimated_bytes,
            items,
            confirmation_text: format!("CLEANUP {} {candidate_count}", request.database_id),
            will_compact: true,
        };
        let identity_candidates = candidates
            .iter()
            .map(|candidate| CleanupCandidate {
                id: candidate.id.clone(),
                category: candidate.category.clone(),
            })
            .collect();
        let fingerprint = preview_cleanup(&request.database_id, identity_candidates, None)
            .map_err(|error| CleanupPreviewError::Rejected(error.to_string()))?
            .fingerprint;
        let approved = ApprovedCleanupPreview {
            response: response.clone(),
            candidates,
            fingerprint,
        };
        let mut previews = self
            .previews
            .lock()
            .map_err(|_| CleanupPreviewError::StateUnavailable)?;
        previews.retain(|_, preview| preview.expires_at > now);
        previews.insert(
            preview_id,
            StoredPreview {
                approved,
                expires_at,
            },
        );
        Ok(response)
    }

    pub fn approved_preview(
        &self,
        preview_id: &str,
        now: OffsetDateTime,
    ) -> Result<Option<ApprovedCleanupPreview>, CleanupPreviewError> {
        let mut previews = self
            .previews
            .lock()
            .map_err(|_| CleanupPreviewError::StateUnavailable)?;
        previews.retain(|_, preview| preview.expires_at > now);
        Ok(previews
            .get(preview_id.trim())
            .map(|preview| preview.approved.clone()))
    }
}

fn normalize_request(
    mut request: CleanupPreviewRequest,
    now: OffsetDateTime,
) -> Result<(CleanupPreviewRequest, CleanupCandidateQuery), CleanupPreviewError> {
    request.kind = request.kind.trim().to_owned();
    request.database_id = request.database_id.trim().to_owned();
    match request.kind.as_str() {
        CLEANUP_BACKTEST_HISTORY => {
            if request.database_id != crate::DATABASE_BACKTEST_RUNS {
                return Err(CleanupPreviewError::Rejected(format!(
                    "backtest history cleanup requires {}",
                    crate::DATABASE_BACKTEST_RUNS
                )));
            }
            if request.older_than_days == 0 {
                request.older_than_days = 30;
            }
            if request.keep_latest == 0 {
                request.keep_latest = 20;
            }
            if !(1..=3650).contains(&request.older_than_days)
                || !(1..=10_000).contains(&request.keep_latest)
            {
                return Err(CleanupPreviewError::Rejected(
                    "backtest retention must use 1-3650 days and keep 1-10000 runs".to_owned(),
                ));
            }
        }
        CLEANUP_SOFT_DELETED => {
            if request.database_id != crate::DATABASE_STRATEGY
                && request.database_id != crate::DATABASE_ADK
            {
                return Err(CleanupPreviewError::Rejected(format!(
                    "soft-deleted cleanup is unsupported for database {:?}",
                    request.database_id
                )));
            }
        }
        _ => {
            return Err(CleanupPreviewError::Rejected(format!(
                "unknown cleanup kind {:?}",
                request.kind
            )));
        }
    }
    let cutoff = if request.kind == CLEANUP_BACKTEST_HISTORY {
        let days = i64::from(request.older_than_days);
        Some(WireTimestamp::from_offset_datetime(
            now.checked_sub(Duration::days(days)).ok_or_else(|| {
                CleanupPreviewError::Rejected("cleanup cutoff is out of range".to_owned())
            })?,
        ))
    } else {
        None
    };
    let query = CleanupCandidateQuery {
        kind: request.kind.clone(),
        older_than_days: request.older_than_days,
        keep_latest: request.keep_latest,
        cutoff,
    };
    Ok((request, query))
}

fn summarize_candidates(candidates: &[CleanupCandidateRecord]) -> (Vec<CleanupPreviewItem>, i64) {
    let mut by_category = BTreeMap::<String, CleanupPreviewItem>::new();
    let mut total = 0_i64;
    for candidate in candidates {
        let item = by_category
            .entry(candidate.category.clone())
            .or_insert_with(|| CleanupPreviewItem {
                kind: candidate.category.clone(),
                label: candidate.category.clone(),
                count: 0,
                estimated_bytes: 0,
            });
        item.count = item.count.saturating_add(1);
        item.estimated_bytes = item
            .estimated_bytes
            .saturating_add(candidate.estimated_bytes);
        total = total.saturating_add(candidate.estimated_bytes);
    }
    (by_category.into_values().collect(), total)
}

fn valid_preview_id(preview_id: &str) -> bool {
    preview_id.len() == 32 && preview_id.bytes().all(|byte| byte.is_ascii_hexdigit())
}

#[cfg(test)]
mod tests {
    use std::collections::BTreeSet;

    use super::*;
    use crate::{DatabaseInspection, StorageStats};

    #[derive(Debug)]
    struct FixedOverview {
        status: DatabaseStatus,
        scheduled: BTreeSet<String>,
    }

    impl DatabaseOverviewPort for FixedOverview {
        fn scheduled_rebuilds(&self) -> Result<BTreeSet<String>, String> {
            Ok(self.scheduled.clone())
        }

        fn inspect(
            &self,
            _descriptor: &DatabaseDescriptor,
            _summary_only: bool,
        ) -> DatabaseInspection {
            DatabaseInspection {
                status: self.status.clone(),
                current_version: Some(1),
                error: String::new(),
                storage: StorageStats::default(),
                cleanable: None,
            }
        }
    }

    #[derive(Debug)]
    struct FixedCandidates(Vec<CleanupCandidateRecord>);

    impl CleanupCandidatePort for FixedCandidates {
        fn candidates(
            &self,
            _descriptor: &DatabaseDescriptor,
            _query: &CleanupCandidateQuery,
        ) -> Result<Vec<CleanupCandidateRecord>, String> {
            Ok(self.0.clone())
        }
    }

    #[derive(Debug)]
    struct FixedId;

    impl CleanupPreviewIdPort for FixedId {
        fn new_preview_id(&self) -> Result<String, String> {
            Ok("0123456789abcdef0123456789abcdef".to_owned())
        }
    }

    fn service(status: DatabaseStatus, scheduled: &[&str]) -> CleanupPreviewService {
        CleanupPreviewService::new(
            vec![DatabaseDescriptor {
                id: crate::DATABASE_BACKTEST_RUNS.to_owned(),
                name: "runs".to_owned(),
                path: "/tmp/runs.db".to_owned(),
                description: String::new(),
                features: Vec::new(),
                expected_version: 1,
            }],
            Arc::new(FixedOverview {
                status,
                scheduled: scheduled.iter().map(|value| (*value).to_owned()).collect(),
            }),
            Arc::new(FixedCandidates(vec![
                CleanupCandidateRecord {
                    id: "run-2".to_owned(),
                    category: "回测结果".to_owned(),
                    estimated_bytes: 9,
                },
                CleanupCandidateRecord {
                    id: "run-1".to_owned(),
                    category: "回测结果".to_owned(),
                    estimated_bytes: 4,
                },
            ])),
            Arc::new(FixedId),
        )
    }

    #[test]
    fn preview_normalizes_defaults_summarizes_and_expires_after_ten_minutes() {
        let service = service(DatabaseStatus::Ready, &[]);
        let now = OffsetDateTime::parse("2026-08-20T00:00:00Z", &Rfc3339).expect("time");
        let response = service
            .preview_at(
                CleanupPreviewRequest {
                    kind: " backtest-history ".to_owned(),
                    database_id: " backtest-runs ".to_owned(),
                    ..CleanupPreviewRequest::default()
                },
                now,
            )
            .expect("preview");
        assert_eq!(response.candidate_count, 2);
        assert_eq!(response.estimated_bytes, 13);
        assert_eq!(response.items[0].label, "回测结果");
        assert_eq!(response.confirmation_text, "CLEANUP backtest-runs 2");
        assert_eq!(response.expires_at, "2026-08-20T00:10:00Z");
        assert!(
            service
                .approved_preview(&response.preview_id, now + Duration::minutes(9))
                .expect("state")
                .is_some()
        );
        assert!(
            service
                .approved_preview(&response.preview_id, now + Duration::minutes(10))
                .expect("state")
                .is_none()
        );
    }

    #[test]
    fn preview_rejects_invalid_retention_and_non_ready_databases() {
        let now = OffsetDateTime::parse("2026-08-20T00:00:00Z", &Rfc3339).expect("time");
        let invalid = service(DatabaseStatus::Ready, &[])
            .preview_at(
                CleanupPreviewRequest {
                    kind: CLEANUP_BACKTEST_HISTORY.to_owned(),
                    database_id: crate::DATABASE_BACKTEST_RUNS.to_owned(),
                    older_than_days: 3651,
                    keep_latest: 1,
                },
                now,
            )
            .expect_err("retention");
        assert_eq!(
            invalid.to_string(),
            "backtest retention must use 1-3650 days and keep 1-10000 runs"
        );
        let scheduled = service(DatabaseStatus::Ready, &[crate::DATABASE_BACKTEST_RUNS])
            .preview_at(
                CleanupPreviewRequest {
                    kind: CLEANUP_BACKTEST_HISTORY.to_owned(),
                    database_id: crate::DATABASE_BACKTEST_RUNS.to_owned(),
                    ..CleanupPreviewRequest::default()
                },
                now,
            )
            .expect_err("scheduled rebuild");
        assert_eq!(
            scheduled.to_string(),
            "database backtest-runs is not ready for cleanup"
        );
    }
}
