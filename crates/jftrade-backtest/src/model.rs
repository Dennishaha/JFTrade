use jftrade_kernel::{Fixed8, WireTimestamp};
use serde::{Deserialize, Serialize};

pub const CORPUS_VERSION: u32 = 1;
pub const EXECUTION_MODEL: &str = "conservative-bar-v1";

#[derive(Clone, Debug, Deserialize)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
pub struct CorpusInput {
    pub version: u32,
    pub cases: Vec<BacktestCase>,
}

#[derive(Clone, Debug, Deserialize)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
pub struct BacktestCase {
    pub id: String,
    pub symbol: String,
    pub base_currency: String,
    pub quote_currency: String,
    pub initial_balance: Fixed8,
    #[serde(default)]
    pub process_orders_on_close: bool,
    #[serde(default)]
    pub slippage_ticks: u32,
    pub market: MarketRules,
    #[serde(default)]
    pub fee_rules: Vec<FeeRule>,
    #[serde(default)]
    pub indicator_periods: Vec<usize>,
    #[serde(default)]
    pub cancel_before_bar: Option<usize>,
    pub candles: Vec<Candle>,
    #[serde(default)]
    pub intents: Vec<OrderIntent>,
}

#[derive(Clone, Debug, Deserialize)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
pub struct MarketRules {
    pub tick_size: Fixed8,
    pub quantity_step: Fixed8,
    pub min_quantity: Fixed8,
}

#[derive(Clone, Debug, Deserialize)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
pub struct Candle {
    pub start: WireTimestamp,
    pub end: WireTimestamp,
    pub open: Fixed8,
    pub high: Fixed8,
    pub low: Fixed8,
    pub close: Fixed8,
    pub volume: Fixed8,
}

#[derive(Clone, Debug, Deserialize)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
pub struct OrderIntent {
    pub bar_index: usize,
    pub action: String,
    pub id: String,
    #[serde(default)]
    pub target_id: String,
    #[serde(default)]
    pub side: String,
    #[serde(default)]
    pub order_type: String,
    #[serde(default)]
    pub quantity: Fixed8,
    #[serde(default)]
    pub limit_price: Fixed8,
    #[serde(default)]
    pub stop_price: Fixed8,
    #[serde(default)]
    pub reduce_only: bool,
    #[serde(default)]
    pub parent_id: String,
    #[serde(default)]
    pub oco_group_id: String,
    #[serde(default)]
    pub atomic_group_id: String,
}

#[derive(Clone, Debug, Deserialize)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
pub struct FeeRule {
    pub id: String,
    pub label: String,
    pub group: String,
    #[serde(default)]
    pub side: String,
    pub basis: String,
    #[serde(default)]
    pub rate: Fixed8,
    #[serde(default)]
    pub fixed_amount: Fixed8,
    #[serde(default)]
    pub min_amount: Fixed8,
    #[serde(default)]
    pub max_amount: Fixed8,
    #[serde(default)]
    pub max_rate: Fixed8,
    #[serde(default)]
    pub rounding: String,
}

#[derive(Clone, Debug, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct CorpusOutput {
    pub version: u32,
    pub execution_model: &'static str,
    pub cases: Vec<BacktestOutput>,
}

#[derive(Clone, Debug, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct BacktestOutput {
    pub id: String,
    pub status: RunStatus,
    pub processed_bars: usize,
    pub cash: String,
    pub base_position: String,
    pub final_equity: String,
    pub realized_pnl: String,
    pub total_broker_fees: String,
    pub total_market_fees: String,
    pub total_fees: String,
    pub total_fills: usize,
    pub total_trades: usize,
    pub winning_trades: usize,
    pub win_rate: String,
    pub max_drawdown: String,
    pub current_drawdown: String,
    pub orders: Vec<OrderOutput>,
    pub fills: Vec<FillOutput>,
    pub equity_curve: Vec<EquityPoint>,
    pub drawdown_curve: Vec<DrawdownPoint>,
    pub fee_breakdown: Vec<FeeBreakdown>,
    pub indicators: Vec<IndicatorOutput>,
    pub warnings: Vec<String>,
    pub result_hash: String,
}

#[derive(Clone, Copy, Debug, Eq, PartialEq, Serialize)]
#[serde(rename_all = "lowercase")]
pub enum RunStatus {
    Completed,
    Cancelled,
}

#[derive(Clone, Debug, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct OrderOutput {
    pub order_id: String,
    pub client_order_id: String,
    pub side: String,
    pub order_type: String,
    pub quantity: String,
    pub status: String,
    pub filled_quantity: String,
    pub filled_price: String,
    pub submitted_at: String,
    pub filled_at: String,
    pub reduce_only: bool,
}

#[derive(Clone, Debug, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct FillOutput {
    pub trade_id: String,
    pub order_id: String,
    pub client_order_id: String,
    pub side: String,
    pub price: String,
    pub quantity: String,
    pub quote_quantity: String,
    pub time: String,
    pub maker: bool,
    pub broker_fee: String,
    pub market_fee: String,
    pub total_fee: String,
    pub realized_pnl: String,
}

#[derive(Clone, Debug, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct EquityPoint {
    pub time: String,
    pub equity: String,
}

#[derive(Clone, Debug, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct DrawdownPoint {
    pub time: String,
    pub drawdown: String,
}

#[derive(Clone, Debug, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct FeeBreakdown {
    pub rule_id: String,
    pub label: String,
    pub group: String,
    pub amount: String,
    pub count: usize,
}

#[derive(Clone, Debug, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct IndicatorOutput {
    pub kind: &'static str,
    pub period: usize,
    pub values: Vec<Option<String>>,
}
