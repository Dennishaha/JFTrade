use std::collections::{BTreeSet, HashMap};

use jftrade_kernel::Fixed8;

use crate::BacktestError;
use crate::fees::{AppliedFees, FeeEngine};
use crate::fingerprint::populate_result_hash;
use crate::indicators::calculate_indicators;
use crate::matching::{MatchMode, event_time, limit_price, stop_market_price};
use crate::model::{
    BacktestCase, BacktestOutput, Candle, EquityPoint, FillOutput, OrderIntent, OrderOutput,
    RunStatus,
};
use crate::report::{drawdown_metrics, metric_text};
use crate::validation::{normalized_order_type, validate_case, validate_submit_intent};

const INITIAL_ORDER_ID: u64 = 1_100_000_000;
const INITIAL_TRADE_ID: u64 = 1_200_000_000;

#[derive(Clone, Copy, Debug, Eq, PartialEq)]
enum Status {
    New,
    PartiallyFilled,
    Filled,
    Cancelled,
}

impl Status {
    const fn text(self) -> &'static str {
        match self {
            Self::New => "NEW",
            Self::PartiallyFilled => "PARTIALLY_FILLED",
            Self::Filled => "FILLED",
            Self::Cancelled => "CANCELED",
        }
    }

    const fn closed(self) -> bool {
        matches!(self, Self::Filled | Self::Cancelled)
    }
}

#[derive(Clone, Debug)]
struct OrderRecord {
    order_id: u64,
    client_order_id: String,
    side: String,
    order_type: String,
    quantity: Fixed8,
    limit_price: Fixed8,
    stop_price: Fixed8,
    reduce_only: bool,
    parent_order_id: u64,
    oco_group_id: String,
    has_children: bool,
    stop_triggered: bool,
    remaining: Fixed8,
    filled: Fixed8,
    filled_notional: Fixed8,
    average_price: Fixed8,
    status: Status,
    submitted_at: String,
    filled_at: String,
    closing_quantity: Fixed8,
    closing_pnl: Fixed8,
    closing_finalized: bool,
}

struct FillEvent {
    trade_id: u64,
    price: Fixed8,
    quantity: Fixed8,
    quote_quantity: Fixed8,
    time: String,
    fees: AppliedFees,
    realized_pnl: Fixed8,
}

struct Engine<'a> {
    case: &'a BacktestCase,
    cash: Fixed8,
    base_position: Fixed8,
    accounting_position: Fixed8,
    average_entry_price: Fixed8,
    position_cost_known: bool,
    realized_pnl: Fixed8,
    orders: Vec<OrderRecord>,
    matching_order: Vec<usize>,
    order_index_by_client_id: HashMap<String, usize>,
    fills: Vec<FillOutput>,
    equity_curve: Vec<EquityPoint>,
    warnings: Vec<String>,
    warning_keys: BTreeSet<String>,
    fee_engine: FeeEngine,
    next_order_id: u64,
    next_trade_id: u64,
    current_bar: Option<&'a Candle>,
    current_bar_budget: Fixed8,
    total_trades: usize,
    winning_trades: usize,
}

impl<'a> Engine<'a> {
    fn new(case: &'a BacktestCase) -> Self {
        Self {
            case,
            cash: case.initial_balance,
            base_position: Fixed8::ZERO,
            accounting_position: Fixed8::ZERO,
            average_entry_price: Fixed8::ZERO,
            position_cost_known: false,
            realized_pnl: Fixed8::ZERO,
            orders: Vec::new(),
            matching_order: Vec::new(),
            order_index_by_client_id: HashMap::new(),
            fills: Vec::new(),
            equity_curve: Vec::new(),
            warnings: Vec::new(),
            warning_keys: BTreeSet::new(),
            fee_engine: FeeEngine::new(&case.fee_rules),
            next_order_id: INITIAL_ORDER_ID,
            next_trade_id: INITIAL_TRADE_ID,
            current_bar: None,
            current_bar_budget: Fixed8::ZERO,
            total_trades: 0,
            winning_trades: 0,
        }
    }

    fn consume_bar(&mut self, candle: &'a Candle) -> Result<(), BacktestError> {
        self.current_bar = Some(candle);
        self.current_bar_budget = candle.volume.checked_mul("0.1".parse()?)?;
        if candle.volume <= Fixed8::ZERO && self.has_pending_orders() {
            self.warn_once(
                format!("zero-volume|{}", self.case.symbol),
                format!(
                    "conservative-bar-v1: {} bar ending {} has no positive volume; pending orders cannot fill on this bar",
                    self.case.symbol, candle.end
                ),
            );
        } else {
            let matching_order = self.matching_order.clone();
            for order_index in matching_order {
                self.try_fill(order_index, candle, MatchMode::FullBar)?;
                if self.current_bar_budget <= Fixed8::ZERO {
                    break;
                }
            }
        }
        self.record_equity(candle)
    }

    fn apply_bar_intents(&mut self, bar_index: usize) -> Result<(), BacktestError> {
        let intents: Vec<&OrderIntent> = self
            .case
            .intents
            .iter()
            .filter(|intent| intent.bar_index == bar_index)
            .collect();
        let mut atomic_groups = BTreeSet::new();
        for intent in &intents {
            if intent.action == "cancel" {
                self.cancel_by_client_id(&intent.target_id)?;
            } else if intent.atomic_group_id.is_empty() {
                self.submit_one(intent, 0, false)?;
            } else {
                atomic_groups.insert(intent.atomic_group_id.clone());
            }
        }
        for group_id in atomic_groups {
            let group: Vec<&OrderIntent> = intents
                .iter()
                .copied()
                .filter(|intent| intent.atomic_group_id == group_id)
                .collect();
            self.submit_atomic(&group_id, &group)?;
        }
        Ok(())
    }

    fn submit_atomic(
        &mut self,
        group_id: &str,
        intents: &[&OrderIntent],
    ) -> Result<(), BacktestError> {
        if group_id.is_empty() || intents.len() < 2 {
            return Err(BacktestError::InvalidInput(
                "atomic group requires an id and at least two orders".to_owned(),
            ));
        }
        let local_ids: BTreeSet<&str> = intents.iter().map(|intent| intent.id.as_str()).collect();
        for intent in intents {
            if !intent.parent_id.is_empty() && !local_ids.contains(intent.parent_id.as_str()) {
                return Err(BacktestError::InvalidInput(format!(
                    "atomic group {group_id} child {} has no parent {}",
                    intent.id, intent.parent_id
                )));
            }
        }
        let mut created = Vec::with_capacity(intents.len());
        for intent in intents {
            created.push(self.submit_one(intent, 0, true)?);
        }
        for (&intent, &order_index) in intents.iter().zip(&created) {
            if intent.parent_id.is_empty() {
                continue;
            }
            let parent_index = *self
                .order_index_by_client_id
                .get(&intent.parent_id)
                .ok_or_else(|| {
                    BacktestError::InvalidInput("atomic parent disappeared".to_owned())
                })?;
            self.orders[order_index].parent_order_id = self.orders[parent_index].order_id;
            self.orders[parent_index].has_children = true;
        }
        created.sort_by_key(|&index| self.atomic_priority(index));
        self.matching_order.extend(created.iter().copied());
        if self.case.process_orders_on_close {
            let candle = self.current_bar.ok_or_else(|| {
                BacktestError::InvalidInput("close execution requires a current bar".to_owned())
            })?;
            for order_index in created {
                self.try_fill(order_index, candle, MatchMode::ClosePoint)?;
            }
        }
        Ok(())
    }

    fn submit_one(
        &mut self,
        intent: &OrderIntent,
        parent_order_id: u64,
        defer_matching_order: bool,
    ) -> Result<usize, BacktestError> {
        validate_submit_intent(intent)?;
        if self.order_index_by_client_id.contains_key(&intent.id) {
            return Err(BacktestError::InvalidInput(format!(
                "duplicate order intent id {}",
                intent.id
            )));
        }
        self.next_order_id = self
            .next_order_id
            .checked_add(1)
            .ok_or_else(|| BacktestError::Arithmetic("order id overflow".to_owned()))?;
        let submitted_at = self
            .current_bar
            .map_or_else(String::new, |candle| candle.end.to_string());
        let order_index = self.orders.len();
        self.orders.push(OrderRecord {
            order_id: self.next_order_id,
            client_order_id: intent.id.clone(),
            side: intent.side.clone(),
            order_type: normalized_order_type(&intent.order_type).to_owned(),
            quantity: intent.quantity,
            limit_price: intent.limit_price,
            stop_price: intent.stop_price,
            reduce_only: intent.reduce_only,
            parent_order_id,
            oco_group_id: intent.oco_group_id.clone(),
            has_children: false,
            stop_triggered: false,
            remaining: intent.quantity,
            filled: Fixed8::ZERO,
            filled_notional: Fixed8::ZERO,
            average_price: Fixed8::ZERO,
            status: Status::New,
            submitted_at,
            filled_at: String::new(),
            closing_quantity: Fixed8::ZERO,
            closing_pnl: Fixed8::ZERO,
            closing_finalized: false,
        });
        self.order_index_by_client_id
            .insert(intent.id.clone(), order_index);
        if !defer_matching_order {
            self.matching_order.push(order_index);
        }
        if self.case.process_orders_on_close && !defer_matching_order {
            let candle = self.current_bar.ok_or_else(|| {
                BacktestError::InvalidInput("close execution requires a current bar".to_owned())
            })?;
            self.try_fill(order_index, candle, MatchMode::ClosePoint)?;
        }
        Ok(order_index)
    }

    fn try_fill(
        &mut self,
        order_index: usize,
        candle: &Candle,
        mode: MatchMode,
    ) -> Result<(), BacktestError> {
        if self.orders[order_index].remaining <= Fixed8::ZERO {
            return Ok(());
        }
        let parent_order_id = self.orders[order_index].parent_order_id;
        if parent_order_id != 0
            && !self
                .orders
                .iter()
                .any(|order| order.order_id == parent_order_id && order.status == Status::Filled)
        {
            return Ok(());
        }
        if self.current_bar_budget <= Fixed8::ZERO {
            return Ok(());
        }
        let Some(price) = self.match_price(order_index, candle, mode)? else {
            return Ok(());
        };
        if price <= Fixed8::ZERO {
            return Ok(());
        }
        let mut quantity = self.orders[order_index]
            .remaining
            .min(self.current_bar_budget);
        if self.orders[order_index].reduce_only {
            let reducible = self.reduce_only_quantity(&self.orders[order_index].side)?;
            if reducible <= Fixed8::ZERO {
                self.cancel_order(order_index, event_time(candle, mode))?;
                return Ok(());
            }
            quantity = quantity.min(reducible);
        }
        quantity = quantity.truncate_to_increment(self.case.market.quantity_step)?;
        if quantity <= Fixed8::ZERO {
            self.warn_once(
                format!("liquidity-step|{}", self.case.symbol),
                format!(
                    "conservative-bar-v1: liquidity budget for {} is below tradable quantity step; order {} remains pending",
                    self.case.symbol, self.orders[order_index].client_order_id
                ),
            );
            return Ok(());
        }
        if self.case.market.min_quantity > Fixed8::ZERO && quantity < self.case.market.min_quantity
        {
            self.warn_once(
                format!("liquidity-min|{}", self.case.symbol),
                format!(
                    "conservative-bar-v1: liquidity budget for {} is below min quantity {}; order {} remains pending",
                    self.case.symbol,
                    self.case.market.min_quantity,
                    self.orders[order_index].client_order_id
                ),
            );
            return Ok(());
        }
        self.apply_fill(order_index, quantity, price, event_time(candle, mode))?;
        self.current_bar_budget = self.current_bar_budget.checked_sub(quantity)?;
        Ok(())
    }

    fn match_price(
        &mut self,
        order_index: usize,
        candle: &Candle,
        mode: MatchMode,
    ) -> Result<Option<Fixed8>, BacktestError> {
        let order = &self.orders[order_index];
        match order.order_type.as_str() {
            "market" => {
                let price = if matches!(mode, MatchMode::ClosePoint) {
                    candle.close
                } else {
                    candle.open
                };
                self.apply_slippage(&order.side, price).map(Some)
            }
            "limit" | "limit_maker" => Ok(limit_price(
                order.side.as_str(),
                order.limit_price,
                candle,
                mode,
            )),
            "stop_market" => stop_market_price(order.side.as_str(), order.stop_price, candle, mode)
                .map(|price| self.apply_slippage(&order.side, price))
                .transpose(),
            "stop_limit" => {
                if !self.orders[order_index].stop_triggered {
                    if stop_market_price(order.side.as_str(), order.stop_price, candle, mode)
                        .is_none()
                    {
                        return Ok(None);
                    }
                    self.orders[order_index].stop_triggered = true;
                    return Ok(None);
                }
                let order = &self.orders[order_index];
                Ok(limit_price(
                    order.side.as_str(),
                    order.limit_price,
                    candle,
                    mode,
                ))
            }
            other => {
                self.warn_once(
                    format!("unsupported-order-type|{other}"),
                    format!("conservative-bar-v1: unsupported order type {other} remains pending"),
                );
                Ok(None)
            }
        }
    }

    fn apply_fill(
        &mut self,
        order_index: usize,
        quantity: Fixed8,
        price: Fixed8,
        at: String,
    ) -> Result<(), BacktestError> {
        let quote_quantity = quantity.checked_mul(price)?;
        let side = self.orders[order_index].side.clone();
        match side.as_str() {
            "buy" => {
                self.cash = self.cash.checked_sub(quote_quantity)?;
                self.base_position = self.base_position.checked_add(quantity)?;
            }
            "sell" => {
                self.cash = self.cash.checked_add(quote_quantity)?;
                self.base_position = self.base_position.checked_sub(quantity)?;
            }
            _ => {
                return Err(BacktestError::InvalidInput(format!(
                    "unsupported side {side}"
                )));
            }
        }
        let (closed_quantity, realized) = self.apply_position_fill(&side, quantity, price)?;
        self.realized_pnl = self.realized_pnl.checked_add(realized)?;
        let order_id = self.orders[order_index].order_id;
        let fees = self.fee_engine.apply(order_id, &side, price, quantity)?;
        self.cash = self.cash.checked_sub(fees.total)?;
        self.update_filled_order(order_index, quantity, quote_quantity, at.clone())?;
        if closed_quantity > Fixed8::ZERO {
            let order = &mut self.orders[order_index];
            order.closing_quantity = order.closing_quantity.checked_add(closed_quantity)?;
            order.closing_pnl = order.closing_pnl.checked_add(realized)?;
        }
        self.next_trade_id = self
            .next_trade_id
            .checked_add(1)
            .ok_or_else(|| BacktestError::Arithmetic("trade id overflow".to_owned()))?;
        let order = &self.orders[order_index];
        self.fills.push(fill_output(
            order,
            FillEvent {
                trade_id: self.next_trade_id,
                price,
                quantity,
                quote_quantity,
                time: at,
                fees,
                realized_pnl: realized,
            },
        ));
        let order_closed = order.status.closed();
        let cancel_oco = !order.oco_group_id.is_empty() && order.filled > Fixed8::ZERO;
        if order_closed {
            self.finalize_closing_order(order_index);
        }
        if cancel_oco {
            self.cancel_oco_siblings(order_index)?;
        }
        Ok(())
    }

    fn update_filled_order(
        &mut self,
        order_index: usize,
        quantity: Fixed8,
        quote_quantity: Fixed8,
        at: String,
    ) -> Result<(), BacktestError> {
        let order = &mut self.orders[order_index];
        order.filled = order.filled.checked_add(quantity)?;
        order.remaining = order.remaining.checked_sub(quantity)?;
        order.filled_notional = order.filled_notional.checked_add(quote_quantity)?;
        order.average_price = order.filled_notional.checked_div(order.filled)?;
        order.filled_at = at;
        order.status = if order.remaining > Fixed8::ZERO {
            Status::PartiallyFilled
        } else {
            Status::Filled
        };
        Ok(())
    }

    fn apply_position_fill(
        &mut self,
        side: &str,
        quantity: Fixed8,
        price: Fixed8,
    ) -> Result<(Fixed8, Fixed8), BacktestError> {
        let delta = if side == "sell" {
            quantity.checked_neg()?
        } else {
            quantity
        };
        let current = self.accounting_position;
        if current.is_zero() || current.signum() == delta.signum() {
            let next = current.checked_add(delta)?;
            if current.is_zero() {
                self.average_entry_price = price;
                self.position_cost_known = price > Fixed8::ZERO;
            } else if self.position_cost_known && price > Fixed8::ZERO {
                let current_cost = self
                    .average_entry_price
                    .checked_mul(current.checked_abs()?)?;
                let fill_cost = price.checked_mul(quantity)?;
                self.average_entry_price = current_cost
                    .checked_add(fill_cost)?
                    .checked_div(next.checked_abs()?)?;
            } else {
                self.average_entry_price = Fixed8::ZERO;
                self.position_cost_known = false;
            }
            self.accounting_position = next;
            return Ok((Fixed8::ZERO, Fixed8::ZERO));
        }
        let closed_quantity = quantity.min(current.checked_abs()?);
        let realized = if !self.position_cost_known || price <= Fixed8::ZERO {
            Fixed8::ZERO
        } else if current > Fixed8::ZERO {
            price
                .checked_sub(self.average_entry_price)?
                .checked_mul(closed_quantity)?
        } else {
            self.average_entry_price
                .checked_sub(price)?
                .checked_mul(closed_quantity)?
        };
        let next = current.checked_add(delta)?;
        self.accounting_position = next;
        if next.is_zero() {
            self.average_entry_price = Fixed8::ZERO;
            self.position_cost_known = false;
        } else if next.signum() != current.signum() {
            self.average_entry_price = price;
            self.position_cost_known = price > Fixed8::ZERO;
        }
        Ok((closed_quantity, realized))
    }

    fn cancel_by_client_id(&mut self, client_order_id: &str) -> Result<(), BacktestError> {
        let Some(&order_index) = self.order_index_by_client_id.get(client_order_id) else {
            return Ok(());
        };
        let at = self
            .current_bar
            .map_or_else(String::new, |candle| candle.end.to_string());
        self.cancel_order(order_index, at)
    }

    fn cancel_order(&mut self, order_index: usize, at: String) -> Result<(), BacktestError> {
        if self.orders[order_index].remaining <= Fixed8::ZERO {
            return Ok(());
        }
        let parent_order_id = self.orders[order_index].order_id;
        self.orders[order_index].remaining = Fixed8::ZERO;
        self.orders[order_index].status = Status::Cancelled;
        self.orders[order_index].filled_at = at.clone();
        self.finalize_closing_order(order_index);
        let dependents: Vec<usize> = self
            .orders
            .iter()
            .enumerate()
            .filter_map(|(index, order)| {
                (order.parent_order_id == parent_order_id && order.remaining > Fixed8::ZERO)
                    .then_some(index)
            })
            .collect();
        for dependent in dependents {
            self.cancel_order(dependent, at.clone())?;
        }
        Ok(())
    }

    fn cancel_oco_siblings(&mut self, filled_index: usize) -> Result<(), BacktestError> {
        let group = self.orders[filled_index].oco_group_id.clone();
        let at = self.orders[filled_index].filled_at.clone();
        let siblings: Vec<usize> = self
            .orders
            .iter()
            .enumerate()
            .filter_map(|(index, order)| {
                (index != filled_index
                    && order.oco_group_id == group
                    && order.remaining > Fixed8::ZERO)
                    .then_some(index)
            })
            .collect();
        for sibling in siblings {
            self.cancel_order(sibling, at.clone())?;
        }
        Ok(())
    }

    fn finalize_closing_order(&mut self, order_index: usize) {
        let order = &mut self.orders[order_index];
        if order.closing_finalized || order.closing_quantity <= Fixed8::ZERO {
            return;
        }
        order.closing_finalized = true;
        self.total_trades += 1;
        if order.closing_pnl > Fixed8::ZERO {
            self.winning_trades += 1;
        }
    }

    fn reduce_only_quantity(&self, side: &str) -> Result<Fixed8, BacktestError> {
        match side {
            "sell" if self.base_position > Fixed8::ZERO => self
                .base_position
                .checked_abs()
                .map_err(BacktestError::from),
            "buy" if self.base_position < Fixed8::ZERO => self
                .base_position
                .checked_abs()
                .map_err(BacktestError::from),
            "buy" | "sell" => Ok(Fixed8::ZERO),
            _ => Err(BacktestError::InvalidInput(format!(
                "unsupported side {side}"
            ))),
        }
    }

    fn apply_slippage(&self, side: &str, price: Fixed8) -> Result<Fixed8, BacktestError> {
        if self.case.slippage_ticks == 0
            || price <= Fixed8::ZERO
            || self.case.market.tick_size <= Fixed8::ZERO
        {
            return Ok(price);
        }
        let ticks: Fixed8 = self.case.slippage_ticks.to_string().parse()?;
        let offset = self.case.market.tick_size.checked_mul(ticks)?;
        let slipped = if side == "buy" {
            price.checked_add(offset)?
        } else {
            price.checked_sub(offset)?
        };
        if slipped <= Fixed8::ZERO {
            return Ok(Fixed8::ZERO);
        }
        slipped
            .truncate_to_increment(self.case.market.tick_size)
            .map_err(BacktestError::from)
    }

    fn record_equity(&mut self, candle: &Candle) -> Result<(), BacktestError> {
        let equity = self
            .cash
            .checked_add(self.base_position.checked_mul(candle.close)?)?;
        self.equity_curve.push(EquityPoint {
            time: candle.end.to_string(),
            equity: equity.storage_text(),
        });
        Ok(())
    }

    fn atomic_priority(&self, order_index: usize) -> u8 {
        let order = &self.orders[order_index];
        if order.parent_order_id == 0 {
            0
        } else if order.order_type == "stop_market" {
            1
        } else {
            2
        }
    }

    fn has_pending_orders(&self) -> bool {
        self.orders
            .iter()
            .any(|order| order.remaining > Fixed8::ZERO)
    }

    fn warn_once(&mut self, key: String, message: String) {
        if self.warning_keys.insert(key) {
            self.warnings.push(message);
        }
    }
}

pub(crate) fn run_case(case: &BacktestCase) -> Result<BacktestOutput, BacktestError> {
    validate_case(case)?;
    let mut engine = Engine::new(case);
    let mut status = RunStatus::Completed;
    let mut processed_bars = 0_usize;
    for (bar_index, candle) in case.candles.iter().enumerate() {
        if case.cancel_before_bar == Some(bar_index) {
            status = RunStatus::Cancelled;
            break;
        }
        engine.consume_bar(candle)?;
        engine.apply_bar_intents(bar_index)?;
        processed_bars += 1;
    }
    for order_index in 0..engine.orders.len() {
        if engine.orders[order_index].status.closed() {
            engine.finalize_closing_order(order_index);
        }
    }
    let last_close = case
        .candles
        .get(processed_bars.saturating_sub(1))
        .map_or(Fixed8::ZERO, |candle| candle.close);
    let final_equity = engine
        .cash
        .checked_add(engine.base_position.checked_mul(last_close)?)?;
    let (max_drawdown, current_drawdown, drawdown_curve) = drawdown_metrics(&engine.equity_curve)?;
    let closes: Vec<Fixed8> = case
        .candles
        .iter()
        .take(processed_bars)
        .map(|candle| candle.close)
        .collect();
    let total_fees = engine
        .fee_engine
        .broker_total()
        .checked_add(engine.fee_engine.market_total())?;
    let win_rate = if engine.total_trades == 0 {
        "0".to_owned()
    } else {
        metric_text(engine.winning_trades as f64 / engine.total_trades as f64)
    };
    let mut output = BacktestOutput {
        id: case.id.clone(),
        status,
        processed_bars,
        cash: engine.cash.storage_text(),
        base_position: engine.base_position.storage_text(),
        final_equity: final_equity.storage_text(),
        realized_pnl: engine.realized_pnl.storage_text(),
        total_broker_fees: engine.fee_engine.broker_total().storage_text(),
        total_market_fees: engine.fee_engine.market_total().storage_text(),
        total_fees: total_fees.storage_text(),
        total_fills: engine.fills.len(),
        total_trades: engine.total_trades,
        winning_trades: engine.winning_trades,
        win_rate,
        max_drawdown,
        current_drawdown,
        orders: engine.orders.iter().map(order_output).collect(),
        fills: engine.fills,
        equity_curve: engine.equity_curve,
        drawdown_curve,
        fee_breakdown: engine.fee_engine.breakdown(),
        indicators: calculate_indicators(&closes, &case.indicator_periods)?,
        warnings: engine.warnings,
        result_hash: String::new(),
    };
    populate_result_hash(&mut output)?;
    Ok(output)
}

fn fill_output(order: &OrderRecord, event: FillEvent) -> FillOutput {
    FillOutput {
        trade_id: event.trade_id.to_string(),
        order_id: order.order_id.to_string(),
        client_order_id: order.client_order_id.clone(),
        side: order.side.clone(),
        price: event.price.storage_text(),
        quantity: event.quantity.storage_text(),
        quote_quantity: event.quote_quantity.storage_text(),
        time: event.time,
        maker: matches!(
            order.order_type.as_str(),
            "limit" | "limit_maker" | "stop_limit"
        ),
        broker_fee: event.fees.broker.storage_text(),
        market_fee: event.fees.market.storage_text(),
        total_fee: event.fees.total.storage_text(),
        realized_pnl: event.realized_pnl.storage_text(),
    }
}

fn order_output(order: &OrderRecord) -> OrderOutput {
    OrderOutput {
        order_id: order.order_id.to_string(),
        client_order_id: order.client_order_id.clone(),
        side: order.side.clone(),
        order_type: order.order_type.clone(),
        quantity: order.quantity.storage_text(),
        status: order.status.text().to_owned(),
        filled_quantity: order.filled.storage_text(),
        filled_price: order.average_price.storage_text(),
        submitted_at: order.submitted_at.clone(),
        filled_at: order.filled_at.clone(),
        reduce_only: order.reduce_only,
    }
}
