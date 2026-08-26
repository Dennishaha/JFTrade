# System control reads

- Group: `system-control-read`.
- Tier: C for the immutable OpenD install projection; Tier B for the seven
  real-trade safety-control projections because they expose runtime risk,
  hard-stop and kill-switch state.
- Owner: Go remains the production owner. Rust reads only the existing
  settings and real-trade control files; it never activates OpenD, changes a
  risk limit, releases a hard stop, toggles the kill switch, writes an event or
  sends a notification.
- Frozen references:
  `tests/fixtures/rust-migration/stage9/product-slice-corpus.json` covers four
  install-guide settings cases, and
  `tests/fixtures/rust-migration/stage9/real-trade-control-corpus.json` plus the
  generated Go reference cover missing, active, runtime-overlay, event and
  malformed fail-closed control states.

The authenticated sidecar rehearsal preserves exact status/body/headers for
all eight operations and verifies error, timeout, crash and restart-time Go
rollback. The Rust authenticated product test verifies the route assembly and
token boundary on a missing control file, then proves the read batch neither
changes settings nor creates `real-trade-control.json`. Populated and malformed
state equivalence remains covered by
`stage9_real_trade_reads_match_current_go_owner`.

## Verification

- Go/Rust leaf references: `pnpm run test:rust:stage9:product-differential`.
- Rust authenticated/read-only route batch: `cargo test -p jftrade-engine --lib product::tests::system_control_read_tests::system_control_reads_are_authenticated_and_do_not_create_control_state -- --exact`.
- Go authenticated rehearsal: `go test ./internal/app/apiserver/servercoretest -run '^TestSystemControlReadRehearsalPreservesWireAndRequiresRestartForGoRollback$' -count=1`.
- Current route coverage: 1 shadow / 118 cutover-test-only / 159 cutover-qualified / 0 remaining / 0 Rust production owner.

All eight route entries are `cutover-qualified`, `productionOwner=go`, and
`goRemovalStatus=retained`. The corresponding seven mutations remain
`cutover-test-only`; no production owner or writer lease moved to Rust.
