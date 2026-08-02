# 前端样式体系

本页定义 `apps/web` 的样式职责边界。目标不是禁止局部样式，而是让主题、间距和结构复用有唯一来源，避免 Vue SFC 随业务迭代持续膨胀。

## 职责分工

| 层 | 负责 | 不负责 |
| --- | --- | --- |
| Vuetify | 对话框、表单交互、Tabs、Window、无障碍状态等完整组件行为 | 页面布局和品牌主题常量 |
| Tailwind | 一次性的布局、对齐、溢出、响应式组合 | 反复出现的业务组件外观、主题色硬编码 |
| 全局 tokens | 颜色语义、间距、圆角、控件高度、阴影和层级 | 具体组件结构 |
| 全局结构 primitives | 至少三个组件共享的稳定结构，例如 `.jf-panel*` | 业务状态和页面专属布局 |
| scoped CSS | 组件独有的状态、复杂布局以及第三方组件的局部 `:deep()` 适配 | 复制全局 token 值或已有 primitive |

一个元素可以同时使用 Vuetify 和 Tailwind，但两者必须各自承担明确职责。例如 Vuetify 提供 Tabs 的交互语义，Tailwind 只排列外层容器；不要用 scoped CSS 重写整套 Vuetify 状态。

## Token 使用规则

全局 token 的唯一入口是 `apps/web/src/styles/tokens.css`，并在应用启动时早于其他本地样式导入。命名约定：

- `--jf-*`：跨领域基础尺度，例如 `--jf-space-3`、`--jf-radius-lg`、`--jf-toolbar-height`。
- `--tv-*`：交易工作区的语义颜色与表面。
- `--card-*`：卡片式页面内容的语义颜色。
- `--adk-*`：仅 ADK 域使用，保留在 `adk-tokens.css`；可引用全局 token，不得重新建立间距和圆角尺度。

新增样式时先寻找语义 token。只有真实的新设计语义才能新增 token；不要为某个组件的单个像素值创建 token。亮色和暗色必须在同一 token 名称下分别赋值。例外：替换历史上主题无关的固定色值时，使用只定义在 `:root, [data-theme="dark"]` 基础块中的固定 token（如 `--jf-accent-*`、`--jf-white`），不为亮色主题另设值，以保持两个主题的视觉不变。

### 字号与字距尺度

字号使用 `--jf-text-1` 到 `--jf-text-18`（7px 到 38px，按全仓实际用值递增编号，中间档不为假想值预留）；字距使用 `--jf-tracking-1` 到 `--jf-tracking-4`（0.14em / 0.16em / 0.18em / 0.2em）。scoped CSS 与全局样式中的 `font-size` 静态声明必须写 `var(--jf-text-N)`，不得直接写 px 字面值。Tailwind 侧用 CSS 变量短横语法引用：`text-(length:--jf-text-5)`、`tracking-(--jf-tracking-2)`，不要再写 `text-[11px]`、`tracking-[0.16em]` 这类任意值。模板内联 `style` 与脚本中的动态字号（如图表配置）暂不受此约束。

## 何时抽取共享样式

满足以下任一条件时，优先提取到 `styles/components.css` 或已有共享组件：

1. 相同结构在三个及以上组件出现；
2. 一组规则同时定义边框、背景、圆角和标题层级；
3. 修改该结构时需要同步多个 SFC。

共享 class 使用 `jf-` 前缀。组件可以保留原有 BEM class 处理自身差异，并在标记上组合 primitive，例如：

```html
<section class="risk-panel jf-panel">
  <header class="risk-panel__head jf-panel__header">
    <span class="jf-panel__title">标题</span>
  </header>
</section>
```

禁止为了降低 `.vue` 行数而把原样 scoped CSS 搬到外部文件。体量门禁会把 `<style src>` 指向的本地样式计入组件样式负担。

## SFC 职责与体量

- 新增或已治理的 `.vue` 文件上限为 800 行。
- 超限组件必须按业务职责拆分：页面负责编排，子组件负责可命名的业务区块，纯格式化或状态机迁到 `.ts` 模块/composable。
- props 应传递稳定业务模型或已计算语义，避免子组件反向依赖整页实例。
- `scripts/web-component-budget.json` 当前保持零例外；上限必须与当前实测值完全相等，并与 merge-base 比较为只减不增。不能通过同步调高 JSON 来放过组件或 scoped CSS 增长。
- 不接受新增体量例外。组件接近上限时应先抽离可命名的业务区块或纯 TypeScript 状态逻辑，再继续叠加功能。

本地执行 `pnpm run check:web-component-budget`；该检查也是 `test:test-policy` 和 preflight 的一部分。
