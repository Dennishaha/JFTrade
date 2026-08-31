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
                    "source": resp.source
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
            let Some(router) = &self.router else {
                return Err(MarketDataQuoteReadSnapshotError::Unavailable(
                    "market-data provider router is not configured".to_owned(),
                ));
            };
            let instrument_id = format!("{market}.{symbol}");
            let now_ms = current_unix_millis();
            let cached_tick = {
                let router_guard = router.lock().unwrap_or_else(|e| e.into_inner());
                let cache_handle = router_guard.cache_handle();
                let cache_guard = cache_handle.lock().unwrap_or_else(|e| e.into_inner());
                match cache_guard.lookup(&instrument_id, now_ms, 30_000) {
                    CacheLookup::Fresh(t) | CacheLookup::Stale(t) => Some(t),
                    CacheLookup::Missing => None,
                }
            };

            let Some(tick) = cached_tick else {
                return Err(MarketDataQuoteReadSnapshotError::Unavailable(format!(
                    "no cached snapshot available for {instrument_id}"
                )));
            };

            let observed_at = format_unix_millis_rfc3339(tick.observed_at_ms);

            return Ok(json!({
                "meta": {
                    "fromCache": true,
                    "instrumentId": instrument_id,
                    "resolvedAt": observed_at,
                    "source": "futu"
                },
                "request": {
                    "instrumentId": instrument_id,
                    "market": market,
                    "symbol": symbol
                },
                "snapshot": {
                    "ask": Value::Null,
                    "at": observed_at,
                    "bid": Value::Null,
                    "extended": {
                        "afterMarket": Value::Null,
                        "overnight": Value::Null,
                        "preMarket": Value::Null
                    },
                    "extendedHours": false,
                    "highPrice": Value::Null,
                    "lastClosePrice": Value::Null,
                    "lowPrice": Value::Null,
                    "observedAt": observed_at,
                    "openPrice": Value::Null,
                    "previousClosePrice": Value::Null,
                    "price": tick.price.to_string(),
                    "session": "regular",
                    "turnover": Value::Null,
                    "volume": tick.volume.as_str(),
                }
            }));
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

        Err(MarketDataQuoteReadSnapshotError::Unavailable(
            "candle provider is not configured".to_owned(),
        ))
    }
}
