#![forbid(unsafe_code)]

//! Host-side adapter for the retained Python market-data helper.

mod asset;
mod client;
mod process;

pub use asset::{AssetBundle, AssetError};
pub use client::{
    HelperClient, HelperClientConfig, HelperErrorEnvelope, HelperHealth, HttpAdapterError,
};
pub use process::{
    HelperProcess, HelperProcessConfig, ProcessError, ProcessSnapshot, ProcessState,
    allocate_loopback_port,
};
