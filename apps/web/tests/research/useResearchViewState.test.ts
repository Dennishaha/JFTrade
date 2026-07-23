// @vitest-environment jsdom

import { afterEach, describe, expect, it } from "vitest";

import {
  clampResearchPaneSizesForWidth,
  RESEARCH_VIEW_STORAGE_KEY,
  readResearchViewState,
  researchPaneBoundsForWidth,
  writeResearchViewState,
} from "../../src/composables/useResearchViewState";

afterEach(() => {
  window.localStorage.clear();
  window.sessionStorage.clear();
});

describe("research view state", () => {
  it("persists the validated rail layout to session and local storage", () => {
    writeResearchViewState({
      railCollapsed: true,
      paneSizes: [68, 32],
    });

    expect(readResearchViewState()).toEqual({
      railCollapsed: true,
      paneSizes: [68, 32],
    });
    expect(
      JSON.parse(
        window.localStorage.getItem(RESEARCH_VIEW_STORAGE_KEY) ?? "{}",
      ),
    ).toEqual({
      railCollapsed: true,
      paneSizes: [68, 32],
    });
  });

  it("rejects malformed or out-of-range pane sizes", () => {
    window.sessionStorage.setItem(
      RESEARCH_VIEW_STORAGE_KEY,
      JSON.stringify({
        railCollapsed: true,
        paneSizes: [90, 10],
      }),
    );

    expect(readResearchViewState()).toEqual({
      railCollapsed: true,
      paneSizes: [72, 28],
    });
  });

  it("converts the 360-520px rail limits into container-relative pane bounds", () => {
    const paneWidth = 1_376 - 6;
    expect(researchPaneBoundsForWidth(1_376)).toEqual({
      leftMinSize: 100 - (520 / paneWidth) * 100,
      leftMaxSize: 100 - (360 / paneWidth) * 100,
      railMinSize: (360 / paneWidth) * 100,
      railMaxSize: (520 / paneWidth) * 100,
    });

    expect(clampResearchPaneSizesForWidth([78, 22], 1_376)).toEqual([
      100 - (360 / paneWidth) * 100,
      (360 / paneWidth) * 100,
    ]);
    expect(clampResearchPaneSizesForWidth([45, 55], 1_376)).toEqual([
      100 - (520 / paneWidth) * 100,
      (520 / paneWidth) * 100,
    ]);
  });
});
