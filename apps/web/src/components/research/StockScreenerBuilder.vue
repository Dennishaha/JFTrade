<script setup lang="ts">
import StockScreenParameterEditor from "./StockScreenParameterEditor.vue";
import { factorRefKey } from "./stockScreenModel";
import { useStockScreenerControllerContext } from "./useStockScreenerController";

const {
  mobilePane,
  filters,
  addFactorButton,
  factorDialogOpen,
  openFactorDialog,
  commonFactors,
  hasDuplicateRef,
  addFilter,
  factorFor,
  useIntervalFilter,
  useSetFilter,
  removeFilter,
  enumOptionsForFactor,
  singleValueInput,
  valuesInput,
  catalog,
  secondFactorInput,
  boundaryInput,
  fieldErrorWithin,
  validationErrors,
  columns,
  columnIdentity,
  moveColumn,
  removeColumn,
  retrievableFactors,
  columnExists,
  addColumn,
  sorts,
  addSort,
  sortIdentity,
  sortableFactors,
  sortFactorInput,
} = useStockScreenerControllerContext();

function addSelectedColumn(event: Event): void {
  const target = event.target as HTMLSelectElement;
  void addColumn(target.value);
  target.value = "";
}
</script>

<template>
  <aside
    class="stock-screener-view__builder"
    :class="{ 'is-mobile-hidden': mobilePane !== 'builder' }"
  >
    <div class="stock-screener-view__panel-head">
      <strong>筛选条件</strong>
      <span>{{ filters.length }}</span>
      <button
        ref="addFactorButton"
        type="button"
        class="stock-screener-view__add-factor"
        aria-haspopup="dialog"
        :aria-expanded="factorDialogOpen"
        @click="openFactorDialog"
      >
        添加因子
      </button>
    </div>
    <div class="stock-screener-view__divider" />

    <div v-if="commonFactors.length" class="stock-screener-view__common">
      <span>常用</span>
      <button
        v-for="factor in commonFactors"
        :key="factor.key"
        type="button"
        :disabled="
          factor.availability === 'unsupported' ||
          hasDuplicateRef(filters, { factor: factor.key })
        "
        :title="
          factor.reason ||
          (hasDuplicateRef(filters, { factor: factor.key })
            ? '已存在相同参数'
            : undefined)
        "
        @click="addFilter(factor)"
      >
        + {{ factor.label }}
      </button>
    </div>

    <div v-if="filters.length === 0" class="stock-screener-view__empty-small">
      添加条件后执行；恢复预设不会自动请求。
    </div>
    <div
      v-for="filter in filters"
      :key="filter.id"
      :data-filter-id="filter.id"
      class="stock-screener-view__condition"
    >
      <div class="stock-screener-view__condition-title">
        <strong>{{ factorFor(filter.factor)?.label ?? filter.factor }}</strong>
        <span
          v-if="factorFor(filter.factor)?.filterKind === 'interval_or_set'"
          class="tv-seg"
        >
          <button
            type="button"
            :class="{ 'is-active': filter.values == null }"
            @click="useIntervalFilter(filter)"
          >
            区间
          </button>
          <button
            type="button"
            :class="{ 'is-active': filter.values != null }"
            @click="useSetFilter(filter)"
          >
            集合
          </button>
        </span>
        <button type="button" @click="removeFilter(filter.id)">移除</button>
      </div>
      <div
        class="stock-screener-view__condition-fields"
        :class="{
          'stock-screener-view__condition-fields--range':
            factorFor(filter.factor)?.filterKind === 'interval' ||
            (factorFor(filter.factor)?.filterKind === 'interval_or_set' &&
              filter.values == null),
        }"
      >
        <template
          v-if="
            ['enum', 'set'].includes(
              factorFor(filter.factor)?.filterKind ?? '',
            ) ||
            (factorFor(filter.factor)?.filterKind === 'interval_or_set' &&
              filter.values != null)
          "
        >
          <select
            v-if="enumOptionsForFactor(factorFor(filter.factor)).length"
            :value="filter.values?.[0]"
            aria-label="枚举条件值"
            @change="singleValueInput(filter, $event)"
          >
            <option
              v-for="option in enumOptionsForFactor(factorFor(filter.factor))"
              :key="option.key"
              :value="option.value"
            >
              {{ option.label }}
            </option>
          </select>
          <input
            v-else
            :value="filter.values?.join(',')"
            aria-label="集合条件值"
            placeholder="整数值，逗号分隔"
            @input="valuesInput(filter, $event)"
          />
        </template>
        <template
          v-else-if="factorFor(filter.factor)?.filterKind === 'position'"
        >
          <select v-model.number="filter.position" aria-label="位置关系">
            <option
              v-for="option in catalog?.enums.position ?? []"
              :key="option.key"
              :value="option.value"
            >
              {{ option.label }}
            </option>
          </select>
          <select
            :value="filter.secondFactor?.factor ?? ''"
            aria-label="比较指标"
            @change="secondFactorInput(filter, $event)"
          >
            <option value="">固定值</option>
            <option
              v-for="factor in
                catalog?.factors.filter(
                  (item) =>
                    item.category === 'indicator' &&
                    item.availability !== 'unsupported',
                ) ?? []"
              :key="factor.key"
              :value="factor.key"
            >
              {{ factor.label }}
            </option>
          </select>
          <input
            v-if="!filter.secondFactor"
            v-model.number="filter.secondValue"
            type="number"
            aria-label="比较值"
            placeholder="比较值"
          />
        </template>
        <template
          v-else-if="factorFor(filter.factor)?.filterKind === 'pattern'"
        >
          <select v-model="filter.match" aria-label="形态匹配">
            <option :value="true">匹配</option>
            <option :value="false">不匹配</option>
          </select>
          <input
            :value="filter.values?.join(',')"
            aria-label="子形态"
            placeholder="子形态值，逗号分隔"
            @input="valuesInput(filter, $event)"
          />
        </template>
        <template v-else>
          <input
            type="number"
            :value="filter.min?.value"
            aria-label="条件下限"
            placeholder="最小值"
            @input="boundaryInput(filter, $event, 'min')"
          />
          <span>至</span>
          <input
            type="number"
            :value="filter.max?.value"
            aria-label="条件上限"
            placeholder="最大值"
            @input="boundaryInput(filter, $event, 'max')"
          />
        </template>
        <label
          v-if="
            ['cumulative', 'financial', 'indicator', 'pattern'].includes(
              factorFor(filter.factor)?.category ?? '',
            )
          "
        >
          连续
          <input
            v-model.number="filter.continuousPeriod"
            type="number"
            min="0"
            aria-label="连续周期"
          />
        </label>
      </div>
      <small
        v-if="fieldErrorWithin(`conditions.${filters.indexOf(filter)}`)"
        class="stock-screener-view__field-error"
      >
        {{ fieldErrorWithin(`conditions.${filters.indexOf(filter)}`) }}
      </small>
      <div
        v-if="factorFor(filter.factor)?.parameters?.length"
        class="stock-screener-view__parameters"
      >
        <StockScreenParameterEditor
          :reference="filter"
          :parameters="factorFor(filter.factor)?.parameters ?? []"
          :enums="catalog?.enums ?? {}"
          :error-prefix="`conditions.${filters.indexOf(filter)}`"
          :validation-errors="validationErrors"
        />
      </div>
      <div
        v-if="
          filter.secondFactor &&
          factorFor(factorRefKey(filter.secondFactor))?.parameters?.length
        "
        class="stock-screener-view__parameters stock-screener-view__parameters--secondary"
      >
        <StockScreenParameterEditor
          :reference="filter.secondFactor"
          :parameters="
            factorFor(factorRefKey(filter.secondFactor))?.parameters ?? []
          "
          :enums="catalog?.enums ?? {}"
          label-prefix="比较 "
          :error-prefix="`conditions.${filters.indexOf(filter)}.secondFactor`"
          :validation-errors="validationErrors"
        />
      </div>
    </div>

    <div class="stock-screener-view__panel-head">
      <strong>结果列</strong>
      <span>{{ columns.length }}</span>
    </div>
    <div class="stock-screener-view__column-picker">
      <div
        v-for="(column, index) in columns"
        :key="columnIdentity(column, index)"
        :data-column-id="columnIdentity(column, index)"
      >
        <span>{{
          factorFor(factorRefKey(column))?.label ?? factorRefKey(column)
        }}</span>
        <button
          type="button"
          :disabled="index === 0"
          aria-label="上移结果列"
          @click="moveColumn(index, -1)"
        >
          ↑
        </button>
        <button
          type="button"
          :disabled="index === columns.length - 1"
          aria-label="下移结果列"
          @click="moveColumn(index, 1)"
        >
          ↓
        </button>
        <button type="button" @click="removeColumn(column)">X</button>
        <div
          v-if="factorFor(factorRefKey(column))?.parameters?.length"
          class="stock-screener-view__parameters stock-screener-view__parameters--compact"
        >
          <StockScreenParameterEditor
            :reference="column"
            :parameters="factorFor(factorRefKey(column))?.parameters ?? []"
            :enums="catalog?.enums ?? {}"
            :error-prefix="`columns.${index}`"
            :validation-errors="validationErrors"
            compact
          />
        </div>
        <small
          v-if="fieldErrorWithin(`columns.${index}`)"
          class="stock-screener-view__field-error"
        >
          {{ fieldErrorWithin(`columns.${index}`) }}
        </small>
      </div>
      <label>
        <span>添加列</span>
        <select aria-label="添加结果列" @change="addSelectedColumn">
          <option value="">选择因子</option>
          <option
            v-for="factor in retrievableFactors"
            :key="factor.key"
            :value="factor.key"
            :disabled="columnExists(factor.key)"
          >
            {{ factor.label }}
          </option>
        </select>
      </label>
    </div>

    <div class="stock-screener-view__panel-head">
      <strong>多字段排序</strong>
      <span>{{ sorts.length }}</span>
      <button type="button" @click="addSort()">添加排序</button>
    </div>
    <div class="stock-screener-view__divider" />
    <div class="stock-screener-view__sorts">
      <div
        v-for="(sort, index) in sorts"
        :key="sortIdentity(sort, index)"
        :data-sort-id="sortIdentity(sort, index)"
      >
        <select
          :value="factorRefKey(sort)"
          aria-label="排序字段"
          @change="sortFactorInput(sort, $event)"
        >
          <option
            v-for="factor in sortableFactors"
            :key="factor.key"
            :value="factor.key"
          >
            {{ factor.label }}
          </option>
        </select>
        <select v-model="sort.direction" aria-label="排序方向">
          <option value="desc">降序</option>
          <option value="asc">升序</option>
          <option value="abs_desc">绝对值降序</option>
          <option value="abs_asc">绝对值升序</option>
        </select>
        <button type="button" @click="sorts.splice(index, 1)">×</button>
        <div
          v-if="factorFor(factorRefKey(sort))?.parameters?.length"
          class="stock-screener-view__parameters stock-screener-view__parameters--compact"
        >
          <StockScreenParameterEditor
            :reference="sort"
            :parameters="factorFor(factorRefKey(sort))?.parameters ?? []"
            :enums="catalog?.enums ?? {}"
            :error-prefix="`sorts.${index}`"
            :validation-errors="validationErrors"
            compact
          />
        </div>
        <small
          v-if="fieldErrorWithin(`sorts.${index}`)"
          class="stock-screener-view__field-error"
        >
          {{ fieldErrorWithin(`sorts.${index}`) }}
        </small>
      </div>
    </div>
  </aside>
</template>

<style scoped>
.stock-screener-view__builder {
  display: grid;
  align-content: start;
  gap: 8px;
  overflow: auto;
  padding: 8px;
}

.stock-screener-view__panel-head {
  display: flex;
  min-width: 0;
  min-height: 32px;
  align-items: center;
  gap: 8px;
}

.stock-screener-view__panel-head > span {
  color: var(--tv-text-muted);
  font-size: 11px;
}

.stock-screener-view__panel-head > button {
  margin-left: auto;
}

.stock-screener-view__divider {
  border-bottom: 1px solid var(--tv-border);
}

.stock-screener-view__add-factor {
  display: inline-flex;
  min-height: 28px;
  align-self: center;
  align-items: center;
  justify-content: center;
  line-height: 1;
}

.stock-screener-view__common {
  display: flex;
  flex-wrap: wrap;
  gap: 4px;
}

.stock-screener-view__common > span {
  align-self: center;
  margin-right: 4px;
  color: var(--tv-text-muted);
  font-size: 11px;
}

.stock-screener-view__condition {
  display: grid;
  min-width: 0;
  gap: 6px;
  padding: 7px;
  border: 1px solid var(--tv-border);
  border-radius: 6px;
  background: var(--tv-bg-surface-2);
}

.stock-screener-view__field-error {
  display: block;
  margin-top: 4px;
  color: #d55353;
  font-size: 11px;
}

.stock-screener-view__condition-title {
  display: flex;
  min-width: 0;
  align-items: center;
  justify-content: space-between;
  gap: 4px;
}

.stock-screener-view__condition-title > strong {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.stock-screener-view__condition-title button {
  min-height: 24px;
  border: 0;
  color: var(--tv-text-muted);
}

.stock-screener-view__condition-fields {
  display: grid;
  min-width: 0;
  grid-template-columns: minmax(0, 0.7fr) minmax(0, 1fr) auto minmax(0, 1fr);
  align-items: center;
  gap: 4px;
}

.stock-screener-view__condition-fields--range {
  grid-template-columns: minmax(0, 1fr) auto minmax(0, 1fr) auto;
}

.stock-screener-view__parameters {
  display: flex;
  min-width: 0;
  max-width: 100%;
  flex-wrap: wrap;
  gap: 6px;
}

.stock-screener-view__parameters > :deep(.stock-screen-parameter-editor) {
  flex: 1 1 100%;
}

.stock-screener-view__parameters label {
  display: grid;
  min-width: 0;
  flex: 1 1 110px;
  gap: 2px;
}

.stock-screener-view__parameters span {
  color: var(--tv-text-muted);
  font-size: 10px;
}

.stock-screener-view__column-picker,
.stock-screener-view__sorts {
  display: grid;
  min-width: 0;
  gap: 4px;
}

.stock-screener-view__column-picker > div,
.stock-screener-view__sorts > div,
.stock-screener-view__column-picker > label {
  display: flex;
  min-width: 0;
  flex-wrap: wrap;
  align-items: center;
  gap: 4px;
}

.stock-screener-view__column-picker > div > span {
  min-width: 0;
  flex: 1 1 80px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.stock-screener-view__column-picker button {
  min-height: 24px;
  padding: 0 6px;
}

.stock-screener-view__column-picker > label > select,
.stock-screener-view__sorts select {
  min-width: 0;
  flex: 1 1 96px;
}

.stock-screener-view__empty-small {
  display: grid;
  min-width: 0;
  min-height: 64px;
  place-items: center;
  border: 1px dashed var(--tv-border);
  border-radius: 6px;
  color: var(--tv-text-dim);
}

@media (max-width: 768px) {
  .stock-screener-view__builder {
    overflow: visible;
  }
}
</style>
