#[cfg(test)]
mod product_production_assembly_tests {
    use std::collections::HashSet;
    use std::fs;
    use std::net::SocketAddr;
    use std::path::PathBuf;
    use std::sync::Arc;

    use jftrade_api::AccessPolicy;
    use jftrade_datamanagement::{DATABASE_ADK, DATABASE_WATCHLIST};
    use serde_json::{Value, json};
    use tempfile::TempDir;

    use crate::product::product_adk_chat_stream_port::{
        AdkChatInput, AdkChatPortError, AdkChatRoute,
    };
    use crate::product::product_adk_mutation_port::{AdkMutationInput, AdkMutationOperation};
    use crate::product::product_alerts_write_port::{
        AlertWriteAction, AlertWritePortError, AlertWriteResolution,
    };
    use crate::product::product_research_preset_write_port::ResearchPresetWriteMutation;
    use crate::product::product_strategy_definition_write_port::{
        StrategyDefinitionWriteInput, StrategyDefinitionWriteOperation,
    };
    use crate::product::product_strategy_runtime_write_port::{
        StrategyRuntimeWriteInput, StrategyRuntimeWriteOperation,
    };
    use crate::product::product_watchlist_write_port::WatchlistWriteMutation;
    use crate::product::tests::request_json_with_status;
    use crate::product::{
        ProductCapabilities, ProductConfig, ProductError, WatchlistReadSnapshotError,
        product_data_management,
        product_production_ports::{ProductionAdapterBinding, production_ports},
        start_product,
    };
    use jftrade_settings::SecuritySettingsService;
    use jftrade_store_settings_file::SettingsFileStore;
    use jftrade_store_sqlite::{ADK_PRODUCTION_PROFILE, AdkStore, CreateAdkRunParams};

    fn setup_test_env() -> (TempDir, PathBuf, ProductConfig, SecuritySettingsService) {
        let temp_dir = TempDir::new().expect("temp dir");
        let settings_path = temp_dir.path().join("settings.json");
        fs::write(&settings_path, b"{}").expect("write settings");

        product_data_management::initialize_production_databases(&settings_path)
            .expect("init databases");

        let store = Arc::new(SettingsFileStore::open(&settings_path).expect("open settings store"));
        let security = SecuritySettingsService::new(store);

        let bind_address: SocketAddr = "127.0.0.1:0".parse().unwrap();
        let token = "a".repeat(32);
        let mut config = ProductConfig::new(
            bind_address,
            &settings_path,
            AccessPolicy::desktop(Some(token)),
        )
        .expect("product config");
        config.capabilities = ProductCapabilities::all();
        config.production = true;

        (temp_dir, settings_path, config, security)
    }

    #[tokio::test]
    async fn production_startup_initializes_and_reports_ready_and_acquired() {
        let (_temp_dir, _settings_path, config, _security) = setup_test_env();

        let handle = start_product(config).await.expect("start product");
        let record = handle.startup_record();

        assert_eq!(record.event, "ready");
        assert_eq!(record.owner, "rust");
        assert_eq!(record.owned_routes, 278);
        assert_eq!(record.ready_routes, 183);
        assert_eq!(record.external_unavailable_routes, 95);
        assert_eq!(
            record.ready_routes + record.external_unavailable_routes,
            record.owned_routes
        );
        assert_eq!(record.route_profile, "production.v1");
        assert_eq!(
            record.route_profile_digest,
            "afa112435ed280dd24d43bb4acaa0f7ca2ab45c01e4e5701efc5ce149e5b85b2"
        );
        assert_eq!(record.runtime_readiness, "degraded");
        assert_eq!(record.database_lease_status, "acquired");
        assert_eq!(record.provider_status, "unavailable");
        assert_eq!(record.opend_status, "unavailable");
        assert_eq!(record.worker_status, "unavailable");
        // The websocket status now derives from the real live-hub lifecycle:
        // the hub is always composed and reported as serving once the HTTP
        // listener is exposed.
        assert_eq!(record.websocket_status, "serving");
        assert!(!record.capabilities.is_empty());

        handle.shutdown().await.expect("shutdown cleanly");
    }

    #[tokio::test]
    async fn production_startup_fails_closed_when_database_is_corrupted() {
        let (_temp_dir, settings_path, config, _security) = setup_test_env();

        let descriptors =
            product_data_management::managed_database_runtime_descriptors(&settings_path);
        let watchlist_desc = descriptors
            .iter()
            .find(|d| d.id == DATABASE_WATCHLIST)
            .expect("watchlist descriptor");

        // Corrupt the database file with invalid header bytes
        fs::write(&watchlist_desc.path, b"NOT_A_VALID_SQLITE_HEADER").expect("corrupt database");

        let result = start_product(config).await;
        match result {
            Ok(_) => panic!("startup must fail closed on corrupted DB"),
            Err(ProductError::Storage(msg)) => {
                assert!(
                    msg.contains("failed to open watchlist production store")
                        || msg.contains("database")
                );
            }
            Err(other) => panic!("expected ProductError::Storage, got {other:?}"),
        }

        // Verify corrupted file was not deleted or replaced
        let bytes = fs::read(&watchlist_desc.path).expect("read file");
        assert_eq!(bytes, b"NOT_A_VALID_SQLITE_HEADER");
    }

    #[tokio::test]
    async fn production_startup_fails_closed_on_writer_lease_conflict() {
        let (_temp_dir, settings_path, config, _security) = setup_test_env();

        let descriptors =
            product_data_management::managed_database_runtime_descriptors(&settings_path);
        let watchlist_desc = descriptors
            .iter()
            .find(|d| d.id == DATABASE_WATCHLIST)
            .expect("watchlist descriptor");

        // Acquire a conflicting writer lease by opening the store in advance
        let _held_store = jftrade_store_sqlite::WatchlistStore::open_existing(
            &watchlist_desc.path,
            jftrade_store_sqlite::WATCHLIST_PRODUCTION_PROFILE,
        )
        .expect("open existing watchlist store holding lease");

        let result = start_product(config).await;
        match result {
            Ok(_) => panic!("startup must fail closed on lease conflict"),
            Err(ProductError::Storage(msg)) => {
                assert!(
                    msg.contains("failed to open watchlist production store")
                        || msg.contains("conflict")
                        || msg.contains("lock")
                        || msg.contains("writer lease")
                );
            }
            Err(other) => panic!("expected ProductError::Storage, got {other:?}"),
        }
    }

    #[tokio::test]
    async fn production_startup_fails_closed_on_backtest_market_data_lease_conflict() {
        let (_temp_dir, settings_path, config, _security) = setup_test_env();
        let descriptors =
            product_data_management::managed_database_runtime_descriptors(&settings_path);
        let backtest_desc = descriptors
            .iter()
            .find(|descriptor| descriptor.id == jftrade_datamanagement::DATABASE_BACKTEST)
            .expect("backtest market-data descriptor");
        let _held_store = jftrade_store_sqlite::BacktestMarketDataStore::open_existing(
            &backtest_desc.path,
            jftrade_store_sqlite::BACKTEST_MARKET_DATA_PRODUCTION_PROFILE,
        )
        .expect("open backtest market-data store holding lease");

        let result = start_product(config).await;
        match result {
            Ok(_) => panic!("startup must fail closed on backtest market-data lease conflict"),
            Err(ProductError::Storage(message)) => assert!(
                message.contains("backtest market-data")
                    || message.contains("writer lease")
                    || message.contains("lock")
            ),
            Err(other) => panic!("expected ProductError::Storage, got {other:?}"),
        }
    }

    #[test]
    fn production_watchlist_port_crud_operations() {
        let (_temp_dir, _settings_path, config, security) = setup_test_env();
        let ports = production_ports(&config, &security).expect("production ports");
        let watchlist = ports.watchlist_write;

        // Create group
        let create_res = watchlist
            .mutate(&WatchlistWriteMutation {
                value: json!({
                    "route": "create-group",
                    "name": "US Tech",
                }),
            })
            .expect("create group");
        assert_eq!(create_res["name"], "US Tech");
        let group_id = create_res["groupId"].as_str().expect("groupId");
        let revision = create_res["revision"].as_i64().expect("revision");

        let stale_update = watchlist
            .mutate(&WatchlistWriteMutation {
                value: json!({
                    "route": "update-group",
                    "groupId": group_id,
                    "name": "stale update",
                    "expectedRevision": revision - 1,
                }),
            })
            .expect_err("stale group update must conflict");
        assert_eq!(stale_update.status, 409);
        assert_eq!(stale_update.code, "WATCHLIST_CONFLICT");

        let missing_delete = watchlist
            .mutate(&WatchlistWriteMutation {
                value: json!({
                    "route": "delete-group",
                    "groupId": "missing-group",
                }),
            })
            .expect_err("missing group delete must return not found");
        assert_eq!(missing_delete.status, 404);
        assert_eq!(missing_delete.code, "WATCHLIST_NOT_FOUND");

        let malformed_memberships = watchlist
            .mutate(&WatchlistWriteMutation {
                value: json!({
                    "route": "replace-memberships",
                    "instrumentId": "HK.00001",
                    "groupIds": [42],
                    "newGroupNames": [],
                    "expectedRevision": 0,
                }),
            })
            .expect_err("malformed membership ids must be rejected");
        assert_eq!(malformed_memberships.status, 400);
        assert_eq!(malformed_memberships.code, "WATCHLIST_INVALID");

        // Read groups
        let read_port = ports.watchlist;
        let list_res = read_port
            .read("/api/v1/watchlist/groups", "")
            .expect("list groups");
        assert!(!list_res["groups"].as_array().unwrap().is_empty());

        // Update group with correct revision
        let update_res = watchlist
            .mutate(&WatchlistWriteMutation {
                value: json!({
                    "route": "update-group",
                    "groupId": group_id,
                    "name": "US Tech & AI",
                    "expectedRevision": revision,
                }),
            })
            .expect("update group");
        assert_eq!(update_res["name"], "US Tech & AI");

        // Delete group
        let del_res = watchlist
            .mutate(&WatchlistWriteMutation {
                value: json!({
                    "route": "delete-group",
                    "groupId": group_id,
                }),
            })
            .expect("delete group");
        assert_eq!(del_res["deleted"], true);
    }

    #[test]
    fn production_watchlist_read_uses_real_pages_and_remote_catalog() {
        let (_temp_dir, settings_path, config, security) = setup_test_env();
        let descriptors =
            product_data_management::managed_database_runtime_descriptors(&settings_path);
        let watchlist_path = descriptors
            .iter()
            .find(|descriptor| descriptor.id == DATABASE_WATCHLIST)
            .expect("watchlist descriptor")
            .path
            .clone();
        let connection = rusqlite::Connection::open(&watchlist_path).expect("open watchlist db");
        connection
            .execute(
                "INSERT INTO watchlist_sources
                    (source_id, broker, display_name, status, last_error, updated_at)
                 VALUES ('futu:default', 'futu', 'Futu', 'ready', '', '2026-08-24T04:00:00Z')",
                [],
            )
            .expect("seed source");
        connection
            .execute(
                "INSERT INTO watchlist_remote_groups
                    (source_id, remote_group_id, name, group_type, ambiguous, member_count, remote_hash, observed_at)
                 VALUES ('futu:default', 'remote-tech', 'Tech', 'stock', 0, 1, 'hash-1', '2026-08-24T04:00:00Z')",
                [],
            )
            .expect("seed remote group");
        drop(connection);

        let ports = production_ports(&config, &security).expect("production ports");
        let group = ports
            .watchlist_write
            .mutate(&WatchlistWriteMutation {
                value: json!({"route": "create-group", "name": "Technology"}),
            })
            .expect("create technology group");
        let group_id = group["groupId"].as_str().expect("group id");
        ports
            .watchlist_write
            .mutate(&WatchlistWriteMutation {
                value: json!({
                    "route": "replace-memberships",
                    "instrumentId": "US.AAPL",
                    "groupIds": [group_id],
                    "newGroupNames": [],
                    "expectedRevision": 0,
                }),
            })
            .expect("create instrument membership");

        let items = ports
            .watchlist
            .read("/api/v1/watchlist/items", "limit=1&market=US&query=US.AAPL")
            .expect("read filtered items");
        assert_eq!(items["items"].as_array().expect("items").len(), 1);
        assert_eq!(items["items"][0]["instrumentId"], "US.AAPL");
        assert_eq!(items["items"][0]["groupIds"][0], group_id);
        assert!(items["nextCursor"].is_null());

        let invalid = ports
            .watchlist
            .read("/api/v1/watchlist/items", "limit=0")
            .expect_err("zero limit must be rejected");
        assert!(matches!(invalid, WatchlistReadSnapshotError::Invalid(_)));

        let remote_groups = ports
            .watchlist
            .read("/api/v1/watchlist/sources/futu:default/groups", "")
            .expect("read persisted remote groups");
        assert_eq!(remote_groups["groups"][0]["remoteGroupId"], "remote-tech");

        let missing_source = ports
            .watchlist
            .read("/api/v1/watchlist/sources/missing/groups", "")
            .expect_err("missing source must be not found");
        assert!(matches!(
            missing_source,
            WatchlistReadSnapshotError::NotFound
        ));
    }

    #[test]
    fn production_strategy_and_research_port_crud_operations() {
        let (_temp_dir, _settings_path, config, security) = setup_test_env();
        let ports = production_ports(&config, &security).expect("production ports");

        // Strategy definition CRUD
        let strat_port = ports.strategy_definition_write;
        let create_res = strat_port
            .mutate(&StrategyDefinitionWriteInput {
                operation: StrategyDefinitionWriteOperation::Create,
                definition_id: Some("strat-alpha".to_owned()),
                definition: Some(json!({
                    "id": "strat-alpha",
                    "name": "Dual Moving Average",
                })),
                binding: None,
                binding_error: None,
            })
            .expect("create strategy");
        assert_eq!(create_res["name"], "Dual Moving Average");

        let strat_read = ports.strategy_definition;
        let list_res = strat_read.list().expect("list strategies");
        assert_eq!(list_res.len(), 1);

        // Research preset CRUD
        let research_write = ports.research_preset_write;
        let preset_res = research_write
            .mutate(&ResearchPresetWriteMutation::Create {
                payload: json!({
                    "name": "High Volume Screen",
                    "filters": [],
                }),
            })
            .expect("create preset");
        assert_eq!(preset_res["name"], "High Volume Screen");

        let research_read = ports.research_preset_read;
        let presets_list = research_read
            .read("/api/v1/research/screens/presets", "")
            .expect("list presets");
        assert!(!presets_list["presets"].as_array().unwrap().is_empty());
    }

    #[test]
    fn production_strategy_runtime_port_instance_mutations() {
        let (_temp_dir, _settings_path, config, security) = setup_test_env();
        let ports = production_ports(&config, &security).expect("production ports");

        let strat_def_port = ports.strategy_definition_write;
        strat_def_port
            .mutate(&StrategyDefinitionWriteInput {
                operation: StrategyDefinitionWriteOperation::Create,
                definition_id: Some("strat-beta".to_owned()),
                definition: Some(json!({
                    "id": "strat-beta",
                    "name": "RSI Strategy",
                })),
                binding: None,
                binding_error: None,
            })
            .expect("create def");
        strat_def_port
            .mutate(&StrategyDefinitionWriteInput {
                operation: StrategyDefinitionWriteOperation::Instantiate,
                definition_id: Some("strat-beta".to_owned()),
                definition: None,
                binding: None,
                binding_error: None,
            })
            .expect("instantiate strategy");

        let runtime_read = ports.strategy_read;
        let instances = runtime_read
            .read("/api/v1/strategies", "")
            .expect("list runtime instances")
            .expect("strategy list response");
        assert_eq!(instances.as_array().expect("strategy array").len(), 1);
        assert!(matches!(
            runtime_read.read("/api/v1/strategies/inst_strat-beta/logs", "limit=bad"),
            Err(crate::product::StrategyReadSnapshotError::Invalid(message))
                if message == "invalid logs query"
        ));

        let runtime_write = ports.strategy_runtime_write;

        // Non-existent instance update should fail with not found
        let update_res = runtime_write.mutate(&StrategyRuntimeWriteInput {
            operation: StrategyRuntimeWriteOperation::Update,
            instance_id: "inst-nonexistent".to_owned(),
            binding: Some(json!({"symbol": "AAPL", "interval": "1m"})),
            runtime_risk: None,
        });
        assert!(update_res.is_err());
    }

    #[tokio::test]
    async fn production_market_data_catalog_and_provider_ports() {
        let (_temp_dir, _settings_path, config, security) = setup_test_env();
        let ports = production_ports(&config, &security).expect("production ports");

        let catalog = ports.catalog;
        let unconfigured_markets = catalog
            .read("/api/v1/market-data/markets", "")
            .await
            .expect_err("unconfigured active provider helper must return unavailable");
        assert!(matches!(
            unconfigured_markets,
            crate::product::MarketDataCatalogReadSnapshotError::Unavailable(_)
        ));

        // When helper is not configured, search instruments fails closed without synthetic resolution
        let unconfigured_search = catalog
            .read("/api/v1/market-data/instruments", "query=AAPL&limit=10")
            .await
            .expect_err("unconfigured search must fail closed");
        assert!(matches!(
            unconfigured_search,
            crate::product::MarketDataCatalogReadSnapshotError::Unavailable(_)
        ));

        // Configure active provider in settings
        fs::write(
            &_settings_path,
            br#"{"activeMarketDataProvider":"akshare"}"#,
        )
        .expect("write active provider");

        let provider = ports.provider;
        let provider_res = provider
            .read("/api/v1/market-data/provider", "")
            .expect("read provider");
        assert_eq!(provider_res["descriptor"]["selectionId"], "akshare");
        assert_eq!(provider_res["descriptor"]["providerId"], "akshare");
        assert_eq!(provider_res["health"]["connected"], false);
        assert_eq!(provider_res["health"]["readiness"], "unavailable");
        assert_eq!(provider_res["health"]["activeCount"], 0);
        assert_eq!(provider_res["runtime"]["Connected"], false);
        assert_eq!(provider_res["runtime"]["Closed"], false);

        // Quote port verification
        let quote = ports.market_data_quote;
        let sub_res = quote
            .read("/api/v1/market-data/subscriptions", "")
            .await
            .expect("read subscriptions without router returns 200 empty projection");
        assert_eq!(sub_res["totalActiveSubscriptions"], 0);
        assert_eq!(sub_res["entries"], serde_json::json!([]));

        let sec_err = quote
            .read("/api/v1/market-data/securities/US/AAPL", "")
            .await
            .expect_err("unconfigured securities without helper/opend must fail closed");
        assert!(matches!(
            sec_err,
            crate::product::MarketDataQuoteReadSnapshotError::Unavailable(_)
        ));

        // When helper/router is not configured, quote snapshots, candles, depth fail closed
        let snap_err = quote
            .read("/api/v1/market-data/snapshots/US/AAPL", "")
            .await
            .expect_err("unconfigured quote snapshot must fail closed");
        assert!(matches!(
            snap_err,
            crate::product::MarketDataQuoteReadSnapshotError::Unavailable(_)
        ));

        let candle_err = quote
            .read("/api/v1/market-data/candles/US/AAPL", "period=1d")
            .await
            .expect_err("unconfigured candles must fail closed");
        assert!(matches!(
            candle_err,
            crate::product::MarketDataQuoteReadSnapshotError::Unavailable(_)
        ));

        let depth_err = quote
            .read("/api/v1/market-data/depth/US/AAPL", "")
            .await
            .expect_err("unconfigured depth must fail closed");
        assert!(matches!(
            depth_err,
            crate::product::MarketDataQuoteReadSnapshotError::Unavailable(_)
        ));

        // Subscription mutation verification (fail closed when router missing)
        let sub_write = ports.market_data_subscription_mutation;
        let acquire_payload = serde_json::to_vec(&json!({
            "consumerId": "client-1",
            "providerBrokerId": "akshare",
            "instruments": [
                {
                    "channel": "SNAPSHOT",
                    "market": "US",
                    "symbol": "AAPL"
                }
            ]
        }))
        .unwrap();
        let acquire_res = sub_write
            .dispatch(&crate::product::product_market_data_subscription_mutation_port::MarketDataSubscriptionMutationRequest {
                method: "POST".to_owned(),
                path: "/api/v1/market-data/subscriptions".to_owned(),
                query: String::new(),
                body: acquire_payload,
            })
            .expect("acquire non-futu returns 200 polling projection");
        assert_eq!(acquire_res["action"], "acquired");
        assert_eq!(acquire_res["transport"]["mode"], "snapshot-poll-fallback");

        let release_payload = serde_json::to_vec(&json!({
            "consumerId": "client-1",
            "providerBrokerId": "akshare",
        }))
        .unwrap();
        let release_res = sub_write
            .dispatch(&crate::product::product_market_data_subscription_mutation_port::MarketDataSubscriptionMutationRequest {
                method: "POST".to_owned(),
                path: "/api/v1/market-data/subscriptions/release".to_owned(),
                query: String::new(),
                body: release_payload,
            })
            .expect("release non-futu returns 200 polling projection");
        assert_eq!(release_res["action"], "released");

        // Futu acquire without router must fail closed with Unavailable
        let futu_payload = serde_json::to_vec(&json!({
            "consumerId": "client-1",
            "providerBrokerId": "futu",
            "instruments": [
                {
                    "channel": "SNAPSHOT",
                    "market": "US",
                    "symbol": "AAPL"
                }
            ]
        }))
        .unwrap();
        let futu_err = sub_write
            .dispatch(&crate::product::product_market_data_subscription_mutation_port::MarketDataSubscriptionMutationRequest {
                method: "POST".to_owned(),
                path: "/api/v1/market-data/subscriptions".to_owned(),
                query: String::new(),
                body: futu_payload,
            })
            .expect_err("futu acquire without router must fail closed");
        assert!(matches!(
            futu_err,
            crate::product::product_market_data_subscription_mutation_port::MarketDataSubscriptionMutationPortError::Unavailable(_)
        ));

        // Provider actions verification
        let actions = ports.market_data_provider_actions;
        let norm_payload = serde_json::to_vec(&json!({
            "market": "US",
            "symbol": "AAPL"
        }))
        .unwrap();
        let norm_res = actions
            .dispatch(&crate::product::product_market_data_provider_actions_port::MarketDataProviderActionsRequest {
                method: "POST".to_owned(),
                path: "/api/v1/market-data/instruments/normalize".to_owned(),
                query: String::new(),
                body: norm_payload,
            })
            .await
            .expect("normalize instrument");
        assert_eq!(norm_res["instrumentId"], "US.AAPL");
        assert_eq!(norm_res["symbol"], "US.AAPL");

        // Verification with configured router
        use crate::product::product_market_data_subscription_mutation_port::MarketDataSubscriptionMutationPort;
        fs::write(&_settings_path, br#"{"activeMarketDataProvider":"futu"}"#)
            .expect("write futu active provider");
        let _test_settings = Arc::new(
            jftrade_store_settings_file::SettingsFileStore::open_read_only(&_settings_path)
                .expect("open settings"),
        );
        let active_state = Arc::new(crate::product::ActiveProviderState::new(Some(
            jftrade_settings::MarketDataProvider::Futu,
        )));
        let router = Arc::new(std::sync::Mutex::new(
            jftrade_marketdata::ProviderRouter::new(100),
        ));
        let router_sub_write = crate::product::product_production_ports::ProductionMarketDataSubscriptionMutationPort::new(
            active_state.clone(),
            Some(router.clone()),
            None,
        );
        let acquire_success = router_sub_write
            .dispatch(&crate::product::product_market_data_subscription_mutation_port::MarketDataSubscriptionMutationRequest {
                method: "POST".to_owned(),
                path: "/api/v1/market-data/subscriptions".to_owned(),
                query: String::new(),
                body: serde_json::to_vec(&json!({
                    "consumerId": "client-1",
                    "providerBrokerId": "futu",
                    "instruments": [
                        {
                            "channel": "SNAPSHOT",
                            "market": "US",
                            "symbol": "AAPL"
                        }
                    ]
                })).unwrap(),
            })
            .expect("acquire subscription with router");
        assert_eq!(acquire_success["totalActiveSubscriptions"], 1);
        assert_eq!(acquire_success["desiredCount"], 1);

        let release_success = router_sub_write
            .dispatch(&crate::product::product_market_data_subscription_mutation_port::MarketDataSubscriptionMutationRequest {
                method: "POST".to_owned(),
                path: "/api/v1/market-data/subscriptions/release".to_owned(),
                query: String::new(),
                body: serde_json::to_vec(&json!({
                    "consumerId": "client-1"
                })).unwrap(),
            })
            .expect("release subscription with router");
        assert_eq!(release_success["released"], true);

        use crate::product::MarketDataQuoteReadSnapshotPort;
        fs::write(&_settings_path, br#"{"activeMarketDataProvider":"futu"}"#)
            .expect("write futu active provider");
        let router_quote =
            crate::product::product_production_ports::ProductionMarketDataQuotePort::new(
                active_state,
                Some(router.clone()),
                None,
                None,
            );
        let sub_get = router_quote
            .read("/api/v1/market-data/subscriptions", "")
            .await
            .expect("read subscriptions with router");
        assert_eq!(sub_get["desiredCount"], 0);
        assert!(sub_get["transport"].is_null());
    }

    #[test]
    fn production_system_read_reports_unavailable_opend_without_fake_health() {
        let (_temp_dir, _settings_path, config, security) = setup_test_env();
        let ports = production_ports(&config, &security).expect("production ports");

        let opend = ports
            .system_read
            .read("/api/v1/system/futu-opend")
            .expect("OpenD health projection");
        assert_eq!(opend["status"], "unavailable");
        assert_eq!(opend["reason"], "broker integration not enabled");
        assert!(opend.get("runtime").is_none());

        let order_updates = ports
            .system_read
            .read("/api/v1/system/worker/broker-order-updates")
            .expect("worker snapshot");
        assert_eq!(order_updates, json!({}));
    }

    #[test]
    fn production_plugin_artifact_operations_are_atomic_and_restart_safe() {
        let (temp_dir, _settings_path, config, security) = setup_test_env();
        let plugin_dir = temp_dir.path().join("plugins");
        fs::create_dir_all(&plugin_dir).expect("create plugin directory");
        let source_path = temp_dir.path().join("alpha-source.so");
        fs::write(&source_path, b"alpha plugin artifact").expect("seed plugin artifact");
        let marker = json!({
            "descriptor": {
                "id": "alpha",
                "type": "strategy-go-plugin",
                "displayName": "Alpha",
                "version": "1.0.0",
                "description": "",
                "keywords": []
            },
            "installation": {
                "sourcePath": source_path.to_string_lossy()
            }
        });
        fs::write(
            plugin_dir.join("alpha.json"),
            serde_json::to_vec_pretty(&marker).expect("encode plugin marker"),
        )
        .expect("seed plugin marker");

        let ports = production_ports(&config, &security).expect("production ports");
        let initial = ports.plugins.catalog().expect("plugin catalog");
        assert_eq!(initial["plugins"][0]["installation"]["installed"], false);
        let install_path = plugin_dir.join("alpha.so");
        assert!(!install_path.exists());
        let installed = ports
            .plugin_write
            .mutate(
                crate::product::product_plugins_write_port::PluginWriteOperation::Install,
                "alpha",
            )
            .expect("install plugin artifact");
        assert_eq!(installed["phase"], "installed");
        assert_eq!(
            fs::read(&install_path).expect("read installed artifact"),
            b"alpha plugin artifact"
        );
        let installed_catalog = ports.plugins.catalog().expect("installed catalog");
        assert_eq!(
            installed_catalog["plugins"][0]["installation"]["status"],
            "INSTALLED"
        );
        let operation_id = installed["operationId"]
            .as_str()
            .expect("operation id")
            .to_owned();
        assert_eq!(
            ports
                .plugins
                .operation(&operation_id)
                .expect("operation lookup")
                .expect("stored operation")["operationId"],
            operation_id
        );

        let uninstalled = ports
            .plugin_write
            .mutate(
                crate::product::product_plugins_write_port::PluginWriteOperation::Uninstall,
                "alpha",
            )
            .expect("uninstall plugin artifact");
        assert_eq!(uninstalled["phase"], "uninstalled");
        assert!(!install_path.exists());
        let marker = fs::read_to_string(plugin_dir.join("alpha.json")).expect("read marker");
        assert!(marker.contains("NOT_INSTALLED"));
        assert!(marker.contains("operations"));

        // The marker is the durable operation registry.  Reopening the
        // production bundle must expose the last operation without relying on
        // the previous in-memory port instance.
        drop(ports);
        let restarted = production_ports(&config, &security).expect("reopen production ports");
        let restarted_catalog = restarted.plugins.catalog().expect("catalog after restart");
        assert_eq!(
            restarted_catalog["plugins"][0]["installation"]["status"],
            "NOT_INSTALLED"
        );
        assert_eq!(
            restarted
                .plugins
                .operation(&operation_id)
                .expect("operation lookup after restart")
                .expect("operation persisted after restart")["operationId"],
            operation_id
        );
    }

    #[test]
    fn production_plugin_install_requires_an_artifact_without_mutating_state() {
        let (temp_dir, _settings_path, config, security) = setup_test_env();
        let plugin_dir = temp_dir.path().join("plugins");
        fs::create_dir_all(&plugin_dir).expect("create plugin directory");
        let marker_path = plugin_dir.join("missing.json");
        let marker = br#"{
            "descriptor": {
                "id": "missing",
                "type": "strategy-go-plugin",
                "displayName": "Missing",
                "version": "1.0.0",
                "description": "",
                "keywords": []
            }
        }"#;
        fs::write(&marker_path, marker).expect("seed plugin marker");
        let before = fs::read(&marker_path).expect("read marker before install");

        let ports = production_ports(&config, &security).expect("production ports");
        let error = ports
            .plugin_write
            .mutate(
                crate::product::product_plugins_write_port::PluginWriteOperation::Install,
                "missing",
            )
            .expect_err("missing artifact must not report a successful install");
        assert!(matches!(
            error,
            crate::product::product_plugins_write_port::PluginWritePortError::Unavailable(_)
        ));
        assert_eq!(
            fs::read(&marker_path).expect("read marker after install"),
            before
        );
        assert!(!plugin_dir.join("missing.so").exists());
    }

    #[test]
    fn production_registry_is_built_from_non_optional_adapters() {
        let (_temp_dir, _settings_path, config, security) = setup_test_env();
        let ports = production_ports(&config, &security).expect("production ports");
        let registry =
            crate::product::product_production_route_registry::ProductionRouteRegistry::bind(
                &ports,
            )
            .expect("all strongly typed production adapters must bind");
        assert_eq!(registry.bindings().len(), 278);
        assert!(registry.bindings().iter().any(|binding| {
            binding.path == "/api/v1/watchlist/groups"
                && binding.adapter_binding == ProductionAdapterBinding::Ready
        }));
        assert!(registry.bindings().iter().any(|binding| {
            binding.path == "/api/v1/market-data/markets"
                && binding.adapter_binding == ProductionAdapterBinding::ExternalUnavailable
        }));
        assert!(registry.bindings().iter().any(|binding| {
            binding.path == "/api/v1/market-data/warrants"
                && binding.adapter_binding == ProductionAdapterBinding::ExternalUnavailable
        }));
        assert!(registry.bindings().iter().any(|binding| {
            binding.method == "DELETE"
                && binding.path == "/api/v1/backtests/{runId}"
                && binding.adapter_binding == ProductionAdapterBinding::Ready
        }));
        assert!(registry.bindings().iter().any(|binding| {
            binding.method == "POST"
                && binding.path == "/api/v1/backtests/sync"
                && binding.adapter_binding == ProductionAdapterBinding::ExternalUnavailable
        }));
        assert!(registry.bindings().iter().any(|binding| {
            binding.method == "GET"
                && binding.path == "/api/v1/backtests/sync/{taskId}"
                && binding.adapter_binding == ProductionAdapterBinding::Ready
        }));
        assert!(registry.bindings().iter().any(|binding| {
            binding.method == "DELETE"
                && binding.path == "/api/v1/backtests/sync/{taskId}"
                && binding.adapter_binding == ProductionAdapterBinding::Ready
        }));
        assert!(registry.bindings().iter().any(|binding| {
            binding.method == "PUT"
                && binding.path == "/api/v1/system/real-trade-risk-limits"
                && binding.adapter_binding == ProductionAdapterBinding::Ready
        }));
        assert!(registry.bindings().iter().any(|binding| {
            binding.method == "POST"
                && binding.path == "/api/v1/system/futu-opend/manual-retry"
                && binding.adapter_binding == ProductionAdapterBinding::ExternalUnavailable
        }));
    }

    #[test]
    fn production_registry_rejects_a_missing_binding_instead_of_registering_it() {
        let (_temp_dir, _settings_path, config, security) = setup_test_env();
        let mut ports = production_ports(&config, &security).expect("production ports");
        ports
            .bound_adapters
            .remove(&crate::product::product_production_route_registry::ProductionRouteAdapter::WatchlistRead);
        let error =
            crate::product::product_production_route_registry::ProductionRouteRegistry::bind(
                &ports,
            )
            .expect_err("missing production adapter must fail startup");
        assert!(matches!(
            error,
            ProductError::MissingProductionAdapter { .. }
        ));
    }

    #[test]
    fn production_adk_and_plugin_and_alert_ports() {
        let (_temp_dir, _settings_path, config, security) = setup_test_env();
        let ports = production_ports(&config, &security).expect("production ports");

        let adk_catalog = ports
            .adk_read
            .read("/api/v1/adk", "")
            .expect("read ADK catalog");
        match adk_catalog {
            crate::product::AdkReadSnapshot::Json(value) => {
                assert_eq!(value["runtimeSettings"]["runTimeoutMs"], 1_800_000);
                assert_eq!(value["runtimeSettings"]["streamIdleTimeoutMs"], 300_000);
                assert!(
                    value["tools"]
                        .as_array()
                        .is_some_and(|tools| !tools.is_empty())
                );
            }
            crate::product::AdkReadSnapshot::Stream(_) => panic!("expected ADK JSON catalog"),
        }

        // ADK Mutation - Create Agent
        let adk_write = ports.adk_mutation;
        let agent_res = adk_write
            .mutate(&AdkMutationInput {
                operation: AdkMutationOperation::CreateAgent,
                identifiers: std::collections::BTreeMap::new(),
                body: json!({
                    "id": "agent-alpha",
                    "name": "Alpha Assistant",
                    "model": "gpt-4",
                }),
                webhook_secret: None,
            })
            .expect("create agent");
        assert_eq!(agent_res["name"], "Alpha Assistant");

        // ADK Read - List Agents
        let adk_read = ports.adk_read;
        let list_res = adk_read
            .read("/api/v1/adk/agents", "")
            .expect("read agents");
        match list_res {
            crate::product::AdkReadSnapshot::Json(val) => {
                let agents = val["agents"].as_array().expect("agents array");
                assert_eq!(agents.len(), 1);
                assert_eq!(agents[0]["id"], "agent-alpha");
            }
            crate::product::AdkReadSnapshot::Stream(_) => panic!("expected json"),
        }

        // ADK Mutation - Delete Agent
        let mut del_idents = std::collections::BTreeMap::new();
        del_idents.insert("agentId".to_owned(), "agent-alpha".to_owned());
        let del_res = adk_write
            .mutate(&AdkMutationInput {
                operation: AdkMutationOperation::DeleteAgent,
                identifiers: del_idents,
                body: json!({}),
                webhook_secret: None,
            })
            .expect("delete agent");
        assert_eq!(del_res["deleted"], true);

        // ADK Chat Stream - Fail-closed when no model runtime configured
        let adk_chat = ports.adk_chat_stream;
        let chat_err = adk_chat
            .dispatch(
                AdkChatRoute::Stream,
                &AdkChatInput {
                    body: br#"{"clientRequestId":"11111111-1111-4111-8111-111111111111"}"#.to_vec(),
                    client_request_id: "11111111-1111-4111-8111-111111111111".to_owned(),
                },
            )
            .expect_err("adk chat stream must fail-closed without configured model");
        assert!(matches!(chat_err, AdkChatPortError::Unavailable(_)));

        // Plugins are a local production catalog.  A fresh install is a valid
        // empty result; plugin mutations remain unavailable until the external
        // artifact/process runtime is configured.
        let plugin_port = ports.plugins;
        let plugin_catalog = plugin_port.catalog().expect("empty plugin catalog");
        assert_eq!(plugin_catalog["plugins"].as_array().map(Vec::len), Some(0));
        assert!(plugin_catalog["targetDir"].is_string());

        let alert_port = ports.alert_snapshot;
        assert!(matches!(
            alert_port.snapshot(crate::product::AlertKind::Price, ""),
            Err(crate::product::AlertSnapshotError::Unavailable(_))
        ));

        let alert_write = ports.alert_write;
        let alert_err = alert_write
            .apply(
                &AlertWriteResolution {
                    broker_id: "futu".to_owned(),
                    security_firm: "futu".to_owned(),
                    capability: "alerts".to_owned(),
                    selection_reason: "default".to_owned(),
                },
                &AlertWriteAction {
                    route: crate::product::product_alerts_write_port::AlertWriteRoute::Price,
                    feature_id: "alerts.price.set",
                    broker_id: "futu".to_owned(),
                    account_id: Some("123".to_owned()),
                    action: "set",
                    payload: Some(json!({})),
                    payload_state:
                        crate::product::product_alerts_write_port::AlertWritePayloadState::EmptyObject,
                },
            )
            .expect_err("alert write apply must fail-closed without configured provider");
        assert!(matches!(alert_err, AlertWritePortError::Unavailable(_)));
    }

    #[test]
    fn production_internal_adapters_dynamic_capability_and_recovery() {
        let (_temp_dir, _settings_path, config, security) = setup_test_env();
        let ports = production_ports(&config, &security).expect("production ports");

        // 1. Broker & Portfolio dynamic fail-closed capability
        let broker_err = ports
            .broker
            .read("/api/v1/brokers/capabilities", "")
            .expect_err("broker read must fail closed when not configured");
        assert!(matches!(
            broker_err,
            crate::product::BrokerReadSnapshotError::Unavailable(_)
        ));

        let portfolio_err = ports
            .portfolio
            .read("/api/v1/portfolio/overview", "")
            .expect_err("portfolio read must fail closed when not configured");
        assert!(matches!(
            portfolio_err,
            crate::product::PortfolioSnapshotError::Unavailable(_)
        ));

        // 2. Research Read & Screen Write dynamic fail-closed capability
        let research_err = ports
            .research_read
            .read("/api/v1/research/calendars", "")
            .expect_err("research read must fail closed when not configured");
        assert!(matches!(
            research_err,
            crate::product::ResearchReadSnapshotError::Unavailable(_)
        ));

        // 3. Remote Watchlist snapshot and write
        let remote_wl_err = ports
            .remote_watchlist
            .read("")
            .expect_err("remote watchlist read must fail closed when not configured");
        assert!(matches!(
            remote_wl_err,
            crate::product::RemoteWatchlistSnapshotError::Unavailable(_)
        ));

        let remote_wl_write_err = ports
            .remote_watchlist_write
            .resolve(Some("futu"), Some("123"))
            .expect_err("remote watchlist write must fail closed when not configured");
        assert!(matches!(
            remote_wl_write_err,
            crate::product::product_watchlist_remote_write_port::RemoteWatchlistWritePortError::Unavailable(_)
        ));

        // 4. Strategy Pine analyze
        let pine_err = ports
            .strategy_pine_analyze
            .analyze(&crate::product::strategy_pine::StrategyPineAnalyzeInput {
                script: "indicator('test')".to_owned(),
                source_format: "pine-v6".to_owned(),
                include_ast: false,
            })
            .expect_err("strategy pine analyze must fail closed when worker not ready");
        assert!(matches!(
            pine_err,
            crate::product::strategy_pine::StrategyPineAnalyzeSnapshotError::Unavailable(_)
        ));

        // 5. Market data derivatives, options, news, prediction
        assert!(matches!(
            ports
                .market_data_derivative
                .read("/api/v1/market-data/warrants", ""),
            Err(crate::product::MarketDataDerivativeReadSnapshotError::Unavailable(_))
        ));
        assert!(matches!(
            ports
                .market_data_options
                .read("/api/v1/market-data/options/US.AAPL/chain", ""),
            Err(crate::product::MarketDataOptionsReadSnapshotError::Unavailable(_))
        ));
        assert!(matches!(
            ports
                .market_data_news_actions
                .read("/api/v1/market-data/news/US.AAPL", ""),
            Err(crate::product::MarketDataNewsActionsReadSnapshotError::Unavailable(_))
        ));
        assert!(matches!(
            ports
                .market_data_news_search
                .read("/api/v1/market-data/news", ""),
            Err(crate::product::MarketDataNewsSearchReadSnapshotError::Unavailable(_))
        ));
        assert!(matches!(
            ports
                .market_data_prediction
                .read("/api/v1/market-data/prediction/events", ""),
            Err(crate::product::MarketDataPredictionReadSnapshotError::Unavailable(_))
        ));
    }

    #[test]
    fn production_builtin_skills_project_bound_tool_catalog() {
        let (_temp_dir, _settings_path, config, security) = setup_test_env();
        let ports = production_ports(&config, &security).expect("production ports");

        let tools = match ports
            .adk_read
            .read("/api/v1/adk/tools", "")
            .expect("read production tools")
        {
            crate::product::AdkReadSnapshot::Json(value) => value["tools"]
                .as_array()
                .expect("tools array")
                .iter()
                .filter_map(|tool| tool["id"].as_str())
                .map(str::to_owned)
                .collect::<std::collections::BTreeSet<_>>(),
            crate::product::AdkReadSnapshot::Stream(_) => panic!("expected tools JSON"),
        };
        let skills = match ports
            .adk_read
            .read("/api/v1/adk/skills", "")
            .expect("read builtin skills")
        {
            crate::product::AdkReadSnapshot::Json(value) => {
                value["skills"].as_array().expect("skills array").clone()
            }
            crate::product::AdkReadSnapshot::Stream(_) => panic!("expected skills JSON"),
        };

        assert_eq!(skills.len(), 11);
        for skill in &skills {
            assert_eq!(skill["source"], "builtin");
            assert_eq!(skill["validationStatus"], "VALID");
            let skill_tools = skill["tools"].as_array().expect("skill tools array");
            assert!(
                skill_tools
                    .iter()
                    .all(|tool| tool.as_str().is_some_and(|id| tools.contains(id))),
                "skill {} contains a tool outside the production catalog: {}",
                skill["id"],
                skill["tools"]
            );
        }

        let market_tools = skills
            .iter()
            .find(|skill| skill["id"] == "jftrade-market")
            .expect("market builtin skill")["tools"]
            .as_array()
            .expect("market skill tools");
        assert!(market_tools.iter().any(|tool| tool == "market.search"));

        let unavailable_skill_ids = [
            "jftrade-derivatives",
            "jftrade-prediction",
            "jftrade-trading",
            "external-http",
        ];
        for id in unavailable_skill_ids {
            let skill = skills
                .iter()
                .find(|skill| skill["id"] == id)
                .expect("builtin skill");
            assert_eq!(skill["tools"], json!([]));
        }
    }

    #[tokio::test]
    async fn production_adk_public_sessions_use_main_store_and_support_crud() {
        let (_temp_dir, _settings_path, config, _security) = setup_test_env();
        let handle = start_product(config).await.expect("start product");
        let address = handle.startup_record().address;
        let authorization = "Bearer aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa";

        let (create_status, create_response) = request_json_with_status(
            address,
            "POST",
            "/api/v1/adk/sessions",
            Some(r#"{"agentId":"jftrade-default","title":"Production Session"}"#),
            &[("Authorization", authorization)],
        )
        .await;
        assert_eq!(create_status, 200, "create response: {create_response}");
        let session_id = create_response["data"]["id"]
            .as_str()
            .expect("created session id")
            .to_owned();
        assert!(session_id.starts_with("session-"));
        assert_eq!(create_response["data"]["agentId"], "jftrade-default");
        assert_eq!(create_response["data"]["title"], "Production Session");

        let (list_status, list_response) = request_json_with_status(
            address,
            "GET",
            "/api/v1/adk/sessions?agentId=jftrade-default&query=production&limit=5",
            None,
            &[("Authorization", authorization)],
        )
        .await;
        assert_eq!(list_status, 200, "list response: {list_response}");
        assert_eq!(list_response["data"]["page"]["total"], 1);
        assert_eq!(list_response["data"]["sessions"][0]["id"], session_id);

        let (detail_status, detail_response) = request_json_with_status(
            address,
            "GET",
            &format!("/api/v1/adk/sessions/{session_id}"),
            None,
            &[("Authorization", authorization)],
        )
        .await;
        assert_eq!(detail_status, 200, "detail response: {detail_response}");
        assert_eq!(detail_response["data"]["session"]["id"], session_id);
        assert!(detail_response["data"]["timeline"].is_array());
        assert_eq!(
            detail_response["data"]["composerState"]["sessionId"],
            session_id
        );

        let (composer_status, composer_response) = request_json_with_status(
            address,
            "PATCH",
            &format!("/api/v1/adk/sessions/{session_id}/composer-state"),
            Some(r#"{"chatDraft":"draft","workModeOverride":"loop","permissionModeOverride":"less_approval","goalObjectiveTouched":true}"#),
            &[("Authorization", authorization)],
        )
        .await;
        assert_eq!(
            composer_status, 200,
            "composer response: {composer_response}"
        );
        assert_eq!(composer_response["data"]["chatDraft"], "draft");
        assert_eq!(composer_response["data"]["workModeOverride"], "loop");

        let (rename_status, rename_response) = request_json_with_status(
            address,
            "PUT",
            &format!("/api/v1/adk/sessions/{session_id}"),
            Some(r#"{"title":"Renamed Session"}"#),
            &[("Authorization", authorization)],
        )
        .await;
        assert_eq!(rename_status, 200, "rename response: {rename_response}");
        assert_eq!(rename_response["data"]["title"], "Renamed Session");

        let (delete_status, delete_response) = request_json_with_status(
            address,
            "DELETE",
            &format!("/api/v1/adk/sessions/{session_id}"),
            None,
            &[("Authorization", authorization)],
        )
        .await;
        assert_eq!(delete_status, 200, "delete response: {delete_response}");
        assert_eq!(delete_response["data"]["deleted"], true);

        let (missing_status, missing_response) = request_json_with_status(
            address,
            "GET",
            &format!("/api/v1/adk/sessions/{session_id}"),
            None,
            &[("Authorization", authorization)],
        )
        .await;
        assert_eq!(missing_status, 404, "missing response: {missing_response}");
        assert_eq!(missing_response["error"]["code"], "NOT_FOUND");

        let (invalid_agent_status, invalid_agent_response) = request_json_with_status(
            address,
            "POST",
            "/api/v1/adk/sessions",
            Some(r#"{"agentId":"missing-agent"}"#),
            &[("Authorization", authorization)],
        )
        .await;
        assert_eq!(invalid_agent_status, 400);
        assert_eq!(
            invalid_agent_response["error"]["code"],
            "ADK_INVALID_REQUEST"
        );

        handle.shutdown().await.expect("shutdown cleanly");
    }

    #[tokio::test]
    async fn production_adk_local_mutations_persist_tasks_memory_and_workflow_triggers() {
        let (_temp_dir, _settings_path, config, _security) = setup_test_env();
        let handle = start_product(config).await.expect("start product");
        let address = handle.startup_record().address;
        let authorization = "Bearer aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa";

        let (workflow_status, workflow_response) = request_json_with_status(
            address,
            "POST",
            "/api/v1/adk/workflows",
            Some(
                r#"{"id":"production-workflow","name":"Production workflow","agentId":"jftrade-default","workMode":"loop","promptTemplate":"run"}"#,
            ),
            &[("Authorization", authorization)],
        )
        .await;
        assert_eq!(
            workflow_status, 200,
            "workflow response: {workflow_response}"
        );

        let (trigger_status, trigger_response) = request_json_with_status(
            address,
            "POST",
            "/api/v1/adk/workflows/production-workflow/triggers",
            Some(r#"{"type":"webhook","status":"ENABLED","config":{"source":"test"}}"#),
            &[("Authorization", authorization)],
        )
        .await;
        assert_eq!(trigger_status, 200, "trigger response: {trigger_response}");
        let trigger_id = trigger_response["data"]["trigger"]["id"]
            .as_str()
            .expect("trigger id")
            .to_owned();
        assert!(
            trigger_response["data"]["secret"]
                .as_str()
                .is_some_and(|value| !value.is_empty())
        );
        assert!(trigger_response["data"]["trigger"]["secretHash"].is_null());

        let (trigger_list_status, trigger_list_response) = request_json_with_status(
            address,
            "GET",
            "/api/v1/adk/workflows/production-workflow/triggers",
            None,
            &[("Authorization", authorization)],
        )
        .await;
        assert_eq!(
            trigger_list_status, 200,
            "trigger list: {trigger_list_response}"
        );
        assert_eq!(
            trigger_list_response["data"]["triggers"]
                .as_array()
                .map(Vec::len),
            Some(1)
        );
        assert!(trigger_list_response["data"]["triggers"][0]["secretHash"].is_null());

        let (trigger_update_status, trigger_update_response) = request_json_with_status(
            address,
            "PUT",
            &format!("/api/v1/adk/workflows/production-workflow/triggers/{trigger_id}"),
            Some(r#"{"type":"webhook","title":"Updated webhook","resetSecret":true}"#),
            &[("Authorization", authorization)],
        )
        .await;
        assert_eq!(
            trigger_update_status, 200,
            "trigger update: {trigger_update_response}"
        );
        assert_eq!(
            trigger_update_response["data"]["trigger"]["title"],
            "Updated webhook"
        );
        assert!(
            trigger_update_response["data"]["secret"]
                .as_str()
                .is_some_and(|value| !value.is_empty())
        );

        let (task_status, task_response) = request_json_with_status(
            address,
            "POST",
            "/api/v1/adk/tasks",
            Some(
                r#"{"id":"production-task","title":"Persist task","status":"TODO","dependsOn":[]}"#,
            ),
            &[("Authorization", authorization)],
        )
        .await;
        assert_eq!(task_status, 200, "task response: {task_response}");
        assert_eq!(task_response["data"]["id"], "production-task");
        let (task_update_status, task_update_response) = request_json_with_status(
            address,
            "PUT",
            "/api/v1/adk/tasks/production-task",
            Some(r#"{"status":"DONE","resultSummary":"finished"}"#),
            &[("Authorization", authorization)],
        )
        .await;
        assert_eq!(
            task_update_status, 200,
            "task update: {task_update_response}"
        );
        assert_eq!(task_update_response["data"]["status"], "DONE");
        assert_eq!(task_update_response["data"]["resultSummary"], "finished");

        let (memory_status, memory_response) = request_json_with_status(
            address,
            "POST",
            "/api/v1/adk/memory",
            Some(r#"{"scope":"workspace","key":"Risk Note","value":"persisted"}"#),
            &[("Authorization", authorization)],
        )
        .await;
        assert_eq!(memory_status, 200, "memory response: {memory_response}");
        let memory_id = memory_response["data"]["id"]
            .as_str()
            .expect("memory id")
            .to_owned();
        assert_eq!(memory_response["data"]["key"], "risk-note");

        let (memory_list_status, memory_list_response) = request_json_with_status(
            address,
            "GET",
            "/api/v1/adk/memory?scope=workspace&key=Risk%20Note",
            None,
            &[("Authorization", authorization)],
        )
        .await;
        assert_eq!(
            memory_list_status, 200,
            "memory list: {memory_list_response}"
        );
        assert_eq!(
            memory_list_response["data"]["entries"]
                .as_array()
                .map(Vec::len),
            Some(1)
        );

        let (trigger_delete_status, trigger_delete_response) = request_json_with_status(
            address,
            "DELETE",
            &format!("/api/v1/adk/workflows/production-workflow/triggers/{trigger_id}"),
            None,
            &[("Authorization", authorization)],
        )
        .await;
        assert_eq!(
            trigger_delete_status, 200,
            "trigger delete: {trigger_delete_response}"
        );
        assert_eq!(trigger_delete_response["data"]["deleted"], true);

        let (memory_delete_status, memory_delete_response) = request_json_with_status(
            address,
            "DELETE",
            &format!("/api/v1/adk/memory/{memory_id}"),
            None,
            &[("Authorization", authorization)],
        )
        .await;
        assert_eq!(
            memory_delete_status, 200,
            "memory delete: {memory_delete_response}"
        );
        assert_eq!(memory_delete_response["data"]["deleted"], true);

        handle.shutdown().await.expect("shutdown cleanly");
    }

    #[tokio::test]
    async fn production_adk_updates_do_not_create_missing_agents_or_providers() {
        let (_temp_dir, _settings_path, config, _security) = setup_test_env();
        let handle = start_product(config).await.expect("start product");
        let address = handle.startup_record().address;
        let authorization = "Bearer aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa";

        let (create_agent_status, create_agent_response) = request_json_with_status(
            address,
            "POST",
            "/api/v1/adk/agents",
            Some(r#"{"name":"Merge Agent","providerId":"","model":"model-a","status":"ENABLED"}"#),
            &[("Authorization", authorization)],
        )
        .await;
        assert_eq!(
            create_agent_status, 200,
            "agent create: {create_agent_response}"
        );
        let created_agent_id = create_agent_response["data"]["id"]
            .as_str()
            .expect("generated agent id")
            .to_owned();

        let (update_agent_status, update_agent_response) = request_json_with_status(
            address,
            "PUT",
            &format!("/api/v1/adk/agents/{created_agent_id}"),
            Some(r#"{"name":"Merged Agent"}"#),
            &[("Authorization", authorization)],
        )
        .await;
        assert_eq!(
            update_agent_status, 200,
            "agent partial update: {update_agent_response}"
        );
        assert_eq!(update_agent_response["data"]["name"], "Merged Agent");
        assert_eq!(update_agent_response["data"]["model"], "model-a");

        let (create_provider_status, create_provider_response) = request_json_with_status(
            address,
            "POST",
            "/api/v1/adk/providers",
            Some(r#"{"displayName":"Merge Provider","baseUrl":"https://example.test/v1","model":"provider-model","enabled":true}"#),
            &[("Authorization", authorization)],
        )
        .await;
        assert_eq!(
            create_provider_status, 200,
            "provider create: {create_provider_response}"
        );
        let created_provider_id = create_provider_response["data"]["id"]
            .as_str()
            .expect("generated provider id")
            .to_owned();

        let (update_provider_status, update_provider_response) = request_json_with_status(
            address,
            "PUT",
            &format!("/api/v1/adk/providers/{created_provider_id}"),
            Some(r#"{"displayName":"Merged Provider"}"#),
            &[("Authorization", authorization)],
        )
        .await;
        assert_eq!(
            update_provider_status, 200,
            "provider partial update: {update_provider_response}"
        );
        assert_eq!(
            update_provider_response["data"]["displayName"],
            "Merged Provider"
        );
        assert_eq!(
            update_provider_response["data"]["baseUrl"],
            "https://example.test/v1"
        );
        assert_eq!(update_provider_response["data"]["model"], "provider-model");

        let (agent_status, agent_response) = request_json_with_status(
            address,
            "PUT",
            "/api/v1/adk/agents/missing-agent",
            Some(r#"{"name":"should not be created"}"#),
            &[("Authorization", authorization)],
        )
        .await;
        assert_eq!(agent_status, 404, "agent update: {agent_response}");
        assert_eq!(agent_response["error"]["code"], "ADK_AGENT_NOT_FOUND");

        let (provider_status, provider_response) = request_json_with_status(
            address,
            "PUT",
            "/api/v1/adk/providers/missing-provider",
            Some(r#"{"name":"should not be created"}"#),
            &[("Authorization", authorization)],
        )
        .await;
        assert_eq!(provider_status, 404, "provider update: {provider_response}");
        assert_eq!(provider_response["error"]["code"], "ADK_PROVIDER_NOT_FOUND");

        let (agent_list_status, agent_list_response) = request_json_with_status(
            address,
            "GET",
            "/api/v1/adk/agents",
            None,
            &[("Authorization", authorization)],
        )
        .await;
        assert_eq!(agent_list_status, 200, "agent list: {agent_list_response}");
        assert!(
            !agent_list_response["data"]["agents"]
                .as_array()
                .expect("agent list")
                .iter()
                .any(|agent| agent["id"] == "missing-agent")
        );

        let (provider_list_status, provider_list_response) = request_json_with_status(
            address,
            "GET",
            "/api/v1/adk/providers",
            None,
            &[("Authorization", authorization)],
        )
        .await;
        assert_eq!(
            provider_list_status, 200,
            "provider list: {provider_list_response}"
        );
        assert!(
            !provider_list_response["data"]["providers"]
                .as_array()
                .expect("provider list")
                .iter()
                .any(|provider| provider["id"] == "missing-provider")
        );

        handle.shutdown().await.expect("shutdown cleanly");
    }

    #[tokio::test]
    async fn production_adk_approval_resolution_is_persisted_and_idempotent() {
        let (_temp_dir, settings_path, config, _security) = setup_test_env();
        let descriptors =
            product_data_management::managed_database_runtime_descriptors(&settings_path);
        let adk_path = &descriptors
            .iter()
            .find(|descriptor| descriptor.id == DATABASE_ADK)
            .expect("ADK descriptor")
            .path;
        let adk =
            AdkStore::open_existing(adk_path, ADK_PRODUCTION_PROFILE).expect("open ADK seed store");
        adk.create_run(CreateAdkRunParams {
            id: "production-approval-run",
            session_id: "production-approval-session",
            agent_id: "jftrade-default",
            status: "PENDING",
            client_request_id: "production-approval-request",
            request_fingerprint: "production-approval-fingerprint",
            payload_json: r#"{
                "id":"production-approval-run",
                "status":"PENDING",
                "toolCalls":[{"id":"approval-call","status":"PENDING_APPROVAL","requiresUser":true}],
                "pendingApprovals":[{"id":"production-approval","status":"PENDING","toolName":"market.write"}]
            }"#,
        })
        .expect("seed pending approval run");
        adk.create_approval(
            "production-approval",
            "production-approval-run",
            "jftrade-default",
            "PENDING",
            r#"{
                "id":"production-approval",
                "runId":"production-approval-run",
                "agentId":"jftrade-default",
                "toolName":"market.write",
                "status":"PENDING"
            }"#,
        )
        .expect("seed approval");
        drop(adk);

        let handle = start_product(config).await.expect("start product");
        let address = handle.startup_record().address;
        let authorization = "Bearer aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa";

        let (status, response) = request_json_with_status(
            address,
            "POST",
            "/api/v1/adk/approvals/production-approval/approve",
            None,
            &[("Authorization", authorization)],
        )
        .await;
        assert_eq!(status, 200, "approval response: {response}");
        assert_eq!(response["data"]["approval"]["id"], "production-approval");
        assert_eq!(response["data"]["approval"]["status"], "APPROVED");
        assert_eq!(response["data"]["run"]["id"], "production-approval-run");
        assert_eq!(response["data"]["run"]["status"], "RUNNING");
        assert_eq!(response["data"]["run"]["resumeState"], "approval_resuming");
        assert_eq!(response["data"]["run"]["toolCalls"][0]["status"], "RUNNING");
        assert_eq!(
            response["data"]["run"]["pendingApprovals"][0]["status"],
            "APPROVED"
        );

        let (repeat_status, repeat_response) = request_json_with_status(
            address,
            "POST",
            "/api/v1/adk/approvals/production-approval/approve",
            None,
            &[("Authorization", authorization)],
        )
        .await;
        assert_eq!(repeat_status, 200, "repeat approval: {repeat_response}");
        assert_eq!(repeat_response["data"]["approval"]["status"], "APPROVED");
        assert!(repeat_response["data"]["run"].is_null());

        let (missing_status, missing_response) = request_json_with_status(
            address,
            "POST",
            "/api/v1/adk/approvals/missing-approval/approve",
            None,
            &[("Authorization", authorization)],
        )
        .await;
        assert_eq!(missing_status, 200, "missing approval: {missing_response}");
        assert_eq!(missing_response["data"]["approval"]["id"], "");

        handle.shutdown().await.expect("shutdown cleanly");
    }

    #[tokio::test]
    async fn production_adk_goal_pause_and_resume_are_persisted_atomically() {
        let (_temp_dir, settings_path, config, _security) = setup_test_env();
        let descriptors =
            product_data_management::managed_database_runtime_descriptors(&settings_path);
        let adk_path = &descriptors
            .iter()
            .find(|descriptor| descriptor.id == DATABASE_ADK)
            .expect("ADK descriptor")
            .path;
        let adk =
            AdkStore::open_existing(adk_path, ADK_PRODUCTION_PROFILE).expect("open ADK seed store");
        adk.create_run(CreateAdkRunParams {
            id: "production-pause-run",
            session_id: "pause-session",
            agent_id: "jftrade-default",
            status: "RUNNING",
            client_request_id: "pause-request",
            request_fingerprint: "pause-fingerprint",
            payload_json: r#"{
                "id":"production-pause-run",
                "status":"RUNNING",
                "workMode":"loop",
                "workflowStatus":"RUNNING",
                "message":"running",
                "toolCalls":[],
                "pendingApprovals":[]
            }"#,
        })
        .expect("seed running goal");
        adk.create_run(CreateAdkRunParams {
            id: "production-resume-run",
            session_id: "resume-session",
            agent_id: "jftrade-default",
            status: "PAUSED",
            client_request_id: "resume-request",
            request_fingerprint: "resume-fingerprint",
            payload_json: r#"{
                "id":"production-resume-run",
                "status":"PAUSED",
                "workMode":"loop",
                "workflowStatus":"PAUSED",
                "pausedReason":"user",
                "resumeState":"user_paused",
                "message":"目标已暂停。",
                "toolCalls":[],
                "pendingApprovals":[]
            }"#,
        })
        .expect("seed paused goal");
        adk.create_run(CreateAdkRunParams {
            id: "production-cancel-run",
            session_id: "cancel-session",
            agent_id: "jftrade-default",
            status: "RUNNING",
            client_request_id: "cancel-request",
            request_fingerprint: "cancel-fingerprint",
            payload_json: r#"{
                "id":"production-cancel-run",
                "status":"RUNNING",
                "workMode":"loop",
                "workflowStatus":"RUNNING",
                "message":"running",
                "toolCalls":[{"id":"call-1","status":"RUNNING","requiresUser":true}],
                "pendingApprovals":[{"id":"approval-1","status":"PENDING"}]
            }"#,
        })
        .expect("seed cancellable goal");
        drop(adk);

        let handle = start_product(config.clone()).await.expect("start product");
        let address = handle.startup_record().address;
        let authorization = "Bearer aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa";

        let (pause_status, pause_response) = request_json_with_status(
            address,
            "POST",
            "/api/v1/adk/runs/production-pause-run/pause",
            None,
            &[("Authorization", authorization)],
        )
        .await;
        assert_eq!(pause_status, 200, "pause response: {pause_response}");
        assert_eq!(pause_response["data"]["status"], "RUNNING");
        assert_eq!(
            pause_response["data"]["resumeState"],
            "user_pause_requested"
        );
        assert!(pause_response["data"]["pauseRequestedAt"].is_string());

        let (cancel_status, cancel_response) = request_json_with_status(
            address,
            "POST",
            "/api/v1/adk/runs/production-cancel-run/cancel",
            None,
            &[("Authorization", authorization)],
        )
        .await;
        assert_eq!(cancel_status, 200, "cancel response: {cancel_response}");
        assert_eq!(cancel_response["data"]["status"], "CANCELLED");
        assert_eq!(cancel_response["data"]["errorCode"], "RUN_CANCELLED");
        assert_eq!(
            cancel_response["data"]["toolCalls"][0]["status"],
            "CANCELLED"
        );
        assert_eq!(cancel_response["data"]["pendingApprovals"], json!([]));

        let (repeat_status, repeat_response) = request_json_with_status(
            address,
            "POST",
            "/api/v1/adk/runs/production-pause-run/pause",
            None,
            &[("Authorization", authorization)],
        )
        .await;
        assert_eq!(
            repeat_status, 200,
            "repeat pause response: {repeat_response}"
        );
        assert_eq!(
            repeat_response["data"]["pauseRequestedAt"],
            pause_response["data"]["pauseRequestedAt"]
        );
        handle.shutdown().await.expect("shutdown cleanly");

        let handle = start_product(config).await.expect("restart product");
        let address = handle.startup_record().address;
        let (resume_status, resume_response) = request_json_with_status(
            address,
            "POST",
            "/api/v1/adk/runs/production-resume-run/resume",
            None,
            &[("Authorization", authorization)],
        )
        .await;
        assert_eq!(resume_status, 200, "resume response: {resume_response}");
        assert_eq!(resume_response["data"]["status"], "RUNNING");
        assert_eq!(resume_response["data"]["workflowStatus"], "RUNNING");
        assert_eq!(resume_response["data"]["resumeState"], "user_resuming");
        assert!(resume_response["data"]["pauseRequestedAt"].is_null());
        assert!(resume_response["data"]["pausedAt"].is_null());
        assert!(resume_response["data"]["pausedReason"].is_null());
        handle.shutdown().await.expect("shutdown after resume");
    }

    #[test]
    fn production_adk_metrics_are_aggregated_from_persisted_records() {
        let (_temp_dir, settings_path, config, security) = setup_test_env();
        let descriptors =
            product_data_management::managed_database_runtime_descriptors(&settings_path);
        let adk_path = &descriptors
            .iter()
            .find(|descriptor| descriptor.id == DATABASE_ADK)
            .expect("ADK descriptor")
            .path;
        let adk =
            AdkStore::open_existing(adk_path, ADK_PRODUCTION_PROFILE).expect("open ADK seed store");
        adk.upsert_agent(
            "metrics-agent",
            r#"{"id":"metrics-agent","providerId":"metrics-provider","status":"ENABLED"}"#,
        )
        .expect("seed agent");
        adk.create_run(CreateAdkRunParams {
            id: "metrics-run",
            session_id: "metrics-session",
            agent_id: "metrics-agent",
            status: "FAILED",
            client_request_id: "metrics-request",
            request_fingerprint: "metrics-fingerprint",
            payload_json: r#"{
                "id":"metrics-run",
                "status":"FAILED",
                "resumeState":"adk_confirmation_resolved",
                "errorCode":"RUN_ORPHANED",
                "usage":{"tokensIn":120,"tokensOut":30},
                "toolCalls":[
                    {"toolName":"market.read","status":"SUCCEEDED","durationMs":20,"output":{"value":1}},
                    {"toolName":"strategy.write","status":"FAILED","durationMs":40,"output":{"truncated":true,"error":{"code":"BROKER_DOWN","retryable":true}}}
                ]
            }"#,
        })
        .expect("seed run");
        adk.upsert_session(
            "metrics-session",
            "metrics-agent",
            r#"{"id":"metrics-session","agentId":"metrics-agent","title":"Metrics session"}"#,
        )
        .expect("seed session");
        adk.create_approval(
            "metrics-approval",
            "metrics-run",
            "metrics-agent",
            "PENDING",
            r#"{"functionCallId":"call-1","confirmationCallId":"confirm-1"}"#,
        )
        .expect("seed approval");
        adk.upsert_workflow(
            "metrics-workflow",
            "ENABLED",
            r#"{"id":"metrics-workflow","status":"ENABLED"}"#,
        )
        .expect("seed workflow");
        adk.upsert_workflow_trigger(
            "metrics-trigger",
            "metrics-workflow",
            "MANUAL",
            "ENABLED",
            "",
            r#"{"id":"metrics-trigger","type":"MANUAL","status":"ENABLED"}"#,
        )
        .expect("seed trigger");
        drop(adk);

        let ports = production_ports(&config, &security).expect("production ports");
        let metrics = match ports
            .adk_read
            .read("/api/v1/adk/metrics", "ignored=true")
            .expect("read metrics")
        {
            crate::product::AdkReadSnapshot::Json(metrics) => metrics,
            crate::product::AdkReadSnapshot::Stream(_) => panic!("expected JSON metrics"),
        };
        assert_eq!(metrics["runs"]["total"], 1);
        assert_eq!(metrics["runs"]["last7Days"], 1);
        assert_eq!(metrics["runs"]["byStatus"]["FAILED"], 1);
        assert_eq!(metrics["runs"]["byAgent"]["metrics-agent"], 1);
        assert_eq!(metrics["runs"]["byProvider"]["metrics-provider"], 1);
        assert_eq!(metrics["runs"]["lifecycle"]["failed"], 1);
        assert_eq!(metrics["runs"]["lifecycle"]["resumed"], 1);
        assert_eq!(metrics["runs"]["lifecycle"]["orphaned"], 1);
        assert_eq!(metrics["tools"]["total"], 2);
        assert_eq!(metrics["tools"]["successful"], 1);
        assert_eq!(metrics["tools"]["averageDurationMs"], 30);
        assert_eq!(metrics["tools"]["errorCount"], 1);
        assert_eq!(metrics["tools"]["retryableErrors"], 1);
        assert_eq!(metrics["tools"]["truncated"], 1);
        assert_eq!(metrics["tools"]["byErrorCode"]["BROKER_DOWN"], 1);
        assert_eq!(metrics["usage"]["samples"], 1);
        assert_eq!(metrics["usage"]["tokensInTotal"], 120);
        assert_eq!(metrics["usage"]["tokensOutAverage"], 30);
        assert_eq!(metrics["approvals"]["pending"], 1);
        assert_eq!(metrics["approvals"]["recoverablePending"], 1);
        assert_eq!(metrics["sessions"]["total"], 1);
        assert_eq!(metrics["workflows"]["definitions"], 1);
        assert_eq!(metrics["workflows"]["enabledDefinitions"], 1);
        assert_eq!(metrics["workflows"]["triggers"], 1);
        assert_eq!(metrics["workflows"]["enabledTriggers"], 1);
    }

    #[tokio::test]
    async fn production_auth_session_and_live_websocket_integration() {
        let (_temp_dir, _settings_path, config, _security) = setup_test_env();
        let handle = start_product(config).await.expect("start product");
        let address = handle.startup_record().address;

        use tokio::io::{AsyncReadExt, AsyncWriteExt};

        // 1. Unauthenticated WebSocket handshake should be rejected
        let mut unauth_stream = tokio::net::TcpStream::connect(address)
            .await
            .expect("connect websocket");
        let unauth_request = format!(
            "GET /api/v1/ws/live HTTP/1.1\r\nHost: {address}\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Key: dGhlIHNhbXBsZSBub25jZQ==\r\nSec-WebSocket-Version: 13\r\n\r\n"
        );
        unauth_stream
            .write_all(unauth_request.as_bytes())
            .await
            .expect("write unauth handshake");
        let mut unauth_response = Vec::new();
        while !unauth_response.ends_with(b"\r\n\r\n") {
            let mut byte = [0_u8; 1];
            unauth_stream
                .read_exact(&mut byte)
                .await
                .expect("read unauth handshake");
            unauth_response.push(byte[0]);
        }
        let unauth_text = String::from_utf8_lossy(&unauth_response);
        assert!(unauth_text.contains("401 Unauthorized"));
        drop(unauth_stream);

        // 2. Authenticated with Desktop Bearer Token -> 101 Switching Protocols
        let mut auth_stream = tokio::net::TcpStream::connect(address)
            .await
            .expect("connect websocket");
        let token = "a".repeat(32);
        let auth_request = format!(
            "GET /api/v1/ws/live HTTP/1.1\r\nHost: {address}\r\nAuthorization: Bearer {token}\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Key: dGhlIHNhbXBsZSBub25jZQ==\r\nSec-WebSocket-Version: 13\r\n\r\n"
        );
        auth_stream
            .write_all(auth_request.as_bytes())
            .await
            .expect("write auth handshake");

        let mut auth_response = Vec::new();
        while !auth_response.ends_with(b"\r\n\r\n") {
            let mut byte = [0_u8; 1];
            auth_stream
                .read_exact(&mut byte)
                .await
                .expect("read auth handshake");
            auth_response.push(byte[0]);
        }
        let auth_text = String::from_utf8_lossy(&auth_response);
        assert!(auth_text.contains("101 Switching Protocols"));
        let heartbeat = read_server_text_frame(&mut auth_stream).await;
        assert_eq!(heartbeat["type"], "heartbeat");
        assert_eq!(heartbeat["payload"]["stale"], true);

        auth_stream
            .write_all(&masked_text_frame(
                br#"{"type":"subscribe","subscriptions":{"providerBrokerId":"futu","activeInstruments":[" us.aapl "]}}"#,
            ))
            .await
            .expect("write live subscription");
        let live_hub = handle.live_hub();
        wait_for_live_hub_subscription(&live_hub, "US.AAPL").await;
        assert!(live_hub.publish(json!({
            "type": "tick",
            "source": "futu",
            "payload": {"providerBrokerId": "other", "instrumentId": "US.AAPL", "price": 1}
        })));
        assert!(live_hub.publish(json!({
            "type": "tick",
            "source": "futu",
            "payload": {"providerBrokerId": "futu", "instrumentId": "US.MSFT", "price": 2}
        })));
        assert!(live_hub.publish(json!({
            "type": "tick",
            "source": "futu",
            "payload": {"providerBrokerId": "futu", "instrumentId": "US.AAPL", "price": 3}
        })));
        let tick = tokio::time::timeout(std::time::Duration::from_secs(2), async {
            loop {
                let event = read_server_text_frame(&mut auth_stream).await;
                if event["type"] == "tick" {
                    break event;
                }
            }
        })
        .await
        .expect("read live tick before timeout");
        assert_eq!(tick["type"], "tick");
        assert_eq!(tick["payload"]["instrumentId"], "US.AAPL");
        assert_eq!(tick["payload"]["price"], 3);

        drop(auth_stream);
        handle.shutdown().await.expect("shutdown cleanly");
    }

    #[tokio::test]
    async fn production_backtest_sync_registry_distinguishes_missing_task_from_runtime_failure() {
        let (_temp_dir, _settings_path, config, _security) = setup_test_env();
        let handle = start_product(config).await.expect("start product");
        let address = handle.startup_record().address;
        let token = "a".repeat(32);

        let (read_status, read_response) = request_json_with_status(
            address,
            "GET",
            "/api/v1/backtests/sync/missing",
            None,
            &[("Authorization", &format!("Bearer {token}"))],
        )
        .await;
        assert_eq!(read_status, 404, "sync read response: {read_response}");
        assert_eq!(read_response["error"]["code"], "NOT_FOUND");

        let (start_status, start_response) = request_json_with_status(
            address,
            "POST",
            "/api/v1/backtests/sync",
            Some("{}"),
            &[("Authorization", &format!("Bearer {token}"))],
        )
        .await;
        assert_eq!(start_status, 503, "sync start response: {start_response}");
        assert_eq!(
            start_response["error"]["code"],
            "BACKTESTS_WRITE_UNAVAILABLE"
        );

        let (cancel_status, cancel_response) = request_json_with_status(
            address,
            "DELETE",
            "/api/v1/backtests/sync/missing",
            None,
            &[("Authorization", &format!("Bearer {token}"))],
        )
        .await;
        assert_eq!(
            cancel_status, 503,
            "sync cancel response: {cancel_response}"
        );
        assert_eq!(
            cancel_response["error"]["code"],
            "BACKTESTS_WRITE_UNAVAILABLE"
        );

        handle.shutdown().await.expect("shutdown cleanly");
    }

    #[tokio::test]
    async fn production_real_trade_controls_persist_while_opend_retry_stays_unavailable() {
        let (_temp_dir, _settings_path, config, _security) = setup_test_env();
        let restart_config = config.clone();
        let handle = start_product(config).await.expect("start product");
        let address = handle.startup_record().address;
        let token = "a".repeat(32);
        let authorization = format!("Bearer {token}");

        let (update_status, update_response) = request_json_with_status(
            address,
            "PUT",
            "/api/v1/system/real-trade-risk-limits",
            Some(r#"{"realTradingEnabled":true,"maxOrderQuantity":10,"operatorId":"tester"}"#),
            &[("Authorization", &authorization)],
        )
        .await;
        assert_eq!(
            update_status, 200,
            "risk update response: {update_response}"
        );
        assert_eq!(update_response["data"]["realTradingEnabled"], true);
        assert_eq!(
            update_response["data"]["effectiveMaxOrderQuantity"].as_f64(),
            Some(10.0)
        );

        let (retry_status, retry_response) = request_json_with_status(
            address,
            "POST",
            "/api/v1/system/futu-opend/manual-retry",
            Some("{}"),
            &[("Authorization", &authorization)],
        )
        .await;
        assert_eq!(retry_status, 503, "OpenD retry response: {retry_response}");
        assert_eq!(retry_response["error"]["code"], "SYSTEM_WRITE_UNAVAILABLE");

        handle.shutdown().await.expect("shutdown cleanly");

        let restarted = start_product(restart_config)
            .await
            .expect("restart product");
        let (read_status, read_response) = request_json_with_status(
            restarted.startup_record().address,
            "GET",
            "/api/v1/system/real-trade-risk-limits",
            None,
            &[("Authorization", &authorization)],
        )
        .await;
        assert_eq!(read_status, 200, "risk read response: {read_response}");
        assert_eq!(read_response["data"]["realTradingEnabled"], true);
        assert_eq!(
            read_response["data"]["runtimeConfiguredMaxOrderQuantity"].as_f64(),
            Some(10.0)
        );
        restarted.shutdown().await.expect("shutdown cleanly");
    }

    #[tokio::test]
    async fn production_startup_preserves_corrupt_real_trade_control_state() {
        let (_temp_dir, _settings_path, config, _security) = setup_test_env();
        fs::write(config.real_trade_control_path(), b"{").expect("write corrupt real-trade state");

        let result = start_product(config.clone()).await;
        assert!(matches!(
            result,
            Err(ProductError::Storage(message))
                if message.contains("failed to open real-trade production control plane")
        ));
        assert_eq!(
            fs::read(config.real_trade_control_path()).expect("read corrupt state"),
            b"{"
        );
    }

    async fn read_server_text_frame(stream: &mut tokio::net::TcpStream) -> serde_json::Value {
        use tokio::io::AsyncReadExt;

        let mut header = [0_u8; 2];
        stream
            .read_exact(&mut header)
            .await
            .expect("read websocket frame header");
        assert_eq!(header[0] & 0x0f, 1, "expected a text frame");
        assert_eq!(header[1] & 0x80, 0, "server frames must not be masked");
        let mut length = u64::from(header[1] & 0x7f);
        if length == 126 {
            let mut extended = [0_u8; 2];
            stream
                .read_exact(&mut extended)
                .await
                .expect("read websocket 16-bit length");
            length = u64::from(u16::from_be_bytes(extended));
        } else if length == 127 {
            let mut extended = [0_u8; 8];
            stream
                .read_exact(&mut extended)
                .await
                .expect("read websocket 64-bit length");
            length = u64::from_be_bytes(extended);
        }
        let length = usize::try_from(length).expect("websocket frame length");
        let mut payload = vec![0_u8; length];
        stream
            .read_exact(&mut payload)
            .await
            .expect("read websocket frame payload");
        serde_json::from_slice(&payload).expect("websocket heartbeat json")
    }

    async fn wait_for_live_hub_subscription(hub: &jftrade_api::LiveHub, instrument: &str) {
        for _ in 0..100 {
            let snapshot = hub.snapshot();
            if snapshot
                .active_instruments
                .iter()
                .any(|value| value == instrument)
            {
                return;
            }
            tokio::time::sleep(std::time::Duration::from_millis(5)).await;
        }
        panic!("live hub subscription was not registered");
    }

    fn masked_text_frame(payload: &[u8]) -> Vec<u8> {
        assert!(payload.len() < 126, "test payload must use the short frame");
        let mask = [0x12_u8, 0x34, 0x56, 0x78];
        let mut frame = Vec::with_capacity(payload.len() + 6);
        frame.push(0x81);
        frame.push(0x80 | payload.len() as u8);
        frame.extend_from_slice(&mask);
        frame.extend(
            payload
                .iter()
                .enumerate()
                .map(|(index, byte)| byte ^ mask[index % mask.len()]),
        );
        frame
    }

    #[tokio::test]
    async fn every_canonical_production_route_has_a_dispatch_adapter() {
        let (_temp_dir, _settings_path, config, _security) = setup_test_env();
        let handle = start_product(config).await.expect("start product");
        let address = handle.startup_record().address;
        let token = "a".repeat(32);
        let ledger: serde_json::Value = serde_json::from_str(include_str!(
            "../../../tests/fixtures/rust-migration/stage9/route-ownership.json"
        ))
        .expect("canonical route ledger");

        for operation in ledger["operations"].as_array().expect("operations") {
            let method = operation["method"].as_str().expect("method");
            let template = operation["path"].as_str().expect("path");
            if template == "/api/v1/ws/live" {
                continue;
            }
            let path = template
                .split('/')
                .map(|segment| {
                    if segment.starts_with('{') && segment.ends_with('}') {
                        "test-id"
                    } else {
                        segment
                    }
                })
                .collect::<Vec<_>>()
                .join("/");
            let body = if method == "GET" { "" } else { "{}" };
            let mut stream = tokio::net::TcpStream::connect(address)
                .await
                .expect("connect production API");
            let request = format!(
                "{method} {path} HTTP/1.1\r\nHost: {address}\r\nAuthorization: Bearer {token}\r\nConnection: close\r\nContent-Type: application/json\r\nContent-Length: {}\r\n\r\n{body}",
                body.len()
            );
            use tokio::io::{AsyncReadExt, AsyncWriteExt};
            stream
                .write_all(request.as_bytes())
                .await
                .expect("write route request");
            let mut headers = Vec::new();
            while !headers.ends_with(b"\r\n\r\n") {
                let mut byte = [0_u8; 1];
                stream
                    .read_exact(&mut byte)
                    .await
                    .expect("read route response headers");
                headers.push(byte[0]);
                assert!(headers.len() < 32 * 1024, "response headers too large");
            }
            let response = String::from_utf8_lossy(&headers);
            let status = response
                .split_whitespace()
                .nth(1)
                .and_then(|value| value.parse::<u16>().ok())
                .expect("HTTP status");
            if let Some(content_length) = response.lines().find_map(|line| {
                let (name, value) = line.split_once(':')?;
                name.eq_ignore_ascii_case("content-length")
                    .then(|| value.trim().parse::<usize>().ok())
                    .flatten()
            }) {
                let mut body_bytes = vec![0_u8; content_length];
                stream
                    .read_exact(&mut body_bytes)
                    .await
                    .expect("read route response body");
                let body = String::from_utf8_lossy(&body_bytes);
                assert!(
                    !body.contains("ROUTE_REGISTRY_INVARIANT"),
                    "canonical route {method} {path} has no dispatch adapter: {body}"
                );
            }
            assert_ne!(
                status, 501,
                "canonical route {method} {path} fell through to an unimplemented adapter"
            );
        }

        handle.shutdown().await.expect("shutdown cleanly");
    }

    #[test]
    fn capability_matrix_derives_correct_route_readiness_for_all_four_combinations() {
        use crate::product::product_production_ports::{
            MarketDataCapabilityMatrix, ProductionAdapterBinding, production_adapter_bindings,
        };
        use crate::product::product_production_route_registry::ProductionRouteAdapter as Adapter;

        // 1. Futu with router
        let futu_matrix = MarketDataCapabilityMatrix::new(Some("futu"), false, true);
        let futu_bindings = production_adapter_bindings(&futu_matrix);
        assert_eq!(
            futu_bindings.get(&Adapter::MarketDataSearchRead),
            Some(&ProductionAdapterBinding::ExternalUnavailable)
        );
        assert_eq!(
            futu_bindings.get(&Adapter::MarketDataCandlesRead),
            Some(&ProductionAdapterBinding::ExternalUnavailable)
        );
        assert_eq!(
            futu_bindings.get(&Adapter::MarketDataSnapshotsRead),
            Some(&ProductionAdapterBinding::Ready)
        );
        assert_eq!(
            futu_bindings.get(&Adapter::MarketDataMarketsRead),
            Some(&ProductionAdapterBinding::Ready)
        );
        assert_eq!(
            futu_bindings.get(&Adapter::MarketDataSecuritiesRead),
            Some(&ProductionAdapterBinding::ExternalUnavailable)
        );
        assert_eq!(
            futu_bindings.get(&Adapter::MarketDataSubscriptionAcquireWrite),
            Some(&ProductionAdapterBinding::Ready)
        );
        assert_eq!(
            futu_bindings.get(&Adapter::MarketDataSubscriptionRead),
            Some(&ProductionAdapterBinding::Ready)
        );
        assert_eq!(
            futu_bindings.get(&Adapter::MarketDataInstrumentsNormalizeWrite),
            Some(&ProductionAdapterBinding::Ready)
        );

        // 2. Yfinance with helper
        let yf_matrix = MarketDataCapabilityMatrix::new(Some("yfinance"), true, false);
        let yf_bindings = production_adapter_bindings(&yf_matrix);
        assert_eq!(
            yf_bindings.get(&Adapter::MarketDataSearchRead),
            Some(&ProductionAdapterBinding::Ready)
        );
        assert_eq!(
            yf_bindings.get(&Adapter::MarketDataCandlesRead),
            Some(&ProductionAdapterBinding::Ready)
        );
        assert_eq!(
            yf_bindings.get(&Adapter::MarketDataSnapshotsRead),
            Some(&ProductionAdapterBinding::Ready)
        );
        assert_eq!(
            yf_bindings.get(&Adapter::MarketDataMarketsRead),
            Some(&ProductionAdapterBinding::Ready)
        );
        assert_eq!(
            yf_bindings.get(&Adapter::MarketDataSecuritiesRead),
            Some(&ProductionAdapterBinding::Ready)
        );
        assert_eq!(
            yf_bindings.get(&Adapter::MarketDataSubscriptionAcquireWrite),
            Some(&ProductionAdapterBinding::Ready)
        );
        assert_eq!(
            yf_bindings.get(&Adapter::MarketDataSubscriptionRead),
            Some(&ProductionAdapterBinding::Ready)
        );

        // 3. AKShare with helper
        let ak_matrix = MarketDataCapabilityMatrix::new(Some("akshare"), true, false);
        let ak_bindings = production_adapter_bindings(&ak_matrix);
        assert_eq!(
            ak_bindings.get(&Adapter::MarketDataSearchRead),
            Some(&ProductionAdapterBinding::Ready)
        );
        assert_eq!(
            ak_bindings.get(&Adapter::MarketDataCandlesRead),
            Some(&ProductionAdapterBinding::Ready)
        );
        assert_eq!(
            ak_bindings.get(&Adapter::MarketDataSubscriptionAcquireWrite),
            Some(&ProductionAdapterBinding::Ready)
        );

        // 4. Unconfigured / None
        let none_matrix = MarketDataCapabilityMatrix::new(None, false, false);
        let none_bindings = production_adapter_bindings(&none_matrix);
        assert_eq!(
            none_bindings.get(&Adapter::MarketDataSearchRead),
            Some(&ProductionAdapterBinding::ExternalUnavailable)
        );
        assert_eq!(
            none_bindings.get(&Adapter::MarketDataCandlesRead),
            Some(&ProductionAdapterBinding::ExternalUnavailable)
        );
        assert_eq!(
            none_bindings.get(&Adapter::MarketDataSnapshotsRead),
            Some(&ProductionAdapterBinding::ExternalUnavailable)
        );
        assert_eq!(
            none_bindings.get(&Adapter::MarketDataSubscriptionAcquireWrite),
            Some(&ProductionAdapterBinding::ExternalUnavailable)
        );
        assert_eq!(
            none_bindings.get(&Adapter::MarketDataInstrumentsNormalizeWrite),
            Some(&ProductionAdapterBinding::Ready)
        );
    }

    #[tokio::test]
    async fn candles_and_search_validation_rules() {
        let (_temp_dir, settings_path, _config, _security) = setup_test_env();
        fs::write(
            &settings_path,
            br#"{"activeMarketDataProvider":"yfinance"}"#,
        )
        .expect("write settings");

        let active_state = Arc::new(crate::product::ActiveProviderState::new(Some(
            jftrade_settings::MarketDataProvider::Yfinance,
        )));
        let catalog =
            crate::product::product_production_ports::ProductionMarketDataCatalogPort::new(
                active_state.clone(),
                None,
            );
        use crate::product::MarketDataCatalogReadSnapshotPort;

        // Missing query
        let err_no_query = catalog
            .read("/api/v1/market-data/instruments", "")
            .await
            .unwrap_err();
        assert!(
            matches!(err_no_query, crate::product::MarketDataCatalogReadSnapshotError::Invalid { code, .. } if code == "MARKET_INSTRUMENT_INVALID")
        );

        // Invalid URL escape in search query
        let err_bad_escape = catalog
            .read("/api/v1/market-data/instruments", "query=%zz")
            .await
            .unwrap_err();
        assert!(
            matches!(err_bad_escape, crate::product::MarketDataCatalogReadSnapshotError::Invalid { code, .. } if code == "MARKET_INSTRUMENT_INVALID")
        );

        // Invalid market
        let err_bad_mkt = catalog
            .read(
                "/api/v1/market-data/instruments",
                "query=AAPL&market=INVALID",
            )
            .await
            .unwrap_err();
        assert!(
            matches!(err_bad_mkt, crate::product::MarketDataCatalogReadSnapshotError::Invalid { code, .. } if code == "MARKET_INSTRUMENT_INVALID")
        );

        // Invalid limit
        let err_bad_lim = catalog
            .read("/api/v1/market-data/instruments", "query=AAPL&limit=500")
            .await
            .unwrap_err();
        assert!(
            matches!(err_bad_lim, crate::product::MarketDataCatalogReadSnapshotError::Invalid { code, .. } if code == "MARKET_INSTRUMENT_INVALID")
        );

        let quote = crate::product::product_production_ports::ProductionMarketDataQuotePort::new(
            active_state,
            None,
            None,
            None,
        );
        use crate::product::MarketDataQuoteReadSnapshotPort;

        // Invalid period (including 1y)
        let err_bad_period = quote
            .read("/api/v1/market-data/candles/US/AAPL", "period=999x")
            .await
            .unwrap_err();
        assert!(matches!(
            err_bad_period,
            crate::product::MarketDataQuoteReadSnapshotError::Failed { status: 400, .. }
        ));

        let err_1y = quote
            .read("/api/v1/market-data/candles/US/AAPL", "period=1y")
            .await
            .unwrap_err();
        assert!(matches!(
            err_1y,
            crate::product::MarketDataQuoteReadSnapshotError::Failed { status: 400, .. }
        ));

        // Invalid session tokens
        let err_bad_session = quote
            .read(
                "/api/v1/market-data/candles/US/AAPL",
                "period=1d&sessions=all",
            )
            .await
            .unwrap_err();
        assert!(matches!(
            err_bad_session,
            crate::product::MarketDataQuoteReadSnapshotError::Failed { status: 400, code, .. } if code == "BAD_REQUEST"
        ));

        let err_empty_session = quote
            .read("/api/v1/market-data/candles/US/AAPL", "period=1d&sessions=")
            .await
            .unwrap_err();
        assert!(matches!(
            err_empty_session,
            crate::product::MarketDataQuoteReadSnapshotError::Failed { status: 400, code, .. } if code == "BAD_REQUEST"
        ));

        // Invalid RFC3339 from
        let err_bad_rfc = quote
            .read(
                "/api/v1/market-data/candles/US/AAPL",
                "period=1d&from=not-a-date",
            )
            .await
            .unwrap_err();
        assert!(matches!(
            err_bad_rfc,
            crate::product::MarketDataQuoteReadSnapshotError::Failed { status: 400, .. }
        ));

        // Tick with before (unsupported historical pagination)
        let err_tick_before = quote
            .read(
                "/api/v1/market-data/candles/US/AAPL",
                "period=tick&before=2026-08-28T00:00:00Z",
            )
            .await
            .unwrap_err();
        assert!(matches!(
            err_tick_before,
            crate::product::MarketDataQuoteReadSnapshotError::Failed { status: 400, .. }
        ));

        // Invalid before + from combination
        let err_combo = quote
            .read(
                "/api/v1/market-data/candles/US/AAPL",
                "period=1d&from=2026-01-01T00:00:00Z&before=2026-02-01T00:00:00Z",
            )
            .await
            .unwrap_err();
        assert!(matches!(
            err_combo,
            crate::product::MarketDataQuoteReadSnapshotError::Failed { status: 400, .. }
        ));

        // Standard GET subscriptions does not include transport mode
        let sub_resp = quote
            .read("/api/v1/market-data/subscriptions", "")
            .await
            .unwrap();
        assert!(sub_resp["transport"].is_null());
    }

    #[tokio::test]
    async fn subscription_heartbeat_and_helper_mutations() {
        let (_temp_dir, settings_path, _config, _security) = setup_test_env();
        fs::write(&settings_path, br#"{"activeMarketDataProvider":"futu"}"#)
            .expect("write settings");

        let active_state = Arc::new(crate::product::ActiveProviderState::new(Some(
            jftrade_settings::MarketDataProvider::Futu,
        )));
        let router = Arc::new(std::sync::Mutex::new(
            jftrade_marketdata::ProviderRouter::new(100),
        ));
        let sub_port = crate::product::product_production_ports::ProductionMarketDataSubscriptionMutationPort::new(
            active_state,
            Some(router.clone()),
            None,
        );
        use crate::product::product_market_data_subscription_mutation_port::{
            MarketDataSubscriptionMutationPort, MarketDataSubscriptionMutationRequest,
        };

        // Missing consumer heartbeat returns 200 snapshot (not 404!)
        let hb_req = MarketDataSubscriptionMutationRequest {
            method: "POST".to_owned(),
            path: "/api/v1/market-data/subscriptions/heartbeat".to_owned(),
            query: String::new(),
            body: serde_json::to_vec(&json!({
                "consumerId": "non-existent-consumer",
                "providerBrokerId": "futu"
            }))
            .unwrap(),
        };
        let hb_res = sub_port
            .dispatch(&hb_req)
            .expect("heartbeat should return 200 snapshot");
        assert_eq!(hb_res["desiredCount"], 0);

        // Polling mode mutation with explicit non-futu broker ID
        let acquire_polling_req = MarketDataSubscriptionMutationRequest {
            method: "POST".to_owned(),
            path: "/api/v1/market-data/subscriptions".to_owned(),
            query: String::new(),
            body: serde_json::to_vec(&json!({
                "consumerId": "chart-main",
                "providerBrokerId": "yfinance",
                "instruments": [{"market": "US", "symbol": "AAPL"}]
            }))
            .unwrap(),
        };
        let acquire_res = sub_port
            .dispatch(&acquire_polling_req)
            .expect("acquire with providerBrokerId should succeed");
        assert_eq!(acquire_res["action"], "acquired");
        assert_eq!(acquire_res["transport"]["mode"], "snapshot-poll-fallback");

        // Standard clear returns cleared: true
        let clear_req = MarketDataSubscriptionMutationRequest {
            method: "DELETE".to_owned(),
            path: "/api/v1/market-data/subscriptions".to_owned(),
            query: String::new(),
            body: vec![],
        };
        let clear_res = sub_port
            .dispatch(&clear_req)
            .expect("clear in helper mode should succeed");
        assert_eq!(clear_res["cleared"], true);
        assert!(clear_res["transport"].is_null());
    }

    #[tokio::test]
    async fn canonical_278_routes_table_driven_reachability_and_auth_matrix() {
        let (_temp_dir, _settings_path, config, security) = setup_test_env();
        let bindings = {
            let ports = production_ports(&config, &security).expect("production ports");
            let registry =
                crate::product::product_production_route_registry::ProductionRouteRegistry::bind(
                    &ports,
                )
                .expect("bind all routes");
            assert_eq!(registry.bindings().len(), 278);
            registry.bindings().to_vec()
        };

        let handle = start_product(config).await.expect("start product");
        let address = handle.startup_record().address;
        let token = "a".repeat(32);

        use tokio::io::{AsyncReadExt, AsyncWriteExt};

        async fn execute_http(
            address: SocketAddr,
            method: &str,
            path: &str,
            token: Option<&str>,
        ) -> u16 {
            let mut stream = tokio::net::TcpStream::connect(address)
                .await
                .expect("connect");
            let auth_header = match token {
                Some(tok) => format!("Authorization: Bearer {tok}\r\n"),
                None => String::new(),
            };
            let body = match method {
                "POST" | "PUT" | "PATCH" => "{}",
                _ => "",
            };
            let content_headers = if !body.is_empty() {
                format!(
                    "Content-Type: application/json\r\nContent-Length: {}\r\n",
                    body.len()
                )
            } else {
                String::new()
            };
            let req = format!(
                "{method} {path} HTTP/1.1\r\nHost: {address}\r\nConnection: close\r\n{auth_header}{content_headers}\r\n{body}"
            );
            stream
                .write_all(req.as_bytes())
                .await
                .expect("write request");
            let mut resp = Vec::new();
            stream.read_to_end(&mut resp).await.expect("read response");
            let text = String::from_utf8_lossy(&resp);
            let status_line = text.lines().next().unwrap_or_default();
            status_line
                .split_whitespace()
                .nth(1)
                .and_then(|s| s.parse::<u16>().ok())
                .unwrap_or(0)
        }

        // 1. Verify unknown endpoint returns 404 (when authenticated) and 401 (when unauthenticated)
        let unauth_unknown = execute_http(
            address,
            "GET",
            "/api/v1/unknown-endpoint-nonexistent-404",
            None,
        )
        .await;
        assert_eq!(
            unauth_unknown, 401,
            "unauthenticated request to unknown endpoint must be 401"
        );
        let auth_unknown = execute_http(
            address,
            "GET",
            "/api/v1/unknown-endpoint-nonexistent-404",
            Some(&token),
        )
        .await;
        assert_eq!(
            auth_unknown, 404,
            "authenticated request to unknown endpoint must be 404"
        );

        async fn execute_http_with_body(
            address: SocketAddr,
            method: &str,
            path: &str,
            token: Option<&str>,
            body: &str,
        ) -> (u16, String) {
            use tokio::io::{AsyncReadExt, AsyncWriteExt};
            let mut stream = tokio::net::TcpStream::connect(address)
                .await
                .expect("connect");
            let auth_header = match token {
                Some(tok) => format!("Authorization: Bearer {tok}\r\n"),
                None => String::new(),
            };
            let content_headers = if body.is_empty() {
                String::new()
            } else {
                format!(
                    "Content-Type: application/json\r\nContent-Length: {}\r\n",
                    body.len()
                )
            };
            let request = format!(
                "{method} {path} HTTP/1.1\r\nHost: {address}\r\nConnection: close\r\n{auth_header}{content_headers}\r\n{body}"
            );
            stream
                .write_all(request.as_bytes())
                .await
                .expect("write request");
            let mut response = Vec::new();
            stream
                .read_to_end(&mut response)
                .await
                .expect("read response");
            let text = String::from_utf8_lossy(&response).to_string();
            let mut parts = text.split("\r\n\r\n");
            let head = parts.next().unwrap_or_default();
            let body_text = parts.next().unwrap_or_default().to_string();
            let status = head
                .lines()
                .next()
                .unwrap_or_default()
                .split_whitespace()
                .nth(1)
                .and_then(|code| code.parse::<u16>().ok())
                .unwrap_or(0);
            (status, body_text)
        }

        /// Per-operation fail-closed evidence for every ExternalUnavailable
        /// binding: (method, path-template, query, body, expected status,
        /// expected baseline error code).  An empty code with status 200 marks
        /// the subscription-clear baseline (cleared snapshot, shared demand
        /// book without a physical router).
        const EXTERNAL_UNAVAILABLE_EVIDENCE: &[(&str, &str, &str, &str, u16, &str)] = &[
            (
                "DELETE",
                "/api/v1/brokers/{brokerId}/orders",
                "",
                "{}",
                503,
                "BROKERS_WRITE_UNAVAILABLE",
            ),
            (
                "DELETE",
                "/api/v1/market-data/prediction/contracts/{code}/subscriptions/{leaseId}",
                "",
                "",
                503,
                "MARKET_DATA_SUBSCRIPTION_MUTATION_UNAVAILABLE",
            ),
            (
                "DELETE",
                "/api/v1/market-data/subscriptions",
                "",
                "",
                200,
                "",
            ),
            (
                "GET",
                "/api/v1/alerts/option-events",
                "",
                "",
                503,
                "ALERTS_UNAVAILABLE",
            ),
            (
                "GET",
                "/api/v1/alerts/price",
                "",
                "",
                503,
                "ALERTS_UNAVAILABLE",
            ),
            (
                "GET",
                "/api/v1/brokers/capabilities",
                "",
                "",
                503,
                "BROKER_READ_UNAVAILABLE",
            ),
            (
                "GET",
                "/api/v1/brokers/{brokerId}/cash-flows",
                "",
                "",
                503,
                "BROKER_READ_UNAVAILABLE",
            ),
            (
                "GET",
                "/api/v1/brokers/{brokerId}/fills",
                "",
                "",
                503,
                "BROKER_READ_UNAVAILABLE",
            ),
            (
                "GET",
                "/api/v1/brokers/{brokerId}/funds",
                "",
                "",
                503,
                "BROKER_READ_UNAVAILABLE",
            ),
            (
                "GET",
                "/api/v1/brokers/{brokerId}/klines",
                "",
                "",
                503,
                "BROKER_READ_UNAVAILABLE",
            ),
            (
                "GET",
                "/api/v1/brokers/{brokerId}/margin-ratios",
                "",
                "",
                503,
                "BROKER_READ_UNAVAILABLE",
            ),
            (
                "GET",
                "/api/v1/brokers/{brokerId}/max-trade-qtys",
                "",
                "",
                503,
                "BROKER_READ_UNAVAILABLE",
            ),
            (
                "GET",
                "/api/v1/brokers/{brokerId}/order-fees",
                "",
                "",
                503,
                "BROKER_READ_UNAVAILABLE",
            ),
            (
                "GET",
                "/api/v1/brokers/{brokerId}/orders",
                "",
                "",
                503,
                "BROKER_READ_UNAVAILABLE",
            ),
            (
                "GET",
                "/api/v1/brokers/{brokerId}/positions",
                "",
                "",
                503,
                "BROKER_READ_UNAVAILABLE",
            ),
            (
                "GET",
                "/api/v1/brokers/{brokerId}/quote",
                "",
                "",
                503,
                "BROKER_READ_UNAVAILABLE",
            ),
            (
                "GET",
                "/api/v1/brokers/{brokerId}/runtime",
                "",
                "",
                503,
                "BROKER_READ_UNAVAILABLE",
            ),
            (
                "GET",
                "/api/v1/brokers/{brokerId}/securities",
                "",
                "",
                503,
                "BROKER_READ_UNAVAILABLE",
            ),
            (
                "GET",
                "/api/v1/market-data/broker-queue/{instrumentId}",
                "",
                "",
                409,
                "BROKER_CAPABILITY_UNAVAILABLE",
            ),
            (
                "GET",
                "/api/v1/market-data/candles/{market}/{symbol}",
                "",
                "",
                503,
                "MARKET_DATA_QUOTE_READ_UNAVAILABLE",
            ),
            (
                "GET",
                "/api/v1/market-data/capital-flow/{instrumentId}",
                "",
                "",
                409,
                "BROKER_CAPABILITY_UNAVAILABLE",
            ),
            (
                "GET",
                "/api/v1/market-data/corporate-actions/{market}/{symbol}",
                "",
                "",
                503,
                "MARKET_DATA_NEWS_ACTIONS_UNAVAILABLE",
            ),
            (
                "GET",
                "/api/v1/market-data/depth/{market}/{symbol}",
                "",
                "",
                503,
                "MARKET_DATA_QUOTE_READ_UNAVAILABLE",
            ),
            (
                "GET",
                "/api/v1/market-data/futures",
                "",
                "",
                503,
                "MARKET_DATA_DERIVATIVE_UNAVAILABLE",
            ),
            (
                "GET",
                "/api/v1/market-data/instruments",
                "query=AAPL&limit=20",
                "",
                503,
                "MARKET_DATA_CATALOG_UNAVAILABLE",
            ),
            (
                "GET",
                "/api/v1/market-data/instruments/{instrumentId}/profile",
                "",
                "",
                409,
                "BROKER_CAPABILITY_UNAVAILABLE",
            ),
            (
                "GET",
                "/api/v1/market-data/intraday/{instrumentId}",
                "",
                "",
                409,
                "BROKER_CAPABILITY_UNAVAILABLE",
            ),
            (
                "GET",
                "/api/v1/market-data/markets",
                "",
                "",
                503,
                "MARKET_DATA_CATALOG_UNAVAILABLE",
            ),
            (
                "GET",
                "/api/v1/market-data/news",
                "",
                "",
                503,
                "MARKET_DATA_NEWS_SEARCH_READ_UNAVAILABLE",
            ),
            (
                "GET",
                "/api/v1/market-data/news/{market}/{symbol}",
                "",
                "",
                503,
                "MARKET_DATA_NEWS_ACTIONS_UNAVAILABLE",
            ),
            (
                "GET",
                "/api/v1/market-data/options/analysis/{instrumentId}",
                "",
                "",
                503,
                "MARKET_DATA_OPTIONS_UNAVAILABLE",
            ),
            (
                "GET",
                "/api/v1/market-data/options/chains/{instrumentId}",
                "",
                "",
                503,
                "MARKET_DATA_OPTIONS_UNAVAILABLE",
            ),
            (
                "GET",
                "/api/v1/market-data/options/events",
                "",
                "",
                503,
                "MARKET_DATA_OPTIONS_UNAVAILABLE",
            ),
            (
                "GET",
                "/api/v1/market-data/options/expirations/{instrumentId}",
                "",
                "",
                503,
                "MARKET_DATA_OPTIONS_UNAVAILABLE",
            ),
            (
                "GET",
                "/api/v1/market-data/options/screens",
                "",
                "",
                503,
                "MARKET_DATA_OPTIONS_UNAVAILABLE",
            ),
            (
                "GET",
                "/api/v1/market-data/prediction/categories",
                "",
                "",
                503,
                "MARKET_DATA_PREDICTION_READ_UNAVAILABLE",
            ),
            (
                "GET",
                "/api/v1/market-data/prediction/combos/eligible-events",
                "",
                "",
                503,
                "MARKET_DATA_PREDICTION_READ_UNAVAILABLE",
            ),
            (
                "GET",
                "/api/v1/market-data/prediction/competitions",
                "",
                "",
                503,
                "MARKET_DATA_PREDICTION_READ_UNAVAILABLE",
            ),
            (
                "GET",
                "/api/v1/market-data/prediction/contracts/{code}/candles",
                "",
                "",
                503,
                "MARKET_DATA_PREDICTION_READ_UNAVAILABLE",
            ),
            (
                "GET",
                "/api/v1/market-data/prediction/contracts/{code}/candles/history",
                "",
                "",
                503,
                "MARKET_DATA_PREDICTION_READ_UNAVAILABLE",
            ),
            (
                "GET",
                "/api/v1/market-data/prediction/contracts/{code}/milestones",
                "",
                "",
                503,
                "MARKET_DATA_PREDICTION_READ_UNAVAILABLE",
            ),
            (
                "GET",
                "/api/v1/market-data/prediction/contracts/{code}/order-book",
                "",
                "",
                503,
                "MARKET_DATA_PREDICTION_READ_UNAVAILABLE",
            ),
            (
                "GET",
                "/api/v1/market-data/prediction/contracts/{code}/snapshot",
                "",
                "",
                503,
                "MARKET_DATA_PREDICTION_READ_UNAVAILABLE",
            ),
            (
                "GET",
                "/api/v1/market-data/prediction/contracts/{code}/ticks",
                "",
                "",
                503,
                "MARKET_DATA_PREDICTION_READ_UNAVAILABLE",
            ),
            (
                "GET",
                "/api/v1/market-data/prediction/events",
                "",
                "",
                503,
                "MARKET_DATA_PREDICTION_READ_UNAVAILABLE",
            ),
            (
                "GET",
                "/api/v1/market-data/prediction/events/{eventId}/contracts",
                "",
                "",
                503,
                "MARKET_DATA_PREDICTION_READ_UNAVAILABLE",
            ),
            (
                "GET",
                "/api/v1/market-data/prediction/series",
                "",
                "",
                503,
                "MARKET_DATA_PREDICTION_READ_UNAVAILABLE",
            ),
            (
                "GET",
                "/api/v1/market-data/securities/{market}/{symbol}",
                "",
                "",
                503,
                "MARKET_DATA_QUOTE_READ_UNAVAILABLE",
            ),
            (
                "GET",
                "/api/v1/market-data/snapshots/{market}/{symbol}",
                "",
                "",
                503,
                "MARKET_DATA_QUOTE_READ_UNAVAILABLE",
            ),
            (
                "GET",
                "/api/v1/market-data/ticks/{instrumentId}",
                "",
                "",
                409,
                "BROKER_CAPABILITY_UNAVAILABLE",
            ),
            (
                "GET",
                "/api/v1/market-data/warrants",
                "",
                "",
                503,
                "MARKET_DATA_DERIVATIVE_UNAVAILABLE",
            ),
            (
                "GET",
                "/api/v1/portfolio/{brokerId}/cash-balances",
                "",
                "",
                503,
                "PORTFOLIO_UNAVAILABLE",
            ),
            (
                "GET",
                "/api/v1/portfolio/{brokerId}/positions",
                "",
                "",
                503,
                "PORTFOLIO_UNAVAILABLE",
            ),
            (
                "GET",
                "/api/v1/research/analyst/{instrumentId}",
                "",
                "",
                503,
                "RESEARCH_UNAVAILABLE",
            ),
            (
                "GET",
                "/api/v1/research/calendars",
                "",
                "",
                503,
                "RESEARCH_UNAVAILABLE",
            ),
            (
                "GET",
                "/api/v1/research/corporate-actions/{instrumentId}",
                "",
                "",
                503,
                "RESEARCH_UNAVAILABLE",
            ),
            (
                "GET",
                "/api/v1/research/financials/{instrumentId}",
                "",
                "",
                503,
                "RESEARCH_UNAVAILABLE",
            ),
            (
                "GET",
                "/api/v1/research/industries",
                "",
                "",
                503,
                "RESEARCH_UNAVAILABLE",
            ),
            (
                "GET",
                "/api/v1/research/institutions",
                "",
                "",
                503,
                "RESEARCH_UNAVAILABLE",
            ),
            (
                "GET",
                "/api/v1/research/instruments/{instrumentId}",
                "",
                "",
                503,
                "RESEARCH_UNAVAILABLE",
            ),
            (
                "GET",
                "/api/v1/research/macro",
                "",
                "",
                503,
                "RESEARCH_UNAVAILABLE",
            ),
            (
                "GET",
                "/api/v1/research/ownership/{instrumentId}",
                "",
                "",
                503,
                "RESEARCH_UNAVAILABLE",
            ),
            (
                "GET",
                "/api/v1/research/rankings",
                "",
                "",
                503,
                "RESEARCH_UNAVAILABLE",
            ),
            (
                "GET",
                "/api/v1/research/screens",
                "",
                "",
                503,
                "RESEARCH_UNAVAILABLE",
            ),
            (
                "GET",
                "/api/v1/research/short-interest/{instrumentId}",
                "",
                "",
                503,
                "RESEARCH_UNAVAILABLE",
            ),
            (
                "GET",
                "/api/v1/research/technical-indicators/{instrumentId}",
                "",
                "",
                503,
                "RESEARCH_UNAVAILABLE",
            ),
            (
                "GET",
                "/api/v1/research/valuation/{instrumentId}",
                "",
                "",
                503,
                "RESEARCH_UNAVAILABLE",
            ),
            (
                "GET",
                "/api/v1/watchlists/remote",
                "",
                "",
                503,
                "WATCHLIST_UNAVAILABLE",
            ),
            (
                "POST",
                "/api/v1/adk/chat",
                "",
                "{\"clientRequestId\":\"6f9619ff-8b86-d011-b42d-00cf96c96d3a\"}",
                503,
                "ADK_UNAVAILABLE",
            ),
            (
                "POST",
                "/api/v1/adk/chat/stream",
                "",
                "{\"clientRequestId\":\"6f9619ff-8b86-d011-b42d-00cf96c96d3a\"}",
                503,
                "ADK_UNAVAILABLE",
            ),
            (
                "POST",
                "/api/v1/alerts/option-events",
                "",
                "{}",
                503,
                "ALERTS_UNAVAILABLE",
            ),
            (
                "POST",
                "/api/v1/alerts/price",
                "",
                "{}",
                503,
                "ALERTS_UNAVAILABLE",
            ),
            (
                "POST",
                "/api/v1/backtests",
                "",
                "{}",
                503,
                "BACKTESTS_WRITE_UNAVAILABLE",
            ),
            (
                "POST",
                "/api/v1/backtests/sync",
                "",
                "{}",
                503,
                "BACKTESTS_WRITE_UNAVAILABLE",
            ),
            (
                "POST",
                "/api/v1/brokers/{brokerId}/orders",
                "",
                "{}",
                503,
                "BROKERS_WRITE_UNAVAILABLE",
            ),
            (
                "POST",
                "/api/v1/brokers/{brokerId}/unlock",
                "",
                "{}",
                503,
                "BROKERS_WRITE_UNAVAILABLE",
            ),
            (
                "POST",
                "/api/v1/execution/buying-power",
                "",
                "{}",
                503,
                "EXECUTION_WRITE_UNAVAILABLE",
            ),
            (
                "POST",
                "/api/v1/execution/combos",
                "",
                "{}",
                503,
                "EXECUTION_WRITE_UNAVAILABLE",
            ),
            (
                "POST",
                "/api/v1/execution/combos/previews",
                "",
                "{}",
                503,
                "EXECUTION_WRITE_UNAVAILABLE",
            ),
            (
                "POST",
                "/api/v1/execution/combos/{internalOrderId}/cancel",
                "",
                "{}",
                503,
                "EXECUTION_WRITE_UNAVAILABLE",
            ),
            (
                "POST",
                "/api/v1/execution/orders",
                "",
                "{}",
                503,
                "EXECUTION_WRITE_UNAVAILABLE",
            ),
            (
                "POST",
                "/api/v1/execution/orders/{internalOrderId}/cancel",
                "",
                "{}",
                503,
                "EXECUTION_WRITE_UNAVAILABLE",
            ),
            (
                "POST",
                "/api/v1/execution/previews",
                "",
                "{}",
                503,
                "EXECUTION_WRITE_UNAVAILABLE",
            ),
            (
                "POST",
                "/api/v1/market-data/options/analysis/{instrumentId}",
                "",
                "{}",
                503,
                "MARKET_DATA_PROVIDER_ACTIONS_UNAVAILABLE",
            ),
            (
                "POST",
                "/api/v1/market-data/options/events/zero-dte-contracts",
                "",
                "{}",
                503,
                "MARKET_DATA_PROVIDER_ACTIONS_UNAVAILABLE",
            ),
            (
                "POST",
                "/api/v1/market-data/prediction/combos/quotes",
                "",
                "{}",
                503,
                "MARKET_DATA_PROVIDER_ACTIONS_UNAVAILABLE",
            ),
            (
                "POST",
                "/api/v1/market-data/prediction/contracts/{code}/subscriptions",
                "",
                "{}",
                503,
                "MARKET_DATA_SUBSCRIPTION_MUTATION_UNAVAILABLE",
            ),
            (
                "POST",
                "/api/v1/market-data/snapshots",
                "",
                "{\"instrumentIds\":[\"US.AAPL\"]}",
                503,
                "MARKET_DATA_PROVIDER_ACTIONS_UNAVAILABLE",
            ),
            (
                "POST",
                "/api/v1/market-data/subscriptions",
                "",
                "{\"consumerId\":\"test-consumer\",\"providerBrokerId\":\"futu\",\"instruments\":[{\"market\":\"US\",\"symbol\":\"AAPL\"}]}",
                503,
                "MARKET_DATA_SUBSCRIPTION_MUTATION_UNAVAILABLE",
            ),
            (
                "POST",
                "/api/v1/market-data/subscriptions/heartbeat",
                "",
                "{\"consumerId\":\"test-consumer\",\"providerBrokerId\":\"futu\"}",
                503,
                "MARKET_DATA_SUBSCRIPTION_MUTATION_UNAVAILABLE",
            ),
            (
                "POST",
                "/api/v1/market-data/subscriptions/release",
                "",
                "{\"consumerId\":\"test-consumer\",\"providerBrokerId\":\"futu\"}",
                503,
                "MARKET_DATA_SUBSCRIPTION_MUTATION_UNAVAILABLE",
            ),
            (
                "POST",
                "/api/v1/research/screens",
                "",
                "{\"querySchemaVersion\":2,\"catalogVersion\":\"futu-stock-screen-v1\",\"market\":\"US\"}",
                503,
                "RESEARCH_SCREEN_UNAVAILABLE",
            ),
            (
                "POST",
                "/api/v1/strategy-pine/analyze",
                "",
                "{}",
                503,
                "STRATEGY_PINE_ANALYZE_UNAVAILABLE",
            ),
            (
                "POST",
                "/api/v1/system/futu-opend/manual-retry",
                "",
                "{}",
                503,
                "SYSTEM_WRITE_UNAVAILABLE",
            ),
            (
                "POST",
                "/api/v1/watchlists/remote",
                "",
                "{}",
                503,
                "WATCHLIST_REMOTE_WRITE_UNAVAILABLE",
            ),
        ];

        let mut covered: HashSet<(String, String)> = HashSet::new();

        // 2. Table-driven test across all 278 canonical routes
        for binding in &bindings {
            // Instantiate parameterized paths with sample values
            let concrete_path = binding
                .path
                .replace("{runId}", "test-run")
                .replace("{id}", "test-id")
                .replace("{symbol}", "AAPL")
                .replace("{market}", "US")
                .replace("{name}", "test")
                .replace("{sessionId}", "test-session")
                .replace("{taskId}", "test-task")
                .replace("{strategyId}", "test-strategy")
                .replace("{ruleId}", "test-rule")
                .replace("{profileId}", "test-profile")
                .replace("{groupId}", "test-group")
                .replace("{presetId}", "test-preset")
                .replace("{code}", "test-code")
                .replace("{templateId}", "test-template")
                .replace("{alertId}", "test-alert")
                .replace("{orderId}", "test-order")
                .replace("{accountId}", "test-account")
                .replace("{streamId}", "test-stream")
                .replace("{instanceId}", "test-instance")
                .replace("{databaseId}", "main")
                .replace("{internalOrderId}", "test-order")
                .replace("{instrumentId}", "US.AAPL")
                .replace("{brokerId}", "futu");

            // A. Unauthenticated request must return 401 (except public auth endpoints)
            let is_auth_route = binding.path.starts_with("/api/v1/auth/");
            let unauth_status = execute_http(address, &binding.method, &concrete_path, None).await;
            if is_auth_route {
                assert!(
                    unauth_status != 500 && unauth_status != 501,
                    "Auth route {} {} returned unexpected 500/501 error: {unauth_status}",
                    binding.method,
                    binding.path
                );
            } else {
                assert_eq!(
                    unauth_status, 401,
                    "Route {} {} without auth must return 401 Unauthorized, got {unauth_status}",
                    binding.method, binding.path
                );
            }

            // B. Authenticated request must reach the handler and NEVER return 500 (Internal Server Error) or 501 (Not Implemented)
            let auth_status =
                execute_http(address, &binding.method, &concrete_path, Some(&token)).await;
            assert!(
                auth_status != 500 && auth_status != 501,
                "Route {} {} returned server error / unimplemented status {auth_status}",
                binding.method,
                binding.path
            );

            if binding.adapter_binding == ProductionAdapterBinding::ExternalUnavailable {
                // C. ExternalUnavailable operations get per-operation evidence:
                // each entry carries a request that passes parameter and body
                // validation, so the operation genuinely reaches its port
                // boundary, plus the exact baseline status and error code it
                // must project.  503/502 is the fail-closed boundary for an
                // absent external provider/worker; 409 BROKER_CAPABILITY_UN-
                // AVAILABLE is the Go baseline for capability-gated broker
                // market-data reads; DELETE subscriptions keeps its baseline
                // 200 shared-demand-book semantics (cleared snapshot).
                let entry = EXTERNAL_UNAVAILABLE_EVIDENCE
                    .iter()
                    .find(|(method, path, _, _, _, _)| {
                        *method == binding.method && *path == binding.path
                    })
                    .unwrap_or_else(|| {
                        panic!(
                            "ExternalUnavailable route {} {} has no per-operation evidence entry",
                            binding.method, binding.path
                        )
                    });
                let (
                    entry_method,
                    entry_path,
                    entry_query,
                    entry_body,
                    expected_status,
                    expected_code,
                ) = entry;
                assert_eq!(entry_method, &binding.method);
                assert_eq!(entry_path, &binding.path);
                let concrete_with_query = if entry_query.is_empty() {
                    concrete_path.clone()
                } else {
                    format!("{concrete_path}?{entry_query}")
                };
                let (strict_status, strict_body) = execute_http_with_body(
                    address,
                    &binding.method,
                    &concrete_with_query,
                    Some(&token),
                    entry_body,
                )
                .await;
                assert_eq!(
                    strict_status, *expected_status,
                    "ExternalUnavailable route {} {} must project its baseline status",
                    binding.method, binding.path
                );
                let payload: Value = serde_json::from_str(&strict_body).unwrap_or_else(|error| {
                    panic!(
                        "route {} {} returned non-JSON body ({error}): {strict_body}",
                        binding.method, binding.path
                    )
                });
                if *expected_status == 200 {
                    // Baseline subscription clear: 200 with the cleared snapshot.
                    assert_eq!(
                        payload["data"]["cleared"],
                        json!(true),
                        "subscription clear must keep its baseline 200 cleared semantics"
                    );
                } else {
                    assert_eq!(
                        payload["ok"],
                        json!(false),
                        "fail-closed route {} {} must use the error envelope",
                        binding.method,
                        binding.path
                    );
                    assert_eq!(
                        payload["error"]["code"],
                        json!(expected_code),
                        "fail-closed route {} {} must project its baseline error code",
                        binding.method,
                        binding.path
                    );
                    let message = payload["error"]["message"].as_str().unwrap_or_default();
                    assert!(
                        !message.is_empty(),
                        "fail-closed route {} {} must carry an error message",
                        binding.method,
                        binding.path
                    );
                    assert!(
                        payload["timestamp"].is_string(),
                        "fail-closed route {} {} must carry a timestamp",
                        binding.method,
                        binding.path
                    );
                }
                covered.insert((binding.method.clone(), binding.path.clone()));
            }
        }

        assert_eq!(
            covered.len(),
            EXTERNAL_UNAVAILABLE_EVIDENCE.len(),
            "every evidence entry must be exercised by exactly one ExternalUnavailable binding"
        );

        handle.shutdown().await.expect("shutdown");
    }
}
