import { describe, expect, it } from "vitest";

import {
  createOptionComboDraftStore,
  type OptionComboLegDraft,
  type OptionContractChoice,
} from "../src/composables/optionComboDraft";
import {
  buildOptionComboTemplate,
  optionComboLocalQuote,
  optionComboSpread,
  optionComboValidationMessage,
  recognizeOptionComboStrategy,
} from "../src/composables/optionComboStrategies";

function contract(
  code: string,
  optionType: "call" | "put",
  strike: number,
  expiry = "2026-07-24",
): OptionContractChoice {
  return {
    instrumentId: `US.${code}`,
    code,
    name: code,
    label: `${expiry} ${optionType} ${strike}`,
    optionType,
    strike,
    multiplier: 100,
    expiry,
    bidPrice: 1,
    askPrice: 1.2,
  };
}

function leg(
  value: OptionContractChoice,
  side: "BUY" | "SELL",
  ratio = 1,
): OptionComboLegDraft {
  return { ...value, side, ratio };
}

const contracts = [
  contract("C90", "call", 90),
  contract("P90", "put", 90),
  contract("C100", "call", 100),
  contract("P100", "put", 100),
  contract("C110", "call", 110),
  contract("P110", "put", 110),
  contract("C100-AUG", "call", 100, "2026-08-21"),
];

describe("option combo strategies", () => {
  it("recognizes all supported strategies with direction and ratio semantics", () => {
    expect(
      recognizeOptionComboStrategy([
        leg(contracts[0]!, "BUY"),
        leg(contracts[2]!, "SELL"),
      ]),
    ).toBe("vertical");
    expect(
      recognizeOptionComboStrategy([
        leg(contracts[2]!, "BUY"),
        leg(contracts[3]!, "BUY"),
      ]),
    ).toBe("straddle");
    expect(
      recognizeOptionComboStrategy([
        leg(contracts[1]!, "BUY"),
        leg(contracts[4]!, "BUY"),
      ]),
    ).toBe("strangle");
    expect(
      recognizeOptionComboStrategy([
        leg(contracts[2]!, "SELL"),
        leg(contracts[6]!, "BUY"),
      ]),
    ).toBe("calendar");
    const butterfly = [
      leg(contracts[0]!, "BUY"),
      leg(contracts[2]!, "SELL", 2),
      leg(contracts[4]!, "BUY"),
    ];
    expect(recognizeOptionComboStrategy(butterfly)).toBe("butterfly");
    expect(optionComboSpread("butterfly", butterfly)).toBe(10);
  });

  it("rejects unsupported custom shapes instead of sending custom strategy", () => {
    expect(optionComboValidationMessage([])).toContain("至少");
    expect(
      optionComboValidationMessage([
        leg(contracts[0]!, "BUY"),
        leg(contracts[2]!, "BUY"),
      ]),
    ).toContain("不属于受支持");
    expect(
      recognizeOptionComboStrategy([
        leg(contracts[0]!, "BUY"),
        leg(contracts[2]!, "SELL", 2),
      ]),
    ).toBeNull();
  });

  it("builds five legal templates from actual contracts", () => {
    for (const strategy of [
      "vertical",
      "straddle",
      "strangle",
      "calendar",
      "butterfly",
    ] as const) {
      const generated = buildOptionComboTemplate(
        strategy,
        contracts,
        "2026-07-24",
        100,
      );
      expect(generated.length).toBeGreaterThanOrEqual(2);
      expect(recognizeOptionComboStrategy(generated)).toBe(strategy);
    }
    expect(
      buildOptionComboTemplate("calendar", contracts.slice(0, 6), "", 100),
    ).toEqual([]);
  });

  it("computes local anchors and keeps a 20-step in-memory undo history", () => {
    const quote = optionComboLocalQuote([
      leg({ ...contracts[0]!, bidPrice: 2, askPrice: 2.2 }, "BUY"),
      leg({ ...contracts[2]!, bidPrice: 1, askPrice: 1.2 }, "SELL"),
    ]);
    expect(quote).toEqual({ bid: 0.8, mid: 1, ask: 1.2 });

    const draft = createOptionComboDraftStore();
    draft.setContext("US.BABA", "US");
    draft.setContracts(contracts);
    draft.toggleLeg(contracts[0]!, "BUY");
    draft.toggleLeg(contracts[2]!, "SELL");
    expect(draft.legs.value).toHaveLength(2);
    draft.toggleLeg(contracts[0]!, "SELL");
    expect(draft.legs.value[0]?.side).toBe("SELL");
    draft.toggleLeg(contracts[0]!, "SELL");
    expect(draft.legs.value).toHaveLength(1);
    draft.undo();
    expect(draft.legs.value).toHaveLength(2);
    draft.redo();
    expect(draft.legs.value).toHaveLength(1);
    expect(draft.setContext("US.AAPL", "US")).toBe(true);
    expect(draft.legs.value).toEqual([]);
  });

  it("handles quote refresh, editing, limits, reversal, and empty history safely", () => {
    const draft = createOptionComboDraftStore();
    draft.undo();
    draft.redo();
    draft.reverseLegs();
    draft.clearLegs();
    draft.setWorkspaceActive(true);
    draft.setDockTab("trade");
    expect(draft.workspaceActive.value).toBe(true);
    expect(draft.activeDockTab.value).toBe("trade");

    draft.setContext("US.BABA", "US");
    draft.setContracts(contracts);
    draft.addLeg(contracts[0]!);
    draft.addLeg(contracts[0]!);
    expect(draft.legs.value).toHaveLength(1);
    expect(draft.selectedLegInstrumentIds.value).toEqual(["US.C90"]);

    draft.updateQuotes([
      { instrumentId: "US.C90", bidPrice: 2.2, askPrice: 2.4 },
      { instrumentId: "US.MISSING", bidPrice: 1, askPrice: 1.1 },
    ]);
    expect(draft.legs.value[0]).toMatchObject({
      bidPrice: 2.2,
      askPrice: 2.4,
    });
    draft.setContracts([
      { ...contracts[0]!, name: "updated", bidPrice: 3, askPrice: 3.2 },
      ...contracts.slice(1),
    ]);
    expect(draft.legs.value[0]).toMatchObject({
      name: "updated",
      side: "BUY",
      ratio: 1,
    });

    const revision = draft.revision.value;
    draft.updateLeg("US.MISSING", { ratio: 5 });
    draft.removeLeg("US.MISSING");
    expect(draft.revision.value).toBe(revision);
    draft.updateLeg("US.C90", { ratio: 200, side: "SELL" });
    expect(draft.legs.value[0]).toMatchObject({ ratio: 100, side: "SELL" });
    draft.updateLeg("US.C90", { ratio: 0 });
    expect(draft.legs.value[0]?.ratio).toBe(1);
    draft.reverseLegs();
    expect(draft.legs.value[0]?.side).toBe("BUY");

    const capacityContracts = Array.from({ length: 9 }, (_, index) =>
      contract(`CAP-${index}`, "call", 120 + index),
    );
    draft.replaceLegs(
      capacityContracts.slice(0, 8).map((item) => leg(item, "BUY")),
    );
    draft.addLeg(capacityContracts[8]!);
    draft.toggleLeg(capacityContracts[8]!, "SELL");
    expect(draft.legs.value).toHaveLength(8);
    draft.removeLeg(capacityContracts[0]!.instrumentId);
    expect(draft.legs.value).toHaveLength(7);
    draft.clearLegs();
    expect(draft.legs.value).toEqual([]);
    expect(draft.comboPrice.value).toBe(0);
    expect(draft.priceSource.value).toBe("mid");
  });

  it("keeps shared contract and leg references stable for equivalent refreshes", () => {
    const draft = createOptionComboDraftStore();
    draft.setContext("US.BABA", "US");
    draft.setContracts(contracts);
    draft.addLeg(contracts[0]!);

    const contractReference = draft.contracts.value;
    const legReference = draft.legs.value;
    draft.setContracts(contracts.map((item) => ({ ...item })));
    draft.updateQuotes([
      {
        instrumentId: contracts[0]!.instrumentId,
        bidPrice: contracts[0]!.bidPrice,
        askPrice: contracts[0]!.askPrice,
      },
    ]);

    expect(draft.contracts.value).toBe(contractReference);
    expect(draft.legs.value).toBe(legReference);

    draft.updateQuotes([
      {
        instrumentId: contracts[0]!.instrumentId,
        bidPrice: 1.1,
        askPrice: 1.3,
      },
    ]);
    expect(draft.contracts.value).not.toBe(contractReference);
    expect(draft.legs.value).not.toBe(legReference);
    expect(draft.legs.value[0]).toMatchObject({
      bidPrice: 1.1,
      askPrice: 1.3,
    });
  });

  it("covers template boundary failures and absent quotes", () => {
    expect(buildOptionComboTemplate("vertical", [], "", null)).toEqual([]);
    expect(
      buildOptionComboTemplate("vertical", [contracts[2]!], "", null),
    ).toEqual([]);
    expect(
      buildOptionComboTemplate("straddle", [contracts[2]!], "", 100),
    ).toEqual([]);
    expect(
      buildOptionComboTemplate(
        "strangle",
        [contracts[2]!, contracts[3]!],
        "",
        100,
      ),
    ).toEqual([]);
    expect(
      buildOptionComboTemplate(
        "butterfly",
        [contracts[0]!, contracts[2]!],
        "",
        90,
      ),
    ).toEqual([]);
    expect(
      optionComboLocalQuote([
        leg({ ...contracts[0]!, askPrice: null }, "BUY"),
      ]),
    ).toEqual({ bid: null, mid: null, ask: null });
    expect(optionComboSpread("vertical", [leg(contracts[0]!, "BUY")])).toBe(
      undefined,
    );
    expect(optionComboSpread("calendar", [])).toBe(undefined);
  });
});
