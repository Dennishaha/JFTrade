# Organized Reference

本目录只做一件事：把当前项目实际会碰到的上游公共接口整理成“可回链”的索引。

它不是原始文档副本，也不替代源码。它的作用是让后续 AI 和开发者先找到正确接口族，再回到原始文档深挖细节。

## 当前内容

- [bbgo-external-interfaces.md](bbgo-external-interfaces.md)：本项目实际依赖的 bbgo 公共接口、扩展点和原始文档位置
- [futu-external-interfaces.md](futu-external-interfaces.md)：本项目实际依赖或需要了解的 Futu OpenD / 行情 / 交易接口族和原始文档位置

## 使用顺序

1. 先看 [../../architecture.md](../../architecture.md)，确认是在改 bbgo 集成、sidecar，还是 Futu 适配层
2. 再看本目录的接口索引，确认“到底该查哪组上游接口”
3. 最后回到原始文档或实现文件

## 维护约定

- 这里只列当前项目真正会碰到的接口，不追求穷举上游全部能力
- 每一项都尽量给出原始文档位置；如果上游文档没有覆盖，会明确标成“文档不足”
- 如果原始文档和当前代码存在命名差异，以当前上游代码语义为准，并在索引里注明