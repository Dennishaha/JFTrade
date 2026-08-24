# Stage 9 closeout shared harness

状态：集成分支维护的共享收口证据；不代表任何 route group 已切换 production owner。

## Three-way review and quirks

quirk: The open-state closeout test matched only the old `remaining operation` blocker. Once the dynamic ownership ledger reached `0 remaining` while still having `252 cutover-test-only` operations, the checker correctly emitted `operation(s) not cutover-qualified` and `check:quick` failed in the test harness.

范围: Stage 9 closeout checker, route ownership snapshot, and closeout evidence manifest
证据: OpenAPI/Go route baseline `tests/fixtures/rust-migration/stage7/api-control-plane-corpus.json`; derived ledger output from `scripts/rust-migration/stage9-route-ownership.mjs`; closeout output and assertion in `scripts/rust-migration/check-stage9-closeout.mjs` and `check-stage9-closeout.test.mjs`; Rust product differential remains green.
分类: harness
判定: confirmed harness drift; the ownership counts and Rust/Go route behavior were not changed by this correction.
处置: 修复 fixture/harness；the assertion accepts either semantically valid open-state blocker, and the manifest evidence defers counts to the dynamic ledger.
风险: low
owner: 集成分支
后续: keep closeout messages derived from the ledger and never hand-maintain route counts in evidence text.

quirk: The Rust default shadow-catalog assertion selected every `cutover-qualified` ledger entry as a default route. After strategy-definitions-read and backtests-run-read reached qualification, their explicit snapshot-port routes were incorrectly added to the expected default catalog even though the product assembly correctly keeps them out without test-cutover ports.

范围: `jftrade-engine` default route catalog assertion / qualified read groups using consumer-owned snapshot ports.
证据: Go ownership dependencies in `tests/fixtures/rust-migration/stage9/route-ownership.json`; default Rust assembly in `product_route_assembly.rs` and `product_routes_backtests.rs`; group tests `product_strategy_definitions_tests.rs` and `product_backtests_tests.rs` proving the routes are absent without a snapshot port; failed `read_only_shadow_catalog_never_registers_write_or_notification_routes` assertion.
三方复核: Go/ledger route dependency classification, Rust default and explicit test-cutover route catalogs, and the shared harness assertion were compared. The Go observable HTTP contract and Rust route implementation were unchanged.
分类: harness
判定: confirmed harness drift
处置: 修复 fixture/harness；the assertion now includes only the three qualified routes intentionally registered in the default authenticated shadow and retains explicit-port routes as test-cutover-only. No route ownership, production owner, or default registration was broadened.
风险: low
owner: 集成分支
后续: when a future group becomes default-shadow registered, update this explicit boundary list together with its composition-root change and evidence.

## Current boundary

The current dynamic snapshot is `23 shadow / 232 cutover-test-only / 23 cutover-qualified / 0 remaining / 0 Rust production owner`. Go remains the only production owner; all route rehearsal profiles stay explicit test-cutover only or authenticated shadow, and Go/Wails deletion remains blocked by the formal gates.
