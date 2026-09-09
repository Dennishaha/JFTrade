use super::*;

impl ProductionMarketDataQuotePort {
    pub(super) async fn read_snapshots(
        &self,
        suffix: &str,
        query: &str,
    ) -> Result<Value, MarketDataQuoteReadSnapshotError> {
        let (market, symbol) = parse_market_symbol_path(suffix)?;
        let query_map =
            QueryMap::parse(query).map_err(|_| MarketDataQuoteReadSnapshotError::Failed {
                status: 400,
                code: "BAD_REQUEST".to_owned(),
                message: "invalid URL escape".to_owned(),
                retry_after_seconds: None,
            })?;
        let refresh = match query_map.get_first("refresh") {
            Some("true") | Some("1") => true,
            Some("false") | Some("0") | None => false,
            Some(_) => {
                return Err(MarketDataQuoteReadSnapshotError::Failed {
                    status: 400,
                    code: "BAD_REQUEST".to_owned(),
                    message: "invalid refresh query".to_owned(),
                    retry_after_seconds: None,
                });
            }
        };

        let provider = self.active_provider()?;

        if let Some(helper) = &self.helper
            && (provider == MarketDataProvider::Yfinance || provider == MarketDataProvider::Akshare)
        {
            let provider_str = match provider {
                MarketDataProvider::Yfinance => "yfinance",
                MarketDataProvider::Akshare => "akshare",
                MarketDataProvider::Futu => "futu",
            };
            let resp = helper
                .get_provider_json::<HelperSnapshotResponse>(
                    provider_str,
                    &["snapshot", &market, &symbol],
                )
                .await
                .map_err(|error| map_helper_quote_error(error, "MARKET_SNAPSHOT_FAILED"))?;

            let instrument_id = format!("{market}.{symbol}");
            let price_str = resp.price.as_str();
            if price_str.trim().is_empty() {
                return Err(MarketDataQuoteReadSnapshotError::Failed {
                    status: 502,
                    code: "MARKET_DATA_PROVIDER_FAILED".to_owned(),
                    message: "empty price string in snapshot response".to_owned(),
                    retry_after_seconds: None,
                });
            }

            if resp.observed_at.trim().is_empty() {
                return Err(MarketDataQuoteReadSnapshotError::Failed {
                    status: 502,
                    code: "MARKET_DATA_PROVIDER_FAILED".to_owned(),
                    message: "missing observedAt timestamp in snapshot response".to_owned(),
                    retry_after_seconds: None,
                });
            }

            let to_json_val = |opt: &Option<HelperPriceValue>| -> Value {
                opt.as_ref()
                    .map(|v| json!(v.as_str()))
                    .unwrap_or(Value::Null)
            };

            return Ok(json!({
                "meta": {
                    "fromCache": !refresh,
                    "instrumentId": instrument_id,
                    "resolvedAt": resp.observed_at,
                    "source": resp.source,
                    "brokerId": provider_str,
                },
                "request": {
                    "instrumentId": instrument_id,
                    "market": market,
                    "symbol": symbol
                },
                "snapshot": {
                    "ask": to_json_val(&resp.ask),
                    "at": resp.observed_at,
                    "bid": to_json_val(&resp.bid),
                    "extended": {
                        "afterMarket": Value::Null,
                        "overnight": Value::Null,
                        "preMarket": Value::Null
                    },
                    "extendedHours": false,
                    "highPrice": to_json_val(&resp.high_price),
                    "lastClosePrice": to_json_val(&resp.last_close_price),
                    "lowPrice": to_json_val(&resp.low_price),
                    "observedAt": resp.observed_at,
                    "openPrice": to_json_val(&resp.open_price),
                    "previousClosePrice": to_json_val(&resp.previous_close_price),
                    "price": price_str,
                    "session": "regular",
                    "turnover": to_json_val(&resp.turnover),
                    "volume": to_json_val(&resp.volume),
                }
            }));
        }

        if provider == MarketDataProvider::Futu {
            let instrument_id = format!("{market}.{symbol}");
            let now_ms = current_unix_millis();
            let cached_tick = self.router.as_ref().and_then(|router| {
                let router_guard = router.lock().unwrap_or_else(|e| e.into_inner());
                let cache_handle = router_guard.cache_handle();
                let cache_guard = cache_handle.lock().unwrap_or_else(|e| e.into_inner());
                match cache_guard.lookup(&instrument_id, now_ms, 30_000) {
                    CacheLookup::Fresh(t) | CacheLookup::Stale(t) => Some(t),
                    CacheLookup::Missing => None,
                }
            });

            if let Some(tick) = cached_tick {
                let observed_at = format_unix_millis_rfc3339(tick.observed_at_ms);
                let snap = tick.snapshot.as_ref();
                let ask = snap
                    .and_then(|s| s.ask_price)
                    .map(|v| json!(v.to_string()))
                    .unwrap_or(Value::Null);
                let bid = snap
                    .and_then(|s| s.bid_price)
                    .map(|v| json!(v.to_string()))
                    .unwrap_or(Value::Null);
                let open_price = snap
                    .and_then(|s| s.open_price)
                    .map(|v| json!(v.to_string()))
                    .unwrap_or(Value::Null);
                let high_price = snap
                    .and_then(|s| s.high_price)
                    .map(|v| json!(v.to_string()))
                    .unwrap_or(Value::Null);
                let low_price = snap
                    .and_then(|s| s.low_price)
                    .map(|v| json!(v.to_string()))
                    .unwrap_or(Value::Null);
                let prev_close = snap
                    .and_then(|s| s.previous_close)
                    .map(|v| json!(v.to_string()))
                    .unwrap_or(Value::Null);
                let turnover = snap
                    .and_then(|s| s.turnover.as_ref())
                    .map(|v| json!(v.as_str()))
                    .unwrap_or(Value::Null);

                return Ok(json!({
                    "meta": {
                        "fromCache": true,
                        "instrumentId": instrument_id,
                        "resolvedAt": observed_at,
                        "source": "futu",
                        "brokerId": "futu",
                    },
                    "request": {
                        "instrumentId": instrument_id,
                        "market": market,
                        "symbol": symbol
                    },
                    "snapshot": {
                        "ask": ask,
                        "at": observed_at,
                        "bid": bid,
                        "extended": {
                            "afterMarket": Value::Null,
                            "overnight": Value::Null,
                            "preMarket": Value::Null
                        },
                        "extendedHours": false,
                        "highPrice": high_price,
                        "lastClosePrice": prev_close.clone(),
                        "lowPrice": low_price,
                        "observedAt": observed_at,
                        "openPrice": open_price,
                        "previousClosePrice": prev_close,
                        "price": tick.price.to_string(),
                        "session": "regular",
                        "turnover": turnover,
                        "volume": tick.volume.as_str(),
                    }
                }));
            }

            let fallback_snapshot = if let (Some(runtime), Some(market_code)) = (
                &self.trade_runtime,
                quote_market_code(&market),
            ) {
                let security = jftrade_integration_futu::TradeSecurity {
                    market: market_code,
                    code: symbol.clone(),
                };
                runtime.security_snapshots(&[security]).ok().and_then(|mut list| {
                    if list.is_empty() { None } else { Some(list.remove(0)) }
                })
            } else {
                None
            };

            if let Some(snap) = fallback_snapshot {
                let price_str = snap.get("lastPrice")
                    .and_then(|v| {
                        if let Some(f) = v.as_f64() {
                            Some(f.to_string())
                        } else {
                            v.as_str().map(str::to_owned)
                        }
                    })
                    .unwrap_or_default();
                if !price_str.trim().is_empty() {
                    let observed_at = snap.get("updateTime")
                        .and_then(|v| v.as_str())
                        .map(str::to_owned)
                        .unwrap_or_else(|| format_unix_millis_rfc3339(now_ms));
                    let to_val = |key: &str| snap.get(key).cloned().unwrap_or(Value::Null);
                    return Ok(json!({
                        "meta": {
                            "fromCache": false,
                            "instrumentId": instrument_id,
                            "resolvedAt": observed_at,
                            "source": "futu",
                            "brokerId": "futu",
                        },
                        "request": {
                            "instrumentId": instrument_id,
                            "market": market,
                            "symbol": symbol,
                        },
                        "snapshot": {
                            "ask": to_val("askPrice"),
                            "at": observed_at,
                            "bid": to_val("bidPrice"),
                            "extended": {
                                "afterMarket": Value::Null,
                                "overnight": Value::Null,
                                "preMarket": Value::Null
                            },
                            "extendedHours": false,
                            "highPrice": to_val("highPrice"),
                            "lastClosePrice": to_val("previousClose"),
                            "lowPrice": to_val("lowPrice"),
                            "observedAt": observed_at,
                            "openPrice": to_val("openPrice"),
                            "previousClosePrice": to_val("previousClose"),
                            "price": price_str,
                            "session": "regular",
                            "turnover": to_val("turnover"),
                            "volume": to_val("volume"),
                        }
                    }));
                }
            }

            return Err(MarketDataQuoteReadSnapshotError::Unavailable(format!(
                "no cached snapshot available for {instrument_id}"
            )));
        }

        Err(MarketDataQuoteReadSnapshotError::Unavailable(format!(
            "snapshot provider is not supported for {market}.{symbol}"
        )))
    }

    pub(super) async fn read_candles(
        &self,
        suffix: &str,
        query: &str,
    ) -> Result<Value, MarketDataQuoteReadSnapshotError> {
        let (market, symbol) = parse_market_symbol_path(suffix)?;
        let query_map =
            QueryMap::parse(query).map_err(|_| MarketDataQuoteReadSnapshotError::Failed {
                status: 400,
                code: "BAD_REQUEST".to_owned(),
                message: "invalid URL escape".to_owned(),
                retry_after_seconds: None,
            })?;
        let period_raw = query_map.get_first("period").unwrap_or("1m");
        let period = normalize_candle_period(period_raw).map_err(|_| {
            MarketDataQuoteReadSnapshotError::Failed {
                status: 400,
                code: "BAD_REQUEST".to_owned(),
                message: "invalid candle query".to_owned(),
                retry_after_seconds: None,
            }
        })?;

        let raw_limit: Option<i64> = if let Some(limit_str) = query_map.get_first("limit") {
            let parsed = limit_str.trim().parse::<i64>().map_err(|_| {
                MarketDataQuoteReadSnapshotError::Failed {
                    status: 400,
                    code: "BAD_REQUEST".to_owned(),
                    message: "limit must be an integer".to_owned(),
                    retry_after_seconds: None,
                }
            })?;
            Some(parsed)
        } else {
            None
        };
        let limit: usize = match raw_limit {
            Some(n) if n <= 0 => 200,
            Some(n) if n > 1000 => 1000,
            Some(n) => usize::try_from(n).unwrap_or(200),
            None => 200,
        };

        let sessions_opt =
            parse_candle_sessions(query_map.get_all("sessions")).map_err(|err| match err {
                CandleSessionError::Empty => MarketDataQuoteReadSnapshotError::Failed {
                    status: 400,
                    code: "BAD_REQUEST".to_owned(),
                    message: "invalid candle sessions: at least one session is required".to_owned(),
                    retry_after_seconds: None,
                },
                CandleSessionError::Invalid(token) => MarketDataQuoteReadSnapshotError::Failed {
                    status: 400,
                    code: "BAD_REQUEST".to_owned(),
                    message: format!("invalid candle sessions: {token:?}"),
                    retry_after_seconds: None,
                },
            })?;
        let sessions = match sessions_opt {
            Some(s) => s,
            None => {
                if market.eq_ignore_ascii_case("US") && is_intraday_candle_period(period) {
                    vec!["regular", "extended"]
                } else {
                    vec!["regular"]
                }
            }
        };

        let from_time = match query_map
            .get_first("from")
            .or_else(|| query_map.get_first("fromTime"))
            .or_else(|| query_map.get_first("from_time"))
        {
            Some(ft) => normalize_optional_query_time(ft).map_err(|_| {
                MarketDataQuoteReadSnapshotError::Failed {
                    status: 400,
                    code: "BAD_REQUEST".to_owned(),
                    message: "time must be a valid timestamp".to_owned(),
                    retry_after_seconds: None,
                }
            })?,
            None => None,
        };

        let to_time = match query_map
            .get_first("to")
            .or_else(|| query_map.get_first("toTime"))
            .or_else(|| query_map.get_first("to_time"))
        {
            Some(tt) => normalize_optional_query_time(tt).map_err(|_| {
                MarketDataQuoteReadSnapshotError::Failed {
                    status: 400,
                    code: "BAD_REQUEST".to_owned(),
                    message: "time must be a valid timestamp".to_owned(),
                    retry_after_seconds: None,
                }
            })?,
            None => None,
        };

        let before = match query_map.get_first("before") {
            Some(bf) => parse_candle_before_time(bf).map_err(|_| {
                MarketDataQuoteReadSnapshotError::Failed {
                    status: 400,
                    code: "BAD_REQUEST".to_owned(),
                    message: "before must be an RFC3339 timestamp".to_owned(),
                    retry_after_seconds: None,
                }
            })?,
            None => None,
        };

        if period == "tick" && before.is_some() {
            return Err(MarketDataQuoteReadSnapshotError::Failed {
                status: 400,
                code: "BAD_REQUEST".to_owned(),
                message: "tick candles do not support historical pagination".to_owned(),
                retry_after_seconds: None,
            });
        }

        if before.is_some() && (from_time.is_some() || to_time.is_some()) {
            return Err(MarketDataQuoteReadSnapshotError::Failed {
                status: 400,
                code: "BAD_REQUEST".to_owned(),
                message: "before cannot be combined with from or to".to_owned(),
                retry_after_seconds: None,
            });
        }

        let provider = self.active_provider()?;

        if let Some(helper) = &self.helper
            && (provider == MarketDataProvider::Yfinance || provider == MarketDataProvider::Akshare)
        {
            let provider_str = match provider {
                MarketDataProvider::Yfinance => "yfinance",
                MarketDataProvider::Akshare => "akshare",
                MarketDataProvider::Futu => "futu",
            };
            let limit_str = limit.to_string();
            let mut query_params = vec![("period", period), ("limit", limit_str.as_str())];
            if let Some(ft) = from_time.as_deref() {
                query_params.push(("from", ft));
            }
            if let Some(tt) = to_time.as_deref() {
                query_params.push(("to", tt));
            }
            if let Some(bf) = before.as_deref() {
                query_params.push(("before", bf));
            }
            let sessions_joined = sessions.join(",");
            query_params.push(("sessions", sessions_joined.as_str()));

            let resp = helper
                .get_provider_json_with_query::<HelperCandlesResponse>(
                    provider_str,
                    &["candles", &market, &symbol],
                    &query_params,
                )
                .await
                .map_err(|error| map_helper_quote_error(error, "OPEND_CANDLES_FAILED"))?;

            return crate::product::product_candle_converter::convert_helper_candles_response(
                resp,
                crate::product::product_candle_converter::HelperCandleConversionParams {
                    market: &market,
                    symbol: &symbol,
                    period,
                    limit,
                    from_time: from_time.as_deref(),
                    to_time: to_time.as_deref(),
                    before: before.as_deref(),
                    sessions: &sessions,
                    is_yfinance: provider == MarketDataProvider::Yfinance,
                    is_akshare: provider == MarketDataProvider::Akshare,
                    calendar: self.calendar.as_deref(),
                },
            );
        }

        if provider == MarketDataProvider::Futu {
            let market_code = quote_market_code(&market)
                .ok_or_else(|| MarketDataQuoteReadSnapshotError::Failed {
                    status: 400,
                    code: "BAD_REQUEST".to_owned(),
                    message: format!("invalid market: {market}"),
                    retry_after_seconds: None,
                })?;
            let Some(runtime) = &self.trade_runtime else {
                return Err(MarketDataQuoteReadSnapshotError::Unavailable(
                    "Futu historical klines runtime is unavailable".to_owned(),
                ));
            };
            let (begin_time, end_time, query_current) = futu_kline_query_window(
                &market,
                period,
                limit,
                from_time.as_deref(),
                to_time.as_deref(),
                before.as_deref(),
            );
            let extended_hours = sessions.iter().any(|s| *s == "extended" || *s == "overnight");
            let futu_result = runtime
                .historical_klines(&jftrade_integration_futu::HistoricalKlineQuery {
                    market: market_code,
                    symbol: symbol.clone(),
                    period: period.to_owned(),
                    adjustment: 1,
                    begin_time,
                    end_time,
                    max_ack_kl_num: Some(limit as i32),
                    next_req_key: Vec::new(),
                    extended_time: Some(extended_hours),
                    session: None,
                });
            let mut result = match futu_result {
                Ok(res) if !res.klines.is_empty() => res,
                other => {
                    if let Some(helper) = &self.helper {
                        let provider_str = if market.eq_ignore_ascii_case("US") {
                            "yfinance"
                        } else {
                            "akshare"
                        };
                        let limit_str = limit.to_string();
                        let mut query_params = vec![("period", period), ("limit", limit_str.as_str())];
                        if let Some(ft) = from_time.as_deref() { query_params.push(("from", ft)); }
                        if let Some(tt) = to_time.as_deref() { query_params.push(("to", tt)); }
                        if let Some(bf) = before.as_deref() { query_params.push(("before", bf)); }
                        let sessions_joined = sessions.join(",");
                        query_params.push(("sessions", sessions_joined.as_str()));

                        if let Ok(resp) = helper
                            .get_provider_json_with_query::<HelperCandlesResponse>(
                                provider_str,
                                &["candles", &market, &symbol],
                                &query_params,
                            )
                            .await
                        {
                            return crate::product::product_candle_converter::convert_helper_candles_response(
                                resp,
                                crate::product::product_candle_converter::HelperCandleConversionParams {
                                    market: &market,
                                    symbol: &symbol,
                                    period,
                                    limit,
                                    from_time: from_time.as_deref(),
                                    to_time: to_time.as_deref(),
                                    before: before.as_deref(),
                                    sessions: &sessions,
                                    is_yfinance: provider_str == "yfinance",
                                    is_akshare: provider_str == "akshare",
                                    calendar: self.calendar.as_deref(),
                                },
                            );
                        }
                    }
                    return Err(other.err().map(MarketDataQuoteReadSnapshotError::Unavailable).unwrap_or_else(|| {
                        MarketDataQuoteReadSnapshotError::Unavailable("no candles found".to_owned())
                    }));
                }
            };

            if query_current {
                let current_query = jftrade_integration_futu::CurrentKlineQuery::new(
                    market_code,
                    &symbol,
                    period,
                );
                if let Ok(current_res) = runtime.current_kline(&current_query) {
                    if !current_res.klines.is_empty() {
                        result.klines = jftrade_integration_futu::kline_query::merge_klines_by_time(
                            &result.klines,
                            &current_res.klines,
                        );
                    }
                    if result.name.is_none() {
                        result.name = current_res.name;
                    }
                }
            }

            let name_missing = result.name.as_deref().unwrap_or_default().trim().is_empty()
                || result.name.as_deref().is_some_and(|n| n.eq_ignore_ascii_case(&symbol));
            if name_missing
                && let Ok(snapshots) = runtime.security_snapshots(&[jftrade_integration_futu::TradeSecurity {
                    market: market_code,
                    code: symbol.clone(),
                }])
            {
                for snap in snapshots {
                    if let Some(n) = snap.get("name").and_then(|v| v.as_str()) {
                        let trimmed = n.trim();
                        if !trimmed.is_empty() && !trimmed.eq_ignore_ascii_case(&symbol) {
                            result.name = Some(trimmed.to_owned());
                            break;
                        }
                    }
                }
            }

            let non_blank_klines: Vec<_> = result.klines.iter().filter(|k| !k.is_blank).collect();
            let mut candles = Vec::with_capacity(non_blank_klines.len());
            let now_ts = jiff::Timestamp::now();
            let total_klines = non_blank_klines.len();

            for (index, kline) in non_blank_klines.into_iter().enumerate() {
                let at = canonical_candle_time(&kline.time, &market);
                let open = time::OffsetDateTime::parse(&at, &time::format_description::well_known::Rfc3339)
                    .map_err(|e| MarketDataQuoteReadSnapshotError::Unavailable(e.to_string()))?;
                let now = time::OffsetDateTime::from_unix_timestamp_nanos(now_ts.as_nanosecond())
                    .map_err(|e| MarketDataQuoteReadSnapshotError::Unavailable(e.to_string()))?;
                let is_closed = index < total_klines - 1 || jftrade_calendar::candle_is_closed(
                    self.calendar.as_deref(), &market, period, open, now, &sessions,
                ).map_err(|e| MarketDataQuoteReadSnapshotError::Unavailable(e.to_string()))?;
                candles.push(json!({
                    "at": at,
                    "close": kline.close_price.map(|v| v.to_string()).unwrap_or_default(),
                    "high": kline.high_price.map(|v| v.to_string()).unwrap_or_default(),
                    "low": kline.low_price.map(|v| v.to_string()).unwrap_or_default(),
                    "open": kline.open_price.map(|v| v.to_string()).unwrap_or_default(),
                    "period": period,
                    "session": "regular",
                    "volume": kline.volume.map(|v| v.to_string()).unwrap_or_else(|| "0".to_owned()),
                    "closed": is_closed,
                }));
            }
            if candles.len() > limit {
                candles = candles.split_off(candles.len() - limit);
            }
            let next_before = candles
                .first()
                .and_then(|c| c.get("at"))
                .and_then(|v| v.as_str())
                .map(str::to_owned);
            let bounded = from_time.is_some() || to_time.is_some() || before.is_some();
            let pagination = if !bounded && !result.next_req_key.is_empty() {
                json!({ "hasMore": true, "nextBefore": next_before })
            } else {
                json!({ "hasMore": false })
            };
            let instrument_id = format!("{market}.{symbol}");
            return Ok(json!({
                "candles": candles,
                "meta": {
                    "brokerId": "futu",
                    "extendedHours": extended_hours,
                    "fromCache": false,
                    "instrumentId": instrument_id,
                    "resolvedAt": format_unix_millis_rfc3339(current_unix_millis()),
                    "sessions": sessions,
                    "source": "futu",
                },
                "pagination": pagination,
                "request": {
                    "instrument": {
                        "instrumentId": instrument_id,
                        "market": market,
                        "symbol": symbol,
                    },
                    "limit": limit,
                    "period": period,
                    "sessions": sessions,
                },
                "totalReturned": candles.len(),
            }));
        }

        Err(MarketDataQuoteReadSnapshotError::Unavailable(
            "candle provider is not configured".to_owned(),
        ))
    }
}

fn effective_period_seconds(period: &str) -> i64 {
    let secs = jftrade_integration_futu::kline_query::period_duration_seconds(period);
    if secs > 0 {
        return secs;
    }
    match period.trim().to_ascii_lowercase().as_str() {
        "1d" | "day" => 86_400,
        "1w" | "week" => 604_800,
        "1mo" | "month" => 2_592_000,
        "1y" | "year" => 31_536_000,
        _ => 86_400,
    }
}

fn parse_futu_time_to_ts(raw: &str, tz: &jiff::tz::TimeZone) -> Option<jiff::Timestamp> {
    let trimmed = raw.trim();
    if trimmed.is_empty() {
        return None;
    }
    if (trimmed.contains('T') || trimmed.ends_with('Z'))
        && let Ok(ts) = trimmed.parse::<jiff::Timestamp>()
    {
        return Some(ts);
    }
    if let Ok(dt) = jiff::civil::DateTime::strptime("%Y-%m-%d %H:%M:%S", trimmed) {
        return dt.to_zoned(tz.clone()).ok().map(|z| z.timestamp());
    }
    if let Ok(d) = jiff::civil::Date::strptime("%Y-%m-%d", trimmed) {
        return d
            .to_datetime(jiff::civil::Time::midnight())
            .to_zoned(tz.clone())
            .ok()
            .map(|z| z.timestamp());
    }
    None
}

fn futu_kline_query_window(
    market: &str,
    period: &str,
    limit: usize,
    from_time: Option<&str>,
    to_time: Option<&str>,
    before: Option<&str>,
) -> (String, String, bool) {
    let tz_str = match market {
        "US" => "America/New_York",
        "HK" => "Asia/Hong_Kong",
        "SH" | "SZ" | "CN" => "Asia/Shanghai",
        "JP" => "Asia/Tokyo",
        _ => "UTC",
    };
    let tz = jiff::tz::TimeZone::get(tz_str).unwrap_or(jiff::tz::TimeZone::UTC);
    let now_ts = jiff::Timestamp::now();
    let bar_duration = effective_period_seconds(period);
    let eff_limit = limit.clamp(1, 1000) as i64;
    let mut lookback_secs = bar_duration.saturating_mul(eff_limit).saturating_mul(4);
    let min_lookback_secs = if bar_duration >= 86_400 {
        45 * 86_400
    } else {
        36 * 3_600
    };
    if lookback_secs < min_lookback_secs {
        lookback_secs = min_lookback_secs;
    }

    let end_ts = if let Some(raw_end) = to_time.or(before) {
        parse_futu_time_to_ts(raw_end, &tz).unwrap_or(now_ts)
    } else {
        now_ts
    };

    let begin_ts = if let Some(raw_begin) = from_time {
        let parsed = parse_futu_time_to_ts(raw_begin, &tz).unwrap_or_else(|| {
            end_ts
                .checked_sub(jiff::SignedDuration::from_secs(lookback_secs))
                .unwrap_or(now_ts)
        });
        if parsed >= end_ts {
            end_ts
                .checked_sub(jiff::SignedDuration::from_secs(lookback_secs))
                .unwrap_or(now_ts)
        } else {
            parsed
        }
    } else {
        end_ts
            .checked_sub(jiff::SignedDuration::from_secs(lookback_secs))
            .unwrap_or(now_ts)
    };

    let begin_zoned = begin_ts.to_zoned(tz.clone());
    let end_zoned = end_ts.to_zoned(tz);
    let begin_str = begin_zoned.strftime("%Y-%m-%d %H:%M:%S").to_string();
    let end_str = end_zoned.strftime("%Y-%m-%d %H:%M:%S").to_string();

    let query_current = end_ts
        >= now_ts
            .checked_sub(jiff::SignedDuration::from_secs(bar_duration))
            .unwrap_or(now_ts);
    (begin_str, end_str, query_current)
}
