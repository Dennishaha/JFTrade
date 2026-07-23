// @vitest-environment jsdom

import { mount } from "@vue/test-utils";
import { afterEach, describe, expect, it, vi } from "vitest";

import SectorHeatmap from "../../src/components/research/SectorHeatmap.vue";
import { squarifiedLayout } from "../../src/components/research/heatmapLayout";

describe("squarifiedLayout", () => {
  it("produces one rect per positive-weight item", () => {
    const layout = squarifiedLayout(
      [{ value: 10 }, { value: 5 }, { value: 3 }, { value: 2 }],
      400,
      300,
    );
    expect(layout).toHaveLength(4);
  });

  it("conserves total area (weight-proportional)", () => {
    const items = [{ value: 7 }, { value: 5 }, { value: 3 }, { value: 1 }];
    const layout = squarifiedLayout(items, 400, 300);
    const totalArea = layout.reduce(
      (sum, cell) => sum + cell.rect.width * cell.rect.height,
      0,
    );
    expect(totalArea).toBeGreaterThan(400 * 300 * 0.99);
    expect(totalArea).toBeLessThanOrEqual(400 * 300 * 1.0001);

    // 面积与权重成正比
    const byIndex = new Map(layout.map((cell) => [cell.index, cell]));
    const area0 = byIndex.get(0)!.rect.width * byIndex.get(0)!.rect.height;
    const area3 = byIndex.get(3)!.rect.width * byIndex.get(3)!.rect.height;
    expect(area0 / area3).toBeGreaterThan(6.5);
    expect(area0 / area3).toBeLessThan(7.5);
  });

  it("keeps every rect inside the container", () => {
    const layout = squarifiedLayout(
      [{ value: 100 }, { value: 50 }, { value: 20 }, { value: 10 }, { value: 5 }],
      337,
      211,
    );
    for (const cell of layout) {
      expect(cell.rect.x).toBeGreaterThanOrEqual(0);
      expect(cell.rect.y).toBeGreaterThanOrEqual(0);
      expect(cell.rect.x + cell.rect.width).toBeLessThanOrEqual(337 + 1e-6);
      expect(cell.rect.y + cell.rect.height).toBeLessThanOrEqual(211 + 1e-6);
      expect(cell.rect.width).toBeGreaterThan(0);
      expect(cell.rect.height).toBeGreaterThan(0);
    }
  });

  it("returns no rects for empty input or zero size", () => {
    expect(squarifiedLayout([], 100, 100)).toEqual([]);
    expect(squarifiedLayout([{ value: 1 }], 0, 100)).toEqual([]);
    expect(squarifiedLayout([{ value: 0 }, { value: -1 }], 100, 100)).toEqual([]);
  });
});

describe("SectorHeatmap", () => {
  const entries = [
    { instrumentId: "US.BK100", name: "科技", productClass: "plate", marketValue: 100, changeRate: 1.2 },
    { instrumentId: "US.BK200", name: "金融", productClass: "plate", marketValue: 60, changeRate: -0.8 },
    { instrumentId: "US.BK300", name: "医药", productClass: "plate", marketValue: 40, changeRate: 0 },
  ];

  it("renders one block per entry with positive weight", () => {
    const wrapper = mount(SectorHeatmap, {
      props: { entries, width: 480, height: 300 },
    });
    const blocks = wrapper.findAll(".sector-heatmap__block");
    expect(blocks).toHaveLength(3);
    expect(wrapper.text()).toContain("科技");
    expect(wrapper.text()).toContain("+1.20%");
    expect(wrapper.text()).toContain("-0.80%");
  });

  it("falls back through marketValue → turnover → volume for weight", () => {
    const wrapper = mount(SectorHeatmap, {
      props: {
        entries: [
          { name: "甲", turnover: 50, changeRate: 1 },
          { name: "乙", volume: 30, changeRate: -1 },
          { name: "丙", changeRate: 5 }, // 无任何权重字段，不渲染
        ],
        width: 400,
        height: 300,
      },
    });
    expect(wrapper.findAll(".sector-heatmap__block")).toHaveLength(2);
    expect(wrapper.text()).not.toContain("丙");
  });

  it("respects an explicit weightField", () => {
    const wrapper = mount(SectorHeatmap, {
      props: {
        entries: [{ name: "甲", floatCap: 10, marketValue: 999 }],
        weightField: "floatCap",
        width: 400,
        height: 300,
      },
    });
    const blocks = wrapper.findAll(".sector-heatmap__block");
    expect(blocks).toHaveLength(1);
    expect(blocks[0]!.attributes("title")).toContain("甲");
  });

  it("shows an empty state without entries", () => {
    const wrapper = mount(SectorHeatmap, {
      props: { entries: [], width: 400, height: 300 },
    });
    expect(wrapper.text()).toContain("暂无数据");
    expect(wrapper.findAll(".sector-heatmap__block")).toHaveLength(0);
  });

  it("emits select with the entry when a block is clicked", async () => {
    const wrapper = mount(SectorHeatmap, {
      props: { entries, width: 480, height: 300 },
    });
    await wrapper.findAll(".sector-heatmap__block")[0]!.trigger("click");
    const events = wrapper.emitted("select");
    expect(events).toHaveLength(1);
    expect(entries).toContainEqual(events![0]![0]);
  });

  it("uses code and unnamed fallbacks across compact block sizes", () => {
    const wrapper = mount(SectorHeatmap, {
      props: {
        entries: [
          { code: "CODE", marketValue: 10, changeRate: null },
          { marketValue: 9, changeRate: 0 },
          { name: "跌", marketValue: 8, changeRate: -4 },
        ],
        width: 120,
        height: 35,
      },
    });
    const blocks = wrapper.findAll(".sector-heatmap__block");
    expect(blocks).toHaveLength(3);
    expect(blocks[0]!.attributes("title")).toContain("CODE");
    expect(blocks.some((block) => block.attributes("title").includes("未命名"))).toBe(
      true,
    );
    expect(
      blocks.some((block) =>
        block.classes().includes("sector-heatmap__block--none"),
      ),
    ).toBe(true);
  });

  it("observes intrinsic width and disconnects on unmount", async () => {
    let callback: ResizeObserverCallback | undefined;
    const observe = vi.fn();
    const disconnect = vi.fn();
    vi.stubGlobal(
      "ResizeObserver",
      class {
        constructor(next: ResizeObserverCallback) {
          callback = next;
        }
        observe = observe;
        disconnect = disconnect;
        unobserve = vi.fn();
      },
    );
    const wrapper = mount(SectorHeatmap, {
      props: {
        entries: [{ name: "动态", marketValue: 10, changeRate: 1 }],
        height: 80,
      },
    });
    callback?.(
      [{ contentRect: { width: 96 } } as ResizeObserverEntry],
      {} as ResizeObserver,
    );
    await wrapper.vm.$nextTick();
    expect(observe).toHaveBeenCalled();
    expect(wrapper.findAll(".sector-heatmap__block")).toHaveLength(1);
    wrapper.unmount();
    expect(disconnect).toHaveBeenCalled();
  });
});

afterEach(() => {
  vi.unstubAllGlobals();
});
