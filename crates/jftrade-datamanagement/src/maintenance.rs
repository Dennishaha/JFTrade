use std::sync::Arc;

use serde::{Deserialize, Serialize};
use thiserror::Error;
use time::OffsetDateTime;

use crate::{ApprovedCleanupPreview, CleanupPreviewError, CleanupPreviewService};

#[derive(Clone, Debug, Default, Eq, PartialEq, Deserialize)]
#[serde(rename_all = "camelCase", default, deny_unknown_fields)]
pub struct CleanupExecuteRequest {
    pub preview_id: String,
    pub confirmation: String,
}

#[derive(Clone, Debug, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct CleanupResult {
    pub database_id: String,
    pub deleted_count: i64,
    pub estimated_bytes: i64,
    pub before_bytes: i64,
    pub after_bytes: i64,
    pub reclaimed_bytes: i64,
    pub compacted: bool,
    #[serde(skip_serializing_if = "String::is_empty")]
    pub warning: String,
}

#[derive(Clone, Debug, Default, Eq, PartialEq, Deserialize)]
#[serde(rename_all = "camelCase", default, deny_unknown_fields)]
pub struct CompactRequest {
    pub confirmation: String,
}

#[derive(Clone, Debug, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct CompactResult {
    pub database_id: String,
    pub before_bytes: i64,
    pub after_bytes: i64,
    pub reclaimed_bytes: i64,
    pub compacted: bool,
}

#[derive(Clone, Debug, Default, Eq, PartialEq, Deserialize)]
#[serde(rename_all = "camelCase", default, deny_unknown_fields)]
pub struct BackupRequest {
    pub database_id: String,
    pub confirmation: String,
}

#[derive(Clone, Debug, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct BackupResult {
    pub database_id: String,
    pub backup_path: String,
    pub size_bytes: i64,
    pub created_at: String,
}

#[derive(Clone, Debug, Default, Eq, PartialEq, Deserialize)]
#[serde(rename_all = "camelCase", default, deny_unknown_fields)]
pub struct RebuildRequest {
    pub database_ids: Vec<String>,
    pub database_id: String,
    pub mode: String,
    pub confirmation: String,
}

#[derive(Clone, Debug, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct RebuildResult {
    pub database_ids: Vec<String>,
    pub restart_required: bool,
    pub scheduled: bool,
}

#[derive(Clone, Debug, Eq, PartialEq, Error)]
pub enum MaintenanceOperationError {
    #[error("cleanup preview not found or expired")]
    PreviewNotFound,
    #[error("{0}")]
    Rejected(String),
    #[error("database maintenance conflict: {0}")]
    Conflict(String),
    #[error("cleanup preview is stale")]
    Stale,
    #[error("database maintenance failed: {0}")]
    Failed(String),
}

pub trait DatabaseMaintenancePort: Send + Sync {
    fn execute_cleanup(
        &self,
        approved: &ApprovedCleanupPreview,
    ) -> Result<CleanupResult, MaintenanceOperationError>;
    fn compact(
        &self,
        database_id: &str,
        created_at: &str,
    ) -> Result<CompactResult, MaintenanceOperationError>;
    fn backup(
        &self,
        database_id: &str,
        created_at: &str,
    ) -> Result<BackupResult, MaintenanceOperationError>;
    fn rebuild(
        &self,
        request: &RebuildRequest,
        created_at: &str,
    ) -> Result<RebuildResult, MaintenanceOperationError>;
}

pub struct MaintenanceService {
    previews: Arc<CleanupPreviewService>,
    port: Arc<dyn DatabaseMaintenancePort>,
}

impl MaintenanceService {
    pub fn new(
        previews: Arc<CleanupPreviewService>,
        port: Arc<dyn DatabaseMaintenancePort>,
    ) -> Self {
        Self { previews, port }
    }

    pub fn execute_cleanup(
        &self,
        request: CleanupExecuteRequest,
    ) -> Result<CleanupResult, MaintenanceOperationError> {
        self.execute_cleanup_at(request, OffsetDateTime::now_utc())
    }

    pub fn execute_cleanup_at(
        &self,
        request: CleanupExecuteRequest,
        now: OffsetDateTime,
    ) -> Result<CleanupResult, MaintenanceOperationError> {
        let approved = self
            .previews
            .approved_preview(request.preview_id.trim(), now)
            .map_err(map_preview_error)?
            .ok_or(MaintenanceOperationError::PreviewNotFound)?;
        if request.confirmation != approved.response.confirmation_text {
            return Err(MaintenanceOperationError::Rejected(
                "confirmation text does not match".to_owned(),
            ));
        }
        let approved = self
            .previews
            .take_approved_preview(request.preview_id.trim(), now)
            .map_err(map_preview_error)?
            .ok_or(MaintenanceOperationError::PreviewNotFound)?;
        self.port.execute_cleanup(&approved)
    }

    pub fn compact(
        &self,
        database_id: &str,
        request: CompactRequest,
        created_at: &str,
    ) -> Result<CompactResult, MaintenanceOperationError> {
        let database_id = database_id.trim();
        if request.confirmation != format!("COMPACT {database_id}") {
            return Err(MaintenanceOperationError::Rejected(
                "confirmation text does not match".to_owned(),
            ));
        }
        self.port.compact(database_id, created_at)
    }

    pub fn backup(
        &self,
        database_id: &str,
        request: BackupRequest,
        created_at: &str,
    ) -> Result<BackupResult, MaintenanceOperationError> {
        let database_id = database_id.trim();
        if request.confirmation.trim() != format!("BACKUP {database_id}") {
            return Err(MaintenanceOperationError::Rejected(
                "confirmation text does not match".to_owned(),
            ));
        }
        self.port.backup(database_id, created_at)
    }

    pub fn rebuild(
        &self,
        request: RebuildRequest,
        created_at: &str,
    ) -> Result<RebuildResult, MaintenanceOperationError> {
        self.port.rebuild(&request, created_at)
    }
}

fn map_preview_error(error: CleanupPreviewError) -> MaintenanceOperationError {
    match error {
        CleanupPreviewError::Rejected(message) => MaintenanceOperationError::Rejected(message),
        CleanupPreviewError::StateUnavailable => {
            MaintenanceOperationError::Failed(error.to_string())
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::cleanup::CleanupPreviewService;
    use crate::overview::{DatabaseDescriptor, DatabaseOverviewPort, DatabaseStatus};
    use crate::{
        CleanupCandidatePort, CleanupCandidateQuery, CleanupCandidateRecord, CleanupPreviewIdPort,
    };
    use crate::{DatabaseInspection, StorageStats};
    use std::collections::BTreeSet;
    use std::sync::Arc;

    #[derive(Debug)]
    struct StubMaintenancePort;
    impl DatabaseMaintenancePort for StubMaintenancePort {
        fn execute_cleanup(
            &self,
            _approved: &ApprovedCleanupPreview,
        ) -> Result<CleanupResult, MaintenanceOperationError> {
            Ok(CleanupResult {
                database_id: "strategy".into(),
                deleted_count: 0,
                estimated_bytes: 0,
                before_bytes: 100,
                after_bytes: 100,
                reclaimed_bytes: 0,
                compacted: false,
                warning: "".into(),
            })
        }
        fn compact(
            &self,
            database_id: &str,
            _executed_at: &str,
        ) -> Result<CompactResult, MaintenanceOperationError> {
            Ok(CompactResult {
                database_id: database_id.into(),
                before_bytes: 100,
                after_bytes: 80,
                reclaimed_bytes: 20,
                compacted: true,
            })
        }
        fn backup(
            &self,
            database_id: &str,
            executed_at: &str,
        ) -> Result<BackupResult, MaintenanceOperationError> {
            Ok(BackupResult {
                database_id: database_id.into(),
                backup_path: "/tmp/backup.db".into(),
                size_bytes: 100,
                created_at: executed_at.into(),
            })
        }
        fn rebuild(
            &self,
            _request: &RebuildRequest,
            _executed_at: &str,
        ) -> Result<RebuildResult, MaintenanceOperationError> {
            Ok(RebuildResult {
                database_ids: vec!["strategy".into()],
                restart_required: false,
                scheduled: false,
            })
        }
    }

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
    struct FixedPreviewId;
    impl CleanupPreviewIdPort for FixedPreviewId {
        fn new_preview_id(&self) -> Result<String, String> {
            Ok("0123456789abcdef0123456789abcdef".into())
        }
    }

    #[test]
    fn maintenance_service_confirmation_validation_and_rejection_parity() {
        // Parity: internal/datamanagement/service_test.go:44 TestServiceFallbacks
        let previews = Arc::new(CleanupPreviewService::new(
            vec![DatabaseDescriptor {
                id: "strategy".into(),
                name: "strategy".into(),
                path: "/tmp/strategy.db".into(),
                description: String::new(),
                features: vec![],
                expected_version: 1,
            }],
            Arc::new(FixedOverview {
                status: DatabaseStatus::Ready,
                scheduled: BTreeSet::new(),
            }),
            Arc::new(FixedCandidates(vec![])),
            Arc::new(FixedPreviewId),
        ));
        let service = MaintenanceService::new(previews, Arc::new(StubMaintenancePort));

        // Invalid compact confirmation rejected
        let err = service
            .compact(
                "strategy",
                CompactRequest {
                    confirmation: "INVALID".into(),
                },
                "2026-09-01T00:00:00Z",
            )
            .unwrap_err();
        assert!(matches!(err, MaintenanceOperationError::Rejected(_)));

        // Valid compact confirmation accepted
        assert!(
            service
                .compact(
                    "strategy",
                    CompactRequest {
                        confirmation: "COMPACT strategy".into(),
                    },
                    "2026-09-01T00:00:00Z",
                )
                .is_ok()
        );

        // Invalid backup confirmation rejected
        let err = service
            .backup(
                "strategy",
                BackupRequest {
                    database_id: "strategy".into(),
                    confirmation: "INVALID".into(),
                },
                "2026-09-01T00:00:00Z",
            )
            .unwrap_err();
        assert!(matches!(err, MaintenanceOperationError::Rejected(_)));

        // Valid backup confirmation accepted
        assert!(
            service
                .backup(
                    "strategy",
                    BackupRequest {
                        database_id: "strategy".into(),
                        confirmation: "BACKUP strategy".into(),
                    },
                    "2026-09-01T00:00:00Z",
                )
                .is_ok()
        );
    }
}
