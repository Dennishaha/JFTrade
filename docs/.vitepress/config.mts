import { defineConfig } from "vitepress";

export default defineConfig({
  title: "JFTrade Docs",
  description: "JFTrade user and reference documentation",
  base: "/docs/",
  cleanUrls: true,
  ignoreDeadLinks: true,
  markdown: {
    config(md) {
      const defaultFenceRenderer = md.renderer.rules.fence;

      md.renderer.rules.fence = (tokens, idx, options, env, self) => {
        const token = tokens[idx];
        const language = token.info.trim().split(/\s+/)[0]?.toLowerCase();

        if (language === "mermaid") {
          const encodedSource = encodeURIComponent(token.content.trim());
          return `<MermaidBlock code="${md.utils.escapeHtml(encodedSource)}" />`;
        }

        if (defaultFenceRenderer) {
          return defaultFenceRenderer(tokens, idx, options, env, self);
        }

        return self.renderToken(tokens, idx, options);
      };
    },
  },
  srcExclude: [
    "reference/Futu-API-Doc-zh-Proto.md",
    "reference/bbgo-doc/**/*.md",
  ],
  themeConfig: {
    nav: [
      { text: "快速开始", link: "/quick-start" },
      { text: "配置", link: "/configuration" },
      { text: "使用指南", link: "/usage" },
      { text: "自选", link: "/watchlist" },
      { text: "排障", link: "/troubleshooting" },
      { text: "开源许可", link: "/legal/license" },
      { text: "参考", link: "/reference/generated/api" },
    ],
    sidebar: [
      {
        text: "用户指南",
        items: [
          { text: "文档首页", link: "/" },
          { text: "快速开始", link: "/quick-start" },
          { text: "配置", link: "/configuration" },
          { text: "使用指南", link: "/usage" },
          { text: "回测执行模型", link: "/backtest-execution-model" },
          { text: "自选系统", link: "/watchlist" },
          { text: "排障", link: "/troubleshooting" },
          { text: "开源许可", link: "/legal/license" },
          { text: "第三方许可证", link: "/legal/third-party-notices" },
        ],
      },
      {
        text: "自动生成参考",
        items: [
          { text: "HTTP API", link: "/reference/generated/api" },
          { text: "数据类型", link: "/reference/generated/types" },
          { text: "Pine v6 支持快照", link: "/reference/generated/pine-v6-support" },
        ],
      },
      {
        text: "维护参考",
        items: [
          { text: "架构", link: "/architecture" },
          { text: "架构 Mermaid 图", link: "/architecture-mermaid" },
          { text: "PineTS 发布清单", link: "/troubleshooting/pinets-worker-release" },
          { text: "ADK", link: "/adk" },
          { text: "活动路线图", link: "/roadmap" },
          { text: "新券商接入", link: "/new-broker-integration-guide" },
          { text: "API 生命周期", link: "/reference/api-lifecycle" },
          { text: "Reference", link: "/reference/" },
        ],
      },
    ],
    search: {
      provider: "local",
    },
  },
});
