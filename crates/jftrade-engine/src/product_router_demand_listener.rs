use std::sync::{Arc, Mutex};
use std::time::{SystemTime, UNIX_EPOCH};

#[derive(Debug)]
pub(crate) struct RouterDemandListener {
    pub(crate) router: Arc<Mutex<jftrade_marketdata::ProviderRouter>>,
}

impl jftrade_api::LiveDemandListener for RouterDemandListener {
    fn on_subscription_change(
        &self,
        connection_id: u64,
        provider_broker_id: &str,
        instruments: &[String],
    ) {
        self.replace_subscription(connection_id, provider_broker_id, instruments, &[], &[]);
    }

    fn on_subscription_snapshot(
        &self,
        connection_id: u64,
        snapshot: &jftrade_api::LiveSubscriptionSnapshot,
    ) {
        self.replace_subscription(
            connection_id,
            &snapshot.provider_broker_id,
            &snapshot.active_instruments,
            &snapshot.security_details,
            &snapshot.depth,
        );
    }

    fn on_disconnect(&self, connection_id: u64) {
        let consumer_id = format!("ws-client-{connection_id}");
        let mut router = self.router.lock().unwrap_or_else(|e| e.into_inner());
        router.release_demand_consumer(&consumer_id);
    }
}

impl RouterDemandListener {
    fn replace_subscription(
        &self,
        connection_id: u64,
        provider_broker_id: &str,
        instruments: &[String],
        security_details: &[jftrade_api::LiveSecuritySubscription],
        depth: &[jftrade_api::LiveDepthSubscription],
    ) {
        let consumer_id = format!("ws-client-{connection_id}");
        let mut router = self.router.lock().unwrap_or_else(|e| e.into_inner());
        let provider_id = provider_broker_id.trim();
        let active_provider = router.runtime().active_provider;
        if !active_provider.is_empty()
            && !provider_id.is_empty()
            && !active_provider.eq_ignore_ascii_case(provider_id)
        {
            router.release_demand_consumer(&consumer_id);
            return;
        }

        let mut refs = instruments
            .iter()
            .map(|instrument| {
                let upper = instrument.trim().to_ascii_uppercase();
                let (market, symbol) = upper.split_once('.').map_or_else(
                    || ("US".to_owned(), upper.clone()),
                    |(market, symbol)| (market.to_owned(), symbol.to_owned()),
                );
                jftrade_marketdata::InstrumentRef {
                    channel: "SNAPSHOT".to_owned(),
                    market,
                    symbol,
                    interval: None,
                }
            })
            .collect::<Vec<_>>();
        refs.extend(
            security_details
                .iter()
                .map(|item| jftrade_marketdata::InstrumentRef {
                    channel: "SNAPSHOT".to_owned(),
                    market: item.market.clone(),
                    symbol: item.symbol.clone(),
                    interval: None,
                }),
        );
        refs.extend(depth.iter().map(|item| jftrade_marketdata::InstrumentRef {
            channel: "ORDER_BOOK".to_owned(),
            market: item.market.clone(),
            symbol: item.symbol.clone(),
            interval: None,
        }));

        if refs.is_empty() {
            router.release_demand_consumer(&consumer_id);
            return;
        }
        let now_ms = SystemTime::now()
            .duration_since(UNIX_EPOCH)
            .ok()
            .and_then(|duration| i64::try_from(duration.as_millis()).ok())
            .unwrap_or_default();
        let _ = router.replace_demand(&consumer_id, refs, false, now_ms);
    }
}
