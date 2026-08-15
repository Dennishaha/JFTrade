# Web 局部指令

- 入口：`apps/web/src/pages`、`components`、`composables`、`features`；API 类型来自 `src/generated/openapi.ts`，wire/view-model 映射位于 `src/contracts` 和 `src/types/view-models`。
- 业务请求统一经 `src/composables/shared/apiClient.ts`；组件不得直接使用 `fetch` 或猜测后端字段。
- 状态优先使用 Vue Query 和页面级 composable；不要新增全局 singleton 承载领域状态。
- 测试在 `apps/web/tests` 按 src 领域镜像；优先断言行为、拒绝路径和恢复路径。
- 最小验证：`pnpm --filter @jftrade/web run test <file>`、`pnpm --filter @jftrade/web run typecheck`。
- 行数门禁：`src`/`tests` 下的 `.ts`/`.vue`/`.css` 均受根目录 `check:web-file-length` 约束（800/1200 行），超限历史文件登记在 `scripts/web-file-length-budget.json`，只许降不许涨。
- 修改公开 API 后运行根目录 `pnpm run check:generated` 和 `pnpm run typecheck:contracts`。
