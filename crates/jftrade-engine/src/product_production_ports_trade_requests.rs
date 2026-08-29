//! Trade request parsing and Futu projection helpers.

use std::time::{SystemTime, UNIX_EPOCH};
#[allow(unused_imports)]
use jftrade_integration_futu::{TradeFilter, TradeFunds, TradeHeader, TradeMaxTradeQuantityRequest, TradeReadPort, TradeSecurity, trade_header};
#[allow(unused_imports)]
use jftrade_settings::MarketDataProvider;
#[allow(unused_imports)]
use serde_json::{Value, json};
use time::format_description::{FormatItem, parse_borrowed};
use time::format_description::well_known::Rfc3339;
use time::OffsetDateTime;
use crate::product::product_query::QueryMap;
use super::trade_projection::*;

#[derive(Debug)]
pub(crate) struct TradeRequest {
    pub(crate) broker_id: String,
    pub(crate) resource: String,
    pub(crate) query: QueryMap,
}

impl TradeRequest {
    pub(crate) fn parse(path: &str, raw_query: &str) -> Result<Self, String> {
        Self::parse_with_prefix(path, raw_query, "/api/v1/brokers/")
    }

    pub(crate) fn parse_with_prefix(path: &str, raw_query: &str, prefix: &str) -> Result<Self, String> {
        let query = QueryMap::parse(raw_query).map_err(|_| "invalid query encoding".to_owned())?;
        let suffix = path.strip_prefix(prefix).unwrap_or(path);
        let (broker_id, resource) = suffix
            .split_once('/')
            .ok_or_else(|| "invalid broker path".to_owned())?;
        if broker_id.is_empty() || resource.is_empty() || resource.contains('/') {
            return Err("invalid broker path".to_owned());
        }
        Ok(Self { broker_id: broker_id.to_owned(), resource: resource.to_owned(), query })
    }

    #[allow(dead_code)]
    pub(crate) fn header(&self) -> Result<TradeHeader, String> {
        let account = self.account_id().ok_or_else(|| "accountId is required".to_owned())?;
        let acc_id = account.parse::<u64>().map_err(|_| "accountId must be a numeric Futu account id".to_owned())?;
        let env = match self.query.get_first("tradingEnvironment").unwrap_or("real").to_ascii_lowercase().as_str() {
            "sim" | "simulate" | "paper" => 0,
            "real" | "production" => 1,
            value => return Err(format!("invalid tradingEnvironment: {value}")),
        };
        let market_name = self.market_label();
        let market = market_code(&market_name)?;
        Ok(trade_header(env, acc_id, market))
    }

    pub(crate) fn resolve_account(&self, client: &dyn TradeReadPort) -> Result<ResolvedTradeRequest, String> {
        self.resolve_account_with_environment(client, None, None)
    }

    pub(crate) fn resolve_account_real_for_market(
        &self,
        client: &dyn TradeReadPort,
        market: i32,
    ) -> Result<ResolvedTradeRequest, String> {
        self.resolve_account_with_environment(client, Some(1), trade_account_market_label(market))
    }

    pub(crate) fn resolve_account_with_environment(
        &self,
        client: &dyn TradeReadPort,
        forced_environment: Option<i32>,
        forced_market: Option<&str>,
    ) -> Result<ResolvedTradeRequest, String> {
        let accounts = client
            .read_accounts(0, None, None)
            .map_err(|error| error.to_string())?;
        if accounts.is_empty() {
            return Err("no Futu trading accounts discovered".to_owned());
        }
        let requested_account = self.account_id().map(str::to_ascii_lowercase);
        let requested_environment = forced_environment.or(self.environment_code()?);
        let requested_market = forced_market
            .map(normalize_trade_account_market)
            .or_else(|| self.query.get_first("market").map(str::trim).filter(|market| !market.is_empty()).map(normalize_trade_account_market));
        let mut candidates = accounts.into_iter().filter(|account| {
            let account_id = account_identity(account);
            let account_matches = requested_account
                .as_deref()
                .is_none_or(|requested| account_id.as_deref().is_some_and(|id| id.eq_ignore_ascii_case(requested)));
            let environment_matches = requested_environment.is_none_or(|environment| account.trd_env == environment);
            let market_matches = requested_market.as_ref().is_none_or(|requested| {
                account
                    .trd_market_auth_list
                    .iter()
                    .any(|market| trade_market_authority(*market) == Some(requested.as_str()))
            });
            account_matches && environment_matches && market_matches
        }).collect::<Vec<_>>();
        if requested_environment.is_none() {
            let simulated = candidates.iter().filter(|account| account.trd_env == 0).count();
            if simulated > 0 {
                candidates.retain(|account| account.trd_env == 0);
            }
        }
        let selected = candidates.into_iter().next();
        let Some(account) = selected else {
            let account_detail = requested_account
                .as_deref()
                .map_or_else(|| "any account".to_owned(), |id| format!("account {id}"));
            return Err(format!(
                "no Futu trading account matched {account_detail} for tradingEnvironment={} market={}",
                environment_label_from_code(requested_environment.unwrap_or(0)),
                requested_market.as_deref().unwrap_or("HK")
            ));
        };
        let account_id = account_identity(&account)
            .ok_or_else(|| "selected Futu account has no usable account identity".to_owned())?;
        let acc_id = account.acc_id;
        let selected_market = requested_market.clone().or_else(|| {
            account
                .trd_market_auth_list
                .iter()
                .find_map(|market| trade_market_authority(*market).map(str::to_owned))
        }).unwrap_or_else(|| "HK".to_owned());
        let header_market = account
            .trd_market_auth_list
            .iter()
            .copied()
            .find(|market| trade_market_authority(*market) == Some(selected_market.as_str()))
            .unwrap_or(market_code(&self.market_label())?);
        Ok(ResolvedTradeRequest {
            account_id,
            environment: environment_label_from_code(account.trd_env).to_owned(),
            market: selected_market,
            header: trade_header(
                account.trd_env,
                acc_id,
                header_market,
            ),
        })
    }

    pub(crate) fn environment_code(&self) -> Result<Option<i32>, String> {
        let Some(raw) = self.query.get_first("tradingEnvironment") else {
            return Ok(None);
        };
        match self
            .query
            .get_first("tradingEnvironment")
            .unwrap_or(raw)
            .trim()
            .to_ascii_lowercase()
            .as_str()
        {
            "sim" | "simulate" | "paper" => Ok(Some(0)),
            "real" | "production" => Ok(Some(1)),
            value => Err(format!("invalid tradingEnvironment: {value}")),
        }
    }

    pub(crate) fn account_id(&self) -> Option<&str> {
        self.query.get_first("accountId").map(str::trim).filter(|value| !value.is_empty())
    }

    pub(crate) fn market_label(&self) -> String {
        self.query.get_first("market").map(str::trim).unwrap_or("HK").to_owned()
    }

    pub(crate) fn refresh_cache(&self) -> Option<bool> {
        self.query.get_first("refreshCache").and_then(|v| match v.to_ascii_lowercase().as_str() { "1" | "true" => Some(true), "0" | "false" => Some(false), _ => None })
    }

    pub(crate) fn clearing_date(&self) -> Option<String> {
        self.query
            .get_first("clearingDate")
            .map(str::trim)
            .filter(|value| !value.is_empty())
            .map(ToOwned::to_owned)
    }

    pub(crate) fn order_id_ex_list(&self) -> Result<Vec<String>, String> {
        let mut values = Vec::new();
        let mut seen = std::collections::HashSet::new();
        for key in ["orderIdEx", "orderIdExList"] {
            for raw in self.query.get_all(key).unwrap_or(&[]) {
                for part in raw.split(',') {
                    let value = part.trim();
                    if value.is_empty() {
                        continue;
                    }
                    let key = value.to_ascii_uppercase();
                    if seen.insert(key) {
                        values.push(value.to_owned());
                    }
                }
            }
        }
        if values.is_empty() {
            Err("query parameter orderIdEx is required".to_owned())
        } else {
            Ok(values)
        }
    }

    pub(crate) fn securities(&self) -> Result<Vec<TradeSecurity>, String> {
        let mut securities = Vec::new();
        let mut seen = std::collections::HashSet::new();
        let explicit_market = self.query.get_first("market").map(str::trim).filter(|v| !v.is_empty()).map(str::to_ascii_uppercase);
        for key in ["symbol", "symbols"] {
            let default_market = self.market_label();
            for raw in self.query.get_all(key).unwrap_or(&[]) {
                for part in raw.split(',') {
                    let value = part.trim();
                    if value.is_empty() {
                        continue;
                    }
                    let (market, code) = value.split_once('.').unwrap_or((default_market.as_str(), value));
                    let market = market.trim().to_ascii_uppercase();
                    let code = code.trim().to_ascii_uppercase();
                    if code.is_empty() {
                        continue;
                    }
                    if let Some(expected) = explicit_market.as_deref()
                        && value.contains('.')
                        && market != expected
                        && !(expected == "CN" && matches!(market.as_str(), "SH" | "SZ"))
                    {
                        return Err(format!("query parameter symbol {value:?} is invalid for market {expected}"));
                    }
                    let market_code = quote_market_code(&market)
                        .ok_or_else(|| format!("invalid market: {market}"))?;
                    if seen.insert(format!("{market}:{code}")) {
                        securities.push(TradeSecurity { market: market_code, code });
                    }
                }
            }
        }
        if securities.is_empty() {
            Err("query parameter symbol is required".to_owned())
        } else {
            Ok(securities)
        }
    }

    pub(crate) fn max_trade_symbol(&self) -> Result<(String, String, i32), String> {
        let raw = self
            .query
            .get_first("symbol")
            .map(str::trim)
            .filter(|value| !value.is_empty())
            .ok_or_else(|| "query parameters symbol, orderType, and price are required".to_owned())?;
        let qualified = raw.contains('.') || raw.contains(':');
        let (market, code) = raw
            .split_once('.')
            .or_else(|| raw.split_once(':'))
            .map(|(market, code)| (market.trim().to_owned(), code.trim().to_owned()))
            .unwrap_or_else(|| (self.market_label().trim().to_owned(), raw.to_owned()));
        if market.is_empty() || code.is_empty() {
            return Err("query parameter symbol is invalid".to_owned());
        }
        let market = market.to_ascii_uppercase();
        if !qualified {
            return Err("query parameter symbol must be in MARKET.CODE form".to_owned());
        }
        if let Some(expected) = self
            .query
            .get_first("market")
            .map(str::trim)
            .filter(|value| !value.is_empty())
        {
            let expected = expected.to_ascii_uppercase();
            if market != expected && !(expected == "CN" && matches!(market.as_str(), "SH" | "SZ"))
            {
                return Err(format!("query parameter symbol {raw:?} is invalid for market {expected}"));
            }
        }
        let sec_market = match market.as_str() {
            "HK" => 1,
            "US" => 2,
            "SH" => 31,
            "SZ" => 32,
            "SG" => 41,
            "JP" => 51,
            "AU" => 61,
            "MY" => 71,
            "CA" => 81,
            _ => return Err(format!("invalid market: {market}")),
        };
        Ok((market, code.to_ascii_uppercase(), sec_market))
    }

    pub(crate) fn max_trade_quantity_request(
        &self,
        header: TradeHeader,
        code: String,
        sec_market: i32,
    ) -> Result<TradeMaxTradeQuantityRequest, String> {
        let order_type = self
            .query
            .get_first("orderType")
            .map(str::trim)
            .filter(|value| !value.is_empty())
            .ok_or_else(|| "query parameters symbol, orderType, and price are required".to_owned())?;
        let order_type = match order_type.to_ascii_uppercase().as_str() {
            "LIMIT" | "LIMIT_MAKER" | "NORMAL" => 1,
            "MARKET" => 2,
            "STOP" => 10,
            "STOP_LIMIT" | "STOPLIMIT" => 11,
            "TAKE_PROFIT_MARKET" | "MARKETIFTOUCHED" => 12,
            "TAKE_PROFIT" | "LIMITIFTOUCHED" => 13,
            value => return Err(format!("unsupported orderType {value:?}")),
        };
        let price_raw = self
            .query
            .get_first("price")
            .map(str::trim)
            .filter(|value| !value.is_empty())
            .ok_or_else(|| "query parameters symbol, orderType, and price are required".to_owned())?;
        let price = price_raw
            .parse::<f64>()
            .map_err(|_| "query parameter price is invalid".to_owned())?;
        if !price.is_finite() || price <= 0.0 {
            return Err("query parameter price must be positive".to_owned());
        }
        let adjust_side_and_limit = self.query.get_first("adjustSideAndLimit").map(str::trim).filter(|value| !value.is_empty()).map(|value| value.parse::<f64>().map_err(|_| "query parameter adjustSideAndLimit is invalid".to_owned())).transpose()?;
        if adjust_side_and_limit.is_some_and(|value| !value.is_finite()) {
            return Err("query parameter adjustSideAndLimit is invalid".to_owned());
        }
        let session = self.query.get_first("session").map(str::trim).filter(|value| !value.is_empty()).map(|value| match value.to_ascii_uppercase().as_str() {
            "NONE" | "SESSION_NONE" => Ok(0),
            "RTH" | "SESSION_RTH" => Ok(1),
            "ETH" | "SESSION_ETH" => Ok(2),
            "ALL" | "SESSION_ALL" => Ok(3),
            "OVERNIGHT" | "SESSION_OVERNIGHT" => Ok(4),
            _ => Err(format!("unsupported session {value:?}")),
        }).transpose()?;
        let position_id = self.query.get_first("positionId").map(str::trim).filter(|value| !value.is_empty()).map(|value| value.parse::<u64>().map_err(|_| "query parameter positionId is invalid".to_owned())).transpose()?;
        let order_id_ex = self.query.get_first("orderIdEx").map(str::trim).filter(|value| !value.is_empty()).map(ToOwned::to_owned);
        Ok(TradeMaxTradeQuantityRequest {
            header,
            order_type,
            code,
            price,
            order_id: None,
            adjust_price: adjust_side_and_limit.map(|value| value != 0.0),
            adjust_side_and_limit,
            sec_market: Some(sec_market),
            order_id_ex,
            session,
            position_id,
        })
    }

    pub(crate) fn cash_flow_direction(&self) -> Option<i32> {
        match self.query.get_first("direction").map(str::trim).map(str::to_ascii_uppercase).as_deref() {
            Some("IN") | Some("CASH_FLOW_DIRECTION_IN") => Some(1),
            Some("OUT") | Some("CASH_FLOW_DIRECTION_OUT") => Some(2),
            _ => None,
        }
    }

    pub(crate) fn filter(&self) -> Option<TradeFilter> {
        self.query.get_first("symbol").map(|symbol| TradeFilter { code_list: vec![symbol.trim().to_ascii_uppercase()], ..TradeFilter::default() })
    }

    pub(crate) fn history_scope(&self) -> Result<bool, String> {
        match self.query.get_first("scope").map(str::trim).unwrap_or("").to_ascii_uppercase().as_str() {
            "" | "CURRENT" => Ok(false),
            "HISTORY" => Ok(true),
            _ => Err("query parameter scope is invalid".to_owned()),
        }
    }

    pub(crate) fn trade_filter(&self, history: bool, market: &str) -> Result<Option<TradeFilter>, String> {
        let symbol = self.query.get_first("symbol").map(str::trim).filter(|value| !value.is_empty());
        let begin_time = if history { self.query.get_first("startTime").map(|value| normalize_history_time(value, market)).transpose()? } else { None };
        let end_time = if history { self.query.get_first("endTime").map(|value| normalize_history_time(value, market)).transpose()? } else { None };
        if !history && (self.query.get_first("startTime").is_some() || self.query.get_first("endTime").is_some()) {
            return Ok(symbol.map(|code| TradeFilter { code_list: vec![code.to_ascii_uppercase()], ..TradeFilter::default() }));
        }
        let has_filter = symbol.is_some() || begin_time.is_some() || end_time.is_some();
        Ok(has_filter.then(|| TradeFilter {
            code_list: symbol.map(|code| vec![code.to_ascii_uppercase()]).unwrap_or_default(),
            begin_time,
            end_time,
            ..TradeFilter::default()
        }))
    }

    pub(crate) fn status_codes(&self) -> Result<Vec<i32>, String> {
        let mut values = Vec::new();
        let mut seen = std::collections::BTreeSet::new();
        for raw in self.query.get_all("status").into_iter().flatten().chain(self.query.get_all("statuses").into_iter().flatten()) {
            for token in raw.split(',').map(str::trim).filter(|token| !token.is_empty()) {
                if let Some(code) = order_status_code(token) && seen.insert(code) {
                    values.push(code);
                }
            }
        }
        Ok(values)
    }
}

pub(crate) fn normalize_history_time(value: &str, market: &str) -> Result<String, String> {
    let trimmed = value.trim();
    if trimmed.is_empty() {
        return Ok(String::new());
    }
    let normalized = crate::product::product_query::normalize_optional_query_time(trimmed)
        .map_err(|_| "invalid history time; expected RFC3339 timestamp".to_owned())?
        .ok_or_else(|| "invalid history time; expected RFC3339 timestamp".to_owned())?;
    if !trimmed.contains('T') && !trimmed.ends_with('Z') && !trimmed.contains('+') {
        if trimmed.len() == 10 {
            return Ok(format!("{trimmed} 00:00:00"));
        }
        return Ok(trimmed.to_owned());
    }
    let parsed = OffsetDateTime::parse(&normalized, &Rfc3339)
        .map_err(|_| "invalid history time; expected RFC3339 timestamp".to_owned())?;
    let timestamp: jiff::Timestamp = normalized
        .parse()
        .map_err(|_| "invalid history time; expected RFC3339 timestamp".to_owned())?;
    let timezone = match market.trim().to_ascii_uppercase().as_str() {
        "US" => "America/New_York",
        "HK" => "Asia/Hong_Kong",
        "CN" | "SH" | "SZ" => "Asia/Shanghai",
        "SG" => "Asia/Singapore",
        "JP" => "Asia/Tokyo",
        "AU" => "Australia/Sydney",
        "MY" => "Asia/Kuala_Lumpur",
        "CA" => "America/Toronto",
        "FUTURES" | "CRYPTO" => "UTC",
        _ => "UTC",
    };
    let local = timestamp
        .to_zoned(jiff::tz::TimeZone::get(timezone).map_err(|error| error.to_string())?);
    let wall_clock = local.strftime("%Y-%m-%d %H:%M:%S").to_string();
    let format: &[FormatItem<'_>] = &parse_borrowed::<2>("[year]-[month]-[day] [hour]:[minute]:[second]")
        .map_err(|_| "invalid history time format".to_owned())?;
    parsed
        .format(format)
        .map_err(|_| "invalid history time format".to_owned())
        .map(|_| wall_clock)
}

pub(crate) fn order_status_code(value: &str) -> Option<i32> {
    match value.trim().to_ascii_uppercase().as_str() {
        "UNKNOWN" => Some(-1), "UNSUBMITTED" => Some(0), "WAITINGSUBMIT" => Some(1),
        "SUBMITTING" => Some(2), "SUBMITFAILED" => Some(3), "TIMEOUT" => Some(4),
        "SUBMITTED" => Some(5), "FILLEDPART" | "FILLED_PART" => Some(10),
        "FILLEDALL" | "FILLED_ALL" => Some(11), "CANCELLINGPART" | "CANCELLING_PART" => Some(12),
        "CANCELLINGALL" | "CANCELLING_ALL" => Some(13), "CANCELLEDPART" | "CANCELLED_PART" => Some(14),
        "CANCELLEDALL" | "CANCELLED_ALL" => Some(15), "FAILED" => Some(21), "DISABLED" => Some(22),
        "DELETED" => Some(23), "FILLCANCELLED" | "FILL_CANCELLED" => Some(24), _ => None,
    }
}

pub(crate) struct ResolvedTradeRequest {
    pub(crate) account_id: String,
    pub(crate) environment: String,
    pub(crate) market: String,
    pub(crate) header: TradeHeader,
}

pub(crate) fn account_identity(account: &jftrade_integration_futu::TradeAccountSnapshot) -> Option<String> {
    if account.acc_id != 0 {
        return Some(account.acc_id.to_string());
    }
    account
        .card_num
        .as_deref()
        .or(account.uni_card_num.as_deref())
        .map(str::trim)
        .filter(|id| !id.is_empty())
        .map(ToOwned::to_owned)
}

pub(crate) fn environment_label_from_code(code: i32) -> &'static str {
    match code {
        0 => "SIMULATE",
        1 => "REAL",
        _ => "UNKNOWN",
    }
}

pub(crate) fn market_code(value: &str) -> Result<i32, String> {
    match value.trim().to_ascii_uppercase().as_str() {
        "HK" => Ok(1),
        "US" => Ok(2),
        "SH" | "SZ" | "CN" => Ok(3),
        "FUTURES" => Ok(5),
        "SG" => Ok(6),
        "CRYPTO" => Ok(7),
        "AU" => Ok(8),
        "JP" => Ok(15),
        "MY" => Ok(111),
        "CA" => Ok(112),
        value => Err(format!("invalid market: {value}")),
    }
}

pub(crate) fn quote_market_code(value: &str) -> Option<i32> {
    match value {
        "HK" => Some(1),
        "US" => Some(11),
        "SH" | "CN" => Some(21),
        "SZ" => Some(22),
        "SG" => Some(31),
        "JP" => Some(41),
        "AU" => Some(51),
        "MY" => Some(61),
        "CA" => Some(71),
        _ => None,
    }
}

pub(crate) fn qot_market_label(value: i32) -> Option<&'static str> {
    match value {
        1 => Some("HK"),
        11 => Some("US"),
        21 => Some("SH"),
        22 => Some("SZ"),
        31 => Some("SG"),
        41 => Some("JP"),
        51 => Some("AU"),
        61 => Some("MY"),
        71 => Some("CA"),
        _ => None,
    }
}

pub(crate) fn trade_account_market_label(value: i32) -> Option<&'static str> {
    match value {
        21 | 22 => Some("CN"),
        _ => qot_market_label(value),
    }
}

pub(crate) fn normalize_trade_account_market(value: &str) -> String {
    match value.trim().to_ascii_uppercase().as_str() {
        "SH" | "SZ" => "CN".to_owned(),
        value => value.to_owned(),
    }
}

pub(crate) fn checked_at() -> String {
    SystemTime::now().duration_since(UNIX_EPOCH).map(|d| d.as_millis().to_string()).unwrap_or_else(|_| "0".to_owned())
}
