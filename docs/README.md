# JFTrade 文档导航

本文档面向两类读者：

- 想快速理解当前系统边界的开发者
- 需要让后续 AI 协作不跑偏的维护者

如果你只看一篇，请先看 [architecture.md](architecture.md)。

## 阅读顺序

### 1. 先理解系统现在是怎么跑的

- [architecture.md](architecture.md)：当前系统架构、双运行模式、核心数据流、职责边界。

### 2. 再看你要改的专题

- [troubleshooting.md](troubleshooting.md)：排障入口，按启动、WebSocket、OpenD、行情时段分流。
- [frontend-kline.md](frontend-kline.md)：前端行情与 K 线专题入口，包含实时合成与防回归约束。

### 3. 最后看参考资料

- [reference/README.md](reference/README.md)：协议细节、上游 bbgo 资料、历史参考文档入口。

## 文档分层约定

为避免信息密度失控，docs 统一按下面的规则组织：

- 顶层入口文档只回答“这块是什么、边界在哪里、先看哪篇”。
- 实现细节、边界条件、回归案例放到专题子目录，不继续堆在总览文档里。
- 协议原文、上游资料、长篇背景说明放到 reference 层，不干扰当前架构判断。

## 当前文档地图

```text
docs/
├── README.md                 文档导航
├── architecture.md           当前系统架构总览
├── troubleshooting.md        排障入口
├── frontend-kline.md         前端行情/K 线入口
├── troubleshooting/          排障专题
├── frontend/                 前端与行情专题
├── reference/                协议与参考资料
	└── bbgo-doc/              上游 bbgo 参考资料（保持原样）
```

## AI 协作约定

后续 AI 在动手前应优先按下面顺序取上下文：

1. 先读 [architecture.md](architecture.md)，确认是在改 sidecar、bbgo 运行时、前端，还是 Futu 适配层。
2. 如果是启动、端口、连接问题，再进 [troubleshooting.md](troubleshooting.md)。
3. 如果是实时行情、K 线、WebSocket、快照合成问题，再进 [frontend-kline.md](frontend-kline.md)。
4. 只有在需要协议或上游背景时，才进入 [reference/README.md](reference/README.md) 或 [reference/bbgo-doc/README.md](reference/bbgo-doc/README.md)。