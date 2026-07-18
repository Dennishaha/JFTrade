// @vitest-environment jsdom

import { mount } from "@vue/test-utils";
import { computed, ref } from "vue";
import { afterEach, describe, expect, it, vi } from "vitest";

import SettingsAppearanceSection from "../src/components/SettingsAppearanceSection.vue";

const state = vi.hoisted(() => ({
  prefs: { upColor: "#15803d", downColor: "#b91c1c" },
  update: vi.fn(),
  reset: vi.fn(),
}));

vi.mock("../src/composables/useUIColorPreferences", () => ({
  useUIColorPreferences: () => ({
    prefs: ref(state.prefs),
    resolved: computed(() => state.prefs),
    update: state.update,
    reset: state.reset,
  }),
}));

afterEach(() => {
  state.prefs = { upColor: "#15803d", downColor: "#b91c1c" };
  state.update.mockReset();
  state.reset.mockReset();
});

describe("SettingsAppearanceSection", () => {
  it("updates the directional colors from either color picker or text input", async () => {
    const wrapper = mount(SettingsAppearanceSection);
    const inputs = wrapper.findAll("input");

    await inputs[0]!.setValue("#00ff00");
    await inputs[1]!.setValue("#119955");
    await inputs[2]!.setValue("#ff0000");
    await inputs[3]!.setValue("#992211");

    expect(state.update.mock.calls).toEqual([
      [{ upColor: "#00ff00" }],
      [{ upColor: "#119955" }],
      [{ downColor: "#ff0000" }],
      [{ downColor: "#992211" }],
    ]);
    expect(wrapper.get(".mt-5.grid.gap-3").attributes("style")).toContain("--tv-price-up: #15803d");
    expect(wrapper.get(".mt-5.grid.gap-3").attributes("style")).toContain("--tv-price-down: #b91c1c");
  });

  it("resets preferences and ignores malformed input events without a field target", async () => {
    const wrapper = mount(SettingsAppearanceSection);
    const setup = wrapper.vm.$.setupState as Record<string, (event: Event) => void>;

    setup.updateUpColor({ target: null } as unknown as Event);
    setup.updateDownColor({ target: null } as unknown as Event);
    expect(state.update).not.toHaveBeenCalled();

    await wrapper.get("button").trigger("click");
    expect(state.reset).toHaveBeenCalledTimes(1);
  });
});
