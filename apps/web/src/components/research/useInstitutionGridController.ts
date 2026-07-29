import { computed, ref, watch } from "vue";

import { useResearchFeature } from "../../composables/useResearchFeature";
import { pickNumber, pickString } from "./researchEntry";
import { isResearchQuoteEntry } from "./researchQuote";

export type InstitutionOperation = "list" | "holding_changes";

export interface InstitutionGridControllerProps {
  market: string;
  brokerId: string;
  operation: InstitutionOperation;
  institutionId: string;
}

export interface InstitutionGridControllerEmit {
  (event: "select", entry: Record<string, unknown>): void;
  (event: "update:institutionId", institutionId: string): void;
}

export function useInstitutionGridController(
  props: Readonly<InstitutionGridControllerProps>,
  emit: InstitutionGridControllerEmit,
) {
  const feature = useResearchFeature(
    () =>
      `/api/v1/research/institutions?market=${encodeURIComponent(props.market)}&operation=list&pageSize=50`,
    { expandCN: false, brokerId: () => props.brokerId },
  );

  const keyword = ref("");
  const selectedInstitution = ref<Record<string, unknown> | null>(null);
  const isHoldingChanges = computed(
    () => props.operation === "holding_changes",
  );

  function institutionKey(entry: Record<string, unknown>): string {
    const value = pickNumber(entry, ["institutionId"]);
    return value == null ? "" : String(Math.trunc(value));
  }

  const institutionId = computed(
    () =>
      String(props.institutionId ?? "").trim() ||
      institutionKey(selectedInstitution.value ?? {}),
  );

  function detailPath(operation: string, enabled = true): string {
    if (!enabled || !institutionId.value) return "";
    return `/api/v1/research/institutions?market=${encodeURIComponent(props.market)}&operation=${operation}&institutionId=${encodeURIComponent(institutionId.value)}&pageSize=50`;
  }

  const profile = useResearchFeature(
    () => detailPath("profile", !isHoldingChanges.value),
    { expandCN: false, brokerId: () => props.brokerId },
  );
  const holdings = useResearchFeature(
    () => detailPath("holdings", !isHoldingChanges.value),
    { expandCN: false, brokerId: () => props.brokerId },
  );
  const distribution = useResearchFeature(
    () => detailPath("distribution", !isHoldingChanges.value),
    { expandCN: false, brokerId: () => props.brokerId },
  );
  const holdingChanges = useResearchFeature(
    () => detailPath("holding_changes", isHoldingChanges.value),
    { expandCN: false, brokerId: () => props.brokerId },
  );

  const profileEntry = computed<Record<string, unknown>>(
    () => profile.entries.value[0] ?? {},
  );

  watch(
    () => props.institutionId,
    (value) => {
      const requested = String(value ?? "").trim();
      if (requested === "") {
        selectedInstitution.value = null;
        return;
      }
      selectedInstitution.value =
        feature.entries.value.find(
          (entry) => institutionKey(entry) === requested,
        ) ?? null;
    },
    { immediate: true },
  );

  watch(
    () => feature.entries.value,
    (entries) => {
      const requested = String(props.institutionId ?? "").trim();
      if (requested === "") return;
      selectedInstitution.value =
        entries.find((entry) => institutionKey(entry) === requested) ?? null;
    },
  );

  watch(
    () => [props.market, props.operation] as const,
    ([market, operation], [previousMarket, previousOperation]) => {
      const keepsInstitutionSelection =
        market === previousMarket &&
        [operation, previousOperation].every((value) =>
          ["list", "holding_changes"].includes(value),
        );
      if (!keepsInstitutionSelection) {
        selectedInstitution.value = null;
      }
    },
  );

  const selectedInstitutionName = computed(
    () =>
      pickString(selectedInstitution.value ?? {}, [
        "name",
        "institutionName",
      ]) ||
      pickString(profileEntry.value, ["institutionName", "name"]) ||
      (institutionId.value ? `机构 ${institutionId.value}` : ""),
  );

  function closeDetails(): void {
    selectedInstitution.value = null;
    emit("update:institutionId", "");
  }

  function selectInstitution(entry: Record<string, unknown>): void {
    selectedInstitution.value = entry;
    emit("update:institutionId", institutionKey(entry));
  }

  function selectHolding(entry: Record<string, unknown>): void {
    if (isResearchQuoteEntry(entry, props.market)) {
      emit("select", entry);
    }
  }

  function holdingIsQuoteable(entry: Record<string, unknown>): boolean {
    return isResearchQuoteEntry(entry, props.market);
  }

  const activeDetail = computed(() =>
    isHoldingChanges.value ? holdingChanges : holdings,
  );
  const activeDetailError = computed(() => {
    if (isHoldingChanges.value) return holdingChanges.error.value;
    return (
      profile.error.value ||
      holdings.error.value ||
      distribution.error.value
    );
  });
  const activeDetailEmptyLabel = computed(() =>
    isHoldingChanges.value ? "暂无持仓变化" : "暂无持仓明细",
  );
  const activeLoadMoreLabel = computed(() =>
    isHoldingChanges.value ? "加载更多变化" : "加载更多持仓",
  );
  const activeLoadingMoreLabel = computed(() =>
    isHoldingChanges.value ? "正在加载变化…" : "加载中…",
  );
  const activeEntries = computed(() => activeDetail.value.entries.value);
  const activeHasMore = computed(() => activeDetail.value.hasMore.value);
  const activeLoading = computed(() => activeDetail.value.loading.value);
  const activeLoadingMore = computed(
    () => activeDetail.value.loadingMore.value,
  );

  function loadMoreDetails(): void {
    void activeDetail.value.loadMore();
  }

  const toolbarTitle = computed(() => {
    const marketLabel = props.market === "HK" ? "港股机构" : "美股机构";
    return isHoldingChanges.value ? `${marketLabel}持仓变化` : marketLabel;
  });
  const selectionHint = computed(() =>
    isHoldingChanges.value ? "请选择机构查看持仓变化" : "",
  );
  const holdingChangesTotal = computed(() => holdingChanges.total.value);
  const holdingChangesWarnings = computed(() => [
    ...holdingChanges.warnings.value,
    ...holdingChanges.partialErrors.value.map((item) => item.message),
  ]);
  const hasHoldingChangesWarnings = computed(
    () => holdingChangesWarnings.value.length > 0,
  );
  const profileDescription = computed(() =>
    pickString(profileEntry.value, ["description", "profile"]),
  );
  const profileDisclosureDate = computed(() =>
    pickString(profileEntry.value, ["disclosureDate", "asOfDate"]),
  );
  const profileCurrency = computed(
    () =>
      pickString(profileEntry.value, ["currency"]) ||
      pickString(profile.metadata.value, ["currency"]) ||
      pickString(feature.metadata.value, ["currency"]) ||
      (props.market === "HK" ? "HKD" : "USD"),
  );

  interface IndustryDistribution {
    key: string;
    name: string;
    positionValue: number | null;
    portfolioPct: number | null;
  }

  const industryDistribution = computed<IndustryDistribution[]>(() =>
    distribution.entries.value.map((entry, index) => ({
      key:
        pickString(entry, ["industryId"]) ||
        pickString(entry, ["industryName"]) ||
        String(index),
      name: pickString(entry, ["industryName", "name"]) || "未分类",
      positionValue: pickNumber(entry, ["positionValue", "marketValue"]),
      portfolioPct: pickNumber(entry, ["portfolioPct"]),
    })),
  );

  const detailTableLabel = computed(() =>
    isHoldingChanges.value ? "机构持仓变化" : "机构持仓明细",
  );

  function rowKey(entry: Record<string, unknown>, index: number): string {
    return (
      pickString(entry, ["instrumentId", "symbol"]) ||
      `${pickString(entry, ["holdingDate"])}-${index}`
    );
  }

  interface InstitutionCard {
    entry: Record<string, unknown>;
    name: string;
    initial: string;
    marketValue: number | null;
    marketValueChange: number | null;
    holdingCount: number | null;
    holdingCountChange: number | null;
    disclosureDate: string;
  }

  const cards = computed<InstitutionCard[]>(() =>
    feature.entries.value.map((entry) => {
      const name = pickString(entry, ["name", "institutionName"]) || "--";
      return {
        entry,
        name,
        initial: name.slice(0, 1),
        marketValue: pickNumber(entry, ["marketValue"]),
        marketValueChange: pickNumber(entry, ["marketValueChange"]),
        holdingCount: pickNumber(entry, ["holdingCount"]),
        holdingCountChange: pickNumber(entry, ["holdingCountChange"]),
        disclosureDate: pickString(entry, ["asOfDate", "disclosureDate"]),
      };
    }),
  );
  const visibleCards = computed(() => {
    const filter = keyword.value.trim().toLocaleLowerCase();
    if (filter === "") return cards.value;
    return cards.value.filter((card) =>
      card.name.toLocaleLowerCase().includes(filter),
    );
  });

  return {
    feature,
    keyword,
    isHoldingChanges,
    institutionKey,
    institutionId,
    profile,
    holdings,
    distribution,
    holdingChanges,
    profileEntry,
    selectedInstitutionName,
    closeDetails,
    selectInstitution,
    selectHolding,
    holdingIsQuoteable,
    activeDetailError,
    activeDetailEmptyLabel,
    activeLoadMoreLabel,
    activeLoadingMoreLabel,
    activeEntries,
    activeHasMore,
    activeLoading,
    activeLoadingMore,
    loadMoreDetails,
    toolbarTitle,
    selectionHint,
    holdingChangesTotal,
    holdingChangesWarnings,
    hasHoldingChangesWarnings,
    profileDescription,
    profileDisclosureDate,
    profileCurrency,
    industryDistribution,
    detailTableLabel,
    rowKey,
    visibleCards,
  };
}
