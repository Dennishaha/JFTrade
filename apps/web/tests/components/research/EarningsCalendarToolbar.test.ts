// @vitest-environment jsdom

import { mount } from "@vue/test-utils";
import { describe, expect, it } from "vitest";

import EarningsCalendarToolbar from "../../../src/components/research/EarningsCalendarToolbar.vue";
import SegmentedControl from "../../../src/components/shared/SegmentedControl.vue";

describe("EarningsCalendarToolbar", () => {
  it("accepts only supported calendar modes", async () => {
    const wrapper = mount(EarningsCalendarToolbar, {
      props: {
        mode: "day",
        anchorKey: "2026-08-03",
        availableSortOptions: [
          { value: "hot", label: "热门", optionOnly: false },
        ],
        selectedSort: "hot",
        selectedSortLabel: "热门",
        activeFilterCount: 0,
      },
    });

    const control = wrapper.findComponent(SegmentedControl);
    control.vm.$emit("update:modelValue", "invalid");
    await wrapper.vm.$nextTick();
    expect(wrapper.emitted("update:mode")).toBeUndefined();

    control.vm.$emit("update:modelValue", "week");
    await wrapper.vm.$nextTick();
    expect(wrapper.emitted("update:mode")).toEqual([["week"]]);
    wrapper.unmount();
  });
});
