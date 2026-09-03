#![forbid(unsafe_code)]

use std::error::Error;
use std::path::PathBuf;

use jftrade_engine::trading_strategy_compatibility::TradingStrategyReplay;
use jftrade_integration_futu::{
    RawOrderUpdate, TradeProtocol, map_order_update, plan_shadow_protocol,
};
use jftrade_kernel::WireTimestamp;
use jftrade_strategy::{ExecutionMode, Signal, StrategyCoordinator, TradePlannerPort};
use jftrade_trading::{
    AccountPortfolio, AccountRefresh, AccountSnapshot, BrokerSession, OrderCommand, RiskConfig,
    canonical_broker_status, canonical_stored_status, reconcile_status,
};
use serde::{Deserialize, Serialize};
use serde_json::{Value, json};

#[derive(Deserialize)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
struct TradingStrategyInput {
    version: String,
    planned_at: WireTimestamp,
    risk_config: RiskConfig,
    status_cases: Vec<String>,
    transitions: Vec<TransitionCase>,
    commands: Vec<OrderCommand>,
    events: Vec<RawOrderUpdate>,
    position_refreshes: Vec<AccountRefresh>,
    session_operations: Vec<SessionOperation>,
    protocols: Vec<TradeProtocol>,
    strategy_scenarios: Vec<StrategyScenario>,
}

#[derive(Deserialize)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
struct TransitionCase {
    current: String,
    incoming: String,
}

#[derive(Deserialize)]
#[serde(tag = "op", rename_all = "snake_case", deny_unknown_fields)]
enum SessionOperation {
    Connect,
    Authenticate {
        #[serde(rename = "quoteLogin")]
        quote_login: bool,
        #[serde(rename = "tradeLogin")]
        trade_login: bool,
        unlocked: bool,
    },
    RefreshAccounts {
        snapshot: AccountSnapshot,
    },
    Disconnect,
    BeginClose,
    FinishClose,
}

#[derive(Deserialize)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
struct StrategyScenario {
    name: String,
    mode: ExecutionMode,
    operations: Vec<StrategyOperation>,
}

#[derive(Deserialize)]
#[serde(tag = "op", rename_all = "snake_case", deny_unknown_fields)]
enum StrategyOperation {
    Start,
    Ready,
    Signal { signal: Box<Signal> },
    Disconnect,
    Pause,
    Resume,
    Stop,
    Stopped,
}

#[derive(Serialize)]
#[serde(rename_all = "camelCase")]
struct TradingStrategyOutput {
    version: String,
    statuses: Vec<Value>,
    transitions: Vec<Value>,
    commands: Vec<Value>,
    events: Vec<Value>,
    orders: Vec<Value>,
    position_refreshes: Vec<Value>,
    positions: Vec<Value>,
    audit: Vec<Value>,
    session: Vec<Value>,
    protocols: Vec<Value>,
    strategies: Vec<Value>,
}

fn main() {
    if let Err(error) = execute() {
        eprintln!("jftrade-trading-strategy-replay: {error}");
        std::process::exit(1);
    }
}

fn execute() -> Result<(), Box<dyn Error>> {
    let input_path = parse_input(std::env::args().skip(1))?;
    let input: TradingStrategyInput = serde_json::from_slice(&std::fs::read(input_path)?)?;
    let assembly = TradingStrategyReplay::new(input.risk_config, input.planned_at);
    let statuses = input
        .status_cases
        .into_iter()
        .map(|raw| json!({ "raw": raw, "status": canonical_broker_status(&raw) }))
        .collect();
    let transitions = input
        .transitions
        .into_iter()
        .map(|case| {
            let (status, accepted) = reconcile_status(
                canonical_stored_status(&case.current),
                canonical_stored_status(&case.incoming),
            );
            json!({
                "current": case.current,
                "incoming": case.incoming,
                "status": status,
                "accepted": accepted,
            })
        })
        .collect();
    let commands = input
        .commands
        .iter()
        .map(|command| {
            let result =
                assembly.with_shadow_mut(|shadow| shadow.plan_order(command, input.planned_at));
            nested_result("plan_order", result)
        })
        .collect();
    let events = input
        .events
        .into_iter()
        .map(|raw| match map_order_update(raw) {
            Ok(event) => nested_result(
                "order_event",
                assembly.with_shadow_mut(|shadow| shadow.apply_event(&event)),
            ),
            Err(error) => error_value("order_event", &error.to_string()),
        })
        .collect();
    let mut portfolio = AccountPortfolio::default();
    let position_refreshes = input
        .position_refreshes
        .iter()
        .map(|refresh| result_value("position_refresh", portfolio.apply_refresh(refresh)))
        .collect();
    let positions = portfolio
        .positions()
        .into_iter()
        .map(serde_json::to_value)
        .collect::<Result<Vec<_>, _>>()?;
    let session = run_session(input.session_operations);
    let protocols = input
        .protocols
        .into_iter()
        .map(|protocol| result_value("protocol", plan_shadow_protocol(protocol)))
        .collect();
    let strategies = input
        .strategy_scenarios
        .into_iter()
        .map(|scenario| run_strategy(&assembly, scenario))
        .collect();
    let orders = assembly.with_shadow(|shadow| {
        shadow
            .orders()
            .into_iter()
            .map(serde_json::to_value)
            .collect::<Result<Vec<_>, _>>()
    })??;
    let audit = assembly.with_shadow(|shadow| {
        shadow
            .audit()
            .iter()
            .map(serde_json::to_value)
            .collect::<Result<Vec<_>, _>>()
    })??;

    println!(
        "{}",
        serde_json::to_string(&TradingStrategyOutput {
            version: input.version,
            statuses,
            transitions,
            commands,
            events,
            orders,
            position_refreshes,
            positions,
            audit,
            session,
            protocols,
            strategies,
        })?
    );
    Ok(())
}

fn run_session(operations: Vec<SessionOperation>) -> Vec<Value> {
    let mut session = BrokerSession::default();
    operations
        .into_iter()
        .map(|operation| {
            let (name, changed) = match operation {
                SessionOperation::Connect => ("connect", session.connect()),
                SessionOperation::Authenticate {
                    quote_login,
                    trade_login,
                    unlocked,
                } => (
                    "authenticate",
                    session.authenticated(quote_login, trade_login, unlocked),
                ),
                SessionOperation::RefreshAccounts { snapshot } => {
                    ("refresh_accounts", session.refresh_accounts(snapshot))
                }
                SessionOperation::Disconnect => ("disconnect", session.disconnected()),
                SessionOperation::BeginClose => ("begin_close", session.begin_close()),
                SessionOperation::FinishClose => ("finish_close", session.finish_close()),
            };
            json!({
                "op": name,
                "changed": changed,
                "state": session.state(),
                "generation": session.generation(),
                "canRead": session.can_read(),
                "canTrade": session.can_trade(),
            })
        })
        .collect()
}

fn run_strategy(assembly: &TradingStrategyReplay, scenario: StrategyScenario) -> Value {
    let mut strategy = assembly.strategy(scenario.mode);
    let operations = scenario
        .operations
        .into_iter()
        .map(|operation| run_strategy_operation(&mut strategy, operation))
        .collect::<Vec<_>>();
    json!({
        "name": scenario.name,
        "mode": scenario.mode,
        "state": strategy.state(),
        "generation": strategy.generation(),
        "operations": operations,
    })
}

fn run_strategy_operation<P: TradePlannerPort>(
    strategy: &mut StrategyCoordinator<P>,
    operation: StrategyOperation,
) -> Value {
    match operation {
        StrategyOperation::Start => state_result("start", strategy.start(), strategy),
        StrategyOperation::Ready => state_result("ready", strategy.ready(), strategy),
        StrategyOperation::Signal { signal } => {
            result_value("signal", strategy.handle_signal(*signal))
        }
        StrategyOperation::Disconnect => {
            state_result("disconnect", strategy.disconnected(), strategy)
        }
        StrategyOperation::Pause => state_result("pause", strategy.pause(), strategy),
        StrategyOperation::Resume => state_result("resume", strategy.resume(), strategy),
        StrategyOperation::Stop => state_result("stop", strategy.stop(), strategy),
        StrategyOperation::Stopped => state_result("stopped", strategy.stopped(), strategy),
    }
}

fn state_result<P: TradePlannerPort>(
    name: &str,
    changed: bool,
    strategy: &StrategyCoordinator<P>,
) -> Value {
    json!({
        "op": name,
        "changed": changed,
        "state": strategy.state(),
        "generation": strategy.generation(),
    })
}

fn nested_result<T, E, O>(operation: &str, result: Result<Result<T, E>, O>) -> Value
where
    T: Serialize,
    E: std::fmt::Display,
    O: std::fmt::Display,
{
    match result {
        Ok(inner) => result_value(operation, inner),
        Err(error) => error_value(operation, &error.to_string()),
    }
}

fn result_value<T: Serialize, E: std::fmt::Display>(
    operation: &str,
    result: Result<T, E>,
) -> Value {
    match result {
        Ok(value) => json!({ "op": operation, "ok": true, "value": value }),
        Err(error) => error_value(operation, &error.to_string()),
    }
}

fn error_value(operation: &str, error: &str) -> Value {
    json!({ "op": operation, "ok": false, "error": error })
}

fn parse_input(mut arguments: impl Iterator<Item = String>) -> Result<PathBuf, Box<dyn Error>> {
    let Some(flag) = arguments.next() else {
        return Err("usage: jftrade-trading-strategy-replay --input <path>".into());
    };
    let Some(path) = arguments.next() else {
        return Err("--input requires a path".into());
    };
    if flag != "--input" || arguments.next().is_some() {
        return Err("usage: jftrade-trading-strategy-replay --input <path>".into());
    }
    Ok(PathBuf::from(path))
}
