import { afterEach, describe, expect, it, vi } from "vitest";

import { readCookie } from "./cookies";

afterEach(() => {
  vi.unstubAllGlobals();
});

describe("readCookie", () => {
  it("returns empty on the server, where there is no document", () => {
    expect(readCookie("lens_aid")).toBe("");
  });

  it("reads a cookie by exact name and decodes it", () => {
    vi.stubGlobal("document", { cookie: "a=1; lens_aid=abc%20def; b=2" });
    expect(readCookie("lens_aid")).toBe("abc def");
    expect(readCookie("a")).toBe("1");
  });

  it("does not match a cookie whose name merely ends with the one asked for", () => {
    vi.stubGlobal("document", { cookie: "xlens_aid=nope" });
    expect(readCookie("lens_aid")).toBe("");
  });

  it("returns empty for a cookie that is not set", () => {
    vi.stubGlobal("document", { cookie: "a=1" });
    expect(readCookie("missing")).toBe("");
  });
});
