// @vitest-environment jsdom

import { mount } from "@vue/test-utils";
import { describe, expect, it } from "vitest";
import { nextTick } from "vue";

import type { ADKTimelineEntry } from "@/contracts";

import ADKChatThread from "../src/components/adk-page/ADKChatThread.vue";
import { createTimelineEntryState } from "../src/composables/adkTimeline";

function mountThread(entries: ADKTimelineEntry[]) {
  return mount(ADKChatThread, {
    props: {
      timelineEntries: entries.map((entry) => createTimelineEntryState(entry)),
      sendingChat: false,
      showTypingIndicator: false,
      errorMessage: "",
      approvalsBusy: false,
      suggestions: [],
      emptyStateTitle: "空",
      emptyStateHint: "空",
      approvalTool: () => undefined,
      clearErrorMessage: () => {},
      preview: (value: unknown) => String(value ?? ""),
      renderMarkdown: (content: string) => content,
      resolveApprovalGroup: () => {},
      resolveApproval: () => {},
    },
    global: {
      stubs: {
        ADKRunTrace: { template: "<div />" },
        "v-alert": { template: "<div><slot /></div>" },
        "v-chip": { template: "<button><slot /></button>" },
        "v-icon": { template: "<span><slot /></span>" },
        "v-progress-circular": { template: "<span />" },
      },
    },
  });
}

describe("ADKChatThread", () => {
  it("shows original user prompt by default and toggles processed prompt", async () => {
    const wrapper = mountThread([
      {
        id: "entry-user-prompt",
        sessionId: "session-1",
        runId: "run-1",
        kind: "user_message",
        createdAt: "2026-06-17T00:00:00Z",
        sequence: 1,
        text: "设计个适合 tme 的策略",
        originalText: "设计个适合 tme 的策略",
        processedText: "请推进这个目标。\n\n用户原始目标：设计个适合 tme 的策略",
      },
    ]);

    expect(wrapper.text()).toContain("设计个适合 tme 的策略");
    expect(wrapper.text()).not.toContain("请推进这个目标");
    expect(wrapper.find(".adk-bubble--user-processed").exists()).toBe(false);

    await wrapper.findAll("button").find((button) => button.text() === "系统处理后")?.trigger("click");
    await nextTick();

    expect(wrapper.text()).toContain("请推进这个目标");
    expect(wrapper.find(".adk-bubble--user-processed").exists()).toBe(true);

    await wrapper.findAll("button").find((button) => button.text() === "原文")?.trigger("click");
    await nextTick();

    expect(wrapper.text()).toContain("设计个适合 tme 的策略");
    expect(wrapper.text()).not.toContain("请推进这个目标");
    expect(wrapper.find(".adk-bubble--user-processed").exists()).toBe(false);
  });

  it("does not show a prompt toggle for unchanged user prompts", () => {
    const wrapper = mountThread([
      {
        id: "entry-user-plain",
        sessionId: "session-1",
        kind: "user_message",
        createdAt: "2026-06-17T00:00:00Z",
        sequence: 1,
        text: "普通问题",
      },
    ]);

    expect(wrapper.text()).toContain("普通问题");
    expect(wrapper.text()).not.toContain("系统处理后");
  });
});
