import { ref } from "vue";
import { afterEach, describe, expect, it, vi } from "vitest";

import { emptyPluginCatalog } from "@/contracts";
import { createConsoleDataPluginController } from "../src/composables/consoleDataPlugins";
import { createResponse } from "./helpers";

afterEach(() => {
  vi.unstubAllGlobals();
});

function createController() {
  const state = {
    pluginCatalog: ref(emptyPluginCatalog),
    pluginError: ref(""),
    installingPluginIds: ref<string[]>([]),
    uninstallingPluginIds: ref<string[]>([]),
  };
  return { controller: createConsoleDataPluginController(state), state };
}

describe("console plugin operations", () => {
  it("loads, installs and uninstalls plugins through terminal operations", async () => {
    const calls: string[] = [];
    vi.stubGlobal("fetch", vi.fn(async (input: string | URL | Request) => {
      const url = String(input);
      calls.push(url);
      if (url.endsWith("/api/v1/plugins/demo/install")) {
        return createResponse({ operation: { operationId: "install-op" } });
      }
      if (url.endsWith("/api/v1/plugins/demo/uninstall")) {
        return createResponse({ operation: { operationId: "uninstall-op" } });
      }
      if (url.includes("/operations/")) {
        return createResponse({ operationId: url.split("/").at(-1), status: "SUCCEEDED", message: "done" });
      }
      if (url.endsWith("/api/v1/plugins")) {
        return createResponse({
          ...emptyPluginCatalog,
          entries: [{ id: "demo", name: "Demo" }],
        });
      }
      throw new Error(`unexpected ${url}`);
    }));
    const { controller, state } = createController();

    await controller.loadPlugins();
    expect(state.pluginCatalog.value.entries).toHaveLength(1);
    await controller.installPlugin("demo");
    expect(state.installingPluginIds.value).toEqual([]);
    await controller.uninstallPlugin("demo");
    expect(state.uninstallingPluginIds.value).toEqual([]);
    expect(state.pluginError.value).toBe("");
    expect(calls).toEqual(expect.arrayContaining([
      expect.stringContaining("install-op"),
      expect.stringContaining("uninstall-op"),
    ]));
  });

  it("reports failed operations and always releases operation locks", async () => {
    vi.stubGlobal("fetch", vi.fn(async (input: string | URL | Request) => {
      const url = String(input);
      if (url.endsWith("/api/v1/plugins/broken/install")) {
        return createResponse({ operation: { operationId: "failed-op" } });
      }
      if (url.endsWith("/operations/failed-op")) {
        return createResponse({ operationId: "failed-op", status: "FAILED", message: "install failed", error: "checksum mismatch" });
      }
      throw new Error("catalog unavailable");
    }));
    const { controller, state } = createController();

    await controller.installPlugin("broken");
    expect(state.pluginError.value).toBe("checksum mismatch");
    expect(state.installingPluginIds.value).toEqual([]);
    await controller.loadPlugins();
    expect(state.pluginError.value).toBe("catalog unavailable");
  });

  it("returns uninstall guidance and handles unavailable guidance", async () => {
    let fail = false;
    vi.stubGlobal("fetch", vi.fn(async () => {
      if (fail) throw "offline";
      return createResponse({ pluginId: "demo", canUninstall: false, blockers: ["active strategy"] });
    }));
    const { controller, state } = createController();

    await expect(controller.loadPluginUninstallGuidance("demo")).resolves.toMatchObject({ canUninstall: false });
    fail = true;
    await expect(controller.loadPluginUninstallGuidance("demo")).resolves.toBeNull();
    expect(state.pluginError.value).toBe("插件卸载指引加载失败。");
  });
});
