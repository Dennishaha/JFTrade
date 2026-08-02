<script setup lang="ts">
import { useOrderEntryPanel } from "@/composables/trading/useOrderEntryPanel";
import InstrumentIdentity from "../domain/market-data/InstrumentIdentity.vue";
import RealTradeConfirmationDialog from "./RealTradeConfirmationDialog.vue";

const {
  notifications,
  side,
  orderType,
  tif,
  orderSession,
  quantity,
  price,
  stopPrice,
  predictionSide,
  hasEditedPrice,
  submitting,
  lastOrderFeedback,
  isRefreshingOrderFeedback,
  realTradeConfirmationOpen,
  realTradeConfirmationText,
  pendingRealTradeSubmission,
  draftClientOrderId,
  orderFeedbackPollIntervalMs,
  orderFeedbackMaxPolls,
  orderFeedbackPolling,
  isRealMode,
  requiredRealTradeConfirmationText,
  realTradeConfirmationMatches,
  isStop,
  isLimit,
  security,
  normalizedSecurityType,
  productClass,
  isEventContract,
  latestSnapshot,
  latestMarketPrice,
  limitPriceStep,
  stopPriceStep,
  tradeQuantityUnit,
  tradeQuantityUnitHint,
  formattedMaxTradeSession,
  activeBrokerId,
  activeTradingEnvironment,
  activeAccountId,
  activeMarket,
  activeInstrument,
  supportsOrderSessionSelection,
  supportsBrokerMaxTradeQuantity,
  maxTradeQuantityRequirements,
  maxTradeQuantityRequiresPrice,
  maxTradeQuantityReferencePrice,
  maxTradeQuantityPrimaryLabel,
  maxTradeQuantityPrimaryValue,
  maxTradeQuantityHint,
  currentMarketSessionLabel,
  orderSessionSummary,
  orderSessionCaution,
  estimate,
  formatMetric,
  countDecimalPlaces,
  resolveReferencePrice,
  resolveOrderPriceStep,
  alignPriceToStep,
  resolveAlignedMarketPrice,
  syncMarketPriceToPriceInput,
  markPriceEdited,
  alignPriceInput,
  alignStopPriceInput,
  formatOrderSession,
  formatInitialMargin,
  resolveOrderRequestTitle,
  createClientOrderId,
  currentClientOrderId,
  resolvePendingOrderSummary,
  resolveOrderFailureReason,
  normalizeOptionalText,
  orderFeedbackAccountHref,
  canCancelFeedbackOrder,
  formatFeedbackOrderStatus,
  formatBrokerAcceptance,
  formatFeedbackCheckedAt,
  stopOrderFeedbackPolling,
  scheduleOrderFeedbackRefresh,
  refreshOrderFeedbackOnce,
  refreshOrderFeedback,
  startOrderFeedbackPolling,
  loadMaxTradeQuantity,
  validateAndBuildExecutionPayload,
  submit,
  cancelRealTradeConfirmation,
  confirmRealTradeSubmission,
  executeOrderSubmission,
  setSide,
  brokerMaxTradeQuantity,
  isLoadingBrokerMaxTradeQuantity,
  loadBrokerMaxTradeQuantity,
  marketDataSnapshot,
  marketSecurityDetails,
  realTradeApprovals,
  realTradeRiskState,
  resolveBrokerReadFeatureQueryRequirements,
  selectedBrokerAccount,
  supportsBrokerReadFeature,
  systemStatus,
  prefs,
  supportsExtendedHoursForMarket,
  formatExecutionEventTypeLabel,
  formatExecutionOrderStatusLabel,
  formatOrderSideLabel,
  formatOrderTypeLabel,
  formatTimeInForceLabel,
  isFinalExecutionOrderStatus,
} = useOrderEntryPanel();
</script>

<template>
  <section class="tv-panel">
    <div class="tv-panel-head">
      <span class="tv-panel-title">下单</span>
      <InstrumentIdentity
        class="order-entry__identity"
        :market="activeMarket"
        :code="prefs.symbol"
        :instrument-id="activeInstrument?.instrumentId"
        :name="security?.name"
        compact
      />
      <div class="flex-1"></div>
      <span
        v-if="isRealMode"
        class="jf-mode-badge"
      >
        实盘
      </span>
    </div>
    <div class="tv-panel-body">
      <div class="tv-seg tv-order-side-seg mb-2.5 w-full">
        <button class="is-buy flex-1" :class="{ 'is-active': side === 'BUY' }" @click="setSide('BUY')">买入</button>
        <button class="is-sell flex-1" :class="{ 'is-active': side === 'SELL' }" @click="setSide('SELL')">卖出</button>
      </div>

      <div class="tv-form-row">
        <label>类型</label>
        <select v-model="orderType" class="tv-select">
          <option value="LIMIT">限价</option>
          <option v-if="!isEventContract" value="MARKET">市价</option>
          <option v-if="!isEventContract" value="STOP">止损</option>
          <option v-if="!isEventContract" value="STOP_LIMIT">止损限价</option>
        </select>
      </div>

      <div class="tv-form-row">
        <label>{{ isEventContract ? "投入金额" : "数量" }}</label>
        <input v-model.number="quantity" type="number" min="1" class="tv-input" />
      </div>

      <div v-if="isEventContract" class="tv-form-row">
        <label>预测方向</label>
        <select v-model="predictionSide" class="tv-select">
          <option value="YES">YES</option>
          <option value="NO">NO</option>
        </select>
      </div>

      <div v-if="isLimit" class="tv-form-row">
        <label>价格</label>
        <div class="grid grid-cols-[minmax(0,1fr)_32px] items-center gap-1.5">
          <input v-model.number="price" type="number" min="0" :step="limitPriceStep" class="tv-input" @input="markPriceEdited" @blur="alignPriceInput" />
          <button
            type="button"
            class="tv-icon-btn"
            title="同步市场价格"
            :disabled="latestMarketPrice == null"
            @click="syncMarketPriceToPriceInput(true)"
          >
            <span class="fa-solid fa-arrows-rotate" aria-hidden="true"></span>
          </button>
        </div>
      </div>

      <div v-if="isStop" class="tv-form-row">
        <label>止损价</label>
        <input v-model.number="stopPrice" type="number" min="0" :step="stopPriceStep" class="tv-input" @blur="alignStopPriceInput" />
      </div>

      <div class="tv-form-row">
        <label>有效期</label>
        <select v-model="tif" class="tv-select">
          <option value="DAY">当日有效</option>
          <option value="GTC">撤单前有效</option>
          <option value="IOC">立即成交剩余取消</option>
          <option value="FOK">全部成交否则取消</option>
        </select>
      </div>

      <div v-if="supportsOrderSessionSelection" class="tv-form-row">
        <label>时段</label>
        <select v-model="orderSession" class="tv-select">
          <option value="RTH">常规交易时段（RTH）</option>
          <option value="ETH">盘前盘后（ETH）</option>
          <option value="ALL">全时段（ALL）</option>
          <option value="OVERNIGHT">夜盘（OVERNIGHT）</option>
        </select>
      </div>

      <div v-if="supportsOrderSessionSelection && orderSessionSummary" class="jf-note -mt-0.5 mb-2">
        {{ orderSessionSummary }}
      </div>

      <div v-if="supportsOrderSessionSelection && orderSessionCaution" class="jf-note jf-note--accent mb-2.5">
        {{ orderSessionCaution }}
      </div>

      <div class="jf-note jf-note--muted mt-1 mb-2.5 flex justify-between">
        <span>名义金额</span>
        <span class="tv-num jf-text">{{ estimate() }}</span>
      </div>

      <div class="jf-estimate-box">
        <div class="flex items-center justify-between gap-2">
          <span class="jf-note jf-note--muted">最大可交易数量</span>
          <span class="jf-note">
            {{ formattedMaxTradeSession || tradeQuantityUnitHint }}
          </span>
        </div>
        <div v-if="isLoadingBrokerMaxTradeQuantity" class="jf-note jf-note--muted mt-1.5">
          正在估算...
        </div>
        <div v-else-if="brokerMaxTradeQuantity.lastError" class="jf-note jf-note--accent mt-1.5">
          {{ brokerMaxTradeQuantity.lastError }}
        </div>
        <template v-else-if="brokerMaxTradeQuantity.maxTradeQuantity">
          <div class="mt-1.5 flex justify-between gap-2">
            <span class="jf-note jf-note--muted">{{ maxTradeQuantityPrimaryLabel }}</span>
            <span class="tv-num jf-text font-semibold text-[var(--jf-text-10)]">
              {{ formatMetric(maxTradeQuantityPrimaryValue) }} {{ tradeQuantityUnit }}
            </span>
          </div>
          <div class="jf-note mt-1">
            {{ tradeQuantityUnitHint }}<span v-if="formattedMaxTradeSession"> · {{ formattedMaxTradeSession }}</span>
          </div>
          <div class="mt-2 grid grid-cols-2 gap-2 text-[var(--jf-text-5)]">
            <div>
              <div class="jf-text-muted">现金可买</div>
              <div class="tv-num jf-text">{{ formatMetric(brokerMaxTradeQuantity.maxTradeQuantity.maxCashBuy) }} {{ tradeQuantityUnit }}</div>
            </div>
            <div>
              <div class="jf-text-muted">融资后可买</div>
              <div class="tv-num jf-text">{{ formatMetric(brokerMaxTradeQuantity.maxTradeQuantity.maxCashAndMarginBuy) }} {{ tradeQuantityUnit }}</div>
            </div>
            <div>
              <div class="jf-text-muted">可卖持仓</div>
              <div class="tv-num jf-text">{{ formatMetric(brokerMaxTradeQuantity.maxTradeQuantity.maxPositionSell) }} {{ tradeQuantityUnit }}</div>
            </div>
            <div>
              <div class="jf-text-muted">可卖空</div>
              <div class="tv-num jf-text">{{ formatMetric(brokerMaxTradeQuantity.maxTradeQuantity.maxSellShort) }} {{ tradeQuantityUnit }}</div>
            </div>
          </div>
          <div class="jf-note jf-note--muted mt-2 flex justify-between gap-2">
            <span title="多头初始保证金；股票通常不返回该字段">多头初始保证金 {{ formatInitialMargin(brokerMaxTradeQuantity.maxTradeQuantity.longRequiredIm) }}</span>
            <span title="空头初始保证金；股票通常不返回该字段">空头初始保证金 {{ formatInitialMargin(brokerMaxTradeQuantity.maxTradeQuantity.shortRequiredIm) }}</span>
          </div>
        </template>
        <div v-else class="jf-note jf-note--muted mt-1.5">
          {{ maxTradeQuantityHint }}
        </div>
      </div>

      <button
        type="button"
        class="tv-btn jf-submit-btn"
        :class="side === 'BUY' ? 'tv-btn-buy' : 'tv-btn-sell'"
        :disabled="submitting"
        @click="submit"
      >
        {{ submitting ? "提交中..." : `${formatOrderSideLabel(side)} ${prefs.symbol}` }}
      </button>
      <div
        v-if="lastOrderFeedback"
        class="tv-order-feedback"
        :class="`is-${lastOrderFeedback.level}`"
        role="status"
        aria-live="polite"
      >
        <div class="tv-order-feedback-title">{{ lastOrderFeedback.title }}</div>
        <div class="tv-order-feedback-message">{{ lastOrderFeedback.message }}</div>
        <div
          v-if="lastOrderFeedback.internalOrderId || lastOrderFeedback.brokerOrderId || lastOrderFeedback.brokerOrderIdEx"
          class="tv-order-receipt-grid"
        >
          <div v-if="lastOrderFeedback.internalOrderId">
            <span>内部单号</span>
            <strong>{{ lastOrderFeedback.internalOrderId }}</strong>
          </div>
          <div v-if="lastOrderFeedback.brokerOrderId || lastOrderFeedback.brokerOrderIdEx">
            <span>券商单号</span>
            <strong>{{ lastOrderFeedback.brokerOrderIdEx ?? lastOrderFeedback.brokerOrderId }}</strong>
          </div>
          <div>
            <span>当前状态</span>
            <strong>{{ formatFeedbackOrderStatus(lastOrderFeedback) }}</strong>
          </div>
          <div>
            <span>券商接受</span>
            <strong>{{ formatBrokerAcceptance(lastOrderFeedback) }}</strong>
          </div>
          <div>
            <span>撤单</span>
            <strong>{{ canCancelFeedbackOrder(lastOrderFeedback) ? "可在账户页提交" : "不可提交" }}</strong>
          </div>
          <div v-if="lastOrderFeedback.rawBrokerStatus">
            <span>券商原始状态</span>
            <strong>{{ lastOrderFeedback.rawBrokerStatus }}</strong>
          </div>
        </div>
        <div v-if="lastOrderFeedback.latestEvent" class="tv-order-feedback-event">
          最近事件：{{ formatExecutionEventTypeLabel(lastOrderFeedback.latestEvent.eventType) }}
        </div>
        <div v-if="lastOrderFeedback.internalOrderId" class="tv-order-feedback-actions">
          <a :href="orderFeedbackAccountHref(lastOrderFeedback)">查看账户订单</a>
          <span v-if="lastOrderFeedback.checkedAt">更新于 {{ formatFeedbackCheckedAt(lastOrderFeedback.checkedAt) }}</span>
          <button
            type="button"
            class="tv-icon-btn"
            title="刷新订单状态"
            :disabled="isRefreshingOrderFeedback"
            @click="refreshOrderFeedback(lastOrderFeedback.internalOrderId, true)"
          >
            <span class="fa-solid fa-arrows-rotate" aria-hidden="true"></span>
          </button>
        </div>
      </div>

      <RealTradeConfirmationDialog
        v-model="realTradeConfirmationOpen"
        v-model:confirmation-text="realTradeConfirmationText"
        :account-id="activeAccountId"
        :confirmation-matches="realTradeConfirmationMatches"
        :max-order-notional="realTradeRiskState.effectiveMaxOrderNotional"
        :max-order-quantity="realTradeRiskState.effectiveMaxOrderQuantity"
        :order-summary="pendingRealTradeSubmission?.orderSummary"
        :real-trading-enabled="systemStatus.realTradingEnabled"
        :required-confirmation-text="requiredRealTradeConfirmationText"
        :submitting="submitting"
        @cancel="cancelRealTradeConfirmation"
        @confirm="confirmRealTradeSubmission"
      />
    </div>
  </section>
</template>

<style scoped>
.order-entry__identity {
  max-width: 60%;
  min-width: 0;
  overflow: hidden;
}

.tv-order-feedback {
  margin-top: 10px;
  border: 1px solid var(--tv-border);
  border-radius: 6px;
  padding: 9px 10px;
  background: rgba(255, 255, 255, 0.03);
  font-size: var(--jf-text-6);
  line-height: 1.45;
}

.tv-order-feedback.is-success {
  border-left: 3px solid var(--tv-accent);
}

.tv-order-feedback.is-error {
  border-left: 3px solid var(--tv-accent-strong);
}

.tv-order-feedback-title {
  color: var(--tv-text);
  font-weight: 600;
}

.tv-order-feedback-message {
  margin-top: 3px;
  color: var(--tv-text-muted);
  overflow-wrap: anywhere;
}

.tv-order-receipt-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 6px;
  margin-top: 8px;
}

.tv-order-receipt-grid div {
  min-width: 0;
  border: 1px solid var(--tv-border);
  border-radius: 5px;
  padding: 6px;
  background: rgba(255, 255, 255, 0.025);
}

.tv-order-receipt-grid span {
  display: block;
  color: var(--tv-text-muted);
  font-size: var(--jf-text-4);
}

.tv-order-receipt-grid strong {
  display: block;
  margin-top: 2px;
  color: var(--tv-text);
  font-weight: 600;
  overflow-wrap: anywhere;
}

.tv-order-feedback-actions {
  margin-top: 8px;
  display: flex;
  align-items: center;
  gap: 8px;
}

.tv-order-feedback-actions a {
  color: var(--tv-accent);
  font-size: var(--jf-text-6);
  font-weight: 600;
  text-decoration: none;
}

.tv-order-feedback-actions a:hover {
  text-decoration: underline;
}

.tv-order-feedback-actions > span {
  margin-left: auto;
  color: var(--tv-text-dim);
  font-size: var(--jf-text-4);
}

.tv-order-feedback-event {
  margin-top: 7px;
  color: var(--tv-text-muted);
  font-size: var(--jf-text-5);
}

</style>
