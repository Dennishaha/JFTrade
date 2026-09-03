import assert from "node:assert/strict";
import test from "node:test";

import {
  checkProductionRoutePolicy,
  productionSourceFiles,
  validateProductionRoutePolicy,
} from "./check-production-route-policy.mjs";

test("production route policy rejects legacy forced registration and fallback markers", () => {
  assert.deepEqual(validateProductionRoutePolicy([
    ["product_route_assembly.rs", "ProductRoutePorts::all()"],
    ["product_wire.rs", "RUST_OWNER_NOT_IMPLEMENTED"],
  ]), [
    "product_route_assembly.rs contains forbidden production route token: ProductRoutePorts::all",
    "product_wire.rs contains forbidden production route token: RUST_OWNER_NOT_IMPLEMENTED",
  ]);
});

test("production route policy rejects market data synthetic fallback and sync blocking", () => {
  assert.deepEqual(validateProductionRoutePolicy([
    ["product_production_ports_market_data_catalog.rs", "fn resolve_instrument_candidates()"],
    ["product_production_ports_market_data_quote.rs", "tokio::task::block_in_place"],
  ]), [
    "product_production_ports_market_data_catalog.rs contains forbidden market data production token: resolve_instrument_candidates",
    "product_production_ports_market_data_quote.rs contains forbidden market data production token: block_in_place",
  ]);
});

test("production route policy permits explicit external-unavailable adapters", () => {
  assert.deepEqual(validateProductionRoutePolicy([
    ["product_production_ports.rs", "ProductionUnavailablePort::new(\"provider unavailable\")"],
  ]), []);
});

test("production route policy scans ProductApi sources but excludes test fixtures", () => {
  const files = productionSourceFiles();
  assert.ok(files.some((file) => file.endsWith("/product_route_assembly.rs")));
  assert.ok(files.every((file) => !file.endsWith("_tests.rs")));
  assert.deepEqual(checkProductionRoutePolicy(), []);
});
