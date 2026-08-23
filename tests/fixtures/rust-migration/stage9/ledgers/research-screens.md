# Research screens POST

状态：worker C → B → A rehearsal 已完成；当前 production owner 仍为 Go。共享
`route-ownership.json`、product assembly、ProductOptionalPorts、ProductApi wire
和 capability wiring 留给集成分支应用。

## Contract

| Method | Path | Boundary | Wire notes |
| --- | --- | --- | --- |
| POST | `/api/v1/research/screens` | strict JSON → V2 normalizer/page defaults → `Service.QueryScreen` → `MarketResearchReader` → typed result projection | 200/400/409/429/502/503; JSON content type; `Retry-After` on rate-limit/provider warm/busy |

The Go handler accepts exactly one JSON value with unknown fields rejected. JSON
`null` reaches V2 validation and reports `querySchemaVersion: must be 2`; an empty
body, trailing value, unknown field, or type error reports
`invalid stock-screen request`. Page limit zero defaults to 50, offset must be
non-negative, and the maximum limit is 100. The handler adds the request
catalog version and definition-derived result columns after the service returns.

## C / B / A evidence

- C fixture/reference: `scripts/rust-migration/stage9_research_screens_reference_test.go`
  and `tests/fixtures/rust-migration/stage9/research-screens.json`.
- B leaf/port: `crates/jftrade-engine/src/product_research_screen_write_port.rs`.
  It has no provider, SQLite, OpenD, network, or durable-state dependency and
  returns an unavailable response when no explicit test port is supplied.
- A rehearsal/replay: `crates/jftrade-engine/tests/stage9_research_screens.rs`
  and `scripts/rust-migration/check-stage9-research-screens.mjs`.
- Corpus: 22 cases / 27 requests, including normal and empty results, null,
  empty, trailing, unknown and wrong-type JSON, page and definition errors,
  query-string handling, rate-limit/provider retry headers, capability and
  broker errors, invalid provider projections, failure recovery, repeated
  requests, and distinct-page concurrency.

## Quirks

quirk: The first fixture broker draft derived `nextOffset` from concurrent call order, which made the golden unstable; it is now derived from the request page offset.
范围: research / POST `/api/v1/research/screens` fixture harness
证据: differential rerun before and after `stage9_research_screens_reference_test.go` change; two consecutive Go fixture checks and the Rust replay now produce the same concurrent corpus.
分类: harness
判定: deviated
处置: 修复 fixture/harness；no Go behavior was changed and the corrected corpus is the reference.
风险: low
owner: Rust worker
后续: preserve request-derived continuation values in future fixture extensions。

quirk: Go caches an identical successful screen query, so two sequential identical requests produce one broker call; Rust leaf replay forwards both explicit test-port calls while preserving the same HTTP envelope.
范围: research / POST `/api/v1/research/screens`
证据: Go `expectedObservation.callCount=1` in `research-screens.json` case `repeated-identical-request`; Rust replay asserts two equal query shapes and identical responses in `stage9_research_screens.rs`.
分类: go-behavior
判定: intended
处置: 复刻 public wire；cache owner and cache-hit evidence remain with the Go service until composition-root integration defines the Rust adapter boundary。
风险: medium
owner: 集成分支
后续: cutover qualification 前补 cache ownership/eviction evidence；不属于 durable lease/recovery 双写 blocker。

quirk: Go `null` is accepted by `encoding/json` as a zero query and fails later on schema validation rather than using the generic bind error.
范围: research / POST `/api/v1/research/screens`
证据: Go fixture case `null-json-body`; Rust replay and direct decoder tests return 400 `BAD_REQUEST` with `querySchemaVersion: must be 2` and no port call.
分类: go-behavior
判定: intended
处置: Rust leaf 复刻；待 Go 删除前保留在 compatibility corpus。
风险: low
owner: Rust worker
后续: hard-cut 前持续由三方 differential 验证。

quirk: Go strict binding collapses trailing JSON, unknown fields, and wrong wire types to the same 400 `invalid stock-screen request` message.
范围: research / POST `/api/v1/research/screens`
证据: Go fixture cases `trailing-json-value`, `unknown-json-field`, `wrong-json-field-type`; Rust replay cases and direct decoder test match status/code/message and zero port calls.
分类: go-behavior
判定: intended
处置: Rust leaf 复刻；不在迁移切片内合理化或修复 Go 行为。
风险: low
owner: Rust worker
后续: hard-cut 前持续由 strict decoder differential 验证。

quirk: Provider rate-limit, warming, busy, capability, and generic failures use distinct status/code/message/header mappings; invalid provider rows and invalid continuation offsets collapse to 502 `BROKER_FEATURE_FAILED`.
范围: research / POST `/api/v1/research/screens`
证据: Go fixture error cases and headers; Rust `port_error_response` plus invalid-row leaf test; no durable lease/recovery state is touched.
分类: go-behavior
判定: intended
处置: 复刻；release/cutover evidence must retain retry and failure mapping.
风险: medium
owner: 集成分支
后续: cutover-qualified 前补 adapter error translation evidence；不阻断 durable lease/recovery because this route owns no durable lease。
