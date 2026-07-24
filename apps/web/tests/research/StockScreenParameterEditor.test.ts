// @vitest-environment jsdom

import { mount } from "@vue/test-utils";
import { describe, expect, it } from "vitest";

import StockScreenParameterEditor from "../../src/components/research/StockScreenParameterEditor.vue";
import type {
  StockScreenFactorParameter,
  StockScreenFactorRef,
} from "../../src/components/research/stockScreenTypes";

describe("StockScreenParameterEditor", () => {
  it("edits union parameters without collapsing integer and array variants to text", async () => {
    const reference: StockScreenFactorRef = {
      factor: "option.stock_iv",
      params: {},
    };
    const parameters: StockScreenFactorParameter[] = [
      {
        name: "optionParam",
        type: "union",
        editorType: "union",
      },
    ];
    const wrapper = mount(StockScreenParameterEditor, {
      props: { reference, parameters, enums: {} },
    });

    await wrapper.get("select").setValue("2");
    await wrapper.get("input").setValue("42");
    expect(reference.params).toEqual({
      optionParamType: 2,
      optionParamInteger: 42,
    });

    await wrapper.get("select").setValue("3");
    await wrapper.get("input").setValue("5, 8, 13");
    expect(reference.params).toEqual({
      optionParamType: 3,
      optionParamIntegers: [5, 8, 13],
    });
  });

  it("uses catalog enums, bounds, steps and visibility dependencies", async () => {
    const reference: StockScreenFactorRef = {
      factor: "financial.roe",
      params: { term: 10, year: 2026 },
    };
    const parameters: StockScreenFactorParameter[] = [
      {
        name: "term",
        type: "integer",
        editorType: "select",
        enum: "term",
      },
      {
        name: "year",
        type: "integer",
        editorType: "number",
        minimum: 2000,
        maximum: 2100,
        step: 1,
        visibleWhen: { term: 10 },
      },
    ];
    const wrapper = mount(StockScreenParameterEditor, {
      props: {
        reference,
        parameters,
        enums: {
          term: [{ key: "latest", value: 10, label: "最新期" }],
        },
      },
    });

    expect(wrapper.get("select").element).toHaveProperty("value", "10");
    expect(wrapper.get('input[type="number"]').attributes()).toMatchObject({
      min: "2000",
      max: "2100",
      step: "1",
    });
    await wrapper.get("select").setValue("");
    expect(wrapper.find('input[type="number"]').exists()).toBe(false);
  });
});
