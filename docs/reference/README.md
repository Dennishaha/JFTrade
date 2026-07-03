# 参考资料入口

本层只放三类内容：

- 协议与编码细节
- 上游参考资料
- 不适合继续放在架构总览里的长背景说明

不要把当前运行架构、排障步骤或前端 K 线规则继续堆到这里。

## 先看什么

- [summary/README.md](summary/README.md)：整理版索引，先看接口族，再回到原始文档
- [opend-protocol.md](opend-protocol.md)：当前项目实际使用的 OpenD 协议入口、端口语义和实现落点
- [trading-platform-reference-guide.md](trading-platform-reference-guide.md)：vn.py、QuantConnect LEAN、OpenBB 等开源项目的参考方向与 JFTrade 落点
- [Futu-API-Doc-zh-Proto.md](Futu-API-Doc-zh-Proto.md)：Futu API 参考资料
- [bbgo-doc/README.md](bbgo-doc/README.md)：上游 bbgo 参考资料，保持原样，不在本次重写范围内

## 参考层维护约定

- 协议实现以代码为准，文档只做导览和边界说明
- 不重复维护同一份帧结构、环境变量和启动约定
- 需要知道当前系统怎么跑，回到 [../architecture.md](../architecture.md)

## 实现锚点

- [../../pkg/futu/codec](../../pkg/futu/codec)
- [../../pkg/futu/opend](../../pkg/futu/opend)
- [../../pkg/futu/exchange.go](../../pkg/futu/exchange.go)
