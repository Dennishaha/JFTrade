<script setup lang="ts">
import { computed, onMounted, ref } from "vue";

import { fetchEnvelope, fetchEnvelopeWithInit } from "../composables/apiClient";

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
};

type StatusResponse = { databases: DatabaseStatus[] };
type RebuildResult = {
  databaseIds: string[];
  restartRequired: boolean;
  scheduled: boolean;
};

const databases = ref<DatabaseStatus[]>([]);
const loading = ref(true);
const submitting = ref(false);
const errorMessage = ref("");
const noticeMessage = ref("");
const confirmation = ref("");
const selectedDatabase = ref<DatabaseStatus | null>(null);
const expandedDatabaseIDs = ref<string[]>([]);
const batchConfirmation = "REBUILD INCOMPATIBLE DATABASES";

const incompatibleDatabases = computed(() =>
  databases.value.filter(
    (database) =>
      ["missing", "incompatible", "unavailable"].includes(database.status) &&
      !database.rebuildScheduled,
  ),
);

const dialogTitle = computed(() =>
  selectedDatabase.value == null
    ? "重建全部不兼容数据库"
    : `重建 ${selectedDatabase.value.name}`,
);

const requiredConfirmation = computed(() =>
  selectedDatabase.value?.confirmationText ?? batchConfirmation,
);

onMounted(() => {
  void loadStatuses();
});

async function loadStatuses(): Promise<void> {
  loading.value = true;
  errorMessage.value = "";
  try {
    const response = await fetchEnvelope<StatusResponse>(
      "/api/v1/settings/data-migration/databases",
    );
    databases.value = response.databases;
  } catch (error) {
    errorMessage.value = error instanceof Error ? error.message : "读取数据库状态失败";
  } finally {
    loading.value = false;
  }
}

function openDatabaseRebuild(database: DatabaseStatus): void {
  selectedDatabase.value = database;
  confirmation.value = "";
}

function openBatchRebuild(): void {
  selectedDatabase.value = null;
  confirmation.value = "";
}

function closeDialog(): void {
  confirmation.value = "";
  selectedDatabase.value = null;
  const dialog = document.getElementById("database-rebuild-dialog") as HTMLDialogElement | null;
  if (typeof dialog?.close === "function") dialog.close();
}

function showDialog(): void {
  const dialog = document.getElementById("database-rebuild-dialog") as HTMLDialogElement | null;
  if (typeof dialog?.showModal === "function") dialog.showModal();
}

async function submitRebuild(): Promise<void> {
  if (confirmation.value !== requiredConfirmation.value || submitting.value) return;
  submitting.value = true;
  errorMessage.value = "";
  try {
    const body =
      selectedDatabase.value == null
        ? { mode: "incompatible", confirmation: confirmation.value }
        : {
            mode: "single",
            databaseId: selectedDatabase.value.id,
            confirmation: confirmation.value,
          };
    const result = await fetchEnvelopeWithInit<RebuildResult>(
      "/api/v1/settings/data-migration/databases/rebuild",
      {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(body),
      },
    );
    noticeMessage.value = result.restartRequired
      ? "已安排重建，请重启 JFTrade。重启完成前相关功能仍不可用。"
      : "重建请求已提交。";
    closeDialog();
    await loadStatuses();
  } catch (error) {
    errorMessage.value = error instanceof Error ? error.message : "安排数据库重建失败";
  } finally {
    submitting.value = false;
  }
}

function statusLabel(status: DatabaseStatus["status"]): string {
  return {
    ready: "可用",
    missing: "缺失",
    incompatible: "不兼容",
    unavailable: "不可用",
  }[status];
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
          <h2 class="text-base font-semibold text-slate-900">数据库重建</h2>
          <p class="mt-1 max-w-3xl text-xs leading-5 text-slate-500">
            当前版本不转换旧数据库。缺少当前 schema 元信息或结构不匹配的数据库必须重建后才能使用。
            设置、密钥、ADK Skill、插件文件和交易日历不会被删除。
          </p>
        </div>
        <button
          data-testid="rebuild-incompatible"
          type="button"
          class="rounded-md bg-rose-600 px-3 py-2 text-xs font-semibold text-white disabled:cursor-not-allowed disabled:opacity-50"
          :disabled="loading || incompatibleDatabases.length === 0"
          @click="openBatchRebuild(); showDialog()"
        >
          重建全部不兼容数据库
        </button>
      </div>
      <p v-if="noticeMessage" class="mt-4 rounded-md border border-amber-200 bg-amber-50 px-3 py-2 text-sm font-medium text-amber-800">
        {{ noticeMessage }}
      </p>
      <p v-if="errorMessage" class="mt-4 text-sm font-medium text-rose-600">{{ errorMessage }}</p>
    </header>

    <div v-if="loading" class="rounded-lg border border-slate-200 bg-white px-5 py-10 text-center text-sm text-slate-500">
      正在检测数据库…
    </div>

    <v-expansion-panels
      v-else
      v-model="expandedDatabaseIDs"
      multiple
      class="database-expansion-panels grid gap-3"
    >
      <v-expansion-panel
        v-for="database in databases"
        :key="database.id"
        :value="database.id"
        :data-testid="`database-card-${database.id}`"
        class="border bg-white"
        :class="database.status === 'ready' ? 'border-slate-200' : 'border-rose-300'"
      >
        <v-expansion-panel-title class="px-5 py-4">
          <div class="flex flex-wrap items-center gap-2">
            <h3 class="text-sm font-semibold text-slate-900">{{ database.name }}</h3>
            <span class="rounded border px-2 py-0.5 text-xs font-medium" :class="statusClass(database.status)">
              {{ statusLabel(database.status) }}
            </span>
            <span v-if="database.rebuildScheduled" class="rounded border border-amber-200 bg-amber-50 px-2 py-0.5 text-xs font-medium text-amber-700">
              等待重启
            </span>
          </div>
        </v-expansion-panel-title>

        <v-expansion-panel-text v-if="expandedDatabaseIDs.includes(database.id)">
          <div class="px-1 pb-2">
            <div class="flex flex-wrap items-start justify-between gap-3">
              <p class="text-xs text-slate-500">{{ database.description }}</p>
              <button
                type="button"
                class="rounded-md border border-rose-200 px-3 py-1.5 text-xs font-medium text-rose-700 hover:bg-rose-50 disabled:cursor-not-allowed disabled:opacity-50"
                :data-testid="`rebuild-${database.id}`"
                :disabled="database.rebuildScheduled"
                @click="openDatabaseRebuild(database); showDialog()"
              >
                {{ database.status === "ready" ? "危险操作：重建" : "重建数据库" }}
              </button>
            </div>

            <dl class="mt-4 grid gap-3 rounded-md bg-slate-50 px-4 py-3 text-xs sm:grid-cols-2">
              <div>
                <dt class="text-slate-400">文件路径</dt>
                <dd class="mt-1 break-all font-mono text-slate-700">{{ database.path }}</dd>
              </div>
              <div>
                <dt class="text-slate-400">Schema 版本</dt>
                <dd class="mt-1 text-slate-700">{{ database.currentVersion ?? "未知" }} / {{ database.expectedVersion }}</dd>
              </div>
              <div>
                <dt class="text-slate-400">影响功能</dt>
                <dd class="mt-1 text-slate-700">{{ database.features.join("、") }}</dd>
              </div>
              <div v-if="database.error">
                <dt class="text-rose-500">检测结果</dt>
                <dd class="mt-1 text-rose-700">{{ database.error }}</dd>
              </div>
            </dl>
            <p v-if="database.status !== 'ready'" class="mt-3 text-xs font-semibold text-rose-700">
              必须重建并重启 JFTrade 后才能使用相关功能。
            </p>
          </div>
        </v-expansion-panel-text>
      </v-expansion-panel>
    </v-expansion-panels>

    <dialog id="database-rebuild-dialog" class="m-auto w-[min(92vw,520px)] rounded-xl border border-slate-200 bg-white p-0 shadow-2xl backdrop:bg-slate-950/35">
      <form class="grid gap-4 p-5" @submit.prevent="submitRebuild">
        <div>
          <h3 class="text-base font-semibold text-slate-900">{{ dialogTitle }}</h3>
          <p class="mt-2 text-sm leading-6 text-rose-700">
            此操作会在下次启动时永久删除所选数据库及其 WAL/SHM 文件，再创建空数据库，无法撤销。
          </p>
        </div>
        <label class="grid gap-2 text-sm text-slate-700">
          输入 <code class="select-all rounded bg-slate-100 px-1 py-0.5">{{ requiredConfirmation }}</code> 以确认
          <input
            v-model="confirmation"
            data-testid="database-rebuild-confirmation"
            autocomplete="off"
            class="rounded-md border border-slate-300 px-3 py-2 font-mono text-sm outline-none focus:border-rose-500"
          />
        </label>
        <div class="flex justify-end gap-2">
          <button type="button" class="rounded-md border border-slate-200 px-3 py-2 text-sm text-slate-600" @click="closeDialog">取消</button>
          <button
            data-testid="confirm-database-rebuild"
            type="submit"
            class="rounded-md bg-rose-600 px-3 py-2 text-sm font-semibold text-white disabled:cursor-not-allowed disabled:opacity-50"
            :disabled="confirmation !== requiredConfirmation || submitting"
          >
            {{ submitting ? "正在安排…" : "确认重建" }}
          </button>
        </div>
      </form>
    </dialog>
  </section>
</template>

<style scoped>
.database-expansion-panels :deep(.v-expansion-panel-title__icon) {
  color: var(--card-text-2) !important;
  opacity: 1;
}
</style>
