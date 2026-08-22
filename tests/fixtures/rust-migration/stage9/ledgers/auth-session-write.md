# Auth Session Write Group Ledger

- Group: `auth-session-write`
- Tier: A, mutation operations
- Operations: `POST /api/v1/auth/login`; `POST /api/v1/auth/logout`
- Current production owner: Go Web access middleware and session service; Rust has no production owner.
- Current route ownership: `remaining` until integration wiring and evidence are complete. The intended test-cutover state registers these routes only when an explicit `AuthSessionWritePort` is supplied; the default profile remains unchanged.
- Fixture: `tests/fixtures/rust-migration/stage9/auth-session-write.json`
- Go reference: `internal/app/apiserver/webaccess/auth_session_write_reference_test.go`
- Rust leaf/test: `crates/jftrade-engine/src/product_auth_session_write_port.rs`; `crates/jftrade-engine/tests/stage9_auth_session_write.rs`
- Differential: `node scripts/rust-migration/check-stage9-auth-session-write.mjs`

| Method | Path | Request, response, and state contract | Error branches covered |
| --- | --- | --- | --- |
| POST | `/api/v1/auth/login` | Origin validation, trusted-desktop bypass, Web access/configuration checks, login rate limiting, first JSON value decoding, password verification, session creation, CSRF/session-cookie projection and dynamic timestamp normalization follow the Go owner. | `400 BAD_REQUEST`, `401 INVALID_PASSWORD`, `403 ORIGIN_FORBIDDEN`/`WEB_ACCESS_DISABLED`, `408 REQUEST_CANCELED`, `409 WEB_AUTH_CONFIGURATION_CHANGED`, `429 LOGIN_RATE_LIMITED` with `Retry-After`, `500 WEB_AUTH_FAILED`, and `503 WEB_AUTH_UNAVAILABLE`. |
| POST | `/api/v1/auth/logout` | Middleware authentication/origin/CSRF precedence is reproduced before the injected port. The handler ignores malformed body, returns the Go success envelope, and carries session-cookie deletion through the consumer-owned port result. | Middleware `401`/`403` responses preserve the Go header omissions; unavailable state port is `503`; injected state failure uses the Go error envelope. |

## Three-way review and quirks

The Go reference runs the actual Web access route and middleware with deterministic settings and an in-memory session boundary. The fixture freezes status, selected headers, error envelope, dynamic-value normalization, and whether the state port was called. The Rust replay uses only an injected `AuthSessionWritePort`; it never verifies a real password, persists a browser session, or creates a production cookie.

quirk: A JSON `null` login body is decoded as an empty password, and the decoder accepts a valid first JSON value followed by trailing JSON.
范围: `auth-session-write` / `POST /api/v1/auth/login`
证据: Go reference cases `login-empty-password` and `login-null-and-trailing-json`; fixture and Rust replay
分类: go-behavior
判定: intended
处置: 复刻，待硬切后修复
风险: low
owner: Go until cutover
后续: Preserve the exact decoder and null semantics; do not add strict trailing-byte validation in the migration slice.

quirk: Trusted desktop login bypasses body/configuration validation and returns an empty `csrfToken` without a session cookie.
范围: `auth-session-write` / `POST /api/v1/auth/login`
证据: Go reference case `login-trusted-desktop-bypasses-payload`; fixture headers/envelope; Rust replay
分类: go-behavior
判定: intended
处置: 复刻，待硬切后修复
风险: medium
owner: Go until cutover
后续: Keep trusted-host fencing in the composition-root adapter and never enable it from an unauthenticated public profile.

quirk: After eight failed attempts for the same remote key, Go rate-limits before reading the request body and returns `429 LOGIN_RATE_LIMITED` with `Retry-After: 300`; the state port is not called.
范围: `auth-session-write` / `POST /api/v1/auth/login`
证据: Go `Auth.Login` rate-limit check and case `login-rate-limit-after-eight-failures`; fixture call trace and headers; Rust replay
分类: go-behavior
判定: intended
处置: 复刻，待硬切后修复
风险: high
owner: Go until cutover
后续: A future adapter must delegate the existing limiter decision and preserve remote-key, ordering, and retry-header semantics without a second Rust limiter.

quirk: The initial Rust replay treated the ninth rate-limited login as a missing port response and returned `500` instead of Go's pre-handler `429`.
范围: `auth-session-write` / `POST /api/v1/auth/login`
证据: first Rust replay failure for `login-rate-limit-after-eight-failures` (`left: 500`, `right: 429`); Go fixture; current `AuthSessionWritePort::login_rate_limit` replay
分类: rust-implementation
判定: deviated
处置: 修复 Rust 使其匹配 Go；the leaf now asks the injected consumer-owned port for the pre-body rate-limit decision and maps `Retry-After`.
风险: high
owner: integration branch
后续: Keep the fix covered by the dedicated differential before any qualification decision.

quirk: The first Rust fixture assertion omitted `Retry-After` from its owned header projection, so a correct `429` response was reported as a header mismatch.
范围: `auth-session-write` / Rust fixture harness
证据: replay after the rate-limit fix; fixture `login-rate-limit-after-eight-failures` expected `retry-after: 300`; `owned_expected_headers` before correction
分类: harness
判定: deviated
处置: 修复 fixture/harness；the assertion now compares `Retry-After` with the Go fixture.
风险: low
owner: integration branch
后续: Keep rate-limit headers in the group differential and do not normalize them away.

quirk: Logout middleware failures omit `Cache-Control`, while successful handler responses include `no-store` and cookie deletion; logout also ignores malformed JSON body.
范围: `auth-session-write` / `POST /api/v1/auth/logout`
证据: Go reference cases `logout-unauthenticated`, `logout-browser-requires-origin`, `logout-browser-rejects-invalid-csrf`, and `logout-browser-ignores-malformed-body`; fixture selected headers; Rust replay
分类: go-behavior
判定: intended
处置: 复刻，待硬切后修复
风险: medium
owner: Go until cutover
后续: Preserve middleware/handler header boundaries and body-ignore behavior exactly.

## Test-cutover status

The leaf is fenced behind an injected state port and has no password, session-store, CSRF-store, or cookie-generation implementation. Go remains the only production owner. Tier A evidence remains outstanding for duplicate-request policy, cancellation/timeout fencing, session persistence/restart recovery, security review, four-platform release/signing, backup/restore, and final unique-owner approval.
