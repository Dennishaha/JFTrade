import { afterEach, describe, expect, it, vi } from "vitest";

import { loadSessionChatHistory } from "../src/composables/adkPageRunHistory";
import { fetchADKPageSessionData } from "../src/composables/adkPageSessionApi";
import {
  fetchADKWorkflowTriggerLogs,
  fetchADKWorkflows,
} from "../src/composables/adkWorkflowsApi";
import { createResponse } from "./helpers";

afterEach(() => {
  vi.unstubAllGlobals();
});

describe("ADK typed API boundary fallbacks", () => {
  it("normalizes a persisted session that predates run and composer state fields", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async () =>
        createResponse({
          session: {
            id: "legacy-session",
            agentId: "researcher",
            title: "旧会话",
            createdAt: "2026-07-01T00:00:00Z",
            updatedAt: "2026-07-01T00:00:00Z",
          },
          timeline: [
            {
              id: "message-1",
              sessionId: "legacy-session",
              kind: "assistant_message",
              createdAt: "2026-07-01T00:00:01Z",
              sequence: 1,
              text: "已恢复",
            },
          ],
        }),
      ),
    );

    await expect(loadSessionChatHistory("legacy-session")).resolves.toMatchObject({
      session: { id: "legacy-session" },
      runs: [],
      composerState: {
        sessionId: "legacy-session",
        chatDraft: "",
        goalObjectiveTouched: false,
      },
      timelineEntries: [expect.objectContaining({ id: "message-1" })],
    });
  });

  it("retains tool descriptors returned by the typed page bootstrap endpoints", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async (request: RequestInfo | URL) => {
        const url = String(request);
        if (url.endsWith("/agents")) return createResponse({ agents: [] });
        if (url.endsWith("/providers")) return createResponse({ providers: [] });
        if (url.endsWith("/sessions")) return createResponse({ sessions: [] });
        if (url.endsWith("/approvals")) return createResponse({ approvals: [] });
        if (url.endsWith("/tools")) {
          return createResponse({
            tools: [
              {
                name: "market.snapshot",
                displayName: "行情快照",
                description: "读取一次行情快照",
                category: "market",
                permission: "read",
                allowedModes: ["approval"],
                requiresApprovalIn: [],
              },
            ],
          });
        }
        throw new Error(`unexpected ADK bootstrap request: ${url}`);
      }),
    );

    await expect(fetchADKPageSessionData()).resolves.toMatchObject({
      tools: [{ name: "market.snapshot", permission: "read" }],
    });
  });

  it("keeps pagination optional for workflow and trigger-log responses", async () => {
    const fetchMock = vi.fn(async (request: RequestInfo | URL) => {
      const url = String(request);
      if (url.startsWith("/api/v1/adk/workflows?")) {
        return createResponse({ workflows: [] });
      }
      if (url.startsWith("/api/v1/adk/workflow-trigger-logs?")) {
        return createResponse({ logs: [] });
      }
      throw new Error(`unexpected workflow request: ${url}`);
    });
    vi.stubGlobal("fetch", fetchMock);

    const firstPage = {
      limit: 20,
      offset: 0,
      total: 0,
      returned: 0,
      hasMore: false,
    };
    await expect(fetchADKWorkflows(firstPage, "")).resolves.toEqual({
      workflows: [],
    });
    await expect(
      fetchADKWorkflowTriggerLogs(firstPage, {
        triggerId: "trigger-1",
        status: "FAILED",
      }),
    ).resolves.toEqual({ logs: [] });

    expect(fetchMock).toHaveBeenLastCalledWith(
      expect.stringContaining("triggerId=trigger-1"),
      expect.any(Object),
    );
  });
});
