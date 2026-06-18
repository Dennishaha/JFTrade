// @vitest-environment jsdom

import { afterEach, describe, expect, it, vi } from "vitest";

import { saveADKAgent } from "../src/composables/adkSettingsApi";
import { createResponse } from "./helpers";

afterEach(() => {
  vi.unstubAllGlobals();
});

describe("saveADKAgent", () => {
  it("sends the default work mode and loop iteration limit", async () => {
    const agent = {
      id: "agent-loop",
      name: "Goal Agent",
      instruction: "Run toward the goal.",
      providerId: "provider-1",
      model: "",
      tools: [],
      skills: [],
      permissionMode: "approval",
      memoryEnabled: true,
      recentUserWindow: 6,
      workMode: "loop" as const,
      loopMaxIterations: 9,
      status: "ENABLED",
    };
    const fetchMock = vi.fn(async () =>
      createResponse({
        ...agent,
        createdAt: "2026-06-18T00:00:00Z",
        updatedAt: "2026-06-18T00:00:00Z",
      }),
    );
    vi.stubGlobal("fetch", fetchMock);

    await saveADKAgent(agent);

    const payload = JSON.parse(
      String((fetchMock.mock.calls[0]?.[1] as RequestInit | undefined)?.body),
    );
    expect(payload).toMatchObject({
      workMode: "loop",
      loopMaxIterations: 9,
    });
  });
});
