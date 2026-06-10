// @vitest-environment jsdom

import { mount } from "@vue/test-utils";
import { afterEach, describe, expect, it } from "vitest";

import ADKChatComposer from "../src/components/adk-page/ADKChatComposer.vue";

afterEach(() => {
  document.body.innerHTML = "";
});

describe("ADKChatComposer", () => {
  it("shows effective context usage plus raw diagnostics when oversized tool responses were trimmed", async () => {
    mount(ADKChatComposer, {
      attachTo: document.body,
      props: {
        canSendChat: false,
        chatDraft: "",
        contextDetailsOpen: true,
        contextSnapshot: {
          sessionId: "session-1",
          currentInputTokens: 1200,
          projectedNextTurnTokens: 1500,
          rawCurrentInputTokens: 57502105,
          rawProjectedNextTurnTokens: 57502105,
          contextWindowTokens: 0,
          usageRatio: 0,
          status: "unknown",
          recentUserWindow: 2,
          retainedRecentUserCount: 1,
          activeHandoffCount: 0,
          rawEventCount: 44,
          compactedEventCount: 0,
          summaryBoundaryEventIndex: 0,
          breakdown: {
            instructionTokens: 50,
            handoffTokens: 0,
            recentUserTokens: 20,
            protectedTailTokens: 900,
            otherVisibleTokens: 0,
            pendingUserTokens: 300,
            toolDeclarationTokens: 230,
          },
          rawBreakdown: {
            instructionTokens: 50,
            handoffTokens: 0,
            recentUserTokens: 20,
            protectedTailTokens: 57499564,
            otherVisibleTokens: 0,
            pendingUserTokens: 300,
            toolDeclarationTokens: 230,
          },
          trimmedToolResponseCount: 5,
          autoCompacted: false,
          degradedSummary: false,
        },
        sendChat: async () => {},
      },
    });

    expect(document.body.textContent).toContain("当前输入 Token");
    expect(document.body.textContent).toContain("1,200");
    expect(document.body.textContent).toContain("原始当前估算");
    expect(document.body.textContent).toContain("57,502,105");
    expect(document.body.textContent).toContain("已裁剪工具响应");
    expect(document.body.textContent).toContain("5");
    expect(document.body.textContent).toContain("原始 Token 构成");
  });

  it("keeps the diagnostics hidden for normal small sessions", async () => {
    mount(ADKChatComposer, {
      attachTo: document.body,
      props: {
        canSendChat: false,
        chatDraft: "",
        contextDetailsOpen: true,
        contextSnapshot: {
          sessionId: "session-2",
          currentInputTokens: 1200,
          projectedNextTurnTokens: 1500,
          rawCurrentInputTokens: 1200,
          rawProjectedNextTurnTokens: 1500,
          contextWindowTokens: 128000,
          usageRatio: 1200 / 128000,
          status: "healthy",
          recentUserWindow: 2,
          retainedRecentUserCount: 1,
          activeHandoffCount: 1,
          rawEventCount: 6,
          compactedEventCount: 2,
          summaryBoundaryEventIndex: 2,
          breakdown: {
            instructionTokens: 50,
            handoffTokens: 200,
            recentUserTokens: 100,
            protectedTailTokens: 600,
            otherVisibleTokens: 0,
            pendingUserTokens: 300,
            toolDeclarationTokens: 250,
          },
          rawBreakdown: {
            instructionTokens: 50,
            handoffTokens: 200,
            recentUserTokens: 100,
            protectedTailTokens: 600,
            otherVisibleTokens: 0,
            pendingUserTokens: 300,
            toolDeclarationTokens: 250,
          },
          trimmedToolResponseCount: 0,
          autoCompacted: false,
          degradedSummary: false,
        },
        sendChat: async () => {},
      },
    });

    expect(document.body.textContent).not.toContain("原始当前估算");
    expect(document.body.textContent).not.toContain("原始 Token 构成");
  });
});
