# Strategy Definitions Write Group Ledger

> Historical/rehearsal evidence notice (pre-2026-08-31): owner labels, route counts, and Go/retained statuses in this ledger describe the qualification snapshot at capture time, not current production ownership.
>
> Current route truth is derived from `node scripts/rust-migration/check-stage9-route-coverage.mjs` and `tests/fixtures/rust-migration/stage9/route-ownership.json`; formal release truth is `node scripts/rust-migration/check-stage9-closeout.mjs --check`. The original evidence below is intentionally retained verbatim.

- Group: `strategy-definitions-write`
- Tier: A mutation/state change; this slice is test-cutover-only.
- Operations: 5: create, update, delete, apply-linked-instances, and instantiate under `/api/v1/strategy-definitions`.
- Go remains the production owner of the strategy definition store, version history, soft-delete guard, catalog instances, Pine compilation, runtime lifecycle, and all SQLite writes.
- Current route coverage is `1 shadow / 118 cutover-test-only / 159 cutover-qualified / 0 remaining / 0 Rust production owner`; all five operations remain `cutover-test-only`.
- Rust boundary: `product_strategy_definition_write_port.rs` accepts a complete consumer-owned mutation projection. The explicit test-cutover adapter in `product_strategy_definition_write_test_cutover.rs` uses an isolated SQLite schema only for durable replay; it has no production route registration and does not open the Go strategy database, PineTS, Provider/OpenD, runtime, notification, or task owners.
- Fixture: `tests/fixtures/rust-migration/stage9/strategy-definitions-write.json` (20 cases).
- Go reference: `scripts/rust-migration/stage9_strategy_definitions_write_reference_test.go`.
- Differential: `scripts/rust-migration/check-stage9-strategy-definitions-write.mjs`.
- Rust leaf: `crates/jftrade-engine/tests/stage9_strategy_definitions_write.rs`.

## Contract ledger

| Method | Path | Go observable behavior | Failure/error precedence |
| --- | --- | --- | --- |
| POST | `/api/v1/strategy-definitions` | JSON definition is bound through the Go service; client-supplied `id` is cleared before the store upsert, and the complete versioned definition is returned in the standard envelope. | Malformed body is `400 BAD_REQUEST` before store access; semantic/script validation remains `400`; store failure is `500 STRATEGY_FAILED`. |
| PUT | `/api/v1/strategy-definitions/{definitionId}` | Path ID overwrites any body ID. The Go store preserves its upsert/version behavior, including repeated updates and missing-ID upsert. | Malformed body is `400`; store/snapshot failure is `500 STRATEGY_FAILED`; the fixture preserves the current version and rollback observations. |
| DELETE | `/api/v1/strategy-definitions/{definitionId}` | Go first checks linked instance IDs. A linked definition returns a `400 BAD_REQUEST`; once unlinked, delete soft-deletes and returns the definition projection. | Invalid path is `400`; linked guard precedes delete; missing definition is `404 NOT_FOUND`; store failure is `500 STRATEGY_FAILED`. |
| POST | `/api/v1/strategy-definitions/{definitionId}/apply-linked-instances` | Go loads the definition, applies the latest version to eligible linked instances, and returns applied/alreadyLatest/skippedBusy counts. | Definition read failure maps to `400`; missing definition is `404`; busy/application errors preserve the Go `400`/`500` mapping. |
| POST | `/api/v1/strategy-definitions/{definitionId}/instantiate` | Go loads the definition before binding the optional body. Empty body creates an instance with zero-value binding; valid body is normalized by the catalog and returns the complete stopped instance projection. | Missing definition is `404` before malformed body; malformed binding is `400`; catalog failure is `500 STRATEGY_FAILED`. |

The fixture normalizes only generated definition/instance IDs and the response clock to `2026-08-22T06:00:00Z`; it keeps input whitespace, nullable fields, version values, status, and catalog projections. Concurrent case output is fixture-owned and preserves the observed Go result multiset.

## Three-way quirks

quirk: Create explicitly clears a client-provided definition ID, while update forces the path ID over a body ID.
范围: `strategy-definitions-write` / POST and PUT definition routes
证据: Go reference `create-success-client-id-ignored`, `update-version-and-duplicate`; fixture `expectedObservation.definitionSaves`; Rust input replay.
分类: go-behavior
判定: intended
处置: 复刻，待硬切后修复
风险: medium
owner: Go until cutover
后续: retain the request precedence in every future adapter.

quirk: Instantiate reads the definition before decoding the optional binding body, so a missing definition returns `404 NOT_FOUND` even when the body is malformed.
范围: `strategy-definitions-write` / POST `.../{definitionId}/instantiate`
证据: Go route order, fixture `instantiate-definition-missing-precedes-malformed-body`, Rust replay.
分类: go-behavior
判定: intended
处置: 复刻，待硬切后修复
风险: high
owner: Go until cutover
后续: preserve error precedence; a production Rust adapter must keep definition lookup and binding validation ordering.

quirk: Delete does a linked-instance guard before soft delete; the first linked request is `400` and a later unlinked request succeeds.
范围: `strategy-definitions-write` / DELETE definition
证据: fixture `delete-linked-guard-then-soft-delete`, Go catalog fixture observations, Rust replay.
分类: go-behavior
判定: intended
处置: 复刻，待硬切后修复
风险: high
owner: Go until cutover
后续: require atomic linked-state/delete recovery evidence before qualification.

quirk: Concurrent updates are observable as repeated successful version projections in the current Go fixture rather than an explicit conflict envelope; no Rust-side conflict policy is inferred from this corpus.
范围: `strategy-definitions-write` / PUT definition concurrent update
证据: Go reference `concurrent-update-no-lost-version`, fixture response multiset and `definitionSaves` observations.
分类: go-behavior
判定: unresolved
处置: 复刻 captured behavior; add a real cancellation/transaction/restart differential before qualification.
风险: release-blocker
owner: Go/integration branch
后续: resolve with a three-way durable-store replay and owner-fencing review; do not mark this A group qualified while unresolved.

quirk: Fixture-generated definition/instance IDs and response timestamps are dynamic and are normalized only in the reference harness; input and business fields are not normalized.
范围: all five operations / fixture harness
证据: reference normalization and fixture `createdAt`, `updatedAt`, generated IDs.
分类: fixture
判定: intended
处置: 修复 fixture/harness；保留 public field shape and timestamp format checks
风险: low
owner: integration branch
后续: compare clock and ID semantics separately in a production rehearsal.

## Verification and integration handoff

quirk: The shared product route-count assertion was updated with the new optional ports but initially expected 196 routes; the ownership ledger and the assembled catalog both resolve to 195 (`278 - 83`), so the assertion was corrected to 195. 三方复核: `route-ownership.json` derived counts, `product_routes` output, and the Go/Rust product differential were compared. 分类: harness. 判定: confirmed. 处置: corrected the test-only count; no route, wire, owner, or default-profile behavior changed.

quirk: The standalone strategy fixture harness used a `match` equivalent to `matches!`, which the repository's `clippy -D warnings` gate rejects. 三方复核: fixture port-call cases, the Go reference's malformed-body precedence, and Rust leaf replay were compared before the mechanical rewrite. 分类: harness. 判定: confirmed. 处置: replaced only the equivalent predicate; port-call precedence and observable responses are unchanged.

- Go reference fixture test: passed.
- Rust standalone fixture replay, exact route inventory, unavailable-port and read-isolation tests: passed.
- Dedicated differential: passed (`node scripts/rust-migration/check-stage9-strategy-definitions-write.mjs`).
- Authenticated Go loopback rehearsal: passed (`go test ./internal/app/apiserver/servercoretest -run '^TestStrategyDefinitionsWriteRehearsalFencesOwnersAndRecoversAcrossRestart$' -count=1 -timeout=300s`). It covers private Bearer/internal protocol fencing, browser Cookie/Origin/Referer/CSRF forwarding, all five operations and raw bodies, duplicate update, owner failure, timeout, cancellation, Rust crash fail-closed behavior, safe Go rollback/restart before real store mutation, and unchanged settings bytes.
- Rust product rehearsal: passed (`cargo test -p jftrade-engine --lib strategy_definition_write_product -- --nocapture`). It covers browser 401/403, unavailable and owner-error recovery, all five operations, duplicate update and instantiate forwarding, restart recovery, explicit test-cutover registration, and unchanged settings bytes.
- Rust durable test-cutover replay: passed (`cargo test -p jftrade-engine --test stage9_strategy_definitions_write -- --nocapture`, 8 tests), including concurrent same-content version fencing, trigger-failure rollback, linked-instance delete guard and soft-delete, missing-definition precedence, and close/reopen persistence. The isolated adapter's product restart replay also passed (`cargo test -p jftrade-engine --lib strategy_definition_sqlite_test_cutover_replays_transport_and_restart -- --nocapture`).
- Shared product differential: passed after integration registration (`pnpm run test:rust:stage9:product-differential`, Rust product library replay passed with 219 tests).
- `route-ownership.json` records all five operations as `cutover-test-only`; `productionOwner=go` and `goRemovalStatus=retained` remain unchanged.
- `pnpm run check:quick` passed (affected quick checks, including the full `servercoretest` package, Rust all-target tests, architecture checks, clippy, and generated-contract check). `pnpm run check:rust` passed (workspace fmt, clippy, all-target tests, Stage 3–8 differentials, full Stage 9 product differential, and supporting package replay). Generated-contract checks passed without modifying the worktree; no public contract changed.
- Remaining blockers: durable definition version/transaction ownership; atomic linked-delete and instantiate/catalog recovery; cancellation/restart fencing; Pine/runtime/activity/notification/task isolation; production unique-writer switching; four-platform signed Tauri release/updater; security/SBOM; backup/restore and hard-cut gates.

## Isolated durability extension (2026-08-26)

The test-cutover adapter now persists generated definition and instance IDs in
the isolated SQLite fixture instead of an in-memory counter. Instantiation is
one `BEGIN IMMEDIATE` transaction that writes the instance projection and the
definition's linked-instance index; linked delete and apply read that same
durable instance set. Triggered instance-write failures prove allocator,
instance-row and linked-index rollback together, while close/reopen tests prove
instance identity and linkage survive restart. The adapter also holds a
test-only file writer lease, so a second owner of the same fixture is rejected
before opening SQLite. This is an isolated test profile and does not claim
compatibility with the Go strategy database or a Rust production writer.

The focused replay now passes 9 tests, including instance transaction rollback,
persisted linked-instance delete fencing, persistent ID allocation across
restart, and second-owner lease rejection. The authenticated product replay
also instantiates through transport, closes, reopens, and instantiates again
without reusing the persisted ID. Go remains the sole production owner and the
route entries remain `cutover-test-only`; production catalog/runtime ownership,
Pine/runtime side-effect fencing, release/signing, security, backup/restore and
hard-cut evidence are still open.
