// @vitest-environment jsdom

import { flushPromises, mount } from "@vue/test-utils";
import { ref } from "vue";
import { beforeEach, describe, expect, it, vi } from "vitest";

const mocks = vi.hoisted(() => ({ useResearchFeature: vi.fn() }));

vi.mock("../../src/composables/useResearchFeature", () => ({
  useResearchFeature: mocks.useResearchFeature,
}));

import IndustryChainView from "../../src/components/research/IndustryChainView.vue";

function state(entries: Record<string, unknown>[] = []) {
  return {
    entries: ref(entries),
    metadata: ref<Record<string, unknown>>({}),
    total: ref(0),
    loading: ref(false),
    error: ref(""),
    asOf: ref(""),
    hasMore: ref(false),
    loadingMore: ref(false),
    refresh: vi.fn(),
    loadMore: vi.fn(),
  };
}

function mountIndustry(states: ReturnType<typeof state>[], props = {}) {
  for (const value of states) {
    mocks.useResearchFeature.mockImplementationOnce((source: unknown) => {
      if (typeof source === "function") source();
      return value;
    });
  }
  return mount(IndustryChainView, { props });
}

beforeEach(() => {
  mocks.useResearchFeature.mockReset();
});

describe("industry-chain research workflow", () => {
  it("filters the chain catalog, paginates, and selects a valid chain", async () => {
    const list = state([
      { chainId: "serial", name: "新能源", chainType: 1, marketCap: 2_000_000_000, stocksNum: 20 },
      { id: "parallel", chainName: "半导体", type: 2, description: "并列供应商", stocksNum: 12 },
      { chainId: "layers", name: "人工智能", chainType: 3 },
      { chainId: "future", name: "未来产业", chainType: 9 },
      { chainId: "custom", name: "自定义", chainType: "生态型" },
      { name: "缺少标识" },
    ]);
    list.hasMore.value = true;
    const wrapper = mountIndustry([list, state(), state(), state(), state()]);

    expect(wrapper.text()).toContain("串联型");
    expect(wrapper.text()).toContain("并列型");
    expect(wrapper.text()).toContain("上中下游型");
    expect(wrapper.text()).toContain("类型 9");
    expect(wrapper.text()).toContain("生态型");
    expect(wrapper.text()).toContain("未知类型");
    expect(wrapper.text()).toContain("请选择产业链");

    await wrapper.get('input[aria-label="搜索产业链"]').setValue("并列");
    expect(wrapper.text()).toContain("半导体");
    expect(wrapper.text()).not.toContain("新能源");
    await wrapper.get('input[aria-label="搜索产业链"]').setValue("");

    const loadMore = wrapper.get(".industry-chain__load-more");
    await loadMore.trigger("click");
    expect(list.loadMore).toHaveBeenCalledOnce();
    list.loadingMore.value = true;
    await flushPromises();
    expect(wrapper.get(".industry-chain__load-more").text()).toContain("加载中");
    expect(wrapper.get(".industry-chain__load-more").attributes("disabled")).toBeDefined();

    await wrapper.findAll(".industry-chain__master nav button").at(5)!.trigger("click");
    expect(wrapper.emitted("update:chainId")).toBeUndefined();
    await wrapper.findAll(".industry-chain__master nav button").at(0)!.trigger("click");
    expect(wrapper.emitted("update:chainId")?.at(-1)).toEqual(["serial"]);
    expect(wrapper.emitted("update:plateId")?.at(-1)).toEqual([""]);
  });

  it("navigates chain nodes, related chains, information, and securities", async () => {
    const list = state([
      {
        chainId: "ai/core",
        name: "人工智能",
        detail: "算力到应用",
        relationSecurityList: [
          { name: "Nvidia", security: { instrumentId: "US.NVDA" } },
          { name: "Microsoft", code: "MSFT" },
          null,
        ],
      },
    ]);
    list.total.value = 1;
    const detail = state();
    detail.asOf.value = "2026-07-24";
    detail.metadata.value = {
      nodeList: [
        { nodeId: "n3", name: "应用", layerSth: 3, plateId: "BK.APP" },
        { nodeId: "n1b", name: "芯片", layerSth: 1 },
        { nodeId: "n1a", name: "半导体", layerSth: 1, plateId: "BK.CHIP" },
        "invalid",
      ],
      informationList: [
        { title: "行业报告", url: "https://example.test/report" },
        null,
      ],
    };
    const related = state([
      { chainId: "robot", name: "机器人" },
      { name: "无标识产业链" },
    ]);
    const plateInfo = state([{ summary: "芯片板块覆盖设计与制造" }]);
    const plateStocks = state([
      { name: "台积电", security: { code: "TSM" } },
      { name: "英伟达", instrumentId: "US.NVDA" },
    ]);
    const wrapper = mountIndustry(
      [list, detail, related, plateInfo, plateStocks],
      { market: "US", brokerId: "futu", chainId: "ai/core" },
    );
    await flushPromises();

    expect(wrapper.text()).toContain("人工智能");
    expect(wrapper.text()).toContain("算力到应用");
    expect(wrapper.text()).toContain("行业报告");
    expect(wrapper.get(".industry-chain__information a").attributes("href")).toBe(
      "https://example.test/report",
    );
    const nodes = wrapper.findAll(".industry-chain__layers button");
    expect(nodes.map((node) => node.text())).toEqual([
      expect.stringContaining("半导体"),
      expect.stringContaining("芯片"),
      expect.stringContaining("应用"),
    ]);
    expect(nodes[1]!.attributes("disabled")).toBeDefined();
    await nodes[1]!.trigger("click");
    expect(wrapper.emitted("update:plateId")).toBeUndefined();

    await nodes[0]!.trigger("click");
    await flushPromises();
    expect(wrapper.emitted("update:plateId")?.at(-1)).toEqual(["BK.CHIP"]);
    expect(wrapper.text()).toContain("芯片板块覆盖设计与制造");
    expect(wrapper.text()).toContain("机器人");
    expect(wrapper.text()).toContain("台积电");

    const security = wrapper.findAll(".industry-chain__securities button")[0]!;
    await security.trigger("click");
    await security.trigger("dblclick");
    expect(wrapper.emitted("select")?.[0]?.[0]).toMatchObject({ name: "台积电" });
    expect(wrapper.emitted("open")?.[0]?.[0]).toMatchObject({ name: "台积电" });

    await wrapper.get(".industry-chain__detail-head button").trigger("click");
    expect(detail.refresh).toHaveBeenCalledOnce();
    const relatedButtons = wrapper.findAll(".industry-chain__related-list button");
    await relatedButtons[1]!.trigger("click");
    expect(wrapper.emitted("update:chainId")).toBeUndefined();
    await relatedButtons[0]!.trigger("click");
    expect(wrapper.emitted("update:chainId")?.at(-1)).toEqual(["robot"]);
    expect(wrapper.emitted("update:plateId")?.at(-1)).toEqual([""]);
  });

  it("falls back to envelope entries and surfaces each loading or error boundary", async () => {
    const list = state([{ chainId: "fallback", name: "回退产业链", securities: [] }]);
    const detail = state([
      { nodeId: "node", nodeName: "原料", layer: 2, plateId: "BK.RAW" },
      { title: "仅 entries 资讯", url: "https://example.test/entry" },
      { title: "无链接资讯" },
    ]);
    detail.metadata.value = { nodeList: "bad", informationList: null };
    const related = state();
    const plateInfo = state();
    const plateStocks = state();
    const wrapper = mountIndustry([list, detail, related, plateInfo, plateStocks], {
      chainId: "fallback",
    });
    await flushPromises();
    expect(wrapper.text()).toContain("原料");
    expect(wrapper.text()).toContain("仅 entries 资讯");

    detail.entries.value = [];
    await flushPromises();
    expect(wrapper.text()).toContain("暂无节点数据");

    list.loading.value = true;
    await flushPromises();
    expect(wrapper.text()).toContain("加载中");
    list.loading.value = false;
    list.error.value = "产业链目录失败";
    await flushPromises();
    expect(wrapper.text()).toContain("产业链目录失败");
    list.error.value = "";
    detail.loading.value = true;
    await flushPromises();
    expect(wrapper.text()).toContain("产业链详情加载中");
    detail.loading.value = false;
    detail.error.value = "产业链详情失败";
    await flushPromises();
    expect(wrapper.text()).toContain("产业链详情失败");

    detail.error.value = "";
    await wrapper.setProps({ plateId: "BK.RAW", chainId: "fallback-2" });
    related.loading.value = true;
    plateStocks.loading.value = true;
    await flushPromises();
    expect(wrapper.text()).toContain("板块成分加载中");
    related.loading.value = false;
    related.error.value = "关联产业链失败";
    plateStocks.loading.value = false;
    plateStocks.error.value = "板块成分失败";
    await flushPromises();
    expect(wrapper.text()).toContain("关联产业链失败");
    expect(wrapper.text()).toContain("板块成分失败");
  });
});
