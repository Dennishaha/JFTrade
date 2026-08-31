# Storage overview read

> Historical/rehearsal evidence notice (pre-2026-08-31): owner labels, route counts, and Go/retained statuses in this ledger describe the qualification snapshot at capture time, not current production ownership.
>
> Current route truth is derived from `node scripts/rust-migration/check-stage9-route-coverage.mjs` and `tests/fixtures/rust-migration/stage9/route-ownership.json`; formal release truth is `node scripts/rust-migration/check-stage9-closeout.mjs --check`. The original evidence below is intentionally retained verbatim.

- Group: `storage-overview-read`.
- Tier: C: this is a side-effect-free immutable projection while persistent
  task queues remain unowned.
- Operation: `GET /api/v1/system/storage/overview`.
- Owner: Go remains the production owner. Both implementations expose the same
  four empty arrays for pending outbox, jobs, audit logs and execution
  commands; Rust opens no database or queue.

The Rust authenticated product test asserts the complete data projection. The
Go authenticated sidecar rehearsal preserves exact status/body/headers and
verifies error, timeout, crash and restart-time Go rollback.

Verification: `go test ./internal/app/apiserver/servercoretest -run '^TestStorageOverviewReadRehearsalPreservesWireAndRequiresRestartForGoRollback$' -count=1`; `cargo test -p jftrade-engine --lib product::tests::system_control_read_tests::storage_overview_matches_the_go_empty_projection_behind_authentication -- --exact`; `pnpm run test:rust:stage9:product-differential`.

The route is `cutover-qualified`, `productionOwner=go`, and
`goRemovalStatus=retained`. Current coverage is 1 shadow / 118
cutover-test-only / 159 cutover-qualified / 0 remaining / 0 Rust production
owner.
