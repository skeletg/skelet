import { afterEach, describe, expect, it, vi } from "vitest";

import { randomId } from "@/app/lib/random-id";

// The whole reason this exists: over a LAN ip the page is not a secure context, crypto is present
// but crypto.randomUUID is not, and the bare call throws - taking Lens tracking and Prism
// assignment down with it

afterEach(() => {
  vi.unstubAllGlobals();
});

describe("randomId", () => {
  it("uses the platform uuid when there is one", () => {
    vi.stubGlobal("crypto", { randomUUID: () => "11111111-2222-3333-4444-555555555555" });
    expect(randomId()).toBe("11111111-2222-3333-4444-555555555555");
  });

  it("falls back when crypto is there but randomUUID is not", () => {
    vi.stubGlobal("crypto", {});
    expect(randomId()).toMatch(/^id_[a-z0-9]+$/);
  });

  it("falls back when there is no crypto at all", () => {
    vi.stubGlobal("crypto", undefined);
    expect(randomId()).toMatch(/^id_[a-z0-9]+$/);
  });

  it("does not repeat itself", () => {
    vi.stubGlobal("crypto", {});
    expect(new Set(Array.from({ length: 200 }, randomId)).size).toBe(200);
  });
});
