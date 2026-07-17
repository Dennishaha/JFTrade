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
    const confirmMock = vi.fn(() => true);
    vi.stubGlobal("confirm", confirmMock);
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

    expect(confirmMock).toHaveBeenCalledOnce();
    const backupCall = fetchMock.mock.calls.find((call) =>
      String(call[0]).endsWith("/databases/watchlist/backup"),
    );
    expect(backupCall?.[1]).toMatchObject({
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ confirmation: "BACKUP watchlist" }),
    });
    expect(wrapper.text()).toContain("已备份 watchlist（4.0 KiB）");
    expect(wrapper.text()).toContain("/var/jftrade-api/backups/watchlist-20260711.db");
  });

  it("keeps summary and per-database load failures visible instead of hiding partial storage state", async () => {
    vi.stubGlobal("fetch", vi.fn(async () => {
      throw new Error("汇总数据库状态不可用");
    }));
    const summaryFailure = mount(SettingsDataMigrationSection, {
      global: { stubs: expansionPanelStubs },
    });
    await flushRequests();
    expect(summaryFailure.text()).toContain("汇总数据库状态不可用");

    const statuses = buildStatuses();
    vi.stubGlobal("fetch", vi.fn(async (input: string | URL | Request) => {
      const url = new URL(String(input), "http://localhost");
      if (url.searchParams.get("summaryOnly") === "true") {
        return createResponse({ databases: buildSummaryStatuses(statuses) });
      }
      if (url.searchParams.get("databaseId") === "strategy") {
        throw new Error("策略库统计超时");
      }
      const databaseId = url.searchParams.get("databaseId");
      const database = statuses.find((item) => item.id === databaseId);
      return createResponse({ databases: database == null ? [] : [database] });
    }));
    const partialFailure = mount(SettingsDataMigrationSection, {
      global: { stubs: expansionPanelStubs },
    });
    await flushRequests();
    await flushRequests();
    expect(partialFailure.text()).toContain("策略库统计超时");
    expect(partialFailure.text()).toContain("adk：已删除项目");
  });

  it("previews bounded history cleanup and handles compact success, refusal, and failure paths", async () => {
    const statuses = buildStatuses();
    let compactAttempts = 0;
    const fetchMock = buildDataManagementFetch(statuses, async (input, init) => {
      const url = String(input);
      if (url.endsWith("/cleanup/preview")) {
        expect(JSON.parse(String(init?.body))).toEqual({
          kind: "backtest-history",
          databaseId: "backtest-runs",
          olderThanDays: 90,
          keepLatest: 50,
        });
        return createResponse({
          previewId: "empty-history",
          expiresAt: "2026-07-16T12:00:00Z",
          kind: "backtest-history",
          databaseId: "backtest-runs",
          candidateCount: 0,
          estimatedBytes: 0,
          items: [],
          confirmationText: "CLEANUP backtest-runs 0",
          willCompact: false,
        });
      }
      if (url.endsWith("/databases/strategy/compact")) {
        compactAttempts += 1;
        if (compactAttempts === 1) {
          return createResponse({ databaseId: "strategy", reclaimedBytes: 2048, compacted: true });
        }
        throw new Error("策略库仍有写入任务");
      }
    });
    vi.stubGlobal("fetch", fetchMock);
    const confirmMock = vi.fn(() => false);
    vi.stubGlobal("confirm", confirmMock);
    const wrapper = mount(SettingsDataMigrationSection, {
      global: { stubs: expansionPanelStubs },
    });
    await flushRequests();
    await flushRequests();

    await wrapper.get("[data-testid='cleanup-older-than-days']").setValue("90");
    await wrapper.get("[data-testid='cleanup-keep-latest']").setValue("50");
    await wrapper.get("[data-testid='preview-backtest-history']").trigger("click");
    await flushRequests();
    expect(wrapper.text()).toContain("当前规则下没有可清理项目。");

    await wrapper.get("[data-testid='cleanup-tab-database']").trigger("click");
    (wrapper.vm as unknown as { expandedDatabaseIDs: string[] }).expandedDatabaseIDs = ["strategy", "watchlist"];
    await nextTick();
    await wrapper.get("[data-testid='backup-watchlist']").trigger("click");
    expect(confirmMock).toHaveBeenCalledOnce();
    expect(fetchMock.mock.calls.some((call) => String(call[0]).endsWith("/databases/watchlist/backup"))).toBe(false);

    await wrapper.get("[data-testid='compact-strategy']").trigger("click");
    await wrapper.get("[data-testid='database-compact-confirmation']").setValue("COMPACT strategy");
    await wrapper.get("[data-testid='confirm-database-compact']").trigger("submit");
    await flushRequests();
    expect(wrapper.text()).toContain("数据库整理完成，释放 2.0 KiB。");

    await wrapper.get("[data-testid='compact-strategy']").trigger("click");
    await wrapper.get("[data-testid='database-compact-confirmation']").setValue("COMPACT strategy");
    await wrapper.get("[data-testid='confirm-database-compact']").trigger("submit");
    await flushRequests();
    expect(wrapper.text()).toContain("策略库仍有写入任务");
  });

  it("keeps the newest refresh authoritative when an older database detail returns late", async () => {
    const staleStatuses = buildStatuses();
    staleStatuses[2].description = "stale strategy detail";
    const freshStatuses = buildStatuses();
    freshStatuses[2].description = "fresh strategy detail";
    let strategyRequests = 0;
    let resolveStaleStrategy: ((response: Response) => void) | null = null;
    let summaryRequests = 0;
    const fetchMock = vi.fn(async (input: string | URL | Request) => {
      const url = new URL(String(input), "http://localhost");
      if (url.searchParams.get("summaryOnly") === "true") {
        summaryRequests += 1;
        return createResponse({
          databases: buildSummaryStatuses(summaryRequests === 1 ? staleStatuses : freshStatuses),
        });
      }
      const databaseId = url.searchParams.get("databaseId");
      if (databaseId === "strategy") {
        strategyRequests += 1;
        if (strategyRequests === 1) {
          return new Promise<Response>((resolve) => { resolveStaleStrategy = resolve; });
        }
      }
      const statuses = summaryRequests === 1 ? staleStatuses : freshStatuses;
      const database = statuses.find((item) => item.id === databaseId);
      return createResponse({ databases: database == null ? [] : [database] });
    });
    vi.stubGlobal("fetch", fetchMock);
    const wrapper = mount(SettingsDataMigrationSection, {
      global: { stubs: expansionPanelStubs },
    });
    await flushRequests();
    expect(strategyRequests).toBe(1);

    await (wrapper.vm as unknown as { loadStatuses: () => Promise<void> }).loadStatuses();
    await flushRequests();

    resolveStaleStrategy?.(createResponse({ databases: [staleStatuses[2]] }));
    await flushRequests();
    await wrapper.get("[data-testid='cleanup-tab-database']").trigger("click");
    (wrapper.vm as unknown as { expandedDatabaseIDs: string[] }).expandedDatabaseIDs = ["strategy"];
    await nextTick();

    expect(wrapper.text()).toContain("fresh strategy detail");
    expect(wrapper.text()).not.toContain("stale strategy detail");
  });

  it("keeps destructive maintenance errors actionable when the service rejects non-Error values", async () => {
    const statuses = buildStatuses();
    statuses[2].cleanable = [{ kind: "soft-deleted", label: "已删除策略", count: 2, estimatedBytes: 2048 }];
    let previewAttempts = 0;
    const fetchMock = buildDataManagementFetch(statuses, async (input) => {
      const path = String(input);
      if (path.endsWith("/databases/rebuild")) throw "rebuild unavailable";
      if (path.endsWith("/cleanup/preview")) {
        previewAttempts += 1;
        if (previewAttempts === 1) throw "preview unavailable";
        return createResponse({
          previewId: "preview-failure",
          expiresAt: "2026-07-16T12:00:00Z",
          kind: "soft-deleted",
          databaseId: "strategy",
          candidateCount: 2,
          estimatedBytes: 2048,
          items: [],
          confirmationText: "CLEANUP strategy 2",
          willCompact: true,
        });
      }
      if (path.endsWith("/cleanup/execute")) throw "cleanup unavailable";
      if (path.endsWith("/databases/strategy/compact")) throw "compact unavailable";
      if (path.endsWith("/databases/watchlist/backup")) throw "backup unavailable";
      return undefined;
    });
    vi.stubGlobal("fetch", fetchMock);
    vi.stubGlobal("confirm", vi.fn(() => true));
    const wrapper = mount(SettingsDataMigrationSection, {
      global: { stubs: expansionPanelStubs },
    });
    await flushRequests();
    await flushRequests();

    await wrapper.get("[data-testid='rebuild-incompatible']").trigger("click");
    await wrapper.get("[data-testid='database-rebuild-confirmation']").setValue("REBUILD INCOMPATIBLE DATABASES");
    await wrapper.get("[data-testid='confirm-database-rebuild']").trigger("submit");
    await flushRequests();
    expect(wrapper.text()).toContain("安排数据库重建失败");

    await wrapper.get("[data-testid='preview-soft-deleted-strategy']").trigger("click");
    await flushRequests();
    expect(wrapper.text()).toContain("生成清理预览失败");

    await wrapper.get("[data-testid='preview-soft-deleted-strategy']").trigger("click");
    await flushRequests();
    await wrapper.get("[data-testid='database-cleanup-confirmation']").setValue("CLEANUP strategy 2");
    await wrapper.get("[data-testid='confirm-database-cleanup']").trigger("submit");
    await flushRequests();
    expect(wrapper.text()).toContain("清理数据库失败");

    await wrapper.get("[data-testid='cleanup-tab-database']").trigger("click");
    (wrapper.vm as unknown as { expandedDatabaseIDs: string[] }).expandedDatabaseIDs = ["strategy", "watchlist"];
    await nextTick();
    await wrapper.get("[data-testid='compact-strategy']").trigger("click");
    await wrapper.get("[data-testid='database-compact-confirmation']").setValue("COMPACT strategy");
    await wrapper.get("[data-testid='confirm-database-compact']").trigger("submit");
    await flushRequests();
    expect(wrapper.text()).toContain("整理数据库失败");

    await wrapper.get("[data-testid='backup-watchlist']").trigger("click");
    await flushRequests();
    expect(wrapper.text()).toContain("备份数据库失败");
  });

  it("keeps unconfirmed maintenance commands inert and incorporates a newly reported database", async () => {
    const statuses = buildStatuses();
    const fetchMock = buildDataManagementFetch(statuses);
    vi.stubGlobal("fetch", fetchMock);
    const wrapper = mount(SettingsDataMigrationSection, {
      global: { stubs: expansionPanelStubs },
    });
    await flushRequests();
    await flushRequests();

    const state = (wrapper.vm as unknown as {
      $: {
        setupState: {
          databases: { value: ReturnType<typeof buildStatuses> };
          executeCleanup: () => Promise<void>;
          executeCompact: () => Promise<void>;
          formatBytes: (value: number) => string;
          previewCleanableItem: (
            database: ReturnType<typeof buildStatuses>[number],
            item: { kind: string; label: string; count: number; estimatedBytes: number },
          ) => void;
          replaceDatabase: (database: ReturnType<typeof buildStatuses>[number]) => void;
          submitRebuild: () => Promise<void>;
        };
      };
    }).$.setupState;
    const requestCount = fetchMock.mock.calls.length;

    state.previewCleanableItem(statuses[2], {
      kind: "temporary-index",
      label: "临时索引",
      count: 4,
      estimatedBytes: 1024,
    });
    await state.submitRebuild();
    await state.executeCleanup();
    await state.executeCompact();

    expect(fetchMock).toHaveBeenCalledTimes(requestCount);
    expect(state.formatBytes(Number.NaN)).toBe("0 B");
    expect(state.formatBytes(10 * 1024)).toBe("10 KiB");

    state.replaceDatabase({
      ...statuses[0],
      id: "imported-history",
      name: "导入历史",
      description: "由服务端新发现的存档库",
      confirmationText: "REBUILD imported-history",
    });
    await wrapper.get("[data-testid='cleanup-tab-database']").trigger("click");
    await nextTick();

    expect(wrapper.get("[data-testid='database-card-imported-history']").text()).toContain("导入历史");
    expect((state.databases as unknown as { value?: unknown }).value ?? state.databases).toHaveLength(8);
  });

  it("keeps empty load progress honest, uses native dialog controls, and does not overlap backups", async () => {
    const statuses = buildStatuses();
    const fetchMock = buildDataManagementFetch(statuses);
    vi.stubGlobal("fetch", fetchMock);
    const wrapper = mount(SettingsDataMigrationSection, {
      attachTo: document.body,
      global: { stubs: expansionPanelStubs },
    });
    await flushRequests();
    await flushRequests();

    const state = (wrapper.vm as unknown as { $: { setupState: Record<string, unknown> } }).$.setupState;
    const read = <T>(value: unknown): T =>
      value !== null && typeof value === "object" && "value" in value
        ? (value as { value: T }).value
        : value as T;
    const write = (key: string, value: unknown) => {
      const current = state[key];
      if (current !== null && typeof current === "object" && "value" in current) {
        (current as { value: unknown }).value = value;
        return;
      }
      state[key] = value;
    };

    write("databases", []);
    expect(read<number>(state.loadProgressPercent)).toBe(0);
    write("databases", statuses);

    const dialog = document.getElementById("database-rebuild-dialog") as HTMLDialogElement;
    const showModal = vi.fn();
    const close = vi.fn();
    Object.assign(dialog, { showModal, close });
    (state.showDialog as (id: string) => void)("database-rebuild-dialog");
    (state.closeDialog as (id: string) => void)("database-rebuild-dialog");
    expect(showModal).toHaveBeenCalledOnce();
    expect(close).toHaveBeenCalledOnce();

    const requestCount = fetchMock.mock.calls.length;
    write("submitting", true);
    await (state.backupDatabase as (database: ReturnType<typeof buildStatuses>[number]) => Promise<void>)(statuses[0]!);
    expect(fetchMock).toHaveBeenCalledTimes(requestCount);
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
