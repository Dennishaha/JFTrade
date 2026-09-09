use super::*;
use crate::product::product_production_ports::SharedTradeReadRuntime;
use jftrade_integration_futu::{
    CurrentKlineError, CurrentKlineQuery, CurrentKlineResult, HistoricalKline,
    HistoricalKlineError, HistoricalKlineQuery, HistoricalKlineReadPort, HistoricalKlineResult,
    HistoricalSecurity,
};

#[derive(Debug, Default)]
struct PagedHistory {
    requests: Mutex<Vec<HistoricalKlineQuery>>,
    fail_second_page: bool,
    current_calls: AtomicUsize,
}

fn candle(minute: usize) -> HistoricalKline {
    HistoricalKline {
        time: format!("2026-01-05 10:{minute:02}:00"),
        is_blank: false,
        high_price: Some(100.0),
        open_price: Some(100.0),
        low_price: Some(100.0),
        close_price: Some(100.0),
        volume: Some(100),
        turnover: None,
        change_rate: None,
    }
}

impl HistoricalKlineReadPort for PagedHistory {
    fn query(
        &self,
        query: &HistoricalKlineQuery,
    ) -> Result<HistoricalKlineResult, HistoricalKlineError> {
        self.requests.lock().unwrap().push(query.clone());
        let first = query.next_req_key.is_empty();
        if !first && self.fail_second_page {
            return Err(HistoricalKlineError::MissingS2c);
        }
        Ok(HistoricalKlineResult {
            security: HistoricalSecurity {
                market: 1,
                code: "00700".into(),
            },
            name: Some("腾讯控股".into()),
            klines: if first {
                vec![candle(0), candle(1)]
            } else {
                vec![candle(2), candle(3), candle(4)]
            },
            next_req_key: if first { vec![1] } else { vec![] },
        })
    }

    fn query_current(
        &self,
        _: &CurrentKlineQuery,
    ) -> Result<CurrentKlineResult, CurrentKlineError> {
        self.current_calls.fetch_add(1, Ordering::SeqCst);
        Ok(CurrentKlineResult {
            security: HistoricalSecurity {
                market: 1,
                code: "00700".into(),
            },
            name: None,
            klines: vec![candle(5)],
        })
    }
}

fn port(reader: Arc<PagedHistory>) -> ProductionMarketDataQuotePort {
    let runtime = Arc::new(SharedTradeReadRuntime::default());
    runtime.set_historical_klines(Some(reader));
    let state = Arc::new(ActiveProviderState::new(Some(
        jftrade_settings::MarketDataProvider::Futu,
    )));
    ProductionMarketDataQuotePort::new(state, None, None, None).with_trade_runtime(Some(runtime))
}

#[tokio::test]
async fn candle_route_keeps_latest_history_after_all_forward_pages_and_current_bar() {
    let reader = Arc::new(PagedHistory::default());
    let result = port(reader.clone())
        .read("/api/v1/market-data/candles/HK/00700", "period=1m&limit=3")
        .await
        .unwrap();
    let candles = result["candles"].as_array().unwrap();
    assert_eq!(candles.len(), 3);
    assert_eq!(candles[0]["at"], "2026-01-05T02:03:00Z");
    assert_eq!(candles[1]["at"], "2026-01-05T02:04:00Z");
    assert_eq!(candles[2]["at"], "2026-01-05T02:05:00Z");
    assert_eq!(result["pagination"]["hasMore"], true);
    assert_eq!(result["pagination"]["nextBefore"], candles[0]["at"]);
    assert_eq!(reader.requests.lock().unwrap().len(), 2);
    assert_eq!(reader.current_calls.load(Ordering::SeqCst), 1);
}

#[tokio::test]
async fn candle_route_excludes_before_boundary_and_can_continue_loading_older_pages() {
    let reader = Arc::new(PagedHistory::default());
    let port = port(reader.clone());
    let result = port
        .read(
            "/api/v1/market-data/candles/HK/00700",
            "period=1m&limit=2&before=2026-01-05T02:04:00Z",
        )
        .await
        .unwrap();
    assert_eq!(result["candles"][0]["at"], "2026-01-05T02:02:00Z");
    assert_eq!(result["candles"][1]["at"], "2026-01-05T02:03:00Z");
    assert_eq!(result["pagination"]["hasMore"], true);
    let result = port
        .read(
            "/api/v1/market-data/candles/HK/00700",
            "period=1m&limit=2&before=2026-01-05T02:02:00Z",
        )
        .await
        .unwrap();
    assert_eq!(result["candles"][0]["at"], "2026-01-05T02:00:00Z");
    assert_eq!(result["candles"][1]["at"], "2026-01-05T02:01:00Z");
    assert_eq!(result["pagination"]["hasMore"], false);
    assert_eq!(reader.current_calls.load(Ordering::SeqCst), 0);
}

#[tokio::test]
async fn candle_route_rejects_partial_history_before_querying_current_bars() {
    let reader = Arc::new(PagedHistory {
        fail_second_page: true,
        ..Default::default()
    });
    assert!(
        port(reader.clone())
            .read("/api/v1/market-data/candles/HK/00700", "period=1m&limit=3")
            .await
            .is_err()
    );
    assert_eq!(reader.current_calls.load(Ordering::SeqCst), 0);
}
