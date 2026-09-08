# 适配器与业务编排的边界参考

本页服务于“协议实现与可复用业务编排如何分开”的架构评审，不定义 JFTrade 新分层，也不要求先创建统一外壳。当前 owner 和依赖方向以 [后端规范](../../../../docs/architecture/backend-coding-standards.md) 为准。

## 上游如何查

在选定的 [StockSharp 源码](https://github.com/StockSharp/StockSharp) 中检索 `IMessageAdapter`、`MessageAdapter`、`Connector`、`BasketMessageAdapter` 等符号，核对接口、连接/订阅托管、能力路由之间的关系。它们是检索线索，不保证路径和实现跨版本不变。

不假设固定本地 checkout；需要引用上游结论时记录 commit 和实际符号位置。只提取当前问题需要的设计依据，不照搬 C# 继承结构、事件总线或目录布局。

## 本地评审清单

| 问题 | 应核对的证据 |
| --- | --- |
| 协议翻译与业务决策是否混合？ | 从请求绑定一路追到领域 port 和 integration，确认协议类型没有泄漏、规则没有重复实现 |
| 订阅/重连由谁管理？ | 区分连接恢复与业务订阅恢复，确认 demand、generation、取消和 shutdown 有明确 owner |
| 新能力如何被发现和拒绝？ | 按 [券商集成指南](../../../../docs/new-broker-integration-guide.md) 核对 capability、API/UI/tool 暴露和不支持路径 |
| 是否确有可复用编排？ | 比较实际调用方的状态与生命周期，不把名字相似的代码直接合并 |
| 是否需要新抽象？ | 指出已有 port 的不足和当前消费者；只有未来可能用到的层或依赖不应提前创建 |

## 实施边界

- 协议 adapter 封装映射和 I/O；共享业务规则留在已有领域 owner，engine 负责生产装配。拆文件不能产生第二个状态写入者。
- 抽取前固定成功、拒绝、重连、取消和恢复行为；提取后以同一组行为测试验证。
- 不把多券商聚合、消息汇流或跨 Provider 回退当作默认优化。若需要新增这些行为，应作为显式需求评估。
- 普通测试使用 fixture、mock server 或 testkit；真实账户/OpenD 不因架构评审而进入验证范围。
- 边界变化时更新对应专题和模块表；没有必要的结构调整时，说明依据并保留实现。
