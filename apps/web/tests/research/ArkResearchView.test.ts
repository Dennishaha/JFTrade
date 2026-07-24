// @vitest-environment jsdom

import { mount } from "@vue/test-utils";
import { afterEach, describe, expect, it, vi } from "vitest";

const mocks = vi.hoisted(() => ({ fetch: vi.fn() }));

vi.mock("../../src/composables/productFeatures", async (importOriginal) => {
  const actual =
    await importOriginal<typeof import("../../src/composables/productFeatures")>();
  return { ...actual, fetchProductFeature: mocks.fetch };
});

import ArkResearchView from "../../src/components/research/ArkResearchView.vue";
import { flushPromises } from "../productTestUtils";

function featureResult(entries: Record<string, unknown>[]) {
  return {
    provider: {
      brokerId: "futu",
      featureId: "research.ark",
      capability: "available" as const,
      selectionReason: "explicit",
      resolvedAt: "2026-07-23T00:00:00Z",
      asOf: "2026-07-23T00:00:00Z",
    },
    asOf: "2026-07-23T00:00:00Z",
    entries,
    total: entries.length,
  };
}

afterEach(() => {
  mocks.fetch.mockReset();
});

describe("ArkResearchView", () => {
  it("separates controls and metadata into wrapping toolbar groups", async () => {
    mocks.fetch.mockResolvedValue(
      featureResult([
        {
          instrumentId: "US.TSLA",
          name: "Tesla",
          shares: 1_000,
        },
      ]),
    );
    const wrapper = mount(ArkResearchView);
    await flushPromises();

    const toolbar = wrapper.get(".ark-research__toolbar");
    const controls = toolbar.get(".ark-research__toolbar-controls");
    const meta = toolbar.get(".ark-research__toolbar-meta");
    expect(controls.get("strong").text()).toBe("ARK 持仓");
    expect(controls.get('select[aria-label="持仓变化类型"]').exists()).toBe(
      true,
    );
    expect(meta.text()).toContain("1 条");
    expect(meta.text()).toContain("更新 2026-07-23T00:00:00Z");
    expect(meta.get("button").text()).toBe("刷新");

    await controls
      .get('select[aria-label="持仓变化类型"]')
      .setValue("1");
    await flushPromises();
    expect(controls.get('select[aria-label="统计周期"]').exists()).toBe(true);
    expect(
      mocks.fetch.mock.calls.some(([path]) =>
        String(path).includes("holdingType=1"),
      ),
    ).toBe(true);
  });
});
