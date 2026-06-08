import { defineConfig } from "vitepress";

export default defineConfig({
  title: "JFTrade Docs",
  description: "JFTrade user and reference documentation",
  base: "/docs/",
  cleanUrls: true,
  ignoreDeadLinks: true,
  srcExclude: [
    "reference/Futu-API-Doc-zh-Proto.md",
    "reference/bbgo-doc/**/*.md",
  ],
  themeConfig: {
    nav: [
      { text: "快速开始", link: "/quick-start" },
      { text: "配置", link: "/configuration" },
      { text: "使用指南", link: "/usage" },
      { text: "排障", link: "/troubleshooting" },
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
          { text: "排障", link: "/troubleshooting" },
        ],
      },
      {
        text: "自动生成参考",
        items: [
          { text: "HTTP API", link: "/reference/generated/api" },
          { text: "数据类型", link: "/reference/generated/types" },
        ],
      },
      {
        text: "维护参考",
        items: [
          { text: "架构", link: "/architecture" },
          { text: "ADK", link: "/adk" },
          { text: "Reference", link: "/reference/" },
        ],
      },
    ],
    search: {
      provider: "local",
    },
  },
});
