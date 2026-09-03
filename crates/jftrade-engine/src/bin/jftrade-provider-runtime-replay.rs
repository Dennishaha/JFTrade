#![forbid(unsafe_code)]

use std::error::Error;
use std::path::PathBuf;
use std::time::Duration;

use jftrade_engine::provider_runtime_compatibility::ProviderRuntimeAssembly;
use jftrade_integration_futu::{
    OpenDProbe, ReconcileAction, WireGlobalState, decode_frame, desired_subscriptions, encode_frame,
};
use jftrade_integration_marketdata_helper::HelperClientConfig;
use jftrade_integration_pine::{SessionOperation, WorkerHealth};
use jftrade_marketdata::{
    ActivationMode, CacheLookup, HealthStatus, InstrumentRef, ProviderDescriptor, Tick,
};
use serde::{Deserialize, Serialize};
use serde_json::{Value, json};

#[derive(Deserialize)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
struct ProviderRuntimeInput {
    version: String,
    providers: Vec<ProviderFixture>,
    helper: HelperFixture,
    pine: PineFixture,
    marketdata_operations: Vec<MarketDataOperation>,
    futu: FutuFixture,
}

#[derive(Deserialize)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
struct ProviderFixture {
    descriptor: ProviderDescriptor,
    health: HealthStatus,
}

#[derive(Deserialize)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
struct HelperFixture {
    endpoint: String,
    bearer_token: Option<String>,
}

#[derive(Deserialize)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
struct PineFixture {
    workers: Vec<PineWorkerFixture>,
    health: Vec<PineHealthFixture>,
    operations: Vec<PineOperation>,
}

#[derive(Deserialize)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
struct PineWorkerFixture {
    worker_id: String,
    address: String,
}

#[derive(Deserialize)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
struct PineHealthFixture {
    worker_id: String,
    health: WorkerHealth,
}

#[derive(Deserialize)]
#[serde(tag = "op", rename_all = "snake_case", deny_unknown_fields)]
enum PineOperation {
    Reserve {
        operation: String,
        #[serde(rename = "sessionId")]
        session_id: Option<String>,
        succeeded: bool,
    },
    Restart {
        #[serde(rename = "workerId")]
        worker_id: String,
    },
}

#[derive(Deserialize)]
#[serde(tag = "op", rename_all = "snake_case", deny_unknown_fields)]
enum MarketDataOperation {
    Activate {
        #[serde(rename = "providerId")]
        provider_id: String,
        mode: String,
    },
    UpdateHealth {
        #[serde(rename = "providerId")]
        provider_id: String,
        health: HealthStatus,
    },
    Acquire {
        #[serde(rename = "consumerId")]
        consumer_id: String,
        refs: Vec<InstrumentRef>,
        managed: bool,
        #[serde(rename = "nowMs")]
        now_ms: i64,
    },
    Release {
        #[serde(rename = "consumerId")]
        consumer_id: String,
    },
    Expire {
        #[serde(rename = "nowMs")]
        now_ms: i64,
        #[serde(rename = "ttlMs")]
        ttl_ms: i64,
    },
    CacheInsert {
        tick: Box<Tick>,
    },
    CacheLookup {
        #[serde(rename = "instrumentId")]
        instrument_id: String,
        #[serde(rename = "nowMs")]
        now_ms: i64,
        #[serde(rename = "maxAgeMs")]
        max_age_ms: i64,
    },
}

#[derive(Deserialize)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
struct FutuFixture {
    refs: Vec<InstrumentRef>,
    frame_proto_id: u32,
    frame_serial_no: u32,
    frame_body: Vec<u8>,
    generation: u64,
    subscribed_at_ms: i64,
    release_before_ms: i64,
    release_at_ms: i64,
    probe_cases: Vec<ProbeFixture>,
}

#[derive(Deserialize)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
struct ProbeFixture {
    name: String,
    state: Option<WireGlobalState>,
    version_supported: bool,
}

#[derive(Serialize)]
#[serde(rename_all = "camelCase")]
struct ProviderRuntimeOutput {
    version: String,
    helper: Value,
    marketdata: Vec<Value>,
    pine: Vec<Value>,
    pine_snapshot: Value,
    futu: Value,
}

fn main() {
    if let Err(error) = execute() {
        eprintln!("jftrade-provider-runtime-replay: {error}");
        std::process::exit(1);
    }
}

fn execute() -> Result<(), Box<dyn Error>> {
    let input_path = parse_input(std::env::args().skip(1))?;
    let input: ProviderRuntimeInput = serde_json::from_slice(&std::fs::read(input_path)?)?;
    let providers = input
        .providers
        .into_iter()
        .map(|provider| (provider.descriptor, provider.health));
    let pine_workers = input
        .pine
        .workers
        .iter()
        .map(|worker| (worker.worker_id.clone(), worker.address.clone()));
    let mut assembly = ProviderRuntimeAssembly::new(
        providers,
        HelperClientConfig {
            base_url: input.helper.endpoint,
            bearer_token: input.helper.bearer_token,
            request_timeout: Duration::from_secs(1),
            max_attempts: 3,
            retry_delay: Duration::from_millis(100),
        },
        pine_workers,
    )?;

    let helper = json!({
        "endpoint": assembly.helper.endpoint(),
        "authenticated": assembly.helper.uses_authentication(),
    });
    let marketdata = run_marketdata(&mut assembly, input.marketdata_operations);
    let pine = run_pine(&mut assembly, input.pine.health, input.pine.operations);
    let pine_snapshot = serde_json::to_value(assembly.pine.snapshot())?;
    let futu = run_futu(&mut assembly, input.futu)?;
    println!(
        "{}",
        serde_json::to_string(&ProviderRuntimeOutput {
            version: input.version,
            helper,
            marketdata,
            pine,
            pine_snapshot,
            futu,
        })?
    );
    Ok(())
}

fn run_marketdata(
    assembly: &mut ProviderRuntimeAssembly,
    operations: Vec<MarketDataOperation>,
) -> Vec<Value> {
    operations
        .into_iter()
        .map(|operation| match operation {
            MarketDataOperation::Activate { provider_id, mode } => {
                let mode = if mode == "startup_restore" {
                    ActivationMode::StartupRestore
                } else {
                    ActivationMode::Explicit
                };
                result_value(
                    "activate",
                    assembly
                        .marketdata
                        .activate(&provider_id, mode)
                        .and_then(|runtime| serde_json::to_value(runtime).map_err(|error| {
                            jftrade_marketdata::MarketDataError::InvalidSubscription(error.to_string())
                        })),
                )
            }
            MarketDataOperation::UpdateHealth { provider_id, health } => result_value(
                "update_health",
                assembly
                    .marketdata
                    .update_health(&provider_id, health)
                    .map(|()| json!({ "providerId": provider_id })),
            ),
            MarketDataOperation::Acquire {
                consumer_id,
                refs,
                managed,
                now_ms,
            } => result_value(
                "acquire",
                assembly
                    .marketdata
                    .acquire_demand(&consumer_id, refs, managed, now_ms)
                    .and_then(|snapshot| serde_json::to_value(snapshot).map_err(|error| {
                        jftrade_marketdata::MarketDataError::InvalidSubscription(error.to_string())
                    })),
            ),
            MarketDataOperation::Release { consumer_id } => json!({
                "op": "release",
                "ok": true,
                "value": { "released": assembly.marketdata.release_demand(&consumer_id) },
            }),
            MarketDataOperation::Expire { now_ms, ttl_ms } => json!({
                "op": "expire",
                "ok": true,
                "value": { "expired": assembly.marketdata.expire_demand(now_ms, ttl_ms) },
            }),
            MarketDataOperation::CacheInsert { tick } => {
                let generation = assembly.marketdata.generation();
                let result = assembly.marketdata.cache_mut().insert(*tick, generation);
                result_value(
                    "cache_insert",
                    result.map(|()| {
                        json!({ "instrumentCount": assembly.marketdata.cache().instrument_count() })
                    }),
                )
            }
            MarketDataOperation::CacheLookup {
                instrument_id,
                now_ms,
                max_age_ms,
            } => {
                let (state, tick) = match assembly
                    .marketdata
                    .cache()
                    .lookup(&instrument_id, now_ms, max_age_ms)
                {
                    CacheLookup::Fresh(tick) => ("fresh", Some(tick)),
                    CacheLookup::Stale(tick) => ("stale", Some(tick)),
                    CacheLookup::Missing => ("missing", None),
                };
                json!({ "op": "cache_lookup", "ok": true, "value": { "state": state, "tick": tick } })
            }
        })
        .collect()
}

fn run_pine(
    assembly: &mut ProviderRuntimeAssembly,
    health: Vec<PineHealthFixture>,
    operations: Vec<PineOperation>,
) -> Vec<Value> {
    let mut output = Vec::new();
    for item in health {
        let result = assembly
            .pine
            .record_health(&item.worker_id, Ok(item.health));
        output.push(match result {
            Ok(()) => json!({ "op": "health", "ok": true, "workerId": item.worker_id }),
            Err(error) => json!({ "op": "health", "ok": false, "error": error.to_string() }),
        });
    }
    for operation in operations {
        match operation {
            PineOperation::Reserve {
                operation,
                session_id,
                succeeded,
            } => {
                let operation = parse_session_operation(&operation);
                match assembly.pine.reserve(operation, session_id.as_deref()) {
                    Ok(reservation) => {
                        let worker_id = reservation.worker_id.clone();
                        let release = assembly.pine.release(reservation, succeeded);
                        output.push(json!({
                            "op": "reserve",
                            "ok": release.is_ok(),
                            "workerId": worker_id,
                            "succeeded": succeeded,
                        }));
                    }
                    Err(error) => output.push(json!({
                        "op": "reserve", "ok": false, "error": error.to_string(),
                    })),
                }
            }
            PineOperation::Restart { worker_id } => {
                let result = assembly.pine.record_restart(&worker_id);
                output.push(match result {
                    Ok(()) => json!({ "op": "restart", "ok": true, "workerId": worker_id }),
                    Err(error) => {
                        json!({ "op": "restart", "ok": false, "error": error.to_string() })
                    }
                });
            }
        }
    }
    output
}

fn run_futu(
    assembly: &mut ProviderRuntimeAssembly,
    fixture: FutuFixture,
) -> Result<Value, Box<dyn Error>> {
    let plan = desired_subscriptions(&fixture.refs);
    let subscribe = assembly.futu_subscriptions.actions(
        &fixture.refs,
        fixture.subscribed_at_ms,
        fixture.generation,
    );
    for action in &subscribe {
        assembly.futu_subscriptions.record_success(
            action,
            fixture.subscribed_at_ms,
            fixture.generation,
        );
    }
    let before_release =
        assembly
            .futu_subscriptions
            .actions(&[], fixture.release_before_ms, fixture.generation);
    let release =
        assembly
            .futu_subscriptions
            .actions(&[], fixture.release_at_ms, fixture.generation);
    let reconnect = assembly.futu_subscriptions.actions(
        &fixture.refs,
        fixture.release_at_ms,
        fixture.generation.saturating_add(1),
    );
    let packet = encode_frame(
        fixture.frame_proto_id,
        fixture.frame_serial_no,
        &fixture.frame_body,
    )?;
    let decoded = decode_frame(&packet)?;
    let probes = fixture
        .probe_cases
        .into_iter()
        .map(|probe| {
            let mapped = OpenDProbe::from_global_state(probe.state, probe.version_supported);
            json!({
                "name": probe.name,
                "marketDataReady": mapped.market_data_ready(),
                "probe": mapped,
            })
        })
        .collect::<Vec<_>>();
    Ok(json!({
        "plan": plan,
        "subscribe": action_keys(&subscribe),
        "beforeRelease": action_keys(&before_release),
        "release": action_keys(&release),
        "reconnect": action_keys(&reconnect),
        "frame": {
            "length": packet.len(),
            "protoId": decoded.header.proto_id,
            "serialNo": decoded.header.serial_no,
            "body": decoded.body,
        },
        "probes": probes,
    }))
}

fn action_keys(actions: &[ReconcileAction]) -> Vec<String> {
    actions
        .iter()
        .map(|action| match action {
            ReconcileAction::Subscribe { subscription } => {
                format!("subscribe:{}", subscription.key)
            }
            ReconcileAction::Unsubscribe { subscription } => {
                format!("unsubscribe:{}", subscription.key)
            }
        })
        .collect()
}

fn result_value<E: std::fmt::Display>(op: &str, result: Result<Value, E>) -> Value {
    match result {
        Ok(value) => json!({ "op": op, "ok": true, "value": value }),
        Err(error) => json!({ "op": op, "ok": false, "error": error.to_string() }),
    }
}

fn parse_session_operation(value: &str) -> SessionOperation {
    match value {
        "open" => SessionOperation::Open,
        "append" => SessionOperation::Append,
        "close" => SessionOperation::Close,
        _ => SessionOperation::None,
    }
}

fn parse_input(arguments: impl Iterator<Item = String>) -> Result<PathBuf, Box<dyn Error>> {
    let mut arguments = arguments;
    match (
        arguments.next().as_deref(),
        arguments.next(),
        arguments.next(),
    ) {
        (Some("--input"), Some(path), None) => Ok(path.into()),
        _ => Err("usage: jftrade-provider-runtime-replay --input <path>".into()),
    }
}
