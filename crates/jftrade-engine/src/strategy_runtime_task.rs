//! Live strategy task: cancellable I/O, durable bar checkpoints and broker intents.
use super::*;
impl StrategyRuntimeManager {
    pub(crate) fn spawn_task(
        &self,
        instance_id: String,
        binding: Value,
        store: Arc<StrategyRuntimeStore>,
    ) -> Result<(), StrategyRuntimeWritePortError> {
        if self.stopping.load(Ordering::Acquire) {
            return Err(StrategyRuntimeWritePortError::Unavailable(
                "strategy runtime is stopping".to_owned(),
            ));
        }
        if self.is_task_alive(&instance_id) && !self.cancel(&instance_id) {
            return Err(StrategyRuntimeWritePortError::Unavailable(
                "strategy is still stopping".to_owned(),
            ));
        }
        let Some(worker) = self.worker.clone() else {
            return Err(StrategyRuntimeWritePortError::Unavailable(
                "strategy PineTS worker is unavailable".to_owned(),
            ));
        };
        let Some(quote) = self.quote.clone() else {
            return Err(StrategyRuntimeWritePortError::Unavailable(
                "strategy market-data quote port is unavailable".to_owned(),
            ));
        };
        let execution = self.execution.as_ref().map(Arc::downgrade);
        let execution_store = self.execution_store.as_ref().map(Arc::downgrade);
        let trade_runtime = self.trade_runtime.clone();
        let notification = self.notification.clone();
        let provider = Arc::clone(&self.provider);
        let router = self.router.clone();
        let script = binding_string_opt(&binding, &["script", "source"]);
        let script = script.ok_or_else(|| StrategyRuntimeWritePortError::Failed {
            status: 400,
            code: "STRATEGY_SCRIPT_REQUIRED".to_owned(),
            message: "strategy runtime requires a non-empty Pine script".to_owned(),
        })?;
        let active_symbols = binding_symbols(&binding).unwrap_or_default();
        if active_symbols.is_empty() {
            return Err(StrategyRuntimeWritePortError::Failed {
                status: 400,
                code: "STRATEGY_SYMBOLS_REQUIRED".to_owned(),
                message: "strategy runtime requires at least one symbol".to_owned(),
            });
        }
        let timeframe = binding_string_opt(&binding, &["interval", "timeframe"])
            .unwrap_or_else(|| "1m".to_owned());
        let script_id = binding_string_opt(&binding, &["scriptId", "definitionId", "strategyId"])
            .unwrap_or_else(|| instance_id.clone());
        let default_market =
            binding_string_opt(&binding, &["market"]).unwrap_or_else(|| "US".to_owned());
        let candle_limit = binding
            .get("candleLimit")
            .or_else(|| binding.get("limit"))
            .and_then(Value::as_u64)
            .map(|value| value.clamp(1, 1_000) as usize)
            .unwrap_or(200);
        let sessions = binding_sessions(&binding);
        let execution_mode = binding_string_opt(&binding, &["executionMode"]);
        let execute_orders = match execution_mode.as_deref() {
            Some("notify_only") => false,
            Some("live") => true,
            _ => binding
                .get("executeOrders")
                .and_then(Value::as_bool)
                .unwrap_or(true),
        };
        if execute_orders {
            if execution.is_none() {
                return Err(StrategyRuntimeWritePortError::Unavailable(
                    "strategy execution order port is unavailable".to_owned(),
                ));
            }
            {
                validate_strategy_execution_binding(&binding, &provider).map_err(|message| {
                    StrategyRuntimeWritePortError::Failed {
                        status: 400,
                        code: "STRATEGY_EXECUTION_BINDING_INVALID".to_owned(),
                        message,
                    }
                })?;
            }
        }
        let cancel = Arc::new(AtomicBool::new(false));
        let cancel_for_thread = Arc::clone(&cancel);
        let wake = Arc::new(tokio::sync::Notify::new());
        let wake_for_thread = Arc::clone(&wake);
        let (done_tx, done_rx) = std::sync::mpsc::channel();
        let id_for_thread = instance_id.clone();
        let weak_store = Arc::downgrade(&store);
        let join = std::thread::Builder::new()
            .name(format!("strategy-runtime-{instance_id}"))
            .spawn(move || {
                let _done_guard = done_tx;
                let runtime = match tokio::runtime::Builder::new_current_thread()
                    .enable_all()
                    .build()
                {
                    Ok(runtime) => runtime,
                    Err(error) => {
                        fail_strategy_task(
                            &store,
                            &router,
                            &id_for_thread,
                            &active_symbols,
                            format!("create strategy runtime executor: {error}"),
                        );
                        return;
                    }
                };
                let (mut last_closed_by_symbol, mut session_state_by_symbol) =
                    match restore_pine_runtime_state(&store, &id_for_thread, &active_symbols) {
                        Ok(state) => state,
                        Err(error) => {
                            fail_strategy_task(&store, &router, &id_for_thread, &active_symbols, error);
                            return;
                        }
                    };
                // The worker process is not assumed to survive an engine
                // restart.  A fresh worker session is opened on the first
                // cycle; durable checkpoints still fence already-processed
                // candles and intent keys. This does not restore JS heap state.
                for state in session_state_by_symbol.values_mut() {
                    state.revision = 0;
                }
                drop(store);
                let mut quote_cache_by_symbol: BTreeMap<String, f64> = BTreeMap::new();
                while !cancel_for_thread.load(Ordering::Acquire) {
                    let Some(store) = weak_store.upgrade() else { break };
                    let current_instance = match store.get_instance(&id_for_thread) {
                        Ok(Some(inst)) => inst,
                        Ok(None) => break,
                        Err(_) => {
                            runtime.block_on(async {
                                tokio::select! {
                                    _ = wake_for_thread.notified() => {}
                                    _ = tokio::time::sleep(std::time::Duration::from_millis(500)) => {}
                                }
                            });
                            continue;
                        }
                    };
                    if current_instance.status.eq_ignore_ascii_case("PAUSED") {
                        runtime.block_on(async {
                            tokio::select! {
                                _ = wake_for_thread.notified() => {}
                                _ = tokio::time::sleep(std::time::Duration::from_millis(500)) => {}
                            }
                        });
                        continue;
                    }
                    if !current_instance.status.eq_ignore_ascii_case("RUNNING")
                        && !current_instance.status.eq_ignore_ascii_case("STARTING")
                    {
                        break;
                    }
                    let current_risk_revision = Some(current_instance.runtime_risk_revision);
                    drop(store);
                    let mut last_closed = None;
                    let mut last_signal = None;
                    let mut last_order = None;
                    let mut cycle_error = None;
                    for requested_symbol in &active_symbols {
                        if cancel_for_thread.load(Ordering::Acquire) {
                            break;
                        }
                        let (market, symbol) =
                            split_strategy_symbol(requested_symbol, &default_market);
                        let candles_result = runtime.block_on(async {
                            tokio::select! {
                                biased;
                                _ = strategy_cancelled(&cancel_for_thread) => None,
                                result = tokio::time::timeout(
                                    STRATEGY_STOP_TIMEOUT,
                                    read_strategy_candles(
                                        quote.as_ref(),
                                        &market,
                                        &symbol,
                                        &timeframe,
                                        candle_limit,
                                        &sessions,
                                    ),
                                ) => Some(result),
                            }
                        });
                        if cancel_for_thread.load(Ordering::Acquire) { break; }
                        let Some(store) = weak_store.upgrade() else { break };
                        let candles = match candles_result {
                            None => break,
                            Some(Err(_)) => {
                                cycle_error = Some("market-data candle read exceeded stop timeout".to_owned());
                                break;
                            }
                            Some(Ok(Ok(candles))) => candles,
                            Some(Ok(Err(error))) => {
                                cycle_error = Some(error);
                                break;
                            }
                        };
                        let closed_count = candles.iter().take_while(|c| c.closed).count();
                        let closed_candles = &candles[..closed_count];
                        let in_progress_candle = candles.last().filter(|c| !c.closed);

                        if let Some(in_progress) = in_progress_candle {
                            quote_cache_by_symbol
                                .insert(requested_symbol.clone(), in_progress.close);
                        } else if let Some(last_candle) = candles.last() {
                            quote_cache_by_symbol
                                .insert(requested_symbol.clone(), last_candle.close);
                        }

                        let session = session_state_by_symbol
                            .entry(requested_symbol.clone())
                            .or_insert_with(|| SymbolSessionState {
                                revision: 0,
                                bar_count: 0,
                                submitted_intents: BTreeSet::new(),
                            });

                        if session.revision == 0 {
                            let recovering = last_closed_by_symbol.get(requested_symbol).copied();
                            let warmup: Vec<_> = closed_candles.iter()
                                .filter(|bar| recovering.is_none_or(|at| bar.open_time <= at)).collect();
                            if recovering.is_some() && warmup.is_empty() {
                                cycle_error = Some("Pine recovery history no longer contains the checkpoint; resync required".to_owned());
                                break;
                            }
                            let Some(latest_closed) = warmup.last() else {
                                continue;
                            };
                            let latest_open_time = latest_closed.open_time;
                            last_closed = Some(last_closed.map_or(latest_open_time, |value: i64| {
                                value.max(latest_open_time)
                            }));
                            let request = PineRunRequest {
                                job_id: format!(
                                    "live:{id_for_thread}:{symbol}:{}",
                                    latest_open_time
                                ),
                                script_id: script_id.clone(),
                                source: script.clone(),
                                symbol: format!("{market}.{symbol}"),
                                timeframe: timeframe.clone(),
                                chart_type: binding_string_opt(&binding, &["chartType"])
                                    .unwrap_or_else(|| "standard".to_owned()),
                                mode: "live".to_owned(),
                                candles: warmup
                                    .iter()
                                    .map(|c| c.candle.clone())
                                    .collect(),
                                params: binding_params(&binding),
                                session_id: format!("strategy:{id_for_thread}:{symbol}"),
                                session_operation: "open".to_owned(),
                                expected_revision: 0,
                            };
                            let response = match runtime.block_on(async {
                                tokio::select! {
                                    biased;
                                    _ = strategy_cancelled(&cancel_for_thread) => Err(PineExecutionError::Cancelled),
                                    result = run_session_request(|request| worker.run(request), request, &mut session.revision) => result,
                                }
                            }) {
                                Ok(response) => response,
                                Err(error) => {
                                    cycle_error = Some(pine_error_message(error));
                                    break;
                                }
                            };
                            session.bar_count = warmup.len();
                            if let Err(error) = record_worker_output(
                                &store,
                                &id_for_thread,
                                &response,
                                latest_open_time,
                            ) {
                                cycle_error = Some(format!("persist Pine worker output: {error}"));
                                break;
                            }
                            if let Err(error) = persist_pine_runtime_checkpoint(
                                &store,
                                &id_for_thread,
                                requested_symbol,
                                session.revision,
                                latest_open_time,
                                &session.submitted_intents,
                            ) {
                                cycle_error =
                                    Some(format!("persist Pine runtime checkpoint: {error}"));
                                break;
                            }
                            last_closed_by_symbol.insert(requested_symbol.clone(), latest_open_time);
                            if recovering.is_none() { continue; }
                        }

                        let last_processed_time = last_closed_by_symbol
                            .get(requested_symbol)
                            .copied()
                            .unwrap_or(0);
                        let newly_closed: Vec<&StrategyCandle> = closed_candles
                            .iter()
                            .filter(|c| c.open_time > last_processed_time)
                            .collect();
                        if newly_closed.is_empty() {
                            continue;
                        }

                        for closed_bar in newly_closed {
                            if cancel_for_thread.load(Ordering::Acquire) {
                                break;
                            }
                            let closed_open_time = closed_bar.open_time;
                            let closed_price = closed_bar.close;
                            last_closed = Some(last_closed.map_or(closed_open_time, |value: i64| {
                                value.max(closed_open_time)
                            }));
                            let closed_bar_index = i32::try_from(session.bar_count).unwrap_or(i32::MAX);
                            let request = PineRunRequest {
                                job_id: format!(
                                    "live:{id_for_thread}:{symbol}:{}",
                                    closed_open_time
                                ),
                                script_id: script_id.clone(),
                                source: script.clone(),
                                symbol: format!("{market}.{symbol}"),
                                timeframe: timeframe.clone(),
                                chart_type: binding_string_opt(&binding, &["chartType"])
                                    .unwrap_or_else(|| "standard".to_owned()),
                                mode: "live".to_owned(),
                                candles: vec![closed_bar.candle.clone()],
                                params: binding_params(&binding),
                                session_id: format!("strategy:{id_for_thread}:{symbol}"),
                                session_operation: "append".to_owned(),
                                expected_revision: session.revision,
                            };
                            let response = match runtime.block_on(run_session_request(
                                |request| worker.run(request),
                                request,
                                &mut session.revision,
                            )) {
                                Ok(response) => response,
                                Err(error) => {
                                    let err_msg = pine_error_message(error);
                                    let _ = store.append_audit_event(
                                        &id_for_thread,
                                        "SESSION_APPEND_RETRY",
                                        &format!(
                                            "Pine append failed; retained revision {}: {err_msg}",
                                            session.revision
                                        ),
                                        now_millis(),
                                    );
                                    break;
                                }
                            };
                            if cancel_for_thread.load(Ordering::Acquire) { break; }
                            session.bar_count += 1;
                            let raw_intents = current_bar_intents(
                                &response.order_intents,
                                closed_bar_index,
                                closed_open_time,
                            );
                            let mut current_intents = Vec::new();
                            for intent in raw_intents {
                                let key = format!(
                                    "{}:{}:{}",
                                    closed_open_time, intent.kind, intent.id
                                );
                                if !session.submitted_intents.contains(&key) {
                                    session.submitted_intents.insert(key);
                                    current_intents.push(intent);
                                }
                            }
                            if !current_intents.is_empty() {
                                last_signal = Some(closed_open_time);
                                if execute_orders {
                                    let account_inputs = match read_strategy_account_inputs(
                                        trade_runtime.as_deref(), &binding, &market, &symbol,
                                    ) {
                                        Ok(inputs) => inputs,
                                        Err(error) => {
                                            cycle_error = Some(error);
                                            break;
                                        }
                                    };
                                    if cancel_for_thread.load(Ordering::Acquire) { break; }
                                    let execution = execution.as_ref().and_then(std::sync::Weak::upgrade);
                                    let execution_store = execution_store.as_ref().and_then(std::sync::Weak::upgrade);
                                    let fallback_price = quote_cache_by_symbol
                                        .get(requested_symbol)
                                        .copied()
                                        .or(Some(closed_price));
                                    let guarded = execution.as_ref().map(|port| CancelledExecution {
                                        port: port.as_ref(), cancel: &cancel_for_thread,
                                    });
                                    match execute_strategy_intents(
                                        StrategyExecutionContext {
                                            execution: guarded.as_ref().map(|p| p as &dyn ExecutionWritePort),
                                            execution_store: execution_store.as_deref(),
                                            provider: &provider,
                                            store: &store,
                                            instance_id: &id_for_thread,
                                            market: &market,
                                            symbol: &symbol,
                                            binding: &binding,
                                            expected_risk_revision: current_risk_revision,
                                            fallback_price,
                                            sellable_quantity: account_inputs.sellable_quantity,
                                            current_position: account_inputs.current_position,
                                            available_cash: account_inputs.available_cash,
                                            virtual_account: None,
                                        },
                                        &current_intents,
                                    ) {
                                        Ok(true) => last_order = Some(closed_open_time),
                                        Ok(false) => {}
                                        Err(error) => {
                                            cycle_error = Some(error);
                                            break;
                                        }
                                    }
                                } else if let Err(error) = notify_strategy_intents(
                                    notification.as_deref(),
                                    &store,
                                    &id_for_thread,
                                    &format!("{market}.{symbol}"),
                                    &current_intents,
                                ) {
                                    cycle_error = Some(error);
                                    break;
                                }
                            }
                            if let Err(error) = record_worker_output(
                                &store,
                                &id_for_thread,
                                &response,
                                closed_open_time,
                            ) {
                                cycle_error = Some(format!("persist Pine worker output: {error}"));
                                break;
                            }
                            if let Err(error) = persist_pine_runtime_checkpoint(
                                &store,
                                &id_for_thread,
                                requested_symbol,
                                session.revision,
                                closed_open_time,
                                &session.submitted_intents,
                            ) {
                                cycle_error =
                                    Some(format!("persist Pine runtime checkpoint: {error}"));
                                break;
                            }
                            last_closed_by_symbol.insert(requested_symbol.clone(), closed_open_time);
                        }
                    }
                    if cancel_for_thread.load(Ordering::Acquire) { break; }
                    let Some(store) = weak_store.upgrade() else { break };
                    if let Some(error) = cycle_error {
                        let is_paused = store
                            .get_instance(&id_for_thread)
                            .ok()
                            .flatten()
                            .is_some_and(|inst| inst.status.eq_ignore_ascii_case("PAUSED"));
                        if is_paused {
                            continue;
                        }
                        close_strategy_pine_sessions(
                            &runtime,
                            worker.as_ref(),
                            &store,
                            &id_for_thread,
                            &script_id,
                            &script,
                            &default_market,
                            &timeframe,
                            &binding,
                            &session_state_by_symbol,
                        );
                        fail_strategy_task(
                            &store,
                            &router,
                            &id_for_thread,
                            &active_symbols,
                            error,
                        );
                        return;
                    }
                    let _ = store.update_observation_with_events(
                        &id_for_thread,
                        "RUNNING",
                        &active_symbols,
                        None,
                        last_closed,
                        last_signal,
                        last_order,
                        now_millis(),
                    );
                    sleep_until_next_strategy_poll(&cancel_for_thread);
                }
                let Some(store) = weak_store.upgrade() else { return };
                close_strategy_pine_sessions(
                    &runtime,
                    worker.as_ref(),
                    &store,
                    &id_for_thread,
                    &script_id,
                    &script,
                    &default_market,
                    &timeframe,
                    &binding,
                    &session_state_by_symbol,
                );
            })
            .map_err(|error| {
                StrategyRuntimeWritePortError::Unavailable(format!(
                    "start strategy runtime task: {error}"
                ))
            })?;
        self.tasks.lock().unwrap_or_else(|e| e.into_inner()).insert(
            instance_id,
            RuntimeTask {
                cancel,
                wake,
                done_rx,
                thread_handle: Some(join),
            },
        );
        Ok(())
    }
}

async fn strategy_cancelled(cancel: &AtomicBool) {
    while !cancel.load(Ordering::Acquire) {
        tokio::time::sleep(std::time::Duration::from_millis(20)).await;
    }
}

struct CancelledExecution<'a> {
    port: &'a dyn ExecutionWritePort,
    cancel: &'a AtomicBool,
}
impl std::fmt::Debug for CancelledExecution<'_> {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        f.write_str("CancelledExecution")
    }
}
impl ExecutionWritePort for CancelledExecution<'_> {
    fn mutate(
        &self,
        input: &crate::product::product_execution_write_port::ExecutionWriteInput,
    ) -> Result<Value, crate::product::product_execution_write_port::ExecutionWritePortError> {
        if self.cancel.load(Ordering::Acquire) {
            return Err(
                crate::product::product_execution_write_port::ExecutionWritePortError::Unavailable(
                    "strategy is stopping".to_owned(),
                ),
            );
        }
        self.port.mutate(input)
    }
}
