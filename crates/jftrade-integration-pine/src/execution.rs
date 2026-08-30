//! Engine-neutral execution port for the retained PineTS worker.
use prost::Message;
use std::collections::BTreeMap;
use std::collections::HashMap;
use std::future::Future;
use std::net::SocketAddr;
use std::pin::Pin;
use std::time::Duration;
use thiserror::Error;
use tokio::time::timeout;
use tonic::metadata::MetadataValue;
use tonic::transport::Endpoint;
use tonic::{Request, Status};
mod wire {
    tonic::include_proto!("jftrade.strategy.pineworker.v1");
}
use wire::pine_worker_client::PineWorkerClient;
use wire::{
    AnalyzeScriptRequest, AnalyzeScriptResponse, CandleBatch, RunScriptRequest, RunScriptResponse,
};
const CANDLE_BATCH_ENCODING_VERSION: u32 = 1;
const CANDLE_BATCH_RECORD_BYTES: usize = 56;
const DEFAULT_MAX_MESSAGE_BYTES: usize = 4 << 20;
const DEFAULT_MAX_SOURCE_BYTES: usize = 1_000_000;
const DEFAULT_MAX_PARAMS: usize = 256;
#[derive(Clone, Debug, Default, PartialEq)]
pub struct PineCandle {
    pub open_time: i64,
    pub close_time: i64,
    pub open: f64,
    pub high: f64,
    pub low: f64,
    pub close: f64,
    pub volume: f64,
}
#[derive(Clone, Debug, Default, PartialEq)]
pub struct PineRunRequest {
    pub job_id: String,
    pub script_id: String,
    pub source: String,
    pub symbol: String,
    pub timeframe: String,
    pub chart_type: String,
    pub mode: String,
    pub candles: Vec<PineCandle>,
    pub params: BTreeMap<String, String>,
    pub session_id: String,
    pub session_operation: String,
    pub expected_revision: u64,
}
#[derive(Clone, Debug, Default, PartialEq)]
pub struct PinePlot {
    pub name: String,
    pub values: Vec<f64>,
}
#[derive(Clone, Debug, Default, Eq, PartialEq)]
pub struct PineDiagnostic {
    pub severity: String,
    pub code: String,
    pub message: String,
    pub line: i32,
    pub column: i32,
}
#[derive(Clone, Debug, Eq, PartialEq)]
pub struct PineWorkerMetadata {
    pub worker_id: String,
    pub version: String,
    pub pine_ts_version: String,
    pub script_hash: String,
    pub data_hash: String,
    pub duration: Duration,
    pub request_bytes: usize,
    pub response_bytes: usize,
    pub peak_rss_bytes: usize,
}
impl Default for PineWorkerMetadata {
    fn default() -> Self {
        Self {
            worker_id: String::new(),
            version: String::new(),
            pine_ts_version: String::new(),
            script_hash: String::new(),
            data_hash: String::new(),
            duration: Duration::ZERO,
            request_bytes: 0,
            response_bytes: 0,
            peak_rss_bytes: 0,
        }
    }
}
#[derive(Clone, Debug, Default, PartialEq)]
pub struct PineOrderIntent {
    pub kind: String,
    pub id: String,
    pub from_entry: String,
    pub direction: String,
    pub quantity: f64,
    pub quantity_pct: f64,
    pub limit_price: f64,
    pub stop_price: f64,
    pub comment: String,
    pub alert_message: String,
    pub disable_alert: bool,
    pub bar_index: i32,
    pub time: i64,
    pub has_quantity: bool,
    pub has_quantity_pct: bool,
    pub has_limit_price: bool,
    pub has_stop_price: bool,
    pub parent_id: String,
    pub atomic_group_id: String,
    pub oco_group_id: String,
    pub reduce_only: bool,
}
#[derive(Clone, Debug, Default, PartialEq)]
pub struct PineRunResult {
    pub job_id: String,
    pub plots: Vec<PinePlot>,
    pub order_intents: Vec<PineOrderIntent>,
    pub logs: Vec<String>,
    pub warnings: Vec<String>,
    pub diagnostics: Vec<PineDiagnostic>,
    pub metadata: PineWorkerMetadata,
    pub alerts: Vec<PineAlert>,
    pub visual_outputs: Vec<PineVisualOutput>,
    pub strategy_metrics: Option<PineStrategyMetrics>,
    pub session_id: String,
    pub session_revision: u64,
}
impl PineRunResult {
    fn from_proto(response: RunScriptResponse) -> Result<Self, PineExecutionError> {
        let metadata = metadata_from_proto(response.metadata)?;
        let plots = response
            .plots
            .into_iter()
            .map(|plot| {
                ensure_finite_values(&plot.values, "plot values")?;
                Ok(PinePlot {
                    name: plot.name,
                    values: plot.values,
                })
            })
            .collect::<Result<Vec<_>, PineExecutionError>>()?;
        let order_intents = response
            .order_intents
            .into_iter()
            .map(order_intent_from_proto)
            .collect::<Result<Vec<_>, PineExecutionError>>()?;
        let diagnostics = response
            .diagnostics
            .into_iter()
            .map(|diagnostic| PineDiagnostic {
                severity: diagnostic.severity,
                code: diagnostic.code,
                message: diagnostic.message,
                line: diagnostic.line,
                column: diagnostic.column,
            })
            .collect();
        let alerts = response
            .alerts
            .into_iter()
            .map(|alert| PineAlert {
                event_type: alert.r#type,
                id: alert.id,
                message: alert.message,
                title: alert.title,
                frequency: alert.frequency,
                bar_index: alert.bar_index,
                time: alert.time,
            })
            .collect();
        let visual_outputs = response
            .visual_outputs
            .into_iter()
            .map(|output| PineVisualOutput {
                kind: output.kind,
                name: output.name,
                payload_json: output.payload_json,
            })
            .collect();
        let strategy_metrics = response
            .strategy_metrics
            .map(|metrics| PineStrategyMetrics {
                buy_and_hold_pnl: metrics.buy_and_hold_pnl,
                buy_and_hold_per_gain: metrics.buy_and_hold_per_gain,
                strategy_outperformance: metrics.strategy_outperformance,
                has_buy_and_hold_pnl: metrics.has_buy_and_hold_pnl,
                has_buy_and_hold_per_gain: metrics.has_buy_and_hold_per_gain,
                has_strategy_outperformance: metrics.has_strategy_outperformance,
            });
        if let Some(metrics) = strategy_metrics.as_ref() {
            ensure_finite(metrics.buy_and_hold_pnl, "strategy buy-and-hold pnl")?;
            ensure_finite(metrics.buy_and_hold_per_gain, "strategy buy-and-hold gain")?;
            ensure_finite(metrics.strategy_outperformance, "strategy outperformance")?;
        }
        Ok(Self {
            job_id: response.job_id,
            plots,
            order_intents,
            logs: response.logs,
            warnings: response.warnings,
            diagnostics,
            metadata,
            alerts,
            visual_outputs,
            strategy_metrics,
            session_id: response.session_id,
            session_revision: response.session_revision,
        })
    }
}
#[derive(Clone, Debug, Default, PartialEq)]
pub struct PineAlert {
    pub event_type: String,
    pub id: String,
    pub message: String,
    pub title: String,
    pub frequency: String,
    pub bar_index: i32,
    pub time: i64,
}
#[derive(Clone, Debug, Default, PartialEq)]
pub struct PineVisualOutput {
    pub kind: String,
    pub name: String,
    pub payload_json: String,
}
#[derive(Clone, Debug, Default, PartialEq)]
pub struct PineStrategyMetrics {
    pub buy_and_hold_pnl: f64,
    pub buy_and_hold_per_gain: f64,
    pub strategy_outperformance: f64,
    pub has_buy_and_hold_pnl: bool,
    pub has_buy_and_hold_per_gain: bool,
    pub has_strategy_outperformance: bool,
}
#[derive(Clone, Debug)]
pub struct PineExecutionConfig {
    pub endpoint: String,
    pub bearer_token: Option<String>,
    pub connect_timeout: Duration,
    pub request_timeout: Duration,
    pub max_message_bytes: usize,
    pub max_source_bytes: usize,
    pub max_params: usize,
    pub max_candles: usize,
}
impl Default for PineExecutionConfig {
    fn default() -> Self {
        Self {
            endpoint: String::new(),
            bearer_token: None,
            connect_timeout: Duration::from_secs(1),
            request_timeout: Duration::from_secs(30),
            max_message_bytes: DEFAULT_MAX_MESSAGE_BYTES,
            max_source_bytes: DEFAULT_MAX_SOURCE_BYTES,
            max_params: DEFAULT_MAX_PARAMS,
            max_candles: 0,
        }
    }
}
impl PineExecutionConfig {
    pub fn for_worker(
        spec: &crate::process::WorkerProcessSpec,
        bearer_token: Option<String>,
        connect_timeout: Duration,
        request_timeout: Duration,
    ) -> Self {
        Self {
            endpoint: format!("http://{}", spec.address()),
            bearer_token,
            connect_timeout,
            request_timeout,
            ..Self::default()
        }
    }
}
#[derive(Debug, Error, Clone, Eq, PartialEq)]
pub enum PineExecutionError {
    #[error("pine worker endpoint is invalid: {0}")]
    InvalidEndpoint(String),
    #[error("pine worker token must contain at least 32 non-whitespace characters")]
    WeakToken,
    #[error("invalid pine worker request: {0}")]
    InvalidRequest(String),
    #[error("pine worker response is invalid: {0}")]
    InvalidResponse(String),
    #[error("pine worker is unavailable: {0}")]
    Unavailable(String),
    #[error("pine worker request timed out")]
    Timeout,
    #[error("pine worker request cancelled")]
    Cancelled,
    #[error("pine worker returned an error: {0}")]
    Remote(String),
    #[error("pine worker transport error: {0}")]
    Transport(String),
}
pub type PineExecutionFuture<'a> =
    Pin<Box<dyn Future<Output = Result<PineRunResult, PineExecutionError>> + Send + 'a>>;
pub trait PineExecutionPort: Send + Sync + std::fmt::Debug {
    fn run<'a>(&'a self, request: PineRunRequest) -> PineExecutionFuture<'a>;
    fn execute<'a>(&'a self, request: PineRunRequest) -> PineExecutionFuture<'a> {
        self.run(request)
    }
}
#[derive(Clone, Debug)]
pub struct GrpcPineExecutionPort {
    endpoint_uri: String,
    endpoint: Endpoint,
    bearer_token: Option<String>,
    request_timeout: Duration,
    max_message_bytes: usize,
    max_source_bytes: usize,
    max_params: usize,
    max_candles: usize,
}
impl GrpcPineExecutionPort {
    pub fn new(config: PineExecutionConfig) -> Result<Self, PineExecutionError> {
        let endpoint =
            normalize_endpoint(&config.endpoint)?.connect_timeout(config.connect_timeout);
        if config.connect_timeout.is_zero() || config.request_timeout.is_zero() {
            return Err(PineExecutionError::InvalidEndpoint(
                "connect and request timeouts must be positive".to_owned(),
            ));
        }
        if config.max_message_bytes == 0 {
            return Err(PineExecutionError::InvalidEndpoint(
                "max message bytes must be positive".to_owned(),
            ));
        }
        let bearer_token = config
            .bearer_token
            .map(|token| token.trim().to_owned())
            .filter(|token| !token.is_empty());
        if bearer_token.as_ref().is_some_and(|token| token.len() < 32) {
            return Err(PineExecutionError::WeakToken);
        }
        Ok(Self {
            endpoint_uri: config.endpoint.trim().to_owned(),
            endpoint,
            bearer_token,
            request_timeout: config.request_timeout,
            max_message_bytes: config.max_message_bytes,
            max_source_bytes: config.max_source_bytes,
            max_params: config.max_params,
            max_candles: config.max_candles,
        })
    }
    pub fn endpoint(&self) -> &str {
        &self.endpoint_uri
    }
    pub async fn run(&self, request: PineRunRequest) -> Result<PineRunResult, PineExecutionError> {
        timeout(self.request_timeout, self.run_inner(request))
            .await
            .map_err(|_| PineExecutionError::Timeout)?
    }
    pub async fn run_script(
        &self,
        request: PineRunRequest,
    ) -> Result<PineRunResult, PineExecutionError> {
        self.run(request).await
    }

    /// Invoke the worker's native AnalyzeScript RPC.  Strategy-pine analysis
    /// is intentionally kept separate from RunScript: the worker performs
    /// parser/diagnostic work without requiring market candles or a runtime
    /// session.  The response is returned as a JSON-compatible projection so
    /// the engine can preserve the public analysis wire shape without
    /// duplicating Pine's evolving schema.
    pub async fn analyze_script(
        &self,
        job_id: &str,
        script_id: &str,
        source: &str,
        include_ast: bool,
    ) -> Result<serde_json::Value, PineExecutionError> {
        let job_id = job_id.trim();
        let script_id = script_id.trim();
        if job_id.is_empty() || script_id.is_empty() {
            return Err(PineExecutionError::InvalidRequest(
                "analysis job and script ids are required".to_owned(),
            ));
        }
        if source.len() > self.max_source_bytes {
            return Err(PineExecutionError::InvalidRequest(format!(
                "source bytes exceed {}",
                self.max_source_bytes
            )));
        }
        timeout(
            self.request_timeout,
            self.analyze_script_inner(job_id, script_id, source, include_ast),
        )
        .await
        .map_err(|_| PineExecutionError::Timeout)?
    }

    async fn analyze_script_inner(
        &self,
        job_id: &str,
        script_id: &str,
        source: &str,
        include_ast: bool,
    ) -> Result<serde_json::Value, PineExecutionError> {
        let mut params = HashMap::new();
        params.insert("includeAst".to_owned(), include_ast.to_string());
        let request = AnalyzeScriptRequest {
            job_id: job_id.to_owned(),
            script_id: script_id.to_owned(),
            source: source.to_owned(),
            params,
        };
        if request.encoded_len() > self.max_message_bytes {
            return Err(PineExecutionError::InvalidRequest(format!(
                "encoded analysis request exceeds {} bytes",
                self.max_message_bytes
            )));
        }
        let channel = self
            .endpoint
            .clone()
            .connect()
            .await
            .map_err(|error| PineExecutionError::Unavailable(error.to_string()))?;
        let mut client = PineWorkerClient::new(channel)
            .max_decoding_message_size(self.max_message_bytes)
            .max_encoding_message_size(self.max_message_bytes);
        let mut tonic_request = Request::new(request);
        if let Some(token) = &self.bearer_token {
            let authorization = MetadataValue::try_from(format!("Bearer {token}"))
                .map_err(|error| PineExecutionError::InvalidRequest(error.to_string()))?;
            tonic_request
                .metadata_mut()
                .insert("authorization", authorization);
        }
        let response = client
            .analyze_script(tonic_request)
            .await
            .map_err(map_status)?
            .into_inner();
        if response.encoded_len() > self.max_message_bytes {
            return Err(PineExecutionError::InvalidResponse(format!(
                "encoded analysis response exceeds {} bytes",
                self.max_message_bytes
            )));
        }
        analysis_response_json(response)
    }
    /// Runs one call while observing an external cancellation future.
    pub async fn run_with_cancellation<F>(
        &self,
        request: PineRunRequest,
        cancellation: F,
    ) -> Result<PineRunResult, PineExecutionError>
    where
        F: Future<Output = ()> + Send,
    {
        tokio::pin!(cancellation);
        tokio::select! {
            result = self.run(request) => result,
            _ = &mut cancellation => Err(PineExecutionError::Cancelled),
        }
    }
    async fn run_inner(
        &self,
        request: PineRunRequest,
    ) -> Result<PineRunResult, PineExecutionError> {
        let proto_request = self.request_to_proto(&request)?;
        if proto_request.encoded_len() > self.max_message_bytes {
            return Err(PineExecutionError::InvalidRequest(format!(
                "encoded request exceeds {} bytes",
                self.max_message_bytes
            )));
        }
        let channel = self
            .endpoint
            .clone()
            .connect()
            .await
            .map_err(|error| PineExecutionError::Unavailable(error.to_string()))?;
        let mut client = PineWorkerClient::new(channel)
            .max_decoding_message_size(self.max_message_bytes)
            .max_encoding_message_size(self.max_message_bytes);
        let mut tonic_request = Request::new(proto_request);
        if let Some(token) = &self.bearer_token {
            let authorization = MetadataValue::try_from(format!("Bearer {token}"))
                .map_err(|error| PineExecutionError::InvalidRequest(error.to_string()))?;
            tonic_request
                .metadata_mut()
                .insert("authorization", authorization);
        }
        let response = client
            .run_script(tonic_request)
            .await
            .map_err(map_status)?
            .into_inner();
        if response.encoded_len() > self.max_message_bytes {
            return Err(PineExecutionError::InvalidResponse(format!(
                "encoded response exceeds {} bytes",
                self.max_message_bytes
            )));
        }
        if !response.error.trim().is_empty() {
            return Err(PineExecutionError::Remote(response.error));
        }
        let mut result = PineRunResult::from_proto(response)
            .map_err(|error| PineExecutionError::InvalidResponse(error.to_string()))?;
        if result.job_id.is_empty() {
            result.job_id = request.job_id.clone();
        }
        if result.job_id != request.job_id {
            return Err(PineExecutionError::InvalidResponse(format!(
                "response job id {:?} does not match request {:?}",
                result.job_id, request.job_id
            )));
        }
        if !request.session_operation.trim().is_empty() && result.session_id.is_empty() {
            result.session_id = request.session_id.clone();
        }
        if result.metadata.request_bytes > self.max_message_bytes
            || result.metadata.response_bytes > self.max_message_bytes
        {
            return Err(PineExecutionError::InvalidResponse(format!(
                "worker metadata exceeds {} bytes",
                self.max_message_bytes
            )));
        }
        validate_session_result(&request, &result)?;
        Ok(result)
    }
    fn request_to_proto(
        &self,
        request: &PineRunRequest,
    ) -> Result<RunScriptRequest, PineExecutionError> {
        validate_request(
            request,
            self.max_source_bytes,
            self.max_params,
            self.max_candles,
        )?;
        let mode = normalize_mode(&request.mode);
        Ok(RunScriptRequest {
            job_id: request.job_id.trim().to_owned(),
            script_id: request.script_id.trim().to_owned(),
            source: request.source.clone(),
            symbol: request.symbol.trim().to_owned(),
            timeframe: request.timeframe.trim().to_owned(),
            mode,
            candles: Some(candles_to_proto(&request.candles)),
            params: request
                .params
                .iter()
                .map(|(key, value)| (key.clone(), value.clone()))
                .collect::<HashMap<_, _>>(),
            include_plots: !matches!(normalize_mode(&request.mode).as_str(), "backtest"),
            session_id: request.session_id.trim().to_owned(),
            session_operation: normalize_session_operation(&request.session_operation),
            expected_revision: request.expected_revision,
            chart_type: normalize_chart_type(&request.chart_type),
        })
    }
}
impl PineExecutionPort for GrpcPineExecutionPort {
    fn run<'a>(&'a self, request: PineRunRequest) -> PineExecutionFuture<'a> {
        Box::pin(async move { self.run(request).await })
    }
}
fn normalize_endpoint(value: &str) -> Result<Endpoint, PineExecutionError> {
    let endpoint = value.trim();
    let uri = endpoint.strip_prefix("http://").ok_or_else(|| {
        PineExecutionError::InvalidEndpoint("only http loopback is supported".to_owned())
    })?;
    let authority = uri.split('/').next().unwrap_or_default();
    let socket = authority.parse::<SocketAddr>().map_err(|_| {
        PineExecutionError::InvalidEndpoint(
            "endpoint must use an explicit loopback port".to_owned(),
        )
    })?;
    if !socket.ip().is_loopback() || socket.port() == 0 {
        return Err(PineExecutionError::InvalidEndpoint(
            "endpoint must use an explicit loopback port".to_owned(),
        ));
    }
    Endpoint::from_shared(format!("http://{authority}"))
        .map_err(|error| PineExecutionError::InvalidEndpoint(error.to_string()))
}
fn validate_request(
    request: &PineRunRequest,
    max_source_bytes: usize,
    max_params: usize,
    max_candles: usize,
) -> Result<(), PineExecutionError> {
    require_text(&request.job_id, "job id")?;
    let operation = normalize_session_operation(&request.session_operation);
    if !request.session_operation.trim().is_empty()
        && !matches!(operation.as_str(), "open" | "append" | "close")
    {
        return Err(PineExecutionError::InvalidRequest(format!(
            "unsupported pine worker session operation: {}",
            request.session_operation
        )));
    }
    if !operation.is_empty() {
        require_text(&request.session_id, "session id")?;
    }
    let mode = normalize_mode(&request.mode);
    if !matches!(mode.as_str(), "backtest" | "live" | "analyze") {
        return Err(PineExecutionError::InvalidRequest(format!(
            "unsupported pine worker mode: {}",
            request.mode
        )));
    }
    if !operation.is_empty() && mode != "live" {
        return Err(PineExecutionError::InvalidRequest(
            "pine worker sessions require live mode".to_owned(),
        ));
    }
    if operation == "open" && request.expected_revision != 0 {
        return Err(PineExecutionError::InvalidRequest(
            "pine worker session open requires expected revision 0".to_owned(),
        ));
    }
    if operation == "append" && request.expected_revision == 0 {
        return Err(PineExecutionError::InvalidRequest(
            "pine worker session append requires a positive expected revision".to_owned(),
        ));
    }
    if operation == "close" {
        return Ok(());
    }
    require_text(&request.source, "source")?;
    require_text(&request.symbol, "symbol")?;
    require_text(&request.timeframe, "timeframe")?;
    let source_bytes = request.source.len();
    if source_bytes > max_source_bytes {
        return Err(PineExecutionError::InvalidRequest(format!(
            "source bytes exceed limit: {source_bytes} > {max_source_bytes}"
        )));
    }
    if request.params.len() > max_params {
        return Err(PineExecutionError::InvalidRequest(format!(
            "param count exceeds limit: {} > {max_params}",
            request.params.len()
        )));
    }
    if request.candles.is_empty() && mode != "analyze" {
        return Err(PineExecutionError::InvalidRequest(
            "candles are required".to_owned(),
        ));
    }
    if max_candles > 0 && request.candles.len() > max_candles {
        return Err(PineExecutionError::InvalidRequest(format!(
            "too many candles: {} > {max_candles}",
            request.candles.len()
        )));
    }
    for (index, candle) in request.candles.iter().enumerate() {
        validate_candle(candle, index)?;
    }
    Ok(())
}
fn validate_candle(candle: &PineCandle, index: usize) -> Result<(), PineExecutionError> {
    if candle.open_time <= 0 {
        return Err(PineExecutionError::InvalidRequest(format!(
            "candle {index}: open time is required"
        )));
    }
    if candle.close_time < candle.open_time && candle.close_time != 0 {
        return Err(PineExecutionError::InvalidRequest(format!(
            "candle {index}: close time is before open time"
        )));
    }
    for (name, value) in [
        ("open", candle.open),
        ("high", candle.high),
        ("low", candle.low),
        ("close", candle.close),
        ("volume", candle.volume),
    ] {
        if !value.is_finite() {
            return Err(PineExecutionError::InvalidRequest(format!(
                "candle {index}: {name} must be finite"
            )));
        }
    }
    if candle.high < candle.low {
        return Err(PineExecutionError::InvalidRequest(format!(
            "candle {index}: high is below low"
        )));
    }
    if candle.open < candle.low || candle.open > candle.high {
        return Err(PineExecutionError::InvalidRequest(format!(
            "candle {index}: open is outside high/low range"
        )));
    }
    if candle.close < candle.low || candle.close > candle.high {
        return Err(PineExecutionError::InvalidRequest(format!(
            "candle {index}: close is outside high/low range"
        )));
    }
    if candle.volume < 0.0 {
        return Err(PineExecutionError::InvalidRequest(format!(
            "candle {index}: volume is negative"
        )));
    }
    Ok(())
}
fn require_text(value: &str, name: &str) -> Result<(), PineExecutionError> {
    if value.trim().is_empty() {
        return Err(PineExecutionError::InvalidRequest(format!(
            "{name} is required"
        )));
    }
    Ok(())
}
fn normalize_mode(value: &str) -> String {
    match value.trim().to_ascii_lowercase().as_str() {
        "" => "backtest".to_owned(),
        value => value.to_owned(),
    }
}
fn normalize_session_operation(value: &str) -> String {
    value.trim().to_ascii_lowercase()
}
fn normalize_chart_type(value: &str) -> String {
    if value.trim().eq_ignore_ascii_case("heikinashi") {
        "heikinashi".to_owned()
    } else {
        "standard".to_owned()
    }
}
fn candles_to_proto(candles: &[PineCandle]) -> CandleBatch {
    let mut payload = vec![0; candles.len() * CANDLE_BATCH_RECORD_BYTES];
    for (index, candle) in candles.iter().enumerate() {
        let offset = index * CANDLE_BATCH_RECORD_BYTES;
        payload[offset..offset + 8].copy_from_slice(&candle.open_time.to_le_bytes());
        payload[offset + 8..offset + 16].copy_from_slice(&candle.close_time.to_le_bytes());
        payload[offset + 16..offset + 24].copy_from_slice(&candle.open.to_bits().to_le_bytes());
        payload[offset + 24..offset + 32].copy_from_slice(&candle.high.to_bits().to_le_bytes());
        payload[offset + 32..offset + 40].copy_from_slice(&candle.low.to_bits().to_le_bytes());
        payload[offset + 40..offset + 48].copy_from_slice(&candle.close.to_bits().to_le_bytes());
        payload[offset + 48..offset + 56].copy_from_slice(&candle.volume.to_bits().to_le_bytes());
    }
    CandleBatch {
        encoding_version: CANDLE_BATCH_ENCODING_VERSION,
        payload,
    }
}
fn metadata_from_proto(
    metadata: Option<wire::WorkerMetadata>,
) -> Result<PineWorkerMetadata, PineExecutionError> {
    let Some(metadata) = metadata else {
        return Ok(PineWorkerMetadata::default());
    };
    let duration_ms = nonnegative_i64(metadata.duration_ms, "duration_ms")?;
    let request_bytes = nonnegative_i32(metadata.request_bytes, "request_bytes")?;
    let response_bytes = nonnegative_i32(metadata.response_bytes, "response_bytes")?;
    let peak_rss_bytes = nonnegative_i64(metadata.peak_rss_bytes, "peak_rss_bytes")?;
    Ok(PineWorkerMetadata {
        worker_id: metadata.worker_id,
        version: metadata.version,
        pine_ts_version: metadata.pinets_version,
        script_hash: metadata.script_hash,
        data_hash: metadata.data_hash,
        duration: Duration::from_millis(duration_ms),
        request_bytes,
        response_bytes,
        peak_rss_bytes: usize::try_from(peak_rss_bytes).map_err(|_| {
            PineExecutionError::InvalidResponse("peak_rss_bytes exceeds platform usize".to_owned())
        })?,
    })
}
fn order_intent_from_proto(
    intent: wire::OrderIntent,
) -> Result<PineOrderIntent, PineExecutionError> {
    for (name, value) in [
        ("quantity", intent.quantity),
        ("quantity_pct", intent.quantity_pct),
        ("limit_price", intent.limit_price),
        ("stop_price", intent.stop_price),
    ] {
        ensure_finite(value, name)?;
    }
    Ok(PineOrderIntent {
        kind: intent.kind,
        id: intent.id,
        from_entry: intent.from_entry,
        direction: intent.direction,
        quantity: intent.quantity,
        quantity_pct: intent.quantity_pct,
        limit_price: intent.limit_price,
        stop_price: intent.stop_price,
        comment: intent.comment,
        alert_message: intent.alert_message,
        disable_alert: intent.disable_alert,
        bar_index: intent.bar_index,
        time: intent.time,
        has_quantity: intent.has_quantity,
        has_quantity_pct: intent.has_quantity_pct,
        has_limit_price: intent.has_limit_price,
        has_stop_price: intent.has_stop_price,
        parent_id: intent.parent_id,
        atomic_group_id: intent.atomic_group_id,
        oco_group_id: intent.oco_group_id,
        reduce_only: intent.reduce_only,
    })
}
fn validate_session_result(
    request: &PineRunRequest,
    result: &PineRunResult,
) -> Result<(), PineExecutionError> {
    let operation = normalize_session_operation(&request.session_operation);
    if operation.is_empty() {
        return Ok(());
    }
    if !result.session_id.is_empty() && result.session_id != request.session_id {
        return Err(PineExecutionError::InvalidResponse(format!(
            "response session id {:?} does not match request {:?}",
            result.session_id, request.session_id
        )));
    }
    let expected = match operation.as_str() {
        "open" => 1,
        "append" => request.expected_revision.saturating_add(1),
        "close" => request.expected_revision.saturating_add(1),
        _ => return Ok(()),
    };
    if result.session_revision != expected {
        return Err(PineExecutionError::InvalidResponse(format!(
            "session operation {operation} returned invalid revision {} (expected {expected})",
            result.session_revision
        )));
    }
    Ok(())
}
fn ensure_finite(value: f64, field: &str) -> Result<(), PineExecutionError> {
    if value.is_finite() {
        Ok(())
    } else {
        Err(PineExecutionError::InvalidResponse(format!(
            "{field} must be finite"
        )))
    }
}
fn ensure_finite_values(values: &[f64], field: &str) -> Result<(), PineExecutionError> {
    values
        .iter()
        .copied()
        .enumerate()
        .try_for_each(|(index, value)| ensure_finite(value, &format!("{field}[{index}]")))
}
fn nonnegative_i32(value: i32, field: &str) -> Result<usize, PineExecutionError> {
    usize::try_from(value)
        .map_err(|_| PineExecutionError::InvalidResponse(format!("{field} must not be negative")))
}
fn nonnegative_i64(value: i64, field: &str) -> Result<u64, PineExecutionError> {
    u64::try_from(value)
        .map_err(|_| PineExecutionError::InvalidResponse(format!("{field} must not be negative")))
}
fn map_status(status: Status) -> PineExecutionError {
    match status.code() {
        tonic::Code::Cancelled => PineExecutionError::Cancelled,
        tonic::Code::DeadlineExceeded => PineExecutionError::Timeout,
        tonic::Code::Unavailable => PineExecutionError::Unavailable(status.message().to_owned()),
        tonic::Code::InvalidArgument
        | tonic::Code::FailedPrecondition
        | tonic::Code::ResourceExhausted => PineExecutionError::Remote(status.message().to_owned()),
        _ => PineExecutionError::Transport(status.to_string()),
    }
}

fn analysis_response_json(
    response: AnalyzeScriptResponse,
) -> Result<serde_json::Value, PineExecutionError> {
    if !response.error.trim().is_empty() {
        return Err(PineExecutionError::Remote(response.error));
    }
    let diagnostics = response
        .diagnostics
        .into_iter()
        .map(|diagnostic| {
            serde_json::json!({
                "severity": diagnostic.severity,
                "code": diagnostic.code,
                "message": diagnostic.message,
                "line": diagnostic.line,
                "column": diagnostic.column,
            })
        })
        .collect::<Vec<_>>();
    let mut value = serde_json::json!({
        "jobId": response.job_id,
        "ok": response.ok,
        "diagnostics": diagnostics,
        "inputs": response.inputs,
        "plots": response.plots,
        "strategyConfig": response.strategy_config,
    });
    if let Some(metadata) = response.metadata {
        value["metadata"] = serde_json::json!({
            "workerId": metadata.worker_id,
            "version": metadata.version,
            "pineTsVersion": metadata.pinets_version,
            "scriptHash": metadata.script_hash,
            "durationMs": metadata.duration_ms,
            "requestBytes": metadata.request_bytes,
            "responseBytes": metadata.response_bytes,
        });
    }
    Ok(value)
}
#[cfg(test)]
mod tests;
