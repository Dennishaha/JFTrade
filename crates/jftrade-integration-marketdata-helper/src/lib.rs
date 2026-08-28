#![forbid(unsafe_code)]

//! Host-side adapter for the retained Python market-data helper.

mod asset;
mod client;
pub mod dto;
mod process;
mod provider;

pub use asset::{AssetBundle, AssetError};
pub use client::{
    HelperClient, HelperClientConfig, HelperErrorEnvelope, HelperHealth, HttpAdapterError,
};
pub use dto::{
    HelperCandle, HelperCandlesResponse, HelperMarketPrecision, HelperMarketProfile,
    HelperMarketsResponse, HelperPriceValue, HelperSearchEntry, HelperSearchResponse,
    HelperSecurityResponse, HelperSnapshotQuote, HelperSnapshotResponse, HelperTradingWindow,
};
pub use process::{
    HelperProcess, HelperProcessConfig, ProcessError, ProcessSnapshot, ProcessState,
    allocate_loopback_port,
};
pub use provider::{akshare_descriptor, provider_descriptors, yfinance_descriptor};
