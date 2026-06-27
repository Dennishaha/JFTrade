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

  it("surfaces strategy.exit OCA as an explicit unsupported order boundary", () => {
    const workflow = createDefaultPineV6Workflow();
    const exit = createPineV6WorkflowBlock("strategy_exit");
    exit.params.oca_name = "group-a";
    exit.params.stop = "98";
    workflow.blocks = [exit];

    expect(assessPineV6Workflow(workflow)).toEqual(
      expect.arrayContaining([
        expect.objectContaining({
          code: "PINE_ORDER_OCA_UNSUPPORTED",
          severity: "warning",
        }),
      ]),
    );
  });

  it("can render strategy.exit without from_entry for auto exits", () => {
    const workflow = createDefaultPineV6Workflow();
    const exit = createPineV6WorkflowBlock("strategy_exit");
    exit.params.from_entry = "";
    exit.params.stop = "98";
    workflow.blocks = [exit];

    const script = buildPineV6WorkflowScript(workflow);

    expect(script).toContain('strategy.exit("Exit", stop=98)');
    expect(script).not.toContain("from_entry");
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

  it("surfaces visual-only Pine workflow blocks as explicit warnings", () => {
    const workflow = createDefaultPineV6Workflow();
    workflow.blocks = [
      createPineV6WorkflowBlock("plot"),
      createPineV6WorkflowBlock("alertcondition"),
    ];

    expect(assessPineV6Workflow(workflow)).toEqual(
      expect.arrayContaining([
        expect.objectContaining({
          code: "PINE_VISUAL_NOOP",
          severity: "warning",
        }),
        expect.objectContaining({
          code: "PINE_ALERTCONDITION_NOOP",
          severity: "warning",
        }),
      ]),
    );
  });

  it("renders qty_percent and trailing exit fields exposed by the main workflow designer", () => {
    const workflow = createDefaultPineV6Workflow();
    const entry = createPineV6WorkflowBlock("strategy_entry");
    entry.params.id = "Long";
    entry.params.qty_percent = "25";
    entry.params.comment = "open";
    entry.params.alert_message = "entry alert";
    entry.params.disable_alert = "false";
    const exit = createPineV6WorkflowBlock("strategy_exit");
    exit.params.id = "Trail";
    exit.params.from_entry = "Long";
    exit.params.qty_percent = "50";
    exit.params.trail_points = "100";
    exit.params.trail_offset = "50";
    exit.params.comment_trailing = "trail note";
    exit.params.alert_trailing = "trail alert";
    exit.params.disable_alert = "false";
    exit.params.when = "close > open";
    workflow.blocks = [entry, exit];

    const script = buildPineV6WorkflowScript(workflow);

    expect(script).toContain("strategy.entry(\"Long\", strategy.long, qty_percent=25, comment=\"open\", alert_message=\"entry alert\", disable_alert=false)");
    expect(script).toContain("strategy.exit(\"Trail\", from_entry=\"Long\", qty_percent=50, trail_points=100, trail_offset=50, comment_trailing=\"trail note\", alert_trailing=\"trail alert\", disable_alert=false, when=close > open)");
  });

  it("renders trail_price exits and flags conflicting trailing inputs", () => {
    const workflow = createDefaultPineV6Workflow();
    const exit = createPineV6WorkflowBlock("strategy_exit");
    exit.params.id = "TrailPrice";
    exit.params.from_entry = "Long";
    exit.params.trail_price = "103";
    exit.params.trail_offset = "50";
    workflow.blocks = [exit];

    const script = buildPineV6WorkflowScript(workflow);
    expect(script).toContain("strategy.exit(\"TrailPrice\", from_entry=\"Long\", trail_price=103, trail_offset=50)");

    exit.params.trail_points = "100";
    expect(assessPineV6Workflow(workflow)).toEqual(
      expect.arrayContaining([
        expect.objectContaining({
          code: "PINE_EXIT_TRAIL_PRICE_CONFLICT",
          severity: "error",
        }),
      ]),
    );
  });

  it("flags invalid workflow combinations before backend compile time", () => {
    const workflow = createDefaultPineV6Workflow();
    const exit = createPineV6WorkflowBlock("strategy_exit");
    exit.params.stop = "98";
    exit.params.trail_points = "100";
    workflow.blocks = [exit];

    expect(assessPineV6Workflow(workflow)).toEqual(
      expect.arrayContaining([
        expect.objectContaining({
          code: "PINE_EXIT_TRAIL_OFFSET_REQUIRED",
          severity: "error",
        }),
        expect.objectContaining({
          code: "PINE_EXIT_TRAIL_CONFLICT",
          severity: "error",
        }),
      ]),
    );
  });

  it("surfaces invalid boolean literals for alert and immediate flags", () => {
    const workflow = createDefaultPineV6Workflow();
    const entry = createPineV6WorkflowBlock("strategy_entry");
    entry.params.disable_alert = "maybe";
    const close = createPineV6WorkflowBlock("strategy_close");
    close.params.immediately = "later";
    const closeAll = createPineV6WorkflowBlock("strategy_close_all");
    closeAll.params.disable_alert = "off";
    workflow.blocks = [entry, close, closeAll];

    expect(assessPineV6Workflow(workflow)).toEqual(
      expect.arrayContaining([
        expect.objectContaining({ code: "PINE_ORDER_DISABLE_ALERT_BOOL", severity: "error" }),
        expect.objectContaining({ code: "PINE_CLOSE_IMMEDIATELY_BOOL", severity: "error" }),
        expect.objectContaining({ code: "PINE_CLOSE_ALL_DISABLE_ALERT_BOOL", severity: "error" }),
      ]),
    );
  });

  it("renders strategy.close limit orders exposed by the main workflow designer", () => {
    const workflow = createDefaultPineV6Workflow();
    const close = createPineV6WorkflowBlock("strategy_close");
    close.params.id = "Long";
    close.params.qty_percent = "50";
    close.params.limit = "101.5";
    close.params.stop = "99";
    close.params.comment = "scale out";
    workflow.blocks = [close];

    const script = buildPineV6WorkflowScript(workflow);

    expect(script).toContain('strategy.close("Long", qty_percent=50, limit=101.5, stop=99, comment="scale out")');
  });
});
