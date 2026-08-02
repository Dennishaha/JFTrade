import type { VuetifyOptions } from "vuetify";

/**
 * Vuetify 主题色的单一事实来源是 `styles/tokens.css`：
 * `background` 必须等于 `--tv-bg-app`，`surface` 必须等于 `--tv-bg-surface`
 * （按各主题分别对齐）。修改 tokens.css 中这两个 token 时必须同步此处。
 *
 * 不能直接写 `var(--tv-*)`：Vuetify 会把主题色 parseColor 成 r,g,b 三元组
 * 注入 `--v-theme-*` 变量（组件以 `rgb(var(--v-theme-*))` 消费），并基于
 * 静态值派生 on-* 对比色与 overlay-multiplier，var() 引用无法被解析。
 */
export const vuetifyTheme: NonNullable<VuetifyOptions["theme"]> = {
  defaultTheme: "dark",
  themes: {
    light: {
      dark: false,
      colors: {
        background: "#f1f5f9",
        surface: "#ffffff",
      },
    },
    dark: {
      dark: true,
      colors: {
        background: "#0a0a0a",
        surface: "#141414",
      },
    },
  },
};
