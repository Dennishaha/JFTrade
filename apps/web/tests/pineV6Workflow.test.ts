import { describe, expect, it } from "vitest";

import {
  assessPineV6Workflow,
  buildPineV6WorkflowScript,
  createDefaultPineV6Workflow,
  createPineV6WorkflowBlock,
} from "../src/features/pineV6Workflow";

describe("pineV6Workflow", () => {
  it("renders Pine v6 workflow blocks to bar-close strategy source", () => {
    const workflow = createDefaultPineV6Workflow();
    const script = buildPineV6WorkflowScript(workflow);

    expect(script).toContain("//@version=6");
    expect(script).toContain("strategy(");
    expect(script).toContain("barClosed = barstate.isconfirmed");
    expect(script).toContain("if barClosed");
    expect(script).toContain("strategy.entry(\"Long\", strategy.long");
    expect(script).toContain("strategy.close(\"Long\", when=ta.crossunder(fast, slow))");
  });

  it("surfaces OCA as an explicit unsupported order boundary", () => {
    const workflow = createDefaultPineV6Workflow();
    const order = createPineV6WorkflowBlock("strategy_order");
    order.params.oca_name = "group-a";
    workflow.blocks = [order];

    expect(assessPineV6Workflow(workflow)).toEqual(
      expect.arrayContaining([
        expect.objectContaining({
          code: "PINE_ORDER_OCA_UNSUPPORTED",
          severity: "warning",
        }),
      ]),
    );
  });

  it("renders request.security and nested if branches", () => {
    const workflow = createDefaultPineV6Workflow();
    workflow.blocks = [
      {
        ...createPineV6WorkflowBlock("request_security"),
        params: {
          name: "dailyClose",
          symbol: "syminfo.tickerid",
          timeframe: "D",
          expression: "close",
        },
      },
      {
        ...createPineV6WorkflowBlock("if"),
        params: { condition: "close > dailyClose" },
        thenBlocks: [createPineV6WorkflowBlock("strategy_entry")],
        elseBlocks: [createPineV6WorkflowBlock("log")],
      },
    ];

    const script = buildPineV6WorkflowScript(workflow);

    expect(script).toContain("dailyClose = request.security(\"syminfo.tickerid\", \"D\", close)");
    expect(script).toContain("if close > dailyClose");
    expect(script).toContain("else");
    expect(script).toContain("log.info(");
  });
});
