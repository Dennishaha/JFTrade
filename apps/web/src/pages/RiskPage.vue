<script setup lang="ts">
import HardStopControlPanel from "../components/risk/HardStopControlPanel.vue";
import RealTradeEmergencyPanel from "../components/risk/RealTradeEmergencyPanel.vue";
import RiskEventTimeline from "../components/risk/RiskEventTimeline.vue";
import RuntimeRiskConfigPanel from "../components/risk/RuntimeRiskConfigPanel.vue";
import StrategyRuntimeRiskSection from "../components/risk/StrategyRuntimeRiskSection.vue";
import RiskPostureSidebar from "../components/risk/RiskPostureSidebar.vue";
import RiskStatusStrip from "../components/risk/RiskStatusStrip.vue";
import ActionConfirmDialog from "../components/shared/ActionConfirmDialog.vue";
import { useRiskPage } from "@/composables/risk/useRiskPage";

const {
  RISK_TABS,
  activeTab,
  strategyInstances,
  strategyRuntimeRiskError,
  realTradeControlError,
  updatingRealTradeControlAction,
  pendingConfirmation,
  pendingRuntimeRiskSave,
  realTradeKillSwitchEvents,
  realTradeKillSwitchState,
  realTradeRiskEvents,
  realTradeRiskState,
  realTradeHardStopEntries,
  hardStopPrefill,
  riskPosture,
  statusRows,
  stripSections,
  sidebarFacts,
  confirmationView,
  confirmationBusy,
  tabBadge,
  setActiveTab,
  loadStrategyInstances,
  runtimeRiskForInstance,
  isUpdatingStrategyRuntimeRisk,
  updateStrategyRuntimeRiskMode,
  refreshRiskState,
  saveRuntimeRisk,
  confirmRuntimeRiskSave,
  disableRuntimeRisk,
  activateKillSwitch,
  releaseKillSwitch,
  activateHardStop,
  releaseHardStop,
  confirmPendingAction,
} = useRiskPage();
</script>

<template>
  <div class="risk-page">
    <RiskPostureSidebar
      :posture="riskPosture"
      :status-rows="statusRows"
      :facts="sidebarFacts"
      @refresh="refreshRiskState"
    />

    <section class="risk-main">
      <RiskStatusStrip :posture="riskPosture" :sections="stripSections" />

      <div class="risk-main__tabs-row">
        <div class="risk-main__tabs" role="tablist" aria-label="风控视图">
          <button
            v-for="tab in RISK_TABS"
            :key="tab.value"
            type="button"
            role="tab"
            :aria-selected="activeTab === tab.value"
            :class="{ 'is-active': activeTab === tab.value }"
            @click="setActiveTab(tab.value)"
          >
            {{ tab.label }}
            <span v-if="tabBadge(tab.value) > 0">{{ tabBadge(tab.value) }}</span>
          </button>
        </div>
      </div>

      <div class="risk-main__content">
        <div
          v-if="realTradeControlError"
          class="risk-main__error tv-status--warning tv-status-surface"
          role="alert"
        >
          {{ realTradeControlError }}
        </div>

        <div
          v-if="activeTab === 'emergency'"
          class="risk-main__danger-grid"
        >
          <RealTradeEmergencyPanel
            :kill-switch="realTradeKillSwitchState"
            :loading-action="updatingRealTradeControlAction"
            @activate="activateKillSwitch"
            @release="releaseKillSwitch"
          />
          <HardStopControlPanel
            :entries="realTradeHardStopEntries"
            :loading-action="updatingRealTradeControlAction"
            :prefill="hardStopPrefill"
            @activate="activateHardStop"
            @release="releaseHardStop"
          />
        </div>

        <RuntimeRiskConfigPanel
          v-else-if="activeTab === 'limits'"
          :loading="updatingRealTradeControlAction === 'runtime-risk.save'"
          :risk-state="realTradeRiskState"
          @disable="disableRuntimeRisk"
          @save="saveRuntimeRisk"
        />

        <StrategyRuntimeRiskSection
          v-else-if="activeTab === 'strategy'"
          :error="strategyRuntimeRiskError"
          :instances="strategyInstances"
          :is-updating="isUpdatingStrategyRuntimeRisk"
          :runtime-risk-for-instance="runtimeRiskForInstance"
          @refresh="loadStrategyInstances"
          @update-mode="updateStrategyRuntimeRiskMode"
        />

        <RiskEventTimeline
          v-else
          :kill-switch-events="realTradeKillSwitchEvents.entries"
          :risk-events="realTradeRiskEvents.entries"
        />
      </div>
    </section>

    <ActionConfirmDialog
      :open="pendingConfirmation != null"
      :title="confirmationView?.title ?? ''"
      :message="confirmationView?.message ?? ''"
      :confirm-label="confirmationView?.confirmLabel ?? '确认'"
      :busy="confirmationBusy"
      @close="pendingConfirmation = null"
      @confirm="confirmPendingAction"
    />

    <ActionConfirmDialog
      :open="pendingRuntimeRiskSave != null"
      title="保存运行时风控配置"
      message="本次变更会开放实盘交易或放宽单笔限额，确认保存吗？"
      confirm-label="确认保存"
      :busy="updatingRealTradeControlAction === 'runtime-risk.save'"
      @close="pendingRuntimeRiskSave = null"
      @confirm="confirmRuntimeRiskSave"
    />
  </div>
</template>

<style scoped>
.risk-page {
  display: flex;
  height: 100%;
  min-width: 0;
  min-height: 0;
  gap: 12px;
  padding: 14px;
  overflow: hidden;
  background:
    radial-gradient(circle at 92% -20%, color-mix(in srgb, var(--tv-accent) 9%, transparent), transparent 36%),
    var(--tv-bg-app);
}

/* ── 右侧主面板（对齐账户主面板） ───────────────────────── */

.risk-main {
  display: flex;
  min-width: 0;
  min-height: 0;
  flex: 1;
  flex-direction: column;
  overflow: hidden;
  border: 1px solid var(--tv-border);
  border-radius: 9px;
  background: var(--tv-bg-surface);
  box-shadow: 0 8px 24px color-mix(in srgb, var(--jf-shadow-color) 8%, transparent);
}

.risk-main__tabs-row {
  display: flex;
  flex: 0 0 auto;
  align-items: stretch;
  min-width: 0;
  border-bottom: 1px solid var(--tv-border);
  background: var(--tv-bg-surface-2);
}

.risk-main__tabs {
  display: flex;
  min-width: 0;
  flex: 1;
  gap: 2px;
  padding: 5px 7px 0;
  overflow-x: auto;
  scrollbar-width: thin;
}

.risk-main__tabs button {
  position: relative;
  flex: 0 0 auto;
  padding: 8px 14px 9px;
  border: 0;
  border-radius: 6px 6px 0 0;
  background: transparent;
  color: var(--tv-text-muted);
  cursor: pointer;
  font-size: var(--jf-text-5);
}

.risk-main__tabs button span {
  margin-left: 4px;
  color: var(--tv-text-dim);
  font-size: var(--jf-text-3);
}

.risk-main__tabs button.is-active {
  background: var(--tv-bg-surface);
  color: var(--tv-text);
  font-weight: 650;
}

.risk-main__tabs button.is-active::after {
  position: absolute;
  right: 8px;
  bottom: -1px;
  left: 8px;
  height: 2px;
  background: var(--tv-accent);
  content: "";
}

.risk-main__content {
  display: grid;
  min-height: 0;
  flex: 1;
  align-content: start;
  gap: 12px;
  overflow: auto;
  padding: 12px;
  scrollbar-width: thin;
}

.risk-main__error {
  padding: 8px 11px;
  border: 1px solid;
  border-radius: 6px;
  font-size: var(--jf-text-5);
}

.risk-main__danger-grid {
  display: grid;
  grid-template-columns: minmax(0, 1fr) minmax(0, 1fr);
  gap: 12px;
  align-items: start;
  padding: 12px;
  border: 1px solid var(--tv-status-error-border);
  border-radius: 10px;
  background: color-mix(
    in srgb,
    var(--tv-status-error-bg) 22%,
    var(--tv-bg-surface)
  );
}

@media (max-width: 1180px) {
  .risk-page {
    flex-direction: column;
    overflow: auto;
  }

  .risk-main {
    flex: 1 0 auto;
    min-height: 480px;
  }

  .risk-main__danger-grid {
    grid-template-columns: minmax(0, 1fr);
  }
}
</style>
