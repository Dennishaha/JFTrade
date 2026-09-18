#![forbid(unsafe_code)]

//! Futu OpenD protocol and subscription adapter boundaries.

mod basic_quote_query;
mod basic_quote_tick;
mod corporate_actions_query;
mod customization;
mod earnings_calendar_query;
mod frame;
mod future_info_query;
mod health;
mod history;
mod history_session_plan;
mod instrument_search_query;
pub mod kline_query;
mod managed_session;
mod market_microstructure_query;
mod market_rules_query;
mod news_query;
pub mod notification;
mod open_d_quote_rights;
mod option_chain_query;
mod option_contract_rank_query;
mod option_earnings_screener_query;
mod option_event_query;
mod option_exercise_probability_query;
mod option_expiration_query;
mod option_market_statistic_query;
mod option_quote_query;
mod option_screen_query;
mod option_seller_screener_query;
mod option_strategy_analysis_query;
mod option_strategy_query;
mod option_strategy_spread_query;
mod option_underlying_his_statistic_query;
mod option_underlying_his_volatility_query;
mod option_underlying_overview_query;
mod option_underlying_rank_query;
mod option_volatility_query;
mod option_zero_dte_contract_query;
mod option_zero_dte_screener_query;
mod order_book_wire;
mod prediction;
mod prediction_push;
mod probe;
mod provider;
mod provider_runtime;
mod quote_push;
mod quote_rights;
mod recoverable_error;
mod research_institutions_query;
mod research_normalization;
mod research_params;
mod research_short_interest_query;
mod runtime_task;
mod security_snapshot_coordinator;
mod security_snapshot_query;
mod session_coordinator;
mod session_event_pump;
mod session_resolver;
mod snapshot_fallback;
mod stock_screen_factors;
mod stock_screen_query;
mod subscription_executor;
mod subscriptions;
mod technical_indicator_query;
mod ticker_query;
mod trade_price;
mod valuation_detail_query;
mod watchlist_reader;
// The generated module is crate-internal; generated messages must not leak to
// engine consumers.  Generated code is intentionally exempt from local lint
// rules because its field/enum names are dictated by the OpenD schema.
#[allow(dead_code, clippy::all)]
pub mod trade_proto;
mod trade_proto_fee_validation;
mod trade_proto_fill_validation;
mod trade_proto_margin_ratio_validation;
mod trade_proto_max_qty_validation;
mod trade_proto_order_validation;
mod trade_proto_validation;
mod trade_session;
mod trade_snapshots;
mod trading;
mod transport;

pub use basic_quote_query::{BasicQuoteQueryError, OpenDBasicQuoteExecutor};
pub use basic_quote_tick::{
    BasicQuoteTickError, basic_quote_ticks, basic_quote_ticks_with_resolver, quote_session_label,
};
pub use corporate_actions_query::{
    CorporateActionKind, FutuCorporateAction, FutuCorporateActionsQuery,
    FutuCorporateActionsQueryError, FutuCorporateActionsReadPort, FutuCorporateActionsResult,
    OpenDCorporateActionsReader,
};
pub use customization::{
    AlertCustomizationReadPort, AlertCustomizationWritePort, FutuAlertQuery, FutuAlertWrite,
    FutuRemoteWatchlistReader, RemoteWatchlistReadPort, RemoteWatchlistWritePort,
};
pub use earnings_calendar_query::{
    EARNINGS_CALENDAR_PROTOCOL_ID, EarningsCalendarBoundary, EarningsCalendarEstimate,
    EarningsCalendarFilter, EarningsCalendarInterval, EarningsCalendarItem, EarningsCalendarPage,
    EarningsCalendarQuery, EarningsCalendarQueryError, EarningsCalendarReadPort,
    EarningsCalendarSecurity, OpenDEarningsCalendarReader, earnings_calendar_market_value,
};
pub use frame::{Frame, FrameError, Header, decode_frame, encode_frame};
pub use future_info_query::{
    FutureInfo, FutureInfoQuery, FutureInfoQueryError, FutureInfoReadPort, FutureInfoSecurity,
    FutureInfoSecurityQuery, FutureTradeTime, OpenDFutureInfoReader,
};
pub use health::{
    OpenDInitializedSession, OpenDTcpProbe, OpenDTcpProbeConfig, OpenDTcpProbeError,
    market_data_health_from_probe,
};
pub use history::{
    HistoricalKline, HistoricalKlineError, HistoricalKlineQuery, HistoricalKlineReadPort,
    HistoricalKlineResult, HistoricalSecurity, OpenDHistoricalKlineReader,
    resolve_historical_kline_page_size,
};
pub use history_session_plan::{
    HistoricalKlineRequestPlan, HistoricalKlineRouteError, MarketSession, SESSION_ALL, SESSION_ETH,
    SESSION_OVERNIGHT, SESSION_RTH, build_request_plans, should_fallback_to_all,
    should_split_historical_kline_requests,
};
pub use instrument_search_query::{
    InstrumentSearchEntry, InstrumentSearchError, InstrumentSearchReadPort, MAX_SEARCH_QUOTE_COUNT,
    OpenDInstrumentSearchReader,
};
pub use kline_query::{
    CurrentKlineError, CurrentKlineQuery, CurrentKlineReadPort, CurrentKlineResult, GET_KL_TIMEOUT,
    adjust_kline_time, merge_klines_by_time, period_duration_seconds, period_to_kl_type,
    period_to_kl_type_checked, query_current_klines, should_query_current_kline,
};
pub use managed_session::{
    OpenDManagedSession, OpenDManagedSessionError, OpenDSessionCloseReason, OpenDSessionEvent,
};
pub use market_microstructure_query::{
    MarketMicrostructureError, MarketMicrostructureOperation, MarketMicrostructureReadPort,
    OpenDMarketMicrostructureReader,
};
pub use market_rules_query::{
    MarketRuleSnapshot, MarketRulesQueryError, MarketRulesReadPort, OpenDMarketRulesReader,
    OpenDSecurityInfoReader, SecurityInfoItem, SecurityInfoReadPort,
};
pub use news_query::{
    FutuNewsEntry, FutuNewsQuery, FutuNewsQueryError, FutuNewsReadPort, FutuNewsResult,
    OpenDNewsReader,
};
pub use open_d_quote_rights::{
    OpenDQuoteRightsError, OpenDQuoteRightsOwner, decode_capability_notification,
    query_quote_rights,
};
pub use option_chain_query::{
    OpenDOptionChainReader, OptionChainDataFilter, OptionChainDate, OptionChainItem,
    OptionChainQuery, OptionChainQueryError, OptionChainReadPort, OptionContract,
    OptionContractBasic, OptionContractExData, OptionSecurity,
};
pub use option_contract_rank_query::{
    OpenDOptionContractRankReader, OptionContractRankItem, OptionContractRankQuery,
    OptionContractRankQueryError, OptionContractRankReadPort, OptionContractRankSecurity,
    OptionContractRankSnapshot,
};
pub use option_earnings_screener_query::{
    OpenDOptionEarningsScreenerReader, OptionEarningsScreenerItem, OptionEarningsScreenerPage,
    OptionEarningsScreenerQuery, OptionEarningsScreenerQueryError, OptionEarningsScreenerReadPort,
};
pub use option_event_query::{
    EventIndicator, EventIndicatorValue, EventSort, OpenDOptionEventReader, OptionEvent,
    OptionEventCorporateAction, OptionEventPage, OptionEventQuery, OptionEventQueryError,
    OptionEventReadPort, OptionEventSecurity,
};
pub use option_exercise_probability_query::{
    OpenDOptionExerciseProbabilityReader, OptionExerciseProbabilityItem,
    OptionExerciseProbabilityQuery, OptionExerciseProbabilityQueryError,
    OptionExerciseProbabilityReadPort, OptionExerciseProbabilitySecurity,
    OptionExerciseProbabilitySnapshot,
};
pub use option_expiration_query::{
    OpenDOptionExpirationReader, OptionExpirationDate, OptionExpirationQuery,
    OptionExpirationQueryError, OptionExpirationReadPort,
};
pub use option_market_statistic_query::{
    OpenDOptionMarketStatisticReader, OptionMarketStatisticItem, OptionMarketStatisticQuery,
    OptionMarketStatisticQueryError, OptionMarketStatisticReadPort, OptionMarketStatisticSnapshot,
    decode_next_page_key as decode_option_market_statistic_cursor,
    encode_next_page_key as encode_option_market_statistic_cursor,
};
pub use option_quote_query::{
    OpenDOptionQuoteReader, OptionQuote, OptionQuoteQuery, OptionQuoteQueryError,
    OptionQuoteReadPort, OptionQuoteSecurity,
};
pub use option_screen_query::{
    OpenDOptionScreenReader, OptionScreenItem, OptionScreenPage, OptionScreenQuery,
    OptionScreenQueryError, OptionScreenReadPort, OptionScreenSecurity,
};
pub use option_seller_screener_query::{
    OpenDOptionSellerScreenerReader, OptionSellerScreenerItem, OptionSellerScreenerQuery,
    OptionSellerScreenerQueryError, OptionSellerScreenerReadPort,
};
pub use option_strategy_analysis_query::{
    OpenDOptionStrategyAnalysisReader, OptionStrategyAnalysisQuery,
    OptionStrategyAnalysisQueryError, OptionStrategyAnalysisReadPort,
    OptionStrategyAnalysisSnapshot,
};
pub use option_strategy_query::{
    OpenDOptionStrategyReader, OptionStrategyItem, OptionStrategyLeg, OptionStrategyQuery,
    OptionStrategyQueryError, OptionStrategyReadPort, OptionStrategySecurity,
    OptionStrategySnapshot,
};
pub use option_strategy_spread_query::{
    OpenDOptionStrategySpreadReader, OptionStrategySpreadItem, OptionStrategySpreadQuery,
    OptionStrategySpreadQueryError, OptionStrategySpreadReadPort, OptionStrategySpreadSnapshot,
};
pub use option_underlying_his_statistic_query::{
    OpenDOptionUnderlyingHisStatisticReader, OptionUnderlyingHisStatisticItem,
    OptionUnderlyingHisStatisticQuery, OptionUnderlyingHisStatisticQueryError,
    OptionUnderlyingHisStatisticReadPort, OptionUnderlyingHisStatisticSecurity,
    OptionUnderlyingHisStatisticSnapshot,
    decode_next_page_key as decode_option_underlying_his_statistic_cursor,
    encode_next_page_key as encode_option_underlying_his_statistic_cursor,
};
pub use option_underlying_his_volatility_query::{
    OpenDOptionUnderlyingHisVolatilityReader, OptionUnderlyingHisVolatilityItem,
    OptionUnderlyingHisVolatilityQuery, OptionUnderlyingHisVolatilityQueryError,
    OptionUnderlyingHisVolatilityReadPort, OptionUnderlyingHisVolatilitySecurity,
    OptionUnderlyingHisVolatilitySnapshot, decode_next_page_key, encode_next_page_key,
};
pub use option_underlying_overview_query::{
    OpenDOptionUnderlyingOverviewReader, OptionUnderlyingHvItem, OptionUnderlyingOverviewItem,
    OptionUnderlyingOverviewQuery, OptionUnderlyingOverviewQueryError,
    OptionUnderlyingOverviewReadPort, OptionUnderlyingOverviewSecurity,
    OptionUnderlyingOverviewSnapshot,
};
pub use option_underlying_rank_query::{
    OpenDOptionUnderlyingRankReader, OptionUnderlyingRankItem, OptionUnderlyingRankQuery,
    OptionUnderlyingRankQueryError, OptionUnderlyingRankReadPort, OptionUnderlyingRankSecurity,
    OptionUnderlyingRankSnapshot,
};
pub use option_volatility_query::{
    OpenDOptionVolatilityReader, OptionVolatilityItem, OptionVolatilityQuery,
    OptionVolatilityQueryError, OptionVolatilityReadPort, OptionVolatilitySecurity,
    OptionVolatilitySnapshot,
};
pub use option_zero_dte_contract_query::{
    OpenDOptionZeroDteContractReader, OptionZeroDteContractItem, OptionZeroDteContractQuery,
    OptionZeroDteContractQueryError, OptionZeroDteContractReadPort,
};
pub use option_zero_dte_screener_query::{
    OpenDOptionZeroDteScreenerReader, OptionZeroDteChainInfo, OptionZeroDteScreenerItem,
    OptionZeroDteScreenerPage, OptionZeroDteScreenerQuery, OptionZeroDteScreenerQueryError,
    OptionZeroDteScreenerReadPort,
};
pub use prediction::{
    OpenDPredictionMarketReader, PredictionComboQuotePort, PredictionMarketReadError,
    PredictionMarketReadPort, PredictionMarketSubscriptionPort, prediction_subscription_body,
};
pub use prediction_push::{
    PredictionDataType, PredictionPushListener, PredictionPushRegistry, PredictionPushRow,
    PredictionPushUnsubscribe, decode_prediction_push, entry_instrument_id, entry_sequence,
};
pub use probe::{MarketState, OpenDProbe, WireGlobalState};
pub use provider::{broker_descriptor, provider_descriptor};
pub use provider_runtime::{
    OpenDProviderRuntime, OpenDProviderRuntimeConfig, OpenDProviderRuntimeError,
};
pub use quote_push::{
    BasicQuote, BasicQuotePush, Kline, KlinePush, OrderBookDetail, OrderBookLevel, OrderBookPush,
    PreAfterMarketData, QuotePush, QuotePushDecodeError, Security, decode_quote_push,
};
pub use quote_rights::{
    ConnectStatusSnapshot, QUOTE_RIGHTS_FAILURE_RETRY_INTERVAL, QuoteRightField,
    QuoteRightSnapshot, QuoteRightState, QuoteRightsError, QuoteRightsFetchOutcome,
    QuoteRightsState,
};
pub use recoverable_error::{
    OpenDRecoverableKind, classify_recoverable, classify_recoverable_io, is_recoverable_error,
};
pub use research_institutions_query::{
    FutuInstitutionEntry, FutuInstitutionOperation, FutuInstitutionQuery,
    FutuInstitutionQueryError, FutuInstitutionReadPort, FutuInstitutionResult,
    FutuInstitutionSecurity, FutuInstitutionSecurityQuery, FutuInstitutionSummary,
    OpenDInstitutionReader,
};
pub use research_normalization::{
    LocalResearchPage, ResearchPayloadEnvelope, apply_research_local_pagination,
    flatten_research_ipo, is_research_normalization_protocol, normalize_research_calendar_fields,
    normalize_research_institution_fields, normalize_research_protocol_payload,
    research_payload_envelope, research_product_class, research_security_type,
};
pub use research_params::{
    ResearchParamsError, ResearchQueryScope, advanced_cursor_field, advanced_has_field,
    advanced_has_market_field, advanced_page_size_field, advanced_page_size_limit,
    advanced_protocol_replay_safe, bounded_research_enum, clamp_advanced_page_size,
    inject_advanced_cursor, inject_advanced_defaults, inject_advanced_option_defaults,
    inject_advanced_page_size, inject_advanced_protocol_defaults,
    inject_advanced_research_defaults, research_number_parity, translate_heat_map_plate_type,
    translate_plate_set_type, translate_top_movers_direction,
};
pub use research_short_interest_query::{
    FutuShortInterestItem, FutuShortInterestQuery, FutuShortInterestQueryError,
    FutuShortInterestReadPort, FutuShortInterestResult, FutuShortInterestSecurity,
    OpenDShortInterestReader, ShortInterestItem, ShortInterestOperation, ShortInterestQuery,
    ShortInterestQueryError, ShortInterestReadPort, ShortInterestResult, ShortInterestSecurity,
};
pub use runtime_task::{
    OpenDSessionEventListener, OpenDSessionRuntime, OpenDSessionRuntimeConfig,
    OpenDSessionRuntimeError, OpenDSessionRuntimeStatus,
};
pub use security_snapshot_coordinator::{
    CachedSecuritySnapshotReader, OpenDSecuritySnapshotBatchReader, SECURITY_SNAPSHOT_CACHE_TTL,
    SECURITY_SNAPSHOT_CALL_LIMIT, SECURITY_SNAPSHOT_CALL_WINDOW, SECURITY_SNAPSHOT_HK_BATCH_SIZE,
    SECURITY_SNAPSHOT_OTHER_BATCH_SIZE, SecuritySnapshotBatchReader, SecuritySnapshotCancelToken,
    SecuritySnapshotClock, SecuritySnapshotCoordinator, SecuritySnapshotCoordinatorError,
    canonical_snapshot_symbols, classify_security_snapshot_fetch_error, snapshot_batches,
};
pub use security_snapshot_query::{
    OpenDSecuritySnapshotReader, SecuritySnapshotQueryError, SecuritySnapshotReadPort,
};
pub use session_coordinator::{
    OpenDSessionCoordinator, OpenDSessionCoordinatorError, OpenDSessionCoordinatorOutcome,
};
pub use session_event_pump::{
    OpenDCapabilityNotificationSink, OpenDSessionEventPump, OpenDSessionPumpError,
    OpenDSessionPumpOutcome,
};
pub use session_resolver::{
    QuoteSessionContext, QuoteSessionResolver, QuoteSessionWindow, fallback_snapshot_session,
    market_sessions_for_candle_sessions,
};
pub use stock_screen_query::{
    OpenDStockScreenReader, STOCK_SCREEN_LIMIT, STOCK_SCREEN_PROTOCOL_ID, STOCK_SCREEN_WINDOW,
    StockScreenItem, StockScreenLimiter, StockScreenPage, StockScreenParam, StockScreenProperty,
    StockScreenPropertyParams, StockScreenQuery, StockScreenQueryError, StockScreenReadPort,
    StockScreenResult, StockScreenSecurity, StockScreenValue,
};

pub use snapshot_fallback::{
    DelayedSnapshotItem, OpenDSnapshotFallbackReader, STOCK_SCREEN_SNAPSHOT_CACHE_TTL,
    STOCK_SCREEN_SNAPSHOT_PAGE_SIZE, STOCK_SCREEN_SNAPSHOT_SOURCE, ScreenRow,
    SnapshotFallbackError, SnapshotFallbackFetchPort, StockIdentity,
    StockScreenSnapshotCoordinator, StockScreenSnapshotFallback, canonical_fallback_symbols,
    fetch_delayed_snapshots, project_screen_page, project_screen_page_at, screen_market_value,
    validate_snapshot_page,
};
pub use subscription_executor::{OpenDSubscriptionExecutor, SubscriptionExecutorError};
pub use subscriptions::{
    OpenDSubscriptionLifecycle, PhysicalSubscription, ReconcileAction, SubscriptionKind,
    SubscriptionPlan, SubscriptionReconciler, desired_subscriptions, retry_delay_ms,
};
pub use technical_indicator_query::{
    FutuIndicatorCalculation, FutuIndicatorInput, FutuIndicatorInputParameter, FutuIndicatorList,
    FutuIndicatorListQuery, FutuIndicatorOutputParameter, FutuIndicatorQueryError,
    FutuIndicatorReadPort, FutuTechnicalIndicatorReader, IndicatorCalcQuery, IndicatorKline,
    IndicatorListQuery, TechnicalIndicatorCalculation, TechnicalIndicatorInfo,
    TechnicalIndicatorList, TechnicalIndicatorQuery, TechnicalIndicatorReadPort,
    TechnicalIndicatorResult,
};
pub use ticker_query::{OpenDTickerQuoteReader, TickerQuoteError, TickerQuoteReadPort};
pub use trade_price::{
    count_step_decimals, normalize_submit_order_price, round_price_to_step, step_rounded_unit,
    submit_order_price_step,
};
pub use trade_proto::ResponseError;
pub use trade_session::{
    OpenDTradeReadClient, TradeModifyOrderRequest, TradePlaceComboOrderRequest,
    TradePlaceComboOrderResult, TradePlaceOrderRequest, TradePlaceOrderResult, TradeReadPort,
    TradeSessionError, TradeSubscribeAccountsRequest, TradeUnlockRequest, TradeWritePort,
    trade_header,
};
pub use trade_snapshots::{
    TradeAccountSnapshot, TradeCashFlowSnapshot, TradeCashInfo, TradeComboLeg,
    TradeComboMaxTradeQuantityRequest, TradeComboMaxTradeQuantitySnapshot, TradeFillSnapshot,
    TradeFilter, TradeFunds, TradeFundsSnapshot, TradeHeader, TradeMarginRatioSnapshot,
    TradeMarketInfo, TradeMaxTradeQuantityRequest, TradeMaxTradeQuantitySnapshot,
    TradeOrderFeeItemSnapshot, TradeOrderFeeSnapshot, TradeOrderSnapshot, TradePositionSnapshot,
    TradeSecurity,
};
pub use trading::{
    RawOrderUpdate, TradeProtocol, TradeProtocolError, TradeProtocolPlan, map_order_update,
    plan_shadow_protocol,
};
pub use transport::{
    OpenDClient, OpenDFrameReader, OpenDTcpTransport, OpenDTransport, TcpTransportError,
    TransportError,
};
pub use valuation_detail_query::{
    OpenDValuationDetailReader, ValuationDetailHistoricalItem, ValuationDetailMarketDistribution,
    ValuationDetailPlateDistribution, ValuationDetailPlateStockItem, ValuationDetailProfitGrowth,
    ValuationDetailProfitGrowthItem, ValuationDetailQuery, ValuationDetailQueryError,
    ValuationDetailReadPort, ValuationDetailSecurity, ValuationDetailSnapshot,
    ValuationDetailTrend,
};
pub use watchlist_reader::{
    CachedRemoteWatchlistReader, WATCHLIST_CACHE_TTL, WATCHLIST_READ_LIMIT, WATCHLIST_READ_WINDOW,
    WatchlistClock, WatchlistReadGate, convert_groups, convert_members, normalize_group_name,
};

pub const PROTO_INIT_CONNECT: u32 = 1001;
pub const PROTO_GET_GLOBAL_STATE: u32 = 1002;
pub const PROTO_NOTIFY: u32 = 1003;
pub const PROTO_KEEP_ALIVE: u32 = 1004;
/// OpenD user-info read. Declared so the protocol-id table can be asserted
/// against Go (`pkg/futu/opend/protocol_ids_test.go`); the product path reads
/// account/quote rights through the typed trade session instead.
pub const PROTO_GET_USER_INFO: u32 = 1005;
pub const PROTO_GET_USER_SECURITY_GROUP: u32 = 3222;
pub const PROTO_GET_USER_SECURITY: u32 = 3213;
pub const PROTO_QOT_SUB: u32 = 3001;
pub const PROTO_GET_SUB_INFO: u32 = 3003;
pub const PROTO_GET_BASIC_QOT: u32 = 3004;
pub const PROTO_GET_STATIC_INFO: u32 = 3202;
pub const PROTO_GET_PLATE_SET: u32 = 3204;
pub const PROTO_GET_PLATE_SECURITY: u32 = 3205;
pub const PROTO_GET_SEARCH_QUOTE: u32 = 3262;
pub const PROTO_GET_SECURITY_SNAPSHOT: u32 = 3203;
pub const PROTO_UPDATE_BASIC_QOT: u32 = 3005;
pub const PROTO_GET_KL: u32 = 3006;
pub const PROTO_UPDATE_KL: u32 = 3007;
pub const PROTO_GET_ORDER_BOOK: u32 = 3012;
pub const PROTO_UPDATE_ORDER_BOOK: u32 = 3013;
pub const PROTO_REQUEST_HISTORY_KL: u32 = 3103;
pub const MINIMUM_OPEND_VERSION: &str = "10.9.6908";
