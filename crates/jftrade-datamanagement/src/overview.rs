use std::collections::{BTreeMap, BTreeSet};
use std::sync::Arc;

use serde::{Deserialize, Serialize};
use thiserror::Error;

pub const DATABASE_BACKTEST: &str = "backtest";
pub const DATABASE_BACKTEST_RUNS: &str = "backtest-runs";
pub const DATABASE_STRATEGY: &str = "strategy";
pub const DATABASE_EXECUTION: &str = "execution-orders";
pub const DATABASE_ADK: &str = "adk";
pub const DATABASE_ADK_SESSION: &str = "adk-session";
pub const DATABASE_ADK_ARTIFACT: &str = "adk-artifact";
pub const DATABASE_WATCHLIST: &str = "watchlist";
pub const DATABASE_RESEARCH: &str = "research";

#[derive(Clone, Debug, Eq, PartialEq)]
pub struct ManagedDatabasePaths {
    paths: BTreeMap<&'static str, String>,
}

impl ManagedDatabasePaths {
    pub fn new(paths: impl IntoIterator<Item = (&'static str, String)>) -> Self {
        Self {
            paths: paths.into_iter().collect(),
        }
    }

    fn path(&self, id: &'static str) -> String {
        self.paths.get(id).cloned().unwrap_or_default()
    }
}

#[derive(Clone, Debug, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct DatabaseDescriptor {
    pub id: String,
    pub name: String,
    pub path: String,
    pub description: String,
    pub features: Vec<String>,
    pub expected_version: i64,
}

#[derive(Clone, Debug, Default, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct StorageStats {
    pub main_bytes: i64,
    pub wal_bytes: i64,
    pub shm_bytes: i64,
    pub total_bytes: i64,
    pub free_page_bytes: i64,
    pub reclaimable_bytes: i64,
    #[serde(skip_serializing_if = "String::is_empty")]
    pub error: String,
}

#[derive(Clone, Debug, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct CleanableItem {
    pub kind: String,
    pub label: String,
    pub count: i64,
    pub estimated_bytes: i64,
}

#[derive(Clone, Debug, Eq, PartialEq)]
pub enum DatabaseStatus {
    Missing,
    Ready,
    Incompatible,
    Unavailable,
}

impl DatabaseStatus {
    const fn as_str(&self) -> &'static str {
        match self {
            Self::Missing => "missing",
            Self::Ready => "ready",
            Self::Incompatible => "incompatible",
            Self::Unavailable => "unavailable",
        }
    }
}

#[derive(Clone, Debug, Eq, PartialEq)]
pub struct DatabaseInspection {
    pub status: DatabaseStatus,
    pub current_version: Option<i64>,
    pub error: String,
    pub storage: StorageStats,
    pub cleanable: Option<Vec<CleanableItem>>,
}

#[derive(Clone, Debug, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct DatabaseOverview {
    #[serde(flatten)]
    pub descriptor: DatabaseDescriptor,
    pub status: String,
    pub current_version: Option<i64>,
    #[serde(skip_serializing_if = "String::is_empty")]
    pub error: String,
    pub rebuild_scheduled: bool,
    pub restart_required: bool,
    pub confirmation_text: String,
    pub storage: StorageStats,
    pub cleanable: Option<Vec<CleanableItem>>,
}

#[derive(Clone, Debug, Default, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct OverviewTotals {
    pub main_bytes: i64,
    pub wal_bytes: i64,
    pub shm_bytes: i64,
    pub total_bytes: i64,
    pub reclaimable_bytes: i64,
}

#[derive(Clone, Debug, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct OverviewResponse {
    pub databases: Vec<DatabaseOverview>,
    pub totals: OverviewTotals,
    pub checked_at: String,
}

#[derive(Clone, Debug, Default, Eq, PartialEq, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct OverviewRequest {
    pub summary_only: bool,
    pub database_id: String,
}

pub trait DatabaseOverviewPort: Send + Sync {
    fn scheduled_rebuilds(&self) -> Result<BTreeSet<String>, String>;
    fn inspect(&self, descriptor: &DatabaseDescriptor, summary_only: bool) -> DatabaseInspection;
}

#[derive(Clone)]
pub struct OverviewService {
    descriptors: Vec<DatabaseDescriptor>,
    port: Arc<dyn DatabaseOverviewPort>,
}

#[derive(Clone, Debug, Eq, PartialEq, Error)]
pub enum OverviewError {
    #[error("unknown database id {0:?}")]
    UnknownDatabase(String),
    #[error("read database rebuild marker: {0}")]
    RebuildMarker(String),
}

impl OverviewService {
    pub fn new(descriptors: Vec<DatabaseDescriptor>, port: Arc<dyn DatabaseOverviewPort>) -> Self {
        Self { descriptors, port }
    }

    pub fn overview(
        &self,
        mut request: OverviewRequest,
        checked_at: String,
    ) -> Result<OverviewResponse, OverviewError> {
        request.database_id = request.database_id.trim().to_owned();
        let scheduled = self
            .port
            .scheduled_rebuilds()
            .map_err(OverviewError::RebuildMarker)?;
        let mut response = OverviewResponse {
            databases: Vec::with_capacity(self.descriptors.len()),
            totals: OverviewTotals::default(),
            checked_at,
        };
        for descriptor in &self.descriptors {
            if !request.database_id.is_empty() && descriptor.id != request.database_id {
                continue;
            }
            let inspection = self.port.inspect(descriptor, request.summary_only);
            let rebuild_scheduled = scheduled.contains(&descriptor.id);
            add_storage(&mut response.totals, &inspection.storage);
            response.databases.push(DatabaseOverview {
                descriptor: descriptor.clone(),
                status: inspection.status.as_str().to_owned(),
                current_version: inspection.current_version,
                error: inspection.error,
                rebuild_scheduled,
                restart_required: rebuild_scheduled,
                confirmation_text: format!("REBUILD {}", descriptor.id),
                storage: inspection.storage,
                cleanable: inspection.cleanable,
            });
        }
        if !request.database_id.is_empty() && response.databases.is_empty() {
            return Err(OverviewError::UnknownDatabase(request.database_id));
        }
        Ok(response)
    }
}

fn add_storage(totals: &mut OverviewTotals, storage: &StorageStats) {
    totals.main_bytes += storage.main_bytes;
    totals.wal_bytes += storage.wal_bytes;
    totals.shm_bytes += storage.shm_bytes;
    totals.total_bytes += storage.total_bytes;
    totals.reclaimable_bytes += storage.reclaimable_bytes;
}

pub fn managed_database_descriptors(paths: &ManagedDatabasePaths) -> Vec<DatabaseDescriptor> {
    vec![
        descriptor(
            DATABASE_BACKTEST,
            "行情回测数据",
            paths.path(DATABASE_BACKTEST),
            "历史 K 线、覆盖范围与行情同步数据。",
            &["回测行情", "K 线同步"],
            3,
        ),
        descriptor(
            DATABASE_BACKTEST_RUNS,
            "回测运行历史",
            paths.path(DATABASE_BACKTEST_RUNS),
            "回测请求、状态和结果。",
            &["回测历史", "研究回测结果"],
            1,
        ),
        descriptor(
            DATABASE_STRATEGY,
            "策略数据",
            paths.path(DATABASE_STRATEGY),
            "策略定义、历史版本、插件目录、运行日志、审计和观察状态。",
            &["策略定义", "版本历史", "策略插件", "策略运行"],
            2,
        ),
        descriptor(
            DATABASE_EXECUTION,
            "执行订单",
            paths.path(DATABASE_EXECUTION),
            "执行订单、状态事件、成交去重和序列。",
            &["订单执行", "成交同步"],
            5,
        ),
        descriptor(
            DATABASE_ADK,
            "ADK 数据",
            paths.path(DATABASE_ADK),
            "模型、智能体、技能、会话运行、任务、审批和记忆。",
            &["智能体配置", "ADK 工作流"],
            4,
        ),
        descriptor(
            DATABASE_ADK_SESSION,
            "ADK 会话",
            paths.path(DATABASE_ADK_SESSION),
            "ADK 原始会话事件和状态。",
            &["对话上下文", "工具事件"],
            4,
        ),
        descriptor(
            DATABASE_ADK_ARTIFACT,
            "ADK 工件",
            paths.path(DATABASE_ADK_ARTIFACT),
            "ADK 工具输出和版本化工件。",
            &["工具工件", "上下文卸载"],
            1,
        ),
        descriptor(
            DATABASE_WATCHLIST,
            "自选股",
            paths.path(DATABASE_WATCHLIST),
            "本地自选分组、成员、券商导入绑定、快照与审计记录。",
            &["自选分组", "券商导入", "来源对账"],
            1,
        ),
        descriptor(
            DATABASE_RESEARCH,
            "研究数据",
            paths.path(DATABASE_RESEARCH),
            "研究中心股票筛选预设与后续研究持久化数据。",
            &["股票筛选预设"],
            1,
        ),
    ]
}

fn descriptor(
    id: &str,
    name: &str,
    path: String,
    description: &str,
    features: &[&str],
    expected_version: i64,
) -> DatabaseDescriptor {
    DatabaseDescriptor {
        id: id.to_owned(),
        name: name.to_owned(),
        path,
        description: description.to_owned(),
        features: features.iter().map(|value| (*value).to_owned()).collect(),
        expected_version,
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    struct MissingPort;

    impl DatabaseOverviewPort for MissingPort {
        fn scheduled_rebuilds(&self) -> Result<BTreeSet<String>, String> {
            Ok(BTreeSet::from([DATABASE_BACKTEST.to_owned()]))
        }

        fn inspect(
            &self,
            _descriptor: &DatabaseDescriptor,
            _summary_only: bool,
        ) -> DatabaseInspection {
            DatabaseInspection {
                status: DatabaseStatus::Missing,
                current_version: None,
                error: String::new(),
                storage: StorageStats::default(),
                cleanable: None,
            }
        }
    }

    #[test]
    fn overview_preserves_go_order_filter_and_rebuild_projection() {
        let paths = ManagedDatabasePaths::new([(DATABASE_BACKTEST, "/tmp/backtest.db".into())]);
        let service =
            OverviewService::new(managed_database_descriptors(&paths), Arc::new(MissingPort));
        let result = service
            .overview(
                OverviewRequest {
                    summary_only: true,
                    database_id: DATABASE_BACKTEST.to_owned(),
                },
                "2026-08-20T00:00:00Z".to_owned(),
            )
            .expect("overview");
        assert_eq!(result.databases.len(), 1);
        assert!(result.databases[0].rebuild_scheduled);
        assert_eq!(result.databases[0].confirmation_text, "REBUILD backtest");
        assert_eq!(
            service.overview(
                OverviewRequest {
                    database_id: "unknown".to_owned(),
                    ..OverviewRequest::default()
                },
                String::new(),
            ),
            Err(OverviewError::UnknownDatabase("unknown".to_owned()))
        );
    }
}
