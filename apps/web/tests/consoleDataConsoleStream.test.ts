// @vitest-environment jsdom

import { afterEach, describe, expect, it, vi } from "vitest";
import { ref } from "vue";

import { createConsoleDataConsoleStreamController } from "../src/composables/consoleDataConsoleStream";
import { resetSharedLiveSocketHubForTests } from "../src/composables/sharedLiveSocket";

afterEach(() => {
  resetSharedLiveSocketHubForTests();
});

describe("createConsoleDataConsoleStreamController", () => {
  it("initializes the console-refresh lease only once for repeated shell startup", async () => {
    const reloadSystemState = vi.fn(async () => undefined);
    const controller = createConsoleDataConsoleStreamController({
      liveStreamStatus: ref<"disconnected" | "connected" | "degraded">("disconnected"),
      liveStreamCheckedAt: ref(""),
      reloadSystemState,
    });

    await controller.initialize();
    await controller.initialize();

    expect(reloadSystemState).toHaveBeenCalledTimes(1);
    expect(reloadSystemState).toHaveBeenCalledWith();
    controller.dispose();
  });
});
