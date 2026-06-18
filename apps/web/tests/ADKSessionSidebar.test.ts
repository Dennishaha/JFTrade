// @vitest-environment jsdom

import { mount } from "@vue/test-utils";
import { describe, expect, it } from "vitest";

import ADKSessionSidebar from "../src/components/adk-page/ADKSessionSidebar.vue";

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
});

function mountSidebar(
  sessions: Array<ReturnType<typeof buildSession>>,
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
      sessions,
      formatPermission: (mode: string) => mode,
      sessionTitle: (session) => session.title,
      agentName: (agentId: string) => agentId,
      creatingSession: false,
      createNewSession: () => undefined,
      selectSession: () => undefined,
      renameSession: () => undefined,
      deleteSession: () => undefined,
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

function buildSession() {
  return {
    id: "session-1",
    agentId: "agent-1",
    title: "会话",
    createdAt: "2026-06-18T00:00:00Z",
    updatedAt: "2026-06-18T00:00:00Z",
  };
}
