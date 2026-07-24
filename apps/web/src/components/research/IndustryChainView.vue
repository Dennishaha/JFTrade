<script setup lang="ts">
import { computed, ref, watch } from "vue";

import { useResearchFeature } from "../../composables/useResearchFeature";
import {
  formatCompactNumber,
  pickNumber,
  pickString,
} from "./researchEntry";

const props = withDefaults(
  defineProps<{
    market?: string;
    brokerId?: string;
    chainId?: string;
    plateId?: string;
  }>(),
  { market: "US", brokerId: "", chainId: "", plateId: "" },
);
const emit = defineEmits<{
  "update:chainId": [chainId: string];
  "update:plateId": [plateId: string];
  select: [entry: Record<string, unknown>];
  open: [entry: Record<string, unknown>];
}>();

function asRecords(value: unknown): Record<string, unknown>[] {
  return Array.isArray(value)
    ? value.filter(
        (item): item is Record<string, unknown> =>
          item != null && typeof item === "object" && !Array.isArray(item),
      )
    : [];
}

function chainTypeLabel(entry: Record<string, unknown>): string {
  const code = pickNumber(entry, ["chainType", "type"]);
  if (code != null) {
    return (
      {
        1: "串联型",
        2: "并列型",
        3: "上中下游型",
      }[code] ?? `类型 ${code}`
    );
  }
  return pickString(entry, ["chainType", "type"]) || "未知类型";
}

const keyword = ref("");
const selectedChainId = ref(props.chainId);
const selectedPlateId = ref(props.plateId);

watch(
  () => props.chainId,
  (value) => {
    selectedChainId.value = value;
  },
);
watch(
  () => props.plateId,
  (value) => {
    selectedPlateId.value = value;
  },
);

const list = useResearchFeature(
  () =>
    `/api/v1/research/industries?market=${encodeURIComponent(props.market)}&operation=chains&pageSize=50`,
  { brokerId: () => props.brokerId },
);
const detail = useResearchFeature(
  () =>
    selectedChainId.value
      ? `/api/v1/research/industries?market=${encodeURIComponent(props.market)}&operation=chain_detail&chainId=${encodeURIComponent(selectedChainId.value)}&pageSize=100`
      : "",
  { brokerId: () => props.brokerId },
);
const related = useResearchFeature(
  () =>
    selectedPlateId.value
      ? `/api/v1/research/industries?market=${encodeURIComponent(props.market)}&operation=chains_by_plate&plateId=${encodeURIComponent(selectedPlateId.value)}&pageSize=100`
      : "",
  { brokerId: () => props.brokerId },
);
const plateInfo = useResearchFeature(
  () =>
    selectedPlateId.value
      ? `/api/v1/research/industries?market=${encodeURIComponent(props.market)}&operation=plate&plateId=${encodeURIComponent(selectedPlateId.value)}&pageSize=1`
      : "",
  { brokerId: () => props.brokerId },
);
const plateStocks = useResearchFeature(
  () =>
    selectedPlateId.value
      ? `/api/v1/research/industries?market=${encodeURIComponent(props.market)}&operation=plate_stocks&plateId=${encodeURIComponent(selectedPlateId.value)}&pageSize=100`
      : "",
  { brokerId: () => props.brokerId },
);

const visibleChains = computed(() => {
  const value = keyword.value.trim().toLocaleLowerCase();
  if (!value) return list.entries.value;
  return list.entries.value.filter((entry) =>
    [
      pickString(entry, ["name", "chainName"]),
      pickString(entry, ["detail", "description"]),
      chainTypeLabel(entry),
    ]
      .join(" ")
      .toLocaleLowerCase()
      .includes(value),
  );
});

const selectedChain = computed(() =>
  list.entries.value.find(
    (entry) =>
      pickString(entry, ["chainId", "id"]) === selectedChainId.value,
  ),
);
const nodes = computed(() => {
  // IndustrialChainDetail 同时包含 informationList 与 nodeList。通用
  // envelope 会把第一个列表作为 entries，其余列表保留在 metadata。
  const metadataNodes = asRecords(detail.metadata.value.nodeList);
  const source =
    metadataNodes.length > 0
      ? metadataNodes
      : detail.entries.value.filter(
          (entry) => pickString(entry, ["nodeId", "plateId"]) !== "",
        );
  return [...source].sort((left, right) => {
    const layerDiff =
      (pickNumber(left, ["layerSth", "layer"]) ?? 0) -
      (pickNumber(right, ["layerSth", "layer"]) ?? 0);
    if (layerDiff !== 0) return layerDiff;
    return pickString(left, ["name"]).localeCompare(
      pickString(right, ["name"]),
      "zh-CN",
    );
  });
});
const informationLinks = computed(() => {
  const metadataLinks = asRecords(detail.metadata.value.informationList);
  if (metadataLinks.length > 0) return metadataLinks;
  return detail.entries.value.filter(
    (entry) =>
      pickString(entry, ["title"]) !== "" &&
      pickString(entry, ["url"]) !== "",
  );
});
const selectedSecurities = computed(() =>
  selectedPlateId.value
    ? plateStocks.entries.value
    : asRecords(
        selectedChain.value?.relationSecurityList ??
          selectedChain.value?.securities,
      ),
);

function selectChain(entry: Record<string, unknown>): void {
  const chainId = pickString(entry, ["chainId", "id"]);
  if (!chainId) return;
  selectedChainId.value = chainId;
  selectedPlateId.value = "";
  emit("update:chainId", chainId);
  emit("update:plateId", "");
}

function selectNode(entry: Record<string, unknown>): void {
  const plateId = pickString(entry, ["plateId"]);
  if (!plateId) return;
  selectedPlateId.value = plateId;
  emit("update:plateId", plateId);
}

function selectRelatedChain(entry: Record<string, unknown>): void {
  selectChain(entry);
}
</script>

<template>
  <section class="industry-chain">
    <aside class="industry-chain__master">
      <header>
        <strong>产业链</strong>
        <small>{{ list.total.value || list.entries.value.length }} 条</small>
      </header>
      <input
        v-model="keyword"
        type="search"
        placeholder="搜索产业链"
        aria-label="搜索产业链"
      />
      <div v-if="list.loading.value" class="industry-chain__status">加载中…</div>
      <div v-else-if="list.error.value" class="industry-chain__status">
        {{ list.error.value }}
      </div>
      <nav v-else>
        <button
          v-for="chain in visibleChains"
          :key="pickString(chain, ['chainId', 'id'])"
          type="button"
          :class="{
            'is-active':
              selectedChainId === pickString(chain, ['chainId', 'id']),
          }"
          @click="selectChain(chain)"
        >
          <span>{{ pickString(chain, ["name", "chainName"]) || "--" }}</span>
          <small>
            {{ chainTypeLabel(chain) }}
            · {{ formatCompactNumber(pickNumber(chain, ["marketCap"])) }}
            · {{ pickNumber(chain, ["stocksNum"]) ?? "--" }} 股
          </small>
        </button>
        <button
          v-if="list.hasMore.value"
          type="button"
          class="industry-chain__load-more"
          :disabled="list.loadingMore.value"
          @click="list.loadMore"
        >
          {{ list.loadingMore.value ? "加载中…" : "加载更多产业链" }}
        </button>
      </nav>
    </aside>

    <main class="industry-chain__detail">
      <div v-if="!selectedChainId" class="industry-chain__status">
        请选择产业链查看上下游结构
      </div>
      <div v-else-if="detail.loading.value" class="industry-chain__status">
        产业链详情加载中…
      </div>
      <div v-else-if="detail.error.value" class="industry-chain__status">
        {{ detail.error.value }}
      </div>
      <template v-else>
        <header class="industry-chain__detail-head">
          <div>
            <strong>{{ pickString(selectedChain ?? {}, ["name", "chainName"]) }}</strong>
            <small>{{ pickString(selectedChain ?? {}, ["detail", "description"]) }}</small>
          </div>
          <span />
          <small v-if="detail.asOf.value">更新 {{ detail.asOf.value }}</small>
          <button type="button" @click="detail.refresh">刷新</button>
        </header>

        <section class="industry-chain__nodes">
          <header>
            <strong>上下游节点</strong>
            <small>点击具备板块标识的节点查看关联产业链</small>
          </header>
          <div v-if="nodes.length === 0" class="industry-chain__status">
            暂无节点数据
          </div>
          <div v-else class="industry-chain__layers">
            <button
              v-for="node in nodes"
              :key="pickString(node, ['nodeId', 'id'])"
              type="button"
              :disabled="!pickString(node, ['plateId'])"
              :class="{
                'is-active':
                  selectedPlateId === pickString(node, ['plateId']),
              }"
              @click="selectNode(node)"
            >
              <small>第 {{ pickNumber(node, ["layerSth", "layer"]) ?? "--" }} 层</small>
              <span>{{ pickString(node, ["name", "nodeName"]) || "--" }}</span>
              <i v-if="pickString(node, ['plateId'])">
                {{ pickString(node, ["plateId"]) }}
              </i>
            </button>
          </div>
        </section>

        <section
          v-if="informationLinks.length > 0"
          class="industry-chain__information"
        >
          <header>
            <strong>产业链资讯</strong>
            <small>{{ informationLinks.length }} 条</small>
          </header>
          <div>
            <a
              v-for="link in informationLinks"
              :key="pickString(link, ['url', 'title'])"
              :href="pickString(link, ['url'])"
              target="_blank"
              rel="noopener noreferrer"
            >
              {{ pickString(link, ["title"]) || pickString(link, ["url"]) }}
            </a>
          </div>
        </section>

        <section v-if="selectedPlateId" class="industry-chain__related">
          <header>
            <strong>板块关联产业链</strong>
            <small>{{ selectedPlateId }}</small>
          </header>
          <p v-if="pickString(plateInfo.entries.value[0] ?? {}, ['summary'])">
            {{ pickString(plateInfo.entries.value[0] ?? {}, ["summary"]) }}
          </p>
          <div v-if="related.loading.value" class="industry-chain__status">加载中…</div>
          <div v-else-if="related.error.value" class="industry-chain__status">
            {{ related.error.value }}
          </div>
          <div v-else class="industry-chain__related-list">
            <button
              v-for="chain in related.entries.value"
              :key="pickString(chain, ['chainId', 'id'])"
              type="button"
              @click="selectRelatedChain(chain)"
            >
              {{ pickString(chain, ["name", "chainName"]) }}
            </button>
          </div>
        </section>

        <section
          v-if="selectedPlateId || selectedSecurities.length > 0"
          class="industry-chain__securities"
        >
          <header>
            <strong>{{ selectedPlateId ? "板块成分" : "相关证券" }}</strong>
            <small>{{ selectedSecurities.length }} 只</small>
          </header>
          <div
            v-if="selectedPlateId && plateStocks.loading.value"
            class="industry-chain__status"
          >
            板块成分加载中…
          </div>
          <div
            v-else-if="selectedPlateId && plateStocks.error.value"
            class="industry-chain__status"
          >
            {{ plateStocks.error.value }}
          </div>
          <div v-else>
            <button
              v-for="security in selectedSecurities"
              :key="
                pickString(
                  (security.security as Record<string, unknown> | undefined) ??
                    security,
                  ['instrumentId', 'code'],
                )
              "
              type="button"
              @click="emit('select', security)"
              @dblclick="emit('open', security)"
            >
              <span>{{ pickString(security, ["name"]) }}</span>
              <small>
                {{
                  pickString(
                    (security.security as Record<string, unknown> | undefined) ??
                      security,
                    ["instrumentId", "code"],
                  )
                }}
              </small>
            </button>
          </div>
        </section>
      </template>
    </main>
  </section>
</template>

<style scoped>
.industry-chain {
  display: grid;
  min-height: 0;
  height: 100%;
  grid-template-columns: minmax(210px, 280px) minmax(0, 1fr);
  gap: 8px;
  color: var(--tv-text);
  font-size: 12px;
}

.industry-chain__master,
.industry-chain__detail > section,
.industry-chain__detail-head {
  border: 1px solid var(--tv-border);
  border-radius: 6px;
  background: var(--tv-bg-surface);
}

.industry-chain__master {
  min-height: 0;
  overflow: hidden;
}

.industry-chain__master > header,
.industry-chain__detail-head,
.industry-chain__nodes > header,
.industry-chain__information > header,
.industry-chain__related > header,
.industry-chain__securities > header {
  display: flex;
  min-height: 32px;
  align-items: center;
  gap: 8px;
  padding: 0 8px;
  border-bottom: 1px solid var(--tv-border);
  background: var(--tv-bg-surface-2);
}

.industry-chain header small,
.industry-chain__master nav small {
  color: var(--tv-text-dim);
}

.industry-chain__master > header,
.industry-chain__nodes > header,
.industry-chain__information > header,
.industry-chain__related > header,
.industry-chain__securities > header {
  justify-content: space-between;
}

.industry-chain__master > input {
  width: calc(100% - 16px);
  height: 28px;
  margin: 8px;
  padding: 0 8px;
  border: 1px solid var(--tv-border);
  border-radius: 4px;
  outline: 0;
  background: var(--tv-bg-surface-2);
  color: var(--tv-text);
  font: inherit;
}

.industry-chain__master > input:focus {
  border-color: var(--tv-accent);
}

.industry-chain__master nav {
  height: calc(100% - 76px);
  overflow: auto;
}

.industry-chain__master nav button {
  display: flex;
  width: 100%;
  min-height: 44px;
  flex-direction: column;
  justify-content: center;
  padding: 5px 8px;
  border: 0;
  border-bottom: 1px solid var(--tv-border);
  background: transparent;
  color: var(--tv-text);
  cursor: pointer;
  text-align: left;
}

.industry-chain__master nav button:hover,
.industry-chain__master nav button.is-active {
  background: var(--tv-bg-elevated);
}

.industry-chain__master nav button.is-active {
  box-shadow: inset 2px 0 var(--tv-accent);
}

.industry-chain__master nav .industry-chain__load-more {
  min-height: 34px;
  align-items: center;
  color: var(--tv-text-muted);
  text-align: center;
}

.industry-chain__detail {
  display: flex;
  min-width: 0;
  min-height: 0;
  flex-direction: column;
  gap: 8px;
  overflow: auto;
}

.industry-chain__detail-head {
  flex: 0 0 auto;
}

.industry-chain__detail-head > div {
  display: flex;
  min-width: 0;
  flex-direction: column;
}

.industry-chain__detail-head > div small {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.industry-chain__detail-head > span {
  flex: 1;
}

.industry-chain button {
  color: inherit;
  font: inherit;
}

.industry-chain__detail-head button {
  height: 24px;
  padding: 0 8px;
  border: 1px solid var(--tv-border);
  border-radius: 4px;
  background: var(--tv-bg-surface-2);
  cursor: pointer;
}

.industry-chain__status {
  display: grid;
  min-height: 120px;
  flex: 1;
  place-items: center;
  color: var(--tv-text-dim);
}

.industry-chain__layers {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(150px, 1fr));
  gap: 6px;
  padding: 8px;
}

.industry-chain__layers button {
  display: flex;
  min-height: 58px;
  flex-direction: column;
  justify-content: center;
  padding: 6px 8px;
  border: 1px solid var(--tv-border);
  border-radius: 5px;
  background: color-mix(
    in srgb,
    var(--tv-accent) 7%,
    var(--tv-bg-surface-2)
  );
  cursor: pointer;
  text-align: left;
}

.industry-chain__layers button:disabled {
  cursor: default;
  opacity: 0.72;
}

.industry-chain__layers button.is-active {
  border-color: var(--tv-accent);
}

.industry-chain__layers button small,
.industry-chain__layers button i {
  color: var(--tv-text-dim);
  font-size: 10px;
  font-style: normal;
}

.industry-chain__related-list,
.industry-chain__securities > div {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  padding: 8px;
}

.industry-chain__information > div {
  display: grid;
  gap: 1px;
  padding: 6px 8px;
}

.industry-chain__information a {
  overflow: hidden;
  padding: 5px 0;
  color: var(--tv-accent);
  text-decoration: none;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.industry-chain__related > p {
  margin: 0;
  padding: 8px;
  border-bottom: 1px solid var(--tv-border);
  color: var(--tv-text-dim);
  line-height: 1.55;
}

.industry-chain__related-list button,
.industry-chain__securities button {
  display: flex;
  min-height: 30px;
  flex-direction: column;
  justify-content: center;
  padding: 3px 8px;
  border: 1px solid var(--tv-border);
  border-radius: 4px;
  background: var(--tv-bg-surface-2);
  cursor: pointer;
}

.industry-chain__related-list button:hover,
.industry-chain__securities button:hover {
  border-color: var(--tv-accent);
}

.industry-chain__securities button small {
  color: var(--tv-text-dim);
}

@media (max-width: 820px) {
  .industry-chain {
    grid-template-columns: 1fr;
    overflow: auto;
  }

  .industry-chain__master {
    max-height: 260px;
  }
}
</style>
