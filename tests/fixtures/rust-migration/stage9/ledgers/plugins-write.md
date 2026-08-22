# Plugins Write Group Ledger

- Group: `plugins-write`
- Tier: A, mutation operations
- Operations: `POST /api/v1/plugins/{pluginId}/install`; `POST /api/v1/plugins/{pluginId}/uninstall`
- Current production owner: Go plugin catalog/service/repository; Rust has no production owner.
- Current route ownership: unchanged by this worker. The integration branch must register both operations as `cutover-test-only` only after applying the shared product wiring patch.
- Fixture: `tests/fixtures/rust-migration/stage9/plugins-write.json`
- Go reference: `scripts/rust-migration/stage9_plugins_write_reference_test.go`
- Rust leaf/test: `crates/jftrade-engine/src/product_plugins_write_port.rs`; `crates/jftrade-engine/tests/product_plugins_write_tests.rs`
- Differential: `node scripts/rust-migration/check-stage9-plugins-write.mjs`
- Worker status: `cutover-test-only` candidate; no production owner, route registration, shared assembly, or ownership ledger change was made by this worker.
- Rust boundary: the leaf accepts only a consumer-owned injected `PluginWritePort`; tests use an in-memory mock and never open SQLite, install a real plugin, start a process/helper, or publish an event.

| Method | Path | Request and success projection | Error branches covered |
| --- | --- | --- | --- |
| POST | `/api/v1/plugins/{pluginId}/install` | Empty or arbitrary body is ignored; returns `200` with `data.operation`, `SUCCEEDED`, `installed`, `100`, metadata message, target/install paths, timestamps and nullable `error` | Encoded blank ID is `400 BAD_REQUEST`; literal empty segment is transport `404`; catalog missing through the current catalog service is observed as `500 INTERNAL_ERROR` because its typed not-found error is not `os.ErrNotExist`; injected `os.ErrNotExist` behavior is represented by the Rust port's `404 NOT_FOUND`; persistence/internal failure is `500 INTERNAL_ERROR`; unavailable port is `503` in the Rust test-only adapter |
| POST | `/api/v1/plugins/{pluginId}/uninstall` | Same envelope and metadata projection, with `phase=uninstalled`, `status=NOT_INSTALLED`, and `plugin metadata uninstalled` | Same invalid, not-found, internal, unavailable, repeated and concurrent cases |

## Three-way review and quirks

quirk: Missing catalog entries from `catalog.Service.changePluginInstallation` return `strategy.NotFoundError`, while `handlePluginMutation` only maps `os.IsNotExist` to `404`; the current Go fixture therefore observes `500 INTERNAL_ERROR` / `plugin <operation> failed`.
范围: `plugins-write` / both POST mutation paths
证据: Go reference cases `missing-catalog-install` and `missing-catalog-uninstall`; fixture responses; Rust replay with the fixture's internal-error mode
分类: go-behavior
判定: intended
处置: 复刻，待硬切后修复
风险: medium
owner: Go/integration branch
后续: Keep the exact observable response through qualification; decide whether to repair the Go error classification only after hard-cut review.

quirk: A repository Save failure returns HTTP 500 but leaves the catalog service's in-memory installation and operation state mutated while the durable repository snapshot remains unchanged.
范围: `plugins-write` / both POST mutation paths
证据: Go reference cases `persist-failure-install` and `persist-failure-uninstall`; fixture `expectedObservation.memory*` versus `durable*`
分类: go-behavior
判定: intended
处置: 复刻，待硬切后修复
风险: high
owner: Go/integration branch
后续: Do not normalize or roll back this state in the migration slice; require explicit transaction/recovery evidence before any mutation cutover.

quirk: Operation IDs and timestamps are wall-clock generated and not stable under replay; the Go reference normalizes them to case labels and a fixed timestamp before writing the fixture.
范围: `plugins-write` / both POST mutation paths
证据: reference normalization in `normalizeStage9PluginWriteResponse`; fixture operation projections
分类: fixture
判定: intended
处置: 修复 fixture/harness
风险: low
owner: worker
后续: Keep dynamic values out of differential equality while preserving their fields and format in the wire projection.

quirk: The mutation handler does not parse or inspect the request body; arbitrary bytes have the same success behavior as an empty body.
范围: `plugins-write` / both POST mutation paths
证据: Go reference case `body-ignored`; Rust leaf replay
分类: go-behavior
判定: intended
处置: 复刻，待硬切后修复
风险: low
owner: Go/integration branch
后续: Preserve until a separate public contract change is approved.

quirk: Repeated and concurrent metadata mutations serialize on the Go catalog mutex, create one successful operation per request, and never create plugin file/process/resource events.
范围: `plugins-write` / both POST mutation paths
证据: Go `install-repeat`, `uninstall-repeat`, `mixed-state`, `concurrent-install`; Rust concurrent test; empty `resourceEvents`
分类: go-behavior
判定: intended
处置: 复刻，待硬切后复查
风险: high
owner: Go/integration branch
后续: A future cutover must prove idempotency policy, cancellation, restart recovery and resource fencing without activating real plugin lifecycle in this leaf.

## Validation and remaining gates

Passed on the worker branch:

- `go test scripts/rust-migration/stage9_plugins_write_reference_test.go -run '^TestStage9PluginsWriteFixtureMatchesCurrentGoOwner$' -count=1`
- `cargo test -p jftrade-engine --test product_plugins_write_tests -- --nocapture` (`4 passed`)
- `cargo check -p jftrade-engine --test product_plugins_write_tests`
- `node scripts/rust-migration/check-stage9-plugins-write.mjs`
- direct `rustfmt --edition 2024` on the two plugins-write Rust files

Not run by this worker because they are shared/integration gates: route coverage after ownership registration, `pnpm run check:quick`, full `pnpm run check:rust`, generated-contract checks, product assembly wiring, test-cutover registration, unique-owner proof, four-platform release/signing/security/recovery gates, and hard-cut Go/Wails removal. Go remains the only production owner.

## Shared integration patch request

The integration branch must apply the smallest shared wiring patch:

1. Add a private `PluginWritePort` field to `ProductConfig`, `ProductOptionalPorts`, and `ProductApi`, plus a `#[cfg(test)]` builder.
2. Add a `ProductCapability::PluginsWrite` and `ProductRoutePorts::plugins_write` gate; include the two exact POST route specs only when the explicit test port is present.
3. Dispatch the two concrete paths in `product_wire.rs` to a small shared adapter that parses the path and maps the port result to the existing envelope. Keep default profiles and Go production owner unchanged.
4. Add the two operations to `route-ownership.json` as `cutover-test-only` with `productionOwner=go`, `goRemovalStatus=retained`, and the group differential as evidence.

This worker intentionally did not edit those shared files.
