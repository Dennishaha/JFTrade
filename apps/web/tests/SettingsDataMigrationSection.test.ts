// @vitest-environment jsdom

import { mount } from "@vue/test-utils";
import { afterEach, describe, expect, it, vi } from "vitest";
import { nextTick } from "vue";

import SettingsDataMigrationSection from "../src/components/SettingsDataMigrationSection.vue";
import { createResponse, flushRequests } from "./helpers";

afterEach(() => {
  vi.unstubAllGlobals();
});

describe("SettingsDataMigrationSection", () => {
  it("renders six databases and schedules a confirmed single rebuild", async () => {
    const statuses = buildStatuses();
    const fetchMock = vi.fn(async (_input: string | URL | Request, init?: RequestInit) => {
      if (init?.method === "POST") {
        const body = JSON.parse(String(init.body));
        expect(body).toEqual({
          mode: "single",
          databaseId: "adk",
          confirmation: "REBUILD adk",
        });
        statuses[4].rebuildScheduled = true;
        statuses[4].restartRequired = true;
        return createResponse({ databaseIds: ["adk"], restartRequired: true, scheduled: true });
      }
      return createResponse({ databases: statuses });
    });
    vi.stubGlobal("fetch", fetchMock);

    const wrapper = mount(SettingsDataMigrationSection, {
      attachTo: document.body,
      global: { stubs: expansionPanelStubs },
    });
    await flushRequests();

    expect(wrapper.findAll("[data-testid^='database-card-']")).toHaveLength(6);
    expect(wrapper.text()).not.toContain("/var/jftrade-api/adk.db");
    expect(wrapper.text()).not.toContain("schema metadata is missing");
    expect(wrapper.find("[data-testid='rebuild-adk']").exists()).toBe(false);

    (wrapper.vm as unknown as { expandedDatabaseIDs: string[] }).expandedDatabaseIDs = ["adk"];
    await nextTick();

    expect(wrapper.text()).toContain("/var/jftrade-api/adk.db");
    expect(wrapper.text()).toContain("schema metadata is missing");
    await wrapper.get("[data-testid='rebuild-adk']").trigger("click");
    await wrapper.get("[data-testid='database-rebuild-confirmation']").setValue("REBUILD adk");
    await wrapper.get("[data-testid='confirm-database-rebuild']").trigger("submit");
    await flushRequests();

    expect(fetchMock.mock.calls.some((call) => call[1]?.method === "POST")).toBe(true);
    expect(wrapper.text()).toContain("已安排重建，请重启 JFTrade");
    expect(wrapper.get("[data-testid='rebuild-adk']").attributes("disabled")).toBeDefined();
  });

  it("batches only incompatible databases with the fixed confirmation text", async () => {
    const statuses = buildStatuses();
    const fetchMock = vi.fn(async (_input: string | URL | Request, init?: RequestInit) => {
      if (init?.method === "POST") {
        expect(JSON.parse(String(init.body))).toEqual({
          mode: "incompatible",
          confirmation: "REBUILD INCOMPATIBLE DATABASES",
        });
        return createResponse({ databaseIds: ["adk"], restartRequired: true, scheduled: true });
      }
      return createResponse({ databases: statuses });
    });
    vi.stubGlobal("fetch", fetchMock);
    const wrapper = mount(SettingsDataMigrationSection, {
      global: { stubs: expansionPanelStubs },
    });
    await flushRequests();

    await wrapper.get("[data-testid='rebuild-incompatible']").trigger("click");
    const confirm = wrapper.get("[data-testid='confirm-database-rebuild']");
    expect(confirm.attributes("disabled")).toBeDefined();
    await wrapper.get("[data-testid='database-rebuild-confirmation']").setValue("REBUILD INCOMPATIBLE DATABASES");
    expect(confirm.attributes("disabled")).toBeUndefined();
    await confirm.trigger("submit");
    await flushRequests();
    expect(fetchMock.mock.calls.some((call) => call[1]?.method === "POST")).toBe(true);
  });
});

const expansionPanelStubs = {
  "v-expansion-panels": { template: "<div><slot /></div>" },
  "v-expansion-panel": { props: ["value"], template: "<section><slot /></section>" },
  "v-expansion-panel-title": { template: "<div><slot /></div>" },
  "v-expansion-panel-text": { template: "<div><slot /></div>" },
};

function buildStatuses() {
  const ids = ["backtest", "backtest-runs", "strategy", "execution-orders", "adk", "adk-session"];
  return ids.map((id, index) => ({
    id,
    name: id,
    path: `/var/jftrade-api/${id}.db`,
    description: `${id} data`,
    features: [`feature-${id}`],
    status: index === 4 ? "incompatible" : "ready",
    currentVersion: index === 4 ? null : 1,
    expectedVersion: 1,
    error: index === 4 ? "schema metadata is missing" : undefined,
    rebuildScheduled: false,
    restartRequired: false,
    confirmationText: `REBUILD ${id}`,
  }));
}
