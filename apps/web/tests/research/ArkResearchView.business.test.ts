// @vitest-environment jsdom

import { flushPromises, mount } from "@vue/test-utils";
import { ref } from "vue";
import { beforeEach, describe, expect, it, vi } from "vitest";

const mocks = vi.hoisted(() => ({ useResearchFeature: vi.fn() }));

vi.mock("../../src/composables/useResearchFeature", () => ({
  useResearchFeature: mocks.useResearchFeature,
}));

import ArkResearchView from "../../src/components/research/ArkResearchView.vue";

function state(entries: Record<string, unknown>[] = []) {
  return {
    entries: ref(entries),
    total: ref(entries.length),
    loading: ref(false),
    error: ref(""),
    asOf: ref("2026-07-24"),
    hasMore: ref(false),
    loadingMore: ref(false),
    refresh: vi.fn(),
    loadMore: vi.fn(),
  };
}

function mountArk(feature: ReturnType<typeof state>, props = {}) {
  mocks.useResearchFeature.mockImplementationOnce((source: unknown) => {
    if (source != null && typeof source === "object" && "value" in source) {
      void (source as { value: unknown }).value;
    }
    return feature;
  });
  return mount(ArkResearchView, { props });
}

beforeEach(() => mocks.useResearchFeature.mockReset());

describe("ARK holdings and transaction workflow", () => {
  it("renders holding metrics, filters periods, emits rows, and paginates", async () => {
    const feature = state([
      {
        name: "Tesla",
        security: { instrumentId: "US.TSLA" },
        shares: 1_500_000,
        sharesChange: 10_000,
        marketValue: 300_000_000,
        weight: 12.5,
        weightChange: -0.4,
      },
      { symbol: "ROKU", sharesChange: -50, weightChange: 0 },
    ]);
    feature.hasMore.value = true;
    const wrapper = mountArk(feature, { market: "US", brokerId: "futu" });

    expect(wrapper.text()).toContain("ARK 持仓");
    expect(wrapper.text()).toContain("150.00万");
    expect(wrapper.text()).toContain("3.00亿");
    const row = wrapper.findAll("tbody tr")[0]!;
    await row.trigger("click");
    await row.trigger("dblclick");
    expect(wrapper.emitted("select")?.[0]?.[0]).toMatchObject({ name: "Tesla" });
    expect(wrapper.emitted("open")?.[0]?.[0]).toMatchObject({ name: "Tesla" });

    const holdingType = wrapper.get('select[aria-label="持仓变化类型"]');
    await holdingType.setValue("1");
    await wrapper.get('select[aria-label="统计周期"]').setValue("4");
    await wrapper.get(".ark-research__toolbar-meta button").trigger("click");
    await wrapper.get(".ark-research__more").trigger("click");
    expect(feature.refresh).toHaveBeenCalledOnce();
    expect(feature.loadMore).toHaveBeenCalledOnce();
    feature.loadingMore.value = true;
    await flushPromises();
    expect(wrapper.get(".ark-research__more").text()).toContain("加载中");
  });

  it("switches to transaction columns and resets holding filters", async () => {
    const feature = state([
      { instrumentId: "US.COIN", changeShares: 12_000, changeAmount: 2_400_000 },
      { name: "Block", changeShares: -100, changeAmount: -5_000 },
    ]);
    const wrapper = mountArk(feature);
    await wrapper.get('select[aria-label="持仓变化类型"]').setValue("3");
    await wrapper.get('select[aria-label="统计周期"]').setValue("2");
    await wrapper.setProps({ operation: "ark_transactions" });
    await flushPromises();

    expect(wrapper.text()).toContain("ARK 交易动态");
    expect(wrapper.text()).toContain("变动股数");
    expect(wrapper.text()).toContain("变动金额");
    expect(wrapper.get('select[aria-label="持仓变化类型"]').element).toHaveProperty("value", "0");
    expect(wrapper.get('select[aria-label="统计周期"]').element).toHaveProperty("value", "0");
    expect(wrapper.text()).toContain("+12000.00");
  });

  it("shows loading, errors, and an empty result without stale metadata", async () => {
    const feature = state();
    feature.total.value = 0;
    feature.asOf.value = "";
    feature.loading.value = true;
    const wrapper = mountArk(feature);
    expect(wrapper.text()).toContain("加载中");
    expect(wrapper.text()).not.toContain("更新");

    feature.loading.value = false;
    feature.error.value = "ARK 数据失败";
    await flushPromises();
    expect(wrapper.text()).toContain("ARK 数据失败");
    feature.error.value = "";
    await flushPromises();
    expect(wrapper.text()).toContain("暂无 ARK 数据");
  });
});
