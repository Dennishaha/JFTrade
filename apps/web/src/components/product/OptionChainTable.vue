<script setup lang="ts">
import type {
  OptionChainRowModel,
  OptionChainSideModel,
} from "../../composables/optionChainModel";
import { formatOptionMetric } from "../../composables/optionChainModel";
import type {
  OptionComboLegDraft,
  OptionComboSide,
} from "../../composables/optionComboDraft";

const props = defineProps<{
  rows: OptionChainRowModel[];
  underlyingInstrumentId: string;
  underlyingPrice: number | null;
  selectedLegs: OptionComboLegDraft[];
}>();

const emit = defineEmits<{
  openContract: [contract: OptionChainSideModel];
  selectLeg: [contract: OptionChainSideModel, side: OptionComboSide];
}>();

function openContract(side: OptionChainSideModel): void {
  if (side.instrumentId) emit("openContract", side);
}

function selectedLeg(
  contract: OptionChainSideModel,
): { sequence: number; side: OptionComboSide } | null {
  const instrumentId = contract.instrumentId.trim().toUpperCase();
  if (!instrumentId) return null;
  const index = props.selectedLegs.findIndex(
    (leg) => leg.instrumentId.trim().toUpperCase() === instrumentId,
  );
  const leg = props.selectedLegs[index];
  return leg == null ? null : { sequence: index + 1, side: leg.side };
}

function selectLeg(
  contract: OptionChainSideModel,
  side: OptionComboSide,
): void {
  if (!contract.instrumentId) return;
  emit("selectLeg", contract, side);
}

function quoteAvailable(value: number | null): boolean {
  return value != null && Number.isFinite(value);
}
</script>

<template>
  <div class="option-chain-table">
    <div class="option-chain-table__groups">
      <div class="option-chain-table__group option-chain-table__group--call">
        <strong>CALL</strong>
        <span>看涨</span>
      </div>
      <div class="option-chain-table__underlying">
        <span>{{ underlyingInstrumentId }}</span>
        <strong>{{ formatOptionMetric(underlyingPrice) }}</strong>
      </div>
      <div class="option-chain-table__group option-chain-table__group--put">
        <strong>PUT</strong>
        <span>看跌</span>
      </div>
    </div>

    <div class="option-chain-table__scroll tv-scrollbar">
      <table>
        <thead>
          <tr>
            <th class="is-contract">合约</th>
            <th>Delta</th>
            <th>IV</th>
            <th class="is-quote">买价</th>
            <th class="is-quote">卖价</th>
            <th class="is-strike">行权价</th>
            <th class="is-quote">买价</th>
            <th class="is-quote">卖价</th>
            <th>IV</th>
            <th>Delta</th>
            <th class="is-contract">合约</th>
          </tr>
        </thead>
        <tbody>
          <tr
            v-for="row in rows"
            :key="row.key"
            :class="{ 'is-atm': row.isAtm }"
          >
            <td
              class="is-contract is-call"
              :class="{ 'is-itm': row.call.moneyness === 'ITM' }"
            >
              <button
                type="button"
                :disabled="!row.call.instrumentId"
                :title="row.call.name"
                @click="openContract(row.call)"
              >
                {{ row.call.code || "—" }}
              </button>
              <small v-if="row.call.moneyness">
                {{ row.call.moneyness }}
              </small>
            </td>
            <td>{{ formatOptionMetric(row.call.delta) }}</td>
            <td>{{ formatOptionMetric(row.call.impliedVolatility, 2) }}</td>
            <td class="is-quote is-bid">
              <button
                class="option-chain-table__quote-button"
                :class="{
                  'is-selected':
                    selectedLeg(row.call)?.side === 'SELL',
                  'is-opposite-selected':
                    selectedLeg(row.call)?.side === 'BUY',
                }"
                type="button"
                :disabled="
                  !row.call.instrumentId || !quoteAvailable(row.call.bidPrice)
                "
                :aria-label="`${row.call.code || '看涨合约'} 买价，加入卖出腿`"
                @click="selectLeg(row.call, 'SELL')"
              >
                <span
                  v-if="selectedLeg(row.call)?.side === 'SELL'"
                  class="option-chain-table__leg-badge"
                >
                  {{ selectedLeg(row.call)?.sequence }} 卖
                </span>
                <span>{{ formatOptionMetric(row.call.bidPrice) }}</span>
              </button>
            </td>
            <td class="is-quote is-ask">
              <button
                class="option-chain-table__quote-button"
                :class="{
                  'is-selected':
                    selectedLeg(row.call)?.side === 'BUY',
                  'is-opposite-selected':
                    selectedLeg(row.call)?.side === 'SELL',
                }"
                type="button"
                :disabled="
                  !row.call.instrumentId || !quoteAvailable(row.call.askPrice)
                "
                :aria-label="`${row.call.code || '看涨合约'} 卖价，加入买入腿`"
                @click="selectLeg(row.call, 'BUY')"
              >
                <span
                  v-if="selectedLeg(row.call)?.side === 'BUY'"
                  class="option-chain-table__leg-badge"
                >
                  {{ selectedLeg(row.call)?.sequence }} 买
                </span>
                <span>{{ formatOptionMetric(row.call.askPrice) }}</span>
              </button>
            </td>
            <td class="is-strike">
              <span>{{ formatOptionMetric(row.strike) }}</span>
              <small v-if="row.isAtm">ATM</small>
            </td>
            <td class="is-quote is-bid">
              <button
                class="option-chain-table__quote-button"
                :class="{
                  'is-selected':
                    selectedLeg(row.put)?.side === 'SELL',
                  'is-opposite-selected':
                    selectedLeg(row.put)?.side === 'BUY',
                }"
                type="button"
                :disabled="
                  !row.put.instrumentId || !quoteAvailable(row.put.bidPrice)
                "
                :aria-label="`${row.put.code || '看跌合约'} 买价，加入卖出腿`"
                @click="selectLeg(row.put, 'SELL')"
              >
                <span
                  v-if="selectedLeg(row.put)?.side === 'SELL'"
                  class="option-chain-table__leg-badge"
                >
                  {{ selectedLeg(row.put)?.sequence }} 卖
                </span>
                <span>{{ formatOptionMetric(row.put.bidPrice) }}</span>
              </button>
            </td>
            <td class="is-quote is-ask">
              <button
                class="option-chain-table__quote-button"
                :class="{
                  'is-selected':
                    selectedLeg(row.put)?.side === 'BUY',
                  'is-opposite-selected':
                    selectedLeg(row.put)?.side === 'SELL',
                }"
                type="button"
                :disabled="
                  !row.put.instrumentId || !quoteAvailable(row.put.askPrice)
                "
                :aria-label="`${row.put.code || '看跌合约'} 卖价，加入买入腿`"
                @click="selectLeg(row.put, 'BUY')"
              >
                <span
                  v-if="selectedLeg(row.put)?.side === 'BUY'"
                  class="option-chain-table__leg-badge"
                >
                  {{ selectedLeg(row.put)?.sequence }} 买
                </span>
                <span>{{ formatOptionMetric(row.put.askPrice) }}</span>
              </button>
            </td>
            <td>{{ formatOptionMetric(row.put.impliedVolatility, 2) }}</td>
            <td>{{ formatOptionMetric(row.put.delta) }}</td>
            <td
              class="is-contract is-put"
              :class="{ 'is-itm': row.put.moneyness === 'ITM' }"
            >
              <button
                type="button"
                :disabled="!row.put.instrumentId"
                :title="row.put.name"
                @click="openContract(row.put)"
              >
                {{ row.put.code || "—" }}
              </button>
              <small v-if="row.put.moneyness">
                {{ row.put.moneyness }}
              </small>
            </td>
          </tr>
        </tbody>
      </table>
    </div>
  </div>
</template>

<style scoped>
.option-chain-table {
  min-width: 860px;
  border-top: 1px solid var(--tv-border);
  border-bottom: 1px solid var(--tv-border);
  background: var(--tv-bg-surface);
}

.option-chain-table__groups {
  display: grid;
  grid-template-columns: minmax(0, 1fr) 150px minmax(0, 1fr);
  min-height: 42px;
  border-bottom: 1px solid var(--tv-border);
  background: var(--tv-bg-surface-2);
}

.option-chain-table__group,
.option-chain-table__underlying {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 8px;
}

.option-chain-table__group strong {
  font-size: 12px;
  letter-spacing: 0.08em;
}

.option-chain-table__group span,
.option-chain-table__underlying span {
  color: var(--tv-text-dim);
  font-size: 9px;
}

.option-chain-table__group--call strong {
  color: var(--tv-price-up);
}

.option-chain-table__group--put strong {
  color: var(--tv-price-down);
}

.option-chain-table__underlying {
  flex-direction: column;
  gap: 0;
  border-right: 1px solid var(--tv-border);
  border-left: 1px solid var(--tv-border);
}

.option-chain-table__underlying strong {
  color: var(--tv-accent);
  font-size: 12px;
}

.option-chain-table__scroll {
  overflow: auto;
}

table {
  width: 100%;
  border-collapse: separate;
  border-spacing: 0;
  color: var(--tv-text);
  font-size: 10px;
  font-variant-numeric: tabular-nums;
}

th {
  position: sticky;
  top: 0;
  z-index: 2;
  height: 32px;
  padding: 0 8px;
  border-bottom: 1px solid var(--tv-border);
  background: color-mix(in srgb, var(--tv-bg-surface-2) 94%, transparent);
  color: var(--tv-text-dim);
  font-size: 9px;
  font-weight: 650;
  text-align: right;
}

td {
  height: 38px;
  padding: 3px 8px;
  border-bottom: 1px solid color-mix(in srgb, var(--tv-border) 72%, transparent);
  text-align: right;
}

tbody tr:hover td {
  background-color: color-mix(in srgb, var(--tv-accent) 5%, transparent);
}

.is-contract {
  width: 122px;
  text-align: left;
}

.is-contract.is-put {
  text-align: right;
}

.is-contract button {
  max-width: 92px;
  overflow: hidden;
  border: 0;
  background: transparent;
  color: var(--tv-text-muted);
  cursor: pointer;
  font: inherit;
  font-weight: 650;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.is-contract button:hover {
  color: var(--tv-accent);
}

.is-contract button:disabled {
  cursor: default;
  opacity: 0.48;
}

.is-contract small {
  margin-left: 5px;
  color: var(--tv-text-dim);
  font-size: 8px;
}

.is-contract.is-itm {
  background: color-mix(in srgb, var(--tv-accent) 5%, transparent);
}

.is-contract.is-itm small {
  color: var(--tv-accent);
}

.is-quote {
  width: 70px;
  font-weight: 650;
  padding: 2px 4px;
}

td.is-bid {
  background: color-mix(in srgb, var(--tv-price-up) 11%, transparent);
  color: color-mix(in srgb, var(--tv-price-up) 80%, var(--tv-text));
}

td.is-ask {
  background: color-mix(in srgb, var(--tv-price-down) 10%, transparent);
  color: color-mix(in srgb, var(--tv-price-down) 78%, var(--tv-text));
}

.option-chain-table__quote-button {
  position: relative;
  display: inline-flex;
  min-width: 62px;
  height: 30px;
  align-items: center;
  justify-content: center;
  border: 1px solid transparent;
  border-radius: 4px;
  background: transparent;
  color: inherit;
  cursor: pointer;
  font: inherit;
  font-variant-numeric: tabular-nums;
}

.option-chain-table__quote-button:hover:not(:disabled),
.option-chain-table__quote-button:focus-visible {
  border-color: color-mix(in srgb, var(--tv-accent) 72%, transparent);
  outline: none;
}

.option-chain-table__quote-button.is-selected {
  border-color: var(--tv-accent);
  background: color-mix(in srgb, var(--tv-accent) 17%, transparent);
  box-shadow: inset 0 0 0 1px
    color-mix(in srgb, var(--tv-accent) 25%, transparent);
}

.option-chain-table__quote-button.is-opposite-selected {
  opacity: 0.7;
}

.option-chain-table__quote-button:disabled {
  cursor: not-allowed;
  opacity: 0.42;
}

.option-chain-table__leg-badge {
  position: absolute;
  top: -6px;
  right: -4px;
  min-width: 24px;
  height: 14px;
  padding: 0 3px;
  border-radius: 7px;
  background: var(--tv-accent);
  color: var(--tv-bg);
  font-size: 7px;
  line-height: 14px;
  text-align: center;
}

.is-strike {
  position: sticky;
  right: auto;
  left: auto;
  width: 92px;
  border-right: 1px solid var(--tv-border);
  border-left: 1px solid var(--tv-border);
  background: var(--tv-bg-surface-2);
  text-align: center;
}

td.is-strike {
  color: var(--tv-text);
  font-size: 11px;
  font-weight: 750;
}

td.is-strike small {
  display: block;
  color: var(--tv-warn);
  font-size: 8px;
}

tr.is-atm td {
  border-top: 1px solid color-mix(in srgb, var(--tv-warn) 55%, transparent);
  border-bottom-color: color-mix(in srgb, var(--tv-warn) 55%, transparent);
}
</style>
