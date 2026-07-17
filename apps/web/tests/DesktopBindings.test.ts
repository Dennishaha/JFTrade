import { describe, expect, it, vi } from "vitest";

const callByID = vi.hoisted(() => vi.fn());

vi.mock("@wailsio/runtime", () => ({
  Call: { ByID: callByID },
}));

import { fontAwesomeIcons } from "../src/fontAwesomeIcons";
import { OpenLink } from "../src/wails/github.com/jftrade/jftrade-main/cmd/jftrade-desktop/desktoplinkservice";
import {
  ListDays,
  OpenFolder,
  ReadPage,
} from "../src/wails/github.com/jftrade/jftrade-main/cmd/jftrade-desktop/desktoplogservice";
import { Check } from "../src/wails/github.com/jftrade/jftrade-main/cmd/jftrade-desktop/desktopupdateservice";

describe("desktop bindings and icon registration", () => {
  it("keeps Vuetify aliases mapped to the bundled Font Awesome set", () => {
    expect(fontAwesomeIcons.defaultSet).toBe("fa");
    expect(fontAwesomeIcons.aliases.close).toContain("fa-xmark");
    expect(fontAwesomeIcons.aliases.command).toContain("fa-keyboard");
    expect(fontAwesomeIcons.sets.fa).toBeTruthy();
  });

  it("forwards generated desktop service calls to their stable Wails IDs", () => {
    const link = OpenLink("https://jftrade.example/docs");
    const days = ListDays();
    const folder = OpenFolder();
    const page = ReadPage("2026-07-16", "WARN", "disconnect", 20, 50);
    const update = Check();

    expect(callByID).toHaveBeenNthCalledWith(1, 1544498133, "https://jftrade.example/docs");
    expect(callByID).toHaveBeenNthCalledWith(2, 3050303072);
    expect(callByID).toHaveBeenNthCalledWith(3, 3240492663);
    expect(callByID).toHaveBeenNthCalledWith(4, 4222000680, "2026-07-16", "WARN", "disconnect", 20, 50);
    expect(callByID).toHaveBeenNthCalledWith(5, 46483848);
    expect(link).toBeUndefined();
    expect(days).toBeUndefined();
    expect(folder).toBeUndefined();
    expect(page).toBeUndefined();
    expect(update).toBeUndefined();
  });
});
