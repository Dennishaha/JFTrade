import {
  type Router,
  type RouterHistory,
  createRouter,
  createWebHistory,
} from "vue-router";

import OobeOverlay from "./components/OobeOverlay.vue";

export function createConsoleRouter(
  history: RouterHistory = createWebHistory(),
): Router {
  return createRouter({
    history,
    routes: [
      { path: "/", redirect: "/workspace" },
      {
        path: "/oobe",
        component: OobeOverlay,
        meta: { title: "初始化" },
      },
      {
        path: "/workspace",
        component: () => import("./pages/WorkspacePage.vue"),
        meta: { title: "交易" },
      },
      {
        path: "/overview",
        component: () => import("./pages/OverviewPage.vue"),
        meta: { title: "概览" },
      },
      {
        path: "/system",
        component: () => import("./pages/SystemPage.vue"),
        meta: { title: "系统" },
      },
      {
        path: "/settings/:section?",
        component: () => import("./pages/SettingsPage.vue"),
        meta: { title: "设置" },
      },
      {
        path: "/account",
        component: () => import("./pages/AccountPage.vue"),
        meta: { title: "账户" },
      },
      {
        path: "/broker",
        redirect: "/account",
      },
      {
        path: "/portfolio",
        redirect: "/account",
      },
      {
        path: "/execution",
        redirect: "/account",
      },
      {
        path: "/market",
        component: () => import("./pages/MarketPage.vue"),
        meta: { title: "行情" },
      },
      {
        path: "/strategy",
        component: () => import("./pages/StrategyPage.vue"),
        meta: { title: "策略" },
      },
      {
        path: "/adk",
        component: () => import("./pages/ADKPage.vue"),
        meta: { title: "Agents" },
      },
      {
        path: "/risk",
        component: () => import("./pages/RiskPage.vue"),
        meta: { title: "风控" },
      },
      {
        path: "/backtest",
        component: () => import("./pages/BacktestPage.vue"),
        meta: { title: "回测" },
      },
    ],
  });
}
