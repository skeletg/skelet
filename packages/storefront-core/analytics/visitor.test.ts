import { afterEach, describe, expect, it, vi } from "vitest";

import { AID_COOKIE, ensureAnonId, fnv1a, sampleBucket, viewportBucket } from "./visitor";

afterEach(() => {
  vi.unstubAllGlobals();
});

// The hex-string FNV-1a LensTracker and LensReplay each carried before the helpers were shared - kept here so the
// shared numeric version is pinned to the same buckets and the same layout hashes
function legacyFnv1aHex(s: string): string {
  let h = 2166136261;
  for (let i = 0; i < s.length; i++) {
    h ^= s.charCodeAt(i);
    h = Math.imul(h, 16777619);
  }
  return (h >>> 0).toString(16);
}

describe("fnv1a", () => {
  it("matches the published 32-bit FNV-1a vectors the Go backend also produces", () => {
    expect(fnv1a("")).toBe(0x811c9dc5);
    expect(fnv1a("a")).toBe(0xe40c292c);
    expect(fnv1a("foobar")).toBe(0xbf9cf968);
  });

  it("gives the same layout hash and sample bucket as the old per-component copies", () => {
    for (const s of ["", "a", "11111111-2222-3333-4444-555555555555", "<div>layout</div>", "ümlaut"]) {
      expect(fnv1a(s).toString(16)).toBe(legacyFnv1aHex(s));
      expect(sampleBucket(s)).toBe(parseInt(legacyFnv1aHex(s), 16) % 100);
    }
  });
});

describe("sampleBucket", () => {
  it("is deterministic and always 0-99", () => {
    for (let i = 0; i < 500; i++) {
      const b = sampleBucket(`visitor-${i}`);
      expect(b).toBeGreaterThanOrEqual(0);
      expect(b).toBeLessThan(100);
      expect(sampleBucket(`visitor-${i}`)).toBe(b);
    }
  });
});

describe("ensureAnonId", () => {
  it("returns empty on the server", () => {
    expect(ensureAnonId()).toBe("");
  });

  it("keeps an existing id", () => {
    vi.stubGlobal("document", { cookie: `${AID_COOKIE}=existing-id` });
    expect(ensureAnonId()).toBe("existing-id");
  });

  it("mints and stores an id on first visit, then reuses it", () => {
    // document.cookie is an accumulating setter in a browser - model just enough of that
    const jar = new Map<string, string>();
    vi.stubGlobal("document", {
      get cookie() {
        return [...jar].map(([k, v]) => `${k}=${v}`).join("; ");
      },
      set cookie(raw: string) {
        const [pair] = raw.split(";");
        const [k, v] = pair.split("=");
        jar.set(k, v);
      },
    });
    vi.stubGlobal("crypto", { randomUUID: () => "11111111-2222-3333-4444-555555555555" });

    const first = ensureAnonId();
    expect(first).toBe("11111111-2222-3333-4444-555555555555");
    expect(jar.get(AID_COOKIE)).toBe(first);

    vi.stubGlobal("crypto", { randomUUID: () => "should-not-be-used" });
    expect(ensureAnonId()).toBe(first);
  });
});

describe("viewportBucket", () => {
  it.each([
    [375, "mobile"],
    [767, "mobile"],
    [768, "tablet"],
    [1023, "tablet"],
    [1024, "desktop"],
    [1920, "desktop"],
  ])("puts %ipx in %s", (width, bucket) => {
    vi.stubGlobal("window", { innerWidth: width });
    expect(viewportBucket()).toBe(bucket);
  });
});
