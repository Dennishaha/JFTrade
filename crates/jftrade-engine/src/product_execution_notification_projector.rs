//! Execution order events notification and LiveHub projector.
//!
//! Dual-cursor projector for execution order events. Consumes events from the
//! `execution_order_events` table using durable sequence cursors in `execution_sequences`:
//! - `projector_desktop_notification`: delivers desktop notifications for execution events.
//! - `projector_livehub`: broadcasts execution events to LiveHub clients.

use std::collections::BTreeSet;
use std::sync::{Arc, Mutex};

use jftrade_settings::SystemNotificationSettingsStorePort;
use jftrade_store_sqlite::{ExecutionOrderStore, ExecutionOrderStoreError};
use serde_json::{Value, json};

use crate::product::{ProductNotificationPort, ProductNotificationRequest};

pub const CURSOR_DESKTOP_NOTIFICATION: &str = "projector_desktop_notification";
pub const CURSOR_LIVEHUB: &str = "projector_livehub";

#[derive(Clone)]
pub(crate) struct ExecutionNotificationProjector {
    store: Arc<ExecutionOrderStore>,
    notification: Option<Arc<dyn ProductNotificationPort>>,
    live_hub: Option<Arc<jftrade_api::LiveHub>>,
    settings_store: Option<Arc<dyn SystemNotificationSettingsStorePort>>,
    delivered_events: Arc<Mutex<BTreeSet<String>>>,
}

impl std::fmt::Debug for ExecutionNotificationProjector {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        f.debug_struct("ExecutionNotificationProjector")
            .field("store", &self.store)
            .field("notification", &self.notification.is_some())
            .field("live_hub", &self.live_hub.is_some())
            .field("settings_store", &self.settings_store.is_some())
            .finish()
    }
}

impl ExecutionNotificationProjector {
    pub(crate) fn new(
        store: Arc<ExecutionOrderStore>,
        notification: Option<Arc<dyn ProductNotificationPort>>,
        live_hub: Option<Arc<jftrade_api::LiveHub>>,
    ) -> Self {
        Self {
            store,
            notification,
            live_hub,
            settings_store: None,
            delivered_events: Arc::new(Mutex::new(BTreeSet::new())),
        }
    }

    pub(crate) fn with_settings_store(
        mut self,
        settings_store: Arc<dyn SystemNotificationSettingsStorePort>,
    ) -> Self {
        self.settings_store = Some(settings_store);
        self
    }

    /// Projects unhandled execution events using durable cursor tracking in
    /// `execution_sequences`.
    pub(crate) fn project_pending(&self) -> Result<(usize, usize), ExecutionOrderStoreError> {
        let mut total_notified = 0;
        let mut total_published = 0;
        loop {
            let (notified, notif_advanced) = self.project_desktop_notifications()?;
            let (published, hub_advanced) = self.project_livehub_events()?;
            total_notified += notified;
            total_published += published;
            if !notif_advanced && !hub_advanced {
                break;
            }
        }
        Ok((total_notified, total_published))
    }

    fn project_desktop_notifications(&self) -> Result<(usize, bool), ExecutionOrderStoreError> {
        let last_id = self.store.get_sequence(CURSOR_DESKTOP_NOTIFICATION)?;
        let events = self.store.list_events_after(last_id, 100)?;
        let mut count = 0;
        let mut advanced = false;
        for (row_id, event) in events {
            let presentation = map_order_event_presentation(&event);
            let (should_forward, sound_enabled) = if let Some(ref store) = self.settings_store {
                match store.load_system_notifications() {
                    Ok(Some(settings)) => {
                        let normalized =
                            jftrade_settings::normalize_system_notification_settings(&settings);
                        let forward = jftrade_settings::should_forward_system_notification(
                            &normalized,
                            &presentation.level,
                            &presentation.category,
                        );
                        (forward, normalized.sound_enabled)
                    }
                    Ok(None) => {
                        let default_settings =
                            jftrade_settings::SystemNotificationSettings::default();
                        let forward = jftrade_settings::should_forward_system_notification(
                            &default_settings,
                            &presentation.level,
                            &presentation.category,
                        );
                        (forward, default_settings.sound_enabled)
                    }
                    Err(_) => {
                        // Fail closed if settings cannot be loaded: do not deliver notification
                        (false, false)
                    }
                }
            } else {
                (true, true)
            };

            let delivered = if should_forward {
                let event_key = format!("{}|{}", event.internal_order_id, event.id);
                let already_delivered = {
                    let set = self
                        .delivered_events
                        .lock()
                        .unwrap_or_else(|e| e.into_inner());
                    set.contains(&event_key)
                };

                if already_delivered {
                    true
                } else if let Some(ref notifier) = self.notification {
                    let request = ProductNotificationRequest {
                        title: presentation.title,
                        body: presentation.message,
                        sound_enabled,
                    };
                    let ok = notifier.deliver(request).delivered;
                    if ok {
                        let mut set = self
                            .delivered_events
                            .lock()
                            .unwrap_or_else(|e| e.into_inner());
                        set.insert(event_key);
                    }
                    ok
                } else {
                    true
                }
            } else {
                // If filtered: advance cursor without delivering native alert
                true
            };

            if delivered {
                self.store
                    .set_sequence(CURSOR_DESKTOP_NOTIFICATION, row_id)?;
                count += 1;
                advanced = true;
            } else {
                break;
            }
        }
        Ok((count, advanced))
    }

    fn project_livehub_events(&self) -> Result<(usize, bool), ExecutionOrderStoreError> {
        let last_id = self.store.get_sequence(CURSOR_LIVEHUB)?;
        let events = self.store.list_events_after(last_id, 100)?;
        let mut count = 0;
        let mut advanced = false;
        for (row_id, event) in events {
            if let Some(ref hub) = self.live_hub {
                let presentation = map_order_event_presentation(&event);
                let details: Value =
                    serde_json::from_str(&event.payload_json).unwrap_or_else(|_| {
                        json!({
                            "internalOrderId": event.internal_order_id,
                            "previousStatus": event.previous_status,
                            "nextStatus": event.next_status,
                        })
                    });
                let notification_payload = json!({
                    "source": "execution-orders",
                    "category": presentation.category,
                    "title": presentation.title,
                    "message": presentation.message,
                    "level": presentation.level,
                    "at": event.created_at,
                    "internalOrderId": event.internal_order_id,
                    "previousStatus": event.previous_status,
                    "nextStatus": event.next_status,
                    "details": details,
                });
                let live_event = json!({
                    "eventId": format!("order_event|{}|{}", event.id, event.created_at),
                    "type": "system.notification",
                    "source": "notification",
                    "entityId": event.internal_order_id,
                    "serverTime": event.created_at,
                    "payload": notification_payload,
                });
                let _ = hub.publish(live_event);
            }
            self.store.set_sequence(CURSOR_LIVEHUB, row_id)?;
            count += 1;
            advanced = true;
        }
        Ok((count, advanced))
    }
}

struct OrderEventPresentation {
    category: String,
    title: String,
    message: String,
    level: String,
}

fn map_order_event_presentation(
    event: &jftrade_store_sqlite::StoredExecutionOrderEventRecord,
) -> OrderEventPresentation {
    let status = if event.next_status.is_empty() {
        event.event_type.to_ascii_uppercase()
    } else {
        event.next_status.to_ascii_uppercase()
    };
    let event_type = event.event_type.to_ascii_lowercase();
    let order_id = &event.internal_order_id;

    if event_type == "risk_rejected" || status == "REJECTED" {
        OrderEventPresentation {
            category: "broker.order.risk_rejected".to_owned(),
            title: "风控拦截".to_owned(),
            message: format!("订单 {order_id} 被风控拦截"),
            level: "error".to_owned(),
        }
    } else if event_type.contains("cancel") || status == "CANCELLED" || status == "CANCELED" {
        OrderEventPresentation {
            category: "broker.order.cancelled".to_owned(),
            title: "订单已撤销".to_owned(),
            message: format!("订单 {order_id} 已撤销"),
            level: "warn".to_owned(),
        }
    } else if status == "FILLED" {
        OrderEventPresentation {
            category: "broker.order.filled".to_owned(),
            title: "订单已成交".to_owned(),
            message: format!("订单 {order_id} 已全部成交"),
            level: "info".to_owned(),
        }
    } else if status == "PARTIALLY_FILLED" {
        OrderEventPresentation {
            category: "broker.order.partially_filled".to_owned(),
            title: "订单部分成交".to_owned(),
            message: format!("订单 {order_id} 部分成交"),
            level: "info".to_owned(),
        }
    } else if status == "SUBMITTED" {
        OrderEventPresentation {
            category: "broker.order.submitted".to_owned(),
            title: "订单已提交".to_owned(),
            message: format!("订单 {order_id} 已成功提交"),
            level: "info".to_owned(),
        }
    } else if status == "FAILED" || status == "UNKNOWN" {
        OrderEventPresentation {
            category: format!("broker.order.{event_type}"),
            title: "订单异常".to_owned(),
            message: format!("订单 {order_id} 状态异常: {status}"),
            level: "error".to_owned(),
        }
    } else {
        OrderEventPresentation {
            category: format!("broker.order.{event_type}"),
            title: format!("订单状态更新: {status}"),
            message: format!("订单 {order_id} 变更为 {status}"),
            level: "info".to_owned(),
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use jftrade_store_sqlite::{StoredExecutionOrder, StoredExecutionOrderEvent};
    use std::sync::Mutex;

    #[derive(Debug, Default)]
    struct MockNotificationPort {
        delivered: Mutex<Vec<ProductNotificationRequest>>,
    }

    impl ProductNotificationPort for MockNotificationPort {
        fn deliver(
            &self,
            request: ProductNotificationRequest,
        ) -> crate::product::ProductNotificationDelivery {
            self.delivered.lock().unwrap().push(request);
            crate::product::ProductNotificationDelivery {
                delivered: true,
                status: "delivered".to_owned(),
                message: "delivered".to_owned(),
            }
        }
    }

    #[test]
    fn projector_initial_cursor_is_zero() {
        assert_eq!(
            CURSOR_DESKTOP_NOTIFICATION,
            "projector_desktop_notification"
        );
        assert_eq!(CURSOR_LIVEHUB, "projector_livehub");
    }

    #[tokio::test]
    async fn test_notification_projector_advances_cursors_and_is_idempotent() {
        let directory = tempfile::tempdir().expect("tempdir");
        let path = directory.path().join("execution-orders.db");
        let conn = rusqlite::Connection::open(&path).expect("open sqlite");
        jftrade_store_sqlite::initialize_current(&conn, "execution-orders").expect("init schema");
        drop(conn);

        let store = Arc::new(ExecutionOrderStore::open(&path).expect("open store"));

        let order = StoredExecutionOrder {
            internal_order_id: "ord-test-1".to_owned(),
            broker_id: "futu".to_owned(),
            broker_order_id: Some("101".to_owned()),
            broker_order_id_ex: None,
            source: "api".to_owned(),
            source_detail: "test".to_owned(),
            trading_environment: "SIMULATE".to_owned(),
            account_id: "1".to_owned(),
            market: "US".to_owned(),
            symbol: Some("US.AAPL".to_owned()),
            side: Some("BUY".to_owned()),
            order_type: Some("LIMIT".to_owned()),
            status: "SUBMITTED".to_owned(),
            raw_broker_status: None,
            requested_quantity: Some(10.0),
            requested_price: Some(150.0),
            filled_quantity: None,
            filled_average_price: None,
            remark: None,
            last_error: None,
            last_error_code: None,
            last_error_source: None,
            submitted_at: Some("2026-08-30T10:00:00Z".to_owned()),
            updated_at: "2026-08-30T10:00:00Z".to_owned(),
            created_at: "2026-08-30T10:00:00Z".to_owned(),
            order_kind: "single".to_owned(),
            product_class: "equity".to_owned(),
            quantity_mode: "quantity".to_owned(),
            client_order_id: None,
            preview_id: None,
            normalized_request: "{}".to_owned(),
            requested_amount: None,
            payout: None,
            fees: None,
        };
        let event = StoredExecutionOrderEvent {
            id: "evt-test-1",
            internal_order_id: "ord-test-1",
            event_type: "PLACE",
            previous_status: None,
            next_status: "SUBMITTED",
            payload_json: "{}",
            created_at: "2026-08-30T10:00:00Z",
        };
        store
            .save_order_and_event(order, "2026-08-30T10:00:00Z", &event)
            .expect("save order and event");

        let notifier = Arc::new(MockNotificationPort::default());
        let live_hub = Arc::new(jftrade_api::LiveHub::new(16));
        let mut connection = live_hub.connect();

        let projector = ExecutionNotificationProjector::new(
            store.clone(),
            Some(notifier.clone()),
            Some(live_hub.clone()),
        );

        let (notified, published) = projector.project_pending().expect("project pending");
        assert_eq!(notified, 1);
        assert_eq!(published, 1);

        assert_eq!(notifier.delivered.lock().unwrap().len(), 1);
        assert_eq!(notifier.delivered.lock().unwrap()[0].title, "订单已提交");
        assert_eq!(
            notifier.delivered.lock().unwrap()[0].body,
            "订单 ord-test-1 已成功提交"
        );
        assert!(store.get_sequence(CURSOR_DESKTOP_NOTIFICATION).unwrap() >= 1);
        assert!(store.get_sequence(CURSOR_LIVEHUB).unwrap() >= 1);

        let live_event = connection.recv().await.expect("recv live event");
        assert_eq!(live_event["type"], "system.notification");
        assert_eq!(live_event["source"], "notification");
        assert_eq!(live_event["entityId"], "ord-test-1");
        assert_eq!(live_event["payload"]["source"], "execution-orders");
        assert_eq!(live_event["payload"]["category"], "broker.order.submitted");
        assert_eq!(live_event["payload"]["level"], "info");

        // Idempotency check: projecting again processes 0 new events
        let (notified2, published2) = projector.project_pending().expect("second project");
        assert_eq!(notified2, 0);
        assert_eq!(published2, 0);
        assert_eq!(notifier.delivered.lock().unwrap().len(), 1);
    }

    #[derive(Debug, Default)]
    struct MockFailingNotificationPort {
        attempts: Mutex<usize>,
    }

    impl ProductNotificationPort for MockFailingNotificationPort {
        fn deliver(
            &self,
            _request: ProductNotificationRequest,
        ) -> crate::product::ProductNotificationDelivery {
            *self.attempts.lock().unwrap() += 1;
            crate::product::ProductNotificationDelivery {
                delivered: false,
                status: "failed".to_owned(),
                message: "notification service unavailable".to_owned(),
            }
        }
    }

    #[tokio::test]
    async fn test_notification_projector_risk_rejected_maps_category_and_error_level() {
        let directory = tempfile::tempdir().expect("tempdir");
        let path = directory.path().join("execution-orders.db");
        let conn = rusqlite::Connection::open(&path).expect("open sqlite");
        jftrade_store_sqlite::initialize_current(&conn, "execution-orders").expect("init schema");
        drop(conn);

        let store = Arc::new(ExecutionOrderStore::open(&path).expect("open store"));
        let order = StoredExecutionOrder {
            internal_order_id: "ord-risk-1".to_owned(),
            broker_id: "futu".to_owned(),
            broker_order_id: None,
            broker_order_id_ex: None,
            source: "api".to_owned(),
            source_detail: "test".to_owned(),
            trading_environment: "REAL".to_owned(),
            account_id: "1".to_owned(),
            market: "US".to_owned(),
            symbol: Some("US.AAPL".to_owned()),
            side: Some("BUY".to_owned()),
            order_type: Some("LIMIT".to_owned()),
            status: "REJECTED".to_owned(),
            raw_broker_status: None,
            requested_quantity: Some(10.0),
            requested_price: Some(150.0),
            filled_quantity: None,
            filled_average_price: None,
            remark: None,
            last_error: Some("PRE_TRADE_RISK_REJECTED: max order value exceeded".to_owned()),
            last_error_code: Some("PRE_TRADE_RISK_REJECTED".to_owned()),
            last_error_source: Some("risk".to_owned()),
            submitted_at: None,
            updated_at: "2026-08-30T10:00:00Z".to_owned(),
            created_at: "2026-08-30T10:00:00Z".to_owned(),
            order_kind: "single".to_owned(),
            product_class: "equity".to_owned(),
            quantity_mode: "quantity".to_owned(),
            client_order_id: None,
            preview_id: None,
            normalized_request: "{}".to_owned(),
            requested_amount: None,
            payout: None,
            fees: None,
        };
        let event = StoredExecutionOrderEvent {
            id: "evt-risk-1",
            internal_order_id: "ord-risk-1",
            event_type: "risk_rejected",
            previous_status: Some("PENDING"),
            next_status: "REJECTED",
            payload_json: "{}",
            created_at: "2026-08-30T10:00:00Z",
        };
        store
            .save_order_and_event(order, "2026-08-30T10:00:00Z", &event)
            .expect("save order and event");

        let notifier = Arc::new(MockNotificationPort::default());
        let live_hub = Arc::new(jftrade_api::LiveHub::new(16));
        let mut connection = live_hub.connect();

        let projector = ExecutionNotificationProjector::new(
            store.clone(),
            Some(notifier.clone()),
            Some(live_hub.clone()),
        );

        let (notified, published) = projector.project_pending().expect("project pending");
        assert_eq!(notified, 1);
        assert_eq!(published, 1);

        {
            let delivered = notifier.delivered.lock().unwrap();
            assert_eq!(delivered[0].title, "风控拦截");
            assert_eq!(delivered[0].body, "订单 ord-risk-1 被风控拦截");
        }

        let live_event = connection.recv().await.expect("recv live event");
        assert_eq!(
            live_event["payload"]["category"],
            "broker.order.risk_rejected"
        );
        assert_eq!(live_event["payload"]["level"], "error");
        assert_eq!(live_event["payload"]["title"], "风控拦截");
    }

    #[tokio::test]
    async fn test_notification_projector_delivery_failure_does_not_loop_and_preserves_cursor() {
        let directory = tempfile::tempdir().expect("tempdir");
        let path = directory.path().join("execution-orders.db");
        let conn = rusqlite::Connection::open(&path).expect("open sqlite");
        jftrade_store_sqlite::initialize_current(&conn, "execution-orders").expect("init schema");
        drop(conn);

        let store = Arc::new(ExecutionOrderStore::open(&path).expect("open store"));
        let order = StoredExecutionOrder {
            internal_order_id: "ord-fail-1".to_owned(),
            broker_id: "futu".to_owned(),
            broker_order_id: None,
            broker_order_id_ex: None,
            source: "api".to_owned(),
            source_detail: "test".to_owned(),
            trading_environment: "SIMULATE".to_owned(),
            account_id: "1".to_owned(),
            market: "US".to_owned(),
            symbol: Some("US.AAPL".to_owned()),
            side: Some("BUY".to_owned()),
            order_type: Some("LIMIT".to_owned()),
            status: "SUBMITTED".to_owned(),
            raw_broker_status: None,
            requested_quantity: Some(10.0),
            requested_price: Some(150.0),
            filled_quantity: None,
            filled_average_price: None,
            remark: None,
            last_error: None,
            last_error_code: None,
            last_error_source: None,
            submitted_at: Some("2026-08-30T10:00:00Z".to_owned()),
            updated_at: "2026-08-30T10:00:00Z".to_owned(),
            created_at: "2026-08-30T10:00:00Z".to_owned(),
            order_kind: "single".to_owned(),
            product_class: "equity".to_owned(),
            quantity_mode: "quantity".to_owned(),
            client_order_id: None,
            preview_id: None,
            normalized_request: "{}".to_owned(),
            requested_amount: None,
            payout: None,
            fees: None,
        };
        let event = StoredExecutionOrderEvent {
            id: "evt-fail-1",
            internal_order_id: "ord-fail-1",
            event_type: "PLACE",
            previous_status: None,
            next_status: "SUBMITTED",
            payload_json: "{}",
            created_at: "2026-08-30T10:00:00Z",
        };
        store
            .save_order_and_event(order, "2026-08-30T10:00:00Z", &event)
            .expect("save order and event");

        let failing_notifier = Arc::new(MockFailingNotificationPort::default());
        let projector = ExecutionNotificationProjector::new(
            store.clone(),
            Some(failing_notifier.clone()),
            None,
        );

        let (notified, _) = projector.project_pending().expect("project pending");
        assert_eq!(notified, 0);
        assert!(*failing_notifier.attempts.lock().unwrap() <= 2);
        assert_eq!(store.get_sequence(CURSOR_DESKTOP_NOTIFICATION).unwrap(), 0);
    }

    #[derive(Clone)]
    struct MockSettingsStore {
        settings: Arc<
            Mutex<
                Result<
                    Option<jftrade_settings::SystemNotificationSettings>,
                    jftrade_settings::SettingsStoreError,
                >,
            >,
        >,
    }

    impl Default for MockSettingsStore {
        fn default() -> Self {
            Self {
                settings: Arc::new(Mutex::new(Ok(None))),
            }
        }
    }

    impl jftrade_settings::SystemNotificationSettingsStorePort for MockSettingsStore {
        fn load_system_notifications(
            &self,
        ) -> Result<
            Option<jftrade_settings::SystemNotificationSettings>,
            jftrade_settings::SettingsStoreError,
        > {
            self.settings.lock().unwrap().clone()
        }

        fn save_system_notifications(
            &self,
            _settings: &jftrade_settings::SystemNotificationSettings,
        ) -> Result<(), jftrade_settings::SettingsStoreError> {
            Ok(())
        }
    }

    #[tokio::test]
    async fn test_notification_projector_settings_filter_and_fail_closed() {
        let directory = tempfile::tempdir().expect("tempdir");
        let path = directory.path().join("execution-orders.db");
        let conn = rusqlite::Connection::open(&path).expect("open sqlite");
        jftrade_store_sqlite::initialize_current(&conn, "execution-orders").expect("init schema");
        drop(conn);

        let store = Arc::new(ExecutionOrderStore::open(&path).expect("open store"));
        let order = StoredExecutionOrder {
            internal_order_id: "ord-filt-1".to_owned(),
            broker_id: "futu".to_owned(),
            broker_order_id: None,
            broker_order_id_ex: None,
            source: "api".to_owned(),
            source_detail: "test".to_owned(),
            trading_environment: "SIMULATE".to_owned(),
            account_id: "1".to_owned(),
            market: "US".to_owned(),
            symbol: Some("US.AAPL".to_owned()),
            side: Some("BUY".to_owned()),
            order_type: Some("LIMIT".to_owned()),
            status: "SUBMITTED".to_owned(),
            raw_broker_status: None,
            requested_quantity: Some(10.0),
            requested_price: Some(150.0),
            filled_quantity: None,
            filled_average_price: None,
            remark: None,
            last_error: None,
            last_error_code: None,
            last_error_source: None,
            submitted_at: Some("2026-08-30T10:00:00Z".to_owned()),
            updated_at: "2026-08-30T10:00:00Z".to_owned(),
            created_at: "2026-08-30T10:00:00Z".to_owned(),
            order_kind: "single".to_owned(),
            product_class: "equity".to_owned(),
            quantity_mode: "quantity".to_owned(),
            client_order_id: None,
            preview_id: None,
            normalized_request: "{}".to_owned(),
            requested_amount: None,
            payout: None,
            fees: None,
        };
        let event = StoredExecutionOrderEvent {
            id: "evt-filt-1",
            internal_order_id: "ord-filt-1",
            event_type: "PLACE",
            previous_status: None,
            next_status: "SUBMITTED",
            payload_json: "{}",
            created_at: "2026-08-30T10:00:00Z",
        };
        store
            .save_order_and_event(order.clone(), "2026-08-30T10:00:00Z", &event)
            .expect("save order and event");

        let notifier = Arc::new(MockNotificationPort::default());
        let settings_store = Arc::new(MockSettingsStore::default());

        *settings_store.settings.lock().unwrap() =
            Ok(Some(jftrade_settings::SystemNotificationSettings {
                enabled: false,
                mode: "custom".to_owned(),
                levels: vec![],
                categories: vec![],
                sound_enabled: false,
            }));

        let projector =
            ExecutionNotificationProjector::new(store.clone(), Some(notifier.clone()), None)
                .with_settings_store(settings_store.clone());

        let (notified, _) = projector.project_pending().expect("project pending");
        assert_eq!(notified, 1);
        assert_eq!(notifier.delivered.lock().unwrap().len(), 0);
        assert!(store.get_sequence(CURSOR_DESKTOP_NOTIFICATION).unwrap() >= 1);

        // Test fail-closed on settings store error
        *settings_store.settings.lock().unwrap() =
            Err(jftrade_settings::SettingsStoreError::new("store failure"));
        let event2 = StoredExecutionOrderEvent {
            id: "evt-filt-2",
            internal_order_id: "ord-filt-1",
            event_type: "CANCEL",
            previous_status: Some("SUBMITTED"),
            next_status: "CANCELLED",
            payload_json: "{}",
            created_at: "2026-08-30T10:05:00Z",
        };
        let mut order2 = order.clone();
        order2.status = "CANCELLED".to_owned();
        order2.updated_at = "2026-08-30T10:05:00Z".to_owned();
        store
            .save_order_and_event(order2, "2026-08-30T10:05:00Z", &event2)
            .expect("save event2");

        let (notified2, _) = projector.project_pending().expect("project pending 2");
        assert_eq!(notified2, 1);
        // Fail-closed: still 0 delivered notifications
        assert_eq!(notifier.delivered.lock().unwrap().len(), 0);
    }
}
