import type { InstrumentResolutionCandidate } from "@/types";
import { computed, ref, watch } from "vue";

import { useResearchFeature } from "@/composables/research/useResearchFeature";
import {
  formatCompactNumber,
  formatPrice,
  formatSigned,
  pickNumber,
  pickString,
} from "./researchEntry";
import type { ResearchTableColumn } from "./researchTable";

export type InstrumentResearchOperation =
  | "profile"
  | "financials"
  | "valuation"
  | "analyst"
  | "ownership"
  | "corporate_actions"
  | "short_interest"
  | "news";

export interface InstrumentResearchControllerProps {
  instrumentId: string;
  brokerId: string;
  operation: InstrumentResearchOperation;
}

export interface InstrumentResearchControllerEmit {
  (event: "update:instrumentId", instrumentId: string): void;
  (event: "select", entry: Record<string, unknown>): void;
  (event: "open", entry: Record<string, unknown>): void;
}

export function useInstrumentResearchController(
  props: Readonly<InstrumentResearchControllerProps>,
  emit: InstrumentResearchControllerEmit,
) {
  function asRecord(value: unknown): Record<string, unknown> | null {
    return value != null && typeof value === "object" && !Array.isArray(value)
      ? (value as Record<string, unknown>)
      : null;
  }

  function asRecords(value: unknown): Record<string, unknown>[] {
    return Array.isArray(value)
      ? value.filter(
          (item): item is Record<string, unknown> => asRecord(item) != null,
        )
      : [];
  }

  const searchValue = ref(props.instrumentId);
  watch(
    () => props.instrumentId,
    (value) => {
      searchValue.value = value;
    },
  );

  function selectCandidate(candidate: InstrumentResolutionCandidate): void {
    if (!candidate.selectable || !candidate.instrumentId) return;
    emit("update:instrumentId", candidate.instrumentId.toUpperCase());
  }

  const path = computed(() => {
    const instrumentId = encodeURIComponent(props.instrumentId);
    switch (props.operation) {
      case "profile":
        return `/api/v1/research/instruments/${instrumentId}?operation=profile&pageSize=100`;
      case "financials":
        return `/api/v1/research/financials/${instrumentId}?operation=statements&pageSize=100`;
      case "valuation":
        return `/api/v1/research/valuation/${instrumentId}?operation=detail&pageSize=100`;
      case "analyst":
        return `/api/v1/research/analyst/${instrumentId}?operation=consensus&pageSize=100`;
      case "ownership":
        return `/api/v1/research/ownership/${instrumentId}?operation=overview&pageSize=100`;
      case "corporate_actions":
        return `/api/v1/research/corporate-actions/${instrumentId}?operation=dividends&pageSize=100`;
      case "short_interest":
        return `/api/v1/research/short-interest/${instrumentId}?operation=daily_volume&pageSize=100`;
      default:
        return "";
    }
  });
  const feature = useResearchFeature(path, {
    expandCN: false,
    brokerId: () => props.brokerId,
  });

  const newsTarget = computed(() => ({
    kind: "instrument" as const,
    instrumentId: props.instrumentId,
    name: "",
    productClass: "equity" as const,
  }));

  const profileGroups = computed(() => {
    const groups: Array<{
      title: string;
      entries: Array<{ name: string; value: string; link: boolean }>;
    }> = [];
    let active = { title: "基本资料", entries: [] as Array<{
      name: string;
      value: string;
      link: boolean;
    }> };
    groups.push(active);
    for (const entry of feature.entries.value) {
      const fieldType = pickString(entry, ["fieldType", "type"]).toLowerCase();
      const name = pickString(entry, ["name", "fieldName"]);
      const value = pickString(entry, ["value", "content"]);
      if (fieldType.includes("title")) {
        active = { title: value || name || "其他资料", entries: [] };
        groups.push(active);
        continue;
      }
      if (!name && !value) continue;
      active.entries.push({
        name: name || "--",
        value: value || "--",
        link: /^https?:\/\//i.test(value),
      });
    }
    return groups.filter((group) => group.entries.length > 0);
  });

  const financialFields = computed(() =>
    asRecords(
      feature.metadata.value.structureList ??
        feature.metadata.value.fields ??
        feature.metadata.value.statementStructureList,
    ).map((entry) => ({
      id: String(entry.fieldId ?? entry.id ?? ""),
      label: pickString(entry, ["displayName", "name", "fieldName"]) || "--",
    })).filter((field) => field.id),
  );
  const financialRows = computed(() =>
    feature.entries.value.map((entry) => {
      const row: Record<string, unknown> = {
        period: pickString(entry, ["periodText", "reportDate", "period"]),
        currency: pickString(entry, ["currencyCode", "currency"]),
      };
      for (const item of asRecords(entry.itemList ?? entry.items)) {
        const id = String(item.fieldId ?? item.id ?? "");
        if (!id) continue;
        row[`field:${id}`] = item.data ?? item.value;
        row[`yoy:${id}`] = item.yoy;
        row[`qoq:${id}`] = item.qoq;
      }
      return row;
    }),
  );
  const financialColumns = computed<ResearchTableColumn[]>(() => [
    {
      key: "period",
      label: "报告期",
      width: "120px",
      value: (entry) => entry.period,
    },
    ...financialFields.value.slice(0, 14).map((field) => ({
      key: field.id,
      label: field.label,
      align: "right" as const,
      value: (entry: Record<string, unknown>) => entry[`field:${field.id}`],
      format: (value: unknown, entry: Record<string, unknown>) => {
        const numeric = typeof value === "number" ? value : Number(value);
        const base = Number.isFinite(numeric)
          ? formatCompactNumber(numeric)
          : String(value ?? "--");
        const yoy = Number(entry[`yoy:${field.id}`]);
        return Number.isFinite(yoy) ? `${base} (${formatSigned(yoy, "%")})` : base;
      },
    })),
  ]);

  const valuationEntry = computed(() => feature.entries.value[0] ?? {});
  const valuationTrend = computed(
    () => asRecord(valuationEntry.value.trend) ?? {},
  );
  const valuationHistory = computed(() =>
    asRecords(valuationTrend.value.historicalItems),
  );
  const marketDistribution = computed(
    () => asRecord(valuationEntry.value.marketDistribution) ?? {},
  );
  const marketDistributionSections = computed(() =>
    asRecords(marketDistribution.value.sections),
  );
  const plateDistribution = computed(
    () => asRecord(valuationEntry.value.plateDistribution) ?? {},
  );
  const plateStocks = computed(() =>
    asRecords(plateDistribution.value.stockItems),
  );
  const profitGrowthRate = computed(
    () => asRecord(valuationEntry.value.profitGrowthRate) ?? {},
  );
  const profitGrowthRows = computed(() =>
    asRecords(profitGrowthRate.value.profitData),
  );
  const hasValuationData = computed(
    () =>
      Object.keys(valuationTrend.value).length > 0 ||
      Object.keys(marketDistribution.value).length > 0 ||
      Object.keys(plateDistribution.value).length > 0 ||
      Object.keys(profitGrowthRate.value).length > 0,
  );

  const valuationTypeLabel = computed(() => {
    const raw = valuationEntry.value.valuationType;
    const numeric = typeof raw === "number" ? raw : Number(raw);
    if (Number.isFinite(numeric)) {
      return (
        ({ 1: "PE", 2: "PB", 3: "PS" } as Record<number, string>)[numeric] ??
        "估值"
      );
    }
    const name = String(raw ?? "")
      .replace(/^ValuationType_/i, "")
      .toUpperCase();
    return ["PE", "PB", "PS"].includes(name) ? name : "估值";
  });

  function securityLabel(value: unknown): string {
    const security = asRecord(value);
    if (security == null) return "--";
    const code = pickString(security, ["code"]);
    const rawMarket = pickNumber(security, ["market"]);
    const market =
      ({
        1: "HK",
        11: "US",
        21: "SH",
        22: "SZ",
        31: "SG",
        41: "JP",
        51: "AU",
        61: "MY",
        71: "CA",
        81: "FX",
        91: "CC",
        101: "US",
      } as Record<number, string>)[rawMarket ?? 0] ||
      pickString(security, ["market"]);
    return market && code ? `${market}.${code}` : code || "--";
  }

  interface ValuationMetric {
    label: string;
    value: string;
  }

  function metric(
    entry: Record<string, unknown>,
    label: string,
    keys: readonly string[],
    suffix = "",
  ): ValuationMetric {
    const value = pickNumber(entry, keys);
    return { label, value: `${formatPrice(value)}${value == null ? "" : suffix}` };
  }

  const trendMetrics = computed<ValuationMetric[]>(() => [
    metric(valuationTrend.value, "当前估值", ["currentValue"]),
    metric(valuationTrend.value, "预测估值", ["forwardValue"]),
    metric(valuationTrend.value, "历史平均", ["averageValue"]),
    metric(valuationTrend.value, "平均 - 1σ", ["avgMinus1Stddev"]),
    metric(valuationTrend.value, "平均 + 1σ", ["avgPlus1Stddev"]),
    metric(
      valuationTrend.value,
      "历史分位",
      ["valuationPercentile"],
      "%",
    ),
  ]);
  const valuationHistoryColumns: ResearchTableColumn[] = [
    {
      key: "date",
      label: "日期",
      value: (entry) => pickString(entry, ["timeStr", "time"]),
    },
    {
      key: "value",
      label: "个股估值",
      align: "right",
      value: (entry) => pickNumber(entry, ["value"]),
      format: (value) => formatPrice(value as number | null),
    },
    {
      key: "plateValue",
      label: "行业均值",
      align: "right",
      value: (entry) => pickNumber(entry, ["plateValue"]),
      format: (value) => formatPrice(value as number | null),
    },
  ];
  const marketMetrics = computed<ValuationMetric[]>(() => [
    metric(marketDistribution.value, "样本总数", ["total"]),
    metric(marketDistribution.value, "市场排名", ["ranking"]),
    metric(marketDistribution.value, "市场平均", ["averageValue"]),
    metric(marketDistribution.value, "市场中位数", ["medianValue"]),
  ]);
  const marketDistributionColumns: ResearchTableColumn[] = [
    {
      key: "interval",
      label: "估值区间",
      value: (entry) => {
        const start = pickNumber(entry, ["start"]);
        const end = pickNumber(entry, ["end"]);
        if (start == null) return "--";
        if (end === 0) return `${formatPrice(start)} 以上`;
        return `${formatPrice(start)} – ${formatPrice(end)}`;
      },
    },
    {
      key: "count",
      label: "个股数",
      align: "right",
      value: (entry) => pickNumber(entry, ["number"]),
      format: (value) => formatCompactNumber(value as number | null),
    },
  ];
  const plateMetrics = computed<ValuationMetric[]>(() => [
    {
      label: "所属板块",
      value:
        pickString(plateDistribution.value, ["plateName"]) ||
        securityLabel(plateDistribution.value.plate),
    },
    metric(plateDistribution.value, "板块平均", ["plateAverageValue"]),
    metric(plateDistribution.value, "板块排名", ["plateRanking"]),
    metric(plateDistribution.value, "成分股数", ["plateStockItemCount"]),
  ]);
  const plateStockColumns: ResearchTableColumn[] = [
    {
      key: "security",
      label: "证券",
      value: (entry) => securityLabel(entry.security),
    },
    {
      key: "name",
      label: "名称",
      value: (entry) => pickString(entry, ["name"]),
    },
    {
      key: "value",
      label: "估值",
      align: "right",
      value: (entry) => pickNumber(entry, ["value"]),
      format: (value) => formatPrice(value as number | null),
    },
    {
      key: "marketCap",
      label: "市值",
      align: "right",
      value: (entry) => pickNumber(entry, ["marketCap"]),
      format: (value) => formatCompactNumber(value as number | null),
    },
  ];
  const profitMetrics = computed<ValuationMetric[]>(() => [
    metric(profitGrowthRate.value, "财务 TTM 倍数", ["financialTtmMultiple"], "×"),
    metric(profitGrowthRate.value, "市值增长倍数", ["marketCapMultiple"], "×"),
    metric(profitGrowthRate.value, "统计年数", ["yearCount"], " 年"),
  ]);
  const profitGrowthColumns: ResearchTableColumn[] = [
    {
      key: "period",
      label: "财报周期",
      value: (entry) =>
        pickString(entry, ["periodStr"]) ||
        [
          pickString(entry, ["financialYear"]),
          pickString(entry, ["financialQuarter"]),
        ]
          .filter(Boolean)
          .join("/"),
    },
    {
      key: "reportDate",
      label: "报告日",
      value: (entry) => pickString(entry, ["reportDateStr", "reportDate"]),
    },
    {
      key: "marketCapMultiple",
      label: "市值倍数",
      align: "right",
      value: (entry) => pickNumber(entry, ["marketCapMultiple"]),
      format: (value) => {
        const number = value as number | null;
        return `${formatPrice(number)}${number == null ? "" : "×"}`;
      },
    },
    {
      key: "financeDataMultiple",
      label: "盈利 / 营收倍数",
      align: "right",
      value: (entry) => pickNumber(entry, ["financeDataMultiple"]),
      format: (value) => {
        const number = value as number | null;
        return `${formatPrice(number)}${number == null ? "" : "×"}`;
      },
    },
  ];

  const analyst = computed(() => feature.entries.value[0] ?? {});
  const analystCount = computed(() =>
    pickNumber(analyst.value, ["analystCount", "total"]),
  );
  const analystRating = computed(() => {
    const raw = analyst.value.rating;
    const numeric = typeof raw === "number" ? raw : Number(raw);
    if (Number.isFinite(numeric)) {
      return (
        ({
          1: "卖出",
          2: "跑输大盘",
          3: "持有",
          4: "买入",
          5: "强力推荐",
        } as Record<number, string>)[numeric] ?? "--"
      );
    }
    const name = String(raw ?? "")
      .replace(/^ResearchRatingType_/i, "")
      .toLowerCase();
    return (
      ({
        sell: "卖出",
        underperform: "跑输大盘",
        hold: "持有",
        buy: "买入",
        strongbuy: "强力推荐",
      } as Record<string, string>)[name] ?? "--"
    );
  });
  const ownershipRows = computed<Record<string, unknown>[]>(() => {
    const rows: Record<string, unknown>[] = [];
    const entryGroups = feature.entries.value.filter(
      (entry) => asRecords(entry.itemList).length > 0,
    );
    const metadataMainGroups = asRecords(
      feature.metadata.value.mainHolderInfoList,
    );
    const metadataHolderTypeGroups = asRecords(
      feature.metadata.value.holderTypeInfoList,
    );
    const entryMainGroups =
      metadataMainGroups.length > 0
        ? []
        : entryGroups.filter((entry) =>
            asRecords(entry.itemList).some(
              (item) => (pickNumber(item, ["holderId"]) ?? 0) > 0,
            ),
          );
    const entryHolderTypeGroups =
      metadataHolderTypeGroups.length > 0
        ? []
        : entryGroups.filter((entry) => !entryMainGroups.includes(entry));
    const groups: Array<{
      category: string;
      records: Record<string, unknown>[];
    }> = [
      {
        category: "主要股东",
        records: [...metadataMainGroups, ...entryMainGroups],
      },
      {
        category: "持股类型",
        records: [...metadataHolderTypeGroups, ...entryHolderTypeGroups],
      },
    ];
    for (const group of groups) {
      for (const entry of group.records) {
        const staticDateStr = pickString(entry, ["staticDateStr", "staticDate"]);
        for (const item of asRecords(entry.itemList)) {
          rows.push({ ...item, category: group.category, staticDateStr });
        }
      }
    }
    return rows;
  });
  const ownershipColumns: ResearchTableColumn[] = [
    {
      key: "category",
      label: "分组",
      width: "92px",
      value: (entry) => pickString(entry, ["category"]),
    },
    {
      key: "period",
      label: "统计日期",
      value: (entry) => pickString(entry, ["staticDateStr"]),
    },
    {
      key: "holder",
      label: "股东 / 类型",
      value: (entry) => pickString(entry, ["name"]),
    },
    {
      key: "ratio",
      label: "占比",
      align: "right",
      value: (entry) =>
        pickNumber(entry, ["holderPct", "holdingRatio", "percentage", "ratio"]),
      format: (value) => `${formatPrice(value as number | null)}%`,
    },
  ];

  const actionColumns: ResearchTableColumn[] = [
    {
      key: "pubDate",
      label: "公告日",
      value: (entry) => pickString(entry, ["pubDate"]),
    },
    {
      key: "statement",
      label: "分配方案",
      value: (entry) => pickString(entry, ["statement"]),
    },
    {
      key: "process",
      label: "进度",
      value: (entry) => pickString(entry, ["process"]),
    },
    {
      key: "recordDate",
      label: "登记日",
      value: (entry) => pickString(entry, ["recordDate"]),
    },
    {
      key: "exDate",
      label: "除权除息日",
      value: (entry) => pickString(entry, ["exDate"]),
    },
    {
      key: "dividendPayableDate",
      label: "派息日",
      value: (entry) => pickString(entry, ["dividendPayableDate"]),
    },
    {
      key: "fiscalYear",
      label: "财政年度",
      value: (entry) => pickString(entry, ["fiscalYear"]),
    },
  ];

  const shortRows = computed(() => [
    ...feature.entries.value,
    ...asRecords(feature.metadata.value.usItemList),
    ...asRecords(feature.metadata.value.hkItemList),
  ]);
  const isHKShortVolume = computed(
    () =>
      shortRows.value.some(
        (entry) =>
          "sharesTraded" in entry ||
          "shortSellSharesTraded" in entry ||
          "shortSellTurnover" in entry,
      ) ||
      pickNumber(feature.metadata.value, ["aggregatedShort"]) != null,
  );
  const shortBaseColumns: ResearchTableColumn[] = [
    {
      key: "date",
      label: "日期",
      value: (entry) =>
        pickString(entry, ["timestampStr", "date", "tradeDate"]),
    },
  ];
  const usShortColumns: ResearchTableColumn[] = [
    {
      key: "totalSharesShort",
      label: "卖空总股数",
      align: "right",
      value: (entry) => pickNumber(entry, ["totalSharesShort"]),
      format: (value) => formatCompactNumber(value as number | null),
    },
    {
      key: "nasdaqSharesShort",
      label: "NASDAQ",
      align: "right",
      value: (entry) => pickNumber(entry, ["nasdaqSharesShort"]),
      format: (value) => formatCompactNumber(value as number | null),
    },
    {
      key: "nyseSharesShort",
      label: "NYSE",
      align: "right",
      value: (entry) => pickNumber(entry, ["nyseSharesShort"]),
      format: (value) => formatCompactNumber(value as number | null),
    },
    {
      key: "shortPercent",
      label: "卖空占比",
      align: "right",
      value: (entry) => pickNumber(entry, ["shortPercent"]),
      format: (value) => `${formatPrice(value as number | null)}%`,
    },
    {
      key: "volume",
      label: "成交量",
      align: "right",
      value: (entry) => pickNumber(entry, ["volume"]),
      format: (value) => formatCompactNumber(value as number | null),
    },
  ];
  const hkShortColumns: ResearchTableColumn[] = [
    {
      key: "sharesTraded",
      label: "成交量",
      align: "right",
      value: (entry) => pickNumber(entry, ["sharesTraded"]),
      format: (value) => formatCompactNumber(value as number | null),
    },
    {
      key: "shortSellSharesTraded",
      label: "做空成交量",
      align: "right",
      value: (entry) => pickNumber(entry, ["shortSellSharesTraded"]),
      format: (value) => formatCompactNumber(value as number | null),
    },
    {
      key: "turnover",
      label: "成交额",
      align: "right",
      value: (entry) => pickNumber(entry, ["turnover"]),
      format: (value) => formatCompactNumber(value as number | null),
    },
    {
      key: "shortSellTurnover",
      label: "做空成交额",
      align: "right",
      value: (entry) => pickNumber(entry, ["shortSellTurnover"]),
      format: (value) => formatCompactNumber(value as number | null),
    },
    {
      key: "openPrice",
      label: "开盘价",
      align: "right",
      value: (entry) => pickNumber(entry, ["openPrice"]),
      format: (value) => formatPrice(value as number | null),
    },
  ];
  const shortPriceColumns: ResearchTableColumn[] = [
    {
      key: "close",
      label: "收盘价",
      align: "right",
      value: (entry) => pickNumber(entry, ["closePrice"]),
      format: (value) => formatPrice(value as number | null),
    },
    {
      key: "lastClosePrice",
      label: "昨收价",
      align: "right",
      value: (entry) => pickNumber(entry, ["lastClosePrice"]),
      format: (value) => formatPrice(value as number | null),
    },
    {
      key: "dailyTradeAvgRatio",
      label: "20 日日均成交比例",
      align: "right",
      value: (entry) => pickNumber(entry, ["dailyTradeAvgRatio"]),
      format: (value) => `${formatPrice(value as number | null)}%`,
    },
  ];
  const shortColumns = computed<ResearchTableColumn[]>(() => [
    ...shortBaseColumns,
    ...(isHKShortVolume.value ? hkShortColumns : usShortColumns),
    ...shortPriceColumns,
  ]);
  const shortSummary = computed<ValuationMetric[]>(() => {
    if (!isHKShortVolume.value) return [];
    return [
      {
        label: "未平仓股数",
        value: formatCompactNumber(
          pickNumber(feature.metadata.value, ["aggregatedShort"]),
        ),
      },
      metric(
        feature.metadata.value,
        "占流通股比例",
        ["aggregatedShortRatio"],
        "%",
      ),
      {
        label: "最新日期",
        value: pickString(feature.metadata.value, ["newTimeStr"]) || "--",
      },
    ];
  });

  const status = computed(() => {
    if (props.operation === "news") return "";
    if (feature.loading.value) return "研究数据加载中…";
    return feature.error.value;
  });

  return {
    searchValue,
    selectCandidate,
    feature,
    newsTarget,
    profileGroups,
    financialRows,
    financialColumns,
    valuationTrend,
    valuationHistory,
    valuationHistoryColumns,
    marketDistribution,
    marketDistributionSections,
    marketDistributionColumns,
    plateDistribution,
    plateStocks,
    plateStockColumns,
    profitGrowthRate,
    profitGrowthRows,
    profitGrowthColumns,
    hasValuationData,
    valuationTypeLabel,
    trendMetrics,
    marketMetrics,
    plateMetrics,
    profitMetrics,
    analyst,
    analystCount,
    analystRating,
    ownershipRows,
    ownershipColumns,
    actionColumns,
    shortRows,
    shortColumns,
    shortSummary,
    status,
  };
}
