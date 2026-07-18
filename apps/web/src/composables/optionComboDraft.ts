import {
  type ComputedRef,
  type InjectionKey,
  type Ref,
  computed,
  inject,
  provide,
  ref,
} from "vue";

export type OptionComboSide = "BUY" | "SELL";
export type OptionComboStrategy =
  | "vertical"
  | "straddle"
  | "strangle"
  | "calendar"
  | "butterfly";
export type OptionComboDockTab = "trade" | "positions" | "orders" | "history";
export type OptionComboPriceSource = "bid" | "mid" | "ask" | "custom";

export interface OptionContractChoice {
  instrumentId: string;
  code: string;
  name: string;
  label: string;
  optionType: "call" | "put";
  strike: number;
  multiplier: number;
  expiry: string;
  bidPrice: number | null;
  askPrice: number | null;
}

export interface OptionComboLegDraft extends OptionContractChoice {
  side: OptionComboSide;
  ratio: number;
}

interface OptionComboHistorySnapshot {
  legs: OptionComboLegDraft[];
}

export interface OptionComboDraftStore {
  underlyingInstrumentId: Ref<string>;
  market: Ref<string>;
  workspaceActive: Ref<boolean>;
  activeDockTab: Ref<OptionComboDockTab>;
  contracts: Ref<OptionContractChoice[]>;
  legs: Ref<OptionComboLegDraft[]>;
  quantity: Ref<number>;
  comboPrice: Ref<number>;
  priceSource: Ref<OptionComboPriceSource>;
  submittedOrderId: Ref<string>;
  revision: Ref<number>;
  selectedLegInstrumentIds: ComputedRef<string[]>;
  canUndo: ComputedRef<boolean>;
  canRedo: ComputedRef<boolean>;
  setContext: (instrumentId: string, market: string) => boolean;
  setWorkspaceActive: (active: boolean) => void;
  setDockTab: (tab: OptionComboDockTab) => void;
  setContracts: (contracts: OptionContractChoice[]) => void;
  updateQuotes: (
    quotes: Array<{
      instrumentId: string;
      bidPrice: number | null;
      askPrice: number | null;
    }>,
  ) => void;
  toggleLeg: (contract: OptionContractChoice, side: OptionComboSide) => void;
  addLeg: (contract: OptionContractChoice) => void;
  removeLeg: (instrumentId: string) => void;
  updateLeg: (
    instrumentId: string,
    patch: Partial<Pick<OptionComboLegDraft, "side" | "ratio">>,
  ) => void;
  replaceLegs: (legs: OptionComboLegDraft[]) => void;
  reverseLegs: () => void;
  clearLegs: () => void;
  undo: () => void;
  redo: () => void;
}

const draftKey: InjectionKey<OptionComboDraftStore> = Symbol(
  "option-combo-draft",
);
const historyLimit = 20;

function cloneLegs(legs: OptionComboLegDraft[]): OptionComboLegDraft[] {
  return legs.map((leg) => ({ ...leg }));
}

function normalizedInstrumentId(value: string | null | undefined): string {
  return value?.trim().toUpperCase() ?? "";
}

function sameContract(
  left: OptionContractChoice,
  right: OptionContractChoice,
): boolean {
  return (
    left.instrumentId === right.instrumentId &&
    left.code === right.code &&
    left.name === right.name &&
    left.label === right.label &&
    left.optionType === right.optionType &&
    left.strike === right.strike &&
    left.multiplier === right.multiplier &&
    left.expiry === right.expiry &&
    left.bidPrice === right.bidPrice &&
    left.askPrice === right.askPrice
  );
}

function sameContractList(
  left: OptionContractChoice[],
  right: OptionContractChoice[],
): boolean {
  return (
    left.length === right.length &&
    left.every((contract, index) => {
      const candidate = right[index];
      return candidate != null && sameContract(contract, candidate);
    })
  );
}

export function createOptionComboDraftStore(): OptionComboDraftStore {
  const underlyingInstrumentId = ref("");
  const market = ref("");
  const workspaceActive = ref(false);
  const activeDockTab = ref<OptionComboDockTab>("positions");
  const contracts = ref<OptionContractChoice[]>([]);
  const legs = ref<OptionComboLegDraft[]>([]);
  const quantity = ref(1);
  const comboPrice = ref(0);
  const priceSource = ref<OptionComboPriceSource>("mid");
  const submittedOrderId = ref("");
  const revision = ref(0);
  const undoStack = ref<OptionComboHistorySnapshot[]>([]);
  const redoStack = ref<OptionComboHistorySnapshot[]>([]);

  const selectedLegInstrumentIds = computed(() =>
    legs.value.map((leg) => normalizedInstrumentId(leg.instrumentId)),
  );
  const canUndo = computed(() => undoStack.value.length > 0);
  const canRedo = computed(() => redoStack.value.length > 0);

  function rememberCurrent(): void {
    undoStack.value = [
      ...undoStack.value.slice(-(historyLimit - 1)),
      { legs: cloneLegs(legs.value) },
    ];
    redoStack.value = [];
  }

  function commit(next: OptionComboLegDraft[], remember = true): void {
    if (remember) rememberCurrent();
    legs.value = cloneLegs(next);
    submittedOrderId.value = "";
    revision.value += 1;
  }

  function setContext(instrumentId: string, nextMarket: string): boolean {
    const normalized = normalizedInstrumentId(instrumentId);
    const marketValue = nextMarket.trim().toUpperCase();
    const changed =
      underlyingInstrumentId.value !== "" &&
      underlyingInstrumentId.value !== normalized;
    underlyingInstrumentId.value = normalized;
    market.value = marketValue;
    if (changed) {
      legs.value = [];
      contracts.value = [];
      comboPrice.value = 0;
      priceSource.value = "mid";
      submittedOrderId.value = "";
      undoStack.value = [];
      redoStack.value = [];
      revision.value += 1;
      activeDockTab.value = "positions";
    }
    return changed;
  }

  function setContracts(next: OptionContractChoice[]): void {
    const normalizedContracts = next.map((contract) => {
      const code = contract.code?.trim().toUpperCase() ?? "";
      const instrumentId = normalizedInstrumentId(
        contract.instrumentId ||
          (market.value && code ? `${market.value}.${code}` : code),
      );
      return {
        ...contract,
        instrumentId,
        code,
        name: contract.name || code,
        bidPrice: contract.bidPrice ?? null,
        askPrice: contract.askPrice ?? null,
      };
    });
    if (!sameContractList(contracts.value, normalizedContracts)) {
      contracts.value = normalizedContracts;
    }
    const byInstrument = new Map(
      normalizedContracts.map((contract) => [
        normalizedInstrumentId(contract.instrumentId),
        contract,
      ]),
    );
    let legsChanged = false;
    const nextLegs = legs.value.map((leg) => {
      const current = byInstrument.get(
        normalizedInstrumentId(leg.instrumentId),
      );
      if (current == null || sameContract(leg, current)) return leg;
      legsChanged = true;
      return { ...leg, ...current, side: leg.side, ratio: leg.ratio };
    });
    if (legsChanged) legs.value = nextLegs;
  }

  function updateQuotes(
    quotes: Array<{
      instrumentId: string;
      bidPrice: number | null;
      askPrice: number | null;
    }>,
  ): void {
    const byInstrument = new Map(
      quotes.map((quote) => [
        normalizedInstrumentId(quote.instrumentId),
        quote,
      ]),
    );
    let contractsChanged = false;
    const nextContracts = contracts.value.map((contract) => {
      const quote = byInstrument.get(
        normalizedInstrumentId(contract.instrumentId),
      );
      if (
        quote == null ||
        (contract.bidPrice === quote.bidPrice &&
          contract.askPrice === quote.askPrice)
      ) {
        return contract;
      }
      contractsChanged = true;
      return { ...contract, ...quote };
    });
    if (contractsChanged) contracts.value = nextContracts;

    let legsChanged = false;
    const nextLegs = legs.value.map((leg) => {
      const quote = byInstrument.get(
        normalizedInstrumentId(leg.instrumentId),
      );
      if (
        quote == null ||
        (leg.bidPrice === quote.bidPrice && leg.askPrice === quote.askPrice)
      ) {
        return leg;
      }
      legsChanged = true;
      return { ...leg, ...quote };
    });
    if (legsChanged) legs.value = nextLegs;
  }

  function toggleLeg(
    contract: OptionContractChoice,
    side: OptionComboSide,
  ): void {
    const instrumentId = normalizedInstrumentId(contract.instrumentId);
    const index = legs.value.findIndex(
      (leg) => normalizedInstrumentId(leg.instrumentId) === instrumentId,
    );
    if (index >= 0 && legs.value[index]?.side === side) {
      commit(legs.value.filter((_, legIndex) => legIndex !== index));
      return;
    }
    if (index >= 0) {
      const next = cloneLegs(legs.value);
      next[index] = { ...next[index]!, ...contract, side };
      commit(next);
      activeDockTab.value = "trade";
      return;
    }
    if (legs.value.length >= 8) return;
    commit([...legs.value, { ...contract, side, ratio: 1 }]);
    activeDockTab.value = "trade";
  }

  function addLeg(contract: OptionContractChoice): void {
    if (
      legs.value.length >= 8 ||
      legs.value.some(
        (leg) =>
          normalizedInstrumentId(leg.instrumentId) ===
          normalizedInstrumentId(contract.instrumentId),
      )
    ) {
      return;
    }
    commit([...legs.value, { ...contract, side: "BUY", ratio: 1 }]);
    activeDockTab.value = "trade";
  }

  function removeLeg(instrumentId: string): void {
    const normalized = normalizedInstrumentId(instrumentId);
    const next = legs.value.filter(
      (leg) => normalizedInstrumentId(leg.instrumentId) !== normalized,
    );
    if (next.length !== legs.value.length) commit(next);
  }

  function updateLeg(
    instrumentId: string,
    patch: Partial<Pick<OptionComboLegDraft, "side" | "ratio">>,
  ): void {
    const normalized = normalizedInstrumentId(instrumentId);
    if (
      !legs.value.some(
        (leg) => normalizedInstrumentId(leg.instrumentId) === normalized,
      )
    ) {
      return;
    }
    const next = legs.value.map((leg) =>
      normalizedInstrumentId(leg.instrumentId) === normalized
        ? {
            ...leg,
            ...patch,
            ratio:
              patch.ratio == null
                ? leg.ratio
                : Math.max(1, Math.min(100, Math.round(patch.ratio))),
          }
        : leg,
    );
    commit(next);
  }

  function replaceLegs(next: OptionComboLegDraft[]): void {
    commit(next.slice(0, 8));
    activeDockTab.value = "trade";
  }

  function reverseLegs(): void {
    if (legs.value.length === 0) return;
    commit(
      legs.value.map((leg) => ({
        ...leg,
        side: leg.side === "BUY" ? "SELL" : "BUY",
      })),
    );
  }

  function clearLegs(): void {
    if (legs.value.length === 0) return;
    commit([]);
    comboPrice.value = 0;
    priceSource.value = "mid";
  }

  function undo(): void {
    const previous = undoStack.value.at(-1);
    if (previous == null) return;
    redoStack.value = [
      ...redoStack.value.slice(-(historyLimit - 1)),
      { legs: cloneLegs(legs.value) },
    ];
    undoStack.value = undoStack.value.slice(0, -1);
    commit(previous.legs, false);
  }

  function redo(): void {
    const next = redoStack.value.at(-1);
    if (next == null) return;
    undoStack.value = [
      ...undoStack.value.slice(-(historyLimit - 1)),
      { legs: cloneLegs(legs.value) },
    ];
    redoStack.value = redoStack.value.slice(0, -1);
    commit(next.legs, false);
  }

  return {
    underlyingInstrumentId,
    market,
    workspaceActive,
    activeDockTab,
    contracts,
    legs,
    quantity,
    comboPrice,
    priceSource,
    submittedOrderId,
    revision,
    selectedLegInstrumentIds,
    canUndo,
    canRedo,
    setContext,
    setWorkspaceActive: (active) => {
      workspaceActive.value = active;
    },
    setDockTab: (tab) => {
      activeDockTab.value = tab;
    },
    setContracts,
    updateQuotes,
    toggleLeg,
    addLeg,
    removeLeg,
    updateLeg,
    replaceLegs,
    reverseLegs,
    clearLegs,
    undo,
    redo,
  };
}

export function provideOptionComboDraftStore(): OptionComboDraftStore {
  const store = createOptionComboDraftStore();
  provide(draftKey, store);
  return store;
}

export function useOptionComboDraftStore(): OptionComboDraftStore {
  return inject(draftKey, null) ?? createOptionComboDraftStore();
}
