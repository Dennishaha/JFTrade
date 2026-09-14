# SQLite、Storage、Settings File 领域对齐批次

本批逐项核对 5 条涉及 rollback、migration、唯一写入和设置持久化的 Go 测试：
规范化 fallback、atomic replace 失败回滚全部 runtime state、bootstrap/migration
失败回滚、managed account backing array 回滚，以及 Futu integration 持久化。

| Go 测试 | Rust 证据 | 结论 |
| --- | --- | --- |
| `internal/store/settingsfile/normalization_and_persistence_test.go:13` | `settings_file_contracts::test_settings_normalization_handles_fallbacks_and_boundaries` | `[~]`/`partial`：仅 execution fallback/clamp，缺 notification、Pine worker、颜色和 mode 边界 |
| `internal/store/settingsfile/rollback_test.go:13` | `settings_file_contracts::test_failed_setting_saves_rollback_all_runtime_state` | `[~]`/`partial`：Rust 仅验证 appearance 成功保存，缺 10 类设置 atomic replace 失败后的内存/磁盘回滚 |
| `internal/store/settingsfile/rollback_test.go:185` | `settings_file_contracts::test_failed_bootstrap_and_migration_rollback_runtime_state` | `[~]`/`partial`：缺 initial integration、bootstrap/migration replace 失败及文件不存在断言 |
| `internal/store/settingsfile/rollback_test.go:232` | `settings_file_contracts::test_failed_managed_account_crud_rolls_back_backing_array` | `[~]`/`partial`：仅初始空数组，缺 create/update/delete 失败注入和 backing array 不变 |
| `internal/store/settingsfile/store_test.go:83` | `settings_file_contracts::product_corpus_replays_frozen_compatibility_and_preserves_unknown_fields` | `[~]`/`partial`：frozen corpus 覆盖配置字段，缺 SaveIntegration runtime env 不变与 replace 流程 |

验证命令已写入 `manual-test-mappings.json`。本批不把 helper 或 corpus 级证据
升级为完整等价；下一步优先补 SettingsFileStore writer lease 冲突、原子替换
失败和 SQLite migration rollback 的独立 Rust 回归测试。
