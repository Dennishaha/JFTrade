// @vitest-environment jsdom

import { afterEach, describe, expect, it, vi } from "vitest";

import { streamADKChat } from "../src/composables/adkChatStream";

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
});
