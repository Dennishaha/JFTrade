# Plugins Write Group Ledger

> Historical/rehearsal evidence notice (pre-2026-08-31): owner labels, route counts, and Go/retained statuses in this ledger describe the qualification snapshot at capture time, not current production ownership.
>
> Current route truth is derived from `node scripts/rust-migration/check-stage9-route-coverage.mjs` and `tests/fixtures/rust-migration/stage9/route-ownership.json`; formal release truth is `node scripts/rust-migration/check-stage9-closeout.mjs --check`. The original evidence below is intentionally retained verbatim.

- Group: `plugins-write`
- Tier: A, mutation operations
- Operations: `POST /api/v1/plugins/{pluginId}/install`; `POST /api/v1/plugins/{pluginId}/uninstall`
- Current production owner: Go plugin catalog/service/repository; Rust has no production owner.
- Current route ownership: `cutover-qualified`; both operations register only when the explicit product test-cutover profile supplies `PluginWritePort`. Go remains the production owner and `goRemovalStatus=retained`.
- Fixture: `tests/fixtures/rust-migration/stage9/plugins-write.json`
- Go reference: `scripts/rust-migration/stage9_plugins_write_reference_test.go`
- Rust leaf/test: `crates/jftrade-engine/src/product_plugins_write_port.rs`; `crates/jftrade-engine/tests/product_plugins_write_tests.rs`
- Differential: `node scripts/rust-migration/check-stage9-plugins-write.mjs`
- Integration status: `cutover-qualified`; no Rust production owner, plugin lifecycle, SQLite write, or resource event ownership was added.
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

Passed on the integration branch:

- `go test scripts/rust-migration/stage9_plugins_write_reference_test.go -run '^TestStage9PluginsWriteFixtureMatchesCurrentGoOwner$' -count=1`
- `cargo test -p jftrade-engine --test product_plugins_write_tests -- --nocapture` (`4 passed`)
- `cargo check -p jftrade-engine --test product_plugins_write_tests`
- `go test ./internal/app/apiserver/servercoretest -run '^TestPluginsWriteRehearsalFencesOwnersAndRecoversAcrossRestart$' -count=1`
- `cargo test -p jftrade-engine --lib plugins_write -- --nocapture`
- `node scripts/rust-migration/check-stage9-plugins-write.mjs`
- direct `rustfmt --edition 2024` on the two plugins-write Rust files

The Go reference fixture, Rust leaf replay, authenticated loopback rehearsal, explicit product test-cutover adapter, and full Stage 9 product differential are green. Evidence covers arbitrary-body forwarding, duplicate and concurrent requests, internal/unavailable errors, timeout/cancellation/crash fail-closed behavior, Go-only fallback after restart, private bearer plus browser Cookie/Origin/Referer/CSRF forwarding, default-profile isolation, and unchanged settings bytes. This group is `cutover-qualified`, not a production migration: Go remains the only production owner, and the Rust port still has no plugin filesystem/process/event/SQLite side effect.

The formal production-owner, durable transaction, plugin lifecycle, release/signing, security, SBOM, backup/restore, and final unique-owner gates remain outstanding.

## Integration Review

- Product wiring adds a private `PluginWritePort: Send + Sync`, `PluginsWrite` capability, and exact POST dispatch through the existing product envelope. The default profile reports 48 routes; the explicit plugin-write test port reports 50.
- The unified product differential runs the Go reference, authenticated servercore rehearsal, and product integration cases, while the group checker replays leaf fixture success, body-ignore, missing-catalog, persistence-failure, repeat, concurrency, timeout/cancel and restart evidence. `route-ownership.json` records both operations as `cutover-qualified` with `productionOwner=go` and `goRemovalStatus=retained`.
- The plugin port deliberately has no filesystem, dynamic-library, process, event, or persistence method. The high-risk Go persist-failure memory/durable divergence and non-idempotent repeated-write behavior remain recorded compatibility quirks; formal production transaction/lifecycle ownership, release/signing, security, backup/restore, and final unique-owner/hard-cut approval remain open.
