<script setup lang="ts">
import { computed, onMounted, ref } from "vue";

import type {
  PineWorkerSettingsResponse,
  RuntimeDependenciesResponse,
  RuntimeDependencyItem,
} from "@/contracts";
import { fetchEnvelope, fetchEnvelopeWithInit } from "../composables/apiClient";
import SectionHeader from "./SectionHeader.vue";

const props = withDefaults(
  defineProps<{
    mode?: "oobe" | "settings";
  }>(),
  {
    mode: "settings",
  },
);

const emit = defineEmits<{
  "status-change": [RuntimeDependenciesResponse];
}>();

const defaultPineWorkerSettings: PineWorkerSettingsResponse = {
  backtestWorkerLimit: 2,
  instanceWorkerLimit: 10,
  nodeBinaryPath: "",
};

const runtimeDependencies = ref<RuntimeDependenciesResponse>({
  checkedAt: "",
  allRequiredSatisfied: false,
  dependencies: [],
});
const pineWorkerSettings = ref<PineWorkerSettingsResponse>({
  ...defaultPineWorkerSettings,
});
const nodeBinaryPathInput = ref("");
const loading = ref(true);
const refreshing = ref(false);
const saving = ref(false);
const errorMessage = ref("");
const noticeMessage = ref("");

const headerDescription = computed(() =>
  props.mode === "oobe"
    ? "先确认策略运行需要的本机依赖。缺失时可以继续配置券商，但策略运行会不可用。"
    : "检查并配置策略运行需要的本机依赖。",
);

const headerTitle = computed(() =>
  props.mode === "oobe" ? "运行时依赖" : "依赖项管理",
);

const requiredIssueCount = computed(
  () =>
    runtimeDependencies.value.dependencies.filter(
      (dependency) =>
        dependency.required && dependency.status.toLowerCase() !== "ok",
    ).length,
);

const nodeDependency = computed(
  () =>
    runtimeDependencies.value.dependencies.find(
      (dependency) => dependency.id === "node",
    ) ?? null,
);

const checkedAtLabel = computed(() =>
  runtimeDependencies.value.checkedAt === ""
    ? "尚未检查"
    : runtimeDependencies.value.checkedAt,
);

onMounted(() => {
  void loadRuntimeDependencyPanel();
});

async function loadRuntimeDependencyPanel(): Promise<void> {
  loading.value = true;
  errorMessage.value = "";
  noticeMessage.value = "";
  try {
    await Promise.all([loadPineWorkerSettings(), refreshDependencies()]);
  } catch (error) {
    errorMessage.value =
      error instanceof Error ? error.message : "读取运行时依赖失败";
  } finally {
    loading.value = false;
  }
}

async function loadPineWorkerSettings(): Promise<void> {
  pineWorkerSettings.value = await fetchEnvelope<PineWorkerSettingsResponse>(
    "/api/v1/settings/pine-worker",
  );
  nodeBinaryPathInput.value = pineWorkerSettings.value.nodeBinaryPath ?? "";
}

async function refreshDependencies(): Promise<void> {
  refreshing.value = true;
  errorMessage.value = "";
  try {
    const response = await fetchEnvelope<RuntimeDependenciesResponse>(
      "/api/v1/system/runtime-dependencies",
    );
    runtimeDependencies.value = response;
    emit("status-change", response);
  } catch (error) {
    errorMessage.value =
      error instanceof Error ? error.message : "检查运行时依赖失败";
  } finally {
    refreshing.value = false;
  }
}

async function saveNodeBinaryPath(): Promise<void> {
  if (saving.value) return;
  saving.value = true;
  errorMessage.value = "";
  noticeMessage.value = "";
  try {
    const next: PineWorkerSettingsResponse = {
      ...pineWorkerSettings.value,
      nodeBinaryPath: nodeBinaryPathInput.value.trim(),
    };
    pineWorkerSettings.value = await fetchEnvelopeWithInit<PineWorkerSettingsResponse>(
      "/api/v1/settings/pine-worker",
      {
        method: "PUT",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(next),
      },
    );
    nodeBinaryPathInput.value = pineWorkerSettings.value.nodeBinaryPath ?? "";
    noticeMessage.value = "Node.js 路径已保存。";
    await refreshDependencies();
  } catch (error) {
    errorMessage.value =
      error instanceof Error ? error.message : "保存 Node.js 路径失败";
  } finally {
    saving.value = false;
  }
}

function dependencyStatusLabel(status: string): string {
  switch (status.toLowerCase()) {
    case "ok":
      return "可用";
    case "missing":
      return "缺失";
    case "outdated":
      return "版本过低";
    default:
      return "异常";
  }
}

function dependencyStatusClass(status: string): string {
  switch (status.toLowerCase()) {
    case "ok":
      return "status-ok";
    case "missing":
    case "outdated":
      return "status-warning";
    default:
      return "status-error";
  }
}

function dependencyVersionLabel(value: string): string {
  return value.trim() === "" ? "-" : value;
}

function dependencyPathLabel(value: string): string {
  return value.trim() === "" ? "自动检测" : value;
}

function dependencySourceLabel(value: string): string {
  if (value === "settings") return "设置";
  if (value === "path") return "PATH";
  if (value.startsWith("env:")) return value.replace("env:", "环境变量 ");
  return value || "-";
}
</script>

<template>
  <section class="runtime-dependencies-panel">
    <SectionHeader :title="headerTitle" :description="headerDescription" />

    <div class="runtime-summary">
      <div class="runtime-summary__content">
        <span
          class="runtime-summary__indicator"
          :class="
            runtimeDependencies.allRequiredSatisfied
              ? 'runtime-summary__indicator--ok'
              : 'runtime-summary__indicator--warning'
          "
          aria-hidden="true"
        />
        <div>
          <div class="runtime-summary__title">
            {{
              runtimeDependencies.allRequiredSatisfied
                ? "必需依赖已满足"
                : `${requiredIssueCount} 个必需依赖需要处理`
            }}
          </div>
          <div class="runtime-summary__meta">检查时间：{{ checkedAtLabel }}</div>
        </div>
      </div>
      <button
        data-testid="runtime-dependencies-refresh"
        type="button"
        class="secondary-button"
        :disabled="loading || refreshing || saving"
        @click="refreshDependencies"
      >
        {{ refreshing ? "检查中" : "重新检查" }}
      </button>
    </div>

    <div
      v-if="props.mode === 'oobe' && !runtimeDependencies.allRequiredSatisfied"
      class="runtime-warning"
    >
      Node.js 缺失或版本过低时，策略回测与运行实例无法启动。可以继续配置券商，
      但在修复依赖前策略系统仍不可用。
    </div>

    <div v-if="loading" class="runtime-empty">正在读取运行时依赖...</div>

    <div v-else class="runtime-dependency-list">
      <article
        v-for="dependency in runtimeDependencies.dependencies"
        :key="dependency.id"
        class="runtime-dependency-card"
        :class="dependencyStatusClass(dependency.status)"
        :data-testid="`runtime-dependency-${dependency.id}`"
      >
        <div class="dependency-card__header">
          <div>
            <div class="dependency-card__title">
              {{ dependency.displayName }}
              <span v-if="dependency.required" class="dependency-required">
                必需
              </span>
            </div>
            <p class="dependency-card__message">
              {{ dependency.message || "尚未返回检查结果。" }}
            </p>
          </div>
          <span
            class="dependency-status"
            :class="dependencyStatusClass(dependency.status)"
          >
            {{ dependencyStatusLabel(dependency.status) }}
          </span>
        </div>

        <dl class="dependency-details">
          <div>
            <dt>最低版本</dt>
            <dd>{{ dependencyVersionLabel(dependency.minimumVersion) }}</dd>
          </div>
          <div>
            <dt>检测版本</dt>
            <dd>{{ dependencyVersionLabel(dependency.detectedVersion) }}</dd>
          </div>
          <div>
            <dt>生效路径</dt>
            <dd>{{ dependencyPathLabel(dependency.effectivePath) }}</dd>
          </div>
          <div>
            <dt>解析路径</dt>
            <dd>{{ dependencyPathLabel(dependency.resolvedPath) }}</dd>
          </div>
          <div>
            <dt>来源</dt>
            <dd>{{ dependencySourceLabel(dependency.source) }}</dd>
          </div>
        </dl>

        <div class="dependency-actions">
          <a
            v-if="dependency.homepageUrl"
            :href="dependency.homepageUrl"
            target="_blank"
            rel="noreferrer"
          >
            打开 {{ dependency.displayName }} 网站
          </a>
        </div>

        <div v-if="dependency.id === 'node'" class="dependency-path-editor">
          <label for="node-binary-path">自定义 Node.js 二进制路径</label>
          <div class="dependency-path-editor__row">
            <input
              id="node-binary-path"
              v-model="nodeBinaryPathInput"
              data-testid="runtime-dependency-node-path-input"
              type="text"
              placeholder="留空自动检测，例如 /opt/node/bin/node 或 C:\Program Files\nodejs\node.exe"
              :disabled="loading || saving"
            />
            <button
              data-testid="runtime-dependency-node-path-save"
              type="button"
              class="primary-button"
              :disabled="loading || saving"
              @click="saveNodeBinaryPath"
            >
              {{ saving ? "保存中" : "保存路径并检查" }}
            </button>
          </div>
        </div>
      </article>

      <div
        v-if="runtimeDependencies.dependencies.length === 0"
        class="runtime-empty"
      >
        当前没有需要检查的运行时依赖。
      </div>
    </div>

    <p v-if="noticeMessage" class="runtime-notice">{{ noticeMessage }}</p>
    <p v-if="errorMessage" class="runtime-error">{{ errorMessage }}</p>
  </section>
</template>

<style scoped>
.runtime-dependencies-panel {
  border: 1px solid var(--card-border);
  background:
    linear-gradient(
      180deg,
      color-mix(in srgb, var(--card-surface) 96%, var(--card-active-surface) 4%) 0%,
      var(--card-surface) 100%
    );
  border-radius: 8px;
  padding: 20px 24px;
  color: var(--card-text-1);
  box-shadow: 0 18px 42px rgba(2, 6, 23, 0.14);
}

.runtime-summary {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 16px;
  margin-top: 18px;
  border: 1px solid var(--card-border);
  border-radius: 8px;
  background: var(--card-surface-raised);
  padding: 16px;
}

.runtime-summary__content {
  display: flex;
  min-width: 0;
  align-items: flex-start;
  gap: 12px;
}

.runtime-summary__indicator {
  position: relative;
  margin-top: 3px;
  display: inline-flex;
  width: 12px;
  height: 12px;
  flex: 0 0 auto;
  border-radius: 999px;
}

.runtime-summary__indicator::after {
  position: absolute;
  inset: -5px;
  border-radius: inherit;
  content: "";
}

.runtime-summary__indicator--ok {
  background: var(--card-active-text);
}

.runtime-summary__indicator--ok::after {
  background: color-mix(in srgb, var(--card-active-text) 18%, transparent);
}

.runtime-summary__indicator--warning {
  background: var(--card-amber-text);
}

.runtime-summary__indicator--warning::after {
  background: color-mix(in srgb, var(--card-amber-text) 18%, transparent);
}

.runtime-summary__title {
  color: var(--card-text-1);
  font-size: 15px;
  font-weight: 700;
}

.runtime-summary__meta {
  margin-top: 4px;
  color: var(--card-text-3);
  font-size: 12px;
}

.runtime-warning {
  margin-top: 14px;
  border: 1px solid var(--card-amber-border);
  background: var(--card-amber-surface);
  color: var(--card-amber-text);
  border-radius: 8px;
  padding: 12px 14px;
  font-size: 13px;
  line-height: 1.7;
}

.runtime-dependency-list {
  display: grid;
  gap: 14px;
  margin-top: 14px;
}

.runtime-dependency-card {
  position: relative;
  overflow: hidden;
  border: 1px solid var(--card-border);
  border-radius: 8px;
  background:
    linear-gradient(
      180deg,
      color-mix(in srgb, var(--card-surface-raised) 86%, transparent) 0%,
      color-mix(in srgb, var(--card-surface) 96%, transparent) 100%
    );
  padding: 16px;
  transition: border-color 0.15s ease, background-color 0.15s ease;
}

.runtime-dependency-card::before {
  position: absolute;
  inset: 0 auto 0 0;
  width: 3px;
  background: var(--card-border);
  content: "";
}

.runtime-dependency-card.status-ok::before {
  background: var(--card-active-text);
}

.runtime-dependency-card.status-warning::before {
  background: var(--card-amber-text);
}

.runtime-dependency-card.status-error::before {
  background: var(--card-red-text);
}

.dependency-card__header {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 14px;
}

.dependency-card__title {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 8px;
  color: var(--card-text-1);
  font-weight: 700;
}

.dependency-required,
.dependency-status {
  border-radius: 999px;
  padding: 3px 8px;
  font-size: 11px;
  font-weight: 700;
  line-height: 1.3;
}

.dependency-required {
  border: 1px solid var(--card-border);
  background: color-mix(in srgb, var(--card-surface-raised) 82%, transparent);
  color: var(--card-text-2);
}

.dependency-status {
  border: 1px solid transparent;
  white-space: nowrap;
}

.runtime-dependency-card.status-ok {
  border-color: color-mix(in srgb, var(--card-active-border) 58%, var(--card-border));
}

.runtime-dependency-card.status-warning {
  border-color: color-mix(in srgb, var(--card-amber-border) 64%, var(--card-border));
}

.runtime-dependency-card.status-error {
  border-color: color-mix(in srgb, var(--card-red-border) 64%, var(--card-border));
}

.dependency-status.status-ok {
  border-color: var(--card-active-border);
  background: var(--card-active-surface);
  color: var(--card-active-text);
}

.dependency-status.status-warning {
  border-color: var(--card-amber-border);
  background: var(--card-amber-surface);
  color: var(--card-amber-text);
}

.dependency-status.status-error {
  border-color: var(--card-red-border);
  background: var(--card-red-surface);
  color: var(--card-red-text);
}

.dependency-card__message {
  margin: 8px 0 0;
  color: var(--card-text-2);
  font-size: 13px;
  line-height: 1.6;
}

.dependency-details {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(160px, 1fr));
  gap: 10px;
  margin: 14px 0 0;
}

.dependency-details div {
  min-width: 0;
  border: 1px solid var(--card-border-muted);
  border-radius: 8px;
  background: color-mix(in srgb, var(--card-surface) 74%, transparent);
  padding: 10px;
}

.dependency-details dt {
  color: var(--card-text-3);
  font-size: 12px;
}

.dependency-details dd {
  margin: 4px 0 0;
  overflow-wrap: anywhere;
  color: var(--card-text-1);
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
  font-size: 12px;
  font-weight: 700;
}

.dependency-actions {
  margin-top: 12px;
  font-size: 13px;
}

.dependency-actions a {
  display: inline-flex;
  align-items: center;
  border: 1px solid var(--card-sky-border);
  border-radius: 8px;
  background: var(--card-sky-surface);
  padding: 7px 10px;
  color: var(--card-sky-text);
  font-weight: 700;
  text-decoration: none;
  transition: border-color 0.15s ease, background-color 0.15s ease;
}

.dependency-actions a:hover {
  border-color: color-mix(in srgb, var(--card-sky-text) 55%, var(--card-sky-border));
  background: color-mix(in srgb, var(--card-sky-surface) 78%, var(--card-sky-text) 10%);
}

.dependency-path-editor {
  display: grid;
  gap: 8px;
  margin-top: 14px;
  border-top: 1px solid var(--card-border-muted);
  padding-top: 14px;
}

.dependency-path-editor label {
  color: var(--card-text-2);
  font-size: 13px;
  font-weight: 700;
}

.dependency-path-editor__row {
  display: grid;
  grid-template-columns: minmax(0, 1fr) auto;
  gap: 10px;
}

.dependency-path-editor input {
  min-width: 0;
  border: 1px solid var(--card-border);
  border-radius: 8px;
  background: color-mix(in srgb, var(--card-surface) 90%, var(--card-surface-raised));
  padding: 9px 10px;
  color: var(--card-text-1);
  font-size: 13px;
  outline: none;
  transition: border-color 0.15s ease, box-shadow 0.15s ease;
}

.dependency-path-editor input::placeholder {
  color: var(--card-text-3);
}

.dependency-path-editor input:focus {
  border-color: var(--card-active-border);
  box-shadow: 0 0 0 3px color-mix(in srgb, var(--card-active-border) 28%, transparent);
}

.primary-button,
.secondary-button {
  border-radius: 8px;
  padding: 9px 12px;
  font-size: 13px;
  font-weight: 700;
  transition: background 0.15s ease, color 0.15s ease, border-color 0.15s ease, transform 0.15s ease;
}

.primary-button {
  border: 1px solid var(--card-teal-btn-bg);
  background: var(--card-teal-btn-bg);
  color: var(--card-teal-btn-text);
}

.secondary-button {
  border: 1px solid var(--card-border);
  background: color-mix(in srgb, var(--card-surface) 86%, transparent);
  color: var(--card-text-2);
}

.primary-button:not(:disabled):hover,
.secondary-button:not(:disabled):hover {
  transform: translateY(-1px);
}

.primary-button:not(:disabled):hover {
  border-color: var(--card-teal-text);
  background: color-mix(in srgb, var(--card-teal-btn-bg) 86%, var(--card-teal-text));
}

.secondary-button:not(:disabled):hover {
  border-color: var(--card-active-border);
  background: var(--card-active-surface);
  color: var(--card-active-text);
}

.primary-button:disabled,
.secondary-button:disabled,
.dependency-path-editor input:disabled {
  cursor: not-allowed;
  opacity: 0.55;
}

.runtime-empty,
.runtime-notice,
.runtime-error {
  margin-top: 14px;
  font-size: 13px;
}

.runtime-empty {
  color: var(--card-text-3);
}

.runtime-notice {
  border: 1px solid var(--card-active-border);
  border-radius: 8px;
  background: var(--card-active-surface);
  padding: 10px 12px;
  color: var(--card-active-text);
  font-weight: 700;
}

.runtime-error {
  border: 1px solid var(--card-red-border);
  border-radius: 8px;
  background: var(--card-red-surface);
  padding: 10px 12px;
  color: var(--card-red-text);
  font-weight: 700;
}

@media (max-width: 720px) {
  .runtime-summary,
  .dependency-card__header,
  .dependency-path-editor__row {
    grid-template-columns: 1fr;
  }

  .runtime-summary,
  .dependency-card__header {
    display: grid;
  }
}
</style>
