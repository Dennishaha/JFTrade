# Web 局部指令

继承根 [AGENTS.md](../../AGENTS.md)。以下路径相对 `apps/web`；命令从仓库根目录运行。

## 入口与边界

- 页面和交互从 `src/pages`、`src/components`、`src/composables`、`src/features` 定位；测试在 `tests` 按源码领域镜像组织。
- wire 类型经 `src/contracts` 引用；生成源为 `src/generated/openapi.ts`，人工 view model 位于 `src/types/view-models`。业务组件不直引生成文件、不猜测后端字段，详见 [API 契约](../../docs/frontend/api-contracts.md)。
- 业务请求统一经 `src/composables/shared/apiClient.ts`；JSON 使用 typed API，SSE 等使用 `apiRawRequest`，组件不得直接使用 `fetch`。
- 服务端状态归 Vue Query，页面状态归页面 composable/context；不新增全局 singleton 复制领域状态。缓存、取消、reset 和测试隔离遵守 [状态管理](../../docs/frontend/state-management.md)。
- 样式优先复用既有 tokens 和 primitives；组件预算不得通过外移原样 CSS 或调高预算绕过，见 [样式规范](../../docs/frontend/styling-guide.md)。

## 最小验证

```bash
pnpm --filter @jftrade/web run test <test-file>
pnpm run typecheck:web
pnpm run check:quick
```

- 公开 API / wire 类型变化额外运行 `pnpm run check:generated` 和 `pnpm run typecheck:web-contracts`；生成步骤遵循根指令。
- `src`/`tests` 下 `.ts`/`.vue`/`.css` 受 `check:web-file-length` 约束，预算见根 `scripts/web-file-length-budget.json`；组件另受 `check:web-component-budget` 约束，历史预算只许降不许涨。
- 测试断言行为、拒绝和恢复路径；时钟、QueryClient、singleton 和监听器必须按用例隔离与清理。
