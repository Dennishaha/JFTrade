// @vitest-environment jsdom

import { beforeEach, describe, expect, it, vi } from "vitest";

const runtime = vi.hoisted(() => {
  const app = { mount: vi.fn(), use: vi.fn() };
  app.use.mockReturnValue(app);
  return {
    app,
    createApp: vi.fn(() => app),
    createConsoleRouter: vi.fn(() => ({ name: "router" })),
    createVuetify: vi.fn(() => ({ name: "vuetify" })),
    initialize: vi.fn(),
  };
});

vi.mock("vue", async (importOriginal) => ({
  ...(await importOriginal<typeof import("vue")>()),
  createApp: runtime.createApp,
}));
vi.mock("vuetify", () => ({ createVuetify: runtime.createVuetify }));
vi.mock("@tanstack/vue-query", () => ({ VueQueryPlugin: { name: "query" } }));
vi.mock("@/composables/settings/serverState", () => ({ queryClient: { name: "client" } }));
vi.mock("@/runtimeConfig", () => ({ initializeTauriRuntimeConfig: runtime.initialize }));
vi.mock("@/router", () => ({ createConsoleRouter: runtime.createConsoleRouter }));
vi.mock("@/fontAwesomeIcons", () => ({ fontAwesomeIcons: {} }));
vi.mock("@/vuetifyTheme", () => ({ vuetifyTheme: {} }));

beforeEach(() => {
  vi.resetModules();
  vi.clearAllMocks();
  runtime.app.use.mockReturnValue(runtime.app);
  document.body.innerHTML = '<div id="app"></div>';
});

describe("web bootstrap", () => {
  it("waits for desktop runtime configuration before mounting Vue", async () => {
    runtime.initialize.mockResolvedValue(undefined);

    await import("@/main");
    await vi.waitFor(() => expect(runtime.app.mount).toHaveBeenCalledWith("#app"));

    expect(runtime.initialize).toHaveBeenCalledOnce();
    expect(runtime.createApp).toHaveBeenCalledOnce();
    expect(runtime.initialize.mock.invocationCallOrder[0]).toBeLessThan(
      runtime.createApp.mock.invocationCallOrder[0] ?? 0,
    );
    expect(runtime.app.use).toHaveBeenCalledTimes(3);
  });

  it("renders a fail-closed startup message when runtime configuration fails", async () => {
    const error = new Error("unsafe runtime");
    const consoleError = vi.spyOn(console, "error").mockImplementation(() => undefined);
    runtime.initialize.mockRejectedValue(error);

    await import("@/main");
    await vi.waitFor(() => expect(document.querySelector("#app")?.textContent).toContain("启动失败"));

    expect(consoleError).toHaveBeenCalledWith("JFTrade web bootstrap failed", error);
    expect(runtime.createApp).not.toHaveBeenCalled();
    consoleError.mockRestore();
  });

  it("reports bootstrap failure even if the mount node is absent", async () => {
    const consoleError = vi.spyOn(console, "error").mockImplementation(() => undefined);
    document.body.innerHTML = "";
    runtime.initialize.mockRejectedValue(new Error("missing runtime"));

    await import("@/main");
    await vi.waitFor(() => expect(consoleError).toHaveBeenCalledOnce());

    expect(document.querySelector("#app")).toBeNull();
    consoleError.mockRestore();
  });
});
