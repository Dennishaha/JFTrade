//! Production broker quote and historical-kline route projections.

use super::*;

impl super::ProductionBrokerPort {
    pub(in crate::product::product_production_ports::product_production_ports_trade) fn read_securities_route(
        &self,
        request: &super::TradeRequest,
    ) -> Result<Value, super::BrokerReadSnapshotError> {
        let securities = request
            .securities()
            .map_err(super::BrokerReadSnapshotError::Invalid)?;
        let runtime = self
            .trade_runtime
            .as_ref()
            .ok_or_else(|| super::unavailable("Futu market-data runtime is unavailable"))?;
        let snapshots = runtime
            .security_snapshots(&securities)
            .map_err(super::unavailable)?;
        Ok(json!({
            "checkedAt": super::checked_at(),
            "connectivity": "connected",
            "securities": {
                "accountId": request.account_id().unwrap_or_default(),
                "snapshots": snapshots,
            },
        }))
    }

    pub(in crate::product::product_production_ports::product_production_ports_trade) fn read_quote_route(
        &self,
        request: &super::TradeRequest,
    ) -> Result<Value, super::BrokerReadSnapshotError> {
        let securities = request
            .securities()
            .map_err(super::BrokerReadSnapshotError::Invalid)?;
        let runtime = self
            .trade_runtime
            .as_ref()
            .ok_or_else(|| super::unavailable("Futu market-data runtime is unavailable"))?;
        let quote = runtime
            .quote_snapshot(&securities, request.account_id().unwrap_or_default())
            .map_err(super::unavailable)?;
        Ok(json!({
            "checkedAt": super::checked_at(),
            "connectivity": "connected",
            "quote": quote,
        }))
    }

    pub(in crate::product::product_production_ports::product_production_ports_trade) fn read_klines_route(
        &self,
        request: &super::TradeRequest,
    ) -> Result<Value, super::BrokerReadSnapshotError> {
        let symbol = request
            .query
            .get_first("symbol")
            .map(str::trim)
            .filter(|value| !value.is_empty())
            .ok_or_else(|| {
                super::BrokerReadSnapshotError::Invalid(
                    "query parameter symbol is required".to_owned(),
                )
            })?;
        let period = request.query.get_first("period").unwrap_or("1d");
        normalize_candle_period(period).map_err(|error| {
            super::BrokerReadSnapshotError::Invalid(format!("invalid candle period: {error:?}"))
        })?;
        if let Some(raw_limit) = request.query.get_first("limit") {
            raw_limit.trim().parse::<i32>().map_err(|_| {
                super::BrokerReadSnapshotError::Invalid(
                    "query parameter limit is invalid".to_owned(),
                )
            })?;
        }
        let before = request.query.get_first("before").unwrap_or("");
        let from = request.query.get_first("fromTime").unwrap_or("");
        let to = request.query.get_first("toTime").unwrap_or("");
        if !before.trim().is_empty() && (!from.trim().is_empty() || !to.trim().is_empty()) {
            return Err(super::BrokerReadSnapshotError::Invalid(
                "beforeTime cannot be combined with fromTime or toTime".to_owned(),
            ));
        }
        if !before.trim().is_empty() {
            parse_candle_before_time(before).map_err(|_| {
                super::BrokerReadSnapshotError::Invalid(
                    "before must be an RFC3339 timestamp".to_owned(),
                )
            })?;
        }
        for value in [from, to] {
            if !value.trim().is_empty() {
                normalize_optional_query_time(value).map_err(|_| {
                    super::BrokerReadSnapshotError::Invalid(
                        "fromTime and toTime must be valid timestamps".to_owned(),
                    )
                })?;
            }
        }
        let runtime = self
            .trade_runtime
            .as_ref()
            .ok_or_else(|| super::unavailable("Futu historical klines runtime is unavailable"))?;
        let (market, code) = symbol.split_once('.').ok_or_else(|| {
            super::BrokerReadSnapshotError::Invalid("symbol must be MARKET.CODE".to_owned())
        })?;
        let market = market.trim().to_ascii_uppercase();
        let market_code = super::quote_market_code(&market).ok_or_else(|| {
            super::BrokerReadSnapshotError::Invalid("symbol market is unsupported".to_owned())
        })?;
        let period = normalize_candle_period(period).map_err(|error| {
            super::BrokerReadSnapshotError::Invalid(format!("invalid candle period: {error:?}"))
        })?;
        let requested_limit = request
            .query
            .get_first("limit")
            .and_then(|raw| raw.trim().parse::<i32>().ok())
            .filter(|value| *value > 0)
            .map(|value| value.min(1000));
        let limit = requested_limit.map_or(500, |value| value.max(200));
        let extended_hours =
            market == "US" && crate::product::product_query::is_intraday_candle_period(period);
        let sessions = parse_requested_sessions(&request.query, extended_hours)
            .map_err(super::BrokerReadSnapshotError::Invalid)?;
        let before = request.query.get_first("before").unwrap_or("").trim();
        let begin = request.query.get_first("fromTime").unwrap_or("").trim();
        let end = request.query.get_first("toTime").unwrap_or("").trim();
        let (begin_time, end_time) = if !before.is_empty() {
            (
                "1970-01-01 00:00:00".to_owned(),
                super::normalize_history_time(before, &market)
                    .map_err(super::BrokerReadSnapshotError::Invalid)?,
            )
        } else {
            (
                super::normalize_history_time(
                    if begin.is_empty() {
                        "1970-01-01 00:00:00"
                    } else {
                        begin
                    },
                    &market,
                )
                .map_err(super::BrokerReadSnapshotError::Invalid)?,
                super::normalize_history_time(
                    if end.is_empty() {
                        "2999-12-31 23:59:59"
                    } else {
                        end
                    },
                    &market,
                )
                .map_err(super::BrokerReadSnapshotError::Invalid)?,
            )
        };
        let session_code = if !extended_hours {
            None
        } else if sessions.len() == 1 {
            Some(match sessions[0] {
                "regular" => 1,
                "extended" => 2,
                _ => 3,
            })
        } else {
            Some(3)
        };
        let adjustment = match request.query.get_first("adjustment").unwrap_or("forward") {
            "none" => 0,
            "backward" => 2,
            "forward" | "" => 1,
            other => {
                return Err(super::BrokerReadSnapshotError::Invalid(format!(
                    "invalid candle adjustment {other:?}"
                )));
            }
        };
        let mut historical = HistoricalKlineResult {
            security: jftrade_integration_futu::HistoricalSecurity {
                market: market_code,
                code: code.trim().to_ascii_uppercase(),
            },
            name: None,
            klines: Vec::new(),
            next_req_key: Vec::new(),
        };
        let plans = if extended_hours && sessions.len() > 1 {
            sessions
                .iter()
                .map(|session| {
                    Some(match *session {
                        "regular" => 1,
                        "extended" => 2,
                        _ => 3,
                    })
                })
                .collect::<Vec<_>>()
        } else {
            vec![session_code]
        };
        for plan in plans {
            let mut cursor = Vec::new();
            let mut exhausted = false;
            for _page_number in 0..32 {
                let page = runtime
                    .historical_klines(&HistoricalKlineQuery {
                        market: market_code,
                        symbol: code.trim().to_ascii_uppercase(),
                        period: period.to_owned(),
                        adjustment,
                        begin_time: begin_time.clone(),
                        end_time: end_time.clone(),
                        max_ack_kl_num: Some(limit),
                        next_req_key: cursor.clone(),
                        extended_time: extended_hours.then_some(true),
                        session: plan,
                    })
                    .map_err(super::unavailable)?;
                historical.name = historical.name.or(page.name);
                historical.klines.extend(page.klines);
                if page.next_req_key.is_empty() {
                    exhausted = true;
                    break;
                }
                cursor = page.next_req_key;
                if historical.klines.len() >= usize::try_from(limit).unwrap_or(usize::MAX) {
                    historical.next_req_key = cursor.clone();
                    exhausted = true;
                    break;
                }
                historical.next_req_key = cursor.clone();
            }
            if !exhausted && !cursor.is_empty() {
                return Err(super::unavailable(
                    "Futu historical klines pagination exceeded 32 pages",
                ));
            }
        }
        historical
            .klines
            .sort_by(|left, right| left.time.cmp(&right.time));
        historical
            .klines
            .dedup_by(|left, right| left.time == right.time);
        Ok(json!({
            "checkedAt": super::checked_at(),
            "connectivity": "connected",
            "klines": historical_snapshot(request, &historical, period, extended_hours, &sessions, requested_limit),
        }))
    }
}
