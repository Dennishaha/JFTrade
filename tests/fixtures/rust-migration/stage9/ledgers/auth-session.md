# Auth Session Route Group Ledger

- Group: `auth-session`
- Tier: B (session/auth context, CSRF/origin/trusted-desktop behavior and transport headers require differential evidence)
- Operation: `GET /api/v1/auth/session`
- Production owner: Go (`internal/app/apiserver/webaccess.Auth.Status`); Rust remains explicit test-cutover or shadow only.
- Route ownership: `cutover-qualified` rehearsal; the route is registered only with the explicit snapshot port. Go remains the production owner and `goRemovalStatus=retained`; the default profile remains unchanged.
- Fixture: `tests/fixtures/rust-migration/stage9/auth-session.json`

## Three-Way Reviewed Quirks

quirk: A prior integration snapshot reported `E0382` from reading `input.path` after `dispatch(input).await`; the current shared router snapshots the auth-session predicate before dispatch.
范围: `auth-session` / transport boundary / shared locked router / `GET /api/v1/auth/session`
证据: `cargo test -p jftrade-api --test auth_session_transport_contracts` compiles `jftrade-api` and passes the dedicated success/error regression.
分类: rust-implementation
判定: deviated
处置: Keep the shared-router ownership fix with the integration branch; this worker adds only the dedicated transport regression and does not modify the router.
风险: low
owner: integration
后续: Preserve the regression while this route remains cutover-test-only.
三方复核结论: Go owner behavior is frozen by the reference test; the generated fixture records the wire contract; the Rust transport compiles and replays the matching route without the former move error.

quirk: The partial Go reference test invoked header collectors that did not exist, so it could not freeze the full auth-session response header contract.
范围: `auth-session` / fixture harness / `GET /api/v1/auth/session`
证据: `go test ./scripts/rust-migration -run '^TestStage9AuthSessionFixtureMatchesCurrentGoOwner$' -count=1` passes after the collectors generate `responseHeaders` and lowercase `absentHeaders` for all five cases.
分类: harness
判定: deviated
处置: Capture the complete application-controlled header inventory and explicitly list every absent origin-specific header; leave HTTP framing headers outside the fixture.
风险: low
owner: worker
后续: Regenerate only through the reference test when Go observable behavior changes.
三方复核结论: Go emits the captured headers, `auth-session.json` stores the result, and the Rust engine test compares every expected and absent header against that fixture.

quirk: An allowed browser origin contains the ephemeral `httptest` port, while CSRF and session expiry are also runtime-generated.
范围: `auth-session` / `GET /api/v1/auth/session` / allowed-origin browser session
证据: The Go reference test normalizes non-empty `csrfToken` to `fixture-csrf-token`, non-empty `expiresAt` to `fixture-time`, and only `access-control-allow-origin` to `https://fixture.jftrade.local`.
分类: fixture
判定: intended
处置: Do not normalize any other response value or header; the Rust replay sends the same fixed allowed origin and `X-Request-ID: fixture-auth-session-id`.
风险: low
owner: worker
后续: Keep the normalization set limited to these three documented dynamic values.
三方复核结论: Go baseline produces the dynamic values, the fixture canonicalizes only those values, and the Rust replay matches the canonical allowed-origin case exactly.

quirk: Responses with no origin, including `ORIGIN_FORBIDDEN`, still expose the three base CORS capability headers but omit `access-control-allow-credentials`, `access-control-allow-origin`, and `vary`.
范围: `auth-session` / `GET /api/v1/auth/session` / unauthenticated, desktop-trusted, and forbidden-origin cases
证据: The Go fixture has all three base CORS headers in every case and lists the three origin-specific names in lowercase `absentHeaders` unless the origin is allowed.
分类: go-behavior
判定: intended
处置: Reproduce the observed header matrix; do not reinterpret CORS absence as an auth or status difference.
风险: low
owner: Go until cutover
后续: Retain the expected/absent header assertions in the test-cutover replay.
三方复核结论: Go middleware generated the matrix, the fixture preserves it, and the Rust test asserts both the complete expected subset and every explicit absence.

quirk: `Cache-Control: no-store` is required on both successful auth-session projections and error envelopes, including `ORIGIN_FORBIDDEN` and test-cutover snapshot-unavailable failures.
范围: `auth-session` / `GET /api/v1/auth/session` / success and error transport paths
证据: Go `writeAuthJSON` and `writeAuthError` set `no-store`; the fixture records it for 200 and 403; the engine replay's unavailable-port path asserts it for 503; `auth_session_transport_contracts.rs` passes for 200 and 403.
分类: go-behavior
判定: intended
处置: Preserve the shared transport behavior through the dedicated regression without modifying the shared router in this slice.
风险: low
owner: integration for shared transport, Go for production owner
后续: Keep the route behind the explicit snapshot port until production-owner and release evidence is complete.
三方复核结论: Go owner, frozen fixture, and Rust engine/API transport replay all agree on `Cache-Control: no-store`.

quirk: Test-cutover distinguishes an injected unavailable snapshot port (`503 AUTH_SESSION_UNAVAILABLE`) from an absent snapshot port (route not registered, `404 NOT_FOUND`).
范围: `auth-session` / explicit test-cutover wiring / unavailable and no-port isolation
证据: `cargo test -p jftrade-engine auth_session --lib` passes all three auth-session tests, including both isolated paths; the default profile remains without this route when no port is injected.
分类: rust-implementation
判定: intended
处置: Keep the port optional and fail closed; do not create a Go-like session store, browser cookie owner, or production route registration in this slice.
风险: medium
owner: Rust test-cutover wiring
后续: Keep the explicit consumer-owned snapshot port throughout test-cutover qualification.
三方复核结论: Go remains the live session owner, the Go fixture supplies only observable session projections, and Rust demonstrates that its test-only adapter cannot silently replace or fall back to the Go-owned route.

quirk: Review reported a duplicate `t.Cleanup(browserServer.Close)`, but the current reference test has exactly one browser-server cleanup at line 85 and a distinct desktop-server cleanup at line 101.
范围: `auth-session` / Go fixture harness / test server resource cleanup
证据: `rg -n 't\\.Cleanup\\(browserServer\\.Close\\)' scripts/rust-migration/stage9_auth_session_reference_test.go` returns only line 85; the focused Go reference test completes successfully after `gofmt`.
分类: harness
判定: deviated
处置: Preserve the single required browser-server cleanup; no duplicate source line exists to remove in the current worktree.
风险: low
owner: worker
后续: Recheck the exact cleanup count if the fixture server setup changes.
三方复核结论: The Go reference test passes with one cleanup per server, the generated fixture remains unchanged and current, and the Rust engine/API transport replays pass after the review check.

quirk: The prior ledger EOF-format observation and the shared production-file size
gate blocker were resolved by the integration branch's composition-file splits;
the original quick-gate failure is retained as historical evidence.
范围: repository quick gate / shared `jftrade-engine` composition files
证据: `pnpm run check:quick` stops at `product.rs` 811, `product_api.rs` 803, and `product_wire.rs` 801 lines against the 800-line limit; no auth-session assertion failed.
分类: harness
判定: deviated
处置: Integration extracted the resource-integrity, Pine worker API, and provider-wire contributors; the current files are below the 800-line limit and retain exactly one terminal newline.
风险: low
owner: integration
后续: Preserve the split and rerun the broad gate; no auth-session behavior change is required.
三方复核结论: The historical Go reference and Rust route/API tests are green, the fixture is current, the extracted files satisfy layout inspection, and the former blocker is no longer present in the current worktree.

## Verification record

- Go reference fixture and Rust product/transport replay: passed (`node scripts/rust-migration/check-stage9-auth-session.mjs`). This runs the Go owner fixture, authenticated loopback rehearsal, Rust product auth-session tests, and the `jftrade-api` auth-session transport contract.
- The authenticated rehearsal covers exact read wire/header forwarding, Rust private bearer fencing, error/timeout/crash fail-closed behavior, restart recovery to the Go owner, and no settings mutation. Go fixture cases separately cover browser session, allowed/forbidden Origin, desktop-trusted access, CORS header presence/absence, and normalized CSRF/session expiry projections.
- Full Stage 9 product differential: passed (`pnpm run test:rust:stage9:product-differential`).
- Quick and full Rust repository gates: passed (`pnpm run check:quick`; `pnpm run check:rust`).
- Route coverage and closeout/ownership tests: passed (`node scripts/rust-migration/check-stage9-route-coverage.mjs`; `node --test scripts/rust-migration/check-stage9-closeout.test.mjs scripts/rust-migration/stage9-route-ownership.test.mjs`).

## Cutover-qualified status

The Go reference fixture, Rust leaf/product replay, authenticated read rehearsal, and transport contract are green for `GET /api/v1/auth/session`. The route is cutover-qualified only under the explicit snapshot port; it has no Rust password, browser-session, CSRF-store, cookie, SQLite, Provider/OpenD, or user-visible production side-effect owner. This is `cutover-qualified`, not a production migration; Go remains the unique production owner.

Production session-store integration, credential/security review, four-platform signed release/updater, SBOM, backup/restore, and final unique-owner/hard-cut approval remain open in the Stage 9 closeout manifest.
