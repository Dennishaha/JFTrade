# 口令哈希域对齐批次（第 108 批）

本文件记录 Go 基线 `internal/security/passwordhash`（`passwordhash.go`、`passwordhash_test.go`）
的 3 条 `missing` 测试迁移到 Rust 的逐条核对结论，涉及
`crates/jftrade-settings/src/password_hash.rs`（`hash_argon2id`/`verify_argon2id` 与本次新增的
参数边界判定）、其调用方 `crates/jftrade-settings/src/security.rs`（Web 访问口令
`verify_web_access_password`）与 `crates/jftrade-settings/src/mcp_server.rs`（MCP 令牌 verifier），
以及 `crates/jftrade-store-settings-file`（口令 verifier 持久化）与 `crates/jftrade-engine`
的认证 wire。

## 第一百零八批：internal/security/passwordhash 收口（3 条）

### 范围与分片

Go 侧 2 个文件、3 条 `missing`：

- P0 `passwordhash_test.go:9`：`TestHashAndVerify`（PHC 产出、Valid、正确/错误口令）。
- P0 `passwordhash_test.go:25`：`TestVerifyRejectsUnsafeParametersBeforeHashing`（超限参数必须在调用 Argon2 之前拒绝）。
- P1 `passwordhash_test.go:32`：`TestValidRejectsMalformedHashes`（12 个畸形 verifier）。

分类：3 条 `[x]`/function_exact；`internal/security/passwordhash` 域内 `missing` 归零。

### 关键事实（本批 recon 与实测）

- **Go 基线形状**：`$argon2id$v=19$m=65536,t=3,p=1$<salt>$<key>`，`RawStdEncoding`（标准字母表、无填充），盐 16 字节、密钥 32 字节；`decode` 先 `TrimSpace` 再按 `$` 切 6 段，校验 `v=19`、`m∈[19*1024,128*1024]`、`t∈[2,5]`、`p∈[1,4]`、盐与密钥解码长度 `∈[16,64]`；`Verify` 用 `subtle.ConstantTimeCompare`。
- **Rust owner 与形状一致**：`hash_argon2id` 用 `Params::new(65_536, 3, 1, Some(32))` 与 `SaltString::encode_b64`；`password_hash::Encoding::B64` 是“标准字母表 + 无填充”，与 Go 的 `RawStdEncoding` 同形（读源码确认，非推测）。`verify_password` 使用 verifier 自带的 m/t/p 参数（`T::Params::try_from(hash)` 后以新实例哈希），因此接受区间放宽不会改变计算语义。
- **本批修复的真实功能缺口**：原 `verify_argon2id` 要求 `m==65536 && t==3 && p==1 && key==32`，比 Go 的有界区间更窄——Go 版本写出的合规 verifier（例如 `m=19456,t=2`）迁移后会被拒绝，表现为 Web 访问口令突然失效。现改为 Go 的有界区间（`verifier_within_supported_bounds`），写入仍固定 canonical 参数；越界者仍在 Argon2 之前失败关闭。
- **Valid 的等价承载**：Rust 无独立 `Valid` API（新增仅供测试使用的函数会触发 `dead_code`，故不引入），改以两组可观察行为钉住：正向是“正确口令 true、错误口令 false”＋产出密文的结构断言；反向是 12 个畸形样本一律 false。

### 逐条结论

- `passwordhash_test.go:9` → `crates/jftrade-settings/src/password_hash.rs::tests::produced_verifier_hides_the_password_and_only_accepts_the_matching_secret`：产出密文不含明文；解析后 algorithm=argon2id、v=19、m=65536/t=3/p=1、盐 16 字节、密钥 32 字节；正确口令 true、错误口令 false。
- `passwordhash_test.go:25` → `::tests::parameters_outside_go_bounds_are_rejected_before_argon2_runs` ＋ `::tests::go_bounded_parameters_keep_verifying_after_migration`：Go 的 1 GiB 样本必须在 1 秒内（实测微秒级）返回 false，证明未触发昂贵计算；同时 m/t/p/盐/密钥的越界边界（19455、131073、t=6、p=5）一律拒绝，而 Go 区间内的最小参数（m=19456,t=2）必须继续可验证。
- `passwordhash_test.go:32` → `::tests::malformed_verifiers_are_rejected_without_a_verification_result`：字面照搬 Go 的 12 个样本，逐条断言 `verify_argon2id` 为 false。

### 探针记录（破 → 红 → 按字节回滚）

- 把 `hash_argon2id` 的输出长度改成 16：正向测试转红（`assert_eq!(…len(), 32)` 报 16≠32）。
- 把内存上限放宽到 `1_048_576`：1 GiB 样本测试转红——Argon2 真的运行了 18.63s，<1s 的“未触发昂贵计算”上界断言失败。
- 把解析失败分支改成 `return true`：畸形样本测试转红（`accepted malformed verifier ""`）。
- 把区间收窄回精确相等（m=65536..=65536，t=3，p=1）：迁移测试转红（`m=19456` 被拒）。

四次探针回滚后文件与探测前逐字节一致（回滚态 shasum `cae41d40a5abf89db54730f8245f4b4e4f33276d59c3d48b4b945978265a1ae2`）；
随后仅按 `check-zero-go` 闸门改写一行文档注释（原文含被禁的 `<go> build` 字样），最终态 shasum
`1e0e89fe6384b79db490877e03b865ad9a47e532ac72221b6208c8b686a378b8`，行为与探针时完全一致。

### 保留差异

- Go 的 `Verify` 返回 `(bool, error)` 并以 `ErrInvalidHash` 区分“格式非法”与“口令不匹配”；Rust 收敛为 `bool` 的 fail-closed 语义，错误通道由调用方的 API 错误码承担。
- Go 有独立 `Valid` API；Rust 以“正向可验证 + 反向一律拒绝”的可观察行为等价承载，未新增仅供测试的函数（避免 `dead_code`）。
- Rust 接受 Go 的整个有界区间，写入仍固定 canonical 参数；区间上限（128 MiB×5 次）与 Go 相同，会消耗可观 CPU/内存，但这是被显式校验过的边界，而非无界请求。

### 后续待办

- 下一批按域余量：`cmd/jftrade-api` 2 → `cmd/check-go-coverage` 2 → `pkg/besteffort` 2 → `internal/app/apiserver/servercoretest` 1 → `internal/app/apiserver/webaccess` 1 → `cmd/jftrade-desktop` 1 → `pkg/chart` 1 → `scripts/archive_frontend_assets_test.go` 1 → 直至 4451 条全部完成。
- 既有独立项：修复审计脚本生成 markdown 时的尖括号占位符（`<go 文件>:<行>`），使 `build:docs:generated` 不再被 markdown-it 判为未闭合 HTML 标签。

验证：
- `cargo fmt --all -- --check` EXIT=0；`cargo clippy -p jftrade-settings
  -p jftrade-store-settings-file -p jftrade-engine -p jftrade-api --all-targets --locked` EXIT=0。
- `node scripts/quality/cargo-nextest.mjs run -p jftrade-settings -p jftrade-store-settings-file
  -p jftrade-engine -p jftrade-api --all-targets --locked --no-fail-fast`：1893 passed /
  0 skipped / 0 failed（含本批新增 4 条；聚焦运行 4/4 passed）。
- 四次探针各自转红并已按字节回滚（输出长度 32→16、内存上限放宽到 1 GiB、解析失败分支返回
  true、区间收窄为精确相等），详见上文探针记录。
- `python3 scripts/compatibility/audit_test_parity.py`：4451 Go / 3050 Rust、`[x]` 1318（+3）、
  `missing` 14 → 11、0 破坏引用、0 重复 rust_entry、0 条 `[x]` 缺少 function_exact；
  未锚定 200（本批 3 条均写锚点，未增加）、partial 无解析引用 7（既有基线）、无断言 2（既有基线）。
- `python3.12 scripts/compatibility/parity_anchor_reconcile.py`：1321 唯一引用（已记账 1266，+3；
  unrecorded 0、unknown 55、stale 0）。
- `pnpm run check:rust:architecture` EXIT=0；`pnpm run check:compatibility` EXIT=0；
  `node scripts/check-zero-go.mjs` EXIT=0（2915 tracked files，含本批新文档；首跑曾因注释里出现
  被禁的 `<go> build` 字样失败，改写注释后通过）；`pnpm run check:ai-context` EXIT=0；
  `git diff --check` 干净。
- `pnpm run check:quick` EXIT=0（22 个 pnpm 阶段，含 target-health、fmt、clippy、契约/路由/
  OpenAPI 边界、兼容回放与 nextest）。
- `pnpm run check:rust` EXIT=1：唯一失败阶段 `check:rust:policy`（`cargo deny`），报
  RUSTSEC-2026-0285 漏洞与陈旧 advisory 告警，与本仓既有基线同源（第 106 批已在干净 HEAD 用
  `git stash` 复现同一失败）。**不记为通过**；该 run 停在静态阶段，等价测试面由上面的 nextest
  结果覆盖。
