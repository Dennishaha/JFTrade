//! Production database lease evidence shared by the composition root and
//! readiness projections.

use jftrade_datamanagement::{
    DATABASE_ADK, DATABASE_ADK_ARTIFACT, DATABASE_ADK_SESSION, DATABASE_BACKTEST,
    DATABASE_BACKTEST_RUNS, DATABASE_EXECUTION, DATABASE_RESEARCH, DATABASE_STRATEGY,
    DATABASE_WATCHLIST,
};

pub const PRODUCTION_DATABASE_IDS: [&str; 9] = [
    DATABASE_WATCHLIST,
    DATABASE_STRATEGY,
    DATABASE_RESEARCH,
    DATABASE_BACKTEST_RUNS,
    DATABASE_BACKTEST,
    DATABASE_EXECUTION,
    DATABASE_ADK,
    DATABASE_ADK_SESSION,
    DATABASE_ADK_ARTIFACT,
];

#[derive(Clone, Debug, Eq, PartialEq)]
pub struct ProductionDatabaseLeaseSnapshot {
    pub expected: usize,
    pub acquired: usize,
    pub databases: Vec<String>,
    pub status: &'static str,
}

impl ProductionDatabaseLeaseSnapshot {
    pub fn new(acquired_databases: Vec<String>) -> Self {
        let expected = PRODUCTION_DATABASE_IDS.len();
        let acquired = acquired_databases.len();
        let status = if acquired == expected && expected > 0 {
            "acquired"
        } else if acquired == 0 {
            "none"
        } else {
            "partial"
        };
        Self {
            expected,
            acquired,
            databases: acquired_databases,
            status,
        }
    }
}
