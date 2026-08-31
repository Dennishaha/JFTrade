# Watchlist Write Group Ledger

> Historical/rehearsal evidence notice (pre-2026-08-31): owner labels, route counts, and Go/retained statuses in this ledger describe the qualification snapshot at capture time, not current production ownership.
>
> Current route truth is derived from `node scripts/rust-migration/check-stage9-route-coverage.mjs` and `tests/fixtures/rust-migration/stage9/route-ownership.json`; formal release truth is `node scripts/rust-migration/check-stage9-closeout.mjs --check`. The original evidence below is intentionally retained verbatim.

- Group: `watchlist-write`
- Tier: A mutation; eight local watchlist routes are covered as one group.
- Production owner: Go remains the sole owner. Rust is a consumer-owned, test-only mutation leaf and is not registered in the default profile.
- Routes:
  - `DELETE /api/v1/watchlist/bindings`
  - `DELETE /api/v1/watchlist/groups/{groupId}`
  - `PATCH /api/v1/watchlist/groups/{groupId}`
  - `POST /api/v1/watchlist/groups`
  - `POST /api/v1/watchlist/imports/preview`
  - `POST /api/v1/watchlist/imports/{previewId}/commit`
  - `POST /api/v1/watchlist/quotes/batch`
  - `PUT /api/v1/watchlist/instruments/{market}/{symbol}/memberships`
- Fixture: `tests/fixtures/rust-migration/stage9/watchlist-write.json` (`stage9.watchlist-write.v1`).
- Go reference: `scripts/rust-migration/stage9_watchlist_write_reference_test.go`. It uses the real Gin handlers and `internal/watchlist.Service`, with an in-memory repository, source reader, and quote double; it does not open production SQLite or connect to OpenD/provider workers.
- Rust leaf: `crates/jftrade-engine/src/product_watchlist_write_port.rs`.
- Rust replay: `crates/jftrade-engine/tests/stage9_watchlist_write.rs`.
- Differential: `scripts/rust-migration/check-stage9-watchlist-write.mjs`.

## Contract and A-tier evidence

| Method | Path | Request/response contract and mutation boundary | Error and recovery coverage |
| --- | --- | --- | --- |
| DELETE | `/api/v1/watchlist/bindings` | Reads `bindingId` from the query and returns `{deleted:true}` in the normal envelope. | Missing binding is `404 WATCHLIST_NOT_FOUND`; missing ID is Go validation; malformed query is `400 BAD_REQUEST`; repeat delete is covered. |
| DELETE | `/api/v1/watchlist/groups/{groupId}` | Deletes the local group and returns `{deleted:true}`. | Missing/protected groups are `404`/`409`; malformed request bodies are ignored by Go and covered; repeat delete and membership cleanup are covered. |
| PATCH | `/api/v1/watchlist/groups/{groupId}` | Binds name and `expectedRevision`, then returns the revised group. | Malformed body, missing group, protected group, stale revision, repository failure/recovery, cancellation, and concurrent revision fencing are covered. |
| POST | `/api/v1/watchlist/groups` | Binds name and returns a newly created group. | Required/trimmed name, duplicate name, unknown/trailing JSON, repository unavailable, failure rollback/recovery, cancellation, and explicit no-port fail-closed behavior are covered. |
| POST | `/api/v1/watchlist/imports/preview` | Builds and persists an import preview from a source group and local/new-group selection. | Malformed body, missing source, ambiguous remote group, local diff, default new-group naming, and unavailable port are covered. |
| POST | `/api/v1/watchlist/imports/{previewId}/commit` | Accepts an empty body or delete list and returns the completed import run. | Empty body, missing/expired/stale preview, repeat commit fencing, invalid delete, and transaction failure fixture paths are covered. |
| POST | `/api/v1/watchlist/quotes/batch` | Normalizes/deduplicates instrument IDs and returns quote/error arrays with an observation timestamp. | Malformed/empty payload, no source, per-item errors, provider/cancellation behavior, and deduplication are covered. |
| PUT | `/api/v1/watchlist/instruments/{market}/{symbol}/memberships` | Replaces memberships with revision fencing and optional new group names. | Alias normalization, idempotent repeat, missing group, stale revision, invalid market, and failure rollback are covered. |

The fixture records request/response headers, envelope, port-call boundary, action payloads, state observation, repeat semantics, and the concurrent revision-fence case. Rust only receives a `WatchlistWritePort` in the explicit replay test; it never constructs a store, quote worker, provider, OpenD client, or production route.

## Quirks and three-way review

### Missing binding query value is an empty string

quirk: `DELETE /api/v1/watchlist/bindings?sourceId=futu:default` reaches the Go service with `bindingId=""`; the consumer mutation payload is an empty JSON string, not JSON `null`.

范围: `watchlist-write` / `DELETE /api/v1/watchlist/bindings`

证据: Go reference fixture case `delete-binding-missing-id` records `calls[0].bindingId == ""` and returns `400 WATCHLIST_INVALID`; the initial Rust replay produced `null` and failed the action comparison; after the Rust `unwrap_or_default()` fix, the fixture replay and dedicated differential are green.

分类: `rust-implementation`

判定: `deviated` in the initial Rust implementation; fixed to match Go observable behavior.

处置: 修复 Rust，使缺失 query value 映射为空字符串；保留该 quirk 作为 wire/port compatibility evidence。

风险: low

owner: Rust test-only leaf

后续: no hard-cut action; re-run the group differential whenever the query adapter changes.

三方复核: Go handler/service baseline → frozen JSON fixture/action trace → Rust leaf replay all agree after the fix.

### Go body binding quirks intentionally preserved

quirk: DELETE group/binding handlers do not bind a JSON body, so malformed body bytes do not prevent a valid delete route from reaching the service. Go JSON binding for create accepts the first JSON value and ignores unknown fields and trailing values. Commit accepts an empty body but rejects a malformed non-empty body.

范围: `watchlist-write` / delete group, delete binding, create group, commit import

证据: fixture cases `delete-group-success-and-repeat`, `delete-binding-success-and-repeat`, `create-unknown-field-and-trailing-value`, and `commit-success-and-repeat`; Rust unit tests and group replay match the same statuses and port-call boundaries.

分类: `go-behavior`

判定: `intended` for migration compatibility

处置: 复刻，待硬切后若产品决定收紧输入再单独做契约变更；本切片不修 Go。

风险: low

owner: Go contract / integration review

后续: retain through Go deletion gate; any contract tightening requires a separately approved API change.

### Quote cancellation remains a successful envelope with item error

quirk: when the quote source observes a canceled context, the Go watchlist service returns HTTP 200 with `SNAPSHOT_FAILED` in the per-item errors array rather than an HTTP 500/503 handler error.

范围: `watchlist-write` / `POST /api/v1/watchlist/quotes/batch`

证据: fixture case `quotes-cancelled-source`; Go reference, frozen envelope, and Rust replay agree on status, error code/message, and empty quotes array.

分类: `go-behavior`

判定: `intended` for migration compatibility

处置: 复刻，待硬切后修复 only if an explicit API decision changes the observable contract.

风险: medium

owner: Go watchlist service

后续: must remain in the final hard-cut quirk inventory; not cutover-qualified by this slice alone.

## Ownership and gate status

- The integration branch registers all eight operations as `cutover-test-only`
  only when the explicit `WatchlistWritePort` is supplied;
  `productionOwner=go` and `goRemovalStatus=retained` remain unchanged.
  Current route coverage is `1 shadow / 118 cutover-test-only / 159
  cutover-qualified / 0 remaining / 0 Rust production owner`.
- No default profile, shared product wiring, unified differential, OpenAPI, SQLite schema, Wails binding, provider/OpenD lifecycle, or production owner changed.
- The authenticated Go loopback rehearsal now proves private Bearer/internal
  protocol fencing, browser Cookie/Origin/Referer/CSRF forwarding, all eight
  operations, duplicate forwarding, error, timeout, cancellation, Rust crash
  fail-closed behavior, Go rollback, restart recovery, and unchanged settings
  bytes. The Rust product replay proves explicit registration, no-port route
  isolation, browser auth/CSRF fencing, unavailable/error recovery, all eight
  route projections, duplicate forwarding, restart, and no settings mutation.
- An isolated SQLite test-cutover adapter now proves concurrent revision
  fencing, transaction rollback on a trigger failure, durable import preview
  and one-shot commit fencing, group/revision/event persistence across close
  and reopen, and allocator recovery from the durable event sequence. The
  authenticated Rust product route replay also reopens that temporary database
  and verifies a post-restart mutation without touching the settings file.
- A-tier gates still not proven by this rehearsal: production unique-owner
  switch, compatibility with the real Go watchlist schema and production
  store, cross-process writer fencing, provider quote lifecycle,
  notification/task side-effect isolation, production backup/restore and crash
  recovery, four-platform signed release, security/SBOM review, and final
  hard-cut checklist. These blockers keep the group `cutover-test-only`.

## Verification handoff

- Go fixture/reference and Rust leaf differential:
  `node scripts/rust-migration/check-stage9-watchlist-write.mjs` passed for all
  eight routes.
- Authenticated Go loopback rehearsal:
  `go test ./internal/app/apiserver/servercoretest -run '^TestWatchlistWriteRehearsalFencesOwnersAndRecoversAcrossRestart$' -count=1 -timeout=300s`
  passed.
- Authenticated Rust product replay:
  `cargo test -p jftrade-engine --lib watchlist_write_product -- --nocapture`
  passed, including route isolation, recovery, restart, and unchanged settings.
- Isolated SQLite durability replay:
  `cargo test -p jftrade-engine --test stage9_watchlist_write -- --nocapture`
  passed all seven tests, including concurrent revision fencing, trigger
  rollback, one-shot import commit, close/reopen persistence, and durable ID
  allocator recovery.
- Product SQLite restart replay:
  `cargo test -p jftrade-engine --lib watchlist_sqlite_test_cutover_replays_transport_and_restart -- --nocapture`
  passed with all eight product routes exercised across the initial process and
  a successful post-restart mutation against the reopened temporary database.
- `pnpm run check:quick` passed after the Rust target-health cache was safely
  cleaned with no Cargo process active.
- `pnpm run check:rust` passed, including workspace fmt/Clippy/all-target tests,
  Stage 9 Go references and authenticated rehearsals, Rust product and
  integration replay, all 221 engine library tests, and supporting package
  contracts. The complete Stage 9 product differential passed in this gate.
- Final route coverage passed at `1 shadow / 118 cutover-test-only / 159
  cutover-qualified / 0 remaining / 0 Rust production owner`; both changed JSON
  files parsed successfully, and `git diff --check` passed.
- No default route, production owner, OpenAPI/Wails contract, SQLite schema, or
  Go/Wails deletion changed.
