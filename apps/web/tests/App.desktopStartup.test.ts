// @vitest-environment jsdom

import { flushPromises, mount } from "@vue/test-utils";
import { afterEach, describe, expect, it, vi } from "vitest";
import {
  createMemoryHistory,
  createRouter,
  type Router,
  type RouteRecordRaw,
} from "vue-router";

const runtime = vi.hoisted(() => ({
  desktopMode: true,
  bridgeAvailable: true,
  authRequired: false,
}));

vi.mock("@/runtimeConfig", () => ({
  resolveDesktopMode: () => runtime.desktopMode,
  resolveDesktopBridgeAvailable: () => runtime.bridgeAvailable,
  resolveAuthRequired: () => runtime.authRequired,
}));

vi.mock("@/components/auth/AuthGate.vue", () => ({
  default: {
    emits: ["authenticated"],
    template:
      '<button data-testid="auth-gate" @click="$emit(\'authenticated\')">auth</button>',
  },
}));

vi.mock("@/components/app-shell/DesktopStartupGate.vue", () => ({
  default: {
    emits: ["ready"],
    template:
      '<button data-testid="startup-gate" @click="$emit(\'ready\')">startup</button>',
  },
}));

vi.mock("@/layout/AppShell.vue", () => ({
  default: { template: '<div data-testid="app-shell">shell</div>' },
}));

const routes: RouteRecordRaw[] = [
  { path: "/workspace", component: { template: "<div>workspace</div>" } },
  {
    path: "/desktop-logs",
    component: { template: '<div data-testid="standalone">logs</div>' },
    meta: { standalone: true },
  },
];

afterEach(() => {
  vi.resetModules();
  runtime.desktopMode = true;
  runtime.bridgeAvailable = true;
  runtime.authRequired = false;
});

async function mountApp(path: string): Promise<{ router: Router; wrapper: ReturnType<typeof mount> }> {
  const router = createRouter({ history: createMemoryHistory(), routes });
  await router.push(path);
  await router.isReady();
  const { default: App } = await import("@/App.vue");
  return { router, wrapper: mount(App, { global: { plugins: [router] } }) };
}

describe("App desktop startup routing", () => {
  it("mounts the shell only after the desktop startup service reports ready", async () => {
    const { wrapper } = await mountApp("/workspace");
    expect(wrapper.find('[data-testid="startup-gate"]').exists()).toBe(true);
    expect(wrapper.find('[data-testid="app-shell"]').exists()).toBe(false);

    await wrapper.get('[data-testid="startup-gate"]').trigger("click");
    expect(wrapper.find('[data-testid="app-shell"]').exists()).toBe(true);
    wrapper.unmount();
  });

  it("keeps standalone desktop routes independent of API startup", async () => {
    const { wrapper } = await mountApp("/desktop-logs");
    expect(wrapper.find('[data-testid="standalone"]').exists()).toBe(true);
    expect(wrapper.find('[data-testid="startup-gate"]').exists()).toBe(false);
    wrapper.unmount();
  });

  it("uses the ready shell immediately when desktop bindings are unavailable", async () => {
    runtime.bridgeAvailable = false;
    const { wrapper } = await mountApp("/workspace");
    expect(wrapper.find('[data-testid="app-shell"]').exists()).toBe(true);
    expect(wrapper.find('[data-testid="startup-gate"]').exists()).toBe(false);
    wrapper.unmount();
  });

  it("preserves Web authentication and responds to the auth-required event", async () => {
    runtime.desktopMode = false;
    runtime.authRequired = true;
    const { wrapper } = await mountApp("/workspace");
    expect(wrapper.find('[data-testid="auth-gate"]').exists()).toBe(true);

    await wrapper.get('[data-testid="auth-gate"]').trigger("click");
    expect(wrapper.find('[data-testid="app-shell"]').exists()).toBe(true);
    window.dispatchEvent(new CustomEvent("jftrade:web-auth-required"));
    await flushPromises();
    expect(wrapper.find('[data-testid="auth-gate"]').exists()).toBe(true);
    wrapper.unmount();
  });
});
