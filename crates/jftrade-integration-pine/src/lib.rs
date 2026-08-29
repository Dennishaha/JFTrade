#![forbid(unsafe_code)]

//! Host lifecycle boundary for the retained Node PineTS worker.

mod asset;
mod backtest;
mod execution;
mod mock_worker;
mod pool;
mod process;

pub use asset::{PineBundle, PineBundleError};
pub use backtest::{
    BacktestExecutionCandle, BacktestExecutionError, BacktestExecutionPort,
    BacktestExecutionRequest, RunJsonBacktestExecutionPort,
};
pub use execution::{
    GrpcPineExecutionPort, PineAlert, PineCandle, PineDiagnostic, PineExecutionConfig,
    PineExecutionError, PineExecutionFuture, PineExecutionPort, PineOrderIntent, PinePlot,
    PineRunRequest, PineRunResult, PineStrategyMetrics, PineVisualOutput, PineWorkerMetadata,
};
pub use mock_worker::{MockPineWorker, spawn_mock_pine_worker, wait_until_listening};
pub use pool::{
    PoolError, SessionOperation, WorkerHealth, WorkerPool, WorkerReservation, WorkerSnapshot,
};
pub use process::{
    GrpcPineReadinessProbe, PineProcess, PineProcessConfig, PineProcessError, PineReadinessPolicy,
    PineReadinessProbe, WorkerProcessSpec,
};
