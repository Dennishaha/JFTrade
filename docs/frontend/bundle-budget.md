# 前端 bundle 预算

前端构建以路由级 code splitting 为基础，并对首屏依赖图和全部异步 JavaScript 建立
可复现的 gzip 预算。预算用于防止依赖意外进入首屏，不以删除必要的 Monaco、Mermaid
或图表能力为目标。

## 命令

```bash
pnpm run build:web:report
```

已有 `apps/web/dist` 时，可只运行：

```bash
pnpm run check:web-bundle
```

`scripts/report-web-bundle.mjs` 从 `dist/index.html` 解析首屏 script、modulepreload 和
stylesheet，再扫描 `dist/assets`，按 gzip level 9 计算稳定指标。`ci-local` 在前端与
文档资产构建后直接执行检查，不重复构建。

## 当前基线

2026-08-09 的 release asset 构建结果：

| 指标 | 实测 | 预算 |
| --- | ---: | ---: |
| 首屏 JavaScript gzip | 310.0 KiB | 351.6 KiB |
| 首屏 CSS gzip | 88.1 KiB | 97.7 KiB |
| 最大异步 JavaScript gzip | 1,448.8 KiB | 1,513.7 KiB |
| 最大异步非 worker JavaScript gzip | 657.3 KiB | 722.7 KiB |
| 全部控制台 JavaScript gzip | 4,217.3 KiB | 4,736.3 KiB |

最大异步文件是 Monaco TypeScript worker，最大异步非 worker 文件是 `editor.api`；
两者分别受预算约束，避免必要的 TypeScript worker 掩盖普通业务 chunk 膨胀。
`editor.api`、Monaco worker、Mermaid core 和 Cytoscape 均不在首屏引用集合中。预算
额外禁止这些重依赖进入 `index.html` 的初始图，即使总体积仍低于阈值也会失败。

Monaco 核心入口不再静态注册 JavaScript/TypeScript 语言服务。Pine/JSON 编辑器不会
触发该依赖路径；只有语言为 JavaScript/TypeScript 或提供 `extraLibs` 时才按需注册。
TypeScript worker 仍保留在构建产物中，以维持这些兼容能力，并由 Monaco 0.56 在首次
实际使用语言服务时创建。

预算保存在 `scripts/web-bundle-budget.json`。只有经过 bundle 报告说明、确认用户
价值与加载边界后才允许提高；普通依赖升级或新增功能应通过拆分、延迟加载或删除旧
代码留在现有预算内。

## 依赖结论

- `monaco-editor` 由代码编辑器按需动态加载，相关 worker 保持异步。
- `mermaid` 只在 ADK 图形首次渲染时动态加载。
- `acorn` 已从 `@jftrade/web` 直接依赖中移除，生产源码无直接 import；它仍由
  `vue-router -> mlly` 间接使用，因此 lockfile 中保留同一版本是正常结果。
- 12 个页面路由继续使用动态 import。修改 router 或顶层 layout 后应特别关注首屏
  gzip 与 forbidden initial asset 检查。
