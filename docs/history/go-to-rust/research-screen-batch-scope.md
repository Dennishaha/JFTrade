# Research Screen 领域对齐批次

本文件记录 Go 基线 `pkg/researchscreen`（股票筛选目录、参数可用性、definition V2
归一化与字段错误契约）迁移到 Rust `jftrade-research`（catalog fixture 投影、
definition 归一化与参数校验）及 `jftrade-engine`（research screen 读写端口）的
逐测试核对结论。

## 第九十四批：`pkg/researchscreen` 全域收口（24 条）

### 范围与分片

`pkg/researchscreen/**` 剩余 **24 条 `missing`**（5 个文件），分三片：
P0 definition 归一化与边界 12 条（`definition_edges_test.go` 6、`definition_test.go` 6）→
P1 catalog 内嵌与核心 9 条（`catalog_embedded_test.go` 5、`catalog_test.go` 4）→
P2 catalog 边界 3 条（`catalog_edges_test.go`）。

### 结果

- 24 条全部给出结论：**7 条 `[x]`/`function_exact` + 16 条 `partial` + 1 条 `boundary`**。
- `pkg/researchscreen/**` 归零：24 = 7 `[x]` + 16 `partial` + 1 `boundary`，0 `missing`。
- 全局：4451 = function_exact **1241** + partial **2507** + boundary **542** + module_only 4 +
  missing **157**（前批为 1234 / 2491 / 541 / 4 / 181），Rust 测试 2944 → **2948**。

### 本批新增与补强测试

本批未发现真实功能缺口：既有 `research-screen-catalogs.json` /
`research-definition-normalization.json` 冻结投影与 Go 语义逐项一致，因此没有生产修复，
只把等价行为固化成显式断言并做了一次 guard 探针。

- `crates/jftrade-research/src/definition/parameters.rs`（新增 `mod tests`）：
  `parameter_validation_rejects_type_range_step_and_enum_errors`（错误码矩阵 8 例 + 2 个通过例）、
  `union_validation_covers_every_provider_member_shape`（4 个通过形状 + 7 个失败形状，含未支持名称）。
- `crates/jftrade-research/tests/catalog_parameter_availability_boundaries.rs`：
  新增 `futu_factor_display_semantics_match_the_editor_table`（10 行 display 语义表）、
  `catalog_condition_and_parameter_editor_variants_match_the_frozen_projection`
  （condition 6 类 + 非 filter 空编辑器 + 参数编辑器 5 类 + 帮助文本/枚举帮助）；
  补强 `futu_parameters_preserve_editor_bounds_defaults_and_enum_metadata`
  （因子级 roles/help/searchKeywords、全目录 filter 因子的 conditionEditor/operators/valueEnum）
  与 `embedded_catalog_keeps_provider_intersection_roles_and_units`
  （embedded header、9 因子、akshare/yfinance markets、simple.price 角色/算子、
  大小写不敏感查找、indicator.ma 缺席）；测试助手 `factor()` 改为大小写不敏感以对齐 Go
  `Lookup`/`LookupEmbedded`。

探针（改坏 → 转红 → 按字节回滚）：把 `validate_parameter_value` 的 step 分支短路
（`.filter(|_| false)`）后，`parameter_validation_rejects_type_range_step_and_enum_errors`
转红；`shasum -a 256` 校验回滚一致（`d4fa7e4e…`）。

### 新增 `[x]` 锚点

| Go 测试 | Rust 入口 | 结论 |
| --- | --- | --- |
| `catalog_test.go:9 TestCatalogIsCompleteStableAndDoesNotExposeProviderEnums` | `catalog_parameter_availability_boundaries.rs::futu_catalog_preserves_stable_shape_and_public_projection` | `[x]`：header、402 因子 / 11 分类、period/term 枚举 10/14、10 个代表因子、无 providerId 泄漏。 |
| `catalog_test.go:45 TestCatalogParametersExposeEditorContract` | `...::futu_parameters_preserve_editor_bounds_defaults_and_enum_metadata` | `[x]`：参数 editor/default/step/minimum + 因子 roles/help/searchKeywords + filter 因子 conditionEditor/operators/valueEnum。 |
| `catalog_test.go:97 TestFactorDisplaySemanticsAreExplicitAndCorrected` | `...::futu_factor_display_semantics_match_the_editor_table`（本批新增） | `[x]`：10 行 unit/currencyBasis/displayFormat 表。 |
| `catalog_embedded_test.go:31 TestEmbeddedCatalogShapeAndSemantics` | `...::embedded_catalog_keeps_provider_intersection_roles_and_units` | `[x]`：embedded header、9 因子、market 列表、角色/单位/格式、大小写不敏感查找、生成期因子缺席。 |
| `definition_edges_test.go:56 TestParameterValidationRejectsTypeRangeStepAndEnumErrors` | `parameters.rs::parameter_validation_rejects_type_range_step_and_enum_errors`（本批新增） | `[x]`：8 类错误码 + 2 个通过例（NaN 例在 Rust 不可达，已记录）。 |
| `definition_edges_test.go:94 TestUnionValidationCoversEveryProviderShape` | `parameters.rs::union_validation_covers_every_provider_member_shape`（本批新增） | `[x]`：4 个通过形状 + 7 个失败形状（含未支持名称分支）。 |
| `definition_edges_test.go:249 TestDefinitionNormalizationRejectsPoolSortAndIdentityErrors` | `definition_normalization_contracts.rs::normalization_and_field_errors_match_the_go_owner_corpus` | `[x]`：9 个冻结语料 case 覆盖 8 类拒绝，逐例断言 path+code+message 全等。 |

### 保留差异候选（保持 `partial`/`boundary` 的理由）

- **P0 definition 归一化同源证据**：`TestNormalizeDefinitionPreservesParameterizedInstances`、
  `RequiresExplicitV2Versions`、`RejectsDuplicateConfigurationWithFieldPath`、
  `RejectsMarketIncompatibleFactor`、`ValidatesParameterTypesEnumsAndUnions`、
  `RejectsWrongOperatorForFactorKind`、`TestConditionValueValidationCoversSetRangePositionAndPattern`、
  `TestDefinitionNormalizationCoversPoolsSortsAndSecondFactors` 的行为都由同一冻结语料测试逐例覆盖，
  但按「`[x]` 的 rust_entry 全局唯一」只能把该 Rust 测试记在
  `definition_edges_test.go:249` 一行，其余保持 `partial` 并写明覆盖证据；
  另：实例 id 稳定哈希的**字面形状**与 Go 不同（`factor_<16hex>` vs `{prefix}-{key}-<10hex>`）。
- **P0 helper 契约**：`TestDefinitionHelperContractsCoverSupportedValueShapes` 直接断言
  `(*FieldError)(nil).Error()`、`cleanIDs`、`numericSlice`、`numericJSONValue` 等 Go 语言层 helper；
  Rust 无这些导出入口（算子推断由语料 `inferred-*` case 覆盖，数值形状由 serde_json 类型系统承担）。
- **P1 embedded 使用标记**：`TestEmbeddedCatalogValidationUseFlags` 的 `ValidateEmbeddedFactorUse`、
  `TestNormalizeDefinitionV2AcceptsEmbeddedCatalog` 的小写 market 接受、
  `TestNormalizeDefinitionV2EmbeddedRejectsOutOfCatalogFactors` 的 column 分支、
  `TestNormalizeDefinitionV2FutuContractUnchanged` 的成功分支均由语料/角色投影覆盖但无逐分支断言。
- **P1 因子用途校验**：`TestValidateFactorUseRejectsUnsupportedPurposes` 的
  `ValidateFactorUse`/`ValidateFactorForMarket` 在 Rust 无对应导出函数，用途约束落在归一化角色
  校验与目录可用性投影。
- **P2 catalog helper 变体**：`TestCatalogHelperContractsCoverEditorVariants` 的合成行
  （Filter=false→空编辑器、FilterKind=unknown→range 回退、自定义参数名）在冻结目录里不可达；
  `TestCatalogParameterBoundsAndAvailability` 的 `int64PointerAtLeast/AtMost` 是生成期 clamp，
  运行时可观察的只是钳制后的 bounds。
- **P2 boundary（1 条）**：`TestValidateCatalogRejectsIncompleteSemanticRows` 依赖替换包级
  `generatedFactors`/`generatedEnums` 后调用 `ValidateCatalog()`；Rust 的目录是冻结 fixture
  （只校验版本与 13 个变体数量），没有运行时目录校验入口与可变目录表，该拒绝矩阵不可达。

### 后续待办

- research screen 参数校验的 `nan`/非有限输入在 Rust JSON 入口下不可达；若未来引入
  非 JSON 参数通道（worker/内部调用），需要补 fail-closed 断言。
- 语料测试的单点引用约束会持续把同源 Go 测试压成 `partial`；后续若要提升清单精度，
  可按语义拆分语料为多条独立 Rust 测试（每条一个行为族）。
- `internal/settings`（24）为下一批，其后 `cmd/generate-futu-proto`（18）等按域余量推进。

验证：`cargo fmt --all -- --check`；`cargo clippy -p jftrade-research -p jftrade-engine -p jftrade-integration-futu --all-targets --locked`；
`node scripts/quality/cargo-nextest.mjs run -p jftrade-research -p jftrade-engine -p jftrade-integration-futu -p jftrade-broker --all-targets --locked --no-fail-fast`（**2281 passed / 1 skipped**）；
`python3 scripts/compatibility/audit_test_parity.py`（4451 Go / **2948 Rust** / **1241 `[x]`**；missing 157、partial 2507、boundary 542、module_only 4；`OK: 1241 function_exact mappings cite existing workspace tests`、0 破坏引用、0 重复 rust_entry、0 条 `[x]` 缺少 function_exact；未锚定 196、7 条 partial 无解析引用、2 条无断言为前批基线告警）；
`python3.12 scripts/compatibility/parity_anchor_reconcile.py`（anchors 1240；already recorded 1145、unrecorded 6 = `internal/settings` 基线、unknown go line 55、stale 34）；
`pnpm run check:rust:architecture`；`pnpm run check:compatibility`（EXIT=0，六组 replay 全通过）；
`node scripts/check-zero-go.mjs`（2895 tracked files / 0 release artifact）；`pnpm run check:ai-context`（EXIT=0）；`git diff --check`。

`pnpm run check:quick` **EXIT=0**（受影响计划 **1758 passed / 0 skipped**；`check:rust:target-health` 本次通过）。

`pnpm run check:rust` **未通过（EXIT=1）**，唯一失败项不在本批 diff 范围内，保留原始证据不记为通过：
`check:rust:policy` 的 `cargo deny check` advisories 失败于 **RUSTSEC-2026-0285**（rustls 0.23.44，修复 >=0.23.45；
根 `Cargo.toml` 精确锁定 `=0.23.44`），另有 8 条 `warning[advisory-not-detected]`（陈旧 ignore）。
同一次运行中 target-health、architecture、production-policy、`cargo fmt --check` 与 clippy 均通过；本批未改 `Cargo.lock` / `deny.toml`。
