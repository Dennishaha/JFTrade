<script setup lang="ts">
import { computed, onMounted, ref } from "vue";

import { fetchEnvelope, fetchEnvelopeWithInit } from "../composables/apiClient";

type StorageStats = {
  mainBytes: number;
  walBytes: number;
  shmBytes: number;
  totalBytes: number;
  freePageBytes: number;
  reclaimableBytes: number;
  error?: string;
};

type CleanableItem = {
  kind: string;
  label: string;
  count: number;
  estimatedBytes: number;
};

type DatabaseStatus = {
  id: string;
  name: string;
  path: string;
  description: string;
  features: string[];
  status: "ready" | "missing" | "incompatible" | "unavailable";
  currentVersion: number | null;
  expectedVersion: number;
  error?: string;
  rebuildScheduled: boolean;
  restartRequired: boolean;
  confirmationText: string;
  storage?: StorageStats;
  cleanable?: CleanableItem[];
};

type OverviewTotals = {
  mainBytes: number;
  walBytes: number;
  shmBytes: number;
  totalBytes: number;
  reclaimableBytes: number;
};

type StatusResponse = {
  databases: DatabaseStatus[];
  totals?: OverviewTotals;
  checkedAt?: string;
};

type RebuildResult = { databaseIds: string[]; restartRequired: boolean; scheduled: boolean };
type CleanupPreview = {
  previewId: string;
  expiresAt: string;
  kind: string;
  databaseId: string;
  candidateCount: number;
  estimatedBytes: number;
  items: CleanableItem[];
  confirmationText: string;
  willCompact: boolean;
};
type CleanupResult = {
  databaseId: string;
  deletedCount: number;
  reclaimedBytes: number;
  compacted: boolean;
  warning?: string;
};
type CompactResult = { databaseId: string; reclaimedBytes: number; compacted: boolean };
type BackupResult = { databaseId: string; backupPath: string; sizeBytes: number; createdAt: string };

const emptyStorage: StorageStats = {
  mainBytes: 0,
  walBytes: 0,
  shmBytes: 0,
  totalBytes: 0,
  freePageBytes: 0,
  reclaimableBytes: 0,
};

const databaseLoadOrder = ["strategy", "adk", "watchlist", "backtest-runs", "backtest", "execution-orders", "adk-session"];
const databases = ref<DatabaseStatus[]>([]);
const loadedDatabaseIds = ref<string[]>([]);
const loadingDatabaseIds = ref<string[]>([]);
const databaseLoadErrors = ref<Record<string, string>>({});
const loading = ref(false);
const summaryLoading = ref(false);
const submitting = ref(false);
const errorMessage = ref("");
const noticeMessage = ref("");
const confirmation = ref("");
const selectedDatabase = ref<DatabaseStatus | null>(null);
const compactDatabase = ref<DatabaseStatus | null>(null);
const cleanupPreview = ref<CleanupPreview | null>(null);
const expandedDatabaseIDs = ref<string[]>([]);
const activeCleanupTab = ref<"type" | "database">("type");
const olderThanDays = ref(30);
const keepLatest = ref(20);
const batchConfirmation = "REBUILD INCOMPATIBLE DATABASES";
let loadGeneration = 0;

const totals = computed<OverviewTotals>(() => {
  return databases.value.reduce<OverviewTotals>((value, database) => {
    const storage = storageOf(database);
    value.mainBytes += storage.mainBytes;
    value.walBytes += storage.walBytes;
    value.shmBytes += storage.shmBytes;
    value.totalBytes += storage.totalBytes;
    value.reclaimableBytes += storage.reclaimableBytes;
    return value;
  }, { mainBytes: 0, walBytes: 0, shmBytes: 0, totalBytes: 0, reclaimableBytes: 0 });
});

const loadProgressPercent = computed(() => {
  if (databases.value.length === 0) return 0;
  const inFlightBonus = loadingDatabaseIds.value.length > 0 ? 0.35 : 0;
  return Math.min(100, Math.round(((loadedDatabaseIds.value.length + inFlightBonus) / databases.value.length) * 100));
});

const incompatibleDatabases = computed(() =>
  databases.value.filter((database) =>
    ["missing", "incompatible", "unavailable"].includes(database.status) &&
    !database.rebuildScheduled,
  ),
);

const backtestRunsDatabase = computed(() =>
  databases.value.find((database) => database.id === "backtest-runs") ?? null,
);

const softDeleteDatabases = computed(() =>
  databases.value.filter((database) =>
    ["strategy", "adk"].includes(database.id),
  ),
);

const dialogTitle = computed(() => selectedDatabase.value == null
  ? "重建全部不兼容数据库"
  : `重建 ${selectedDatabase.value.name}`,
);
const requiredConfirmation = computed(() => selectedDatabase.value?.confirmationText ?? batchConfirmation);

onMounted(() => { void loadStatuses(); });

async function loadStatuses(): Promise<void> {
  const generation = loadGeneration + 1;
  loadGeneration = generation;
  loading.value = true;
  summaryLoading.value = true;
  errorMessage.value = "";
  try {
    const response = await fetchEnvelope<StatusResponse>("/api/v1/settings/data-management/databases?summaryOnly=true");
    if (generation !== loadGeneration) return;
    databases.value = response.databases;
    loadedDatabaseIds.value = [];
    loadingDatabaseIds.value = [];
    databaseLoadErrors.value = {};
  } catch (error) {
    if (generation !== loadGeneration) return;
    errorMessage.value = error instanceof Error ? error.message : "读取数据库状态失败";
    loading.value = false;
    summaryLoading.value = false;
    return;
  } finally {
    if (generation === loadGeneration) summaryLoading.value = false;
  }
  for (const databaseId of orderedDatabaseIds()) {
    if (generation !== loadGeneration) return;
    await refreshDatabase(databaseId, generation);
  }
  if (generation === loadGeneration) loading.value = false;
}

async function refreshDatabase(databaseId: string, generation = loadGeneration): Promise<void> {
  markDatabaseLoading(databaseId, true);
  try {
    const response = await fetchEnvelope<StatusResponse>(
      `/api/v1/settings/data-management/databases?databaseId=${encodeURIComponent(databaseId)}`,
    );
    if (generation !== loadGeneration) return;
    const database = response.databases[0];
    if (database != null) replaceDatabase(database);
    markDatabaseLoaded(databaseId);
    clearDatabaseLoadError(databaseId);
  } catch (error) {
    if (generation !== loadGeneration) return;
    setDatabaseLoadError(databaseId, error instanceof Error ? error.message : "统计数据库失败");
  } finally {
    if (generation === loadGeneration) markDatabaseLoading(databaseId, false);
  }
}

function orderedDatabaseIds(ids = databases.value.map((database) => database.id)): string[] {
  const known = databaseLoadOrder.filter((databaseId) => ids.includes(databaseId));
  const rest = ids.filter((databaseId) => !databaseLoadOrder.includes(databaseId));
  return [...known, ...rest];
}

function replaceDatabase(next: DatabaseStatus): void {
  const index = databases.value.findIndex((database) => database.id === next.id);
  if (index === -1) {
    databases.value = [...databases.value, next];
    return;
  }
  const updated = [...databases.value];
  updated[index] = next;
  databases.value = updated;
}

function markDatabaseLoaded(databaseId: string): void {
  if (!loadedDatabaseIds.value.includes(databaseId)) {
    loadedDatabaseIds.value = [...loadedDatabaseIds.value, databaseId];
  }
}

function markDatabaseLoading(databaseId: string, value: boolean): void {
  const current = loadingDatabaseIds.value.filter((id) => id !== databaseId);
  loadingDatabaseIds.value = value ? [...current, databaseId] : current;
}

function setDatabaseLoadError(databaseId: string, message: string): void {
  databaseLoadErrors.value = { ...databaseLoadErrors.value, [databaseId]: message };
}

function clearDatabaseLoadError(databaseId: string): void {
  const next = { ...databaseLoadErrors.value };
  delete next[databaseId];
  databaseLoadErrors.value = next;
}

function isDatabaseLoaded(databaseId: string): boolean {
  return loadedDatabaseIds.value.includes(databaseId);
}

function isDatabaseLoading(databaseId: string): boolean {
  return loadingDatabaseIds.value.includes(databaseId);
}

function loadErrorFor(databaseId: string): string {
  return databaseLoadErrors.value[databaseId] ?? "";
}

function storageOf(database: DatabaseStatus): StorageStats {
  return database.storage ?? emptyStorage;
}

function cleanableCount(database: DatabaseStatus, kind: string): number {
  return (database.cleanable ?? [])
    .filter((item) => item.kind === kind)
    .reduce((total, item) => total + item.count, 0);
}

function cleanableBytes(database: DatabaseStatus, kind: string): number {
  return (database.cleanable ?? [])
    .filter((item) => item.kind === kind)
    .reduce((total, item) => total + item.estimatedBytes, 0);
}

function cleanableItems(database: DatabaseStatus): CleanableItem[] {
  return (database.cleanable ?? []).filter((item) => item.count > 0);
}

function isCleanupKind(kind: string): kind is "soft-deleted" | "backtest-history" {
  return kind === "soft-deleted" || kind === "backtest-history";
}

function previewCleanableItem(database: DatabaseStatus, item: CleanableItem): void {
  if (!isCleanupKind(item.kind)) return;
  void previewCleanup(item.kind, database.id);
}

function formatBytes(value: number): string {
  if (!Number.isFinite(value) || value <= 0) return "0 B";
  const units = ["B", "KiB", "MiB", "GiB", "TiB"];
  const index = Math.min(Math.floor(Math.log(value) / Math.log(1024)), units.length - 1);
  const scaled = value / (1024 ** index);
  return `${scaled >= 10 || index === 0 ? scaled.toFixed(0) : scaled.toFixed(1)} ${units[index]}`;
}

function showDialog(id: string): void {
  const dialog = document.getElementById(id) as HTMLDialogElement | null;
  if (typeof dialog?.showModal === "function") dialog.showModal();
}

function closeDialog(id: string): void {
  const dialog = document.getElementById(id) as HTMLDialogElement | null;
  if (typeof dialog?.close === "function") dialog.close();
}

function openDatabaseRebuild(database: DatabaseStatus): void {
  selectedDatabase.value = database;
  confirmation.value = "";
}

function openBatchRebuild(): void {
  selectedDatabase.value = null;
  confirmation.value = "";
}

function closeRebuildDialog(): void {
  confirmation.value = "";
  selectedDatabase.value = null;
  closeDialog("database-rebuild-dialog");
}

async function submitRebuild(): Promise<void> {
  if (confirmation.value !== requiredConfirmation.value || submitting.value) return;
  submitting.value = true;
  errorMessage.value = "";
  const targetDatabaseId = selectedDatabase.value?.id ?? "";
  try {
    const body = selectedDatabase.value == null
      ? { mode: "incompatible", confirmation: confirmation.value }
      : { mode: "single", databaseId: selectedDatabase.value.id, confirmation: confirmation.value };
    const result = await fetchEnvelopeWithInit<RebuildResult>(
      "/api/v1/settings/data-management/databases/rebuild",
      { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify(body) },
    );
    noticeMessage.value = result.restartRequired
      ? "已安排重建，请重启 JFTrade。重启完成前相关功能仍不可用。"
      : "重建请求已提交。";
    closeRebuildDialog();
    if (targetDatabaseId !== "") {
      await refreshDatabase(targetDatabaseId);
    } else {
      await loadStatuses();
    }
  } catch (error) {
    errorMessage.value = error instanceof Error ? error.message : "安排数据库重建失败";
  } finally {
    submitting.value = false;
  }
}

async function previewCleanup(kind: "soft-deleted" | "backtest-history", databaseId: string): Promise<void> {
  submitting.value = true;
  errorMessage.value = "";
  try {
    const body = kind === "backtest-history"
      ? { kind, databaseId, olderThanDays: olderThanDays.value, keepLatest: keepLatest.value }
      : { kind, databaseId };
    cleanupPreview.value = await fetchEnvelopeWithInit<CleanupPreview>(
      "/api/v1/settings/data-management/cleanup/preview",
      { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify(body) },
    );
    confirmation.value = "";
    showDialog("database-cleanup-dialog");
  } catch (error) {
    errorMessage.value = error instanceof Error ? error.message : "生成清理预览失败";
  } finally {
    submitting.value = false;
  }
}

function closeCleanupDialog(): void {
  cleanupPreview.value = null;
  confirmation.value = "";
  closeDialog("database-cleanup-dialog");
}

async function executeCleanup(): Promise<void> {
  const preview = cleanupPreview.value;
  if (preview == null || preview.candidateCount === 0 || confirmation.value !== preview.confirmationText || submitting.value) return;
  submitting.value = true;
  errorMessage.value = "";
  try {
    const result = await fetchEnvelopeWithInit<CleanupResult>(
      "/api/v1/settings/data-management/cleanup/execute",
      {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ previewId: preview.previewId, confirmation: confirmation.value }),
      },
    );
    noticeMessage.value = result.warning || `已永久清理 ${result.deletedCount} 项，释放 ${formatBytes(result.reclaimedBytes)}。`;
    closeCleanupDialog();
    await refreshDatabase(preview.databaseId);
  } catch (error) {
    errorMessage.value = error instanceof Error ? error.message : "清理数据库失败";
  } finally {
    submitting.value = false;
  }
}

function openCompact(database: DatabaseStatus): void {
  compactDatabase.value = database;
  confirmation.value = "";
  showDialog("database-compact-dialog");
}

function closeCompactDialog(): void {
  compactDatabase.value = null;
  confirmation.value = "";
  closeDialog("database-compact-dialog");
}

async function executeCompact(): Promise<void> {
  const database = compactDatabase.value;
  if (database == null || confirmation.value !== `COMPACT ${database.id}` || submitting.value) return;
  submitting.value = true;
  errorMessage.value = "";
  try {
    const result = await fetchEnvelopeWithInit<CompactResult>(
      `/api/v1/settings/data-management/databases/${encodeURIComponent(database.id)}/compact`,
      {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ confirmation: confirmation.value }),
      },
    );
    noticeMessage.value = `数据库整理完成，释放 ${formatBytes(result.reclaimedBytes)}。`;
    closeCompactDialog();
    await refreshDatabase(database.id);
  } catch (error) {
    errorMessage.value = error instanceof Error ? error.message : "整理数据库失败";
  } finally {
    submitting.value = false;
  }
}

async function backupDatabase(database: DatabaseStatus): Promise<void> {
  if (submitting.value) return;
  const confirmation = `BACKUP ${database.id}`;
  if (!window.confirm(`确认创建“${database.name}”的本地数据库备份？`)) return;
  submitting.value = true;
  errorMessage.value = "";
  try {
    const result = await fetchEnvelopeWithInit<BackupResult>(
      `/api/v1/settings/data-management/databases/${encodeURIComponent(database.id)}/backup`,
      {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ confirmation }),
      },
    );
    noticeMessage.value = `已备份 ${database.name}（${formatBytes(result.sizeBytes)}）：${result.backupPath}`;
  } catch (error) {
    errorMessage.value = error instanceof Error ? error.message : "备份数据库失败";
  } finally {
    submitting.value = false;
  }
}

function statusLabel(status: DatabaseStatus["status"]): string {
  return { ready: "可用", missing: "缺失", incompatible: "不兼容", unavailable: "不可用" }[status];
}

function statusClass(status: DatabaseStatus["status"]): string {
  return status === "ready"
    ? "border-emerald-200 bg-emerald-50 text-emerald-700"
    : "border-rose-200 bg-rose-50 text-rose-700";
}
</script>

<template>
  <section class="grid gap-5">
    <header class="rounded-lg border border-slate-200 bg-white px-5 py-5">
      <div class="flex flex-wrap items-start justify-between gap-4">
        <div>
          <h2 class="text-base font-semibold text-slate-900">数据管理</h2>
          <p class="mt-1 max-w-3xl text-xs leading-5 text-slate-500">
            查看本地数据库实际占用，创建一致性备份，清理不再需要的数据，并在必要时重建不兼容数据库。
          </p>
        </div>
        <button type="button" class="rounded-md border border-slate-200 px-3 py-2 text-xs font-medium text-slate-700 disabled:opacity-50" :disabled="loading" @click="loadStatuses">
          {{ summaryLoading ? "加载中…" : loading ? "刷新中…" : "刷新统计" }}
        </button>
      </div>
      <div v-if="loading && databases.length > 0" class="mt-4 grid gap-2" data-testid="database-loading-progress">
        <div class="flex items-center gap-2 text-xs text-slate-500">
          <span class="database-spinner" aria-hidden="true"></span>
          <span>已加载 {{ loadedDatabaseIds.length }} / {{ databases.length }} 个数据库，统计结果会实时叠加。</span>
        </div>
        <div class="h-1.5 overflow-hidden rounded-full bg-slate-100">
          <div class="database-progress-bar h-full rounded-full bg-slate-700 transition-[width] duration-300" :style="{ width: `${loadProgressPercent}%` }"></div>
        </div>
      </div>
      <p v-if="noticeMessage" class="mt-4 rounded-md border border-amber-200 bg-amber-50 px-3 py-2 text-sm font-medium text-amber-800">{{ noticeMessage }}</p>
      <p v-if="errorMessage" class="mt-4 text-sm font-medium text-rose-600">{{ errorMessage }}</p>
    </header>

    <section aria-labelledby="storage-overview-title" class="grid gap-3">
      <h3 id="storage-overview-title" class="text-sm font-semibold text-slate-900">存储概览</h3>
      <div class="grid gap-3 sm:grid-cols-2 xl:grid-cols-4">
        <div v-for="item in [
          ['总占用', totals.totalBytes],
          ['数据库主文件', totals.mainBytes],
          ['WAL / SHM', totals.walBytes + totals.shmBytes],
          ['可回收估算', totals.reclaimableBytes],
        ]" :key="String(item[0])" class="rounded-lg border border-slate-200 bg-white px-4 py-3">
          <p class="text-xs text-slate-500">{{ item[0] }}</p>
          <p class="mt-1 text-lg font-semibold text-slate-900">{{ formatBytes(Number(item[1])) }}</p>
        </div>
      </div>
    </section>

    <div v-if="loading && databases.length === 0" class="rounded-lg border border-slate-200 bg-white px-5 py-10 text-center text-sm text-slate-500">
      <span class="mr-2 inline-flex align-[-2px]"><span class="database-spinner" aria-hidden="true"></span></span>
      正在读取数据库列表…
    </div>

    <section v-else class="grid gap-3" aria-labelledby="optimization-title">
      <div class="flex flex-wrap items-start justify-between gap-3">
        <div>
          <h3 id="optimization-title" class="text-sm font-semibold text-slate-900">空间优化</h3>
          <p class="mt-1 text-xs text-slate-500">清理前会生成精确预览；执行后自动整理目标数据库。</p>
        </div>
        <div class="inline-flex rounded-lg border border-slate-200 bg-white p-1" role="tablist" aria-label="空间优化视角">
          <button
            type="button"
            data-testid="cleanup-tab-type"
            role="tab"
            :aria-selected="activeCleanupTab === 'type'"
            class="rounded-md px-3 py-1.5 text-xs font-semibold"
            :class="activeCleanupTab === 'type' ? 'bg-slate-800 text-white' : 'text-slate-600 hover:bg-slate-50'"
            @click="activeCleanupTab = 'type'"
          >
            按类型整理
          </button>
          <button
            type="button"
            data-testid="cleanup-tab-database"
            role="tab"
            :aria-selected="activeCleanupTab === 'database'"
            class="rounded-md px-3 py-1.5 text-xs font-semibold"
            :class="activeCleanupTab === 'database' ? 'bg-slate-800 text-white' : 'text-slate-600 hover:bg-slate-50'"
            @click="activeCleanupTab = 'database'"
          >
            按库整理
          </button>
        </div>
      </div>

      <div v-if="activeCleanupTab === 'type'" data-testid="cleanup-by-type" class="grid gap-4">
        <section class="grid gap-3" aria-labelledby="soft-deleted-cleanup-title">
          <h4 id="soft-deleted-cleanup-title" class="text-xs font-semibold uppercase tracking-wide text-slate-500">已删除项目</h4>
          <div class="grid gap-3 lg:grid-cols-2">
            <article v-for="database in softDeleteDatabases" :key="`cleanup-${database.id}`" class="rounded-lg border border-slate-200 bg-white p-4">
              <h5 class="text-sm font-semibold text-slate-900">{{ database.name }}：已删除项目</h5>
              <p v-if="isDatabaseLoading(database.id) && !isDatabaseLoaded(database.id)" class="mt-2 inline-flex items-center gap-2 text-xs text-slate-500"><span class="database-spinner database-spinner-sm" aria-hidden="true"></span>正在统计该库…</p>
              <p v-else-if="loadErrorFor(database.id)" class="mt-2 text-xs text-rose-600">{{ loadErrorFor(database.id) }}</p>
              <p v-else class="mt-2 text-xs text-slate-500">{{ cleanableCount(database, "soft-deleted") }} 项 · 内容约 {{ formatBytes(cleanableBytes(database, "soft-deleted")) }}</p>
              <button type="button" class="mt-4 rounded-md bg-slate-800 px-3 py-2 text-xs font-semibold text-white disabled:opacity-50" :data-testid="`preview-soft-deleted-${database.id}`" :disabled="database.status !== 'ready' || database.rebuildScheduled || !isDatabaseLoaded(database.id) || cleanableCount(database, 'soft-deleted') === 0 || submitting" @click="previewCleanup('soft-deleted', database.id)">预览永久清理</button>
            </article>
          </div>
        </section>

        <section class="rounded-lg border border-slate-200 bg-white p-4" aria-labelledby="backtest-history-cleanup-title">
          <h4 id="backtest-history-cleanup-title" class="text-sm font-semibold text-slate-900">旧回测结果</h4>
          <p v-if="backtestRunsDatabase != null && isDatabaseLoading(backtestRunsDatabase.id) && !isDatabaseLoaded(backtestRunsDatabase.id)" class="mt-2 inline-flex items-center gap-2 text-xs text-slate-500"><span class="database-spinner database-spinner-sm" aria-hidden="true"></span>正在统计回测运行历史…</p>
          <p v-else-if="backtestRunsDatabase != null && loadErrorFor(backtestRunsDatabase.id)" class="mt-2 text-xs text-rose-600">{{ loadErrorFor(backtestRunsDatabase.id) }}</p>
          <p v-else class="mt-2 text-xs text-slate-500">只处理 completed、failed、cancelled；queued 和 running 不会进入候选集。</p>
          <div class="mt-3 grid gap-3 sm:grid-cols-2">
            <label class="grid gap-1 text-xs text-slate-600">早于天数<input v-model.number="olderThanDays" data-testid="cleanup-older-than-days" type="number" min="1" max="3650" class="rounded border border-slate-300 px-2 py-1.5" /></label>
            <label class="grid gap-1 text-xs text-slate-600">至少保留<input v-model.number="keepLatest" data-testid="cleanup-keep-latest" type="number" min="1" max="10000" class="rounded border border-slate-300 px-2 py-1.5" /></label>
          </div>
          <button type="button" data-testid="preview-backtest-history" class="mt-4 rounded-md bg-slate-800 px-3 py-2 text-xs font-semibold text-white disabled:opacity-50" :disabled="submitting || backtestRunsDatabase == null || !isDatabaseLoaded('backtest-runs') || olderThanDays < 1 || olderThanDays > 3650 || keepLatest < 1 || keepLatest > 10000" @click="previewCleanup('backtest-history', 'backtest-runs')">预览历史清理</button>
        </section>
      </div>

      <v-expansion-panels v-else v-model="expandedDatabaseIDs" multiple data-testid="cleanup-by-database" class="database-expansion-panels grid gap-3">
        <v-expansion-panel v-for="database in databases" :key="database.id" :value="database.id" :data-testid="`database-card-${database.id}`" class="border bg-white" :class="database.status === 'ready' ? 'border-slate-200' : 'border-rose-300'">
          <v-expansion-panel-title class="px-5 py-4">
            <div class="flex w-full flex-wrap items-center gap-2 pr-3">
              <h3 class="text-sm font-semibold text-slate-900">{{ database.name }}</h3>
              <span class="rounded border px-2 py-0.5 text-xs font-medium" :class="statusClass(database.status)">{{ statusLabel(database.status) }}</span>
              <span v-if="database.rebuildScheduled" class="rounded border border-amber-200 bg-amber-50 px-2 py-0.5 text-xs font-medium text-amber-700">等待重启</span>
              <span v-if="isDatabaseLoading(database.id)" class="inline-flex items-center gap-1.5 rounded border border-slate-200 bg-slate-50 px-2 py-0.5 text-xs font-medium text-slate-500"><span class="database-spinner database-spinner-xs" aria-hidden="true"></span>统计中…</span>
              <span class="ml-auto text-xs font-semibold text-slate-700">{{ formatBytes(storageOf(database).totalBytes) }}</span>
            </div>
          </v-expansion-panel-title>
          <v-expansion-panel-text v-if="expandedDatabaseIDs.includes(database.id)">
            <div class="px-1 pb-2">
              <div class="flex flex-wrap items-start justify-between gap-3">
                <p class="text-xs text-slate-500">{{ database.description }}</p>
                <div class="flex gap-2">
                  <button type="button" class="rounded-md border border-slate-200 px-3 py-1.5 text-xs font-medium text-slate-700 disabled:opacity-50" :disabled="database.status === 'missing' || database.status === 'unavailable' || !isDatabaseLoaded(database.id) || submitting" :data-testid="`backup-${database.id}`" @click="backupDatabase(database)">创建备份</button>
                  <button type="button" class="rounded-md border border-slate-200 px-3 py-1.5 text-xs font-medium text-slate-700 disabled:opacity-50" :disabled="database.status !== 'ready' || database.rebuildScheduled || !isDatabaseLoaded(database.id) || submitting" :data-testid="`compact-${database.id}`" @click="openCompact(database)">整理空间</button>
                  <button type="button" class="rounded-md border border-rose-200 px-3 py-1.5 text-xs font-medium text-rose-700 disabled:opacity-50" :data-testid="`rebuild-${database.id}`" :disabled="database.rebuildScheduled" @click="openDatabaseRebuild(database); showDialog('database-rebuild-dialog')">{{ database.status === "ready" ? "危险操作：重建" : "重建数据库" }}</button>
                </div>
              </div>
              <dl class="mt-4 grid gap-3 rounded-md bg-slate-50 px-4 py-3 text-xs sm:grid-cols-2 xl:grid-cols-3">
                <div><dt class="text-slate-400">文件路径</dt><dd class="mt-1 break-all font-mono text-slate-700">{{ database.path }}</dd></div>
                <div><dt class="text-slate-400">Schema 版本</dt><dd class="mt-1 text-slate-700">{{ database.currentVersion ?? "未知" }} / {{ database.expectedVersion }}</dd></div>
                <div><dt class="text-slate-400">主文件</dt><dd class="mt-1 text-slate-700">{{ formatBytes(storageOf(database).mainBytes) }}</dd></div>
                <div><dt class="text-slate-400">WAL / SHM</dt><dd class="mt-1 text-slate-700">{{ formatBytes(storageOf(database).walBytes) }} / {{ formatBytes(storageOf(database).shmBytes) }}</dd></div>
                <div><dt class="text-slate-400">空闲页</dt><dd class="mt-1 text-slate-700">{{ formatBytes(storageOf(database).freePageBytes) }}</dd></div>
                <div><dt class="text-slate-400">可回收估算</dt><dd class="mt-1 text-slate-700">{{ formatBytes(storageOf(database).reclaimableBytes) }}</dd></div>
                <div v-if="database.error || storageOf(database).error || loadErrorFor(database.id)"><dt class="text-rose-500">检测结果</dt><dd class="mt-1 text-rose-700">{{ database.error || storageOf(database).error || loadErrorFor(database.id) }}</dd></div>
              </dl>
              <div class="mt-4 rounded-md border border-slate-200 px-4 py-3">
                <h4 class="text-xs font-semibold text-slate-700">可清理项目</h4>
                <p v-if="isDatabaseLoading(database.id) && !isDatabaseLoaded(database.id)" class="mt-2 inline-flex items-center gap-2 text-xs text-slate-500"><span class="database-spinner database-spinner-sm" aria-hidden="true"></span>正在统计可清理项目…</p>
                <p v-else-if="cleanableItems(database).length === 0" class="mt-2 text-xs text-slate-500">暂无可清理项目。</p>
                <div v-else class="mt-3 grid gap-2">
                  <div v-for="item in cleanableItems(database)" :key="`${database.id}-${item.kind}-${item.label}`" class="flex flex-wrap items-center justify-between gap-2 rounded bg-slate-50 px-3 py-2">
                    <p class="text-xs text-slate-600"><span class="font-medium text-slate-800">{{ item.label }}</span> · {{ item.count }} 项 · 内容约 {{ formatBytes(item.estimatedBytes) }}</p>
                    <button type="button" class="rounded-md bg-slate-800 px-3 py-1.5 text-xs font-semibold text-white disabled:opacity-50" :data-testid="`preview-${item.kind}-${database.id}`" :disabled="database.status !== 'ready' || database.rebuildScheduled || submitting || !isCleanupKind(item.kind)" @click="previewCleanableItem(database, item)">预览清理</button>
                  </div>
                </div>
              </div>
            </div>
          </v-expansion-panel-text>
        </v-expansion-panel>
      </v-expansion-panels>
    </section>

    <section class="rounded-lg border border-rose-200 bg-white px-5 py-5">
      <div class="flex flex-wrap items-start justify-between gap-4"><div><h3 class="text-sm font-semibold text-slate-900">数据库重建</h3><p class="mt-1 max-w-3xl text-xs leading-5 text-slate-500">不兼容数据库必须重建后才能使用。重建会在下次启动时永久删除整个数据库。</p></div><button data-testid="rebuild-incompatible" type="button" class="rounded-md bg-rose-600 px-3 py-2 text-xs font-semibold text-white disabled:opacity-50" :disabled="loading || incompatibleDatabases.length === 0" @click="openBatchRebuild(); showDialog('database-rebuild-dialog')">重建全部不兼容数据库</button></div>
    </section>

    <dialog id="database-cleanup-dialog" class="m-auto w-[min(92vw,560px)] rounded-xl border border-slate-200 bg-white p-0 shadow-2xl backdrop:bg-slate-950/35">
      <form class="grid gap-4 p-5" @submit.prevent="executeCleanup">
        <div><h3 class="text-base font-semibold text-slate-900">永久清理预览</h3><p class="mt-2 text-sm text-slate-600">将永久删除 {{ cleanupPreview?.candidateCount ?? 0 }} 项，内容约 {{ formatBytes(cleanupPreview?.estimatedBytes ?? 0) }}，随后整理数据库。</p></div>
        <ul v-if="cleanupPreview?.items.length" class="grid gap-1 rounded bg-slate-50 p-3 text-xs text-slate-600"><li v-for="item in cleanupPreview.items" :key="item.label">{{ item.label }}：{{ item.count }} 项</li></ul>
        <p v-if="cleanupPreview?.candidateCount === 0" class="rounded border border-emerald-200 bg-emerald-50 px-3 py-2 text-sm text-emerald-700">当前规则下没有可清理项目。</p>
        <label class="grid gap-2 text-sm text-slate-700">输入 <code class="select-all rounded bg-slate-100 px-1 py-0.5">{{ cleanupPreview?.confirmationText }}</code> 以确认<input v-model="confirmation" data-testid="database-cleanup-confirmation" autocomplete="off" class="rounded-md border border-slate-300 px-3 py-2 font-mono text-sm" /></label>
        <div class="flex justify-end gap-2"><button type="button" class="rounded-md border border-slate-200 px-3 py-2 text-sm text-slate-600" @click="closeCleanupDialog">取消</button><button data-testid="confirm-database-cleanup" type="submit" class="rounded-md bg-rose-600 px-3 py-2 text-sm font-semibold text-white disabled:opacity-50" :disabled="cleanupPreview == null || cleanupPreview.candidateCount === 0 || confirmation !== cleanupPreview.confirmationText || submitting">{{ submitting ? "正在清理…" : "确认永久清理" }}</button></div>
      </form>
    </dialog>

    <dialog id="database-compact-dialog" class="m-auto w-[min(92vw,520px)] rounded-xl border border-slate-200 bg-white p-0 shadow-2xl backdrop:bg-slate-950/35">
      <form class="grid gap-4 p-5" @submit.prevent="executeCompact"><div><h3 class="text-base font-semibold text-slate-900">整理 {{ compactDatabase?.name }}</h3><p class="mt-2 text-sm text-slate-600">将执行 WAL checkpoint 和 VACUUM，期间相关写入会暂停。存在活动任务时操作会被拒绝。</p></div><label class="grid gap-2 text-sm text-slate-700">输入 <code class="select-all rounded bg-slate-100 px-1 py-0.5">COMPACT {{ compactDatabase?.id }}</code> 以确认<input v-model="confirmation" data-testid="database-compact-confirmation" class="rounded-md border border-slate-300 px-3 py-2 font-mono text-sm" /></label><div class="flex justify-end gap-2"><button type="button" class="rounded-md border border-slate-200 px-3 py-2 text-sm text-slate-600" @click="closeCompactDialog">取消</button><button data-testid="confirm-database-compact" type="submit" class="rounded-md bg-slate-800 px-3 py-2 text-sm font-semibold text-white disabled:opacity-50" :disabled="compactDatabase == null || confirmation !== `COMPACT ${compactDatabase.id}` || submitting">确认整理</button></div></form>
    </dialog>

    <dialog id="database-rebuild-dialog" class="m-auto w-[min(92vw,520px)] rounded-xl border border-slate-200 bg-white p-0 shadow-2xl backdrop:bg-slate-950/35">
      <form class="grid gap-4 p-5" @submit.prevent="submitRebuild"><div><h3 class="text-base font-semibold text-slate-900">{{ dialogTitle }}</h3><p class="mt-2 text-sm leading-6 text-rose-700">此操作会在下次启动时永久删除所选数据库及其 WAL/SHM 文件，再创建空数据库，无法撤销。</p></div><label class="grid gap-2 text-sm text-slate-700">输入 <code class="select-all rounded bg-slate-100 px-1 py-0.5">{{ requiredConfirmation }}</code> 以确认<input v-model="confirmation" data-testid="database-rebuild-confirmation" autocomplete="off" class="rounded-md border border-slate-300 px-3 py-2 font-mono text-sm" /></label><div class="flex justify-end gap-2"><button type="button" class="rounded-md border border-slate-200 px-3 py-2 text-sm text-slate-600" @click="closeRebuildDialog">取消</button><button data-testid="confirm-database-rebuild" type="submit" class="rounded-md bg-rose-600 px-3 py-2 text-sm font-semibold text-white disabled:opacity-50" :disabled="confirmation !== requiredConfirmation || submitting">{{ submitting ? "正在安排…" : "确认重建" }}</button></div></form>
    </dialog>
  </section>
</template>

<style scoped>
.database-expansion-panels :deep(.v-expansion-panel-title__icon) {
  color: var(--card-text-2) !important;
  opacity: 1;
}

.database-spinner {
  display: inline-block;
  width: 0.875rem;
  height: 0.875rem;
  border: 2px solid rgb(203 213 225);
  border-top-color: rgb(51 65 85);
  border-radius: 9999px;
  animation: database-spin 0.8s linear infinite;
}

.database-spinner-sm {
  width: 0.75rem;
  height: 0.75rem;
}

.database-spinner-xs {
  width: 0.625rem;
  height: 0.625rem;
  border-width: 1.5px;
}

.database-progress-bar {
  position: relative;
  overflow: hidden;
}

.database-progress-bar::after {
  position: absolute;
  inset: 0;
  content: "";
  background: linear-gradient(90deg, transparent, rgb(255 255 255 / 45%), transparent);
  transform: translateX(-100%);
  animation: database-progress-shimmer 1.15s ease-in-out infinite;
}

@keyframes database-spin {
  to {
    transform: rotate(360deg);
  }
}

@keyframes database-progress-shimmer {
  to {
    transform: translateX(100%);
  }
}
</style>
