// Package jftradeapi implements the JFTrade API sidecar server.
//
// This package serves as the HTTP/WebSocket API layer between the Vue3 frontend
// and the broker abstraction layer (pkg/broker). It is organized by file prefix:
//
// # Server & Infrastructure
//
//   - server.go: Server struct, startup, routing dispatch
//   - server_frontend.go: static frontend asset serving
//   - api_only.go: API-only run mode
//   - runtime_defaults.go: development defaults
//   - runtime_layout.go: file system paths
//
// # Settings (broker configuration & accounts)
//
//   - settings_store.go: SettingsStore persistence (JSON file)
//   - settings_handlers.go: HTTP handlers for settings CRUD
//   - settings_routes.go: route dispatch
//   - settings_futu_config.go: Futu-specific config normalization
//   - settings_account_normalization.go: account ID normalization
//
// # Broker Routes (read-side market data via broker.Broker)
//
//   - broker_routes.go: broker read routes → broker.MarketDataReader
//   - broker_order_updates_worker.go: order sync & push subscription
//
// # Execution (write-side trading via broker.Broker)
//
//   - execution_routes.go: place/cancel order routes → broker.TradingService
//   - execution_store.go: in-memory order tracking
//   - execution_notifications.go: order lifecycle notifications
//
// # Market Data (quote & kline serving)
//
//   - market_data.go: kline/snapshot/ticker query handlers
//   - market_routes.go: market route dispatch
//   - market_live.go: live market stream (SSE)
//   - market_subscriptions.go: subscription manager
//   - market_subscription_state.go: subscription state tracking
//   - market_tick_*.go: tick sampling, caching, candles, serialization
//   - market_security_serialization.go: security snapshot serialization
//   - market_query_params.go: query parameter parsing
//   - instrument_ref.go: instrument reference mapping
//
// # Live WebSocket
//
//   - live_websocket_dispatcher.go: WebSocket event dispatcher
//   - live_socket_pool.go: connection pool
//   - live_notification_cache.go: notification buffer
//   - live_runtime_state.go: runtime state queries
//
// # Notifications
//
//   - notifications.go: notification hub
//   - notification_source_futu.go: Futu system notification bridge
//   - notification_source_bbgo.go: bbgo notification bridge
//
// # Strategy (catalog, design, runtime)
//
//   - strategy_routes.go: strategy route dispatch
//   - strategy_catalog_store.go: strategy definition catalog (SQLite)
//   - strategy_catalog_store_*_helpers.go: catalog store aspect modules
//   - strategy_design_store.go: strategy visual design store (SQLite)
//   - strategy_definition_view.go: definition view model
//   - strategy_runtime_manager.go: strategy lifecycle orchestrator
//   - strategy_runtime_store.go: runtime state store (SQLite)
//   - strategy_runtime_manager_test_helpers.go: test stub exchange
//
// # Backtest
//
//   - backtest_routes.go: backtest API routes
//   - backtest_state.go: backtest run state machine
//
// # Futu Runtime (OpenD bridge)
//
//   - futu_runtime.go: OpenD probe, broker descriptor, system status
//
// # OpenAPI / Swagger
//
//   - openapi.go: spec builder & Swagger UI handler
//   - openapi_paths.go: path definitions
//   - openapi_components.go: component schemas
//
// # Other
//
//   - plugin_routes.go: plugin catalog routes
//   - portfolio_routes.go: portfolio summary routes
//   - system_routes.go: system status routes
//
// # Multi-Broker Architecture
//
// The Server uses broker.Registry to manage multiple broker adapters.
// New brokers implement the broker.Broker interface and register at startup.
// See pkg/broker/ and docs/new-broker-integration-guide.md for details.
package jftradeapi
