# Claude Code instructions

仓库级规则统一维护在 [`AGENTS.md`](AGENTS.md)，Claude Code 进入任意任务前应加载它以及目标目录下最近的局部 `AGENTS.md`。

@AGENTS.md

Claude 专用 agent/instruction 只描述交互方式，不复制模块边界、命令或已删除路径。需要架构事实时，以 [`scripts/module-map.json`](scripts/module-map.json) 和 [`docs/README.md`](docs/README.md) 为准。
