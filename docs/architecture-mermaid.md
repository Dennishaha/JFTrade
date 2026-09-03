# JFTrade 架构图

更新时间：2026-09-02。本文只展示当前 Rust/Tauri 产品架构；迁移历史不进入运行图。

## 产品组件

```mermaid
flowchart TB
    subgraph Clients[客户端]
        Desktop[Tauri 2 desktop]
        Browser[Optional browser]
        Web[Vue 3 console]
        Desktop --> Web
        Browser --> Web
    end

    subgraph Rust[Rust product]
        Engine[jftrade-engine\ncomposition + lifecycle]
        API[jftrade-api\nHTTP / SSE / WebSocket]
        Domains[domain crates]
        Stores[SQLite/settings stores\nWriterLease]
        Market[jftrade-marketdata\nProviderRouter]
        Futu[jftrade-integration-futu]
        Pine[jftrade-integration-pine]
        Helper[jftrade-integration-marketdata-helper]
        Engine --> API
        Engine --> Domains
        Domains --> Stores
        Engine --> Market
        Market --> Futu
        Engine --> Pine
        Market --> Helper
    end

    Web -->|/api/v1 REST / SSE / WS| API
    Futu --> OpenD[Futu OpenD]
    Pine --> PineWorker[Node PineTS worker]
    Helper --> Python[Python market-data helper]
```

## 启动与关闭

```mermaid
sequenceDiagram
    participant Shell as CLI/Tauri
    participant Engine as ProductRuntimeBuilder
    participant Store as Stores/WriterLease
    participant Runtime as Provider/Workers
    participant API as Rust API

    Shell->>Engine: build product config
    Engine->>Store: validate schema + migrate + acquire leases
    Engine->>Runtime: start OpenD/Pine/Python as configured
    Engine->>API: bind 278 production routes + listeners
    API-->>Shell: ready
    Shell->>Engine: shutdown
    Engine->>API: stop admission/listeners
    Engine->>Runtime: cancel + join child runtimes
    Engine->>Store: flush + release leases
```

## 行情与实时流

```mermaid
flowchart LR
    UI[Vue pages] --> API[/market-data + SSE/WS]
    API --> Hub[LiveHub]
    Hub --> Router[ProviderRouter + DemandBook]
    Router --> Futu[OpenD push/query]
    Router --> Python[yfinance/AKShare polling]
    Router --> Cache[normalized cache]
    Cache --> Hub
```

## 策略、回测与交易

```mermaid
flowchart LR
    UI[Strategy/backtest UI] --> API[Rust API]
    API --> Engine[jftrade-engine ports]
    Engine --> Strategy[jftrade-strategy]
    Engine --> Pine[Pine integration]
    Pine --> Worker[PineTS worker\nsignals/plots/order intents]
    Engine --> Backtest[jftrade-backtest\nmatching/equity/risk]
    Engine --> Trading[jftrade-trading\nbroker orders]
    Engine --> Store[SQLite stores]
    Trading --> Futu[Futu integration]
```

## 契约和发布链

```mermaid
flowchart TB
    OpenAPI[contracts/openapi/openapi.json] --> WebTypes[Web generated API types]
    OpenAPI --> Docs[Generated reference docs]
    OpenAPI --> RouteGate[278 route set gate]
    Proto[proto/futu + proto/pineworker] --> RustProto[Rust private generated types]
    Proto --> NodeProto[Node runtime loader]
    Fixtures[Frozen product fixtures] --> Replay[Rust compatibility replay]
    Baseline[Previous official release bytes] --> Upgrade[Upgrade/rollback drill]
    RouteGate --> Local[check:rust + check:contracts + check:zero-go]
    Replay --> Local
    Local --> Bundle[Tauri platform bundles]
    Bundle --> ArtifactGate[zero-Go + SBOM/provenance + signing]
    Upgrade --> Candidate[release candidate evidence]
    ArtifactGate --> Candidate
    Candidate --> Publish[Manual publish of sealed candidate]
    Publish --> Post[Independent post-release validation]
```

## 状态边界

```mermaid
stateDiagram-v2
    [*] --> SourceAdmitted: exact SHA passes Build & Test
    SourceAdmitted --> Rehearsed: unsigned four-platform rehearsal
    SourceAdmitted --> CandidateReady: signing, upgrade and security evidence complete
    CandidateReady --> Published: manual workflow consumes the sealed candidate
    Published --> Validated: independent post-release validation passes

    note right of Rehearsed
      releaseQualified=false
      rehearsal artifacts cannot publish
    end note
```

历史文档和 fixture 可以记录来源，但不连接到当前产品图，也不参与路由或发布状态计算。
