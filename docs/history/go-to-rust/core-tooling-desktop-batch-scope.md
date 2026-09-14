# Core、Tooling、桌面运行时对齐批次

本批范围限定为 `cmd/jftrade-desktop`、`internal/datamanagement`、
`internal/buildinfo` 的 Go 测试，以及 Rust API static asset owner 的桌面路由行为。
Go/Wails 与 Rust/Tauri 的进程、窗口和插件装配不同；只有真实 Rust 函数级断言才记为
部分覆盖，不能把桌面 replay 或脚本检查写成 Wails 行为等价。

## 已核对条目

| 状态 | Go 文件:行号:测试名 | Rust 证据 | 风险/差异结论 | 验证 |
| --- | --- | --- | --- | --- |
| [~] | `cmd/jftrade-desktop/main_test.go:131:TestDesktopAssetHandlerDoesNotFallbackForMissingStaticAsset` | `jftrade-api::transport_contracts::desktop_missing_static_assets_do_not_use_spa_fallback` | 发现并修复 Rust static_response 对 `/assets/`、`/docs`、带扩展名缺失路径错误回退 index 的差异；仍与 Wails handler 装配不同。 | `node scripts/quality/cargo-nextest.mjs run -p jftrade-api --test transport_contracts --all-targets --locked` |
| [~] | `cmd/jftrade-desktop/main_test.go:195:TestDesktopAssetHandlerFallsBackForUnknownClientRoute` | `jftrade-api::transport_contracts::desktop_unknown_client_routes_use_spa_fallback_without_file_paths` | 无扩展名未知 client route 保持 SPA fallback，API/static/docs 路径不回退。 | 同上 |
| [~] | `cmd/jftrade-desktop/main_test.go:426:TestDesktopLogPageCapsLimitAndPaginatesAllLines` | `jftrade-desktop::native::tests::log_reader_caps_page_limit_and_paginates_all_lines` | 修正页大小常量为 Go 的 default=200、max=500；新增 2,005 行首尾页回归。Rust `DesktopLogPage` 尚未提供 Go 的 `nextOffset` 字段。 | `node scripts/quality/cargo-nextest.mjs run -p jftrade-desktop --all-targets --locked` |
| [~] | `cmd/jftrade-desktop/main_test.go:395:TestListDesktopLogDaysAndReadsFilteredPage` | `jftrade-desktop::native::tests::log_reader_matches_go_filter_paging_and_day_order` | 覆盖日期排序、非法文件名忽略、level/query 过滤；fixture 和 owner 与 Go 不同。 | 同上 |
| [~] | `cmd/jftrade-desktop/main_test.go:464:TestDesktopLogPageTailOffsetReturnsLastPageInFileOrder` | `jftrade-desktop::native::tests::log_reader_matches_go_filter_paging_and_day_order` | 覆盖 latest offset 的过滤后尾页顺序；未覆盖 Go 的 partial/full 两个独立 fixture 和 `nextOffset=nil`。 | 同上 |
| [~] | `cmd/jftrade-desktop/main_test.go:506:TestDesktopLogPageTailOffsetAppliesFiltersBeforePaging` | `jftrade-desktop::native::tests::log_reader_matches_go_filter_paging_and_day_order` | 证明先过滤再计算 latest offset；仍缺 Go 的完整 nextOffset wire 断言。 | 同上 |
| [~] | `cmd/jftrade-desktop/main_test.go:523:TestListDesktopLogDaysMissingDirReturnsEmpty` | `jftrade-desktop::native::tests::test_list_desktop_log_days_missing_dir_returns_empty` | 不存在目录返回空列表；入口从 Go service helper 迁移到 Tauri DesktopPort。 | 同上 |
| [~] | `cmd/jftrade-desktop/desktop_window_state_test.go:22:TestDesktopWindowStateRoundTrip` | `jftrade-desktop::window_state::tests::state_round_trip_is_atomic_and_rejects_invalid_bounds` | 覆盖原子保存、权限、round-trip 和非法边界；未模拟真实 WebviewWindow capture。 | 同上 |
| [~] | `cmd/jftrade-desktop/desktop_window_state_test.go:60:TestEnsureDesktopWindowVisibleMovesOffscreenWindow` | `jftrade-desktop::window_state::tests::visibility_requires_a_real_intersection` | 覆盖真实 monitor 交集判定；offscreen 居中位置仍待可注入 Tauri 窗口测试。 | 同上 |
| [~] | `cmd/jftrade-desktop/desktop_startup_test.go:124:TestDesktopShutdownIsIdempotent` | `jftrade-desktop::native::tests::stop_product_is_idempotent_across_concurrent_invocations` | 覆盖并发 stop owner 清空；Go 的 shutdown closure 调用计数尚未注入。 | 同上 |
| [~] | `cmd/jftrade-desktop/desktop_startup_test.go:93:TestDesktopShutdownCancelsStartupAndReclaimsLateResources` | 边界保留 | Tauri setup cancellation 与 late runtime handle 回收没有 headless 注入点，不能伪造 Rust 等价测试。 | 同上 |
| [~] | `cmd/jftrade-desktop/desktop_updates_test.go:9:TestDesktopUpdateServiceSelectsLatestStableDesktopRelease` | 待补 Rust updater fixture | 当前仅有 updater 配置校验，缺 stable/prerelease/draft 过滤和版本选择测试。 | 同上 |
| [~] | `internal/datamanagement/service_test.go:44:TestServiceFallbacks` | `jftrade-datamanagement::maintenance::maintenance_service_confirmation_validation_and_rejection_parity` | Rust 覆盖确认字段 fail-closed；Go nil backend 的 overview 空结果和各方法 fallback 尚未完整映射。 | `node scripts/quality/cargo-nextest.mjs run -p jftrade-datamanagement --all-targets --locked` |
| [~] | `internal/datamanagement/service_test.go:71:TestServiceDelegatesTypedRequests` | 同上 | Rust 调用 maintenance port，但未逐一覆盖 Go 六类 typed request 转发。 | 同上 |
| [~] | `internal/datamanagement/maintenance_test.go:33:TestMaintenanceRegistryFailsClosedForMissingCapabilities` | `jftrade-datamanagement::tests::busy_owner_blocks_preview_and_execute_without_mutation` | 覆盖 busy owner 阻断 mutation；缺 Go 每个 nil capability 的独立断言。 | 同上 |
| [~] | `internal/buildinfo/buildinfo_test.go:8:TestSnapshotTrimsBuildMetadataAndDefaultsBuildTime` | 边界保留 | Rust 没有 Go buildinfo snapshot API；Cargo/Tauri metadata 不迁移该旧 owner。 | `pnpm run check:ai-context` |

其余 `cmd/jftrade-desktop` 的 Wails window zoom、single-instance callback、tray menu、
`runtime-config.js` handler、open-folder 平台命令和 startup safe-message 条目已在全量
索引中逐条保留为 `partial` 或 `boundary`，没有把 Tauri plugin 行为冒充函数级等价。

## 未完成与下一批

- `DesktopLogPage.nextOffset` wire 字段是否需要保留，需在 API/前端契约批次单独决定。
- updater stable release 选择、Tauri window state fake、startup cancellation 注入测试仍待补。
- `internal/datamanagement` 的 nil backend、六类 typed delegation 和首个 busy reason 仍是部分覆盖。
- `cmd/generate-*`、`cmd/check-go-coverage` 等 Go 专用 tooling 只保留边界，不迁移旧 Go 实现。

