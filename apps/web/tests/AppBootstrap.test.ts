import { beforeEach, describe, expect, it, vi } from "vitest";

const bootstrap = vi.hoisted(() => {
  const app = {
    use: vi.fn(),
    mount: vi.fn(),
  };
  return {
    app,
    createApp: vi.fn(() => app),
    createPinia: vi.fn(() => ({ kind: "pinia" })),
    createRouter: vi.fn(() => ({ kind: "router" })),
    createVuetify: vi.fn(() => ({ kind: "vuetify" })),
  };
});

vi.mock("vue", async (importOriginal) => ({
  ...(await importOriginal<typeof import("vue")>()),
  createApp: bootstrap.createApp,
}));
vi.mock("pinia", () => ({ createPinia: bootstrap.createPinia }));
vi.mock("vuetify", () => ({ createVuetify: bootstrap.createVuetify }));
vi.mock("vuetify/components", () => ({}));
vi.mock("vuetify/directives", () => ({}));
vi.mock("vuetify/styles", () => ({}));
vi.mock("@tanstack/vue-query", () => ({ VueQueryPlugin: { kind: "query-plugin" } }));
vi.mock("../src/App.vue", () => ({ default: { name: "JFTradeConsole" } }));
vi.mock("../src/router", () => ({ createConsoleRouter: bootstrap.createRouter }));
vi.mock("../src/fontAwesomeIcons", () => ({ fontAwesomeIcons: { defaultSet: "fa" } }));
vi.mock("../src/composables/serverState", () => ({ queryClient: { kind: "query-client" } }));

describe("application bootstrap", () => {
  beforeEach(() => {
    vi.resetModules();
    bootstrap.app.use.mockReset().mockReturnValue(bootstrap.app);
    bootstrap.app.mount.mockReset();
    bootstrap.createApp.mockClear();
    bootstrap.createPinia.mockClear();
    bootstrap.createRouter.mockClear();
    bootstrap.createVuetify.mockClear();
  });

  it("creates one dark-console application with state, query, router, and Vuetify plugins", async () => {
    await import("../src/main");

    expect(bootstrap.createApp).toHaveBeenCalledWith({ name: "JFTradeConsole" });
    expect(bootstrap.createPinia).toHaveBeenCalledOnce();
    expect(bootstrap.createRouter).toHaveBeenCalledOnce();
    expect(bootstrap.createVuetify).toHaveBeenCalledWith(
      expect.objectContaining({
        icons: { defaultSet: "fa" },
        theme: expect.objectContaining({
          defaultTheme: "dark",
          themes: expect.objectContaining({
            dark: expect.objectContaining({ dark: true }),
            light: expect.objectContaining({ dark: false }),
          }),
        }),
      }),
    );
    expect(bootstrap.app.use).toHaveBeenNthCalledWith(1, { kind: "pinia" });
    expect(bootstrap.app.use).toHaveBeenNthCalledWith(
      2,
      { kind: "query-plugin" },
      { queryClient: { kind: "query-client" } },
    );
    expect(bootstrap.app.use).toHaveBeenNthCalledWith(3, { kind: "router" });
    expect(bootstrap.app.use).toHaveBeenNthCalledWith(4, { kind: "vuetify" });
    expect(bootstrap.app.mount).toHaveBeenCalledWith("#app");
  });
});
