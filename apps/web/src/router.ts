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
      },
      {
        path: "/workspace",
        component: () => import("./pages/WorkspacePage.vue"),
      },
      {
        path: "/overview",
        component: () => import("./pages/OverviewPage.vue"),
      },
      {
        path: "/system",
        component: () => import("./pages/SystemPage.vue"),
      },
      {
        path: "/settings",
        component: () => import("./pages/SettingsPage.vue"),
      },
      {
        path: "/account",
        component: () => import("./pages/AccountPage.vue"),
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
      },
      {
        path: "/strategy",
        component: () => import("./pages/StrategyPage.vue"),
      },
      {
        path: "/risk",
        component: () => import("./pages/RiskPage.vue"),
      },
      {
        path: "/backtest",
        component: () => import("./pages/BacktestPage.vue"),
      },
    ],
  });
}
