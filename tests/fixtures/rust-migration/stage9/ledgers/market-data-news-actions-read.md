# Market-Data News Actions Read Group Ledger

- Group: `market-data-news-actions-read`
- Tier: B. Both reads are owned by the active market-data Provider and inherit
  provider lifecycle, capability, fallback, warming, and bounded-worker busy
  behavior.
- Operations: 2.
- Production owner: Go. Rust is limited to a consumer-owned snapshot port in
  explicit `cutover-test-only` wiring; it must not activate a Provider, OpenD,
  sidecar, subscription, cache, or write path.
- Fixture: `tests/fixtures/rust-migration/stage9/market-data-news-actions-read.json`
- Go reference: `scripts/rust-migration/stage9_market_data_news_actions_read_reference_test.go`

## Initial Unresolved Investigation Log

### Q1: Omitted news limit and explicit zero have different route semantics

quirk: The service accepts `limit == 0` as its internal default, while an HTTP
request that explicitly supplies `limit=0` may be rejected before the service
is called.
scope: `market-data-news-actions-read` / `GET /api/v1/market-data/news/{market}/{symbol}`
evidence: `internal/api/marketdata/routes.go`, `internal/marketdata/news.go`,
and `internal/api/marketdata/routes_news_actions_test.go`.
classification: unknown
judgment: unresolved
disposition: capture both route cases in the Go fixture before selecting a Rust
projection.
risk: medium
owner: Go/Rust worker
follow-up: resolve after the Go fixture and isolated Rust design are compared.

### Q2: CN aggregate input must preserve Go's provider-boundary normalization

quirk: `CN` is a UI aggregate, but a qualified `CN/SH.600519` or
`CN/SZ.000001` read may reach the provider as an exchange leaf. A raw-snapshot
port must not independently rewrite the public request path.
scope: both operations
evidence: `internal/marketdata/news.go`, `internal/marketdata/service.go`, and
`internal/marketdata/news_facade_test.go`.
classification: unknown
judgment: unresolved
disposition: freeze raw request paths and the observed provider call/output.
risk: high
owner: Go/Rust worker
follow-up: resolve after Go fixture capture and port request design.

### Q3: Nullable entries and action numeric fields must survive unchanged

quirk: News entry text/timestamps and corporate-action `amount`/`ratio` are
nullable; `entries` and `events` may also be empty or null when a Provider
returns a nil slice.
scope: both operations
evidence: `internal/marketdata/news.go` and provider conversion tests.
classification: unknown
judgment: unresolved
disposition: capture populated, empty, and null model cases; keep the Rust port
at `serde_json::Value` rather than adding a partial DTO.
risk: high
owner: Go/Rust worker
follow-up: resolve after fixture replay design is complete.

### Q4: Provider capability, fallback, warming, and busy errors have distinct wire behavior

quirk: Capability errors are `409`; generic provider failures are `502`; warming
and busy are `503` with `Retry-After: 1` and `Retry-After: 2` respectively.
scope: both operations
evidence: `internal/api/marketdata/routes.go`,
`internal/api/marketdata/routes_boundaries_test.go`, and Provider client tests.
classification: unknown
judgment: unresolved
disposition: capture all four branches in the Go fixture, including retry
headers, before choosing the snapshot error shape.
risk: high
owner: Go/Rust worker
follow-up: resolve after the Go fixture and Rust transport design are compared.

### Q5: Current shared Rust transport cannot carry `Retry-After` on `ApiFailure`

quirk: `jftrade-api::ApiFailure` has only status/code/message and the router
only emits fixed transport headers. Neither `ApiFailure` nor `ApiOutput::Raw`
can carry a route-specific `Retry-After` value.
scope: both warming and busy branches
evidence: `crates/jftrade-api/src/envelope.rs`,
`crates/jftrade-api/src/ports.rs`, and `crates/jftrade-api/src/router.rs`.
classification: harness
judgment: unresolved
disposition: retain retry metadata in the dedicated Rust snapshot result and
require an integration-owned shared transport extension before route wiring.
risk: high
owner: integration branch
follow-up: must resolve before this group can be registered or differentially
validated as `cutover-test-only`.

## Three-Way Conclusions After Go Fixture and Rust Design

### Q1: Omitted news limit and explicit zero have different route semantics

- Go owner: the reference harness captures an omitted `limit` as a provider
  call with `10`, while an explicit `limit=0` is rejected with `400 BAD_REQUEST`
  before the provider is called. It also captures malformed and over-limit
  query values as route-level `400` responses.
- Fixture: `market-data-news-actions-read.json` preserves those request paths,
  statuses, messages, and null provider calls in four news validation cases.
- Rust design: the snapshot port receives the raw path and query and does not
  apply a default, validate the limit, or invoke a provider. The Go snapshot
  therefore remains the only source of this distinction.
- 三方复核结论: Go behavior, frozen fixture evidence, and the Rust raw-snapshot
  design agree. This quirk is resolved as intended and must not be normalized
  in Rust.

### Q2: CN aggregate input must preserve Go's provider-boundary normalization

- Go owner: `CN/SH.600519` reaches the news provider as `SH/600519`, and
  `CN/SZ.000001` reaches the corporate-actions provider as `SZ/000001`; the
  response identity is the normalized leaf market and symbol.
- Fixture: both cases store the public request path, normalized `providerCall`,
  and normalized response data. `providerCall` is explicitly test evidence,
  not a public wire field.
- Rust design: the port is keyed by the raw path and raw query and returns the
  complete captured JSON value. It does not parse or rewrite CN aggregate
  instruments.
- 三方复核结论: Go owns normalization, the fixture proves its observed
  boundary result, and Rust preserves that result without duplicating the
  normalization rule.

### Q3: Nullable entries and action numeric fields must survive unchanged

- Go owner: provider-neutral news text/timestamp fields and corporate-action
  `amount`/`ratio` are nullable; a nil provider slice serializes as `null`,
  while an allocated empty slice serializes as `[]`.
- Fixture: populated nullable fields, empty arrays, and null arrays/events are
  captured for both operations, including nullable action numbers.
- Rust design: the snapshot port carries `serde_json::Value` rather than a
  partial DTO, so null versus empty and all nullable fields remain unchanged.
- 三方复核结论: the Go model, generated JSON fixture, and Rust value-preserving
  projection agree; no Rust-side DTO or null coercion is allowed.

### Q4: Provider capability, fallback, warming, and busy errors have distinct wire behavior

- Go owner: unsupported capabilities map to `409`, generic provider failures to
  route-specific `502` codes, provider warming to `503` with `Retry-After: 1`,
  and provider busy to `503` with `Retry-After: 2`. Provider-generation changes
  map to `409 MARKET_DATA_PROVIDER_CHANGED`.
- Fixture: both routes contain capability, fallback, warming, busy, and
  provider-change cases with exact statuses, codes, messages, and retry
  headers.
- Rust design: snapshot failures retain status, code, message, and optional
  retry seconds. The API mapper applies `with_retry_after(1|2)` only when the
  captured failure contains that metadata; it does not recreate provider or
  lifecycle behavior.
- 三方复核结论: the Go mapping and fixture are fully represented by the Rust
  error shape and mapper. The route remains test-cutover-only and provider
  lifecycle ownership remains in Go.

### Q5: Current shared Rust transport cannot carry `Retry-After` on `ApiFailure`

- Go owner: warming and busy responses emit the required `Retry-After` values
  on the HTTP error response.
- Fixture: all four retry cases record the exact header value and omit it from
  non-retry branches.
- Rust design: the dedicated error variant preserves `retry_after_seconds`,
  and the API mapper calls the integration-provided
  `ApiFailure::with_retry_after(u64)` builder. No shared transport file is
  modified by this worker.
- 三方复核结论: the Go wire, fixture, and Rust mapper contract agree on the
  metadata path. Integration added the type-constrained
  `ApiFailure::with_retry_after(u64)` extension and the shared transport
  regression confirms that `Retry-After` is emitted only when metadata is
  present. The group is eligible for `cutover-test-only` registration; Go
  remains the production owner.

quirk: The original shared Rust transport could not carry route-specific
`Retry-After` metadata through `ApiFailure`.
范围: `market-data-news-actions-read` / warming and busy error responses
证据: Go reference fixture, Rust group replay, and
`cargo test -p jftrade-api --test transport_contracts` all pass with exact
`Retry-After: 1|2` values and no header on ordinary errors.
分类: harness
判定: deviated
处置: Extend the shared failure envelope with optional numeric retry metadata;
preserve the route's existing status/code/message and emit the header only
when the worker-owned snapshot error supplies it.
风险: low
owner: integration
后续: Retain the regression and recheck this wire during future B-tier
cutover qualification.
