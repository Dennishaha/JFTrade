# ADK Chat Stream Route Group Ledger

- Group: `adk-chat-stream`
- Tier: B
- Operations: 2 (`POST /api/v1/adk/chat`, `POST /api/v1/adk/chat/stream`)
- Current status: `cutover-test-only`; Go remains the sole production owner.
- Production owner: Go `internal/api/assistant/chat.go`, `chat_stream_hub.go`, and the Assistant service/runtime. Rust has no production registration, runtime, Provider, SQLite, network, or side-effect path.
- Fixture: `tests/fixtures/rust-migration/stage9/adk-chat-stream.json`
- Go reference: `scripts/rust-migration/stage9_adk_chat_stream_reference_test.go`
- Rust replay: `crates/jftrade-engine/src/product_adk_chat_stream_port.rs` and `crates/jftrade-engine/tests/stage9_adk_chat_stream.rs`
- Differential: `node scripts/rust-migration/check-stage9-adk-chat-stream.mjs`

## Frozen Contract

The Go reference freezes 17 cases covering normal chat, empty message, malformed JSON, invalid UUID, runtime unavailable, provider failure, idempotency conflict, authentication/CSRF failures, unknown method, stream event ordering and IDs, retry directive, idle timeout header, terminal close, and client disconnect with retained replay. The provider is an `httptest` local Responses endpoint; no real model/provider, production runtime, or production SQLite is used.

The Rust leaf accepts a complete Go-owned projection through `AdkChatStreamPort`. It preserves JSON envelopes, stream `Content-Type`, `Cache-Control`, `Connection`, idle timeout, retry directive, event IDs, event order, event data, terminal state, 400/409/502/503 mapping, and fail-closed behavior when no port is injected. The port is intentionally consumer-injected and has no default registration.

## Three-Way Review And Quirks

quirk: Go success and failure projections contain runtime-generated session/run IDs, context revision IDs, final message IDs, timestamps, elapsed durations, and a temporary local provider port.
范围: `adk-chat-stream` / JSON and SSE success/provider-failure projections
证据: repeated Go reference runs; `stage9NormalizeADKValue`, provider URL normalization, and regenerated fixture; Rust replay
分类: fixture
判定: intended
处置: Normalize only these execution metadata values to `fixture-*`; preserve all user-visible fields, event names, IDs/sequence, ordering, and error text.
风险: low
owner: Go reference harness
后续: Regenerate through the reference test when the Go wire projection changes.
三方复核结论: Go owner produced the values, the fixture records the canonical projection, and Rust replay matches the canonical data and wire encoding.

quirk: A provider HTTP 500 is observable as HTTP 200 with a failed run/final response payload for both chat forms; it is not promoted to an HTTP 5xx by the Go route.
范围: `POST /api/v1/adk/chat` and `/stream` / provider failure
证据: local provider failure cases in the Go reference and fixture; Rust replay of `MODEL_CALL_FAILED` payload and final SSE event
分类: go-behavior
判定: intended
处置: Reproduce the 200 projection; do not “fix” the Go error precedence in this migration slice.
风险: medium
owner: Go until cutover
后续: Integration must retain this behavior in product differential and hard-cut review.
三方复核结论: Go response, frozen fixture, and Rust port replay agree; no real provider is contacted.

quirk: Malformed JSON on the stream route returns status 200 SSE with `retry: 3000` followed by a terminal `type=error` event, while a syntactically valid body with an invalid/missing UUID returns JSON 400 and retains `X-ADK-Stream-Idle-Timeout-Ms`.
范围: `/api/v1/adk/chat/stream` / payload decode and identity validation precedence
证据: `chat.go` decode-before-normalize order, Go fixture `stream-invalid-json` and `stream-invalid-client-request-id`, Rust dispatch tests
分类: go-behavior
判定: intended
处置: Preserve the split; do not apply stricter Rust schema validation before the stream handler's JSON/SSE decision.
风险: high
owner: Go until cutover
后续: Integration must include both cases in the authenticated product differential.
三方复核结论: Reference transport, fixture frames/headers, and Rust leaf assertions agree.

quirk: The runtime-unavailable stream case has no idle-timeout header because the Go `requireAvailable` middleware rejects before `handleADKChatStream` sets that header; stream errors reached after handler entry do carry it.
范围: `/api/v1/adk/chat/stream` / unavailable runtime versus handler-level error precedence
证据: `internal/api/assistant/handler.go`, Go `stream-runtime-unavailable` and `stream-idempotency-conflict` fixture cases, Rust port/no-port tests
分类: go-behavior
判定: intended
处置: Keep the no-port leaf fail-closed response separate from handler-level stream error mapping.
风险: high
owner: integration composition and Go until cutover
后续: Product wiring must preserve middleware ordering; do not move availability checks into the Rust leaf without a contract review.
三方复核结论: Go middleware/reference, fixture header matrix, and Rust focused replay agree for the leaf-visible split; full product wiring remains pending.

quirk: A client disconnect can make the initial SSE response body empty after the retry write fails, while the detached Go execution continues, reaches a terminal event, and remains replayable by stream ID.
范围: `/api/v1/adk/chat/stream` / failed writer, background execution, reconnect replay
证据: `runStage9ClientDisconnect`, `stage9FailingSSEWriter`, fixture `observation.replay`, and Go transport disconnect tests
分类: go-behavior
判定: unresolved
处置: Preserve the quirk and do not cancel the Go-owned background run. The Rust leaf test verifies terminal snapshot semantics but cannot emulate an HTTP writer failure without the integration transport adapter.
风险: blocking for qualification
owner: Go transport until integration cutover
后续: Integration must add a test-cutover HTTP disconnect/reconnect rehearsal and prove replay retention, terminal close, cancellation, and recovery before qualification.
三方复核结论: Go reference and fixture agree; Rust leaf replay covers the available snapshot boundary; end-to-end transport evidence is outstanding.

quirk: Authentication and CSRF errors are produced by shared middleware before the ADK handler and are included in the Go corpus, but this worker's leaf has no access-policy or CSRF state and does not replay those two cases.
范围: `/api/v1/adk/chat` / `401 WEB_AUTH_REQUIRED`, `403 CSRF_FAILED`
证据: Go `middleware.Auth`, fixture cases `chat-auth-required` and `chat-csrf-forbidden`, explicit skip count in `stage9_adk_chat_stream.rs`
分类: boundary
判定: unresolved
处置: Keep the cases in the frozen corpus; leave enforcement to the shared integration router and do not duplicate auth state in Rust.
风险: blocking for qualification
owner: integration shared transport
后续: Inject the ADK port only behind the existing authenticated loopback/test-cutover assembly and run the two cases through the product differential.
三方复核结论: Go middleware and fixture are frozen; Rust confirms the boundary cases are intentionally outside the leaf, so product-level evidence is required.

## Integration Checklist

- Add the private module/adapter and route dispatch only in the integration branch; do not alter this worker commit's leaf contract by default registration.
- Inject `AdkChatStreamPort` only in explicit test-cutover; keep Go Assistant runtime, Provider/session lifecycle, SQLite, background execution, reconnect hub, and all writes as the sole production owner.
- Add the two operations to the integration-owned route catalog/ownership ledger and run the product differential with authenticated desktop/browser cases.
- Resolve the disconnect/reconnect qualification blocker, then complete four-platform release, signing, security, recovery, hard-cut, and only then Go/Wails removal gates.

## Verification

Focused commands for this worker:

```text
go test scripts/rust-migration/stage9_adk_chat_stream_reference_test.go -run '^TestStage9ADKChatStreamFixtureMatchesCurrentGoOwner$' -count=1
cargo test -p jftrade-engine --test stage9_adk_chat_stream -- --nocapture
node scripts/rust-migration/check-stage9-adk-chat-stream.mjs
git diff --check
```

`check:quick`, `check:rust`, the unified Stage 9 product differential, ownership changes, product assembly wiring, and release/hard-cut gates are intentionally not claimed by this worker. The group remains `cutover-test-only`, not cutover-qualified.
