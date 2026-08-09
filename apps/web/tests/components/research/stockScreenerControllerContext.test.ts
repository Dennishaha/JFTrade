// @vitest-environment jsdom

import { mount } from "@vue/test-utils";
import { describe, expect, it, vi } from "vitest";
import { defineComponent, h } from "vue";

import { useStockScreenerControllerContext } from "@/components/research/stockScreenerControllerContext";

describe("stock screener controller context", () => {
  it("throws when no controller is provided on the component tree", () => {
    const spy = vi.spyOn(console, "error").mockImplementation(() => {});
    mount(
      defineComponent({
        setup() {
          expect(() => useStockScreenerControllerContext()).toThrow(
            "Stock screener controller is not available",
          );
          return () => h("div");
        },
      }),
    );
    spy.mockRestore();
  });
});
