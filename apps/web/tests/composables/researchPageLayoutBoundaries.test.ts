// @vitest-environment jsdom

import { mount } from "@vue/test-utils";
import { afterEach, describe, expect, it, vi } from "vitest";
import { defineComponent, h, ref } from "vue";

import { clampResearchPaneSizesForWidth } from "@/composables/research/useResearchViewState";
import { useResearchPageLayout } from "@/pages/useResearchPageLayout";

afterEach(() => {
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
  window.localStorage.clear();
  window.sessionStorage.clear();
});

describe("research page layout boundaries", () => {
  it("ignores width syncs while the rail is active and when pane sizes are stable", () => {
    const api = mountLayout();

    api.marketRailDrawer.value = true;
    api.syncResearchPageWidth(1200);
    expect(api.marketPaneSizes.value).toEqual([72, 28]);

    api.marketRailDrawer.value = false;
    api.marketRailCollapsed.value = true;
    api.syncResearchPageWidth(1200);
    expect(api.marketPaneSizes.value).toEqual([72, 28]);

    api.marketRailCollapsed.value = false;
    const stable = clampResearchPaneSizesForWidth([72, 28], 1200);
    api.marketPaneSizes.value = stable;
    api.syncResearchPageWidth(1200);
    expect(api.marketPaneSizes.value).toEqual(stable);
  });

  it("clamps and persists pane sizes when the page width changes", () => {
    const api = mountLayout();
    api.syncResearchPageWidth(1200);
    expect(api.marketPaneSizes.value).not.toEqual([72, 28]);
    expect(api.researchPaneBounds.value.leftMinSize).toBeGreaterThan(0);
  });

  it("falls back to the element width when a resize entry has no content rect", () => {
    const api = mountLayoutWithElement();
    expect(api.researchPageWidth.value).toBe(1200);
  });
});

function mountLayout() {
  let api!: ReturnType<typeof useResearchPageLayout>;
  mount(
    defineComponent({
      setup() {
        api = useResearchPageLayout(ref(null));
        return () => h("div");
      },
    }),
  );
  return api;
}

function mountLayoutWithElement() {
  vi.stubGlobal(
    "ResizeObserver",
    class {
      private callback: ResizeObserverCallback;

      constructor(callback: ResizeObserverCallback) {
        this.callback = callback;
      }

      observe(): void {
        this.callback(
          [{ contentRect: { width: undefined } }] as unknown as ResizeObserverEntry[],
          this as unknown as ResizeObserver,
        );
      }

      disconnect(): void {}
    },
  );
  vi.spyOn(Element.prototype, "getBoundingClientRect").mockReturnValue({
    width: 1200,
  } as DOMRect);
  let api!: ReturnType<typeof useResearchPageLayout>;
  mount(
    defineComponent({
      setup() {
        api = useResearchPageLayout(ref(null));
        return () => h("div", { ref: api.researchPageRef });
      },
    }),
  );
  return api;
}
