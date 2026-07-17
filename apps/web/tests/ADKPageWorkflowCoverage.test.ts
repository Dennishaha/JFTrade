// @vitest-environment jsdom

import { flushPromises, mount } from "@vue/test-utils";
import { createMemoryHistory, createRouter } from "vue-router";
import { describe, expect, it, vi } from "vitest";

const pageApi = vi.hoisted(() => ({ fetchADKPageSessionData: vi.fn() }));

vi.mock("../src/composables/adkPageSessionApi", () => ({
  fetchADKPageSessionData: pageApi.fetchADKPageSessionData,
}));

import ADKPage from "../src/pages/ADKPage.vue";

const tabsStub = {
  props: ["modelValue"],
  emits: ["update:modelValue"],
  template: "<div data-testid='tabs'><slot /></div>",
};
const tabStub = { template: "<button type='button'><slot /></button>" };
const shellStub = { template: "<section data-testid='adk-workspace-shell' />" };
const studioStub = {
  props: ["agents", "providers", "formatDateTime", "viewMode"],
  template: "<section data-testid='adk-workflow-studio'>{{ agents.length }} agents / {{ providers.length }} providers / {{ formatDateTime('2026-01-01T00:00:00Z') }}</section>",
};
const alertStub = { template: "<div role='alert'><slot /></div>" };
const progressStub = { template: "<div data-testid='workflow-loading' />" };

async function mountADKWorkflowView(path: string) {
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [{ path: "/adk/:view?", component: ADKPage }],
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
        ADKWorkspaceShell: shellStub,
        ADKWorkflowStudio: studioStub,
      },
    },
  });
  await flushPromises();
  return { router, wrapper };
}

describe("ADKPage workflow view coverage", () => {
  it("keeps agents lightweight until the workflow studio is selected", async () => {
    pageApi.fetchADKPageSessionData.mockResolvedValue({ agents: [], providers: [] });
    const { wrapper } = await mountADKWorkflowView("/adk/agents");
    expect(wrapper.get("[data-testid='adk-workspace-shell']").exists()).toBe(true);
    expect(pageApi.fetchADKPageSessionData).not.toHaveBeenCalled();
  });

  it("loads workflow resources once and passes them to the studio", async () => {
    pageApi.fetchADKPageSessionData.mockResolvedValue({
      agents: [{ id: "agent-1", name: "研究员" }],
      providers: [{ id: "provider-1", name: "Provider" }],
    });
    const { wrapper } = await mountADKWorkflowView("/adk/workflows");
    await flushPromises();
    expect(pageApi.fetchADKPageSessionData).toHaveBeenCalledTimes(1);
    expect(wrapper.get("[data-testid='adk-workflow-studio']").text()).toContain("1 agents / 1 providers");
    expect(wrapper.text()).toContain("2026-01-01");
  });

  it("shows a workflow-resource failure without replacing the workflow route", async () => {
    pageApi.fetchADKPageSessionData.mockRejectedValue(new Error("资源服务不可用"));
    const { router, wrapper } = await mountADKWorkflowView("/adk/workflows");
    expect(router.currentRoute.value.path).toBe("/adk/workflows");
    expect(wrapper.get("[role='alert']").text()).toContain("资源服务不可用");
  });
});
