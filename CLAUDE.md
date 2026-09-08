# Claude Code instructions

仓库级规则统一维护在 [AGENTS.md](AGENTS.md)。先加载根规则，再读取从根到目标路径沿途适用的局部 `AGENTS.md`；局部规则只补充所属范围。

@AGENTS.md

本文件只负责 Claude Code 的加载入口，不复制命令、工具链版本或架构边界。任务路由、验证和交付要求均遵循根规则；实现与文档冲突时先核对源码、配置和测试，不依据旧快照改回实现。
