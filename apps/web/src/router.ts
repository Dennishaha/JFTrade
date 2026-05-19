import {
  type Router,
  type RouterHistory,
  createRouter,
  createWebHistory,
} from "vue-router";

export function createConsoleRouter(
  history: RouterHistory = createWebHistory(),
): Router {
  return createRouter({
    history,
    routes: [
      { path: "/", redirect: "/workspace" },
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
        path: "/broker",
        component: () => import("./pages/BrokerPage.vue"),
      },
      {
        path: "/portfolio",
        component: () => import("./pages/PortfolioPage.vue"),
      },
      {
        path: "/execution",
        component: () => import("./pages/ExecutionPage.vue"),
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
    ],
  });
}
