//! US session-aware historical K-line request planning.
//!
//! OpenD's `Qot_RequestHistoryKL` only supports session routing on some
//! builds: the caller fans out over `RTH`/`ETH`/`ALL`, merges the returned
//! candles by bucket start, and falls back to a single `Session_ALL` request
//! when the server rejects a routed call.
//!
//! Parity: `pkg/futu/exchange_kline_session.go`.

/// OpenD `Common.Session` wire values used by the planner.
pub const SESSION_RTH: i32 = 1;
pub const SESSION_ETH: i32 = 2;
pub const SESSION_ALL: i32 = 3;
pub const SESSION_OVERNIGHT: i32 = 4;

/// Market session labels shared with the engine-side candle projection.
#[derive(Clone, Copy, Debug, Eq, PartialEq, Ord, PartialOrd)]
pub enum MarketSession {
    Regular,
    Pre,
    After,
    Overnight,
}

impl MarketSession {
    pub const fn as_str(self) -> &'static str {
        match self {
            Self::Regular => "regular",
            Self::Pre => "pre",
            Self::After => "after",
            Self::Overnight => "overnight",
        }
    }
}

/// One routed history request: the OpenD session filter plus the market
/// sessions whose candles must be retained after resolution.
#[derive(Clone, Debug, Eq, PartialEq)]
pub struct HistoricalKlineRequestPlan {
    pub extended_time: bool,
    pub session: Option<i32>,
    pub keep_sessions: Vec<MarketSession>,
}

impl HistoricalKlineRequestPlan {
    /// The untagged plan used for daily bars and non-US markets.
    pub fn unsegmented() -> Self {
        Self {
            extended_time: false,
            session: None,
            keep_sessions: Vec::new(),
        }
    }

    /// `Session_ALL` fallback used when OpenD rejects session routing.
    pub fn all(keep_sessions: Vec<MarketSession>) -> Self {
        Self {
            extended_time: true,
            session: Some(SESSION_ALL),
            keep_sessions,
        }
    }

    /// Resolves the market session for a routed plan.
    ///
    /// `clock_session` is the exchange-calendar classification produced by the
    /// caller; routed plans override it where OpenD already knows the answer.
    pub fn resolve_market_session(&self, clock_session: MarketSession) -> MarketSession {
        match self.session {
            Some(SESSION_RTH) => MarketSession::Regular,
            Some(SESSION_OVERNIGHT) => MarketSession::Overnight,
            _ => clock_session,
        }
    }

    /// ETH keeps the pre/after classification; overnight bars returned by an
    /// ETH route belong to the overnight carry only when the calendar says so.
    pub fn should_keep_market_session(&self, session: MarketSession) -> bool {
        self.keep_sessions.is_empty() || self.keep_sessions.contains(&session)
    }
}

/// True when the symbol/interval combination needs US extended-session
/// routing: US symbols at 1 hour or below.
pub fn should_split_historical_kline_requests(symbol: &str, period_seconds: i64) -> bool {
    is_us_symbol(symbol) && period_seconds > 0 && period_seconds <= 3600
}

fn is_us_symbol(symbol: &str) -> bool {
    let trimmed = symbol.trim();
    trimmed.len() >= 3
        && trimmed[..2].eq_ignore_ascii_case("US")
        && trimmed.as_bytes().get(2) == Some(&b'.')
}

/// Builds the routed request plans for the requested market sessions.
///
/// `None` means "every session" (regular + extended + overnight), matching
/// Go's `buildHistoricalKLineRequestPlansForSessions`.
pub fn build_request_plans(
    symbol: &str,
    period_seconds: i64,
    sessions: Option<&[MarketSession]>,
) -> Vec<HistoricalKlineRequestPlan> {
    if !should_split_historical_kline_requests(symbol, period_seconds) {
        return vec![HistoricalKlineRequestPlan::unsegmented()];
    }
    let default = [
        MarketSession::Regular,
        MarketSession::Pre,
        MarketSession::After,
        MarketSession::Overnight,
    ];
    let sessions = sessions.unwrap_or(&default);
    let contains = |session: MarketSession| sessions.contains(&session);
    let mut plans = Vec::with_capacity(3);
    if contains(MarketSession::Regular) {
        plans.push(HistoricalKlineRequestPlan {
            extended_time: true,
            session: Some(SESSION_RTH),
            keep_sessions: vec![MarketSession::Regular],
        });
    }
    if contains(MarketSession::Pre) || contains(MarketSession::After) {
        let keep = [MarketSession::Pre, MarketSession::After]
            .into_iter()
            .filter(|session| contains(*session))
            .collect();
        plans.push(HistoricalKlineRequestPlan {
            extended_time: true,
            session: Some(SESSION_ETH),
            keep_sessions: keep,
        });
    }
    if contains(MarketSession::Overnight) {
        plans.push(HistoricalKlineRequestPlan {
            extended_time: true,
            session: Some(SESSION_ALL),
            keep_sessions: vec![MarketSession::Overnight],
        });
    }
    plans
}

/// True when a rejected routed request should be retried as `Session_ALL`.
///
/// OpenD answers with messages such as
/// `获取历史K线的时段仅支持设置 RTH，ETH，ALL`; only session-marker messages that
/// also say the route is unsupported/invalid trigger the fallback.
pub fn should_fallback_to_all(
    plan: &HistoricalKlineRequestPlan,
    response: &HistoricalKlineRouteError,
) -> bool {
    let (Some(plan_session), Some(response_session)) = (plan.session, response.session) else {
        return false;
    };
    if plan_session != response_session {
        return false;
    }
    let message = response.ret_msg.trim().to_ascii_uppercase();
    if message.is_empty() {
        return false;
    }
    let has_marker = message.contains("OVERNIGHT")
        || message.contains("SESSION")
        || message.contains("时段")
        || (message.contains("RTH") && message.contains("ETH") && message.contains("ALL"));
    if !has_marker {
        return false;
    }
    message.contains("NOT SUPPORT")
        || message.contains("UNSUPPORTED")
        || message.contains("INVALID")
        || message.contains("ONLY SUPPORT")
        || message.contains("SUPPORT ONLY")
        || message.contains("不支持")
        || message.contains("无效")
        || message.contains("仅支持")
}

/// OpenD rejection of a routed history request.
#[derive(Clone, Debug, Eq, PartialEq)]
pub struct HistoricalKlineRouteError {
    pub session: Option<i32>,
    pub ret_type: i32,
    pub err_code: i32,
    pub ret_msg: String,
}

#[cfg(test)]
mod tests {
    use super::*;

    /// Parity: pkg/futu/exchange_kline_test.go:131
    /// TestHistoricalKLineSessionHelpersFilterAndPlanExplicitSelections
    #[test]
    fn session_planner_selects_explicit_routes_and_keep_sets() {
        let plans = build_request_plans("US.AAPL", 60, Some(&[MarketSession::Pre]));
        assert_eq!(plans.len(), 1);
        assert_eq!(plans[0].session, Some(SESSION_ETH));
        assert_eq!(plans[0].keep_sessions, vec![MarketSession::Pre]);
        let all = HistoricalKlineRequestPlan::all(vec![MarketSession::Overnight]);
        assert_eq!(all.keep_sessions, vec![MarketSession::Overnight]);
    }

    /// Parity: pkg/futu/exchange_kline_test.go:106
    /// TestQueryKLinesForSessionsFiltersUSHistoricalRoutes
    #[test]
    fn regular_only_selection_produces_a_single_rth_route() {
        let plans = build_request_plans("US.NVDA", 60, Some(&[MarketSession::Regular]));
        assert_eq!(plans.len(), 1);
        assert_eq!(plans[0].session, Some(SESSION_RTH));
        assert!(plans[0].extended_time);
        assert_eq!(plans[0].keep_sessions, vec![MarketSession::Regular]);
    }

    /// Parity: pkg/futu/exchange_kline_test.go:50
    /// TestQueryKLinesSplitsUSHistoricalRequestsBySessionAndMergesResults
    #[test]
    fn default_selection_produces_rth_eth_and_all_routes() {
        let plans = build_request_plans("US.NVDA", 60, None);
        let routes = plans.iter().map(|plan| plan.session).collect::<Vec<_>>();
        assert_eq!(
            routes,
            vec![Some(SESSION_RTH), Some(SESSION_ETH), Some(SESSION_ALL)]
        );
        assert_eq!(
            plans[1].keep_sessions,
            vec![MarketSession::Pre, MarketSession::After]
        );
        assert_eq!(plans[2].keep_sessions, vec![MarketSession::Overnight]);
    }

    /// Parity: pkg/futu/exchange_kline_test.go:17
    /// TestQueryTickersBatchesBasicQotRequests (routing precondition)
    #[test]
    fn non_us_and_daily_requests_stay_unsegmented() {
        assert_eq!(
            build_request_plans("HK.00700", 60, None),
            vec![HistoricalKlineRequestPlan::unsegmented()]
        );
        assert_eq!(
            build_request_plans("US.NVDA", 0, None),
            vec![HistoricalKlineRequestPlan::unsegmented()]
        );
        assert_eq!(
            build_request_plans("US.NVDA", 86_400, None),
            vec![HistoricalKlineRequestPlan::unsegmented()]
        );
    }

    /// Parity: pkg/futu/exchange_kline_test.go:178
    /// TestResolveHistoricalRequestSessionUsesRouteForRTHAndOvernight
    #[test]
    fn routed_sessions_override_the_clock_classification() {
        let rth = HistoricalKlineRequestPlan {
            extended_time: true,
            session: Some(SESSION_RTH),
            keep_sessions: vec![MarketSession::Regular],
        };
        assert_eq!(
            rth.resolve_market_session(MarketSession::Pre),
            MarketSession::Regular
        );
        let overnight = HistoricalKlineRequestPlan {
            extended_time: true,
            session: Some(SESSION_OVERNIGHT),
            keep_sessions: vec![MarketSession::Overnight],
        };
        assert_eq!(
            overnight.resolve_market_session(MarketSession::Regular),
            MarketSession::Overnight
        );
        assert!(!rth.should_keep_market_session(MarketSession::Pre));
        assert!(rth.should_keep_market_session(MarketSession::Regular));
    }

    /// Parity: pkg/futu/exchange_kline_test.go:228
    /// TestShouldFallbackHistoricalKLineSplitRecognizesChineseSupportedSessionsMessage
    #[test]
    fn chinese_supported_session_message_triggers_the_all_fallback() {
        let plan = HistoricalKlineRequestPlan::all(Vec::new());
        let error = HistoricalKlineRouteError {
            session: plan.session,
            ret_type: 1,
            err_code: 0,
            ret_msg: "获取历史K线的时段仅支持设置 RTH，ETH，ALL".to_owned(),
        };
        assert!(should_fallback_to_all(&plan, &error));
    }

    /// Parity: pkg/futu/exchange_kline_test.go:192
    /// TestQueryKLinesFallsBackToSessionAllWhenHistoricalRouteUnsupported
    #[test]
    fn fallback_requires_the_same_route_and_an_unsupported_marker() {
        let plan = HistoricalKlineRequestPlan {
            extended_time: true,
            session: Some(SESSION_ETH),
            keep_sessions: vec![MarketSession::Pre],
        };
        let same_route = HistoricalKlineRouteError {
            session: Some(SESSION_ETH),
            ret_type: 1,
            err_code: 0,
            ret_msg: "session is invalid".to_owned(),
        };
        assert!(should_fallback_to_all(&plan, &same_route));
        let other_route = HistoricalKlineRouteError {
            session: Some(SESSION_ALL),
            ..same_route.clone()
        };
        assert!(!should_fallback_to_all(&plan, &other_route));
        let unrelated = HistoricalKlineRouteError {
            session: Some(SESSION_ETH),
            ret_type: 1,
            err_code: 0,
            ret_msg: "subscription quota exceeded".to_owned(),
        };
        assert!(!should_fallback_to_all(&plan, &unrelated));
        let unsegmented = HistoricalKlineRequestPlan::unsegmented();
        assert!(!should_fallback_to_all(&unsegmented, &same_route));
    }
}
