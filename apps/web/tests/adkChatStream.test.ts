// @vitest-environment jsdom

import { afterEach, describe, expect, it, vi } from "vitest";

import {
  resumeADKChatStream,
  streamADKChat,
} from "../src/composables/adkChatStream";

afterEach(() => {
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

describe("streamADKChat", () => {
  it("prefers the server-provided idle timeout header", async () => {
    const timeoutSpy = vi.spyOn(globalThis, "setTimeout");
    const body = [
      'data: {"type":"final","response":{"reply":"ok","session":{"id":"session-1","agentId":"agent-1","title":"Test","createdAt":"2026-06-08T00:00:00Z","updatedAt":"2026-06-08T00:00:00Z"},"run":{"id":"run-1","sessionId":"session-1","agentId":"agent-1","status":"COMPLETED","message":"completed","toolCalls":[],"pendingApprovals":[],"createdAt":"2026-06-08T00:00:00Z","updatedAt":"2026-06-08T00:00:00Z"},"pendingApprovals":[],"timeline":[]}}',
      "",
      "",
    ].join("\n");

    vi.stubGlobal(
      "fetch",
      vi.fn(
        async () =>
          new Response(body, {
            headers: {
              "Content-Type": "text/event-stream",
              "X-ADK-Stream-Idle-Timeout-Ms": "123000",
            },
          }),
      ),
    );

    const response = await streamADKChat(
      { agentId: "agent-1", sessionId: "session-1", message: "hello" },
      vi.fn(),
    );

    expect(response.reply).toBe("ok");
    expect(response.timeline).toEqual([]);
    expect(timeoutSpy).toHaveBeenCalled();
    const delays = timeoutSpy.mock.calls
      .map((call) => call[1])
      .filter((delay): delay is number => typeof delay === "number");
    expect(delays).toContain(123000);
  });

  it("resolves failed terminal runs from final events", async () => {
    const body = [
      'data: {"type":"final","response":{"reply":"fallback","session":{"id":"session-1","agentId":"agent-1","title":"Test","createdAt":"2026-06-08T00:00:00Z","updatedAt":"2026-06-08T00:00:00Z"},"run":{"id":"run-1","sessionId":"session-1","agentId":"agent-1","status":"FAILED","message":"disk full","failureReason":"disk full","errorCode":"TOOL_EXECUTION_FAILED","toolCalls":[{"id":"tool-1","runId":"run-1","toolName":"strategy.save_draft","permission":"write_strategy","status":"FAILED","error":"disk full","requiresUser":false,"createdAt":"2026-06-08T00:00:00Z","updatedAt":"2026-06-08T00:00:00Z"}],"pendingApprovals":[],"createdAt":"2026-06-08T00:00:00Z","updatedAt":"2026-06-08T00:00:00Z"},"pendingApprovals":[],"timeline":[]}}',
      "",
      "",
    ].join("\n");

    vi.stubGlobal(
      "fetch",
      vi.fn(
        async () =>
          new Response(body, {
            headers: {
              "Content-Type": "text/event-stream",
            },
          }),
      ),
    );

    const response = await streamADKChat(
      { agentId: "agent-1", sessionId: "session-1", message: "hello" },
      vi.fn(),
    );

    expect(response.run.status).toBe("FAILED");
    expect(response.run.failureReason).toBe("disk full");
    expect(response.run.toolCalls[0]?.error).toBe("disk full");
  });

  it("replays a persisted stream after the last received sequence", async () => {
    const body = [
      'id: stream-1:3\ndata: {"type":"run","streamId":"stream-1","sequence":3,"runId":"run-1","replay":true,"run":{"id":"run-1","sessionId":"session-1","agentId":"agent-1","status":"RUNNING","message":"running","toolCalls":[],"pendingApprovals":[],"createdAt":"2026-06-08T00:00:00Z","updatedAt":"2026-06-08T00:00:01Z"}}',
      'id: stream-1:4\ndata: {"type":"final","streamId":"stream-1","sequence":4,"runId":"run-1","replay":true,"response":{"reply":"done","session":{"id":"session-1","agentId":"agent-1","title":"Test","createdAt":"2026-06-08T00:00:00Z","updatedAt":"2026-06-08T00:00:02Z"},"run":{"id":"run-1","sessionId":"session-1","agentId":"agent-1","status":"COMPLETED","message":"completed","toolCalls":[],"pendingApprovals":[],"createdAt":"2026-06-08T00:00:00Z","updatedAt":"2026-06-08T00:00:02Z"},"pendingApprovals":[],"timeline":[]}}',
      "",
      "",
    ].join("\n\n");
    const onEvent = vi.fn();
    const fetchMock = vi.fn(async () => new Response(body));
    vi.stubGlobal("fetch", fetchMock);

    const response = await resumeADKChatStream(
      { streamId: "stream-1", runId: "run-1", after: 2 },
      onEvent,
    );

    expect(String(fetchMock.mock.calls[0]?.[0])).toContain(
      "/api/v1/adk/streams/stream-1?after=2",
    );
    expect(onEvent).toHaveBeenCalledTimes(2);
    expect(onEvent.mock.calls[0]?.[0]).toMatchObject({
      streamId: "stream-1",
      sequence: 3,
      replay: true,
    });
    expect(response?.reply).toBe("done");
  });

  it("returns null when the in-memory stream has expired", async () => {
    vi.stubGlobal("fetch", vi.fn(async () => new Response("", { status: 404 })));

    await expect(
      resumeADKChatStream({ runId: "run-missing" }, vi.fn()),
    ).resolves.toBeNull();
  });

  it("does not reconnect without a persisted stream or run cursor", async () => {
    const fetchMock = vi.fn();
    vi.stubGlobal("fetch", fetchMock);

    await expect(resumeADKChatStream({ streamId: " ", runId: " " }, vi.fn())).resolves.toBeNull();
    expect(fetchMock).not.toHaveBeenCalled();
  });

  it("reconnects by encoded run id with a rounded cursor and abort signal", async () => {
    const controller = new AbortController();
    const fetchMock = vi.fn(async () => new Response(finalFrame("resumed")));
    vi.stubGlobal("fetch", fetchMock);

    const result = await resumeADKChatStream(
      { runId: "run/with space", after: 3.9, signal: controller.signal },
      vi.fn(),
    );

    expect(String(fetchMock.mock.calls[0]?.[0])).toContain(
      "/api/v1/adk/runs/run%2Fwith%20space/stream?after=3",
    );
    expect(fetchMock.mock.calls[0]?.[1]).toMatchObject({
      method: "GET",
      credentials: "include",
      signal: controller.signal,
    });
    expect(result?.reply).toBe("resumed");
  });

  it("surfaces POST and reconnect HTTP failures with stable fallback messages", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn()
        .mockResolvedValueOnce(new Response("provider unavailable", { status: 503 }))
        .mockResolvedValueOnce(new Response("", { status: 502 })),
    );

    await expect(streamADKChat({ message: "hello" }, vi.fn())).rejects.toThrow(
      "provider unavailable",
    );
    await expect(
      resumeADKChatStream({ streamId: "stream-1" }, vi.fn()),
    ).rejects.toThrow("Agents stream reconnect failed");
  });

  it("rejects responses without a readable stream body", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async () => ({
        ok: true,
        status: 200,
        body: null,
        headers: new Headers(),
      } as Response)),
    );

    await expect(streamADKChat({ message: "hello" }, vi.fn())).rejects.toThrow(
      "流式响应不可用",
    );
  });

  it("consumes a final event left in the trailing buffer", async () => {
    const onEvent = vi.fn();
    vi.stubGlobal("fetch", vi.fn(async () => new Response(finalFrame("tail", false))));

    const result = await streamADKChat({ message: "hello" }, onEvent);

    expect(result.reply).toBe("tail");
    expect(onEvent).toHaveBeenCalledOnce();
  });

  it("turns streamed error events into rejected chat operations", async () => {
    const onEvent = vi.fn();
    vi.stubGlobal(
      "fetch",
      vi.fn(async () =>
        new Response('data: {"type":"error","message":"tool gateway closed"}\n\n'),
      ),
    );

    await expect(streamADKChat({ message: "hello" }, onEvent)).rejects.toThrow(
      "tool gateway closed",
    );
    expect(onEvent).toHaveBeenCalledWith({
      type: "error",
      message: "tool gateway closed",
    });
  });

  it("rejects an error event delivered as the unterminated trailing frame", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async () =>
        new Response('data: {"type":"error","message":"trailing failure"}'),
      ),
    );

    await expect(streamADKChat({ message: "hello" }, vi.fn())).rejects.toThrow(
      "trailing failure",
    );
  });

  it("recovers a terminal run when the final SSE event was lost", async () => {
    const terminalRun = {
      id: "run-terminal",
      sessionId: "session-terminal",
      agentId: "agent-1",
      status: "FAILED",
      message: "approval interrupted",
      toolCalls: [
        {
          id: "tool-pending",
          runId: "run-terminal",
          toolName: "strategy.save_definition",
          permission: "write_strategy",
          status: "PENDING_APPROVAL",
          input: { name: "Momentum" },
          requiresUser: true,
          createdAt: "2026-06-08T00:00:00Z",
          updatedAt: "2026-06-08T00:00:01Z",
        },
      ],
      pendingApprovals: [],
      createdAt: "2026-06-08T00:00:00Z",
      updatedAt: "2026-06-08T00:00:01Z",
    };
    const body = [
      `data: ${JSON.stringify({ type: "session", session: buildSession("Recovered title") })}`,
      `data: ${JSON.stringify({ type: "run", run: terminalRun })}`,
      "",
      "",
    ].join("\n\n");
    vi.stubGlobal("fetch", vi.fn(async () => new Response(body)));

    const result = await streamADKChat({ message: "hello" }, vi.fn());

    expect(result.reply).toBe("Recovered title");
    expect(result.run.status).toBe("FAILED");
    expect(result.pendingApprovals).toEqual([
      expect.objectContaining({
        id: "tool-pending",
        runId: "run-terminal",
        toolName: "strategy.save_definition",
        status: "PENDING",
        reason: "write_strategy",
        input: { name: "Momentum" },
      }),
    ]);
  });

  it("distinguishes an interrupted stream from one with no valid frames", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn()
        .mockResolvedValueOnce(
          new Response('data: {"type":"session","session":{"id":"session-1","agentId":"agent-1","title":"Started","createdAt":"2026-06-08T00:00:00Z","updatedAt":"2026-06-08T00:00:00Z"}}\n\n'),
        )
        .mockResolvedValueOnce(new Response("event: ping\n\n")),
    );

    await expect(streamADKChat({ message: "first" }, vi.fn())).rejects.toThrow(
      "流式连接中断，未收到最终结果。",
    );
    await expect(streamADKChat({ message: "second" }, vi.fn())).rejects.toThrow(
      "流式连接未返回有效数据。",
    );
  });

  it("skips malformed SSE frames and still accepts a later final response", async () => {
    const warn = vi.spyOn(console, "warn").mockImplementation(() => {});
    const body = `data: not-json\n\n${finalFrame("after malformed")}`;
    vi.stubGlobal("fetch", vi.fn(async () => new Response(body)));

    const result = await streamADKChat({ message: "hello" }, vi.fn());

    expect(result.reply).toBe("after malformed");
    expect(warn).toHaveBeenCalledWith(
      "[ADK SSE] Failed to parse frame, skipping:",
      "not-json",
    );
  });

  it("normalizes timeline updates and recovers a terminal run from a trailing frame", async () => {
    const run = {
      id: "run-trailing", sessionId: "session-1", agentId: "agent-1", status: "FAILED",
      message: "provider stopped", toolCalls: [], pendingApprovals: [],
      createdAt: "2026-06-08T00:00:00Z", updatedAt: "2026-06-08T00:00:01Z",
    };
    const encoder = new TextEncoder();
    const body = new ReadableStream<Uint8Array>({
      start(controller) {
        controller.enqueue(encoder.encode(`data: ${JSON.stringify({
          type: "timeline",
          timeline: { id: "timeline-1", sessionId: "session-1", kind: "assistant_message", sequence: 1, status: "final", text: "working", createdAt: "2026-06-08T00:00:00Z" },
        })}\n\n`));
        controller.enqueue(encoder.encode(`data: ${JSON.stringify({ type: "session", session: buildSession("Recovered") })}\n\ndata: ${JSON.stringify({ type: "run", run })}`));
        controller.close();
      },
    });
    const onEvent = vi.fn();
    vi.stubGlobal("fetch", vi.fn(async () => new Response(body)));

    const result = await streamADKChat({ message: "resume" }, onEvent);

    expect(onEvent.mock.calls[0]?.[0]).toMatchObject({
      type: "timeline", timeline: { id: "timeline-1", status: "final" },
    });
    expect(result).toMatchObject({ reply: "Recovered", run: { id: "run-trailing", status: "FAILED" } });
  });

  it("cancels a stream that stays idle beyond the server-provided timeout", async () => {
    vi.useFakeTimers();
    let finishRead: ((result: { done: boolean; value?: Uint8Array }) => void) | undefined;
    const reader = {
      read: vi.fn(() => new Promise<{ done: boolean; value?: Uint8Array }>((resolve) => {
        finishRead = resolve;
      })),
      cancel: vi.fn(() => {
        finishRead?.({ done: true });
        return Promise.resolve();
      }),
    };
    const warn = vi.spyOn(console, "warn").mockImplementation(() => {});
    vi.stubGlobal("fetch", vi.fn(async () => ({
      ok: true,
      body: { getReader: () => reader },
      headers: new Headers({ "X-ADK-Stream-Idle-Timeout-Ms": "5" }),
    } as Response)));

    const operation = streamADKChat({ message: "wait" }, vi.fn());
    const rejection = expect(operation).rejects.toThrow("流式连接未返回有效数据。");
    await vi.advanceTimersByTimeAsync(0);
    await vi.advanceTimersByTimeAsync(5);

    await rejection;
    expect(reader.cancel).toHaveBeenCalledOnce();
    expect(warn).toHaveBeenCalledWith(
      "[ADK SSE] Idle timeout - no data for", 5, "ms, aborting stream",
    );
  });
});

function finalFrame(reply: string, terminated = true): string {
  const response = {
    reply,
    session: buildSession("Test"),
    run: {
      id: "run-1",
      sessionId: "session-1",
      agentId: "agent-1",
      status: "COMPLETED",
      message: reply,
      toolCalls: [],
      pendingApprovals: [],
      createdAt: "2026-06-08T00:00:00Z",
      updatedAt: "2026-06-08T00:00:01Z",
    },
    pendingApprovals: [],
    timeline: [],
  };
  return `data: ${JSON.stringify({ type: "final", response })}${terminated ? "\n\n" : ""}`;
}

function buildSession(title: string) {
  return {
    id: "session-1",
    agentId: "agent-1",
    title,
    createdAt: "2026-06-08T00:00:00Z",
    updatedAt: "2026-06-08T00:00:00Z",
  };
}
