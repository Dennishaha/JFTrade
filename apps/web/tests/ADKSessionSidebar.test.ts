// @vitest-environment jsdom

import { mount } from "@vue/test-utils";
import { describe, expect, it } from "vitest";

import type { ADKSession } from "@/contracts";
import type { ADKSessionGroup } from "@/composables/useADKPageSessionState";

import ADKSessionSidebar from "../src/components/adk-page/ADKSessionSidebar.vue";

interface SidebarOverrides {
  visibleSessionGroups?: ADKSessionGroup[];
  showSessionGroups?: boolean;
}

describe("ADKSessionSidebar", () => {
  it("distinguishes an empty list from filters with no matches", async () => {
    const wrapper = mountSidebar([]);
    expect(wrapper.text()).toContain("暂无会话");

    await wrapper.setProps({
      sessions: [buildSession()],
      visibleSessions: [],
    });
    expect(wrapper.text()).toContain("没有匹配会话");
    expect(wrapper.text()).not.toContain("暂无会话");
  });

  it("keeps the flat list for the default conversation group", () => {
    const wrapper = mountSidebar([buildSession({ title: "普通对话" })]);

    expect(wrapper.find(".adk-session-group__header").exists()).toBe(false);
    expect(wrapper.text()).toContain("普通对话");
  });

  it("renders workflow group headers when workflow sessions are present", () => {
    const defaultSession = buildSession({ title: "普通对话" });
    const workflowSession = buildSession({
      id: "session-workflow",
      title: "工作流对话",
      workflowId: "workflow-1",
      workflowName: "每日盘点",
    });
    const wrapper = mountSidebar([defaultSession, workflowSession], {
      showSessionGroups: true,
      visibleSessionGroups: [
        {
          id: "__default_conversation__",
          title: "对话",
          sessions: [defaultSession],
          isDefault: true,
        },
        {
          id: "workflow-1",
          title: "每日盘点",
          sessions: [workflowSession],
          isDefault: false,
        },
      ],
    });

    const headers = wrapper.findAll(".adk-session-group__header");
    expect(
      headers.map((header) => header.find("span:first-child").text()),
    ).toEqual(["对话", "每日盘点"]);
    expect(headers.map((header) => header.find("small").text())).toEqual(["1", "1"]);
    expect(wrapper.findAll(".adk-session-item")).toHaveLength(2);
  });

  it("collapses and expands grouped sessions with the caret button", async () => {
    const defaultSession = buildSession({ title: "普通对话" });
    const workflowSession = buildSession({
      id: "session-workflow",
      title: "工作流对话",
      workflowId: "workflow-1",
      workflowName: "每日盘点",
    });
    const wrapper = mountSidebar([defaultSession, workflowSession], {
      showSessionGroups: true,
      visibleSessionGroups: [
        {
          id: "__default_conversation__",
          title: "对话",
          sessions: [defaultSession],
          isDefault: true,
        },
        {
          id: "workflow-1",
          title: "每日盘点",
          sessions: [workflowSession],
          isDefault: false,
        },
      ],
    });

    const toggles = wrapper.findAll(".adk-session-group__toggle");
    expect(toggles.map((toggle) => toggle.text())).toEqual([
      "fa-solid fa-chevron-up",
      "fa-solid fa-chevron-up",
    ]);

    await toggles[1]!.trigger("click");

    expect(wrapper.findAll(".adk-session-group__toggle")[1]!.text()).toBe(
      "fa-solid fa-chevron-down",
    );
    expect(wrapper.text()).toContain("普通对话");
    expect(wrapper.text()).not.toContain("工作流对话");
    expect(wrapper.findAll(".adk-session-item")).toHaveLength(1);

    await wrapper.findAll(".adk-session-group__toggle")[1]!.trigger("click");

    expect(wrapper.findAll(".adk-session-group__toggle")[1]!.text()).toBe(
      "fa-solid fa-chevron-up",
    );
    expect(wrapper.text()).toContain("工作流对话");
    expect(wrapper.findAll(".adk-session-item")).toHaveLength(2);
  });
});

function mountSidebar(
  sessions: ADKSession[],
  overrides: SidebarOverrides = {},
) {
  return mount(ADKSessionSidebar, {
    props: {
      selectedAgentId: "agent-1",
      selectedSessionId: "",
      selectedAgent: null,
      sessionSearch: "",
      sessionAgentFilter: "",
      agentOptions: [],
      visibleSessions: sessions,
      visibleSessionGroups: [
        {
          id: "__default_conversation__",
          title: "对话",
          sessions,
          isDefault: true,
        },
      ],
      showSessionGroups: false,
      sessions,
      formatPermission: (mode: string) => mode,
      sessionTitle: (session) => session.title,
      agentName: (agentId: string) => agentId,
      creatingSession: false,
      createNewSession: () => undefined,
      selectSession: () => undefined,
      renameSession: () => undefined,
      deleteSession: () => undefined,
      ...overrides,
    },
    global: {
      stubs: {
        "v-btn": { template: "<button><slot /></button>" },
        "v-chip": { template: "<span><slot /></span>" },
        "v-icon": { template: "<span><slot /></span>" },
        "v-select": { template: "<select />" },
        "v-text-field": { template: "<input />" },
      },
    },
  });
}

function buildSession(overrides: Partial<ADKSession> = {}): ADKSession {
  return {
    id: "session-1",
    agentId: "agent-1",
    title: "会话",
    createdAt: "2026-06-18T00:00:00Z",
    updatedAt: "2026-06-18T00:00:00Z",
    ...overrides,
  };
}
