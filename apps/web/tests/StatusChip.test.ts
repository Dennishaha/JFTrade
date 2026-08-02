// @vitest-environment jsdom

import { mount } from "@vue/test-utils";
import { describe, expect, it } from "vitest";
import { defineComponent } from "vue";

import StatusChip from "../src/components/shared/StatusChip.vue";

const chipProbe = defineComponent({
  props: ["color", "size", "variant"],
  template: "<span class='chip-probe' :data-color='color' :data-size='size' :data-variant='variant'><slot /></span>",
});

function mountChip(props: Record<string, unknown>) {
  return mount(StatusChip, {
    props,
    global: { stubs: { "v-chip": chipProbe } },
  });
}

describe("StatusChip", () => {
  it("colors known statuses from the shared status tone and shows the raw status text", () => {
    const wrapper = mountChip({ status: "COMPLETED" });
    const chip = wrapper.get(".chip-probe");
    expect(chip.attributes("data-color")).toBe("success");
    expect(chip.attributes("data-size")).toBe("small");
    expect(chip.attributes("data-variant")).toBe("tonal");
    expect(chip.text()).toBe("COMPLETED");
  });

  it("normalizes lowercase and hyphenated status words", () => {
    expect(mountChip({ status: "timed-out" }).get(".chip-probe").attributes("data-color")).toBe("error");
    expect(mountChip({ status: "in_progress" }).get(".chip-probe").attributes("data-color")).toBe("info");
  });

  it("falls back to the default color for unmapped statuses", () => {
    expect(mountChip({ status: "CANCELLED" }).get(".chip-probe").attributes("data-color")).toBe("default");
    expect(mountChip({ status: "WHATEVER" }).get(".chip-probe").attributes("data-color")).toBe("default");
  });

  it("lets domains override color and label while keeping chip defaults", () => {
    const wrapper = mountChip({ status: "CANCELLED", color: "grey", label: "已取消", size: "x-small", variant: "outlined" });
    const chip = wrapper.get(".chip-probe");
    expect(chip.attributes("data-color")).toBe("grey");
    expect(chip.attributes("data-size")).toBe("x-small");
    expect(chip.attributes("data-variant")).toBe("outlined");
    expect(chip.text()).toBe("已取消");
  });
});
