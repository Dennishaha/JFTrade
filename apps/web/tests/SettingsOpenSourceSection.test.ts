// @vitest-environment jsdom

import { mount } from "@vue/test-utils";
import { beforeEach, describe, expect, it, vi } from "vitest";

const externalLinks = vi.hoisted(() => ({
  handleExternalLinkClick: vi.fn(),
}));

vi.mock("../src/composables/externalLink", () => ({
  useExternalLink: () => externalLinks,
}));

import SettingsOpenSourceSection from "../src/components/SettingsOpenSourceSection.vue";
import {
  JFTRADE_SOURCE_REPOSITORY_URL,
  resolveCorrespondingSourceUrl,
} from "../src/features/openSourceLicense";

const releaseBuild = {
  version: "1.2.3",
  commit: "0123456789abcdef0123456789abcdef01234567",
  buildTime: "2026-07-18T00:00:00Z",
  goos: "darwin",
  goarch: "arm64",
};

describe("SettingsOpenSourceSection", () => {
  beforeEach(() => {
    externalLinks.handleExternalLinkClick.mockClear();
  });

  it("shows the license, warranty disclaimer, notices, and exact source", async () => {
    const wrapper = mount(SettingsOpenSourceSection, {
      props: { build: releaseBuild },
    });

    expect(wrapper.text()).toContain("Copyright (C) 2026 JFTrade Contributors");
    expect(wrapper.text()).toContain("AGPL-3.0-only");
    expect(wrapper.text()).toContain("不提供任何明示或默示担保");
    expect(wrapper.get("[data-testid='open-source-license-link']").attributes("href"))
      .toBe("/docs/legal/license");
    expect(wrapper.get("[data-testid='third-party-notices-link']").attributes("href"))
      .toBe("/docs/legal/third-party-notices");
    expect(wrapper.get("[data-testid='corresponding-source-link']").attributes("href"))
      .toBe(JFTRADE_SOURCE_REPOSITORY_URL + "/tree/" + releaseBuild.commit);
    expect(wrapper.get("[data-testid='open-source-build-label']").text())
      .toContain("1.2.3 · 0123456789ab");

    const licenseLink = wrapper.get("[data-testid='open-source-license-link']");
    const noticesLink = wrapper.get("[data-testid='third-party-notices-link']");
    const sourceLink = wrapper.get("[data-testid='corresponding-source-link']");
    await licenseLink.trigger("click");
    await noticesLink.trigger("click");
    await sourceLink.trigger("click");
    expect(externalLinks.handleExternalLinkClick).toHaveBeenNthCalledWith(
      1,
      expect.any(MouseEvent),
      "/docs/legal/license",
    );
    expect(externalLinks.handleExternalLinkClick).toHaveBeenNthCalledWith(
      2,
      expect.any(MouseEvent),
      "/docs/legal/third-party-notices",
    );
    expect(externalLinks.handleExternalLinkClick).toHaveBeenNthCalledWith(
      3,
      expect.any(MouseEvent),
      JFTRADE_SOURCE_REPOSITORY_URL + "/tree/" + releaseBuild.commit,
    );
  });

  it("falls back to the repository for development and invalid commits", () => {
    expect(resolveCorrespondingSourceUrl(undefined))
      .toBe(JFTRADE_SOURCE_REPOSITORY_URL);
    expect(resolveCorrespondingSourceUrl({
      ...releaseBuild,
      version: "dev",
      commit: "unknown",
    })).toBe(JFTRADE_SOURCE_REPOSITORY_URL);

    const wrapper = mount(SettingsOpenSourceSection, {
      props: {
        build: { ...releaseBuild, version: "dev", commit: "unknown" },
      },
    });
    expect(wrapper.get("[data-testid='corresponding-source-link']").attributes("href"))
      .toBe(JFTRADE_SOURCE_REPOSITORY_URL);
  });
});
