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
  it("renders seven databases and schedules a confirmed single rebuild", async () => {
    const statuses = buildStatuses();
    const fetchMock = buildDataManagementFetch(statuses, async (_input, init) => {
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
    });
    vi.stubGlobal("fetch", fetchMock);

    const wrapper = mount(SettingsDataMigrationSection, {
      attachTo: document.body,
      global: { stubs: expansionPanelStubs },
    });
    await flushRequests();
    await flushRequests();

    expect(wrapper.get("[data-testid='cleanup-tab-type']").attributes("aria-selected")).toBe("true");
    expect(wrapper.find("[data-testid='cleanup-by-type']").exists()).toBe(true);
    expect(wrapper.findAll("[data-testid^='database-card-']")).toHaveLength(0);

    await wrapper.get("[data-testid='cleanup-tab-database']").trigger("click");
    await nextTick();

    expect(wrapper.findAll("[data-testid^='database-card-']")).toHaveLength(7);
    expect(wrapper.get("[data-testid='cleanup-tab-database']").attributes("aria-selected")).toBe("true");
    expect(wrapper.find("[data-testid='compact-backtest']").exists()).toBe(false);
    expect(wrapper.text()).not.toContain("/var/jftrade-api/adk.db");
    expect(wrapper.text()).not.toContain("schema metadata is missing");
    expect(wrapper.find("[data-testid='rebuild-adk']").exists()).toBe(false);

    (wrapper.vm as unknown as { expandedDatabaseIDs: string[] }).expandedDatabaseIDs = ["adk"];
    await nextTick();

    expect(wrapper.text()).toContain("/var/jftrade-api/adk.db");
    expect(wrapper.text()).toContain("schema metadata is missing");
    expect(wrapper.find("[data-testid='compact-backtest']").exists()).toBe(false);
    await wrapper.get("[data-testid='rebuild-adk']").trigger("click");
    await wrapper.get("[data-testid='database-rebuild-confirmation']").setValue("REBUILD adk");
    await wrapper.get("[data-testid='confirm-database-rebuild']").trigger("submit");
    await flushRequests();

    expect(fetchMock.mock.calls.some((call) => call[1]?.method === "POST")).toBe(true);
    expect(wrapper.text()).toContain("已安排重建，请重启 JFTrade");
    expect(wrapper.get("[data-testid='rebuild-adk']").attributes("disabled")).toBeDefined();
  });

  it("renders summary first, then overlays database stats as each database loads", async () => {
    const statuses = buildStatuses();
    statuses[2].cleanable = [{ kind: "soft-deleted", label: "已删除策略", count: 1, estimatedBytes: 2048 }];
    let resolveStrategy: (() => void) | null = null;
    const fetchMock = vi.fn(async (input: string | URL | Request) => {
      const url = new URL(String(input), "http://localhost");
      const databaseId = url.searchParams.get("databaseId");
      if (url.searchParams.get("summaryOnly") === "true") {
        return createResponse({ databases: buildSummaryStatuses(statuses) });
      }
      if (databaseId === "strategy") {
        await new Promise<void>((resolve) => { resolveStrategy = resolve; });
      }
      if (databaseId === "adk") {
        await new Promise<void>(() => {});
      }
      if (databaseId != null) {
        const database = statuses.find((item) => item.id === databaseId);
        return createResponse({ databases: database == null ? [] : [database], totals: buildTotals(database == null ? [] : [database]) });
      }
      return createResponse({ databases: statuses, totals: buildTotals(statuses) });
    });
    vi.stubGlobal("fetch", fetchMock);

    const wrapper = mount(SettingsDataMigrationSection, { global: { stubs: expansionPanelStubs } });
    await flushRequests();

    expect(String(fetchMock.mock.calls[0][0])).toContain("summaryOnly=true");
    expect(wrapper.find("[data-testid='cleanup-by-type']").exists()).toBe(true);
    expect(wrapper.get("[data-testid='preview-soft-deleted-strategy']").attributes("disabled")).toBeDefined();
    expect(wrapper.text()).toContain("已加载 0 / 7 个数据库");
    expect(wrapper.find(".database-spinner").exists()).toBe(true);
    expect(wrapper.find(".database-progress-bar").exists()).toBe(true);

    if (resolveStrategy != null) resolveStrategy();
    await flushRequests();

    expect(wrapper.text()).toContain("已加载 1 / 7 个数据库");
    expect(wrapper.text()).toContain("1 项 · 内容约 2.0 KiB");
    expect(wrapper.get("[data-testid='preview-soft-deleted-strategy']").attributes("disabled")).toBeUndefined();
  });

  it("shows per-database maintenance only after switching to database tab", async () => {
    const statuses = buildStatuses();
    statuses[2].cleanable = [{ kind: "soft-deleted", label: "已删除策略", count: 1, estimatedBytes: 2048 }];
    vi.stubGlobal("fetch", buildDataManagementFetch(statuses));
    const wrapper = mount(SettingsDataMigrationSection, {
      global: { stubs: expansionPanelStubs },
    });
    await flushRequests();
    await flushRequests();

    expect(wrapper.find("[data-testid='cleanup-by-type']").exists()).toBe(true);
    expect(wrapper.find("[data-testid='cleanup-by-database']").exists()).toBe(false);
    expect(wrapper.find("[data-testid='compact-strategy']").exists()).toBe(false);

    await wrapper.get("[data-testid='cleanup-tab-database']").trigger("click");
    await nextTick();

    expect(wrapper.find("[data-testid='cleanup-by-type']").exists()).toBe(false);
    expect(wrapper.find("[data-testid='cleanup-by-database']").exists()).toBe(true);
    expect(wrapper.findAll("[data-testid^='database-card-']")).toHaveLength(7);

    (wrapper.vm as unknown as { expandedDatabaseIDs: string[] }).expandedDatabaseIDs = ["strategy"];
    await nextTick();

    expect(wrapper.get("[data-testid='compact-strategy']").exists()).toBe(true);
    expect(wrapper.get("[data-testid='preview-soft-deleted-strategy']").exists()).toBe(true);
    expect(wrapper.text()).toContain("已删除策略");
  });

  it("batches only incompatible databases with the fixed confirmation text", async () => {
    const statuses = buildStatuses();
    const fetchMock = buildDataManagementFetch(statuses, async (_input, init) => {
      if (init?.method === "POST") {
        expect(JSON.parse(String(init.body))).toEqual({
          mode: "incompatible",
          confirmation: "REBUILD INCOMPATIBLE DATABASES",
        });
        return createResponse({ databaseIds: ["adk"], restartRequired: true, scheduled: true });
      }
    });
    vi.stubGlobal("fetch", fetchMock);
    const wrapper = mount(SettingsDataMigrationSection, {
      global: { stubs: expansionPanelStubs },
    });
    await flushRequests();
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

  it("previews and executes an exact soft-deleted cleanup", async () => {
    const statuses = buildStatuses();
    statuses[2].cleanable = [{ kind: "soft-deleted", label: "已删除策略", count: 2, estimatedBytes: 4096 }];
    const fetchMock = buildDataManagementFetch(statuses, async (input, init) => {
      const url = String(input);
      if (url.endsWith("/cleanup/preview")) {
        expect(JSON.parse(String(init?.body))).toEqual({ kind: "soft-deleted", databaseId: "strategy" });
        return createResponse({
          previewId: "preview-1",
          expiresAt: "2026-07-03T12:10:00Z",
          kind: "soft-deleted",
          databaseId: "strategy",
          candidateCount: 2,
          estimatedBytes: 4096,
          items: [{ kind: "策略定义", label: "策略定义", count: 2, estimatedBytes: 4096 }],
          confirmationText: "CLEANUP strategy 2",
          willCompact: true,
        });
      }
      if (url.endsWith("/cleanup/execute")) {
        expect(JSON.parse(String(init?.body))).toEqual({ previewId: "preview-1", confirmation: "CLEANUP strategy 2" });
        statuses[2].cleanable = [];
        return createResponse({ databaseId: "strategy", deletedCount: 2, reclaimedBytes: 4096, compacted: true });
      }
    });
    vi.stubGlobal("fetch", fetchMock);
    const wrapper = mount(SettingsDataMigrationSection, { global: { stubs: expansionPanelStubs } });
    await flushRequests();
    await flushRequests();

    await wrapper.get("[data-testid='preview-soft-deleted-strategy']").trigger("click");
    await flushRequests();
    expect(wrapper.text()).toContain("将永久删除 2 项");
    await wrapper.get("[data-testid='database-cleanup-confirmation']").setValue("CLEANUP strategy 2");
    await wrapper.get("[data-testid='confirm-database-cleanup']").trigger("submit");
    await flushRequests();

    expect(wrapper.text()).toContain("已永久清理 2 项");
    expect(fetchMock.mock.calls.some((call) => String(call[0]).endsWith("/cleanup/execute"))).toBe(true);
  });

  it("creates a consistent backup for the watchlist database", async () => {
    const statuses = buildStatuses();
    const fetchMock = buildDataManagementFetch(statuses, async (input, init) => {
      if (String(input).endsWith("/databases/watchlist/backup") && init?.method === "POST") {
        return createResponse({
          databaseId: "watchlist",
          backupPath: "/var/jftrade-api/backups/watchlist-20260711.db",
          sizeBytes: 4096,
          createdAt: "2026-07-11T15:00:00Z",
        });
      }
    });
    vi.stubGlobal("fetch", fetchMock);
    const wrapper = mount(SettingsDataMigrationSection, {
      global: { stubs: expansionPanelStubs },
    });
    await flushRequests();
    await flushRequests();
    await wrapper.get("[data-testid='cleanup-tab-database']").trigger("click");
    (wrapper.vm as unknown as { expandedDatabaseIDs: string[] }).expandedDatabaseIDs = ["watchlist"];
    await nextTick();

    await wrapper.get("[data-testid='backup-watchlist']").trigger("click");
    await flushRequests();

    expect(fetchMock.mock.calls.some((call) => String(call[0]).endsWith("/databases/watchlist/backup"))).toBe(true);
    expect(wrapper.text()).toContain("已备份 watchlist（4.0 KiB）");
    expect(wrapper.text()).toContain("/var/jftrade-api/backups/watchlist-20260711.db");
  });
});

const expansionPanelStubs = {
  "v-expansion-panels": { template: "<div><slot /></div>" },
  "v-expansion-panel": { props: ["value"], template: "<section><slot /></section>" },
  "v-expansion-panel-title": { template: "<div><slot /></div>" },
  "v-expansion-panel-text": { template: "<div><slot /></div>" },
};

function buildStatuses() {
  const ids = ["backtest", "backtest-runs", "strategy", "execution-orders", "adk", "adk-session", "watchlist"];
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
    storage: {
      mainBytes: (index + 1) * 1024,
      walBytes: 0,
      shmBytes: 0,
      totalBytes: (index + 1) * 1024,
      freePageBytes: 0,
      reclaimableBytes: 0,
    },
    cleanable: [],
  }));
}

function buildSummaryStatuses(statuses: ReturnType<typeof buildStatuses>) {
  return statuses.map((status) => ({
    ...status,
    storage: {
      mainBytes: 0,
      walBytes: 0,
      shmBytes: 0,
      totalBytes: 0,
      freePageBytes: 0,
      reclaimableBytes: 0,
    },
    cleanable: [],
  }));
}

function buildDataManagementFetch(
  statuses: ReturnType<typeof buildStatuses>,
  postHandler?: (input: string | URL | Request, init?: RequestInit) => Promise<Response | undefined>,
) {
  return vi.fn(async (input: string | URL | Request, init?: RequestInit) => {
    if (init?.method === "POST" && postHandler != null) {
      const response = await postHandler(input, init);
      if (response != null) return response;
    }
    const url = new URL(String(input), "http://localhost");
    const databaseId = url.searchParams.get("databaseId");
    if (databaseId != null) {
      const database = statuses.find((status) => status.id === databaseId);
      return createResponse({ databases: database == null ? [] : [database], totals: buildTotals(database == null ? [] : [database]) });
    }
    if (url.searchParams.get("summaryOnly") === "true") {
      return createResponse({ databases: buildSummaryStatuses(statuses) });
    }
    return createResponse({ databases: statuses, totals: buildTotals(statuses) });
  });
}

function buildTotals(statuses = buildStatuses()) {
  return statuses.reduce((totals, status) => {
    totals.mainBytes += status.storage.mainBytes;
    totals.walBytes += status.storage.walBytes;
    totals.shmBytes += status.storage.shmBytes;
    totals.totalBytes += status.storage.totalBytes;
    totals.reclaimableBytes += status.storage.reclaimableBytes;
    return totals;
  }, { mainBytes: 0, walBytes: 0, shmBytes: 0, totalBytes: 0, reclaimableBytes: 0 });
}
