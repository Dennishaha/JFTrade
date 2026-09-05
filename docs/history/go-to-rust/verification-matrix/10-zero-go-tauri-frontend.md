# 领域 10：零 Go 残留、Tauri 发布及前端一致性

> **2026-09-05 复核提示：本卷以下内容为原始核查材料，非已确认缺陷或实施指令。** 风险状态、事实勘误、修复限制和后续验收以[主文](../go_to_rust_comprehensive_verification_matrix.md)为准。下文的绝对化结论、覆盖统计、平台枚举、旧行号及修复建议尚未逐项重验；不得据此直接改契约/schema、静默丢数据、自动重报订单或宣告发布就绪。接手对应任务时，应将复现/反证同步回本卷。

- **关联主索引**: [全景验证矩阵主导航](../go_to_rust_comprehensive_verification_matrix.md)
- **基线版本（Go）**: `origin/go` commit `452dea11`
- **目标版本（Rust）**: `main` (HEAD)

---

### 2.10 领域 10：零 Go 残留、Tauri 发布及前端一致性（Zero-Go, Tauri Packaging & Frontend Route Coverage）

#### 2.10.1 Go 基线路径、符号、关键行号与历史行为
- **源码路径**:
  - 历史 Go 入口: `cmd/jftrade-api`
  - 历史 Wails 桌面壳: `wails`, `wails3`
  - 历史构建文件: `go.mod`, `go.sum`, `go.work`
- **历史行为**:
  在历史架构中，JFTrade 是基于 Go API Server + Wails 桌面壳构建的混合应用，跨平台打包强依赖 Go 编译器与 CGO 交叉编译环境。

#### 2.10.2 Rust 当前实现路径、符号、关键行号与架构机制
- **源码路径**:
  - 门禁脚本: `scripts/check-zero-go.mjs`
  - 平台配置: `scripts/lib/desktop-release-inputs.mjs:14-25`
  - 运行时打包: `scripts/prepare-tauri-release-runtime.mjs:56-100`
  - Sidecar 构建: `scripts/build-marketdata-sidecar.mjs:27-89`
  - 前端源码: `apps/web/src` (548 个文件)
- **关键机制**:
  1. **零 Go 门禁校验**: `pnpm run check:zero-go` 严格扫描 2,624 个纳管文件，严禁 `.go` 后缀、退役入口符号，并检测二进制 ELF/Mach-O 中的 `\xff Go buildinf:` 魔数。
  2. **4 平台 Sidecar 自包含**: 支持 `darwin-arm64`, `darwin-amd64`, `linux-amd64`, `windows-amd64`。PyInstaller 编译 Python Sidecar，Rolldown 打包 Node Pineworker。
  3. **前端路由覆盖与盲区**: 前端实际覆盖 265 条路由，剩余 13 条存在业务或调试属性的盲区。

#### 2.10.3 微观差异与破坏性边界失效推演

#### 1. 前端第 11 项盲区：Futu 交易解锁接口补齐 (P0-04 闭环)
- **路由路径**: `POST /api/v1/brokers/{brokerId}/unlock`
- **历史问题**: Futu 官方 OpenD 在启动后，交易通道默认处于**锁定（LOCKED）状态**，实盘下单必须先输入交易密码执行解锁。历史前端交易控制台未设计交易密码输入弹窗，实盘下单遭遇 `ACCOUNT_LOCKED` 错误。
- **闭环实现**:
  1. `apps/web/src/composables/trading/brokerUnlockCrypto.ts`: 零外部依赖 RFC 1321 纯 TypeScript MD5 实现，完全兼容标准向量与 UTF-8，规避 Web SubtleCrypto 不支持 MD5 的限制。
  2. `apps/web/src/composables/trading/useBrokerUnlock.ts`: 券商解锁状态机，支持主动解锁（`requestUnlock`）与被动拦截响应（`watch(lastOrderFeedback)`）；向 `/api/v1/brokers/{brokerId}/unlock` 发送 `{ unlock: true, passwordMd5 }`；针对 400（参数错误）、502（密码错误）、504（超时）提供精确错误映射；模拟盘环境（`SIMULATE`）与非锁定券商默认免解锁放行。
  3. `apps/web/src/components/workspace/BrokerUnlockDialog.vue`: 交易密码输入弹窗，提交/取消后即时擦除密码变量（凭据不落日志/不持久化），防止重试阶段重放订单。
  4. `apps/web/src/components/workspace/OrderEntryPanel.vue`: 挂载解锁弹窗与面板头部“未解锁”状态徽标，实盘确认报单前拦截并弹窗解锁。
- **验证证据**:
  - `apps/web/tests/composables/brokerUnlockCrypto.test.ts` (3 tests PASS)
  - `apps/web/tests/composables/useBrokerUnlock.test.ts` (6 tests PASS)
  - `apps/web/tests/components/workspace/BrokerUnlockDialog.test.ts` (6 tests PASS)
  - `apps/web/tests/components/workspace/OrderEntryPanel.test.ts` (16 tests PASS)
  - `apps/web/tests/shared/ThemeTokenUsage.contract.test.ts` (2 tests PASS)

#### 2. 13 个路由盲区全景清单与根因分析

| 序号 | HTTP 方法与路径 | 能力域 (Capability) | 盲区性质与根因分析 |
| :---: | :--- | :---: | :--- |
| 1 | `GET /api/v1/brokers/{brokerId}/klines` | `brokers` | **被替代冗余路由**。前端行情统一使用 `/api/v1/market-data/candles/*`，底层券商直出 K 线在 UI 中无入口。 |
| 2 | `GET /api/v1/brokers/{brokerId}/quote` | `brokers` | **底层探针路由**。前端使用通用报价面板，券商专用报价探针仅用于后端链路诊断。 |
| 3 | `GET /api/v1/brokers/{brokerId}/securities` | `brokers` | **无对应 UI 组件**。前端标的搜索基于全市场证券池，未提供单券商可交易标的过滤视图。 |
| 4 | `GET /api/v1/market-data/corporate-actions/{market}/{symbol}` | `market-data` | **功能未闭环**。除权除息接口已就绪，但前端 K 线和标的详情页尚未接入公司行动事件展示。 |
| 5 | `GET /api/v1/market-data/news/{market}/{symbol}` | `market-data` | **功能未闭环**。个股新闻接口已就绪，但前端面板仅对接了宏观新闻流。 |
| 6 | `GET /api/v1/settings/execution` | `settings` | **合并归并**。前端设置页统一通过 `/api/v1/settings` 获取全量树，未单独调用执行设置子路由。 |
| 7 | `PUT /api/v1/settings/execution` | `settings` | **合并归并**。保存系统设置通过全量保存完成，未单独绑定该细粒度提交路由。 |
| 8 | `GET /api/v1/system/exchange-calendars/sources` | `system` | **管理后台缺位**。日历来源列表接口已就绪，但前端仅提供了刷新开关，未提供数据源列表管理。 |
| 9 | `GET /api/v1/system/storage/overview` | `system` | **历史路径替代**。前端使用 `/api/v1/data-management/overview`，系统组下的存储总览成为死路径。 |
| 10 | `GET /api/v1/system/worker/broker-order-updates` | `system` | **内部调试流**。该路由为长轮询调试接口，实盘推送前端统一使用全局 WebSocket。 |
| 11 | `POST /api/v1/brokers/{brokerId}/unlock` | `brokers` | **已闭环 (P0-04)**。已补齐前端交易密码弹窗与解锁链路，全链路通过单元与组件测试。 |
| 12 | `POST /api/v1/execution/buying-power` | `execution` | **功能未闭环**。购买力预估接口。前端仅展示静态可用资金，未在输入数量时动态联动预估。 |
| 13 | `POST /api/v1/strategy-definitions/{definitionId}/apply-linked-instances` | `strategy-definitions` | **批处理未挂载**。定义更新批量同步至运行实例接口。前端保存定义时未提供“同步更新运行实例”按钮。 |

#### 2.10.4 Release Qualification 验证清单
- [ ] **RQ-ZERO-01（门禁流）**: 执行 `pnpm run check:zero-go`，核验返回 0 且无遗留 Go/Wails 符号。
- [ ] **RQ-PACK-02（打包流）**: 在 4 平台（macOS arm64/x64, Linux, Windows）运行 Tauri 打包，核验二进制内嵌 Python 和 Node Sidecar 正常拉起。
- [x] **RQ-FE-03（前端闭环 - 阻断门禁）**: 在前端交易控制台补齐券商交易密码解锁弹窗，并在报单前完成解锁全链路验证（P0-04 闭环）。
- [ ] **RQ-FE-04（盲区审计）**: 按照 13 个盲区清单逐一核对，对功能未闭环的路由制定前端接入排期。
