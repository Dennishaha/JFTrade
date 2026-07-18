<script setup lang="ts">
import { computed, ref, watch } from "vue";

import {
  buildNewsItem,
  filterNewsItems,
  type NewsCategory,
} from "../../composables/newsPresentation";
import {
  fetchProductFeature,
  type ProductFeatureResult,
} from "../../composables/productFeatures";
import {
  useBrokerProviderSelection,
  withBrokerProvider,
} from "../../composables/brokerProviderSelection";
import { useExternalLink } from "../../composables/externalLink";
import ProductPanelToolbar from "./ProductPanelToolbar.vue";
import ProductToolbarRefreshButton from "./ProductToolbarRefreshButton.vue";

const props = defineProps<{
  instrumentId: string;
  path: string;
}>();
const emit = defineEmits<{ openInstrument: [instrumentId: string] }>();

const loading = ref(false);
const error = ref("");
const result = ref<ProductFeatureResult | null>(null);
const category = ref<NewsCategory>("all");
const filter = ref("");
const { selectedBrokerId } = useBrokerProviderSelection();
const { handleExternalLinkClick } = useExternalLink();
let requestToken = 0;

const categories: Array<{ value: NewsCategory; label: string }> = [
  { value: "all", label: "全部" },
  { value: "news", label: "新闻" },
  { value: "notice", label: "公告" },
  { value: "rating", label: "评级" },
];
const items = computed(() =>
  (result.value?.entries ?? []).map((entry, index) =>
    buildNewsItem(entry, index),
  ),
);
const visibleItems = computed(() =>
  filterNewsItems(items.value, category.value, filter.value),
);
const requestPath = computed(() =>
  withBrokerProvider(props.path, selectedBrokerId.value),
);
const asOfLabel = computed(() => {
  const value = result.value?.asOf;
  if (!value) return "";
  const timestamp = Date.parse(value);
  return Number.isNaN(timestamp)
    ? value
    : new Intl.DateTimeFormat("zh-CN", {
        month: "2-digit",
        day: "2-digit",
        hour: "2-digit",
        minute: "2-digit",
      }).format(timestamp);
});

async function load(refresh = false): Promise<void> {
  const token = ++requestToken;
  if (!props.path) {
    result.value = null;
    error.value = "";
    loading.value = false;
    return;
  }
  loading.value = true;
  error.value = "";
  try {
    const path = requestPath.value;
    const separator = path.includes("?") ? "&" : "?";
    const response = await fetchProductFeature(
      refresh ? `${path}${separator}refresh=true` : path,
    );
    if (token === requestToken) result.value = response;
  } catch (cause) {
    if (token !== requestToken) return;
    error.value = cause instanceof Error ? cause.message : String(cause);
    result.value = null;
  } finally {
    if (token === requestToken) loading.value = false;
  }
}

watch(
  () => [props.path, selectedBrokerId.value] as const,
  () => void load(),
  { immediate: true },
);
</script>

<template>
  <section class="news-workspace">
    <ProductPanelToolbar title="资讯">
      <v-btn-toggle
        v-model="category"
        class="news-workspace__categories product-segmented-control"
        mandatory
        density="compact"
      >
        <v-btn v-for="item in categories" :key="item.value" :value="item.value">
          {{ item.label }}
        </v-btn>
      </v-btn-toggle>
      <v-text-field
        v-model="filter"
        class="news-workspace__filter product-compact-control"
        density="compact"
        variant="outlined"
        hide-details
        clearable
        prepend-inner-icon="fa-solid fa-magnifying-glass"
        placeholder="筛选资讯"
      />
      <span class="news-workspace__count"> {{ visibleItems.length }} 条 </span>
      <ProductToolbarRefreshButton :loading="loading" @refresh="load(true)" />
    </ProductPanelToolbar>

    <v-progress-linear v-if="loading" indeterminate />
    <v-alert v-if="error" type="warning" variant="tonal" density="compact">
      {{ error }}
    </v-alert>
    <template v-else-if="result">
      <v-alert
        v-for="warning in result.warnings ?? []"
        :key="warning"
        type="warning"
        variant="tonal"
        density="compact"
      >
        {{ warning }}
      </v-alert>
      <v-alert
        v-for="partialError in result.partialErrors ?? []"
        :key="`${partialError.scope}-${partialError.code}`"
        type="warning"
        variant="outlined"
        density="compact"
      >
        {{ partialError.scope }} · {{ partialError.message }}
      </v-alert>

      <div v-if="visibleItems.length" class="news-workspace__list">
        <article
          v-for="item in visibleItems"
          :key="item.key"
          class="news-workspace__item"
        >
          <div class="news-workspace__body">
            <div class="news-workspace__meta">
              <span :class="`is-${item.category}`">
                {{ item.categoryLabel }}
              </span>
              <span>{{ item.source }}</span>
              <span>{{ item.publishTime }}</span>
              <span v-if="item.viewCount">{{ item.viewCount }}</span>
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
            <div
              v-if="item.relatedInstruments.length"
              class="news-workspace__related"
            >
              <button
                v-for="instrument in item.relatedInstruments"
                :key="instrument"
                type="button"
                @click="emit('openInstrument', instrument)"
              >
                {{ instrument }}
              </button>
            </div>
          </div>
          <img
            v-if="item.imageUrl"
            :src="item.imageUrl"
            :alt="item.title"
            loading="lazy"
          />
        </article>
      </div>
      <div v-else class="news-workspace__empty">
        <strong>暂无匹配资讯</strong>
        <span>可切换分类、清除筛选或刷新后重试。</span>
      </div>
      <footer v-if="asOfLabel" class="news-workspace__footer">
        更新于 {{ asOfLabel }}
      </footer>
    </template>
  </section>
</template>

<style scoped>
.news-workspace {
  display: flex;
  height: 100%;
  min-height: 0;
  flex-direction: column;
  overflow: hidden;
  background: var(--tv-bg-surface);
  color: var(--tv-text);
}

.news-workspace__categories :deep(.v-btn) {
  min-width: 43px;
  height: 26px;
  padding-inline: 8px;
  border-radius: 4px !important;
  color: var(--tv-text-muted);
  font-size: 9px;
  letter-spacing: 0;
  text-transform: none;
}

.news-workspace__categories :deep(.v-btn--active) {
  background: color-mix(in srgb, var(--tv-accent) 14%, var(--tv-bg-surface));
  color: var(--tv-text);
}

.news-workspace__filter {
  max-width: 184px;
  flex: 0 1 184px;
}

.news-workspace__count,
.news-workspace__footer {
  color: var(--tv-text-dim);
  font-size: 9px;
  white-space: nowrap;
}

.news-workspace__list {
  min-height: 0;
  flex: 1;
  overflow: auto;
}

.news-workspace__item {
  display: flex;
  gap: 11px;
  padding: 9px 15px;
  transition: background 120ms ease;
}

.news-workspace__item:hover {
  background: var(--tv-bg-surface-2);
}

.news-workspace__body {
  min-width: 0;
  flex: 1;
}

.news-workspace__meta {
  display: flex;
  align-items: center;
  gap: 7px;
  color: var(--tv-text-dim);
  font-size: 9px;
}

.news-workspace__meta > span:first-child {
  padding: 1px 5px;
  border-radius: 3px;
  background: color-mix(in srgb, var(--tv-text) 6%, var(--tv-bg-surface));
  color: var(--tv-text-muted);
}

.news-workspace__meta > span.is-news {
  color: var(--tv-accent);
}

.news-workspace__meta > span.is-notice {
  color: var(--tv-warn);
}

.news-workspace__meta > span.is-rating {
  color: var(--tv-accent);
}

.news-workspace__item h3 {
  margin: 3px 0 0;
  font-size: 13px;
  font-weight: 650;
  line-height: 1.38;
}

.news-workspace__item h3 a {
  color: inherit;
  text-decoration: none;
}

.news-workspace__item h3 a:hover {
  color: var(--tv-accent);
}

.news-workspace__item p {
  display: -webkit-box;
  margin: 2px 0 0;
  overflow: hidden;
  color: var(--tv-text-muted);
  font-size: 10px;
  line-height: 1.5;
  -webkit-box-orient: vertical;
  -webkit-line-clamp: 2;
}

.news-workspace__related {
  display: flex;
  gap: 5px;
  margin-top: 4px;
}

.news-workspace__related button {
  padding: 2px 6px;
  border: 1px solid var(--tv-border);
  border-radius: 4px;
  background: transparent;
  color: var(--tv-text-dim);
  cursor: pointer;
  font-size: 8px;
}

.news-workspace__related button:hover {
  border-color: var(--tv-accent);
  color: var(--tv-accent);
}

.news-workspace__item img {
  width: 104px;
  height: 66px;
  flex: 0 0 auto;
  border-radius: 5px;
  object-fit: cover;
}

.news-workspace__empty {
  display: grid;
  min-height: 180px;
  flex: 1;
  place-content: center;
  justify-items: center;
  gap: 4px;
  color: var(--tv-text-muted);
  font-size: 9px;
}

.news-workspace__empty strong {
  color: var(--tv-text);
  font-size: 11px;
}

.news-workspace__footer {
  padding: 5px 12px;
  border-top: 1px solid var(--tv-border);
  text-align: right;
}

@media (max-width: 760px) {
  .news-workspace__filter {
    max-width: 138px;
  }

  .news-workspace__item {
    padding-block: 8px;
    padding-inline: 11px;
  }

  .news-workspace__item img {
    width: 82px;
    height: 56px;
  }
}
</style>
