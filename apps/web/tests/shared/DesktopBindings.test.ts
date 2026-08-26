import { describe, expect, it } from "vitest";

import { fontAwesomeIcons } from "../../src/fontAwesomeIcons";

describe("desktop icons registration", () => {
  it("keeps Vuetify aliases mapped to the bundled Font Awesome set", () => {
    expect(fontAwesomeIcons.defaultSet).toBe("fa");
    expect(fontAwesomeIcons.aliases.close).toContain("fa-xmark");
    expect(fontAwesomeIcons.aliases.command).toContain("fa-keyboard");
    expect(fontAwesomeIcons.sets.fa).toBeTruthy();
  });
});
