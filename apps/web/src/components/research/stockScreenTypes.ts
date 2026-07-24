export type StockScreenValueType =
  | "string"
  | "integer"
  | "integer_array"
  | "number"
  | "missing";

export type StockScreenAvailability =
  | "available"
  | "experimental"
  | "unsupported";

export type StockScreenFilterKind =
  | "enum"
  | "interval"
  | "position"
  | "pattern"
  | "interval_or_set"
  | "set"
  | "";

export interface StockScreenEnumOption {
  key: string;
  value: number;
  label: string;
}

export interface StockScreenFactorParameter {
  name: string;
  type: string;
  editorType?: string;
  enum?: string;
  required?: boolean;
  default?: unknown;
  minimum?: number;
  maximum?: number;
  step?: number;
  unit?: string;
  visibleWhen?: Record<string, unknown>;
  help?: string;
}

export interface StockScreenFactor {
  key: string;
  category: string;
  label: string;
  valueType: string;
  unit?: string;
  currencyBasis?: "quote" | "reporting";
  displayFormat?:
    | "price"
    | "compact_amount"
    | "percent"
    | "integer"
    | "timestamp"
    | "number";
  filterKind?: StockScreenFilterKind;
  conditionEditor?:
    | "singleSelect"
    | "multiSelect"
    | "integer"
    | "integerSet"
    | "range"
    | "rangeOrSet"
    | "indicatorCompare"
    | "pattern";
  valueEnum?: string;
  operators?: string[];
  filter: boolean;
  retrieve: boolean;
  sort: boolean;
  parameters?: StockScreenFactorParameter[];
  roles?: string[];
  markets?: string[];
  availability: StockScreenAvailability;
  reason?: string;
  help?: string;
  searchKeywords?: string[];
}

export interface StockScreenFactorCategory {
  key: string;
  label: string;
  count: number;
}

export interface StockScreenCatalog {
  version: string;
  schemaVersion: number;
  querySchemaVersion: 2;
  provider: string;
  providerVersion: string;
  market?: string;
  markets: string[];
  categories: StockScreenFactorCategory[];
  factors: StockScreenFactor[];
  enums: Record<string, StockScreenEnumOption[]>;
  rateLimit: {
    requests: number;
    windowSeconds: number;
  };
}

export interface StockScreenFactorParams {
  days?: number;
  periodAverage?: number;
  term?: number;
  duration?: number;
  year?: number;
  futureDuration?: number;
  period?: number;
  rangePeriod?: number;
  firstCustomParam?: number;
  indicatorParams?: number[];
  brokerParam?: string;
  optionParamType?: number;
  optionParamString?: string;
  optionParamInteger?: number;
  optionParamIntegers?: number[];
  optionHvPeriod?: number;
}

export interface StockScreenFactorRef {
  /** Stable identity of one configured factor instance. */
  instanceId?: string;
  /** V2 factor key mirrored by `factor` inside the editor draft. */
  factorKey?: string;
  factor: string;
  params?: StockScreenFactorParams;
}

export interface StockScreenBoundary {
  value: number;
  includes?: boolean;
}

export interface StockScreenInterval {
  min?: StockScreenBoundary;
  max?: StockScreenBoundary;
  unit?: number;
}

export interface StockScreenFilter extends StockScreenFactorRef {
  conditionId?: string;
  min?: StockScreenBoundary;
  max?: StockScreenBoundary;
  intervals?: StockScreenInterval[];
  values?: number[];
  continuousPeriod?: number;
  position?: number;
  secondFactor?: StockScreenFactorRef;
  secondValue?: number;
  match?: boolean;
}

export interface StockScreenEditorFilter extends StockScreenFilter {
  id: string;
}

export interface StockScreenColumn extends StockScreenFactorRef {
  /** Stable result-column identity, distinct from the factor key. */
  columnId?: string;
}

export interface StockScreenSort extends StockScreenFactorRef {
  sortId?: string;
  direction: "asc" | "desc" | "abs_asc" | "abs_desc";
}

export interface StockScreenPool {
  watchlistStockIds?: string[];
  plates?: Array<{
    parentPlateId?: string;
    plateIds: string[];
  }>;
}

/** Mutable editor state. It is converted to a V2 definition at API boundaries. */
export interface StockScreenDraft {
  brokerId?: string;
  market: string;
  pool?: StockScreenPool;
  filters?: StockScreenFilter[];
  columns?: StockScreenColumn[];
  sort?: StockScreenSort[];
}

export interface StockScreenQuery extends StockScreenDefinitionV2 {
  accountId?: string;
  tradingEnvironment?: string;
  page: {
    offset: number;
    limit: number;
  };
}

export interface StockScreenValue {
  type: StockScreenValueType;
  string?: string;
  integer?: number;
  integers?: number[];
  number?: number;
  enumType?: string;
  enumName?: string;
  endTime?: number;
  unit?: string;
}

export interface StockScreenEntry {
  stockId: string;
  instrumentId?: string;
  market?: string;
  symbol?: string;
  name?: string;
  industry?: string;
  quoteCurrency?: string;
  productClass: string;
  /** V2 result cells keyed by column identity. */
  cells: Record<string, StockScreenResultCell>;
}

export interface StockScreenResultCell {
  columnId: string;
  instanceId: string;
  factorKey?: string;
  value: StockScreenValue;
}

export interface StockScreenResult {
  provider: {
    brokerId: string;
    securityFirm?: string;
    featureId: string;
    capability: string;
    selectionReason: string;
    resolvedAt: string;
    asOf: string;
  };
  asOf: string;
  entries: StockScreenEntry[];
  nextOffset?: number;
  hasMore: boolean;
  total?: number;
  warnings?: string[];
  partialErrors?: Array<{
    code?: string;
    message?: string;
    [key: string]: unknown;
  }>;
  /** Column metadata returned by V2 adapters. */
  columns?: Array<StockScreenResultColumn>;
}

export interface StockScreenResultColumn {
  columnId: string;
  instanceId?: string;
  factorKey: string;
  label?: string;
}

export interface StockScreenFactorRefV2 {
  instanceId: string;
  factorKey: string;
  params?: StockScreenFactorParams;
}

export interface StockScreenConditionV2 {
  id: string;
  factor: StockScreenFactorRefV2;
  operator: string;
  value?: unknown;
  secondFactor?: StockScreenFactorRefV2;
}

export interface StockScreenColumnV2 {
  columnId: string;
  factor: StockScreenFactorRefV2;
  label?: string;
}

export interface StockScreenSortV2 {
  sortId?: string;
  columnId?: string;
  factor: StockScreenFactorRefV2;
  direction: StockScreenSort["direction"];
}

export interface StockScreenDefinitionV2 {
  brokerId?: string;
  market: string;
  pool?: StockScreenPool;
  conditions: StockScreenConditionV2[];
  columns: StockScreenColumnV2[];
  sorts: StockScreenSortV2[];
  catalogVersion: string;
  querySchemaVersion: 2;
}

export interface StockScreenPreset {
  presetId: string;
  name: string;
  querySchemaVersion: 2;
  definition: StockScreenDefinitionV2;
  revision: number;
  createdAt: string;
  updatedAt: string;
}

export interface StockScreenPresetList {
  presets: StockScreenPreset[];
}
