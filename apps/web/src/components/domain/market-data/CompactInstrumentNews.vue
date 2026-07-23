<script setup lang="ts">
import { computed, ref, watch } from "vue";

import {
  buildNewsItem,
  filterNewsItems,
  type NewsCategory,
} from "../../../composables/newsPresentation";
import {
  fetchProductFeature,
  type ProductFeatureResult,
} from "../../../composables/productFeatures";
import { withBrokerProvider } from "../../../composables/brokerProviderSelection";
import { useExternalLink } from "../../../composables/externalLink";
import type { QuoteWorkbenchTarget } from "./quoteWorkbench";

const props = withDefaults(
  defineProps<{
    target: QuoteWorkbenchTarget;
    queryInstrumentId?: string;
    brokerId?: string;
    active?: boolean;
    pending?: boolean;
  }>(),
  {
    queryInstrumentId: "",
    brokerId: "",
    active: false,
    pending: false,
  },
);

const emit = defineEmits<{
  selectTarget: [target: QuoteWorkbenchTarget];
}>();

const categories: ReadonlyArray<{ value: NewsCategory; label: string }> = [
  { value: "all", label: "全部" },
  { value: "news", label: "新闻" },
  { value: "notice", label: "公告" },
  { value: "rating", label: "评级" },
];
const category = ref<NewsCategory>("all");
const loading = ref(false);
const error = ref("");
const result = ref<ProductFeatureResult | null>(null);
const loadedKey = ref("");
let requestToken = 0;

const normalizedBrokerId = computed(() =>
  props.brokerId.trim().toLowerCase(),
);
const normalizedInstrumentId = computed(() =>
  props.queryInstrumentId.trim().toUpperCase(),
);
const requestKey = computed(
  () => `${normalizedBrokerId.value}:${normalizedInstrumentId.value}`,
);
const items = computed(() =>
  (result.value?.entries ?? [])
    .slice(0, 30)
    .map((entry, index) => buildNewsItem(entry, index)),
);
const visibleItems = computed(() =>
  filterNewsItems(items.value, category.value, ""),
);
const { handleExternalLinkClick } = useExternalLink();

function parseInstrumentId(
  instrumentId: string,
): { market: string; symbol: string } | null {
  const separator = instrumentId.indexOf(".");
  if (separator <= 0 || separator >= instrumentId.length - 1) return null;
  return {
    market: instrumentId.slice(0, separator),
    symbol: instrumentId.slice(separator + 1),
  };
}

async function load(refresh = false): Promise<void> {
  const key = requestKey.value;
  const parts = parseInstrumentId(normalizedInstrumentId.value);
  if (
    props.target.kind === "plate" ||
    !props.active ||
    parts == null ||
    normalizedBrokerId.value === ""
  ) {
    return;
  }
  if (!refresh && loadedKey.value === key) return;

  const token = ++requestToken;
  loading.value = true;
  error.value = "";
  const params = new URLSearchParams({
    market: parts.market,
    code: normalizedInstrumentId.value,
    operation: "search",
    pageSize: "30",
  });
  const path = withBrokerProvider(
    `/api/v1/market-data/news?${params.toString()}`,
    normalizedBrokerId.value,
  );
  try {
    const response = await fetchProductFeature(
      refresh ? `${path}&refresh=true` : path,
    );
    if (token !== requestToken) return;
    result.value = {
      ...response,
      entries: (response.entries ?? []).slice(0, 30),
    };
    loadedKey.value = key;
  } catch (cause) {
    if (token !== requestToken) return;
    error.value =
      cause instanceof Error && cause.message.trim() !== ""
        ? cause.message
        : "相关资讯加载失败";
  } finally {
    if (token === requestToken) loading.value = false;
  }
}

function selectRelated(instrumentId: string): void {
  emit("selectTarget", {
    kind: "instrument",
    instrumentId,
    name: "",
    productClass: "unknown",
  });
}

watch(
  requestKey,
  (next, previous) => {
    requestToken++;
    if (next !== previous) {
      result.value = null;
      error.value = "";
      loadedKey.value = "";
      category.value = "all";
    }
    if (props.active) void load();
  },
);
watch(
  () => props.active,
  (active) => {
    if (active) void load();
  },
  { immediate: true },
);

defineExpose({ refresh: () => load(true) });
</script>

<template>
  <section class="compact-instrument-news">
    <header class="compact-instrument-news__header">
      <div>
        <strong>相关资讯</strong>
        <small>关键词搜索</small>
      </div>
      <div
        class="compact-instrument-news__categories"
        role="tablist"
        aria-label="资讯分类"
      >
        <button
          v-for="option in categories"
          :key="option.value"
          type="button"
          role="tab"
          :aria-selected="category === option.value"
          :class="{ 'is-active': category === option.value }"
          @click="category = option.value"
        >
          {{ option.label }}
        </button>
      </div>
    </header>

    <div v-if="pending" class="compact-instrument-news__state">
      正在识别正股，识别完成后加载相关资讯…
    </div>
    <div
      v-else-if="normalizedBrokerId === ''"
      class="compact-instrument-news__state"
    >
      请选择支持资讯的数据源
    </div>
    <div
      v-else-if="normalizedInstrumentId === ''"
      class="compact-instrument-news__state"
    >
      暂未取得可查询的关联标的
    </div>
    <div
      v-else-if="loading && result == null"
      class="compact-instrument-news__state"
    >
      资讯加载中…
    </div>
    <div
      v-else-if="error && result == null"
      class="compact-instrument-news__state tv-status--warning"
    >
      {{ error }}
    </div>
    <template v-else>
      <div
        v-if="error"
        class="compact-instrument-news__notice tv-status--warning"
      >
        刷新失败：{{ error }}
      </div>
      <div
        v-for="warning in result?.warnings ?? []"
        :key="warning"
        class="compact-instrument-news__notice tv-status--warning"
      >
        {{ warning }}
      </div>
      <div
        v-for="partialError in result?.partialErrors ?? []"
        :key="`${partialError.scope}-${partialError.code}`"
        class="compact-instrument-news__notice tv-status--warning"
      >
        {{ partialError.scope }} · {{ partialError.message }}
      </div>

      <div v-if="visibleItems.length" class="compact-instrument-news__list">
        <article
          v-for="item in visibleItems"
          :key="item.key"
          class="compact-instrument-news__item"
        >
          <div class="compact-instrument-news__meta">
            <span>{{ item.categoryLabel }}</span>
            <span>{{ item.source }}</span>
            <span>{{ item.publishTime }}</span>
          </div>
          <h3>
            <a
              v-if="item.url"
              :href="item.url"
              target="_blank"
              rel="noopener noreferrer"
              @click="handleExternalLinkClick($event, item.url)"
            >
              {{ item.title }}
            </a>
            <span v-else>{{ item.title }}</span>
          </h3>
          <p v-if="item.summary">{{ item.summary }}</p>
          <img
            v-if="item.imageUrl"
            :src="item.imageUrl"
            :alt="item.title"
            loading="lazy"
          />
          <div
            v-if="item.relatedInstruments.length"
            class="compact-instrument-news__related"
          >
            <button
              v-for="instrumentId in item.relatedInstruments"
              :key="instrumentId"
              type="button"
              @click="selectRelated(instrumentId)"
            >
              {{ instrumentId }}
            </button>
          </div>
        </article>
      </div>
      <div v-else class="compact-instrument-news__state">
        暂无匹配资讯
      </div>
    </template>
  </section>
</template>

<style scoped>
.compact-instrument-news {
  min-height: 0;
  color: var(--tv-text);
}

.compact-instrument-news__header {
  display: flex;
  min-height: 50px;
  align-items: center;
  justify-content: space-between;
  gap: 10px;
  padding: 8px 14px;
  border-bottom: 1px solid var(--tv-border);
}

.compact-instrument-news__header > div:first-child {
  display: flex;
  min-width: 0;
  flex-direction: column;
}

.compact-instrument-news__header strong { font-size: 12px; }
.compact-instrument-news__header small { color: var(--tv-text-dim); font-size: 9px; }

.compact-instrument-news__categories {
  display: flex;
  align-items: center;
  gap: 2px;
}

.compact-instrument-news__categories button {
  padding: 4px 6px;
  border: 0;
  border-radius: 3px;
  background: transparent;
  color: var(--tv-text-dim);
  cursor: pointer;
  font-size: 10px;
}

.compact-instrument-news__categories button:hover,
.compact-instrument-news__categories button.is-active {
  background: var(--tv-bg-surface-2);
  color: var(--tv-text);
}

.compact-instrument-news__state {
  display: grid;
  min-height: 160px;
  place-items: center;
  padding: 20px;
  color: var(--tv-text-dim);
  text-align: center;
}

.compact-instrument-news__notice {
  padding: 7px 14px;
  border-bottom: 1px solid var(--tv-border);
  font-size: 10px;
}

.compact-instrument-news__item {
  position: relative;
  padding: 11px 14px;
  border-bottom: 1px solid var(--tv-border);
}

.compact-instrument-news__item:hover { background: var(--tv-bg-surface-2); }

.compact-instrument-news__meta {
  display: flex;
  min-width: 0;
  gap: 7px;
  color: var(--tv-text-dim);
  font-size: 9px;
}

.compact-instrument-news__meta span {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.compact-instrument-news__item h3 {
  margin: 5px 0 0;
  font-size: 12px;
  line-height: 1.45;
}

.compact-instrument-news__item h3 a {
  color: inherit;
  text-decoration: none;
}

.compact-instrument-news__item h3 a:hover { color: var(--tv-accent); }

.compact-instrument-news__item p {
  display: -webkit-box;
  margin: 5px 0 0;
  overflow: hidden;
  color: var(--tv-text-muted);
  font-size: 10px;
  -webkit-box-orient: vertical;
  -webkit-line-clamp: 3;
}

.compact-instrument-news__item img {
  width: 100%;
  max-height: 128px;
  margin-top: 8px;
  border-radius: 3px;
  object-fit: cover;
}

.compact-instrument-news__related {
  display: flex;
  flex-wrap: wrap;
  gap: 4px;
  margin-top: 7px;
}

.compact-instrument-news__related button {
  padding: 2px 5px;
  border: 1px solid var(--tv-border);
  border-radius: 3px;
  background: transparent;
  color: var(--tv-text-dim);
  cursor: pointer;
  font-size: 9px;
}

.compact-instrument-news__related button:hover {
  border-color: var(--tv-accent);
  color: var(--tv-accent);
}
</style>
