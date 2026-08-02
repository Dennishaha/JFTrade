<script setup lang="ts">
import { useStockScreenerControllerContext } from "./useStockScreenerController";

const {
  pendingDraftAction,
  pendingDraftActionLabel,
  savingPreset,
  savePendingDraft,
  discardPendingDraft,
  factorDialogOpen,
  closeFactorDialog,
  factorSearchInput,
  catalogSearch,
  activeFactorRole,
  canScrollCategoriesLeft,
  canScrollCategoriesRight,
  scrollCategories,
  categoryScroller,
  updateCategoryScrollState,
  activeCategory,
  catalog,
  visibleCatalogFactors,
  hasDuplicateRef,
  filters,
  addFilter,
  columnExists,
  addColumn,
  sorts,
  addSort,
} = useStockScreenerControllerContext();
</script>

<template>
  <Teleport to="body">
    <div
      v-if="pendingDraftAction"
      class="stock-screener-view__factor-dialog-backdrop"
    >
      <section
        class="stock-screener-view__draft-dialog"
        role="dialog"
        aria-modal="true"
        aria-labelledby="stock-screener-draft-dialog-title"
      >
        <header>
          <strong id="stock-screener-draft-dialog-title">
            当前策略有未保存修改
          </strong>
          <span>{{ pendingDraftActionLabel }}前，请选择如何处理当前草稿。</span>
        </header>
        <div class="stock-screener-view__draft-dialog-actions">
          <button
            type="button"
            :disabled="savingPreset"
            @click="savePendingDraft"
          >
            {{ savingPreset ? "保存中…" : "保存后继续" }}
          </button>
          <button type="button" @click="discardPendingDraft">放弃修改</button>
          <button type="button" @click="pendingDraftAction = null">
            取消
          </button>
        </div>
      </section>
    </div>
  </Teleport>

  <Teleport to="body">
    <div
      v-if="factorDialogOpen"
      class="stock-screener-view__factor-dialog-backdrop"
      @click.self="closeFactorDialog"
      @keydown.esc.stop.prevent="closeFactorDialog"
    >
      <section
        class="stock-screener-view__factor-dialog"
        role="dialog"
        aria-modal="true"
        aria-labelledby="stock-screener-factor-dialog-title"
      >
        <header class="stock-screener-view__factor-dialog-head">
          <div>
            <strong id="stock-screener-factor-dialog-title">添加因子</strong>
            <span>选择因子后会立即插入并定位到编辑行</span>
          </div>
          <button
            type="button"
            class="stock-screener-view__factor-dialog-close"
            aria-label="关闭添加因子"
            @click="closeFactorDialog"
          >
            <svg viewBox="0 0 24 24" aria-hidden="true">
              <path d="m6 6 12 12M18 6 6 18" />
            </svg>
          </button>
        </header>

        <div class="stock-screener-view__catalog" aria-label="因子目录">
          <input
            ref="factorSearchInput"
            v-model="catalogSearch"
            type="search"
            aria-label="搜索因子"
            placeholder="搜索名称、键或说明"
          />
          <div
            class="stock-screener-view__factor-roles"
            role="tablist"
            aria-label="因子用途"
          >
            <button
              type="button"
              role="tab"
              :aria-selected="activeFactorRole === 'filter'"
              :class="{ 'is-active': activeFactorRole === 'filter' }"
              @click="activeFactorRole = 'filter'"
            >
              条件
            </button>
            <button
              type="button"
              role="tab"
              :aria-selected="activeFactorRole === 'column'"
              :class="{ 'is-active': activeFactorRole === 'column' }"
              @click="activeFactorRole = 'column'"
            >
              结果列
            </button>
            <button
              type="button"
              role="tab"
              :aria-selected="activeFactorRole === 'sort'"
              :class="{ 'is-active': activeFactorRole === 'sort' }"
              @click="activeFactorRole = 'sort'"
            >
              排序
            </button>
          </div>
          <div class="stock-screener-view__category-nav">
            <button
              type="button"
              class="stock-screener-view__category-scroll stock-screener-view__category-scroll--previous"
              aria-label="向左滚动因子分类"
              :disabled="!canScrollCategoriesLeft"
              @click="scrollCategories(-1)"
            >
              <svg viewBox="0 0 24 24" aria-hidden="true">
                <path d="m15 18-6-6 6-6" />
              </svg>
            </button>
            <div
              ref="categoryScroller"
              class="stock-screener-view__categories"
              @scroll="updateCategoryScrollState"
            >
              <button
                type="button"
                :class="{ 'is-active': activeCategory === '' }"
                @click="activeCategory = ''"
              >
                全部
              </button>
              <button
                v-for="category in catalog?.categories ?? []"
                :key="category.key"
                type="button"
                :class="{ 'is-active': activeCategory === category.key }"
                @click="activeCategory = category.key"
              >
                {{ category.label }} {{ category.count }}
              </button>
            </div>
            <button
              type="button"
              class="stock-screener-view__category-scroll stock-screener-view__category-scroll--next"
              aria-label="向右滚动因子分类"
              :disabled="!canScrollCategoriesRight"
              @click="scrollCategories(1)"
            >
              <svg viewBox="0 0 24 24" aria-hidden="true">
                <path d="m9 18 6-6-6-6" />
              </svg>
            </button>
          </div>
          <div class="stock-screener-view__factor-list">
            <article
              v-for="factor in visibleCatalogFactors"
              :key="factor.key"
              :class="{
                'is-disabled': factor.availability === 'unsupported',
                'is-experimental': factor.availability === 'experimental',
              }"
            >
              <div>
                <strong>{{ factor.label }}</strong>
                <code>{{ factor.key }}</code>
                <small v-if="factor.availability === 'experimental'">
                  实验
                </small>
                <p>
                  {{
                    factor.availability === "unsupported"
                      ? factor.reason || "当前市场不可用"
                      : `${factor.category} · ${factor.filterKind || factor.valueType}`
                  }}
                </p>
              </div>
              <span>
                <button
                  type="button"
                  :disabled="
                    !factor.filter ||
                    factor.availability === 'unsupported' ||
                    hasDuplicateRef(filters, { factor: factor.key })
                  "
                  @click="addFilter(factor)"
                >
                  条件
                </button>
                <button
                  type="button"
                  :disabled="
                    !factor.retrieve ||
                    factor.availability === 'unsupported' ||
                    columnExists(factor.key)
                  "
                  @click="addColumn(factor.key)"
                >
                  列
                </button>
                <button
                  type="button"
                  :disabled="
                    !factor.sort ||
                    factor.availability === 'unsupported' ||
                    hasDuplicateRef(sorts, { factor: factor.key })
                  "
                  @click="addSort(factor.key)"
                >
                  排序
                </button>
              </span>
            </article>
          </div>
        </div>
      </section>
    </div>
  </Teleport>
</template>

<style scoped>
.stock-screener-view__catalog {
  box-sizing: border-box;
  display: flex;
  width: 100%;
  min-width: 0;
  min-height: 0;
  flex-direction: column;
  gap: 6px;
  overflow: hidden;
  padding: 12px;
}

.stock-screener-view__category-nav {
  box-sizing: border-box;
  display: grid;
  width: 100%;
  min-width: 0;
  max-width: 100%;
  height: 32px;
  min-height: 32px;
  flex: 0 0 32px;
  grid-template-columns: 28px minmax(0, 1fr) 28px;
  align-items: center;
  gap: 4px;
}

.stock-screener-view__categories {
  display: flex;
  min-width: 0;
  align-items: center;
  gap: 4px;
  overflow-x: auto;
  overflow-y: hidden;
  overscroll-behavior-inline: contain;
  scrollbar-width: none;
  scroll-behavior: smooth;
  white-space: nowrap;
}

.stock-screener-view__categories::-webkit-scrollbar { display: none; }

.stock-screener-view__category-scroll {
  display: inline-grid;
  width: 28px;
  min-height: 28px !important;
  place-items: center;
  padding: 0 !important;
}

.stock-screener-view__category-scroll svg {
  width: 16px;
  fill: none;
  stroke: currentColor;
  stroke-linecap: round;
  stroke-linejoin: round;
  stroke-width: 2;
}

.stock-screener-view__factor-roles {
  display: flex;
  gap: 4px;
}

.stock-screener-view__factor-roles button {
  flex: 1;
  border-color: transparent;
  background: transparent;
  color: var(--tv-text-muted);
}

.stock-screener-view__factor-roles button.is-active {
  border-color: var(--tv-border);
  background: var(--tv-bg-surface-2);
  color: var(--tv-text);
}

.stock-screener-view__categories > button {
  flex: 0 0 auto;
  border-color: transparent;
  background: transparent;
  color: var(--tv-text-muted);
}

.stock-screener-view__categories > button.is-active {
  border-color: var(--tv-border);
  background: var(--tv-bg-surface-2);
  color: var(--tv-text);
}

.stock-screener-view__categories > button:last-child { margin-right: 0; }

.stock-screener-view__factor-list {
  display: grid;
  min-height: 0;
  align-content: start;
  gap: 4px;
  overflow-y: auto;
  padding-right: 2px;
}

.stock-screener-view__factor-list article {
  display: flex;
  min-width: 0;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  padding: 6px;
  border: 1px solid var(--tv-border);
  border-radius: 4px;
}

.stock-screener-view__factor-list article.is-disabled { opacity: 0.6; }

.stock-screener-view__factor-list article.is-experimental { border-color: var(--tv-status-warning-border); }

.stock-screener-view__factor-list article > div { min-width: 0; }

.stock-screener-view__factor-list code,
.stock-screener-view__factor-list small {
  margin-left: 6px;
  color: var(--tv-text-muted);
  font-size: var(--jf-text-4);
}

.stock-screener-view__factor-list p {
  margin: 2px 0 0;
  overflow: hidden;
  color: var(--tv-text-muted);
  font-size: var(--jf-text-5);
  text-overflow: ellipsis;
  white-space: nowrap;
}

.stock-screener-view__factor-list article > span {
  display: flex;
  flex: none;
  gap: 4px;
}

.stock-screener-view__factor-dialog-backdrop {
  position: fixed;
  z-index: 1200;
  inset: 0;
  display: grid;
  place-items: center;
  padding: 24px;
  background: rgb(0 0 0 / 54%);
  backdrop-filter: blur(2px);
}

.stock-screener-view__factor-dialog {
  display: grid;
  width: min(760px, calc(100vw - 48px));
  min-width: 0;
  max-height: min(680px, calc(100vh - 48px));
  grid-template-rows: auto minmax(0, 1fr);
  overflow: hidden;
  border: 1px solid var(--tv-border);
  border-radius: 8px;
  background: var(--tv-bg-elevated);
  box-shadow: 0 24px 64px rgb(0 0 0 / 42%);
  color: var(--tv-text);
  font-size: var(--jf-text-6);
}

.stock-screener-view__draft-dialog {
  display: grid;
  width: min(420px, calc(100vw - 32px));
  gap: 18px;
  padding: 18px;
  border: 1px solid var(--tv-border);
  border-radius: 8px;
  background: var(--tv-bg-elevated);
  box-shadow: 0 24px 64px rgb(0 0 0 / 42%);
  color: var(--tv-text);
}

.stock-screener-view__draft-dialog header {
  display: grid;
  gap: 6px;
}

.stock-screener-view__draft-dialog header span {
  color: var(--tv-text-muted);
  font-size: var(--jf-text-6);
}

.stock-screener-view__draft-dialog-actions {
  display: flex;
  justify-content: flex-end;
  gap: 6px;
}

.stock-screener-view__draft-dialog-actions button {
  min-height: 32px;
  padding: 0 10px;
  border: 1px solid var(--tv-border);
  border-radius: 4px;
  background: var(--tv-bg-surface);
  color: var(--tv-text);
  font: inherit;
  cursor: pointer;
}

.stock-screener-view__draft-dialog-actions button:first-child { border-color: var(--tv-accent); color: var(--tv-accent); }

.stock-screener-view__factor-dialog button,
.stock-screener-view__factor-dialog input {
  min-height: 28px;
  border: 1px solid var(--tv-border);
  border-radius: 4px;
  background: var(--tv-bg-surface);
  color: var(--tv-text);
  font: inherit;
}

.stock-screener-view__factor-dialog button {
  padding: 0 8px;
  cursor: pointer;
}

.stock-screener-view__factor-dialog button:hover:not(:disabled) {
  border-color: var(--tv-accent);
  background: var(--tv-bg-surface-2);
}

.stock-screener-view__factor-dialog button:disabled { cursor: not-allowed; opacity: 0.45; }

.stock-screener-view__factor-dialog input { min-width: 0; padding: 0 8px; }

.stock-screener-view__factor-dialog button:focus-visible,
.stock-screener-view__factor-dialog input:focus-visible {
  outline: 2px solid var(--tv-accent);
  outline-offset: 2px;
}

.stock-screener-view__factor-dialog-head {
  display: flex;
  min-height: 54px;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  padding: 8px 12px;
  border-bottom: 1px solid var(--tv-border);
}

.stock-screener-view__factor-dialog-head > div {
  display: grid;
  gap: 2px;
}

.stock-screener-view__factor-dialog-head span {
  color: var(--tv-text-muted);
  font-size: var(--jf-text-5);
}

.stock-screener-view__factor-dialog-close {
  display: inline-grid;
  width: 30px;
  flex: 0 0 auto;
  place-items: center;
  padding: 0 !important;
  border-color: transparent !important;
  background: transparent !important;
}

.stock-screener-view__factor-dialog-close svg {
  width: 18px;
  fill: none;
  stroke: currentColor;
  stroke-linecap: round;
  stroke-width: 1.8;
}

@media (max-width: 900px) {
  .stock-screener-view__factor-dialog-backdrop {
    padding: 12px;
  }

  .stock-screener-view__factor-dialog {
    width: calc(100vw - 24px);
    max-height: calc(100vh - 24px);
  }
}
</style>
