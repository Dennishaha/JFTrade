import { describe, expect, it } from "vitest";

import type { MarketSecurityDetails } from "../src/contracts";
import {
  isWorkspaceProductTab,
  resolveWorkspaceProductClass,
  workspaceTabsForProduct,
} from "../src/composables/workspaceProductTabs";
import {
  capabilitySurfaceManifest,
  isCapabilitySurfaceID,
} from "../src/features/capabilitySurfaces";

function details(
  value: Partial<MarketSecurityDetails>,
): MarketSecurityDetails {
  return value as MarketSecurityDetails;
}

function tabValues(
  market: string,
  productClass: Parameters<typeof workspaceTabsForProduct>[1],
): string[] {
  return workspaceTabsForProduct(market, productClass).map(
    (tab) => tab.value,
  );
}

describe("workspace product tabs", () => {
  it("uses only registered capability surface IDs", () => {
    for (const productClass of [
      "equity",
      "fund",
      "option",
      "warrant",
      "cbbc",
      "future",
      "event_contract",
      "index",
      "bond",
      "plate",
      "unknown",
    ] as const) {
      for (const tab of workspaceTabsForProduct("HK", productClass)) {
        expect(isCapabilitySurfaceID(tab.surfaceId)).toBe(true);
        expect(capabilitySurfaceManifest[tab.surfaceId].route).toBe("/workspace");
      }
    }
  });

  it("uses the dedicated prediction-contract surface", () => {
    expect(
      workspaceTabsForProduct("US", "event_contract").map((tab) => tab.value),
    ).toEqual(["contract", "depth", "chart", "ticks", "rules"]);
    expect(isWorkspaceProductTab("contract")).toBe(true);
    expect(isWorkspaceProductTab("rules")).toBe(true);
  });
  it("prefers the broker-neutral product identity", () => {
    expect(
      resolveWorkspaceProductClass(
        details({
          productClass: "future",
          securityType: "Eqty",
          equity: {} as MarketSecurityDetails["equity"],
        }),
      ),
    ).toBe("future");
    expect(
      resolveWorkspaceProductClass(
        details({
          productClass: "cbbc",
          securityType: "Warrant",
          warrant: {
            warrantType: "Bull",
          } as MarketSecurityDetails["warrant"],
        }),
      ),
    ).toBe("cbbc");
  });

  it("keeps legacy broker security types as a compatibility fallback", () => {
    const cases = [
      ["SecurityType_Eqty", "equity"],
      ["Trust", "fund"],
      ["Drvt", "option"],
      ["Warrant", "warrant"],
      ["Future", "future"],
      ["Index", "index"],
      ["Bond", "bond"],
      ["PlateSet", "plate"],
      ["EventContract", "event_contract"],
      ["", "unknown"],
    ] as const;
    for (const [securityType, expected] of cases) {
      expect(resolveWorkspaceProductClass(null, securityType)).toBe(expected);
    }

    expect(
      resolveWorkspaceProductClass(
        details({
          productClass: "unknown",
          warrant: {
            warrantType: "Bear",
          } as MarketSecurityDetails["warrant"],
        }),
      ),
    ).toBe("cbbc");

    const detailBlocks = [
      ["future", "future"],
      ["option", "option"],
      ["trust", "fund"],
      ["index", "index"],
      ["plate", "plate"],
      ["equity", "equity"],
    ] as const;
    for (const [block, expected] of detailBlocks) {
      expect(
        resolveWorkspaceProductClass(
          details({
            productClass: "unknown",
            [block]: {},
          }),
        ),
      ).toBe(expected);
    }
  });

  it("shows warrants only for eligible Hong Kong underlyings", () => {
    for (const productClass of ["equity", "fund", "index"] as const) {
      expect(tabValues("HK", productClass)).toContain("warrants");
      expect(
        workspaceTabsForProduct("HK", productClass).find(
          (tab) => tab.value === "warrants",
        )?.label,
      ).toBe("轮证");
      expect(tabValues("US", productClass)).not.toContain("warrants");
      expect(tabValues("SH", productClass)).not.toContain("warrants");
      expect(tabValues("SZ", productClass)).not.toContain("warrants");
    }
    for (const productClass of [
      "option",
      "warrant",
      "cbbc",
      "future",
      "event_contract",
    ] as const) {
      expect(tabValues("HK", productClass)).not.toContain("warrants");
    }
  });

  it("treats futures and unresolved products as generic instruments", () => {
    expect(tabValues("HK", "future")).toEqual(["chart", "news"]);
    expect(tabValues("US", "future")).toEqual(["chart", "news"]);
    expect(tabValues("HK", "unknown")).toEqual(["chart", "news"]);
    expect(isWorkspaceProductTab("futures")).toBe(false);
    expect(isWorkspaceProductTab("warrants")).toBe(true);
  });
});
