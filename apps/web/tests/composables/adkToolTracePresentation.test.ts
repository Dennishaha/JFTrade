import { describe, expect, it } from "vitest";

import type { ADKToolCall } from "../../src/types";
import {
  classifyToolAction,
  formatTraceDuration,
  parseTraceTime,
  summarizeToolGroup,
  toolPrimaryArgument,
  toolResultMeta,
  toolTraceRowLabel,
} from "@/composables/adk/adkToolTracePresentation";

function buildToolCall(
  overrides: Partial<ADKToolCall> & { toolName: string },
): ADKToolCall {
  return {
    id: overrides.id ?? `call-${overrides.toolName}`,
    runId: "run-1",
    permission: "read",
    status: "SUCCEEDED",
    requiresUser: false,
    createdAt: "2026-08-15T00:00:00Z",
    updatedAt: "2026-08-15T00:00:01Z",
    ...overrides,
  };
}

describe("adkToolTracePresentation", () => {
  it("classifies known tools into readable actions", () => {
    expect(classifyToolAction("market.search").verb).toBe("搜索");
    expect(classifyToolAction("strategy.validate_pine").verb).toBe("校验");
    expect(classifyToolAction("strategy.research_backtest").verb).toBe("回测");
    expect(classifyToolAction("execution.order_place").verb).toBe("下单");
    expect(classifyToolAction("execution.order_cancel").verb).toBe("撤单");
    expect(classifyToolAction("interaction.request_user").verb).toBe("提问");
  });

  it("falls back to a generic query action for read tools", () => {
    expect(classifyToolAction("market.candles").verb).toBe("查询");
    expect(classifyToolAction("portfolio.summary").verb).toBe("查询");
    expect(classifyToolAction("some.unknown_tool").verb).toBe("查询");
  });

  it("extracts the primary argument by priority and skips script payloads", () => {
    expect(
      toolPrimaryArgument({ symbol: "AAPL", script: "indicator(...) very long" }),
    ).toBe("AAPL");
    expect(toolPrimaryArgument({ query: "file.length|max-lines" })).toBe(
      "file.length|max-lines",
    );
    expect(toolPrimaryArgument({ symbols: ["AAPL", "TSLA", "NVDA", "MSFT"] })).toBe(
      "AAPL、TSLA、NVDA 等 4 个",
    );
    expect(toolPrimaryArgument({ script: "only script" })).toBe("");
    expect(toolPrimaryArgument(undefined)).toBe("");
    expect(toolPrimaryArgument({ symbol: "x".repeat(120) })).toHaveLength(60);
  });

  it("summarizes tool results as row meta", () => {
    expect(
      toolResultMeta(
        buildToolCall({
          toolName: "market.candles",
          output: { candles: [{}, {}, {}] },
        }),
      ),
    ).toBe("3 条");
    expect(
      toolResultMeta(
        buildToolCall({ toolName: "strategy.validate_pine", output: { ok: true } }),
      ),
    ).toBe("通过");
    expect(
      toolResultMeta(
        buildToolCall({ toolName: "strategy.validate_pine", output: { ok: false } }),
      ),
    ).toBe("未通过");
    expect(
      toolResultMeta(
        buildToolCall({
          toolName: "market.candles",
          status: "FAILED",
          error: "provider unavailable",
        }),
      ),
    ).toBe("provider unavailable");
    expect(
      toolResultMeta(buildToolCall({ toolName: "market.candles", status: "RUNNING" })),
    ).toBe("");
  });

  it("aggregates a tool group into verb counts, status and elapsed time", () => {
    const summary = summarizeToolGroup([
      buildToolCall({
        id: "c1",
        toolName: "market.candles",
        startedAt: "2026-08-15T00:00:00Z",
        completedAt: "2026-08-15T00:00:02Z",
      }),
      buildToolCall({
        id: "c2",
        toolName: "market.snapshot",
        startedAt: "2026-08-15T00:00:02Z",
        completedAt: "2026-08-15T00:00:03Z",
      }),
      buildToolCall({
        id: "c3",
        toolName: "strategy.validate_pine",
        startedAt: "2026-08-15T00:00:03Z",
        completedAt: "2026-08-15T00:00:04Z",
      }),
    ]);

    expect(summary.parts).toEqual(["已查询了 2 项", "已校验了 1 个脚本"]);
    expect(summary.total).toBe(3);
    expect(summary.status).toBe("COMPLETED");
    expect(summary.durationMs).toBe(4000);
  });

  it("marks groups with failed or pending-approval tools", () => {
    const failed = summarizeToolGroup([
      buildToolCall({ id: "c1", toolName: "market.candles" }),
      buildToolCall({ id: "c2", toolName: "market.snapshot", status: "FAILED" }),
    ]);
    expect(failed.status).toBe("FAILED");

    const waiting = summarizeToolGroup([
      buildToolCall({
        id: "c3",
        toolName: "execution.order_place",
        status: "PENDING_APPROVAL",
      }),
    ]);
    expect(waiting.status).toBe("PENDING_APPROVAL");
  });

  it("formats numeric, empty and short list arguments for tool rows", () => {
    expect(toolPrimaryArgument({ runId: 42 })).toBe("42");
    expect(toolPrimaryArgument({ runId: Number.NaN })).toBe("");
    expect(toolPrimaryArgument({ symbols: [] })).toBe("");
    expect(toolPrimaryArgument({ symbols: ["AAPL", "TSLA"] })).toBe("AAPL、TSLA");
  });

  it("summarizes array, scalar and nested-record tool outputs", () => {
    expect(
      toolResultMeta(
        buildToolCall({ toolName: "market.news", output: [{}, {}] }),
      ),
    ).toBe("2 条");
    expect(
      toolResultMeta(buildToolCall({ toolName: "market.news", output: "done" })),
    ).toBe("");
    expect(
      toolResultMeta(
        buildToolCall({
          toolName: "strategy.research_backtest",
          output: { note: "finished", payload: { runs: [{}, {}, {}] } },
        }),
      ),
    ).toBe("3 条");
    expect(
      toolResultMeta(
        buildToolCall({ toolName: "portfolio.summary", output: { status: "done" } }),
      ),
    ).toBe("");
    expect(
      toolResultMeta(buildToolCall({ toolName: "market.candles", status: undefined })),
    ).toBe("");
  });

  it("sums tool durations when timestamps are unavailable", () => {
    const timeless = {
      createdAt: undefined,
      startedAt: undefined,
      updatedAt: undefined,
      completedAt: undefined,
    };
    const summary = summarizeToolGroup([
      buildToolCall({ id: "c1", toolName: "market.candles", durationMs: 1200, ...timeless }),
      buildToolCall({ id: "c2", toolName: "market.candles", durationMs: 800, ...timeless }),
      buildToolCall({ id: "c3", toolName: "market.candles", durationMs: undefined, ...timeless }),
    ]);

    expect(summary.durationMs).toBe(2000);
  });

  it("omits the group duration when neither timestamps nor durations exist", () => {
    const summary = summarizeToolGroup([
      buildToolCall({
        id: "c1",
        toolName: "market.candles",
        createdAt: undefined,
        startedAt: undefined,
        updatedAt: undefined,
        completedAt: undefined,
        durationMs: undefined,
      }),
    ]);

    expect(summary.durationMs).toBeUndefined();
  });

  it("rejects unparseable trace timestamps", () => {
    expect(parseTraceTime("not-a-date")).toBeNull();
    expect(parseTraceTime(undefined)).toBeNull();
  });

  it("formats durations in the compact trace style", () => {
    expect(formatTraceDuration(320)).toBe("320ms");
    expect(formatTraceDuration(0)).toBe("0ms");
    expect(formatTraceDuration(4200)).toBe("4.2s");
    expect(formatTraceDuration(20000)).toBe("20s");
    expect(formatTraceDuration(102000)).toBe("1m42s");
    expect(formatTraceDuration(undefined)).toBe("");
  });

  it("prefers the tool descriptor display name for row labels", () => {
    const toolCall = buildToolCall({
      toolName: "market.candles",
      input: { symbol: "AAPL" },
    });
    const withDescriptor = toolTraceRowLabel(toolCall, {
      displayName: "K 线查询",
    } as never);
    expect(withDescriptor.label).toBe("K 线查询");
    expect(withDescriptor.argument).toBe("AAPL");

    const withoutDescriptor = toolTraceRowLabel(toolCall, undefined);
    expect(withoutDescriptor.label).toBe("查询");
    expect(withoutDescriptor.argument).toBe("AAPL");

    const bareToolCall = buildToolCall({ toolName: "portfolio.summary" });
    const bare = toolTraceRowLabel(bareToolCall, undefined);
    expect(bare.label).toBe("查询");
    expect(bare.argument).toBe("portfolio.summary");
  });
});
