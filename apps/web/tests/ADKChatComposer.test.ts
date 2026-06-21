// @vitest-environment jsdom

import { mount } from "@vue/test-utils";
import { afterEach, describe, expect, it, vi } from "vitest";
import { defineComponent, h } from "vue";

import ADKChatComposer from "../src/components/adk-page/ADKChatComposer.vue";
import { inputStub, selectStub } from "./helpers";

afterEach(() => {
  document.body.innerHTML = "";
});

describe("ADKChatComposer", () => {
  it("shows zero percent healthy context for a new empty session", () => {
    const wrapper = mount(ADKChatComposer, {
      props: {
        layout: "mobile",
        canSendChat: false,
        chatDraft: "",
        sendingChat: false,
        selectedSessionId: "session-empty",
        contextSnapshot: {
          sessionId: "session-empty",
          currentInputTokens: 0,
          projectedNextTurnTokens: 0,
          rawCurrentInputTokens: 0,
          rawProjectedNextTurnTokens: 0,
          contextWindowTokens: 128000,
          usageRatio: 0,
          status: "healthy",
          recentUserWindow: 2,
          retainedRecentUserCount: 0,
          activeHandoffCount: 0,
          rawEventCount: 0,
          compactedEventCount: 0,
          summaryBoundaryEventIndex: 0,
          breakdown: {
            instructionTokens: 0,
            handoffTokens: 0,
            recentUserTokens: 0,
            protectedTailTokens: 0,
            otherVisibleTokens: 0,
            pendingUserTokens: 0,
            toolDeclarationTokens: 0,
          },
          autoCompacted: false,
          degradedSummary: false,
        },
        sendChat: async () => {},
      },
    });

    expect(wrapper.text()).toContain("0% 正常");
  });

  it("shows concrete work modes and marks the agent mode as default", () => {
    const wrapper = mount(ADKChatComposer, {
      attachTo: document.body,
      props: {
        canSendChat: true,
        chatDraft: "",
        defaultWorkMode: "task",
        sendChat: async () => {},
      },
      global: {
        stubs: {
          "v-select": workModeAwareSelectStub,
        },
      },
    });

    const text = wrapper.text();
    expect(text).not.toContain("跟随 Agent");
    expect(text).toContain("对话");
    expect(text).toContain("任务");
    expect(text).toContain("目标");
    expect(text).toContain("任务默认");
    expect(text).not.toContain("顺序");
    expect(text).not.toContain("并行");
  });

  it("stores the agent default mode as an empty override", async () => {
    const wrapper = mount(ADKChatComposer, {
      attachTo: document.body,
      props: {
        canSendChat: true,
        chatDraft: "",
        defaultWorkMode: "loop",
        workModeOverride: "task",
        sendChat: async () => {},
      },
      global: {
        stubs: {
          "v-select": selectStub,
        },
      },
    });

    const modeSelect = wrapper.find(".adk-work-mode-select");
    await modeSelect.setValue("loop");
    expect(wrapper.emitted("update:workModeOverride")?.[0]).toEqual([""]);

    await modeSelect.setValue("chat");
    expect(wrapper.emitted("update:workModeOverride")?.[1]).toEqual(["chat"]);
  });

  it("shows approval levels beside the add button and clears the default override", async () => {
    const menuStub = defineComponent({
      setup(_, { slots }) {
        return () =>
          h("div", [
            slots.activator?.({ props: {} }),
            slots.default?.(),
          ]);
      },
    });
    const listItemStub = defineComponent({
      emits: ["click"],
      setup(_, { emit, slots }) {
        return () =>
          h(
            "button",
            { type: "button", onClick: () => emit("click") },
            [slots.prepend?.(), slots.default?.()],
          );
      },
    });
    const wrapper = mount(ADKChatComposer, {
      attachTo: document.body,
      props: {
        canSendChat: true,
        chatDraft: "",
        defaultPermissionMode: "all",
        permissionModeOverride: "approval",
        sendingChat: false,
        sendChat: async () => {},
      },
      global: {
        stubs: {
          "v-menu": menuStub,
          "v-list": { template: "<div><slot /></div>" },
          "v-list-item": listItemStub,
          "v-icon": { template: "<i><slot /></i>" },
          "v-select": selectStub,
        },
      },
    });

    expect(wrapper.find(".adk-composer-left").text()).toContain("请求批准");
    expect(wrapper.text()).toContain("完全访问");
    expect(wrapper.text()).toContain("默认");

    const options = wrapper.findAll(".adk-permission-option");
    await options[2]!.trigger("click");
    expect(wrapper.emitted("update:permissionModeOverride")?.[0]).toEqual([""]);

    await options[1]!.trigger("click");
    expect(wrapper.emitted("update:permissionModeOverride")?.[1]).toEqual([
      "less_approval",
    ]);
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
          contextRevisionId: "ctx-current-7",
          contextRevisionCreatedAt: "2026-06-18T10:00:00Z",
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
          compactedEventCount: 7,
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

    expect(document.body.textContent).toContain("当前上下文 Token");
    expect(document.body.textContent).toContain("1,200");
    expect(document.body.textContent).toContain("当前上下文版本");
    expect(document.body.textContent).toContain("ctx-current-7");
    expect(document.body.textContent).toContain("已压缩事件数");
    expect(document.body.textContent).toContain("7");
    expect(document.body.textContent).toContain("版本创建时间");
    expect(document.body.textContent).toContain("2026-06-18T10:00:00Z");
    expect(document.body.textContent).toContain("原始会话当前估算");
    expect(document.body.textContent).toContain("57,502,105");
    expect(document.body.textContent).toContain("已裁剪工具响应");
    expect(document.body.textContent).toContain("5");
    expect(document.body.textContent).toContain("原始会话诊断 Token 构成");
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

    expect(document.body.textContent).not.toContain("原始会话当前估算");
    expect(document.body.textContent).not.toContain("原始会话诊断 Token 构成");
  });
});

const workModeAwareSelectStub = defineComponent({
  props: ["items"],
  setup(props) {
    return () =>
      h(
        "div",
        {},
        ((props.items as Array<{
          title: string;
          isDefault?: boolean;
        }>) ?? []).map((item) =>
          h("span", {}, `${item.title}${item.isDefault ? "默认" : ""}`),
        ),
      );
  },
});
