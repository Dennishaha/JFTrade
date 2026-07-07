// @vitest-environment jsdom

import { mount } from "@vue/test-utils";
import { describe, expect, it, vi } from "vitest";
import { defineComponent, h } from "vue";

import ADKChatComposer from "../src/components/adk-page/ADKChatComposer.vue";
import type { ADKProvider } from "../src/contracts";

const menuStub = defineComponent({
  props: ["modelValue"],
  emits: ["update:modelValue"],
  setup(_, { slots }) {
    return () => h("div", [slots.activator?.({ props: {} }), slots.default?.()]);
  },
});

const menuModelStub = defineComponent({
  props: ["modelValue"],
  emits: ["update:modelValue"],
  setup(props, { slots, emit }) {
    return () =>
      h("div", [
        slots.activator?.({ props: {} }),
        slots.default?.(),
        h(
          "button",
          {
            type: "button",
            class: "menu-model-toggle",
            onClick: () => emit("update:modelValue", !props.modelValue),
          },
          "toggle",
        ),
      ]);
  },
});

const safeButtonStub = defineComponent({
  props: ["disabled", "loading", "title"],
  emits: ["click"],
  template:
    "<button type='button' :disabled='disabled' :title='title' @click=\"$emit('click')\"><slot /></button>",
});

const listStub = { template: "<div><slot /></div>" };

const listItemStub = defineComponent({
  props: ["disabled", "active"],
  emits: ["click"],
  template:
    "<button type='button' :disabled='disabled' :data-active='active' @click=\"$emit('click')\"><slot name='prepend' /><slot /><slot name='append' /></button>",
});

const passthroughStub = defineComponent({
  setup(_, { slots }) {
    return () => h("div", slots.default?.());
  },
});

describe("ADKChatComposer business flows", () => {
  it("applies explicit permission overrides and propagates agent/provider changes", async () => {
    const handleAgentChange = vi.fn();
    const handleProviderChange = vi.fn();
    const openProviderSettings = vi.fn();
    const wrapper = mountComposer({
      canSendChat: true,
      chatDraft: "",
      defaultPermissionMode: "approval",
      permissionModeOverride: "all",
      selectedAgentId: "agent-1",
      selectedProviderId: "provider-2",
      selectedProvider: null,
      agentOptions: [
        { title: "交易助手 · approval", value: "agent-1" },
        { title: "目标助手 · approval", value: "agent-2" },
      ],
      providerOptions: [
        { title: "默认模型 · OpenAI · gpt-4.1", value: "", providerId: "provider-1", model: "gpt-4.1", isDefault: true },
        { title: "Claude · claude-sonnet", value: "provider-2", providerId: "provider-2", model: "claude-sonnet" },
      ],
      handleAgentChange,
      handleProviderChange,
      openProviderSettings,
      sendChat: async () => {},
    });

    expect(wrapper.text()).toContain("完全访问");
    expect(wrapper.text()).toContain("默认");

    await wrapper.find(".adk-agent-select").setValue("agent-2");
    expect(wrapper.emitted("update:selectedAgentId")?.[0]).toEqual(["agent-2"]);
    expect(handleAgentChange).toHaveBeenCalledOnce();

    await wrapper.find(".adk-provider-select").setValue("provider-2");
    expect(wrapper.emitted("update:selectedProviderId")?.[0]).toEqual([
      "provider-2",
    ]);
    expect(handleProviderChange).toHaveBeenCalledWith("provider-2");

    await wrapper.findAll("button").find((button) => button.attributes("title") === "添加模型服务")!.trigger("click");
    await wrapper.findAll("button").find((button) => button.attributes("title") === "Agent 设置")!.trigger("click");
    expect(openProviderSettings).toHaveBeenCalledTimes(2);

    await wrapper.find(".adk-work-mode-select").setValue("loop");
    expect(wrapper.emitted("update:workModeOverride")?.at(-1)).toEqual(["loop"]);
  });

  it("navigates slash commands from the keyboard and falls back to normal composer handling", async () => {
    const runSlashCommand = vi.fn();
    const handleComposerKeydown = vi.fn();
    const wrapper = mountComposer({
      canSendChat: true,
      chatDraft: "/",
      sendingChat: false,
      slashCommands: [
        {
          id: "context",
          command: "/context",
          title: "上下文",
          description: "查看上下文",
        },
        {
          id: "compact",
          command: "/compact",
          title: "压缩",
          description: "压缩上下文",
        },
      ],
      runSlashCommand,
      handleComposerKeydown,
      sendChat: async () => {},
    });

    const input = wrapper.find("textarea");
    await input.trigger("keydown", {
      key: "ArrowDown",
      preventDefault: vi.fn(),
    });
    expect(wrapper.findAll(".adk-slash-menu__item")[1]?.classes()).toContain(
      "adk-slash-menu__item--active",
    );

    await input.trigger("keydown", {
      key: "ArrowUp",
      preventDefault: vi.fn(),
    });
    expect(wrapper.findAll(".adk-slash-menu__item")[0]?.classes()).toContain(
      "adk-slash-menu__item--active",
    );

    await input.trigger("keydown", {
      key: "Escape",
      preventDefault: vi.fn(),
    });
    expect(wrapper.find(".adk-slash-menu").exists()).toBe(false);

    await wrapper.setProps({ chatDraft: "/context" });
    await wrapper.find(".adk-composer-send").trigger("click");
    expect(runSlashCommand).toHaveBeenCalledWith("context");
    expect(wrapper.emitted("update:chatDraft")?.at(-1)).toEqual([""]);

    await wrapper.setProps({ chatDraft: "普通问题" });
    await input.trigger("keydown", { key: "Enter" });
    expect(handleComposerKeydown).toHaveBeenCalled();
  });

  it("handles provider picks from the menu list and dispatches interrupt-send actions", async () => {
    const handleProviderChange = vi.fn();
    const interruptAndQueueChat = vi.fn();
    const wrapper = mountComposer({
      canSendChat: true,
      canInterruptChat: true,
      chatDraft: "排队发送这个请求",
      selectedAgentId: "agent-1",
      selectedProviderId: "provider-1",
      providerOptions: [
        { title: "默认模型 · OpenAI · gpt-4.1", value: "provider-1", providerId: "provider-1", model: "gpt-4.1", isDefault: true },
        { title: "Claude · claude-sonnet", value: "provider-2", providerId: "provider-2", model: "claude-sonnet" },
      ],
      handleProviderChange,
      interruptAndQueueChat,
      sendChat: async () => {},
    });

    const providerItems = wrapper.findAll(".adk-provider-menu button");
    await providerItems[1]!.trigger("click");
    expect(handleProviderChange).toHaveBeenCalledWith("provider-2");

    await wrapper.find(".adk-composer-interrupt").trigger("click");
    expect(interruptAndQueueChat).toHaveBeenCalledOnce();
  });

  it("picks agents and work modes from compact menus while keeping the context menu available", async () => {
    const handleAgentChange = vi.fn();
    const wrapper = mountComposer({
      canSendChat: true,
      chatDraft: "",
      selectedAgentId: "agent-1",
      selectedSessionId: "session-1",
      contextSnapshot: {
        sessionId: "session-1",
        currentInputTokens: 120,
        projectedNextTurnTokens: 180,
        rawCurrentInputTokens: 120,
        rawProjectedNextTurnTokens: 180,
        contextWindowTokens: 4_000,
        usageRatio: 0.05,
        status: "healthy",
        recentUserWindow: 2,
        retainedRecentUserCount: 1,
        activeHandoffCount: 0,
        rawEventCount: 1,
        compactedEventCount: 0,
        summaryBoundaryEventIndex: 0,
        breakdown: undefined,
        rawBreakdown: undefined,
        latestHandoffPreview: "",
        summaryPreview: "",
        lastCompactionMode: "manual",
        autoCompacted: false,
        degradedSummary: false,
      },
      agentOptions: [
        { title: "交易助手 · approval", value: "agent-1" },
        { title: "目标助手 · approval", value: "agent-2" },
      ],
      handleAgentChange,
      sendChat: async () => {},
    });

    const compactMenus = wrapper.findAll(".adk-compact-menu");
    await compactMenus[0]!.findAll("button")[1]!.trigger("click");
    expect(wrapper.emitted("update:selectedAgentId")?.at(-1)).toEqual(["agent-2"]);
    expect(handleAgentChange).toHaveBeenCalledOnce();

    await compactMenus[1]!.findAll("button")[1]!.trigger("click");
    expect(wrapper.emitted("update:workModeOverride")?.at(-1)).toEqual(["loop"]);
    expect(wrapper.text()).toContain("上下文");
  });

  it("covers goal lifecycle actions, context warning states, and stop handling", async () => {
    const cancelGoalObjective = vi.fn();
    const resumeGoalRun = vi.fn();
    const pauseGoalRun = vi.fn();
    const cancelActiveRun = vi.fn();
    const wrapper = mountComposer({
      layout: "mobile",
      canSendChat: true,
      chatDraft: "",
      sendingChat: true,
      activeRunId: "run-1",
      hasBlockingRun: true,
      cancelActiveRun,
      showGoalObjectiveEditor: true,
      goalObjectiveDraft: "处理风控异常",
      goalObjectiveError: "保存失败",
      cancelGoalObjective,
      canResumeGoal: true,
      goalPaused: true,
      resumeGoalRun,
      contextDetailsOpen: true,
      contextSnapshot: {
        sessionId: "session-1",
        currentInputTokens: 1200,
        projectedNextTurnTokens: 1500,
        rawCurrentInputTokens: 1200,
        rawProjectedNextTurnTokens: 1500,
        contextWindowTokens: 0,
        usageRatio: 0.9,
        status: "warning",
        recentUserWindow: 2,
        retainedRecentUserCount: 1,
        activeHandoffCount: 1,
        rawEventCount: 2,
        compactedEventCount: 1,
        summaryBoundaryEventIndex: 0,
        breakdown: {
          instructionTokens: 50,
          handoffTokens: 10,
          recentUserTokens: 100,
          protectedTailTokens: 700,
          otherVisibleTokens: 0,
          pendingUserTokens: 300,
          toolDeclarationTokens: 40,
        },
        rawBreakdown: undefined,
        latestHandoffPreview: "",
        summaryPreview: "",
        lastCompactionMode: "aggressive",
        autoCompacted: false,
        degradedSummary: false,
      },
      sendChat: async () => {},
    });

    expect(wrapper.text()).toContain("保存失败");
    expect(wrapper.text()).toContain("1,200 Tokens");
    await wrapper.find("[data-testid='adk-mobile-composer-toggle']").trigger("click");
    expect(wrapper.text()).toContain("注意");
    expect(wrapper.text()).toContain("激进");
    expect(wrapper.text()).toContain("暂无 handoff 摘要");

    await wrapper.find("[aria-label='取消目标']").trigger("click");
    expect(cancelGoalObjective).toHaveBeenCalledOnce();

    await wrapper.find("[title='运行目标']").trigger("click");
    expect(resumeGoalRun).toHaveBeenCalledOnce();

    expect(wrapper.find("[data-testid='adk-mobile-controls-panel']").exists()).toBe(true);
    await wrapper.setProps({
      layout: "desktop",
      canPauseGoal: true,
      canResumeGoal: false,
      goalPaused: false,
      goalPauseRequested: false,
      goalLifecycleBusy: false,
      pauseGoalRun,
      contextSnapshot: {
        sessionId: "session-1",
        currentInputTokens: 0,
        projectedNextTurnTokens: 0,
        rawCurrentInputTokens: 0,
        rawProjectedNextTurnTokens: 0,
        contextWindowTokens: 0,
        usageRatio: 0,
        status: "critical",
        recentUserWindow: 2,
        retainedRecentUserCount: 0,
        activeHandoffCount: 0,
        rawEventCount: 0,
        compactedEventCount: 0,
        summaryBoundaryEventIndex: 0,
        breakdown: undefined,
        rawBreakdown: undefined,
        latestHandoffPreview: "",
        summaryPreview: "",
        lastCompactionMode: "manual",
        autoCompacted: false,
        degradedSummary: false,
      },
    });
    expect(wrapper.find("[data-testid='adk-mobile-controls-panel']").exists()).toBe(false);
    expect(wrapper.text()).toContain("危险");
    expect(wrapper.text()).toContain("手动");

    await wrapper.find("[title='暂停目标']").trigger("click");
    expect(pauseGoalRun).toHaveBeenCalledOnce();

    await wrapper.find(".adk-composer-stop").trigger("click");
    expect(cancelActiveRun).toHaveBeenCalledOnce();
  });

  it("renders queue states, revokes eligible items, and emits draft updates", async () => {
    const revokeQueuedMessage = vi.fn();
    const wrapper = mountComposer({
      canSendChat: true,
      chatDraft: "初始草稿",
      hasBlockingRun: true,
      queueDispatchingId: "msg-3",
      queuedMessages: [
        {
          id: "msg-1",
          sessionKey: "session-1",
          text: "先中断当前运行",
          mode: "interrupt",
          createdAt: "2026-07-03T00:00:00Z",
        },
        {
          id: "msg-2",
          sessionKey: "session-1",
          text: "然后继续排队",
          mode: "queued",
          createdAt: "2026-07-03T00:01:00Z",
        },
        {
          id: "msg-3",
          sessionKey: "session-1",
          text: "正在发送下一条",
          mode: "queued",
          createdAt: "2026-07-03T00:02:00Z",
        },
      ],
      revokeQueuedMessage,
      sendChat: async () => {},
    });

    const badges = wrapper.findAll(".adk-queue-item__badge");
    expect(badges.map((badge) => badge.text())).toEqual([
      "interrupting",
      "queued",
      "sending next",
    ]);
    expect(badges[0]?.classes()).toContain("is-interrupting");
    expect(badges[2]?.classes()).toContain("is-sending-next");

    const revokeButtons = wrapper.findAll(".adk-queue-item__remove");
    expect(revokeButtons[2]?.attributes("disabled")).toBeDefined();
    await revokeButtons[0]!.trigger("click");
    expect(revokeQueuedMessage).toHaveBeenCalledWith("msg-1");

    await wrapper.find("textarea").setValue("新的输入");
    expect(wrapper.emitted("update:chatDraft")?.at(-1)).toEqual(["新的输入"]);
  });

  it("executes slash items from click and syncs the context menu model", async () => {
    const runSlashCommand = vi.fn();
    const openContextDetails = vi.fn();
    const wrapper = mountComposer(
      {
        canSendChat: true,
        chatDraft: "/context",
        selectedSessionId: "session-1",
        contextDetailsOpen: false,
        contextSnapshot: {
          sessionId: "session-1",
          currentInputTokens: 320,
          projectedNextTurnTokens: 500,
          rawCurrentInputTokens: 320,
          rawProjectedNextTurnTokens: 500,
          contextWindowTokens: 4000,
          usageRatio: 0.08,
          status: "healthy",
          recentUserWindow: 2,
          retainedRecentUserCount: 1,
          activeHandoffCount: 0,
          rawEventCount: 2,
          compactedEventCount: 0,
          summaryBoundaryEventIndex: 0,
          breakdown: undefined,
          rawBreakdown: undefined,
          latestHandoffPreview: "",
          summaryPreview: "",
          lastCompactionMode: "manual",
          autoCompacted: false,
          degradedSummary: false,
        },
        slashCommands: [
          {
            id: "context",
            command: "/context",
            title: "上下文",
            description: "查看上下文窗口",
          },
        ],
        openContextDetails,
        runSlashCommand,
        sendChat: async () => {},
      },
      {
        "v-menu": menuModelStub,
      },
    );

    await wrapper.find(".adk-slash-menu__item").trigger("click");
    expect(runSlashCommand).toHaveBeenCalledWith("context");
    expect(wrapper.emitted("update:chatDraft")?.at(-1)).toEqual([""]);

    await wrapper.find(".adk-context-pill").trigger("click");
    expect(openContextDetails).toHaveBeenCalledOnce();
    expect(wrapper.emitted("update:contextDetailsOpen")?.at(-1)).toEqual([
      true,
    ]);

    await wrapper.setProps({ contextDetailsOpen: true });
    const contextToggle = wrapper.get(".adk-composer-utility .menu-model-toggle");
    await contextToggle.trigger("click");
    expect(wrapper.emitted("update:contextDetailsOpen")?.at(-1)).toEqual([
      false,
    ]);
  });
});

function mountComposer(
  props: Partial<InstanceType<typeof ADKChatComposer>["$props"]> & {
    canSendChat: boolean;
    chatDraft: string;
    sendChat: () => void | Promise<void>;
  },
  stubs: Record<string, unknown> = {},
) {
  return mount(ADKChatComposer, {
    attachTo: document.body,
    props: {
      sendingChat: false,
      ...props,
    },
    global: {
      stubs: {
        "v-btn": safeButtonStub,
        "v-card": passthroughStub,
        "v-card-text": passthroughStub,
        "v-card-title": passthroughStub,
        "v-chip": { template: "<span><slot /></span>" },
        "v-icon": { template: "<i><slot /></i>" },
        "v-list": listStub,
        "v-list-item": listItemStub,
        "v-list-item-title": passthroughStub,
        "v-list-item-subtitle": passthroughStub,
        "v-menu": menuStub,
        "v-progress-circular": passthroughStub,
        "v-progress-linear": passthroughStub,
        "v-select": {
          props: ["modelValue", "items"],
          emits: ["update:modelValue"],
          template:
            "<select :value='modelValue' @change=\"$emit('update:modelValue', $event.target.value)\"><option v-for='item in items' :key='item.value ?? item' :value='item.value ?? item'>{{ item.title ?? item }}</option></select>",
        },
        "v-textarea": {
          props: ["modelValue"],
          emits: ["update:modelValue"],
          template:
            "<textarea :value='modelValue' @input=\"$emit('update:modelValue', $event.target.value)\"></textarea>",
        },
        ...stubs,
      },
    },
  });
}

function buildProvider(overrides: Partial<ADKProvider> = {}): ADKProvider {
  return {
    id: "provider-1",
    displayName: "OpenAI",
    baseUrl: "https://api.openai.com/v1",
    model: "gpt-4.1",
    requestTimeoutMs: 180_000,
    enabled: true,
    default: true,
    hasApiKey: true,
    createdAt: "2026-07-01T00:00:00Z",
    updatedAt: "2026-07-01T00:00:00Z",
    ...overrides,
  };
}
