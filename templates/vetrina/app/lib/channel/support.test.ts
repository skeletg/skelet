import { describe, expect, it } from "vitest";

import { supportEmailFrom } from "@/app/lib/channel/support";

describe("supportEmailFrom", () => {
  it("prefers an explicit override", () => {
    expect(supportEmailFrom("https://shop.example", " help@vetrina.test ")).toBe(
      "help@vetrina.test",
    );
  });

  it("derives from the channel public url", () => {
    expect(supportEmailFrom("https://www.vetrina.test/")).toBe(
      "support@vetrina.test",
    );
    expect(supportEmailFrom("https://shop.vetrina.test")).toBe(
      "support@shop.vetrina.test",
    );
  });

  it("returns null when there is nothing to derive from", () => {
    expect(supportEmailFrom(null)).toBeNull();
    expect(supportEmailFrom("")).toBeNull();
    expect(supportEmailFrom("not a url")).toBeNull();
    expect(supportEmailFrom(undefined, "   ")).toBeNull();
  });
});
