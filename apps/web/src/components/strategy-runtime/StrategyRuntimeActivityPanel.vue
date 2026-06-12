<script setup lang="ts">
import { computed, ref, watch } from "vue";
import type {
    StrategyAuditEntryDocument,
} from "@/contracts";

import MonacoCodeEditor from "../MonacoCodeEditor.vue";

type StrategyActivityTab = "logs" | "audit";
type StrategyActivityLevel = "all" | "error" | "warning" | "info";

interface StrategyTimestampParts {
    display: string;
    utc: string;
    timestampMs: number | null;
}

interface StrategyLogViewEntry {
    raw: string;
    message: string;
    at: string;
    timestampMs: number | null;
    level: Exclude<StrategyActivityLevel, "all">;
}

interface StrategyAuditViewEntry extends StrategyAuditEntryDocument {
    detailText: string;
    label: string;
    level: Exclude<StrategyActivityLevel, "all">;
    timestampMs: number | null;
}

interface StrategyActivityDetailView {
    title: string;
    kindLabel: string;
    summary: string;
    detail: string;
    at: string;
    utc: string;
    level: Exclude<StrategyActivityLevel, "all">;
    rawKind?: string;
}

const props = defineProps<{
    isLoadingDetails: boolean;
    strategyLogs: string[];
    strategyAuditEntries: StrategyAuditEntryDocument[];
    selectedStrategyParamsJson: string;
}>();

const strategyActivityTab = ref<StrategyActivityTab>("logs");
const strategyActivityLevelFilter = ref<StrategyActivityLevel>("all");
const strategyParamsDialogOpen = ref(false);
const strategyActivityDetailDialogOpen = ref(false);
const selectedStrategyActivityDetail = ref<StrategyActivityDetailView | null>(null);

const localTimestampFormatter = new Intl.DateTimeFormat("zh-CN", {
    year: "numeric",
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
    second: "2-digit",
    hour12: false,
});

const utcTimestampFormatter = new Intl.DateTimeFormat("zh-CN", {
    year: "numeric",
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
    second: "2-digit",
    hour12: false,
    timeZone: "UTC",
});

const strategyLogViewEntries = computed<StrategyLogViewEntry[]>(() =>
    sortActivityEntriesByTime(props.strategyLogs.map((entry) => parseStrategyLogEntry(entry))),
);

const strategyAuditViewEntries = computed<StrategyAuditViewEntry[]>(() =>
    sortActivityEntriesByTime(props.strategyAuditEntries.map((entry) => ({
        ...entry,
        detailText: entry.detail ?? "生命周期变更",
        label: formatAuditKind(entry.kind),
        level: classifyStrategyAuditLevel(entry),
        timestampMs: formatTimestampParts(entry.at).timestampMs,
    }))),
);

const strategyActivityTabs = computed(() => [
    {
        value: "logs" as const,
        label: "运行日志",
        count: strategyLogViewEntries.value.length,
    },
    {
        value: "audit" as const,
        label: "运行审计",
        count: strategyAuditViewEntries.value.length,
    },
]);

const strategyActivityLevelOptions = computed(() => {
    const items = strategyActivityTab.value === "logs"
        ? strategyLogViewEntries.value
        : strategyAuditViewEntries.value;
    const counts = new Map<Exclude<StrategyActivityLevel, "all">, number>([
        ["error", 0],
        ["warning", 0],
        ["info", 0],
    ]);
    for (const item of items) {
        counts.set(item.level, (counts.get(item.level) ?? 0) + 1);
    }
    const options: Array<{
        value: StrategyActivityLevel;
        label: string;
        count: number;
    }> = [{
        value: "all" as const,
        label: formatStrategyActivityLevel("all"),
        count: items.length,
    }];
    for (const level of ["error", "warning", "info"] as const) {
        const count = counts.get(level) ?? 0;
        if (count === 0) {
            continue;
        }
        options.push({
            value: level,
            label: formatStrategyActivityLevel(level),
            count,
        });
    }
    return options;
});

const filteredStrategyLogViewEntries = computed(() => {
    if (strategyActivityLevelFilter.value === "all") {
        return strategyLogViewEntries.value;
    }
    return strategyLogViewEntries.value.filter(
        (entry) => entry.level === strategyActivityLevelFilter.value,
    );
});

const filteredStrategyAuditViewEntries = computed(() => {
    if (strategyActivityLevelFilter.value === "all") {
        return strategyAuditViewEntries.value;
    }
    return strategyAuditViewEntries.value.filter(
        (entry) => entry.level === strategyActivityLevelFilter.value,
    );
});

const strategyActivityEmptyMessage = computed(() => {
    if (strategyActivityTab.value === "logs") {
        return strategyActivityLevelFilter.value === "all"
            ? "暂无日志。"
            : "当前筛选下暂无日志。";
    }
    return strategyActivityLevelFilter.value === "all"
        ? "暂无审计记录。"
        : "当前筛选下暂无审计记录。";
});

watch(
    strategyActivityLevelOptions,
    (options) => {
        if (!options.some((option) => option.value === strategyActivityLevelFilter.value)) {
            strategyActivityLevelFilter.value = "all";
        }
    },
    { immediate: true },
);

function normalizeText(value: unknown): string {
    return typeof value === "string" ? value.trim() : "";
}

function formatTimestampParts(value: unknown): StrategyTimestampParts {
    const normalized = normalizeText(value);
    if (normalized === "") {
        return {
            display: "暂无",
            utc: "暂无",
            timestampMs: null,
        };
    }

    const parsed = new Date(normalized);
    if (Number.isNaN(parsed.getTime())) {
        const fallback = normalized.replace("T", " ").replace(".000Z", "Z");
        return {
            display: fallback,
            utc: fallback,
            timestampMs: null,
        };
    }

    return {
        display: localTimestampFormatter.format(parsed),
        utc: `${utcTimestampFormatter.format(parsed)} UTC`,
        timestampMs: parsed.getTime(),
    };
}

function formatTimestamp(value: unknown): string {
    return formatTimestampParts(value).display;
}

function formatTimestampTooltip(value: unknown): string {
    return formatTimestampParts(value).utc;
}

function sortActivityEntriesByTime<T extends { timestampMs: number | null }>(items: T[]): T[] {
    return items
        .map((item, index) => ({ item, index }))
        .sort((left, right) => {
            const leftTime = left.item.timestampMs ?? Number.NEGATIVE_INFINITY;
            const rightTime = right.item.timestampMs ?? Number.NEGATIVE_INFINITY;
            if (rightTime !== leftTime) {
                return rightTime - leftTime;
            }
            return right.index - left.index;
        })
        .map(({ item }) => item);
}

function formatAuditKind(kind: unknown): string {
    switch (normalizeText(kind).toLowerCase()) {
        case "instantiated":
            return "已实例化";
        case "binding.updated":
            return "已更新绑定";
        case "created":
            return "已创建";
        case "started":
            return "已启动";
        case "running":
            return "运行中";
        case "paused":
            return "已暂停";
        case "stopped":
            return "已停止";
        case "failed":
            return "执行失败";
        default:
            return normalizeText(kind) || "未知";
    }
}

function formatStrategyActivityLevel(level: StrategyActivityLevel): string {
    switch (level) {
        case "error":
            return "高优先";
        case "warning":
            return "需关注";
        case "info":
            return "常规";
        default:
            return "全部";
    }
}

function classifyStrategyLogLevel(message: string): Exclude<StrategyActivityLevel, "all"> {
    const normalized = normalizeText(message).toLowerCase();
    if (["panic", "fatal", "error", "failed", "exception", "reject", "denied", "timeout"].some(
        (keyword) => normalized.includes(keyword),
    )) {
        return "error";
    }
    if (["warn", "warning", "paused", "pause", "stopped", "stop", "retry", "skip", "throttle"].some(
        (keyword) => normalized.includes(keyword),
    )) {
        return "warning";
    }
    return "info";
}

function classifyStrategyAuditLevel(entry: StrategyAuditEntryDocument): Exclude<StrategyActivityLevel, "all"> {
    const signal = `${normalizeText(entry.kind)} ${normalizeText(entry.detail)}`.toLowerCase();
    if (["failed", "panic", "error", "exception", "reject", "denied", "timeout"].some(
        (keyword) => signal.includes(keyword),
    )) {
        return "error";
    }
    if (["paused", "pause", "stopped", "stop", "retry", "warning", "warn"].some(
        (keyword) => signal.includes(keyword),
    )) {
        return "warning";
    }
    return "info";
}

function parseStrategyLogEntry(entry: string): StrategyLogViewEntry {
    const raw = normalizeText(entry);
    const matched = raw.match(/^(\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d+)?Z)\s*(.*)$/);
    const at = matched?.[1] ?? "";
    const message = normalizeText(matched?.[2]) || raw;
    const timestampMs = at === "" ? null : formatTimestampParts(at).timestampMs;
    return {
        raw: entry,
        message,
        at,
        timestampMs,
        level: classifyStrategyLogLevel(message || raw),
    };
}

function openStrategyActivityDetail(detail: StrategyActivityDetailView): void {
    selectedStrategyActivityDetail.value = detail;
    strategyActivityDetailDialogOpen.value = true;
}

function closeStrategyActivityDetailDialog(): void {
    strategyActivityDetailDialogOpen.value = false;
    selectedStrategyActivityDetail.value = null;
}

function buildLogActivityDetail(entry: StrategyLogViewEntry): StrategyActivityDetailView {
    return {
        title: "运行日志",
        kindLabel: "日志详情",
        summary: entry.message,
        detail: entry.raw,
        at: formatTimestamp(entry.at),
        utc: formatTimestampTooltip(entry.at),
        level: entry.level,
    };
}

function buildAuditActivityDetail(entry: StrategyAuditViewEntry): StrategyActivityDetailView {
    return {
        title: entry.label,
        kindLabel: "审计详情",
        summary: entry.detailText,
        detail: [
            `instanceId: ${entry.instanceId}`,
            `kind: ${entry.kind}`,
            `detail: ${entry.detailText}`,
            `at: ${entry.at}`,
        ].join("\n"),
        at: formatTimestamp(entry.at),
        utc: formatTimestampTooltip(entry.at),
        level: entry.level,
        rawKind: entry.kind,
    };
}
</script>

<template>
    <div class="strategy-activity-panel rounded-[28px] border border-slate-200 bg-white p-4"
        data-testid="strategy-activity-panel">
        <div class="flex flex-col gap-4 xl:flex-row xl:items-start xl:justify-between">
            <div>
                <div class="text-xl font-semibold text-slate-900">运行明细</div>
                <div class="mt-1 text-sm text-slate-500">
                    在运行日志与运行审计之间切换，并按重要性快速聚焦当前实例的问题与状态变化。
                </div>
            </div>
            <div class="flex flex-wrap items-center gap-2 xl:justify-end">
                <button v-for="tab in strategyActivityTabs" :key="tab.value" type="button"
                    class="strategy-activity-tab"
                    :class="{ 'is-active': strategyActivityTab === tab.value }"
                    :data-testid="`strategy-activity-tab-${tab.value}`"
                    @click="strategyActivityTab = tab.value">
                    <span>{{ tab.label }}</span>
                    <span class="strategy-activity-tab-count">{{ tab.count }}</span>
                </button>
                <button type="button" class="strategy-runtime-params-trigger"
                    data-testid="strategy-open-params-dialog" aria-label="查看运行参数"
                    @click="strategyParamsDialogOpen = true">
                    <span aria-hidden="true">{}</span>
                </button>
            </div>
        </div>

        <div class="mt-4 flex flex-wrap gap-2">
            <button v-for="option in strategyActivityLevelOptions" :key="option.value"
                type="button" class="strategy-activity-filter"
                :class="[
                    `strategy-activity-filter--${option.value}`,
                    { 'is-active': strategyActivityLevelFilter === option.value },
                ]"
                :data-testid="`strategy-activity-filter-${option.value}`"
                @click="strategyActivityLevelFilter = option.value">
                <span>{{ option.label }}</span>
                <span class="strategy-activity-filter-count">{{ option.count }}</span>
            </button>
        </div>

        <div class="mt-4">
            <div v-if="isLoadingDetails" class="strategy-activity-empty">
                正在加载运行明细…
            </div>

            <ul v-else-if="strategyActivityTab === 'logs' && filteredStrategyLogViewEntries.length > 0"
                class="strategy-activity-viewport" data-testid="strategy-log-list">
                <li v-for="(entry, index) in filteredStrategyLogViewEntries" :key="entry.raw"
                    class="strategy-activity-entry" :class="`strategy-activity-entry--${entry.level}`"
                    :data-testid="`strategy-log-entry-${index}`">
                    <div class="flex flex-wrap items-center justify-between gap-3">
                        <div class="flex flex-wrap items-center gap-2">
                            <span class="strategy-activity-level-badge"
                                :class="`strategy-activity-level-badge--${entry.level}`">{{
                                    formatStrategyActivityLevel(entry.level) }}</span>
                            <span class="strategy-activity-kind-badge">运行日志</span>
                        </div>
                        <span class="strategy-activity-time strategy-time-display"
                            :title="entry.at === '' ? '' : formatTimestampTooltip(entry.at)">{{
                            entry.at === '' ? '未标注时间' : formatTimestamp(entry.at) }}</span>
                    </div>
                    <div class="mt-3 flex items-center gap-2">
                        <div class="strategy-activity-entry__summary strategy-activity-entry__summary--log min-w-0 flex-1"
                            :title="entry.message">
                            {{ entry.message }}
                        </div>
                        <button type="button" class="strategy-activity-entry__detail-trigger"
                            :data-testid="`strategy-log-detail-trigger-${index}`"
                            @click="openStrategyActivityDetail(buildLogActivityDetail(entry))">
                            ....
                        </button>
                    </div>
                </li>
            </ul>

            <ul v-else-if="strategyActivityTab === 'audit' && filteredStrategyAuditViewEntries.length > 0"
                class="strategy-activity-viewport" data-testid="strategy-audit-list">
                <li v-for="(entry, index) in filteredStrategyAuditViewEntries"
                    :key="`${entry.at}-${entry.kind}-${entry.detail ?? ''}`"
                    class="strategy-activity-entry" :class="`strategy-activity-entry--${entry.level}`"
                    :data-testid="`strategy-audit-entry-${index}`">
                    <div class="flex flex-wrap items-center justify-between gap-3">
                        <div class="flex flex-wrap items-center gap-2">
                            <span class="strategy-activity-level-badge"
                                :class="`strategy-activity-level-badge--${entry.level}`">{{
                                    formatStrategyActivityLevel(entry.level) }}</span>
                            <span class="strategy-activity-kind-badge">{{ entry.label }}</span>
                        </div>
                        <span class="strategy-activity-time strategy-time-display"
                            :title="formatTimestampTooltip(entry.at)">{{ formatTimestamp(entry.at) }}</span>
                    </div>
                    <div class="mt-3 flex items-center gap-2">
                        <div class="strategy-activity-entry__summary strategy-activity-entry__summary--audit min-w-0 flex-1"
                            :title="entry.detailText">
                            {{ entry.detailText }}
                        </div>
                        <button type="button" class="strategy-activity-entry__detail-trigger"
                            :data-testid="`strategy-audit-detail-trigger-${index}`"
                            @click="openStrategyActivityDetail(buildAuditActivityDetail(entry))">
                            ....
                        </button>
                    </div>
                </li>
            </ul>

            <div v-else class="strategy-activity-empty">
                {{ strategyActivityEmptyMessage }}
            </div>
        </div>

        <v-dialog v-model="strategyParamsDialogOpen" max-width="840">
            <div class="strategy-instance-dialog strategy-params-dialog"
                data-testid="strategy-params-dialog">
                <div class="flex flex-wrap items-start justify-between gap-4">
                    <div>
                        <div class="text-xl font-semibold text-slate-900">运行参数</div>
                        <div class="mt-1 text-sm text-slate-500">
                            当前实例启动时使用的参数快照，通过对话框查看避免占用主面板高度。
                        </div>
                    </div>
                    <button type="button" class="strategy-params-dialog-close"
                        data-testid="strategy-close-params-dialog"
                        @click="strategyParamsDialogOpen = false">
                        关闭
                    </button>
                </div>
                <div class="mt-4 strategy-params-editor">
                    <MonacoCodeEditor
                        :model-value="selectedStrategyParamsJson"
                        language="json"
                        :read-only="true"
                        min-height="18rem"
                        height="min(56vh, 34rem)"
                        test-id="strategy-params-editor"
                    />
                </div>
            </div>
        </v-dialog>

        <v-dialog v-model="strategyActivityDetailDialogOpen" max-width="760">
            <div class="strategy-instance-dialog strategy-activity-detail-dialog"
                data-testid="strategy-activity-detail-dialog">
                <div class="flex flex-wrap items-start justify-between gap-4">
                    <div>
                        <div class="text-sm font-semibold uppercase tracking-[0.16em] text-slate-500">
                            {{ selectedStrategyActivityDetail?.kindLabel ?? '详情' }}
                        </div>
                        <div class="mt-1 text-xl font-semibold text-slate-900">
                            {{ selectedStrategyActivityDetail?.title ?? '活动详情' }}
                        </div>
                    </div>
                    <button type="button" class="strategy-params-dialog-close"
                        data-testid="strategy-close-activity-detail-dialog"
                        @click="closeStrategyActivityDetailDialog">
                        关闭
                    </button>
                </div>

                <div v-if="selectedStrategyActivityDetail !== null"
                    class="mt-4 grid gap-3 sm:grid-cols-2">
                    <div class="rounded-3xl bg-slate-50 px-4 py-4">
                        <div class="text-[11px] uppercase tracking-[0.18em] text-slate-500">本地时间</div>
                        <div class="mt-2 text-sm font-medium text-slate-900 strategy-time-display"
                            :title="selectedStrategyActivityDetail.utc">
                            {{ selectedStrategyActivityDetail.at }}
                        </div>
                    </div>
                    <div class="rounded-3xl bg-slate-50 px-4 py-4">
                        <div class="text-[11px] uppercase tracking-[0.18em] text-slate-500">UTC</div>
                        <div class="mt-2 text-sm font-medium text-slate-900 strategy-time-display"
                            :title="selectedStrategyActivityDetail.utc">
                            {{ selectedStrategyActivityDetail.utc }}
                        </div>
                    </div>
                    <div class="rounded-3xl bg-slate-50 px-4 py-4">
                        <div class="text-[11px] uppercase tracking-[0.18em] text-slate-500">级别</div>
                        <div class="mt-2 text-sm font-medium text-slate-900">
                            {{ formatStrategyActivityLevel(selectedStrategyActivityDetail.level) }}
                        </div>
                    </div>
                    <div v-if="selectedStrategyActivityDetail.rawKind !== undefined"
                        class="rounded-3xl bg-slate-50 px-4 py-4">
                        <div class="text-[11px] uppercase tracking-[0.18em] text-slate-500">原始类型</div>
                        <div class="mt-2 text-sm font-medium text-slate-900 break-all">
                            {{ selectedStrategyActivityDetail.rawKind }}
                        </div>
                    </div>
                </div>

                <div v-if="selectedStrategyActivityDetail !== null"
                    class="mt-4 rounded-3xl bg-slate-50 px-4 py-4">
                    <div class="text-xs font-semibold uppercase tracking-[0.16em] text-slate-500">摘要</div>
                    <div class="mt-2 text-sm text-slate-700 break-words">
                        {{ selectedStrategyActivityDetail.summary }}
                    </div>
                    <div class="mt-4 text-xs font-semibold uppercase tracking-[0.16em] text-slate-500">详情</div>
                    <div class="mt-2 whitespace-pre-wrap break-words text-sm leading-6 text-slate-700">
                        {{ selectedStrategyActivityDetail.detail }}
                    </div>
                </div>
            </div>
        </v-dialog>
    </div>
</template>

<style scoped>
.strategy-instance-dialog {
    max-height: calc(100vh - 2rem);
    overflow-y: auto;
    overflow-x: hidden;
    border-radius: 1.75rem;
    border: 1px solid var(--card-border);
    background:
        linear-gradient(
            180deg,
            color-mix(in srgb, var(--card-surface) 96%, transparent),
            var(--card-surface)
        );
    color: var(--card-text-1);
    padding: 1.25rem;
    box-shadow: 0 24px 90px rgb(2 6 23 / 0.24);
    backdrop-filter: blur(18px);
    scrollbar-gutter: stable both-edges;
}

.tv-main .strategy-activity-panel {
    border-color: var(--card-border);
    background: var(--card-surface);
}

.tv-main .strategy-activity-entry__summary {
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    color: var(--card-text-1);
}

.tv-main .strategy-activity-entry__summary--log {
    font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, "Liberation Mono", "Courier New", monospace;
    font-size: 0.78rem;
    line-height: 1.6;
}

.tv-main .strategy-activity-entry__summary--audit {
    font-size: 0.9rem;
    font-weight: 600;
    line-height: 1.5;
}

.tv-main .strategy-activity-entry__detail-trigger {
    flex-shrink: 0;
    border: 1px solid var(--card-border);
    border-radius: 999px;
    background: color-mix(in srgb, var(--tv-bg-surface) 76%, transparent);
    color: var(--card-text-2);
    padding: 0.15rem 0.55rem;
    font-size: 0.78rem;
    font-weight: 800;
    letter-spacing: 0.14em;
    cursor: pointer;
    transition: border-color 140ms ease, background-color 140ms ease, color 140ms ease, transform 140ms ease;
}

.tv-main .strategy-activity-entry__detail-trigger:hover {
    border-color: var(--card-active-border);
    background: color-mix(in srgb, var(--card-active-surface) 82%, var(--card-surface));
    color: var(--card-active-text);
    transform: translateY(-1px);
}

.tv-main .strategy-activity-entry__detail-trigger:focus-visible {
    outline: 2px solid color-mix(in srgb, var(--tv-accent) 70%, var(--card-surface));
    outline-offset: 2px;
}

.strategy-params-dialog-close {
    display: inline-flex;
    align-items: center;
    gap: 0.5rem;
    border-radius: 999px;
    border: 1px solid var(--card-border);
    background: color-mix(in srgb, var(--tv-bg-surface) 72%, transparent);
    color: var(--card-text-2);
    cursor: pointer;
    transition: border-color 140ms ease, background-color 140ms ease, color 140ms ease, transform 140ms ease;
}

.tv-main .strategy-activity-tab,
.tv-main .strategy-activity-filter,
.tv-main .strategy-runtime-params-trigger {
    border-radius: 999px;
    border: 1px solid var(--card-border);
    background: color-mix(in srgb, var(--tv-bg-surface) 72%, transparent);
    color: var(--card-text-2);
    cursor: pointer;
    transition: border-color 140ms ease, background-color 140ms ease, color 140ms ease, transform 140ms ease;
}

.tv-main .strategy-activity-tab,
.tv-main .strategy-activity-filter {
    padding: 0.5rem 0.85rem;
    font-size: 0.76rem;
    font-weight: 700;
    letter-spacing: 0.08em;
    text-transform: uppercase;
}

.tv-main .strategy-runtime-params-trigger,
.strategy-params-dialog-close {
    padding: 0.55rem 0.95rem;
    font-size: 0.8rem;
    font-weight: 700;
}

.tv-main .strategy-runtime-params-trigger {
    font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, "Liberation Mono", "Courier New", monospace;
}

.tv-main .strategy-activity-tab:hover,
.tv-main .strategy-activity-filter:hover,
.tv-main .strategy-runtime-params-trigger:hover,
.strategy-params-dialog-close:hover {
    border-color: var(--card-border);
    background: var(--card-surface-raised);
    color: var(--card-text-1);
    transform: translateY(-1px);
}

.tv-main .strategy-activity-tab.is-active {
    border-color: var(--card-active-border);
    background: color-mix(in srgb, var(--card-active-surface) 82%, var(--card-surface));
    color: var(--card-active-text);
}

.tv-main .strategy-activity-filter.is-active {
    color: var(--card-text-1);
}

.tv-main .strategy-activity-filter--error.is-active {
    border-color: var(--card-red-border);
    background: var(--card-red-surface);
    color: var(--card-red-text);
}

.tv-main .strategy-activity-filter--warning.is-active {
    border-color: var(--card-amber-border);
    background: var(--card-amber-surface);
    color: var(--card-amber-text);
}

.tv-main .strategy-activity-filter--info.is-active {
    border-color: var(--card-active-border);
    background: color-mix(in srgb, var(--card-active-surface) 80%, var(--card-surface));
    color: var(--card-active-text);
}

.tv-main .strategy-activity-tab-count,
.tv-main .strategy-activity-filter-count {
    border-radius: 999px;
    padding: 0.15rem 0.45rem;
    background: color-mix(in srgb, var(--tv-text) 7%, transparent);
    font-size: 0.72rem;
    line-height: 1;
}

.tv-main .strategy-activity-viewport {
    display: grid;
    gap: 0.75rem;
    max-height: 24rem;
    overflow-y: auto;
    padding-right: 0.35rem;
}

.tv-main .strategy-activity-entry {
    border-radius: 1.5rem;
    border: 1px solid var(--card-border);
    background: var(--card-surface-raised);
    padding: 1rem;
}

.tv-main .strategy-activity-entry--error {
    border-color: var(--card-red-border);
    background: var(--card-red-surface);
}

.tv-main .strategy-activity-entry--warning {
    border-color: var(--card-amber-border);
    background: var(--card-amber-surface);
}

.tv-main .strategy-activity-level-badge,
.tv-main .strategy-activity-kind-badge {
    display: inline-flex;
    align-items: center;
    border-radius: 999px;
    padding: 0.22rem 0.65rem;
    font-size: 0.68rem;
    font-weight: 700;
    letter-spacing: 0.12em;
    text-transform: uppercase;
}

.tv-main .strategy-activity-level-badge--error {
    border: 1px solid var(--card-red-border);
    background: color-mix(in srgb, var(--card-red-surface) 88%, transparent);
    color: var(--card-red-text);
}

.tv-main .strategy-activity-level-badge--warning {
    border: 1px solid var(--card-amber-border);
    background: color-mix(in srgb, var(--card-amber-surface) 88%, transparent);
    color: var(--card-amber-text);
}

.tv-main .strategy-activity-level-badge--info {
    border: 1px solid var(--card-active-border);
    background: color-mix(in srgb, var(--card-active-surface) 85%, transparent);
    color: var(--card-active-text);
}

.tv-main .strategy-activity-kind-badge {
    border: 1px solid var(--card-border);
    background: color-mix(in srgb, var(--tv-text) 5%, transparent);
    color: var(--card-text-2);
}

.tv-main .strategy-activity-time {
    color: var(--card-text-3);
    font-size: 0.76rem;
}

.tv-main .strategy-time-display {
    cursor: help;
}

.tv-main .strategy-activity-entry .text-slate-900,
.tv-main .strategy-activity-entry .text-slate-700 {
    color: var(--card-text-1);
}

.tv-main .strategy-activity-empty {
    border-radius: 1.5rem;
    border: 1px dashed var(--card-border);
    background: color-mix(in srgb, var(--card-surface-raised) 76%, transparent);
    padding: 1.25rem 1rem;
    color: var(--card-text-2);
    font-size: 0.92rem;
}

.strategy-params-dialog {
    border-color: var(--card-border);
    background: var(--card-surface);
}

.strategy-params-editor {
    overflow: hidden;
    border-radius: 1.25rem;
}

.strategy-activity-detail-dialog {
    border-color: var(--card-border);
    background: var(--card-surface);
}

.strategy-activity-detail-dialog .bg-slate-50 {
    background: var(--card-surface-raised);
}

.strategy-activity-detail-dialog .text-slate-900,
.strategy-activity-detail-dialog .text-slate-800,
.strategy-activity-detail-dialog .text-slate-700 {
    color: var(--card-text-1);
}

.strategy-activity-detail-dialog .text-slate-600,
.strategy-activity-detail-dialog .text-slate-500 {
    color: var(--card-text-2);
}

.strategy-activity-detail-dialog .text-slate-400 {
    color: var(--card-text-3);
}

.strategy-instance-dialog .text-slate-900,
.strategy-instance-dialog .text-slate-800,
.strategy-instance-dialog .text-slate-700 {
    color: var(--card-text-1);
}

.strategy-instance-dialog .text-slate-600,
.strategy-instance-dialog .text-slate-500 {
    color: var(--card-text-2);
}

.strategy-instance-dialog .text-slate-400 {
    color: var(--card-text-3);
}

.strategy-instance-dialog .bg-white,
.strategy-instance-dialog .bg-slate-50 {
    background: var(--card-surface-raised);
}

.strategy-instance-dialog .border-slate-200,
.strategy-instance-dialog .border-slate-300 {
    border-color: var(--card-border);
}
</style>
