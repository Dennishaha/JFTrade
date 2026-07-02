// @vitest-environment jsdom

import { mount } from "@vue/test-utils";
import { createMemoryHistory, createRouter } from "vue-router";
import { afterEach, describe, expect, it, vi } from "vitest";
import { defineComponent, nextTick } from "vue";

const { fetchADKPageSessionDataMock } = vi.hoisted(() => ({
  fetchADKPageSessionDataMock: vi.fn(),
}));

vi.mock("../src/composables/adkPageSessionApi", () => ({
  fetchADKPageSessionData: (...args: unknown[]) =>
    fetchADKPageSessionDataMock(...args),
}));

import ADKPage from "../src/pages/ADKPage.vue";

const tabsStub = defineComponent({
  props: ["modelValue"],
  emits: ["update:modelValue"],
  template: `
    <div class='v-tabs-stub'>
      <button type='button' class='emit-agents-tab' @click="$emit('update:modelValue', 'agents')">agents</button>
      <button type='button' class='emit-workflows-tab' @click="$emit('update:modelValue', 'workflows')">workflows</button>
      <button type='button' class='emit-invalid-tab' @click="$emit('update:modelValue', 'invalid')">invalid</button>
      <slot />
    </div>
  `,
});

const tabStub = defineComponent({
  props: ["value"],
  template: "<button type='button' class='v-tab-stub'><slot /></button>",
});

const alertStub = defineComponent({
  template: "<div class='alert-stub'><slot /></div>",
});

const progressStub = defineComponent({
  template: "<div class='progress-stub'>loading</div>",
});

const workspaceShellStub = defineComponent({
  props: ["layout"],
  template:
    "<div class='adk-workspace-shell-stub'>workspace:{{ layout }}</div>",
});

const workflowStudioStub = defineComponent({
  props: ["agents", "providers", "formatDateTime", "viewMode"],
  template:
    "<div class='adk-workflow-studio-stub'>workflow:{{ viewMode }} agents:{{ agents.length }} providers:{{ providers.length }} formatted:{{ formatDateTime('2026-07-03T00:00:00Z') }}</div>",
});

function buildPageData() {
  return {
    agents: [
      {
        id: "agent-1",
        name: "Agent One",
        instruction: "",
        providerId: "provider-1",
        model: "gpt-4o-mini",
        tools: [],
        skills: [],
        permissionMode: "approval",
        memoryEnabled: true,
        recentUserWindow: 6,
        workMode: "chat",
        loopMaxIterations: 5,
        status: "ENABLED",
        createdAt: "2026-07-03T00:00:00Z",
        updatedAt: "2026-07-03T00:00:00Z",
      },
    ],
    providers: [
      {
        id: "provider-1",
        displayName: "OpenAI",
        baseUrl: "https://api.openai.com/v1",
        model: "gpt-4o-mini",
        requestTimeoutMs: 180000,
        enabled: true,
        default: true,
        hasApiKey: true,
        createdAt: "2026-07-03T00:00:00Z",
        updatedAt: "2026-07-03T00:00:00Z",
      },
    ],
    sessions: [],
    approvals: [],
    tools: [],
  };
}

async function mountPage(path: "/adk/agents" | "/adk/workflows") {
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: "/adk/agents", component: { template: "<div />" } },
      { path: "/adk/workflows", component: { template: "<div />" } },
    ],
  });
  await router.push(path);
  await router.isReady();

  const wrapper = mount(ADKPage, {
    global: {
      plugins: [router],
      stubs: {
        "v-tabs": tabsStub,
        "v-tab": tabStub,
        "v-alert": alertStub,
        "v-progress-linear": progressStub,
        ADKWorkspaceShell: workspaceShellStub,
        ADKWorkflowStudio: workflowStudioStub,
      },
    },
  });

  return { wrapper, router };
}

async function flushUi() {
  await Promise.resolve();
  await nextTick();
  await Promise.resolve();
  await nextTick();
}

afterEach(() => {
  vi.restoreAllMocks();
  fetchADKPageSessionDataMock.mockReset();
  document.body.innerHTML = "";
});

describe("ADKPage workflow container", () => {
  it("loads workflow resources once on direct workflow routes and shows the loading state", async () => {
    let resolveFetch: ((value: ReturnType<typeof buildPageData>) => void) | null =
      null;
    fetchADKPageSessionDataMock.mockImplementationOnce(
      () =>
        new Promise((resolve) => {
          resolveFetch = resolve;
        }),
    );

    const { wrapper } = await mountPage("/adk/workflows");
    await flushUi();

    expect(fetchADKPageSessionDataMock).toHaveBeenCalledTimes(1);
    expect(wrapper.find(".progress-stub").exists()).toBe(true);
    expect(wrapper.find(".adk-workflow-studio-stub").exists()).toBe(false);

    resolveFetch?.(buildPageData());
    await flushUi();

    expect(wrapper.find(".progress-stub").exists()).toBe(false);
    expect(wrapper.find(".adk-workflow-studio-stub").text()).toContain(
      "workflow:workflows agents:1 providers:1",
    );
    expect(wrapper.find(".adk-workspace-shell-stub").exists()).toBe(false);
  });

  it("switches between agents and workflows, retries failed workflow loads, and preserves query parameters", async () => {
    fetchADKPageSessionDataMock.mockRejectedValueOnce(
      new Error("workflow load failed"),
    );

    const { wrapper, router } = await mountPage("/adk/agents");
    await router.push({ path: "/adk/agents", query: { q: "keep-me" } });
    await router.isReady();
    await flushUi();

    expect(wrapper.find(".adk-workspace-shell-stub").text()).toContain(
      "workspace:desktop",
    );
    expect(fetchADKPageSessionDataMock).not.toHaveBeenCalled();

    await router.push({ path: "/adk/workflows", query: { q: "keep-me" } });
    await router.isReady();
    await flushUi();

    expect(router.currentRoute.value.path).toBe("/adk/workflows");
    expect(router.currentRoute.value.query).toEqual({ q: "keep-me" });
    expect(fetchADKPageSessionDataMock).toHaveBeenCalledTimes(1);
    expect(wrapper.text()).toContain("workflow load failed");
    expect(wrapper.find(".adk-workflow-studio-stub").text()).toContain(
      "agents:0 providers:0",
    );

    fetchADKPageSessionDataMock.mockResolvedValueOnce(buildPageData());
    await router.push({ path: "/adk/agents", query: { q: "keep-me" } });
    await router.isReady();
    await flushUi();
    await router.push({ path: "/adk/workflows", query: { q: "keep-me" } });
    await router.isReady();
    await flushUi();

    expect(fetchADKPageSessionDataMock).toHaveBeenCalledTimes(2);
    expect(wrapper.find(".adk-workflow-studio-stub").text()).toContain("formatted:");
  });

});
