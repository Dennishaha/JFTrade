// @vitest-environment jsdom

import { mount } from "@vue/test-utils";
import { afterEach, describe, expect, it, vi } from "vitest";

import ADKChatComposer from "../src/components/adk-page/ADKChatComposer.vue";
import { inputStub, selectStub } from "./helpers";

afterEach(() => {
  document.body.innerHTML = "";
});

describe("ADKChatComposer", () => {
  it("hides sequential and parallel work mode entries from the composer", () => {
    const wrapper = mount(ADKChatComposer, {
      attachTo: document.body,
      props: {
        canSendChat: true,
        chatDraft: "",
        sendChat: async () => {},
      },
      global: {
        stubs: {
          "v-select": selectStub,
        },
      },
    });

    const text = wrapper.text();
    expect(text).toContain("跟随 Agent");
    expect(text).toContain("对话");
    expect(text).toContain("任务");
    expect(text).toContain("目标");
    expect(text).not.toContain("顺序");
    expect(text).not.toContain("并行");
  });

  it("shows an editable goal objective box and saves active goal changes", async () => {
    const updateDraft = vi.fn();
    const saveGoal = vi.fn();
    const wrapper = mount(ADKChatComposer, {
      attachTo: document.body,
      props: {
        canSendChat: true,
        chatDraft: "生成交易计划",
        sendingChat: false,
        sendChat: async () => {},
        activeRunId: "run-goal",
        showGoalObjectiveEditor: true,
        goalObjectiveDraft: "生成交易计划并检查风险",
        canSaveGoalObjective: true,
        updateGoalObjectiveDraft: updateDraft,
        updateGoalObjective: saveGoal,
      },
      global: {
        stubs: {
          "v-btn": {
            props: ["disabled", "loading"],
            emits: ["click"],
            template:
              "<button type='button' :disabled='disabled' @click=\"$emit('click')\"><slot /></button>",
          },
          "v-select": selectStub,
          "v-textarea": inputStub,
        },
      },
    });

    expect(wrapper.text()).toContain("目标");
    expect(wrapper.text()).toContain("已修改");
    expect(wrapper.find(".adk-goal-editor input").exists()).toBe(false);

    await wrapper.find('[aria-label="编辑目标"]').trigger("click");
    const input = wrapper.find(".adk-goal-editor input");
    await input.setValue("新的目标");
    expect(updateDraft).toHaveBeenCalledWith("新的目标");

    const saveButton = wrapper
      .findAll(".adk-goal-editor button")
      .find((button) => button.text().includes("保存"));
    expect(saveButton).toBeTruthy();
    await saveButton!.trigger("click");
    expect(saveGoal).toHaveBeenCalledTimes(1);
  });

  it("cancels goal mode from the goal objective card", async () => {
    const wrapper = mount(ADKChatComposer, {
      attachTo: document.body,
      props: {
        canSendChat: true,
        chatDraft: "生成交易计划",
        sendingChat: false,
        sendChat: async () => {},
        showGoalObjectiveEditor: true,
        goalObjectiveDraft: "生成交易计划",
        workModeOverride: "loop",
      },
      global: {
        stubs: {
          "v-icon": true,
          "v-select": selectStub,
        },
      },
    });

    await wrapper.find('[aria-label="取消目标"]').trigger("click");
    expect(wrapper.emitted("update:workModeOverride")?.[0]).toEqual([""]);
  });

  it("cancels the active goal run from the goal objective card", async () => {
    const cancelRun = vi.fn();
    const wrapper = mount(ADKChatComposer, {
      attachTo: document.body,
      props: {
        activeRunId: "run-goal",
        canSendChat: true,
        chatDraft: "生成交易计划",
        sendingChat: true,
        sendChat: async () => {},
        showGoalObjectiveEditor: true,
        goalObjectiveDraft: "生成交易计划",
        cancelActiveRun: cancelRun,
      },
      global: {
        stubs: {
          "v-icon": true,
          "v-select": selectStub,
        },
      },
    });

    await wrapper.find('[aria-label="取消目标"]').trigger("click");
    expect(cancelRun).toHaveBeenCalledTimes(1);
    expect(wrapper.emitted("update:workModeOverride")).toBeUndefined();
  });

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
