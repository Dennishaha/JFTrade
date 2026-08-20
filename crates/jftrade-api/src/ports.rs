use std::collections::BTreeMap;
use std::future::Future;
use std::pin::Pin;

use serde::{Deserialize, Serialize};
use serde_json::Value;

use crate::{ApiFailure, SseEvent};

#[derive(Clone, Debug, Eq, PartialEq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct ApiRequest {
    pub method: String,
    pub path: String,
    pub query: String,
    pub body: Vec<u8>,
    pub request_id: String,
    #[serde(default)]
    pub desktop_trusted: bool,
}

#[derive(Clone, Debug, PartialEq)]
pub enum ApiOutput {
    Json(Value),
    Sse(Vec<SseEvent>),
    NoContent,
    Raw {
        status: u16,
        content_type: String,
        body: Vec<u8>,
    },
}

pub type PortFuture<'a> = Pin<Box<dyn Future<Output = Result<ApiOutput, ApiFailure>> + Send + 'a>>;

pub trait ApiPort: Send + Sync {
    fn dispatch(&self, request: ApiRequest) -> PortFuture<'_>;
}

#[derive(Clone, Debug, Eq, PartialEq)]
pub struct Asset {
    pub content_type: String,
    pub bytes: Vec<u8>,
}

#[derive(Clone, Debug, Default, Eq, PartialEq)]
pub struct AssetBundle {
    assets: BTreeMap<String, Asset>,
}

impl AssetBundle {
    pub fn new(assets: impl IntoIterator<Item = (String, Asset)>) -> Self {
        Self {
            assets: assets.into_iter().collect(),
        }
    }

    pub fn get(&self, path: &str) -> Option<&Asset> {
        self.assets.get(path.trim_start_matches('/'))
    }

    pub fn spa_index(&self) -> Option<&Asset> {
        self.assets.get("index.html")
    }
}
