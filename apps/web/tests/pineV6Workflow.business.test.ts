import { describe, expect, it } from "vitest";

import {
  assessPineV6Workflow,
  buildPineV6WorkflowScript,
  createDefaultPineV6Workflow,
  createPineV6WorkflowBlock,
  normalizePineV6Workflow,
  PINE_V6_WORKFLOW_ENGINE,
} from "../src/features/pineV6Workflow";

describe("pineV6Workflow business boundaries", () => {
  it("normalizes malformed workflow payloads back to supported runtime defaults", () => {
    const fallback = normalizePineV6Workflow(null);
    expect(fallback.engine).toBe(PINE_V6_WORKFLOW_ENGINE);
    expect(fallback.inputs).toHaveLength(2);

    const normalized = normalizePineV6Workflow({
      engine: PINE_V6_WORKFLOW_ENGINE,
      version: "bad",
      declaration: {
        title: " Strategy ",
        overlay: false,
        initialCapital: "",
        currency: "",
        pyramiding: "oops",
        defaultQtyType: "",
        defaultQtyValue: "NaN",
        calcOnEveryTick: true,
        processOrdersOnClose: true,
      },
      inputs: [
        { id: "", name: "123 bad", title: "", type: "bool", defaultValue: "true" },
        { id: "input-2", name: "tf value", title: "TF", type: "timeframe", defaultValue: "" },
        { id: "input-3", name: "fallback", title: "Fallback", type: "mystery", defaultValue: "" },
      ],
      blocks: [
        { id: "", kind: "if", title: "", params: { condition: "" }, thenBlocks: "bad", elseBlocks: "bad" },
      ],
      runtimeBindingDraft: {
        market: " hk ",
        code: " 00700 ",
        interval: "15m",
        executionMode: "notify_only",
        brokerAccountKey: "acct-1",
        useExtendedHours: true,
        runtimeRisk: { dailyLoss: 1000 },
      },
    });

    expect(normalized.version).toBe(1);
    expect(normalized.declaration).toMatchObject({
      title: "Strategy",
      overlay: false,
      initialCapital: 100000,
      currency: "",
      pyramiding: 0,
      defaultQtyType: "strategy.percent_of_equity",
      defaultQtyValue: 10,
      calcOnEveryTick: true,
      processOrdersOnClose: true,
    });
    expect(normalized.inputs).toEqual([
      expect.objectContaining({ name: "inputValue", title: "123 bad", type: "bool", defaultValue: "true" }),
      expect.objectContaining({ name: "tf_value", title: "TF", type: "timeframe", defaultValue: "1" }),
      expect.objectContaining({ name: "fallback", title: "Fallback", type: "int", defaultValue: "1" }),
    ]);
    expect(normalized.blocks[0]).toMatchObject({
      kind: "if",
      title: "if",
      thenBlocks: [],
      elseBlocks: [],
    });
    expect(normalized.runtimeBindingDraft).toMatchObject({
      market: "HK",
      code: "00700",
      interval: "15m",
      executionMode: "notify_only",
      brokerAccountKey: "acct-1",
      useExtendedHours: true,
      runtimeRisk: { dailyLoss: 1000 },
    });
  });

  it("renders every supported workflow block family with runtime-safe defaults", () => {
    const workflow = createDefaultPineV6Workflow();
    workflow.inputs = [
      { id: "float", name: "threshold", title: "Threshold", type: "float", defaultValue: "2.5" },
      { id: "bool", name: "confirm", title: "Confirm", type: "bool", defaultValue: "true" },
      { id: "string", name: "note", title: "Note", type: "string", defaultValue: "hello" },
      { id: "source", name: "baseline", title: "Baseline", type: "source", defaultValue: "hl2" },
      { id: "time", name: "reset", title: "Reset", type: "time", defaultValue: "timestamp(2026, 1, 1, 0, 0)" },
      { id: "timeframe", name: "higher_tf", title: "Higher TF", type: "timeframe", defaultValue: "60" },
      { id: "color", name: "theme", title: "Theme", type: "color", defaultValue: "color.red" },
      { id: "int", name: "1 invalid", title: "", type: "int", defaultValue: "7" },
    ];
    workflow.blocks = [
      { ...createPineV6WorkflowBlock("series_assign"), params: { name: " signal value ", expression: "" } },
      { ...createPineV6WorkflowBlock("var_state"), params: { name: "state value", initial: "" } },
      {
        ...createPineV6WorkflowBlock("if"),
        params: { condition: "" },
        thenBlocks: [],
        elseBlocks: [],
      },
      { ...createPineV6WorkflowBlock("request_security"), params: { name: "bad name", symbol: "", timeframe: "", expression: "" } },
      { ...createPineV6WorkflowBlock("array_op"), params: { name: "values", mode: "push", value: "" } },
      { ...createPineV6WorkflowBlock("array_op"), params: { name: "values", mode: "median", output: "" } },
      { ...createPineV6WorkflowBlock("array_op"), params: { name: "values", mode: "new_float" } },
      { ...createPineV6WorkflowBlock("strategy_order"), params: { id: "Net", direction: "strategy.short", qty_percent: "10", stop: "99", comment: "net", alert_message: "alert", disable_alert: "false", when: "close > open" } },
      { ...createPineV6WorkflowBlock("strategy_close_all"), params: { immediately: "true", comment: "flatten", alert_message: "done", disable_alert: "false" } },
      createPineV6WorkflowBlock("strategy_cancel"),
      createPineV6WorkflowBlock("strategy_cancel_all"),
      { ...createPineV6WorkflowBlock("strategy_risk_allow_entry_in"), params: { direction: "invalid" } },
      { ...createPineV6WorkflowBlock("strategy_risk_max_drawdown"), params: { value: "", type: "", alert_message: "dd" } },
      { ...createPineV6WorkflowBlock("strategy_risk_max_intraday_loss"), params: { value: "", type: "", alert_message: "idl" } },
      { ...createPineV6WorkflowBlock("strategy_risk_max_intraday_filled_orders"), params: { count: "", alert_message: "filled" } },
      createPineV6WorkflowBlock("strategy_risk_max_position_size"),
      createPineV6WorkflowBlock("strategy_risk_max_cons_loss_days"),
      { ...createPineV6WorkflowBlock("alertcondition"), params: { condition: "", title: "", message: "" } },
      createPineV6WorkflowBlock("log"),
      { ...createPineV6WorkflowBlock("strategy_entry"), enabled: false, params: { id: "Hidden", direction: "strategy.long" } },
    ];

    const script = buildPineV6WorkflowScript(workflow);

    expect(script).toContain('threshold = input.float(2.5, "Threshold")');
    expect(script).toContain('confirm = input.bool(true, "Confirm")');
    expect(script).toContain('note = input.string("hello", "Note")');
    expect(script).toContain('baseline = input.source(hl2, "Baseline")');
    expect(script).toContain('reset = input.time(timestamp(2026, 1, 1, 0, 0), "Reset")');
    expect(script).toContain('higher_tf = input.timeframe("60", "Higher TF")');
    expect(script).toContain('theme = input.color(color.red, "Theme")');
    expect(script).toContain('inputValue = input.int(7, "1 invalid")');
    expect(script).toContain("signal_value = close");
    expect(script).toContain("var state_value = na");
    expect(script).toContain("if false");
    expect(script).toContain("// then");
    expect(script).toContain('bad_name = request.security("syminfo.tickerid", "D", close)');
    expect(script).toContain("array.push(values, close)");
    expect(script).toContain("medianValue = array.median(values)");
    expect(script).toContain("var values = array.new_float()");
    expect(script).toContain('strategy.order("Net", strategy.short, qty_percent=10, stop=99, comment="net", alert_message="alert", disable_alert=false, when=close > open)');
    expect(script).toContain('strategy.close_all(immediately=true, comment="flatten", alert_message="done", disable_alert=false)');
    expect(script).toContain('strategy.cancel("Order")');
    expect(script).toContain("strategy.cancel_all()");
    expect(script).toContain("strategy.risk.allow_entry_in(strategy.direction.all)");
    expect(script).toContain('strategy.risk.max_drawdown(10, strategy.percent_of_equity, alert_message="dd")');
    expect(script).toContain('strategy.risk.max_intraday_loss(10, strategy.percent_of_equity, alert_message="idl")');
    expect(script).toContain('strategy.risk.max_intraday_filled_orders(10, alert_message="filled")');
    expect(script).toContain("strategy.risk.max_position_size(1)");
    expect(script).toContain("strategy.risk.max_cons_loss_days(3)");
    expect(script).toContain('alertcondition(false, title="提醒", message="Pine v6 工作流提醒")');
    expect(script).toContain('log.info("Pine v6 工作流")');
    expect(script).not.toContain("Hidden");
  });

  it("surfaces invalid workflow combinations while ignoring disabled draft blocks", () => {
    const workflow = createDefaultPineV6Workflow();
    workflow.declaration.title = " ";

    const disabled = createPineV6WorkflowBlock("strategy_entry");
    disabled.enabled = false;
    disabled.params.qty = "10";
    disabled.params.qty_percent = "5";

    const entry = createPineV6WorkflowBlock("strategy_entry");
    entry.params.qty = "10";
    entry.params.qty_percent = "5";

    const conditional = createPineV6WorkflowBlock("if");
    conditional.params.condition = "";

    const exit = createPineV6WorkflowBlock("strategy_exit");
    exit.params.qty = "10";
    exit.params.qty_percent = "5";

    const close = createPineV6WorkflowBlock("strategy_close");
    close.params.qty = "2";
    close.params.qty_percent = "50";

    workflow.blocks = [disabled, entry, conditional, exit, close];

    expect(assessPineV6Workflow(workflow)).toEqual(
      expect.arrayContaining([
        expect.objectContaining({ code: "PINE_ORDER_QTY_CONFLICT", severity: "error" }),
        expect.objectContaining({ code: "PINE_WORKFLOW_EMPTY_IF", severity: "error" }),
        expect.objectContaining({ code: "PINE_EXIT_QTY_CONFLICT", severity: "error" }),
        expect.objectContaining({ code: "PINE_EXIT_TRIGGER_REQUIRED", severity: "error" }),
        expect.objectContaining({ code: "PINE_CLOSE_QTY_CONFLICT", severity: "error" }),
      ]),
    );
  });
});
