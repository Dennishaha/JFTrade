import { describe, expect, it } from "vitest";

import { PINE_V6_BLOCK_KINDS } from "../src/features/pineV6Workflow";
import {
  buildPineSourceStructureIndex,
  parseSourceToBlocks,
  buildWorkflowSnapshotFromSource,
  deleteSourceBlock,
  duplicateSourceBlock,
  insertSourceBlock,
  moveSourceBlock,
  renderBlockToSource,
  renderDefaultSourceBlock,
  replaceSourceRange,
  replaceSourceBlockKind,
  updateInstructionBlockParam,
} from "../src/features/pineSourceStructureIndex";

describe("pineSourceStructureIndex", () => {
  it("classifies Pine source into visual structure nodes and raw anchors", () => {
    const nodes = buildPineSourceStructureIndex(`//@version=6
strategy("Indexed", overlay=true)
fastLen = input.int(12, "Fast")
fast = ta.ema(close, fastLen)
if ta.crossover(close, fast)
    strategy.entry("Long", strategy.long)
import TradingView/ta/7 as ta7
`);

    expect(nodes.map((node) => node.kind)).toEqual([
      "version",
      "strategy",
      "input",
      "assignment",
      "condition",
      "order",
      "library",
    ]);
    expect(nodes.find((node) => node.kind === "order")).toMatchObject({
      label: "入场订单",
      depth: 1,
      match: expect.objectContaining({ type: "instruction" }),
    });
    expect(nodes.find((node) => node.kind === "library")).toMatchObject({
      label: "导入库 ta7",
      match: { type: "raw" },
      lineRange: { start: 7, end: 7 },
    });
  });

  it("edits a matched instruction block by replacing only its source range", () => {
    const source = `//@version=6
strategy("Indexed", overlay=true)
if close > open
    strategy.entry("Long", strategy.long)
import TradingView/ta/7 as ta7
`;
    const order = buildPineSourceStructureIndex(source).find((node) => node.kind === "order");
    expect(order).toBeDefined();

    const updated = updateInstructionBlockParam(
      updateInstructionBlockParam(order!, "direction", "strategy.short"),
      "qty",
      "25",
    );
    const nextSource = replaceSourceRange(source, order!.sourceRange, renderBlockToSource(updated));

    expect(nextSource).toContain('strategy.entry("Long", strategy.short, qty=25)');
    expect(nextSource).toContain("import TradingView/ta/7 as ta7");
  });

  it("preserves richer strategy order parameters when editing matched blocks", () => {
    const source = `strategy.entry("Long", strategy.long, qty=1, oca_name="main", oca_type=strategy.oca.cancel, comment="enter", alert_message="go", disable_alert=true)
strategy.exit("Exit", from_entry="Long", qty_percent=50, profit=100, loss=50, trail_points=20, trail_offset=5, comment_profit="tp", comment_loss="sl", alert_trailing="trail")
strategy.close("Long", qty_percent=25, limit=101.5, stop=99, comment="close", alert_message="done", immediately=true, disable_alert=false)
`;
    const nodes = buildPineSourceStructureIndex(source);
    const entry = nodes.find((node) => node.raw.includes("strategy.entry"))!;
    const exit = nodes.find((node) => node.raw.includes("strategy.exit"))!;
    const close = nodes.find((node) => node.raw.includes("strategy.close"))!;

    const nextEntry = renderBlockToSource(updateInstructionBlockParam(entry, "qty", "2"));
    expect(nextEntry).toContain('strategy.entry("Long", strategy.long, qty=2');
    expect(nextEntry).toContain('oca_name="main"');
    expect(nextEntry).toContain("oca_type=strategy.oca.cancel");
    expect(nextEntry).toContain('comment="enter"');
    expect(nextEntry).toContain('alert_message="go"');
    expect(nextEntry).toContain("disable_alert=true");

    const nextExit = renderBlockToSource(updateInstructionBlockParam(exit, "trail_offset", "8"));
    expect(nextExit).toContain('strategy.exit("Exit", from_entry="Long"');
    expect(nextExit).toContain("qty_percent=50");
    expect(nextExit).toContain("profit=100");
    expect(nextExit).toContain("loss=50");
    expect(nextExit).toContain("trail_points=20");
    expect(nextExit).toContain("trail_offset=8");
    expect(nextExit).toContain('comment_profit="tp"');
    expect(nextExit).toContain('comment_loss="sl"');
    expect(nextExit).toContain('alert_trailing="trail"');

    const nextClose = renderBlockToSource(updateInstructionBlockParam(close, "comment", "flat"));
    expect(nextClose).toContain('strategy.close("Long"');
    expect(nextClose).toContain("qty_percent=25");
    expect(nextClose).toContain("limit=101.5");
    expect(nextClose).toContain("stop=99");
    expect(nextClose).toContain('comment="flat"');
    expect(nextClose).toContain('alert_message="done"');
    expect(nextClose).toContain("immediately=true");
    expect(nextClose).toContain("disable_alert=false");
  });

  it("preserves omitted strategy.exit from_entry when editing matched blocks", () => {
    const source = `strategy.exit("Auto", stop=98, qty_percent=25)\n`;
    const [exit] = buildPineSourceStructureIndex(source);

    const nextExit = renderBlockToSource(updateInstructionBlockParam(exit!, "stop", "97"));

    expect(nextExit).toBe('strategy.exit("Auto", qty_percent=25, stop=97)');
    expect(nextExit).not.toContain("from_entry");
  });

  it("preserves extra named Pine call parameters when editing matched blocks", () => {
    const source = `strategy("Extra Args", overlay=true, commission_type=strategy.commission.percent, commission_value=0.1, slippage=2)
daily = request.security(syminfo.tickerid, "D", close, gaps=barmerge.gaps_on, lookahead=barmerge.lookahead_off, calc_bars_count=100)
plot(close, "Close", color=color.red, linewidth=2, display=display.all)
alertcondition(close > open, "Bull", "go", display=display.all)
`;
    const nodes = buildPineSourceStructureIndex(source);
    const strategy = nodes.find((node) => node.kind === "strategy")!;
    const request = nodes.find((node) => node.kind === "request")!;
    const plot = nodes.find((node) => node.raw.startsWith("plot("))!;
    const alert = nodes.find((node) => node.raw.startsWith("alertcondition("))!;

    const nextStrategy = renderBlockToSource(updateInstructionBlockParam(strategy, "title", "Edited"));
    expect(nextStrategy).toContain('strategy("Edited"');
    expect(nextStrategy).toContain("commission_type=strategy.commission.percent");
    expect(nextStrategy).toContain("commission_value=0.1");
    expect(nextStrategy).toContain("slippage=2");

    const nextRequest = renderBlockToSource(updateInstructionBlockParam(request, "timeframe", "W"));
    expect(nextRequest).toContain('request.security("syminfo.tickerid", "W", close');
    expect(nextRequest).toContain("gaps=barmerge.gaps_on");
    expect(nextRequest).toContain("lookahead=barmerge.lookahead_off");
    expect(nextRequest).toContain("calc_bars_count=100");

    const nextPlot = renderBlockToSource(updateInstructionBlockParam(plot, "series", "open"));
    expect(nextPlot).toContain('plot(open, title="Close", color=color.red');
    expect(nextPlot).toContain("linewidth=2");
    expect(nextPlot).toContain("display=display.all");

    const nextAlert = renderBlockToSource(updateInstructionBlockParam(alert, "condition", "close < open"));
    expect(nextAlert).toContain('alertcondition(close < open, title="Bull", message="go"');
    expect(nextAlert).toContain("display=display.all");
  });

  it("matches common multiline calls as editable structure blocks", () => {
    const source = `//@version=6
strategy(
    "Multiline",
    overlay=true,
    default_qty_value=15
)
fastLen = input.int(
    12,
    "Fast"
)
if ta.crossover(close, ta.ema(close, fastLen))
    strategy.entry(
        "Long",
        strategy.long,
        qty=5
    )
`;

    const nodes = buildPineSourceStructureIndex(source);
    const strategy = nodes.find((node) => node.kind === "strategy");
    const input = nodes.find((node) => node.kind === "input");
    const order = nodes.find((node) => node.kind === "order");

    expect(strategy).toMatchObject({
      lineRange: { start: 2, end: 6 },
      match: expect.objectContaining({ type: "strategy" }),
    });
    expect(input).toMatchObject({
      lineRange: { start: 7, end: 10 },
      match: expect.objectContaining({ type: "input" }),
    });
    expect(order).toMatchObject({
      lineRange: { start: 12, end: 16 },
      match: expect.objectContaining({ type: "instruction" }),
    });

    const updated = updateInstructionBlockParam(order!, "qty", "10");
    const nextSource = replaceSourceRange(source, order!.sourceRange, renderBlockToSource(updated));

    expect(nextSource).toContain('strategy.entry("Long", strategy.long, qty=10)');
    expect(nextSource).not.toContain("qty=5");

    const snapshot = buildWorkflowSnapshotFromSource(source);
    expect(snapshot.declaration.title).toBe("Multiline");
    expect(snapshot.declaration.defaultQtyValue).toBe(15);
    expect(snapshot.inputs[0]).toMatchObject({ name: "fastLen", defaultValue: "12" });
  });

  it("matches timeframe and color inputs as editable source parameters", () => {
    const source = `higherTf = input.timeframe("D", "Higher TF")
plotColor = input.color(color.teal, "Plot Color")
`;

    const nodes = buildPineSourceStructureIndex(source);
    const timeframe = nodes.find((node) => node.raw.includes("input.timeframe"))!;
    const color = nodes.find((node) => node.raw.includes("input.color"))!;

    expect(timeframe).toMatchObject({
      kind: "input",
      match: expect.objectContaining({
        type: "input",
        input: expect.objectContaining({ name: "higherTf", type: "timeframe", defaultValue: "D" }),
      }),
    });
    expect(color).toMatchObject({
      kind: "input",
      match: expect.objectContaining({
        type: "input",
        input: expect.objectContaining({ name: "plotColor", type: "color", defaultValue: "color.teal" }),
      }),
    });

    expect(renderBlockToSource(updateInstructionBlockParam(timeframe, "defaultValue", "W"))).toBe(
      'higherTf = input.timeframe("W", "Higher TF")',
    );
    expect(renderBlockToSource(updateInstructionBlockParam(color, "defaultValue", "color.orange"))).toBe(
      'plotColor = input.color(color.orange, "Plot Color")',
    );
  });

  it("parses positional strategy entry and close arguments into editable source blocks", () => {
    const source = `strategy.entry("Long", strategy.long, 5)
strategy.close("Long", 2)
`;
    const nodes = buildPineSourceStructureIndex(source);
    const entry = nodes.find((node) => node.raw.includes("strategy.entry"))!;
    const close = nodes.find((node) => node.raw.includes("strategy.close"))!;

    expect(entry).toMatchObject({
      match: expect.objectContaining({
        type: "instruction",
        block: expect.objectContaining({
          params: expect.objectContaining({ qty: "5" }),
        }),
      }),
    });
    expect(close).toMatchObject({
      match: expect.objectContaining({
        type: "instruction",
        block: expect.objectContaining({
          params: expect.objectContaining({
            qty: "2",
          }),
        }),
      }),
    });
    expect(renderBlockToSource(updateInstructionBlockParam(close, "comment", "trim"))).toBe('strategy.close("Long", qty=2, comment="trim")');
  });

  it("indexes common raw Pine v6 calls with readable categories while extracting supported order controls", () => {
    const source = `//@version=6
strategy("Raw indexes", overlay=true)
plotshape(close > open, title="Up")
var table dashboard = table.new(position.top_right, 2, 2)
label lastSignal = label.new(bar_index, high, "Signal")
alert("manual alert")
runtime.error("stop")
strategy.close_all(immediately=true, comment="flat")
strategy.cancel("Long")
strategy.cancel_all()
strategy.risk.allow_entry_in(strategy.direction.long)
strategy.risk.max_drawdown(10, strategy.percent_of_equity)
`;

    const nodes = buildPineSourceStructureIndex(source);

    expect(nodes.find((node) => node.raw.includes("plotshape"))).toMatchObject({
      kind: "visual",
      label: "形状绘图",
      match: { type: "raw" },
    });
    expect(nodes.find((node) => node.raw.includes("table.new"))).toMatchObject({
      kind: "visual",
      label: "表格绘制",
      match: { type: "raw" },
    });
    expect(nodes.find((node) => node.raw.includes("label.new"))).toMatchObject({
      kind: "visual",
      label: "标签绘制",
      match: { type: "raw" },
    });
    expect(nodes.find((node) => node.raw.includes('alert("manual alert")'))).toMatchObject({
      kind: "alert",
      label: "即时提醒",
      match: { type: "raw" },
    });
    expect(nodes.find((node) => node.raw.includes("runtime.error"))).toMatchObject({
      kind: "runtime",
      label: "运行时错误",
      match: { type: "raw" },
    });
    expect(nodes.find((node) => node.raw.includes("strategy.close_all"))).toMatchObject({
      kind: "order",
      label: "全部平仓",
      match: expect.objectContaining({ type: "instruction" }),
    });
    expect(nodes.find((node) => node.raw.includes("strategy.cancel"))).toMatchObject({
      kind: "order",
      label: "撤销订单",
      match: expect.objectContaining({ type: "instruction" }),
    });
    expect(nodes.find((node) => node.raw.includes("strategy.cancel_all"))).toMatchObject({
      kind: "order",
      label: "撤销全部订单",
      match: expect.objectContaining({ type: "instruction" }),
    });
    expect(nodes.find((node) => node.raw.includes("strategy.risk.allow_entry_in"))).toMatchObject({
      kind: "order",
      label: "允许入场方向",
      match: expect.objectContaining({ type: "instruction" }),
    });
    expect(nodes.find((node) => node.raw.includes("strategy.risk.max_drawdown"))).toMatchObject({
      kind: "order",
      label: "最大回撤风控",
      match: expect.objectContaining({ type: "instruction" }),
    });

    expect(buildWorkflowSnapshotFromSource(source).blocks.map((block) => block.kind)).toEqual([
      "strategy_close_all",
      "strategy_cancel",
      "strategy_cancel_all",
      "strategy_risk_allow_entry_in",
      "strategy_risk_max_drawdown",
    ]);
  });

  it("renders and edits supported strategy close cancel and risk controls", () => {
    const source = `strategy.close_all(true, "flat", "done", false)
strategy.cancel("LimitLong")
strategy.cancel_all()
strategy.risk.allow_entry_in(strategy.direction.short)
strategy.risk.max_drawdown(10, strategy.percent_of_equity, "dd")
strategy.risk.max_intraday_loss(5, strategy.cash)
strategy.risk.max_intraday_filled_orders(12, "orders")
strategy.risk.max_position_size(3)
strategy.risk.max_cons_loss_days(2, "days")
`;
    const nodes = buildPineSourceStructureIndex(source);
    const closeAll = nodes.find((node) => node.raw.includes("close_all"))!;
    const cancel = nodes.find((node) => node.raw.includes("strategy.cancel("))!;
    const cancelAll = nodes.find((node) => node.raw.includes("cancel_all"))!;
    const allowEntry = nodes.find((node) => node.raw.includes("allow_entry_in"))!;
    const maxDrawdown = nodes.find((node) => node.raw.includes("max_drawdown"))!;
    const maxIntradayLoss = nodes.find((node) => node.raw.includes("max_intraday_loss"))!;
    const maxFilledOrders = nodes.find((node) => node.raw.includes("max_intraday_filled_orders"))!;
    const maxPosition = nodes.find((node) => node.raw.includes("max_position_size"))!;
    const maxConsLossDays = nodes.find((node) => node.raw.includes("max_cons_loss_days"))!;

    expect(renderBlockToSource(updateInstructionBlockParam(closeAll, "comment", "exit now"))).toContain(
      'strategy.close_all(immediately=true, comment="exit now", alert_message="done", disable_alert=false)',
    );
    expect(renderBlockToSource(updateInstructionBlockParam(cancel, "id", "StopLong"))).toBe('strategy.cancel("StopLong")');
    expect(renderBlockToSource(cancelAll)).toBe("strategy.cancel_all()");
    expect(renderBlockToSource(updateInstructionBlockParam(allowEntry, "direction", "strategy.direction.long"))).toBe(
      "strategy.risk.allow_entry_in(strategy.direction.long)",
    );
    expect(renderBlockToSource(updateInstructionBlockParam(maxDrawdown, "value", "15"))).toBe(
      'strategy.risk.max_drawdown(15, strategy.percent_of_equity, alert_message="dd")',
    );
    expect(renderBlockToSource(updateInstructionBlockParam(maxIntradayLoss, "type", "strategy.percent_of_equity"))).toBe(
      "strategy.risk.max_intraday_loss(5, strategy.percent_of_equity)",
    );
    expect(renderBlockToSource(updateInstructionBlockParam(maxFilledOrders, "count", "8"))).toBe(
      'strategy.risk.max_intraday_filled_orders(8, alert_message="orders")',
    );
    expect(renderBlockToSource(updateInstructionBlockParam(maxPosition, "contracts", "5"))).toBe(
      "strategy.risk.max_position_size(5)",
    );
    expect(renderBlockToSource(updateInstructionBlockParam(maxConsLossDays, "count", "4"))).toBe(
      'strategy.risk.max_cons_loss_days(4, alert_message="days")',
    );
  });

  it("indexes Pine declaration and reassignment forms without flattening them into workflow blocks", () => {
    const source = `//@version=6
strategy("Declarations", overlay=true)
varip float intrabarHigh = high
const int maxLookback = 500
float basis = ta.ema(close, 20)
basis := basis + 1
[macdLine, signalLine, histLine] = ta.macd(close, 12, 26, 9)
[dailyClose, dailyVolume] = request.security(syminfo.tickerid, "D", [close, volume])
`;

    const nodes = buildPineSourceStructureIndex(source);

    expect(nodes.find((node) => node.raw.includes("varip float"))).toMatchObject({
      kind: "declaration",
      label: "Bar 内持久变量 intrabarHigh",
      match: { type: "raw" },
    });
    expect(nodes.find((node) => node.raw.includes("const int"))).toMatchObject({
      kind: "declaration",
      label: "常量声明 maxLookback",
      match: { type: "raw" },
    });
    expect(nodes.find((node) => node.raw.includes("float basis"))).toMatchObject({
      kind: "declaration",
      label: "类型声明 basis",
      match: { type: "raw" },
    });
    expect(nodes.find((node) => node.raw.includes("basis :="))).toMatchObject({
      kind: "assignment",
      label: "重赋值 basis",
      match: { type: "raw" },
    });
    expect(nodes.find((node) => node.raw.includes("ta.macd"))).toMatchObject({
      kind: "declaration",
      label: "Tuple 解构",
      match: { type: "raw" },
    });
    expect(nodes.find((node) => node.raw.includes("request.security"))).toMatchObject({
      kind: "request",
      label: "跨周期请求",
      match: { type: "raw" },
    });
    expect(buildWorkflowSnapshotFromSource(source).blocks).toEqual([]);
  });

  it("indexes broader array map and matrix calls while preserving editable array operations", () => {
    const source = `var values = array.new_float()
array.push(values, close)
seeded = array.from(open, high, low, close)
sorted = array.copy(seeded)
array.sort(sorted, order.ascending)
var map<string, float> weights = map.new<string, float>()
map.put(weights, "fast", close)
keys = map.keys(weights)
var matrix<float> grid = matrix.new<float>(2, 2, 0.0)
matrix.set(grid, 0, 1, close)
cell = matrix.get(grid, 0, 1)
rows = matrix.rows(grid)
`;

    const nodes = buildPineSourceStructureIndex(source);

    expect(nodes.find((node) => node.raw.includes("array.new_float"))).toMatchObject({
      kind: "collection",
      label: "集合操作",
      match: expect.objectContaining({ type: "instruction" }),
    });
    expect(nodes.find((node) => node.raw.includes("array.push"))).toMatchObject({
      kind: "collection",
      label: "集合操作",
      match: expect.objectContaining({ type: "instruction" }),
    });
    expect(nodes.find((node) => node.raw.includes("array.from"))).toMatchObject({
      kind: "collection",
      label: "数组操作",
      match: { type: "raw" },
    });
    expect(nodes.find((node) => node.raw.includes("map.new"))).toMatchObject({
      kind: "collection",
      label: "Map 操作",
      match: { type: "raw" },
    });
    expect(nodes.find((node) => node.raw.includes("map.put"))).toMatchObject({
      kind: "collection",
      label: "Map 操作",
      match: { type: "raw" },
    });
    expect(nodes.find((node) => node.raw.includes("matrix.new"))).toMatchObject({
      kind: "collection",
      label: "矩阵操作",
      match: { type: "raw" },
    });
    expect(nodes.find((node) => node.raw.includes("matrix.rows"))).toMatchObject({
      kind: "collection",
      label: "矩阵操作",
      match: { type: "raw" },
    });

    const snapshot = buildWorkflowSnapshotFromSource(source);
    expect(snapshot.blocks.map((block) => block.kind)).toEqual(["array_op", "array_op"]);
  });

  it("keeps nested else branches under the matching condition block", () => {
    const [condition] = parseSourceToBlocks(`if close > open
    strategy.entry("Long", strategy.long)
else
    strategy.close("Long")
`);

    expect(condition?.kind).toBe("condition");
    expect(condition?.children.map((child) => child.kind)).toEqual(["order", "branch"]);
    expect(condition?.children[1]?.children[0]).toMatchObject({
      kind: "order",
      match: expect.objectContaining({ type: "instruction" }),
    });
    expect(condition?.sourceRange.end).toBe(`if close > open
    strategy.entry("Long", strategy.long)
else
    strategy.close("Long")`.length);
  });

  it("keeps Pine control structures and multiline UDF bodies as raw parent scopes", () => {
    const source = `for i = 0 to 3
    array.push(values, close)
while close > open
    strategy.close("Long")
switch
    close > open => 1
    => 0
score(src) =>
    result = ta.ema(src, 3)
    result
`;
    const blocks = parseSourceToBlocks(source);

    expect(blocks.map((block) => block.kind)).toEqual(["loop", "loop", "switch", "function"]);
    expect(blocks[0]).toMatchObject({
      label: "循环结构",
      match: { type: "raw" },
      children: [expect.objectContaining({ kind: "collection" })],
      lineRange: { start: 1, end: 2 },
    });
    expect(blocks[1]).toMatchObject({
      label: "循环结构",
      children: [expect.objectContaining({ kind: "order" })],
      lineRange: { start: 3, end: 4 },
    });
    expect(blocks[2]).toMatchObject({
      label: "条件选择",
      children: [
        expect.objectContaining({ kind: "raw" }),
        expect.objectContaining({ kind: "raw" }),
      ],
      lineRange: { start: 5, end: 7 },
    });
    expect(blocks[3]).toMatchObject({
      label: "函数 score",
      children: [
        expect.objectContaining({ kind: "assignment" }),
        expect.objectContaining({ kind: "raw" }),
      ],
      lineRange: { start: 8, end: 10 },
    });
    expect(blocks[3]?.sourceRange.end).toBe(source.trimEnd().length);
    expect(buildWorkflowSnapshotFromSource(source).blocks).toEqual([]);
  });

  it("indexes library, exported type, and method definitions as raw source scopes", () => {
    const source = `//@version=6
library("Helpers", overlay=true)
import TradingView/ta/7 as ta7
export type SignalBox
    float price
    int bars
export method score(SignalBox this, float weight) =>
    weighted = this.price * weight
    weighted
export normalize(src) =>
    ta.ema(src, 3)
indicator("Helper indicator")
`;
    const nodes = buildPineSourceStructureIndex(source);
    const roots = parseSourceToBlocks(source);

    expect(nodes.find((node) => node.raw.includes('library("Helpers"'))).toMatchObject({
      kind: "library",
      label: "库声明",
      match: { type: "raw" },
    });
    expect(nodes.find((node) => node.raw.includes("TradingView/ta/7"))).toMatchObject({
      kind: "library",
      label: "导入库 ta7",
      match: { type: "raw" },
    });
    expect(roots.find((node) => node.kind === "type")).toMatchObject({
      label: "导出类型 SignalBox",
      children: [
        expect.objectContaining({ kind: "declaration" }),
        expect.objectContaining({ kind: "declaration" }),
      ],
      lineRange: { start: 4, end: 6 },
      match: { type: "raw" },
    });
    expect(roots.find((node) => node.kind === "method")).toMatchObject({
      label: "导出方法 score",
      children: [
        expect.objectContaining({ kind: "object", label: "对象字段读取 this" }),
        expect.objectContaining({ kind: "raw" }),
      ],
      lineRange: { start: 7, end: 9 },
      match: { type: "raw" },
    });
    expect(nodes.find((node) => node.raw.includes("export normalize"))).toMatchObject({
      kind: "function",
      label: "导出函数 normalize",
      match: { type: "raw" },
    });
    expect(nodes.find((node) => node.raw.includes('indicator("Helper indicator")'))).toMatchObject({
      kind: "declaration",
      label: "指标声明",
      match: { type: "raw" },
    });
    expect(buildWorkflowSnapshotFromSource(source).blocks).toEqual([]);
  });

  it("indexes object constructors field access and method chains without treating them as series assignments", () => {
    const source = `type SignalBox
    float price
    int bars
var SignalBox activeBox = SignalBox.new(close, 0)
activeBox.price := high
previousPrice = activeBox[1].price
score = activeBox.score(2.0)
normalized = activeBox.normalize().score(weight=2.0)
fast = ta.ema(close, 12)
label.set_text(debugLabel, "ok")
`;

    const nodes = buildPineSourceStructureIndex(source);

    expect(nodes.find((node) => node.raw.includes("SignalBox.new"))).toMatchObject({
      kind: "object",
      label: "对象构造 SignalBox",
      match: { type: "raw" },
    });
    expect(nodes.find((node) => node.raw.includes("activeBox.price :="))).toMatchObject({
      kind: "object",
      label: "对象字段更新 activeBox.price",
      match: { type: "raw" },
    });
    expect(nodes.find((node) => node.raw.includes("activeBox[1].price"))).toMatchObject({
      kind: "object",
      label: "对象历史读取 activeBox",
      match: { type: "raw" },
    });
    expect(nodes.find((node) => node.raw.includes("activeBox.score"))).toMatchObject({
      kind: "object",
      label: "对象方法 activeBox",
      match: { type: "raw" },
    });
    expect(nodes.find((node) => node.raw.includes("activeBox.normalize"))).toMatchObject({
      kind: "object",
      label: "对象方法 activeBox",
      match: { type: "raw" },
    });
    expect(nodes.find((node) => node.raw.includes("ta.ema"))).toMatchObject({
      kind: "assignment",
      label: "fast",
      match: expect.objectContaining({ type: "instruction" }),
    });
    expect(nodes.find((node) => node.raw.includes("label.set_text"))).toMatchObject({
      kind: "visual",
      label: "标签绘制",
      match: { type: "raw" },
    });

    expect(buildWorkflowSnapshotFromSource(source).blocks.map((block) => block.kind)).toEqual(["series_assign"]);
  });

  it("does not include raw-only source in the compatible workflow snapshot", () => {
    const snapshot = buildWorkflowSnapshotFromSource(`//@version=6
strategy("Raw", overlay=true)
import TradingView/ta/7 as ta7
`);

    expect(snapshot.declaration.title).toBe("Raw");
    expect(snapshot.blocks).toEqual([]);
    expect(snapshot.inputs).toEqual([]);
  });

  it("renders a default source fragment for every supported workflow block kind", () => {
    for (const option of PINE_V6_BLOCK_KINDS) {
      const rendered = renderDefaultSourceBlock(option.kind, 1);
      expect(rendered.startsWith("    ")).toBe(true);
      expect(rendered.trim()).not.toBe("");
    }
  });

  it("inserts new blocks into the selected scope or bar-closed main flow", () => {
    const source = `//@version=6
strategy("Insert", overlay=true)
barClosed = barstate.isconfirmed
if barClosed
    fast = close
`;
    const noSelection = insertSourceBlock(source, null, "strategy_order").source;
    expect(noSelection).toContain('    strategy.order("Order", strategy.long)');

    const condition = buildPineSourceStructureIndex(source).find((node) => node.kind === "condition");
    const selectedIf = insertSourceBlock(source, condition!.id, "strategy_entry").source;
    expect(selectedIf).toContain('    strategy.entry("Long", strategy.long)');

    const siblingSource = `if close > open
    fast = close
    slow = open
`;
    const siblingNodes = buildPineSourceStructureIndex(siblingSource);
    const firstAssignment = siblingNodes.find((node) => node.label === "fast")!;
    const selectedSibling = insertSourceBlock(siblingSource, firstAssignment.id, "plot").source;
    expect(selectedSibling.indexOf("slow = open")).toBeLessThan(selectedSibling.indexOf("plot(close"));

    const elseSource = `if close > open
    strategy.entry("Long", strategy.long)
else
    strategy.close("Long")
`;
    const elseNode = buildPineSourceStructureIndex(elseSource).find((node) => node.kind === "branch")!;
    const selectedElse = insertSourceBlock(elseSource, elseNode.id, "log").source;
    expect(selectedElse).toContain('    log.info("Pine v6 工作流")');
    expect(selectedElse.indexOf('strategy.close("Long")')).toBeLessThan(selectedElse.indexOf("log.info"));
  });

  it("supports deleting, duplicating, moving, and replacing source blocks", () => {
    const source = `//@version=6
strategy("Ops", overlay=true)
if close > open
    strategy.entry("Long", strategy.long)
    plot(close)
import TradingView/ta/7 as ta7
`;
    const nodes = buildPineSourceStructureIndex(source);
    const order = nodes.find((node) => node.kind === "order")!;
    const plot = nodes.find((node) => node.kind === "visual")!;
    const library = nodes.find((node) => node.kind === "library")!;

    expect(library.match).toEqual({ type: "raw" });
    expect(duplicateSourceBlock(source, library.id).source.match(/import TradingView\/ta\/7 as ta7/g)).toHaveLength(2);
    expect(deleteSourceBlock(source, library.id).source).not.toContain("TradingView/ta/7");

    const moved = moveSourceBlock(source, plot.id, -1).source;
    expect(moved.indexOf("plot(close)")).toBeLessThan(moved.indexOf("strategy.entry"));
    const movedRaw = moveSourceBlock(source, library.id, -1).source;
    expect(movedRaw.indexOf("import TradingView")).toBeLessThan(movedRaw.indexOf("if close > open"));

    const replaced = replaceSourceBlockKind(source, order.id, "strategy_order").source;
    expect(replaced).toContain('strategy.order("Order", strategy.long)');
    expect(replaced).not.toContain('strategy.entry("Long", strategy.long)');
  });
});
