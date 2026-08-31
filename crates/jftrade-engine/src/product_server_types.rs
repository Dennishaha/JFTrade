#[derive(Clone, Debug, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct ProductStartupRecord {
    pub event: &'static str,
    pub address: SocketAddr,
    pub owner: &'static str,
    pub owned_routes: usize,
    pub ready_routes: usize,
    pub external_unavailable_routes: usize,
    pub protocol_version: &'static str,
    pub route_profile: &'static str,
    pub route_profile_digest: String,
    pub capabilities: Vec<String>,
    pub resource_sha256: String,
    pub runtime_readiness: &'static str,
    pub database_lease_status: &'static str,
    pub provider_status: &'static str,
    pub opend_status: &'static str,
    pub worker_status: &'static str,
    pub websocket_status: &'static str,
}

#[derive(Clone, Debug, Eq, PartialEq)]
pub struct ProductNotificationRequest {
    pub title: String,
    pub body: String,
    pub sound_enabled: bool,
}

#[derive(Clone, Debug, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct ProductNotificationDelivery {
    pub delivered: bool,
    pub status: String,
    #[serde(skip_serializing_if = "String::is_empty")]
    pub message: String,
}

pub trait ProductNotificationPort: Send + Sync + std::fmt::Debug {
    fn deliver(&self, request: ProductNotificationRequest) -> ProductNotificationDelivery;
}

fn route_profile_digest(capabilities: &[String]) -> String {
    let mut digest = Sha256::new();
    for capability in capabilities {
        digest.update(capability.as_bytes());
        digest.update(b"\n");
    }
    encode_sha256(digest.finalize())
}
