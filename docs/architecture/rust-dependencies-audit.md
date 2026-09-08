# JFTrade 工作区 Rust 依赖审计与自研模块权衡分析报告

> **标准与基线**: 本报告是 JFTrade Rust 工作区的依赖架构与通用功能自研审计事实源。
> 审计基准为 Rust `1.97.1`、2024 Edition、工作区根目录 `#![forbid(unsafe_code)]` 内存安全门禁与 `AGPL-3.0-only` 开源许可证基准。
> 报告中引用的所有源码路径、符号名称、数据结构及行号均 100% 映射自代码仓库当前真实提交状态。

---

## 目录
1. [执行摘要 (Executive Summary)](#1-执行摘要-executive-summary)
2. [全工作区 Member Crates 清单与依赖拓扑 (Workspace Member Crate Inventory)](#2-全工作区-member-crates-清单与依赖拓扑-workspace-member-crate-inventory)
3. [按技术领域深度排查与识别清单 (Identified Custom Implementations by Technical Domain)](#3-按技术领域深度排查与识别清单-identified-custom-implementations-by-technical-domain)
   - [3.1 安全与并发原语 (Security & Concurrency Primitives)](#31-安全与并发原语-security--concurrency-primitives)
   - [3.2 限流、网络协议与通信编解码 (Rate Limiting & Networking/Codecs)](#32-限流网络协议与通信编解码-rate-limiting--networkingcodecs)
   - [3.3 桌面端与系统工具 (Desktop & System Utilities)](#33-桌面端与系统工具-desktop--system-utilities)
   - [3.4 金融数学与交易日历 (Financial Mathematics & Trading Calendar)](#34-金融数学与交易日历-financial-mathematics--trading-calendar)
   - [3.5 存储、Schema 校验与专用语言编译引擎 (Storage, Schema & Language Engines)](#35-存储schema-校验与专用语言编译引擎-storage-schema--language-engines)
   - [3.6 AI 助手与大模型驱动接入 (AI Assistant & LLM Drivers)](#36-ai-助手与大模型驱动接入-ai-assistant--llm-drivers)
4. [全景对比与决断矩阵 (Full-Spectrum Comparison & Trade-Off Matrices)](#4-全景对比与决断矩阵-full-spectrum-comparison--trade-off-matrices)
5. [保留自研的核心业务辩护 (Deep-Dive Rationale for Custom Code Retention)](#5-保留自研的核心业务辩护-deep-dive-rationale-for-custom-code-retention)
   - [5.1 PineTS V8 影子对齐与技术指标状态机](#51-pinets-v8-影子对齐与技术指标状态机)
   - [5.2 单写属主租约与行情代际围栏防护](#52-单写属主租约与行情代际围栏防护)
   - [5.3 SQLite Fail-Closed PRAGMA 校验与动态分表安全](#53-sqlite-fail-closed-pragma-校验与动态分表安全)
   - [5.4 券商私有二进制协议的高内聚与零抽象损耗](#54-券商私有二进制协议的高内聚与零抽象损耗)
   - [5.5 多市场复杂交易时段与法定节假日仲裁](#55-多市场复杂交易时段与法定节假日仲裁)
   - [5.6 桌面多 Profile 隔离与断电强安全持久化 (`window_state.rs`)](#56-桌面多-profile-隔离与断电强安全持久化-window_staters)
   - [5.7 纯安全微算法与轻量工具的零依赖哲学 (Zero-Dependency Micro-Utilities)](#57-纯安全微算法与轻量工具的零依赖哲学-zero-dependency-micro-utilities)
6. [分级演进路线图 (Prioritized Migration Roadmap: P0 / P1 / P2)](#6-分级演进路线图-prioritized-migration-roadmap-p0--p1--p2)
   - [P0 紧急安全修复 (Immediate Security Fix)](#p0-紧急安全修复-immediate-security-fix)
   - [P1 架构与稳定性治理 (Architecture, Performance & Reliability)](#p1-架构与稳定性治理-architecture-performance--reliability)
   - [P2 生态收敛与体验优化 (Cleanliness & Ecosystem Convergence)](#p2-生态收敛与体验优化-cleanliness--ecosystem-convergence)
7. [独立复现与验证指南 (Independent Verification Guide)](#7-独立复现与验证指南-independent-verification-guide)

---

## 1. 执行摘要 (Executive Summary)

### 1.1 审计背景与工作区规模
JFTrade 是由 Rust 核心引擎、Axum API 网关、Vue 3 本地控制台、Tauri 2 原生桌面壳、Node.js PineTS 策略 worker 与 Python market-data helper 组成的现代化本地量化工作台。系统承载全量 278 条 REST/SSE/WebSocket 生产 API，管控 9 个权威 SQLite 数据库文件，并已实现完全零 Go（Zero-Go）的生产交付。

全工作区（Cargo Workspace）由根目录 `Cargo.toml` 统一编排，包含 **21 个 Member Crates**，源码与测试总规模达 **255,091 行 Rust 代码**（其中生产源码 `src/` 为 179,069 行，集成测试与基准套件 `tests/` 为 76,022 行）。

### 1.2 合规与安全基准
1. **开源许可证约束**:
   - 依据根目录 `Cargo.toml` 与 `deny.toml`，JFTrade 开源协议为 `AGPL-3.0-only`。
   - 依赖项许可证白名单严格限定为：`AGPL-3.0-only`、`Apache-2.0`、`BSD-3-Clause`、`MIT`、`Unicode-3.0`（除少数底层 GUI 系统库的显式例外外）。任何外部引入的社区依赖必须与此许可强兼容。
2. **内存安全约束**:
   - 根 `Cargo.toml` 全局启用：
     ```toml
     [workspace.lints.rust]
     unsafe_code = "forbid"
     ```
   - 全工作区 21 个 crate 均强制遵守禁止裸指针操作与未审计 FFI 的安全红线。任何引入大量裸 `unsafe` 且缺乏严密形式化验证的社区 crate 均不予接纳。

### 1.3 审计发现与总体结论
本次审计对工作区 21 个 crate 进行了全景逐行扫描与工程比对，共识别出 **16 个关键自研实现点**。评估结果高度两极化，充分体现了金融量化系统的工程特异性：

- **【建议替换 (Replace)】（共 4 项）**: 集中在 Web 会话密码学比对、API/客户端限流、异步流式文本解析与桌面日志日期校验等**通用横切关注点**。此类模块手写不仅容易引入时序攻击（Timing Attack）或跨分块切片 Bug，而且社区已有高质量工业标准 crate。
  - **P0 级（1 项）**: `product_auth_session_crypto.rs` 中手写的 `constant_time_eq`，在 Release 模式 LTO 激进优化下极易丧失常数时间特性，建议立即替换为已有隐式依赖的 `subtle::ConstantTimeEq`（引入 0 个新依赖，0 编译开销）。
  - **P1 级（3 项）**: 引入真实存在且验证稳定的 `governor`（建议锁定 `=0.8.1` 或评估最新 `=0.10.4`）统一内外限流（替换手写加锁修剪字典，并在 `jftrade-integration-futu` 建立主动令牌桶，杜绝券商端 30 秒 10 次报错）；采用 `tokio::task::JoinSet` 消除 ADK 续跑专用 OS Reaper 线程；引入 `eventsource-stream`（`=0.2.3`）替代手写 SSE 状态机（**0 个新增外部依赖**，其自身及底层 `nom v7.1.3` 解析器已由 `rig-core` 间接引入并锁定在 `Cargo.lock` 中）。
  - **P2 级（1 项）**: 桌面端日志日期校验切换至已深度集成的 `jiff::civil::Date::strptime`，单行消灭手写字节切片遍历。
- **【建议保留自研 (Retain)】（共 9 项）**: 集中在定点数精确财务计算、单写属主租约锁（`WriterLease`）、SQLite PRAGMA 动态模式防御内省、自研 2,800+ 行原生 Pine Script v6 编译器、富途 44 字节二进制报文编解码、微秒级代际行情缓存（`TickCache`），以及纯安全微算法（如 FNV-1a 回测指纹与查表十六进制编码 `encode_hex`，严格遵守 10 行以内纯 Safe 微工具避免引入长尾依赖的架构原则）。此类自研实现**绝非盲目造轮子，而是紧扣量化撮合确定性、单写属主强一致性、微秒级安全与零外部进程依赖的核心架构基石**，盲目替换必将导致回归破坏与性能衰退。
- **【中立/混合演进 (Neutral / Hybrid Evolution)】（共 3 项）**:
  - **桌面窗口几何状态与多屏幕适配（`window_state.rs`）**: 现有实现强绑定 `native_lifecycle.rs` 的多 Profile 动态路径隔离，且依托 `tempfile` 原子替换 + POSIX `0o600` + `sync_all` 强刷盘保障掉电抗截断韧性；官方 `tauri-plugin-window-state` 硬编码配置路径且使用非原子 `fs::write`，无法直接无缝平替。建议采取混合演进方案：保持现有原子落盘与 Profile 路径路由，仅在需要时借鉴官方插件的多屏热插拔与 DPI 换算逻辑。
  - **策略多实例 Tokio 运行时收敛（`strategy_runtime.rs`）**: 避免每个策略实例创建独立多线程 Runtime 导致线程池膨胀，收敛至单线程或 Ambient Runtime。
  - **AI 助手端模型驱动双轨标准化**: `jftrade-assistant` 的 `rig-core` 抽象与 `jftrade-engine` 的原生 `reqwest` `/responses` 双轨调用的标准化收敛。

---

## 2. 全工作区 Member Crates 清单与依赖拓扑 (Workspace Member Crate Inventory)

下表汇总了 JFTrade 工作区全部 21 个 Member Crate 的代码路径、精确代码行数统计、核心架构职责以及当前的依赖选型姿态：

| Crate 名称 | 相对路径 | 架构分类 | 生产源码 (src LOC) | 测试代码 (tests LOC) | 总行数 (Total LOC) | 核心职责与架构定位 | 依赖姿态与自研/社区策略 |
|---|---|---|---:|---:|---:|---|---|
| `jftrade-owner-lock` | `crates/jftrade-owner-lock` | 系统与并发 | 196 | 0 | 196 | 跨进程排他文件锁与 `WriterLease` 属主租约协议 | **高度自研**：基于 Rust 1.97+ stdlib `File::try_lock`，纯安全代码，内嵌诊断元数据 |
| `jftrade-kernel` | `crates/jftrade-kernel` | 基础内核 | 873 | 107 | 980 | `Fixed8` 定点数、`DecimalTradingExt` 步长对齐、`DecimalText` 百万位展开 | **领域扩展**：底层依赖 `rust_decimal`，深度扩展金融量化专用操作 |
| `jftrade-calendar` | `crates/jftrade-calendar` | 交易日历 | 3,624 | 1,000 | 4,624 | 美/港/A股多交易时段仲裁引擎、法定节假日字典与快照存储 | **高度自研**：深度集成 `jiff::TimeZone`，手写复活节高斯算法与多源仲裁 |
| `jftrade-store-settings-file` | `crates/jftrade-store-settings-file` | 存储持久化 | 762 | 1,021 | 1,783 | 读写型 JSON 配置持久化、单写租约保护与 POSIX 0600 加固 | **混合架构**：复用 `tempfile::Builder` 原子替换，承载 14 个领域 Port |
| `jftrade-store-sqlite` | `crates/jftrade-store-sqlite` | 存储持久化 | 16,995 | 6,850 | 23,845 | 9 个 SQLite 数据库读写、Schema Manifest PRAGMA 强校验与在线热备 | **标准驱动+强自研校验**：采用官方 `rusqlite`，自研 PRAGMA 深度内省与动态分表校验 |
| `jftrade-engine` | `crates/jftrade-engine` | 核心宿主 | 95,868 | 52,567 | 148,435 | 生产系统枢纽、278 条 API 后端、MCP 服务器、会话安全与策略调度 | **核心枢纽**：混合使用 Axum/Tokio，包含若干待优化的通用并发/密码学轮子 |
| `jftrade-api` | `crates/jftrade-api` | 接口网关 | 2,954 | 442 | 3,396 | 六边形 API 外壳、Axum 网关 fallback 路由、SSE 与 WebSocket 协议 | **架构规范**：严格遵循端口隔离，集中派发 278 条路由 Envelope 契约 |
| `jftrade-strategy` | `crates/jftrade-strategy` | 策略引擎 | 4,384 | 549 | 4,933 | Pine Script v6 本地词法分析、递归下降语法解析、AST、语义与计划器 | **高度自研**：2,811 行原生 Rust 编译器管线，填补 Rust 生态空白 |
| `jftrade-backtest` | `crates/jftrade-backtest` | 回测分析 | 2,078 | 5,380 | 7,458 | Bar 级确定性撮合、动态费率、技术指标计算与 FNV-1a 结果指纹 | **领域专属**：高保真模拟 PineTS V8 JS 舍入逻辑，确保回测结果绝对一致 |
| `jftrade-marketdata` | `crates/jftrade-marketdata` | 行情驱动 | 2,930 | 668 | 3,598 | 行情 Catalog、需求账本 `DemandBook`、代际围栏 `TickCache` 与路由分发 | **领域专属**：微秒级环形队列，强绑定 `provider_generation` 切换防穿透 |
| `jftrade-trading` | `crates/jftrade-trading` | 交易风控 | 2,539 | 848 | 3,387 | 订单执行状态机、前置硬风控检查（手数/名义金额/熔断）与影子账本 | **纯业务逻辑**：零多余轮子，依赖核心类型 |
| `jftrade-broker` | `crates/jftrade-broker` | 券商抽象 | 358 | 311 | 669 | 交易账户、持仓与订单请求领域 Port 契约 | **纯抽象接口**：零外部依赖 |
| `jftrade-assistant` | `crates/jftrade-assistant` | AI 助理 | 2,210 | 1,145 | 3,355 | 助理审批、工作流状态机、`rig-core` 适配器与上下文编排 | **双轨演进**：已声明 `rig-core`，但与 engine 存在双轨调用脱节 |
| `jftrade-integration-futu` | `crates/jftrade-integration-futu` | 外部集成 | 29,004 | 3,553 | 32,557 | 富途 OpenD 44 字节二进制报文封包、TCP 长连接会话多路复用与交易转换 | **协议专属**：基于 protobuf 与 sha1 实现 112 行纯 safe 帧解析，高度稳定 |
| `jftrade-integration-pine` | `crates/jftrade-integration-pine` | 外部集成 | 3,816 | 699 | 4,515 | Node.js PineTS 外部运行时管理、gRPC 通信池化与会话钉扎 | **进程生命周期**：标准 `tokio::process` 与 `tonic` gRPC 组合 |
| `jftrade-integration-marketdata-helper` | `crates/jftrade-integration-marketdata-helper` | 外部集成 | 1,484 | 0 | 1,484 | Python yfinance 辅助进程生命周期与 HTTP 健康探测 | **标准进程原语**：使用 `tokio::process` 与 `reqwest` |
| `jftrade-datamanagement` | `crates/jftrade-datamanagement` | 数据维护 | 1,188 | 0 | 1,188 | 离线行情导入导出、分卷压缩与校验 | **标准工具**：基于 `zip` 与 `serde` |
| `jftrade-research` | `crates/jftrade-research` | 投研分析 | 1,359 | 422 | 1,781 | 选股方案预设（ScreenPreset）乐观并发版本控制与因子库 | **业务领域**：纯业务状态转换 |
| `jftrade-settings` | `crates/jftrade-settings` | 系统设置 | 3,465 | 107 | 3,572 | 设置模型、Argon2id 密码哈希与安全性校验 | **规范集成**：采用标准 `argon2` 依赖实现安全加盐验密 |
| `jftrade-watchlist` | `crates/jftrade-watchlist` | 自选标的 | 184 | 0 | 184 | 标的代码格式标准化与自选股群组替换差异算法 | **纯业务逻辑**：轻量级实用工具 |
| `jftrade-desktop` | `apps/desktop/src-tauri` | 桌面外壳 | 2,798 | 353 | 3,151 | Tauri 2 原生桌面壳、窗口状态几何持久化与本地日志轮转 | **混合架构**：Tauri 2 插件体系，手写了待替换的窗口几何恢复逻辑 |
| **全工作区总计** | **21 个 Crates** | — | **179,069** | **76,022** | **255,091** | **JFTrade 全功能量化工作台** | **总体工程成熟，局部存在高价值改进点** |

---

## 3. 按技术领域深度排查与识别清单 (Identified Custom Implementations by Technical Domain)

### 3.1 安全与并发原语 (Security & Concurrency Primitives)

#### 3.1.1 手写恒定时间比较与查表十六进制编码
- **源码实证**:
  - 文件: `crates/jftrade-engine/src/product_auth_session_crypto.rs`
  - 行号: 15–26 (`constant_time_eq`), 28–37 (`encode_hex`)
  - 调用方: `crates/jftrade-engine/src/product_auth_session_manager.rs:192` (`is_csrf_valid`)
- **代码实现细节**:
  ```rust
  // crates/jftrade-engine/src/product_auth_session_crypto.rs:15-26
  pub(super) fn constant_time_eq(a: &str, b: &str) -> bool {
      let a_bytes = a.as_bytes();
      let b_bytes = b.as_bytes();
      if a_bytes.len() != b_bytes.len() {
          return false;
      }
      let mut diff = 0;
      for (x, y) in a_bytes.iter().zip(b_bytes.iter()) {
          diff |= x ^ y;
      }
      diff == 0
  }

  // crates/jftrade-engine/src/product_auth_session_crypto.rs:28-37
  pub(super) fn encode_hex(bytes: impl AsRef<[u8]>) -> String {
      const HEX_CHARS: &[u8; 16] = b"0123456789abcdef";
      let bytes = bytes.as_ref();
      let mut hex = String::with_capacity(bytes.len() * 2);
      for &byte in bytes {
          hex.push(HEX_CHARS[(byte >> 4) as usize] as char);
          hex.push(HEX_CHARS[(byte & 0x0f) as usize] as char);
      }
      hex
  }
  ```
- **问题分析与工程权衡**:
  1. **恒定时间比较 (`constant_time_eq`)**: 上述手写代码缺乏底层内存屏障（如 `core::hint::black_box`）。在工作区 `[profile.release]` 开启 `codegen-units = 1` 与 `lto = "thin"` 激进优化时，LLVM 极其擅长将简单的异或归约循环重写为提前终止的分支逻辑，进而破坏常数时间特性，在 Web 身份验证与 CSRF 校验中埋下时序侧信道攻击隐患。社区对标为 `subtle = "=2.6.1"` (dalek-cryptography 维护，密码学界常数时间操作事实标准)。
     - **零成本优势**: 运行 `cargo tree -i subtle` 证实，`subtle v2.6.1` **已经被现有依赖 `argon2`、`rustls` 与 `digest` 间接引入并固定在 `Cargo.lock` 中**。在 `crates/jftrade-engine/Cargo.toml` 声明显式接入将带来 **0 个新 crate、0 字节体积增加、0 额外编译耗时**！
  2. **查表十六进制编码 (`encode_hex`)**: 代码仅 9 行纯 Safe Rust，逻辑为确定性的常量表查表与高低 4 位移位拼接，不涉及任何时序敏感或密码学黑盒语义。
     - **微工具依赖哲学对齐**: 经查验，`Cargo.lock` 中当前完全不存在 `hex` crate。若为了 9 行无外部安全隐患的简单代数查表而引入外部 `hex` 依赖，将直接与第 3.4.5 节针对 FNV-1a（10 行简单代数）所确立的“微工具零外部依赖”原则自相矛盾。因此，应秉持同一哲学保持自研。
- **评估结论**:
  - `constant_time_eq`: **【建议替换 (Replace)】— 优先级: P0** (采用已在 lockfile 中的 `subtle::ConstantTimeEq`)。
  - `encode_hex`: **【建议保留自研 (Retain)】** (对齐 FNV-1a 微工具零外部依赖标准)。

#### 3.1.2 跨进程排他写入租约 (`jftrade-owner-lock`)
- **源码实证**:
  - 文件: `crates/jftrade-owner-lock/src/lib.rs:73-128` (全文件 197 行)
  - 关键符号: `WriterLease`, `OwnerDiagnostic`, `File::try_lock()` (Line 103), `secure_permissions(0o600)` (Line 160-164)
- **实现剖析与辩护**:
  `jftrade-owner-lock` 直接调用 Rust 1.97+ 标准库稳定支持的 `std::fs::File::try_lock()` 与 `std::fs::TryLockError::WouldBlock`，做到了全模块 **100% Safe Rust (`#![forbid(unsafe_code)]`)**。
  更关键的是，它在锁文件中原子写入了当前持有者的元数据结构 `OwnerDiagnostic`（包含 PID、启动时间戳、所属 Profile），使任何争锁失败的进程均能精确报出 `WriterLeaseError::Held { path }` 并附带持有者 PID。
- **社区对比 (`fd-lock`, `fslock`, `fs2`)**:
  社区类似 crate 普遍依赖 `libc::flock` 或 Win32 裸句柄，含有大量 `unsafe` 块；且仅能锁定文件句柄，不具备诊断载荷写入功能。
- **结论**: **【建议保留自研 (Retain)】**。

#### 3.1.3 ADK ContinuationSupervisor 自研线程管理器与 Reaper 线程
- **源码实证**:
  - 文件: `crates/jftrade-engine/src/product_adk_model_runtime.rs:133-186`
  - 关键符号: `ContinuationSupervisor`, `ContinuationTask`, `reaper` (Line 159-178), `spawn` (Line 188-250)
- **代码细节与缺陷**:
  ```rust
  // product_adk_model_runtime.rs:159-178 启动专用 Reaper 回收线程
  let reaper = thread::Builder::new()
      .name("jftrade-adk-continuation-reaper".to_owned())
      .spawn(move || {
          while let Ok(completion) = completion_rx.recv() {
              ...
              if let Some(completed) = completed
                  && let Ok(mut join) = completed.join.lock()
                  && let Some(handle) = join.take()
              {
                  let _ = handle.join();
              }
          }
      })
      .ok();
  ```
  该模块为管理 AI 模型的审批与长时间流式续跑，自行发明了一套 OS 线程管理器：每个任务分配一个独立系统线程 `thread::Builder::spawn`，并常驻一个专用后台 Reaper 线程监听 `mpsc::channel` 并执行 `handle.join()`。每个 OS 线程至少占据 2~8MB 虚拟栈空间，在高并发或网络抖动时极易造成系统线程与文件句柄浪费。
- **社区对标**:
  工作区中已重度使用 `tokio = "=1.53.1"`。Tokio 内置的 `tokio::task::JoinSet` 与 `tokio_util::sync::CancellationToken` 完全能够原生提供有界协程生命周期管理与完成回收，无需专用线程。
- **结论**: **【建议替换 (Replace)】— 优先级: P1**。

#### 3.1.4 策略运行时多实例独立创建 Multi-Threaded Tokio Runtime
- **源码实证**:
  - 文件: `crates/jftrade-engine/src/strategy_runtime.rs:27-31, 45-55, 323-338`
  - 核心逻辑:
    ```rust
    // strategy_runtime.rs:323-328
    let join = std::thread::Builder::new()
        .name(format!("strategy-runtime-{instance_id}"))
        .spawn(move || {
            let runtime = match tokio::runtime::Runtime::new() {
                Ok(runtime) => runtime, ...
            };
            ...
        });
    ```
- **问题分析**:
  每启动一个策略实例，`StrategyRuntimeManager` 就会派生一个独立 OS 线程，并在线程内调用 `Runtime::new()` 创建一套完整的多线程 Tokio Runtime（包含工作窃取线程池、Reactor 事件轮询器与定时轮盘）。若用户启动 10 个策略实例，系统将瞬间激增数十个底层工作线程，引发严重的 CPU 线程调度上下文切换。
- **演进考量**:
  策略内部存在少量调用同步 SQLite 读写与 gRPC 流的操作。建议后续将其降级为 `tokio::runtime::Builder::new_current_thread()`（单线程 runtime），或直接收拢到 Engine 主 runtime 下的 `tokio::task::spawn` 并配合 `spawn_blocking`。
- **结论**: **【中立/视后续演进而定 (Neutral/Evolve)】— 优先级: P1**。

---

### 3.2 限流、网络协议与通信编解码 (Rate Limiting & Networking/Codecs)

#### 3.2.1 手写滑动窗口登录限流与券商接口被动限流降级
- **源码实证**:
  - Web 会话限流: `crates/jftrade-engine/src/product_auth_session_manager.rs:27-29, 52, 129-165` (`LoginAttempt`, `failed_attempts: Arc<Mutex<BTreeMap<String, LoginAttempt>>>`, `prune_login_attempts`, `evict_oldest_login_attempt`)
  - 券商接口被动降级: `crates/jftrade-engine/src/product_trade_margin_route.rs:106-113` (`is_margin_ratio_rate_limited`)
- **代码实录**:
  ```rust
  // crates/jftrade-engine/src/product_trade_margin_route.rs:106-113
  fn is_margin_ratio_rate_limited(message: &str) -> bool {
      let lower = message.to_ascii_lowercase();
      lower.contains("频率太高")
          || lower.contains("每30秒最多10次")
          || (lower.contains("too high") && lower.contains("request"))
          || lower.contains("rate limit")
          || lower.contains("rate-limit")
  }
  ```
- **问题分析**:
  1. `product_auth_session_manager.rs` 自行维护互斥锁保护的 `BTreeMap`，每次请求均需加锁并循环清理过期时间点，并在超出 1,024 个键时执行繁重的手动逐出（Eviction），并发拓展性差。
  2. 针对富途 OpenD 明确规定的保证金比率查询限频（如 `Trd_GetMarginRatio` 每 30 秒上限 10 次），系统目前没有任何主动防护机制，而是放任请求打向券商服务端；当接口报错后，靠粗糙的中文字符串匹配识别限流并回退至缓存。这存在极高的被券商风控封号风险。
- **社区对标与依赖拓扑深度透视**:
  - **版本矫正**: 社区推荐库为 `governor`。注意 crates.io 上**并不存在 `=0.8.2` 版本**（该分支生命周期为 `0.8.0`、`0.8.1`、`0.9.0`、...、`0.10.4`）。工程落地应精确锁定稳定版本 `governor = "=0.8.1"`（或评估升级至现代主流 `=0.10.4`）。
  - **传递依赖与 unsafe 属性客观披露**:
    即使配置 `default-features = false, features = ["std"]`，`governor` 绝非单个轻量叶子 crate，而是会引入 `nonzero_ext`、`portable-atomic`、`spinning_top`、`smallvec`、`web-time`、`parking_lot`、`parking_lot_core`、`futures-timer`、`futures-util` 等 **8~10 个传递依赖**。其中核心并发底层 `parking_lot`、`portable-atomic` 与 `smallvec` 在各自 crate 内部重度使用底层 `unsafe` 原语（futex 系统调用、裸指针内联汇编等）。虽然 `governor` 自身暴露 safe 抽象，但在 workspace 严苛的 `#![forbid(unsafe_code)]` 准则下，必须对其传递性 unsafe 拓扑保持清醒认知。
  - **Keyed 模式并发锁机制**:
    在 `features = ["std"]` 且未开启 `dashmap` 时，`governor` 的 Keyed 状态存储默认实现为 `HashMapStateStore<K, S> = Mutex<HashMap<K, InMemoryState, S>>`，底层受全局 `parking_lot::Mutex` 互斥保护（官方源码注释亦注明 "not very performant"），并非全局无锁。每次按键状态检查均需抢占互斥锁；若在超大规模并发场景下追求无锁/分段锁，需显式开启 `dashmap` 特性（进而引入 `dashmap` 依赖）。
- **架构价值与收益评估**:
  - 尽管存在上述传递依赖体积与 Mutex 锁特征，但在 JFTrade 的业务边界内，Web 登录 IP/用户基数小，券商接口种类收敛，`parking_lot::Mutex` 的微秒级开销对吞吐量毫无瓶颈。
  - 核心架构收益在于 **GCRA（通用信元速率算法）的主动流控能力**：使用 `governor::RateLimiter::keyed` 既可直接移除 `product_auth_session_manager.rs` 中 ~80 行易错的手写加锁过期淘汰逻辑，更能在 `jftrade-integration-futu` 中为 `Trd_GetMarginRatio` 等高敏接口建立 30 秒 10 次的主动平滑令牌桶。请求在本地排队或即时拒绝，杜绝触发券商远端风控封号，彻底终结被动的中文报错正则匹配。
- **结论**: **【建议替换 (Replace)】— 优先级: P1** (推荐配置 `governor = { version = "=0.8.1", default-features = false, features = ["std"] }`)。

#### 3.2.2 手写 ADK SSE 流式文本解码器 (`SseDecoder`)
- **源码实证**:
  - 文件: `crates/jftrade-engine/src/product_adk_model_stream.rs:237-298`
  - 核心结构: `SseDecoder`, `event_boundary`, `decode_frame`
- **代码实现细节**:
  手写结构体 `SseDecoder { buffer: String }`，通过 `event_boundary` 函数手动匹配 `\r\n\r\n` 或 `\n\n` 分界符，然后调用 `decode_frame` 逐行剥离 `data:` 前缀，并识别 `[DONE]` 终止符。
- **风险与社区对标**:
  手写文本流状态机在遭遇极端 TCP 报文切片（如 `\r` 与 `\n\n` 分处不同 chunk）或超长 UTF-8 跨帧多字节字符时极易出现切片损坏。社区成熟库 `eventsource-stream = "=0.2.3"` 提供基于 `futures::Stream` 扩展的标准化事件解析，经受了全球海量 LLM 流式调用的检验。
- **零成本重大事实**:
  运行 `cargo tree -i eventsource-stream` 与 `cargo tree -i nom` 证实：
  ```text
  eventsource-stream v0.2.3
  └── rig-core v0.42.0
      └── jftrade-assistant v0.1.0
          └── jftrade-engine v0.1.0
  ```
  `eventsource-stream = "=0.2.3"` 及其底层解析器 `nom v7.1.3` **早已被工作区的 `jftrade-assistant`（通过 `rig-core`）间接引入并严格锁定在 `Cargo.lock` 中**！
  在 `crates/jftrade-engine/Cargo.toml` 中显式添加该依赖，**引入 0 个新 crate 到项目 Lockfile 中，带来 0 额外编译开销**。
- **结论**: **【建议替换 (Replace)】— 优先级: P1** (高收益、零新增依赖的纯净重构)。

#### 3.2.3 富途 OpenD 44 字节二进制报文编解码 (`frame.rs`)
- **源码实证**:
  - 文件: `crates/jftrade-integration-futu/src/frame.rs:4-94` (全文件 113 行)
  - 关键常量: `HEADER_LEN: usize = 44`, 魔数 `"FT"`, SHA-1 校验
- **实现剖析与辩护**:
  代码仅 113 行，采用零额外开销的纯 Safe Rust 实现。精确遵循富途私有 C++/Python 报文标准：前 2 字节魔数 `"FT"`、4 字节小端 `proto_id`、1 字节 `proto_format`、1 字节 `proto_version`、4 字节 `serial_no`、4 字节包体长度、20 字节 SHA-1 哈希校验及 8 字节保留字段。crates.io 上无任何官方受维护的通用替代项。
- **结论**: **【建议保留自研 (Retain)】**。

#### 3.2.4 代际围栏行情微秒级缓存 (`TickCache`)
- **源码实证**:
  - 文件: `crates/jftrade-marketdata/src/cache.rs:13-88` (全文件 171 行)
  - 核心方法: `insert`, `lookup_for_generation`
- **实现剖析与辩护**:
  `TickCache` 专为高频行情设计，每个标的仅维护容量为 2 的双端队列（`VecDeque`）。其核心价值在于 `lookup_for_generation` 逻辑：当底层行情连接发生断开重连（代际递增为 $G+1$）时，即便旧缓存的时间戳未超时，只要代际不匹配便立刻返回 `CacheLookup::Missing`，**从根源上杜绝策略撮合使用前一代际的脏数据下单**。引入通用大缓存库（如 `moka`）不仅带来 30+ 个间接依赖与后台 GC 线程，且完全无法提供代际防穿透语义。
- **结论**: **【建议保留自研 (Retain)】**。

---

### 3.3 桌面端与系统工具 (Desktop & System Utilities)

#### 3.3.1 桌面窗口几何状态与多屏幕适配 (`WindowStateStore`)
- **源码实证**:
  - 文件: `apps/desktop/src-tauri/src/window_state.rs:1-320`
  - 关键结构: `DesktopWindowState`, `DesktopRect`, `WindowStateStore::apply` (Line 98-137), `save_state` (Line 194-228)
  - 调用与路径注入: `apps/desktop/src-tauri/src/native_lifecycle.rs:28-29` (`bootstrap.profile.window_state_path`)
- **代码实现细节与领域约束**:
  手写 320 行代码实现了桌面窗口几何位置的恢复与持久化：
  1. **多 Profile 路径隔离 (Dynamic Profile Isolation)**:
     在 `native_lifecycle.rs:28-29` 中，窗口状态通过 `WindowStateStore::load(bootstrap.profile.window_state_path.as_deref())` 加载。JFTrade 桌面端支持多运行时 Profile（如模拟演练环境、生产实盘、自动化测试与多账户隔离），窗口几何状态文件路径是由启动 Profile 动态决定的。
  2. **金融级原子写入与防掉电截断 (Crash Durability & Atomic Write)**:
     `window_state.rs:194-228` 的 `save_state` 实现了极其严密的防断电数据落盘协议：
     - 使用 `tempfile::Builder::new().prefix(".desktop-state-").suffix(".tmp").tempfile_in(directory)` 在目标目录生成隐藏临时文件；
     - 显式通过 POSIX `0o600`（`secure_file`）设定严格的私有权限；
     - 调用 `as_file().sync_all()` 强制将操作系统页缓存脏数据物理刷盘（fsync）；
     - 调用 `temporary.persist(path)` 执行文件系统原子重命名（Atomic Rename）。
     这一机制确保即便系统在写盘瞬间发生物理掉电或进程强杀，旧文件依然完好，绝对不会产生 0 字节截断或损坏文件。
  3. **多显示器断开与最小可见交集保障 (Multi-Monitor Fallback)**:
     `window_state.rs:106-130` 显式检测窗口与系统全部可用屏幕工作区是否至少存在 64x64px 交集（`VISIBLE_INTERSECTION = 64`）。一旦用户拔除外接扩展屏导致原坐标落入虚空，系统自动将窗口居中重置至主显示器（`window.primary_monitor()?`），防止窗口“永久走失”。
- **官方插件对标与缺陷分析 (`tauri-plugin-window-state`)**:
  - 官方 `tauri-plugin-window-state` 固然提供了深度的多屏 DPI 换算与系统事件监听，但其内部实现存在两个对量化系统不可接受的硬伤：
    1. **硬编码配置路径**: 插件内部写死使用 `app.path().app_config_dir().join(&plugin_state.filename)`，完全无法支持 JFTrade 所需的动态外部 Profile 路径注入，将直接破坏多环境数据隔离。
    2. **非原子写入风险**: 插件保存时仅调用简单的 `std::fs::write(state_path, ...)`。在桌面进程异常退出或掉电时，极易将配置文件写坏或截断为 0 字节，导致下次启动静默重置。
- **演进策略与混合重构方案 (Hybrid Evolution)**:
  不宜采取“一刀切”的无脑全量替换。推荐采取**混合演进策略（Hybrid Approach）**：坚决保留自研的原子落盘引擎（`tempfile` + `0o600` + `sync_all`）与动态 Profile 路径路由；仅在未来多屏管理需要更复杂的 DPI 缩放或热插拔事件感知时，局部借鉴或桥接插件的几何计算逻辑。
- **结论**: **【中立/混合演进 (Neutral / Hybrid Evolution)】— 优先级: P1**。

#### 3.3.2 桌面端日志日期字节检查 (`normalized_day`)
- **源码实证**:
  - 文件: `apps/desktop/src-tauri/src/native_logs.rs:1-34`
- **代码实现细节**:
  手写字节切片遍历校验：
  ```rust
  let valid_shape = bytes.len() == 10
      && bytes[4] == b'-'
      && bytes[7] == b'-'
      && bytes.iter().enumerate().all(|(index, byte)| matches!(index, 4 | 7) || byte.is_ascii_digit());
  ```
  在校验合法后，又手动分割字符串并调用 `time::Date::from_calendar_date`。令人意外的是，在同一文件第 4 行，已经直接调用了 `jiff::Zoned::now().date().to_string()`。
- **社区对标**:
  直接使用项目中已深度集成的 `jiff::civil::Date::strptime`，单行即可完成严格的 ISO-8601 日期解析与校验。
- **结论**: **【建议替换 (Replace)】— 优先级: P2**。

---

### 3.4 金融数学与交易日历 (Financial Mathematics & Trading Calendar)

#### 3.4.1 Go 契约对齐定点数 `Fixed8` 与交易步长扩展 `DecimalTradingExt`
- **源码实证**:
  - 文件: `crates/jftrade-kernel/src/fixed8.rs:13-31, 249-346`
  - 核心定义: `Fixed8 { non_finite: i8, scaled: i64 }`, `Fixed8::NEG_INFINITY`, `Fixed8::POS_INFINITY`, `DecimalTradingExt`
- **实现剖析与辩护**:
  1. `Fixed8` 将 8 位定点数压缩为 `i64`（内部放大 $10^8$ 倍），并通过 `non_finite` 标志位原生支持正负无穷大哨兵（`NEG_INFINITY` / `POS_INFINITY`）。这是 JFTrade 从历史 Go 系统无缝平移、且保证数据库存储与二进制序列化完全兼容的核心基石。社区 `fixed` crate 是以 2 为基数的二进制微定点数，无法精确表示十进制小数；而 `rust_decimal` 是 96 位浮点。
  2. `DecimalTradingExt` 为 `rust_decimal::Decimal` 扩展了量化交易所必需的 `truncate_to_increment`（向零截断至最小申报增量）、`ceil_to_increment`（向上进位对齐）与 `align_to_step`（四舍五入对齐步长），属于高度贴合撮合领域的专属扩展 trait。
- **结论**: **【建议保留自研 (Retain)】**。

#### 3.4.2 防御性大数展开 `DecimalText`
- **源码实证**:
  - 文件: `crates/jftrade-kernel/src/decimal.rs:8-35` (`const MAX_EXPANDED_DIGITS: i64 = 1_000_000;`, `pub struct DecimalText(String);`)
- **实现剖析与辩护**:
  外部券商接口返回的价格经常掺杂科学计数法（如 `1e-8`）甚至非规范的 Null/String 混杂格式。`DecimalText` 实现了上限 1,000,000 位的科学计数法防御性展开，并提供兼容 String、Number 与 Null 自动转 `"0"` 的容错反序列化能力，有效防御了上游行情脏数据造成的程序 Panic。
- **结论**: **【建议保留自研 (Retain)】**。

#### 3.4.3 技术指标与 JavaScript 浮点精度模拟 (`js_math_round`)
- **源码实证**:
  - 文件: `crates/jftrade-backtest/src/indicators.rs:197-228`
  - 核心函数: `pine_precision`, `js_math_round`
- **源码注释与实现揭示**:
  ```rust
  // crates/jftrade-backtest/src/indicators.rs:199-204
  /// Matches PineTS's `Math.round(value * 1e10) / 1e10` operation.
  ///
  /// JavaScript rounds halfway cases toward positive infinity, unlike Rust's
  /// `f64::round`, which rounds halfway cases away from zero. The explicit
  /// negative-zero branch also preserves `Math.round(-0.5) === -0`.
  fn js_math_round(value: f64) -> f64 {
      if value.is_nan() || value.is_infinite() || value == 0.0 {
          return value;
      }
      let lower = value.floor();
      let rounded = if value - lower >= 0.5 { lower + 1.0 } else { lower };
      if rounded == 0.0 && value.is_sign_negative() { -0.0 } else { rounded }
  }
  ```
- **核心辩护**:
  JFTrade 策略引擎的黄金基准是运行在 Node.js V8 引擎中的 PineTS。JavaScript 的 `Math.round` 对于 `0.5` 临界点是向正无穷大（Positive Infinity）舍入，而 Rust 原生 `f64::round` 是向远离零方向舍入；此外，JavaScript 还具有特殊的 `-0.0` 符号语义。自研的 `js_math_round` 是确保 Rust 回测系统与 Node.js 策略端在 10 位小数精度下**绝对吻合（Bit-level Parity）**的生命线。任何社区 TA crate（如 `ta`, `technical-indicators`）均无法提供此一致性保证。
- **结论**: **【建议保留自研 (Retain)】**。

#### 3.4.4 多市场交易时段划分与仲裁日历 (`jftrade-calendar`)
- **源码实证**:
  - 文件: `crates/jftrade-calendar/src/manager_policy.rs:87-129, 298-321, 347-411`
  - 核心定义: `builtin_schedule`, `easter_sunday`, `mainland_holiday_reason`, `market_day_start`
- **实现剖析与辩护**:
  1. `jftrade-calendar` 原生支持美股 4 个交易时段（Overnight 00:00-04:00, Pre 04:00-09:30, Regular 09:30-16:00, After 16:00-20:00）及半天提前休市；同时支持港股与 A 股带午间休市（11:30-13:00）的复杂切片模型。
  2. 内置 24 行 Spencer / Meeus 复活节高斯代数算法，以及 2025-2027 年国务院法定节假日调休字典。
  3. 通过 `CalendarSourcePort` 实现了 Builtin 规则、用户控制台手动覆盖与外部行情源的动态多源仲裁。社区没有任何开源 Rust 库能同时覆盖美/港/A股的跨时区混合交易日历。
- **结论**: **【建议保留自研 (Retain)】**。

#### 3.4.5 回测 FNV-1a 结果指纹与微工具零依赖标准
- **源码实证**:
  - 文件: `crates/jftrade-backtest/src/fingerprint.rs:4-14` (全文件 15 行)
- **分析与微工具标准对齐**:
  代码仅 10 行纯代数运算，计算 64 位 FNV-1a 哈希。社区虽有 `fnv` crate，但引入外部依赖替换 10 行安全代数代码并无实际架构价值。
  这一裁决与 `crates/jftrade-engine/src/product_auth_session_crypto.rs:28-37` 的 `encode_hex`（9 行查表）完全保持一致：对于 10 行以内的纯 Safe Rust 简单代数/查表微工具，坚决避免引入 `fnv` 或 `hex` 等未使用的外部依赖，统一践行“零外部依赖、自包含高可靠”的精炼工程哲学。
- **结论**: **【建议保留自研 (Retain)】**。

---

### 3.5 存储、Schema 校验与专用语言编译引擎 (Storage, Schema & Language Engines)

#### 3.5.1 SQLite Schema Manifest 强一致性防御引擎
- **源码实证**:
  - 文件: `crates/jftrade-store-sqlite/src/schema_manifest.rs:162-258`
  - 核心符号: `validate_current`, `inspect_schema`, `SCHEMA_DEFINITIONS`, `KLINE_PATTERN`
- **实现剖析与辩护**:
  系统启动时通过 `PRAGMA table_list`、`table_info`、`index_list`、`foreign_key_list` 深度内省 SQLite 实盘数据库，与编译期内嵌的 JSON Schema 进行字段级精确比对；尤其支持匹配正则 `local_klines__*` 的动态 K 线分表结构原型比对。一旦发现任何表结构偏差、未申报列或索引损坏，**立即拒绝启动 (Fail-Closed)**。社区迁移工具（如 `refinery`, `diesel_migrations`）仅仅维护一条单向递增的版本号表，根本无法阻断用户使用外部 SQLite 工具手动修改库表引发的静默数据污染。
- **结论**: **【建议保留自研 (Retain)】**。

#### 3.5.2 单写属主原子配置存储 (`jftrade-store-settings-file`)
- **源码实证**:
  - 文件: `crates/jftrade-store-settings-file/src/lib.rs:665-705`
  - 核心逻辑: `WriterLease::acquire`, `tempfile::Builder`, `harden_file(0o600)`, `harden_directory(0o700)`, 双重 `sync_all`
- **实现剖析与辩护**:
  该模块并非静态只读配置读取器（如 `config-rs`, `figment`），而是承载用户在前端界面动态写回的权威持久化引擎。它集成了 `WriterLease` 跨进程排他锁、`tempfile` 临时文件原子替换、POSIX 权限加固以及针对数据文件和父目录的双重 `fsync` 刷盘，构成了金融级的防掉电损坏屏障。
- **结论**: **【建议保留自研 (Retain)】**。

#### 3.5.3 原生 Pine Script v6 编译分析前端
- **源码实证**:
  - 目录: `crates/jftrade-strategy/src/pine/`
  - 文件与规模: `lexer.rs` (271行), `parser.rs` (1,066行), `semantic.rs` (559行), `lower.rs` (325行), `planner.rs` (590行)，总计 **2,811 行代码**。
- **实现剖析与辩护**:
  1. **填补生态空白**: TradingView 的 Pine Script 属于商业私有 DSL，crates.io 社区**不存在任何现成受维护的 Pine v6 解析器 crate**。
  2. **毫秒级离线静态分析**: 系统在用户输入策略或执行 MCP 工具调用时，必须在毫秒级提取指标依赖周期（`planner.rs`）并校验语法合法性。自研递归下降解析器提供了零 Node.js 进程开销的纯 Rust 极速语法校验。
- **结论**: **【建议保留自研 (Retain)】**。

#### 3.5.4 六边形 API 网关与集中路由派发
- **源码实证**:
  - 文件: `crates/jftrade-api/src/router.rs:118-120` (`.fallback(dispatch)`)
- **实现剖析与辩护**:
  Axum 路由树仅注册单一 WebSocket 终结点，全量 277 条 HTTP 路由统一汇入 `.fallback(dispatch)`。这严格遵守了 `AGENTS.md` 规定的端口隔离原则——`jftrade-api` 作为接口外壳，严禁直接绑定具体数据库连接或领域运行时，必须通过统一的六边形网关分发器进行 Envelope 信封打包与权限鉴权。
- **结论**: **【建议保留自研 (Retain)】**。

---

### 3.6 AI 助手与大模型驱动接入 (AI Assistant & LLM Drivers)

#### 3.6.1 双轨模型调用现状与收敛分析
- **源码实证**:
  - 适配器: `crates/jftrade-assistant/src/rig_adapter.rs:1-245` (声明依赖 `rig-core = "=0.42.0"`)
  - 运行时调用: `crates/jftrade-engine/src/product_adk_model_runtime_adapters.rs:531-541` (使用 `reqwest` 手工拼接 `/responses`)
- **分析与评估**:
  `jftrade-assistant` 构建了映射至 `rig_core::completion` 的投射层，但在 `jftrade-engine` 的实际生产运行时中，代码直接采用 `reqwest` 手工构造针对 OpenAI `/v1/responses` 的 HTTP 请求。这形成了“声明了通用 SDK 抽象、底层却手工拼接 HTTP 客户端”的双轨脱节。
  鉴于 OpenAI 的 Responses API 仍处于快速演变期，过早强行绑定某一方 SDK 均可能受限其特异字段支持。建议在 P2 阶段持续跟进社区生态，待规范稳定后再行统一。
- **结论**: **【中立/视后续演进而定 (Neutral/Evolve)】— 优先级: P2**。

---

## 4. 全景对比与决断矩阵 (Full-Spectrum Comparison & Trade-Off Matrices)

下表对审计排查发现的全部 16 项自研功能模块与社区方案进行了全方位量化评估：

| 领域 / 功能模块 | 源码路径与定位符号 | 社区主流对标 Crate (版本 / 维护者) | 最终评估决策 | 功能覆盖度与 API 人机工效 | 预计代码行数变化 (LOC Delta) | 依赖树体积与编译耗时影响 | AGPL-3.0 许可证合规性 | `#![forbid(unsafe_code)]` 安全合规性 |
|---|---|---|---|---|---:|---|---|---|
| **恒定时间比较与查表十六进制** | `jftrade-engine`: `product_auth_session_crypto.rs:15` (`constant_time_eq`) & `28` (`encode_hex`) | `subtle` (=2.6.1)<br>dalek-cryptography | **【建议替换 (P0)】(时序比对)**<br>**【建议保留】(十六进制)** | `subtle` 杜绝 LLVM 编译短路优化；`encode_hex` 为 9 行纯安全查表，保留自研零外部依赖 | `constant_time_eq` 净减 ~15 行；`encode_hex` 0 行 | **0 新增依赖 (subtle 已在 Cargo.lock)**<br>0 秒编译开销 | 兼容 (Apache-2.0 / BSD-3) | 100% 兼容 (pure safe) |
| **登录与券商主动限流** | `jftrade-engine`: `product_auth_session_manager.rs:52` & `product_trade_margin_route.rs:106` | `governor` (=0.8.1 或最新 =0.10.4)<br>antifuchs | **【建议替换】(P1)** | GCRA 算法提供 Keyed 限流与券商（富途 OpenD）主动频控，消除被动报错 | 净减 ~80 行 | 引入 ~8~10 个传递依赖 (`parking_lot`, `portable-atomic` 等)<br>增加 ~2.5s 编译耗时 | 兼容 (MIT) | 需注意底层传递依赖含 unsafe；std 下 Keyed 默认基于 Mutex-HashMap 同步 |
| **ADK 异步任务与回收** | `jftrade-engine`: `product_adk_model_runtime.rs:133` (`ContinuationSupervisor`) | `tokio::task::JoinSet`<br>Tokio 官方 | **【建议替换】(P1)** | 原生协程生命周期驱动，消灭独立 OS Reaper 线程与手动 Join | 净减 ~110 行 | **0 新增依赖 (已在 Cargo.lock)**<br>0 秒编译开销 | 兼容 (MIT) | 100% 兼容 |
| **桌面窗口状态恢复与持久化** | `apps/desktop/src-tauri`: `src/window_state.rs:1-320` (`WindowStateStore`) | `tauri-plugin-window-state` (=2.x)<br>Tauri Core Team | **【中立/混合演进】(P1)** | 官方插件硬编码配置路径且使用非原子 `fs::write`；自研实现独有动态 Profile 路径路由与 `tempfile` 原子防截断持久化。建议保持原子落盘，按需借鉴插件多屏几何逻辑 | 净变动 ~0 行 (架构解耦) | 暂不引入或按需局部引入<br>0 秒额外编译开销 | 兼容 (MIT / Apache) | 100% 兼容 (受审官方库) |
| **ADK SSE 流分帧解析** | `jftrade-engine`: `product_adk_model_stream.rs:237` (`SseDecoder`) | `eventsource-stream` (=0.2.3)<br>jpopesculian | **【建议替换】(P1)** | 标准 Stream 扩展方法，杜绝跨分片与畸形字符分割 bug | 净减 ~70 行 | **0 新增依赖 (已由 rig-core 间接引入并锁定在 Cargo.lock，含 nom v7.1.3)**<br>0 秒编译开销 | 兼容 (MIT / Apache) | 100% 兼容 |
| **桌面日志日期校验** | `apps/desktop/src-tauri`: `src/native_logs.rs:1` (`normalized_day`) | `jiff::civil::Date`<br>BurntSushi | **【建议替换】(P2)** | 原生 `strptime` 单行解析，消灭手动字节切片比对 | 净减 ~30 行 | **0 新增依赖 (已深度使用 jiff)**<br>0 秒编译开销 | 兼容 (MIT / Unlicense) | 100% 兼容 |
| **策略多 Runtime 隔离** | `jftrade-engine`: `strategy_runtime.rs:326` (`Runtime::new`) | `tokio::runtime::Builder` (current_thread) | **【中立/演进】(P1)** | 消除策略实例暴增造成的系统底层调度碎片化 | 净减 ~20 行 | 0 新增依赖 | 兼容 (MIT) | 100% 兼容 |
| **AI 模型驱动双轨收敛** | `jftrade-assistant`: `rig_adapter.rs` vs `engine/.../reqwest` | `rig-core` 或 `async-openai` | **【中立/演进】(P2)** | 待 OpenAI Responses API 规范稳定后再评估统一接入 | 净减 ~150 行 | 待定 | 兼容 (MIT) | 100% 兼容 |
| **跨进程排他租约锁** | `jftrade-owner-lock`: `lib.rs:73` (`WriterLease`) | `fd-lock` / `fslock` | **【建议保留自研】** | 基于 Rust 1.97+ stdlib `File::try_lock`，带 PID 诊断元数据 | 0 行 | 避免引入含 unsafe 的 C-FFI 库 | 兼容 (AGPL-3.0) | **100% Safe (社区库含 unsafe)** |
| **定点数与交易步长扩展** | `jftrade-kernel`: `fixed8.rs:13` (`Fixed8`, `DecimalTradingExt`) | `fixed` / `rust_decimal` | **【建议保留自研】** | 严格对齐 Go 历史协议，封装量化步长与申报增量扩展 | 0 行 | 避免引入不适用的二进制微定点库 | 兼容 (AGPL-3.0) | 100% 兼容 |
| **防御性大数展开** | `jftrade-kernel`: `decimal.rs:10` (`DecimalText`) | `bigdecimal` | **【建议保留自研】** | 防御性展开 100 万位科学计数法，支持 untagged 容错反序列化 | 0 行 | 避免引入重型大数依赖 | 兼容 (AGPL-3.0) | 100% 兼容 |
| **JS 浮点舍入精度模拟** | `jftrade-backtest`: `indicators.rs:212` (`js_math_round`) | `ta` / `technical-indicators` | **【建议保留自研】** | 模拟 V8 `Math.round` 正无穷舍入与 `-0.0`，保证与 PineTS 100% 对齐 | 0 行 | 社区库缺乏 JS 特有浮点舍入语义 | 兼容 (AGPL-3.0) | 100% 兼容 |
| **多时段多源仲裁日历** | `jftrade-calendar`: `manager_policy.rs:87` (`builtin_schedule`) | `trading-calendars` | **【建议保留自研】** | 支持美/港/A股四时段与午休，内置调休字典与动态仲裁 | 0 行 | 社区无支持中国/香港午间休市的 Rust 库 | 兼容 (AGPL-3.0) | 100% 兼容 |
| **回测结果 FNV-1a 指纹** | `jftrade-backtest`: `fingerprint.rs:4` (`populate_result_hash`) | `fnv` | **【建议保留自研】** | 10 行简单代数运算，对齐微工具零外部依赖哲学，无需引入外部包 | 0 行 | 节约依赖树复杂度，保持零新增依赖 | 兼容 (AGPL-3.0) | 100% 兼容 |
| **SQLite PRAGMA Manifest** | `jftrade-store-sqlite`: `schema_manifest.rs:162` (`validate_current`) | `refinery` / `sqlx-migrate` | **【建议保留自研】** | PRAGMA 全量模式内省与动态分表原型校验，Fail-Closed 防坏库 | 0 行 | 社区库无法提供全量 Schema 校验能力 | 兼容 (AGPL-3.0) | 100% 兼容 |
| **单写原子配置加固** | `jftrade-store-settings-file`: `lib.rs:665` (`persist_document`) | `config-rs` / `figment` | **【建议保留自研】** | 整合 `WriterLease`、POSIX 0600 权限加固与双重 `fsync` 刷盘 | 0 行 | 社区库仅支持启动只读合并，不支持权威写回 | 兼容 (AGPL-3.0) | 100% 兼容 |
| **Pine Script v6 编译前端** | `jftrade-strategy`: `src/pine/*` (2,811 行全套编译器) | *无任何可用开源 Crate* | **【建议保留自研】** | 零 Node 开销的纯 Rust 离线词法、递归下降语法分析与指标提取 | 0 行 | 填补 Rust 生态空白，不可替代 | 兼容 (AGPL-3.0) | 100% 兼容 |
| **富途 OpenD 44B 帧编解码** | `jftrade-integration-futu`: `frame.rs:37` (`encode_frame`) | *无官方/通用对标* | **【建议保留自研】** | 113 行纯 Safe 代码精确解析富途私有报文，零多余抽象 | 0 行 | 紧凑高效，绝无外包风险 | 兼容 (AGPL-3.0) | 100% 兼容 |

---

## 5. 保留自研的核心业务辩护 (Deep-Dive Rationale for Custom Code Retention)

### 5.1 PineTS V8 影子对齐与技术指标状态机
在量化交易系统构建中，回测结果与实盘信号的“绝对一致性”（Determinism）是系统的生命线。
- **IEEE 754 舍入陷阱**: JFTrade 的生产策略脚本由 Node.js 运行环境（PineTS）解释执行。JavaScript 标准的 `Math.round(x)` 在处理正负半数（halfway cases，如 `0.5`, `-0.5`）时向正无穷方向进位（`Math.round(-0.5) === -0`）；而 Rust 原生 `f64::round` 则是向远离零方向进位（`-0.5_f64.round() == -1.0`）。`crates/jftrade-backtest/src/indicators.rs:212` 中的 `js_math_round` 是经过严格数学建模的模拟实现，确保在 10 位有效数字下与 Node.js 产生完全一致的中间值。
- **状态保留与输出截断分离**: 在 `pine_compatible_ema` 等指标计算中，系统内部迭代保留了未经精度衰减的原始浮点状态，仅在最终交付给图表和撮合逻辑时应用 `pine_precision`。社区中的任何通用 TA 库（如 `ta`）均无法提供此类针对特定脚本宿主定制的双轨状态机。

### 5.2 单写属主租约与行情代际围栏防护
- **零 Unsafe 的标准库文件锁**: `crates/jftrade-owner-lock` 摒弃了传统第三方库调用 libc `flock` 的 FFI 模式，完全基于 Rust 1.97+ 标准库内置的 `File::try_lock()`，实现了 100% 纯 Safe Rust 编译。更重要的是，它在加锁成功后原子写入 `OwnerDiagnostic` 载荷（记录当前进程 PID 与 profile），在争用失败时能明确告知操作者“被 PID 为 12345 的进程锁定”，具备无可替代的生产可观测性。
- **代际失效（Generation Fencing）防穿透**: `crates/jftrade-marketdata/src/cache.rs` 中的 `TickCache` 绝非普通键值缓存。在网络抖动导致券商网关重连时，行情的 `provider_generation` 递增；`lookup_for_generation` 强行阻断旧代际数据的复用。即便旧数据在物理时间上仅过去了 10 毫秒，只要代际不匹配立即判定为 `Missing`，杜绝了策略在连接震荡期基于陈旧报价产生非预期下单。

### 5.3 SQLite Fail-Closed PRAGMA 校验与动态分表安全
JFTrade 核心引擎持有 9 个权威 SQLite 数据库文件。
- **防范 Schema Drift 的终极屏障**: 社区传统的数据库迁移工具（如 `refinery` 或 `sqlx-migrate`）仅在数据库中记录一条递增的版本号整数，一旦外部用户使用 Navicat 或 SQLiteStudio 等第三方工具误删列、改动约束或更改默认值，传统迁移工具完全失灵。
- **全量 PRAGMA 内省**: `crates/jftrade-store-sqlite/src/schema_manifest.rs` 在系统启动阶段调用 `PRAGMA table_list`、`table_info`、`index_list`、`foreign_key_list` 深度遍历每个表结构的每一列、每一个主外键索引，并比对内嵌的元数据规范。更特别的是，它能动态识别匹配 `local_klines__*` 模式的动态 K 线分表并基于原型表进行校验。任何微小的不一致均会导致启动熔断（Fail-Closed），这是防止静默数据损坏的最高安全防线。

### 5.4 券商私有二进制协议的高内聚与零抽象损耗
- **报文协议专有性**: 富途 OpenD 的 44 字节二进制报文头（包含 2 字节 `"FT"` 魔数、小端序列号、协议 ID 及 20 字节 SHA-1 哈希）是该商业软件专有的私有通信协议。
- **纯 Safe 实现**: `crates/jftrade-integration-futu/src/frame.rs` 仅用 113 行代码即完成了极其健壮的打包与拆包校验，不含任何内存不安全操作，内存零额外拷贝分配。引入第三方长尾非标库没有任何技术收益，反而会埋下不可控的安全风险。

### 5.5 多市场复杂交易时段与法定节假日仲裁
- **多时段切片模型**: `crates/jftrade-calendar` 原生支持美股四时段（夜盘、盘前、常规、盘后）与提前休市，同时支持港股与 A 股带午间休市（11:30–13:00）的复杂切片。
- **动态仲裁机制**: 考虑到中国国务院每年的法定节假日调休方案并不存在固定的数学公式，系统设计了 `CalendarSourcePort` 仲裁管道：内置 2025–2027 年法定节假日字典保障离线可用，同时支持控制台手动下发覆盖规则（`ManualOverride`），使系统在无需重新发版编译的情况下动态适应节假日调整。

### 5.6 桌面多 Profile 隔离与断电强安全持久化 (`window_state.rs`)
在桌面量化终端的生命周期管理中，窗口几何状态的保存不仅关乎用户交互体验，更直接牵涉到运行态数据隔离与物理断电下的文件系统完整性。
- **多 Profile 路径隔离 (Dynamic Profile Isolation)**:
  `apps/desktop/src-tauri/src/native_lifecycle.rs:28-29` 明确将窗口状态存储路径交由运行时参数驱动：
  ```rust
  let window_state = Arc::new(WindowStateStore::load(
      bootstrap.profile.window_state_path.as_deref(),
  ));
  ```
  JFTrade 支持以不同的 Profile 启动桌面客户端（例如模拟测试环境、实盘交易环境与多账户沙盒），每个 Profile 拥有完全独立的持久化目录与状态文件。社区官方插件 `tauri-plugin-window-state` 内部硬编码为 `app.path().app_config_dir().join(&plugin_state.filename)`，缺乏动态路径注入扩展点。若盲目引入官方插件，多 Profile 间的数据隔离将被彻底打破，产生状态污染。
- **金融级原子写入与抗掉电截断 (Crash Durability)**:
  `apps/desktop/src-tauri/src/window_state.rs:194-228` 的持久化落盘实现了极高的防御水准：
  1. 使用 `tempfile::Builder` 在状态文件同级目录下生成命名隐蔽的临时文件；
  2. 显式调用 POSIX `0o600` 权限（`secure_file`）防止本地其他低权限进程窥探；
  3. 通过 `as_file().sync_all()` 强刷脏页至物理磁盘（fsync），保证写入已真正到达物理存储介质；
  4. 最终调用 `temporary.persist(path)` 执行原子重命名（Atomic Rename）。
  这意味着在系统遭遇异常断电、外设拔脱或进程被 `kill -9` 强行终止时，文件系统只可能存在“写入前的完整旧文件”或“写入后的完整新文件”，绝对不会出现 0 字节截断或 JSON 半截损坏的非一致状态。相比之下，官方插件仅调用非原子的 `std::fs::write`，在相同崩溃故障下存在极高的文件损坏风险。
- **混合演进决断**:
  保留现有具备原子落盘保障与动态 Profile 路径注入的持久化核心，仅在未来需要更高级的跨屏幕热插拔感知与系统级 DPI 换算时，局部引入或借鉴官方插件的事件层，此为最优的混合演进方案。

### 5.7 纯安全微算法与轻量工具的零依赖哲学 (Zero-Dependency Micro-Utilities)
在架构审计中，必须确立清晰、统一的微工具（Micro-Utilities）引入标准，杜绝为极小代码量引入庞大外部依赖树的“依赖膨胀病”：
- **微代码的双重案例**:
  - `crates/jftrade-backtest/src/fingerprint.rs:4-14`: 仅用 10 行简单 Safe 代数实现标准 64 位 FNV-1a 哈希，用于回测结果的轻量校验。
  - `crates/jftrade-engine/src/product_auth_session_crypto.rs:28-37`: 仅用 9 行纯 Safe Rust 实现十六进制字符查表（`encode_hex`），用于会话 Token 的 Hex 序列化。
- **统一的自研保留标准**:
  上述两处实现均具备“行数不足 10 行、纯 Safe Rust 实现、无外部网络/系统交互、无复杂算法分支、无密码学时序攻击漏洞”的共性。当前工作区中均未引入外部 `fnv` 与 `hex` crate。若为 9~10 行确定性基础代数引入新的第三方 crate，不仅无法带来任何功能或安全性提升，反而会增加 `Cargo.lock` 解析负担、依赖审核成本与供应链攻击面。因此，系统坚决将此类纯安全微工具划入【建议保留自研】范畴。

---

## 6. 分级演进路线图 (Prioritized Migration Roadmap: P0 / P1 / P2)

综合考量安全性收益、系统稳定性、开发投入比与回归测试风险，制定以下结构化分级演进路线图：

### P0 紧急安全修复 (Immediate Security Fix)

#### 任务 P0-1: 替换手写恒定时间比较为 `subtle::ConstantTimeEq`
- **目标文件**: `crates/jftrade-engine/src/product_auth_session_crypto.rs:15-26`
- **现状风险**: 手写循环缺乏内存黑盒阻断，Release 模式 LTO 编译下存在被 LLVM 优化为提前退出分支的风险，破坏 CSRF/会话校验的恒定时间特性。
- **改造方案**:
  1. 在 `crates/jftrade-engine/Cargo.toml` 中声明 `subtle.workspace = true`（工作区已由 `rustls`/`argon2` 锁定 `subtle = "=2.6.1"`）。
  2. 改造 `constant_time_eq`:
     ```rust
     use subtle::ConstantTimeEq;

     pub(super) fn constant_time_eq(a: &str, b: &str) -> bool {
         a.as_bytes().ct_eq(b.as_bytes()).into()
     }
     ```
- **预期收益**: 消除侧信道时序漏洞；**0 个新增外部依赖，0 额外编译开销**。
- **验证命令**: `cargo test -p jftrade-engine --lib product_auth_session`

---

### P1 架构与稳定性治理 (Architecture, Performance & Reliability)

#### 任务 P1-1: 引入 `governor` 统一内外限流（登录防爆破与券商主动频控）
- **目标文件**:
  - `crates/jftrade-engine/src/product_auth_session_manager.rs:52, 129-165`
  - `crates/jftrade-engine/src/product_trade_margin_route.rs:106-113`
- **现状风险**: Web 登录使用加锁 `BTreeMap` 并发修剪淘汰；富途保证金接口无前端保护，任由请求打崩券商接口后靠匹配中文 `"频率太高"` 错误字符串被动降级，存在被券商封禁风险。
- **改造方案**:
  1. 在根 `Cargo.toml` 引入 `governor = { version = "=0.8.1", default-features = false, features = ["std"] }`。
  2. 如实评估依赖树：该配置引入 `nonzero_ext`、`portable-atomic`、`parking_lot` 等 ~8~10 个传递依赖（含底层 unsafe 并发原语）；在 std 模式下其 Keyed 模式默认基于 `parking_lot::Mutex<HashMap<...>>` 互斥同步。
  3. 会话管理使用 `governor::RateLimiter::keyed` 实现纳秒级 GCRA 限流，彻底删除手写加锁淘汰代码 ~80 行。
  4. 在 `jftrade-integration-futu` 注入券商限频令牌桶（针对 `Trd_GetMarginRatio` 设置 30 秒 10 次配额），前置主动拦截并排队，根除被动报错。
- **预期收益**: 减除 80 行繁重代码，杜绝券商接口超频封号风险。

#### 任务 P1-2: 采用 `tokio::task::JoinSet` 消除 ADK 续跑专用 OS Reaper 线程
- **目标文件**: `crates/jftrade-engine/src/product_adk_model_runtime.rs:133-186`
- **现状风险**: 手写 `ContinuationSupervisor` 为每个续跑任务分配独立 OS 线程，并额外常驻一个 Dedicated Reaper 线程监听 channel 执行阻塞 `handle.join()`，耗费系统栈内存。
- **改造方案**:
  1. 使用 Tokio 原生 `JoinSet` 管理续跑任务：
     ```rust
     let mut join_set = tokio::task::JoinSet::new();
     join_set.spawn(async move { ... });
     ```
  2. 配合 `tokio_util::sync::CancellationToken` 实现级联取消。
- **预期收益**: 彻底删除专用 Reaper 线程与 OS 线程手动 Join 机制，消除栈内存膨胀，精简代码 ~110 行。

#### 任务 P1-3: 混合演进桌面窗口状态恢复 (`window_state.rs` 架构解耦与多屏事件借鉴)
- **目标文件**: `apps/desktop/src-tauri/src/window_state.rs:1-320` 与 `apps/desktop/src-tauri/src/native_lifecycle.rs:28-29`
- **现状评估与架构边界**:
  `apps/desktop/src-tauri/src/window_state.rs` 承担着多 Profile 动态路径注入（`bootstrap.profile.window_state_path`）以及基于 `tempfile` 原子替换 + POSIX `0o600` + `sync_all` 强刷盘的物理掉电防截断屏障。
  官方 `tauri-plugin-window-state` 硬编码为 `app_config_dir` 且仅执行普通 `std::fs::write`。直接全量平替将破坏多 Profile 数据隔离并降低掉电持久化安全性，不可取。
- **改造方案 (混合演进)**:
  1. **保持核心自研持久化**: 坚决保留现有自研的原子落盘引擎与动态 Profile 路径路由逻辑，保障金融级掉电抗损坏。
  2. **局部借鉴插件能力**: 评估引入 `tauri-plugin-window-state` 的事件监听能力，或仅抽取其跨平台多显示器热插拔事件感知与复杂 DPI 缩放几何算法，增强现有的 `VISIBLE_INTERSECTION` 居中兜底逻辑。
- **预期收益**: 在 100% 保持多 Profile 隔离与物理掉电零截断安全性的前提下，增强跨屏幕多显示器热插拔的窗口恢复鲁棒性。

#### 任务 P1-4: 引入 `eventsource-stream` 替代手写 ADK SSE 流解码器
- **目标文件**: `crates/jftrade-engine/src/product_adk_model_stream.rs:237-298`
- **现状风险**: 手写字符串缓存查找 `\r\n\r\n` 或 `\n\n`，容易在网络异常切片时出现跨边界解析故障。
- **改造方案**:
  1. 在 `crates/jftrade-engine/Cargo.toml` 声明引入 `eventsource-stream = "=0.2.3"`。
     （**零成本重大事实**: `eventsource-stream = "=0.2.3"` 与底层解析器 `nom v7.1.3` **已由 `rig-core` 锁定在工作区 `Cargo.lock` 中**，引入到 engine 带来 **0 个新外部依赖，0 额外编译开销**！）
  2. 对 `reqwest::Response::bytes_stream()` 直接调用 `.eventsource()` 解码，流式处理 JSON payload。
- **预期收益**: 净减 ~70 行手写解析逻辑，杜绝网络异常分片故障，0 依赖成本享受工业级解析器。

---

### P2 生态收敛与体验优化 (Cleanliness & Ecosystem Convergence)

#### 任务 P2-1: 桌面日志日期与级别解析全面切换至 `jiff`
- **目标文件**: `apps/desktop/src-tauri/src/native_logs.rs:1-34`
- **改造方案**: 移除手写字节比对，直接调用 `jiff::civil::Date::strptime("%Y-%m-%d", value)`。
- **预期收益**: 代码精简 ~30 行，统一全工作区的时间解析风格。

#### 任务 P2-2: 策略多实例运行时收敛为单一 Runtime 调度
- **目标文件**: `crates/jftrade-engine/src/strategy_runtime.rs:323-338`
- **改造方案**: 避免每个策略实例初始化完整的多线程 Tokio Runtime，将其收拢到 Engine 的 Ambient Tokio Runtime 协程调度中，或降级为单线程 `current_thread` runtime。
- **预期收益**: 彻底杜绝多策略并发时的底层 OS 线程暴增。

#### 任务 P2-3: 统一 AI 助手端 rig-core 与引擎原生 reqwest 双轨架构
- **目标文件**: `crates/jftrade-assistant/src/rig_adapter.rs` 与 `crates/jftrade-engine/src/product_adk_model_runtime_adapters.rs`
- **改造方案**: 跟踪 OpenAI Responses API 规范演进，适时将请求流统一为单一通道，消除闲置的抽象投影层。

#### 任务 P2-4: 保持十六进制编码自研并对齐微工具零依赖标准 (`encode_hex`)
- **目标文件**: `crates/jftrade-engine/src/product_auth_session_crypto.rs:28-37`
- **方案分析与标准对齐**:
  `encode_hex` 仅为 9 行纯 Safe Rust 静态常量查表代码，属于无时序敏感性与无外部风险的极简代数转换。当前工作区完全未引入 `hex` crate。根据第 3.4.5 节与第 5.7 节确立的微工具零依赖准则，坚决不对 10 行以内的纯安全微工具引入外部依赖。
- **决断**: 保持自研不变，取消引入外部 `hex` crate 的建议，杜绝长尾依赖膨胀。

---

## 7. 独立复现与验证指南 (Independent Verification Guide)

为保证审计结果与代码引用的 100% 真实可信，任何架构评审员均可通过以下标准命令对本报告的每一项数据与结论进行独立查验：

### 7.1 工作区 Member Crates 与代码行数复现
```bash
# 1. 验证工作区 21 个 Member Crate 清单
cargo metadata --no-deps --format-version 1 | jq -r '.packages[] | "\(.name)	\(.manifest_path)"'

# 2. 统计各 Crate 生产源码与测试源码精确行数
python3 -c '
import os
crates = [
    ("jftrade-desktop", "apps/desktop/src-tauri"),
    ("jftrade-engine", "crates/jftrade-engine"),
    ("jftrade-api", "crates/jftrade-api"),
    ("jftrade-assistant", "crates/jftrade-assistant"),
    ("jftrade-kernel", "crates/jftrade-kernel"),
    ("jftrade-calendar", "crates/jftrade-calendar"),
    ("jftrade-datamanagement", "crates/jftrade-datamanagement"),
    ("jftrade-integration-futu", "crates/jftrade-integration-futu"),
    ("jftrade-broker", "crates/jftrade-broker"),
    ("jftrade-marketdata", "crates/jftrade-marketdata"),
    ("jftrade-trading", "crates/jftrade-trading"),
    ("jftrade-integration-marketdata-helper", "crates/jftrade-integration-marketdata-helper"),
    ("jftrade-integration-pine", "crates/jftrade-integration-pine"),
    ("jftrade-backtest", "crates/jftrade-backtest"),
    ("jftrade-owner-lock", "crates/jftrade-owner-lock"),
    ("jftrade-research", "crates/jftrade-research"),
    ("jftrade-settings", "crates/jftrade-settings"),
    ("jftrade-store-settings-file", "crates/jftrade-store-settings-file"),
    ("jftrade-store-sqlite", "crates/jftrade-store-sqlite"),
    ("jftrade-strategy", "crates/jftrade-strategy"),
    ("jftrade-watchlist", "crates/jftrade-watchlist"),
]
for name, path in crates:
    src, test = 0, 0
    for root, _, files in os.walk(path):
        for f in files:
            if f.endswith(".rs"):
                fp = os.path.join(root, f)
                c = sum(1 for _ in open(fp, "r", encoding="utf-8", errors="ignore"))
                if "/tests/" in fp or "/benches/" in fp or fp.endswith("_test.rs") or fp.endswith("_tests.rs"):
                    test += c
                else:
                    src += c
    print(f"{name:38} | src: {src:6} | test: {test:6} | total: {src+test:6}")
'
```

### 7.2 依赖拓扑与许可证、禁令复查
```bash
# 1. 验证 subtle v2.6.1 已经存在于当前依赖树中（0 额外依赖）
cargo tree -i subtle

# 2. 验证 eventsource-stream 与 nom 已存在于当前依赖树中（0 额外依赖）
cargo tree -i eventsource-stream
cargo tree -i nom

# 3. 验证 governor 真实版本序列（证实 0.8.2 不存在，推荐锁定 0.8.1 或评估最新 0.10.4）
python3 -c 'import urllib.request, json; data = json.loads(urllib.request.urlopen("https://crates.io/api/v1/crates/governor").read()); print([v["num"] for v in data["versions"]][:10])'

# 4. 验证许可证与开源合规性 (AGPL-3.0-only 白名单)
cargo deny check licenses

# 5. 验证多版本与供应链禁令
cargo deny check bans
```

### 7.3 关键自研符号定位与行号核验
```bash
# 1. 核验 P0 项：constant_time_eq (第 15-26 行)
sed -n '15,26p' crates/jftrade-engine/src/product_auth_session_crypto.rs

# 2. 核验 jftrade-owner-lock 零 unsafe 与 stdlib File::try_lock
head -n 1 crates/jftrade-owner-lock/src/lib.rs
sed -n '103,115p' crates/jftrade-owner-lock/src/lib.rs

# 3. 核验 ContinuationSupervisor 与 Reaper 线程
sed -n '133,178p' crates/jftrade-engine/src/product_adk_model_runtime.rs

# 4. 核验策略独立创建 Tokio Runtime
sed -n '323,338p' crates/jftrade-engine/src/strategy_runtime.rs

# 5. 核验券商中文报错匹配与被动限流降级
sed -n '106,113p' crates/jftrade-engine/src/product_trade_margin_route.rs

# 6. 核验回测指标 JS Math.round 浮点精度模拟
sed -n '197,228p' crates/jftrade-backtest/src/indicators.rs

# 7. 核验富途 OpenD 44 字节二进制报文头
sed -n '4,15p' crates/jftrade-integration-futu/src/frame.rs

# 8. 核验 SQLite PRAGMA Schema Manifest 强校验
sed -n '162,195p' crates/jftrade-store-sqlite/src/schema_manifest.rs

# 9. 核验桌面端 Profile 路径动态注入与原子防截断持久化 (window_state.rs)
grep -n "window_state_path" apps/desktop/src-tauri/src/native_lifecycle.rs
sed -n '194,228p' apps/desktop/src-tauri/src/window_state.rs

# 10. 核验微工具零依赖：encode_hex 9 行纯安全查表
sed -n '28,37p' crates/jftrade-engine/src/product_auth_session_crypto.rs
```

### 7.4 编译与测试门禁自查
```bash
# 1. 基础内核与锁原语编译验证
cargo check -p jftrade-kernel -p jftrade-owner-lock

# 2. 单元测试全量验证
cargo test -p jftrade-kernel -p jftrade-owner-lock -p jftrade-integration-futu

# 3. 官方只读 Clippy 门禁自查
pnpm run check:clippy
```

---

> **结语**: JFTrade 的 Rust 架构展现了极高的工业水准与工程克制。在核心交易撮合、代际行情隔离、模式完整性防护与专用脚本编译领域，自研代码坚不可摧且不可轻易替代；而在通用会话密码学、网络流解析与主动限流领域，顺应社区顶级标准进行针对性替换（P0/P1）；在桌面几何状态领域采取兼顾 Profile 隔离与原子落盘的混合演进；在纯安全微工具领域保持自研零外部依赖——将以最小的边际成本消除安全与稳定性隐患，推动系统架构迈向卓越。
